//go:build windows

package ai

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchKeyListPersistsPrivatelyWithOrderAndDisabledKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ai.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	c := s.Configuration()
	if err := json.Unmarshal([]byte(`{"searchAPIs":[{"vendor":"tavily","keyList":[{"id":"second","key":"fixture-two","off":true},{"id":"first","key":"fixture-one"}]}]}`), &c.MagpieSettings); err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(saved)
	disk, _ := os.ReadFile(path)
	if strings.Contains(string(data)+string(disk), "fixture-") {
		t.Fatal("plaintext search credentials escaped")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := reopened.MagpieRuntime()
	if err != nil {
		t.Fatal(err)
	}
	data, _ = json.Marshal(runtime)
	if !strings.Contains(string(data), `"key":"fixture-one"`) || !strings.Contains(string(data), `"key":"fixture-two","off":true`) {
		t.Fatal("runtime lost ordered or disabled keys")
	}
	// Saving the public snapshot must preserve the credentials unchanged.
	if _, err = reopened.SaveConfiguration(reopened.Configuration(), nil); err != nil {
		t.Fatal(err)
	}
	got, err := reopened.RevealSearchKey("tavily/first")
	if err != nil || got != "fixture-one" {
		t.Fatal("individual reveal did not preserve the selected key", err)
	}
	c = reopened.Configuration()
	// Adding an existing disabled key must keep its original slot and state,
	// including when the earlier slot's plaintext is no longer in the draft.
	*c.MagpieSettings.SearchAPIs[0].KeyList = append(*c.MagpieSettings.SearchAPIs[0].KeyList, SearchKey{ID: "repeat", Key: "fixture-two"})
	duplicateSaved, err := reopened.SaveConfiguration(c, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := *duplicateSaved.MagpieSettings.SearchAPIs[0].KeyList
	if len(list) != 2 || list[0].ID != "second" || !list[0].Off {
		t.Fatal("repeated key changed the original slot or enabled it")
	}
	c = reopened.Configuration()
	keys := []SearchKey{}
	c.MagpieSettings.SearchAPIs[0].KeyList = &keys
	if _, err = reopened.SaveConfiguration(c, nil); err != nil {
		t.Fatal("cannot remove last search key", err)
	}
	if _, err = reopened.RevealSearchKey("tavily/first"); err == nil {
		t.Fatal("removed key remains revealable")
	}
}
