package d2server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket/d2netpackettype"
)

// recordingClient is a connected client that keeps what the server sends it.
type recordingClient struct {
	id   string
	sent []d2netpacket.NetPacket
}

func (c *recordingClient) GetUniqueID() string { return c.id }

func (c *recordingClient) GetConnectionType() d2clientconnectiontype.ClientConnectionType {
	return d2clientconnectiontype.LANClient
}

func (c *recordingClient) SendPacketToClient(p d2netpacket.NetPacket) error {
	c.sent = append(c.sent, p)
	return nil
}

func (c *recordingClient) GetPlayerState() *d2hero.HeroState            { return &d2hero.HeroState{} }
func (c *recordingClient) SetPlayerState(playerState *d2hero.HeroState) {}

// testServer is a GameServer of the given game with two clients connected and
// its log in the returned buffer. No world is built: nothing here reads one.
func testServer(t *testing.T, classic bool) (*GameServer, []*recordingClient, *bytes.Buffer) {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	asset.SetClassic(classic)

	var log bytes.Buffer

	logger := d2util.NewLogger()
	logger.Writer = &log
	logger.SetColorEnabled(false)
	logger.SetLevel(d2util.LogLevelWarning)

	clients := []*recordingClient{{id: "host"}, {id: "guest"}}
	g := &GameServer{asset: asset, connections: map[string]ClientConnection{}, maxConnections: 8, Logger: logger}

	for _, c := range clients {
		g.connections[c.id] = c
	}

	return g, clients, &log
}

// A Strigoi host rebroadcasts no Diablo II cast: its clients load no skill
// table, and the cast a -classic client sent reached the host's own client as
// a nil record it dereferenced (the tables burst's review, B1). The first
// refusal is logged; a -classic host still rebroadcasts every cast (the
// control).
//
// Negative control (27 Sep 2026): delete the !Classic() refusal and the
// Strigoi case fails -- both clients are sent the cast.
func TestAStrigoiHostRebroadcastsNoCast(t *testing.T) {
	cast, err := d2netpacket.CreateCastPacket("guest", 36, 4, 5)
	if err != nil {
		t.Fatal(err)
	}

	g, clients, log := testServer(t, false)

	for i := 0; i < 2; i++ {
		if err := g.OnPacketReceived(clients[1], cast); err != nil {
			t.Fatalf("a refused cast is not an error: %v", err)
		}
	}

	for _, c := range clients {
		if len(c.sent) != 0 {
			t.Errorf("Strigoi's host sent %s %d packet(s) for a Diablo II cast", c.id, len(c.sent))
		}
	}

	if n := strings.Count(log.String(), "refused a Diablo II cast from guest"); n != 1 {
		t.Errorf("two refusals logged %d time(s), want once:\n%s", n, log.String())
	}

	// The control: -classic's host rebroadcasts it to every client.
	g, clients, _ = testServer(t, true)

	if err := g.OnPacketReceived(clients[1], cast); err != nil {
		t.Fatal(err)
	}

	for _, c := range clients {
		if len(c.sent) != 1 || c.sent[0].PacketType != d2netpackettype.CastSkill {
			t.Errorf("control: -classic's host sent %s %v, want the cast", c.id, c.sent)
		}
	}
}

// A host refuses a join from the other game and tells the client why, in a
// JoinRefused packet it writes before the connection closes; the client is
// not registered.
//
// Negative control (27 Sep 2026): delete the joinRefusal check in
// registerConnection and this fails -- the join is taken (and, with no world
// built here, the server reaches for its first map engine).
func TestAHostRefusesTheOtherGamesJoin(t *testing.T) {
	for _, c := range []struct {
		host, client bool
		flag         string
	}{
		{host: false, client: true, flag: "without -classic"},
		{host: true, client: false, flag: "with -classic"},
	} {
		g, _, log := testServer(t, c.host)

		req, err := d2netpacket.CreatePlayerConnectionRequestPacket("joiner", &d2hero.HeroState{}, c.client)
		if err != nil {
			t.Fatal(err)
		}

		server, client := net.Pipe()
		read := make(chan d2netpacket.NetPacket, 1)

		go func() {
			var p d2netpacket.NetPacket
			if json.NewDecoder(client).Decode(&p) == nil {
				read <- p
			}

			close(read)
		}()

		_ = server.SetDeadline(time.Now().Add(5 * time.Second))

		_, err = g.registerConnection(req.PacketData, server)
		_ = server.Close()

		if !errors.Is(err, errOtherGame) {
			t.Fatalf("host classic=%v, client classic=%v: the join returned %v, want it refused", c.host, c.client, err)
		}

		if _, ok := g.connections["joiner"]; ok {
			t.Fatalf("host classic=%v: the refused client was registered", c.host)
		}

		p, ok := <-read
		if !ok || p.PacketType != d2netpackettype.JoinRefused {
			t.Fatalf("host classic=%v: the client was sent %v (%v), want the refusal", c.host, p.PacketType, ok)
		}

		refused, err := d2netpacket.UnmarshalJoinRefused(p.PacketData)
		if err != nil || !strings.Contains(refused.Reason, c.flag) {
			t.Fatalf("host classic=%v: the refusal reads %q (%v), want it to say to launch %s", c.host, refused.Reason, err, c.flag)
		}

		if !strings.Contains(log.String(), "refused the join of joiner") {
			t.Fatalf("host classic=%v: the refusal was not logged: %q", c.host, log.String())
		}
	}
}

// The same game joins: joinRefusal is empty for either game, and names the
// flag to change otherwise.
func TestJoinRefusalNamesTheFlag(t *testing.T) {
	if joinRefusal(false, false) != "" || joinRefusal(true, true) != "" {
		t.Fatal("a client of the host's own game was refused")
	}

	if r := joinRefusal(false, true); !strings.Contains(r, "without -classic") {
		t.Fatalf("Strigoi's host to a -classic client: %q", r)
	}

	if r := joinRefusal(true, false); !strings.Contains(r, "with -classic") || strings.Contains(r, "without") {
		t.Fatalf("-classic's host to a Strigoi client: %q", r)
	}
}
