package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (w *Warlock) applyForeverPetTalents() {
	val := func(id string, index int) float64 { return w.ForeverValue("warlock.talent."+id, index, 0) }
	f := w.foreverState
	if w.ForeverRank("warlock.talent.master-demonologist") > 0 {
		for _, pet := range w.BasePets {
			pet := pet
			modify := func(u *core.Unit, mult float64) {
				p := val("master-demonologist", 0) / 100
				switch pet {
				case w.Imp:
					u.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] *= 1 + mult*p
				case w.Succubus:
					u.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1 + mult*p
				case w.Voidwalker:
					u.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical] *= 1 - mult*p
				case w.Felhunter:
					u.PseudoStats.SchoolDamageTakenMultiplier.MultiplyMagicSchools(1 - mult*p)
				}
			}
			// Reverse with reciprocals to avoid changing another source's multiplier.
			reverse := func(u *core.Unit) {
				p := val("master-demonologist", 0) / 100
				switch pet {
				case w.Imp:
					u.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] /= 1 + p
				case w.Succubus:
					u.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= 1 + p
				case w.Voidwalker:
					u.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical] /= 1 - p
				case w.Felhunter:
					u.PseudoStats.SchoolDamageTakenMultiplier.MultiplyMagicSchools(1 / (1 - p))
				}
			}
			aura := w.RegisterAura(core.Aura{Label: "Forever Master Demonologist " + pet.Label, ActionID: w.ForeverAction("warlock.talent.master-demonologist"), Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) { modify(&w.Unit, 1); modify(&pet.Unit, 1) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { reverse(&w.Unit); reverse(&pet.Unit) }})
			pet.ApplyOnPetEnable(func(sim *core.Simulation) { aura.Activate(sim) })
			pet.ApplyOnPetDisable(func(sim *core.Simulation, sac bool) { aura.Deactivate(sim) })
		}
	}
	if w.ForeverRank("warlock.talent.soul-link") > 0 {
		aura := w.RegisterAura(core.Aura{Label: "Forever Soul Link", ActionID: w.ForeverAction("warlock.talent.soul-link"), Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) { w.PseudoStats.DamageDealtMultiplier *= 1.03 }, OnExpire: func(a *core.Aura, sim *core.Simulation) { w.PseudoStats.DamageDealtMultiplier /= 1.03 }})
		for _, p := range w.BasePets {
			p := p
			petAura := p.RegisterAura(core.Aura{Label: "Forever Soul Link", ActionID: w.ForeverAction("warlock.talent.soul-link"), Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) { p.PseudoStats.DamageDealtMultiplier *= 1.03 }, OnExpire: func(a *core.Aura, sim *core.Simulation) { p.PseudoStats.DamageDealtMultiplier /= 1.03 }})
			p.ApplyOnPetDisable(func(sim *core.Simulation, sac bool) { aura.Deactivate(sim); petAura.Deactivate(sim) })
		}
		w.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if aura.IsActive() && w.ActivePet != nil && w.ActivePet.IsActive() {
				transfer := r.Damage * .3
				r.Damage -= transfer
				pet := w.ActivePet
				pet.RemoveHealth(sim, transfer)
				if pet.CurrentHealth() <= 0 {
					w.changeActivePet(sim, nil, false)
				}
			}
		})
		w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 19028}, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 173}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet != nil }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			aura.Activate(sim)
			w.ActivePet.GetAura("Forever Soul Link").Activate(sim)
		}})
	}
	if w.ForeverRank("warlock.talent.demonic-energies") > 0 {
		healing := w.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: w.ForeverAction("warlock.talent.demonic-energies"), SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagHelpful | core.SpellFlagPassiveSpell | core.SpellFlagIgnoreAttackerModifiers, ProcMask: core.ProcMaskSpellHealing, DamageMultiplier: 1, ThreatMultiplier: 0})
		trigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if w.ActivePet != nil && w.ActivePet.IsActive() && r.Damage > 0 && s.DefenseType == core.DefenseTypeMagic {
				healing.CalcAndDealHealing(sim, &w.ActivePet.Unit, r.Damage*val("demonic-energies", 0)/100, healing.OutcomeHealing)
			}
		}
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Demonic Energies", OnSpellHitDealt: trigger, OnPeriodicDamageDealt: trigger}))
	}
	if w.ForeverRank("warlock.talent.demonic-brand") > 0 {
		for _, p := range w.BasePets {
			p := p
			school := core.SpellSchoolShadow
			if p == w.Imp {
				school = core.SpellSchoolFire
			}
			damage := p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: w.ForeverAction("warlock.talent.demonic-brand"), SpellSchool: school, DefenseType: core.DefenseTypeMagic, Flags: core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: 2})
			core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Demonic Brand Trigger", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
				brand := f.brand.Get(r.Target)
				if s != damage && r.Landed() && brand.IsActive() {
					brand.RemoveStack(sim)
					damage.CalcAndDealDamage(sim, r.Target, sim.Roll(39, 42), damage.OutcomeAlwaysHit)
				}
			}}))
		}
	}
	// Soul Harvest uses actual target deaths, not an assumed recurring proc.
	if w.ForeverRank("warlock.talent.soul-harvesting") > 0 {
		harvest := w.RegisterAura(core.Aura{Label: "Forever Soul Harvest", ActionID: w.ForeverAction("warlock.talent.soul-harvesting"), Duration: 10 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SpiritRegenRateCasting += val("soul-harvesting", 1) / 100
			w.PseudoStats.SpiritRegenMultiplier *= 1 + val("soul-harvesting", 2)/100
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SpiritRegenRateCasting -= val("soul-harvesting", 1) / 100
			w.PseudoStats.SpiritRegenMultiplier /= 1 + val("soul-harvesting", 2)/100
		}})
		w.OnSpellRegistered(func(spell *core.Spell) {
			if spell.SpellCode != SpellCode_WarlockDrainSoul {
				return
			}
			for _, dot := range spell.Dots() {
				if dot == nil {
					continue
				}
				kill := func(a *core.Aura, sim *core.Simulation, damage *core.Spell, r *core.SpellResult) {
					if r.Target.HasHealthBar() && r.Target.CurrentHealth() <= 0 && r.Damage > 0 {
						harvest.Activate(sim)
					}
				}
				dot.OnSpellHitTaken = kill
				dot.OnPeriodicDamageTaken = kill
			}
		})
	}
}
func (w *Warlock) registerForeverPetUtilities() {
	val := func(id string, index int) float64 { return w.ForeverValue("warlock.talent."+id, index, 0) }
	f := w.foreverState
	// Damage/healing magnitudes lacking Forever ranks inherit the closest Classic
	// pet ability scenario. Every such approximation remains PREDICTED metadata.
	if w.HasForeverMechanic("warlock.baseline.incubus") {
		// Same Sayaad pet/attack table; the alternate summon is separately selectable.
		s := w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: w.ForeverAction("warlock.baseline.incubus"), SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: w.BaseMana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 10 * time.Second}, ModifyCast: func(sim *core.Simulation, s *core.Spell, c *core.Cast) { w.changeActivePet(sim, nil, false) }}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { w.changeActivePet(sim, w.Succubus, false) }})
		w.SummonDemonSpells = append(w.SummonDemonSpells, s)
	}
	if w.ForeverRank("warlock.talent.improved-health-funnel") > 0 {
		healthFunnel := w.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: w.ForeverAction("warlock.talent.improved-health-funnel"), SpellSchool: core.SpellSchoolShadow, ProcMask: core.ProcMaskSpellHealing, Flags: WarlockFlagDemonology | core.SpellFlagHelpful | core.SpellFlagAPL | core.SpellFlagChanneled, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1 + val("improved-health-funnel", 0)/100, ThreatMultiplier: 1 - val("improved-health-funnel", 2)/100,
			Hot: core.DotConfig{SelfOnly: true, Aura: core.Aura{Label: "Forever Health Funnel"}, NumberOfTicks: 10, TickLength: time.Second, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				if w.ActivePet == nil || w.CurrentHealth() <= 50 {
					d.Deactivate(sim)
					return
				}
				w.RemoveHealth(sim, 50*(1-val("improved-health-funnel", 1)/100))
				d.Spell.CalcAndDealHealing(sim, &w.ActivePet.Unit, 50, d.Spell.OutcomeHealing)
			}},
			ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
				return w.ActivePet != nil && (w.ForeverRank("warlock.talent.improved-health-funnel") > 0 || w.ActivePet.CurrentHealth() < w.ActivePet.MaxHealth())
			}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { s.SelfHot().Apply(sim) },
		})
		_ = healthFunnel
	}
	// Fire Shield damage is a real retaliation on melee hits while the Imp lives.
	fireShield := w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: w.ForeverAction("warlock.talent.improved-imp"), SpellSchool: core.SpellSchoolFire, Flags: core.SpellFlagPassiveSpell, DamageMultiplier: 1 + val("improved-imp", 0)/100, ThreatMultiplier: 1})
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Fire Shield", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if w.ActivePet == w.Imp && r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) {
			fireShield.CalcAndDealDamage(sim, s.Unit, 13, fireShield.OutcomeAlwaysHit)
		}
	}}))
	w.registerForeverDemonCommands()
	// Voidwalker sacrifice grants a shield and dismisses the pet. The base absorbs
	// 1905 at level60, then the observed effectiveness talent modifies it.
	vwShield := core.NewForeverAbsorb(&w.Unit, "Forever Voidwalker Sacrifice", core.ActionID{SpellID: 19443}, 30*time.Second, 0)
	w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 19443}, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 30 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Voidwalker }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		vwShield.Apply(sim, 1905*(1+val("improved-voidwalker", 0)/100), false)
		w.changeActivePet(sim, nil, true)
	}})
	// Pyroclasm uses a per-tick probability whose product equals the stated chance
	// over the full channel; direct Soul Fire uses the tooltip chance directly.
	stuns := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Pyroclasm-"+w.Label, w.ForeverAction("warlock.talent.pyroclasm"), core.ForeverStun, 3*time.Second)
	})
	dazes := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Aftermath-"+w.Label, w.ForeverAction("warlock.talent.aftermath"), core.ForeverSnare, 5*time.Second)
	})
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Destruction Control", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if r.Landed() {
			if s.SpellCode == SpellCode_WarlockSoulFire && sim.Proc(val("pyroclasm", 0)/100, "Forever Pyroclasm") {
				stuns.Get(r.Target).Activate(sim)
			}
			if s.SpellCode == SpellCode_WarlockConflagrate && sim.Proc(val("aftermath", 1)/100, "Forever Aftermath") {
				dazes.Get(r.Target).Activate(sim)
			}
		}
	}}))
	_ = f
}
