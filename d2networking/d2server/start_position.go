package d2server

import (
	"math"
	"sync"

	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
)

// THE NEXT GAME'S START POSITION (M4.6 B4a, the world save's load). The server
// puts every player it connects on the map's start tile -- "the server
// overwrites X/Y with the map start on connect" (build plan section 1) -- so a
// resumed game would open with him at the village gate while the world file
// says he stood somewhere else. A load sets the position he was saved at, in
// SUB-TILES (the world file's hero.pos), immediately before the game client
// constructs the in-process GameServer, exactly as SetNextGameSeed hands it the
// seed. One-shot: the next NewGameServer takes it, and only the LOCAL client
// (the one whose save is being loaded) is placed there; a remote player joins
// where he always did. When none is set the behaviour is the old one.
//
// The AddPlayer packet carries whole sub-tiles, so the server places him on
// the sub-tile his saved point lies in (floored); the load then stands him at
// the saved point exactly (Player.StandAt), which a float in a packet could not
// carry bit-exact anyway.

// nolint:gochecknoglobals // deliberate one-shot cross-package handoff, as nextGameSeed
var nextStart struct {
	sync.Mutex
	set  bool
	x, y float64
}

// startPosition is a start position taken from the one-shot, held by the server
// until the local client connects.
type startPosition struct {
	set  bool
	x, y float64 // sub-tiles
}

// SetNextStartPosition sets where, in sub-tiles, the next game server puts
// its local player.
func SetNextStartPosition(subX, subY float64) {
	nextStart.Lock()
	defer nextStart.Unlock()

	nextStart.set, nextStart.x, nextStart.y = true, subX, subY
}

// ClearNextStartPosition drops a start position no game took (a load refused
// before its game was opened).
func ClearNextStartPosition() {
	nextStart.Lock()
	defer nextStart.Unlock()

	nextStart.set = false
}

// ClearNextGame drops everything a load armed for the next game server --
// its seed (SetNextGameSeed) and its start position -- when no server took
// them: the game never opened (the B4a review, C5; before it only the start
// position was dropped, and the next game of any hero began on the refused
// world file's seed).
func ClearNextGame() {
	SetNextGameSeed(0)
	ClearNextStartPosition()
}

// takeNextStartPosition consumes the one-shot.
func takeNextStartPosition() startPosition {
	nextStart.Lock()
	defer nextStart.Unlock()

	p := startPosition{set: nextStart.set, x: nextStart.x, y: nextStart.y}
	nextStart.set = false

	return p
}

// placeOnConnect is where a connecting client is put: the map's start tile
// (tileX, tileY), in the sub-tile the AddPlayer packet carries and in the
// world tiles the player state keeps -- or, for the LOCAL client of a server
// that took a start position, the sub-tile that position lies in. The start
// position is spent by the first local client either way.
func (p *startPosition) placeOnConnect(conn d2clientconnectiontype.ClientConnectionType,
	tileX, tileY float64) (subX, subY int, worldX, worldY float64) {
	subX = int(tileX*subtilesPerTile) + middleOfTileOffset
	subY = int(tileY*subtilesPerTile) + middleOfTileOffset
	worldX, worldY = tileX, tileY

	if !p.set || conn != d2clientconnectiontype.Local {
		return subX, subY, worldX, worldY
	}

	p.set = false

	return int(math.Floor(p.x)), int(math.Floor(p.y)), p.x / subtilesPerTile, p.y / subtilesPerTile
}
