package d2client

import (
	"bytes"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
)

// testClient is a GameClient over an asset manager with no MPQ and no table
// loaded -- Strigoi's game as far as a cast is concerned: no skill record --
// whose log is kept in the returned buffer.
func testClient(t *testing.T) (*GameClient, *d2asset.AssetManager, *bytes.Buffer) {
	t.Helper()

	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	var log bytes.Buffer

	logger := d2util.NewLogger()
	logger.Writer = &log
	logger.SetColorEnabled(false)
	logger.SetLevel(d2util.LogLevelWarning)

	return &GameClient{asset: asset, Players: map[string]*d2mapentity.Player{}, Logger: logger}, asset, &log
}

// A cast of a skill this game has no record of -- every Diablo II skill, in
// Strigoi's game -- is dropped before anything is loaded or dereferenced for
// it, and the first drop is logged, once (the tables burst's review, B1: a
// -classic client's right-click, rebroadcast by the host, took the host's own
// client down on the nil record at game_client.go's .Cltmissile).
//
// Negative control (27 Sep 2026): delete the nil-record drop and this fails
// -- the handler goes on to load the cast tables for a skill it cannot cast,
// and with none to load returns an error for the packet.
func TestACastOfASkillThisGameLacksIsDropped(t *testing.T) {
	g, asset, log := testClient(t)

	cast, err := d2netpacket.CreateCastPacket("a -classic client", 36, 10, 12)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		if err := g.OnPacketReceived(cast); err != nil {
			t.Fatalf("cast %d: a cast of a skill this game lacks must be dropped, not an error: %v", i+1, err)
		}
	}

	if n := strings.Count(log.String(), "dropped a cast of skill 36"); n != 1 {
		t.Fatalf("three drops logged %d time(s), want once:\n%s", n, log.String())
	}

	if got := g.castsDropped.Load(); got != 3 {
		t.Fatalf("%d drops counted, want 3", got)
	}

	for _, table := range d2resource.CastRecords {
		if asset.RecordsLoaded(table) {
			t.Errorf("%s was loaded for a cast that was dropped", table)
		}
	}
}

// The host's refusal of this join is kept for the game screen, which goes back
// to the main menu with it (Game.Advance), and said loudly.
//
// Negative control (27 Sep 2026): remove the JoinRefused case from
// OnPacketReceived and this fails -- the packet is an unknown type.
func TestAJoinRefusalIsKeptForTheScreen(t *testing.T) {
	g, _, log := testClient(t)

	if got := g.JoinRefused(); got != "" {
		t.Fatalf("a client nobody refused reports %q", got)
	}

	refused, err := d2netpacket.CreateJoinRefusedPacket("launch with -classic to join it")
	if err != nil {
		t.Fatal(err)
	}

	if err := g.OnPacketReceived(refused); err != nil {
		t.Fatal(err)
	}

	if got := g.JoinRefused(); got != "launch with -classic to join it" {
		t.Fatalf("the refusal kept is %q", got)
	}

	if !strings.Contains(log.String(), "THE HOST REFUSED THIS JOIN: launch with -classic") {
		t.Fatalf("the refusal was not said: %q", log.String())
	}
}
