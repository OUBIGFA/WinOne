package interconnect

import "testing"

func TestCloudDownloadLinkRejectsShellTargets(t *testing.T) {
	for _, link := range []string{"https://gofile.io/d/ABC123", " https://GOFILE.io/d/Ab12Cd34 "} {
		if _, err := cloudDownloadLink(link); err != nil {
			t.Fatal(err)
		}
	}
	for _, link := range []string{"http://gofile.io/d/ABC123", "https://gofile.io.evil.test/d/ABC123", "https://gofile.io@evil.test/d/ABC123", "file:///C:/example", "https://gofile.io/d/ABC123?token=private", "https://gofile.io/d/ABC123#secret", "https://gofile.io/d/../../accounts"} {
		if _, err := cloudDownloadLink(link); err == nil {
			t.Fatalf("unsafe link accepted: %s", link)
		}
	}
}
