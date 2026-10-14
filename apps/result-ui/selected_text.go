package resultui

// Queue the read until the next layout has processed the text control's input.
// No clipboard copy, synthetic key press or accessibility polling is needed.
func (c *Controller) ReadSelection(request string) {
	c.selectionRead = request
	if c.Toolbar {
		c.selectedText = ""
		c.completeSelectionRead()
		return
	}
	c.Window.Invalidate()
}
func (c *Controller) completeSelectionRead() {
	if c.selectionRead == "" {
		return
	}
	request, text := c.selectionRead, c.selectedText
	c.selectionRead = ""
	if len(text) > 65536 {
		text = ""
	}
	go func() {
		_ = c.Client.Call(c.ctx, "results.selected", map[string]string{"id": c.Initial.ID, "request": request, "text": text}, nil)
	}()
}

func ForegroundResultWindow() uintptr {
	h, _, _ := frameUser.NewProc("GetForegroundWindow").Call()
	return h
}
