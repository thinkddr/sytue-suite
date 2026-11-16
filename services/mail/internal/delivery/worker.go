// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package delivery

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

// Sender entrega un mensaje a un MTA remoto. Abstraído para test (el worker no
// abre sockets en pruebas). El MX se resuelve por DNS del dominio destino.
type Sender interface {
	// Send entrega el .eml a los destinatarios del dominio. Devuelve (permanente,
	// error): permanente=true es un 5xx (bounce), false un 4xx (defer).
	Send(ctx context.Context, mailFrom string, recipients []string, raw io.Reader) (permanent bool, err error)
}

// QueueWorker drena la cola de salida de PostgreSQL (sin Redis/Valkey) con
// reintentos y backoff exponencial. Reparte el trabajo entre N workers usando
// SELECT ... FOR UPDATE SKIP LOCKED. LISTEN/NOTIFY despierta antes; un poll de
// respaldo garantiza progreso si se pierde un NOTIFY.
type QueueWorker struct {
	store  *store.Store
	blobs  *objectstore.Client
	sender Sender
	logger *slog.Logger

	workers      int
	maxAttempts  int
	baseBackoff  time.Duration
	pollInterval time.Duration
	hubNotifier  *hubnotify.Client
}

// QueueConfig configura el worker de cola.
type QueueConfig struct {
	Workers      int
	MaxAttempts  int
	BaseBackoff  time.Duration
	PollInterval time.Duration
	// HubNotifier manda la señal de cambios del widget de Mail cuando el aviso de no
	// entrega entra en la Entrada de quien envió. Nil: sin señal.
	HubNotifier *hubnotify.Client
}

// NewQueueWorker construye el worker de cola. Si sender es nil, usa SMTPSender.
func NewQueueWorker(st *store.Store, blobs *objectstore.Client, sender Sender, logger *slog.Logger, cfg QueueConfig) *QueueWorker {
	if sender == nil {
		sender = NewSMTPSender(logger)
	}
	if cfg.Workers <= 0 {
		cfg.Workers = 2
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 15 * time.Second
	}
	return &QueueWorker{
		store: st, blobs: blobs, sender: sender, logger: logger,
		workers:      cfg.Workers,
		maxAttempts:  cfg.MaxAttempts,
		baseBackoff:  cfg.BaseBackoff,
		pollInterval: cfg.PollInterval,
		hubNotifier:  cfg.HubNotifier,
	}
}

// Run arranca el bucle de drenado hasta que el contexto se cancele.
func (q *QueueWorker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	for i := 0; i < q.workers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			q.loop(ctx, id)
		}(i)
	}
	// GC periódico de la tabla de rate-limit (anti-crecimiento ilimitado).
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(5 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = q.store.GCInboundRate(ctx, 10*time.Minute)
				_ = q.store.GCAuthFailRate(ctx, 10*time.Minute)
			}
		}
	}()
	// Despertar los hilos pospuestos vencidos: vuelven a Recibidos sin leer. Una sola
	// goroutine por réplica basta (SKIP LOCKED reparte entre réplicas) y el ritmo del
	// sondeo de la cola es de sobra: posponer se elige al minuto, no al segundo.
	wg.Add(1)
	go func() {
		defer wg.Done()
		t := time.NewTicker(q.pollInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				q.despertarPospuestos(ctx)
			}
		}
	}()
	wg.Wait()
}

// despertarPospuestos devuelve a Recibidos los hilos cuya posposición venció, por
// lotes hasta vaciar lo pendiente, y manda la señal de cambios del widget de Mail a
// cada cuenta: su contador de la Entrada acaba de cambiar sin que nadie escribiera.
func (q *QueueWorker) despertarPospuestos(ctx context.Context) {
	for {
		lote, err := q.store.DespertarPospuestos(ctx, time.Now(), 50)
		if err != nil {
			q.logger.Error("queue: despertar pospuestos", "error", err)
			return
		}
		for _, d := range lote {
			q.hubNotifier.AvisarCambio(ctx, d.Tenant, "mail", d.Subject)
		}
		if len(lote) < 50 {
			return
		}
	}
}

func (q *QueueWorker) loop(ctx context.Context, id int) {
	ticker := time.NewTicker(q.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				// Primero LIBERAR los diferidos vencidos (programado/deshacer): se
				// convierten en filas de entrega normales que el drenado de abajo recoge
				// en el mismo tick. Un fallo al liberar no debe frenar la entrega.
				nr, rerr := q.releaseBatch(ctx)
				if rerr != nil {
					// El lote se revirtió (nada se commiteó): no cuenta como progreso, o
					// un error persistente haría girar este bucle. Reintento al próximo tick.
					q.logger.Error("queue: liberar diferidos", "worker", id, "error", rerr)
					nr = 0
				}
				n, err := q.drainBatch(ctx)
				if err != nil {
					q.logger.Error("queue: drenar lote", "worker", id, "error", err)
					break
				}
				if nr == 0 && n == 0 {
					break // nada pendiente; esperar al siguiente tick/NOTIFY
				}
			}
		}
	}
}

func (q *QueueWorker) drainBatch(ctx context.Context) (int, error) {
	items, err := q.store.ClaimDue(ctx, 10)
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		q.deliver(ctx, it)
	}
	return len(items), nil
}

// releaseBatch libera los diferidos vencidos: crea su copia en Sent y encola su
// entrega por dominio (PromoteDueHeld lo hace atómicamente, cross-réplica seguro
// vía SKIP LOCKED). getBlob baja el .eml firmado de S3 bajo el tenant de la fila.
func (q *QueueWorker) releaseBatch(ctx context.Context) (int, error) {
	return q.store.PromoteDueHeld(ctx, time.Now(), 10, func(ctx context.Context, tenant, key string) ([]byte, error) {
		t, err := tenancy.New(tenant)
		if err != nil {
			return nil, err
		}
		rc, err := q.blobs.Get(ctx, t, key)
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return io.ReadAll(rc)
	})
}

func (q *QueueWorker) deliver(ctx context.Context, it domain.OutboundMessage) {
	t, err := tenancy.New(it.Tenant)
	if err != nil {
		// Sin tenant válido no hay bandeja a la que avisar: se cierra y se registra.
		_ = q.store.MarkBounced(ctx, it.ID, "tenant inválido")
		q.logger.Error("queue: tenant inválido en la cola", "id", it.ID, "tenant", it.Tenant)
		return
	}
	tctx := tenancy.Into(ctx, t)

	rc, err := q.blobs.Get(tctx, t, it.BlobKey)
	if err != nil {
		// Blob ausente: error permanente (no se puede reenviar). Avisa igual: para
		// quien escribió, un mensaje que no sale por un fallo nuestro se ve idéntico
		// a uno rechazado por el destino, y en los dos casos hay que enterarse.
		q.bounce(ctx, it, "blob no encontrado: "+err.Error())
		return
	}
	defer rc.Close()

	permanent, sendErr := q.sender.Send(ctx, it.From, it.Recipients, rc)
	if sendErr == nil {
		if err := q.store.MarkSent(ctx, it.ID); err != nil {
			q.logger.Error("queue: marcar enviado", "id", it.ID, "error", err)
		}
		return
	}
	if permanent {
		q.bounce(ctx, it, sendErr.Error())
		q.logger.Warn("queue: rebote permanente", "id", it.ID, "error", sendErr)
		return
	}
	backoff := q.backoffFor(it.Attempts)
	bounced, err := q.store.MarkDeferred(ctx, it.ID, sendErr.Error(), backoff, q.maxAttempts)
	if err != nil {
		q.logger.Error("queue: diferir", "id", it.ID, "error", err)
	} else if bounced {
		// MarkDeferred ya dejó la fila en 'bounced': aquí solo falta el aviso, que
		// es justo lo que no existía. Agotar reintentos es el camino MÁS probable
		// —un destino que no contesta, un puerto 25 cerrado— y era el más callado.
		q.logger.Warn("queue: agotados reintentos -> bounce", "id", it.ID)
		if nerr := q.notifySender(ctx, it, sendErr.Error()); nerr != nil {
			q.logger.Error("queue: avisar del rebote", "id", it.ID, "error", nerr)
		}
	}
}

// backoffFor calcula el backoff exponencial con jitter para el intento n.
func (q *QueueWorker) backoffFor(attempts int) time.Duration {
	base := q.baseBackoff
	if base <= 0 {
		base = 30 * time.Second
	}
	shift := attempts
	if shift > 6 {
		shift = 6 // tope ~ base*64
	}
	d := base << shift
	jitter := time.Duration(rand.Int63n(int64(base))) //nolint:gosec // G404: jitter de reintento, sin valor criptográfico.
	return d + jitter
}

// ---------------------------------------------------------------------------
// SMTPSender: entrega real vía SMTP a los MX de cada dominio (RFC 5321 §5).
// ---------------------------------------------------------------------------

// SMTPSender relaya mensajes a los MTA remotos con STARTTLS oportunista.
type SMTPSender struct {
	logger           *slog.Logger
	resolver         *net.Resolver
	hostname         string
	timeout          time.Duration
	relay            *Relay
	privateTransit   bool
	privateTLSConfig *tls.Config
}

// Relay es un servidor SMTP de salida (smarthost) por el que se entrega TODO en
// vez de hablar con el MX de cada destino.
//
// POR QUÉ EXISTE. Entregar directo exige lo que una conexión doméstica no da:
// IP fija con PTR propio, puerto 25 saliente sin bloquear por el ISP y una
// reputación que un rango residencial no tiene. Sin eso el correo no rebota —
// eso sería fácil de ver—: acaba en spam, silenciosamente y solo en algunos
// destinatarios.
//
// LO QUE NO ARREGLA. Solo la salida. El correo ENTRANTE sigue necesitando que
// el MX apunte aquí con el 25 abierto, y eso no puede pasar por un túnel HTTP.
type Relay struct {
	Host     string
	Port     string
	Username string
	Password string
}

// NewSMTPSender construye el emisor SMTP saliente con entrega directa por MX.
func NewSMTPSender(logger *slog.Logger) *SMTPSender {
	return &SMTPSender{logger: logger, resolver: net.DefaultResolver, hostname: "mx.sytue.eu", timeout: 60 * time.Second}
}

// NewSMTPSenderConRelay construye el emisor que entrega por un smarthost.
// Con relay nil se comporta EXACTAMENTE igual que NewSMTPSender: quien no lo
// configure sigue entregando por MX, que es el camino por defecto.
func NewSMTPSenderConRelay(logger *slog.Logger, relay *Relay) *SMTPSender {
	s := NewSMTPSender(logger)
	if relay != nil && relay.Host != "" {
		if relay.Port == "" {
			relay.Port = "587"
		}
		s.relay = relay
	}
	return s
}

// Send entrega el mensaje a los destinatarios (todos del mismo dominio).
func (s *SMTPSender) Send(ctx context.Context, mailFrom string, recipients []string, raw io.Reader) (bool, error) {
	if len(recipients) == 0 {
		return true, fmt.Errorf("sin destinatarios")
	}
	dom := domainOf(recipients[0])
	// Con relé no se consulta el MX del destino: TODO sale por el smarthost, que
	// es quien decide cómo llegar. Un solo host, así que si falla no hay
	// alternativa que probar —y eso es correcto: caer al MX directo desde una IP
	// doméstica es justo lo que el relé existe para evitar, y encima entregaría
	// unos mensajes por un camino y otros por otro sin que nadie lo notara.
	var hosts []string
	if s.relay != nil {
		hosts = []string{s.relay.Host}
	} else {
		hosts = s.mxHosts(ctx, dom)
	}

	body, err := io.ReadAll(raw)
	if err != nil {
		return false, err
	}
	defer clear(body)

	var lastErr error
	for _, host := range hosts {
		permanent, err := s.deliverTo(ctx, host, mailFrom, recipients, body)
		if err == nil {
			return false, nil
		}
		lastErr = err
		if errors.Is(err, ErrSMTPAmbiguous) {
			return false, err
		}
		if permanent {
			return true, err // 5xx: no probar más MX
		}
	}
	return false, fmt.Errorf("todos los MX fallaron: %w", lastErr)
}

func (s *SMTPSender) deliverTo(ctx context.Context, host, mailFrom string, rcpts []string, body []byte) (permanent bool, err error) {
	c, stopCancel, err := s.openSMTPClient(ctx, host)
	if err != nil {
		return false, err
	}
	defer stopCancel()
	defer c.Close()

	// (Re)envía EHLO con nuestro hostname; tras STARTTLS el saludo previo se
	// descarta y hay que repetirlo sobre el canal cifrado (RFC 3207 §4.2).
	if err := c.Hello(s.hostname); err != nil {
		return false, err
	}

	// AUTH solo con relé, y DESPUÉS del EHLO cifrado. Un MX ajeno no espera que
	// nos autentiquemos —ni debemos ofrecerle credenciales—; el smarthost sí las
	// exige, y sin ellas rechaza con 5xx cada mensaje.
	if s.relay != nil && s.relay.Username != "" {
		if err := c.Auth(sasl.NewPlainClient("", s.relay.Username, s.relay.Password)); err != nil {
			// Permanente: una credencial mala no mejora reintentando, y dejar el
			// mensaje dando vueltas en la cola oculta el problema real.
			return true, fmt.Errorf("relé %s: autenticación rechazada: %w", host, err)
		}
	}

	if err := c.Mail(mailFrom, nil); err != nil {
		return isPermanent(err), err
	}
	for _, rcpt := range rcpts {
		if err := c.Rcpt(rcpt, nil); err != nil {
			return isPermanent(err), err
		}
	}
	return sendSMTPData(c, body)
}

func (s *SMTPSender) openSMTPClient(ctx context.Context, host string) (*gosmtp.Client, func() bool, error) {
	puerto := "25"
	if s.relay != nil {
		puerto = s.relay.Port
	}
	dialer := &net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, puerto))
	if err != nil {
		return nil, nil, err
	}

	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	deadline := time.Now().Add(s.timeout)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	_ = conn.SetDeadline(deadline)

	// STARTTLS oportunista (RFC 3207): si el MX lo soporta, ciframos el canal.
	// NewClientStartTLS hace EHLO + STARTTLS + handshake y cierra la conexión si
	// falla; en ese caso reconectamos y seguimos en claro (best effort de entrega).
	tlsCfg := &tls.Config{ServerName: strings.TrimSuffix(host, "."), MinVersion: tls.VersionTLS12}
	if s.privateTLSConfig != nil {
		tlsCfg = s.privateTLSConfig.Clone()
		tlsCfg.ServerName = strings.TrimSuffix(host, ".")
	}
	c, err := gosmtp.NewClientStartTLS(conn, tlsCfg)
	if err != nil {
		stopCancel()
		// CON RELÉ NO HAY VUELTA ATRÁS EN CLARO, y esta es la diferencia que de
		// verdad importa: hacia el smarthost viajan usuario y contraseña. El
		// «best effort» que tiene sentido hacia un MX ajeno —mejor entregar sin
		// cifrar que no entregar— aquí significaría publicar la credencial en la
		// red. Se falla, y como temporal: el mensaje se reintenta.
		if s.relay != nil || s.privateTransit {
			return nil, nil, fmt.Errorf("SMTP: STARTTLS obligatorio y no disponible: %w", err)
		}
		s.logger.Warn("smtpsender: STARTTLS no disponible, continuando en claro", "host", host, "error", err)
		conn, err = dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, puerto))
		if err != nil {
			return nil, nil, err
		}
		stopCancel = context.AfterFunc(ctx, func() { _ = conn.Close() })
		_ = conn.SetDeadline(deadline)
		// NewClient no dial­a ni valida: el saludo/errores del servidor SMTP surgen
		// en el primer comando (HELO/EHLO), que comprobamos justo debajo.
		c = gosmtp.NewClient(conn)
	}
	return c, stopCancel, nil
}

func sendSMTPData(c *gosmtp.Client, body []byte) (bool, error) {
	w, err := c.Data()
	if err != nil {
		return isPermanent(err), err
	}
	if _, err := w.Write(body); err != nil {
		return false, errors.Join(ErrSMTPAmbiguous, err)
	}
	if err := w.Close(); err != nil {
		var failure *gosmtp.SMTPError
		if errors.As(err, &failure) {
			return isPermanent(err), err
		}
		return false, errors.Join(ErrSMTPAmbiguous, err)
	}
	// DATA acceptance is final. A lost QUIT response must never trigger a
	// second delivery to this host or the next MX.
	_ = c.Quit()
	return false, nil
}

func (s *SMTPSender) mxHosts(ctx context.Context, dom string) []string {
	mxs, err := s.resolver.LookupMX(ctx, dom)
	if err != nil || len(mxs) == 0 {
		// Sin MX: fallback al registro A del dominio (RFC 5321 §5.1).
		return []string{dom}
	}
	sort.Slice(mxs, func(i, j int) bool { return mxs[i].Pref < mxs[j].Pref })
	hosts := make([]string, 0, len(mxs))
	for _, mx := range mxs {
		hosts = append(hosts, mx.Host)
	}
	return hosts
}

// isPermanent clasifica un error SMTP como permanente (5xx) vs temporal (4xx).
func isPermanent(err error) bool {
	var smtpErr *gosmtp.SMTPError
	if errors.As(err, &smtpErr) {
		return smtpErr.Code >= 500 && smtpErr.Code < 600
	}
	return false
}
