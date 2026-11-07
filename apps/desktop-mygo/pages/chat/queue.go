package chat

import "github.com/egoist/mygo/ui"

// The native queue keeps Cindy's interaction lock while dragging. Pointer and
// keyboard reordering share the same Host insertion-index contract, so a live
// turn cannot drain the row while its destination is being chosen.
type queueUI struct {
	rows   map[string]ui.Rect
	order  []string
	drag   *queueDrag
	scroll ui.ScrollState
}
type queueDrag struct {
	id           string
	from, target int
	y            float32
	moved        bool
}

func (p *Chat) queueDragInput(id string, index int) func(ui.InputEvent) bool {
	return func(event ui.InputEvent) bool {
		if event.Button != 0 {
			return false
		}
		s := &p.ui.queue
		switch event.Kind {
		case ui.InputPointerDown:
			s.drag = &queueDrag{id: id, from: index, target: index, y: s.rows[id].Y + event.Y}
			p.queueAction("interaction", "", map[string]any{"locked": true}, p.showError)
			return true
		case ui.InputPointerMove:
			if s.drag == nil || s.drag.id != id {
				return false
			}
			y := s.rows[id].Y + event.Y
			if y == s.drag.y {
				return true
			}
			s.drag.moved, s.drag.target = true, len(s.order)
			for position, rowID := range s.order {
				if row, ok := s.rows[rowID]; ok && y < row.Y+row.H/2 {
					s.drag.target = position
					break
				}
			}
			return true
		case ui.InputPointerUp:
			if s.drag == nil || s.drag.id != id {
				return false
			}
			drag := s.drag
			s.drag = nil
			// System capture loss must roll back the gesture while releasing
			// the existing Host interaction lock in its original serial order.
			if !event.Canceled && drag.moved && drag.target != drag.from && drag.target != drag.from+1 {
				p.queueAction("reorder", id, map[string]any{"targetIndex": drag.target}, p.showError)
			}
			p.queueAction("interaction", "", map[string]any{"locked": false}, p.showError)
			return true
		}
		return false
	}
}

func (p *Chat) queueReorderKey(handle ui.Element, id string, index, count int) {
	target := -1
	switch {
	case handle.Shortcut(0, ui.KeyUp) && index > 0:
		target = index - 1
	case handle.Shortcut(0, ui.KeyDown) && index < count-1:
		target = index + 2
	case handle.Shortcut(0, ui.KeyHome) && index > 0:
		target = 0
	case handle.Shortcut(0, ui.KeyEnd) && index < count-1:
		target = count
	}
	if target >= 0 {
		p.queueAction("reorder", id, map[string]any{"targetIndex": target}, p.showError)
	}
}
