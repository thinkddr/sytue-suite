// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Preferencias de LECTURA de la cuenta (migración 0023): a dónde se va tras sacar
// la conversación abierta de la vista (archivarla, tirarla, moverla) y qué hace
// deslizar una fila con el dedo. Viven en la cuenta y no en el navegador: son
// costumbres de quien lee, las mismas en el portátil y en el teléfono.

// AutoAvance es a dónde va el lector cuando la conversación abierta sale de la vista.
const (
	// AvanceMasAntigua abre la siguiente de la lista (la más antigua): lo que se
	// espera al vaciar la bandeja de arriba abajo. Es el valor por defecto.
	AvanceMasAntigua = "older"
	// AvanceMasReciente abre la anterior (la más reciente).
	AvanceMasReciente = "newer"
	// AvanceLista vuelve a la lista, sin abrir nada (lo que pasaba antes).
	AvanceLista = "list"
)

// Lo que hace deslizar una fila de la lista en el móvil.
const (
	GestoArchivar = "archive"
	GestoPapelera = "trash"
	GestoLeido    = "read"
	GestoNinguno  = "none"
)

// Listas cerradas y no texto libre: la interfaz pinta un selector con estas
// opciones, y un valor guardado por otro cliente que no estuviera aquí no tendría
// opción que marcar.
var (
	autoAvances = []string{AvanceMasAntigua, AvanceMasReciente, AvanceLista}
	gestos      = []string{GestoArchivar, GestoPapelera, GestoLeido, GestoNinguno}
)

// ReadingPrefs son las preferencias de lectura de una cuenta.
type ReadingPrefs struct {
	AutoAdvance string `json:"auto_advance"`
	SwipeRight  string `json:"swipe_right"`
	SwipeLeft   string `json:"swipe_left"`
}

// ReadingPrefsPorDefecto son las de una cuenta que nunca las tocó (las de 0023).
var ReadingPrefsPorDefecto = ReadingPrefs{
	AutoAdvance: AvanceMasAntigua,
	SwipeRight:  GestoArchivar,
	SwipeLeft:   GestoPapelera,
}

// ErrInvalidReadingPrefs: algún valor no es una de las opciones.
var ErrInvalidReadingPrefs = errors.New("domain: invalid reading preferences")

// Validar rechaza cualquier valor fuera de las listas. No se corrige al más
// parecido: guardar otra cosa y contestar 204 dejaría al cliente enseñando un
// ajuste que no es el guardado.
func (p ReadingPrefs) Validar() error {
	if !slices.Contains(autoAvances, p.AutoAdvance) ||
		!slices.Contains(gestos, p.SwipeRight) ||
		!slices.Contains(gestos, p.SwipeLeft) {
		return ErrInvalidReadingPrefs
	}
	return nil
}

// RemitentesDeConfianzaPorCuenta es el tope de remitentes cuyas imágenes se
// enseñan sin preguntar. Holgado para cualquier persona; existe para que un
// cliente roto no llene la tabla.
const RemitentesDeConfianzaPorCuenta = 1000

// ErrTrustedSenderLimite: la cuenta ya tiene todos los remitentes de confianza.
var ErrTrustedSenderLimite = errors.New("domain: trusted sender limit reached")

// NormalizarRemitente deja una dirección como se guarda: sin espacios alrededor y
// en minúsculas («Ana@X.eu» y «ana@x.eu» son el mismo remitente). Rechaza lo que
// no tiene forma de dirección (`algo@dominio`) con ErrInvalidAddress.
func NormalizarRemitente(addr string) (string, error) {
	a := strings.ToLower(strings.TrimSpace(addr))
	local, dominio, ok := strings.Cut(a, "@")
	if !ok || local == "" || dominio == "" || strings.Contains(dominio, "@") ||
		utf8.RuneCountInString(a) > 320 {
		return "", ErrInvalidAddress
	}
	if strings.ContainsFunc(a, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r) || strings.ContainsRune("<>,;\"", r)
	}) {
		return "", ErrInvalidAddress
	}
	return a, nil
}
