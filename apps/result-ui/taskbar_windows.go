package resultui

import "errors"

// Selection windows keep the same tool-window identity while switching from
// the action strip to results. This runs before their first visible frame.
func prepareSelectionToolWindow(window uintptr) error {
	// WS_EX_TOOLWINDOW excludes the HWND from taskbar/Alt+Tab before Show.
	// MyGo's SkipTaskbar alone deletes a button after Explorer has created it.
	// Preserve all other extended styles, including activation and topmost.
	index := ^uintptr(19) // GWL_EXSTYLE (-20)
	style, _, _ := frameStyle.Call(window, index)
	wanted := (style | 0x80) &^ 0x40000
	frameUser.NewProc("SetWindowLongPtrW").Call(window, index, wanted)
	actual, _, _ := frameStyle.Call(window, index)
	if actual != wanted {
		return errors.New("无法设置划词工具窗口样式")
	}
	if ok, _, _ := frameUser.NewProc("SetWindowPos").Call(window, 0, 0, 0, 0, 0, 0x37); ok == 0 {
		return errors.New("无法刷新划词工具窗口样式")
	}
	return nil
}

func (c *Controller) SkipTaskbar() bool {
	return c.Toolbar || c.Initial.Source == "selection"
}

func (c *Controller) PrepareFrame() error {
	if c.SkipTaskbar() {
		if err := prepareSelectionToolWindow(c.Window.NativeHandle()); err != nil {
			return err
		}
	}
	if err := PrepareWindowFrame(c.Window.NativeHandle(), c.Panel.Theme); err != nil {
		return err
	}
	if !c.Toolbar {
		return fitResultWindow(c.Window.NativeHandle(), 0)
	}
	return nil
}
