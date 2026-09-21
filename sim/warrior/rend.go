package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerRendSpell() {

	rank, spellID := warrior.trainerRank("Rend")
	if rank == 0 {
		return
	}
	rend := struct {
		ticks   int32
		damage  float64
		spellID int32
	}{spellID: spellID, damage: rendTick[rank-1], ticks: rendTicks[rank-1]}
	// Tick count: the client spell's duration over its effect 0 period. The tick damage stays the
	// rank table's Classic value on purpose: Rend is not a melee-defense spell, so the client-data
	// layer (core/forever_clientdata.go) already scales it by the client's Forever/Classic effect 0
	// ratio; reading the client tick here as well would apply a client change twice.
	tickLength := time.Second * 3
	if client := warrior.ClientSpell(spellID); client != nil && client.DurationMs > 0 && client.Effect(0) != nil && client.Effect(0).PeriodMs > 0 {
		rend.ticks = int32(client.DurationMs / client.Effect(0).PeriodMs)
		tickLength = time.Duration(client.Effect(0).PeriodMs) * time.Millisecond
	}

	baseDamage := rend.damage

	damageMultiplier := []float64{1, 1.15, 1.25, 1.35}[warrior.Talents.ImprovedRend]
	if warrior.ForeverRank("warrior.talent.improved-rend") > 0 {
		// Client effect 0: bleed damage percent (12/23/35).
		damageMultiplier = 1 + clientTalent(&warrior.Character, "warrior.talent.improved-rend", 0, 1, warrior.ForeverValue("warrior.talent.improved-rend", 0, 0))/100
	}

	warrior.Rend = warrior.RegisterSpell(BattleStance|DefensiveStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorRend,
		ActionID:    core.ActionID{SpellID: rend.spellID},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagAPL | core.SpellFlagNoOnCastComplete | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost:   10,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		DamageMultiplier: damageMultiplier,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "Rend",
				Tag:   "Rend",
			},
			NumberOfTicks: rend.ticks,
			TickLength:    tickLength,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, baseDamage, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHitNoHitCounter)
			if result.Landed() {
				spell.Dot(target).Apply(sim)
			} else {
				spell.IssueRefund(sim)
			}

			spell.DealOutcome(sim, result)
		},
	})

}
