package resultui

import "github.com/egoist/mygo/ui"

// Preserve the complete brand asset, including its original proportions and
// whitespace. Only the enclosing control has a circular shape.
func toolbarLogo() *ui.Bitmap {
	if value, ok := iconCache.Load("bitmap/toolbar-logo"); ok {
		return value.(*ui.Bitmap)
	}
	bitmap, err := ui.DecodeBitmap(Logo())
	if err != nil {
		panic(err)
	}
	value, _ := iconCache.LoadOrStore("bitmap/toolbar-logo", bitmap)
	return value.(*ui.Bitmap)
}
