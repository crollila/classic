package shaman

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"math"
	"time"
)

// Stoneclaw rank6 is present in the checked-in Classic tooltip cache: 480 HP,
// 140 Mana, 30 sec cooldown, 15 sec duration and8yd taunt radius. Its health
// is a real pet health bar; Earth’s Grasp scales it before combat begins.
type foreverStoneclaw struct {
	core.Pet
	previous map[*core.Unit]*core.Unit
	rank     foreverStoneclawRank
}

// Lower Stoneclaw ranks use the Forever client tooltips (health, Mana); the top rank keeps
// the Classic cache values above.
type foreverStoneclawRank struct {
	level   int32
	spellID int32
	health  float64
	mana    float64
}

var foreverStoneclawRanks = []foreverStoneclawRank{
	{8, 5730, 206, 15}, {18, 6390, 276, 30}, {28, 6391, 316, 55}, {38, 6392, 346, 75}, {48, 10427, 426, 105}, {58, 10428, 480, 140},
}

// foreverStoneclawRankAt is the highest Stoneclaw rank known at level; ok is false below level 8.
func foreverStoneclawRankAt(level int32) (rank foreverStoneclawRank, ok bool) {
	for _, r := range foreverStoneclawRanks {
		if r.level <= level {
			rank, ok = r, true
		}
	}
	return rank, ok
}

func newForeverStoneclaw(s *Shaman) *foreverStoneclaw {
	// Core applies a -180 Health first-20-Stamina offset even to zero-Stamina totems.
	rank, _ := foreverStoneclawRankAt(s.Level)
	p := &foreverStoneclaw{Pet: core.NewPet("Stoneclaw Totem", &s.Character, stats.Stats{stats.Health: 180 + rank.health*(1+.25*s.fr("earth-s-grasp"))}, func(stats.Stats) stats.Stats { return stats.Stats{} }, false, true), previous: map[*core.Unit]*core.Unit{}, rank: rank}
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
	core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Stoneclaw Death", OnSpellHitTaken: damage, OnPeriodicDamageTaken: damage}))
}
func (s *Shaman) registerForeverTotems() {
	if s.foreverStoneclaw != nil {
		pet := s.foreverStoneclaw
		s.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: pet.rank.spellID}, Flags: core.SpellFlagAPL | SpellFlagTotem, ManaCost: core.ManaCostOptions{FlatCost: pet.rank.mana, Multiplier: s.totemManaMultiplier()}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 30 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
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
	if s.fr("guardian-totems") > 0 && s.Level >= 30 {
		// One charge covers nearby party members. Explicitly classified harmful
		// single-target spells are consumed before damage or control is applied.
		// The damage fallback retains provisional behavior for older encounter
		// spells lacking targeting metadata; periodic AoE spells are excluded.
		position := 0.0
		aura := s.RegisterAura(core.Aura{Label: "Forever Grounding Totem", ActionID: core.ActionID{SpellID: 8177}, Duration: 45 * time.Second})
		for _, agent := range s.Party.Players {
			c := agent.GetCharacter()
			c.AddForeverSpellInterceptor(func(sim *core.Simulation, sp *core.Spell) bool {
				if !aura.IsActive() || math.Abs(c.DistanceFromTarget-position) > 20 {
					return false
				}
				aura.Deactivate(sim)
				return true
			})
			c.AddDynamicDamageTakenModifier(func(sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
				if aura.IsActive() && r.Damage > 0 && sp.DefenseType == core.DefenseTypeMagic && sp.AOEDot() == nil && math.Abs(c.DistanceFromTarget-position) <= 20 {
					r.Damage = 0
					aura.Deactivate(sim)
				}
			})
		}
		s.RegisterSpell(core.SpellConfig{ActionID: aura.ActionID, Flags: core.SpellFlagAPL | SpellFlagTotem, ManaCost: core.ManaCostOptions{BaseCost: .06, Multiplier: s.totemManaMultiplier()}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 15*time.Second - time.Duration(s.fr("guardian-totems"))*time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			position = s.DistanceFromTarget
			s.setActiveAirTotem(sim, sp, aura)
		}})
	}
}
