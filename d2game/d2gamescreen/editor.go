package d2gamescreen

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/png" // the palette's art is PNG; image.Decode needs the decoder registered
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2config"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2harness"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapgen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maprenderer"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2maptiled"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mapedit"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2mappalette"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2screen"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2ui"
)

// The World Editor screen (M5.4). Josh builds the world by clicking; Claude
// builds it by writing the same records; the .tmj file is the contract and this
// screen is a view onto it, not the source of truth.
//
// WHY IT IS A SCREEN AND NOT A PANEL. d2ui has no modal or input-capture
// concept at all: the UIManager and every widget bind at PriorityDefault, and
// propagate() (d2core/d2input/input_manager.go) stops only at a priority
// boundary, so returning true from a handler does NOT stop a d2ui.Button
// underneath -- the "d2ui-button hole" that BUG-26 fixed by hand for the talk
// panel. A screen swap hands out a fresh UIManager (d2screen's screen_manager),
// so an editor screen has no foreign clickable underneath it to fight. That is
// the whole reason for this shape.
//
// WHY IT DRAWS WITH FILLS AND LABELS. Every d2ui chrome widget -- Button,
// Checkbox, Scrollbar, TextBox, UIFrame -- needs a Diablo II .dc6. Only Label is
// Strigoi's own. So the editor draws like the journal does: one full-screen
// custom widget, fillRect for panels and edges, and a POOL of Labels allocated
// once in OnLoad and re-pointed each frame. Allocating a Label per frame is the
// mistake the journal's build note warns about.
//
// WHY THE MAP IS DRAWN BY THE REAL RENDERER. The editing view has to look like
// the game or it is lying about what you are building. So the screen keeps a
// real MapEngine, lays the document into it through d2mapgen.LayAuthoredMap --
// the same function the game uses -- and lets MapRenderer draw it, strips,
// draw order and all. After every edit the document is re-parsed by the
// engine's own d2maptiled.Parse and laid in again, so what is on screen is what
// the game would build from the file as it now stands.
const (
	editorScreenW, editorScreenH = 800, 600

	edPaletteW = 208 // the palette column down the right
	// edToolbarH holds TWO lines of verbs. One line does not fit: the whole
	// bound set measures wider than 800px in font16, and a toolbar that
	// truncates its own key list is the opposite of what it is for.
	edToolbarH = 44
	// edStatusH holds FOUR lines: the file, the message, the validator's count
	// and first problem, and the sealed-map warning when there is one.
	edStatusH       = 72
	edLineH         = 17 // one line of font16, with its leading
	edRowH          = 34 // one palette row: two lines beside a swatch
	edSwatch        = 28 // the thumbnail box in a palette row
	edPad           = 6
	edTabH          = 18
	edReasonLines   = 3  // how much of an unavailable tab's reason the column takes
	edScrollStep    = 3  // palette rows per wheel notch
	edMaxPaletteRow = 64 // the Label pool's size; a longer catalog scrolls
	edMaxPeople     = 32 // name labels for people; more are marked but not named
	edNoticeLines   = 7  // how much of a refused map's reason the notice shows

	// edLabelZoom is the zoom below which people's names are drawn only for
	// the one under the cursor and the one selected (the second 28 Sep
	// review: at the fit zoom the village's people stand a few pixels apart
	// and "start" was drawn over the smith's mark). Their marks are drawn at
	// every zoom. [DIAL]
	edLabelZoom = 0.2

	// edMinScale is the viewport's OWN minScale (viewport.go:36), not a
	// rounder number above it: a 48x48 map is 7680 ortho pixels wide and the
	// map's view is 592, so the whole village only fits at 0.077 -- an editor
	// floor of 0.1 would open on a map it could not show all of.
	edMinScale, edMaxScale = 0.0625, 2.0
	edZoomPerNotch         = 1.15
)

// The editor's own colours. It has no art, so every panel, edge and marker here
// is a fill.
const (
	edColPanel    = 0x0c0c14f0
	edColStrip    = 0x1a1a28f8
	edColEdge     = 0x50506aff
	edColRowPick  = 0x2a3c5aff
	edColSwatchBg = 0x000000ff
	edColText     = 0xd8d0c0ff
	edColDim      = 0x78786eff
	edColWarn     = 0xff5050ff
	edColGood     = 0x70ff90ff
	edColGrid     = 0x46566c70
	edColSelect   = 0xffe080ff
	edColGhostOK  = 0x70ff90ff
	edColGhostNo  = 0xff5050ff
	// edColVoid is the map view behind the map, so what is not the map is
	// one known colour rather than whatever the window held (the 28 Sep review's
	// screenshots showed white).
	edColVoid = 0x0a0a10ff
	// edColPerson and edColStart mark the people the map places and the
	// player_start (B5): the view draws no entities, so without a marker the
	// only way to find the smith was to drop a house on him.
	edColPerson = 0x60d8ffff
	edColStart  = 0xffb030ff
	// edColNotice is the panel a refused map's reason is drawn on (C).
	edColNotice = 0x1a0808f4

	// edDimBrightness is how far down a greyed row's art is drawn.
	edDimBrightness = 0.45
)

// editorVerbs is the toolbar, and it is the ONE place the key list is written
// down in code. docs/editor.md says the same thing in prose; OnKeyDown binds
// exactly these and nothing else.
var editorVerbs = [2]string{
	"left-click select / place    right-drag pan    wheel zoom    Tab palette tab    G grid",
	"Ctrl+Z undo    Ctrl+Y redo    Ctrl+S save    Del delete    D duplicate    P playtest    Esc back",
}

// editorTool is what a left-click on the map does.
type editorTool int

const (
	toolSelect editorTool = iota
	toolPlace
)

// Editor is the World Editor screen.
type Editor struct {
	asset        *d2asset.AssetManager
	renderer     d2interface.Renderer
	terminal     d2interface.Terminal
	inputManager d2interface.InputManager
	ui           *d2ui.UIManager
	navigator    d2interface.Navigator

	// mapPath is the map as the GAME reads it: a path its loader resolves in
	// its own folders ("data/strigoi/maps/village.tmj"). diskPath is the file
	// the editor reads and writes, ABSOLUTE and fixed at open (28 Sep review,
	// B1's hazard: the save target used to be relative to the working
	// directory, so it moved with it). assetRoot is the folder mapPath is
	// relative to. See editorResolve.
	mapPath   string
	diskPath  string
	assetRoot string

	doc     *d2mapedit.Doc
	stack   *d2mapedit.Stack
	catalog *d2mappalette.Catalog
	art     d2mapedit.Art
	// load is how the engine's parser reads the map's art: the game's own
	// loader, so rebuild, save and playtest all ask exactly what the game
	// would do with the file.
	load d2maptiled.Loader

	mapEngine   *d2mapengine.MapEngine
	mapRenderer *d2maprenderer.MapRenderer

	// The palette: the tab on show, what is picked, and where the list is
	// scrolled to.
	tabs      []d2mappalette.Tab
	tab       int
	rows      []d2mappalette.Entry
	rowTop    int
	picked    *d2mappalette.Entry
	thumbs    map[string]d2interface.Surface
	thumbFail map[string]bool

	// gids maps a catalog entry -- by its id, and by its resolved art path --
	// to the tileset gid THIS map spends on it. It is built with the document so
	// the per-frame placement ghost does not walk the tileset.
	gids map[string]int

	tool     editorTool
	selected int // an object id, 0 for nothing

	// Input state. There is no drag state anywhere in the engine, so the
	// anchor is tracked here.
	mouseX, mouseY     int
	panning            bool
	panLastX, panLastY int

	showGrid   bool
	leaveArmed bool // Escape has been pressed once on an unsaved map
	message    string
	problems   []d2mapedit.Problem
	sealed     bool // the last validate found the start walled in

	// reasonLines is the current tab's reason wrapped to the column, done once
	// per tab rather than once a frame; reasonFor is the tab it was wrapped for.
	reasonLines []string
	reasonFor   int

	// engineErr is why the game's own parser refuses the document as it now
	// stands, nil when it takes it; laidOnce is whether any version of it was
	// ever laid into the view. The notice in the map area is drawn from both
	// (28 Sep review, C: a refused map used to open as an empty grid with the
	// reason one line of the status bar).
	engineErr   error
	laidOnce    bool
	noticeLines []string
	noticeFor   string

	// loaded is whether OnLoad has run before: the screen is loaded again when
	// a playtest hands it back (A2), and then it keeps the view it had --
	// viewScale and viewCam, taken in OnUnload -- instead of re-fitting.
	loaded    bool
	viewScale float64
	viewCam   *d2vector.Position

	// A pool of Labels allocated once. Index 0..edMaxPaletteRow-1 are the
	// palette rows; the rest are named below.
	rowLabels  []*d2ui.Label
	tabLabels  []*d2ui.Label
	titleLbl   *d2ui.Label
	statusLbl  *d2ui.Label
	hintLbl    *d2ui.Label
	toolLbl    *d2ui.Label
	headerLbl  *d2ui.Label
	reasonLbl  *d2ui.Label
	problemLbl *d2ui.Label
	sealedLbl  *d2ui.Label
	countLbl   *d2ui.Label
	noticeLbl  *d2ui.Label
	personLbls []*d2ui.Label

	// peopleDrawn is where drawPeople put each person's mark and name on the
	// last frame, for the harness (editorProvider.people).
	peopleDrawn map[int]editorPersonDrawn

	canvas *d2ui.CustomWidget

	*d2util.Logger
	logLevel d2util.LogLevel
}

// CreateEditor opens mapPath in the World Editor. It fails rather than opening
// an empty screen: a designer who asked to edit a map and got a blank grid has
// been told a lie about his file.
func CreateEditor(
	mapPath string,
	asset *d2asset.AssetManager,
	term d2interface.Terminal,
	renderer d2interface.Renderer,
	inputManager d2interface.InputManager,
	ui *d2ui.UIManager,
	navigator d2interface.Navigator,
	l d2util.LogLevel,
) (*Editor, error) {
	disk, assetPath, root, workdir, err := editorResolve(mapPath, editorAssetRoots())
	if err != nil {
		return nil, err
	}

	if EditorOpenGuard != nil {
		if err := EditorOpenGuard(disk, workdir); err != nil {
			return nil, err
		}
	}

	doc, err := d2mapedit.OpenFile(disk)
	if err != nil {
		return nil, fmt.Errorf("opening %s in the editor: %w", disk, err)
	}

	e := &Editor{
		asset:        asset,
		renderer:     renderer,
		terminal:     term,
		inputManager: inputManager,
		ui:           ui,
		navigator:    navigator,
		mapPath:      assetPath,
		diskPath:     disk,
		assetRoot:    root,
		doc:          doc,
		stack:        d2mapedit.NewStack(doc),
		art:          d2mapedit.DirArt(filepath.Dir(disk)),
		load:         asset.LoadFile,
		thumbs:       map[string]d2interface.Surface{},
		thumbFail:    map[string]bool{},
		gids:         map[string]int{},
		showGrid:     true,
		tool:         toolSelect,
		logLevel:     l,
		reasonFor:    -1,
	}

	e.Logger = d2util.NewLogger()
	e.Logger.SetPrefix(logPrefix)
	e.Logger.SetLevel(l)

	e.catalog = d2mappalette.NewCatalog()

	// The catalog is rooted at the folder the GAME reads the map from and
	// handed the map's own path, not rooted at the map's own directory: the
	// village's tileset points at "../structures/village-well/intact.png" and
	// an fs.FS cannot be escaped upwards, so a catalog rooted at
	// data/strigoi/maps could not read half the art. Rooting it there also
	// makes Entry.ImagePath exactly the path AssetManager.LoadFile takes,
	// which is what the thumbnails are loaded by. (It was the working
	// directory until the 28 Sep review; for a game started from its own
	// folder that is the same place.)
	if err := e.catalog.ReadMap(os.DirFS(e.assetRoot), e.mapPath); err != nil {
		// A catalog that cannot be built is not fatal -- the map still opens,
		// and saying so is better than a palette that is silently empty.
		e.Warningf("the palette could not read %s: %v", e.mapPath, err)
		e.message = "the palette could not be built from this map: " + err.Error()
	}

	e.tabs = e.catalog.Tabs()
	e.refreshRows()
	e.refreshGIDs()

	return e, nil
}

// DefaultEditorMap is the map the editor opens when none is named: the village,
// which is the only .tmj that exists.
const DefaultEditorMap = "data/strigoi/maps/village.tmj"

// EditorOpenGuard, when it is set, may refuse a map before the editor opens it.
// The game leaves it nil. The playtest harness sets one (d2app/harness.go): the
// harness runs the game with the repository as its working directory, so a
// script that pressed Ctrl+S in an editor opened on a file in the source tree
// would have overwritten it (28 Sep review, B1, and the second review's B,
// which found the first guard covered village.tmj alone). disk is the absolute
// path editorResolve settled on; workdir says it got there by the
// working-directory fallback -- a relative path under none of the game's own
// folders -- rather than by one of those folders.
var EditorOpenGuard func(disk string, workdir bool) error //nolint:gochecknoglobals // a harness seam, nil in the game

// editorAssetRoots are the folders the game's loader reads loose files from, in
// its own order: the executable's folder, then %AppData%\OpenDiablo2 -- the two
// file-system sources App.Run adds (d2app/app.go), read the same way here so
// the two cannot disagree.
func editorAssetRoots() []string {
	return []string{filepath.Dir(d2config.LocalConfigPath()), filepath.Dir(d2config.DefaultConfigPath())}
}

// editorResolve turns the map the editor was asked for into the three things it
// works with: disk, the ABSOLUTE file it reads and writes; assetPath, the path
// the game's loader reads that same file by; and root, the folder assetPath is
// relative to (the catalog reads the art from there).
//
// WHY ABSOLUTE, AND WHY AT OPEN (28 Sep review, B1). The editor used to keep
// the path it was given, relative, and write to it relative to the working
// directory at the moment of the save. The playtest harness runs the game with
// the REPOSITORY as its working directory, so a scripted Ctrl+S under a bare
// -editor wrote the shipped village. Now the file is fixed once, here, and the
// save goes to that file whatever the working directory later becomes.
//
// A map under one of roots is read by the game from there, so its asset path is
// its path below that root. A RELATIVE path under none of them is what it
// always was: the game's loader reads it by that same relative path from its
// own folders, which for a game run from its own folder is the file itself.
// (Where the two differ -- a harness build lives in a temporary folder with a
// COPY of data/strigoi -- the playtest's read-back check catches it before a
// game starts on the wrong bytes; see playtest.) An ABSOLUTE path under none of
// them is refused: the game could not load it, nor the art beside it.
//
// workdir is true exactly when the file was found by that last, relative rule --
// under the working directory, not under one of roots. A game run from its own
// folder never needs it (its folder is roots[0]); the playtest harness, which
// runs the game from the REPOSITORY, refuses it (EditorOpenGuard).
func editorResolve(asked string, roots []string) (disk, assetPath, root string, workdir bool, err error) {
	if strings.TrimSpace(asked) == "" {
		asked = DefaultEditorMap
	}

	native := filepath.FromSlash(strings.ReplaceAll(asked, "\\", "/"))
	relative := !filepath.IsAbs(native)

	if relative {
		// The engine's slash-rooted spelling ("/data/strigoi/...") is a
		// game-relative path, as SetAuthoredMap and -map read it.
		native = filepath.FromSlash(editorAssetPath(asked))
	}

	abs, err := filepath.Abs(native)
	if err != nil {
		return "", "", "", false, fmt.Errorf("resolving %s: %w", asked, err)
	}

	disk = filepath.Clean(abs)

	for _, r := range roots {
		if strings.TrimSpace(r) == "" {
			continue
		}

		ra, err := filepath.Abs(r)
		if err != nil {
			continue
		}

		rel, err := filepath.Rel(ra, disk)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
			filepath.IsAbs(rel) {
			continue
		}

		return disk, filepath.ToSlash(rel), filepath.Clean(ra), false, nil
	}

	if relative {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", "", false, err
		}

		return disk, editorAssetPath(asked), cwd, true, nil
	}

	return "", "", "", false, fmt.Errorf("%s is outside the game's own folders (%s), so the game could not load it or "+
		"the art beside it; copy it under data/strigoi/maps in the game's folder", disk, strings.Join(roots, ", "))
}

// editorAssetPath is the path the ENGINE takes: forward slashes, cleaned, with
// no leading slash -- exactly the shape d2mapgen hands d2maptiled.Parse and
// AssetManager.LoadFile ("data/strigoi/maps/village.tmj").
func editorAssetPath(p string) string {
	return strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(p, "\\", "/")), "/")
}

// editorDiskPath turns the engine's slash-rooted asset path into a path on
// disk, RELATIVE to the working directory. The editor saved to this until the
// 28 Sep review (B1): a relative save target moves with the working directory,
// and the harness runs the game with the repository as its working directory.
// editorResolve now fixes the file as an absolute path at open; this is kept
// for the normalisation its test pins.
func editorDiskPath(p string) string {
	return filepath.FromSlash(editorAssetPath(p))
}

// OnLoad builds the engine map and the widgets.
func (e *Editor) OnLoad(loading d2screen.LoadingState) {
	if err := e.inputManager.BindHandler(e); err != nil {
		e.Error("the editor could not bind input: " + err.Error())
	}

	loading.Progress(twentyPercent)

	e.mapEngine = d2mapengine.CreateMapEngine(e.logLevel, e.asset)
	e.mapRenderer = d2maprenderer.CreateMapRenderer(e.asset, e.renderer, e.mapEngine,
		e.terminal, e.logLevel, 0.0, 0.0)

	loading.Progress(fiftyPercent)

	if err := e.rebuild(); err != nil {
		e.Error("the editor could not lay the map: " + err.Error())

		if !e.loaded {
			e.message = "this map does not load: " + editorOneLine(err)
		}
	}

	loading.Progress(seventyPercent)

	// The Label pool, allocated ONCE. A Label per frame is the journal's
	// documented mistake.
	e.rowLabels = make([]*d2ui.Label, edMaxPaletteRow)
	for i := range e.rowLabels {
		e.rowLabels[i] = e.newLabel()
	}

	e.tabLabels = make([]*d2ui.Label, len(e.tabs))
	for i := range e.tabLabels {
		e.tabLabels[i] = e.newLabel()
	}

	e.titleLbl = e.newLabel()
	e.statusLbl = e.newLabel()
	e.hintLbl = e.newLabel()
	e.toolLbl = e.newLabel()
	e.headerLbl = e.newLabel()
	e.reasonLbl = e.newLabel()
	e.problemLbl = e.newLabel()
	e.sealedLbl = e.newLabel()
	e.countLbl = e.newLabel()
	e.noticeLbl = e.newLabel()

	e.personLbls = make([]*d2ui.Label, edMaxPeople)
	for i := range e.personLbls {
		e.personLbls[i] = e.newLabel()
	}

	e.canvas = e.ui.NewCustomWidget(e.renderChrome, editorScreenW, editorScreenH)
	e.canvas.SetRenderPriority(d2ui.RenderPriorityForeground)

	// Back from a playtest (A2), the view is where it was left; the first time,
	// it opens on the whole map.
	if e.loaded && e.viewScale > 0 && e.viewCam != nil {
		e.mapRenderer.SetScale(e.viewScale)

		position := d2vector.NewPosition(e.viewCam.X(), e.viewCam.Y())
		e.mapRenderer.SetCameraPosition(&position)
	} else {
		e.centreOnStart()
	}

	e.revalidate()

	e.loaded = true

	// The harness's "editor" system while this screen lives; last, so every
	// widget it reports exists. OnUnload removes it.
	d2harness.Register(editorProvider{e})
}

// newLabel is one Label of the pool. NewLabel already registers it with the
// UIManager and leaves it INVISIBLE, so the manager never draws it; the chrome
// re-points it and calls Render itself, which is what lets one Label serve a
// different row every frame.
func (e *Editor) newLabel() *d2ui.Label {
	return e.ui.NewLabel(d2resource.Font16, d2resource.PaletteStatic)
}

// OnUnload releases the input binding, and keeps the view for the next load: a
// playtest unloads the editor and hands the same screen back afterwards (A2).
func (e *Editor) OnUnload() error {
	d2harness.Unregister(editorProvider{e})

	if e.mapRenderer != nil {
		e.viewScale = e.mapRenderer.Scale()

		if cam := e.mapRenderer.Camera.GetPosition(); cam != nil {
			position := d2vector.NewPosition(cam.X(), cam.Y())
			e.viewCam = &position
		}

		if err := e.mapRenderer.UnbindTerminalCommands(e.terminal); err != nil {
			e.Warningf("the editor could not unbind the map renderer's commands: %v", err)
		}
	}

	return e.inputManager.UnbindHandler(e)
}

// rebuild re-parses the document with the ENGINE's own parser and lays it into
// the engine, so the view is what the game would build from the file as it
// stands. It is the only place the engine map is written.
//
// Its verdict is kept (engineErr) and drawn over the map when it is a refusal:
// a map the engine refuses on open shows nothing to lay, and an edit it refuses
// leaves the view on the last version it took -- and either way the screen has
// to say so where the designer is looking, not in one line of the status bar.
func (e *Editor) rebuild() error {
	data, err := e.doc.Bytes()
	if err != nil {
		e.engineErr = err
		return err
	}

	m, err := d2maptiled.Parse(data, path.Dir(e.mapPath), e.load)
	e.engineErr = err

	if err != nil {
		return err
	}

	d2mapgen.LayAuthoredMap(e.mapEngine, m)
	e.mapRenderer.SetMapEngine(e.mapEngine)
	e.laidOnce = true

	return nil
}

// engine is the question Save and the playtest put to the game before they
// write anything: rebuild's own call -- the engine's parser with the game's
// loader -- over the exact bytes about to be written (28 Sep review, B2).
func (e *Editor) engine() d2mapedit.Engine {
	return d2mapedit.EngineParse(path.Dir(e.mapPath), e.load)
}

// dirty is whether the document differs from the file: the undo history knows
// where the last save sits in it, so undoing back to the saved map is clean
// again (28 Sep review, C).
func (e *Editor) dirty() bool {
	// An editor with no history has nothing unsaved. Only a unit test holds
	// one (d2app's playtest tests hand back a bare Editor); CreateEditor always
	// builds the stack.
	return e.stack != nil && e.stack.Dirty()
}

// editorOneLine is an error as one line of the status bar: the validator's
// refusal is a list, one problem a line, and a label draws each on its own row.
func editorOneLine(err error) string {
	if err == nil {
		return ""
	}

	return strings.Join(strings.Fields(err.Error()), " ")
}

// refreshGIDs indexes this map's tileset by the two things a palette entry can
// be matched on: the loader's own "<tileset>#<id>" name, which is exactly
// Entry.ID, and the art path resolved the way the loader resolves it, which is
// exactly Entry.ImagePath.
func (e *Editor) refreshGIDs() {
	dir := path.Dir(e.mapPath)

	for k := range e.gids {
		delete(e.gids, k)
	}

	for _, k := range e.doc.Kinds() {
		e.gids[k.Name] = k.GID

		if k.Image != "" {
			e.gids[path.Join(dir, strings.ReplaceAll(k.Image, "\\", "/"))] = k.GID
		}
	}
}

// mapView is the part of the screen the map is drawn in: everything the
// toolbar, the status area and the palette column leave.
func mapViewRect() image.Rectangle {
	return image.Rect(0, edToolbarH, editorScreenW-edPaletteW, editorScreenH-edStatusH)
}

// centreOnStart opens on the WHOLE map, with the player_start in the middle of
// the part of the screen the map is drawn in. The wheel takes it from there.
func (e *Editor) centreOnStart() {
	x, y := float64(e.doc.Size().X)/2, float64(e.doc.Size().Y)/2
	if s, ok := e.doc.Start(); ok {
		x, y = s.X, s.Y
	}

	view := mapViewRect()

	e.mapRenderer.SetScale(e.fitScale(view))

	// There is no SetCameraPositionTile. The camera's position is in ORTHO
	// pixels -- Viewport.getCameraOffset reads it straight -- so a tile is
	// turned into one by WorldToOrtho first. This is map_engine_testing's idiom.
	position := d2vector.NewPosition(e.mapRenderer.WorldToOrtho(x, y))
	e.mapRenderer.SetCameraPosition(&position)

	// And then ASK the renderer where that point actually landed, instead of
	// assuming it is the middle of the screen. It is not: getCameraOffset takes
	// the half screen off the camera UNSCALED and OrthoToScreenF puts it back
	// SCALED (viewport.go:235-236, :395-396), so the camera's own point drifts
	// from the middle as the zoom changes -- and the middle of the map's view is
	// not the middle of the window anyway, with a 208-pixel palette beside it.
	sx, sy := e.mapRenderer.WorldToScreen(x, y)
	e.panBy(view.Min.X+view.Dx()/2-sx, view.Min.Y+view.Dy()/2-sy)
}

// fitScale is the zoom at which the whole map fits the view, measured through
// the renderer's OWN WorldToOrtho rather than from the tile size: the viewport's
// ortho units are half a tile and its constants are unexported, so the honest
// way to ask how big the map is in them is to transform its corners.
func (e *Editor) fitScale(view image.Rectangle) float64 {
	w, h := float64(e.doc.Size().X), float64(e.doc.Size().Y)

	left, _ := e.mapRenderer.WorldToOrtho(0, h)
	right, _ := e.mapRenderer.WorldToOrtho(w, 0)
	_, top := e.mapRenderer.WorldToOrtho(0, 0)
	_, bottom := e.mapRenderer.WorldToOrtho(w, h)

	spanX, spanY := right-left, bottom-top
	if spanX <= 0 || spanY <= 0 {
		return 1
	}

	fit := math.Min(float64(view.Dx())/spanX, float64(view.Dy())/spanY)

	return math.Max(edMinScale, math.Min(edMaxScale, fit))
}

// ---- input -----------------------------------------------------------------

// OnMouseMove tracks the cursor and drags the camera.
func (e *Editor) OnMouseMove(event d2interface.MouseMoveEvent) bool {
	e.mouseX, e.mouseY = event.X(), event.Y()

	if e.panning {
		e.panBy(e.mouseX-e.panLastX, e.mouseY-e.panLastY)
		e.panLastX, e.panLastY = e.mouseX, e.mouseY
	}

	return false
}

// panBy drags the world by dx, dy SCREEN pixels.
//
// The camera moves the OTHER WAY, and divided by the scale. Both halves were
// read off Viewport.OrthoToScreenF, which is
//
//	screenX = (orthoX - camOrthoX - half)*scale + left + half
//
// so for a fixed world point to travel +dx across the screen, camOrthoX must
// FALL by dx/scale. Dragging right therefore moves the world right, which is
// what a hand on a map does. (game_controls.go:660-674 does this with
// halfScreenWidth in place of mouseX-halfScreenWidth and is broken; it was not
// copied.)
//
// HOW THE SIGN WAS CHECKED, 28 Sep 2026, in the running editor. centreOnStart
// moves the view with this same function, so the sign is testable by clicking:
// with the minus signs here, a click on the middle of the map's view answered
// "selected start at 23,28" -- the village's player_start, the tile the camera
// was aimed at. With them removed the correction doubles instead of cancelling,
// the village is thrown into the bottom-right corner, and the same click
// answers "tile 2,40: open".
func (e *Editor) panBy(dx, dy int) {
	scale := e.mapRenderer.Scale()
	if scale <= 0 {
		scale = 1
	}

	e.mapRenderer.MoveCameraBy(d2vector.NewVector(-float64(dx)/scale, -float64(dy)/scale))
}

// OnMouseButtonDown starts a pan, picks from the palette, or acts on the map.
func (e *Editor) OnMouseButtonDown(event d2interface.MouseEvent) bool {
	e.mouseX, e.mouseY = event.X(), event.Y()

	// The right button drags the view, everywhere. A right-click on the map in
	// the GAME is the torch verb; in the editor there is no torch, and a drag
	// is what a designer reaches for.
	if event.Button() == d2enum.MouseButtonRight {
		e.startPan()
		return true
	}

	if event.Button() != d2enum.MouseButtonLeft {
		return false
	}

	if e.mouseX >= editorScreenW-edPaletteW {
		e.clickPalette()
		return true
	}

	if e.mouseY < edToolbarH {
		return true // the toolbar draws its keys; it is not clickable in v0
	}

	e.clickMap()

	return true
}

// OnMouseButtonUp ends a pan.
func (e *Editor) OnMouseButtonUp(event d2interface.MouseEvent) bool {
	if event.Button() == d2enum.MouseButtonRight {
		e.panning = false
		return true
	}

	return false
}

func (e *Editor) startPan() {
	e.panning = true
	e.panLastX, e.panLastY = e.mouseX, e.mouseY
}

// OnMouseWheel zooms about the cursor. The engine had no wheel at all before
// this burst; the transform lives on the viewport so a click and a draw agree.
// Over the palette the same wheel scrolls the list, because that is what a
// wheel means over a list.
func (e *Editor) OnMouseWheel(event d2interface.MouseWheelEvent) bool {
	dy := event.ScrollY()
	if dy == 0 {
		return false
	}

	if event.X() >= editorScreenW-edPaletteW {
		e.scrollPalette(dy)
		return true
	}

	scale := e.mapRenderer.Scale()
	if scale <= 0 {
		scale = 1
	}

	if dy > 0 {
		scale *= edZoomPerNotch
	} else {
		scale /= edZoomPerNotch
	}

	e.zoomAbout(event.X(), event.Y(), scale)

	return true
}

// zoomAbout sets the zoom, clamped to the editor's range, holding the world
// point under the screen pixel x, y where it is. It is the wheel's whole effect,
// and the harness's "editor" zoom field calls it too, so a script that zooms
// goes through the same line the wheel does (the harness has no wheel verb).
func (e *Editor) zoomAbout(x, y int, scale float64) {
	scale = math.Max(edMinScale, math.Min(edMaxScale, scale))
	e.mapRenderer.ZoomAt(x, y, scale)
}

func (e *Editor) scrollPalette(dy float64) {
	step := edScrollStep
	if dy > 0 {
		step = -step
	}

	e.rowTop = editorClampTop(e.rowTop+step, len(e.rows), e.paletteVisibleRows())
}

// editorClampTop keeps the palette's scroll offset on the list: never past the
// last screenful, never below zero, and always zero when everything fits.
func editorClampTop(top, count, visible int) int {
	last := count - visible
	if last < 0 {
		last = 0
	}

	if top > last {
		top = last
	}

	if top < 0 {
		top = 0
	}

	return top
}

// OnKeyDown carries the editor's verbs. They are listed in docs/editor.md and
// drawn along the top of the screen, so a first-time user is not guessing.
func (e *Editor) OnKeyDown(event d2interface.KeyEvent) bool {
	ctrl := event.KeyMod()&d2enum.KeyModControl != 0

	switch {
	case event.Key() == d2enum.KeyEscape:
		if e.tool == toolPlace || e.picked != nil {
			e.tool, e.picked = toolSelect, nil
			e.message = "placement cancelled"

			return true
		}

		if e.selected != 0 {
			e.selected = 0
			return true
		}

		e.leave()

		return true

	case ctrl && event.Key() == d2enum.KeyZ:
		e.runUndo()
		return true

	case ctrl && event.Key() == d2enum.KeyY:
		e.runRedo()
		return true

	case ctrl && event.Key() == d2enum.KeyS:
		e.save()
		return true

	case event.Key() == d2enum.KeyDelete:
		e.deleteSelected()
		return true

	case event.Key() == d2enum.KeyG:
		e.showGrid = !e.showGrid
		return true

	case event.Key() == d2enum.KeyD:
		e.duplicateSelected()
		return true

	case event.Key() == d2enum.KeyP:
		e.playtest()
		return true

	case event.Key() == d2enum.KeyTab:
		e.cycleTab()
		return true
	}

	return false
}

// cycleTab moves to the next palette tab, wrapping. It is what Tab does, and
// the only way to reach a tab without the mouse.
func (e *Editor) cycleTab() {
	if len(e.tabs) == 0 {
		return
	}

	e.tab = (e.tab + 1) % len(e.tabs)
	e.rowTop = 0

	e.refreshRows()
}

// ---- the map ---------------------------------------------------------------

// hoverTile is the tile under the cursor, in whole tiles.
func (e *Editor) hoverTile() (int, int, bool) {
	if e.mouseX >= editorScreenW-edPaletteW || e.mouseY < edToolbarH ||
		e.mouseY > editorScreenH-edStatusH {
		return 0, 0, false
	}

	px, py := e.mapRenderer.ScreenToWorld(e.mouseX, e.mouseY)
	x, y := int(math.Floor(px)), int(math.Floor(py))

	if x < 0 || y < 0 || x >= e.doc.Size().X || y >= e.doc.Size().Y {
		return 0, 0, false
	}

	return x, y, true
}

func (e *Editor) clickMap() {
	x, y, ok := e.hoverTile()
	if !ok {
		return
	}

	if e.tool == toolPlace && e.picked != nil {
		e.place(x, y)
		return
	}

	// Select whatever is on that tile: a structure first, then a point object
	// standing on it.
	if s, ok := e.doc.StructureOn(x, y); ok {
		e.selected = s.ID
		e.message = fmt.Sprintf("selected %s at %d,%d", editorObjectName(s), x, y)

		return
	}

	for _, o := range e.doc.Objects() {
		if o.IsStructure() {
			continue
		}

		if t := o.Tile(); t.X == x && t.Y == y {
			e.selected = o.ID
			e.message = fmt.Sprintf("selected %s at %d,%d", editorObjectName(o), x, y)

			return
		}
	}

	e.selected = 0
	e.message = fmt.Sprintf("tile %d,%d: %s", x, y, e.tileWord(x, y))
}

func (e *Editor) tileWord(x, y int) string {
	parts := []string{}
	if e.doc.Blocked(x, y) {
		parts = append(parts, "blocked")
	} else {
		parts = append(parts, "open")
	}

	if e.doc.BlocksSight(x, y) {
		parts = append(parts, "blocks sight")
	}

	if e.doc.Inside(x, y) {
		parts = append(parts, "inside")
	}

	// Fog of war F4: raised ground and towers.
	if h := e.doc.HeightAt(x, y); h > 0 {
		parts = append(parts, fmt.Sprintf("height %d", h))
	}

	if r := e.doc.TowerSightAt(x, y); r > 0 {
		parts = append(parts, fmt.Sprintf("a tower: sees %d", r))
	}

	return strings.Join(parts, ", ")
}

// editorStructureAnchor turns the tile a designer CLICKED into the anchor
// Doc.PlaceStructure wants.
//
// A structure's anchor is its footprint's BOTTOM CORNER, which is where Tiled
// puts a tile object and where the loader lays the footprint back from:
// structure() builds image.Rect(x-w, y-h, x, y) (d2maptiled/tiled.go:1013-1014)
// and a Go rectangle's Max is EXCLUSIVE. So the anchor tile is NOT itself
// covered, and the last tile that is covered is the one before it on both axes.
// The shipped village agrees: its object 3 has footprint (27,18)-(30,21) and
// stands at tile (30,21).
//
// For the tile a designer clicked to be the footprint's front tile, the anchor
// is therefore one past it on both axes. TestEditorStructureAnchorIsOnePastTheClick
// pins that against the shipped village rather than against this comment.
func editorStructureAnchor(clickX, clickY int) (int, int) {
	return clickX + 1, clickY + 1
}

// editorFootprintAt is the loader's own footprint rectangle for an anchor.
func editorFootprintAt(anchorX, anchorY int, fp image.Point) image.Rectangle {
	return image.Rect(anchorX-fp.X, anchorY-fp.Y, anchorX, anchorY)
}

// place puts the picked entry down.
func (e *Editor) place(x, y int) {
	ent := e.picked
	if ent == nil {
		return
	}

	if ok, why := e.canPlace(*ent, x, y); !ok {
		e.message = "cannot place there: " + why
		return
	}

	gid, ok := e.gidFor(*ent)
	if !ok {
		e.message = "that piece is not in this map's tileset yet, so it cannot be placed"
		return
	}

	var (
		cmd d2mapedit.Cmd
		err error
	)

	switch ent.Layer {
	case d2maptiled.LayerStructure:
		ax, ay := editorStructureAnchor(x, y)
		cmd, err = e.doc.PlaceStructure(gid, ax, ay)
	case d2maptiled.LayerFloor:
		cmd, err = e.doc.SetFloorTile(x, y, gid)
	default:
		cmd, err = e.doc.SetWallTile(x, y, gid)
	}

	if err != nil {
		e.message = "cannot place there: " + err.Error()
		return
	}

	e.apply(cmd)
}

// gidFor finds the tileset gid for a catalog entry in THIS map. v0 places only
// pieces the map's tileset already carries: adding a new tile to an embedded
// tileset is a bigger change than v0 makes, and offering it without doing it
// would be the misleading preview the brief warns against.
//
// The id is tried first -- Entry.ID and d2mapedit.Kind.Name are both
// "<tileset>#<local id>", so they match exactly -- and the resolved art path
// second, which is how a piece read off disk finds the tileset tile pointing at
// the same file.
func (e *Editor) gidFor(ent d2mappalette.Entry) (int, bool) {
	if gid, ok := e.gids[ent.ID]; ok {
		return gid, true
	}

	gid, ok := e.gids[ent.ImagePath]

	return gid, ok
}

// canPlace answers the question the GHOST asks and the question place asks, out
// of one piece of arithmetic, so what the cursor shows and what a click does
// cannot disagree. Every refusal here is one the loader would make anyway --
// structure() refuses an overlap, a wall under a footprint and bare ground
// (tiled.go:1022-1032) -- and making it here means the document never takes an
// edit that would have the engine throw the whole map away.
//
// THE TAB IT ASKS ABOUT IS THE HELD PIECE'S OWN (28 Sep review, B4), not the tab
// on show: holding a house and looking at the greyed Terrain tab used to refuse
// the click with Terrain's reason.
func (e *Editor) canPlace(ent d2mappalette.Entry, x, y int) (bool, string) {
	if tab, ok := e.entryTab(ent); ok && !tab.Available {
		return false, tab.Why
	}

	if !ent.Placeable() {
		return false, ent.Why()
	}

	if _, ok := e.gidFor(ent); !ok {
		return false, "that piece is not in this map's tileset yet, so it cannot be placed"
	}

	bounds := image.Rect(0, 0, e.doc.Size().X, e.doc.Size().Y)

	if ent.Layer != d2maptiled.LayerStructure {
		if !image.Pt(x, y).In(bounds) {
			return false, "off the map"
		}

		if ent.Layer != d2maptiled.LayerWall {
			return true, "" // a floor tile goes anywhere on the map
		}

		if !e.doc.HasWalls() {
			return false, "this map has no walls layer; add one in Tiled"
		}

		// A wall under a structure is refused by the loader outright
		// (tiled.go:1029), and a BLOCKED wall under a person is refused by the
		// standable rule, the same way a footprint is.
		if e.hasStructure(x, y) {
			return false, fmt.Sprintf("a structure stands on %d,%d", x, y)
		}

		if ent.Blocked {
			return e.nobodyStandsIn(image.Rect(x, y, x+1, y+1))
		}

		return true, ""
	}

	ax, ay := editorStructureAnchor(x, y)

	r := editorFootprintAt(ax, ay, ent.Footprint)
	if !r.In(bounds) {
		return false, fmt.Sprintf("a %dx%d footprint here reaches off the %dx%d map",
			ent.Footprint.X, ent.Footprint.Y, bounds.Dx(), bounds.Dy())
	}

	return e.footprintIsClear(r)
}

func (e *Editor) footprintIsClear(r image.Rectangle) (bool, string) {
	// Off the map first: every tile past the edge reads as bare ground, and
	// "bare ground" is the wrong thing to tell a designer who duplicated a
	// house against the east edge (28 Sep review, C).
	if w, h := e.doc.Size().X, e.doc.Size().Y; !r.In(image.Rect(0, 0, w, h)) {
		return false, fmt.Sprintf("a %dx%d footprint at %d,%d reaches off the %dx%d map",
			r.Dx(), r.Dy(), r.Min.X, r.Min.Y, w, h)
	}

	for ty := r.Min.Y; ty < r.Max.Y; ty++ {
		for tx := r.Min.X; tx < r.Max.X; tx++ {
			switch {
			case e.hasStructure(tx, ty):
				return false, fmt.Sprintf("another structure stands on %d,%d", tx, ty)
			case e.doc.WallTile(tx, ty) != 0:
				return false, fmt.Sprintf("a wall stands on %d,%d", tx, ty)
			case e.doc.FloorTile(tx, ty) == 0:
				return false, fmt.Sprintf("%d,%d is bare ground; a structure needs a floor under it", tx, ty)
			}
		}
	}

	return e.nobodyStandsIn(r)
}

// nobodyStandsIn refuses a footprint that would be dropped on somebody.
//
// MEASURED, 28 Sep 2026: with this check missing, dropping a peasant house on
// the smith put a person under a solid footprint and the status line came back
// "the edit made a map the engine refuses: npc charsi (object 12) stands on
// tile 25,29, which is blocked or has no floor". A structure's footprint is
// forced blocked (tiled.go:887-890) and the loader refuses a person on a
// blocked tile, so this is the loader's own rule asked before the edit rather
// than after it -- which is the whole point of the ghost.
func (e *Editor) nobodyStandsIn(r image.Rectangle) (bool, string) {
	for _, o := range e.doc.Objects() {
		if o.Class != d2mapedit.ClassNPC && o.Class != d2mapedit.ClassPlayerStart {
			continue
		}

		if t := o.Tile(); t.In(r) {
			return false, fmt.Sprintf("%s stands on %d,%d", editorObjectName(o), t.X, t.Y)
		}
	}

	return true, ""
}

func (e *Editor) hasStructure(x, y int) bool {
	_, ok := e.doc.StructureOn(x, y)
	return ok
}

func (e *Editor) apply(cmd d2mapedit.Cmd) {
	if cmd == nil {
		return
	}

	if err := e.stack.Do(cmd); err != nil {
		e.message = "refused: " + err.Error()
		return
	}

	e.afterEdit(cmd.Label())
}

func (e *Editor) afterEdit(what string) {
	e.leaveArmed = false

	// The validator runs WHATEVER the engine said, and before the early return:
	// an edit the engine refuses is exactly the moment the problem list must not
	// be the one from before the edit. (Measured 28 Sep 2026: it was, and the
	// status line read "the game would take this map as it stands" under a
	// message saying the engine had just refused it.)
	e.refreshGIDs()
	e.revalidate()

	if err := e.rebuild(); err != nil {
		e.message = "the edit made a map the engine refuses: " + err.Error()
		return
	}

	e.message = what
}

func (e *Editor) runUndo() {
	if !e.stack.CanUndo() {
		e.message = "nothing to undo"
		return
	}

	label := e.stack.UndoLabel()
	if err := e.stack.Undo(); err != nil {
		e.message = "undo failed: " + err.Error()

		return
	}

	e.afterEdit("undid " + label)
}

func (e *Editor) runRedo() {
	if !e.stack.CanRedo() {
		e.message = "nothing to redo"
		return
	}

	label := e.stack.RedoLabel()
	if err := e.stack.Redo(); err != nil {
		e.message = "redo failed: " + err.Error()

		return
	}

	e.afterEdit("redid " + label)
}

func (e *Editor) deleteSelected() {
	if e.selected == 0 {
		e.message = "nothing selected"
		return
	}

	cmd, err := e.doc.DeleteObject(e.selected)
	if err != nil {
		e.message = "cannot delete: " + err.Error()
		return
	}

	e.selected = 0
	e.apply(cmd)
}

// duplicateSelected copies the selected structure ONE FOOTPRINT east, which is
// where the next house of a row goes and is the nearest place the copy does not
// overlap the original: the loader refuses two structures on one tile
// (tiled.go:1027), so a one-tile offset would be refused every time.
func (e *Editor) duplicateSelected() {
	if e.selected == 0 {
		e.message = "nothing selected"
		return
	}

	o, ok := e.doc.Object(e.selected)
	if !ok || !o.IsStructure() {
		e.message = "only a structure can be duplicated in v0"
		return
	}

	fp := o.Footprint

	if ok, why := e.footprintIsClear(fp.Add(image.Pt(fp.Dx(), 0))); !ok {
		e.message = "cannot duplicate there: " + why
		return
	}

	cmd, err := e.doc.PlaceStructure(o.GID, fp.Max.X+fp.Dx(), fp.Max.Y)
	if err != nil {
		e.message = "cannot duplicate there: " + err.Error()
		return
	}

	e.apply(cmd)
}

// revalidate runs the document's validator -- which is checked against the
// engine's own Parse -- and the reachability flood fill, so a designer is told
// he has sealed the village at the moment he does it rather than at playtest.
func (e *Editor) revalidate() {
	e.problems = e.doc.Validate(e.art)

	r, err := e.doc.Reachable()
	e.sealed = err == nil && r.Count() > 0 && !e.reachesAnEdge(r)
}

// reachesAnEdge asks whether the flood fill from the start touches the map's
// border. The village's whole point is that the night comes in by the gate, so
// an enclosure with no way out at all is a mistake worth naming.
func (e *Editor) reachesAnEdge(r d2mapedit.Reach) bool {
	w, h := e.doc.Size().X, e.doc.Size().Y

	for x := 0; x < w; x++ {
		if r.At(x, 0) || r.At(x, h-1) {
			return true
		}
	}

	for y := 0; y < h; y++ {
		if r.At(0, y) || r.At(w-1, y) {
			return true
		}
	}

	return false
}

// save writes the document to the file it was opened from -- diskPath, fixed at
// open -- once the validator AND the engine have taken the exact bytes
// (d2mapedit.Doc.Save; 28 Sep review, B2).
func (e *Editor) save() {
	if err := e.doc.Save(e.diskPath, e.art, e.engine()); err != nil {
		e.message = "NOT SAVED: " + editorOneLine(err)
		return
	}

	e.stack.MarkSaved()
	e.leaveArmed = false
	e.message = fmt.Sprintf("saved %s (previous kept as %s)", filepath.Base(e.diskPath),
		filepath.Base(d2mapedit.BackupPath(e.diskPath)))
}

// leave goes back to the main menu, and asks first if there is work in the
// document that is not on disk. The dirty flag is NOT cleared by the asking:
// the file really is unsaved, and a status line that said otherwise would be
// exactly the small lie this editor is built not to tell.
func (e *Editor) leave() {
	if e.dirty() && !e.leaveArmed {
		e.leaveArmed = true
		e.message = "unsaved changes -- Ctrl+S to save, or Escape again to leave them behind"

		return
	}

	e.navigator.ToMainMenu()
}

// ---- the palette -----------------------------------------------------------

func (e *Editor) refreshRows() {
	e.rows = nil
	e.reasonFor = -1

	if e.catalog == nil || len(e.tabs) == 0 {
		return
	}

	e.rows = e.catalog.InCategory(e.tabs[e.tab].Category)

	sort.SliceStable(e.rows, func(i, j int) bool {
		if e.rows[i].Placeable() != e.rows[j].Placeable() {
			return e.rows[i].Placeable()
		}

		return e.rows[i].DisplayName < e.rows[j].DisplayName
	})
}

// The palette's geometry lives in these four methods so the drawing and the
// hit-testing cannot drift apart. Each is a pure function of the tab strip.
func (e *Editor) paletteTabsTop() int { return edToolbarH + edLineH }

func (e *Editor) paletteReasonH() int {
	if len(e.tabs) == 0 || e.tabs[e.tab].Available {
		return 0
	}

	return edReasonLines * edLineH
}

func (e *Editor) paletteRowsTop() int {
	return e.paletteTabsTop() + len(e.tabs)*edTabH + e.paletteReasonH()
}

func (e *Editor) paletteVisibleRows() int {
	n := (editorScreenH - edStatusH - edLineH - e.paletteRowsTop()) / edRowH
	if n < 0 {
		return 0
	}

	return n
}

// editorPaletteHit is the palette column's hit test: which tab, or which row of
// the list, the point x, y is on. It is pure arithmetic over the geometry above
// so it can be tested without a screen. -1 means neither.
func editorPaletteHit(x, y, tabsTop, tabCount, rowsTop, rowTop, visible, rowCount int) (tab, row int) {
	if x < editorScreenW-edPaletteW {
		return -1, -1
	}

	if y >= tabsTop && y < tabsTop+tabCount*edTabH {
		return (y - tabsTop) / edTabH, -1
	}

	if y < rowsTop || y >= rowsTop+visible*edRowH {
		return -1, -1
	}

	i := rowTop + (y-rowsTop)/edRowH
	if i < 0 || i >= rowCount {
		return -1, -1
	}

	return -1, i
}

func (e *Editor) clickPalette() {
	tab, row := editorPaletteHit(e.mouseX, e.mouseY, e.paletteTabsTop(), len(e.tabs),
		e.paletteRowsTop(), e.rowTop, e.paletteVisibleRows(), len(e.rows))

	if tab >= 0 {
		e.tab, e.rowTop = tab, 0
		e.refreshRows()

		if !e.tabs[e.tab].Available {
			e.message = e.tabs[e.tab].Title + ": " + e.tabs[e.tab].Why
		}

		return
	}

	if row < 0 {
		return
	}

	ent := e.rows[row]

	if ok, why := e.canPickable(ent); !ok {
		e.message = ent.DisplayName + ": " + why
		return
	}

	e.picked, e.tool = &ent, toolPlace
	e.message = "placing " + ent.DisplayName + " -- click the map, Escape cancels"
}

// canPickable is why a palette row cannot be picked up at all, as against why
// one particular tile will not take it. The tab's reason comes first because it
// is the reason a whole category is greyed.
func (e *Editor) canPickable(ent d2mappalette.Entry) (bool, string) {
	if tab, ok := e.entryTab(ent); ok && !tab.Available {
		return false, tab.Why
	}

	if !ent.Placeable() {
		return false, ent.Why()
	}

	if _, ok := e.gidFor(ent); !ok {
		return false, "not in this map's tileset, so v0 cannot place it"
	}

	return true, ""
}

// entryTab is the palette tab a piece belongs to: the one whose category is its
// own. A piece is judged by ITS tab, whichever one is on show (B4).
func (e *Editor) entryTab(ent d2mappalette.Entry) (d2mappalette.Tab, bool) {
	for _, tab := range e.tabs {
		if tab.Category == ent.Category {
			return tab, true
		}
	}

	return d2mappalette.Tab{}, false
}

// ---- playtest --------------------------------------------------------------

// playtest writes the document to a TEMPORARY copy and starts the real game on
// it, so a run's deaths, destroyed objects and simulation state can never reach
// the authoring file. The authoring file is not touched at all, saved or not.
//
// MEASURED (27 Sep): the .tmj is re-read from disk on every start_game and the
// loader has no cache, so a write followed by a launch is enough; and the map
// argument is STICKY per process, so it is always passed explicitly.
//
// WHAT CHANGED ON 28 SEP (the review):
//   - the bytes are the ones the validator AND the engine have taken
//     (Doc.Checked, B2), so P can no more start the game on a map it would
//     refuse than Ctrl+S can save one;
//   - the scratch file is read back THE WAY THE GAME WILL READ IT -- by its
//     asset path, through the game's loader -- and the playtest refuses to
//     start unless that is the bytes just written. The loader looks in the
//     game's folders in its own order, and a playtest that ran on some other
//     copy of playtest-scratch.tmj would be a playtest of the wrong map;
//   - THIS SCREEN IS NOT THROWN AWAY. The App keeps it and hands it back when
//     the playtest game ends (App.ToPlaytest, ReturnFromPlaytest), so the
//     document, its unsaved changes and its undo history are all still here
//     afterwards. P used to drop them and the menu reopened the file from disk.
func (e *Editor) playtest() {
	data, err := e.doc.Checked(e.art, e.engine())
	if err != nil {
		e.message = "cannot playtest: " + editorOneLine(err)
		return
	}

	tmp := filepath.Join(filepath.Dir(e.diskPath), editorPlaytestName)
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		e.message = "cannot playtest: " + err.Error()
		return
	}

	scratch := path.Join(path.Dir(e.mapPath), editorPlaytestName)

	back, err := e.load(scratch)
	if err != nil || !bytes.Equal(back, data) {
		why := "it reads a different " + editorPlaytestName
		if err != nil {
			why = err.Error()
		}

		e.message = fmt.Sprintf("cannot playtest: the game would not read the map just written to %s (%s)", tmp, why)

		return
	}

	e.message = "playtesting " + editorPlaytestName + " -- the authoring file is untouched"
	e.navigator.ToPlaytest(scratch)
}

// ReturnFromPlaytest is how the App gives this screen back when a playtest game
// ends (A2). Nothing needs restoring: the document, the undo history and the
// unsaved mark are this screen's own fields and were never let go -- the screen
// manager only unloaded it, and OnLoad keeps the view it had. note is why the
// game ended when that is worth saying (it could not start, say).
func (e *Editor) ReturnFromPlaytest(note string) {
	msg := "back from the playtest -- the map is as you left it"
	if e.dirty() {
		msg += ", unsaved changes and all"
	}

	if note = strings.TrimSpace(note); note != "" {
		msg = note + " -- " + msg
	}

	e.message = msg
	e.leaveArmed = false
}

// PlaytestNotStarted says why a playtest could not begin, when the App finds
// out before it leaves this screen (it could not make the playtest hero).
func (e *Editor) PlaytestNotStarted(reason string) {
	e.message = "cannot playtest: " + reason
}

// editorPlaytestName is the temporary map a playtest runs on. It sits beside the
// authoring file because a tileset's image paths are relative to the map.
const editorPlaytestName = "playtest-scratch.tmj"

// ---- drawing ---------------------------------------------------------------

// Render draws the map, then the editor's own chrome over it. The map's view is
// cleared to one known colour first, so what is not the map reads as nothing
// rather than as whatever the window held.
func (e *Editor) Render(target d2interface.Surface) {
	view := mapViewRect()
	editorFillRect(target, view.Min.X, view.Min.Y, view.Dx(), view.Dy(), edColVoid)

	e.mapRenderer.Render(target)

	if e.canvas != nil {
		e.canvas.Render(target)
	}
}

// Advance runs the renderer's own advance so the view keeps up with the camera.
// MapRenderer.Advance returns NOTHING.
func (e *Editor) Advance(elapsed float64) error {
	e.mapEngine.Advance(elapsed)
	e.mapRenderer.Advance(elapsed)

	return nil
}

// renderChrome draws everything that is not the map: the grid and the markers
// on it first, because the panels have to cover where those spill, and then the
// toolbar, the palette column and the status area.
func (e *Editor) renderChrome(target d2interface.Surface) {
	if e.showGrid {
		e.drawGrid(target)
	}

	e.drawPeople(target)
	e.drawSelection(target)
	e.drawGhost(target)
	e.drawNotice(target)
	e.drawToolbar(target)
	e.drawPalette(target)
	e.drawStatus(target)
}

// editorFillRect draws one opaque rect at absolute screen coordinates.
//
// d2player has one of these and it is UNEXPORTED, so it cannot be reached from
// this package; this is the same shape (overhead_bars.go:203-211). Note what
// that one's comment says: DrawRect ignores the brightness and effect stack,
// which is why the alpha goes in the colour.
func editorFillRect(target d2interface.Surface, x, y, w, h int, rgba uint32) {
	if w <= 0 || h <= 0 {
		return
	}

	target.PushTranslation(x, y)
	target.DrawRect(w, h, d2util.Color(rgba))
	target.Pop()
}

// editorPanel is a filled rect with a one-pixel edge.
func editorPanel(target d2interface.Surface, x, y, w, h int, fill, edge uint32) {
	editorFillRect(target, x, y, w, h, fill)
	editorFillRect(target, x, y, w, 1, edge)
	editorFillRect(target, x, y+h-1, w, 1, edge)
	editorFillRect(target, x, y, 1, h, edge)
	editorFillRect(target, x+w-1, y, 1, h, edge)
}

// editorLine draws one straight line between two screen points.
func editorLine(target d2interface.Surface, x0, y0, x1, y1 int, rgba uint32) {
	target.PushTranslation(x0, y0)
	target.DrawLine(x1-x0, y1-y0, d2util.Color(rgba))
	target.Pop()
}

// drawTileRect outlines a rectangle of TILES as the isometric diamond it really
// is, through the renderer's own WorldToScreen so the outline and the art agree
// at every zoom and every pan.
func (e *Editor) drawTileRect(target d2interface.Surface, r image.Rectangle, rgba uint32) {
	ax, ay := e.mapRenderer.WorldToScreen(float64(r.Min.X), float64(r.Min.Y))
	bx, by := e.mapRenderer.WorldToScreen(float64(r.Max.X), float64(r.Min.Y))
	cx, cy := e.mapRenderer.WorldToScreen(float64(r.Max.X), float64(r.Max.Y))
	dx, dy := e.mapRenderer.WorldToScreen(float64(r.Min.X), float64(r.Max.Y))

	editorLine(target, ax, ay, bx, by, rgba)
	editorLine(target, bx, by, cx, cy, rgba)
	editorLine(target, cx, cy, dx, dy, rgba)
	editorLine(target, dx, dy, ax, ay, rgba)
}

// drawGrid draws the tile grid as 2*(n+1) long isometric lines rather than four
// per tile: a 48x48 map is 9216 segments the other way, every frame.
func (e *Editor) drawGrid(target d2interface.Surface) {
	w, h := e.doc.Size().X, e.doc.Size().Y

	for x := 0; x <= w; x++ {
		ax, ay := e.mapRenderer.WorldToScreen(float64(x), 0)
		bx, by := e.mapRenderer.WorldToScreen(float64(x), float64(h))
		editorLine(target, ax, ay, bx, by, edColGrid)
	}

	for y := 0; y <= h; y++ {
		ax, ay := e.mapRenderer.WorldToScreen(0, float64(y))
		bx, by := e.mapRenderer.WorldToScreen(float64(w), float64(y))
		editorLine(target, ax, ay, bx, by, edColGrid)
	}
}

// drawSelection outlines what is selected: a structure's whole footprint, an
// inside area's whole rectangle, or the single tile a person stands on.
func (e *Editor) drawSelection(target d2interface.Surface) {
	if e.selected == 0 {
		return
	}

	o, ok := e.doc.Object(e.selected)
	if !ok {
		return
	}

	r := image.Rect(o.Tile().X, o.Tile().Y, o.Tile().X+1, o.Tile().Y+1)

	switch {
	case o.IsStructure():
		r = o.Footprint
	case !o.Rect.Empty():
		r = o.Rect
	}

	e.drawTileRect(target, r, edColSelect)
}

// drawGhost shows, on the tile under the cursor, the footprint the picked entry
// would really take and whether it may go there. Valid is green and invalid is
// red, and the reason is drawn beside the cursor: a ghost that does not say no
// lets a designer build a map the engine will refuse.
func (e *Editor) drawGhost(target d2interface.Surface) {
	if e.tool != toolPlace || e.picked == nil {
		return
	}

	x, y, ok := e.hoverTile()
	if !ok {
		return
	}

	ent := *e.picked

	okHere, why := e.canPlace(ent, x, y)

	colour := uint32(edColGhostNo)
	if okHere {
		colour = edColGhostOK
	}

	r := image.Rect(x, y, x+1, y+1)
	if ent.Layer == d2maptiled.LayerStructure {
		ax, ay := editorStructureAnchor(x, y)
		r = editorFootprintAt(ax, ay, ent.Footprint)
	}

	for ty := r.Min.Y; ty < r.Max.Y; ty++ {
		for tx := r.Min.X; tx < r.Max.X; tx++ {
			e.drawTileRect(target, image.Rect(tx, ty, tx+1, ty+1), colour)
		}
	}

	e.drawTileRect(target, r, colour)

	note := fmt.Sprintf("%s  %dx%d  at %d,%d", ent.DisplayName, ent.Footprint.X, ent.Footprint.Y, x, y)
	if !okHere {
		note = "NO: " + why
	}

	e.text(target, e.hintLbl, e.mouseX+12, e.mouseY+12, colour, note,
		editorScreenW-edPaletteW-e.mouseX-16)
}

// drawPeople marks every person the map places and its player_start (28 Sep
// review, B5). The editor's view is the engine's map with no entities in it, so
// until this the only way to learn where the smith stood was to drop a house on
// him and watch the ghost turn red. Each gets his tile outlined and a mark at
// the point he stands, sized with the zoom (12 px across at 1.0, never under 6),
// and his name beside it; the start is a cross in its own colour. They are
// SELECTABLE as they always were -- a click on the tile selects the person on it
// -- and nothing here moves them: dragging is v1.
//
// NAMES (the second 28 Sep review). At the fit zoom the village's people stand a
// few pixels apart, and "start" was drawn over the smith's mark; at 0.25 the
// headman's name ran into the palette and was cut to "headman ...". So:
//
//   - below edLabelZoom a name is drawn only for the person under the cursor
//     and the one selected -- the marks say where everyone is, the cursor says
//     who;
//   - a name is placed clear of every other mark -- with room to spare, so it
//     never sits against someone else's and reads as his -- and every name
//     already placed: beside its own mark, right or left, or centred just above
//     or below it; failing those, beside it up to three lines up or down, with
//     a leader line back to the mark (editorPlaceLabel);
//   - it is drawn whole and wholly inside the map's view, or not at all -- it
//     is never cut short, and never drawn under the palette.
func (e *Editor) drawPeople(target d2interface.Surface) {
	scale := e.mapRenderer.Scale()
	half := int(math.Max(3, math.Round(6*scale)))
	view := mapViewRect()

	type person struct {
		o      d2mapedit.Object
		x, y   int
		colour uint32
		name   string
	}

	people := make([]person, 0, edMaxPeople)
	marks := make([]image.Rectangle, 0, edMaxPeople)

	if e.peopleDrawn == nil {
		e.peopleDrawn = map[int]editorPersonDrawn{}
	}

	for id := range e.peopleDrawn {
		delete(e.peopleDrawn, id)
	}

	for _, o := range e.doc.Objects() {
		var colour uint32

		name := editorObjectName(o)

		switch o.Class {
		case d2mapedit.ClassNPC:
			colour = edColPerson
		case d2mapedit.ClassPlayerStart:
			colour, name = edColStart, "start"
		default:
			continue
		}

		t := o.Tile()
		e.drawTileRect(target, image.Rect(t.X, t.Y, t.X+1, t.Y+1), colour)

		x, y := e.mapRenderer.WorldToScreen(o.X, o.Y)

		if o.Class == d2mapedit.ClassPlayerStart {
			editorLine(target, x-half, y-half, x+half, y+half, colour)
			editorLine(target, x-half, y+half, x+half, y-half, colour)
			editorLine(target, x-half+1, y-half, x+half+1, y+half, colour)
			editorLine(target, x-half+1, y+half, x+half+1, y-half, colour)
		} else {
			editorFillRect(target, x-half, y-half, 2*half, 2*half, colour)
			editorFillRect(target, x-half, y-half, 2*half, 1, edColSwatchBg)
			editorFillRect(target, x-half, y+half-1, 2*half, 1, edColSwatchBg)
		}

		mark := editorMarkRect(x, y, half)
		marks = append(marks, mark)
		people = append(people, person{o: o, x: x, y: y, colour: colour, name: name})
		e.peopleDrawn[o.ID] = editorPersonDrawn{mark: mark}
	}

	hx, hy, hovering := e.hoverTile()
	names := make([]image.Rectangle, 0, len(people))
	named := 0

	for i, p := range people {
		if named >= len(e.personLbls) {
			break
		}

		t := p.o.Tile()
		if scale < edLabelZoom && p.o.ID != e.selected && (!hovering || t.X != hx || t.Y != hy) {
			continue
		}

		lbl := e.personLbls[named]
		w, _ := lbl.GetTextMetrics(editorPlain(p.name))

		others := append(append(make([]image.Rectangle, 0, len(marks)), marks[:i]...), marks[i+1:]...)

		r, leader, ok := editorPlaceLabel(marks[i], w, edLineH, view, others, names)
		if !ok {
			continue
		}

		named++
		names = append(names, r)

		if leader {
			end := r.Min.X - 1
			if r.Max.X <= p.x {
				end = r.Max.X
			}

			editorLine(target, p.x, p.y, end, r.Min.Y+edLineH/2, p.colour)
		}

		// maxW is the name's own width, so editorFit never has to cut it.
		e.text(target, lbl, r.Min.X, r.Min.Y, p.colour, p.name, w+1)

		d := e.peopleDrawn[p.o.ID]
		d.label, d.text = r, p.name
		e.peopleDrawn[p.o.ID] = d
	}
}

// editorPersonDrawn is where one person's mark and name went on the last
// frame; label is empty when the name was not drawn.
type editorPersonDrawn struct {
	mark, label image.Rectangle
	text        string
}

// editorMarkRect is the box a person's mark (or the start's cross) covers on
// screen, a pixel of margin all round.
func editorMarkRect(x, y, half int) image.Rectangle {
	return image.Rect(x-half-1, y-half-1, x+half+2, y+half+2)
}

// edLabelClear is how far a name keeps from another person's mark, and edLabelGap
// from its own. A name that touches someone else's mark reads as his: on the
// second review's 0.25 screenshot the headman's name, flipped left of his own
// mark, began one pixel after the woman's. [DIAL]
const (
	edLabelClear = 8
	edLabelGap   = 2
)

// editorPlaceLabel finds where a w x h name goes for the mark whose box is own.
// It tries, in order: beside the mark on the right, beside it on the left,
// centred just above it, centred just below it -- and then the right and the
// left again a line down, up, two down, two up, three down, three up, which
// want a leader line back to the mark (leader). The first place wholly inside
// view that keeps edLabelClear from every one of marks (the OTHER people's) and
// overlaps none of names is taken; ok is false when there is none, and the
// name is not drawn.
func editorPlaceLabel(own image.Rectangle, w, h int, view image.Rectangle,
	marks, names []image.Rectangle) (r image.Rectangle, leader, ok bool) {
	mid := (own.Min.Y + own.Max.Y) / 2
	centre := (own.Min.X+own.Max.X)/2 - w/2
	right, left := own.Max.X+edLabelGap, own.Min.X-edLabelGap-w

	type place struct {
		x, y   int
		leader bool
	}

	places := []place{
		{right, mid - h/2, false},
		{left, mid - h/2, false},
		{centre, own.Min.Y - edLabelGap - h, false},
		{centre, own.Max.Y + edLabelGap, false},
	}

	for _, x := range []int{right, left} {
		for _, lines := range []int{1, -1, 2, -2, 3, -3} {
			places = append(places, place{x, mid - h/2 + lines*h, true})
		}
	}

	for _, p := range places {
		try := image.Rect(p.x, p.y, p.x+w, p.y+h)
		if !try.In(view) {
			continue
		}

		clear := true

		for _, m := range marks {
			if try.Overlaps(m.Inset(-edLabelClear)) {
				clear = false
				break
			}
		}

		for _, n := range names {
			if !clear || try.Overlaps(n) {
				clear = false
				break
			}
		}

		if clear {
			return try, p.leader, true
		}
	}

	return image.Rectangle{}, false, false
}

// drawNotice puts the engine's refusal in the middle of the map's view (28 Sep
// review, C). A map the engine refuses used to open as an empty grid with the
// reason squeezed into the status bar; an edit it refuses leaves the view on the
// last version it took, which is a picture of a map that no longer exists. Both
// are said here, in the place the designer is looking.
func (e *Editor) drawNotice(target d2interface.Surface) {
	if e.engineErr == nil || e.noticeLbl == nil {
		return
	}

	view := mapViewRect()
	w := view.Dx() - 40

	head := "THE GAME REFUSES THIS MAP -- it would build Diablo II's Act 1 instead"
	if e.laidOnce {
		head = "THE GAME REFUSES THE MAP AS IT NOW STANDS -- the view shows the last version it took"
	}

	reason := editorOneLine(e.engineErr)
	if e.noticeFor != reason {
		e.noticeLines = editorWrap(e.noticeLbl, reason, w-16, edNoticeLines)
		e.noticeFor = reason
	}

	foot := "Ctrl+S and P refuse it: mend the file, or the art it names, and open it again"
	if e.laidOnce {
		foot = "Ctrl+Z takes the edit back; until then Ctrl+S and P refuse this map"
	}

	// A label draws an empty line as nothing at all, so the spacers are a space.
	lines := append([]string{head, " "}, e.noticeLines...)
	lines = append(lines, " ", foot)

	// A label steps down by each line's own height (label.go Render), which
	// for this font is more than edLineH, so the panel is sized the same way.
	h := 16
	for _, l := range lines {
		_, lh := e.noticeLbl.GetTextMetrics(editorPlain(l))
		h += lh
	}

	x0 := view.Min.X + 20
	y0 := view.Min.Y + (view.Dy()-h)/2

	editorPanel(target, x0, y0, w, h, edColNotice, edColWarn)
	e.textLines(target, e.noticeLbl, x0+8, y0+8, edColWarn, lines, w-16)
}

func (e *Editor) drawToolbar(target d2interface.Surface) {
	editorPanel(target, 0, 0, editorScreenW, edToolbarH, edColStrip, edColEdge)

	e.textLines(target, e.toolLbl, edPad, 3, edColText, editorVerbs[:], editorScreenW-2*edPad)
}

// drawStatus is the bottom of the screen: which file this is and whether it is
// saved, what just happened, what the validator says, and -- loudly -- whether
// anything can still walk out of the map at all.
func (e *Editor) drawStatus(target d2interface.Surface) {
	top := editorScreenH - edStatusH
	editorPanel(target, 0, top, editorScreenW, edStatusH, edColStrip, edColEdge)

	mark := ""
	colour := uint32(edColText)

	if e.dirty() {
		mark, colour = "  *UNSAVED*", edColSelect
	}

	head := fmt.Sprintf("%s   %s%s   zoom %.2f   kinds %d/%d   undo %d",
		e.doc.DisplayName(), e.mapPath, mark, e.mapRenderer.Scale(),
		e.catalog.KindsUsed(), d2mappalette.MaxKinds, e.stack.Depth())

	e.text(target, e.titleLbl, edPad, top+2, colour, head, editorScreenW-2*edPad)
	e.text(target, e.statusLbl, edPad, top+2+edLineH, edColText, e.message, editorScreenW-2*edPad)

	problems := "the game would take this map as it stands"
	pColour := uint32(edColGood)

	switch {
	case len(e.problems) > 0:
		problems = fmt.Sprintf("%d problem(s), first: %s", len(e.problems), e.problems[0].Error())
		pColour = edColWarn
	case e.engineErr != nil:
		// The validator reads a PNG's header; the engine decodes it. When they
		// disagree the engine is the one the game asks, so this line must not
		// say the game would take the map (28 Sep review: it did, under a
		// notice saying the opposite).
		problems = "the game's own loader refuses this map: " + editorOneLine(e.engineErr)
		pColour = edColWarn
	}

	e.text(target, e.problemLbl, edPad, top+2+2*edLineH, pColour, problems, editorScreenW-2*edPad)

	if e.sealed {
		e.text(target, e.sealedLbl, edPad, top+2+3*edLineH, edColWarn,
			"SEALED: nothing can walk from the player_start to the edge of the map",
			editorScreenW-2*edPad)
	}
}

func (e *Editor) drawPalette(target d2interface.Surface) {
	x0 := editorScreenW - edPaletteW
	editorPanel(target, x0, edToolbarH, edPaletteW, editorScreenH-edToolbarH-edStatusH, edColPanel, edColEdge)

	inner := edPaletteW - 2*edPad

	e.text(target, e.headerLbl, x0+edPad, edToolbarH+1, edColText,
		fmt.Sprintf("Palette  %d/%d kinds", e.catalog.KindsUsed(), d2mappalette.MaxKinds), inner)

	e.drawTabs(target, x0, inner)
	e.drawReason(target, x0, inner)
	e.drawRows(target, x0, inner)
}

// drawTabs draws the tab strip down the top of the column. An UNAVAILABLE tab is
// drawn dim and says so: it still carries its count, because Terrain really does
// hold six catalogued floor tiles and only the painting tool is missing.
func (e *Editor) drawTabs(target d2interface.Surface, x0, inner int) {
	top := e.paletteTabsTop()

	for i, tab := range e.tabs {
		y := top + i*edTabH

		if i == e.tab {
			editorFillRect(target, x0+1, y, edPaletteW-2, edTabH, edColRowPick)
		}

		colour := uint32(edColText)
		line := fmt.Sprintf("%s (%d)", tab.Title, tab.Count)

		if !tab.Available {
			colour = edColDim
			line += " - not in v0"
		}

		if i < len(e.tabLabels) {
			e.text(target, e.tabLabels[i], x0+edPad, y+1, colour, line, inner)
		}
	}
}

// drawReason prints why an unavailable tab is unavailable. The reasons are
// paragraphs -- they carry their citations -- and a 196-pixel column takes three
// lines of one, so it is wrapped and cut with an ellipsis rather than silently
// shortened. Clicking the tab puts the whole sentence in the status line, and it
// is in d2mappalette's Tab.Why and in docs/editor.md.
func (e *Editor) drawReason(target d2interface.Surface, x0, inner int) {
	if e.paletteReasonH() == 0 || e.reasonLbl == nil {
		return
	}

	if e.reasonFor != e.tab {
		e.reasonLines = editorWrap(e.reasonLbl, e.tabs[e.tab].Why, inner, edReasonLines)
		e.reasonFor = e.tab
	}

	top := e.paletteTabsTop() + len(e.tabs)*edTabH

	e.textLines(target, e.reasonLbl, x0+edPad, top, edColDim, e.reasonLines, inner)
}

func (e *Editor) drawRows(target d2interface.Surface, x0, inner int) {
	top := e.paletteRowsTop()
	visible := e.paletteVisibleRows()

	e.rowTop = editorClampTop(e.rowTop, len(e.rows), visible)

	for i := 0; i < visible; i++ {
		idx := e.rowTop + i
		if idx >= len(e.rows) || i >= len(e.rowLabels) {
			break
		}

		e.drawRow(target, e.rows[idx], e.rowLabels[i], x0, top+i*edRowH, inner)
	}

	shown := visible
	if len(e.rows) < shown {
		shown = len(e.rows)
	}

	count := fmt.Sprintf("%d of %d", shown, len(e.rows))
	if len(e.rows) > visible {
		count = fmt.Sprintf("%d-%d of %d -- wheel scrolls", e.rowTop+1, e.rowTop+shown, len(e.rows))
	}

	e.text(target, e.countLbl, x0+edPad, editorScreenH-edStatusH-edLineH, edColDim, count, inner)
}

// drawRow is one palette row: the art's thumbnail, its human name, and its
// footprint and layer -- or, when it cannot be placed, the reason, dimmed.
func (e *Editor) drawRow(target d2interface.Surface, ent d2mappalette.Entry, lbl *d2ui.Label,
	x0, y, inner int) {
	usable, why := e.canPickable(ent)

	if e.picked != nil && e.picked.ID == ent.ID {
		editorFillRect(target, x0+1, y, edPaletteW-2, edRowH, edColRowPick)
	}

	e.drawSwatch(target, ent, x0+edPad, y+2, usable)

	colour := uint32(edColText)
	second := fmt.Sprintf("%dx%d  %s  %s", ent.Footprint.X, ent.Footprint.Y, ent.Layer, ent.Status)

	if !usable {
		colour = edColDim
		second = "no: " + editorFirstSentence(why)
	}

	textX := x0 + edPad + edSwatch + edPad
	textW := inner - edSwatch - edPad

	e.textLines(target, lbl, textX, y+1, colour, []string{ent.DisplayName, second}, textW)
}

// drawSwatch draws the entry's own art, letterboxed into the swatch box at its
// true aspect so a 160x330 church tower is not squashed into a square. A piece
// whose art will not load draws a crossed box ONCE and is never retried.
func (e *Editor) drawSwatch(target d2interface.Surface, ent d2mappalette.Entry, x, y int, usable bool) {
	editorFillRect(target, x, y, edSwatch, edSwatch, edColSwatchBg)

	img := e.thumb(ent)
	if img == nil {
		editorLine(target, x, y, x+edSwatch, y+edSwatch, edColDim)
		editorLine(target, x+edSwatch, y, x, y+edSwatch, edColDim)
		editorFillRect(target, x, y, edSwatch, 1, edColEdge)
		editorFillRect(target, x, y+edSwatch-1, edSwatch, 1, edColEdge)

		return
	}

	w, h := ent.PixelWidth, ent.PixelHeight
	if w <= 0 || h <= 0 {
		w, h = img.GetSize()
	}

	scale := math.Min(float64(edSwatch)/float64(w), float64(edSwatch)/float64(h))
	offX := x + (edSwatch-int(float64(w)*scale))/2
	offY := y + (edSwatch-int(float64(h)*scale))/2

	target.PushTranslation(offX, offY)
	target.PushScale(scale, scale)

	// Brightness is a MULTIPLIER here, not an offset: 1 is the art as it is and
	// the surface only applies it when it is not 1 (ebiten_surface.go:143). A
	// negative value draws the art black, which is what a greyed-out row looked
	// like on the first run (28 Sep 2026).
	if !usable {
		target.PushBrightness(edDimBrightness)
	}

	target.Render(img)

	if !usable {
		target.Pop()
	}

	target.PopN(2)
}

// thumb loads one entry's PNG ONCE. A failure is remembered as a failure, so a
// missing file costs one read for the life of the screen and not one a frame.
func (e *Editor) thumb(ent d2mappalette.Entry) d2interface.Surface {
	if s, ok := e.thumbs[ent.ImagePath]; ok {
		return s
	}

	if e.thumbFail[ent.ImagePath] {
		return nil
	}

	s, err := e.loadThumb(ent.ImagePath)
	if err != nil {
		e.Warningf("the palette could not draw %s: %v", ent.ImagePath, err)
		e.thumbFail[ent.ImagePath] = true

		return nil
	}

	e.thumbs[ent.ImagePath] = s

	return s
}

// loadThumb decodes one PNG into a surface.
//
// The decode goes through image.Decode into an *image.RGBA on purpose:
// ReplacePixels wants PREMULTIPLIED alpha and image/png hands back *image.NRGBA
// for most files, so art with real partial alpha draws with bright halos
// otherwise. d2asset's decodePNG says the same thing and is unexported
// (png_animation.go:40-52, :169-187).
func (e *Editor) loadThumb(p string) (d2interface.Surface, error) {
	data, err := e.asset.LoadFile(p)
	if err != nil {
		return nil, err
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	b := src.Bounds()
	if b.Empty() {
		return nil, fmt.Errorf("%s has no pixels", p)
	}

	rgba := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, b.Min, draw.Src)

	sfc := e.renderer.NewSurface(b.Dx(), b.Dy())
	sfc.ReplacePixels(rgba.Pix)

	return sfc, nil
}

// ---- text ------------------------------------------------------------------

// text re-points one pooled Label and draws it.
func (e *Editor) text(target d2interface.Surface, lbl *d2ui.Label, x, y int, rgba uint32, s string, maxW int) {
	e.textLines(target, lbl, x, y, rgba, []string{s}, maxW)
}

// textLines draws several lines through ONE pooled Label -- Label.Render splits
// on "\n" itself. SetText is only called when the text has actually changed,
// because it compiles two regular expressions on every call (label.go:124-125)
// and this screen re-points two dozen labels a frame.
func (e *Editor) textLines(target d2interface.Surface, lbl *d2ui.Label, x, y int,
	rgba uint32, lines []string, maxW int) {
	if lbl == nil {
		return
	}

	fitted := make([]string, 0, len(lines))
	for _, line := range lines {
		fitted = append(fitted, editorFit(lbl, editorPlain(line), maxW))
	}

	s := strings.Join(fitted, "\n")
	if lbl.GetText() != s {
		lbl.SetText(s)
	}

	// AFTER SetText, never before: processColorTokens resets Color[0] to white
	// for any string with no colour token in it (label.go:136-138).
	lbl.Color[0] = d2util.Color(rgba)

	lbl.SetPosition(x, y)
	lbl.Render(target)
}

// editorPlain takes the square brackets out of a line. d2ui reads "[...]" as a
// colour token and DELETES it from the label (label.go:124, :131), so an error
// message or a rectangle printed with brackets would lose part of itself on the
// way to the screen.
func editorPlain(s string) string {
	if !strings.ContainsAny(s, "[]") {
		return s
	}

	return strings.NewReplacer("[", "(", "]", ")").Replace(s)
}

// editorFit cuts a line to the width it is given, with an ellipsis. The first
// guess is proportional, so this settles in a couple of measurements rather than
// walking the string a character at a time.
func editorFit(lbl *d2ui.Label, s string, maxW int) string {
	if s == "" || maxW <= 0 {
		return s
	}

	w, _ := lbl.GetTextMetrics(s)
	if w <= maxW {
		return s
	}

	r := []rune(s)

	n := len(r) * maxW / w
	if n > len(r) {
		n = len(r)
	}

	for ; n > 0; n-- {
		cut := string(r[:n]) + "..."
		if w, _ = lbl.GetTextMetrics(cut); w <= maxW {
			return cut
		}
	}

	return ""
}

// editorWrap breaks a paragraph into at most max lines of the given width,
// ending the last one with an ellipsis when there is more of it.
func editorWrap(lbl *d2ui.Label, s string, maxW, max int) []string {
	words := strings.Fields(s)
	lines := []string{}
	line := ""

	for i := range words {
		try := words[i]
		if line != "" {
			try = line + " " + words[i]
		}

		if w, _ := lbl.GetTextMetrics(editorPlain(try)); w <= maxW || line == "" {
			line = try
			continue
		}

		lines = append(lines, line)
		line = words[i]

		if len(lines) == max {
			lines[max-1] = editorFit(lbl, editorPlain(lines[max-1])+" ...", maxW)
			return lines
		}
	}

	if line != "" && len(lines) < max {
		lines = append(lines, editorFit(lbl, editorPlain(line), maxW))
	}

	return lines
}

// editorFirstSentence is the first sentence of a reason, for a line with room
// for one. The whole reason goes in the status line when the row is clicked.
func editorFirstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}

	return s
}

func editorObjectName(o d2mapedit.Object) string {
	if o.Name != "" {
		return o.Name
	}

	if o.Class != "" {
		return o.Class
	}

	return fmt.Sprintf("object %d", o.ID)
}
