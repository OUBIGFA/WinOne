package pdfium

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// A minimal, first-party PDF fixture exercises text, URI links and AcroForms.
// It is built with actual object/xref offsets, not a substitute for the engine.
func fixturePDF() []byte {
	stream := func(text string) string {
		return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(text), text)
	}
	appearance := func(text string) string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Form /BBox [0 0 20 20] /Resources << >> /Length %d >>\nstream\n%s\nendstream", len(text), text)
	}
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R /AcroForm 7 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 300] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R /Annots [6 0 R 8 0 R 9 0 R] >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		stream("BT /F1 18 Tf 40 250 Td (Hello PDF) Tj ET"),
		"<< /Type /Annot /Subtype /Link /Rect [40 200 200 220] /A << /S /URI /URI (https://example.invalid/pdf-link) >> >>",
		"<< /Fields [8 0 R 9 0 R] /DR << /Font << /F1 4 0 R >> >> /DA (/F1 12 Tf 0 g) /NeedAppearances true >>",
		"<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /V (Initial value) /Rect [40 140 200 165] /P 3 0 R /DA (/F1 12 Tf 0 g) >>",
		"<< /Type /Annot /Subtype /Widget /FT /Btn /T (checkbox) /V /Off /AS /Off /Rect [40 100 60 120] /P 3 0 R /AP << /N << /Off 10 0 R /Yes 11 0 R >> >> >>",
		appearance("1 1 1 rg 0 0 20 20 re f 0 0 0 RG 1 1 18 18 re S"),
		appearance("1 1 1 rg 0 0 20 20 re f 0 0 0 RG 2 w 2 2 m 18 18 l S 2 18 m 18 2 l S"),
	}
	var result bytes.Buffer
	fmt.Fprint(&result, "%PDF-1.7\n")
	offsets := []int{0}
	for i, object := range objects {
		offsets = append(offsets, result.Len())
		fmt.Fprintf(&result, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := result.Len()
	fmt.Fprintf(&result, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&result, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&result, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return result.Bytes()
}
func helper(t *testing.T) *Client {
	t.Helper()
	executable := os.Getenv("WINONE_PDF_WORKER")
	if executable == "" {
		t.Skip("explicit verified PDF helper is required")
	}
	library := filepath.Join(filepath.Dir(executable), "pdfium.dll")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	client, err := Start(ctx, executable, library)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return client
}
func call(t *testing.T, client *Client, request Request) Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, err := client.Call(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}
func TestRealPDFHelperRendersTextLinksAndFormMetadata(t *testing.T) {
	client := helper(t)
	response := call(t, client, Request{Method: "open", Data: fixturePDF()})
	if response.Count != 1 {
		t.Fatalf("page count=%d", response.Count)
	}
	info := call(t, client, Request{Method: "page", Page: 0}).Page
	if info == nil || info.Width != 300 || info.Height != 300 {
		t.Fatalf("page dimensions=%#v", info)
	}
	var text strings.Builder
	for _, character := range info.Characters {
		text.WriteString(character.Text)
	}
	if !strings.Contains(text.String(), "Hello PDF") {
		t.Fatalf("native text extraction=%q", text.String())
	}
	if len(info.Links) != 1 || info.Links[0].URL != "https://example.invalid/pdf-link" || info.Links[0].Box.Top != 80 {
		t.Fatalf("native URI/geometry=%#v", info.Links)
	}
	fields := map[string]Field{}
	for _, field := range info.Fields {
		fields[field.Name] = field
	}
	if fields["name"].Kind != FieldText || fields["name"].Value != "Initial value" || fields["checkbox"].Kind != FieldCheckbox {
		t.Fatalf("native AcroForms=%#v", fields)
	}
	rendered := call(t, client, Request{Method: "render", Page: 0, Width: 600, Height: 600})
	image, err := png.Decode(bytes.NewReader(rendered.PNG))
	if err != nil || image.Bounds().Dx() != 600 {
		t.Fatalf("native render failed: %v", err)
	}
	notWhite := false
	for y := 70; y < 120 && !notWhite; y++ {
		for x := 70; x < 300; x++ {
			r, g, b, _ := image.At(x, y).RGBA()
			if r < 0xffff || g < 0xffff || b < 0xffff {
				notWhite = true
				break
			}
		}
	}
	if !notWhite {
		t.Fatal("native render returned a blank page")
	}
}
func TestRealPDFHelperEditsFormsAndKeepsDocumentScope(t *testing.T) {
	client := helper(t)
	call(t, client, Request{Method: "open", Data: fixturePDF()})
	info := call(t, client, Request{Method: "page"}).Page
	var text, checkbox Field
	for _, field := range info.Fields {
		if field.Name == "name" {
			text = field
		}
		if field.Name == "checkbox" {
			checkbox = field
		}
	}
	call(t, client, Request{Method: "form", Page: 0, Annotation: text.Index, Value: "Changed value"})
	checked := true
	call(t, client, Request{Method: "form", Page: 0, Annotation: checkbox.Index, Checked: &checked})
	current := call(t, client, Request{Method: "page"}).Page
	values := map[string]Field{}
	for _, field := range current.Fields {
		values[field.Name] = field
	}
	if values["name"].Value != "Changed value" || !values["checkbox"].Checked {
		t.Fatalf("form updates did not reach PDFium: %#v", values)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Call(ctx, Request{Method: "render", Width: 16000, Height: 16000}); err == nil {
		t.Fatal("unbounded page allocation accepted")
	}
	if _, err := client.Call(ctx, Request{Method: "page", Page: 99}); err == nil {
		t.Fatal("out-of-document page accepted")
	}
}
func TestRealPDFHelperConcurrentCloseAndParentCancellation(t *testing.T) {
	executable := os.Getenv("WINONE_PDF_WORKER")
	if executable == "" {
		t.Skip("explicit verified PDF helper is required")
	}
	// This is the real native process and the public lifetime API. A parent
	// cancellation and several tab-close requests must terminate it once,
	// without converting Windows' pending exit into an access-denied error.
	for range 12 {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		client, err := Start(ctx, executable, filepath.Join(filepath.Dir(executable), "pdfium.dll"))
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		start := make(chan struct{})
		errors := make(chan error, 4)
		var group sync.WaitGroup
		for range 4 {
			group.Add(1)
			go func() { defer group.Done(); <-start; errors <- client.Close() }()
		}
		group.Add(1)
		go func() { defer group.Done(); <-start; cancel() }()
		close(start)
		group.Wait()
		close(errors)
		for err := range errors {
			if err != nil {
				t.Fatal("concurrent native teardown failed", err)
			}
		}
	}
}

func TestPDFHelperRejectsMalformedDocumentAndCloseIsIdempotent(t *testing.T) {
	client := helper(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Call(ctx, Request{Method: "open", Data: []byte("not a PDF")}); err == nil {
		t.Fatal("malformed PDF accepted")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}
