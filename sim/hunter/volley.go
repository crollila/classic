package hunter

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

func (hunter *Hunter) registerVolleySpell() {
	ranks := 3

	for i := ranks; i >= 1; i-- {
		config := hunter.getVolleyConfig(i)

		if config.RequiredLevel <= int(hunter.Level) {
			hunter.Volley = hunter.GetOrRegisterSpell(config)
			break
		}
	}
}

func (hunter *Hunter) getVolleyConfig(rank int) core.SpellConfig {
	spellId := [4]int32{0, 1510, 14294, 14295}[rank]
	baseDamage := [4]float64{0, 50, 65, 80}[rank]
	manaCost := [4]float64{0, 350, 420, 490}[rank]
	level := [4]int{0, 40, 50, 58}[rank]

	manaCostModifer := 100 - 2*hunter.Talents.Efficiency
	numTicks, tickLength := int32(6), time.Second
	cooldown := time.Second * 60
	coefficient := .056
	if hunter.Forever != nil {
		// Forever: the channel (1510/14294/14295) deals its damage through a Forever-only spell
		// per rank whose effect 0 is the Arcane damage per second (1.60.1: 70/91/112); the
		// channel's duration over its effect 2 period gives the ticks. Forever Efficiency names
		// Shots, Stings and melee abilities, which Volley is not.
		damageID := [4]int32{0, 1279721, 1279719, 1279715}[rank]
		baseDamage = hunter.ClientEffectValue(damageID, 0, [4]float64{0, 70, 91, 112}[rank])
		numTicks, tickLength = clientTicks(&hunter.Character, spellId, 2, numTicks, tickLength)
		manaCostModifer = 100
		if hunter.GameData != nil {
			// The client damage spells carry coefficient 0 and the channel's dummy 0.03; neither
			// is clearly the damage's spell power scaling, so Classic's 0.056 is kept.
			coefficient = gamedata.Use("hunter: Volley", "spell power coefficient", coefficient, "PROVISIONAL",
				"client damage spells carry 0 and the channel's dummy effect 0.03; Classic 0.056 kept")
			// The client lists no cooldown for Forever Volley. A missing cooldown is not proof of
			// removal (the client tables are partial), so Classic's 60 sec stays, recorded.
			if s := hunter.GameData.Spell(spellId); s != nil && max(s.CooldownMs, s.CategoryCooldownMs) > 0 {
				cooldown = clientCooldown(&hunter.Character, spellId, cooldown)
			} else {
				cooldown = time.Duration(gamedata.Use("hunter: Volley", "cooldown sec", cooldown.Seconds(), "PROVISIONAL",
					"client lists no cooldown; Classic 60 sec kept")) * time.Second
			}
		}
	}

	return core.SpellConfig{
		SpellCode:   SpellCode_HunterVolley,
		ActionID:    core.ActionID{SpellID: spellId},
		SpellSchool: core.SpellSchoolArcane,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagChanneled | core.SpellFlagAPL,

		RequiredLevel: level,
		Rank:          rank,

		ManaCost: core.ManaCostOptions{
			FlatCost:   manaCost,
			Multiplier: manaCostModifer,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    hunter.NewTimer(),
				Duration: cooldown,
			},
		},

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: fmt.Sprintf("Volley (Rank %d)", rank),
			},
			NumberOfTicks:    numTicks,
			TickLength:       tickLength,
			BonusCoefficient: coefficient,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				damage := baseDamage
				dot.Snapshot(target, damage, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeTick)
				}
			},
		},

		CritDamageBonus:  (1 + hunter.mortalShots()) * (1 + (0.05 * float64(hunter.Talents.Barrage))),
		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			hunter.Unit.AutoAttacks.DelayRangedUntil(sim, sim.CurrentTime+time.Duration(numTicks)*tickLength)
			spell.AOEDot().Apply(sim)
		},
	}
}
