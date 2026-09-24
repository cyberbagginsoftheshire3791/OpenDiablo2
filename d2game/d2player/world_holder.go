package d2player

// What holds the world still (history item 121). The game screen owns the
// answer (Game.WorldHeldBy); the ui provider reports it as world_held_by, so
// the harness can refuse to step world minutes through a hold no number of
// ticks will lift.
const (
	WorldHeldByMenu    = "escape_menu" // the escape menu, in a single-player game
	WorldHeldByLoadout = "loadout"     // he is choosing how he carries himself (T2)
	WorldHeldByTalk    = "talk"        // a conversation is open (T4)
	WorldHeldByFight   = "fight"       // a paced fight: the clock moves a round at a time (T1)
	WorldHeldByJournal = "journal"     // he is reading his journal (J1)
)

// WorldHolder is the game screen's answer to "what holds the world?".
type WorldHolder interface {
	// WorldHeldBy is one of the WorldHeldBy constants, or "" while the world
	// runs.
	WorldHeldBy() string
}

// SetWorldHolder attaches the game screen's world state.
func (g *GameControls) SetWorldHolder(h WorldHolder) { g.worldHolder = h }

// worldHeldByReport is the ui provider's world_held_by: "unknown" until the
// game screen attaches, so a harness guard never reads a missing holder as a
// running world.
func (g *GameControls) worldHeldByReport() string {
	if g.worldHolder == nil {
		return "unknown"
	}

	return g.worldHolder.WorldHeldBy()
}
