package d2netpacket

import (
	"encoding/json"

	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"

	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket/d2netpackettype"
)

// PlayerConnectionRequestPacket contains a player ID and game state.
// It is sent by a remote client to initiate a connection (join a game).
//
// Classic is the game the client launched: true for -classic (Diablo II's
// game), false for Strigoi's (d2asset.AssetManager.Classic). The two games
// load different tables -- Strigoi's has no Diablo II skill -- so a host
// refuses a join from the other one (GameServer.registerConnection, the
// JoinRefused packet) rather than let one game's packets reach a client that
// cannot read them (the tables burst's review, B1, 27 Sep 2026). A client
// from before the field reads as Strigoi's game.
type PlayerConnectionRequestPacket struct {
	ID          string            `json:"id"`
	PlayerState *d2hero.HeroState `json:"gameState"`
	Classic     bool              `json:"classic,omitempty"`
}

// CreatePlayerConnectionRequestPacket returns a NetPacket which defines a
// PlayerConnectionRequestPacket with the given ID, game state and game.
func CreatePlayerConnectionRequestPacket(id string, playerState *d2hero.HeroState, classic bool) (NetPacket, error) {
	playerConnectionRequest := PlayerConnectionRequestPacket{
		ID:          id,
		PlayerState: playerState,
		Classic:     classic,
	}

	b, err := json.Marshal(playerConnectionRequest)
	if err != nil {
		return NetPacket{PacketType: d2netpackettype.PlayerConnectionRequest}, err
	}

	return NetPacket{
		PacketType: d2netpackettype.PlayerConnectionRequest,
		PacketData: b,
	}, nil
}

// UnmarshalPlayerConnectionRequest unmarshals the given data to a
// PlayerConnectionRequestPacket struct
func UnmarshalPlayerConnectionRequest(packet []byte) (PlayerConnectionRequestPacket, error) {
	var resp PlayerConnectionRequestPacket

	if err := json.Unmarshal(packet, &resp); err != nil {
		return PlayerConnectionRequestPacket{}, err
	}

	return resp, nil
}
