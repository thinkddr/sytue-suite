// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package card

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

// pngDataURI es un PNG 1x1 válido (magic bytes reconocidos por el sniff).
const pngDataURI = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func sampleCard(t *testing.T) string {
	t.Helper()
	card, err := EncodeContact(Contact{UID: NewUID(), FormattedName: "Ada Lovelace", GivenName: "Ada", FamilyName: "Lovelace"}, "")
	if err != nil {
		t.Fatalf("EncodeContact: %v", err)
	}
	return card
}

func TestValidatePhotoDataURIOK(t *testing.T) {
	norm, err := ValidatePhotoDataURI(pngDataURI)
	if err != nil {
		t.Fatalf("PNG válido rechazado: %v", err)
	}
	if !strings.HasPrefix(norm, "data:image/png;base64,") {
		t.Errorf("data URI no normalizado: %q", norm)
	}
}

func TestValidatePhotoRejectsSVG(t *testing.T) {
	// SVG no está en la lista blanca raster (evita XSS por vectorial con script).
	if _, err := ValidatePhotoDataURI("data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="); !errors.Is(err, ErrInvalidPhoto) {
		t.Fatalf("SVG debería rechazarse, err=%v", err)
	}
}

func TestValidatePhotoRejectsMismatchedBytes(t *testing.T) {
	// Etiquetado image/png pero el contenido es texto plano: el sniff no cuadra.
	if _, err := ValidatePhotoDataURI("data:image/png;base64,aGVsbG8gd29ybGQ="); !errors.Is(err, ErrInvalidPhoto) {
		t.Fatalf("contenido no-raster debería rechazarse, err=%v", err)
	}
}

func TestValidatePhotoRejectsTooLarge(t *testing.T) {
	png, _, err := PhotoBytes(pngDataURI)
	if err != nil {
		t.Fatal(err)
	}
	// Keep a valid PNG signature on BOTH sides of the boundary: invalid image
	// bytes would also be rejected with the size guard accidentally removed.
	for _, size := range []int{MaxPhotoBytes, MaxPhotoBytes + 1} {
		raw := make([]byte, size)
		copy(raw, png)
		uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)
		_, err := ValidatePhotoDataURI(uri)
		if size == MaxPhotoBytes && err != nil {
			t.Fatal("image exactly at cap rejected", err)
		}
		if size > MaxPhotoBytes && !errors.Is(err, ErrInvalidPhoto) {
			t.Fatal("image above cap accepted", err)
		}
	}
}

func TestValidatePhotoRejectsBadBase64(t *testing.T) {
	if _, err := ValidatePhotoDataURI("data:image/png;base64,@@@notbase64@@@"); !errors.Is(err, ErrInvalidPhoto) {
		t.Fatalf("base64 ilegible debería rechazarse, err=%v", err)
	}
}

func TestSetAndRemoveCardPhotoRoundTrip(t *testing.T) {
	card := sampleCard(t)
	if HasEmbeddedPhoto(card) {
		t.Fatal("la tarjeta base no debería tener foto")
	}

	withPhoto, err := SetCardPhoto(card, pngDataURI)
	if err != nil {
		t.Fatalf("SetCardPhoto: %v", err)
	}
	if !HasEmbeddedPhoto(withPhoto) {
		t.Fatal("la foto no quedó embebida")
	}
	// Round-trip: el data URI se recupera intacto de la vCard.
	got, err := PhotoDataURI(withPhoto)
	if err != nil {
		t.Fatalf("PhotoDataURI: %v", err)
	}
	if got != pngDataURI {
		t.Errorf("data URI no round-trip: got %q", got[:40]+"...")
	}
	// La proyección normalizada NO expone data URIs como PhotoURL (photoRef).
	c, err := DecodeContact(withPhoto)
	if err != nil {
		t.Fatalf("DecodeContact: %v", err)
	}
	if c.PhotoURL != "" {
		t.Errorf("PhotoURL debería estar vacío para foto embebida, got %q", c.PhotoURL)
	}

	// Bytes servibles.
	raw, mime, err := PhotoBytes(withPhoto2DataURI(t, withPhoto))
	if err != nil {
		t.Fatalf("PhotoBytes: %v", err)
	}
	if mime != "image/png" || len(raw) == 0 {
		t.Errorf("bytes/mime inesperados: mime=%q len=%d", mime, len(raw))
	}

	without, err := RemoveCardPhoto(withPhoto)
	if err != nil {
		t.Fatalf("RemoveCardPhoto: %v", err)
	}
	if HasEmbeddedPhoto(without) {
		t.Fatal("la foto no se quitó")
	}
}

func withPhoto2DataURI(t *testing.T, card string) string {
	t.Helper()
	du, err := PhotoDataURI(card)
	if err != nil {
		t.Fatalf("PhotoDataURI: %v", err)
	}
	return du
}
