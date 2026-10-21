// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package jmap

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/blobs"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
)

// Email/import (RFC 8621 §2.7) es cómo entra un correo que NO llega por SMTP:
// una mudanza desde otro proveedor, la restauración de un archivo, un .eml que
// alguien arrastra al buzón.
//
// Existe porque `Email/set` solo actualiza y destruye —crear un correo no era
// posible—, y sin esto no hay forma de traerse el histórico de un buzón: la
// migración tendría que inventarse una puerta trasera contra la base de datos, y
// eso se salta las cuotas, los índices de hilo y la auditoría.
//
// Lo que hace honesta a la importación es que respeta el ORIGINAL: la fecha de
// recepción, el estado de leído y la carpeta se toman de lo que se importa, no
// del momento de importarlo. Un buzón migrado se lee igual que el de origen.

// maxImportPerCall acota cuántos mensajes acepta una sola llamada. El cliente
// (o la migración) trocea; sin tope, una petición podría tener el proceso
// ocupado indefinidamente.
const maxImportPerCall = 100

// importArgs son los argumentos de Email/import.
type importArgs struct {
	Emails map[string]importEmail `json:"emails"`
}

// importEmail describe UN mensaje a importar.
type importEmail struct {
	// BlobID es el .eml crudo ya subido por /jmap/upload.
	BlobID string `json:"blobId"`
	// MailboxIDs es dónde aterriza. JMAP admite varios; aquí manda el primero:
	// el modelo de Sytue Mail guarda un mensaje en UN buzón (como IMAP).
	MailboxIDs map[string]bool `json:"mailboxIds"`
	// Keywords son los estados del mensaje ($seen, $flagged, $draft…).
	Keywords map[string]bool `json:"keywords"`
	// ReceivedAt es la fecha ORIGINAL de recepción. Vacía ⇒ la del propio
	// mensaje (cabecera Date) y, si tampoco está, ahora.
	ReceivedAt string `json:"receivedAt"`
}

// emailImport crea los mensajes indicados a partir de sus blobs.
func (a *API) emailImport(r *http.Request, acc domain.Account, raw rawArgs) (any, error) {
	var args importArgs
	raw.into(&args)

	if a.blobs == nil {
		return nil, httpx503("el almacenamiento de mensajes no está disponible")
	}
	if len(args.Emails) == 0 {
		return nil, errInvalidArguments("no hay mensajes que importar")
	}
	if len(args.Emails) > maxImportPerCall {
		return nil, errInvalidArguments(fmt.Sprintf(
			"demasiados mensajes en una sola llamada (máximo %d); trócealo", maxImportPerCall,
		))
	}
	t, err := tenancy.From(r.Context())
	if err != nil {
		return nil, err
	}

	created := map[string]any{}
	notCreated := map[string]any{}
	for clientID, em := range args.Emails {
		msg, err := a.importOne(r.Context(), t, acc, em)
		if err != nil {
			notCreated[clientID] = setError(err)
			continue
		}
		created[clientID] = map[string]any{
			"id":         msg.ID.String(),
			"blobId":     msg.BlobKey,
			"threadId":   msg.ThreadID.String(),
			"size":       msg.SizeBytes,
			"receivedAt": msg.ReceivedAt.UTC().Format(time.RFC3339),
		}
	}
	return map[string]any{
		"accountId":  acc.ID.String(),
		"oldState":   "0",
		"newState":   "0",
		"created":    created,
		"notCreated": notCreated,
	}, nil
}

// importOne persiste un mensaje: lee el blob, lo parsea, lo copia a su clave
// definitiva y guarda el sobre. Es la MISMA ruta que usa la entrega SMTP, sin la
// parte que solo tiene sentido en una entrega (SPF/DKIM/DMARC, reglas de usuario,
// autorespuestas): un correo que ya se recibió hace tres años en otro proveedor
// no se vuelve a autenticar ni dispara una respuesta de vacaciones.
func (a *API) importOne(ctx context.Context, t tenancy.Tenant, acc domain.Account, em importEmail) (domain.Message, error) {
	key, ok := blobKey(em.BlobID)
	if !ok {
		return domain.Message{}, errInvalidArguments("blobId inválido")
	}
	mailboxID, err := firstMailbox(em.MailboxIDs)
	if err != nil {
		return domain.Message{}, err
	}
	// El buzón tiene que ser de ESTA cuenta: sin la comprobación, un id ajeno
	// colaría un mensaje en el buzón de otra persona.
	mb, err := a.store.MailboxByID(ctx, acc.ID, mailboxID)
	if err != nil {
		return domain.Message{}, err
	}

	rc, err := a.blobs.Get(ctx, t, key)
	if err != nil {
		return domain.Message{}, fmt.Errorf("jmap: leer el blob a importar: %w", err)
	}
	defer func() { _ = rc.Close() }()
	rawMsg, err := io.ReadAll(io.LimitReader(rc, a.cfg.MaxMessageBytes+1))
	if err != nil {
		return domain.Message{}, fmt.Errorf("jmap: leer el blob a importar: %w", err)
	}
	if int64(len(rawMsg)) > a.cfg.MaxMessageBytes {
		return domain.Message{}, errInvalidArguments("el mensaje supera el tamaño máximo")
	}

	parsed, err := mime.Parse(bytes.NewReader(rawMsg))
	if err != nil {
		return domain.Message{}, fmt.Errorf("jmap: el blob no es un mensaje RFC 5322: %w", err)
	}

	// Clave definitiva: la compartida por contenido (internal/blobs). El blob de
	// subida es temporal y se borra abajo.
	msgKey, err := blobs.GuardarMensaje(ctx, a.store, a.blobs, t, rawMsg)
	if err != nil {
		return domain.Message{}, fmt.Errorf("jmap: guardar el mensaje importado: %w", err)
	}

	m := parsed.Envelope
	m.AccountID = acc.ID
	m.MailboxID = mb.ID
	m.BlobKey = msgKey
	m.SizeBytes = int64(len(rawMsg))
	m.ReceivedAt = importReceivedAt(em.ReceivedAt, m)
	m.Flags = importFlags(em.Keywords)

	if err := a.store.InsertMessage(ctx, &m); err != nil {
		// Sin compensación en S3: el blob es compartido (otra fila puede usarlo) y
		// sin referencias lo recoge el barrido de huérfanos.
		return domain.Message{}, err
	}
	// El blob de subida ya cumplió: dejarlo sería duplicar cada mensaje migrado en
	// el almacenamiento de la organización (y en su factura).
	_ = a.blobs.Delete(ctx, t, key)
	return m, nil
}

// importReceivedAt decide la fecha de recepción: la que pide quien importa, la
// del propio mensaje o, en último caso, ahora. Nunca se pierde el orden real de
// un buzón migrado por no mirar la cabecera.
func importReceivedAt(want string, m domain.Message) time.Time {
	if s := strings.TrimSpace(want); s != "" {
		if ts, err := time.Parse(time.RFC3339, s); err == nil {
			return ts.UTC()
		}
	}
	if !m.SentAt.IsZero() {
		return m.SentAt.UTC()
	}
	if !m.ReceivedAt.IsZero() {
		return m.ReceivedAt.UTC()
	}
	return time.Now().UTC()
}

// importFlags traduce los keywords JMAP a los flags del mensaje. Sin keywords, el
// mensaje entra como NO leído: es lo que espera quien migra un buzón con correo
// pendiente, y marcarlo leído por defecto escondería trabajo real.
func importFlags(keywords map[string]bool) []string {
	flags := make([]string, 0, len(keywords))
	for kw, on := range keywords {
		if !on {
			continue
		}
		flags = append(flags, keywordToFlag(strings.ToLower(kw)))
	}
	return flags
}

// firstMailbox extrae el buzón destino de mailboxIds.
func firstMailbox(ids map[string]bool) (uuid.UUID, error) {
	for id, on := range ids {
		if !on {
			continue
		}
		mid, err := uuid.Parse(id)
		if err != nil {
			return uuid.Nil, errInvalidArguments("mailboxIds contiene un identificador inválido")
		}
		return mid, nil
	}
	return uuid.Nil, errInvalidArguments("falta el buzón de destino (mailboxIds)")
}

// errInvalidArguments es el error JMAP canónico de argumentos inválidos.
func errInvalidArguments(detail string) error {
	return &jmapError{Type: "invalidArguments", Description: detail}
}

// httpx503 señala que falta una pieza del despliegue (no es culpa de quien llama).
func httpx503(detail string) error {
	return &jmapError{Type: "serverFail", Description: detail}
}

// jmapError es un error de método con el tipo que exige el protocolo.
type jmapError struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

func (e *jmapError) Error() string {
	if e.Description == "" {
		return e.Type
	}
	return e.Type + ": " + e.Description
}

// MarshalJSON deja el error listo para viajar en la respuesta JMAP.
func (e *jmapError) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]string{"type": e.Type, "description": e.Description})
}
