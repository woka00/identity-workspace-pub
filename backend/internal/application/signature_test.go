package application

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func signatureTestDataURL(t *testing.T, width, height int) string {
	t.Helper()
	value := image.NewNRGBA(image.Rect(0, 0, width, height))
	value.Set(0, 0, color.NRGBA{A: 255})
	var output bytes.Buffer
	if err := png.Encode(&output, value); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(output.Bytes())
}

func TestValidateSignatureDataURL(t *testing.T) {
	for _, value := range []string{"", signatureTestDataURL(t, 1200, 360)} {
		if err := validateSignatureDataURL(value); err != nil {
			t.Fatalf("expected valid signature: %v", err)
		}
	}

	for _, value := range []string{
		"data:image/jpeg;base64,AAAA",
		"data:image/png;base64,not-base64",
		signatureTestDataURL(t, 2049, 1),
	} {
		if err := validateSignatureDataURL(value); err == nil {
			t.Fatalf("expected invalid signature for %q", value[:min(len(value), 40)])
		}
	}
}
