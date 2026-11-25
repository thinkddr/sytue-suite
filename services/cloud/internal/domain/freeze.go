// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MaxFreezeReasonLen acota el motivo de una congelación, en runas. Es un aviso
// para quien intente desplegar, no un documento: 280 caben en un toast. La
// migración 0050 repite el tope como CHECK, que es solo la última red.
const MaxFreezeReasonLen = 280

// DeployFreeze es la congelación de los despliegues de PRODUCCIÓN de un proyecto.
//
// Mientras existe, nada nuevo llega a producción: ni el push a la rama de
// producción, ni un despliegue manual, ni el disparador de un CMS, ni publicar
// una preview. Lo que sigue funcionando es lo que no cambia lo que ve el
// público —las previews— y la salida de emergencia —el rollback—.
type DeployFreeze struct {
	ProjectID uuid.UUID `json:"project_id"`
	Reason    string    `json:"reason"`
	FrozenBy  string    `json:"frozen_by,omitempty"`
	FrozenAt  time.Time `json:"frozen_at"`
}

// NormalizeFreezeReason recorta el motivo y lo valida. Vacío es válido: congelar
// sin explicar el porqué sigue siendo mejor que no poder congelar.
func NormalizeFreezeReason(reason string) (string, error) {
	// Una sola línea: el motivo llega a un aviso y a la auditoría, donde un salto
	// de línea metido a propósito parecería otra entrada.
	r := strings.Join(strings.Fields(reason), " ")
	if utf8.RuneCountInString(r) > MaxFreezeReasonLen {
		return "", fmt.Errorf("%w: el motivo admite como mucho %d caracteres", ErrInvalidInput, MaxFreezeReasonLen)
	}
	return r, nil
}
