package d2gamescreen

import (
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2client/d2clientconnectiontype"
	"github.com/OpenDiablo2/OpenDiablo2/d2networking/d2netpacket"
)

// menuNavigator records where the game screen sent the player.
type menuNavigator struct{ mainMenu []string }

func (n *menuNavigator) ToMainMenu(msg ...string) {
	n.mainMenu = append(n.mainMenu, strings.Join(msg, ""))
}
func (n *menuNavigator) ToSelectHero(d2clientconnectiontype.ClientConnectionType, string) {}
func (n *menuNavigator) ToCreateGame(string, d2clientconnectiontype.ClientConnectionType, string) {
}
func (n *menuNavigator) ToCharacterSelect(d2clientconnectiontype.ClientConnectionType, string) {}
func (n *menuNavigator) ToMapEngineTest(int, int)                                              {}
func (n *menuNavigator) ToCredits()                                                            {}
func (n *menuNavigator) ToWorldEditor(string)                                                  {}
func (n *menuNavigator) ToPlaytest(string)                                                     {}
func (n *menuNavigator) ToCinematics()                                                         {}

// A host that refused this join -- the client launched the other game --
// sends the player back to the main menu with the host's reason, once, and
// nothing of the refused game advances (the tables burst's review, B1, 27 Sep
// 2026). Before, a refused LAN client sat on a world that never came.
//
// Negative control (27 Sep 2026): delete the JoinRefused check at the top of
// Game.Advance and this fails -- the screen advances a game that does not
// exist (here: a nil sound engine) and never goes to the menu.
func TestARefusedJoinGoesBackToTheMainMenuOnce(t *testing.T) {
	client := &d2client.GameClient{}

	refused, err := d2netpacket.CreateJoinRefusedPacket("launch with -classic to join it")
	if err != nil {
		t.Fatal(err)
	}

	if err := client.OnPacketReceived(refused); err != nil {
		t.Fatal(err)
	}

	nav := &menuNavigator{}
	v := &Game{gameClient: client, navigator: nav}

	for i := 0; i < 3; i++ {
		if err := v.Advance(1.0 / 60); err != nil {
			t.Fatalf("frame %d: %v", i, err)
		}
	}

	if len(nav.mainMenu) != 1 || !strings.Contains(nav.mainMenu[0], "launch with -classic to join it") {
		t.Fatalf("three frames of a refused game went to the main menu %d time(s): %q; want once, with the host's reason",
			len(nav.mainMenu), nav.mainMenu)
	}
}

// Diablo II's cast is -classic's: in Strigoi's game OnPlayerCast sends nothing
// and touches nothing, whoever asks -- the controls do not ask (the right
// button is the torch, shift-click does nothing), and this is what stands
// behind them if one ever does.
//
// Negative control (27 Sep 2026): delete OnPlayerCast's !Classic() return and
// this fails -- it reaches for the game client to send the cast (nil here).
func TestStrigoisGameSendsNoCast(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelError)
	if err != nil {
		t.Fatal(err)
	}

	v := &Game{asset: asset} // no client: a cast that reached for one would fall over

	v.OnPlayerCast(36, 10, 12)
}
