package aiservices

import (
	"reflect"
	"testing"
)

func TestHeadersFollowMagpie(t *testing.T) {
	got := headersOf([]headerRow{{"X-Team", "first", false}, {"x-team", " last ", false}, {"Empty", "", false},
		{"Version", "2.0", false}, {"X-JSON", "{\n \"mode\": \"on\"\n}", true}})
	want := map[string]string{"x-team": "last", "Version": "2.0", "X-JSON": `{"mode":"on"}`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%v", got)
	}
	if prettyJSON(`{"a":1}`) != "{\n  \"a\": 1\n}" {
		t.Fatal(prettyJSON(`{"a":1}`))
	}
}

func TestProxyLoginRoundTrips(t *testing.T) {
	value := "socks5://na%40me:p%3Ass@127.0.0.1:1080"
	f := proxyFields(value)
	if f.Address != "socks5://127.0.0.1:1080" || f.Username != "na@me" || f.Password != "p:ss" {
		t.Fatalf("%+v", f)
	}
	if got, ok := proxyAddress(f.Address, f, f.Username, f.Password); !ok || got != value {
		t.Fatal(got)
	}
	// An escape that is not UTF-8 stays until its field is edited.
	f = proxyFields("http://user:%FF@proxy.invalid:8080")
	if got, _ := proxyAddress(f.Address, f, f.Username, f.Password); got != "http://user:%FF@proxy.invalid:8080" {
		t.Fatal(got)
	}
	if got, _ := proxyAddress(f.Address, f, "new@user", "new pass"); got != "http://new%40user:new%20pass@proxy.invalid:8080" {
		t.Fatal(got)
	}
}
