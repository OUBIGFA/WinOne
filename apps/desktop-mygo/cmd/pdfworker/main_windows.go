//go:build windows

// The PDF helper has no window, account, shell or network API. The owning MyGo
// page starts it on demand with the bundled library, feeds document bytes and
// closes it with that page; it is never a resident service.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"winone/desktopmygo/chat/pdfium"
)

func main() {
	library := flag.String("library", "", "absolute path to the reviewed PDFium DLL")
	flag.Parse()
	if !filepath.IsAbs(*library) {
		fmt.Fprintln(os.Stderr, "an absolute PDFium library path is required")
		os.Exit(1)
	}
	if err := pdfium.Serve(os.Stdin, os.Stdout, *library); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
