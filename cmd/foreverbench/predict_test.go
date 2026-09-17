package main

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

type predictOutput struct {
	Caveats []string `json:"caveats"`
	Results map[string]struct {
		Rotation  string           `json:"rotation"`
		Abilities []PredictAbility `json:"abilities"`
		Character struct {
			FinalStats map[string]float64 `json:"final_stats"`
		} `json:"character"`
		Target struct {
			BaseArmor  float64 `json:"base_armor"`
			Armor      float64 `json:"armor_after_debuffs"`
			PhysicalDR float64 `json:"physical_damage_reduction_pct"`
		} `json:"target"`
		Debuffs   map[string]interface{} `json:"debuffs"`
		Overrides struct {
			AppliedCount      *int               `json:"applied_count"`
			RequestParameters map[string]float64 `json:"request_parameters"`
		} `json:"forever_overrides"`
		Caveats []string `json:"caveats"`
	} `json:"results"`
}

func runPredictCLI(t *testing.T, args ...string) predictOutput {
	t.Helper()
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	out := predictOutput{}
	if err := json.Unmarshal(runCLI(t, append([]string{"predict", "-compact-json"}, args...)...), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestPredictCasterSpellScalesWithSpellPower(t *testing.T) {
	avgHit := func(spellPower string) float64 {
		out := runPredictCLI(t, "-spec", "mage-frost", "-phase", "P1", "-game", "classic", "-ability", "frostbolt", "-iterations", "150",
			"-stats", "spell_power="+spellPower, "-buffs", "none", "-debuffs", "none")
		r := out.Results["classic"]
		if len(r.Abilities) != 1 || r.Abilities[0].AvgHit == nil || r.Abilities[0].Counts.Casts == 0 || r.Abilities[0].School != "frost" {
			t.Fatalf("unexpected abilities: %+v", r.Abilities)
		}
		if got := r.Character.FinalStats["spell_power"]; got != mustFloat(t, spellPower) {
			t.Fatalf("final spell power %v, want %s", got, spellPower)
		}
		if len(r.Debuffs) != 0 {
			t.Fatalf("-debuffs none left debuffs: %v", r.Debuffs)
		}
		return *r.Abilities[0].AvgHit
	}
	low, high := avgHit("400"), avgHit("600")
	// Frostbolt: 3.0s cast, 0.814 coefficient, up to +6% from Piercing Ice.
	if perPoint := (high - low) / 200; perPoint < 0.78 || perPoint > 0.9 {
		t.Fatalf("frostbolt gained %.4f damage per spell power (%.1f -> %.1f), want about 0.814-0.863", perPoint, low, high)
	}
}

func TestPredictMeleeAbilityWithArmorDebuff(t *testing.T) {
	run := func(debuffs string) (PredictAbility, float64, float64) {
		out := runPredictCLI(t, "-spec", "warrior-fury", "-phase", "P1", "-ability", "Bloodthirst", "-iterations", "150", "-ab-iterations", "20",
			"-target-armor", "3731", "-debuffs", debuffs, "-stats", "attack_power=1500,melee_crit=28.5")
		r := out.Results["forever"]
		if len(r.Abilities) != 1 || r.Abilities[0].ID != "spell:23894" || r.Abilities[0].AvgHit == nil || r.Abilities[0].AvgCrit == nil || !r.Abilities[0].Melee {
			t.Fatalf("unexpected abilities: %+v", r.Abilities)
		}
		if r.Character.FinalStats["attack_power"] != 1500 || r.Character.FinalStats["melee_crit"] != 28.5 {
			t.Fatalf("forced stats not applied: %v", r.Character.FinalStats)
		}
		if r.Overrides.AppliedCount == nil || !strings.HasPrefix(r.Rotation, "spec rotation") {
			t.Fatalf("missing override report or unexpected rotation %q", r.Rotation)
		}
		return r.Abilities[0], r.Target.Armor, r.Target.PhysicalDR
	}
	bare, bareArmor, bareDR := run("none")
	sundered, sunderedArmor, sunderedDR := run("none,sunderArmor")
	// Without raid debuffs the Fury rotation still applies its own single Sunder Armor.
	if bareArmor != 3731-450 || sunderedArmor != 3731-5*450 || sunderedDR >= bareDR {
		t.Fatalf("armor %v -> %v, physical reduction %v -> %v", bareArmor, sunderedArmor, bareDR, sunderedDR)
	}
	if *sundered.AvgHit <= *bare.AvgHit*1.1 || *sundered.AvgCrit <= *bare.AvgCrit {
		t.Fatalf("5 Sunders did not raise Bloodthirst: hit %.1f -> %.1f, crit %.1f -> %.1f", *bare.AvgHit, *sundered.AvgHit, *bare.AvgCrit, *sundered.AvgCrit)
	}
	// Same attack power and armor: the hit scales with the armor damage modifier only.
	want := (1 - sunderedDR/100) / (1 - bareDR/100)
	if got := *sundered.AvgHit / *bare.AvgHit; got < want*0.97 || got > want*1.03 {
		t.Fatalf("Bloodthirst hit ratio %.4f, armor modifier ratio %.4f", got, want)
	}
}

func TestPredictUnknownAbilityAndFallback(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	var out bytes.Buffer
	code := run([]string{"predict", "-spec", "warrior-fury", "-ability", "definitely not a spell", "-iterations", "20", "-ab-iterations", "10", "-compact-json"}, &out, io.Discard)
	if code != 1 || !strings.Contains(out.String(), "Abilities used by the rotation") || !strings.Contains(out.String(), "Bloodthirst [spell:23894]") {
		t.Fatalf("unhelpful unknown-ability error (exit %d): %s", code, out.String())
	}
	out.Reset()
	if code := run([]string{"predict", "-spec", "warrior-fury", "-ability", "Bloodthirst", "-stats", "no_such_stat=1"}, &out, io.Discard); code != 1 || !strings.Contains(out.String(), "attack_power") {
		t.Fatalf("unknown stat accepted: %s", out.String())
	}

	// Slam is registered but not part of the Fury rotation: single-cast APL fallback.
	res := runPredictCLI(t, "-spec", "warrior-fury", "-ability", "slam", "-iterations", "30", "-ab-iterations", "10")
	r := res.Results["forever"]
	if len(r.Abilities) == 0 || r.Abilities[0].Counts.Casts == 0 || !strings.HasPrefix(r.Rotation, "fallback") || len(r.Caveats) == 0 {
		t.Fatalf("fallback not used: %+v", r)
	}
}

func TestPredictAttackTableParameter(t *testing.T) {
	dodge := func(params string) float64 {
		args := []string{"-spec", "warrior-fury", "-ability", "Bloodthirst", "-iterations", "150", "-ab-iterations", "10"}
		if params != "" {
			args = append(args, "-parameters", params)
		}
		return runPredictCLI(t, args...).Results["forever"].Abilities[0].TablePct["dodge"]
	}
	if base, none := dodge(""), dodge("core.combat.dodge_offset=-0.25"); base <= 3 || none != 0 {
		t.Fatalf("dodge %.2f%% -> %.2f%% with a -25%% dodge offset", base, none)
	}
}

func TestPredictBuffFieldLists(t *testing.T) {
	req := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{{}}}}}}
	if err := applyBuffLists(req, "none,rallying_cry_of_the_dragonslayer,battleShout=regular,powerInfusions=2", "none,sunderArmor,curse_of_weakness,faerie_fire=false"); err != nil {
		t.Fatal(err)
	}
	player := req.Raid.Parties[0].Players[0]
	if !player.Buffs.RallyingCryOfTheDragonslayer || req.Raid.Buffs.BattleShout != proto.TristateEffect_TristateEffectRegular || player.Buffs.PowerInfusions != 2 {
		t.Fatalf("buffs not set: %v %v", req.Raid.Buffs, player.Buffs)
	}
	if !req.Raid.Debuffs.SunderArmor || req.Raid.Debuffs.FaerieFire || req.Raid.Debuffs.CurseOfWeakness != core.FullDebuffs.CurseOfWeakness {
		t.Fatalf("debuffs not set: %v", req.Raid.Debuffs)
	}
	buffs, debuffs := buffMessages(req)
	if got := activeFields(debuffs); len(got) != 2 || got["sunder_armor"] != true {
		t.Fatalf("active debuffs: %v", got)
	}
	if len(activeFields(buffs)) != 3 || len(settableFields(buffs)) < 20 {
		t.Fatalf("active buffs: %v", activeFields(buffs))
	}
	for _, bad := range []string{"no_such_buff", "battle_shout=huge", "sunder_armor,none"} {
		if err := applyBuffLists(req, bad, bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func mustFloat(t *testing.T, s string) float64 {
	t.Helper()
	v, err := parseKeyValues("v=" + s)
	if err != nil {
		t.Fatal(err)
	}
	return v["v"]
}
