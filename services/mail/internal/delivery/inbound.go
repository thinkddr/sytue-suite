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
	"os"
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
	store         *store.Store
	blobs         *objectstore.Client
	resolver      auth.Resolver
	autoSender    autoSender
	logger        *slog.Logger
	channels      *ChannelBridge
	calendario    *PuenteCalendario
	hubNotifier   *hubnotify.Client
	mailPublicURL string

	maxBytes int64
	// Cupos de la recepción: cuántas a la vez (disco) y cuántos bytes en memoria. Ver
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
}

// NewInbound construye el entregador entrante. `auto` despacha las autorespuestas
// de vacaciones (nil las desactiva, p. ej. en dev sin saliente).
func NewInbound(st *store.Store, objetos *objectstore.Client, resolver auth.Resolver, auto autoSender, logger *slog.Logger, cfg InboundConfig) *Inbound {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	return &Inbound{
		store: st, blobs: objetos, resolver: resolver, autoSender: auto, logger: logger,
		channels:      cfg.Channels,
		calendario:    cfg.Calendario,
		hubNotifier:   cfg.HubNotifier,
		mailPublicURL: cfg.MailPublicURL,
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
	seccion := func() *io.SectionReader { return io.NewSectionReader(r, 0, r.Size()) }

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
	guardar := func(ctx context.Context, t tenancy.Tenant) (string, error) {
		if k, ok := claves[t.Slug]; ok {
			return k, nil
		}
		k, err := blobs.GuardarMensajeDesde(ctx, in.store, in.blobs, t, seccion(), r.Size())
		if err != nil {
			return "", fmt.Errorf("delivery: guardar blob: %w", err)
		}
		claves[t.Slug] = k
		return k, nil
	}
	for _, rcpt := range msg.Recipients {
		if err := in.deliverTo(ctx, rcpt, seccion, r.Size(), guardar, env, junk, msg.MailFrom, autoEligible, dmarc.Alineado); err != nil {
			return err // el MX devolverá 4xx y el remitente reintentará todo el lote
		}
	}
	return nil
}

// Cupos de la recepción (COM-06). Los dos acotan lo que un remitente cualquiera —el MX
// no pide credenciales— puede hacerle gastar al pod.
const (
	// recepcionesALaVez acota el DISCO: cada recepción en curso ocupa hasta maxBytes
	// en /tmp. Con el tope de 50 MB son 800 MB, por debajo del sizeLimit del chart.
	// ponytail: cupo global; por IP si un remitente lento llega a acapararlo.
	recepcionesALaVez = 16
	// esperaDeMemoria es lo que un mensaje ya recibido aguarda su turno de memoria
	// antes de contestar 451: menos que los 10 min que el remitente espera el 250,
	// para que no reintente algo que acabaríamos entregando (duplicado).
	esperaDeMemoria = 2 * time.Minute
)

// errOcupado: no hay hueco ahora. El MX lo traduce a 451 y el remitente reintenta.
var errOcupado = errors.New("delivery: demasiadas recepciones a la vez")

// recibir lee el mensaje SIN tenerlo en memoria mientras llega y devuelve un
// SectionReader sobre el fichero temporal (ya desenlazado de /tmp para no dejar
// ficheros huérfanos en el directorio), de modo que el parseo MIME, la verificación
// DKIM y la subida de blobs trabajen en flujo desde el fichero dentro del presupuesto
// compartido (COM-06).
//
// soltar cierra el fichero temporal y devuelve su cuota al presupuesto; se llama al
// acabar de procesar.
func (in *Inbound) recibir(ctx context.Context, cuerpo io.Reader) (r *io.SectionReader, soltar func(), err error) {
	recepciones, memoria := in.cupos()
	if !recepciones.TryAcquire(1) {
		return nil, nil, errOcupado
	}
	defer recepciones.Release(1)

	f, err := os.CreateTemp("", "sytue-mail-entrante-*")
	if err != nil {
		return nil, nil, fmt.Errorf("delivery: abrir temporal: %w", err)
	}
	_ = os.Remove(f.Name())
	cerrarEnError := true
	defer func() {
		if cerrarEnError {
			_ = f.Close()
		}
	}()
	n, err := io.Copy(f, io.LimitReader(cuerpo, in.maxBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("delivery: leer cuerpo: %w", err)
	}
	if n > in.maxBytes {
		return nil, nil, domain.ErrMessageTooLarge
	}

	espera, cancelar := context.WithTimeout(ctx, esperaDeMemoria)
	defer cancelar()
	if err := memoria.Acquire(espera, n); err != nil {
		return nil, nil, fmt.Errorf("%w: sin memoria libre para %d bytes", errOcupado, n)
	}
	cerrarEnError = false
	return io.NewSectionReader(f, 0, n), sync.OnceFunc(func() {
		_ = f.Close()
		memoria.Release(n)
	}), nil
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
func (in *Inbound) deliverTo(ctx context.Context, rcpt string, seccion func() *io.SectionReader, size int64, guardar func(context.Context, tenancy.Tenant) (string, error), env domain.Message, junk bool, envelopeFrom string, autoEligible, fromAutenticado bool) error {
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
	mailbox, err := in.store.MailboxByRole(tctx, resolved.AccountID, role)
	if err != nil {
		return fmt.Errorf("delivery: localizar buzón destino: %w", err)
	}
	blobKey, err := guardar(tctx, t)
	if err != nil {
		return err
	}

	m := env // copia del sobre por destinatario
	m.AccountID = resolved.AccountID
	m.MailboxID = mailbox.ID
	m.BlobKey = blobKey
	m.SizeBytes = size

	// Reglas de usuario: solo sobre correo legítimo (el spam va a Junk sin filtrar).
	// Pueden mover el mensaje a otra carpeta y/o marcarlo leído/destacado ANTES de
	// persistir el sobre, de modo que se inserte ya en su buzón final con su UID.
	if !junk {
		in.applyFilters(tctx, resolved.AccountID, &m)
	}
	if err := in.store.InsertMessage(tctx, &m); err != nil {
		// Sin compensación en S3: el blob es compartido (otros buzones pueden
		// usarlo) y, sin referencias, lo recoge el barrido de huérfanos.
		return fmt.Errorf("delivery: persistir sobre: %w", err)
	}

	// Notificar a Hub/cliente (nuevo correo) vía outbox de eventos sobre Postgres.
	// El correo que va a Spam no suena en la campana: sería avisar del spam.
	in.publishDelivered(tctx, resolved.Tenant, resolved.AccountID, resolved.Subject, m, !junk)
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

// applyFilters evalúa las reglas activas de la cuenta contra el mensaje entrante
// y aplica la primera que casa (mover de carpeta / marcar leído/destacado).
// Best-effort: si la consulta de reglas falla, se entrega sin filtrar (nunca se
// pierde ni se retiene el correo por un fallo de esta capa de conveniencia).
func (in *Inbound) applyFilters(ctx context.Context, accountID uuid.UUID, m *domain.Message) {
	filters, err := in.store.ListActiveFilters(ctx, accountID)
	if err != nil {
		in.logger.Warn("inbound: cargar reglas de filtrado fallido", "error", err)
		return
	}
	if dest := domain.ApplyInbound(filters, m); dest != nil {
		m.MailboxID = *dest
	}
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
