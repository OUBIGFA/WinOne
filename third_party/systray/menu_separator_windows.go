//go:build windows

package systray

import "sync"

// MenuSeparator reserves its insertion order even while hidden. This lets a
// dynamic group add/remove its divider without rebuilding the fixed footer,
// including in high-contrast mode where native menus are not owner-drawn.
type MenuSeparator struct {
	mu sync.Mutex
	id uint32
	visible bool
}

// NewMenuSeparator starts hidden. Reserve it before creating the fixed rows;
// collected entries added with AddMenuItemFirst remain above it.
func NewMenuSeparator() *MenuSeparator { return &MenuSeparator{id: currentID.Add(1)} }

func (s *MenuSeparator) SetVisible(visible bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.visible == visible { return nil }
	var err error
	if visible { err = wt.addSeparatorMenuItem(s.id, 0) } else { err = wt.removeMenuItem(s.id, 0) }
	if err == nil { s.visible = visible }
	return err
}
