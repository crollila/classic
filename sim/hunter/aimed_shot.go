package hunter

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

// Forever beta client (1.60.1) Aimed Shot: baseline, learned at the Classic levels,
// 2 sec cast, 6 sec cooldown shared with Multi-Shot, "increases ranged damage by"
// 20/34/55/89/125/166. Mana costs are unchanged from Classic.
var foreverAimedShotBonus = [7]float64{0, 20, 34, 55, 89, 125, 166}

// Forever Sniper Shot (talent): ranks learned at 40/48/58, +160/225/295, 365 mana,
// 4 sec cast, 15 sec cooldown of its own.
var sniperShotRanks = []struct {
	id    int32
	level int32
	bonus float64
}{{1310687, 40, 160}, {1310785, 48, 225}, {1310786, 58, 295}}

// sniperShotRank returns the Sniper Shot rank a hunter of the given level knows. The
// talent itself grants rank 1, so a talented hunter always has at least that.
func sniperShotRank(level int32) (id int32, bonus float64) {
	id, bonus = sniperShotRanks[0].id, sniperShotRanks[0].bonus
	for _, r := range sniperShotRanks {
		if r.level <= level {
			id, bonus = r.id, r.bonus
		}
	}
	return id, bonus
}

func (hunter *Hunter) getAimedShotConfig(rank int, timer *core.Timer) core.SpellConfig {
	spellId := [7]int32{0, 19434, 20900, 20901, 20902, 20903, 20904}[rank]
	baseDamage := [7]float64{0, 70, 125, 200, 330, 460, 600}[rank]
	manaCost := [7]float64{0, 75, 115, 160, 210, 260, 310}[rank]
	level := [7]int{0, 20, 28, 36, 44, 52, 60}[rank]

	castTime := 3500 * time.Millisecond
	cooldown := 6 * time.Second
	if hunter.Forever != nil {
		baseDamage = foreverAimedShotBonus[rank]
		castTime = 2 * time.Second
	}
	return core.SpellConfig{
		SpellCode:     SpellCode_HunterAimedShot,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolPhysical,
		DefenseType:   core.DefenseTypeRanged,
		ProcMask:      core.ProcMaskRangedSpecial,
		Flags:         core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagShot,
		CastType:      proto.CastType_CastTypeRanged,
		Rank:          rank,
		RequiredLevel: level,
		MissileSpeed:  24,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: castTime,
			},
			CD: core.Cooldown{
				Timer:    timer,
				Duration: cooldown,
			},
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				cast.CastTime = spell.CastTime()
				hunter.Unit.AutoAttacks.CancelAutoSwing(sim)
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
			CastTime: func(spell *core.Spell) time.Duration {
				return time.Duration(float64(spell.DefaultCast.CastTime) / hunter.RangedSwingSpeed())
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			if hunter.Forever != nil {
				return hunter.DistanceFromTarget >= 8
			}
			return hunter.DistanceFromTarget >= core.MinRangedAttackDistance
		},

		CritDamageBonus: hunter.mortalShots(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := hunter.AutoAttacks.Ranged().CalculateNormalizedWeaponDamage(sim, spell.RangedAttackPower(target, false)) +
				hunter.AmmoDamageBonus +
				baseDamage

			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
			hunter.Unit.AutoAttacks.EnableAutoSwing(sim)
			spell.WaitTravelTime(sim, func(s *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	}
}

// getSniperShotConfig is the Forever Sniper Shot talent: the Aimed Shot shape with its
// own spell code (Barrage names Aimed Shot, not Sniper Shot), timer and per-rank bonus.
func (hunter *Hunter) getSniperShotConfig() core.SpellConfig {
	config := hunter.getAimedShotConfig(1, hunter.NewTimer())
	_, bonus := sniperShotRank(hunter.Level)
	config.SpellCode = SpellCode_HunterSniperShot
	config.ActionID = hunter.ForeverAction("hunter.talent.sniper-shot")
	config.Rank = 0
	config.RequiredLevel = 0
	config.ManaCost.FlatCost = 365
	config.Cast.DefaultCast.CastTime = 4 * time.Second
	config.Cast.CD.Duration = 15 * time.Second
	config.ApplyEffects = func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		baseDamage := hunter.AutoAttacks.Ranged().CalculateNormalizedWeaponDamage(sim, spell.RangedAttackPower(target, false)) +
			hunter.AmmoDamageBonus + bonus
		result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)
		hunter.Unit.AutoAttacks.EnableAutoSwing(sim)
		spell.WaitTravelTime(sim, func(s *core.Simulation) {
			spell.DealDamage(sim, result)
		})
	}
	return config
}

// registerAimedShotSpell registers Aimed Shot. In Forever it is a baseline spell sharing
// its cooldown with Multi-Shot (multiShotTimer); the Sniper Shot talent adds a separate
// spell rather than replacing it. Classic keeps the talent-gated Aimed Shot.
func (hunter *Hunter) registerAimedShotSpell(timer *core.Timer, multiShotTimer *core.Timer) {
	if hunter.Forever != nil {
		for i := 1; i <= 6; i++ {
			config := hunter.getAimedShotConfig(i, multiShotTimer)
			if config.RequiredLevel <= int(hunter.Level) {
				hunter.AimedShot = hunter.GetOrRegisterSpell(config)
			}
		}
		if hunter.ForeverRank("hunter.talent.sniper-shot") > 0 {
			hunter.SniperShot = hunter.GetOrRegisterSpell(hunter.getSniperShotConfig())
		}
		return
	}

	if !hunter.Talents.AimedShot {
		return
	}

	for i := 1; i <= 6; i++ {
		config := hunter.getAimedShotConfig(i, timer)

		if config.RequiredLevel <= int(hunter.Level) {
			hunter.AimedShot = hunter.GetOrRegisterSpell(config)
		}
	}
}
