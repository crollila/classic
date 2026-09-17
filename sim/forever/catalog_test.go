package forever

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic contract fixture only. It never enters the shipped catalog or engine.
func syntheticMechanic() Mechanic {
	return Mechanic{
		MechanicID: "test-only-001", Kind: "talent", Change: "changed", EntityID: 1, ClassID: 8, RaceID: 8, RequiredPieces: 2,
		Confidence: "low", Evidence: []Evidence{{SourceID: "test-fixture", Locator: "test://contract", Note: "Synthetic validator input; no gameplay claim"}},
		Implementation: "sim/forever/catalog.go", Tests: []TestRef{{File: "sim/forever/catalog_test.go", Function: "TestCatalogKinds"}},
	}
}

func syntheticCatalog() Catalog {
	catalog := Baseline()
	catalog.RulesetID = "test-only-ruleset"
	catalog.DatabaseRevision = "test-only-revision"
	catalog.Mechanics = []Mechanic{syntheticMechanic()}
	return catalog
}

func TestCatalogKinds(t *testing.T) {
	for _, kind := range []string{"talent", "ability", "racial", "item", "set_bonus", "combat_mechanic"} {
		for _, change := range []string{"new", "changed"} {
			t.Run(kind+"/"+change, func(t *testing.T) {
				catalog := syntheticCatalog()
				catalog.Mechanics[0].Kind, catalog.Mechanics[0].Change = kind, change
				if err := catalog.Validate(); err != nil {
					t.Fatal(err)
				}
				if err := catalog.ValidateExecutable(); err == nil {
					t.Fatal("importing metadata enabled an unimplemented mechanic")
				}
			})
		}
	}
}

func TestCatalogRejectsMissingProvenance(t *testing.T) {
	cases := map[string]func(*Catalog){
		"schema":            func(c *Catalog) { c.SchemaVersion = 2 },
		"upstream":          func(c *Catalog) { c.UpstreamCommit = "other" },
		"revision":          func(c *Catalog) { c.DatabaseRevision = "" },
		"ruleset":           func(c *Catalog) { c.RulesetID = "" },
		"missing mechanics": func(c *Catalog) { c.Mechanics = nil },
		"id":                func(c *Catalog) { c.Mechanics[0].MechanicID = "" },
		"duplicate":         func(c *Catalog) { c.Mechanics = append(c.Mechanics, c.Mechanics[0]) },
		"kind":              func(c *Catalog) { c.Mechanics[0].Kind = "typo" },
		"change":            func(c *Catalog) { c.Mechanics[0].Change = "typo" },
		"entity id":         func(c *Catalog) { c.Mechanics[0].EntityID = 0 },
		"class id":          func(c *Catalog) { c.Mechanics[0].ClassID = 0 },
		"race id":           func(c *Catalog) { c.Mechanics[0].Kind = "racial"; c.Mechanics[0].RaceID = 0 },
		"set pieces":        func(c *Catalog) { c.Mechanics[0].Kind = "set_bonus"; c.Mechanics[0].RequiredPieces = 0 },
		"confidence":        func(c *Catalog) { c.Mechanics[0].Confidence = "" },
		"evidence":          func(c *Catalog) { c.Mechanics[0].Evidence = nil },
		"empty evidence":    func(c *Catalog) { c.Mechanics[0].Evidence[0].Locator = "" },
		"tests":             func(c *Catalog) { c.Mechanics[0].Tests = nil },
		"test function":     func(c *Catalog) { c.Mechanics[0].Tests[0].Function = "example" },
		"test escape":       func(c *Catalog) { c.Mechanics[0].Tests[0].File = "sim/forever/../../other_test.go" },
		"implementation":    func(c *Catalog) { c.Mechanics[0].Implementation = "sim/core/sim.go" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			catalog := syntheticCatalog()
			mutate(&catalog)
			if err := catalog.Validate(); err == nil {
				t.Fatal("accepted invalid provenance")
			}
		})
	}
}

func TestDecodeCatalogStrict(t *testing.T) {
	valid, err := json.Marshal(Baseline())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeCatalog(strings.NewReader(string(valid))); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"null", "{}", string(valid) + "{}", strings.Replace(string(valid), `"schema_version":1`, `"schema_version":1,"typo":true`, 1)} {
		if _, err := DecodeCatalog(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid catalog %s", input)
		}
	}
}

func TestBaselineExplicitAndEmpty(t *testing.T) {
	baseline := Baseline()
	if err := baseline.ValidateExecutable(); err != nil {
		t.Fatal(err)
	}
	if len(baseline.Mechanics) != 0 {
		t.Fatal("initial milestone must not ship speculative mechanics")
	}
	baseline.RulesetID = "unknown"
	if err := baseline.ValidateExecutable(); err == nil {
		t.Fatal("silently accepted unknown ruleset")
	}
}

// CI gate: shipped mechanics must point to implementation files and real Go tests.
// This checks traceability; reviewers still assess evidence and assertion quality.
func TestShippedMechanicReferences(t *testing.T) {
	for _, mechanic := range Baseline().Mechanics {
		if _, err := os.Stat(filepath.Join("../..", mechanic.Implementation)); err != nil {
			t.Fatal(err)
		}
		for _, ref := range mechanic.Tests {
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join("../..", ref.File), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv != nil || fn.Name.Name != ref.Function || fn.Type.Params.NumFields() != 1 {
					continue
				}
				star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				selector, ok := star.X.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				pkg, ok := selector.X.(*ast.Ident)
				found = ok && pkg.Name == "testing" && selector.Sel.Name == "T"
			}
			if !found {
				t.Fatalf("%s: missing Go test %s in %s", mechanic.MechanicID, ref.Function, ref.File)
			}
		}
	}
}
