package d2mapentity

import (
	"fmt"
	"math/rand"
	"sync"

	"github.com/google/uuid"

	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2enum"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2interface"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2math/d2vector"
	"github.com/OpenDiablo2/OpenDiablo2/d2common/d2resource"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2hero"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2inventory"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2item/diablo2item"
	"github.com/OpenDiablo2/OpenDiablo2/d2core/d2records"
)

const (
	subtilesPerTile       = 5
	retailFps             = 25.0
	millisecondsPerSecond = 1000.0
)

// NewMapEntityFactory creates a MapEntityFactory instance with the given asset manager
func NewMapEntityFactory(asset *d2asset.AssetManager) (*MapEntityFactory, error) {
	itemFactory, err := diablo2item.NewItemFactory(asset)
	if err != nil {
		return nil, err
	}

	stateFactory, err := d2hero.NewHeroStateFactory(asset)
	if err != nil {
		return nil, err
	}

	entityFactory := &MapEntityFactory{
		HeroStateFactory: stateFactory,
		asset:            asset,
		item:             itemFactory,
	}

	return entityFactory, nil
}

// MapEntityFactory creates map entities for the MapEngine
type MapEntityFactory struct {
	*d2hero.HeroStateFactory
	asset *d2asset.AssetManager
	item  *diablo2item.ItemFactory
	rng   *rand.Rand // the world RNG (P3 E4); nil falls back to the global generator

	// The next-id seam (M4.6 B2b, entity_id.go): the id the next NewNPC or
	// NewCreature takes, and every id handed out through it, so none is
	// handed out twice. seamMu guards both (the B2b review: the factory is
	// reached from the game's goroutine and, in a harness build, the MCP
	// handler's; a load sets and a construction takes, and neither may see
	// the other half-done).
	seamMu       sync.Mutex
	nextEntityID string
	givenIDs     map[string]bool
}

// SetRand hands the factory the world RNG, so entity creation rolls
// (equipment variants) and per-entity behaviour seeds derive from the map
// seed. MapEngine.SetSeed calls it.
func (f *MapEntityFactory) SetRand(r *rand.Rand) {
	f.rng = r
}

// WorldRand is the RNG SetRand handed the factory (nil before), read-only. It
// is how the map engine's restore test sees that a restored world stream
// reached the factory that rolls every new entity (M4.6 B1 review, C4).
func (f *MapEntityFactory) WorldRand() *rand.Rand {
	return f.rng
}

func (f *MapEntityFactory) randIntn(n int) int {
	if f.rng != nil {
		return f.rng.Intn(n)
	}

	// nolint:gosec // not cryptographic; pre-seed fallback only
	return rand.Intn(n)
}

func (f *MapEntityFactory) randInt63() int64 {
	if f.rng != nil {
		return f.rng.Int63()
	}

	// nolint:gosec // not cryptographic; pre-seed fallback only
	return rand.Int63()
}

// NewAnimatedEntity creates an instance of AnimatedEntity
func NewAnimatedEntity(x, y int, animation d2interface.Animation) *AnimatedEntity {
	entity := &AnimatedEntity{
		mapEntity: newMapEntity(x, y),
		animation: animation,
	}
	entity.mapEntity.directioner = entity.rotate

	return entity
}

// NewCreature loads a project-owned PNG animation and creates a creature that
// can move through the same map and world systems as an inherited NPC. standIn
// preserves the world-RNG draws the replaced NPC construction used to consume;
// changing art must not silently retune every later arrival in the night.
// CreatureAnimationPaths names the optional independent sheet for each mode.
// Idle is required and is the fallback while a creature's art is incomplete.
type CreatureAnimationPaths struct {
	Idle, Walk, Attack, Hit, Death, Dead string
}

func (f *MapEntityFactory) NewCreature(x, y int, name string, paths CreatureAnimationPaths, direction int,
	standIn *d2records.MonStatRecord) (*Creature, error) {
	// Taken first, whatever follows: a construction that fails must not
	// leave its id waiting for the next entity (entity_id.go).
	id := f.takeEntityID()

	if standIn != nil {
		_ = f.randInt63() // NewNPC's per-entity behaviour seed.

		if standInEx := f.asset.Records.Monster.Stats2[standIn.ExtraDataKey]; standInEx != nil {
			for _, options := range standInEx.EquipmentOptions {
				_ = f.selectEquip(options)
			}
		}
	}

	definitions := map[creatureMode]string{
		creatureIdle: paths.Idle, creatureWalk: paths.Walk,
		creatureAttack: paths.Attack, creatureHit: paths.Hit,
		creatureDeath: paths.Death, creatureDead: paths.Dead,
	}
	animations := make(map[creatureMode]d2interface.Animation, len(definitions))
	for mode, path := range definitions {
		if path == "" {
			continue
		}
		animation, err := f.asset.LoadAnimation(path, "")
		if err != nil {
			return nil, fmt.Errorf("load creature %q %s animation: %w", name, mode, err)
		}
		animation.PlayForward()
		animations[mode] = animation
	}

	creature, err := newCreature(x, y, id, name, animations, direction)
	if err != nil {
		return nil, err
	}

	// setMode only ever lands on a mode that loaded (a missing one falls back
	// to idle), so the map needs no fallback of its own.
	creature.sheets = definitions

	return creature, nil
}

// NewPlayer creates a new player entity and returns a pointer to it.
func (f *MapEntityFactory) NewPlayer(id, name string, x, y, direction int, heroType d2enum.Hero,
	stats *d2hero.HeroStatsState, skills map[int]*d2hero.HeroSkill, equipment *d2inventory.CharacterEquipment,
	leftSkill, rightSkill, gold int) *Player {
	// The next-id seam (entity_id.go): a waiting id is spent here, first,
	// and never worn -- the player's id is his connection's, and a save
	// names him by a word, not an id.
	f.spendEntityID()

	layerEquipment := &[d2enum.CompositeTypeMax]string{
		d2enum.CompositeTypeHead:      equipment.Head.GetArmorClass(),
		d2enum.CompositeTypeTorso:     equipment.Torso.GetArmorClass(),
		d2enum.CompositeTypeLegs:      equipment.Legs.GetArmorClass(),
		d2enum.CompositeTypeRightArm:  equipment.RightArm.GetArmorClass(),
		d2enum.CompositeTypeLeftArm:   equipment.LeftArm.GetArmorClass(),
		d2enum.CompositeTypeRightHand: equipment.RightHand.GetItemCode(),
		d2enum.CompositeTypeLeftHand:  equipment.LeftHand.GetItemCode(),
		d2enum.CompositeTypeShield:    equipment.Shield.GetItemCode(),
	}

	// The body: a PNG hero when one is set (M5.3, player_body.go), else
	// Diablo II's composite for the class -- and the composite too when the
	// hero art is refused, with the reason kept for HeroArtReport.
	var body playerBody

	if p := heroArtPath(); p != "" {
		hero, err := f.loadHeroBody(p, equipment.RightHand.GetWeaponClass(), direction)
		recordHeroArt(p, err)

		if err == nil {
			body = hero
		} else {
			fmt.Printf("hero art %s refused, drawing the Diablo II hero instead: %v\n", p, err)
		}
	}

	if body == nil {
		composite, err := f.asset.LoadComposite(d2enum.ObjectTypePlayer, heroType.GetToken(),
			d2resource.PaletteUnits)
		if err != nil {
			panic(err)
		}

		if err := composite.SetMode(d2enum.PlayerAnimationModeTownNeutral, equipment.RightHand.GetWeaponClass()); err != nil {
			panic(err)
		}

		composite.SetDirection(direction)

		if err := composite.Equip(layerEquipment); err != nil {
			fmt.Printf("failed to equip, err: %v\n", err)
		}

		body = compositeBody{composite}
	}

	// The run drain is the hero manifest's in Strigoi's game, charstats.txt's
	// under -classic (d2hero.StaminaRunDrain); NextLevelExp is 0 without
	// experience.txt, which Strigoi's game does not load (M5.3's tables burst).
	drain := d2hero.StaminaRunDrain(f.asset, heroType)

	stats.NextLevelExp = f.asset.Records.GetExperienceBreakpoint(heroType, stats.Level)
	stats.Stamina = float64(stats.MaxStamina)

	heroState, _ := f.CreateHeroState(name, heroType, stats)

	result := &Player{
		mapEntity:       newMapEntity(x, y),
		composite:       body,
		staminaRunDrain: drain,
		Equipment:       equipment,
		Stats:           heroState.Stats,
		Skills:          heroState.Skills,
		LeftSkill:       heroState.Skills[leftSkill],
		RightSkill:      heroState.Skills[rightSkill],
		name:            name,
		Class:           heroType,
		//nameLabel:    d2ui.NewLabel(d2resource.FontFormal11, d2resource.PaletteStatic),
		isRunToggled: false,
		isInTown:     true,
		isRunning:    false,
		Gold:         gold,
		Act:          1,
	}

	result.mapEntity.uuid = id
	result.SetSpeed(baseWalkSpeed)
	result.mapEntity.directioner = result.rotate

	return result
}

// NewMissile creates a new Missile and initializes it's animation.
func (f *MapEntityFactory) NewMissile(x, y int, record *d2records.MissileRecord) (*Missile, error) {
	// The next-id seam (entity_id.go): a waiting id is spent here, first,
	// and never worn -- a missile is not saved.
	f.spendEntityID()

	animation, err := f.asset.LoadAnimation(
		fmt.Sprintf("%s/%s.dcc", d2resource.MissileData, record.Animation.CelFileName),
		d2resource.PaletteUnits,
	)
	if err != nil {
		return nil, err
	}

	if record.Animation.HasSubLoop {
		animation.SetSubLoop(record.Animation.SubStartingFrame, record.Animation.SubEndingFrame)
	}

	animation.SetEffect(d2enum.DrawEffectModulate)
	animation.SetPlayLoop(record.Animation.LoopAnimation)
	animation.PlayForward()
	entity := NewAnimatedEntity(x, y, animation)

	result := &Missile{
		AnimatedEntity: entity,
		record:         record,
	}
	result.Speed = float64(record.Velocity)

	return result, nil
}

// NewItem creates an item map entity
func (f *MapEntityFactory) NewItem(x, y int, codes ...string) (*Item, error) {
	// The next-id seam (entity_id.go): a waiting id is spent here, first,
	// and never worn -- an item on the ground is not saved.
	f.spendEntityID()

	item, err := f.item.NewItem(codes...)

	if err != nil {
		return nil, err
	}

	filename := item.CommonRecord().FlippyFile
	filepath := fmt.Sprintf("%s/%s.DC6", d2resource.ItemGraphics, filename)
	animation, err := f.asset.LoadAnimation(filepath, d2resource.PaletteUnits)

	if err != nil {
		return nil, err
	}

	animation.PlayForward()
	animation.SetPlayLoop(false)
	entity := NewAnimatedEntity(x*subtilesPerTile, y*subtilesPerTile, animation)

	result := &Item{
		AnimatedEntity: entity,
		Item:           item,
	}

	return result, nil
}

// NewNPC creates a new NPC and returns a pointer to it.
func (f *MapEntityFactory) NewNPC(x, y int, monstat *d2records.MonStatRecord, direction int) (*NPC, error) {
	// https://github.com/OpenDiablo2/OpenDiablo2/issues/803
	result := &NPC{
		mapEntity:     newMapEntityWithID(x, y, f.takeEntityID()),
		HasPaths:      false,
		monstatRecord: monstat,
		monstatEx:     f.asset.Records.Monster.Stats2[monstat.ExtraDataKey],
	}

	// Per-entity behaviour RNG, seeded from the world RNG at creation time
	// (creation order is deterministic under a seeded map), so idle-timing
	// draws never interleave across entities via a shared stream (P3 E4).
	result.rng = rand.New(rand.NewSource(f.randInt63()))

	var equipment [16]string

	for compType, opts := range result.monstatEx.EquipmentOptions {
		equipment[compType] = f.selectEquip(opts)
	}

	composite, err := f.asset.LoadComposite(d2enum.ObjectTypeCharacter, monstat.AnimationDirectoryToken,
		d2resource.PaletteUnits)

	if err != nil {
		return nil, err
	}

	result.composite = composite

	if err := composite.SetMode(d2enum.MonsterAnimationModeNeutral,
		result.monstatEx.BaseWeaponClass); err != nil {
		return nil, err
	}

	if err := composite.Equip(&equipment); err != nil {
		return nil, err
	}

	result.SetSpeed(float64(monstat.SpeedBase))
	result.mapEntity.directioner = result.rotate

	result.composite.SetDirection(direction)

	if result.monstatRecord != nil && result.monstatRecord.IsInteractable {
		result.name = f.asset.TranslateString(result.monstatRecord.NameString)
	}

	return result, nil
}

// NewCastOverlay creates a cast overlay map entity
func (f *MapEntityFactory) NewCastOverlay(x, y int, overlayRecord *d2records.OverlayRecord) (*CastOverlay, error) {
	// The next-id seam (entity_id.go): a waiting id is spent here, first,
	// and never worn -- an overlay is not saved.
	f.spendEntityID()

	animation, err := f.asset.LoadAnimationWithEffect(
		fmt.Sprintf("/data/Global/Overlays/%s.dcc", overlayRecord.Filename),
		d2resource.PaletteUnits,
		d2enum.DrawEffectModulate,
	)

	if err != nil {
		return nil, err
	}

	animation.Rewind()
	animation.ResetPlayedCount()

	animationSpeed := float64(overlayRecord.AnimRate*retailFps) / millisecondsPerSecond
	playLoop := false // https://github.com/OpenDiablo2/OpenDiablo2/issues/804

	animation.SetPlayLength(animationSpeed)
	animation.SetPlayLoop(playLoop)
	animation.PlayForward()

	targetX := x + overlayRecord.XOffset
	targetY := y + overlayRecord.YOffset

	entity := NewAnimatedEntity(targetX, targetY, animation)

	result := &CastOverlay{
		AnimatedEntity: entity,
		record:         overlayRecord,
		playLoop:       playLoop,
	}

	return result, nil
}

// NewObject creates an instance of AnimatedComposite
func (f *MapEntityFactory) NewObject(x, y int, objectRec *d2records.ObjectDetailRecord,
	palettePath string) (*Object, error) {
	// The next-id seam (entity_id.go): an object mints its own id from the
	// uuid stream, so a waiting id is spent here, first, and never worn --
	// objects are stamped by the map and never saved. (Until the B2b review
	// NewObject took no part; the seam test now reads an inline uuid.New as
	// a birth, and the rule is every construction, no exceptions.)
	f.spendEntityID()

	locX, locY := float64(x), float64(y)
	entity := &Object{
		uuid:         uuid.New().String(),
		objectRecord: objectRec,
		Position:     d2vector.NewPosition(locX, locY),
		name:         f.asset.TranslateString(objectRec.Name),
	}
	if err := f.asset.EnsureRecords(d2resource.ObjectType); err != nil {
		return nil, err
	}

	objectType := f.asset.Records.Object.Types[objectRec.Index]

	composite, err := f.asset.LoadComposite(d2enum.ObjectTypeItem, objectType.Token,
		palettePath)
	if err != nil {
		return nil, err
	}

	entity.composite = composite

	err = entity.setMode(d2enum.ObjectAnimationModeNeutral, 0, false)
	if err != nil {
		return nil, err
	}

	_, err = initObject(entity)
	if err != nil {
		return nil, err
	}

	return entity, nil
}
