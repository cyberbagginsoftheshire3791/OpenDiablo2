package d2hero

import "github.com/OpenDiablo2/OpenDiablo2/d2core/d2asset"

// HydrateSkills will load the SkillRecord & SkillDescriptionRecord from the asset manager, using the skill ID.
// This is done to avoid serializing the whole record data of HeroSkill to a game save or network packets.
// We cant do this while unmarshalling because there is no reference to the asset manager.
//
// A skill whose record is not loaded is dropped rather than dereferenced:
// Strigoi's game loads no skill table and its heroes carry no skills
// (CreateHeroSkillsState), so one arriving here is a Diablo II hero's.
func HydrateSkills(skills map[int]*HeroSkill, asset *d2asset.AssetManager) {
	for skillID, skill := range skills {
		if skill == nil {
			delete(skills, skillID)
			continue
		}

		skill.SkillRecord = asset.Records.Skill.Details[skillID]
		if skill.SkillRecord == nil {
			delete(skills, skillID)
			continue
		}

		skill.SkillDescriptionRecord = asset.Records.Skill.Descriptions[skill.SkillRecord.Skilldesc]
	}
}
