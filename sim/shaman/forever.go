package shaman

// Forever records are referenced by stable manifest ID. Where client details are
// absent, Classic coefficients and attack tables are retained; the declaration
// file records predictions separately from observed tooltip values.
import (
	"fmt"
	"math"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

func (s *Shaman) fr(name string) float64       { return float64(s.ForeverRank("shaman.talent." + name)) }
func (s *Shaman) fa(name string) core.ActionID { return s.ForeverAction("shaman.talent." + name) }

func (s *Shaman) applyForeverTalents() {
	if s.Forever == nil {
		return
	}
	if s.fr("tidal-focus") > 0 {
		s.AddStat(stats.MeleeHit, s.fr("tidal-focus"))
		s.AddStat(stats.SpellHit, s.fr("tidal-focus"))
	}
	s.MultiplyStat(stats.Health, 1+.02*s.fr("improved-reincarnation"))
	for _, school := range []stats.SchoolIndex{stats.SchoolIndexFire, stats.SchoolIndexFrost, stats.SchoolIndexNature} {
		s.PseudoStats.SchoolDamageTakenMultiplier[school] *= 1 - .03*s.fr("elemental-warding")
	}
	if s.fr("spirit-weapons") > 0 {
		s.PseudoStats.CanParry = true
		if s.Consumes.MainHandImbue == proto.WeaponImbue_RockbiterWeapon {
			s.PseudoStats.ThreatMultiplier *= 1.3
		} else {
			s.OnSpellRegistered(func(sp *core.Spell) {
				if sp.ProcMask.Matches(core.ProcMaskMeleeOrRanged) {
					sp.ThreatMultiplier *= .7
				}
			})
		}
	}
	s.OnSpellRegistered(func(sp *core.Spell) {
		if sp.DefenseType == core.DefenseTypeMagic && !sp.ProcMask.Matches(core.ProcMaskSpellHealing) && sp.SpellSchool.Matches(core.SpellSchoolFire|core.SpellSchoolFrost|core.SpellSchoolNature) {
			sp.CritDamageBonus += .2 * s.fr("elemental-fury")
		}
		if sp.ProcMask.Matches(core.ProcMaskSpellHealing) {
			sp.PushbackReduction += s.fr("healing-focus") * .23
			// Tidal Mastery's Classic hook also affects lightning; replace its
			// bridge for Forever and apply the healing-only wording here.
			sp.BonusCritRating += s.fr("tidal-mastery") * core.SpellCritRatingPerCritChance
			sp.DamageMultiplier *= 1 + .02*s.fr("purification")
			if sp.SpellCode == SpellCode_ShamanHealingWave {
				sp.DamageMultiplier *= 1 + .08*s.fr("healing-way")
				sp.DefaultCast.CastTime -= time.Duration(s.fr("improved-healing-wave")*100) * time.Millisecond
			}
		}
		if sp.SpellCode == SpellCode_ShamanLightningBolt || sp.SpellCode == SpellCode_ShamanChainLightning {
			sp.BonusCritRating += 3 * s.fr("call-of-thunder") * core.SpellCritRatingPerCritChance
			sp.DefaultCast.CastTime -= time.Duration(s.ForeverValue("shaman.talent.elemental-alacrity", 0, 0)*1000) * time.Millisecond
			sp.PushbackReduction += s.fr("eye-of-the-storm") * .23
			s.ForeverSpellRange(sp, 30+3*s.fr("elemental-reach"))
		}
		if sp.SpellCode == SpellCode_ShamanFlameShock {
			sp.DamageMultiplier *= 1 + .05*s.fr("call-of-flame")
			s.ForeverSpellRange(sp, 20+8*s.fr("elemental-reach"))
		}
		if sp.SpellCode == SpellCode_ShamanLightningShield || sp.SpellCode == SpellCode_ShamanEarthShock || sp.SpellCode == SpellCode_ShamanFlameShock || sp.SpellCode == SpellCode_ShamanFrostShock {
			if sp.Cost != nil {
				sp.Cost.Multiplier -= int32(45 * s.fr("shamanistic-focus"))
			}
		}
	})
	if s.fr("ancestral-healing") > 0 {
		auras := map[*core.Unit]*core.Aura{}
		for _, t := range s.Env.AllUnits {
			if s.IsOpponent(t) {
				continue
			}
			target := t
			m := 1 + s.ForeverValue("shaman.talent.ancestral-healing", 0, 0)/100
			dep := target.NewDynamicMultiplyStat(stats.Armor, m)
			auras[target] = target.RegisterAura(core.Aura{Label: fmt.Sprintf("Forever Ancestral Healing-%d", s.UnitIndex), ActionID: s.fa("ancestral-healing"), Duration: 15 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) { target.EnableDynamicStatDep(sim, dep) }, OnExpire: func(_ *core.Aura, sim *core.Simulation) { target.DisableDynamicStatDep(sim, dep) }})
		}
		core.MakePermanent(s.RegisterAura(core.Aura{Label: "Forever Ancestral Healing", OnHealDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if !r.DidCrit() {
				return
			}
			a := auras[r.Target]
			if a != nil {
				a.Activate(sim)
			}
		}}))
	}
	if s.fr("lightning-overload") > 0 {
		// Each duplicate owns its result buffers and travel callbacks. Reusing the
		// original Chain Lightning closure would overwrite its remaining bounces.
		overloads := map[*core.Spell]*core.Spell{}
		primaryTargets := map[*core.Spell]*core.Unit{}
		s.OnSpellRegistered(func(sp *core.Spell) {
			if sp.Tag != 0 {
				return
			}
			var cfg core.SpellConfig
			switch sp.SpellCode {
			case SpellCode_ShamanLightningBolt:
				cfg = s.newLightningBoltSpellConfig(sp.Rank)
			case SpellCode_ShamanChainLightning:
				cfg = s.newChainLightningSpellConfig(sp.Rank, s.NewTimer())
			default:
				return
			}
			cfg.ActionID.Tag = 1
			cfg.ManaCost = core.ManaCostOptions{}
			cfg.Cast = core.CastConfig{}
			cfg.Flags &^= core.SpellFlagAPL
			cfg.Flags |= core.SpellFlagNoOnCastComplete
			cfg.DamageMultiplier *= .5
			cfg.ThreatMultiplier = 0
			overloads[sp] = s.RegisterSpell(cfg)
			original := sp.ApplyEffects
			sp.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { primaryTargets[sp] = t; original(sim, t, sp) }
		})
		core.MakePermanent(s.RegisterAura(core.Aura{Label: "Forever Lightning Overload", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			extra := overloads[sp]
			if extra == nil || !r.Landed() {
				return
			}
			// A Chain Lightning cast rolls once on its first selected target.
			if sp.SpellCode == SpellCode_ShamanChainLightning && r.Target != primaryTargets[sp] {
				return
			}
			if sim.Proc(.03*s.fr("lightning-overload"), "Forever Lightning Overload") {
				extra.Cast(sim, r.Target)
			}
		}}))
	}
	if s.fr("maelstrom-weapon") > 0 {
		var affected []*core.Spell
		s.OnSpellRegistered(func(sp *core.Spell) {
			if sp.SpellCode == SpellCode_ShamanLightningBolt {
				affected = append(affected, sp)
			}
		})
		amount := .04 * s.fr("maelstrom-weapon")
		a := s.RegisterAura(core.Aura{Label: "Forever Maelstrom Weapon", ActionID: s.fa("maelstrom-weapon"), Duration: 30 * time.Second, MaxStacks: 5, OnStacksChange: func(_ *core.Aura, sim *core.Simulation, old, new int32) {
			for _, sp := range affected {
				sp.CastTimeMultiplier -= float64(new-old) * amount
				sp.Cost.Multiplier -= int32(float64(new-old) * amount * 100)
			}
		}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellCode == SpellCode_ShamanLightningBolt {
				a.Deactivate(sim)
			}
		}})
		// PREDICTED 20% per landed melee attack; the tooltip supplies no proc rate.
		core.MakePermanent(s.RegisterAura(core.Aura{Label: "Forever Maelstrom Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.Landed() && sp.ProcMask.Matches(core.ProcMaskMelee) && sim.Proc(s.ForeverParameter("maelstrom_proc_chance", .20), "Forever Maelstrom") {
				a.Activate(sim)
				a.AddStack(sim)
			}
		}}))
	}
}

func (s *Shaman) registerForeverSpells() {
	if s.Forever == nil {
		return
	}
	s.registerForeverHealing()
	s.registerForeverUtility()
	s.registerForeverTotems()

	if s.fr("improved-reincarnation") > 0 {
		// Classic Reincarnation's 20% health/Mana restoration and 60-minute
		// cooldown, modified by the observed Forever talent. Death occurrence
		// remains recorded in metrics even when the player resurrects.
		action := core.ActionID{SpellID: 20608}
		health := s.NewHealthMetrics(action)
		mana := s.NewManaMetrics(action)
		s.RegisterSpell(core.SpellConfig{ActionID: action, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{CD: core.Cooldown{Timer: s.NewTimer(), Duration: time.Hour - time.Duration(10*s.fr("improved-reincarnation"))*time.Minute}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return s.CurrentHealth() <= 0 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			fraction := .2 + .1*s.fr("improved-reincarnation")
			s.GainHealth(sim, s.MaxHealth()*fraction, health)
			delta := s.MaxMana()*fraction - s.CurrentMana()
			if delta >= 0 {
				s.AddMana(sim, delta, mana)
			} else {
				s.SpendMana(sim, -delta, mana)
			}
		}})
	}
	if s.fr("lava-burst") > 0 {
		sp := s.RegisterSpell(core.SpellConfig{ActionID: s.fa("lava-burst"), SpellSchool: core.SpellSchoolFire, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagShaman | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: 165, Multiplier: 100 - 2*s.Talents.Convection}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 2500*time.Millisecond - time.Duration(s.ForeverValue("shaman.talent.elemental-alacrity", 0, 0)*1000)*time.Millisecond}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 10 * time.Second}}, PushbackReduction: s.fr("eye-of-the-storm") * .23,
			DamageMultiplier: 1 + .05*s.fr("call-of-flame"), ThreatMultiplier: 1, BonusCoefficient: 2.5 / 3.5,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				mult := sp.DamageMultiplier
				for _, fs := range s.FlameShock {
					if fs != nil && fs.Dot(t).IsActive() {
						sp.DamageMultiplier *= 1.2
						break
					}
				}
				sp.CalcAndDealDamage(sim, t, sim.Roll(158, 187), sp.OutcomeMagicHitAndCrit)
				sp.DamageMultiplier = mult
			}})
		s.ForeverSpellRange(sp, 30+3*s.fr("elemental-reach"))
	}
	// Fire Nova is evidenced as a baseline replacement for Fire Nova Totem.
	if !foreverdata.IsStrict(s.Forever) && (s.HasForeverMechanic("shaman.baseline.fire-nova") || s.fr("improved-fire-nova") > 0 || s.fr("call-of-flame") > 0) {
		sp := s.RegisterSpell(core.SpellConfig{ActionID: s.ForeverAction("shaman.baseline.fire-nova"), SpellSchool: core.SpellSchoolFire, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagShaman | core.SpellFlagAPL,
			ManaCost: core.ManaCostOptions{FlatCost: 520}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 15*time.Second - time.Duration(s.fr("improved-fire-nova")*2)*time.Second}}, DamageMultiplier: 1 + .10*s.fr("improved-fire-nova") + .05*s.fr("call-of-flame"), ThreatMultiplier: 1, BonusCoefficient: .214,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				for _, target := range s.Env.Encounter.Targets {
					sp.CalcAndDealDamage(sim, &target.Unit, sim.Roll(413, 459), sp.OutcomeMagicHitAndCrit)
				}
			}})
		s.ForeverSpellRange(sp, 10+3*s.fr("elemental-reach"))
	}
	if s.fr("water-shield") > 0 {
		metrics := s.NewManaMetrics(s.fa("water-shield"))
		cd := core.Cooldown{Timer: s.NewTimer(), Duration: time.Duration(s.ForeverParameter("water_shield_icd_seconds", 3.5) * float64(time.Second))}
		var a *core.Aura
		proc := func(sim *core.Simulation) {
			if cd.IsReady(sim) {
				cd.Use(sim)
				s.AddMana(sim, .02*s.MaxMana(), metrics)
				a.RemoveStack(sim)
			}
		}
		a = s.RegisterAura(core.Aura{Label: "Forever Water Shield", ActionID: s.fa("water-shield"), Duration: 10 * time.Minute, MaxStacks: 3, OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.Landed() {
				proc(sim)
			}
		}, OnHealDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidCrit() {
				proc(sim)
			}
		}})
		s.RegisterSpell(core.SpellConfig{ActionID: s.fa("water-shield"), Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 15 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if s.ActiveShieldAura != nil {
				s.ActiveShieldAura.Deactivate(sim)
			}
			a.Activate(sim)
			a.SetStacks(sim, 3)
			s.ActiveShield = sp
			s.ActiveShieldAura = a
		}})
	}
	if s.fr("mana-tide-totem") > 0 {
		metrics := map[*core.Character]*core.ResourceMetrics{}
		for _, a := range s.Party.Players {
			c := a.GetCharacter()
			if c.HasManaBar() {
				metrics[c] = c.NewManaMetrics(s.fa("mana-tide-totem"))
			}
		}
		sp := s.RegisterSpell(core.SpellConfig{ActionID: s.fa("mana-tide-totem"), Flags: core.SpellFlagAPL | core.SpellFlagHelpful | SpellFlagTotem, ManaCost: core.ManaCostOptions{FlatCost: 10, Multiplier: s.totemManaMultiplier()}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 5 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			position := s.DistanceFromTarget
			s.ActiveTotems[WaterTotem] = sp
			s.TotemExpirations[WaterTotem] = sim.CurrentTime + 12*time.Second
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 3 * time.Second, NumTicks: 4, OnAction: func(sim *core.Simulation) {
				if s.ActiveTotems[WaterTotem] == sp {
					for c, m := range metrics {
						if math.Abs(c.DistanceFromTarget-position) <= 30 {
							c.AddMana(sim, 88, m)
						}
					}
				}
			}})
		}})
		s.AddMajorCooldown(core.MajorCooldown{Spell: sp, Type: core.CooldownTypeMana, ShouldActivate: func(sim *core.Simulation, c *core.Character) bool { return c.CurrentManaPercent() < .65 }})
	}
}

func (s *Shaman) registerForeverHealing() {
	// Classic level-60 counterparts are an explicit provisional baseline; their
	// IDs identify those Classic ranks, not alleged new Forever client ranks.
	for _, v := range []struct {
		id              int32
		code            int32
		mana, low, high float64
		cast            time.Duration
	}{{25357, SpellCode_ShamanHealingWave, 620, 1620, 1850, 3 * time.Second}, {10468, SpellCode_ShamanLesserHealingWave, 380, 832, 928, 1500 * time.Millisecond}, {10623, SpellCode_ShamanChainHeal, 405, 567, 646, 2500 * time.Millisecond}} {
		v := v
		sp := s.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: v.id}, SpellCode: v.code, SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagShaman | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: v.mana, Multiplier: 100 - int32(s.fr("tidal-focus"))}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: v.cast}}, DamageMultiplier: 1, ThreatMultiplier: .5, BonusCoefficient: v.cast.Seconds() / 3.5,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				if s.IsOpponent(t) {
					t = &s.Unit
				}
				heal := sim.Roll(v.low, v.high)
				if v.code == SpellCode_ShamanChainHeal {
					originalMultiplier := sp.DamageMultiplier
					defer func() { sp.DamageMultiplier = originalMultiplier }()
					if hot := t.GetAura(fmt.Sprintf("Forever Riptide-%d", s.UnitIndex)); hot != nil && hot.IsActive() {
						sp.DamageMultiplier *= 1.25
					}
					sp.CalcAndDealHealing(sim, t, heal, sp.OutcomeHealingCrit)
					sp.DamageMultiplier *= .5
					n := 0
					party := s.Env.Raid.GetPlayerParty(t)
					if party == nil || len(party.Players) == 0 {
						party = s.Party
					}
					for _, a := range party.Players {
						u := &a.GetCharacter().Unit
						if u != t && n < 2 {
							sp.CalcAndDealHealing(sim, u, heal, sp.OutcomeHealingCrit)
							sp.DamageMultiplier *= .5
							n++
						}
					}
				} else {
					sp.CalcAndDealHealing(sim, t, heal, sp.OutcomeHealingCrit)
				}
			}})
		s.ForeverSpellRange(sp, 40)
		switch v.code {
		case SpellCode_ShamanHealingWave:
			s.HealingWave = []*core.Spell{sp}
		case SpellCode_ShamanLesserHealingWave:
			s.LesserHealingWave = []*core.Spell{sp}
		case SpellCode_ShamanChainHeal:
			s.ChainHeal = []*core.Spell{sp}
		}
	}
	if s.fr("riptide") > 0 {
		s.RegisterSpell(core.SpellConfig{ActionID: s.fa("riptide"), SpellSchool: core.SpellSchoolNature, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagShaman | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 245, Multiplier: 100 - int32(s.fr("tidal-focus"))}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 6 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: .5, BonusCoefficient: .429,
			Hot: core.DotConfig{Aura: core.Aura{Label: "Forever Riptide"}, NumberOfTicks: 5, TickLength: 3 * time.Second, OnSnapshot: func(sim *core.Simulation, t *core.Unit, d *core.Dot, b bool) {
				d.SnapshotBaseDamage = 499.0/5 + .1*d.Spell.HealingPower(t)
				d.SnapshotAttackerMultiplier = d.Spell.CasterHealingMultiplier()
			}, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				d.CalcAndDealPeriodicSnapshotHealing(sim, t, d.OutcomeTick)
			}},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				if s.IsOpponent(t) {
					t = &s.Unit
				}
				sp.CalcAndDealHealing(sim, t, sim.Roll(479, 528), sp.OutcomeHealingCrit)
				sp.Hot(t).Apply(sim)
			}})
	}
}
