package shaman

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

// Stoneclaw rank6 is present in the checked-in Classic tooltip cache: 480 HP,
// 140 Mana, 30 sec cooldown, 15 sec duration and8yd taunt radius. Its health
// is a real pet health bar; Earth’s Grasp scales it before combat begins.
type foreverStoneclaw struct {
	core.Pet
	previous map[*core.Unit]*core.Unit
}

func newForeverStoneclaw(s *Shaman) *foreverStoneclaw {
	// Core applies a -180 Health first-20-Stamina offset even to zero-Stamina totems.
	p := &foreverStoneclaw{Pet: core.NewPet("Stoneclaw Totem", &s.Character, stats.Stats{stats.Health: 180 + 480*(1+.25*s.fr("earth-s-grasp"))}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, true), previous: map[*core.Unit]*core.Unit{}}
	p.OnPetDisable = func(sim *core.Simulation) {
		for t, old := range p.previous {
			if t.CurrentTarget == &p.Unit {
				t.CurrentTarget = old
			}
		}
		p.previous = map[*core.Unit]*core.Unit{}
	}
	return p
}
func (p *foreverStoneclaw) GetPet() *core.Pet          { return &p.Pet }
func (p *foreverStoneclaw) Reset(sim *core.Simulation) { p.Disable(sim) }
func (p *foreverStoneclaw) ExecuteCustomRotation(sim *core.Simulation) {
	p.WaitUntil(sim, sim.CurrentTime+time.Second)
}
func (p *foreverStoneclaw) Initialize() {
	core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Stoneclaw Death", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
		if p.CurrentHealth() <= 0 {
			p.Disable(sim)
		}
	}}))
}
func (s *Shaman) registerForeverTotems() {
	if s.foreverStoneclaw != nil {
		pet := s.foreverStoneclaw
		s.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 10428}, Flags: core.SpellFlagAPL | SpellFlagTotem, ManaCost: core.ManaCostOptions{FlatCost: 140, Multiplier: s.totemManaMultiplier()}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 30 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			pet.Disable(sim)
			pet.EnableWithTimeout(sim, pet, 15*time.Second)
			pet.GainHealth(sim, pet.MaxHealth(), pet.NewHealthMetrics(sp.ActionID))
			s.ActiveTotems[EarthTotem] = sp
			s.TotemExpirations[EarthTotem] = sim.CurrentTime + 15*time.Second
			for _, target := range s.Env.Encounter.Targets {
				if target.Level < 63 && s.DistanceFromTarget <= 8 {
					pet.previous[&target.Unit] = target.CurrentTarget
					target.CurrentTarget = &pet.Unit
				}
			}
		}})
	}
	if s.fr("guardian-totems") > 0 {
		shield := core.NewForeverAbsorb(&s.Unit, "Forever Grounding Totem", core.ActionID{SpellID: 8177}, 45*time.Second, core.SpellSchoolArcane|core.SpellSchoolFire|core.SpellSchoolFrost|core.SpellSchoolHoly|core.SpellSchoolNature|core.SpellSchoolShadow)
		shield.Aura.OnSpellHitTaken = func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if sp.DefenseType == core.DefenseTypeMagic && r.Landed() {
				a.Deactivate(sim)
			}
		}
		s.RegisterSpell(core.SpellConfig{ActionID: shield.Aura.ActionID, Flags: core.SpellFlagAPL | SpellFlagTotem, ManaCost: core.ManaCostOptions{BaseCost: .06, Multiplier: s.totemManaMultiplier()}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 15*time.Second - time.Duration(s.fr("guardian-totems"))*time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			s.setActiveAirTotem(sim, sp, shield.Aura)
			shield.Apply(sim, 1e15, false)
		}})
	}
}
