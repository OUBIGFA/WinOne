package stroke

import (
	"encoding/json"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsRestrictGesturesToExplorerFolders(t *testing.T) {
	p := Defaults()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.DefaultPolicy != "disabled" || p.TriggerButton != "right" || p.ScriptThreadCount != 8 {
		t.Fatal("unsafe global defaults", p)
	}
	enabled := 0
	for _, scope := range p.Scopes {
		if scope.Enabled {
			enabled++
			if scope.Kind != "explorer-folders" {
				t.Fatal("non-Explorer scope enabled")
			}
		}
	}
	if enabled != 1 || len(p.Gestures) < 20 || len(p.ActionPackages) == 0 {
		t.Fatal("Stroke defaults incomplete")
	}
}

func TestPenSwitchPreservesLegacyEnabledDefaultAndExplicitOff(t *testing.T) {
	if !Defaults().Pen.IsEnabled() {
		t.Fatal("new pen default is off")
	}
	var legacy Pen
	if err := json.Unmarshal([]byte(`{"color":"#1f7fff","opacity":0.8,"thickness":5}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if !legacy.IsEnabled() {
		t.Fatal("legacy pen was disabled by the new switch")
	}
	var disabled Pen
	if err := json.Unmarshal([]byte(`{"enabled":false,"color":"#1f7fff","opacity":0.8,"thickness":5}`), &disabled); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(disabled)
	if err != nil {
		t.Fatal(err)
	}
	var restored Pen
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.IsEnabled() || restored.Color != legacy.Color || restored.Thickness != legacy.Thickness || restored.Opacity != legacy.Opacity {
		t.Fatal("switch round trip altered pen settings", restored)
	}
}

func TestPackagedSchemaAcceptsCompleteDefaults(t *testing.T) {
	raw, err := os.ReadFile("schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema, defaults any
	if err = json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(defaultJSON, &defaults); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err = compiler.AddResource("https://winone.invalid/stroke", schema); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile("https://winone.invalid/stroke")
	if err != nil {
		t.Fatal(err)
	}
	if err = compiled.Validate(defaults); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultsRetainEveryOfficialReleaseAction(t *testing.T) {
	raw, err := os.ReadFile("upstream/defaults-2026-05-01.json")
	if err != nil {
		t.Fatal(err)
	}
	var original struct {
		ActionPackages []struct {
			Name    string
			Actions []struct{ Name, Gesture, Code string }
		}
		Namespaces, Assemblies, Filtrations []string
		ScriptThreadCount                   int
	}
	if err = json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	p := Defaults()
	if len(p.ActionPackages) != len(original.ActionPackages) || !reflect.DeepEqual(p.Namespaces, original.Namespaces) || !reflect.DeepEqual(p.Assemblies, original.Assemblies) || !reflect.DeepEqual(p.Filters, original.Filtrations) || p.ScriptThreadCount != original.ScriptThreadCount {
		t.Fatal("official defaults were reduced")
	}
	names := map[string]string{}
	for _, g := range p.Gestures {
		names[g.ID] = g.Name
	}
	// Index shipped actions by name and match on name+script, so a default
	// relocated across packages (后退/前进/回到上一层 moved to the front) still
	// maps back to its official release action.
	byName := map[string][]Action{}
	for _, pack := range p.ActionPackages {
		for _, a := range pack.Actions {
			byName[a.Name] = append(byName[a.Name], a)
		}
	}
	// WinOne's requested default swaps the 复制 and 回到上一层 trigger gestures.
	swappedGesture := map[string]string{"复制": "up-then-left", "回到上一层": "up"}
	for _, pack := range original.ActionPackages {
		for _, a := range pack.Actions {
			expected := strings.ReplaceAll(strings.ReplaceAll(a.Code, `"C:"`, `Environment.GetFolderPath(Environment.SpecialFolder.UserProfile)`), `"D:"`, `"shell:MyComputerFolder"`)
			var actual Action
			found := false
			for _, cand := range byName[a.Name] {
				if cand.Code == expected {
					actual = cand
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("official action %s/%s lost or changed", pack.Name, a.Name)
			}
			gesture := names[actual.Gesture]
			if strings.HasPrefix(actual.Gesture, "#") {
				gesture = actual.Gesture
			}
			want := a.Gesture
			if id, swapped := swappedGesture[a.Name]; swapped {
				want = names[id]
			}
			if gesture != want {
				t.Fatalf("action %s gesture is %s, want %s", a.Name, gesture, want)
			}
		}
	}
	// Only 后退/前进/回到上一层 are enabled by default; every other action is off.
	enabled := map[string]bool{}
	for _, pack := range p.ActionPackages {
		for _, a := range pack.Actions {
			if a.Enabled {
				enabled[a.Name] = true
			}
		}
	}
	if len(enabled) != 3 || !enabled["后退"] || !enabled["前进"] || !enabled["回到上一层"] {
		t.Fatalf("default enabled actions changed: %v", enabled)
	}
}

func TestConfigurationRoundTripPreservesUserScopesAndRecordedVectors(t *testing.T) {
	p := Defaults()
	p.Scopes[1].Enabled = true
	p.Gestures[0].Vectors = make([][]int, 128)
	for i := range p.Gestures[0].Vectors {
		p.Gestures[0].Vectors[i] = []int{0, -127}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored Preferences
	if err = json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if err = restored.Validate(); err != nil {
		t.Fatal(err)
	}
	if !restored.Scopes[1].Enabled || len(restored.Gestures[0].Vectors) != 128 {
		t.Fatal("configuration reset explicit user values")
	}
}

func TestConfigurationRejectsMalformedVectorsAndUnknownActionTargets(t *testing.T) {
	p := Defaults()
	p.Gestures[0].Vectors = [][]int{{1, 2}}
	if p.Validate() == nil {
		t.Fatal("accepted malformed Stroke vector")
	}
	p = Defaults()
	p.Scopes[0].Packages = []string{"missing"}
	if p.Validate() == nil {
		t.Fatal("accepted unknown action package")
	}
}
