package d2player

import (
	"fmt"
	"math"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapentity"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2world"
)

const (
	runButtonX = 255
	runButtonY = 570
)

const (
	zoneChangeTextX = screenWidth / 2
	zoneChangeTextY = screenHeight / 4
)

const (
	expBarWidth          = 120.0
	expBarHeight         = 4
	staminaBarWidth      = 102.0
	staminaBarHeight     = 19.0
	hoverLabelOuterPad   = 5
	percentStaminaBarLow = 0.25
)

const (
	frameHealthStatus      = 0
	frameManaStatus        = 1
	frameNewStatsSelector  = 1
	frameStamina           = 2
	framePotions           = 3
	frameNewSkillsSelector = 4
	frameRightGlobeHolder  = 5
	frameRightGlobe        = 1
)

const (
	staminaBarOffsetX  = 273
	staminaBarOffsetY  = 572
	staminaExperienceY = 535

	experienceBarOffsetX = 256
	experienceBarOffsetY = 561

	rightGlobeOffsetX = 8
	rightGlobeOffsetY = -8

	miniPanelButtonOffsetX = -8
	miniPanelButtonOffsetY = -38

	miniPanelTooltipOffsetX = 7
	miniPanelTooltipOffsetY = -14
)

const (
	lightBrownAlpha72 = 0xaf8848c8
	redAlpha72        = 0xff0000c8
	whiteAlpha100     = 0xffffffff
)

const (
	addStatsButtonX, addStatsButtonY = 206, 561
	addSkillButtonX, addSkillButtonY = 563, 561
)

// The always-visible clock strip (M4.4a). Sized from the §0 Font16
// measurement: the widest full line is 686px, so the whole strip is one line
// at the top-left, well inside the 800px viewport. Occlusion by an open panel
// is accepted (ruling 11 Sep).
const (
	clockStripX      = 8
	clockStripY      = 2
	clockStripWidth  = 700
	clockStripHeight = 18
)

// HUD represents the always visible user interface of the game
type HUD struct {
	actionableRegions  []actionableRegion
	asset              *d2asset.AssetManager
	uiManager          *d2ui.UIManager
	mapEngine          *d2mapengine.MapEngine
	mapRenderer        *d2maprenderer.MapRenderer
	lastMouseX         int
	lastMouseY         int
	hero               *d2mapentity.Player
	mainPanel          *d2ui.Sprite
	globeSprite        *d2ui.Sprite
	hpManaStatusSprite *d2ui.Sprite
	leftSkillResource  *SkillResource
	rightSkillResource *SkillResource
	runButton          *d2ui.Button
	zoneChangeText     *d2ui.Label
	miniPanel          *miniPanel
	isZoneTextShown    bool
	hpStatsIsVisible   bool
	manaStatsIsVisible bool
	skillSelectMenu    *SkillSelectMenu
	staminaTooltip     *d2ui.Tooltip
	runWalkTooltip     *d2ui.Tooltip
	experienceTooltip  *d2ui.Tooltip
	nameLabel          *d2ui.Label
	healthGlobe        *globeWidget
	manaGlobe          *globeWidget
	widgetStamina      *d2ui.CustomWidget
	widgetExperience   *d2ui.CustomWidget
	widgetLeftSkill    *d2ui.CustomWidget
	widgetRightSkill   *d2ui.CustomWidget
	panelBackground    *d2ui.CustomWidget
	addStatsButton     *d2ui.Button
	addSkillButton     *d2ui.Button
	panelGroup         *d2ui.WidgetGroup
	gameControls       *GameControls

	// The clock strip (M4.4a): the world clock read at construction, an
	// invisible Font16 label rendered manually inside a CustomWidget in
	// panelGroup (seam B), and the strings it last computed. The strings are
	// also what the "ui" harness provider reports, so a playtest can assert
	// what the player sees. lastStripMinute gates the refresh on the integer
	// minute-of-day changing, so the readout updates once a world minute
	// rather than every frame.
	clock            *d2world.Clock
	clockStrip       *d2ui.Label
	clockStripWidget *d2ui.CustomWidget
	stripDate        string
	stripFeast       string
	stripMoon        string
	stripText        string
	stripHoursToDusk float64
	lastStripMinute  int

	// The overhead bars (M4.4c-1): one uncached CustomWidget in panelGroup
	// draws a thin bar over the player's squad and over beasts and men, from
	// the bar source the game screen provides. overheadBars is the projected
	// cache the render callback draws and the "ui" harness provider reports,
	// so a playtest can assert on the same rects the player sees.
	bars              BarSource
	overheadBarWidget *d2ui.CustomWidget
	overheadBars      []overheadBarRender

	// T1: the tactical overlay -- tile diamonds and the combat panel -- drawn
	// by one full-viewport foreground widget while a paced fight runs.
	tactical       *tacticalOverlay
	tacticalWidget *d2ui.CustomWidget

	// T2: the loadout choice and the kit panel, one foreground widget.
	kit       *kitOverlay
	kitWidget *d2ui.CustomWidget

	// T3: the talent panel.
	talents       *talentOverlay
	talentsWidget *d2ui.CustomWidget

	// T4: the talk panel.
	talk       *talkOverlay
	talkWidget *d2ui.CustomWidget

	// J1: the journal, and its notice line.
	journal       *journalOverlay
	journalWidget *d2ui.CustomWidget

	// Death screen v0: over everything.
	death       *deathOverlay
	deathWidget *d2ui.CustomWidget

	// The selected-squad sheet (M4.4c-1, ruled ask 2/6): a panel of cards drawn
	// by sheetWidget when sheetOpen, its Font16 lines painted through the
	// invisible sheetLabel. It opens on selection and closes on deselect.
	sheetOpen   bool
	sheetLabel  *d2ui.Label
	sheetWidget *d2ui.CustomWidget

	*d2util.Logger
}

// NewHUD creates a HUD object
func NewHUD(
	asset *d2asset.AssetManager,
	ui *d2ui.UIManager,
	hero *d2mapentity.Player,
	miniPanel *miniPanel,
	actionableRegions []actionableRegion,
	mapEngine *d2mapengine.MapEngine,
	l d2util.LogLevel,
	gameControls *GameControls,
	mapRenderer *d2maprenderer.MapRenderer,
	clock *d2world.Clock,
	bars BarSource,
) *HUD {
	nameLabel := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
	nameLabel.Alignment = d2ui.HorizontalAlignCenter
	nameLabel.SetText(d2ui.ColorTokenize("", d2ui.ColorTokenServer))

	zoneLabel := ui.NewLabel(d2resource.Font30, d2resource.PaletteUnits)
	zoneLabel.Alignment = d2ui.HorizontalAlignCenter

	// The clock strip's label. It stays invisible (like nameLabel): the strip
	// is drawn by clockStripWidget's render function, not by the UIManager, so
	// making it visible here would double-render it.
	clockStrip := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
	clockStrip.Alignment = d2ui.HorizontalAlignLeft

	// The selected-squad sheet's label (M4.4c-1). Invisible like clockStrip: the
	// sheet is drawn by sheetWidget's render func, not by the UIManager.
	sheetLabel := ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
	sheetLabel.Alignment = d2ui.HorizontalAlignLeft

	healthGlobe := newGlobeWidget(ui, asset,
		0, screenHeight,
		typeHealthGlobe,
		&hero.Stats.Health, &hero.Stats.MaxHealth,
		l)
	manaGlobe := newGlobeWidget(ui, asset,
		screenWidth-manaGlobeScreenOffsetX, screenHeight,
		typeManaGlobe,
		&hero.Stats.Mana, &hero.Stats.MaxMana,
		l)

	hud := &HUD{
		asset:             asset,
		uiManager:         ui,
		hero:              hero,
		mapEngine:         mapEngine,
		mapRenderer:       mapRenderer,
		miniPanel:         miniPanel,
		actionableRegions: actionableRegions,
		nameLabel:         nameLabel,
		skillSelectMenu:   NewSkillSelectMenu(asset, ui, l, hero),
		zoneChangeText:    zoneLabel,
		healthGlobe:       healthGlobe,
		manaGlobe:         manaGlobe,
		gameControls:      gameControls,
		clock:             clock,
		clockStrip:        clockStrip,
		lastStripMinute:   -1,
		bars:              bars,
		sheetLabel:        sheetLabel,
		tactical:          newTacticalOverlay(ui),
		kit:               newKitOverlay(ui),
		talents:           newTalentOverlay(ui),
		death:             newDeathOverlay(ui),
		talk:              newTalkOverlay(ui),
		journal:           newJournalOverlay(ui),
	}

	hud.Logger = d2util.NewLogger()
	hud.Logger.SetPrefix(logPrefix)
	hud.Logger.SetLevel(l)

	return hud
}

// Load creates the ui elemets
func (h *HUD) Load() {
	h.panelGroup = h.uiManager.NewWidgetGroup(d2ui.RenderPriorityHUDPanel)

	h.loadSprites()

	h.healthGlobe.load()
	h.healthGlobe.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.healthGlobe)

	h.manaGlobe.load()
	h.manaGlobe.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.manaGlobe)

	h.loadTooltips()
	h.loadSkillResources()
	h.loadCustomWidgets()
	h.loadUIButtons()

	// nolint:gomnd // dividing by 2 (const)
	h.addStatsButton = h.uiManager.NewButton(d2ui.ButtonTypeAddSkill, "")
	h.addStatsButton.SetPosition(addStatsButtonX, addStatsButtonY)
	h.addStatsButton.SetVisible(false)
	bw, bh := h.addStatsButton.GetSize()
	statsTooltip := h.uiManager.NewTooltip(d2resource.Font16, d2resource.PaletteSky, d2ui.TooltipXCenter, d2ui.TooltipYTop)
	statsTooltip.SetPosition(addStatsButtonX+bw/2, addStatsButtonY-bh/2)
	statsTooltip.SetText(h.asset.TranslateString("strlvlup"))
	h.addStatsButton.SetTooltip(statsTooltip)
	h.panelGroup.AddWidget(h.addStatsButton)

	h.addSkillButton = h.uiManager.NewButton(d2ui.ButtonTypeAddSkill, "")
	h.addSkillButton.SetPosition(addSkillButtonX, addSkillButtonY)
	h.addSkillButton.SetVisible(false)
	bw, bh = h.addSkillButton.GetSize()
	skillTooltip := h.uiManager.NewTooltip(d2resource.Font16, d2resource.PaletteSky, d2ui.TooltipXCenter, d2ui.TooltipYTop)
	skillTooltip.SetPosition(addSkillButtonX+bw/2, addSkillButtonY-bh/2)
	skillTooltip.SetText(h.asset.TranslateString("strnewskl"))
	h.addSkillButton.SetTooltip(skillTooltip)
	h.panelGroup.AddWidget(h.addSkillButton)

	h.panelGroup.SetVisible(true)
}

func (h *HUD) loadCustomWidgets() {
	// static background
	_, height, err := h.mainPanel.GetFrameSize(0) // health globe is the frame with max height
	if err != nil {
		h.Error(err.Error())
		return
	}

	h.panelBackground = h.uiManager.NewCustomWidgetCached(h.renderPanelStatic, screenWidth, height)
	h.panelBackground.SetPosition(0, screenHeight-height)
	h.panelGroup.AddWidget(h.panelBackground)

	// stamina bar
	h.widgetStamina = h.uiManager.NewCustomWidget(h.renderStaminaBar, staminaBarWidth, staminaBarHeight)
	h.widgetStamina.SetPosition(staminaBarOffsetX, staminaBarOffsetY)
	h.widgetStamina.SetTooltip(h.staminaTooltip)
	h.panelGroup.AddWidget(h.widgetStamina)

	// experience bar
	h.widgetExperience = h.uiManager.NewCustomWidget(h.renderExperienceBar, expBarWidth, expBarHeight)
	h.widgetExperience.SetPosition(experienceBarOffsetX, experienceBarOffsetY)
	h.widgetExperience.SetTooltip(h.experienceTooltip)
	h.panelGroup.AddWidget(h.widgetExperience)

	// Left skill widget
	leftRenderFunc := func(target d2interface.Surface) {
		x, y := h.widgetLeftSkill.GetPosition()
		h.renderLeftSkill(x, y, target)
	}

	h.widgetLeftSkill = h.uiManager.NewCustomWidget(leftRenderFunc, skillIconWidth, skillIconHeight)
	h.widgetLeftSkill.SetPosition(leftSkillX, screenHeight)
	h.panelGroup.AddWidget(h.widgetLeftSkill)

	// Right skill widget
	rightRenderFunc := func(target d2interface.Surface) {
		x, y := h.widgetRightSkill.GetPosition()
		h.renderRightSkill(x, y, target)
	}

	h.widgetRightSkill = h.uiManager.NewCustomWidget(rightRenderFunc, skillIconWidth, skillIconHeight)
	h.widgetRightSkill.SetPosition(rightSkillX, screenHeight)
	h.panelGroup.AddWidget(h.widgetRightSkill)

	// The always-visible clock strip (M4.4a, seam B). Drawn last, over the map,
	// so nothing but an open panel occludes it.
	h.clockStripWidget = h.uiManager.NewCustomWidget(h.renderClockStrip, clockStripWidth, clockStripHeight)
	h.clockStripWidget.SetPosition(clockStripX, clockStripY)
	h.clockStripWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.clockStripWidget)

	// The overhead bars (M4.4c-1). ONE widget for all bars, declared at the
	// full viewport, positioned (0,0), foreground priority so nothing but an
	// open panel occludes it; the callback iterates the cache and DrawRects.
	// No tooltip: a CustomWidget swallows no clicks and its declared size is
	// nearly inert (brief §3.10), so the full-viewport size costs nothing.
	h.overheadBarWidget = h.uiManager.NewCustomWidget(h.renderOverheadBars, screenWidth, screenHeight)
	h.overheadBarWidget.SetPosition(0, 0)
	h.overheadBarWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.overheadBarWidget)

	// The selected-squad sheet (M4.4c-1). A full-viewport foreground widget that
	// draws the cards when the sheet is open; sized from §0 part 1.
	h.sheetWidget = h.uiManager.NewCustomWidget(h.renderSquadSheet, screenWidth, screenHeight)
	h.sheetWidget.SetPosition(0, 0)
	h.sheetWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.sheetWidget)

	// T1: the tactical overlay. Full viewport, foreground, and it draws nothing
	// at all outside a paced fight.
	h.tacticalWidget = h.uiManager.NewCustomWidget(h.renderTactical, screenWidth, screenHeight)
	h.tacticalWidget.SetPosition(0, 0)
	h.tacticalWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.tacticalWidget)

	// T2: the kit panel and the loadout choice, drawn over everything else.
	h.kitWidget = h.uiManager.NewCustomWidget(h.renderKit, screenWidth, screenHeight)
	h.kitWidget.SetPosition(0, 0)
	h.kitWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.kitWidget)

	// T3: the talent panel.
	h.talentsWidget = h.uiManager.NewCustomWidget(h.renderTalents, screenWidth, screenHeight)
	h.talentsWidget.SetPosition(0, 0)
	h.talentsWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.talentsWidget)

	// T4: the talk panel, under the death screen.
	h.talkWidget = h.uiManager.NewCustomWidget(h.renderTalk, screenWidth, screenHeight)
	h.talkWidget.SetPosition(0, 0)
	h.talkWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.talkWidget)

	// J1: the journal, over the other panels and under the death screen.
	h.journalWidget = h.uiManager.NewCustomWidget(h.renderJournal, screenWidth, screenHeight)
	h.journalWidget.SetPosition(0, 0)
	h.journalWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.journalWidget)

	// Death screen v0, added last so it draws over every other panel.
	h.deathWidget = h.uiManager.NewCustomWidget(h.renderDeath, screenWidth, screenHeight)
	h.deathWidget.SetPosition(0, 0)
	h.deathWidget.SetRenderPriority(d2ui.RenderPriorityForeground)
	h.panelGroup.AddWidget(h.deathWidget)
}

func (h *HUD) loadSkillResources() {
	genericSkillsSprite, err := h.uiManager.NewSprite(d2resource.GenericSkills, d2resource.PaletteSky)
	if err != nil {
		h.Error(err.Error())
	}

	attackIconID := 2

	h.leftSkillResource = &SkillResource{
		SkillIcon:         genericSkillsSprite,
		IconNumber:        attackIconID,
		SkillResourcePath: d2resource.GenericSkills,
	}

	h.rightSkillResource = &SkillResource{
		SkillIcon:         genericSkillsSprite,
		IconNumber:        attackIconID,
		SkillResourcePath: d2resource.GenericSkills,
	}
}

func (h *HUD) loadSprites() {
	var err error

	h.globeSprite, err = h.uiManager.NewSprite(d2resource.GameGlobeOverlap, d2resource.PaletteSky)
	if err != nil {
		h.Error(err.Error())
	}

	h.hpManaStatusSprite, err = h.uiManager.NewSprite(d2resource.HealthManaIndicator, d2resource.PaletteSky)
	if err != nil {
		h.Error(err.Error())
	}

	h.mainPanel, err = h.uiManager.NewSprite(d2resource.GamePanels, d2resource.PaletteSky)
	if err != nil {
		h.Error(err.Error())
	}
}

func (h *HUD) loadTooltips() {
	// stamina tooltip
	h.staminaTooltip = h.uiManager.NewTooltip(d2resource.Font16, d2resource.PaletteSky, d2ui.TooltipXCenter, d2ui.TooltipYTop)
	rect := &h.actionableRegions[stamina].rect

	halfButtonWidth := rect.Width >> 1
	centerX := rect.Left + halfButtonWidth

	_, labelHeight := h.staminaTooltip.GetSize()
	halfLabelHeight := labelHeight >> 1

	labelX := centerX
	labelY := staminaExperienceY - halfLabelHeight
	h.staminaTooltip.SetPosition(labelX, labelY)

	// experience tooltip
	h.experienceTooltip = h.uiManager.NewTooltip(d2resource.Font16, d2resource.PaletteSky, d2ui.TooltipXCenter, d2ui.TooltipYTop)
	rect = &h.actionableRegions[stamina].rect

	halfButtonWidth = rect.Width >> 1
	centerX = rect.Left + halfButtonWidth

	_, labelHeight = h.experienceTooltip.GetSize()
	halfLabelHeight = labelHeight >> 1

	labelX = centerX
	labelY = staminaExperienceY - halfLabelHeight
	h.experienceTooltip.SetPosition(labelX, labelY)
}

func (h *HUD) loadUIButtons() {
	// Run button
	h.runButton = h.uiManager.NewButton(d2ui.ButtonTypeRun, "")
	h.runButton.SetPosition(runButtonX, runButtonY)
	h.runButton.OnActivated(func() { h.onToggleRunButton(false) })

	h.runWalkTooltip = h.uiManager.NewTooltip(d2resource.Font16, d2resource.PaletteSky, d2ui.TooltipXCenter, d2ui.TooltipYTop)
	// we must set text first, and then we're getting its height
	h.updateRunTooltipText()

	bw, bh := h.runButton.GetSize()
	_, lh := h.runWalkTooltip.GetSize()
	// nolint:gomnd // dividing by 2 (const)
	labelX := runButtonX + bw/2
	// nolint:gomnd // dividing by 2 (const)
	labelY := runButtonY - bh/2 - lh/2

	h.runWalkTooltip.SetPosition(labelX, labelY)
	h.runButton.SetTooltip(h.runWalkTooltip)

	h.panelGroup.AddWidget(h.runButton)

	if h.hero.IsRunToggled() {
		h.runButton.Toggle()
	}
}

func (h *HUD) updateRunTooltipText() {
	var stringTableKey string
	if h.hero.IsRunToggled() {
		stringTableKey = "RunOff"
	} else {
		stringTableKey = "RunOn"
	}

	h.runWalkTooltip.SetText(h.asset.TranslateString(stringTableKey))
}

func (h *HUD) onToggleRunButton(noButton bool) {
	// J1 (review B1): the run button is a d2ui widget and takes a click under
	// the open journal whatever the controls answer; it does nothing there.
	// (It cannot simply be disabled: it has no disabled face, and drawing it
	// disabled panics -- measured 24 Sep.)
	if h.journal != nil && h.journal.open {
		return
	}

	if !noButton {
		h.runButton.Toggle()
	}

	h.hero.ToggleRunWalk()
	h.updateRunTooltipText()

	h.hero.SetIsRunning(h.hero.IsRunToggled())
}

// NOTE: the positioning of all of the panel elements is coupled to the rendering order :(
// don't change the order in which the render methods are called, as there is an x,y offset
// that is updated between render calls
func (h *HUD) renderPanelStatic(target d2interface.Surface) {
	_, height := target.GetSize()
	offsetX, offsetY := 0, height

	// Main panel background
	if err := h.renderPanel(offsetX, offsetY, target); err != nil {
		h.Error(err.Error())
		return
	}

	// New Stats Button
	w, _ := h.mainPanel.GetCurrentFrameSize()
	offsetX += w + skillIconWidth

	if err := h.renderNewStatsButton(offsetX, offsetY, target); err != nil {
		h.Error(err.Error())
		return
	}

	// Stamina
	w, _ = h.mainPanel.GetCurrentFrameSize()
	offsetX += w

	if err := h.renderStamina(offsetX, offsetY, target); err != nil {
		h.Error(err.Error())
		return
	}

	// Potions
	w, _ = h.mainPanel.GetCurrentFrameSize()
	offsetX += w

	if err := h.renderPotions(offsetX, offsetY, target); err != nil {
		h.Error(err.Error())
		return
	}

	// New Skills Button
	w, _ = h.mainPanel.GetCurrentFrameSize()
	offsetX += w

	if err := h.renderNewSkillsButton(offsetX, offsetY, target); err != nil {
		h.Error(err.Error())
		return
	}

	// Empty Mana Globe
	w, _ = h.mainPanel.GetCurrentFrameSize()
	offsetX += w + skillIconWidth

	if err := h.mainPanel.SetCurrentFrame(frameRightGlobeHolder); err != nil {
		h.Error(err.Error())
		return
	}

	h.mainPanel.SetPosition(offsetX, height)
	h.mainPanel.Render(target)
}

func (h *HUD) renderPanel(x, y int, target d2interface.Surface) error {
	if err := h.mainPanel.SetCurrentFrame(0); err != nil {
		return err
	}

	h.mainPanel.SetPosition(x, y)
	h.mainPanel.Render(target)

	return nil
}

func (h *HUD) renderLeftSkill(x, y int, target d2interface.Surface) {
	newSkillResourcePath := h.getSkillResourceByClass(h.hero.LeftSkill.Charclass)
	if newSkillResourcePath != h.leftSkillResource.SkillResourcePath {
		h.leftSkillResource.SkillResourcePath = newSkillResourcePath
		h.leftSkillResource.SkillIcon, _ = h.uiManager.NewSprite(newSkillResourcePath, d2resource.PaletteSky)
	}

	if err := h.leftSkillResource.SkillIcon.SetCurrentFrame(h.hero.LeftSkill.IconCel); err != nil {
		h.Error(err.Error())
		return
	}

	h.leftSkillResource.SkillIcon.SetPosition(x, y)
	h.leftSkillResource.SkillIcon.Render(target)
}

func (h *HUD) renderRightSkill(x, _ int, target d2interface.Surface) {
	_, height := target.GetSize()

	newSkillResourcePath := h.getSkillResourceByClass(h.hero.RightSkill.Charclass)
	if newSkillResourcePath != h.rightSkillResource.SkillResourcePath {
		h.rightSkillResource.SkillIcon, _ = h.uiManager.NewSprite(newSkillResourcePath, d2resource.PaletteSky)
		h.rightSkillResource.SkillResourcePath = newSkillResourcePath
	}

	if err := h.rightSkillResource.SkillIcon.SetCurrentFrame(h.hero.RightSkill.IconCel); err != nil {
		h.Error(err.Error())
		return
	}

	h.rightSkillResource.SkillIcon.SetPosition(x, height)
	h.rightSkillResource.SkillIcon.Render(target)
}

func (h *HUD) renderNewStatsButton(x, y int, target d2interface.Surface) error {
	if err := h.mainPanel.SetCurrentFrame(frameNewStatsSelector); err != nil {
		return err
	}

	h.mainPanel.SetPosition(x, y)
	h.mainPanel.Render(target)

	return nil
}

func (h *HUD) renderStamina(x, y int, target d2interface.Surface) error {
	if err := h.mainPanel.SetCurrentFrame(frameStamina); err != nil {
		return err
	}

	h.mainPanel.SetPosition(x, y)
	h.mainPanel.Render(target)

	return nil
}

func (h *HUD) renderStaminaBar(target d2interface.Surface) {
	target.PushTranslation(staminaBarOffsetX, staminaBarOffsetY)
	defer target.Pop()

	target.PushEffect(d2enum.DrawEffectModulate)
	defer target.Pop()

	staminaPercent := h.hero.Stats.Stamina / float64(h.hero.Stats.MaxStamina)

	staminaBarColor := d2util.Color(lightBrownAlpha72)
	if staminaPercent < percentStaminaBarLow {
		staminaBarColor = d2util.Color(redAlpha72)
	}

	target.DrawRect(int(staminaPercent*staminaBarWidth), staminaBarHeight, staminaBarColor)
}

func (h *HUD) renderExperienceBar(target d2interface.Surface) {
	target.PushTranslation(experienceBarOffsetX, experienceBarOffsetY)
	defer target.Pop()

	expPercent := float64(h.hero.Stats.Experience) / float64(h.hero.Stats.NextLevelExp)

	target.DrawRect(int(expPercent*expBarWidth), 2, d2util.Color(whiteAlpha100))
}

func (h *HUD) renderPotions(x, _ int, target d2interface.Surface) error {
	_, height := target.GetSize()

	if err := h.mainPanel.SetCurrentFrame(framePotions); err != nil {
		return err
	}

	h.mainPanel.SetPosition(x, height)
	h.mainPanel.Render(target)

	return nil
}

func (h *HUD) renderNewSkillsButton(x, _ int, target d2interface.Surface) error {
	_, height := target.GetSize()

	if err := h.mainPanel.SetCurrentFrame(frameNewSkillsSelector); err != nil {
		return err
	}

	h.mainPanel.SetPosition(x, height)
	h.mainPanel.Render(target)

	return nil
}

func (h *HUD) setStaminaTooltipText() {
	// Create and format Stamina string from string lookup table.
	fmtStamina := h.asset.TranslateString("panelstamina")
	staminaCurr, staminaMax := int(h.hero.Stats.Stamina), h.hero.Stats.MaxStamina
	strPanelStamina := fmt.Sprintf(fmtStamina, staminaCurr, staminaMax)

	h.staminaTooltip.SetText(strPanelStamina)
}

func (h *HUD) setExperienceTooltipText() {
	// Create and format Experience string from string lookup table.
	fmtExp := h.asset.TranslateString("panelexp")

	// The English string for "panelexp" is "Experience: %u / %u", however %u doesn't
	// translate well. So we need to rewrite %u into a formatable Go verb. %d is used in other
	// strings, so we go with that, keeping in mind that %u likely referred to
	// an unsigned integer.
	fmtExp = strings.ReplaceAll(fmtExp, "%u", "%d")

	expCurr, expMax := uint(h.hero.Stats.Experience), uint(h.hero.Stats.NextLevelExp)
	strPanelExp := fmt.Sprintf(fmtExp, expCurr, expMax)

	h.experienceTooltip.SetText(strPanelExp)
}

// hoveredEntity is the selectable entity under a screen point, or nil. T4
// lifted it out of the hover-label renderer so the talk click finds exactly
// the entity whose label the player is looking at.
func (h *HUD) hoveredEntity(mx, my int) d2interface.MapEntity {
	return h.hoveredEntityWhere(mx, my, nil)
}

// hoveredEntityWhere is the first selectable entity under a screen point that
// keep accepts (nil keeps any). The talk click asks for a VILLAGER, so a dog
// standing in front of the headman does not swallow the click meant for him
// (found by the T6 playtest: a pack's dog stood over his sprite).
func (h *HUD) hoveredEntityWhere(mx, my int, keep func(d2interface.MapEntity) bool) d2interface.MapEntity {
	for entityIdx := range h.mapEngine.Entities() {
		entity := (h.mapEngine.Entities())[entityIdx]
		if !entity.Selectable() || (keep != nil && !keep(entity)) {
			continue
		}

		entScreenXf, entScreenYf := h.mapRenderer.WorldToScreenF(entity.GetPositionF())
		entScreenX := int(math.Floor(entScreenXf))
		entScreenY := int(math.Floor(entScreenYf))
		entityWidth, entityHeight := entity.GetSize()
		halfWidth, halfHeight := entityWidth>>1, entityHeight>>1
		l, r := entScreenX-halfWidth-hoverLabelOuterPad, entScreenX+halfWidth+hoverLabelOuterPad
		t, b := entScreenY-halfHeight-hoverLabelOuterPad, entScreenY+halfHeight-hoverLabelOuterPad

		if l <= mx && r >= mx && t <= my && b >= my {
			return entity
		}
	}

	return nil
}

func (h *HUD) renderForSelectableEntitiesHovered(target d2interface.Surface) {
	// J1: nothing under the open journal is hovered.
	if h.journal != nil && h.journal.open {
		return
	}

	entity := h.hoverTarget(h.lastMouseX, h.lastMouseY)
	if entity == nil {
		return
	}

	entPos := entity.GetPosition()
	entOffset := entPos.RenderOffset()
	entScreenXf, entScreenYf := h.mapRenderer.WorldToScreenF(entity.GetPositionF())
	entScreenX := int(math.Floor(entScreenXf))
	entScreenY := int(math.Floor(entScreenYf))
	_, entityHeight := entity.GetSize()
	xOff, yOff := int(entOffset.X()), int(entOffset.Y())

	// T4: a villager's label is his ROLE, not the D2 stand-in's name; M4.7:
	// one of the dead is named once he knows them.
	label := entity.Label()
	if h.gameControls != nil {
		label = h.gameControls.hoverLabel(entity)
	}

	h.nameLabel.SetText(label)

	xLabel, yLabel := entScreenX-xOff, entScreenY-yOff-entityHeight-hoverLabelOuterPad
	h.nameLabel.SetPosition(xLabel, yLabel)

	h.nameLabel.Render(target)
	entity.Highlight()
}

// Render draws the HUD to the screen
func (h *HUD) Render(target d2interface.Surface) error {
	// M4.7: where the dead lie -- drawn first, so the HUD's own panels cover them.
	h.renderCorpses(target)

	h.renderForSelectableEntitiesHovered(target)

	if h.isZoneTextShown {
		h.zoneChangeText.SetPosition(zoneChangeTextX, zoneChangeTextY)
		h.zoneChangeText.Render(target)
	}

	if h.skillSelectMenu.IsOpen() {
		h.skillSelectMenu.Render(target)
	}

	return nil
}

func (h *HUD) getSkillResourceByClass(class string) string {
	resourceMap := map[string]string{
		"":    d2resource.GenericSkills,
		"bar": d2resource.BarbarianSkills,
		"nec": d2resource.NecromancerSkills,
		"pal": d2resource.PaladinSkills,
		"ass": d2resource.AssassinSkills,
		"sor": d2resource.SorcererSkills,
		"ama": d2resource.AmazonSkills,
		"dru": d2resource.DruidSkills,
	}

	entry, found := resourceMap[class]
	if !found {
		h.Errorf("Unknown class token: '%s'", class)
	}

	return entry
}

// refreshClockStrip recomputes the clock strip's text when the world clock's
// integer minute-of-day changes (the [DIAL] refresh trigger), so the strip
// updates once a world minute instead of every frame. It reads the date and
// weekday the clock computes, the feast and moon-phase names for today from
// the generated day table, and the time-to-sunset the clock counts down to
// DuskStart. Outside the slice the day table has no row, so the feast and moon
// text are simply omitted. MinuteOfDay is float64; the truncation is explicit.
func (h *HUD) refreshClockStrip() {
	if h.clock == nil || h.clockStrip == nil {
		return
	}

	minute := int(h.clock.MinuteOfDay())
	if minute == h.lastStripMinute {
		return
	}

	h.lastStripMinute = minute

	year, month, day := h.clock.Date()
	h.stripHoursToDusk = h.clock.HoursToDusk()
	h.stripDate = fmt.Sprintf("%s %d %s %d", h.clock.Weekday(), day, strigoiMonthName(month), year)

	h.stripFeast, h.stripMoon = "", ""
	if entry, ok := h.clock.Today(); ok {
		h.stripFeast, h.stripMoon = entry.Feast, entry.MoonPhase
	}

	parts := []string{h.stripDate}
	if h.stripFeast != "" {
		parts = append(parts, h.stripFeast)
	}

	parts = append(parts, strigoiSunsetLabel(h.stripHoursToDusk))

	if h.stripMoon != "" {
		parts = append(parts, h.stripMoon)
	}

	h.stripText = strings.Join(parts, strigoiStripSeparator)
	h.clockStrip.SetText(h.stripText)
}

// renderClockStrip draws the clock strip. The label is invisible to the
// UIManager, so this is the only thing that paints it (no double render).
func (h *HUD) renderClockStrip(target d2interface.Surface) {
	if h.clock == nil || h.clockStrip == nil || h.stripText == "" {
		return
	}

	h.clockStrip.SetPosition(clockStripX, clockStripY)
	h.clockStrip.Render(target)
}

// Advance updates syncs data on widgets that might have changed. I.e. the current stamina value
// in the stamina tooltip
func (h *HUD) Advance(elapsed float64) {
	h.refreshClockStrip()
	h.refreshOverheadBars()
	h.refreshTactical(elapsed)
	h.refreshKit()
	h.refreshTalents()
	h.refreshTalk()
	h.refreshDeath()
	h.setStaminaTooltipText()
	h.setExperienceTooltipText()

	if err := h.healthGlobe.Advance(elapsed); err != nil {
		h.Error(err.Error())
	}

	if err := h.manaGlobe.Advance(elapsed); err != nil {
		h.Error(err.Error())
	}
}

// OnMouseMove handles mouse move events
func (h *HUD) OnMouseMove(event d2interface.MouseMoveEvent) bool {
	mx, my := event.X(), event.Y()
	h.lastMouseX = mx
	h.lastMouseY = my

	h.skillSelectMenu.LeftPanel.HandleMouseMove(mx, my)
	h.skillSelectMenu.RightPanel.HandleMouseMove(mx, my)

	return false
}
