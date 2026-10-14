package resultui

// Activation no longer waits for a persistence transaction: closed Quick
// windows retain no content, and still-open windows keep their current draft.
func (c *Controller) KeepOpen() {}
func (c *Controller) CloseWindow() {
	if c.Initial.Source == "quick-assistant" {
		c.clearAll()
	}
	if c.Window != nil {
		c.Window.Close()
	}
}
