// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strings"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/mail/model"
)

// Filter es una regla de filtrado del lado servidor: cuando sus condiciones casan
// con un mensaje entrante, se aplican sus acciones. Modelo propio, minimalista y
// clean-room sobre la idea de Sieve (RFC 5228); no ejecuta un lenguaje, solo un
// AND de subcadenas + un puñado de acciones.
type Filter model.Filter

func (f Filter) HasCondition() bool { return model.Filter(f).HasCondition() }
func (f Filter) HasAction() bool    { return model.Filter(f).HasAction() }
func (f Filter) Validate() error    { return model.Filter(f).Validate() }

// Matches reporta si un mensaje satisface las condiciones (AND de las no vacías).
// Sin ninguna condición devuelve false (defensivo: una regla así no debe existir).
func (f Filter) Matches(m Message) bool {
	if !f.HasCondition() {
		return false
	}
	if s := strings.TrimSpace(f.MatchFrom); s != "" &&
		!containsFold(m.FromAddr, s) && !containsFold(m.FromName, s) {
		return false
	}
	if s := strings.TrimSpace(f.MatchSubject); s != "" && !containsFold(m.Subject, s) {
		return false
	}
	return true
}

// ApplyInbound evalúa las reglas (ya ordenadas por prioridad) contra un mensaje
// entrante y aplica las acciones de la PRIMERA que casa: muta los flags del
// mensaje (marcar leído/destacado) y devuelve el buzón al que debe archivarse
// (nil = dejarlo donde iba, típicamente Inbox). Lógica pura, testeable sin BD.
//
// Primera-que-casa: predecible y suficiente; sin acumulación de acciones de
// varias reglas. ponytail: si hiciera falta "seguir evaluando", añade un flag
// stop-vs-continue por regla y acumula, pero YAGNI hasta que se pida.
func ApplyInbound(filters []Filter, m *Message) *uuid.UUID {
	for _, f := range filters {
		if !f.Enabled || !f.Matches(*m) {
			continue
		}
		if f.MarkRead && !m.HasFlag(FlagSeen) {
			m.Flags = append(m.Flags, FlagSeen)
		}
		if f.MarkFlagged && !m.HasFlag(FlagFlagged) {
			m.Flags = append(m.Flags, FlagFlagged)
		}
		return f.MoveTo
	}
	return nil
}

// containsFold reporta si substr aparece en s sin mirar mayúsculas ni acentos.
func containsFold(s, substr string) bool { return buscar.Contiene(s, substr) }
