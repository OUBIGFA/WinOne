package chat

import (
	"encoding/json"
	"testing"

	"winone/desktopmygo/chat/model"
)

func TestASpacedLocalImagePathIsBracketed(t *testing.T) {
	cases := map[string]string{
		`![a](C:\Users\Me\My Pictures\a b.png)`: `![a](<C:/Users/Me/My Pictures/a b.png>)`,
		`![a](./out dir/shot.webp) tail`:        `![a](<./out dir/shot.webp>) tail`,
		`![a](plain.png)`:                       `![a](plain.png)`,
		`![a](https://example.com/a b.png)`:     `![a](https://example.com/a b.png)`,
		`text with no picture`:                  `text with no picture`,
	}
	for in, want := range cases {
		if got := quoteSpacedImagePaths(in); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}

func TestAPluginCardIsFoundOnlyOnAPluginCall(t *testing.T) {
	result := json.RawMessage(`{"content":[{"type":"text","text":"{\"ok\":true,\"xdt_card_id\":\"call-1\"}"}]}`)
	if id := pluginCardID(model.Tool{Name: "ghost_call", Result: result}); id != "call-1" {
		t.Fatalf("card id = %q", id)
	}
	if id := pluginCardID(model.Tool{Name: "powershell", Result: result}); id != "" {
		t.Fatalf("a shell result named a card: %q", id)
	}
}
