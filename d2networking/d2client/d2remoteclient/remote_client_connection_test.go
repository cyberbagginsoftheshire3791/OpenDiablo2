package d2remoteclient

import (
	"encoding/json"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket/d2netpackettype"
)

// A joining client says which game it launched, so a host of the other one
// can refuse it (GameServer.registerConnection; the tables burst's review,
// B1, 27 Sep 2026). Read here off the wire, as the host reads it.
//
// Negative control (27 Sep 2026): send false whatever the launch and the
// -classic case fails.
func TestTheJoinRequestSaysWhichGame(t *testing.T) {
	for _, classic := range []bool{false, true} {
		asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
		if err != nil {
			t.Fatal(err)
		}

		host, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}

		got := make(chan d2netpacket.NetPacket, 1)

		go func() {
			conn, err := host.Accept()
			if err != nil {
				close(got)
				return
			}

			defer conn.Close()

			var p d2netpacket.NetPacket
			if json.NewDecoder(conn).Decode(&p) == nil {
				got <- p
			}

			close(got)
		}()

		r, err := Create(d2util.LogLevelError, asset)
		if err != nil {
			t.Fatal(err)
		}

		// After Create: a -classic hero factory would load Diablo II's item
		// tables, and there are none here. The request reads the switch when
		// it is sent.
		asset.SetClassic(classic)

		// No save: the request carries no hero, which is all this reads past.
		if err := r.Open(host.Addr().String(), filepath.Join(t.TempDir(), "none.od2")); err != nil {
			t.Fatal(err)
		}

		var p d2netpacket.NetPacket

		select {
		case p = <-got:
		case <-time.After(5 * time.Second):
			t.Fatal("the host never read a join request")
		}

		_ = r.tcpConnection.Close()
		_ = host.Close()

		if p.PacketType != d2netpackettype.PlayerConnectionRequest {
			t.Fatalf("the first packet is %s, want the join request", p.PacketType)
		}

		req, err := d2netpacket.UnmarshalPlayerConnectionRequest(p.PacketData)
		if err != nil {
			t.Fatal(err)
		}

		if req.Classic != classic {
			t.Fatalf("a client launched classic=%v asked to join as classic=%v", classic, req.Classic)
		}
	}
}

// A host's refusal is decoded like any other packet the game client handles.
//
// Negative control (27 Sep 2026): remove its case from decodeToPacket and
// this fails -- "unrecognized packet type".
func TestARefusalIsDecoded(t *testing.T) {
	r := &RemoteClientConnection{}

	p, err := r.decodeToPacket(d2netpackettype.JoinRefused, `{"reason":"launch without -classic to join it"}`)
	if err != nil {
		t.Fatal(err)
	}

	refused, err := d2netpacket.UnmarshalJoinRefused(p.PacketData)
	if err != nil || refused.Reason != "launch without -classic to join it" || p.PacketType != d2netpackettype.JoinRefused {
		t.Fatalf("decoded %s %+v (%v)", p.PacketType, refused, err)
	}
}
