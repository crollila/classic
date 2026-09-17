package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (warlock *Warlock) applyDemonArmor() {
	spellID := map[int32]int32{
		25: 706,
		40: 11733,
		50: 11734,
		60: 11735,
	}[warlock.Level]

	armor := map[int32]float64{
		25: 210.0,
		40: 390.0,
		50: 480.0,
		60: 570.0,
	}[warlock.Level]

	shadowRes := map[int32]float64{
		25: 3.0,
		40: 9.0,
		50: 12.0,
		60: 15.0,
	}[warlock.Level]

	armor *= 1 + warlock.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100
	shadowRes *= 1 + warlock.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100
	warlock.AddStat(stats.Armor, armor)
	warlock.AddStat(stats.ShadowResistance, shadowRes)

	if warlock.Forever != nil {
		health := map[int32]float64{25: 7, 40: 11, 50: 13, 60: 15}[warlock.Level] * (1 + warlock.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100)
		metrics := warlock.NewHealthMetrics(core.ActionID{SpellID: spellID})
		core.MakePermanent(warlock.RegisterAura(core.Aura{Label: "Forever Demon Armor regeneration", OnReset: func(a *core.Aura, sim *core.Simulation) {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 5 * time.Second, OnAction: func(sim *core.Simulation) { warlock.GainHealth(sim, health, metrics) }})
		}}))
	}
	warlock.GetOrRegisterAura(core.Aura{
		Label:    "Demon Armor",
		ActionID: core.ActionID{SpellID: spellID},
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
	})
}
