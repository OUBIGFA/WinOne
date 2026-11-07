package webui

import "testing"

func TestHTMLPreviewOnlyAcceptsTheHostDocumentCapability(t *testing.T) {
	for _, address := range []string{"https://127.0.0.1:42/grant/a.html", "http://localhost:42/grant/a.html", "http://127.0.0.1:42/", "http://127.0.0.1:42/grant/a.html?other=grant", "http://user@127.0.0.1:42/grant/a.html", "file:///C:/a.html", "http://127.0.0.1:42/grant/%5ca.html"} {
		if _, err := htmlPreviewURL(address); err == nil {
			t.Fatalf("accepted %q", address)
		}
	}
	if _, err := htmlPreviewURL("http://127.0.0.1:42/document-grant/a%20b.html#heading"); err != nil {
		t.Fatal(err)
	}
}
