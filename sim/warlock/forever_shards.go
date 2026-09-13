package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"time"
)

// Best-guess preparation is an explicit bounded scenario input. STRICT keeps
// Classic's sufficient-shard preparation policy because inventory is unobserved.
func (w *Warlock) registerForeverSoulShards() {
	if w.Forever == nil || foreverdata.IsStrict(w.Forever) {
		return
	}
	action := w.ForeverAction("warlock.talent.shadowburn")
	shards := w.RegisterAura(core.Aura{Label: "Forever Soul Shards", ActionID: action, Duration: core.NeverExpires, MaxStacks: 100, OnReset: func(a *core.Aura, sim *core.Simulation) {
		a.Activate(sim)
		a.SetStacks(sim, int32(min(100, max(0, w.ForeverParameter("warlock.soul_shards", 20)))))
	}})
	add := func(sim *core.Simulation) { shards.Activate(sim); shards.AddStack(sim) }
	costs := map[*core.Spell]bool{}
	for _, s := range w.Spellbook {
		costs[s] = s.SpellCode == SpellCode_WarlockSoulFire || s.SpellCode == SpellCode_WarlockShadowburn || s.SpellID == 691 || s.SpellID == 697 || s.SpellID == 712 || s.ActionID.SameAction(w.ForeverAction("warlock.baseline.incubus"))
		if !costs[s] {
			continue
		}
		old := s.ExtraCastCondition
		s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
			free := s.SpellCode == SpellCode_WarlockSoulFire && w.foreverState.decimation.IsActive()
			return (free || shards.GetStacks() > 0) && (old == nil || old(sim, t))
		}
	}
	debuffs := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Shadowburn refund-" + w.Label, ActionID: action, Duration: 8 * time.Second, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if t.HasHealthBar() && t.CurrentHealth() <= 0 && r.Damage > 0 && t.Level >= w.Level-8 {
				a.Deactivate(sim)
				add(sim)
			}
		}})
	})
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Soul Shard Rules", OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
		if costs[s] && !(s.SpellCode == SpellCode_WarlockSoulFire && w.foreverState.decimation.IsActive()) {
			shards.RemoveStack(sim)
		}
	}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if s.SpellCode == SpellCode_WarlockShadowburn && r.Landed() {
			if r.Target.HasHealthBar() && r.Target.CurrentHealth() <= 0 && r.Target.Level >= w.Level-8 {
				add(sim)
			} else {
				debuffs.Get(r.Target).Activate(sim)
			}
			if w.ForeverRank("warlock.talent.shadow-and-flame") > 0 && sim.Proc(w.ForeverValue("warlock.talent.shadow-and-flame", 4, 0)/100, "Forever Shadowburn refund") {
				add(sim)
			}
		}
	}, OnPeriodicDamageDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if s.SpellCode == SpellCode_WarlockDrainSoul && r.Target.HasHealthBar() && r.Target.CurrentHealth() <= 0 && r.Damage > 0 {
			add(sim)
		}
	}}))
}
