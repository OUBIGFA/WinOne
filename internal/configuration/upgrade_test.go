package configuration

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestUpgradeCheckUsesValidatorsWithoutLifecycleEffects(t *testing.T) {
	adapter := ModuleAdapter{ID: "capture", Validate: func(raw json.RawMessage) error {
		if string(raw) != `{"valid":true}` {
			return errors.New("unsupported module settings")
		}
		return nil
	}, Load: func() (json.RawMessage, error) { t.Fatal("legacy reader called"); return nil, nil },
		Apply: func(json.RawMessage, uint64) error { t.Fatal("runtime apply called"); return nil }}
	valid := []byte(`{"committed":{"schemaVersion":1,"revision":3,"moduleEnabled":{"capture":false},"values":{"host":{"schemaVersion":1,"revision":0,"theme":"system","language":"zh-CN","captureShortcut":"","pinShortcut":""},"capture":{"valid":true}}}}`)
	if err := CheckUpgrade(valid, adapter); err != nil {
		t.Fatal(err)
	}
	var state diskState
	json.Unmarshal(valid, &state)
	cases := []func(*diskState){
		func(s *diskState) { next := clone(s.Committed); next.Revision++; s.Pending = &next },
		func(s *diskState) { delete(s.Committed.Values, "capture") },
		func(s *diskState) { s.Committed.Values["capture"] = json.RawMessage(`{"valid":false}`) },
		func(s *diskState) { s.Committed.SchemaVersion = 2 },
		func(s *diskState) { s.Committed.ModuleEnabled = nil },
	}
	for i, mutate := range cases {
		var next diskState
		json.Unmarshal(valid, &next)
		mutate(&next)
		raw, _ := json.Marshal(next)
		if err := CheckUpgrade(raw, adapter); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
}
