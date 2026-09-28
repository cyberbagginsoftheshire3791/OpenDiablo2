package d2maprenderer

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"strconv"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2fileformats/d2ds1"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2util"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2map/d2mapengine"
)

const (
	logPrefix = "Map Renderer"
)

const (
	two = 2

	dbgOffsetXY   = 40
	dbgBoxWidth   = 220
	dbgBoxHeight  = 60
	dbgBoxPadding = 10

	dbgCollisionSize    = 5
	dbgCollisionOffsetX = -3
	dbgCollisionOffsetY = 4

	whiteHalfOpacity        = 0xffffff7f
	blackQuarterOpacity     = 0x00000040
	lightGreenFullOpacity   = 0x40ff00ff
	magentaFullOpacity      = 0xff00ffff
	yellowFullOpacity       = 0xffff00ff
	lightBlueQuarterOpacity = 0x5050ff32
	whiteQuarterOpacity     = 0xffffff64
	redQuarterOpacity       = 0x74000064

	subtilesPerTile    = 5
	orthoSubTileWidth  = 16
	orthoSubTileHeight = 8
	orthoTileWidth     = subtilesPerTile * orthoSubTileWidth
	orthoTileHeight    = subtilesPerTile * orthoSubTileHeight
)

// MapRenderer manages the game viewport and Camera. It requests tile and entity data from MapEngine and renders it.
type MapRenderer struct {
	asset               *d2asset.AssetManager
	renderer            d2interface.Renderer   // Used for drawing operations
	mapEngine           *d2mapengine.MapEngine // The map engine that is being rendered
	palette             d2interface.Palette    // The palette used for this map
	viewport            *Viewport              // Used for rendering offsets
	Camera              Camera                 // Used to determine where on the map we are rendering
	imageCacheRecords   map[uint32]d2interface.Surface
	mapDebugVisLevel    int     // Map debug visibility index (0=none, 1=tiles, 2=sub-tiles)
	entityDebugVisLevel int     // Entity Debug visibility index (0=none, 1=vectors)
	lastFrameTime       float64 // The last time the map was rendered
	currentFrame        int     // Current render frame (for animations)

	// lightSampler is how lit each tile is (M4.1). nil is full daylight,
	// which is exactly how this renderer behaved before there was light.
	lightSampler LightSampler

	*d2util.Logger
}

// CreateMapRenderer creates a new MapRenderer, sets the required fields and returns a pointer to it.
func CreateMapRenderer(asset *d2asset.AssetManager, renderer d2interface.Renderer,
	mapEngine *d2mapengine.MapEngine,
	term d2interface.Terminal, l d2util.LogLevel, startX, startY float64) *MapRenderer {
	result := &MapRenderer{
		asset:     asset,
		renderer:  renderer,
		mapEngine: mapEngine,
		viewport:  NewViewport(0, 0, 800, 600),
	}

	result.Logger = d2util.NewLogger()
	result.Logger.SetPrefix(logPrefix)
	result.Logger.SetLevel(l)

	result.Camera = Camera{}
	rx, ry := result.WorldToOrtho(startX, startY)
	startPosition := d2vector.NewPosition(rx, ry)
	result.Camera.position = &startPosition
	result.viewport.SetCamera(&result.Camera)

	var name, desc, level string

	name, desc = "mapdebugvis", "set map debug visualization level"
	level = "level"

	if err := term.Bind(name, desc, []string{level}, result.commandMapDebugVis); err != nil {
		result.Errorf("could not bind the mapdebugvis action, err: %v", err)
	}

	name, desc = "entitydebugvis", "set entity debug visualization level"

	if err := term.Bind(name, desc, []string{level}, result.commandEntityDebugVis); err != nil {
		result.Errorf("could not bind the entitydebugvis action, err: %v", err)
	}

	if mapEngine.LevelType().ID != 0 {
		result.generateTileCache()
	}

	return result
}

// UnbindTerminalCommands unbinds commands from the terminal
func (mr *MapRenderer) UnbindTerminalCommands(term d2interface.Terminal) error {
	return term.Unbind("mapdebugvis", "entitydebugvis")
}

func (mr *MapRenderer) commandMapDebugVis(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("invalid argument supplied")
	}

	level, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid argument supplied")
	}

	mr.mapDebugVisLevel = level

	return nil
}

func (mr *MapRenderer) commandEntityDebugVis(args []string) error {
	level, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid argument supplied")
	}

	mr.entityDebugVisLevel = level

	return nil
}

// RegenerateTileCache calls MapRenderer.generateTileCache().
func (mr *MapRenderer) RegenerateTileCache() {
	mr.generateTileCache()
}

// SetMapEngine sets the MapEngine this renderer is rendering.
func (mr *MapRenderer) SetMapEngine(mapEngine *d2mapengine.MapEngine) {
	mr.mapEngine = mapEngine
	mr.generateTileCache()
}

// Render determines the width and height of map tiles that should be rendered. The following four render passes are
// made in succession:
//
// Pass 1: Lower wall tiles, tile shadows and floor tiles.
//
// Pass 2: Entities below walls.
//
// Pass 3: Upper wall tiles and entities above walls.
//
// Pass 4: Roof tiles.
func (mr *MapRenderer) Render(target d2interface.Surface) {
	// https://github.com/OpenDiablo2/OpenDiablo2/issues/789
	// Prevents concurrent map read & write exceptions that otherwise occur when we join a TCP game
	// as a remote client, due to rendering before we have handled the GenerateMapPacket.
	if mr.mapEngine.IsLoading {
		return
	}

	mapSize := mr.mapEngine.Size()

	// The probe points, the clamping and the floor/ceil are all unchanged; they
	// moved to the viewport because that is where the scale is, and so a test can
	// reach them without a renderer (viewport.go).
	startX, startY, endX, endY := mr.viewport.cullRange(mapSize.Width, mapSize.Height)

	mr.renderPass1(target, startX, startY, endX, endY)
	mr.renderPass2(target, startX, startY, endX, endY)

	if mr.mapDebugVisLevel > 0 {
		mr.renderMapDebug(mr.mapDebugVisLevel, target, startX, startY, endX, endY)
	}

	// A structure's strips stand up to maxStructureHeight above their tiles,
	// so on an authored map the wall pass looks a few rows further down than
	// the floor passes: a house whose front tiles are just below the screen
	// still shows its roof.
	endX3, endY3 := endX, endY
	if len(mr.mapEngine.AuthoredImages()) > 0 {
		rows := mr.viewport.authoredCullRows()
		endX3 = int(math.Min(float64(mapSize.Width), float64(endX+rows)))
		endY3 = int(math.Min(float64(mapSize.Height), float64(endY+rows)))
	}

	mr.renderPass3(target, startX, startY, endX3, endY3)
	mr.renderPass4(target, startX, startY, endX, endY)

	if mr.entityDebugVisLevel > 0 {
		mr.renderEntityDebug(target)
	}
}

// MoveCameraTo sets the position of the Camera to the given x and y coordinates.
func (mr *MapRenderer) MoveCameraTo(position *d2vector.Position) {
	mr.Camera.MoveTo(position)
}

// MoveCameraBy adds the given vector to the current position of the Camera.
func (mr *MapRenderer) MoveCameraBy(vector *d2vector.Vector) {
	mr.Camera.MoveBy(vector)
}

// MoveCameraTargetBy adds the given vector to the current position of the Camera.
func (mr *MapRenderer) MoveCameraTargetBy(vector *d2vector.Vector) {
	mr.Camera.MoveTargetBy(vector)
}

// ScreenToWorld returns the world position for the given screen (pixel) position.
func (mr *MapRenderer) ScreenToWorld(x, y int) (worldX, worldY float64) {
	return mr.viewport.ScreenToWorld(x, y)
}

// Scale returns the viewport's zoom. 1.0 is unzoomed, which is the only value
// the shipped game uses.
func (mr *MapRenderer) Scale() float64 {
	return mr.viewport.Scale()
}

// SetScale sets the viewport's zoom without moving the camera, so the world point
// at the middle of the screen stays there. ZoomAt is the one that holds a point
// under the cursor instead.
func (mr *MapRenderer) SetScale(scale float64) {
	mr.viewport.SetScale(scale)
}

// ZoomAt sets the viewport's zoom to newScale and moves the camera so the world
// point under the given screen pixel stays under it. This is the whole of
// wheel-zoom-about-the-cursor: a wheel handler reads the event's amount and
// position and calls this.
//
// It moves the camera to the answer rather than aiming Camera.target at it,
// because a target is smoothed over following frames (Camera.advanceToTarget) and
// a zoom that arrives a few frames late does not hold the point under the cursor.
func (mr *MapRenderer) ZoomAt(screenX, screenY int, newScale float64) {
	camX, camY := mr.viewport.ZoomAtScreen(screenX, screenY, newScale)
	mr.viewport.SetScale(newScale)

	position := d2vector.NewPosition(camX, camY)
	mr.Camera.MoveTo(&position)
}

// ScreenToOrtho returns the orthogonal position, without accounting for the isometric angle, for the given screen
// (pixel) position.
func (mr *MapRenderer) ScreenToOrtho(x, y int) (orthoX, orthoY float64) {
	return mr.viewport.ScreenToOrtho(x, y)
}

// WorldToOrtho returns the orthogonal position for the given isometric world position.
func (mr *MapRenderer) WorldToOrtho(x, y float64) (orthoX, orthoY float64) {
	return mr.viewport.WorldToOrtho(x, y)
}

// Lower wall tiles, tile shadows and floor tiles.
func (mr *MapRenderer) renderPass1(target d2interface.Surface, startX, startY, endX, endY int) {
	for tileY := startY; tileY < endY; tileY++ {
		for tileX := startX; tileX < endX; tileX++ {
			tile := mr.mapEngine.TileAt(tileX, tileY)
			mr.viewport.PushTranslationWorld(float64(tileX), float64(tileY))
			mr.pushTileLight(target, tileX, tileY)
			mr.renderTilePass1(tile, target)
			target.Pop()
			mr.viewport.PopTranslation()
		}
	}
}

// Entities below walls.
func (mr *MapRenderer) renderPass2(target d2interface.Surface, startX, startY, endX, endY int) {
	for tileY := startY; tileY < endY; tileY++ {
		for tileX := startX; tileX < endX; tileX++ {
			mr.viewport.PushTranslationWorld(float64(tileX), float64(tileY))
			mr.pushTileLight(target, tileX, tileY)

			tileEnt := mr.getEntitiesBelowWalls(tileX, tileY)

			for subY := 0; subY < 5; subY++ {
				for subX := 0; subX < 5; subX++ {
					for _, mapEntity := range tileEnt {
						pos := mapEntity.GetPosition()
						if (int(pos.SubTileOffset().X()) != subX) || (int(pos.SubTileOffset().Y()) != subY) {
							continue
						}

						target.PushTranslation(mr.viewport.GetTranslationScreen())
						mapEntity.Render(target)
						target.Pop()
					}
				}
			}

			target.Pop()
			mr.viewport.PopTranslation()
		}
	}
}

func (mr *MapRenderer) getEntitiesBelowWalls(tileX, tileY int) []d2interface.MapEntity {
	entities := make([]d2interface.MapEntity, 0)

	// need to add render culling
	// https://github.com/OpenDiablo2/OpenDiablo2/issues/821
	for _, mapEntity := range mr.mapEngine.Entities() {
		pos := mapEntity.GetPosition()
		vec := pos.World()
		entityX, entityY := vec.X(), vec.Y()

		if mapEntity.GetLayer() != 1 {
			continue
		}

		if (int(entityX) != tileX) || (int(entityY) != tileY) {
			continue
		}

		entities = append(entities, mapEntity)
	}

	return entities
}

// Upper wall tiles and entities above walls.
func (mr *MapRenderer) renderPass3(target d2interface.Surface, startX, startY, endX, endY int) {
	for tileY := startY; tileY < endY; tileY++ {
		for tileX := startX; tileX < endX; tileX++ {
			tile := mr.mapEngine.TileAt(tileX, tileY)
			mr.viewport.PushTranslationWorld(float64(tileX), float64(tileY))
			mr.pushTileLight(target, tileX, tileY)
			mr.renderTilePass2(tile, target)

			entities := mr.getEntitiesAboveWalls(tileX, tileY)

			for subY := 0; subY < 5; subY++ {
				for subX := 0; subX < 5; subX++ {
					for _, entity := range entities {
						pos := entity.GetPosition()
						if (int(pos.SubTileOffset().X()) != subX) || (int(pos.SubTileOffset().Y()) != subY) {
							continue
						}

						target.PushTranslation(mr.viewport.GetTranslationScreen())
						entity.Render(target)
						target.Pop()
					}
				}
			}

			target.Pop()
			mr.viewport.PopTranslation()
		}
	}
}

func (mr *MapRenderer) getEntitiesAboveWalls(tileX, tileY int) []d2interface.MapEntity {
	entities := make([]d2interface.MapEntity, 0)

	// need to add render culling
	// https://github.com/OpenDiablo2/OpenDiablo2/issues/821
	for _, mapEntity := range mr.mapEngine.Entities() {
		pos := mapEntity.GetPosition()
		vec := pos.World()
		entityX, entityY := vec.X(), vec.Y()

		if mapEntity.GetLayer() == 1 {
			continue
		}

		if (int(entityX) != tileX) || (int(entityY) != tileY) {
			continue
		}

		entities = append(entities, mapEntity)
	}

	return entities
}

// Roof tiles.
func (mr *MapRenderer) renderPass4(target d2interface.Surface, startX, startY, endX, endY int) {
	for tileY := startY; tileY < endY; tileY++ {
		for tileX := startX; tileX < endX; tileX++ {
			tile := mr.mapEngine.TileAt(tileX, tileY)
			mr.viewport.PushTranslationWorld(float64(tileX), float64(tileY))
			mr.pushTileLight(target, tileX, tileY)
			mr.renderTilePass3(tile, target)
			target.Pop()
			mr.viewport.PopTranslation()
		}
	}
}

func (mr *MapRenderer) renderTilePass1(tile *d2mapengine.MapTile, target d2interface.Surface) {
	for _, wall := range tile.Components.Walls {
		if !wall.Hidden() && wall.Prop1 != 0 && wall.Type.LowerWall() {
			mr.renderWall(wall, mr.viewport, target)
		}
	}

	for _, floor := range tile.Components.Floors {
		if !floor.Hidden() && floor.Prop1 != 0 {
			mr.renderFloor(floor, target)
		}
	}

	for _, shadow := range tile.Components.Shadows {
		if !shadow.Hidden() && shadow.Prop1 != 0 {
			mr.renderShadow(shadow, target)
		}
	}
}

func (mr *MapRenderer) renderTilePass2(tile *d2mapengine.MapTile, target d2interface.Surface) {
	for _, wall := range tile.Components.Walls {
		if !wall.Hidden() && wall.Type.UpperWall() {
			mr.renderWall(wall, mr.viewport, target)
		}
	}
}

func (mr *MapRenderer) renderTilePass3(tile *d2mapengine.MapTile, target d2interface.Surface) {
	for _, wall := range tile.Components.Walls {
		if wall.Type == d2enum.TileRoof {
			mr.renderWall(wall, mr.viewport, target)
		}
	}
}

func (mr *MapRenderer) renderFloor(tile d2ds1.Tile, target d2interface.Surface) {
	var img d2interface.Surface
	if !tile.Animated {
		img = mr.getImageCacheRecord(tile.Style, tile.Sequence, 0, tile.RandomIndex)
	} else {
		img = mr.getImageCacheRecord(tile.Style, tile.Sequence, 0, byte(mr.currentFrame))
	}

	if img == nil {
		mr.Warningf("Render called on uncached floor {%v,%v}", tile.Style, tile.Sequence)
		return
	}

	mr.viewport.PushTranslationOrtho(-80, float64(tile.YAdjust))
	defer mr.viewport.PopTranslation()

	target.PushTranslation(mr.viewport.GetTranslationScreen())
	defer target.Pop()

	mr.drawTileArt(target, img)
}

func (mr *MapRenderer) renderWall(tile d2ds1.Tile, viewport *Viewport, target d2interface.Surface) {
	img := mr.getImageCacheRecord(tile.Style, tile.Sequence, tile.Type, tile.RandomIndex)
	if img == nil {
		mr.Warningf("Render called on uncached wall {%v,%v,%v}", tile.Style, tile.Sequence, tile.Type)
		return
	}

	// A DT1 wall is one tile wide and hangs 80 pixels left of the tile's top
	// corner. An authored wall is placed by d2mapengine.AuthoredWallLeft: the
	// same for a one-tile wall, and half a diamond left or right for the
	// strips a structure is cut into (M5.4).
	left := -80.0
	if d2mapengine.IsAuthoredTile(&tile) {
		w, _ := img.GetSize()
		left = d2mapengine.AuthoredWallLeft(tile.Type, w)
	}

	viewport.PushTranslationOrtho(left, float64(tile.YAdjust))
	defer viewport.PopTranslation()

	target.PushTranslation(viewport.GetTranslationScreen())
	defer target.Pop()

	mr.drawTileArt(target, img)
}

func (mr *MapRenderer) renderShadow(tile d2ds1.Tile, target d2interface.Surface) {
	img := mr.getImageCacheRecord(tile.Style, tile.Sequence, 13, tile.RandomIndex)
	if img == nil {
		mr.Warningf("Render called on uncached shadow {%v,%v}", tile.Style, tile.Sequence)
		return
	}

	defer mr.viewport.PushTranslationOrtho(-80, float64(tile.YAdjust)).PopTranslation()

	target.PushTranslation(mr.viewport.GetTranslationScreen())
	defer target.Pop()

	target.PushColor(color.RGBA{R: 255, G: 255, B: 255, A: 160}) //nolint:gomnd // Not a magic number...
	defer target.Pop()

	mr.drawTileArt(target, img)
}

// drawTileArt draws one floor, wall or shadow image at the target's current
// translation, which the caller has already worked out through the viewport.
//
// AT SCALE 1.0 THIS IS target.Render(img) AND NOTHING ELSE. The shipped game
// never leaves 1.0, and that branch is the draw call the three callers made
// before there was a zoom, character for character: no PushScale, no Pop, no
// arithmetic on the size. TestDrawTileArtAtScale1IsTheUnscaledDraw pins it.
//
// AT ANY OTHER SCALE THE ART IS SCALED WITH ITS POSITION. The viewport has
// scaled positions since the editor-zoom burst, and until the 28 Sep review
// nothing scaled the art: the World Editor opened at about 0.08 and drew every
// tree and house at full size on anchors 0.08 apart, and every floor tile hung
// (80(1-s), 40(1-s)) screen pixels off its own tile, because the art's top-left
// was placed by the scaled transform and the rest of it was not. Scaling after
// the translation is the whole fix: the ebiten surface applies the scale and
// THEN the translation (ebiten_surface.go createDrawImageOptions), so a pixel
// (u, v) of the art lands at translation + (u*s, v*s) -- exactly where the
// viewport puts ortho point anchor + (u, v). TestDrawTileArtScalesTheArt
// measures the painted pixels, not the positions.
//
// seamPad is one screen pixel of overdraw on each axis. Neighbouring floor
// tiles are placed on whole pixels (OrthoToScreen floors) while their scaled
// size is fractional, and a nearest-neighbour shrink of interlocking diamonds
// leaves hairline gaps between them; drawing each tile one pixel wider and
// taller closes them, and a pixel of overlap between two pieces of ground is
// invisible. It is never applied at 1.0.
func (mr *MapRenderer) drawTileArt(target d2interface.Surface, img d2interface.Surface) {
	s := mr.viewport.Scale()
	if s == defaultScale {
		target.Render(img)
		return
	}

	w, h := img.GetSize()
	target.PushScale(seamScale(w, s), seamScale(h, s))
	target.Render(img)
	target.Pop()
}

// seamPad is how many screen pixels of overdraw drawTileArt adds to a scaled
// tile on each axis. See there. [DIAL]
const seamPad = 1.0

// seamScale is the scale that draws n art pixels as n*s screen pixels plus
// seamPad. An image with no size is drawn at s, which draws nothing anyway.
func seamScale(n int, s float64) float64 {
	if n <= 0 {
		return s
	}

	return (float64(n)*s + seamPad) / float64(n)
}

func (mr *MapRenderer) renderMapDebug(mapDebugVisLevel int, target d2interface.Surface, startX, startY, endX, endY int) {
	for tileY := startY; tileY < endY; tileY++ {
		for tileX := startX; tileX < endX; tileX++ {
			mr.viewport.PushTranslationWorld(float64(tileX), float64(tileY))
			mr.renderTileDebug(tileX, tileY, mapDebugVisLevel, target)
			mr.viewport.PopTranslation()
		}
	}
}

//nolint:funlen // doesn't make sense to split this function
func (mr *MapRenderer) renderEntityDebug(target d2interface.Surface) {
	entities := mr.mapEngine.Entities()

	for idx := range entities {
		e := entities[idx]
		pos := e.GetPosition()
		world := pos
		x, y := world.X()/subtilesPerTile, world.Y()/subtilesPerTile
		velocity := e.GetVelocity()
		velocity = *velocity.Clone()
		vx, vy := mr.viewport.WorldToOrtho(velocity.X(), velocity.Y())
		screenX, screenY := mr.viewport.WorldToScreen(x, y)

		offX, offY := dbgOffsetXY, -dbgOffsetXY

		entScreenXf, entScreenYf := mr.WorldToScreenF(e.GetPositionF())
		entScreenX := int(math.Floor(entScreenXf))
		entScreenY := int(math.Floor(entScreenYf))
		entityWidth, entityHeight := e.GetSize()
		halfWidth, halfHeight := entityWidth/two, entityHeight/two
		l, r := entScreenX-halfWidth, entScreenX+halfWidth
		t, b := entScreenY-halfHeight, entScreenY+halfHeight
		mx, my := mr.renderer.GetCursorPos()
		xWithin := (l <= mx) && (r >= mx)
		yWithin := (t <= my) && (b >= my)
		within := xWithin && yWithin

		boxLineColor := d2util.Color(magentaFullOpacity)
		boxHoverColor := d2util.Color(yellowFullOpacity)

		boxColor := boxLineColor

		if within {
			boxColor = boxHoverColor
		}

		stack := 0
		// box
		mr.viewport.PushTranslationWorld(x, y)

		target.PushTranslation(screenX, screenY)
		stack++

		target.PushTranslation(-halfWidth, -halfHeight)
		stack++

		target.DrawLine(0, entityHeight, boxColor)
		target.DrawLine(entityWidth, 0, boxColor)

		target.PushTranslation(entityWidth, entityHeight)
		stack++

		target.DrawLine(-entityWidth, 0, boxColor)
		target.DrawLine(0, -entityHeight, boxColor)
		target.PopN(stack)
		mr.viewport.PopTranslation()

		// hover
		if within {
			mr.viewport.PushTranslationWorld(x, y)
			target.PushTranslation(screenX, screenY)
			target.DrawLine(offX, offY, d2util.Color(whiteHalfOpacity))
			target.PushTranslation(offX+dbgBoxPadding, offY-dbgBoxPadding*two)
			target.PushTranslation(-dbgOffsetXY, -dbgOffsetXY)
			target.DrawRect(dbgBoxWidth, dbgBoxHeight, d2util.Color(blackQuarterOpacity))
			target.Pop()
			target.DrawTextf("World (%.2f, %.2f)\nVelocity (%.2f, %.2f)", x, y, vx, vy)
			target.Pop()
			target.DrawLine(int(vx), int(vy), d2util.Color(lightGreenFullOpacity))
			target.Pop()
			mr.viewport.PopTranslation()
		}
	}
}

// WorldToScreen returns the screen (pixel) position for the given isometric world position as two ints.
func (mr *MapRenderer) WorldToScreen(x, y float64) (screenX, screenY int) {
	return mr.viewport.WorldToScreen(x, y)
}

// WorldToScreenF returns the screen (pixel) position for the given isometric world position as two float64s.
func (mr *MapRenderer) WorldToScreenF(x, y float64) (screenX, screenY float64) {
	return mr.viewport.WorldToScreenF(x, y)
}

func (mr *MapRenderer) renderTileDebug(ax, ay, debugVisLevel int, target d2interface.Surface) {
	subTileColor := d2util.Color(lightBlueQuarterOpacity)
	tileColor := d2util.Color(whiteQuarterOpacity)
	tileCollisionColor := d2util.Color(redQuarterOpacity)

	screenX1, screenY1 := mr.viewport.WorldToScreen(float64(ax), float64(ay))
	screenX2, screenY2 := mr.viewport.WorldToScreen(float64(ax+1), float64(ay))
	screenX3, screenY3 := mr.viewport.WorldToScreen(float64(ax), float64(ay+1))

	target.PushTranslation(screenX1, screenY1)
	defer target.Pop()

	target.DrawLine(screenX2-screenX1, screenY2-screenY1, tileColor)
	target.DrawLine(screenX3-screenX1, screenY3-screenY1, tileColor)
	target.PushTranslation(-10, 10)
	target.DrawTextf("%v, %v", ax, ay)
	target.Pop()

	if debugVisLevel > 1 {
		for i := 1; i <= 4; i++ {
			x2 := i * orthoSubTileWidth
			y2 := i * orthoSubTileHeight

			target.PushTranslation(-x2, y2)
			target.DrawLine(orthoTileWidth, orthoTileHeight, subTileColor)
			target.Pop()

			target.PushTranslation(x2, y2)
			target.DrawLine(-orthoTileWidth, orthoTileHeight, subTileColor)
			target.Pop()
		}

		tile := mr.mapEngine.TileAt(ax, ay)

		for i, wall := range tile.Components.Walls {
			if wall.Type.Special() {
				target.PushTranslation(-20, 10+(i+1)*14) // nolint:gomnd // just for debug
				target.DrawTextf("s: %v-%v", wall.Style, wall.Sequence)
				target.Pop()
			}
		}

		for yy := 0; yy < 5; yy++ {
			for xx := 0; xx < 5; xx++ {
				isoX := (xx - yy) * orthoSubTileWidth
				isoY := (xx + yy) * orthoSubTileHeight

				blocked := tile.GetSubTileFlags(xx, yy).BlockWalk

				if blocked {
					target.PushTranslation(isoX+dbgCollisionOffsetX, isoY+dbgCollisionOffsetY)
					target.DrawRect(dbgCollisionSize, dbgCollisionSize, tileCollisionColor)
					target.Pop()
				}
			}
		}
	}
}

const (
	frameOverflow = 10
	frameLength   = 1.0 / frameOverflow
)

// Advance is called once per frame and maintains the MapRenderer's previous
// render timestamp and current frame.
func (mr *MapRenderer) Advance(elapsed float64) {
	mr.lastFrameTime += elapsed
	framesAdvanced := int(mr.lastFrameTime / frameLength)
	mr.lastFrameTime -= float64(framesAdvanced) * frameLength

	mr.currentFrame += framesAdvanced
	if mr.currentFrame >= frameOverflow {
		mr.currentFrame = 0
	}

	mr.Camera.Advance(elapsed)
}

func (mr *MapRenderer) loadPaletteForAct(levelType d2enum.RegionIdType) (d2interface.Palette,
	error) {
	var palettePath string

	switch levelType {
	case d2enum.RegionAct1Town, d2enum.RegionAct1Wilderness, d2enum.RegionAct1Cave, d2enum.RegionAct1Crypt,
		d2enum.RegionAct1Monestary, d2enum.RegionAct1Courtyard, d2enum.RegionAct1Barracks,
		d2enum.RegionAct1Jail, d2enum.RegionAct1Cathedral, d2enum.RegionAct1Catacombs, d2enum.RegionAct1Tristram:
		palettePath = d2resource.PaletteAct1
	case d2enum.RegionAct2Town, d2enum.RegionAct2Sewer, d2enum.RegionAct2Harem, d2enum.RegionAct2Basement,
		d2enum.RegionAct2Desert, d2enum.RegionAct2Tomb, d2enum.RegionAct2Lair, d2enum.RegionAct2Arcane:
		palettePath = d2resource.PaletteAct2
	case d2enum.RegionAct3Town, d2enum.RegionAct3Jungle, d2enum.RegionAct3Kurast, d2enum.RegionAct3Spider,
		d2enum.RegionAct3Dungeon, d2enum.RegionAct3Sewer:
		palettePath = d2resource.PaletteAct3
	case d2enum.RegionAct4Town, d2enum.RegionAct4Mesa, d2enum.RegionAct4Lava, d2enum.RegionAct5Lava:
		palettePath = d2resource.PaletteAct4
	case d2enum.RegonAct5Town, d2enum.RegionAct5Siege, d2enum.RegionAct5Barricade, d2enum.RegionAct5Temple,
		d2enum.RegionAct5IceCaves, d2enum.RegionAct5Baal:
		palettePath = d2resource.PaletteAct5
	default:
		return nil, errors.New("failed to find palette for region")
	}

	return mr.asset.LoadPalette(palettePath)
}

// ViewportToLeft moves the viewport to the left.
func (mr *MapRenderer) ViewportToLeft() {
	mr.viewport.toLeft()
}

// ViewportToRight moves the viewport to the right.
func (mr *MapRenderer) ViewportToRight() {
	mr.viewport.toRight()
}

// ViewportDefault resets the viewport to it's default position.
func (mr *MapRenderer) ViewportDefault() {
	mr.viewport.resetAlign()
}

// SetCameraTarget sets the Camera target
func (mr *MapRenderer) SetCameraTarget(position *d2vector.Position) {
	mr.Camera.SetTarget(position)
}

// SetCameraPosition sets the Camera position
func (mr *MapRenderer) SetCameraPosition(position *d2vector.Position) {
	mr.Camera.MoveTo(position)
}

// InvalidateImageCache the global region image cache. Call this when you are changing regions.
func (mr *MapRenderer) InvalidateImageCache() {
	mr.imageCacheRecords = nil
}

func (mr *MapRenderer) getImageCacheRecord(style, sequence byte, tileType d2enum.TileType, randomIndex byte) d2interface.Surface {
	lookupIndex := uint32(style)<<24 | uint32(sequence)<<16 | uint32(tileType)<<8 | uint32(randomIndex)
	return mr.imageCacheRecords[lookupIndex]
}

func (mr *MapRenderer) setImageCacheRecord(style, sequence byte, tileType d2enum.TileType, randomIndex byte, image d2interface.Surface) {
	lookupIndex := uint32(style)<<24 | uint32(sequence)<<16 | uint32(tileType)<<8 | uint32(randomIndex)

	if mr.imageCacheRecords == nil {
		mr.imageCacheRecords = make(map[uint32]d2interface.Surface)
	}

	mr.imageCacheRecords[lookupIndex] = image
}
