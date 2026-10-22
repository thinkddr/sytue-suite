// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
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
	big := "data:image/png;base64," + strings.Repeat("AAAA", 100000) // ~300 KiB
	if _, err := ValidatePhotoDataURI(big); !errors.Is(err, ErrInvalidPhoto) {
		t.Fatalf("imagen > 256 KiB debería rechazarse, err=%v", err)
	}
}

func TestValidatePhotoRejectsBadBase64(t *testing.T) {
	if _, err := ValidatePhotoDataURI("data:image/png;base64,@@@notbase64@@@"); !errors.Is(err, ErrInvalidPhoto) {
		t.Fatalf("base64 ilegible debería rechazarse, err=%v", err)
	}
}

func TestSetAndRemoveCardPhotoRoundTrip(t *testing.T) {
	card := sampleCard(t)
	if CardHasEmbeddedPhoto(card) {
		t.Fatal("la tarjeta base no debería tener foto")
	}

	withPhoto, err := SetCardPhoto(card, pngDataURI)
	if err != nil {
		t.Fatalf("SetCardPhoto: %v", err)
	}
	if !CardHasEmbeddedPhoto(withPhoto) {
		t.Fatal("la foto no quedó embebida")
	}
	// Round-trip: el data URI se recupera intacto de la vCard.
	got, err := CardPhotoDataURI(withPhoto)
	if err != nil {
		t.Fatalf("CardPhotoDataURI: %v", err)
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
	if CardHasEmbeddedPhoto(without) {
		t.Fatal("la foto no se quitó")
	}
}

func withPhoto2DataURI(t *testing.T, card string) string {
	t.Helper()
	du, err := CardPhotoDataURI(card)
	if err != nil {
		t.Fatalf("CardPhotoDataURI: %v", err)
	}
	return du
}
