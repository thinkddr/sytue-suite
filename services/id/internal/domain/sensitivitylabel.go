// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strings"
	"time"
)

// Etiquetas de sensibilidad (paridad Purview / Google Drive labels). La TAXONOMÍA de
// una organización vive en Sytue ID; el contenido de cada producto guarda un SNAPSHOT
// denormalizado de la etiqueta (ver mig 0024). Este fichero define el tipo y su
// validación PURA (sin BD ni HTTP).
//
// Alcance honesto de esta ola: taxonomía + etiquetado manual + UNA política real
// (BlockExternalShare). NO hay cifrado por etiqueta ni clasificación automática.

const (
	maxLabelName = 80
	maxLabelDesc = 500
	// maxLabelRank acota el orden de sensibilidad a un rango manejable por la UI
	// (y cabe en el smallint de la columna).
	maxLabelRank = 1000
)

// SensitivityLabel es una etiqueta de la taxonomía de una organización.
type SensitivityLabel struct {
	ID    string
	OrgID string
	Name  string
	// Rank ordena las etiquetas de menor a mayor sensibilidad. Lo define el admin.
	Rank        int
	Color       string
	Description string
	// BlockExternalShare: el contenido con esta etiqueta no admite enlaces
	// públicos/externos. El enforcement vive en el producto (Drive).
	BlockExternalShare bool
	CreatedBy          string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Normalize recorta y valida la etiqueta ANTES de guardarla. Devuelve ErrInvalidInput
// (que el handler traduce a 400) si el nombre falta o algún campo se pasa de largo.
// El color es opcional; si viene, debe ser #rrggbb (la UI lo pinta tal cual).
func (l *SensitivityLabel) Normalize() error {
	l.Name = strings.TrimSpace(l.Name)
	l.Description = strings.TrimSpace(l.Description)
	l.Color = strings.ToLower(strings.TrimSpace(l.Color))
	switch {
	case l.Name == "":
		return fmt.Errorf("%w: el nombre de la etiqueta es obligatorio", ErrInvalidInput)
	case len(l.Name) > maxLabelName:
		return fmt.Errorf("%w: el nombre no puede pasar de %d caracteres", ErrInvalidInput, maxLabelName)
	case len(l.Description) > maxLabelDesc:
		return fmt.Errorf("%w: la descripción no puede pasar de %d caracteres", ErrInvalidInput, maxLabelDesc)
	case l.Rank < 0 || l.Rank > maxLabelRank:
		return fmt.Errorf("%w: el nivel debe estar entre 0 y %d", ErrInvalidInput, maxLabelRank)
	case l.Color != "" && !validHexColor(l.Color):
		return fmt.Errorf("%w: el color debe ser #rrggbb", ErrInvalidInput)
	}
	return nil
}

// validHexColor acepta exactamente #rrggbb (minúsculas ya normalizadas).
func validHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
