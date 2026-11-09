package studybook

import (
	"github.com/egoist/mygo/ui"
	"image"
	"testing"
)

func TestNativeImageMasksDrawUndoAndRemoveWithoutLosingOtherGroups(t *testing.T) {
	b, h := bookFixture(t)
	tt := ui.NewTester(b.View, 800, 2500)
	h.Settle(tt)
	b.image = &imageDraft{bitmap: ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 400, 200))), shape: "rect", OccludeInactive: true}
	tt.Frame()
	r, ok := tt.Find(text("maskCanvas"))
	if !ok || r.W < 100 {
		t.Fatalf("canvas not available: %+v", r)
	}
	tt.Press(r.X+r.W*.1, r.Y+r.H*.1)
	tt.Move(r.X+r.W*.4, r.Y+r.H*.4)
	tt.Release(r.X+r.W*.4, r.Y+r.H*.4)
	if len(b.image.Occlusions) != 1 || !b.image.dirty {
		t.Fatalf("drag did not create mask: %+v", b.image.Occlusions)
	}
	shape := b.image.Occlusions[0].Shapes[0]
	if maskNumber(shape, "width") < .29 || maskNumber(shape, "width") > .31 {
		t.Fatalf("mask width %v", maskNumber(shape, "width"))
	}
	if err := tt.Click(text("undo")); err != nil {
		t.Fatal(err)
	}
	if len(b.image.Occlusions) != 0 {
		t.Fatal("undo did not remove drawn mask")
	}
	b.image.add(shape)
	b.image.add(shape)
	tt.Frame()
	if err := tt.Click(text("remove")); err != nil {
		t.Fatal(err)
	}
	if len(b.image.Occlusions) != 1 {
		t.Fatal("remove lost or retained wrong groups")
	}
}

func TestRecordedPCMExportsValidWaveHeader(t *testing.T) {
	data := wavPCM([]byte{1, 0, 2, 0})
	if len(data) != 48 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" || string(data[36:40]) != "data" || data[40] != 4 {
		t.Fatalf("invalid WAV: %v", data)
	}
}
