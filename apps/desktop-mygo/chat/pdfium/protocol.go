package pdfium

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"runtime"
)

// Request is private to one PDF helper and holds no filesystem path. Document
// bytes are supplied by the authenticated Host preview URL, not by PDF actions.
type Request struct {
	ID                                        uint64
	Method                                    string
	Data                                      []byte
	Password                                  string
	Page, Width, Height, Rotation, Annotation int
	Value                                     string
	Checked                                   *bool
	Option                                    *int
}
type Response struct {
	ID    uint64
	Error string
	Count int
	Page  *Page
	PNG   []byte
}

// Serve confines all PDFium calls to this helper's one OS thread. EOF closes
// the document and DLL. A native crash terminates only this helper process.
func Serve(input io.Reader, output io.Writer, library string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	engine, err := Load(library)
	if err != nil {
		return err
	}
	defer engine.Close()
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64<<10), 180<<20)
	encoder := json.NewEncoder(output)
	if err := encoder.Encode(map[string]any{"ready": true}); err != nil {
		return err
	}
	for scanner.Scan() {
		var request Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			return errors.New("invalid PDF helper request")
		}
		response := Response{ID: request.ID}
		switch request.Method {
		case "open":
			response.Count, err = engine.Open(request.Data, request.Password)
		case "page":
			var page Page
			page, err = engine.Page(request.Page)
			if err == nil {
				response.Page = &page
			}
		case "render":
			image, renderErr := engine.Render(request.Page, request.Width, request.Height, request.Rotation)
			err = renderErr
			if err == nil {
				var buffer bytes.Buffer
				err = png.Encode(&buffer, image)
				if err == nil {
					response.PNG = buffer.Bytes()
				}
			}
		case "form":
			err = engine.SetField(request.Page, request.Annotation, request.Value, request.Checked, request.Option)
		case "close":
			return nil
		default:
			err = errors.New("unknown PDF helper method")
		}
		if err != nil {
			response.Error = err.Error()
		}
		if err = encoder.Encode(response); err != nil {
			return err
		}
	}
	return scanner.Err()
}
