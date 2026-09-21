package warlock

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

const ConflagrateRanks = 4

// foreverConflagrateRank is one Forever Conflagrate rank. The beta client adds two ranks
// below Classic's four (1293817 at 25, 1293818 at 32, values from their tooltips; no
// override exists for them). Classic's four ranks keep Classic-scale values here because
// the client overrides for 17962-18932 scale them at registration (to 134-170, 178-222,
// 219-273 and 251-313); 18930 uses Era's 326-407 so that ratio lands on the tooltip.
type foreverConflagrateRank struct {
	id       int32
	min, max float64
	mana     float64
	level    int
}

var foreverConflagrateRanks = []foreverConflagrateRank{
	{1293817, 88, 110, 100, 25}, {1293818, 112, 142, 130, 32},
	{17962, 249, 316, 165, 40}, {18930, 326, 407, 200, 48}, {18931, 395, 491, 230, 54}, {18932, 447, 557, 255, 60},
}

func (warlock *Warlock) getConflagrateConfig(rank int) core.SpellConfig {
	var spellId int32
	var baseDamageMin, baseDamageMax, manaCost float64
	var level int
	if warlock.Forever != nil {
		r := foreverConflagrateRanks[rank-1]
		spellId, baseDamageMin, baseDamageMax, manaCost, level = r.id, r.min, r.max, r.mana, r.level
	} else {
		spellId = [ConflagrateRanks + 1]int32{0, 17962, 18930, 18931, 18932}[rank]
		baseDamageMin = [ConflagrateRanks + 1]float64{0, 249, 319, 395, 447}[rank]
		baseDamageMax = [ConflagrateRanks + 1]float64{0, 316, 400, 491, 557}[rank]
		manaCost = [ConflagrateRanks + 1]float64{0, 165, 200, 230, 255}[rank]
		level = [ConflagrateRanks + 1]int{0, 40, 48, 54, 60}[rank]
	}

	spCoeff := 0.429

	return core.SpellConfig{
		SpellCode:     SpellCode_WarlockConflagrate,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolFire,
		DefenseType:   core.DefenseTypeMagic,
		ProcMask:      core.ProcMaskSpellDamage,
		Flags:         core.SpellFlagAPL | WarlockFlagDestruction,
		Rank:          rank,
		RequiredLevel: level,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: time.Second * 10,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warlock.getActiveImmolateSpell(target) != nil
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: spCoeff,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(baseDamageMin, baseDamageMax)

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)

			immoSpell := warlock.getActiveImmolateSpell(target)
			if immoSpell != nil && !(warlock.ForeverRank("warlock.talent.shadow-and-flame") > 0 && sim.Proc(warlock.ForeverValue("warlock.talent.shadow-and-flame", 4, 0)/100, "Forever Shadow and Flame")) {
				immoSpell.Dot(target).Deactivate(sim)
			}
		},
	}
}

func (warlock *Warlock) registerConflagrateSpell() {
	if !warlock.Talents.Conflagrate {
		return
	}

	warlock.Conflagrate = make([]*core.Spell, 0)
	ranks := ConflagrateRanks
	if warlock.Forever != nil {
		ranks = len(foreverConflagrateRanks)
	}
	for rank := 1; rank <= ranks; rank++ {
		config := warlock.getConflagrateConfig(rank)

		if config.RequiredLevel <= int(warlock.Level) {
			warlock.Conflagrate = append(warlock.Conflagrate, warlock.GetOrRegisterSpell(config))
		}
	}
}
