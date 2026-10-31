package resultui

// ReadSelection answers the Host's read of the selected text, as when the
// user selects text in a result to use the selection assistant on it again.
// The read waits for the next build, which has processed the pointer and
// the keys before it: each Selectable block reports its selection
// (Element.SelectedTexts) and the input its own (Element.TextSelection),
// MyGo v0.7.3's public APIs. No clipboard copy, synthetic key press or
// accessibility polling is needed, and passwords never show here.
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
