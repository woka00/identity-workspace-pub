package application

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func imageDataURL(t *testing.T, mime string, encode func(*bytes.Buffer) error) string {
	t.Helper()
	var buffer bytes.Buffer
	if err := encode(&buffer); err != nil {
		t.Fatal(err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(buffer.Bytes())
}

func TestValidatePhotoDataURL(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.White)
	pngURL := imageDataURL(t, "image/png", func(buffer *bytes.Buffer) error { return png.Encode(buffer, img) })
	jpegURL := imageDataURL(t, "image/jpeg", func(buffer *bytes.Buffer) error { return jpeg.Encode(buffer, img, nil) })
	for _, value := range []string{"", pngURL, jpegURL} {
		if err := validatePhotoDataURL(value); err != nil {
			t.Fatalf("valid photo rejected: %v", err)
		}
	}
}

func TestValidatePhotoRejectsActiveOrMismatchedContent(t *testing.T) {
	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`))
	for _, value := range []string{
		"data:image/svg+xml;base64," + svg,
		"data:image/png;base64," + svg,
		"data:image/png;base64,not-base64",
		"data:text/html;base64," + svg,
	} {
		if err := validatePhotoDataURL(value); err == nil {
			t.Fatalf("invalid photo accepted: %q", value[:min(len(value), 48)])
		}
	}
}

func TestValidateWebPDimensions(t *testing.T) {
	webp := make([]byte, 30)
	copy(webp[0:4], "RIFF")
	copy(webp[8:12], "WEBP")
	copy(webp[12:16], "VP8X")
	// Width=2 and height=3 are stored minus one.
	webp[24], webp[27] = 1, 2
	value := "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
	if err := validatePhotoDataURL(value); err != nil {
		t.Fatalf("valid WebP header rejected: %v", err)
	}
	webp[24], webp[25] = 0xff, 0xff
	value = "data:image/webp;base64," + base64.StdEncoding.EncodeToString(webp)
	if err := validatePhotoDataURL(value); err == nil {
		t.Fatal("oversized WebP accepted")
	}
}

func FuzzValidatePhotoDataURL(f *testing.F) {
	f.Add("")
	f.Add("data:image/png;base64,not-base64")
	f.Add("data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=")
	f.Fuzz(func(t *testing.T, value string) {
		_ = validatePhotoDataURL(value)
	})
}
