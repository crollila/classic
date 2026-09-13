package mage

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"slices"
	"time"
)

type foreverMageState struct {
	blast, missiles, hotStreak, fingers *core.Aura
	frozen, scorch, winter              core.AuraArray
}

// Numerical effects use the discovery record; interactions retain Classic
// spell registration, snapshots and additive talent stacking (PROVISIONAL).
func (m *Mage) applyForeverCasterTalents() {
	if m.Forever == nil {
		return
	}
	f := &foreverMageState{}
	m.foreverState = f
	rank := func(id string) float64 { return float64(m.ForeverRank("mage.talent." + id)) }
	value := func(id string, index int, fallback float64) float64 {
		return m.ForeverValue("mage.talent."+id, index, fallback)
	}
	m.AddStat(stats.SpellPenetration, value("arcane-subtlety", 0, 0))
	if rank("arcane-instability") > 0 {
		m.AddStat(stats.MeleeCrit, rank("arcane-instability")*core.CritRatingPerCritChance)
	}
	if rank("magic-absorption") > 0 {
		m.AddResistances(value("magic-absorption", 0, 0))
		timer := m.NewTimer()
		mana := m.NewManaMetrics(m.ForeverAction("mage.talent.magic-absorption"))
		core.MakePermanent(m.RegisterAura(core.Aura{Label: "Forever Magic Absorption", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.DefenseType == core.DefenseTypeMagic && r.Outcome.Matches(core.OutcomeMiss) && timer.IsReady(sim) {
				m.AddMana(sim, m.MaxMana()*value("magic-absorption", 1, 0)/100, mana)
				timer.Set(sim.CurrentTime + time.Second)
			}
		}}))
	}
	f.frozen = m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Frozen-"+m.Label, m.ForeverAction("mage.talent.frostbite"), core.ForeverRoot, 5*time.Second)
	})
	f.scorch = m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Scorch-" + m.Label, ActionID: m.ForeverAction("mage.talent.improved-scorch"), Duration: 30 * time.Second, MaxStacks: 5})
	})
	f.winter = m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Winter's Chill-" + m.Label, ActionID: m.ForeverAction("mage.talent.winter-s-chill"), Duration: 15 * time.Second, MaxStacks: max(1, int32(value("winter-s-chill", 3, 1)))})
	})
	m.ForeverDamageMultiplier(func(s *core.Spell, at *core.AttackTable) float64 {
		if s.SpellSchool.Matches(core.SpellSchoolFire) {
			return 1 + .03*float64(f.scorch.Get(at.Defender).GetStacks())
		}
		return 1
	})
	f.hotStreak = m.RegisterAura(core.Aura{Label: "Forever Hot Streak", ActionID: m.ForeverAction("mage.talent.hot-streak"), Duration: 15 * time.Second, MaxStacks: 3,
		OnStacksChange: func(a *core.Aura, sim *core.Simulation, old, new int32) {
			for _, s := range m.Pyroblast {
				if s != nil {
					s.CastTimeMultiplier -= .25 * float64(new-old)
				}
			}
		},
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			for _, p := range m.Pyroblast {
				if p == s {
					a.Deactivate(sim)
					break
				}
			}
		},
	})
	f.fingers = m.RegisterAura(core.Aura{Label: "Forever Fingers of Frost", ActionID: m.ForeverAction("mage.talent.fingers-of-frost"), Duration: 15 * time.Second, MaxStacks: max(1, int32(value("fingers-of-frost", 1, 1))),
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			if s.Flags.Matches(core.SpellFlagAPL) && s.Flags.Matches(SpellFlagMage) && s.DefenseType == core.DefenseTypeMagic && a.RemainingDuration(sim) != a.Duration {
				a.RemoveStack(sim)
			}
		},
	})
	f.blast = m.RegisterAura(core.Aura{Label: "Forever Arcane Blast", ActionID: m.ForeverAction("mage.talent.arcane-blast"), Duration: 8 * time.Second, MaxStacks: 4,
		OnStacksChange: func(a *core.Aura, sim *core.Simulation, old, new int32) {
			for _, s := range m.Spellbook {
				if !s.Flags.Matches(SpellFlagMage) {
					continue
				}
				if s.SpellCode == SpellCode_MageArcaneBlast {
					s.Cost.Multiplier += 175 * (new - old)
				} else {
					s.DamageMultiplierAdditive += .10 * float64(new-old)
				}
			}
		},
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			if s.Flags.Matches(SpellFlagMage) && s.SpellCode != SpellCode_MageArcaneBlast && s.DefenseType == core.DefenseTypeMagic && !s.Flags.Matches(core.SpellFlagChanneled) && s.SpellCode != SpellCode_MageArcaneMissilesTick {
				a.Deactivate(sim)
			}
		},
	})
	f.missiles = m.RegisterAura(core.Aura{Label: "Forever Missile Barrage", ActionID: m.ForeverAction("mage.talent.missile-barrage"), Duration: 15 * time.Second,
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range m.ArcaneMissiles {
				if s != nil {
					s.Cost.Multiplier -= 100
					for _, d := range s.Dots() {
						if d != nil {
							d.TickLength /= 2
						}
					}
				}
			}
		},
		OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range m.ArcaneMissiles {
				if s != nil {
					s.Cost.Multiplier += 100
					for _, d := range s.Dots() {
						if d != nil {
							d.TickLength *= 2
						}
					}
				}
			}
		},
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			if s.SpellCode == SpellCode_MageArcaneMissiles {
				a.Deactivate(sim)
			}
		},
	})
	m.OnSpellRegistered(func(s *core.Spell) {
		if s.ProcMask.Matches(core.ProcMaskRanged) && s.OtherID == proto.OtherAction_OtherActionShoot {
			s.DamageMultiplier *= 1 + value("wand-specialization", 0, 0)/100
		}
		if !s.Flags.Matches(SpellFlagMage) {
			return
		}
		if s.SpellSchool.Matches(core.SpellSchoolArcane) {
			s.ThreatMultiplier *= 1 - value("arcane-subtlety", 1, 0)/100
		}
		if s.SpellSchool.Matches(core.SpellSchoolFire) {
			s.ThreatMultiplier *= 1 - value("burning-soul", 1, 0)/100
			s.PushbackReduction += value("burning-soul", 0, 0) / 100
		}
		if s.SpellCode == SpellCode_MageArcaneMissiles {
			s.PushbackReduction += value("improved-channeling", 0, 0) / 100
			for _, d := range s.Dots() {
				if d == nil {
					continue
				}
				oldExpire := d.OnExpire
				d.OnExpire = func(a *core.Aura, sim *core.Simulation) {
					if oldExpire != nil {
						oldExpire(a, sim)
					}
					f.blast.Deactivate(sim)
				}
			}
		}
		if s.SpellCode == SpellCode_MageArcaneBlast {
			s.PushbackReduction += value("improved-channeling", 1, 0) / 100
		}
		if s.SpellCode == SpellCode_MageFireBlast {
			s.CD.Duration -= time.Duration(value("wake-of-fire", 0, 0) * float64(time.Second))
		}
		if s.SpellCode == SpellCode_MageFireBlast || s.SpellCode == SpellCode_MageScorch || s.SpellCode == SpellCode_MageIceLance || s.SpellCode == SpellCode_MageArcaneBlast {
			s.BonusCritRating += value("incineration", 0, 0) * core.SpellCritRatingPerCritChance
		}
		yards := 30.0
		if s.SpellCode == SpellCode_MageFireball || s.SpellCode == SpellCode_MageFrostfireBolt {
			yards = 35
		}
		if s.SpellCode == SpellCode_MageFireBlast {
			yards = 20
		}
		if s.SpellCode == SpellCode_MageArcaneExplosion {
			yards = 10
		}
		if s.SpellSchool.Matches(core.SpellSchoolArcane) {
			yards += value("arcane-geometry", 0, 0)
		}
		if s.SpellSchool.Matches(core.SpellSchoolFire) {
			yards += value("flame-throwing", 0, 0)
		}
		if s.SpellCode == SpellCode_MageFrostbolt || s.SpellCode == SpellCode_MageFrostfireBolt || slices.Contains(BlizzardSpellId[1:], s.SpellID) {
			yards *= 1 + value("arctic-reach", 0, 0)/100
		}
		m.ForeverSpellRange(s, yards)
		old := s.ApplyEffects
		if old != nil {
			s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				if !m.IsOpponent(t) {
					old(sim, t, sp)
					return
				}
				frozen := f.frozen.Get(t).IsActive() || f.fingers.IsActive()
				crit := 0.0
				if frozen {
					crit += value("shatter", 0, 0) * core.SpellCritRatingPerCritChance
				}
				if sp.SpellCode == SpellCode_MageIceLance || sp.SpellCode == SpellCode_MageFrostbolt {
					crit += 2 * float64(f.winter.Get(t).GetStacks()) * core.SpellCritRatingPerCritChance
				}
				sp.BonusCritRating += crit
				old(sim, t, sp)
				sp.BonusCritRating -= crit
			}
		}
	})
	core.MakePermanent(m.RegisterAura(core.Aura{Label: "Forever Mage Triggers", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.Landed() || !s.Flags.Matches(SpellFlagMage) {
			return
		}
		if s.SpellCode == SpellCode_MageScorch && rank("improved-scorch") > 0 && sim.Proc(min(1, value("improved-scorch", 0, 0)/100), "Forever Scorch") {
			a := f.scorch.Get(r.Target)
			a.Activate(sim)
			a.AddStack(sim)
		}
		if s.SpellSchool.Matches(core.SpellSchoolFrost) && rank("winter-s-chill") > 0 && sim.Proc(min(1, value("winter-s-chill", 0, 0)/100), "Forever Winter's Chill") {
			a := f.winter.Get(r.Target)
			a.Activate(sim)
			a.AddStack(sim)
		}
		chill := s.Flags.Matches(SpellFlagChillSpell) || s.SpellCode == SpellCode_MageFrostbolt || s.SpellCode == SpellCode_MageFrostfireBolt
		if chill {
			if rank("frostbite") > 0 && sim.Proc(value("frostbite", 0, 0)/100, "Forever Frostbite") {
				f.frozen.Get(r.Target).Activate(sim)
			}
			if rank("fingers-of-frost") > 0 && sim.Proc(value("fingers-of-frost", 0, 0)/100, "Forever Fingers of Frost") {
				f.fingers.Activate(sim)
				f.fingers.SetStacks(sim, max(1, int32(value("fingers-of-frost", 1, 1))))
			}
			slow := r.Target.ForeverSnareAura("Forever Chill-"+m.Label, m.ForeverAction("mage.talent.permafrost"), time.Duration(8*float64(time.Second)*(1+value("permafrost", 0, 0)/100)), .4+value("permafrost", 1, 0)/100)
			slow.Activate(sim)
		}
		if s.SpellSchool.Matches(core.SpellSchoolFire) && rank("impact") > 0 && sim.Proc(value("impact", 0, 0)/100, "Forever Impact") {
			r.Target.ForeverControlAura("Forever Impact-"+m.Label, m.ForeverAction("mage.talent.impact"), core.ForeverStun, 2*time.Second).Activate(sim)
		}
		eligible := s.SpellCode == SpellCode_MageFireball || s.SpellCode == SpellCode_MageFireBlast || s.SpellCode == SpellCode_MageScorch || s.SpellCode == SpellCode_MageFrostfireBolt
		if eligible && r.DidCrit() && rank("hot-streak") > 0 {
			f.hotStreak.Activate(sim)
			f.hotStreak.AddStack(sim)
		}
		chance := 0.0
		if s.SpellCode == SpellCode_MageArcaneBlast {
			chance = .4
		} else if s.SpellCode == SpellCode_MageFireball || s.SpellCode == SpellCode_MageFrostbolt || s.SpellCode == SpellCode_MageFrostfireBolt {
			chance = .2
		}
		if chance > 0 && rank("missile-barrage") > 0 && sim.Proc(chance, "Forever Missile Barrage") {
			f.missiles.Activate(sim)
		}
	}}))
	// Pre-register control auras because adding auras while a simulation runs would
	// violate the engine's initialization contract.
	m.Env.RegisterPreFinalizeEffect(func() {
		for _, t := range m.Env.Encounter.Targets {
			t.ForeverSnareAura("Forever Chill-"+m.Label, m.ForeverAction("mage.talent.permafrost"), time.Duration(8*float64(time.Second)*(1+value("permafrost", 0, 0)/100)), .4+value("permafrost", 1, 0)/100)
			t.ForeverControlAura("Forever Impact-"+m.Label, m.ForeverAction("mage.talent.impact"), core.ForeverStun, 2*time.Second)
		}
	})
}

func (m *Mage) registerForeverSpells() {
	if m.Forever == nil {
		return
	}
	f := m.foreverState
	if m.ForeverRank("mage.talent.arcane-blast") > 0 {
		m.RegisterSpell(core.SpellConfig{ActionID: m.ForeverAction("mage.talent.arcane-blast"), SpellCode: SpellCode_MageArcaneBlast, SpellSchool: core.SpellSchoolArcane, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: 122}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 2500 * time.Millisecond}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 2.5 / 3.5,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				s.CalcAndDealDamage(sim, t, sim.Roll(95, 104), s.OutcomeMagicHitAndCrit)
				f.blast.Activate(sim)
				f.blast.AddStack(sim)
			},
		})
	}
	if m.ForeverRank("mage.talent.ice-lance") > 0 {
		m.RegisterSpell(core.SpellConfig{ActionID: m.ForeverAction("mage.talent.ice-lance"), SpellCode: SpellCode_MageIceLance, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: 45}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 1.5 / 3.5 / 3,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				mult := 1.0
				if f.frozen.Get(t).IsActive() || f.fingers.IsActive() {
					mult = 4
				}
				s.DamageMultiplier *= mult
				s.CalcAndDealDamage(sim, t, sim.Roll(28, 33), s.OutcomeMagicHitAndCrit)
				s.DamageMultiplier /= mult
			},
		})
	}
	// PREDICTED baseline: Frostbolt's highest existing rank is used as the damage,
	// mana and coefficient analogue; Fire/Frost dual school and Fireball talents
	// are established by the referencing Forever talents.
	if m.HasForeverMechanic("mage.baseline.frostfire-bolt") {
		m.RegisterSpell(core.SpellConfig{ActionID: m.ForeverAction("mage.baseline.frostfire-bolt"), SpellCode: SpellCode_MageFrostfireBolt, SpellSchool: core.SpellSchoolFire | core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | SpellFlagChillSpell | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: 290}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 3*time.Second - 100*time.Millisecond*time.Duration(m.ForeverRank("mage.talent.improved-fireball"))}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: .814,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				s.CalcAndDealDamage(sim, t, sim.Roll(429, 463), s.OutcomeMagicHitAndCrit)
			},
		})
	}
	m.RegisterForeverWand()
	m.registerForeverDefenses()
}
