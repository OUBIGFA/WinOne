package ai

import "time"

// The Host's outer deadline includes drawing and fetching an image URL, as
// Magpie's image tool does. Each phase can use the configured image timeout;
// the extra minute only covers transport/shutdown, not another vendor attempt.
// Other requests retain their existing 10-minute ceiling.
func (s *MagpieSettings) GatewayWait(path string) time.Duration {
	if path != "/v1/images/generations" && path != "/v1/images/edits" {
		return 10 * time.Minute
	}
	minutes := 5
	if s != nil && s.ImageTimeout > 0 {
		minutes = min(60, s.ImageTimeout)
	}
	return 2*time.Duration(minutes)*time.Minute + time.Minute
}
