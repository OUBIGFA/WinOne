//go:build windows

package chat

// ActiveRequest is used only to bind Host-owned automation tools to the turn
// currently accepted by this conversation, including across queued turns.
func (s *Service) ActiveRequest(id string) string {
	p, err := s.session(id)
	if err != nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running {
		return ""
	}
	return p.activeRequest
}
