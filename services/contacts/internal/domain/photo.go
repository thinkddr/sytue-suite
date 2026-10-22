// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"

	"github.com/emersion/go-vcard"

	"git.sytue.eu/sytue/one/platform/pkg/validation"
)

// La foto de un contacto se guarda EMBEBIDA en la propiedad PHOTO de la vCard
// (RFC 6350 6.2.4) como data URI base64 (data:image/...;base64,...). Asi viaja en
// el mismo card_objects.data que ya round-trip-ea el servicio (CardDAV / export
// vCard): sin object storage (S3) y sin columnas nuevas. Solo raster acotado en
// tamano; nunca SVG (evita XSS por <script> en un vectorial servido como imagen).
//
// La validacion del data URI (tope de tamano, lista blanca de tipos, sniff de
// bytes) es la MISMA que usa el avatar propio de Sytue ID: vive una vez en
// platform/pkg/validation. Lo propio de aqui es SOLO el round-trip con la vCard.

// MaxPhotoBytes acota el tamano de la imagen DECODIFICADA (no el base64).
const MaxPhotoBytes = validation.MaxRasterImageBytes

// ErrInvalidPhoto se devuelve cuando la imagen no es un data URI raster valido o
// supera el limite de tamano.
var ErrInvalidPhoto = validation.ErrInvalidRasterImage

// ValidatePhotoDataURI valida un data URI de imagen: prefijo data:image/<t>;base64,
// con <t> en la lista blanca raster, base64 decodificable, tamano <= MaxPhotoBytes
// y bytes cuyo tipo real (sniff) coincide con la etiqueta. Devuelve el data URI
// normalizado (canonico, sin espacios) listo para incrustar en la vCard.
func ValidatePhotoDataURI(dataURI string) (string, error) {
	return validation.RasterImageDataURI(dataURI)
}

// PhotoBytes decodifica el data URI de una PHOTO a sus bytes y tipo MIME. Se usa
// para servir la imagen cruda (avatar) sin exponer el data URI entero.
func PhotoBytes(dataURI string) ([]byte, string, error) {
	return validation.RasterImageBytes(dataURI)
}

// SetCardPhoto incrusta (o reemplaza) la PHOTO de una vCard con el data URI dado y
// devuelve la tarjeta re-serializada canonica. Conserva el resto de propiedades.
func SetCardPhoto(cardData, dataURI string) (string, error) {
	card, err := parseSingle(cardData)
	if err != nil {
		return "", err
	}
	card.SetValue(vcard.FieldPhoto, dataURI)
	return marshal(card)
}

// RemoveCardPhoto elimina la PHOTO de una vCard y devuelve la tarjeta
// re-serializada. Idempotente (si no habia PHOTO, no cambia nada relevante).
func RemoveCardPhoto(cardData string) (string, error) {
	card, err := parseSingle(cardData)
	if err != nil {
		return "", err
	}
	delete(card, vcard.FieldPhoto)
	return marshal(card)
}

// CardHasEmbeddedPhoto indica si una vCard lleva una PHOTO embebida (data URI),
// no una mera referencia externa (URL).
func CardHasEmbeddedPhoto(cardData string) bool {
	card, err := parseSingle(cardData)
	if err != nil {
		return false
	}
	f := card.Get(vcard.FieldPhoto)
	return f != nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(f.Value)), "data:")
}

// CardPhotoDataURI devuelve el data URI de la PHOTO embebida de una vCard, o ""
// si no hay foto embebida.
func CardPhotoDataURI(cardData string) (string, error) {
	card, err := parseSingle(cardData)
	if err != nil {
		return "", err
	}
	f := card.Get(vcard.FieldPhoto)
	if f == nil {
		return "", nil
	}
	v := strings.TrimSpace(f.Value)
	if !strings.HasPrefix(strings.ToLower(v), "data:") {
		return "", nil
	}
	return v, nil
}
