// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	stdmime "mime"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/auth"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// Outbound acepta correo saliente del MSA/JMAP, lo firma con DKIM, lo guarda en
// la carpeta Sent y lo encola en PostgreSQL para entrega diferida.
type Outbound struct {
	store    *store.Store
	blobs    *objectstore.Client
	signer   *auth.Signer
	logger   *slog.Logger
	maxBytes int64
	// quotaBytes es la cuota con la que se aprovisiona un buzón nuevo por esta
	// vía. Tiene que ser la MISMA que usa el alta por webmail, o el buzón
	// tendría un tamaño distinto según por dónde llegara el primer mensaje.
	quotaBytes int64
	// smime firma el correo con el certificado del buzón. Nil ⇒ el correo sale
	// sin firmar, exactamente como antes de que existiera esta pieza.
	smime SMIMESigner
}

// signSMIME firma el mensaje si hay con qué. Nunca falla hacia arriba: si algo
// va mal, el correo sale sin firma y queda constancia en el log.
func (out *Outbound) signSMIME(ctx context.Context, raw []byte, msg SubmitMessage) []byte {
	if out.smime == nil {
		return raw
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return raw
	}
	resolved, err := out.store.LookupAddress(ctx, msg.From)
	if err != nil {
		// Cuenta aún sin aprovisionar: se firmará a partir del siguiente envío,
		// cuando ya exista el buzón al que emitirle el certificado.
		return raw
	}
	return out.smime.SignMessage(ctx, t.Slug, t.Slug, resolved.AccountID, msg.From, "", raw)
}

// DefaultQuotaBytes es la cuota de un buzón recién aprovisionado cuando nadie
// configura otra cosa. Ya NO se usa directamente: la cuota efectiva entra por
// parámetro desde la configuración (DEFAULT_MAILBOX_QUOTA_GB), para que un
// servidor pequeño pueda bajarla sin recompilar. Se conserva como defecto
// único, de modo que las dos vías de alta —webmail y SMTP— no diverjan.
const DefaultQuotaBytes = int64(10) * 1024 * 1024 * 1024

// NewOutbound construye el emisor saliente.
// SMIMESigner firma un mensaje saliente con el certificado del buzón. Devuelve
// el mensaje TAL CUAL si no hay identidad: un fallo al firmar no puede dejar a
// nadie sin poder enviar correo.
type SMIMESigner interface {
	SignMessage(ctx context.Context, tenant, org string, accountID uuid.UUID, address, displayName string, raw []byte) []byte
}

// WithSMIME activa la firma S/MIME del correo saliente.
func (out *Outbound) WithSMIME(s SMIMESigner) *Outbound {
	out.smime = s
	return out
}

func NewOutbound(st *store.Store, blobs *objectstore.Client, signer *auth.Signer, logger *slog.Logger, maxBytes, quotaBytes int64) *Outbound {
	if quotaBytes <= 0 {
		quotaBytes = DefaultQuotaBytes
	}
	return &Outbound{store: st, blobs: blobs, signer: signer, logger: logger, maxBytes: maxBytes, quotaBytes: quotaBytes}
}

// SubmitMessage es un mensaje saliente listo para encolar (desde SMTP o JMAP).
type SubmitMessage struct {
	Subject    string
	From       string
	Recipients []string
	Body       io.Reader
}

// Submit firma el mensaje (DKIM), lo persiste en Sent y lo encola por dominio
// de destinatario. Debe ejecutarse con el tenant ya fijado en el contexto.
func (out *Outbound) Submit(ctx context.Context, msg SubmitMessage) error {
	p, err := out.prepare(ctx, msg)
	if err != nil {
		return err
	}

	// Guardar copia en Sent (el .eml firmado ya está en S3).
	sentMbox, err := out.store.MailboxByRole(ctx, p.accountID, domain.RoleSent)
	if err != nil {
		return fmt.Errorf("delivery: localizar Sent: %w", err)
	}
	env := p.env
	env.MailboxID = sentMbox.ID
	env.Flags = []string{domain.FlagSeen}
	if err := out.store.InsertMessage(ctx, &env); err != nil {
		_ = out.blobs.Delete(ctx, p.tenant, p.blobKey)
		return fmt.Errorf("delivery: persistir saliente: %w", err)
	}

	// Encolar por dominio de destinatario para reintento independiente.
	for domainName, rcpts := range groupByDomain(msg.Recipients) {
		o := &domain.OutboundMessage{
			AccountID:  p.accountID,
			MessageID:  env.ID,
			From:       msg.From,
			Recipients: rcpts,
			BlobKey:    p.blobKey,
		}
		if err := out.store.Enqueue(ctx, o); err != nil {
			return fmt.Errorf("delivery: encolar para %s: %w", domainName, err)
		}
	}
	return nil
}

// Schedule DIFIERE un envío (programado o deshacer): firma + sube el .eml y
// encola UNA fila RETENIDA con instante de liberación releaseAt (hold =
// HoldScheduled | HoldUndo). NO crea copia en Sent ni divide por dominio: eso lo
// hace el worker al LIBERARLA (PromoteDueHeld), reusando la misma entrega/reintento
// que un envío normal. Persistir en la cola (no en el navegador) garantiza que la
// ventana de deshacer no pierda el correo ante un cierre/caída del cliente.
// Devuelve el id del diferido (para cancelar/editar/deshacer). tenant en el ctx.
func (out *Outbound) Schedule(ctx context.Context, from string, recipients []string, raw []byte, releaseAt time.Time, hold string) (uuid.UUID, error) {
	p, err := out.prepare(ctx, SubmitMessage{From: from, Recipients: recipients, Body: bytes.NewReader(raw)})
	if err != nil {
		return uuid.Nil, err
	}
	o := &domain.OutboundMessage{
		AccountID:  p.accountID,
		From:       from,
		Recipients: recipients,
		BlobKey:    p.blobKey,
		Subject:    p.env.Subject,
	}
	id, err := out.store.EnqueueHeld(ctx, o, releaseAt, hold)
	if err != nil {
		_ = out.blobs.Delete(ctx, p.tenant, p.blobKey)
		return uuid.Nil, err
	}
	return id, nil
}

// prepared es el resultado común de firmar+subir un saliente antes de encolarlo.
type prepared struct {
	tenant    tenancy.Tenant
	accountID uuid.UUID
	blobKey   string
	env       domain.Message // sobre parseado (con AccountID/BlobKey/SizeBytes fijados)
}

// prepare lee (con límite de tamaño), firma DKIM, resuelve la cuenta emisora,
// parsea el sobre y sube el .eml firmado a S3. Núcleo compartido por Submit
// (inmediato) y Schedule (diferido).
func (out *Outbound) prepare(ctx context.Context, msg SubmitMessage) (prepared, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return prepared{}, fmt.Errorf("delivery: sin tenant en submit: %w", err)
	}
	raw, err := io.ReadAll(io.LimitReader(msg.Body, out.maxBytes+1))
	if err != nil {
		return prepared{}, fmt.Errorf("delivery: leer saliente: %w", err)
	}
	if int64(len(raw)) > out.maxBytes {
		return prepared{}, domain.ErrMessageTooLarge
	}
	// Resolver la cuenta emisora por su dirección (= identidad autenticada).
	//
	// Si todavía no existe, se APROVISIONA aquí con el subject autenticado. El
	// buzón se creaba solo al abrir JMAP (`EnsureAccount` en la sesión), así que
	// quien estrenaba su cuenta desde un cliente de escritorio por SMTP —o
	// cualquier remitente de servicio que nunca abre el webmail— recibía
	// «cuenta emisora desconocida» y no podía enviar. La evidencia es la misma
	// que usa JMAP para provisionar: un token válido de Sytue ID cuyo email
	// coincide con la identidad presentada, ya verificado por el MSA.
	resolved, err := out.store.LookupAddress(ctx, msg.From)
	if errors.Is(err, domain.ErrNoSuchUser) && msg.Subject != "" {
		acc, provErr := out.store.EnsureAccount(ctx, msg.Subject, msg.From, "", out.quotaBytes)
		if provErr != nil {
			return prepared{}, fmt.Errorf("delivery: aprovisionar cuenta emisora: %w", provErr)
		}
		// Solo AccountID se usa aguas abajo; el tenant ya viene del contexto.
		resolved = store.ResolvedAddress{AccountID: acc.ID}
		err = nil
	}
	if err != nil {
		return prepared{}, fmt.Errorf("delivery: cuenta emisora desconocida: %w", err)
	}
	// EL `From:` QUE VE QUIEN RECIBE. Hasta aquí solo se ha autorizado el remitente
	// de SOBRE (el MSA exige que sea la identidad autenticada; JMAP, que sea una
	// identidad concedida). La cabecera la escribe el cliente, y es la que se
	// enseña y la que firma el relé: sin contrastarla, cualquier cuenta mandaba
	// correo «de» otra persona con el DKIM del dominio. Va ANTES de firmar y por
	// aquí pasan los cuatro caminos de envío (inmediato, programado, SMTP y
	// automáticos).
	if err := exigirFromDeLaCuenta(ctx, out.store, raw, msg.From, resolved.AccountID); err != nil {
		return prepared{}, err
	}
	// S/MIME primero y DKIM después, en ese orden y no al revés: DKIM firma el
	// mensaje TAL Y COMO SALE, y envolverlo en un multipart/signed después de
	// firmarlo con DKIM invalidaría esa firma en el primer salto.
	raw = out.signSMIME(ctx, raw, msg)

	// Firmar DKIM (no-op si la firma está desactivada en dev).
	signed, err := out.signer.Sign(raw)
	if err != nil {
		return prepared{}, fmt.Errorf("delivery: firmar DKIM: %w", err)
	}
	parsed, _ := mime.Parse(bytes.NewReader(signed))
	env := parsed.Envelope
	env.AccountID = resolved.AccountID
	blobKey := fmt.Sprintf("messages/%s/%s.eml", resolved.AccountID, uuid.NewString())
	if err := out.blobs.Put(ctx, t, blobKey, bytes.NewReader(signed), int64(len(signed)),
		objectstore.PutOptions{ContentType: "message/rfc822"}); err != nil {
		return prepared{}, fmt.Errorf("delivery: subir saliente: %w", err)
	}
	env.BlobKey = blobKey
	env.SizeBytes = int64(len(signed))
	return prepared{tenant: t, accountID: resolved.AccountID, blobKey: blobKey, env: env}, nil
}

// resolvedorDeDirecciones es lo único que la verja del `From:` necesita del store.
type resolvedorDeDirecciones interface {
	LookupAddress(ctx context.Context, address string) (store.ResolvedAddress, error)
}

// exigirFromDeLaCuenta comprueba que la cabecera `From:` del mensaje es una
// dirección de la MISMA cuenta que el remitente de sobre ya autorizado: su
// principal o uno de sus alias. Exactamente una cabecera y exactamente una
// dirección: con dos, cada receptor enseña la que quiere.
//
// Devuelve domain.ErrFromNoAutorizado; el MSA lo traduce a 550 y la API a 403.
func exigirFromDeLaCuenta(ctx context.Context, r resolvedorDeDirecciones, raw []byte, sobre string, cuenta uuid.UUID) error {
	de, err := fromDeCabecera(raw)
	if err != nil {
		return err
	}
	if strings.EqualFold(de, sobre) {
		return nil
	}
	otra, err := r.LookupAddress(ctx, de)
	if errors.Is(err, domain.ErrNoSuchUser) {
		return domain.ErrFromNoAutorizado
	}
	if err != nil {
		return fmt.Errorf("delivery: resolver el From de la cabecera: %w", err)
	}
	if otra.AccountID != cuenta {
		return domain.ErrFromNoAutorizado
	}
	return nil
}

// fromDeCabecera devuelve la única dirección de la única cabecera `From:`.
func fromDeCabecera(raw []byte) (string, error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", domain.ErrFromNoAutorizado
	}
	if len(m.Header["From"]) != 1 {
		return "", domain.ErrFromNoAutorizado
	}
	lista, err := lectorDeFrom.ParseList(m.Header.Get("From"))
	if err != nil || len(lista) != 1 {
		return "", domain.ErrFromNoAutorizado
	}
	return lista[0].Address, nil
}

// lectorDeFrom lee la cabecera sin tropezar en el juego de caracteres del NOMBRE
// visible: aquí solo importa la dirección, y rechazar un envío porque el cliente
// codificó el nombre en windows-1252 sería romper el correo de quien no hace nada malo.
var lectorDeFrom = mail.AddressParser{WordDecoder: &stdmime.WordDecoder{
	CharsetReader: func(_ string, r io.Reader) (io.Reader, error) { return r, nil },
}}

// SubmitRaw encola un mensaje ya construido (usado por JMAP Email/send con un
// blob previamente subido). tenant viene del contexto.
func (out *Outbound) SubmitRaw(ctx context.Context, from string, recipients []string, body []byte) error {
	return out.Submit(ctx, SubmitMessage{From: from, Recipients: recipients, Body: bytes.NewReader(body)})
}

// SubmitAuto encola una AUTORESPUESTA (vacaciones/RFC 3834): firma + sube el .eml y
// lo encola por dominio de destinatario, reusando la misma preparación y cola que
// un envío normal, pero SIN copia en Enviados (una autorespuesta automática no
// ensucia la carpeta del usuario). tenant viene del contexto (el del destinatario
// original, fijado por la entrega entrante).
func (out *Outbound) SubmitAuto(ctx context.Context, from string, recipients []string, raw []byte) error {
	p, err := out.prepare(ctx, SubmitMessage{From: from, Recipients: recipients, Body: bytes.NewReader(raw)})
	if err != nil {
		return err
	}
	for domainName, rcpts := range groupByDomain(recipients) {
		o := &domain.OutboundMessage{
			AccountID:  p.accountID,
			From:       from,
			Recipients: rcpts,
			BlobKey:    p.blobKey,
		}
		if err := out.store.Enqueue(ctx, o); err != nil {
			return fmt.Errorf("delivery: encolar autorespuesta para %s: %w", domainName, err)
		}
	}
	return nil
}

// groupByDomain agrupa destinatarios por dominio (un lote de cola por dominio).
func groupByDomain(rcpts []string) map[string][]string {
	out := make(map[string][]string)
	for _, r := range rcpts {
		d := domainOf(r)
		out[d] = append(out[d], r)
	}
	return out
}
