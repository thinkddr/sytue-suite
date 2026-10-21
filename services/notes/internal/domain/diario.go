// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
package domain

import (
	"time"

	"github.com/google/uuid"
)

// DailyDay es un día del diario con sus cuentas: los puntos de la barra de semana y
// las vistas Semana, Días y «Este día». Notes solo viaja cuando se pide.
type DailyDay struct {
	Date       string    `json:"date"`
	NotebookID uuid.UUID `json:"notebook_id"`
	Total      int       `json:"total"`
	Pending    int       `json:"pending"`
	Notes      []Note    `json:"notes,omitempty"`
}

// Topes de un rango del diario: 62 días de cuentas (dos meses de puntos) y 31 con
// las notas dentro, que es lo que pesa.
const (
	MaxDiasCuentas = 62
	MaxDiasNotas   = 31
)

// FechaDiaria dice si `s` es un día civil `AAAA-MM-DD` válido.
func FechaDiaria(s string) bool {
	d, err := time.Parse(time.DateOnly, s)
	return err == nil && d.Format(time.DateOnly) == s && d.Year() >= 1
}

// RangoDiario valida `[from, to]`: dos días válidos, en orden y como mucho `maximo`
// días contando los dos extremos.
func RangoDiario(from, to string, maximo int) bool {
	if !FechaDiaria(from) || !FechaDiaria(to) {
		return false
	}
	a, _ := time.Parse(time.DateOnly, from)
	b, _ := time.Parse(time.DateOnly, to)
	dias := int(b.Sub(a).Hours()/24) + 1
	return dias >= 1 && dias <= maximo
}
