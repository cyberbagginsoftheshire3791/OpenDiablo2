package d2mapentity

// Entity-level state for the Phase 3 playtest harness (P3 spec §3.5): the
// player and NPC kinds satisfy d2harness.Stateful so strigoi_get_entity can
// show them and the state digest can hash them. Compiled in every build;
// nothing here depends on the harness itself. Values must stay JSON-encodable
// and free of presentation noise (no animation frame counters, no wall time):
// whatever appears here is asserted identical across seeded launches.

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
		"has_paths":   v.HasPaths,
		"paths":       len(v.Paths),
		"path_index":  v.path,
		"action":      v.action,
		"repetitions": v.repetitions,
		"done":        v.isDone,
		"path_len":    len(v.mapEntity.path),
		"speed":       v.Speed,
	}

	if v.monstatRecord != nil {
		state["monstat"] = v.monstatRecord.Key
		state["monstat_id"] = v.monstatRecord.ID
	}

	if v.composite != nil {
		state["animation_mode"] = v.composite.GetAnimationMode()
		state["direction"] = v.composite.GetDirection()
	}

	return state
}
