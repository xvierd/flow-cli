package notification

import (
	"bytes"
	"image/png"
	"testing"
)

func TestIconPNGIsValid(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		t.Fatalf("embedded icon is not a PNG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != b.Dy() || b.Dx() < 128 {
		t.Fatalf("icon should be square and at least 128px, got %v", b)
	}
}
