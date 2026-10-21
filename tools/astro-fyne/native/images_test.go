// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Astro Fyne contributors.

package webui

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
)

func bitmapFixture(t *testing.T, format string, paint color.NRGBA) fyne.Resource {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 8))
	for y := range 8 {
		for x := range 16 {
			img.SetNRGBA(x, y, paint)
		}
	}
	var encoded bytes.Buffer
	var err error
	if format == "jpeg" {
		err = jpeg.Encode(&encoded, img, &jpeg.Options{Quality: 100})
	} else {
		err = png.Encode(&encoded, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	return fyne.NewStaticResource("fixture."+format, encoded.Bytes())
}

func TestBitmapFullDecodeAndDimensions(t *testing.T) {
	for _, format := range []string{"png", "jpeg"} {
		t.Run(format, func(t *testing.T) {
			resource := bitmapFixture(t, format, color.NRGBA{R: 255, A: 255})
			size, err := ValidateBitmap(resource)
			if err != nil || size != fyne.NewSize(16, 8) {
				t.Fatalf("valid %s dimensions=%v error=%v", format, size, err)
			}
			// Preserve all header data while removing the pixel stream's ending.
			truncated := fyne.NewStaticResource(resource.Name(), resource.Content()[:len(resource.Content())-8])
			if _, _, err := image.DecodeConfig(bytes.NewReader(truncated.Content())); err != nil {
				t.Fatalf("fixture must retain a decodable header to exercise full pixel validation: %v", err)
			}
			if _, err := ValidateBitmap(truncated); err == nil {
				t.Fatal("header-only/truncated resource was accepted without decoding pixels")
			}
		})
	}
	var typedNil *fyne.StaticResource
	for _, invalid := range []fyne.Resource{nil, typedNil, fyne.NewStaticResource("empty.png", nil), fyne.NewStaticResource("vector.svg", []byte("<svg/>")), fyne.NewStaticResource("fake.png", []byte("garbage"))} {
		if _, err := ValidateBitmap(invalid); err == nil {
			t.Fatalf("invalid bitmap accepted: %v", invalid)
		}
	}
}

func TestBitmapRejectsIgnoredBrowserTransformMetadata(t *testing.T) {
	p := bitmapFixture(t, "png", color.NRGBA{A: 255}).Content()
	for _, kind := range []string{"iCCP", "gAMA", "cHRM", "eXIf", "acTL"} {
		t.Run(kind, func(t *testing.T) {
			chunk := make([]byte, 12)
			copy(chunk[4:8], kind)
			binary.BigEndian.PutUint32(chunk[8:], crc32.ChecksumIEEE(chunk[4:8]))
			withMetadata := append(bytes.Clone(p[:33]), chunk...)
			withMetadata = append(withMetadata, p[33:]...)
			if _, err := ValidateBitmap(fyne.NewStaticResource("metadata.png", withMetadata)); err == nil || !strings.Contains(err.Error(), kind) {
				t.Fatalf("PNG metadata was ignored: %v", err)
			}
		})
	}
	j := bitmapFixture(t, "jpeg", color.NRGBA{A: 255}).Content()
	for _, metadata := range []struct {
		marker byte
		data   string
	}{{0xe1, "Exif\x00\x00"}, {0xe2, "ICC_PROFILE\x00"}, {0xee, "Adobe\x00\x64\x00\x00\x00\x00\x00"}} {
		segment := []byte{0xff, metadata.marker, 0, byte(len(metadata.data) + 2)}
		segment = append(segment, metadata.data...)
		withMetadata := append(bytes.Clone(j[:2]), segment...)
		withMetadata = append(withMetadata, j[2:]...)
		if _, err := ValidateBitmap(fyne.NewStaticResource("metadata.jpeg", withMetadata)); err == nil || !strings.Contains(err.Error(), "metadata") {
			t.Fatalf("JPEG metadata was ignored: %v", err)
		}
	}
}

func TestBitmapRejectsUnverifiedPNGColorDepthAndPalette(t *testing.T) {
	for _, pixels := range []image.Image{
		image.NewNRGBA64(image.Rect(0, 0, 2, 2)),
		image.NewGray(image.Rect(0, 0, 2, 2)),
		image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White}),
	} {
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, pixels); err != nil {
			t.Fatal(err)
		}
		if _, err := ValidateBitmap(fyne.NewStaticResource("unsupported.png", encoded.Bytes())); err == nil || !strings.Contains(err.Error(), "8-bit RGB") {
			t.Fatalf("unverified PNG color conversion accepted for %T: %v", pixels, err)
		}
	}
}

func TestImageUsesIntrinsicSizeAndPreservesAspectForOneDimension(t *testing.T) {
	resource := bitmapFixture(t, "png", color.NRGBA{R: 255, A: 255})
	style := Style{}
	v := NewView(func() []Node { return []Node{{ID: "image", Kind: "image", ImageResource: resource, Style: style}} })
	if v.Error() != nil || v.Object("image").MinSize() != fyne.NewSize(16, 8) {
		t.Fatalf("intrinsic dimensions changed: %v error=%v", v.Object("image").MinSize(), v.Error())
	}
	style.Width = 32
	v.Refresh()
	if v.Object("image").MinSize() != fyne.NewSize(32, 16) {
		t.Fatalf("single width lost source aspect: %v", v.Object("image").MinSize())
	}
	style.Width, style.Height = 0, 24
	v.Refresh()
	if v.Object("image").MinSize() != fyne.NewSize(48, 24) {
		t.Fatalf("single height lost source aspect: %v", v.Object("image").MinSize())
	}
}

func TestMeasuredImagePaintsFirstFrameAndInvalidatesChangedContent(t *testing.T) {
	red := color.NRGBA{R: 255, A: 255}
	resource := bitmapFixture(t, "png", red)
	v := NewView(func() []Node {
		return []Node{{ID: "root", Kind: "container", Children: []Node{{ID: "image", Kind: "image", ImageResource: resource, AccessibleLabel: "Red sample"}}}}
	})
	v.SetViewport(40, 30)
	profile := map[string]Style{
		"root":  {Width: 40, Height: 30, Background: "#ffffff", Opacity: 1, Measured: true},
		"image": {X: 5, Y: 7, Width: 16, Height: 8, Opacity: 1, Measured: true},
	}
	if err := v.ApplyMeasurements(profile); err != nil {
		t.Fatal(err)
	}
	first := capture(v, 1, 40, 30)
	for y := range 8 {
		for x := range 16 {
			if actual := rgba(first.At(5+x, 7+y)); actual != red {
				t.Fatalf("first-frame image pixel %d,%d = %v", x, y, actual)
			}
		}
	}
	object := v.Object("image")
	if object.(fyne.Accessible).AccessibilityLabel() != "Red sample" {
		t.Fatal("image's alternative text was lost")
	}
	blue := color.NRGBA{B: 255, A: 255}
	resource = bitmapFixture(t, "png", blue) // Same resource name and dimensions.
	v.Refresh()
	if v.Error() == nil || !strings.Contains(v.Error().Error(), "visual state changed") {
		t.Fatal("changing image bytes reused a measurement certificate for different pixels")
	}
	if v.Object("image") != object {
		t.Fatal("changing a bitmap replaced its stable native control")
	}
	if err := v.ApplyMeasurements(profile); err != nil {
		t.Fatal(err)
	}
	second := capture(v, 1, 40, 30)
	if actual := rgba(second.At(5, 7)); actual != blue {
		t.Fatalf("new validated resource did not replace the old pixels: %v", actual)
	}
}

func TestInvalidImagePreservesValidatedPixelsAndRejectsUnsupportedStyles(t *testing.T) {
	resource := bitmapFixture(t, "png", color.NRGBA{R: 255, A: 255}).(*fyne.StaticResource)
	style := Style{}
	v := NewView(func() []Node { return []Node{{ID: "image", Kind: "image", ImageResource: resource, Style: style}} })
	object := v.Object("image")
	frozen := bytes.Clone(v.elements["image"].node.ImageResource.Content())
	resource.StaticContent = []byte("malformed")
	v.Refresh()
	if v.Error() == nil || v.Object("image") != object || !bytes.Equal(v.elements["image"].node.ImageResource.Content(), frozen) {
		t.Fatal("invalid mutable resource changed the last validated image tree")
	}
	resource.StaticContent = frozen
	v.Refresh()
	if v.Error() != nil {
		t.Fatal(v.Error())
	}
	for _, unsupported := range []Style{{Radius: 4}, {BorderWidth: 1}, {PaddingLeft: 1}, {PaddingTop: 1}, {PaddingRight: 1}, {PaddingBottom: 1}} {
		style = unsupported
		v.Refresh()
		if v.Error() == nil {
			t.Fatalf("unsupported source image style accepted: %+v", style)
		}
	}
	style = Style{}
	v.Refresh()
	if err := v.ApplyMeasurements(map[string]Style{"image": {Width: 16, Height: 8, Radius: 2, Opacity: 1, Measured: true}}); err == nil {
		t.Fatal("capture bypassed unsupported image clipping guard")
	}
}
