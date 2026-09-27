package d2netpacket

import (
	"encoding/json"

	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket/d2netpackettype"
)

// JoinRefusedPacket is sent by the server to a client whose join it refuses,
// with the reason in words a player can act on. The one refusal today is the
// other game: a -classic client joining Strigoi's host, or the reverse
// (PlayerConnectionRequestPacket.Classic). The server closes the connection
// after it.
type JoinRefusedPacket struct {
	Reason string `json:"reason"`
}

// CreateJoinRefusedPacket returns a NetPacket which declares a
// JoinRefusedPacket with the given reason.
func CreateJoinRefusedPacket(reason string) (NetPacket, error) {
	b, err := json.Marshal(JoinRefusedPacket{Reason: reason})
	if err != nil {
		return NetPacket{PacketType: d2netpackettype.JoinRefused}, err
	}

	return NetPacket{
		PacketType: d2netpackettype.JoinRefused,
		PacketData: b,
	}, nil
}

// UnmarshalJoinRefused unmarshals the given data to a JoinRefusedPacket struct.
func UnmarshalJoinRefused(packet []byte) (JoinRefusedPacket, error) {
	var resp JoinRefusedPacket

	if err := json.Unmarshal(packet, &resp); err != nil {
		return resp, err
	}

	return resp, nil
}
