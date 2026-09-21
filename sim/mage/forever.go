package mage

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"slices"
	"time"
)

type foreverMageState struct {
	blast, missiles, hotStreak, fingers *core.Aura
	frozen, scorch, winter              core.AuraArray

	// The client ranks (and their values at the character's level) the level-appropriate
	// talent/baseline spells were built from.
	arcaneBlast, iceLance, frostfireBolt foreverRankSpell
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
	// Talent values are the client's (per-rank points, else the talent spell's effect) at
	// the chosen rank, by CLIENT effect index; signs and units are the client's.
	value := func(id string, effect int) float64 { return m.clientTalent(id, effect, 0) }
	// Arcane Subtlety: effect 1 lowers target resistances (negative), effect 0 threat.
	m.AddStat(stats.SpellPenetration, -value("arcane-subtlety", 1))
	if rank("arcane-instability") > 0 {
		// Effect 2 (all critical strike chance) per rank; the spell part is Classic code.
		m.AddStat(stats.MeleeCrit, value("arcane-instability", 2)*core.CritRatingPerCritChance)
	}
	if rank("magic-absorption") > 0 {
		// Effect 1: resistances; effect 0: % of total mana restored; the talent spell's ICD.
		m.AddResistances(value("magic-absorption", 1))
		timer := m.NewTimer()
		icd := time.Second
		if s := m.ClientSpell(29441); s != nil && s.ProcICDMs > 0 {
			icd = time.Duration(s.ProcICDMs) * time.Millisecond
		}
		mana := m.NewManaMetrics(m.ForeverAction("mage.talent.magic-absorption"))
		core.MakePermanent(m.RegisterAura(core.Aura{Label: "Forever Magic Absorption", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.DefenseType == core.DefenseTypeMagic && r.Outcome.Matches(core.OutcomeMiss) && timer.IsReady(sim) {
				m.AddMana(sim, m.MaxMana()*value("magic-absorption", 0)/100, mana)
				timer.Set(sim.CurrentTime + icd)
			}
		}}))
	}
	// Proc auras' numbers: Frostbite freeze 12494, Fire Vulnerability 22959, Winter's Chill
	// 12579, Hot Streak 400625, Fingers of Frost 400669, Arcane Blast 400573, Missile
	// Barrage 400588 (talent) / 400589 (buff), Impact stun 12355.
	freezeDuration := m.clientDuration(12494, 5*time.Second)
	scorchPerStack := m.clientEffect(22959, 0, 3) / 100
	winterCritPerStack := m.clientEffect(12579, 0, 2)
	// Winter's Chill: effect 1 is the proc chance, effect 0 the stack limit ("Stacks up to
	// $s1 times"), capped by the debuff's own limit.
	winterStacks := max(1, min(m.clientMaxStacks(12579, 5), int32(value("winter-s-chill", 0))))
	hotStreakCast := -m.clientEffect(400625, 0, -25) / 100
	// Fingers of Frost: effect 1 is the chance, effect 0 the number of spells it covers.
	fingersCharges := max(1, int32(value("fingers-of-frost", 0)))
	blastDamage, blastCost := m.clientEffect(400573, 0, 10)/100, int32(m.clientEffect(400573, 1, 175))
	barrageChance := m.clientTalent("missile-barrage", 0, 40) / 100
	barrageDuration := 1 + m.clientEffect(400589, 0, -50)/100
	barrageCost := int32(-m.clientEffect(400589, 2, -100))
	stunDuration := m.clientDuration(12355, 2*time.Second)
	// Permafrost: effect 0 lengthens Chill (%), effect 1 deepens its slow (negative points).
	chillDuration := time.Duration(8 * float64(time.Second) * (1 + value("permafrost", 0)/100))
	chillSlow := .4 - value("permafrost", 1)/100
	f.frozen = m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Frozen-"+m.Label, m.ForeverAction("mage.talent.frostbite"), core.ForeverRoot, freezeDuration)
	})
	f.scorch = m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Scorch-" + m.Label, ActionID: m.ForeverAction("mage.talent.improved-scorch"), Duration: m.clientDuration(22959, 30*time.Second), MaxStacks: m.clientMaxStacks(22959, 5)})
	})
	f.winter = m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Winter's Chill-" + m.Label, ActionID: m.ForeverAction("mage.talent.winter-s-chill"), Duration: m.clientDuration(12579, 15*time.Second), MaxStacks: winterStacks})
	})
	m.ForeverDamageMultiplier(func(s *core.Spell, at *core.AttackTable) float64 {
		if s.SpellSchool.Matches(core.SpellSchoolFire) {
			return 1 + scorchPerStack*float64(f.scorch.Get(at.Defender).GetStacks())
		}
		return 1
	})
	f.hotStreak = m.RegisterAura(core.Aura{Label: "Forever Hot Streak", ActionID: m.ForeverAction("mage.talent.hot-streak"), Duration: m.clientDuration(400625, 15*time.Second), MaxStacks: m.clientMaxStacks(400625, 3),
		OnStacksChange: func(a *core.Aura, sim *core.Simulation, old, new int32) {
			for _, s := range m.Pyroblast {
				if s != nil {
					s.CastTimeMultiplier -= hotStreakCast * float64(new-old)
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
	// A charge is spent by the next spell cast. Arcane Missiles' missiles land after its
	// cast completes, so its charge is spent when the channel ends instead, letting the
	// missiles see the Frozen state they paid for.
	fingersMissiles := false
	f.fingers = m.RegisterAura(core.Aura{Label: "Forever Fingers of Frost", ActionID: m.ForeverAction("mage.talent.fingers-of-frost"), Duration: m.clientDuration(400669, 15*time.Second), MaxStacks: fingersCharges,
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			if s.Flags.Matches(core.SpellFlagAPL) && s.Flags.Matches(SpellFlagMage) && s.DefenseType == core.DefenseTypeMagic && a.RemainingDuration(sim) != a.Duration {
				if s.SpellCode == SpellCode_MageArcaneMissiles {
					fingersMissiles = true
					return
				}
				a.RemoveStack(sim)
			}
		},
	})
	f.blast = m.RegisterAura(core.Aura{Label: "Forever Arcane Blast", ActionID: m.ForeverAction("mage.talent.arcane-blast"), Duration: m.clientDuration(400573, 8*time.Second), MaxStacks: m.clientMaxStacks(400573, 4),
		OnStacksChange: func(a *core.Aura, sim *core.Simulation, old, new int32) {
			for _, s := range m.Spellbook {
				if !s.Flags.Matches(SpellFlagMage) {
					continue
				}
				if s.SpellCode == SpellCode_MageArcaneBlast {
					s.Cost.Multiplier += blastCost * (new - old)
				} else {
					s.DamageMultiplierAdditive += blastDamage * float64(new-old)
				}
			}
		},
		OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			// "Lasts 8 sec or until any other damage spell is cast." Arcane Missiles drops the
			// stacks when its channel ends (below) so its missiles keep the bonus; other
			// channels such as Blizzard snapshot the bonus before this runs.
			if s.Flags.Matches(SpellFlagMage) && s.SpellCode != SpellCode_MageArcaneBlast && s.DefenseType == core.DefenseTypeMagic && s.ProcMask.Matches(core.ProcMaskSpellDamage) && s.SpellCode != SpellCode_MageArcaneMissiles && s.SpellCode != SpellCode_MageArcaneMissilesTick {
				a.Deactivate(sim)
			}
		},
	})
	f.missiles = m.RegisterAura(core.Aura{Label: "Forever Missile Barrage", ActionID: m.ForeverAction("mage.talent.missile-barrage"), Duration: m.clientDuration(400589, 15*time.Second),
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range m.ArcaneMissiles {
				if s != nil {
					s.Cost.Multiplier -= barrageCost
					for _, d := range s.Dots() {
						if d != nil {
							d.TickLength = time.Duration(float64(d.TickLength) * barrageDuration)
						}
					}
				}
			}
		},
		OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range m.ArcaneMissiles {
				if s != nil {
					s.Cost.Multiplier += barrageCost
					for _, d := range s.Dots() {
						if d != nil {
							d.TickLength = time.Duration(float64(d.TickLength) / barrageDuration)
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
			s.DamageMultiplier *= 1 + value("wand-specialization", 0)/100
		}
		if !s.Flags.Matches(SpellFlagMage) {
			return
		}
		if s.SpellSchool.Matches(core.SpellSchoolArcane) {
			s.ThreatMultiplier *= 1 + value("arcane-subtlety", 0)/100
		}
		if s.SpellSchool.Matches(core.SpellSchoolFire) {
			s.ThreatMultiplier *= 1 + value("burning-soul", 1)/100
			s.PushbackReduction += value("burning-soul", 0) / 100
		}
		if s.SpellCode == SpellCode_MageArcaneMissiles {
			s.PushbackReduction += value("improved-channeling", 0) / 100
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
					if fingersMissiles {
						fingersMissiles = false
						if f.fingers.IsActive() {
							f.fingers.RemoveStack(sim)
						}
					}
				}
			}
		}
		if s.SpellCode == SpellCode_MageArcaneBlast {
			s.PushbackReduction += value("improved-channeling", 1) / 100
		}
		if s.SpellCode == SpellCode_MageFireBlast {
			// Wake of Fire effect 0: cooldown change in ms (negative).
			s.CD.Duration += time.Duration(value("wake-of-fire", 0)) * time.Millisecond
		}
		if s.SpellCode == SpellCode_MageFireBlast || s.SpellCode == SpellCode_MageScorch || s.SpellCode == SpellCode_MageIceLance || s.SpellCode == SpellCode_MageArcaneBlast {
			s.BonusCritRating += value("incineration", 0) * core.SpellCritRatingPerCritChance
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
			yards += value("arcane-geometry", 0)
		}
		if s.SpellSchool.Matches(core.SpellSchoolFire) {
			yards += value("flame-throwing", 0)
		}
		if s.SpellCode == SpellCode_MageFrostbolt || s.SpellCode == SpellCode_MageFrostfireBolt || slices.Contains(BlizzardSpellId[1:], s.SpellID) {
			yards *= 1 + value("arctic-reach", 0)/100
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
					crit += value("shatter", 0) * core.SpellCritRatingPerCritChance
				}
				if sp.SpellCode == SpellCode_MageIceLance || sp.SpellCode == SpellCode_MageFrostbolt {
					crit += winterCritPerStack * float64(f.winter.Get(t).GetStacks()) * core.SpellCritRatingPerCritChance
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
		if s.SpellCode == SpellCode_MageScorch && rank("improved-scorch") > 0 && sim.Proc(min(1, value("improved-scorch", 0)/100), "Forever Scorch") {
			a := f.scorch.Get(r.Target)
			a.Activate(sim)
			a.AddStack(sim)
		}
		if s.SpellSchool.Matches(core.SpellSchoolFrost) && rank("winter-s-chill") > 0 && sim.Proc(min(1, value("winter-s-chill", 1)/100), "Forever Winter's Chill") {
			a := f.winter.Get(r.Target)
			a.Activate(sim)
			a.AddStack(sim)
		}
		chill := s.Flags.Matches(SpellFlagChillSpell) || s.SpellCode == SpellCode_MageFrostbolt || s.SpellCode == SpellCode_MageFrostfireBolt
		if chill {
			if rank("frostbite") > 0 && sim.Proc(value("frostbite", 0)/100, "Forever Frostbite") {
				f.frozen.Get(r.Target).Activate(sim)
			}
			if rank("fingers-of-frost") > 0 && sim.Proc(value("fingers-of-frost", 1)/100, "Forever Fingers of Frost") {
				f.fingers.Activate(sim)
				f.fingers.SetStacks(sim, fingersCharges)
			}
			slow := r.Target.ForeverSnareAura("Forever Chill-"+m.Label, m.ForeverAction("mage.talent.permafrost"), chillDuration, chillSlow)
			slow.Activate(sim)
		}
		if s.SpellSchool.Matches(core.SpellSchoolFire) && rank("impact") > 0 && sim.Proc(value("impact", 0)/100, "Forever Impact") {
			r.Target.ForeverControlAura("Forever Impact-"+m.Label, m.ForeverAction("mage.talent.impact"), core.ForeverStun, stunDuration).Activate(sim)
		}
		eligible := s.SpellCode == SpellCode_MageFireball || s.SpellCode == SpellCode_MageFireBlast || s.SpellCode == SpellCode_MageScorch || s.SpellCode == SpellCode_MageFrostfireBolt
		if eligible && r.DidCrit() && rank("hot-streak") > 0 {
			f.hotStreak.Activate(sim)
			f.hotStreak.AddStack(sim)
		}
		chance := 0.0
		// "Arcane Blast a $m1% chance, and Fireball, Frostbolt and Frostfire Bolt ${$m1/2}%".
		if s.SpellCode == SpellCode_MageArcaneBlast {
			chance = barrageChance
		} else if s.SpellCode == SpellCode_MageFireball || s.SpellCode == SpellCode_MageFrostbolt || s.SpellCode == SpellCode_MageFrostfireBolt {
			chance = barrageChance / 2
		}
		if chance > 0 && rank("missile-barrage") > 0 && sim.Proc(chance, "Forever Missile Barrage") {
			f.missiles.Activate(sim)
		}
	}}))
	// Pre-register control auras because adding auras while a simulation runs would
	// violate the engine's initialization contract.
	m.Env.RegisterPreFinalizeEffect(func() {
		for _, t := range m.Env.Encounter.Targets {
			t.ForeverSnareAura("Forever Chill-"+m.Label, m.ForeverAction("mage.talent.permafrost"), chillDuration, chillSlow)
			t.ForeverControlAura("Forever Impact-"+m.Label, m.ForeverAction("mage.talent.impact"), core.ForeverStun, stunDuration)
		}
	})
}

func (m *Mage) registerForeverSpells() {
	if m.Forever == nil {
		return
	}
	f := m.foreverState
	// Talent spells keep their Forever action at every rank (rotations name it); the rank
	// the character's level has learned is read from the client: damage (with per-level
	// growth), cost, cast time and coefficient.
	if m.ForeverRank("mage.talent.arcane-blast") > 0 {
		ab := m.clientRank(foreverTalentRankAt(m.Level, foreverArcaneBlastRanks), 0, -1, 2500*time.Millisecond, 2.5/3.5)
		f.arcaneBlast = ab
		m.RegisterSpell(core.SpellConfig{ActionID: m.ForeverAction("mage.talent.arcane-blast"), SpellCode: SpellCode_MageArcaneBlast, SpellSchool: core.SpellSchoolArcane, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | core.SpellFlagAPL,
			ManaCost: ab.manaCost(), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: ab.cast}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: ab.coefficient,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				s.CalcAndDealDamage(sim, t, sim.Roll(ab.low, ab.high), s.OutcomeMagicHitAndCrit)
				f.blast.Activate(sim)
				f.blast.AddStack(sim)
			},
		})
	}
	if m.ForeverRank("mage.talent.ice-lance") > 0 {
		il := m.clientRank(foreverTalentRankAt(m.Level, foreverIceLanceRanks), 0, -1, 0, 1.5/3.5/3)
		f.iceLance = il
		if !il.hasCoefficientData {
			// The client's damage effect carries no coefficient; 1.5/3.5/3 stays an assumption.
			gamedata.Use("mage: Ice Lance", "spell power coefficient", il.coefficient, "PROVISIONAL", "client damage effect lists coefficient 0; 1.5/3.5/3 (instant, one third) assumed")
		}
		// "Deals $s2% increased damage to Frozen targets": effect 1.
		frozenMult := 1 + m.clientEffect(il.id, 1, 300)/100
		m.RegisterSpell(core.SpellConfig{ActionID: m.ForeverAction("mage.talent.ice-lance"), SpellCode: SpellCode_MageIceLance, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | core.SpellFlagAPL,
			ManaCost: il.manaCost(), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: il.coefficient,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				mult := 1.0
				if f.frozen.Get(t).IsActive() || f.fingers.IsActive() {
					mult = frozenMult
				}
				s.DamageMultiplier *= mult
				s.CalcAndDealDamage(sim, t, sim.Roll(il.low, il.high), s.OutcomeMagicHitAndCrit)
				s.DamageMultiplier /= mult
			},
		})
	}
	// Frostfire Bolt (learned at 40): Fire and Frost; effect 1 is the direct damage, effect 2
	// the periodic part. The top rank keeps the Forever action so the rotations that name it
	// keep working.
	if fr0, ok := foreverRankAt(m.Level, foreverFrostfireBoltRanks); ok && m.HasForeverMechanic("mage.baseline.frostfire-bolt") {
		action := core.ActionID{SpellID: fr0.id}
		if fr0.id == foreverFrostfireBoltRanks[len(foreverFrostfireBoltRanks)-1].id {
			action = m.ForeverAction("mage.baseline.frostfire-bolt")
		}
		fr := m.clientRank(fr0, 1, 2, 3*time.Second, .814)
		f.frostfireBolt = fr
		dotTick, ticks, tickLength := foreverFrostfireBoltDot[fr0.level]/3, int32(3), 3*time.Second
		if fr.period > 0 && fr.duration > 0 {
			dotTick, ticks, tickLength = fr.periodic, int32(fr.duration/fr.period), fr.period
		}
		// The client lists coefficient 0 for the periodic part; it is simulated without one.
		gamedata.Use("mage: Frostfire Bolt", "periodic spell power coefficient", 0, "PROVISIONAL", "client periodic effect lists coefficient 0")
		// Improved Fireball: client effect 0 is the cast time change in ms (-100 a rank).
		castTime := fr.cast + time.Duration(m.clientTalent("improved-fireball", 0, -100*float64(m.ForeverRank("mage.talent.improved-fireball"))))*time.Millisecond
		m.RegisterSpell(core.SpellConfig{ActionID: action, SpellCode: SpellCode_MageFrostfireBolt, SpellSchool: core.SpellSchoolFire | core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | SpellFlagChillSpell | core.SpellFlagAPL,
			ManaCost: fr.manaCost(), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: castTime}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: fr.coefficient,
			Dot: core.DotConfig{Aura: core.Aura{Label: "Frostfire Bolt"}, NumberOfTicks: ticks, TickLength: tickLength,
				OnSnapshot: func(sim *core.Simulation, t *core.Unit, dot *core.Dot, isRollover bool) {
					dot.Snapshot(t, dotTick, isRollover)
				},
				OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, t, dot.OutcomeTick)
				},
			},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				if s.CalcAndDealDamage(sim, t, sim.Roll(fr.low, fr.high), s.OutcomeMagicHitAndCrit).Landed() && dotTick > 0 {
					s.Dot(t).Apply(sim)
				}
			},
		})
	}
	m.RegisterForeverWand()
	m.registerForeverDefenses()
}
