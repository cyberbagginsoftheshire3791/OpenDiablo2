package d2remoteclient

import (
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
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

// BUG-24 (27 Sep 2026): a full server and a spawned item are decoded like
// every other packet the game client handles. decodeToPacket had no case for
// either, so a client refused for a full server never reached GameClient's
// "Server is full", and a host's item never reached a remote client's map.
//
// Negative control (27 Sep 2026): remove either case from decodeToPacket and
// its half fails -- "unrecognized packet type".
func TestAFullServerAndAnItemAreDecoded(t *testing.T) {
	r := &RemoteClientConnection{}

	full, err := d2netpacket.CreateServerFullPacket()
	if err != nil {
		t.Fatal(err)
	}

	p, err := r.decodeToPacket(full.PacketType, string(full.PacketData))
	if err != nil || p.PacketType != d2netpackettype.ServerFull {
		t.Fatalf("a full server decoded as %s (%v)", p.PacketType, err)
	}

	if _, err := d2netpacket.UnmarshalServerFull(p.PacketData); err != nil {
		t.Fatalf("the decoded ServerFull does not read back: %v", err)
	}

	item, err := d2netpacket.CreateSpawnItemPacket(12, 34, "hax", "rin")
	if err != nil {
		t.Fatal(err)
	}

	p, err = r.decodeToPacket(item.PacketType, string(item.PacketData))
	if err != nil || p.PacketType != d2netpackettype.SpawnItem {
		t.Fatalf("a spawned item decoded as %s (%v)", p.PacketType, err)
	}

	got, err := d2netpacket.UnmarshalSpawnItem(p.PacketData)
	if err != nil || got.X != 12 || got.Y != 34 || strings.Join(got.Codes, ",") != "hax,rin" {
		t.Fatalf("the decoded SpawnItem reads back as %+v (%v); want 12,34 hax,rin", got, err)
	}
}

// handed is a game client that keeps what the connection hands it.
type handed chan d2netpacket.NetPacket

func (h handed) OnPacketReceived(p d2netpacket.NetPacket) error {
	h <- p
	return nil
}

// BUG-24's other half: a packet the client cannot decode is dropped, not
// handed to the game client as decodeToPacket's zero packet -- which reads as
// an empty UpdateServerInfo, type 0, a packet the host never sent. And a full
// server, sent over the wire, reaches the game client as ServerFull.
//
// Negative control (27 Sep 2026): remove serverListener's continue after a
// decode error and this fails -- the first packet the game client is handed
// is the zero packet.
func TestAnUndecodablePacketIsNotHandedOn(t *testing.T) {
	host, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	defer host.Close()

	accepted := make(chan net.Conn, 1)

	go func() {
		conn, err := host.Accept()
		if err == nil {
			accepted <- conn
		}

		close(accepted)
	}()

	conn, err := net.DialTCP("tcp", nil, host.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}

	defer conn.Close()

	server, ok := <-accepted
	if !ok {
		t.Fatal("the host accepted no connection")
	}

	defer server.Close()

	got := make(handed, 4)
	r := &RemoteClientConnection{tcpConnection: conn, clientListener: got, Logger: d2util.NewLogger()}
	r.Logger.SetLevel(d2util.LogLevelFatal)

	go r.serverListener()

	enc := json.NewEncoder(server)

	// A packet no host sends a client, so the client has no case for it.
	if err := enc.Encode(d2netpacket.NetPacket{
		PacketType: d2netpackettype.PlayerConnectionRequest, PacketData: []byte(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	full, err := d2netpacket.CreateServerFullPacket()
	if err != nil {
		t.Fatal(err)
	}

	if err := enc.Encode(full); err != nil {
		t.Fatal(err)
	}

	select {
	case p := <-got:
		if p.PacketType != d2netpackettype.ServerFull {
			t.Fatalf("the first packet the game client was handed is %s %q; want ServerFull, the undecodable one dropped",
				p.PacketType, p.PacketData)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the game client was handed nothing")
	}
}
