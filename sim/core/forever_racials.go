package core

import (
	"github.com/wowsims/classic/sim/core/proto"
)

func (c *Character) applyForeverRacials() {
	if c.Forever == nil {
		return
	}
	c.applyExpandedForeverRacials()
	if c.HasForeverMechanic("racials.tauren.endurance") {
		c.foreverAllHit(1)
	} // Classic already grants the 5% health.
	specs := []struct {
		id     string
		weapon proto.WeaponType
		crit   float64
	}{
		{"racials.human.sword-specialization", proto.WeaponType_WeaponTypeSword, 2},
		{"racials.orc.axe-specialization", proto.WeaponType_WeaponTypeAxe, 1},
		{"racials.dwarf.mace-specialization", proto.WeaponType_WeaponTypeMace, 1},
	}
	for _, spec := range specs {
		if c.HasForeverMechanic(spec.id) {
			// A weapon in either hand qualifies. Proc masks exclude white attacks because
			// the observed wording explicitly says spells and abilities.
			c.OnSpellRegistered(func(s *Spell) {
				if s.ProcMask.Matches(ProcMaskMeleeSpecial | ProcMaskRangedSpecial | ProcMaskSpellDamage | ProcMaskSpellHealing) {
					equipped := func() bool { return c.MainHand().WeaponType == spec.weapon || c.OffHand().WeaponType == spec.weapon }
					// Existing weapon-swap callbacks update the same per-spell crit contribution.
					applied := false
					update := func() {
						active := equipped()
						if active != applied {
							if active {
								s.BonusCritRating += spec.crit
							} else {
								s.BonusCritRating -= spec.crit
							}
							applied = active
						}
					}
					update()
					c.RegisterOnItemSwap(func(sim *Simulation) { update() })
				}
			})
		}
	}
	if c.HasForeverMechanic("racials.dwarf.big-game-hunter") {
		c.Env.RegisterPostFinalizeEffect(func() {
			for _, target := range c.Env.Encounter.Targets {
				if target.MobType == proto.MobType_MobTypeBeast {
					for _, at := range c.AttackTables[target.UnitIndex] {
						at.DamageDealtMultiplier *= 1.05
						at.CritMultiplier *= 1.05
					}
				}
			}
		})
	}
}
