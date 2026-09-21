package hunter

import (
	"github.com/wowsims/classic/sim/core"
)

var wingClipLevel = [4]int{0, 12, 38, 60}

func (hunter *Hunter) getWingClipConfig(rank int) core.SpellConfig {
	spellId := [4]int32{0, 2974, 14267, 14268}[rank]
	baseDamage := [4]float64{0, 5, 25, 50}[rank]
	manaCost := [4]float64{0, 40, 60, 80}[rank]
	level := wingClipLevel[rank]

	return core.SpellConfig{
		SpellCode:     SpellCode_HunterWingClip,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolPhysical,
		DefenseType:   core.DefenseTypeMelee,
		ProcMask:      core.ProcMaskMeleeMHSpecial,
		Flags:         core.SpellFlagMeleeMetrics | core.SpellFlagAPL | core.SpellFlagBinary,
		Rank:          rank,
		RequiredLevel: level,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return hunter.DistanceFromTarget <= core.MaxMeleeAttackDistance
		},

		CritDamageBonus:  hunter.meleeMortalShots(),
		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
		},
	}
}

func (hunter *Hunter) registerWingClipSpell() {
	rank := rankForLevel(wingClipLevel[:], hunter.Level)
	if rank == 0 {
		return
	}

	config := hunter.getWingClipConfig(rank)
	hunter.WingClip = hunter.GetOrRegisterSpell(config)
}
