package d2mapentity

// Entity-level state for the Phase 3 playtest harness (P3 spec §3.5): the
// player and NPC kinds satisfy d2harness.Stateful so strigoi_get_entity can
// show them and the state digest can hash them. Compiled in every build;
// nothing here depends on the harness itself. Values must stay JSON-encodable
// and free of presentation noise (no animation frame counters, no wall time):
// whatever appears here is asserted identical across seeded launches.

// harnessMotion adds where the entity is walking (M4.6 B1): the point it is
// stepping toward now and the waypoints still ahead of it, in world tiles.
// path_len alone cannot tell two walks of the same length apart, and the
// world save carries both -- so a resume that dropped one is visible here
// before it is visible as a wolf walking the wrong way.
func (m *mapEntity) harnessMotion(state map[string]interface{}) {
	t := m.Target.World()
	state["target"] = [2]float64{t.X(), t.Y()}

	waypoints := make([][2]float64, 0, len(m.path))

	for i := range m.path {
		w := m.path[i].World()
		waypoints = append(waypoints, [2]float64{w.X(), w.Y()})
	}

	state["waypoints"] = waypoints

	// And the velocity it last stepped with, in world tiles per step (the B4b
	// review fixes, BUG-82): the world save carries it, so a resume that lost
	// it is visible here and not only in the renderer's debug overlay. It is
	// recomputed by the next Step, and exact across a relaunch -- the file
	// keeps it bit for bit.
	//
	// A ZERO IS REPORTED AS ZERO, whatever its sign. A walk that ends scales
	// its velocity to length 0, which leaves -0 in a component it was
	// heading down (0 * -y); a player loaded standing (rule 4) has +0. The
	// two are the same standstill -- the next Step recomputes either -- and
	// JSON writes them "0" and "-0": the first whole run with this field was
	// red at act 4 on exactly that, his velocity [0,-0] at T and [0,0]
	// resumed (wt-b4b-fix\saveresume-1-whole.txt).
	vel := m.velocity.Clone()
	vel.DivideScalar(subtilesPerTile)
	state["velocity"] = [2]float64{unsignedZero(vel.X()), unsignedZero(vel.Y())}
}

// unsignedZero is f, with -0 made +0.
func unsignedZero(f float64) float64 {
	if f == 0 {
		return 0
	}

	return f
}

// HarnessState reports the player's observable simulation state.
func (p *Player) HarnessState() map[string]interface{} {
	state := map[string]interface{}{
		"name":           p.name,
		"class":          int(p.Class),
		"act":            p.Act,
		"gold":           p.Gold,
		"in_town":        p.isInTown,
		"running":        p.isRunning,
		"run_toggled":    p.isRunToggled,
		"casting":        p.isCasting,
		"animation_mode": p.animationMode,
		"path_len":       len(p.path),
		"speed":          p.Speed,
	}

	p.harnessMotion(state)

	if p.Stats != nil {
		state["level"] = p.Stats.Level
		state["experience"] = p.Stats.Experience
		state["health"] = p.Stats.Health
		state["max_health"] = p.Stats.MaxHealth
		state["mana"] = p.Stats.Mana
		state["max_mana"] = p.Stats.MaxMana
		state["stamina"] = p.Stats.Stamina
		state["max_stamina"] = p.Stats.MaxStamina
		state["strength"] = p.Stats.Strength
		state["dexterity"] = p.Stats.Dexterity
		state["vitality"] = p.Stats.Vitality
		state["energy"] = p.Stats.Energy
	}

	if p.composite != nil {
		state["direction"] = p.composite.GetDirection()

		// Which body draws him: Diablo II's composite or a PNG hero (M5.3).
		state["body"] = "composite"
		if png, ok := p.composite.(*pngBody); ok {
			state["body"] = "png"
			state["body_sheet"] = png.Sheet()
		}
	}

	if p.LeftSkill != nil && p.LeftSkill.SkillRecord != nil {
		state["left_skill"] = p.LeftSkill.ID
	}

	if p.RightSkill != nil && p.RightSkill.SkillRecord != nil {
		state["right_skill"] = p.RightSkill.ID
	}

	return state
}

// HarnessState reports the NPC's observable simulation state: which monstat
// it is, where it is on its waypoint loop, and what it is doing.
func (v *NPC) HarnessState() map[string]interface{} {
	state := map[string]interface{}{
		"name":        v.name,
		"name_key":    v.NameKey(), // who stands in for him ("Warriv"), whatever his label says
		"has_paths":   v.HasPaths,
		"paths":       len(v.Paths),
		"path_index":  v.path,
		"action":      v.action,
		"repetitions": v.repetitions,
		"done":        v.isDone,
		"path_len":    len(v.mapEntity.path),
		"speed":       v.Speed,
	}

	v.harnessMotion(state)

	if v.monstatRecord != nil {
		state["monstat"] = v.monstatRecord.Key
		state["monstat_id"] = v.monstatRecord.ID
	}

	if v.composite != nil {
		state["animation_mode"] = v.composite.GetAnimationMode()
		state["direction"] = v.composite.GetDirection()
	}

	// The pose flags the world save carries (the B4b review fixes, BUG-82):
	// the action held until it has played through ("" when none; "action"
	// above is the patrol's), and the Dead pose.
	state["held"] = ""
	if v.held {
		state["held"] = v.heldMode.String()
	}

	state["corpse"] = v.corpse

	return state
}
