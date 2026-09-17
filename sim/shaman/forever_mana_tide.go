package shaman

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

// The observed Mana Tide tooltip supplies five health. A guardian has no raid
// stat inheritance; the180 offset compensates core's universal first20Stamina
// adjustment, exactly as for Stoneclaw. Incoming damage uses real combat results.
type foreverManaTide struct {
	core.Pet
	shaman     *Shaman
	spell      *core.Spell
	generation uint64
}

func newForeverManaTide(s *Shaman) *foreverManaTide {
	p := &foreverManaTide{Pet: core.NewPet("Mana Tide Totem", &s.Character, stats.Stats{stats.Health: 185}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, true), shaman: s}
	p.OnPetDisable = func(sim *core.Simulation) {
		p.generation++
		if p.spell != nil && s.ActiveTotems[WaterTotem] == p.spell {
			s.ActiveTotems[WaterTotem] = nil
			s.TotemExpirations[WaterTotem] = sim.CurrentTime
		}
	}
	return p
}
func (p *foreverManaTide) GetPet() *core.Pet          { return &p.Pet }
func (p *foreverManaTide) Reset(sim *core.Simulation) { p.Disable(sim) }
func (p *foreverManaTide) ExecuteCustomRotation(sim *core.Simulation) {
	p.WaitUntil(sim, sim.CurrentTime+time.Second)
}
func (p *foreverManaTide) Initialize() {
	damage := func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
		if !p.IsEnabled() {
			return
		}
		if r.Damage > 0 {
			p.RemoveHealth(sim, r.Damage)
		}
		if p.CurrentHealth() <= 0 {
			p.Metrics.Died = true
			p.Disable(sim)
		}
	}
	core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Mana Tide Health", OnSpellHitTaken: damage, OnPeriodicDamageTaken: damage}))
	core.MakePermanent(p.shaman.RegisterAura(core.Aura{Label: "Forever Mana Tide Replacement", OnCastComplete: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell) {
		if p.IsEnabled() && p.shaman.ActiveTotems[WaterTotem] != p.spell {
			p.Disable(sim)
		}
	}}))
}
