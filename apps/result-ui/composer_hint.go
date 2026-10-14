package resultui

import "context"

// Claim only when this window exposes a composer. A selection toolbar alone
// must not consume the one-time instruction for the whole machine.
func (c *Controller) claimComposerHint() {
	if c.Toolbar || c.composerHintRequested || c.Client == nil {
		return
	}
	c.composerHintRequested = true
	go func() {
		var result struct {
			Show bool `json:"show"`
		}
		err := c.Client.Call(c.ctx, "results.composerHint.claim", map[string]string{"id": c.Initial.ID}, &result)
		if c.ctx.Err() != nil || err == context.Canceled {
			return
		}
		c.update(func() {
			if err != nil {
				c.Panel.Error = c.Panel.T("无法保存首次使用提示状态：", "Could not save first-use hint state: ") + err.Error()
				return
			}
			// A delayed reply must not interrupt typing that already began.
			if !c.Panel.composerEdited {
				c.Panel.ShowComposerHint = result.Show
			}
		})
	}()
}
