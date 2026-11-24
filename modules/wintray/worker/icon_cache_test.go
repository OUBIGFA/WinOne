package worker

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestCachedPNGEncodesEachImageOnceAndReleasesRemovedOnes(t *testing.T) {
	first := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	first.Set(1, 1, color.NRGBA{R: 255, A: 255})
	second := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	second.Set(2, 2, color.NRGBA{G: 255, A: 255})

	poll := func(previous map[*image.NRGBA][]byte, images ...*image.NRGBA) (map[*image.NRGBA][]byte, [][]byte) {
		next := map[*image.NRGBA][]byte{}
		var out [][]byte
		for _, img := range images {
			data, err := cachedPNG(previous, next, img)
			if err != nil {
				t.Fatal(err)
			}
			out = append(out, data)
		}
		return next, out
	}

	cache, out := poll(nil, first, second, nil)
	if out[2] != nil {
		t.Fatal("an icon without an image must stay without image data")
	}
	for i, img := range []*image.NRGBA{first, second} {
		decoded, err := png.Decode(bytes.NewReader(out[i]))
		if err != nil {
			t.Fatal(err)
		}
		if decoded.At(i+1, i+1) != img.At(i+1, i+1) {
			t.Fatalf("image %d was not encoded faithfully", i)
		}
	}

	// An unchanged icon reuses the previous encoding instead of re-encoding.
	again, repeated := poll(cache, first)
	if &repeated[0][0] != &out[0][0] {
		t.Fatal("unchanged image was encoded again")
	}
	if _, kept := again[second]; kept || len(again) != 1 {
		t.Fatalf("encodings of icons no longer shown were retained: %d entries", len(again))
	}

	// A program setting a new icon publishes a new image, which is encoded.
	replaced := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	_, fresh := poll(again, replaced)
	if &fresh[0][0] == &out[0][0] {
		t.Fatal("a replaced image reused a stale encoding")
	}
	if allocs := testing.AllocsPerRun(20, func() {
		next := make(map[*image.NRGBA][]byte, 1)
		if _, err := cachedPNG(again, next, first); err != nil {
			t.Fatal(err)
		}
	}); allocs > 2 {
		t.Fatalf("cached poll allocated %.0f objects; PNG encoding must not run again", allocs)
	}
}
