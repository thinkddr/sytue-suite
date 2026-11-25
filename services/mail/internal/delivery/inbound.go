// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package delivery orquesta el flujo de correo de Sytue Mail: recepción
// (verificación SPF/DKIM/DMARC, parseo MIME, persistencia PG + S3, entrega al
// buzón) y envío (firma DKIM, encolado en PostgreSQL y workers de entrega con
// reintentos). Es la capa que une store + objectstore + mime + auth.
package delivery

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/semaphore"

	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/auth"
	"git.sytue.eu/sytue/one/services/mail/internal/blobs"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/mime"
	"git.sytue.eu/sytue/one/services/mail/internal/push"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// ErrRejectedPolicy indica rechazo por política (DMARC=reject configurado).
var ErrRejectedPolicy = errors.New("delivery: rechazado por política DMARC")

// Inbound entrega correo entrante del MX a los buzones locales.
type Inbound struct {
	store          *store.Store
	blobs          blobs.Almacen
	privateObjects objectstore.PrivateIngressObjects
	resolver       auth.Resolver
	autoSender     autoSender
	logger         *slog.Logger
	channels       *ChannelBridge
	calendario     *PuenteCalendario
	hubNotifier    *hubnotify.Client
	mailPublicURL  string
	// autoridad y planes deciden si el correo de una cuenta espera al traspaso a
	// la bóveda (aplazar.go). Sin autoridad no se aplaza nada.
	autoridad AutoridadCorreo
	planes    planesDeEntrada
	// authservID firma la cabecera Authentication-Results que el MX antepone
	// (RFC 8601): el FQDN del MTA.
	authservID string

	maxBytes int64
	// Cupos de la recepción: cuántas a la vez y cuántos bytes en memoria. Ver
	// recibir y cupos.
	recepciones *semaphore.Weighted
	memoria     *semaphore.Weighted
	cuposUnaVez sync.Once

	verifySPF    bool
	verifyDKIM   bool
	enforceDMARC bool
	rejectOnFail bool
}

// InboundConfig configura la entrega entrante.
type InboundConfig struct {
	MaxBytes     int64
	VerifySPF    bool
	VerifyDKIM   bool
	EnforceDMARC bool
	RejectOnFail bool
	Channels     *ChannelBridge
	// Calendario pasa a Sytue Calendar los REPLY de invitaciones (imip.go); nil, no.
	Calendario    *PuenteCalendario
	HubNotifier   *hubnotify.Client
	MailPublicURL string
	// AuthServID es el nombre del MX en la cabecera Authentication-Results.
	AuthServID string
	// Autoridad es Rooms: con ella, el correo para un plan de entrada de Matrix
	// que ya nadie reclama se aplaza en vez de quedar ilegible (aplazar.go). Nil,
	// como antes.
	Autoridad AutoridadCorreo
}

// NewInbound construye el entregador entrante. `auto` despacha las autorespuestas
// de vacaciones (nil las desactiva, p. ej. en dev sin saliente).
func NewInbound(st *store.Store, objetos *objectstore.Client, resolver auth.Resolver, auto autoSender, logger *slog.Logger, cfg InboundConfig) *Inbound {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	var privateObjects objectstore.PrivateIngressObjects
	if objetos != nil {
		privateObjects = objetos
	}
	var planes planesDeEntrada
	if st != nil {
		planes = st
	}
	return &Inbound{
		autoridad: cfg.Autoridad, planes: planes,
		store: st, blobs: objetos, privateObjects: privateObjects, resolver: resolver, autoSender: auto, logger: logger,
		channels:      cfg.Channels,
		calendario:    cfg.Calendario,
		hubNotifier:   cfg.HubNotifier,
		mailPublicURL: cfg.MailPublicURL,
		authservID:    cfg.AuthServID,
		maxBytes:      cfg.MaxBytes,
		verifySPF:     cfg.VerifySPF,
		verifyDKIM:    cfg.VerifyDKIM,
		enforceDMARC:  cfg.EnforceDMARC,
		rejectOnFail:  cfg.RejectOnFail,
	}
}

// HitRate incrementa y devuelve el contador de rate-limit de la IP (anti-abuso).
func (in *Inbound) HitRate(ctx context.Context, ip string) (int, error) {
	return in.store.HitInboundRate(ctx, ip)
}

// HitAuthFail incrementa y devuelve el contador de FALLOS de autenticación de la
// IP. Lo usa submission tras rechazar unas credenciales.
func (in *Inbound) HitAuthFail(ctx context.Context, ip string) (int, error) {
	return in.store.HitAuthFailRate(ctx, ip)
}

// AuthFailRate devuelve el contador de fallos de la IP sin incrementarlo, para
// decidir si ni siquiera se intenta validar las credenciales.
func (in *Inbound) AuthFailRate(ctx context.Context, ip string) (int, error) {
	return in.store.AuthFailRate(ctx, ip)
}

// Resolve comprueba que una dirección es local (anti-relay en RCPT).
//
// Una dirección de canal de Sytue Rooms no tiene buzón: se acepta aquí sin consultar la
// base porque quien decide si el token existe es Rooms, en la entrega. Rechazarla en RCPT
// exigiría preguntárselo a Rooms por cada destinatario de cada correo.
func (in *Inbound) Resolve(ctx context.Context, address string) (store.ResolvedAddress, error) {
	if _, ok := in.channels.Token(address); ok {
		return store.ResolvedAddress{Address: address}, nil
	}
	return in.store.LookupAddress(ctx, address)
}

// InboundMessage es un mensaje recibido por el MX, pendiente de entregar.
type InboundMessage struct {
	RemoteIP   net.IP
	MailFrom   string
	Recipients []string
	Body       io.Reader
}

// Deliver verifica el remitente, parsea el MIME, sube el .eml a S3 y guarda el
// sobre en el Inbox de cada destinatario local. Se llama desde la fase DATA del MX.
func (in *Inbound) Deliver(ctx context.Context, msg InboundMessage) error {
	r, soltar, err := in.recibir(ctx, msg.Body)
	if err != nil {
		return err
	}
	defer soltar()
	seccion := r.section

	env, err := mime.ParseSobre(seccion(), r.Size())
	if err != nil {
		return fmt.Errorf("delivery: parsear: %w", err)
	}

	// --- Autenticación del remitente (SPF/DKIM/DMARC) ---
	spf, dkimRes, dmarc := in.authenticate(ctx, msg, seccion(), env.FromAddr)
	env.SPFResult = spf.ToDomain()
	env.DKIMResult = dkimRes.ToDomain()
	env.DMARCResult = dmarc.Result.ToDomain()

	if in.enforceDMARC && dmarc.Disposition == "reject" && dmarc.Result == auth.ResultFail {
		if in.rejectOnFail {
			in.logger.Warn("inbound: rechazado por DMARC=reject",
				"from", logsafe.Text(env.FromAddr), "ip", msg.RemoteIP.String())
			return ErrRejectedPolicy
		}
	}
	// Lo que se guarda lleva delante el veredicto de este MX (RFC 8601): los
	// clientes IMAP y el .eml descargado lo ven igual que el lector web, que lo
	// lee de la base. Va antes que cualquier cabecera del remitente, que es lo
	// que la RFC pide para distinguir el nuestro de uno falsificado más abajo.
	cabecera := auth.CabeceraAuthenticationResults(in.authservID, spf, dkimRes, dmarc.Result,
		domainOf(msg.MailFrom), domainOf(env.FromAddr))
	r.SectionReader = conPrefijo(cabecera, r.SectionReader)

	junk := in.enforceDMARC && dmarc.Result == auth.ResultFail &&
		(dmarc.Disposition == "quarantine" || dmarc.Disposition == "reject")

	// Gate RFC 3834 (una vez por mensaje): ¿puede este correo disparar una
	// autorespuesta de vacaciones? Depende solo de sus cabeceras + remitente de
	// sobre, no del destinatario. La comprobación «uno mismo» y de vacaciones
	// activas la hace maybeAutoReply por destinatario.
	autoEligible := autoReplyEligibleReader(seccion(), msg.MailFrom)

	// Entregar una copia a cada destinatario local (fan-out).
	// UN objeto por mensaje y organización, compartido por todos sus buzones que
	// lo reciban (ver internal/blobs).
	claves := map[string]string{}
	guardar := func(ctx context.Context, t tenancy.Tenant, registry blobs.Registro) (string, error) {
		if k, ok := claves[t.Slug]; ok {
			return k, nil
		}
		k, err := blobs.GuardarMensajeDesde(ctx, registry, in.blobs, t, seccion(), r.Size())
		if err != nil {
			return "", fmt.Errorf("delivery: guardar blob: %w", err)
		}
		claves[t.Slug] = k
		return k, nil
	}
	for _, rcpt := range msg.Recipients {
		if err := in.deliverTo(ctx, rcpt, r, guardar, env, junk, msg.MailFrom, autoEligible, dmarc.Alineado); err != nil {
			return err // el MX devolverá 4xx y el remitente reintentará todo el lote
		}
	}
	return nil
}

// inboundBody retains one admitted RAM allocation. Independent sections let
// MIME, DKIM, channel/iMIP adapters and blob uploads stream without copying the
// complete message. Only the synchronous private seal borrows the original view;
// release clears the complete reserved allocation, including after errors.
type inboundBody struct {
	*io.SectionReader
	raw []byte
}

func (b *inboundBody) section() *io.SectionReader {
	return io.NewSectionReader(b.SectionReader, 0, b.Size())
}

// Cupos de recepción (COM-06): el MX admite remitentes sin credenciales, por lo
// que la memoria se reserva antes de leer. El presupuesto compartido equivale al
// máximo configurado (50 MiB por defecto); nunca se escribe MIME en /tmp.
const (
	recepcionesALaVez = 16
	esperaDeMemoria   = 2 * time.Minute
)

var errOcupado = errors.New("delivery: demasiadas recepciones a la vez")

// receiveCapacity only trusts readers whose exact remaining extent is owned by
// the standard library. Arbitrary Len methods cannot reduce admission: an
// untrusted reader could otherwise claim zero and return an unbounded body.
func receiveCapacity(body io.Reader, limit int64) int64 {
	switch reader := body.(type) {
	case *bytes.Reader:
		return min(int64(reader.Len()), limit)
	case *strings.Reader:
		return min(int64(reader.Len()), limit)
	default:
		return limit
	}
}

type inboundContextReader struct {
	ctx  context.Context
	body io.Reader
}

func (r inboundContextReader) Read(out []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.body.Read(out)
}

// recibir reserves the entire allocation before reading or parsing. Unknown
// lengths reserve the maximum; known bounded readers allow small messages to
// coexist. io.ReadFull never grows the allocation beyond that reservation. The
// single-byte overflow probe adds fixed overhead, bounded by recepcionesALaVez.
func (in *Inbound) recibir(ctx context.Context, cuerpo io.Reader) (result *inboundBody, soltar func(), err error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if cuerpo == nil || in.maxBytes <= 0 {
		return nil, nil, domain.ErrMessageTooLarge
	}
	recepciones, memoria := in.cupos()
	if !recepciones.TryAcquire(1) {
		return nil, nil, errOcupado
	}
	defer recepciones.Release(1)

	capacity := receiveCapacity(cuerpo, in.maxBytes)
	espera, cancelar := context.WithTimeout(ctx, esperaDeMemoria)
	defer cancelar()
	if err := memoria.Acquire(espera, capacity); err != nil {
		return nil, nil, fmt.Errorf("%w: sin memoria libre: %w", errOcupado, err)
	}
	raw := make([]byte, int(capacity))
	allocated := raw
	var once sync.Once
	release := func() { once.Do(func() { clear(allocated); memoria.Release(capacity) }) }
	success := false
	defer func() {
		if !success {
			release()
		}
	}()

	stop := func() bool { return true }
	if closer, ok := cuerpo.(io.ReadCloser); ok {
		stop = context.AfterFunc(ctx, func() { _ = closer.Close() })
	}
	defer stop()
	reader := inboundContextReader{ctx: ctx, body: cuerpo}
	n, readErr := io.ReadFull(reader, raw)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return nil, nil, fmt.Errorf("delivery: leer cuerpo: %w", errors.Join(readErr, ctx.Err()))
	}
	if readErr == nil {
		var excess [1]byte
		if count, readErr := io.ReadFull(reader, excess[:]); count != 0 {
			clear(excess[:])
			return nil, nil, domain.ErrMessageTooLarge
		} else if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, nil, fmt.Errorf("delivery: leer cuerpo: %w", errors.Join(readErr, ctx.Err()))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	raw = raw[:n]
	success = true
	return &inboundBody{SectionReader: io.NewSectionReader(bytes.NewReader(raw), 0, int64(n)), raw: raw}, release, nil
}

// cupos crea los dos presupuestos al primer uso, para que un Inbound literal (los
// tests) valga igual que el de NewInbound.
func (in *Inbound) cupos() (recepciones, memoria *semaphore.Weighted) {
	in.cuposUnaVez.Do(func() {
		in.recepciones = semaphore.NewWeighted(recepcionesALaVez)
		in.memoria = semaphore.NewWeighted(max(in.maxBytes, 1))
	})
	return in.recepciones, in.memoria
}

func (in *Inbound) authenticate(ctx context.Context, msg InboundMessage, r io.Reader, fromAddr string) (auth.Result, auth.Result, auth.DMARCResult) {
	fromDomains, r := dominiosFromDeCabecera(r, fromAddr)
	mailFromDomain := domainOf(msg.MailFrom)

	spf := auth.ResultNone
	if in.verifySPF {
		spf = auth.VerifySPF(ctx, in.resolver, msg.RemoteIP, mailFromDomain)
	}
	dkimRes := auth.ResultNone
	// Los dominios que de verdad firmaron (`d=`). NUNCA el del From: darlo por
	// supuesto dejaba pasar DMARC a cualquier correo firmado por quien fuera.
	var dkimDomains []string
	if in.verifyDKIM {
		var err error
		dkimRes, dkimDomains, err = auth.VerifyDKIMReader(ctx, in.resolver, r)
		if err != nil {
			dkimRes, dkimDomains = auth.ResultTempError, nil
		}
	}
	if len(fromDomains) > 1 {
		if in.enforceDMARC {
			return spf, dkimRes, auth.VerifyDMARCFroms(ctx, in.resolver, fromDomains, mailFromDomain, spf, dkimDomains, dkimRes)
		}
		return spf, dkimRes, auth.DMARCResult{Result: auth.ResultFail, Disposition: "none", Alineado: false}
	}
	fromDomain := domainOf(fromAddr)
	if len(fromDomains) == 1 && fromDomains[0] != "" {
		fromDomain = fromDomains[0]
	}
	dmarc := auth.DMARCResult{
		Result: auth.ResultNone, Disposition: "none",
		Alineado: auth.Alineado(fromDomain, mailFromDomain, spf, dkimDomains, dkimRes, false, false),
	}
	if in.enforceDMARC && fromDomain != "" {
		dmarc = auth.VerifyDMARC(ctx, in.resolver, fromDomain, mailFromDomain, spf, dkimDomains, dkimRes)
	}
	return spf, dkimRes, dmarc
}

// dominiosFromDeCabecera devuelve un dominio por cada cabecera `From:` y por cada
// dirección que contenga (COM-05). Así un mensaje con dos cabeceras `From:` —o dos
// direcciones en `From:`— nunca se evalúa solo por la primera permitiendo que la
// segunda suplante a otro remitente. Rebobina r (o lo reconstruye) para que DKIM lea
// el mensaje íntegro desde el principio.
func dominiosFromDeCabecera(r io.Reader, fromAddr string) ([]string, io.Reader) {
	if r == nil {
		if d := domainOf(fromAddr); d != "" {
			return []string{d}, nil
		}
		return nil, nil
	}
	var m *mail.Message
	var err error
	if rs, ok := r.(io.ReadSeeker); ok {
		_, _ = rs.Seek(0, io.SeekStart)
		m, err = mail.ReadMessage(rs)
		_, _ = rs.Seek(0, io.SeekStart)
	} else {
		raw, rerr := io.ReadAll(r)
		if rerr != nil {
			return []string{domainOf(fromAddr)}, bytes.NewReader(raw)
		}
		m, err = mail.ReadMessage(bytes.NewReader(raw))
		r = bytes.NewReader(raw)
	}
	if err != nil || len(m.Header["From"]) == 0 {
		if d := domainOf(fromAddr); d != "" {
			return []string{d}, r
		}
		return nil, r
	}
	cabeceras := m.Header["From"]
	out := make([]string, 0, len(cabeceras))
	for _, h := range cabeceras {
		lista, perr := lectorDeFrom.ParseList(h)
		if perr != nil || len(lista) == 0 {
			if dir := rescatarAngulos(h); dir != "" {
				out = append(out, domainOf(dir))
			} else {
				out = append(out, domainOf(fromAddr))
			}
			continue
		}
		for _, a := range lista {
			out = append(out, domainOf(a.Address))
		}
	}
	return out, r
}

func rescatarAngulos(valor string) string {
	valor = strings.TrimSpace(valor)
	if i, j := strings.IndexByte(valor, '<'), strings.IndexByte(valor, '>'); i >= 0 && j > i {
		return strings.TrimSpace(valor[i+1 : j])
	}
	if strings.Contains(valor, "@") && !strings.ContainsAny(valor, " \t\r\n\"") {
		return valor
	}
	return ""
}

// fromAutenticado: el `From:` quedó autenticado por SPF o DKIM alineados. Solo
// entonces se pasa a Calendar la respuesta a una invitación que traiga el correo.
func (in *Inbound) deliverTo(ctx context.Context, rcpt string, body *inboundBody, guardar func(context.Context, tenancy.Tenant, blobs.Registro) (string, error), env domain.Message, junk bool, envelopeFrom string, autoEligible, fromAutenticado bool) error {
	seccion, size := body.section, body.Size()
	// Correo a un canal de Sytue Rooms: no hay buzón que abrir. Va antes del lookup
	// porque esa dirección no existe en la tabla de cuentas ni de alias.
	if token, ok := in.channels.Token(rcpt); ok {
		if esAutomaticoReader(seccion()) {
			// Una autorespuesta publicada en el canal sería la mitad de un bucle.
			in.logger.Debug("correo a canal: descartado por automático")
			return nil
		}
		return in.channels.PublishReader(ctx, token, seccion(), size)
	}
	resolved, err := in.store.LookupAddress(ctx, rcpt)
	if err != nil {
		return err
	}
	// Fijar el tenant del destinatario para todas las escrituras (RLS).
	t, err := tenancy.New(resolved.Tenant)
	if err != nil {
		return fmt.Errorf("delivery: tenant inválido %q: %w", resolved.Tenant, err)
	}
	tctx := tenancy.Into(ctx, t)
	tctx = log.With(tctx, "tenant", resolved.Tenant, "account", resolved.AccountID.String())

	role := domain.RoleInbox
	if junk {
		role = domain.RoleJunk
	}
	var m domain.Message
	silenciado := false
	private, err := in.store.AdmitInbound(tctx, resolved, role, func(admission *store.InboundLegacyAdmission) error {
		mailbox, err := admission.Mailbox(tctx, role)
		if err != nil {
			return fmt.Errorf("delivery: localizar buzón destino: %w", err)
		}
		blobKey, err := guardar(tctx, t, admission)
		if err != nil {
			return err
		}
		m = env
		m.AccountID = resolved.AccountID
		m.MailboxID = mailbox.ID
		m.BlobKey = blobKey
		m.SizeBytes = size
		// Reglas de usuario: solo sobre correo legítimo (el spam va a Junk sin filtrar).
		if junk {
			return admission.Insert(tctx, &m)
		}
		if err := in.applyAdmittedFilters(tctx, admission, &m); err != nil {
			return err
		}
		// Lo legítimo entra por InsertEntrante, que mira el hilo antes de decidir:
		// la respuesta a una conversación silenciada va al Archivo, y la de una
		// pospuesta la devuelve a la Entrada. El spam no pasa por ahí: un hilo de
		// Spam no se silencia ni se pospone.
		silenciado, err = admission.InsertEntrante(tctx, &m)
		return err
	})
	if err != nil {
		return fmt.Errorf("delivery: admitir destinatario: %w", err)
	}
	if private != nil {
		return in.persistPrivateInbound(tctx, *private, body.raw)
	}

	// Notificar a Hub/cliente (nuevo correo) vía outbox de eventos sobre Postgres.
	// El correo que va a Spam no suena en la campana: sería avisar del spam. El de
	// una conversación silenciada tampoco: silenciarla es pedir justo eso.
	in.publishDelivered(tctx, resolved.Tenant, resolved.AccountID, resolved.Subject, m, !junk && !silenciado)
	// La respuesta a una invitación de Calendar, si la trae (y no es spam).
	if !junk {
		in.calendario.PasarReader(tctx, resolved.Tenant, env, seccion(), fromAutenticado)
	}

	// Autorespuesta de vacaciones (best-effort, tras la entrega). No sobre spam
	// (el correo en Junk no merece respuesta). Responde DESDE la dirección local
	// contactada (resolved.Address: primaria o alias) al remitente de sobre.
	if !junk {
		in.maybeAutoReply(tctx, resolved.AccountID, resolved.Address, envelopeFrom, m.MessageID, autoEligible)
	}
	return nil
}

// applyAdmittedFilters uses the admitted transaction. A database failure returns
// a temporary SMTP error; no partially committed plaintext message is accepted.
func (in *Inbound) applyAdmittedFilters(ctx context.Context, admission *store.InboundLegacyAdmission, m *domain.Message) error {
	filters, err := admission.Filters(ctx)
	if err != nil {
		return err
	}
	if destination := domain.ApplyInbound(filters, m); destination != nil {
		m.MailboxID = *destination
	}
	return nil
}

func (in *Inbound) publishDelivered(ctx context.Context, tenant string, accountID uuid.UUID, recipientSubject string, m domain.Message, campana bool) {
	ev, err := events.NewEvent("mail", "mail.message.received", tenant, map[string]any{
		"account_id": accountID,
		"message_id": m.ID,
		"mailbox_id": m.MailboxID,
		"subject":    m.Subject,
		"from":       m.FromAddr,
	})
	if err != nil {
		return
	}
	if err := events.PublishTx(ctx, in.store.Pool(), ev); err != nil {
		in.logger.Warn("inbound: publicar evento fallido", "error", err)
	}
	// Y el aviso al webmail que lo tenga abierto. Es lo que hace que el correo
	// APAREZCA, en vez de esperar a que alguien vuelva a la pestaña. Best-effort:
	// si falla, el mensaje ya está guardado y se ve al refrescar.
	if err := push.Publicar(ctx, in.store.Pool(), tenant, push.Llegada{
		AccountID: accountID,
		MailboxID: m.MailboxID,
		MessageID: m.ID,
		Subject:   m.Subject,
		From:      m.FromAddr,
	}); err != nil {
		in.logger.Warn("inbound: avisar del correo nuevo fallido", "error", err)
	}
	// Notificación unificada a Sytue Hub para la campanita del shell y el buzón de notificaciones.
	if campana && in.hubNotifier != nil && recipientSubject != "" {
		in.hubNotifier.SendAsync(ctx, avisoDeLlegada(in.mailPublicURL, tenant, recipientSubject, m))
	}
	// Y la señal de cambios para el widget de Mail, aparte de la campana: sale también
	// sin aviso (spam, Mail silenciado en el Hub), porque el contador de la Entrada
	// cambia igual. Nil-safe y sin nada del mensaje.
	in.hubNotifier.AvisarCambio(ctx, tenant, "mail", recipientSubject)
}

// avisoDeLlegada es el aviso del Hub de un correo que acaba de entrar: abre SU
// conversación. El título se compone en el idioma de quien lo lee con `params`; el de
// aquí es el respaldo. Quien escribe va por su nombre si lo trae.
func avisoDeLlegada(publicURL, tenant, subject string, m domain.Message) hubnotify.Notification {
	from := cmp.Or(strings.TrimSpace(m.FromName), m.FromAddr)
	title := "Nuevo correo"
	if from != "" {
		title = "Nuevo correo de " + from
	}
	return hubnotify.Notification{
		Tenant:  tenant,
		Subject: subject,
		Source:  "mail",
		Type:    "mail.message.received",
		Level:   "info",
		Title:   title,
		Body:    m.Subject,
		URL:     hubnotify.EnlaceHilo(publicURL, tenant, m.ThreadID.String()),
		Params:  map[string]string{"from": from},
	}
}

func domainOf(addr string) string {
	if i := lastIndexByte(addr, '@'); i >= 0 && i < len(addr)-1 {
		return addr[i+1:]
	}
	return ""
}

func lastIndexByte(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// conPrefijo devuelve un SectionReader que lee `prefijo` y luego `r`, sin copiar el
// mensaje: sigue siendo un fichero temporal en flujo (COM-06).
func conPrefijo(prefijo string, r *io.SectionReader) *io.SectionReader {
	p := strings.NewReader(prefijo)
	return io.NewSectionReader(lectorConcatenado{p, r, p.Size()}, 0, p.Size()+r.Size())
}

type lectorConcatenado struct {
	a, b io.ReaderAt
	n    int64
}

func (l lectorConcatenado) ReadAt(buf []byte, off int64) (int, error) {
	leidos := 0
	if off < l.n {
		k, err := l.a.ReadAt(buf, off)
		leidos = k
		if err != nil && err != io.EOF {
			return k, err
		}
		if k == len(buf) {
			return k, nil
		}
		buf, off = buf[k:], l.n
	}
	k, err := l.b.ReadAt(buf, off-l.n)
	return leidos + k, err
}
