package ocr

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"

	"github.com/TimLai666/coimnet/tasks/ocr/glyphs"
)

const (
	defaultRasterMaxEncodedBytes int64 = 16 << 20
	defaultRasterMaxPixels             = 1 << 20
	rasterSchemaVersion                = "coimnet-ocr-raster/v1"
)

// ImageOptions bounds PNG/JPEG input and optionally sets the output height.
// Zero byte/pixel limits use 16 MiB and 1,048,576 pixels respectively.
type ImageOptions struct {
	MaxEncodedBytes int64
	MaxPixels       int
	Height          int
	LightInk        bool
}

// ImageMetadata describes the verified source and stored-pixel transformation.
type ImageMetadata struct {
	SchemaVersion    string  `json:"schema_version"`
	SourceSHA256     string  `json:"source_sha256"`
	Format           string  `json:"format"`
	SourceColorSpace string  `json:"source_color_space"`
	ScalingMethod    string  `json:"scaling_method"`
	Orientation      string  `json:"orientation"`
	TextDirection    string  `json:"text_direction"`
	CoordinateSpace  string  `json:"coordinate_space"`
	SourceWidth      int     `json:"source_width"`
	SourceHeight     int     `json:"source_height"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	ScaleX           float64 `json:"scale_x"`
	ScaleY           float64 `json:"scale_y"`
	Region           Rect    `json:"region"`
}

// Raster owns a normalized ink image and metadata from the same bounded read.
type Raster struct {
	Image    glyphs.Image  `json:"image"`
	Metadata ImageMetadata `json:"metadata"`
}

// LoadImage reads PNG/JPEG with bounded decoding and returns no partial image
// on malformed input, unsupported format, exceeded limits or cancellation.
func LoadImage(ctx context.Context, path string, options ImageOptions) (Raster, error) {
	var zero Raster
	if ctx == nil {
		return zero, errors.New("ocr: context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	maxEncodedBytes := options.MaxEncodedBytes
	if maxEncodedBytes == 0 {
		maxEncodedBytes = defaultRasterMaxEncodedBytes
	}
	if maxEncodedBytes < 0 {
		return zero, errors.New("ocr: max encoded bytes cannot be negative")
	}
	maxPixels := options.MaxPixels
	if maxPixels == 0 {
		maxPixels = defaultRasterMaxPixels
	}
	if maxPixels < 0 {
		return zero, errors.New("ocr: max pixels cannot be negative")
	}
	if options.Height < 0 {
		return zero, errors.New("ocr: output height cannot be negative")
	}

	f, err := os.Open(path)
	if err != nil {
		return zero, fmt.Errorf("ocr: open image: %w", err)
	}
	defer f.Close()
	encoded, err := readRasterBytes(ctx, f, maxEncodedBytes)
	if err != nil {
		return zero, err
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil {
		return zero, fmt.Errorf("ocr: decode image config: %w", err)
	}
	if format != "png" && format != "jpeg" {
		return zero, fmt.Errorf("ocr: unsupported image format %q", format)
	}
	sourceWidth, sourceHeight := config.Width, config.Height
	sourceArea, err := rasterImageArea(sourceWidth, sourceHeight)
	if err != nil {
		return zero, err
	}
	if sourceArea > maxPixels {
		return zero, fmt.Errorf("ocr: source image has %d pixels, exceeds limit %d", sourceArea, maxPixels)
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	decoded, decodedFormat, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return zero, fmt.Errorf("ocr: decode image: %w", err)
	}
	if decodedFormat != format {
		return zero, fmt.Errorf("ocr: image format changed during decode from %q to %q", format, decodedFormat)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() != sourceWidth || bounds.Dy() != sourceHeight {
		return zero, errors.New("ocr: decoded image dimensions differ from DecodeConfig")
	}
	if _, err := rasterImageArea(bounds.Dx(), bounds.Dy()); err != nil {
		return zero, err
	}

	outputWidth, outputHeight := sourceWidth, sourceHeight
	scalingMethod := "identity"
	if options.Height > 0 {
		outputHeight = options.Height
		maxInt := int(^uint(0) >> 1)
		if sourceWidth > maxInt/outputHeight {
			return zero, errors.New("ocr: scaled image dimensions overflow")
		}
		outputWidth = sourceWidth * outputHeight / sourceHeight
		if outputWidth < 1 {
			outputWidth = 1
		}
		scalingMethod = "nearest-v1"
	}
	outputArea, err := rasterImageArea(outputWidth, outputHeight)
	if err != nil {
		return zero, err
	}
	if outputArea > maxPixels {
		return zero, fmt.Errorf("ocr: scaled image has %d pixels, exceeds limit %d", outputArea, maxPixels)
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	pixels := make([]float64, outputArea)
	for y := 0; y < outputHeight; y++ {
		if err := ctx.Err(); err != nil {
			return zero, err
		}
		sy, err := rasterNearestIndex(y, sourceHeight, outputHeight)
		if err != nil {
			return zero, err
		}
		for x := 0; x < outputWidth; x++ {
			sx, err := rasterNearestIndex(x, sourceWidth, outputWidth)
			if err != nil {
				return zero, err
			}
			pixels[y*outputWidth+x] = rasterGrayInk(decoded, bounds.Min.X+sx, bounds.Min.Y+sy, options.LightInk)
		}
	}
	if err := ctx.Err(); err != nil {
		return zero, err
	}

	sourceHash := sha256.Sum256(encoded)
	return Raster{
		Image: glyphs.Image{Width: outputWidth, Height: outputHeight, Pixels: pixels},
		Metadata: ImageMetadata{
			SchemaVersion:    rasterSchemaVersion,
			SourceSHA256:     hex.EncodeToString(sourceHash[:]),
			Format:           format,
			SourceColorSpace: "decoded-rgb-gray-v1-white-alpha",
			ScalingMethod:    scalingMethod,
			Orientation:      "stored-pixel-order",
			TextDirection:    "horizontal-left-to-right",
			CoordinateSpace:  "source-pixel-inclusive-exclusive-v1",
			SourceWidth:      sourceWidth,
			SourceHeight:     sourceHeight,
			Width:            outputWidth,
			Height:           outputHeight,
			ScaleX:           float64(outputWidth) / float64(sourceWidth),
			ScaleY:           float64(outputHeight) / float64(sourceHeight),
			Region:           Rect{X0: 0, Y0: 0, X1: sourceWidth, Y1: sourceHeight},
		},
	}, nil
}

func readRasterBytes(ctx context.Context, f *os.File, limit int64) ([]byte, error) {
	const chunkSize = 32 << 10
	var out bytes.Buffer
	chunk := make([]byte, chunkSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := f.Read(chunk)
		if n > 0 {
			if int64(n) > limit-total {
				return nil, fmt.Errorf("ocr: encoded image exceeds limit %d bytes", limit)
			}
			if _, writeErr := out.Write(chunk[:n]); writeErr != nil {
				return nil, fmt.Errorf("ocr: buffer image: %w", writeErr)
			}
			total += int64(n)
		}
		if err == io.EOF {
			return out.Bytes(), nil
		}
		if err != nil {
			return nil, fmt.Errorf("ocr: read image: %w", err)
		}
	}
}

func rasterImageArea(width, height int) (int, error) {
	if width <= 0 || height <= 0 {
		return 0, errors.New("ocr: image dimensions must be positive")
	}
	maxInt := int(^uint(0) >> 1)
	if width > maxInt/height {
		return 0, errors.New("ocr: image pixel count overflows int")
	}
	area := width * height
	if area > maxInt/8 {
		return 0, errors.New("ocr: image pixel buffer size overflows int")
	}
	return area, nil
}

func rasterNearestIndex(index, sourceSize, outputSize int) (int, error) {
	if index < 0 || sourceSize <= 0 || outputSize <= 0 || index >= outputSize {
		return 0, errors.New("ocr: invalid nearest-neighbor coordinate")
	}
	maxInt := int(^uint(0) >> 1)
	if index > maxInt/sourceSize {
		return 0, errors.New("ocr: nearest-neighbor coordinate overflows int")
	}
	position := index * sourceSize / outputSize
	if position >= sourceSize {
		position = sourceSize - 1
	}
	return position, nil
}

func rasterGrayInk(img image.Image, x, y int, lightInk bool) float64 {
	r, g, b, a := img.At(x, y).RGBA()
	const max = uint32(0xffff)
	if r > a {
		r = a
	}
	if g > a {
		g = a
	}
	if b > a {
		b = a
	}
	r += max - a
	g += max - a
	b += max - a
	gray := color.GrayModel.Convert(color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: uint16(max)}).(color.Gray)
	intensity := float64(gray.Y) / 255
	if lightInk {
		return intensity
	}
	return 1 - intensity
}
