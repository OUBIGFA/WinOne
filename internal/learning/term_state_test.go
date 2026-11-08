package learning

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTermStatesPersistReplaceAndRestore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.json")
	if _, err := SetTermState(path, "  Look   up ", Focused); err != nil {
		t.Fatal(err)
	}
	states, err := SetTermState(path, "World", Ignored)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTermStates(path)
	if err != nil || !reflect.DeepEqual(states, loaded) {
		t.Fatalf("reload %v %v", loaded, err)
	}
	if got := loaded.Matching("We LOOK UP words in a worldwide dictionary.", Focused); !reflect.DeepEqual(got, []string{"look up"}) {
		t.Fatal(got)
	}
	if len(loaded.Matching("worldwide", Ignored)) != 0 {
		t.Fatal("matched part of a word")
	}
	states, err = SetTermState(path, "WORLD", Focused)
	if err != nil || states.State("world") != Focused {
		t.Fatal(states, err)
	}
	states, err = SetTermState(path, "world", "")
	if err != nil || states.State("world") != "" || states.State("look up") != Focused {
		t.Fatal(states, err)
	}
}
func TestCorruptTermStatesAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.json")
	before := []byte(`{"world":`)
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := SetTermState(path, "hello", Ignored); err == nil {
		t.Fatal("corruption ignored")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("previous data overwritten")
	}
}

func TestIgnoredVocabularyFiltersTablesButPreservesSourceAndCode(t *testing.T) {
	text := "The world is changing.\n\n| Meaning | Word |\n|---|---|\n| 世界 | **world** |\n| 改变 | changing |\n\n```markdown\n| Word | Meaning |\n|---|---|\n| world | code example |\n```"
	got := FilterIgnoredVocabulary(text, TermStates{"world": Ignored})
	if strings.Contains(got, "**world**") || !strings.Contains(got, "The world is changing.") || !strings.Contains(got, "changing |") || !strings.Contains(got, "world | code example") {
		t.Fatal(got)
	}
}
