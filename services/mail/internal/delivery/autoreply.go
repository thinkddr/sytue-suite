// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
)

// autoReplyWindow es la ventana de dedupe por remitente (RFC 3834): no se
// autorresponde al mismo remitente más de una vez cada tanto. 4 días cubre un
// fin de semana largo sin repetir; el dedupe es por (cuenta, remitente).
// ponytail: constante; si hiciera falta ajustarla por usuario, moverla a la config
// de vacaciones (mail_vacation) y pasarla a ClaimAutoreply.
const autoReplyWindow = 4 * 24 * time.Hour

// autoSender es la superficie mínima para despachar una autorespuesta: reencola un
// saliente ya compuesto SIN copia en Enviados. La satisface *Outbound.SubmitAuto.
type autoSender interface {
	SubmitAuto(ctx context.Context, from string, recipients []string, raw []byte) error
}

// autoReplyEligible aplica el filtro RFC 3834: decide si un mensaje entrante puede
// disparar una autorespuesta MIRANDO SUS CABECERAS y el remitente de sobre. Es
// pura (sin cuenta ni estado), así se prueba de forma aislada. Bloquea:
//   - remitente de sobre vacío / <> (rebotes y notificaciones del sistema);
//   - Auto-Submitted distinto de «no» (correo auto-generado / auto-respondido);
//   - Precedence: bulk | list | junk (correo masivo);
//   - cualquier cabecera List-* (RFC 2369/2919: listas de distribución);
//   - X-Auto-Response-Suppress con All/OOF/AutoReply (supresión explícita).
//
// La comprobación de «uno mismo» NO va aquí (depende del destinatario): la hace el
// llamador comparando el remitente con la dirección de la cuenta.
func autoReplyEligible(raw []byte, envelopeFrom string) bool {
	return autoReplyEligibleReader(bytes.NewReader(raw), envelopeFrom)
}

func autoReplyEligibleReader(r io.Reader, envelopeFrom string) bool {
	ef := strings.Trim(strings.TrimSpace(envelopeFrom), "<>")
	if ef == "" {
		return false // MAIL FROM:<> — rebote / mensaje de sistema: nunca responder.
	}
	m, err := mail.ReadMessage(r)
	if err != nil {
		return false // sin cabeceras legibles: fail-safe, no responder.
	}
	h := m.Header

	if as := strings.ToLower(strings.TrimSpace(h.Get("Auto-Submitted"))); as != "" && as != "no" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(h.Get("Precedence"))) {
	case "bulk", "list", "junk":
		return false
	}
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "list-") {
			return false
		}
	}
	if sup := strings.ToLower(h.Get("X-Auto-Response-Suppress")); sup != "" {
		if strings.Contains(sup, "all") || strings.Contains(sup, "oof") || strings.Contains(sup, "autoreply") {
			return false
		}
	}
	return true
}

// buildAutoReply compone el .eml RFC 5322 de la autorespuesta. Marca la propia
// respuesta con Auto-Submitted: auto-replied (RFC 3834, CRÍTICO: rompe bucles con
// otro auto-respondedor) y Precedence: bulk + X-Auto-Response-Suppress: All como
// defensa en profundidad. Hila con el original (In-Reply-To/References). Cuerpo en
// text/plain UTF-8 base64. `from` y `to` son direcciones ya normalizadas.
func buildAutoReply(from, to, subject, body, origMessageID string) []byte {
	if strings.TrimSpace(subject) == "" {
		subject = "Respuesta automática"
	}
	fromDomain := "mail.sytue"
	if at := strings.LastIndexByte(from, '@'); at >= 0 && at < len(from)-1 {
		fromDomain = from[at+1:]
	}
	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + encodeHeaderWord(subject),
		"Date: " + time.Now().UTC().Format(time.RFC1123Z),
		"Message-ID: <" + uuid.NewString() + "@" + fromDomain + ">",
		"Auto-Submitted: auto-replied",
		"Precedence: bulk",
		"X-Auto-Response-Suppress: All",
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"Content-Transfer-Encoding: base64",
	}
	if id := strings.Trim(strings.TrimSpace(origMessageID), "<>"); id != "" {
		headers = append(headers,
			"In-Reply-To: <"+id+">",
			"References: <"+id+">")
	}
	enc := base64.StdEncoding.EncodeToString([]byte(body))
	// Envolver el base64 a 76 columnas (RFC 2045).
	var wrapped strings.Builder
	for len(enc) > 76 {
		wrapped.WriteString(enc[:76])
		wrapped.WriteString("\r\n")
		enc = enc[76:]
	}
	wrapped.WriteString(enc)
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + wrapped.String())
}

// encodeHeaderWord codifica una cabecera con no-ASCII como encoded-word UTF-8
// base64 (RFC 2047); si es ASCII imprimible la deja tal cual.
func encodeHeaderWord(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return "=?utf-8?B?" + base64.StdEncoding.EncodeToString([]byte(s)) + "?="
		}
	}
	return s
}

// maybeAutoReply despacha una autorespuesta de vacaciones para la cuenta receptora
// si procede. `rcpt` es la dirección local a la que llegó el correo (será el From de
// la respuesta: «respondes desde la dirección que te contactaron»); `envelopeFrom`
// el remitente de sobre (destino de la respuesta). Best-effort: cualquier fallo se
// registra y se ignora (NUNCA compromete la entrega del correo entrante).
//
// Cadena de guardas (todas se deben cumplir para responder):
//  1. gate RFC 3834 (autoReplyEligible), calculado una vez por mensaje;
//  2. no responderse a uno mismo (remitente == destinatario local);
//  3. vacaciones activas y en rango (Vacation.ActiveAt);
//  4. dedupe por remitente dentro de la ventana (ClaimAutoreply, atómico).
func (in *Inbound) maybeAutoReply(ctx context.Context, accountID uuid.UUID, rcpt, envelopeFrom, origMessageID string, eligible bool) {
	if in.autoSender == nil || !eligible {
		return
	}
	replyTo, err := domain.ParseAddress(envelopeFrom)
	if err != nil {
		return // sin dirección de respuesta válida: nada que hacer.
	}
	if strings.EqualFold(replyTo.String(), rcpt) {
		return // no responderse a uno mismo.
	}
	vac, err := in.store.GetVacation(ctx, accountID)
	if err != nil {
		in.logger.Warn("autoreply: leer config de vacaciones", "error", err)
		return
	}
	if !vac.ActiveAt(time.Now()) {
		return
	}
	claimed, err := in.store.ClaimAutoreply(ctx, accountID, replyTo.String(), autoReplyWindow)
	if err != nil {
		in.logger.Warn("autoreply: dedupe", "error", err)
		return
	}
	if !claimed {
		return // ya respondido a este remitente dentro de la ventana.
	}
	raw := buildAutoReply(rcpt, replyTo.String(), vac.Subject, vac.Body, origMessageID)
	if err := in.autoSender.SubmitAuto(ctx, rcpt, []string{replyTo.String()}, raw); err != nil {
		in.logger.Warn("autoreply: encolar respuesta", "error", logsafe.Text(err), "to", logsafe.Text(replyTo.String()))
	}
}
