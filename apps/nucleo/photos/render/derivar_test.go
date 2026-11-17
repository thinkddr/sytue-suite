// SPDX-License-Identifier: Apache-2.0
package render

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/photos/model"
)

func TestDerivationsRetainOneImageBeforeCancellationAndEraseOwnedBuffers(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, patron()); err != nil {
		t.Fatal(err)
	}
	retained := bytes.Clone(input.Bytes())
	owned := bytes.Clone(retained)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	calls, loads := 0, 0
	var borrowed []byte
	err := Derivar(ctx, func(context.Context) ([]byte, error) { loads++; return owned, nil }, 6, model.RecetaVacia(), true, []int{1, 2, 3}, func(side int, format, contentType string, raw []byte) error {
		calls++
		borrowed = raw
		if side != 1 || format != "png" || contentType != "image/png" {
			t.Fatal("cascade encoding contract changed")
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil || img.Bounds() != image.Rect(0, 0, 1, 1) {
			t.Fatal("first retained image absent", err)
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || calls != 1 || loads != 1 {
		t.Fatal("canceled recipe retained further images", err, calls, loads)
	}
	if !bytes.Equal(retained, input.Bytes()) || !bytes.Equal(owned, make([]byte, len(owned))) || !bytes.Equal(borrowed, make([]byte, len(borrowed))) {
		t.Fatal("derivation changed retained original or kept owned plaintext buffers")
	}
}

func TestDerivedJPEGPreservesTheExistingWhiteTransparencyBackground(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	calls := 0
	err := Derivar(t.Context(), func(context.Context) ([]byte, error) { return bytes.Clone(input.Bytes()), nil }, 1, model.RecetaVacia(), false, []int{2}, func(_ int, format, contentType string, raw []byte) error {
		calls++
		img, kind, err := image.Decode(bytes.NewReader(raw))
		if err != nil || kind != "jpeg" || format != "jpg" || contentType != "image/jpeg" {
			t.Fatal("derived JPEG contract changed", err)
		}
		red, green, blue, _ := img.At(0, 0).RGBA()
		if red < 65000 || green < 65000 || blue < 65000 {
			t.Fatal("derived JPEG lost white background")
		}
		return nil
	})
	if err != nil || calls != 1 {
		t.Fatal("derivation did not retain JPEG", err, calls)
	}
}
