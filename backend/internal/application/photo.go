package application

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
)

const (
	maxPhotoBytes      = 4_500_000
	maxPhotoDimension  = 4096
	maxPhotoPixelCount = 16_000_000
)

func validatePhotoDataURL(dataURL string) error {
	if dataURL == "" {
		return nil
	}
	separator := strings.IndexByte(dataURL, ',')
	if separator <= 0 {
		return invalidf("invalid photo data URL")
	}
	header := strings.ToLower(strings.TrimSpace(dataURL[:separator]))
	var mimeType string
	switch header {
	case "data:image/png;base64":
		mimeType = "image/png"
	case "data:image/jpeg;base64", "data:image/jpg;base64":
		mimeType = "image/jpeg"
	case "data:image/webp;base64":
		mimeType = "image/webp"
	default:
		return invalidf("photo must be PNG, JPEG or WebP base64")
	}

	encoded := dataURL[separator+1:]
	if len(encoded) > base64.StdEncoding.EncodedLen(maxPhotoBytes)+4 {
		return invalidf("photo is too large")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return invalidf("photo base64 is invalid")
	}
	if len(decoded) == 0 || len(decoded) > maxPhotoBytes {
		return invalidf("photo must be 1..%d bytes", maxPhotoBytes)
	}

	width, height, err := photoDimensions(decoded, mimeType)
	if err != nil {
		return invalidf("photo file is invalid")
	}
	if width <= 0 || height <= 0 || width > maxPhotoDimension || height > maxPhotoDimension || width*height > maxPhotoPixelCount {
		return invalidf("photo dimensions are too large")
	}
	return nil
}

func photoDimensions(data []byte, mimeType string) (int, int, error) {
	if mimeType != "image/webp" {
		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			return 0, 0, err
		}
		if (mimeType == "image/png" && format != "png") || (mimeType == "image/jpeg" && format != "jpeg") {
			return 0, 0, errors.New("image type mismatch")
		}
		return config.Width, config.Height, nil
	}
	return webPDimensions(data)
}

func webPDimensions(data []byte) (int, int, error) {
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return 0, 0, errors.New("invalid WebP header")
	}
	switch string(data[12:16]) {
	case "VP8X":
		width := 1 + int(data[24]) + int(data[25])<<8 + int(data[26])<<16
		height := 1 + int(data[27]) + int(data[28])<<8 + int(data[29])<<16
		return width, height, nil
	case "VP8 ":
		if len(data) < 30 || data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
			return 0, 0, errors.New("invalid lossy WebP frame")
		}
		width := int(data[26]) | int(data[27]&0x3f)<<8
		height := int(data[28]) | int(data[29]&0x3f)<<8
		return width, height, nil
	case "VP8L":
		if len(data) < 25 || data[20] != 0x2f {
			return 0, 0, errors.New("invalid lossless WebP frame")
		}
		bits := uint32(data[21]) | uint32(data[22])<<8 | uint32(data[23])<<16 | uint32(data[24])<<24
		width := int(bits&0x3fff) + 1
		height := int((bits>>14)&0x3fff) + 1
		return width, height, nil
	default:
		return 0, 0, fmt.Errorf("unsupported WebP chunk %q", string(data[12:16]))
	}
}
