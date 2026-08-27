package judge

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func testImage(t *testing.T, format string) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.SetRGBA(x, y, color.RGBA{R: 40, G: 180, B: 70, A: 255})
		}
	}
	var raw bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&raw, img)
	case "jpeg":
		err = jpeg.Encode(&raw, img, &jpeg.Options{Quality: 85})
	default:
		t.Fatalf("unknown test image format %q", format)
	}
	if err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func TestSmallModelImageIsNotReencoded(t *testing.T) {
	source := &Image{Media: "image/png", Bytes: testImage(t, "png")}

	prepared, err := prepareModelImage(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if prepared != source {
		t.Fatal("a photograph already inside the model envelope was reencoded")
	}
}

func TestLargeModelImageIsOrientedAndBounded(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 3000, 1500))
	for y := 0; y < source.Bounds().Dy(); y++ {
		for x := 0; x < source.Bounds().Dx(); x++ {
			source.SetRGBA(x, y, color.RGBA{R: uint8(x % 255), G: 140, B: uint8(y % 255), A: 255})
		}
	}

	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	raw := jpegWithOrientation(encoded.Bytes(), 6)
	if got := jpegOrientation(raw); got != 6 {
		t.Fatalf("orientation = %d, want 6", got)
	}

	prepared, err := prepareModelImage(context.Background(), &Image{Media: "image/jpeg", Bytes: raw})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Media != "image/jpeg" {
		t.Fatalf("media = %q", prepared.Media)
	}
	if len(prepared.Bytes) > modelImageMaxBytes {
		t.Fatalf("prepared image is %d bytes, limit is %d", len(prepared.Bytes), modelImageMaxBytes)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(prepared.Bytes))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Fatalf("format = %q", format)
	}
	if config.Width != 1024 || config.Height != 2048 {
		t.Fatalf("dimensions = %dx%d, want 1024x2048", config.Width, config.Height)
	}
}

func jpegWithOrientation(raw []byte, orientation byte) []byte {
	app1 := []byte{
		0xff, 0xe1, 0x00, 0x22,
		'E', 'x', 'i', 'f', 0x00, 0x00,
		'M', 'M', 0x00, 0x2a, 0x00, 0x00, 0x00, 0x08,
		0x00, 0x01,
		0x01, 0x12, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, orientation, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}
	out := make([]byte, 0, len(raw)+len(app1))
	out = append(out, raw[:2]...)
	out = append(out, app1...)
	return append(out, raw[2:]...)
}
