package paladin

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"time"
)

// Classic client tooltip 20164 supplies 13% base Mana, a 30-second seal and
// 2-second stun. The unobserved 2-PPM proc rate is explicitly PREDICTED, exposed
// as an option and disabled in STRICT. This also supplies Twist of Light's
// documented Justice payload instead of silently omitting that seal.
func (p *Paladin) registerForeverJustice() {
	if foreverdata.IsStrict(p.Forever) || p.fr("twist-of-light") == 0 {
		return
	}
	action := core.ActionID{SpellID: 20164}
	stuns := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Seal of Justice Stun-"+p.Label, action, core.ForeverStun, 2*time.Second)
	})
	seal := p.RegisterAura(core.Aura{Label: "Forever Seal of Justice", ActionID: action, Duration: 30 * time.Second, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
		if sp.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && r.Landed() && sim.Proc(p.ForeverParameter("seal_of_justice_ppm", 2)*p.AutoAttacks.MH().SwingSpeed/60, "Forever Seal of Justice") {
			stuns.Get(r.Target).Activate(sim)
		}
	}})
	// Judgement's prevention of NPC fleeing is retained as a named target aura.
	// Encounter AI currently never flees, so this world behavior has no combat delta.
	judgements := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.RegisterAura(core.Aura{Label: "Judgement of Justice-" + p.Label, ActionID: core.ActionID{SpellID: 20184}, Duration: 10 * time.Second, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if sp.Unit == &p.Unit && sp.ProcMask.Matches(core.ProcMaskMelee) && r.Landed() {
				a.Refresh(sim)
			}
		}})
	})
	judge := p.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 20184}, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
		if sp.CalcOutcome(sim, t, sp.OutcomeMagicHit).Landed() {
			judgements.Get(t).Activate(sim)
		}
	}})
	p.RegisterSpell(core.SpellConfig{ActionID: action, SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{BaseCost: .13}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { p.applySeal(seal, judge, sim) }})
}
