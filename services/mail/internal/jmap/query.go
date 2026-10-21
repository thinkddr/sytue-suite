// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"encoding/json"
	"errors"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// emailQueryEnvelope es la forma cruda del método Email/query (RFC 8621 §4.4).
type emailQueryEnvelope struct {
	Filter emailFilter `json:"filter"`
	Limit  *int        `json:"limit"`
	Offset int         `json:"position"`
}

// emailFilter es el subconjunto de FilterCondition de Email/query que soporta
// Sytue Mail. Los operadores lógicos (AND/OR/NOT) no se aceptan en esta versión;
// el filtro es una única condición (que es lo que usa el cliente web).
type emailFilter struct {
	InMailbox string `json:"inMailbox"`
	Text      string `json:"text"`
	From      string `json:"from"`
	Subject   string `json:"subject"`
	// HasKeyword/NotKeyword restringen por keyword ($seen, $flagged, ...).
	HasKeyword string `json:"hasKeyword"`
	NotKeyword string `json:"notKeyword"`
	// HasAttachment restringe a mensajes con adjunto (operador has:attachment).
	HasAttachment bool `json:"hasAttachment"`
}

// maxQueryLimit es el tope de resultados de Email/query (defensa anti-abuso).
const maxQueryLimit = 200

// parseEmailQueryFilter traduce los argumentos crudos de Email/query a un
// store.SearchFilter. Es lógica pura (sin I/O) y por tanto testeable de forma
// unitaria: valida el límite, mapea keywords a flags y el mailbox a UUID.
func parseEmailQueryFilter(raw rawArgs) store.SearchFilter {
	var env emailQueryEnvelope
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &env)
	}

	// El texto libre puede traer operadores (`from:ana has:attachment
	// after:2026-01-01`): se desmontan aquí, donde ya se traduce el resto del
	// filtro. Una consulta sin operadores sale igual que entró, así que ningún
	// cliente cambia de comportamiento por esto. Los criterios que vengan por su
	// campo propio de JMAP mandan sobre los del texto: quien los envía así está
	// siendo explícito.
	q := domain.ParseSearchQuery(env.Filter.Text)
	f := store.SearchFilter{
		Text:          q.Text,
		From:          firstNonEmpty(env.Filter.From, q.From),
		To:            q.To,
		Subject:       firstNonEmpty(env.Filter.Subject, q.Subject),
		HasAttachment: env.Filter.HasAttachment || q.HasAttachment,
		Seen:          q.Seen,
		Unseen:        q.Unseen,
		Flagged:       q.Flagged,
		After:         q.After,
		Before:        q.Before,
		Offset:        max0(env.Offset),
	}
	if env.Filter.InMailbox != "" {
		if id, err := uuid.Parse(env.Filter.InMailbox); err == nil {
			f.MailboxID = id
		}
	}
	switch env.Filter.HasKeyword {
	case "$seen":
		f.Seen = true
	case "$flagged":
		f.Flagged = true
	}
	if env.Filter.NotKeyword == "$seen" {
		f.Unseen = true
	}

	f.Limit = 50
	if env.Limit != nil {
		f.Limit = *env.Limit
	}
	if f.Limit <= 0 || f.Limit > maxQueryLimit {
		f.Limit = maxQueryLimit
	}
	return f
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// isDomain reporta si err coincide (errors.Is) con un error de dominio dado.
func isDomain(err, target error) bool { return errors.Is(err, target) }

// firstNonEmpty devuelve el primer valor no vacío. Sirve para que un criterio
// explícito del filtro JMAP tenga prioridad sobre el mismo criterio deducido de
// los operadores del texto libre.
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
