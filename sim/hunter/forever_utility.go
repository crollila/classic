package hunter

import (
	"fmt"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

// Pathfinding modifies the actual Cheetah/Pack recipients. The Classic Pack
// baseline supplies the 30-yard party radius and four-second damage daze.
// Source: assets/db_inputs/wowhead_spell_tooltips.csv, spells 5118 and 13159.
// Party separation uses the engine's one-dimensional distance convention;
// the 250-ms range refresh and 50% daze slow remain provisional analogues.
func (h *Hunter) foreverMovementAspect(aspect *core.Aura, id core.ActionID, pack bool) {
	units := []*core.Unit{&h.Unit}
	if pack {
		units = nil
		for _, player := range h.Party.Players {
			c := player.GetCharacter()
			units = append(units, &c.Unit)
			for _, pet := range c.Pets {
				units = append(units, &pet.Unit)
			}
		}
	}
	benefits := make([]*core.Aura, 0, len(units))
	for _, unit := range units {
		key := id // Separate modifier identity for each source/recipient pair.
		daze := unit.ForeverSnareAura("Aspect Daze", id, 4*time.Second, .5)
		onDamage := func(_ *core.Aura, sim *core.Simulation, _ *core.Spell, result *core.SpellResult) {
			if result.Damage > 0 {
				daze.Activate(sim)
			}
		}
		benefits = append(benefits, unit.RegisterAura(core.Aura{
			Label: fmt.Sprintf("%s benefit-%s", aspect.Label, h.Label), ActionID: id, Duration: core.NeverExpires,
			OnGain: func(a *core.Aura, sim *core.Simulation) {
				unit.AddMoveSpeedModifier(&key, 1.3+h.clientTalent("hunter.talent.pathfinding", 0, 3*float64(h.ForeverRank("hunter.talent.pathfinding")))/100)
			},
			OnExpire:        func(a *core.Aura, sim *core.Simulation) { unit.RemoveMoveSpeedModifier(&key) },
			OnSpellHitTaken: onDamage, OnPeriodicDamageTaken: onDamage,
		}))
	}
	refresh := func(sim *core.Simulation) {
		for _, benefit := range benefits {
			distance := benefit.Unit.DistanceFromTarget - h.DistanceFromTarget
			if benefit.Unit.IsEnabled() && distance >= -30 && distance <= 30 {
				benefit.Activate(sim)
			} else {
				benefit.Deactivate(sim)
			}
		}
	}
	generation := 0
	aspect.OnGain = func(a *core.Aura, sim *core.Simulation) {
		generation++
		current := generation
		refresh(sim)
		if pack {
			var tick func(*core.Simulation)
			tick = func(sim *core.Simulation) {
				if !aspect.IsActive() || current != generation {
					return
				}
				refresh(sim)
				core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: sim.CurrentTime + 250*time.Millisecond, OnAction: tick})
			}
			core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: sim.CurrentTime + 250*time.Millisecond, OnAction: tick})
		}
	}
	aspect.OnExpire = func(a *core.Aura, sim *core.Simulation) {
		generation++
		for _, benefit := range benefits {
			benefit.Deactivate(sim)
		}
	}
}

// Baselines are retained from the Classic spell tables. The only added cooldown
// assumption is Viper's TBC 15s cooldown, explicitly excluded from STRICT.
// https://www.wowhead.com/classic/spell=14277/scorpid-sting
// https://www.wowhead.com/tbc/spell=14280/viper-sting
func (h *Hunter) registerForeverStings() {
	category := "Forever Hunter Sting-" + h.Label
	// Improved Stings effect 2: Scorpid Sting duration change (ms); effect 1: Viper Sting cooldown
	// change (ms).
	stings := float64(h.ForeverRank("hunter.talent.improved-stings"))
	duration := 20*time.Second + time.Duration(h.clientTalent("hunter.talent.improved-stings", 2, 15000*stings))*time.Millisecond
	viperCooldown := 15*time.Second + time.Duration(h.clientTalent("hunter.talent.improved-stings", 1, -2000*stings))*time.Millisecond
	hawkEye := h.clientTalent("hunter.talent.hawk-eye", 0, 2*float64(h.ForeverRank("hunter.talent.hawk-eye")))
	debuffs := h.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		delta := stats.Stats{stats.Strength: -68, stats.Agility: -68}
		// BEST_GUESS uses the Classic warrior NPC analogue for derived offensive
		// stats; raw tooltip stat reductions are always retained.
		if !foreverdata.IsStrict(h.Forever) {
			delta[stats.AttackPower] = -136
			delta[stats.Armor] = -136
		}
		a := t.GetOrRegisterAura(core.Aura{Label: "Scorpid Sting-" + h.Label, ActionID: core.ActionID{SpellID: 14277}, Tag: "forever-debuff-poison", Duration: duration}).AttachStatsBuff(delta)
		a.NewExclusiveEffect(category, true, core.ExclusiveEffect{})
		if h.SerpentSting != nil {
			h.SerpentSting.Dot(t).NewExclusiveEffect(category, true, core.ExclusiveEffect{})
		}
		return a
	})
	castRange := func(sim *core.Simulation, t *core.Unit) bool {
		return h.DistanceFromTarget >= 8 && h.DistanceFromTarget <= 35+hawkEye
	}
	h.ScorpidSting = h.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 14277}, Flags: core.SpellFlagAPL | core.SpellFlagPoison | SpellFlagSting, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeRanged, ProcMask: core.ProcMaskRangedSpecial, ManaCost: core.ManaCostOptions{FlatCost: 165}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true}, ExtraCastCondition: castRange,
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r := s.CalcOutcome(sim, t, s.OutcomeRangedHitNoHitCounter)
			s.DealOutcome(sim, r)
			if r.Landed() {
				h.replaceForeverSting(sim, t, debuffs.Get(t))
				debuffs.Get(t).Activate(sim)
			}
		},
	})
	if foreverdata.IsStrict(h.Forever) || h.Level < 36 {
		return // Viper Sting is learned at level 36
	}
	manaMetrics := map[*core.Unit]*core.ResourceMetrics{}
	for _, t := range h.Env.Encounter.TargetUnits {
		pool := h.ForeverParameter("scenario.enemy_mana", 0)
		if pool <= 0 {
			pool = t.GetStat(stats.Mana)
		}
		t.ForeverEnableManaPool(pool)
		manaMetrics[t] = t.NewManaMetrics(core.ActionID{SpellID: 14280})
	}
	viper := h.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 14280}, Flags: core.SpellFlagAPL | core.SpellFlagPoison | SpellFlagSting, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeRanged, ProcMask: core.ProcMaskRangedSpecial, ManaCost: core.ManaCostOptions{FlatCost: 215}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: h.NewTimer(), Duration: viperCooldown}},
		ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return castRange(sim, t) && t.HasManaBar() && t.CurrentMana() > 0
		},
		Dot: core.DotConfig{Aura: core.Aura{Label: "Viper Sting", Tag: "forever-debuff-poison"}, NumberOfTicks: 4, TickLength: 2 * time.Second, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
			t.SpendMana(sim, min(277, t.CurrentMana()), manaMetrics[t])
		}},
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r := s.CalcOutcome(sim, t, s.OutcomeRangedHitNoHitCounter)
			s.DealOutcome(sim, r)
			if r.Landed() {
				h.replaceForeverSting(sim, t, s.Dot(t).Aura)
				s.Dot(t).Apply(sim)
			}
		},
	})
	for _, t := range h.Env.Encounter.TargetUnits {
		viper.Dot(t).NewExclusiveEffect(category, true, core.ExclusiveEffect{})
	}
}

// Feign Death's Classic80mana/30s baseline and spell hit table are provisionally
// retained. Threat reset is recorded as negative threat, so later attacks create
// new threat rather than restoring the cleared amount.
// https://www.wowhead.com/classic/spell=5384/feign-death
func (h *Hunter) registerForeverFeignDeath() {
	id := core.ActionID{SpellID: 5384}
	paused := map[*core.Unit]bool{}
	a := h.RegisterAura(core.Aura{Label: "Feign Death", ActionID: id, Duration: 6 * time.Minute,
		OnGain: func(a *core.Aura, sim *core.Simulation) { h.AutoAttacks.CancelAutoSwing(sim) },
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			if s.ActionID != id {
				a.Deactivate(sim)
			}
		},
		OnExpire: func(a *core.Aura, sim *core.Simulation) {
			if !h.IsMoving() {
				h.AutoAttacks.EnableAutoSwing(sim)
			}
			for t := range paused {
				t.AutoAttacks.EnableAutoSwing(sim)
				delete(paused, t)
			}
		},
	})
	h.RegisterResetEffect(func(sim *core.Simulation) { clear(paused) })
	h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskEmpty, BonusHitRating: h.clientTalent("hunter.talent.survival-tactics", 0, 5*float64(h.ForeverRank("hunter.talent.survival-tactics"))), ManaCost: core.ManaCostOptions{FlatCost: 80}, Cast: core.CastConfig{CD: core.Cooldown{Timer: h.NewTimer(), Duration: 30 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return !h.IsMoving() },
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			a.Activate(sim)
			for _, enemy := range h.Env.Encounter.TargetUnits {
				result := s.CalcOutcome(sim, enemy, s.OutcomeMagicHit)
				s.DealOutcome(sim, result)
				if !result.Landed() {
					continue
				}
				threat := 0.
				for _, spell := range h.Spellbook {
					threat += spell.SpellMetrics[enemy.UnitIndex].TotalThreat
				}
				s.SpellMetrics[enemy.UnitIndex].TotalThreat -= max(0, threat)
				if enemy.CurrentTarget == &h.Unit {
					enemy.AutoAttacks.CancelAutoSwing(sim)
					paused[enemy] = true
				}
			}
		},
	})
}

// Freezing/Frost traps use the same trigger placement convention as the existing
// WoWSims traps; terrain/arming timing remains provisional. Exact duration and
// slow numbers retain Classic baselines plus the observed Forever talent values.
// https://www.wowhead.com/classic/spell=13809/frost-trap
func (h *Hunter) registerForeverFrostTrap(timer *core.Timer) {
	// Clever Traps effect 0: Freezing and Frost trap effect duration change (%).
	cleverTraps := h.clientTalent("hunter.talent.clever-traps", 0, 15*float64(h.ForeverRank("hunter.talent.clever-traps"))) / 100
	h.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 13809}, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: core.SpellFlagAPL | SpellFlagTrap, ManaCost: core.ManaCostOptions{FlatCost: 60}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: timer, Duration: clientCooldown(&h.Character, 13809, 15*time.Second)}},
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			for _, enemy := range h.Env.Encounter.TargetUnits {
				if enemy.DistanceFromTarget > 10 {
					continue
				}
				result := s.CalcOutcome(sim, enemy, s.OutcomeMagicHit)
				s.DealOutcome(sim, result)
				if result.Landed() {
					enemy.ForeverSnareAura("Frost Trap", s.ActionID, time.Duration(float64(30*time.Second)*(1+cleverTraps)), .6).Activate(sim)
				}
			}
		},
	})
}

// https://classicdb.ch/?spell=13544 ; each tick gets its own talent cleanse roll.
func (h *Hunter) registerForeverMendPet() {
	id := core.ActionID{SpellID: 13544}
	metrics := h.pet.NewHealthMetrics(id)
	// Improved Mend Pet: effect 0 cleanse chance per heal (15/50%), effect 1 mana cost change (10/20%).
	mendRank := float64(h.ForeverRank("hunter.talent.improved-mend-pet"))
	cleanse := h.clientTalent("hunter.talent.improved-mend-pet", 0, []float64{0, 15, 50}[int(mendRank)]) / 100
	mendCost := int32(h.clientTalent("hunter.talent.improved-mend-pet", 1, 10*mendRank))
	h.RegisterSpell(core.SpellConfig{ActionID: id, SpellSchool: core.SpellSchoolNature, Flags: core.SpellFlagAPL | core.SpellFlagHelpful | core.SpellFlagChanneled, ManaCost: core.ManaCostOptions{FlatCost: 480, Multiplier: 100 - mendCost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return h.pet.IsEnabled() },
		Dot: core.DotConfig{SelfOnly: true, Aura: core.Aura{Label: "Mend Pet", OnExpire: func(a *core.Aura, sim *core.Simulation) {
			if !h.IsMoving() {
				h.AutoAttacks.EnableAutoSwing(sim)
			}
		}}, NumberOfTicks: 5, TickLength: time.Second,
			OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				if !h.pet.IsEnabled() {
					d.Cancel(sim)
					return
				}
				h.pet.GainHealth(sim, 245, metrics)
				if sim.Proc(cleanse, "Improved Mend Pet") {
					h.pet.ForeverDispelOne(sim, "curse", "disease", "magic", "poison")
				}
			},
		},
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			h.AutoAttacks.CancelAutoSwing(sim)
			s.AOEDot().Apply(sim)
		},
	})
}

func (h *Hunter) replaceForeverSting(sim *core.Simulation, t *core.Unit, incoming *core.Aura) {
	if active := t.GetExclusiveEffectCategory("Forever Hunter Sting-" + h.Label).GetActiveAura(); active != nil && active != incoming {
		active.Deactivate(sim)
	}
}
