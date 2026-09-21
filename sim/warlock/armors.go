package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

// Demon Skin ranks 1-2 then Demon Armor ranks 1-5: spell id, learned level, armor,
// Shadow resistance and health restored every 5 seconds.
var demonArmorRanks = []struct {
	spellID                 int32
	level                   int32
	armor, shadowRes, regen float64
}{
	{687, 1, 40, 0, 3},
	{696, 10, 120, 0, 5},
	{706, 20, 210, 3, 7},
	{1086, 30, 300, 6, 9},
	{11733, 40, 390, 9, 11},
	{11734, 50, 480, 12, 13},
	{11735, 60, 570, 15, 15},
}

func (warlock *Warlock) applyDemonArmor() {
	// Demon Skin until Demon Armor is learned at 20; both share one aura.
	var spellID int32
	var armor, shadowRes, regen float64
	for _, r := range demonArmorRanks {
		if r.level <= warlock.Level {
			spellID, armor, shadowRes, regen = r.spellID, r.armor, r.shadowRes, r.regen
		}
	}
	if spellID == 0 {
		return
	}

	armor *= 1 + warlock.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100
	shadowRes *= 1 + warlock.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100
	warlock.AddStat(stats.Armor, armor)
	warlock.AddStat(stats.ShadowResistance, shadowRes)

	if warlock.Forever != nil {
		health := regen * (1 + warlock.ForeverValue("warlock.talent.demonic-aegis", 0, 0)/100)
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
