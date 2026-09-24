package d2netpacket

import (
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
)

// The host's map travels in the packet, and a packet from a host that sent
// none reads as the generated world.
func TestGenerateMapPacketCarriesTheHostsMap(t *testing.T) {
	p, err := CreateGenerateMapPacket(d2enum.RegionAct1Town, "/data/strigoi/maps/village.tmj", "abc123")
	if err != nil {
		t.Fatal(err)
	}

	got, err := UnmarshalGenerateMap(p.PacketData)
	if err != nil {
		t.Fatal(err)
	}

	if got.RegionType != d2enum.RegionAct1Town || got.Map != "/data/strigoi/maps/village.tmj" || got.MapSHA256 != "abc123" {
		t.Fatalf("round trip: %+v", got)
	}

	old, err := UnmarshalGenerateMap([]byte(`{"regionType":1}`))
	if err != nil {
		t.Fatal(err)
	}

	if old.Map != "" || old.MapSHA256 != "" {
		t.Fatalf("an old host's packet read as map %q sha %q; want the generated world", old.Map, old.MapSHA256)
	}
}
