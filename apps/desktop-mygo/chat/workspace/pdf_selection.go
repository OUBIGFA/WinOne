package workspace

import (
	"github.com/egoist/mygo/ui"
	"math"
	"strings"
	"winone/desktopmygo/chat/pdfium"
)

type pdfTextMatch struct{ start, end int }

func findPDFText(page pdfium.Page, query string) []pdfTextMatch {
	query = strings.ToLower(query)
	if query == "" {
		return nil
	}
	var content strings.Builder
	charForByte := []int{}
	for index, character := range page.Characters {
		lower := strings.ToLower(character.Text)
		content.WriteString(lower)
		for range len(lower) {
			charForByte = append(charForByte, index)
		}
	}
	text := content.String()
	matches := []pdfTextMatch{}
	for offset := 0; offset < len(text); {
		index := strings.Index(text[offset:], query)
		if index < 0 {
			break
		}
		index += offset
		matches = append(matches, pdfTextMatch{charForByte[index], charForByte[index+len(query)-1]})
		if len(matches) >= 2000 {
			break
		}
		offset = index + len(query)
	}
	return matches
}

// Coalesce neighbouring character bounds before painting a PDF selection.
// The actual coordinates stay PDFium's; merging only reduces draw calls for
// a selected paragraph instead of adding one shape for every glyph.
func (p *PDFPane) selectionRects(bounds ui.Rect) []ui.Rect {
	if p.anchor < 0 || p.selectionEnd < 0 {
		return nil
	}
	first, last := min(p.anchor, p.selectionEnd), max(p.anchor, p.selectionEnd)
	result := []ui.Rect{}
	for i := max(0, first); i <= last && i < len(p.page.Characters); i++ {
		r := p.box(p.page.Characters[i].Box, bounds)
		if r.W <= 0 || r.H <= 0 {
			continue
		}
		if len(result) > 0 {
			previous := &result[len(result)-1]
			if math.Abs(float64(previous.Y-r.Y)) < 1.5 && math.Abs(float64(previous.H-r.H)) < 1.5 && r.X <= previous.X+previous.W+3 && r.X+r.W >= previous.X-3 {
				right := max(previous.X+previous.W, r.X+r.W)
				previous.X = min(previous.X, r.X)
				previous.W = right - previous.X
				continue
			}
		}
		result = append(result, r)
	}
	return result
}
