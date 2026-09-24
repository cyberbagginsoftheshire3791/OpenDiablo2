package d2player

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2geom"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2items"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

const (
	logPrefix = "Player"
)

// Panel represents the panel at the bottom of the game screen
type Panel interface {
	IsOpen() bool
	Open()
	Close()
}

// mouseBtnActionsThreshold is the minimum spacing, in seconds of the controls'
// own clock, between two click-and-hold actions (move or cast).
const mouseBtnActionsThreshold = 0.25

// repeatDue reports whether a held-button action may fire again: the first
// action always fires (last starts at -threshold), later ones wait the
// threshold. now and last are readings of the controls' accumulated clock.
func repeatDue(now, last float64) bool {
	return now-last >= mouseBtnActionsThreshold
}

const (
	// Since they require special handling, not considering (1) globes, (2) content of the mini panel, (3) belt
	leftSkill actionableType = iota
	xp
	stamina
	rightSkill
	hpGlobe
	manaGlobe
)

const (
	leftSkillX,
	leftSkillY,
	leftSkillWidth,
	leftSkillHeight = 117, 550, 50, 50

	xpX,
	xpY,
	xpWidth,
	xpHeight = 253, 560, 125, 5

	staminaX,
	staminaY,
	staminaWidth,
	staminaHeight = 273, 573, 105, 20

	rightSkillX,
	rightSkillY,
	rightSkillWidth,
	rightSkillHeight = 635, 550, 50, 50

	hpGlobeX,
	hpGlobeY,
	hpGlobeWidth,
	hpGlobeHeight = 30, 525, 80, 60

	manaGlobeX,
	manaGlobeY,
	manaGlobeWidth,
	manaGlobeHeight = 695, 525, 80, 60
)

const (
	menuBottomRectX,
	menuBottomRectY,
	menuBottomRectW,
	menuBottomRectH = 0, 550, 800, 50

	menuLeftRectX,
	menuLeftRectY,
	menuLeftRectW,
	menuLeftRectH = 0, 0, 400, 600

	menuRightRectX,
	menuRightRectY,
	menuRightRectW,
	menuRightRectH = 400, 0, 400, 600
)

// NewGameControls creates a GameControls instance and returns a pointer to it
// nolint:funlen // doesn't make sense to split this up
func NewGameControls(
	asset *d2asset.AssetManager,
	renderer d2interface.Renderer,
	hero *d2mapentity.Player,
	mapEngine *d2mapengine.MapEngine,
	escapeMenu *EscapeMenu,
	mapRenderer *d2maprenderer.MapRenderer,
	inputListener inputCallbackListener,
	term d2interface.Terminal,
	ui *d2ui.UIManager,
	keyMap *KeyMap,
	audioProvider d2interface.AudioProvider,
	l d2util.LogLevel,
	isSinglePlayer bool,
	players map[string]*d2mapentity.Player,
	clock *d2world.Clock,
	squads *d2world.Squads,
	combat *d2world.Combat,
	light *d2world.Light,
	bars BarSource,
) (*GameControls, error) {
	var inventoryRecordKey string

	switch hero.Class {
	case d2enum.HeroAssassin:
		inventoryRecordKey = "Assassin2"
	case d2enum.HeroAmazon:
		inventoryRecordKey = "Amazon2"
	case d2enum.HeroBarbarian:
		inventoryRecordKey = "Barbarian2"
	case d2enum.HeroDruid:
		inventoryRecordKey = "Druid2"
	case d2enum.HeroNecromancer:
		inventoryRecordKey = "Necromancer2"
	case d2enum.HeroPaladin:
		inventoryRecordKey = "Paladin2"
	case d2enum.HeroSorceress:
		inventoryRecordKey = "Sorceress2"
	default:
		return nil, fmt.Errorf("unknown hero class: %d", hero.Class)
	}

	actionableRegions := []actionableRegion{
		{leftSkill, d2geom.Rectangle{
			Left:   leftSkillX,
			Top:    leftSkillY,
			Width:  leftSkillWidth,
			Height: leftSkillHeight,
		}},
		{xp, d2geom.Rectangle{
			Left:   xpX,
			Top:    xpY,
			Width:  xpWidth,
			Height: xpHeight,
		}},
		{stamina, d2geom.Rectangle{
			Left:   staminaX,
			Top:    staminaY,
			Width:  staminaWidth,
			Height: staminaHeight,
		}},
		{rightSkill, d2geom.Rectangle{
			Left:   rightSkillX,
			Top:    rightSkillY,
			Width:  rightSkillWidth,
			Height: rightSkillHeight,
		}},
		{hpGlobe, d2geom.Rectangle{
			Left:   hpGlobeX,
			Top:    hpGlobeY,
			Width:  hpGlobeWidth,
			Height: hpGlobeHeight,
		}},
		{manaGlobe, d2geom.Rectangle{
			Left:   manaGlobeX,
			Top:    manaGlobeY,
			Width:  manaGlobeWidth,
			Height: manaGlobeHeight,
		}},
	}
	inventoryRecord := asset.Records.Layout.Inventory[inventoryRecordKey]

	heroStatsPanel := NewHeroStatsPanel(asset, ui, hero.Name(), hero.Class, l, hero.Stats)

	questLog := NewQuestLog(asset, ui, l, audioProvider, hero.Act)

	inventory, err := NewInventory(asset, ui, l, hero.Gold, inventoryRecord)
	if err != nil {
		return nil, err
	}

	skilltree := newSkillTree(hero.Skills, hero.Class, hero.Stats, asset, l, ui)

	miniPanel := newMiniPanel(asset, ui, l, isSinglePlayer)

	heroState, err := d2hero.NewHeroStateFactory(asset)
	if err != nil {
		return nil, err
	}

	helpOverlay := NewHelpOverlay(asset, ui, l, keyMap)

	const blackAlpha50percent = 0x0000007f

	gc := &GameControls{
		asset:          asset,
		ui:             ui,
		renderer:       renderer,
		hero:           hero,
		heroState:      heroState,
		escapeMenu:     escapeMenu,
		inputListener:  inputListener,
		mapRenderer:    mapRenderer,
		inventory:      inventory,
		skilltree:      skilltree,
		heroStatsPanel: heroStatsPanel,
		questLog:       questLog,
		HelpOverlay:    helpOverlay,
		keyMap:         keyMap,
		squads:         squads,
		combat:         combat,
		light:          light,
		torchesCarried: defaultTorchesCarried,
		bottomMenuRect: &d2geom.Rectangle{
			Left:   menuBottomRectX,
			Top:    menuBottomRectY,
			Width:  menuBottomRectW,
			Height: menuBottomRectH,
		},
		leftMenuRect: &d2geom.Rectangle{
			Left:   menuLeftRectX,
			Top:    menuLeftRectY,
			Width:  menuLeftRectW,
			Height: menuLeftRectH,
		},
		rightMenuRect: &d2geom.Rectangle{
			Left:   menuRightRectX,
			Top:    menuRightRectY,
			Width:  menuRightRectW,
			Height: menuRightRectH,
		},
		actionableRegions:      actionableRegions,
		lastLeftBtnActionTime:  -mouseBtnActionsThreshold, // the first held-click action fires at once
		lastRightBtnActionTime: -mouseBtnActionsThreshold,
		isSinglePlayer:         isSinglePlayer,
	}

	if !isSinglePlayer {
		PartyPanel := NewPartyPanel(asset, ui, hero.Name(), l, hero, hero.Stats, players)
		gc.PartyPanel = PartyPanel
	}

	hud := NewHUD(asset, ui, hero, miniPanel, actionableRegions, mapEngine, l, gc, mapRenderer, clock, bars)
	gc.hud = hud

	hoverLabel := hud.nameLabel
	hoverLabel.SetBackgroundColor(d2util.Color(blackAlpha50percent))

	gc.heroStatsPanel.SetOnCloseCb(gc.onCloseHeroStatsPanel)
	gc.questLog.SetOnCloseCb(gc.onCloseQuestLog)
	gc.inventory.SetOnCloseCb(gc.onCloseInventory)
	gc.skilltree.SetOnCloseCb(gc.onCloseSkilltree)

	gc.escapeMenu.SetOnCloseCb(gc.hud.miniPanel.restoreDisabled)
	gc.HelpOverlay.SetOnCloseCb(gc.hud.miniPanel.restoreDisabled)

	err = gc.bindTerminalCommands(term)
	if err != nil {
		return nil, err
	}

	gc.Logger = d2util.NewLogger()
	gc.Logger.SetLevel(l)
	gc.Logger.SetPrefix(logPrefix)

	return gc, nil
}

// GameControls represents the game's controls on the screen
type GameControls struct {
	keyMap                 *KeyMap
	actionableRegions      []actionableRegion
	asset                  *d2asset.AssetManager
	renderer               d2interface.Renderer // https://github.com/OpenDiablo2/OpenDiablo2/issues/798
	inputListener          inputCallbackListener
	hero                   *d2mapentity.Player
	heroState              *d2hero.HeroStateFactory
	mapRenderer            *d2maprenderer.MapRenderer
	escapeMenu             *EscapeMenu
	ui                     *d2ui.UIManager
	inventory              *Inventory
	hud                    *HUD
	skilltree              *skillTree
	heroStatsPanel         *HeroStatsPanel
	PartyPanel             *PartyPanel
	questLog               *QuestLog
	HelpOverlay            *HelpOverlay
	bottomMenuRect         *d2geom.Rectangle
	leftMenuRect           *d2geom.Rectangle
	rightMenuRect          *d2geom.Rectangle
	lastMouseX             int
	lastMouseY             int
	lastLeftBtnActionTime  float64
	lastRightBtnActionTime float64
	FreeCam                bool
	isSinglePlayer         bool

	// combat and light are M4.4c-2a's seam into d2player, threaded the way
	// clock was for M4.4a and squads for c-1. The key handlers are the only
	// readers: F commits the Action, L works the carried source, E closes the
	// turn.
	combat *d2world.Combat
	light  *d2world.Light

	// torchesCarried is how many unlit torches remain to be taken out. [DIAL],
	// 1 for the slice: the ration is the sixty minutes of burn, not the number
	// of sticks, and one torch is what makes the rationing bite.
	torchesCarried int

	// kitHolder is the game screen's owner of the hero's gear (T2).
	kitHolder KitHolder

	// worldHolder answers what holds the world still (history item 121).
	worldHolder WorldHolder

	// progressHolder is the owner of his experience and talents (T3).
	progressHolder ProgressHolder

	// deathHolder is the owner of his death (death screen v0).
	deathHolder DeathHolder

	// talkHolder is the owner of the village's talk (T4).
	talkHolder TalkHolder

	// forageHolder gathers from the land (T7).
	forageHolder ForageHolder

	// corpseHolder owns the dead (M4.7).
	corpseHolder CorpseHolder

	// journalHolder owns his journal (J1).
	journalHolder JournalHolder

	// squads is the owner the player commands (M4.4c-1): the click handler and
	// the cycle key select through it, and the selection hit test reads its
	// model entities. It is the same instance the game screen registered as the
	// "meters" provider.
	squads *d2world.Squads

	// clock is the controls' own monotonic clock in seconds, accumulated from
	// the deltas Advance receives. It replaces wall-clock reads for click
	// repeat timing, so scripted input under the harness's stepped clock is
	// deterministic and the same script replays the same actions (P3 spec
	// §2.2, §3.4). In live play it advances exactly as wall time does.
	clock float64

	*d2util.Logger
}

type actionableType int

type actionableRegion struct {
	actionableTypeID actionableType
	rect             d2geom.Rectangle
}

// SkillResource represents a Skill with its corresponding icon sprite, path to DC6 file and icon number.
// SkillResourcePath points to a DC6 resource which contains the icons of multiple skills as frames.
// The IconNumber is the frame at which we can find our skill sprite in the DC6 file.
type SkillResource struct {
	SkillResourcePath string // path to a skills DC6 file(see getSkillResourceByClass)
	IconNumber        int    // the index of the frame in the DC6 file
	SkillIcon         *d2ui.Sprite
}

// OnKeyRepeat is called to handle repeated key presses
func (g *GameControls) OnKeyRepeat(event d2interface.KeyEvent) bool {
	if g.dead() || g.talking() || g.journalOpen() {
		return true
	}

	if g.FreeCam {
		var moveSpeed float64 = 8
		if event.KeyMod() == d2enum.KeyModShift {
			moveSpeed *= 2
		}

		if event.Key() == d2enum.KeyDown {
			v := d2vector.NewVector(0, moveSpeed)
			g.mapRenderer.MoveCameraTargetBy(v)

			return true
		}

		if event.Key() == d2enum.KeyUp {
			v := d2vector.NewVector(0, -moveSpeed)
			g.mapRenderer.MoveCameraTargetBy(v)

			return true
		}

		if event.Key() == d2enum.KeyRight {
			v := d2vector.NewVector(moveSpeed, 0)
			g.mapRenderer.MoveCameraTargetBy(v)

			return true
		}

		if event.Key() == d2enum.KeyLeft {
			v := d2vector.NewVector(-moveSpeed, 0)
			g.mapRenderer.MoveCameraTargetBy(v)

			return true
		}
	}

	return false
}

// OnKeyDown handles key presses
func (g *GameControls) OnKeyDown(event d2interface.KeyEvent) bool {
	// Death screen v0: a dead man has two keys, Enter and Escape, and nothing
	// else reaches the game -- not a panel, not a verb.
	if g.deathKey(event) {
		return true
	}

	// J1: the open journal takes every key -- before the talk, the verbs and
	// Escape (A3).
	if g.journalKey(event) {
		return true
	}

	// T4: a conversation takes every key -- numbers answer, Escape leaves.
	if g.talkKey(event) {
		return true
	}

	// M4.7: X drives a stake through the body at his feet.
	if event.Key() == stakeKey && g.corpseHolder != nil && !g.dead() {
		_ = g.corpseHolder.Stake()

		return true
	}

	// M4.7 step 2: D digs a hasty grave for the body at his feet.
	if event.Key() == digKey && g.corpseHolder != nil && !g.dead() {
		_ = g.corpseHolder.Dig()

		return true
	}

	// J2b: U searches the dead man at his feet.
	if event.Key() == searchKey && g.corpseHolder != nil && !g.dead() {
		_ = g.corpseHolder.Search()

		return true
	}

	// T7: K forages (the game refuses it in a fight, a talk, or the choice).
	if event.Key() == forageKey && g.forageHolder != nil && !g.dead() {
		_ = g.forageHolder.Forage()

		return true
	}

	if event.Key() == d2enum.KeyEscape {
		// T2: Escape closes the kit panel first, like any other panel.
		if g.hud != nil && g.hud.kit != nil && g.hud.kit.open {
			g.hud.kit.open = false
			return true
		}

		// T3: and the talent panel.
		if g.hud != nil && g.hud.talents != nil && g.hud.talents.open {
			g.hud.talents.open = false
			return true
		}

		g.onEscKey()

		return true
	}

	// T2: nothing happens before he has chosen his loadout.
	if g.kitKey(event) {
		return true
	}

	gameEvent := g.keyMap.getGameEvent(event.Key())

	switch gameEvent {
	case d2enum.ClearScreen:
		g.clearScreen()
		g.updateLayout()
	case d2enum.ToggleInventoryPanel:
		g.inventoryAction()
	case d2enum.TogglePartyPanel:
		if !g.isSinglePlayer {
			g.togglePartyPanel()
		}
	case d2enum.ToggleSkillTreePanel:
		g.skillsAction()
	case d2enum.ToggleCharacterPanel:
		g.toggleHeroStatsPanel()
	case d2enum.ToggleQuestLog:
		g.questAction()
	case d2enum.ToggleRunWalk:
		g.hud.onToggleRunButton(false)
	case d2enum.HoldRun:
		g.hud.onToggleRunButton(true)
	case d2enum.CycleSquad:
		g.cycleSquad()
	case d2enum.CombatStrike:
		g.combatStrike()
	case d2enum.CombatTorch:
		g.combatTorch()
	case d2enum.CombatEndTurn:
		g.combatEndTurn()
	case d2enum.ToggleHelpScreen:
		g.toggleHelpOverlay()
	default:
		return false
	}

	return false
}

// OnKeyUp handles key release
func (g *GameControls) OnKeyUp(event d2interface.KeyEvent) bool {
	// J1 (review C6): a key let go under the open journal does nothing --
	// releasing the run key would otherwise toggle run and walk.
	if g.journalOpen() {
		return true
	}

	gameEvent := g.keyMap.getGameEvent(event.Key())

	if gameEvent == d2enum.HoldRun {
		g.hud.onToggleRunButton(true)
	}

	return false
}

// When escape is pressed:
// 1. If there was some overlay or panel open, close it
// 2. Otherwise, if the Escape Menu was open, let the Escape Menu handle it
// 3. If nothing was open, open the Escape Menu
func (g *GameControls) onEscKey() {
	escHandled := false

	escHandled = g.hasOpenPanels() || g.HelpOverlay.IsOpen() || g.hud.skillSelectMenu.IsOpen()
	g.clearScreen()

	if escHandled {
		g.updateLayout()
		return
	}

	if g.escapeMenu.IsOpen() {
		g.escapeMenu.OnEscKey()
	} else {
		g.openEscMenu()
	}
}

func truncateFloat64(n float64) float64 {
	const ten = 10.0
	return float64(int(n*ten)) / ten
}

// OnMouseButtonRepeat handles repeated mouse clicks
func (g *GameControls) OnMouseButtonRepeat(event d2interface.MouseEvent) bool {
	if g.dead() || g.talking() || g.journalOpen() {
		return true
	}

	const (
		screenWidth, screenHeight         = 800, 600
		halfScreenWidth, halfScreenHeight = screenWidth / 2, screenHeight / 2
		subtilesPerTile                   = 5
	)

	px, py := g.mapRenderer.ScreenToWorld(event.X(), event.Y())
	px = truncateFloat64(px)
	py = truncateFloat64(py)

	now := g.clock
	button := event.Button()
	isLeft := button == d2enum.MouseButtonLeft
	isRight := button == d2enum.MouseButtonRight
	inRect := !g.isInActiveMenusRect(event.X(), event.Y())
	shouldDoLeft := repeatDue(now, g.lastLeftBtnActionTime)
	shouldDoRight := repeatDue(now, g.lastRightBtnActionTime)

	// T2: nothing walks or casts while he is choosing his loadout, and a held
	// button over the kit panel is a click on the panel (review finding).
	if g.kitHolder != nil && (g.kitHolder.ChoosingLoadout() || g.overKitPanel(event.X(), event.Y())) {
		return true
	}

	if g.overTalentPanel(event.X(), event.Y()) {
		return true
	}

	// T1: no held-button walking inside a paced fight. A Move is one click,
	// checked against his turn and his range; a held button would re-send it
	// every repeat and earn a refusal each time.
	if g.combat != nil && g.combat.Fighting() && g.combat.Paced() {
		return true
	}

	if isLeft && shouldDoLeft && inRect && !g.hero.IsCasting() {
		g.lastLeftBtnActionTime = now

		// A HELD click over a squad model must not walk the hero either. The
		// single-click path consumes the select by returning before
		// OnPlayerMove (OnMouseButtonDown below); THIS is a SECOND caller of
		// OnPlayerMove, reached once the button is down past
		// mouseBtnActionsThreshold (0.25 s, repeatDue above, on the controls' clock),
		// so without this guard holding the button on a model orders exactly
		// the walk clause 10 forbids. Found by the c-1 review, 15 Sep 2026.
		//
		// Asserted by playtest/squads_test.go act 9 (history item 123): a
		// 40-frame held click on a second squad's model walks nobody, and
		// with this guard disabled the same hold walks him. (Until 24 Sep a
		// paused held click never made repeatDue true, so nothing reached
		// this line -- BUG-7.)
		if g.squadAtScreen(event.X(), event.Y()) != "" {
			return true
		}

		if event.KeyMod() == d2enum.KeyModShift {
			g.inputListener.OnPlayerCast(g.hero.LeftSkill.ID, px, py)
		} else {
			g.inputListener.OnPlayerMove(px, py)
		}

		if g.FreeCam {
			if event.Button() == d2enum.MouseButtonLeft {
				camVect := g.mapRenderer.Camera.GetPosition().Vector

				x := float64(halfScreenWidth) / subtilesPerTile
				y := float64(halfScreenHeight) / subtilesPerTile

				targetPosition := d2vector.NewPositionTile(x, y)
				targetPosition.Add(&camVect)

				g.mapRenderer.SetCameraTarget(&targetPosition)

				return true
			}
		}

		return true
	}

	if isRight && shouldDoRight && inRect && !g.hero.IsCasting() {
		g.lastRightBtnActionTime = now

		g.inputListener.OnPlayerCast(g.hero.RightSkill.ID, px, py)

		return true
	}

	return true
}

// OnMouseMove handles mouse movement events
func (g *GameControls) OnMouseMove(event d2interface.MouseMoveEvent) bool {
	mx, my := event.X(), event.Y()
	g.lastMouseX = mx
	g.lastMouseY = my
	g.inventory.lastMouseX = mx
	g.inventory.lastMouseY = my

	for i := range g.actionableRegions {
		// Mouse over a game control element
		if g.actionableRegions[i].rect.IsInRect(mx, my) {
			g.onHoverActionable(g.actionableRegions[i].actionableTypeID)
		}
	}

	g.hud.OnMouseMove(event)

	if g.PartyPanel != nil {
		g.PartyPanel.OnMouseMove(event)
	}

	return false
}

// OnMouseButtonUp handles mouse button presses
func (g *GameControls) OnMouseButtonUp(event d2interface.MouseEvent) bool {
	return false
}

// OnMouseButtonDown handles mouse button presses
func (g *GameControls) OnMouseButtonDown(event d2interface.MouseEvent) bool {
	mx, my := event.X(), event.Y()

	if g.dead() {
		return true
	}

	// J1: the open journal is modal -- a click on a tab or a row, and
	// nothing reaches the world under it (A3).
	if g.journalOpen() {
		if event.Button() == d2enum.MouseButtonLeft {
			g.journalClick(mx, my)
		}

		return true
	}

	// T4: a talk is modal -- a click answers or does nothing -- and a left
	// click on a villager in reach opens one.
	if g.talking() {
		if event.Button() == d2enum.MouseButtonLeft {
			g.talkClick(mx, my)
		}

		return true
	}

	if event.Button() == d2enum.MouseButtonLeft && g.talkClick(mx, my) {
		return true
	}

	// T2: the loadout choice is modal, and a click on the kit panel is a
	// click on the kit.
	// T3: the talent panel takes its own clicks, and nothing walks through it.
	// Tested BEFORE the kit, which it overlaps (review finding).
	if g.overTalentPanel(mx, my) {
		if event.Button() == d2enum.MouseButtonLeft {
			g.talentClick(mx, my)
		}

		return true
	}

	if event.Button() == d2enum.MouseButtonLeft && g.kitClick(mx, my) {
		return true
	}

	// And no right-click cast through the choice or the panel either.
	if g.kitHolder != nil && (g.kitHolder.ChoosingLoadout() || g.overKitPanel(mx, my)) {
		return true
	}

	for i := range g.actionableRegions {
		// If click is on a game control element
		if g.actionableRegions[i].rect.IsInRect(mx, my) {
			g.onClickActionable(g.actionableRegions[i].actionableTypeID)
			return false
		}
	}

	if g.hud.skillSelectMenu.IsOpen() && event.Button() == d2enum.MouseButtonLeft {
		g.lastLeftBtnActionTime = g.clock
		g.hud.skillSelectMenu.HandleClick(mx, my)
		g.hud.skillSelectMenu.ClosePanels()

		return false
	}

	px, py := g.mapRenderer.ScreenToWorld(mx, my)
	px = truncateFloat64(px)
	py = truncateFloat64(py)

	if event.Button() == d2enum.MouseButtonLeft && !g.isInActiveMenusRect(mx, my) && !g.hero.IsCasting() {
		g.lastLeftBtnActionTime = g.clock

		// A select-click on a squad model selects that squad and opens its
		// sheet, and is CONSUMED by returning BEFORE OnPlayerMove, so it does
		// not also order a walk (brief §4.5, clause 10). The EARLY RETURN is
		// what stops the walk -- returning true does not, because escapeMenu and
		// gameControls bind at the same input priority (brief §3.11).
		if squadID := g.squadAtScreen(mx, my); squadID != "" {
			g.selectSquad(squadID)
			return true
		}

		// T1: a click on an enemy in a paced fight is a strike (or a walk to
		// one). Consumed by returning BEFORE OnPlayerMove, like the squad select.
		if enemyID := g.tacticalEnemyAt(mx, my); enemyID != "" {
			g.inputListener.OnTacticalTarget(enemyID)
			return true
		}

		// A click on the combat panel is a click on the panel, not on the
		// tile behind it.
		if g.inTacticalPanel(mx, my) {
			return true
		}

		// A left click on empty ground orders a walk and closes the sheet
		// (ruled ask 6: the sheet closes on deselect).
		g.closeSquadSheet()

		if event.KeyMod() == d2enum.KeyModShift {
			g.inputListener.OnPlayerCast(g.hero.LeftSkill.ID, px, py)
		} else {
			g.inputListener.OnPlayerMove(px, py)
		}

		return true
	}

	if event.Button() == d2enum.MouseButtonRight && !g.isInActiveMenusRect(mx, my) && !g.hero.IsCasting() {
		g.lastRightBtnActionTime = g.clock

		g.inputListener.OnPlayerCast(g.hero.RightSkill.ID, px, py)

		return true
	}

	return false
}

func (g *GameControls) clearLeftScreenSide() {
	g.heroStatsPanel.Close()

	if g.PartyPanel != nil {
		g.PartyPanel.Close()
	}

	g.questLog.Close()
	g.hud.skillSelectMenu.ClosePanels()
	g.updateLayout()
}

func (g *GameControls) clearRightScreenSide() {
	g.inventory.Close()
	g.skilltree.Close()
	g.hud.skillSelectMenu.ClosePanels()
	g.updateLayout()
}

func (g *GameControls) clearScreen() {
	g.clearRightScreenSide()
	g.clearLeftScreenSide()
	g.hud.skillSelectMenu.ClosePanels()
	g.HelpOverlay.Close()
}

// defaultTorchesCarried is the [DIAL] at 1.
const defaultTorchesCarried = 1

// combatStrike is F. It commits the Action against the first living adjacent
// enemy in D8 order -- the policy's own target, exactly -- so the key
// reproduces what the stand-in would have done and the only NEW choice is the
// click, which is c-2b's.
//
// EACH OF THESE THREE HANDLERS OPENS WITH THE MENU GUARD, and section 0
// measured that the guard is currently UNREACHABLE -- which contradicts the
// brief and is recorded rather than quietly built around.
//
// The brief's premise was that "G and H work under an open escape menu today".
// They do not, at this HEAD. Measured twice, two keys, counters read directly:
// CycleSquad under an open menu did not change the selection, and F under an
// open menu moved neither commits_by_input nor commits_refused. The key is
// consumed upstream and never reaches OnKeyDown at all.
//
// The line stays for two reasons and neither is "just in case". First, it is
// the only thing that makes this handler correct ON ITS OWN, rather than
// correct because something else upstream happens to be. Second, input routing
// is not this milestone's to depend on: a later change to the escape menu's
// consumption would make the guard load-bearing without anyone editing these
// three functions.
//
// WHAT IT COSTS, NAMED: the brief's negative control 12 -- "F/L/E lose their
// escapeMenu.IsOpen() line, and act 2's menu control goes red" -- CANNOT FIRE,
// because the menu blocks the key before the missing guard would matter. An
// assertion that cannot fail is this project's named disease, so act 2's menu
// control asserts the behaviour that IS observable (the key is consumed
// upstream: no commit and no refusal) against the paired positive case (the
// same F with the menu closed does commit).
//
// A refused commit is silent here on purpose. commits_refused counts it and
// the harness reports it; the strip that would SAY so is c-2b's, and inventing
// a message channel for it now would be building c-2b early.
func (g *GameControls) combatStrike() {
	if g.escapeMenu.IsOpen() || g.combat == nil {
		return
	}

	_ = g.combat.Commit(d2world.CommitStrike, "")
}

// combatTorch is L, and it is ONE key: light if there is no carried source,
// relight if there is an unlit one with burn left, douse if it is lit.
//
// Out of a fight it is free and real-time. IN a fight it costs the Action --
// the one place rationing bites -- and a refused commit must change nothing,
// which is why the light is not touched until the commit has been accepted.
// Douse KEEPS THE BURN: S1 §4 says a source burns "only while lit", and the
// 12 Sep ruling's Light.Remove belongs to the burnt-out torch, not to douse.
func (g *GameControls) combatTorch() {
	if g.escapeMenu.IsOpen() || g.light == nil {
		return
	}

	carried := g.light.Carried()

	// Decide what the key means before anything is spent.
	var choice string

	// T2: with a kit, the torch is the one in his off-hand -- sword-and-board
	// has none, and that is the dilemma the loadout chose. Without a kit (a
	// harness or unit build that never bound one) the old counter stands.
	var kitTorch *d2items.Instance

	// With a kit, the only torch he can light or douse is the one in his hand.
	if carried != nil && g.kitHolder != nil && g.kitHolder.Kit() != nil {
		if _, _, ok := g.kitHolder.Kit().OffHandTorch(); !ok {
			g.TacticalNotice(TacticalNoTorch)
			return
		}
	}

	switch {
	case carried == nil && g.kitHolder != nil && g.kitHolder.Kit() != nil:
		_, inst, ok := g.kitHolder.Kit().OffHandTorch()
		if !ok || inst.BurnLeft <= 0 {
			g.TacticalNotice(TacticalNoTorch)
			return // refused: nothing in his off-hand to light
		}

		kitTorch = inst
		choice = d2world.CommitLight
	case carried == nil:
		if g.torchesCarried <= 0 {
			return // refused: no torch remains
		}

		choice = d2world.CommitLight
	case carried.Lit:
		choice = d2world.CommitDouse
	case carried.Burn > 0:
		choice = d2world.CommitLight
	default:
		return // a burnt-out torch lights nothing
	}

	if g.combat != nil && g.combat.Awaiting() {
		if err := g.combat.Commit(choice, ""); err != nil {
			return
		}
	}

	if carried == nil {
		src := g.light.Add(d2world.SourceTorch, true, 0, 0)

		// The light model owns the burn from here; the kit's minutes are
		// what this torch had left when it was last put away.
		//
		// OWNERSHIP MOVES, it is not copied: the kit keeps zero from here, so
		// whatever takes the lit torch away -- burning out, being put away, a
		// script removing the source -- leaves the kit nothing stale to
		// relight. Measured: a copied burn relit a "spent" torch at 60 minutes.
		if kitTorch != nil {
			src.Burn = kitTorch.BurnLeft
			kitTorch.BurnLeft = 0
		} else {
			g.torchesCarried--
		}

		return
	}

	carried.Lit = choice == d2world.CommitLight
}

// combatEndTurn is E, and it is one key with two verbs behind it: hold ends a
// turn whose Action is UNSPENT (its signed meaning) and end closes one whose
// Action is already spent. The gap end fills had no verb at all until it was
// ruled on 17 September 2026 -- Commit is refused unless a turn is waiting,
// and hold is defined as the unspent case, so nothing could close a turn after
// a strike or a torch.
func (g *GameControls) combatEndTurn() {
	if g.escapeMenu.IsOpen() || g.combat == nil {
		return
	}

	choice := d2world.CommitHold
	if g.combat.ActionSpent() {
		choice = d2world.CommitEnd
	}

	_ = g.combat.Commit(choice, "")
}

func (g *GameControls) openLeftPanel(panel Panel) {
	if !g.HelpOverlay.IsOpen() && !g.escapeMenu.IsOpen() {
		isOpen := panel.IsOpen()

		g.clearLeftScreenSide()

		if !isOpen {
			panel.Open()
			g.updateLayout()
		}
	}
}

func (g *GameControls) openRightPanel(panel Panel) {
	if !g.HelpOverlay.IsOpen() && !g.escapeMenu.IsOpen() {
		isOpen := panel.IsOpen()

		g.clearRightScreenSide()

		if !isOpen {
			panel.Open()
			g.updateLayout()
		}
	}
}

func (g *GameControls) toggleHeroStatsPanel() {
	g.openLeftPanel(g.heroStatsPanel)
}

func (g *GameControls) togglePartyPanel() {
	g.openLeftPanel(g.PartyPanel)
}

func (g *GameControls) onCloseHeroStatsPanel() {
	g.updateLayout()
}

func (g *GameControls) toggleLeftSkillPanel() {
	if !g.HelpOverlay.IsOpen() {
		g.clearScreen()
		g.hud.skillSelectMenu.ToggleLeftPanel()
	}
}

func (g *GameControls) toggleRightSkillPanel() {
	if !g.HelpOverlay.IsOpen() {
		g.clearScreen()
		g.hud.skillSelectMenu.ToggleRightPanel()
	}
}

func (g *GameControls) toggleQuestLog() {
	g.openLeftPanel(g.questLog)
}

func (g *GameControls) onCloseQuestLog() {
	g.updateLayout()
}

func (g *GameControls) toggleHelpOverlay() {
	if !g.isRightPanelOpen() || g.isLeftPanelOpen() {
		g.HelpOverlay.updateKeyMap(g.keyMap)
		g.hud.skillSelectMenu.ClosePanels()
		g.hud.miniPanel.openDisabled()
		g.HelpOverlay.Toggle()
		g.updateLayout()
	}
}

func (g *GameControls) toggleInventoryPanel() {
	g.openRightPanel(g.inventory)
}

// inventoryAction is the inventory key AND the mini-panel's inventory button:
// Strigoi's kit panel whenever the game screen holds the kit (T2 -- always, in
// the game; before a loadout is chosen the kit panel simply does not open),
// Diablo II's grid only with no kit holder at all.
// The button used to open the grid regardless -- Diablo II's inventory, with
// OpenDiablo2's test items in it, one click away in the default game (found
// 23 Sep 2026).
func (g *GameControls) inventoryAction() {
	if g.kitHolder != nil {
		g.toggleKitPanel()
		return
	}

	g.toggleInventoryPanel()
}

// skillsAction is the skill key and the mini-panel's skill button: Strigoi's
// talent tree when progress is bound (T3), Diablo II's skill tree otherwise.
func (g *GameControls) skillsAction() {
	if g.progressHolder != nil {
		g.toggleTalentPanel()
		return
	}

	g.toggleSkilltreePanel()
}

func (g *GameControls) onCloseInventory() {
	g.updateLayout()
}

func (g *GameControls) toggleSkilltreePanel() {
	g.openRightPanel(g.skilltree)
}

func (g *GameControls) onCloseSkilltree() {
	g.updateLayout()
}

func (g *GameControls) openEscMenu() {
	g.clearScreen()
	g.hud.miniPanel.closeDisabled()
	g.escapeMenu.open()
	g.updateLayout()
}

// Load the resources required for the GameControls
func (g *GameControls) Load() {
	g.hud.Load()

	// Diablo II's inventory grid, skill tree, character panel and quest log
	// load when first opened (their Open), not here: Strigoi's kit and
	// talents replace the first two, and a game that never opens the others
	// never reads their art from the MPQs (history item 114).

	if g.PartyPanel != nil {
		g.PartyPanel.Load()
	}

	g.HelpOverlay.Load()

	g.loadAddButtons()
	g.setAddButtons()

	miniPanelActions := &miniPanelActions{
		characterToggle: g.toggleHeroStatsPanel,
		partyToggle:     g.togglePartyPanel,
		inventoryToggle: g.inventoryAction,
		skilltreeToggle: g.skillsAction,
		menuToggle:      g.openEscMenu,
		questToggle:     g.questAction,
	}
	g.hud.miniPanel.load(miniPanelActions)
}

// Advance advances the state of the GameControls
func (g *GameControls) Advance(elapsed float64) error {
	g.clock += elapsed

	g.mapRenderer.Advance(elapsed)
	g.hud.Advance(elapsed)
	g.hud.advanceJournal(elapsed)
	g.inventory.Advance(elapsed)
	g.questLog.Advance(elapsed)

	if g.PartyPanel != nil {
		g.PartyPanel.Advance(elapsed)
	}

	if err := g.escapeMenu.Advance(elapsed); err != nil {
		return err
	}

	if g.heroStatsPanel.IsOpen() || g.skilltree.IsOpen() {
		g.setAddButtons()
	}

	return nil
}

func (g *GameControls) updateLayout() {
	isRightPanelOpen := g.isLeftPanelOpen()
	isLeftPanelOpen := g.isRightPanelOpen()

	switch {
	case isRightPanelOpen == isLeftPanelOpen:
		g.hud.miniPanel.ResetPosition()
		g.mapRenderer.ViewportDefault()
	case isRightPanelOpen:
		g.hud.miniPanel.SetMovedRight(true)
		g.mapRenderer.ViewportToLeft()
	case isLeftPanelOpen:
		g.hud.miniPanel.SetMovedLeft(true)
		g.mapRenderer.ViewportToRight()
	}
}

func (g *GameControls) isLeftPanelOpen() bool {
	var partyPanel bool

	if g.PartyPanel != nil {
		partyPanel = g.PartyPanel.IsOpen()
	} else {
		partyPanel = false
	}

	return g.heroStatsPanel.IsOpen() || partyPanel || g.questLog.IsOpen() || g.inventory.moveGoldPanel.IsOpen()
}

func (g *GameControls) isRightPanelOpen() bool {
	return g.inventory.IsOpen() || g.skilltree.IsOpen()
}

func (g *GameControls) hasOpenPanels() bool {
	return g.isRightPanelOpen() || g.isLeftPanelOpen() || g.hud.skillSelectMenu.IsOpen()
}

func (g *GameControls) isInActiveMenusRect(px, py int) bool {
	if g.bottomMenuRect.IsInRect(px, py) {
		return true
	}

	if g.isLeftPanelOpen() && g.leftMenuRect.IsInRect(px, py) {
		return true
	}

	if g.isRightPanelOpen() && g.rightMenuRect.IsInRect(px, py) {
		return true
	}

	if g.hud.miniPanel.IsOpen() && g.hud.miniPanel.IsInRect(px, py) {
		return true
	}

	if g.escapeMenu.IsOpen() {
		return true
	}

	if g.overKitPanel(px, py) || g.overTalentPanel(px, py) {
		return true
	}

	if g.HelpOverlay.IsOpen() && g.HelpOverlay.IsInRect(px, py) {
		return true
	}

	if g.hud.skillSelectMenu.IsOpen() {
		return true
	}

	return false
}

// Render draws the GameControls onto the target
func (g *GameControls) Render(target d2interface.Surface) error {
	if err := g.hud.Render(target); err != nil {
		return err
	}

	if err := g.renderPanels(target); err != nil {
		return err
	}

	if err := g.escapeMenu.Render(target); err != nil {
		return err
	}

	return nil
}

func (g *GameControls) renderPanels(target d2interface.Surface) error {
	g.inventory.Render(target)

	return nil
}

// SetZoneChangeText sets the zoneChangeText
func (g *GameControls) SetZoneChangeText(text string) {
	g.hud.zoneChangeText.SetText(text)
}

// ShowZoneChangeText shows the zoneChangeText
func (g *GameControls) ShowZoneChangeText() {
	g.hud.isZoneTextShown = true
}

// HideZoneChangeTextAfter hides the zoneChangeText after the given amount of seconds
func (g *GameControls) HideZoneChangeTextAfter(delay float64) {
	time.AfterFunc(time.Duration(delay)*time.Second, func() {
		g.hud.isZoneTextShown = false
	})
}

// HpStatsIsVisible returns true if the hp and mana stats are visible to the player
func (g *GameControls) HpStatsIsVisible() bool {
	return g.hud.hpStatsIsVisible
}

// ManaStatsIsVisible returns true if the hp and mana stats are visible to the player
func (g *GameControls) ManaStatsIsVisible() bool {
	return g.hud.manaStatsIsVisible
}

// ToggleHpStats toggles the visibility of the hp and mana stats placed above their respective globe and load only if they do not match
func (g *GameControls) ToggleHpStats() {
	g.hud.hpStatsIsVisible = !g.hud.hpStatsIsVisible
}

// ToggleManaStats toggles the visibility of the hp and mana stats placed above their respective globe
func (g *GameControls) ToggleManaStats() {
	g.hud.manaStatsIsVisible = !g.hud.manaStatsIsVisible
}

// Handles what to do when an actionable is hovered
func (g *GameControls) onHoverActionable(item actionableType) {
	hoverMap := map[actionableType]func(){
		leftSkill:  func() {},
		xp:         func() {},
		stamina:    func() {},
		rightSkill: func() {},
		hpGlobe:    func() {},
		manaGlobe:  func() {},
	}

	onHover, found := hoverMap[item]
	if !found {
		g.Errorf("Unrecognized actionableType(%d) being hovered", item)
		return
	}

	onHover()
}

// Handles what to do when an actionable is clicked
func (g *GameControls) onClickActionable(item actionableType) {
	actionMap := map[actionableType]func(){
		leftSkill: func() {
			g.toggleLeftSkillPanel()
		},

		xp: func() {
			g.Info("XP Action Pressed")
		},

		stamina: func() {
			g.Info("Stamina Action Pressed")
		},

		rightSkill: func() {
			g.toggleRightSkillPanel()
		},

		hpGlobe: func() {
			g.ToggleHpStats()
			g.Info("HP Globe Pressed")
		},

		manaGlobe: func() {
			g.ToggleManaStats()
			g.Info("Mana Globe Pressed")
		},
	}

	action, found := actionMap[item]
	if !found {
		// Warning, because some action types are still todo, and could return this error
		g.Warningf("Unrecognized actionableType(%d) being clicked", item)
		return
	}

	action()
}

func (g *GameControls) bindTerminalCommands(term d2interface.Terminal) error {
	if err := term.Bind("freecam", "toggle free camera movement", nil, g.commandFreeCam); err != nil {
		return err
	}

	if err := term.Bind("setleftskill", "set skill to fire on left click", []string{"id"}, g.commandSetLeftSkill(term)); err != nil {
		return err
	}

	if err := term.Bind("setrightskill", "set skill to fire on right click", []string{"id"}, g.commandSetRightSkill(term)); err != nil {
		return err
	}

	if err := term.Bind("learnskills", "learn all skills for the a given class", []string{"token"}, g.commandLearnSkills(term)); err != nil {
		return err
	}

	if err := term.Bind("learnskillid", "learn a skill by a given ID", []string{"id"}, g.commandLearnSkillID(term)); err != nil {
		return err
	}

	return nil
}

// UnbindTerminalCommands unbinds commands from the terminal
func (g *GameControls) UnbindTerminalCommands(term d2interface.Terminal) error {
	return term.Unbind("freecam", "setleftskill", "setrightskill", "learnskills", "learnskillid")
}

func (g *GameControls) setAddButtons() {
	g.hud.addStatsButton.SetEnabled(g.hero.Stats.StatsPoints > 0)
	g.hud.addSkillButton.SetEnabled(g.hero.Stats.SkillPoints > 0)
}

func (g *GameControls) loadAddButtons() {
	g.hud.addStatsButton.OnActivated(func() { g.toggleHeroStatsPanel() })
	g.hud.addSkillButton.OnActivated(func() { g.skillsAction() })
}

func (g *GameControls) commandFreeCam([]string) error {
	g.FreeCam = !g.FreeCam

	return nil
}

func (g *GameControls) commandSetLeftSkill(term d2interface.Terminal) func(args []string) error {
	return func(args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			term.Errorf("invalid argument")
			return nil
		}

		skill, err := g.heroSkillByID(id)
		if err != nil {
			term.Errorf(err.Error())
			return nil
		}

		g.hero.LeftSkill = skill

		return nil
	}
}

func (g *GameControls) commandSetRightSkill(term d2interface.Terminal) func(args []string) error {
	return func(args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			term.Errorf("invalid argument")
			return nil
		}

		skill, err := g.heroSkillByID(id)
		if err != nil {
			term.Errorf(err.Error())
			return nil
		}

		g.hero.RightSkill = skill

		return nil
	}
}

func (g *GameControls) commandLearnSkillID(term d2interface.Terminal) func(args []string) error {
	return func(args []string) error {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			term.Errorf("invalid argument")
			return nil
		}

		skill, err := g.heroSkillByID(id)
		if err != nil {
			term.Errorf(err.Error())
			return nil
		}

		g.hero.Skills[skill.ID] = skill
		g.hud.skillSelectMenu.RegenerateImageCache()
		g.Infof("Learned skill: %s", skill.Skill)

		return nil
	}
}

func (g *GameControls) heroSkillByID(id int) (*d2hero.HeroSkill, error) {
	skillRecord := g.asset.Records.Skill.Details[id]
	if skillRecord == nil {
		return nil, fmt.Errorf("cannot find a skill record for ID: %d", id)
	}

	skill, err := g.heroState.CreateHeroSkill(1, skillRecord.Skill)
	if err != nil {
		return nil, fmt.Errorf("cannot create skill with ID of %d", id)
	}

	return skill, nil
}

func (g *GameControls) commandLearnSkills(term d2interface.Terminal) func(args []string) error {
	const classTokenLength = 3

	return func(args []string) error {
		token := args[0]
		if len(token) < classTokenLength {
			term.Errorf("The given class token should be at least 3 characters")
			return nil
		}

		validPrefixes := []string{"ama", "ass", "nec", "bar", "sor", "dru", "pal"}
		classToken := strings.ToLower(token)
		tokenPrefix := classToken[0:3]
		isValidToken := false

		for idx := range validPrefixes {
			if strings.Compare(tokenPrefix, validPrefixes[idx]) == 0 {
				isValidToken = true
			}
		}

		if !isValidToken {
			fmtInvalid := "Invalid class, must be a value starting with(case insensitive): %s"
			term.Errorf(fmtInvalid, strings.Join(validPrefixes, ", "))

			return nil
		}

		var err error

		learnedSkillsCount := 0

		for _, skillDetailRecord := range g.asset.Records.Skill.Details {
			if skillDetailRecord.Charclass != classToken {
				continue
			}

			if skill, ok := g.hero.Skills[skillDetailRecord.ID]; ok {
				skill.SkillPoints++
				learnedSkillsCount++
			} else {
				skill, skillErr := g.heroState.CreateHeroSkill(1, skillDetailRecord.Skill)
				if skill == nil {
					continue
				}

				learnedSkillsCount++

				g.hero.Skills[skill.ID] = skill

				if skillErr != nil {
					err = skillErr
					break
				}
			}
		}

		g.hud.skillSelectMenu.RegenerateImageCache()
		g.Infof("Learned %d skills", learnedSkillsCount)

		if err != nil {
			term.Errorf("cannot learn skill for class, error: %s", err)
			return nil
		}

		return nil
	}
}
