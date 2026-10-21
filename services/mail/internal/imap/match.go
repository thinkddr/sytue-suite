// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package imap

import (
	"errors"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// matchCriteria evalúa si un mensaje (con su número de secuencia dentro del
// buzón) satisface los criterios de SEARCH. Implementa el subconjunto útil de
// RFC 9051 §7.4.4: conjuntos seq/uid, fechas (SINCE/BEFORE/SENTSINCE/SENTBEFORE),
// cabeceras FROM/SUBJECT/TO/CC, BODY/TEXT, FLAG/NOT FLAG, LARGER/SMALLER, y la
// composición booleana NOT / OR. Es lógica pura (sin I/O): testeable.
//
//nolint:gocognit,gocyclo // evaluador de criterios SEARCH (RFC 9051 §6.4.4): una condición por criterio del protocolo.
func matchCriteria(m domain.Message, seq uint32, c *imap.SearchCriteria) bool {
	if c == nil {
		return true
	}

	// Conjuntos de número de secuencia (todos deben contener el seq).
	for _, set := range c.SeqNum {
		if !set.Contains(seq) {
			return false
		}
	}
	// Conjuntos de UID.
	for _, set := range c.UID {
		if !set.Contains(imap.UID(m.UID)) {
			return false
		}
	}

	// Fechas de recepción (INTERNALDATE) y de envío (Date:), solo parte de día.
	if !c.Since.IsZero() && m.ReceivedAt.Before(truncDay(c.Since)) {
		return false
	}
	if !c.Before.IsZero() && !m.ReceivedAt.Before(truncDay(c.Before)) {
		return false
	}
	if !c.SentSince.IsZero() && m.SentAt.Before(truncDay(c.SentSince)) {
		return false
	}
	if !c.SentBefore.IsZero() && !m.SentAt.Before(truncDay(c.SentBefore)) {
		return false
	}

	// Cabeceras: FROM/TO/CC/SUBJECT y cabeceras genéricas.
	for _, h := range c.Header {
		if !matchHeader(m, h.Key, h.Value) {
			return false
		}
	}

	// BODY/TEXT: buscamos sobre los campos indexados (asunto+remitente+snippet).
	haystack := strings.ToLower(m.Subject + " " + m.FromAddr + " " + m.FromName + " " + m.Snippet)
	for _, s := range c.Body {
		if !strings.Contains(haystack, strings.ToLower(s)) {
			return false
		}
	}
	for _, s := range c.Text {
		if !strings.Contains(haystack, strings.ToLower(s)) {
			return false
		}
	}

	// Flags requeridos y prohibidos.
	for _, f := range c.Flag {
		if !m.HasFlag(string(f)) {
			return false
		}
	}
	for _, f := range c.NotFlag {
		if m.HasFlag(string(f)) {
			return false
		}
	}

	// Tamaño.
	if c.Larger > 0 && m.SizeBytes <= c.Larger {
		return false
	}
	if c.Smaller > 0 && m.SizeBytes >= c.Smaller {
		return false
	}

	// NOT: ninguno de los subcriterios debe casar.
	for i := range c.Not {
		if matchCriteria(m, seq, &c.Not[i]) {
			return false
		}
	}
	// OR: al menos uno de cada par debe casar.
	for i := range c.Or {
		if !matchCriteria(m, seq, &c.Or[i][0]) && !matchCriteria(m, seq, &c.Or[i][1]) {
			return false
		}
	}
	return true
}

func matchHeader(m domain.Message, key, value string) bool {
	value = buscar.Plegar(value)
	var field string
	switch strings.ToLower(key) {
	case "from":
		field = m.FromAddr + " " + m.FromName
	case "subject":
		field = m.Subject
	case "to":
		field = strings.Join(m.To, " ")
	case "cc":
		field = strings.Join(m.Cc, " ")
	case "message-id":
		field = m.MessageID
	default:
		// Cabeceras no indexadas: no podemos afirmar coincidencia sin abrir el
		// blob; devolvemos true solo si el valor buscado está vacío (HEADER key "").
		return value == ""
	}
	if value == "" {
		return field != ""
	}
	return strings.Contains(buscar.Plegar(field), value)
}

// toIMAPEnvelope construye el ENVELOPE IMAP (RFC 9051 §7.5.2) desde el sobre.
func toIMAPEnvelope(m domain.Message) *imap.Envelope {
	env := &imap.Envelope{
		Date:      m.SentAt,
		Subject:   m.Subject,
		From:      addrs(oneAddr(m.FromAddr, m.FromName)),
		To:        addrs(m.To...),
		Cc:        addrs(m.Cc...),
		Bcc:       addrs(m.Bcc...),
		ReplyTo:   addrs(m.ReplyTo...),
		MessageID: m.MessageID,
	}
	if m.InReplyTo != "" {
		env.InReplyTo = []string{m.InReplyTo}
	}
	return env
}

func oneAddr(addr, name string) string {
	if addr == "" {
		return ""
	}
	if name != "" {
		return name + " <" + addr + ">"
	}
	return addr
}

// addrs convierte direcciones "local@host" (con posible display name) al tipo
// imap.Address (Mailbox/Host separados).
func addrs(list ...string) []imap.Address {
	out := make([]imap.Address, 0, len(list))
	for _, raw := range list {
		if raw == "" {
			continue
		}
		name, addr := splitNameAddr(raw)
		local, host := splitAt(addr)
		if local == "" {
			continue
		}
		out = append(out, imap.Address{Name: name, Mailbox: local, Host: host})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitNameAddr(s string) (name, addr string) {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.LastIndex(s, ">"); j > i {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1 : j])
		}
	}
	return "", s
}

func splitAt(addr string) (local, host string) {
	if i := strings.LastIndexByte(addr, '@'); i >= 0 {
		return addr[:i], addr[i+1:]
	}
	return addr, ""
}

// truncDay reduce t a medianoche UTC (SEARCH usa solo la fecha, RFC 9051).
func truncDay(t time.Time) time.Time {
	y, mo, d := t.UTC().Date()
	return time.Date(y, mo, d, 0, 0, 0, 0, time.UTC)
}

// mapIMAPErr traduce errores de dominio a *imap.Error para respuestas IMAP.
func mapIMAPErr(err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeNonExistent, Text: "Mailbox does not exist"}
	case errors.Is(err, domain.ErrMailboxExists):
		return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeAlreadyExists, Text: "Mailbox already exists"}
	case errors.Is(err, domain.ErrSpecialMailbox):
		return &imap.Error{Type: imap.StatusResponseTypeNo, Code: imap.ResponseCodeCannot, Text: "Cannot modify a system mailbox"}
	case errors.Is(err, domain.ErrMailboxNotEmpty):
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox is not empty"}
	case errors.Is(err, domain.ErrInvalidAddress):
		return &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Invalid mailbox name"}
	default:
		return err
	}
}
