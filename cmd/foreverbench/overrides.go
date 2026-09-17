package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/game"
)

// OverrideUse aggregates one override path across every spec preset that evaluated it.
type OverrideUse struct {
	Kind    string   `json:"kind"`
	ID      string   `json:"id"`
	Field   string   `json:"field"`
	Class   string   `json:"class,omitempty"`
	Classic *float64 `json:"classic,omitempty"`
	Forever *float64 `json:"forever,omitempty"`
	Reason  string   `json:"reason,omitempty"`
	Beliefs []string `json:"beliefs,omitempty"`
	Specs   []string `json:"specs"`
}

// OverrideSpec is what one preset uses: its class and equipped items (for affected-spec analysis).
type OverrideSpec struct {
	Spec     string  `json:"spec"`
	Class    string  `json:"class"`
	Items    []int32 `json:"items"`
	Applied  int     `json:"applied"`
	Rejected int     `json:"rejected"`
	Error    string  `json:"error,omitempty"`
}

// cmdOverrides runs every preset once in Forever mode and reports which best-known
// overrides were applied or rejected (and why). The knowledge pipeline turns
// rejections into implementation candidates.
func cmdOverrides(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("overrides", stderr)
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	entries, excluded, err := selectEntries(c.phaseValue, c.role, c.specs, c.gearFill)
	if err != nil {
		return nil, err
	}
	entries, excluded = b.withTalents(entries, excluded, "forever")
	enc := EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}
	applied, rejected := map[string]*OverrideUse{}, map[string]*OverrideUse{}
	specs := make([]OverrideSpec, len(entries))
	jobs := []func(){}
	results := make([]*proto.RaidSimRequest, len(entries))
	reports := make([][2][]foreverdata.OverrideEvent, len(entries))
	for i, e := range entries {
		i, e := i, e
		jobs = append(jobs, func() {
			spec := OverrideSpec{Spec: e.Spec, Class: e.Class, Items: []int32{}}
			defer func() {
				if r := recover(); r != nil {
					spec.Error = fmt.Sprintf("panic: %v", r)
				}
				specs[i] = spec
			}()
			race := b.chooseRace(e, false, game.Forever, enc)
			req, err := b.classicRequest(e, race.race, enc, b.norm.Iterations)
			if err != nil {
				spec.Error = err.Error()
				return
			}
			freq, _, _, _, err := b.foreverRequest(req, e)
			if err != nil {
				spec.Error = "forever setup: " + err.Error()
				return
			}
			results[i] = freq
			for _, party := range freq.GetRaid().GetParties() {
				for _, player := range party.GetPlayers() {
					for _, item := range player.GetEquipment().GetItems() {
						if item.GetId() > 0 {
							spec.Items = append(spec.Items, item.GetId())
						}
					}
				}
			}
			_, _, report, err := game.RunRaidSimWithOverrideReport(game.Selection{Version: game.Forever, Discovery: true}, freq)
			if err != nil {
				spec.Error = firstLine(err.Error())
			}
			if report == nil {
				return
			}
			for _, player := range report.Players {
				reports[i][0] = append(reports[i][0], player.Applied...)
				reports[i][1] = append(reports[i][1], player.Rejected...)
			}
			spec.Applied, spec.Rejected = len(reports[i][0]), len(reports[i][1])
		})
	}
	runJobs(jobs, b.parallel)
	add := func(into map[string]*OverrideUse, ev foreverdata.OverrideEvent, e PresetEntry) {
		key := ev.Scope + "|" + ev.ID + "|" + ev.Field + "|" + ev.Reason
		use, ok := into[key]
		if !ok {
			use = &OverrideUse{Kind: ev.Scope, ID: ev.ID, Field: ev.Field, Class: strings.ToUpper(e.Class), Classic: ev.Classic, Forever: ev.Forever,
				Reason: ev.Reason, Beliefs: ev.Beliefs}
			into[key] = use
		}
		for _, s := range use.Specs {
			if s == e.Spec {
				return
			}
		}
		use.Specs = append(use.Specs, e.Spec)
	}
	for i, e := range entries {
		for _, ev := range reports[i][0] {
			add(applied, ev, e)
		}
		for _, ev := range reports[i][1] {
			add(rejected, ev, e)
		}
	}
	flatten := func(m map[string]*OverrideUse) []*OverrideUse {
		out := make([]*OverrideUse, 0, len(m))
		for _, v := range m {
			sort.Strings(v.Specs)
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Kind != out[j].Kind {
				return out[i].Kind < out[j].Kind
			}
			if out[i].ID != out[j].ID {
				return out[i].ID < out[j].ID
			}
			return out[i].Field < out[j].Field
		})
		return out
	}
	return map[string]interface{}{
		"command":    "overrides",
		"phase":      c.phaseValue.String(),
		"overrides":  foreverdata.OverridesInfo(),
		"applied":    flatten(applied),
		"rejected":   flatten(rejected),
		"specs":      specs,
		"excluded":   excludedList(excluded),
		"provenance": provenanceBlock(root, map[string]interface{}{"normalization": c.normalization()}),
		"elapsed_ms": time.Since(start).Milliseconds(),
	}, nil
}
