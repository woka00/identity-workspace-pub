package application

import (
	"encoding/base64"
	"strings"
)

const (
	maxSignatureBytes      = 500_000
	maxSignatureDimension  = 2048
	maxSignaturePixelCount = 1_500_000
)

func validateSignatureDataURL(dataURL string) error {
	if dataURL == "" {
		return nil
	}
	const header = "data:image/png;base64,"
	if !strings.HasPrefix(strings.ToLower(dataURL), header) {
		return invalidf("signature must be a PNG base64 image")
	}
	encoded := dataURL[len(header):]
	if len(encoded) > base64.StdEncoding.EncodedLen(maxSignatureBytes)+4 {
		return invalidf("signature is too large")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return invalidf("signature base64 is invalid")
	}
	if len(decoded) == 0 || len(decoded) > maxSignatureBytes {
		return invalidf("signature must be 1..%d bytes", maxSignatureBytes)
	}
	width, height, err := photoDimensions(decoded, "image/png")
	if err != nil {
		return invalidf("signature file is invalid")
	}
	if width <= 0 || height <= 0 || width > maxSignatureDimension || height > maxSignatureDimension || width*height > maxSignaturePixelCount {
		return invalidf("signature dimensions are too large")
	}
	return nil
}
