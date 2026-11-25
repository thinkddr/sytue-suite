// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"slices"
	"time"
)

// La política de cada organización para el contacto con gente de fuera en Rooms (mig
// 0083): abierta —la de siempre, y la de una organización sin fila—, cerrada, o solo
// con las organizaciones de confianza. Es la misma forma que la de las peticiones de
// firma de CertiPDF (ADR 0059).
const (
	ContactoAbierto   = "abierta"
	ContactoCerrado   = "cerrada"
	ContactoConfianza = "confianza"
	// MaxConfianzaContacto es cuántas organizaciones de confianza caben.
	MaxConfianzaContacto = 100
)

// ErrContactoExternoCerrado: la política de una de las dos organizaciones no admite
// este contacto. A quien abre la conversación se le dice —es la regla de su propia
// organización—; desde fuera, la conversación sencillamente no aparece.
var ErrContactoExternoCerrado = errors.New("chat: la organización no admite contacto con gente de fuera")

// PoliticaContactoExterno es la política de una organización.
type PoliticaContactoExterno struct {
	Modo        string     `json:"mode"`
	Confianza   []string   `json:"trusted"`
	Actualizada *time.Time `json:"updated_at,omitempty"`
}

// Admite dice si la política deja el contacto con la organización `otra`.
func (p PoliticaContactoExterno) Admite(otra string) bool {
	switch p.Modo {
	case ContactoCerrado:
		return false
	case ContactoConfianza:
		return slices.Contains(p.Confianza, otra)
	default:
		return true
	}
}

// AdmiteAlguna dice si la política deja el contacto con alguien que está en alguna de
// `orgs` (una persona puede estar en varias; basta con que una sea de confianza).
func (p PoliticaContactoExterno) AdmiteAlguna(orgs []string) bool {
	for _, o := range orgs {
		if p.Admite(o) {
			return true
		}
	}
	return p.Modo == "" || p.Modo == ContactoAbierto
}
