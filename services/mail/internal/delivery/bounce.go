// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/blobs"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// bounce cierra una unidad de salida que NO se va a entregar: la marca en la cola
// y AVISA a quien la escribió.
//
// El aviso es la mitad que faltaba. Hasta ahora un mensaje que agotaba reintentos
// —o que el destino rechazaba con un 5xx— solo cambiaba de estado en una tabla:
// sin correo de vuelta, sin nada en la interfaz y sin más rastro que una línea de
// log que nadie mira. Desde la silla de quien escribe, «enviar» y no enviarse
// nunca son indistinguibles, que es el peor fallo posible en un correo: uno cree
// que la otra parte lo tiene.
//
// Se entrega en la BANDEJA del remitente, como hace cualquier servidor con el
// mailer-daemon, y por el mismo camino que un correo de fuera: blob en S3 + sobre
// en PostgreSQL. Reusar ese camino es lo que hace que salga en la lista, en la
// búsqueda, en el hilo y en el móvil sin tocar ni una línea más.
func (q *QueueWorker) bounce(ctx context.Context, it domain.OutboundMessage, reason string) {
	if err := q.store.MarkBounced(ctx, it.ID, reason); err != nil {
		q.logger.Error("queue: marcar rebotado", "id", it.ID, "error", err)
	}
	if err := q.notifySender(ctx, it, reason); err != nil {
		// Best-effort: la cola ya está cerrada y reintentar el aviso duplicaría el
		// correo. Se registra con el motivo original para no perderlo del todo.
		q.logger.Error("queue: avisar del rebote", "id", it.ID, "motivo", reason, "error", err)
	}
}

// notifySender entrega el aviso de no-entrega en la bandeja de quien envió.
func (q *QueueWorker) notifySender(ctx context.Context, it domain.OutboundMessage, reason string) error {
	// El remitente del sobre es una dirección local (el servicio ya lo validó al
	// aceptar el envío): de ahí salen su tenant y su cuenta.
	resolved, err := q.store.LookupAddress(ctx, it.From)
	if err != nil {
		return fmt.Errorf("bounce: resolver remitente %q: %w", it.From, err)
	}
	t, err := tenancy.New(resolved.Tenant)
	if err != nil {
		return fmt.Errorf("bounce: tenant %q: %w", resolved.Tenant, err)
	}
	tctx := tenancy.Into(ctx, t)

	mailbox, err := q.store.MailboxByRole(tctx, resolved.AccountID, domain.RoleInbox)
	if err != nil {
		return fmt.Errorf("bounce: bandeja de entrada: %w", err)
	}

	raw := buildBounceNotice(it, reason, time.Now())
	// Se parsea lo que se acaba de componer, en vez de rellenar el sobre a mano:
	// así el asunto, el remitente y el extracto salen del MISMO parser que trata
	// el correo de fuera, y no hay dos maneras de construir un sobre.
	parsed, err := mime.Parse(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("bounce: parsear el aviso: %w", err)
	}

	blobKey, err := blobs.GuardarMensaje(tctx, q.store, q.blobs, t, raw)
	if err != nil {
		return fmt.Errorf("bounce: subir el aviso: %w", err)
	}

	m := parsed.Envelope
	m.AccountID = resolved.AccountID
	m.MailboxID = mailbox.ID
	m.BlobKey = blobKey
	m.SizeBytes = int64(len(raw))
	if err := q.store.InsertMessage(tctx, &m); err != nil {
		return fmt.Errorf("bounce: persistir el aviso: %w", err)
	}
	return nil
}

// buildBounceNotice compone el aviso (RFC 5322, texto plano) que recibe quien
// envió. Sin adjuntar el original: el mensaje sigue en Enviados y duplicar el
// cuerpo en la bandeja multiplicaría el consumo del buzón justo cuando el destino
// puede ser una lista de cien direcciones.
//
// ponytail: texto plano en castellano, como el resto del correo que emite la
// plataforma (`services/id/internal/http/mailer.go`). Traducirlo pide saber el
// idioma de la cuenta, que hoy no viaja hasta este worker; cuando viaje, el
// cambio es este único sitio.
func buildBounceNotice(it domain.OutboundMessage, reason string, now time.Time) []byte {
	daemon := "mailer-daemon@" + domainOf(it.From)
	asunto := strings.TrimSpace(it.Subject)
	if asunto == "" {
		asunto = "(sin asunto)"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: Sytue Mail <%s>\r\n", daemon)
	fmt.Fprintf(&b, "To: %s\r\n", it.From)
	fmt.Fprintf(&b, "Subject: No se ha podido entregar: %s\r\n", sanitizeHeader(asunto))
	fmt.Fprintf(&b, "Date: %s\r\n", now.UTC().Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", uuid.NewString(), domainOf(it.From))
	// Marcas RFC 3834: esto es una respuesta automática. Sin ellas, una regla de
	// vacaciones del propio buzón podría contestarle al daemon y montar un bucle.
	b.WriteString("Auto-Submitted: auto-replied\r\n")
	b.WriteString("X-Auto-Response-Suppress: All\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")

	b.WriteString("Tu mensaje no ha llegado a su destino.\r\n\r\n")
	fmt.Fprintf(&b, "Asunto:        %s\r\n", asunto)
	fmt.Fprintf(&b, "Destinatarios: %s\r\n", strings.Join(it.Recipients, ", "))
	fmt.Fprintf(&b, "Intentos:      %d\r\n", it.Attempts+1)
	fmt.Fprintf(&b, "Motivo:        %s\r\n\r\n", strings.TrimSpace(reason))
	b.WriteString("El mensaje original sigue en Enviados: puedes corregir la direccion\r\n")
	b.WriteString("y reenviarlo desde ahi. Si la direccion es correcta, el servidor de\r\n")
	b.WriteString("destino la ha rechazado y hay que hablar con quien lo administra.\r\n")
	return []byte(b.String())
}

// sanitizeHeader impide que un asunto con salto de línea parta la cabecera en dos
// (inyección de cabeceras). El asunto llega de quien escribió el correo, así que
// es entrada de usuario aunque venga de nuestra propia base de datos.
func sanitizeHeader(v string) string {
	return strings.NewReplacer("\r", " ", "\n", " ", "\x00", "").Replace(v)
}
