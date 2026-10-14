package ai

import (
	"encoding/json"
	"testing"
)

func TestAccountIconsPreservePublicAccountMetadata(t *testing.T) {
	output, err := WithAccountIcons("accounts.list", "cursor", json.RawMessage(`{"accounts":[{"agent":"cursor","user":"example@invalid.test","plan":"Pro","lapsed":"expired","extra":7}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Accounts []struct {
			Agent, User, Plan, Lapsed, Icon string
			Extra                           int
		}
	}
	if err = json.Unmarshal(output, &decoded); err != nil {
		t.Fatal(err)
	}
	row := decoded.Accounts[0]
	if row.Icon != "cursor" || row.User != "example@invalid.test" || row.Plan != "Pro" || row.Lapsed != "expired" || row.Extra != 7 {
		t.Fatalf("account metadata changed: %+v", row)
	}
	catalog, err := WithAccountIcons("accounts.catalog", "", json.RawMessage(`{"agents":[{"id":"qoder-cn","name":"Qoder CN"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var listing struct {
		Agents []struct{ ID, Name, Icon string }
	}
	_ = json.Unmarshal(catalog, &listing)
	if listing.Agents[0].Icon != "qoder-color" || listing.Agents[0].Name != "Qoder CN" {
		t.Fatal("sign-in catalog lost its label or icon")
	}
}
