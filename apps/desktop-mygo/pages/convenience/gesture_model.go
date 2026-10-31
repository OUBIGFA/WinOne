package convenience

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"winone/modules/stroke"
)

// The scene model edits mouse gestures by scene (WGestures' app → gesture
// → command flow) over Stroke's package format, which the Host and the
// worker keep unchanged.
//
// React source: features/mouse-gestures-scenes-model.ts.

// strokeID makes a new ID, as crypto.randomUUID without its dashes.
func strokeID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return hex.EncodeToString(b)
}

// escapedProcessPattern makes a path a pattern that matches it exactly.
func escapedProcessPattern(path string) string { return regexp.QuoteMeta(path) }

// cloneStroke copies preferences deeply, so a draft never shares a slice
// with what the Host stored.
func cloneStroke(p stroke.Preferences) stroke.Preferences {
	data, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	var out stroke.Preferences
	if err := json.Unmarshal(data, &out); err != nil {
		panic(err)
	}
	return out
}

// orderedScopes shows Explorer's scenes first; the stored order, which
// imported rules keep, is never rewritten.
func orderedScopes(scopes []stroke.Scope) []stroke.Scope {
	ordered := slices.Clone(scopes)
	slices.SortStableFunc(ordered, func(a, b stroke.Scope) int {
		explorer := func(s stroke.Scope) int {
			if s.Kind == "explorer-folders" {
				return 0
			}
			return 1
		}
		return explorer(a) - explorer(b)
	})
	return ordered
}

// sceneEntry is an action a scene runs, and the action that wins its
// gesture instead, when another does.
type sceneEntry struct {
	pack         stroke.ActionPackage
	action       stroke.Action
	overriddenBy string
}

// sceneActions lists a scene's actions. A gesture runs, as Stroke's
// GestureEngine.Find does, the first enabled action of the last package
// that has one; shadowed actions stay listed, to edit.
func sceneActions(p stroke.Preferences, scope stroke.Scope) []sceneEntry {
	var packages []stroke.ActionPackage
	for _, pack := range p.ActionPackages {
		if slices.Contains(scope.Packages, pack.ID) {
			packages = append(packages, pack)
		}
	}
	type winner struct{ pack, action string }
	winners := map[string]winner{}
	names := map[winner]string{}
	for i := len(packages) - 1; i >= 0; i-- {
		for _, action := range packages[i].Actions {
			if _, taken := winners[action.Gesture]; action.Enabled && !taken {
				winners[action.Gesture] = winner{packages[i].ID, action.ID}
				names[winner{packages[i].ID, action.ID}] = action.Name
			}
		}
	}
	var entries []sceneEntry
	for _, pack := range packages {
		for _, action := range pack.Actions {
			entry := sceneEntry{pack: pack, action: action}
			if w, ok := winners[action.Gesture]; action.Enabled && ok && w != (winner{pack.ID, action.ID}) {
				entry.overriddenBy = names[w]
			}
			entries = append(entries, entry)
		}
	}
	// Installations keep their original packages, Explorer's "parent"
	// included. Only the shipped action orders are shown navigation first;
	// a reordered or extended package keeps its order. Sorting the view
	// never changes precedence or what is saved.
	navigation := []string{"back", "forward", "parent"}
	shipped := []string{
		"copy,paste,cut,select-all,undo,redo,back,forward",
		"back,forward,parent,copy,paste,cut,select-all,undo,redo",
		"refresh,fullscreen,new-folder,parent,new-window",
	}
	for _, pack := range packages {
		ids := make([]string, len(pack.Actions))
		navigates := false
		for i, action := range pack.Actions {
			ids[i] = action.ID
			navigates = navigates || slices.Contains(navigation, action.ID)
		}
		if navigates && !slices.Contains(shipped, strings.Join(ids, ",")) {
			return entries
		}
	}
	rank := func(id string) int {
		if i := slices.Index(navigation, id); i >= 0 {
			return i
		}
		return len(navigation)
	}
	slices.SortStableFunc(entries, func(a, b sceneEntry) int { return rank(a.action.ID) - rank(b.action.ID) })
	return entries
}

// packageName is name shortened to Stroke's limit (240 UTF-8 bytes leave
// room for 54 characters and a suffix), made unique with " (2)" and on.
func packageName(p stroke.Preferences, name string) string {
	runes := []rune(name)
	base := string(runes[:min(len(runes), 54)])
	result := base
	for index := 2; slices.ContainsFunc(p.ActionPackages, func(pack stroke.ActionPackage) bool { return pack.Name == result }); index++ {
		result = base + " (" + strconv.Itoa(index) + ")"
	}
	return result
}

// The scene model's failures, shown as "stroke."+message.
var (
	errMissingSceneAction = errors.New("missingSceneAction")
	errPackageLimit       = errors.New("packageLimit")
)

// editScenePackage edits the actions of one of a scene's packages. A
// package other scenes use too is copied for this scene first, so an
// edit never changes another scene; merely visiting a scene copies
// nothing. It returns the preferences and the ID of the edited package.
func editScenePackage(p stroke.Preferences, scopeID, packageID string, edit func([]stroke.Action) ([]stroke.Action, error)) (stroke.Preferences, string, error) {
	scopeIndex := slices.IndexFunc(p.Scopes, func(s stroke.Scope) bool { return s.ID == scopeID })
	packIndex := slices.IndexFunc(p.ActionPackages, func(pack stroke.ActionPackage) bool { return pack.ID == packageID })
	if scopeIndex < 0 || packIndex < 0 || !slices.Contains(p.Scopes[scopeIndex].Packages, packageID) {
		return p, "", errMissingSceneAction
	}
	shared := slices.ContainsFunc(p.Scopes, func(s stroke.Scope) bool { return s.ID != scopeID && slices.Contains(s.Packages, packageID) })
	if shared && len(p.ActionPackages) >= 32 {
		return p, "", errPackageLimit
	}
	actions, err := edit(slices.Clone(p.ActionPackages[packIndex].Actions))
	if err != nil {
		return p, "", err
	}
	next := cloneStroke(p)
	pack := next.ActionPackages[packIndex]
	pack.Actions = actions
	if !shared {
		next.ActionPackages[packIndex] = pack
		return next, packageID, nil
	}
	pack.ID = strokeID()
	pack.Name = packageName(p, p.Scopes[scopeIndex].Name+" · "+pack.Name)
	next.ActionPackages = slices.Insert(next.ActionPackages, packIndex+1, pack)
	scope := &next.Scopes[scopeIndex]
	for i, id := range scope.Packages {
		if id == packageID {
			scope.Packages[i] = pack.ID
		}
	}
	return next, pack.ID, nil
}

func updateSceneAction(p stroke.Preferences, scopeID, packageID, actionID string, patch func(*stroke.Action)) (stroke.Preferences, string, error) {
	return editScenePackage(p, scopeID, packageID, func(actions []stroke.Action) ([]stroke.Action, error) {
		i := slices.IndexFunc(actions, func(a stroke.Action) bool { return a.ID == actionID })
		if i < 0 {
			return nil, errMissingSceneAction
		}
		patch(&actions[i])
		return actions, nil
	})
}

func removeSceneAction(p stroke.Preferences, scopeID, packageID, actionID string) (stroke.Preferences, error) {
	next, _, err := editScenePackage(p, scopeID, packageID, func(actions []stroke.Action) ([]stroke.Action, error) {
		return slices.DeleteFunc(actions, func(a stroke.Action) bool { return a.ID == actionID }), nil
	})
	return next, err
}

// addSceneAction adds an action to a scene: to its last package when no
// other scene uses that package, else to a new package at the end, which
// makes its precedence plain.
func addSceneAction(p stroke.Preferences, scopeID string, action stroke.Action) (stroke.Preferences, error) {
	scopeIndex := slices.IndexFunc(p.Scopes, func(s stroke.Scope) bool { return s.ID == scopeID })
	if scopeIndex < 0 {
		return p, errMissingSceneAction
	}
	scope := p.Scopes[scopeIndex]
	action.ID = strokeID()
	if n := len(p.ActionPackages); n > 0 {
		last := p.ActionPackages[n-1]
		own := slices.Contains(scope.Packages, last.ID) && len(last.Actions) < 256 &&
			!slices.ContainsFunc(p.Scopes, func(s stroke.Scope) bool { return s.ID != scopeID && slices.Contains(s.Packages, last.ID) })
		if own {
			next, _, err := editScenePackage(p, scopeID, last.ID, func(actions []stroke.Action) ([]stroke.Action, error) {
				return append(actions, action), nil
			})
			return next, err
		}
	}
	if len(p.ActionPackages) >= 32 {
		return p, errPackageLimit
	}
	next := cloneStroke(p)
	pack := stroke.ActionPackage{ID: strokeID(), Name: packageName(p, scope.Name), Actions: []stroke.Action{action}}
	next.ActionPackages = append(next.ActionPackages, pack)
	next.Scopes[scopeIndex].Packages = append(next.Scopes[scopeIndex].Packages, pack.ID)
	return next, nil
}
