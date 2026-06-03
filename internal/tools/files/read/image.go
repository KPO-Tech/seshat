package read

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/disintegration/imaging"
)

// Image processing constants
const (
	// DefaultMaxImageDimension is the default maximum width/height
	DefaultMaxImageDimension = 2048

	// DefaultThumbnailDimension is the default thumbnail size
	DefaultThumbnailDimension = 512

	// TokenEstimationFactor is the factor to estimate tokens from base64 length
	// base64_length * 0.125 ≈ token count
	TokenEstimationFactor = 0.125

	// DefaultMaxImageTokens is the default maximum tokens for images
	DefaultMaxImageTokens = 4096
)

// ImageProcessingResult represents the result of image processing
type ImageProcessingResult struct {
	Base64          string           `json:"base64"`
	MimeType        string           `json:"mime_type"`
	OriginalSize    int64            `json:"original_size"`
	ProcessedSize   int64            `json:"processed_size"`
	Dimensions      *ImageDimensions `json:"dimensions,omitempty"`
	WasResized      bool             `json:"was_resized"`
	WasCompressed   bool             `json:"was_compressed"`
	EstimatedTokens int              `json:"estimated_tokens"`
}

// ReadAndProcessImage reads and processes an image file
func ReadAndProcessImage(filePath string, maxTokens int) (*ImageProcessingResult, error) {
	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read image: %w", err)
	}

	originalSize := int64(len(data))

	// Detect format
	mimeType := GetImageMimeType(filePath)
	if mimeType == "" {
		return nil, fmt.Errorf("unsupported image format")
	}

	// Decode to get dimensions and validate
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	dimensions := &ImageDimensions{
		Width:  img.Bounds().Dx(),
		Height: img.Bounds().Dy(),
	}

	// Process image (resize/compress based on token budget)
	result, err := processImageWithTokenBudget(img, format, mimeType, data, maxTokens, dimensions, originalSize)
	if err != nil {
		return nil, fmt.Errorf("failed to process image: %w", err)
	}

	result.OriginalSize = originalSize

	return result, nil
}

// processImageWithTokenBudget processes image with token budget consideration
func processImageWithTokenBudget(
	img image.Image,
	format string,
	mimeType string,
	originalData []byte,
	maxTokens int,
	dimensions *ImageDimensions,
	originalSize int64,
) (*ImageProcessingResult, error) {
	result := &ImageProcessingResult{
		MimeType:   mimeType,
		Dimensions: dimensions,
	}

	// Try standard resize first
	resizedImg, wasResized := maybeResizeImage(img, dimensions)
	result.WasResized = wasResized

	// Encode resized image
	encodedData, err := encodeImage(resizedImg, format)
	if err != nil {
		// Fallback to original data
		encodedData = originalData
		result.WasResized = false
	}

	// Estimate tokens
	base64Str := base64.StdEncoding.EncodeToString(encodedData)
	estimatedTokens := int(float64(len(base64Str)) * TokenEstimationFactor)
	result.EstimatedTokens = estimatedTokens

	// Check if it fits token budget
	if estimatedTokens <= maxTokens {
		result.Base64 = base64Str
		result.ProcessedSize = int64(len(encodedData))
		return result, nil
	}

	// Too large - aggressive compression
	compressed, compressedTokens, err := compressImageToFitTokenBudget(
		resizedImg,
		format,
		maxTokens,
		dimensions,
	)
	if err != nil {
		// Fallback: return original even if it exceeds budget
		result.Base64 = base64Str
		result.ProcessedSize = int64(len(encodedData))
		result.WasCompressed = false
		return result, nil
	}

	result.Base64 = compressed
	result.EstimatedTokens = compressedTokens
	result.ProcessedSize = int64(len(compressed) / 4 * 3) // Approximate decoded size
	result.WasCompressed = true

	return result, nil
}

// maybeResizeImage resizes image if it's too large
func maybeResizeImage(img image.Image, dimensions *ImageDimensions) (image.Image, bool) {
	maxDim := dimensions.Width
	if dimensions.Height > maxDim {
		maxDim = dimensions.Height
	}

	// No resize needed
	if maxDim <= DefaultMaxImageDimension {
		return img, false
	}

	// Calculate new dimensions maintaining aspect ratio
	var width, height int
	if dimensions.Width > dimensions.Height {
		width = DefaultMaxImageDimension
		height = int(float64(dimensions.Height) * float64(DefaultMaxImageDimension) / float64(dimensions.Width))
	} else {
		height = DefaultMaxImageDimension
		width = int(float64(dimensions.Width) * float64(DefaultMaxImageDimension) / float64(dimensions.Height))
	}

	// Resize using high-quality resampling
	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	return resized, true
}

// compressImageToFitTokenBudget aggressively compresses image to fit token budget
func compressImageToFitTokenBudget(
	img image.Image,
	format string,
	maxTokens int,
	dimensions *ImageDimensions,
) (string, int, error) {
	// Try progressively smaller sizes
	sizes := []int{
		DefaultThumbnailDimension,
		512,
		256,
		128,
	}

	for _, size := range sizes {
		// Skip if current dimensions are already smaller
		if dimensions.Width <= size && dimensions.Height <= size {
			continue
		}

		// Resize
		var resized image.Image
		if dimensions.Width > dimensions.Height {
			width := size
			height := int(float64(dimensions.Height) * float64(size) / float64(dimensions.Width))
			resized = imaging.Resize(img, width, height, imaging.Lanczos)
		} else {
			height := size
			width := int(float64(dimensions.Width) * float64(size) / float64(dimensions.Height))
			resized = imaging.Resize(img, width, height, imaging.Lanczos)
		}

		// Encode
		encodedData, err := encodeImage(resized, format)
		if err != nil {
			continue
		}

		// Check tokens
		base64Str := base64.StdEncoding.EncodeToString(encodedData)
		estimatedTokens := int(float64(len(base64Str)) * TokenEstimationFactor)

		if estimatedTokens <= maxTokens {
			return base64Str, estimatedTokens, nil
		}
	}

	// Last resort: return smallest size even if it exceeds budget
	smallest := sizes[len(sizes)-1]
	var resized image.Image
	if dimensions.Width > dimensions.Height {
		width := smallest
		height := int(float64(dimensions.Height) * float64(smallest) / float64(dimensions.Width))
		resized = imaging.Resize(img, width, height, imaging.Lanczos)
	} else {
		height := smallest
		width := int(float64(dimensions.Width) * float64(smallest) / float64(dimensions.Height))
		resized = imaging.Resize(img, width, height, imaging.Lanczos)
	}

	encodedData, err := encodeImage(resized, format)
	if err != nil {
		return "", 0, err
	}

	base64Str := base64.StdEncoding.EncodeToString(encodedData)
	estimatedTokens := int(float64(len(base64Str)) * TokenEstimationFactor)

	return base64Str, estimatedTokens, nil
}

// encodeImage encodes an image to the specified format
func encodeImage(img image.Image, format string) ([]byte, error) {
	var buf bytes.Buffer

	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		err := imaging.Encode(&buf, img, imaging.JPEG)
		if err != nil {
			return nil, err
		}
	case "png":
		err := imaging.Encode(&buf, img, imaging.PNG)
		if err != nil {
			return nil, err
		}
	default:
		// Default to JPEG
		err := imaging.Encode(&buf, img, imaging.JPEG)
		if err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// EstimateImageTokens estimates token count from image data
func EstimateImageTokens(imageData []byte) int {
	base64Len := base64.StdEncoding.EncodedLen(len(imageData))
	return int(float64(base64Len) * TokenEstimationFactor)
}

// GetImageDimensions extracts dimensions from image data
func GetImageDimensions(imageData []byte) (*ImageDimensions, error) {
	img, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return nil, err
	}

	return &ImageDimensions{
		Width:  img.Bounds().Dx(),
		Height: img.Bounds().Dy(),
	}, nil
}
