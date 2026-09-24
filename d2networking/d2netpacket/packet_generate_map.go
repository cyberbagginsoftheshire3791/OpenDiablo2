package d2netpacket

import (
	"encoding/json"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket/d2netpackettype"
)

// GenerateMapPacket contains an enumerable representing a region. It
// is sent by the server to generate the map for the given region on
// a client.
//
// Map and MapSHA256 are the world the host built (Strigoi, 23 Sep 2026): the
// authored map's game-relative path and the SHA-256 of its .tmj, or empty for
// Diablo II's generated world. A client builds that world whatever its own
// switches say, and says so when its copy is not the host's
// (d2mapgen.GenerateHostWorld).
type GenerateMapPacket struct {
	RegionType d2enum.RegionIdType `json:"regionType"`
	Map        string              `json:"map,omitempty"`
	MapSHA256  string              `json:"mapSha256,omitempty"`
}

// CreateGenerateMapPacket returns a NetPacket which declares a
// GenerateMapPacket with the given regionType and the host's map ("" for the
// generated world).
func CreateGenerateMapPacket(regionType d2enum.RegionIdType, mapPath, mapSHA256 string) (NetPacket, error) {
	generateMapPacket := GenerateMapPacket{
		RegionType: regionType,
		Map:        mapPath,
		MapSHA256:  mapSHA256,
	}

	b, err := json.Marshal(generateMapPacket)
	if err != nil {
		return NetPacket{PacketType: d2netpackettype.GenerateMap}, err
	}

	return NetPacket{
		PacketType: d2netpackettype.GenerateMap,
		PacketData: b,
	}, nil
}

// UnmarshalGenerateMap unmarshals the given packet data into a GenerateMapPacket struct
func UnmarshalGenerateMap(packet []byte) (GenerateMapPacket, error) {
	var p GenerateMapPacket
	if err := json.Unmarshal(packet, &p); err != nil {
		return p, err
	}

	return p, nil
}
