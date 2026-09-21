package warlock

import (
	"math"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

type foreverWarlockState struct {
	sacrificed                          *WarlockPet
	execute20                           bool
	banes                               core.AuraArray
	havoc                               *core.Unit
	havocAuras, hope, shadowBolt, brand core.AuraArray
	decimation, shadowFlame, fireFlame  *core.Aura
	drainsBonus, siphonPerEffect        float64
}

// foreverDotOutcome lets Forever damage over time crit at its snapshot crit chance with or
// without Pandemic: the client's Pandemic "increases the critical strike damage bonus" of
// DoTs, which implies they already crit (ElliotWood/Forever crits every DoT). Needs
// in-game confirmation. Classic ticks never crit.
func (w *Warlock) foreverDotOutcome(dot *core.Dot) core.OutcomeApplier {
	if w.Forever != nil {
		return dot.OutcomeSnapshotCrit
	}
	return dot.OutcomeTick
}
func (w *Warlock) applyForeverCasterTalents() {
	if w.Forever == nil {
		return
	}
	f := &foreverWarlockState{}
	w.foreverState = f
	c := &w.Character
	f.drainsBonus = w.talentValueOr("improved-drains", 0, 1, 100*foreverImprovedDrains[min(3, w.ForeverRank("warlock.talent.improved-drains"))]) / 100
	// Client: "$s1% per each of your other Affliction effects ... up to ${$m1*3}%". The research
	// record's 17/34/51 and 10/20/30 are the demo's numbers.
	f.siphonPerEffect = w.talentValueOr("soul-siphon", 0, 1, 4*float64(w.ForeverRank("warlock.talent.soul-siphon"))) / 100
	rank := func(id string) int32 { return w.ForeverRank("warlock.talent." + id) }
	// Talent numbers at the chosen rank, by client effect (the research record's value order is
	// not the client's, and the client stores reductions as negative numbers).
	seconds := func(id string, effect, recordIndex int) float64 {
		return w.talentValueOr(id, effect, -1, 1000*w.ForeverValue("warlock.talent."+id, recordIndex, 0)) / 1000
	}
	isb := w.talentValue("improved-shadow-bolt", 0, 1, 0) / 100
	wrackBonus := 0.0
	if rank("drain-hope") > 0 {
		wrackBonus = foreverClientValue(c, foreverWrackID, 1, 10) / 100
	}
	decimationCast := w.talentValue("decimation", 0, -1, 4) / 100
	decimationCooldown := w.talentValue("decimation", 1, -1, 0) / 100
	decimationThreshold := w.talentValue("decimation", 2, 1, 1) / 100
	decimationDamage := w.talentValue("decimation", 3, 1, 2) / 100
	shadowFlameShadow := w.talentValue("shadow-and-flame", 2, 1, 0) / 100
	shadowFlameFire := w.talentValue("shadow-and-flame", 3, 1, 2) / 100
	// Client: the brand arms 2/4/6 pet attacks (the demo record reads 2 at every rank).
	brandStacks := int32(w.talentValueOr("demonic-brand", 1, 1, float64(2*min(3, rank("demonic-brand")))))
	// Client: 17/33/50% less threat (the demo record reads 17/34/51).
	brandThreat := w.talentValueOr("demonic-brand", 0, -1, 100*[4]float64{0, .17, .33, .50}[min(3, rank("demonic-brand"))]) / 100
	destructiveReach := w.talentValue("destructive-reach", 0, 1, 0) / 100
	cataclysm := w.talentValue("cataclysm", 0, -1, 0)
	ruin := w.talentValue("ruin", 0, 1, 0) / 100
	agonizingDamage := w.talentValue("agonizing-flames", 1, 1, 1) / 100
	agonizingCrit := w.talentValue("agonizing-flames", 0, 1, 0)
	intensity := w.talentValue("intensity", 0, 1, 0) / 100
	corruptionCast := seconds("improved-corruption", 0, 0)
	corruptionDamage := w.talentValue("improved-corruption", 1, 1, 1) / 100
	agonyDamage := w.talentValue("improved-bane-of-agony", 0, 1, 0) / 100
	brimstone := w.talentValue("fire-and-brimstone", 1, 1, 0)
	baneBolt, baneSoulFire := seconds("bane", 0, 0), seconds("bane", 1, 1)
	felConcentration := w.talentValue("fel-concentration", 0, 1, 0) / 100
	malediction := w.talentValue("malediction", 0, 1, 0) / 100
	pandemic := w.talentValue("pandemic", 0, 1, 0) / 100
	shadowFlameDuration, fireFlameDuration := 20*time.Second, 20*time.Second
	if rank("shadow-and-flame") > 0 {
		// The talent's tooltip names buff spells 1293816/426311 that the build does not carry.
		shadowFlameDuration = foreverDuration(c, 1293816, shadowFlameDuration, "Shadow and Flame Shadow buff")
		fireFlameDuration = foreverDuration(c, 426311, fireFlameDuration, "Shadow and Flame Fire buff")
	}
	f.banes = make(core.AuraArray, len(w.Env.AllUnits))
	f.hope = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Drain Hope-" + w.Label, ActionID: w.ForeverAction("warlock.talent.drain-hope"), Duration: foreverDuration(c, foreverWrackID, 6*time.Second, "Wrack")})
	})
	f.shadowBolt = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Improved Shadow Bolt-" + w.Label, ActionID: w.ForeverAction("warlock.talent.improved-shadow-bolt"), Duration: foreverDuration(c, 17794, 12*time.Second, "Shadow Vulnerability")})
	})
	f.havocAuras = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Bane of Havoc-" + w.Label, Tag: "forever-debuff-bane", ActionID: w.ForeverAction("warlock.talent.bane-of-havoc"), Duration: foreverDuration(c, foreverHavocID, 5*time.Minute, "Bane of Havoc")})
	})
	f.brand = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Demonic Brand-" + w.Label, ActionID: w.ForeverAction("warlock.talent.demonic-brand"), Duration: foreverDuration(c, 1293696, 10*time.Second, "Demonic Brand debuff"), MaxStacks: max(6, brandStacks)})
	})
	f.decimation = w.RegisterAura(core.Aura{Label: "Forever Decimation", ActionID: w.ForeverAction("warlock.talent.decimation"), Duration: foreverDuration(c, 440873, 10*time.Second, "Decimation buff"),
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range w.SoulFire {
				s.CastTimeMultiplier -= decimationCast
			}
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range w.SoulFire {
				s.CastTimeMultiplier += decimationCast
			}
		},
	})
	f.shadowFlame = w.RegisterAura(core.Aura{Label: "Forever Shadow and Flame Shadow", ActionID: w.ForeverAction("warlock.talent.shadow-and-flame"), Duration: shadowFlameDuration,
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1 + shadowFlameShadow
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= 1 + shadowFlameShadow
		},
	})
	f.fireFlame = w.RegisterAura(core.Aura{Label: "Forever Shadow and Flame Fire", ActionID: w.ForeverAction("warlock.talent.shadow-and-flame"), Duration: fireFlameDuration,
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] *= 1 + shadowFlameFire
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] /= 1 + shadowFlameFire
		},
	})
	w.ForeverDamageMultiplier(func(s *core.Spell, at *core.AttackTable) float64 {
		mult := 1.0
		if s.SpellSchool.Matches(core.SpellSchoolShadow) && f.shadowBolt.Get(at.Defender).IsActive() {
			mult *= 1 + isb
		}
		if s.SpellSchool.Matches(core.SpellSchoolShadow) && s.SpellCode != SpellCode_WarlockDrainHope && len(s.Dots()) > 0 && f.hope.Get(at.Defender).IsActive() {
			mult *= 1 + wrackBonus
		}
		if isForeverDrain(s) {
			mult *= w.foreverDrainMultiplier(s, at.Defender)
		}
		return mult
	})
	w.OnSpellRegistered(func(s *core.Spell) {
		if !s.Flags.Matches(SpellFlagWarlock) {
			return
		}
		if s.DefenseType == core.DefenseTypeMagic {
			// The client's Improved Drains no longer extends Drain Life's range.
			w.ForeverSpellRange(s, 30*(1+destructiveReach))
		}
		if s.Flags.Matches(WarlockFlagDestruction) {
			if s.Cost != nil {
				s.Cost.Multiplier -= int32(math.Round(cataclysm))
			}
			s.CritDamageBonus += ruin
			s.DamageMultiplierAdditive += agonizingDamage
			s.PushbackReduction += intensity
		}
		if s.SpellCode == SpellCode_WarlockCorruption {
			s.DefaultCast.CastTime -= time.Duration(corruptionCast * float64(time.Second))
			s.DamageMultiplierAdditive += corruptionDamage
		}
		if s.SpellCode == SpellCode_WarlockCurseOfAgony {
			s.DamageMultiplierAdditive += agonyDamage
		}
		if s.SpellCode == SpellCode_WarlockSearingPain {
			s.BonusCritRating += agonizingCrit * core.SpellCritRatingPerCritChance
			s.ThreatMultiplier *= 1 - brandThreat
		}
		if s.SpellCode == SpellCode_WarlockConflagrate {
			s.BonusCritRating += brimstone * core.SpellCritRatingPerCritChance
		}
		if s.SpellCode == SpellCode_WarlockShadowBolt || s.SpellCode == SpellCode_WarlockImmolate || s.SpellCode == SpellCode_WarlockIncinerate {
			s.DefaultCast.CastTime -= time.Duration(baneBolt * float64(time.Second))
		}
		if s.SpellCode == SpellCode_WarlockSoulFire {
			s.DefaultCast.CastTime -= time.Duration(baneSoulFire * float64(time.Second))
			s.CD.Duration = time.Duration(float64(s.CD.Duration) * (1 - decimationCooldown))
		}
		if isForeverDrain(s) {
			s.PushbackReduction += felConcentration
		}
		if d := s.AOEDot(); d != nil {
			d.DamageMultiplier *= 1 + malediction
		}
		for _, d := range s.Dots() {
			if d == nil {
				continue
			}
			d.DamageMultiplier *= 1 + malediction
		}
		if s.SpellSchool.Matches(core.SpellSchoolShadow) && len(s.Dots()) > 0 {
			s.CritDamageBonus += pandemic
		}
		if s.SpellCode == SpellCode_WarlockCurseOfAgony || s.SpellCode == SpellCode_WarlockCurseOfDoom {
			for _, d := range s.Dots() {
				if d != nil {
					d.Tag = "forever-debuff-bane"
				}
			}
			old := s.ApplyEffects
			s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				oldCurse := w.ActiveCurseAura[t.UnitIndex]
				w.ActiveCurseAura[t.UnitIndex] = f.banes[t.UnitIndex]
				old(sim, t, sp)
				f.banes[t.UnitIndex] = w.ActiveCurseAura[t.UnitIndex]
				w.ActiveCurseAura[t.UnitIndex] = oldCurse
			}
		}
		old := s.ApplyEffects
		s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			execute := (sp.SpellCode == SpellCode_WarlockSearingPain || sp.SpellCode == SpellCode_WarlockShadowBolt) && rank("decimation") > 0 && ((decimationThreshold == .35 && sim.IsExecutePhase35()) || (t.HasHealthBar() && t.CurrentHealthPercent() < decimationThreshold))
			mult := 1.0
			if execute {
				mult += decimationDamage
				f.decimation.Activate(sim)
			}
			sp.DamageMultiplier *= mult
			old(sim, t, sp)
			sp.DamageMultiplier /= mult
		}
	})
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Warlock triggers", OnReset: func(a *core.Aura, sim *core.Simulation) {
		f.havoc = nil
		f.sacrificed = nil
		f.execute20 = false
		sim.RegisterExecutePhaseCallback(func(sim *core.Simulation, phase int32) {
			if phase == 20 {
				f.execute20 = true
			}
		})
		for i := range f.banes {
			f.banes[i] = nil
		}
	}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.Landed() {
			return
		}
		if s.SpellCode == SpellCode_WarlockShadowBolt && r.DidCrit() && rank("improved-shadow-bolt") > 0 {
			f.shadowBolt.Get(r.Target).Activate(sim)
		}
		if s.SpellCode == SpellCode_WarlockSearingPain && rank("demonic-brand") > 0 {
			a := f.brand.Get(r.Target)
			a.Activate(sim)
			a.SetStacks(sim, brandStacks)
		}
		if rank("shadow-and-flame") > 0 {
			if s.SpellCode == SpellCode_WarlockConflagrate {
				f.shadowFlame.Activate(sim)
			}
			if s.SpellCode == SpellCode_WarlockShadowburn {
				f.fireFlame.Activate(sim)
			}
		}
	}}))
	w.applyForeverPetTalents()
}
func (w *Warlock) registerForeverSpells() {
	if w.Forever == nil {
		return
	}
	f := w.foreverState
	c := &w.Character
	if w.ForeverRank("warlock.talent.incinerate") > 0 {
		// The talent keeps its Forever action at every rank; the client rank the level has
		// learned supplies damage (with per-level growth), mana, cast time, coefficient and
		// the bonus against Immolate.
		i, id := foreverRankAt(c, foreverIncinerateIDs(), foreverIncinerateLevels())
		fb := foreverIncinerateRanks[i]
		lo, hi := foreverClientRange(c, id, 0, fb.min, fb.max)
		immolateBonus := foreverClientValue(c, id, 1, 25) / 100
		w.RegisterSpell(core.SpellConfig{ActionID: w.ForeverAction("warlock.talent.incinerate"), SpellCode: SpellCode_WarlockIncinerate, SpellSchool: core.SpellSchoolFire, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: WarlockFlagDestruction | core.SpellFlagAPL,
			ManaCost: foreverManaCost(c, id, fb.mana, "Incinerate"), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: foreverCastTime(c, id, 2500*time.Millisecond, "Incinerate")}}, DamageMultiplier: 1, ThreatMultiplier: 1,
			BonusCoefficient: foreverCoefficient(c, id, 0, 2.5/3.5, "Incinerate"),
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				mult := 1.0
				if w.getActiveImmolateSpell(t) != nil {
					mult = 1 + immolateBonus
				}
				s.DamageMultiplier *= mult
				s.CalcAndDealDamage(sim, t, sim.Roll(lo, hi), s.OutcomeMagicHitAndCrit)
				s.DamageMultiplier /= mult
			},
		})
	}
	if w.ForeverRank("warlock.talent.drain-hope") > 0 {
		// Wrack (1316697) in the client: the tick, period, duration, coefficient and mana come
		// from it (36 Shadow a second for 6 sec at 14.3% a tick, 200 mana in the beta build).
		// The talent record still carries the demo's "Drain Hope" 52 and 10%.
		tick := foreverClientValue(c, foreverWrackID, 0, 36)
		period := foreverPeriod(c, foreverWrackID, 0, time.Second, "Wrack")
		ticks := int32(max(1, foreverDuration(c, foreverWrackID, 6*time.Second, "Wrack")/period))
		w.RegisterSpell(core.SpellConfig{ActionID: w.ForeverAction("warlock.talent.drain-hope"), SpellCode: SpellCode_WarlockDrainHope, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: WarlockFlagAffliction | core.SpellFlagAPL | core.SpellFlagChanneled, ManaCost: foreverManaCost(c, foreverWrackID, 200, "Wrack"), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: 1,
			Dot: core.DotConfig{Aura: core.Aura{Label: "Forever Drain Hope damage-" + w.Label}, NumberOfTicks: ticks, TickLength: period, BonusCoefficient: foreverCoefficient(c, foreverWrackID, 0, .143, "Wrack"), OnSnapshot: func(sim *core.Simulation, t *core.Unit, d *core.Dot, roll bool) { d.Snapshot(t, tick, roll) }, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				d.CalcAndDealPeriodicSnapshotDamage(sim, t, w.foreverDotOutcome(d))
			}},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				r := s.CalcAndDealOutcome(sim, t, s.OutcomeMagicHit)
				if r.Landed() {
					s.Dot(t).Apply(sim)
					f.hope.Get(t).Activate(sim)
				}
			},
		})
	}
	if w.ForeverRank("warlock.talent.bane-of-havoc") > 0 {
		action := w.ForeverAction("warlock.talent.bane-of-havoc")
		share := foreverClientValue(c, foreverHavocID, 0, w.ForeverValue("warlock.talent.bane-of-havoc", 1, 15)) / 100
		copy := w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action.WithTag(-action.Tag), SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagPassiveSpell | core.SpellFlagIgnoreAttackerModifiers | core.SpellFlagIgnoreTargetModifiers, DamageMultiplier: 1, ThreatMultiplier: 1})
		trigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if w.IsOpponent(r.Target) && r.Damage > 0 && s != copy && f.havoc != nil && r.Target != f.havoc && f.havocAuras.Get(f.havoc).IsActive() {
				copy.CalcAndDealDamage(sim, f.havoc, r.Damage*share, copy.OutcomeAlwaysHit)
			}
		}
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Bane of Havoc trigger", OnSpellHitDealt: trigger, OnPeriodicDamageDealt: trigger}))
		w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: core.SpellSchoolShadow, Flags: WarlockFlagDestruction | core.SpellFlagAPL, ManaCost: foreverManaCost(c, foreverHavocID, 43, "Bane of Havoc"), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if f.havoc != nil {
				f.havocAuras.Get(f.havoc).Deactivate(sim)
			}
			if old := f.banes.Get(t); old != nil {
				old.Deactivate(sim)
			}
			f.havoc = t
			f.banes[t.UnitIndex] = f.havocAuras.Get(t)
			f.banes.Get(t).Activate(sim)
		}})
	}
	if w.ForeverRank("warlock.talent.curse-of-exhaustion") > 0 {
		// Curse of Exhaustion (18223): snare, duration and mana from the client; Amplify
		// Curse (18288) adds its effect 1 to the snare.
		duration := foreverDuration(c, foreverExhaustionID, 12*time.Second, "Curse of Exhaustion")
		snare := -foreverClientValue(c, foreverExhaustionID, 0, -30) / 100
		amplifiedSnare := snare + w.talentValue("amplify-curse", 1, -1, 1)/100
		auras := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverSnareAura("Forever Curse of Exhaustion-"+w.Label, w.ForeverAction("warlock.talent.curse-of-exhaustion"), duration, snare)
		})
		amplified := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverSnareAura("Forever Amplified Exhaustion-"+w.Label, w.ForeverAction("warlock.talent.curse-of-exhaustion"), duration, amplifiedSnare)
		})
		w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: w.ForeverAction("warlock.talent.curse-of-exhaustion"), SpellSchool: core.SpellSchoolShadow, Flags: WarlockFlagAffliction | core.SpellFlagAPL, ManaCost: foreverManaCost(c, foreverExhaustionID, 69, "Curse of Exhaustion"), Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if old := w.ActiveCurseAura.Get(t); old != nil {
				old.Deactivate(sim)
			}
			a := auras.Get(t)
			if w.AmplifyCurseAura != nil && w.AmplifyCurseAura.IsActive() {
				w.AmplifyCurseAura.Deactivate(sim)
				a = amplified.Get(t)
			}
			w.ActiveCurseAura[t.UnitIndex] = a
			a.Activate(sim)
		}})
	}
	// Health Funnel is a channeled health transfer. The Classic 10-second channel
	// and 50-per-second transfer are explicit best-guess baseline parameters.
	w.registerForeverMissingAffliction()
	w.registerForeverPetUtilities()
	w.registerForeverSoulShards()
}

// Client ids of Forever-only warlock spells.
const (
	foreverWrackID      = 1316697
	foreverHavocID      = 1225228
	foreverExhaustionID = 18223
)

// foreverIncinerateRanks: 412758 (40), 1293812 (50), 1293813 (60). The numbers are only the
// fallback (beta-client tooltips at each rank's top level) for a build without these spells;
// the client rank spell supplies the values.
var foreverIncinerateRanks = []struct {
	id             int32
	level          int32
	mana, min, max float64
}{{412758, 40, 205, 99, 113}, {1293812, 50, 265, 145, 167}, {1293813, 60, 325, 201, 233}}

func foreverIncinerateIDs() []int32 {
	out := []int32{}
	for _, r := range foreverIncinerateRanks {
		out = append(out, r.id)
	}
	return out
}
func foreverIncinerateLevels() []int32 {
	out := []int32{}
	for _, r := range foreverIncinerateRanks {
		out = append(out, r.level)
	}
	return out
}

// isForeverDrain is Drain Life, Drain Soul or Wrack, the spells Improved Drains, Soul
// Siphon, Fel Concentration and Nightfall name in the Forever client.
func isForeverDrain(s *core.Spell) bool {
	return s.SpellCode == SpellCode_WarlockDrainLife || s.SpellCode == SpellCode_WarlockDrainSoul || s.SpellCode == SpellCode_WarlockDrainHope
}

// foreverImprovedDrains is the beta client's flat Improved Drains bonus per rank: the fallback
// for a build without the talent (client effect 0 supplies it).
var foreverImprovedDrains = [4]float64{0, .07, .13, .20}

// foreverDrainMultiplier is Improved Drains (client effect 0: flat 7/13/20%) times Soul Siphon
// (client effect 0: 4/8/12% for each of the warlock's other Affliction effects on the target,
// up to three times that). The two talents are separate multipliers, as ElliotWood/Forever has them.
func (w *Warlock) foreverDrainMultiplier(s *core.Spell, target *core.Unit) float64 {
	mult := 1 + w.foreverState.drainsBonus
	if w.ForeverRank("warlock.talent.soul-siphon") == 0 {
		return mult
	}
	count := 0
	for _, other := range w.Spellbook {
		if other != s && other.Flags.Matches(WarlockFlagAffliction) && len(other.Dots()) > 0 {
			if d := other.Dot(target); d != nil && d.IsActive() {
				count++
			}
		}
	}
	if curse := w.ActiveCurseAura.Get(target); curse != nil && curse.IsActive() {
		count++
	}
	perEffect := w.foreverState.siphonPerEffect
	limit := 3 * perEffect
	return mult * (1 + min(float64(count)*perEffect, limit))
}
