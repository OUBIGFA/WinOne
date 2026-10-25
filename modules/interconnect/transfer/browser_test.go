package main

import (
	"net/url"
	"testing"

	"github.com/schollz/croc/v11/src/croc"
	"github.com/schollz/croc/v11/src/models"
)

func TestBrowserReceiveLinkCapabilities(t *testing.T) {
	for _, test := range []struct {
		name      string
		public    bool
		transport croc.TransportMode
		password  string
		browser   bool
	}{
		{"automatic public", true, "auto", models.DEFAULT_PASSPHRASE, true},
		{"relay public", true, "relay", models.DEFAULT_PASSPHRASE, true},
		{"strict DERP", true, "derp", models.DEFAULT_PASSPHRASE, false},
		{"custom relay or LAN", false, "auto", models.DEFAULT_PASSPHRASE, false},
		{"custom password", true, "auto", "private", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			link := browserReceiveURL("1234-phone-share", test.public, croc.Options{Transport: test.transport, RelayPassword: test.password})
			if (link != "") != test.browser {
				t.Fatalf("unexpected browser support: %q", link)
			}
			if link != "" {
				parsed, err := url.Parse(link)
				if err != nil || parsed.Scheme != "https" || parsed.Host != "getcroc.com" || parsed.Query().Get("code") != "1234-phone-share" {
					t.Fatalf("invalid browser link: %q", link)
				}
				code, err := receiveCode(link)
				if err != nil || code != "1234-phone-share" {
					t.Fatalf("cannot receive copied browser link: %q %v", code, err)
				}
			}
		})
	}
}

func TestReceiveCodeRejectsUnrelatedURLs(t *testing.T) {
	for _, value := range []string{"http://getcroc.com/?code=1234-test", "https://getcroc.com.evil.invalid/?code=1234-test", "https://getcroc.com/", "https://user@getcroc.com/?code=1234-test", "https://getcroc.com/s/id#key"} {
		if _, err := receiveCode(value); err == nil {
			t.Errorf("accepted unrelated URL: %s", value)
		}
	}
	code, err := receiveCode("1234-phone-share")
	if err != nil || code != "1234-phone-share" {
		t.Fatalf("plain code changed: %q %v", code, err)
	}
}
