package d2client

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2localclient"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2remoteclient"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket/d2netpackettype"
	"github.com/OpenDiablo2/OpenDiablo2/d2script"
)

const logPrefix = "Game Client"

const (
	numSubtilesPerTile = 5
)

// GameClient manages a connection to d2server.GameServer
// and keeps a synchronized copy of the map and entities.
type GameClient struct {
	clientConnection ServerConnection                            // Abstract local/remote connection
	connectionType   d2clientconnectiontype.ClientConnectionType // Type of connection (local or remote)
	asset            *d2asset.AssetManager
	scriptEngine     *d2script.ScriptEngine
	GameState        *d2hero.HeroState              // local player state
	MapEngine        *d2mapengine.MapEngine         // Map and entities
	mapGen           *d2mapgen.MapGenerator         // map generator
	PlayerID         string                         // ID of the local player
	Players          map[string]*d2mapentity.Player // IDs of the other players
	Seed             int64                          // Map seed
	RegenMap         bool                           // Regenerate tile cache on render (map has changed)

	// mapMismatch is why the world this client built is not the host's
	// (d2mapgen.GenerateHostWorld), or nil. Written by the packet goroutine.
	mapMismatch   error
	mapMismatchMu sync.Mutex

	// joinRefused is why the host refused this client's join (the
	// JoinRefused packet: it launched the other game), or "". Written by the
	// packet goroutine, read by the game screen, which goes back to the main
	// menu with it (JoinRefused).
	joinRefused   string
	joinRefusedMu sync.Mutex

	// castsDropped counts the casts of a skill this game has no record of
	// (handleCastSkillPacket); the first is logged.
	castsDropped atomic.Int64

	// SaveFilePath is the hero save this client opened. Strigoi keeps its own
	// per-hero data (the kit, T2) in a sidecar beside it, so it must know which
	// save is in play. Empty for a remote client.
	SaveFilePath string

	*d2util.Logger
}

// Create constructs a new GameClient and returns a pointer to it.
func Create(connectionType d2clientconnectiontype.ClientConnectionType,
	asset *d2asset.AssetManager,
	l d2util.LogLevel,
	scriptEngine *d2script.ScriptEngine) (*GameClient, error) {
	result := &GameClient{
		asset:          asset,
		MapEngine:      d2mapengine.CreateMapEngine(l, asset),
		Players:        make(map[string]*d2mapentity.Player),
		connectionType: connectionType,
		scriptEngine:   scriptEngine,
	}

	result.Logger = d2util.NewLogger()
	result.Logger.SetPrefix(logPrefix)
	result.Logger.SetLevel(l)

	// for a remote client connection, set loading to true - wait until we process the GenerateMapPacket
	// before we start updating map entites
	result.MapEngine.IsLoading = connectionType == d2clientconnectiontype.LANClient

	mapGen, err := d2mapgen.NewMapGenerator(asset, l, result.MapEngine)
	if err != nil {
		return nil, err
	}

	result.mapGen = mapGen

	switch connectionType {
	case d2clientconnectiontype.LANClient:
		result.clientConnection, err = d2remoteclient.Create(l, asset)
	case d2clientconnectiontype.LANServer:
		result.clientConnection, err = d2localclient.Create(asset, l, true)
	case d2clientconnectiontype.Local:
		result.clientConnection, err = d2localclient.Create(asset, l, false)
	default:
		err = fmt.Errorf("unknown client connection type specified: %d", connectionType)
	}

	if err != nil {
		return nil, err
	}

	result.clientConnection.SetClientListener(result)

	return result, nil
}

// Open creates the server and connects to it if the client is local.
// If the client is remote it sends a PlayerConnectionRequestPacket to the
// server (see d2netpacket).
func (g *GameClient) Open(connectionString, saveFilePath string) error {
	g.SaveFilePath = saveFilePath

	switch g.connectionType {
	case d2clientconnectiontype.LANServer, d2clientconnectiontype.Local:
		g.scriptEngine.AllowEval()
	}

	return g.clientConnection.Open(connectionString, saveFilePath)
}

// Close destroys the server if the client is local. For remote clients
// it sends a DisconnectRequestPacket (see d2netpacket).
func (g *GameClient) Close() error {
	switch g.connectionType {
	case d2clientconnectiontype.LANServer, d2clientconnectiontype.Local:
		g.scriptEngine.DisallowEval()
	}

	return g.clientConnection.Close()
}

// Destroy does the same thing as Close.
func (g *GameClient) Destroy() error {
	return g.Close()
}

// OnPacketReceived is called by the ClientConection and processes incoming
// packets.
// nolint:gocyclo // switch statement on packet type makes sense, no need to change
func (g *GameClient) OnPacketReceived(packet d2netpacket.NetPacket) error {
	switch packet.PacketType {
	case d2netpackettype.GenerateMap:
		if err := g.handleGenerateMapPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.UpdateServerInfo:
		if err := g.handleUpdateServerInfoPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.AddPlayer:
		if err := g.handleAddPlayerPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.MovePlayer:
		if err := g.handleMovePlayerPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.CastSkill:
		if err := g.handleCastSkillPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.SpawnItem:
		if err := g.handleSpawnItemPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.Ping:
		if err := g.handlePingPacket(); err != nil {
			g.Errorf("GameClient: error responding to server ping: %s", err)
		}
	case d2netpackettype.PlayerDisconnectionNotification:
		if err := g.handlePlayerDisconnectionPacket(packet); err != nil {
			return err
		}
	case d2netpackettype.ServerClosed:
		// https://github.com/OpenDiablo2/OpenDiablo2/issues/802
		g.Infof("Server has been closed")
		os.Exit(0)
	case d2netpackettype.ServerFull:
		g.Infof("Server is full") // need to be verified
		os.Exit(0)
	case d2netpackettype.JoinRefused:
		if err := g.handleJoinRefusedPacket(packet); err != nil {
			return err
		}
	default:
		g.Fatalf("Invalid packet type: %d", packet.PacketType)
	}

	return nil
}

// SendPacketToServer calls server.OnPacketReceived if the client is local.
// If it is remote the NetPacket sent over a UDP connection to the server.
func (g *GameClient) SendPacketToServer(packet d2netpacket.NetPacket) error {
	return g.clientConnection.SendPacketToServer(packet)
}

func (g *GameClient) handleGenerateMapPacket(packet d2netpacket.NetPacket) error {
	mapData, err := d2netpacket.UnmarshalGenerateMap(packet.PacketData)
	if err != nil {
		return err
	}

	if mapData.RegionType == d2enum.RegionAct1Town {
		// The host's world, not this process's own setting: see
		// d2mapgen.GenerateHostWorld. A client whose copy differs is told
		// loudly and keeps the reason for whoever asks (MapMismatch).
		mismatch := g.mapGen.GenerateHostWorld(mapData.Map, mapData.MapSHA256)
		if mismatch != nil {
			g.Errorf("THIS IS NOT THE HOST'S WORLD: %v", mismatch)
		}

		g.mapMismatchMu.Lock()
		g.mapMismatch = mismatch
		g.mapMismatchMu.Unlock()
	}

	g.RegenMap = true

	return nil
}

func (g *GameClient) handleUpdateServerInfoPacket(packet d2netpacket.NetPacket) error {
	serverInfo, err := d2netpacket.UnmarshalUpdateServerInfo(packet.PacketData)
	if err != nil {
		return err
	}

	g.MapEngine.SetSeed(serverInfo.Seed)
	g.PlayerID = serverInfo.PlayerID
	g.Seed = serverInfo.Seed
	g.Infof("Player id set to %s", serverInfo.PlayerID)

	return nil
}

func (g *GameClient) handleAddPlayerPacket(packet d2netpacket.NetPacket) error {
	player, err := d2netpacket.UnmarshalAddPlayer(packet.PacketData)
	if err != nil {
		return err
	}

	d2hero.HydrateSkills(player.Skills, g.asset)

	newPlayer := g.MapEngine.NewPlayer(player.ID, player.Name, player.X, player.Y, 0,
		player.HeroType, player.Stats, player.Skills, &player.Equipment, player.LeftSkill, player.RightSkill, player.Gold)

	g.Players[newPlayer.ID()] = newPlayer
	g.MapEngine.AddEntity(newPlayer)

	return nil
}

func (g *GameClient) handleSpawnItemPacket(packet d2netpacket.NetPacket) error {
	item, err := d2netpacket.UnmarshalSpawnItem(packet.PacketData)
	if err != nil {
		return err
	}

	itemEntity, err := g.MapEngine.NewItem(item.X, item.Y, item.Codes...)

	if err == nil {
		g.MapEngine.AddEntity(itemEntity)
	}

	return err
}

func (g *GameClient) handleMovePlayerPacket(packet d2netpacket.NetPacket) error {
	movePlayer, err := d2netpacket.UnmarshalMovePlayer(packet.PacketData)
	if err != nil {
		return err
	}

	player := g.Players[movePlayer.PlayerID]
	start := d2vector.NewPositionTile(movePlayer.StartX, movePlayer.StartY)
	dest := d2vector.NewPositionTile(movePlayer.DestX, movePlayer.DestY)
	path := g.MapEngine.PathFind(start, dest)

	if len(path) > 0 {
		player.SetPath(path, func() {
			tilePosition := player.Position.Tile()
			tile := g.MapEngine.TileAt(int(tilePosition.X()), int(tilePosition.Y()))

			if tile == nil {
				return
			}

			player.SetIsInTown(tile.RegionType == d2enum.RegionAct1Town)

			err := player.SetAnimationMode(player.GetAnimationMode())

			if err != nil {
				fmtStr := "GameClient: error setting animation mode for player %s: %s"
				g.Errorf(fmtStr, player.ID(), err)
			}
		})
	}

	return nil
}

func (g *GameClient) handleCastSkillPacket(packet d2netpacket.NetPacket) error {
	playerCast, err := d2netpacket.UnmarshalCast(packet.PacketData)
	if err != nil {
		return err
	}

	// A cast of a skill this game has no record of is dropped, before
	// anything is loaded or dereferenced for it. Strigoi's game loads no
	// skill table, so every Diablo II cast is such a skill there -- and one
	// reached the host's own client from a -classic client and took the host
	// down on its nil record (the tables burst's review, B1, 27 Sep 2026).
	// The host refuses both the join and the cast now (GameServer); this is
	// the client not trusting that. The first drop is logged, the rest
	// counted.
	skillRecord := g.asset.Records.Skill.Details[playerCast.SkillID]
	if skillRecord == nil {
		if g.castsDropped.Add(1) == 1 {
			g.Warningf("dropped a cast of skill %d by %s: this game has no record of it (Strigoi's game has no Diablo II skills; further drops are not logged)",
				playerCast.SkillID, playerCast.SourceEntityID)
		}

		return nil
	}

	// Diablo II's missiles and cast overlays load on the first cast, not at
	// boot (d2resource.CastRecords): a game that never casts never reads them.
	if err := g.asset.EnsureRecords(d2resource.CastRecords...); err != nil {
		return err
	}

	player := g.Players[playerCast.SourceEntityID]
	player.StopMoving()

	castX := playerCast.TargetX * numSubtilesPerTile
	castY := playerCast.TargetY * numSubtilesPerTile

	direction := player.Position.DirectionTo(*d2vector.NewVector(castX, castY))
	player.SetDirection(direction)

	missileEntities, err := g.createMissileEntities(skillRecord, player, castX, castY)
	if err != nil {
		return err
	}

	var summonedNpcEntity *d2mapentity.NPC
	if skillRecord.Summon != "" {
		summonedNpcEntity, err = g.createSummonedNpcEntity(skillRecord, int(castX), int(castY))

		if err != nil {
			return err
		}
	}

	player.StartCasting(skillRecord.Anim, func() {
		if len(missileEntities) > 0 {
			// shoot the missiles of the skill after the player has finished casting
			for _, missileEntity := range missileEntities {
				g.MapEngine.AddEntity(missileEntity)
			}
		}

		if summonedNpcEntity != nil {
			// summon the referenced NPC after the player has finished casting
			g.MapEngine.AddEntity(summonedNpcEntity)
		}
	})

	overlayRecord := g.asset.Records.Layout.Overlays[skillRecord.Castoverlay]

	return g.playCastOverlay(overlayRecord, int(player.Position.X()), int(player.Position.Y()))
}

func (g *GameClient) createSummonedNpcEntity(skillRecord *d2records.SkillRecord, x, y int) (*d2mapentity.NPC, error) {
	monsterStatsRecord := g.asset.Records.Monster.Stats[skillRecord.Summon]

	if monsterStatsRecord == nil {
		fmtErr := "cannot cast skill - No monstat entry for \"%s\""
		return nil, fmt.Errorf(fmtErr, skillRecord.Summon)
	}

	// https://github.com/OpenDiablo2/OpenDiablo2/issues/803
	summonedNpcEntity, err := g.MapEngine.NewNPC(x, y, monsterStatsRecord, 0)
	if err != nil {
		return nil, err
	}

	return summonedNpcEntity, nil
}

func (g *GameClient) createMissileEntities(
	skillRecord *d2records.SkillRecord,
	player *d2mapentity.Player,
	castX, castY float64,
) ([]*d2mapentity.Missile, error) {
	missileRecords := []*d2records.MissileRecord{
		g.asset.Records.GetMissileByName(skillRecord.Cltmissile),
		g.asset.Records.GetMissileByName(skillRecord.Cltmissilea),
		g.asset.Records.GetMissileByName(skillRecord.Cltmissileb),
		g.asset.Records.GetMissileByName(skillRecord.Cltmissilec),
		g.asset.Records.GetMissileByName(skillRecord.Cltmissiled),
	}

	missileEntities := make([]*d2mapentity.Missile, 0)

	for _, missileRecord := range missileRecords {
		if missileRecord == nil {
			continue
		}

		missileEntity, err := g.createMissileEntity(missileRecord, player, castX, castY)
		if err != nil {
			return nil, err
		}

		missileEntities = append(missileEntities, missileEntity)
	}

	return missileEntities, nil
}

func (g *GameClient) createMissileEntity(
	missileRecord *d2records.MissileRecord,
	player *d2mapentity.Player,
	castX, castY float64,
) (*d2mapentity.Missile, error) {
	if missileRecord == nil {
		return nil, nil
	}

	radians := d2math.GetRadiansBetween(
		player.Position.X(),
		player.Position.Y(),
		castX,
		castY,
	)

	missileEntity, err := g.MapEngine.NewMissile(
		int(player.Position.X()),
		int(player.Position.Y()),
		g.asset.Records.Missiles[missileRecord.Id],
	)

	if err != nil {
		return nil, err
	}

	missileEntity.SetRadians(radians, func() {
		g.MapEngine.RemoveEntity(missileEntity)
	})

	return missileEntity, nil
}

func (g *GameClient) playCastOverlay(overlayRecord *d2records.OverlayRecord, x, y int) error {
	if overlayRecord == nil {
		return nil
	}

	overlayEntity, err := g.MapEngine.NewCastOverlay(
		x,
		y,
		overlayRecord,
	)
	if err != nil {
		return err
	}

	overlayEntity.SetOnDoneFunc(func() {
		g.MapEngine.RemoveEntity(overlayEntity)
	})

	g.MapEngine.AddEntity(overlayEntity)

	return nil
}

func (g *GameClient) handlePingPacket() error {
	pongPacket, err := d2netpacket.CreatePongPacket(g.PlayerID)
	if err != nil {
		return err
	}

	err = g.clientConnection.SendPacketToServer(pongPacket)

	if err != nil {
		return err
	}

	return nil
}

func (g *GameClient) handlePlayerDisconnectionPacket(packet d2netpacket.NetPacket) error {
	disconnectPacket, err := d2netpacket.UnmarshalPlayerDisconnectionRequest(packet.PacketData)
	if err != nil {
		return err
	}

	player := g.Players[disconnectPacket.ID]
	g.MapEngine.RemoveEntity(player)
	delete(g.Players, disconnectPacket.ID)

	return nil
}

// IsSinglePlayer returns a bool for whether the game is a single-player game
func (g *GameClient) IsSinglePlayer() bool {
	return g.connectionType == d2clientconnectiontype.Local
}

// handleJoinRefusedPacket keeps the host's reason for refusing this client's
// join and says it loudly; the game screen takes the player back to the main
// menu with it (JoinRefused).
func (g *GameClient) handleJoinRefusedPacket(packet d2netpacket.NetPacket) error {
	refused, err := d2netpacket.UnmarshalJoinRefused(packet.PacketData)
	if err != nil {
		return err
	}

	reason := refused.Reason
	if reason == "" {
		reason = "the host refused this game"
	}

	g.Errorf("THE HOST REFUSED THIS JOIN: %s", reason)

	g.joinRefusedMu.Lock()
	g.joinRefused = reason
	g.joinRefusedMu.Unlock()

	return nil
}

// JoinRefused is why the host refused this client's join, or "" when it has
// not: a host refuses a client that launched the other game (-classic, or
// not). The game screen reads it every frame (Game.Advance).
func (g *GameClient) JoinRefused() string {
	g.joinRefusedMu.Lock()
	defer g.joinRefusedMu.Unlock()

	return g.joinRefused
}

// MapMismatch reports why the world this client built is not the one its host
// built -- another map, or a different copy of it -- or nil when it is.
func (g *GameClient) MapMismatch() error {
	g.mapMismatchMu.Lock()
	defer g.mapMismatchMu.Unlock()

	return g.mapMismatch
}
