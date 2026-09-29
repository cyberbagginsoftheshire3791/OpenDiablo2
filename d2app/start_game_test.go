//go:build harness

package d2app

import (
	"errors"
	"strings"
	"testing"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2game/d2gamescreen"
)

// countingConn is a game client that counts what startGame did with it.
type countingConn struct {
	opens, closes int
	openErr       error
}

func (c *countingConn) Open(string, string) error {
	c.opens++
	return c.openErr
}

func (c *countingConn) Close() error {
	c.closes++
	return nil
}

// BUG-25 (27 Sep 2026): a CreateGame that fails closes the client it was
// given and answers why -- and no screen. It used to be logged and the screen
// set anyway: a nil *Game, and a client left open (a local game's server
// holding the port). Forced here with the REAL CreateGame on an asset manager
// with nothing in it, so its first step, loading the bestiary, fails.
//
// Negative controls (27 Sep 2026): drop startGame's client.Close and the close
// count fails; hand back create()'s screen and no reason whatever its error
// (the old ToCreateGame) and the reason check fails.
func TestAFailedGameClosesItsClient(t *testing.T) {
	asset, err := d2asset.NewAssetManager(d2util.LogLevelNone)
	if err != nil {
		t.Fatal(err)
	}

	conn := &countingConn{}

	game, reason := startGame(conn, "", "hero.od2", func() (*d2gamescreen.Game, error) {
		return d2gamescreen.CreateGame(nil, asset, nil, nil, nil, nil, nil, nil, d2util.LogLevelNone, nil, nil)
	})

	if game != nil {
		t.Fatal("a failed CreateGame handed back a screen")
	}

	if conn.opens != 1 || conn.closes != 1 {
		t.Fatalf("a failed CreateGame: the client was opened %d time(s) and closed %d; want 1 and 1", conn.opens, conn.closes)
	}

	if !strings.HasPrefix(reason, gameStartFailed) || !strings.Contains(reason, "bestiary") {
		t.Fatalf("the main menu is told %q; want %q and the bestiary's failure", reason, gameStartFailed)
	}
}

// The other three ways out of startGame keep its contract -- a screen or a
// reason, never both, never neither -- and close only what they opened.
func TestStartGameAnswersAScreenOrAReason(t *testing.T) {
	built := &d2gamescreen.Game{}

	for name, c := range map[string]struct {
		openErr     error
		screen      *d2gamescreen.Game
		createErr   error
		wantScreen  bool
		wantCloses  int
		wantCreates int
		wantReason  string
	}{
		"a game":                 {screen: built, wantScreen: true, wantCreates: 1},
		"no host":                {openErr: errors.New("refused"), wantReason: "can not connect to the host"},
		"no screen and no error": {wantCloses: 1, wantCreates: 1, wantReason: gameStartFailed},
		"a screen and an error":  {screen: built, createErr: errors.New("half built"), wantCloses: 1, wantCreates: 1, wantReason: "half built"},
		"an error and no screen": {createErr: errors.New("no bestiary"), wantCloses: 1, wantCreates: 1, wantReason: "no bestiary"},
	} {
		conn := &countingConn{openErr: c.openErr}
		creates := 0

		game, reason := startGame(conn, "host", "hero.od2", func() (*d2gamescreen.Game, error) {
			creates++
			return c.screen, c.createErr
		})

		if (game != nil) != c.wantScreen || (reason == "") != c.wantScreen {
			t.Errorf("%s: screen %v, reason %q; want a screen %v and a reason exactly when there is none", name, game != nil, reason, c.wantScreen)
		}

		if conn.closes != c.wantCloses || creates != c.wantCreates {
			t.Errorf("%s: closed %d time(s), created %d; want %d and %d", name, conn.closes, creates, c.wantCloses, c.wantCreates)
		}

		if !strings.Contains(reason, c.wantReason) {
			t.Errorf("%s: reason %q; want it to say %q", name, reason, c.wantReason)
		}
	}
}
