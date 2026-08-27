package judge

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	modelImageMaxDimension = 2048
	modelImageMaxBytes     = 2 << 20
	modelImageMaxPixels    = 64_000_000
)

func prepareRequestImages(ctx context.Context, req Request) (Request, error) {
	turns := make([]Turn, len(req.Turns))
	for turnIndex, turn := range req.Turns {
		turns[turnIndex] = turn
		turns[turnIndex].Parts = make([]Part, len(turn.Parts))
		copy(turns[turnIndex].Parts, turn.Parts)
		for partIndex, part := range turns[turnIndex].Parts {
			if part.Image == nil {
				continue
			}
			prepared, err := prepareModelImage(ctx, part.Image)
			if err != nil {
				return Request{}, err
			}
			turns[turnIndex].Parts[partIndex].Image = prepared
		}
	}
	req.Turns = turns
	return req, nil
}

func prepareModelImage(ctx context.Context, source *Image) (*Image, error) {
	if source == nil || len(source.Bytes) == 0 {
		return nil, permanent(fmt.Errorf("model photograph is empty"))
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(source.Bytes))
	if err != nil {
		return nil, permanent(fmt.Errorf("decode model photograph metadata: %w", err))
	}
	media, ok := modelImageMedia(format)
	if !ok {
		return nil, permanent(fmt.Errorf("model photograph format %q is unsupported", format))
	}
	if config.Width <= 0 || config.Height <= 0 ||
		int64(config.Width)*int64(config.Height) > modelImageMaxPixels {
		return nil, permanent(fmt.Errorf("model photograph dimensions %dx%d are unsafe",
			config.Width, config.Height))
	}
	if config.Width <= modelImageMaxDimension && config.Height <= modelImageMaxDimension &&
		len(source.Bytes) <= modelImageMaxBytes {
		if source.Media == media {
			return source, nil
		}
		return &Image{Media: media, Bytes: source.Bytes}, nil
	}

	decoded, _, err := image.Decode(bytes.NewReader(source.Bytes))
	if err != nil {
		return nil, permanent(fmt.Errorf("decode model photograph: %w", err))
	}

	resized := fitImage(decoded, modelImageMaxDimension)
	orientation := 1
	if format == "jpeg" {
		orientation = jpegOrientation(source.Bytes)
	}
	oriented := orientImage(resized, orientation)
	flattened := flattenImage(oriented)

	prepared, err := encodeModelJPEG(flattened)
	if err != nil {
		return nil, permanent(err)
	}
	outputConfig, _, err := image.DecodeConfig(bytes.NewReader(prepared))
	if err != nil {
		return nil, permanent(fmt.Errorf("verify model photograph: %w", err))
	}

	out := &Image{Media: "image/jpeg", Bytes: prepared}
	trace.SpanFromContext(ctx).AddEvent("model.image.normalized", trace.WithAttributes(
		attribute.String("image.input_media", source.Media),
		attribute.String("image.output_media", out.Media),
		attribute.Int("image.input_width", config.Width),
		attribute.Int("image.input_height", config.Height),
		attribute.Int("image.output_width", outputConfig.Width),
		attribute.Int("image.output_height", outputConfig.Height),
		attribute.Int("image.input_bytes", len(source.Bytes)),
		attribute.Int("image.output_bytes", len(out.Bytes)),
	))
	return out, nil
}

func modelImageMedia(format string) (string, bool) {
	switch format {
	case "jpeg":
		return "image/jpeg", true
	case "png":
		return "image/png", true
	case "webp":
		return "image/webp", true
	default:
		return "", false
	}
}

func fitImage(source image.Image, maximum int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= maximum && height <= maximum {
		return source
	}
	if width >= height {
		height = max(1, height*maximum/width)
		width = maximum
	} else {
		width = max(1, width*maximum/height)
		height = maximum
	}

	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(out, out.Bounds(), source, bounds, xdraw.Over, nil)
	return out
}

func orientImage(source image.Image, orientation int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if orientation < 2 || orientation > 8 {
		return source
	}

	outWidth, outHeight := width, height
	if orientation >= 5 {
		outWidth, outHeight = height, width
	}
	out := image.NewNRGBA(image.Rect(0, 0, outWidth, outHeight))
	for y := 0; y < outHeight; y++ {
		for x := 0; x < outWidth; x++ {
			var sourceX, sourceY int
			switch orientation {
			case 2:
				sourceX, sourceY = width-1-x, y
			case 3:
				sourceX, sourceY = width-1-x, height-1-y
			case 4:
				sourceX, sourceY = x, height-1-y
			case 5:
				sourceX, sourceY = y, x
			case 6:
				sourceX, sourceY = y, height-1-x
			case 7:
				sourceX, sourceY = width-1-y, height-1-x
			case 8:
				sourceX, sourceY = width-1-y, x
			}
			out.Set(x, y, source.At(bounds.Min.X+sourceX, bounds.Min.Y+sourceY))
		}
	}
	return out
}

func flattenImage(source image.Image) image.Image {
	bounds := source.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(out, out.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(out, out.Bounds(), source, bounds.Min, draw.Over)
	return out
}

func encodeModelJPEG(source image.Image) ([]byte, error) {
	current := source
	for {
		for _, quality := range []int{85, 75, 65, 55} {
			var encoded bytes.Buffer
			if err := jpeg.Encode(&encoded, current, &jpeg.Options{Quality: quality}); err != nil {
				return nil, fmt.Errorf("encode model photograph: %w", err)
			}
			if encoded.Len() <= modelImageMaxBytes {
				return encoded.Bytes(), nil
			}
		}

		bounds := current.Bounds()
		maximum := max(bounds.Dx(), bounds.Dy()) * 3 / 4
		if maximum < 512 {
			return nil, fmt.Errorf("model photograph cannot fit within %d bytes", modelImageMaxBytes)
		}
		current = fitImage(current, maximum)
	}
}

func jpegOrientation(raw []byte) int {
	if len(raw) < 4 || raw[0] != 0xff || raw[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(raw); {
		if raw[offset] != 0xff {
			return 1
		}
		marker := raw[offset+1]
		offset += 2
		if marker == 0xd9 || marker == 0xda {
			return 1
		}
		if offset+2 > len(raw) {
			return 1
		}
		length := int(binary.BigEndian.Uint16(raw[offset : offset+2]))
		if length < 2 || offset+length > len(raw) {
			return 1
		}
		segment := raw[offset+2 : offset+length]
		if marker == 0xe1 && len(segment) >= 14 && bytes.Equal(segment[:6], []byte("Exif\x00\x00")) {
			if orientation := tiffOrientation(segment[6:]); orientation != 1 {
				return orientation
			}
		}
		offset += length
	}
	return 1
}

func tiffOrientation(raw []byte) int {
	if len(raw) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(raw[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(raw[2:4]) != 42 {
		return 1
	}
	ifd := int(order.Uint32(raw[4:8]))
	if ifd < 0 || ifd+2 > len(raw) {
		return 1
	}
	entries := int(order.Uint16(raw[ifd : ifd+2]))
	for index := 0; index < entries; index++ {
		offset := ifd + 2 + index*12
		if offset+12 > len(raw) {
			return 1
		}
		entry := raw[offset : offset+12]
		if order.Uint16(entry[:2]) != 0x0112 || order.Uint16(entry[2:4]) != 3 ||
			order.Uint32(entry[4:8]) != 1 {
			continue
		}
		orientation := int(order.Uint16(entry[8:10]))
		if orientation >= 1 && orientation <= 8 {
			return orientation
		}
		return 1
	}
	return 1
}
