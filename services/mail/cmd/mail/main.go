// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command mail es el binario del servicio Sytue Mail (servidor de correo propio
// de Sytue One): SMTP entrante (MX), SMTP submission (MSA), IMAP y la API JMAP
// HTTP de cara al cliente. Datastore único PostgreSQL (metadatos + cola de
// salida); cuerpos MIME y adjuntos en Object Storage S3. Autenticación de la API
// vía Sytue ID (JWKS). Sin Redis/Valkey; sin motores de correo de terceros AGPL.
package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	gosmtp "github.com/emersion/go-smtp"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	maildb "git.sytue.eu/sytue/one/services/mail/db"
	"git.sytue.eu/sytue/one/services/mail/internal/auth"
	"git.sytue.eu/sytue/one/services/mail/internal/autoconfig"
	"git.sytue.eu/sytue/one/services/mail/internal/config"
	"git.sytue.eu/sytue/one/services/mail/internal/delivery"
	"git.sytue.eu/sytue/one/services/mail/internal/domain"
	"git.sytue.eu/sytue/one/services/mail/internal/idauth"
	"git.sytue.eu/sytue/one/services/mail/internal/imap"
	"git.sytue.eu/sytue/one/services/mail/internal/inboundhook"
	"git.sytue.eu/sytue/one/services/mail/internal/jmap"
	"git.sytue.eu/sytue/one/services/mail/internal/push"
	"git.sytue.eu/sytue/one/services/mail/internal/smime"
	"git.sytue.eu/sytue/one/services/mail/internal/smtp"
	"git.sytue.eu/sytue/one/services/mail/internal/store"
)

func main() {
	// Subcomando healthcheck (usado por el HEALTHCHECK de Docker): consulta
	// /healthz en el puerto local y devuelve 0 si el servicio está sano.
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	if len(os.Args) > 1 && os.Args[1] == "avisos-historicos" {
		if err := runAvisosHistoricos(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "avisos-historicos:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		// El logger ya pudo no estar inicializado: usar stderr de stdlib.
		panic(err)
	}
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/mail","healthcheck"]).
// Consulta /healthz en el puerto local (API JMAP/HTTP) y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8086"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", http.NoBody)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := log.New(log.Options{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		Service: cfg.ServiceName,
		Version: cfg.Version,
	})

	// Contexto raíz cancelado por SIGINT/SIGTERM (apagado graceful).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	// Telemetría. Este servicio no la tenía: sin ella no hay forma de saber
	// cuántas peticiones atiende ni con qué latencia, que es lo que tiene que
	// decidir cuándo escalar.
	tp, errTel := telemetry.NewProvider(ctx, telemetry.Config{Service: cfg.ServiceName, Version: cfg.Version})
	if errTel != nil {
		return fmt.Errorf("telemetría: %w", errTel)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Provider de la instrumentación transversal: el middleware HTTP común mide
	// con él sin que el servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// --- Migraciones (sobre PostgreSQL) ---
	if err := pg.MigrateUp(maildb.Migrations, maildb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return err
	}

	// --- PostgreSQL (único datastore: estado + cola) ---
	pool, err := pg.Connect(ctx, pg.Config{
		DSN:      cfg.DatabaseURL,
		MaxConns: cfg.DBMaxConns,
	})
	if err != nil {
		return err
	}
	defer pool.Close()
	st := store.New(pool)

	// --- Object Storage S3 (blobs: .eml + adjuntos) ---
	blobs, err := objectstore.New(objectstore.Config{
		Endpoint:     cfg.S3Endpoint,
		Region:       cfg.S3Region,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		UseSSL:       cfg.S3UseSSL,
		Bucket:       cfg.S3Bucket,
		Remote:       cfg.S3Remote,
		LocalTenants: cfg.S3LocalTenants,
	})
	if err != nil {
		return err
	}
	if !cfg.Env.IsProd() {
		// Dev/seed: crear el bucket si no existe (en prod lo gestiona Terraform).
		if err := blobs.EnsureBucket(ctx, cfg.S3Region); err != nil {
			logger.Warn("no se pudo asegurar el bucket S3", "error", err)
		}
	}

	// --- Verificador de tokens de Sytue ID (JWKS) ---
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return err
	}
	// Con contraseñas de aplicación: lo que se escribe en Thunderbird, Outlook,
	// Apple Mail o el móvil. Sin secreto configurado se queda como estaba (solo
	// JWT) y ningún cliente de correo puede entrar.
	idVerifier := idauth.New(verifier).WithAppPasswords(cfg.IDInternalURL, cfg.IDInternalSecret)

	// --- Firma DKIM de salientes (clave desde Secret Manager; vacía => dev) ---
	signer, err := newSigner(cfg)
	if err != nil {
		return err
	}

	// --- S/MIME: firma del correo saliente, gratis y sin fricción ---
	// Cada organización tiene su autoridad y cada buzón su certificado, emitidos
	// y renovados solos. Sin KEK no se guarda ninguna clave privada y el correo
	// sale sin firmar: mejor no firmar que guardar una clave en claro.
	smimeSigner, err := newSMIME(cfg, st, logger)
	if err != nil {
		return err
	}

	// --- Capa de entrega (inbound/outbound) y cola ---
	// Outbound primero: el inbound lo usa para despachar autorespuestas de vacaciones.
	outbound := delivery.NewOutbound(st, blobs, signer, logger, cfg.MaxMessageBytes, cfg.DefaultMailboxQuotaBytes)
	if smimeSigner != nil {
		outbound = outbound.WithSMIME(smimeSigner)
	}
	// Hub notifier para notificaciones unificadas en tiempo real en la suite.
	hubNotifier := hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)

	inbound := delivery.NewInbound(st, blobs, nil, outbound, logger, delivery.InboundConfig{
		MaxBytes:      cfg.MaxMessageBytes,
		VerifySPF:     cfg.VerifySPF,
		VerifyDKIM:    cfg.VerifyDKIM,
		EnforceDMARC:  cfg.EnforceDMARC,
		RejectOnFail:  cfg.RejectOnDMARCFail,
		HubNotifier:   hubNotifier,
		MailPublicURL: cfg.PublicURL,
		// Correo a canal de Sytue Rooms: nil si falta cualquiera de las dos piezas,
		// y entonces esas direcciones no existen para el MX.
		Channels: delivery.NewChannelBridge(cfg.ChannelMailDomain, cfg.RoomsURL),
		// El REPLY de una invitación de Calendar contestada por correo.
		Calendario: delivery.NewPuenteCalendario(cfg.CalendarURL, cfg.HubInternalSecret, logger),
	})
	// Alias de sistema (MAIL_SYSTEM_ALIASES): en cada arranque, así que un despliegue
	// los crea y repetirlo no cambia nada. Uno que no se puede crear —el buzón de
	// destino aún no existe— se dice y no para el servicio.
	for _, par := range cfg.AliasSistema {
		alias, destino, _ := strings.Cut(par, "=")
		alias, destino = strings.TrimSpace(alias), strings.TrimSpace(destino)
		err := st.AsegurarAlias(ctx, alias, destino, func(r store.ResolvedAddress) bool {
			return domain.AliasDomainAllowed(alias, domain.Account{Tenant: r.Tenant, Address: r.Address}, cfg.PrimaryDomain, cfg.RootTenant, nil)
		})
		if err != nil {
			logger.Warn("mail: alias de sistema sin crear", "alias", alias, "destino", destino, "error", err)
		}
	}
	// Emisor de salida. Con SMTP_RELAY_HOST vacío se entrega por MX, como
	// siempre; con relé, todo sale por el smarthost. Se decide UNA vez aquí y no
	// por mensaje: mezclar los dos caminos haría que unos correos salieran con la
	// reputación del relé y otros con la de la IP de casa, sin que nada lo dijera.
	var relay *delivery.Relay
	if cfg.RelayHost != "" {
		relay = &delivery.Relay{
			Host:     cfg.RelayHost,
			Port:     cfg.RelayPort,
			Username: cfg.RelayUsername,
			Password: cfg.RelayPassword,
		}
		logger.Info("salida por relé SMTP", "host", cfg.RelayHost, "puerto", cfg.RelayPort)
	}
	queue := delivery.NewQueueWorker(st, blobs, delivery.NewSMTPSenderConRelay(logger, relay), logger, delivery.QueueConfig{
		Workers:      cfg.QueueWorkers,
		MaxAttempts:  cfg.QueueMaxAttempts,
		BaseBackoff:  cfg.QueueBaseBackoff,
		PollInterval: cfg.QueuePollInterval,
	})

	// --- TLS para SMTPS/IMAPS (implícito). Vacío en dev (Traefik termina TLS) ---
	tlsConf, err := loadTLS(cfg)
	if err != nil {
		return err
	}

	// --- Router HTTP (API JMAP/REST) ---
	// El tope del router acompaña al del mensaje: sin él manda el defecto de 1
	// MiB y un correo con un adjunto normal no se puede ni componer, por mucho
	// que MAX_MESSAGE_MB diga 50. Ver la nota de services/drive.
	router := httpx.NewRouter(httpx.RouterOptions{
		Logger:         logger,
		RequestTimeout: 30 * time.Second,
		MaxBodyBytes:   cfg.MaxMessageBytes + (1 << 20),
	})
	httpx.MountHealth(
		router, cfg.Version, 3*time.Second,
		httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
		httpx.CheckerFunc{Label: "objectstore", Fn: blobs.Healthz},
	)
	api := jmap.New(cfg, st, blobs, outbound, logger).WithHub(hubNotifier)
	// El mismo servicio que firma publica su ancla: sin esto la ruta contesta 503
	// y el correo sale firmado por una autoridad que nadie puede instalar.
	//
	// El `if` va FUERA y no dentro de WithSMIME: `smimeSigner` es un puntero, y
	// uno nil envuelto en una interfaz no es `== nil` —lo avisa el propio
	// `jmap.New` dos líneas más abajo—. Pasarlo sin comprobar dejaría `a.smime`
	// con un no-nil que revienta al llamarlo. Mismo patrón que el `outbound` de
	// arriba.
	if smimeSigner != nil {
		api = api.WithSMIME(smimeSigner)
	}

	// AVISO EN EL MOMENTO DE QUE HA LLEGADO CORREO. Hasta ahora el webmail no
	// tenía ningún transporte de tiempo real, así que con la bandeja abierta un
	// mensaje nuevo no aparecía hasta volver a la pestaña.
	//
	// Cada réplica escucha por su cuenta: PostgreSQL entrega el NOTIFY a TODAS
	// las sesiones que hacen LISTEN, así que cada una reparte a sus propios
	// clientes sin coordinarse con nadie. Va ANTES de Mount porque la ruta solo
	// se monta si hay repartidor.
	repartidor := push.NewHub(0)
	api.UsarPush(repartidor)
	go func() {
		if err := push.NewRelay(st.Pool(), repartidor).Run(ctx); err != nil {
			logger.Error("mail: el aviso de correo nuevo se detuvo", "error", err)
		}
	}()

	api.Mount(router, verifier)
	// Autoconfiguración de clientes de correo (Thunderbird / Outlook Autodiscover):
	// pública por protocolo (el cliente aún no tiene credenciales).
	autoconfig.Mount(router, cfg)
	// RECEPCIÓN POR HTTPS cuando el 25 no llega. Se le da su PROPIO procesador,
	// idéntico salvo en una cosa: VerifySPF=false. El SPF comprueba que la IP que
	// conecta esté autorizada por el dominio del remitente, y por aquí la IP que
	// conecta es la del reenviador —no la del remitente—, así que verificarlo
	// rechazaría todo el correo legítimo. Se compensa con DKIM, que viaja firmado
	// dentro del mensaje y sobrevive al reenvío.
	//
	// Un procesador aparte y no un interruptor por mensaje: así el MX de verdad
	// conserva su verificación intacta y no hay forma de que una ruta se cuele
	// por la política de la otra.
	if cfg.InboundWebhookToken != "" {
		inboundHTTP := delivery.NewInbound(st, blobs, nil, outbound, logger, delivery.InboundConfig{
			MaxBytes:      cfg.MaxMessageBytes,
			VerifySPF:     false,
			VerifyDKIM:    cfg.VerifyDKIM,
			EnforceDMARC:  cfg.EnforceDMARC,
			RejectOnFail:  cfg.RejectOnDMARCFail,
			HubNotifier:   hubNotifier,
			MailPublicURL: cfg.PublicURL,
			Channels:      delivery.NewChannelBridge(cfg.ChannelMailDomain, cfg.RoomsURL),
			Calendario:    delivery.NewPuenteCalendario(cfg.CalendarURL, cfg.HubInternalSecret, logger),
		})
		inboundhook.Mount(router, inboundhook.Deps{
			Inbound: inboundHTTP,
			Token:   cfg.InboundWebhookToken,
			Logger:  logger,
		})
	}

	httpServer := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	// --- Servidores SMTP (MX, submission, submissions) ---
	smtpServers := smtp.NewServers(smtp.Deps{
		Cfg:       cfg,
		Logger:    logger,
		Inbound:   inbound,
		Outbound:  outbound,
		Verifier:  idVerifier,
		TLSConfig: tlsConf,
	})

	// --- Servidor IMAP (IMAPS :993) ---
	imapServer := imap.New(imap.Deps{
		Addr:        cfg.IMAPSAddr,
		Hostname:    cfg.Hostname,
		Store:       st,
		Blobs:       blobs,
		Auth:        idVerifier,
		TLSConfig:   tlsConf,
		ImplicitTLS: tlsConf != nil,
		Logger:      logger,

		MaxAuthFailuresPerMinPerIP: cfg.MaxAuthFailuresPerMinPerIP,
	})

	// --- Arranque concurrente de todos los servidores + worker de cola ---
	errCh := make(chan error, 8)

	go func() { errCh <- httpServer.Run(ctx) }()
	go func() { queue.Run(ctx) }()

	go func() { errCh <- serveSMTP(smtpServers.MX, false, logger, "mx") }()
	go func() { errCh <- serveSMTP(smtpServers.Submission, false, logger, "submission") }()
	if tlsConf != nil {
		go func() { errCh <- serveSMTP(smtpServers.Submissions, true, logger, "submissions") }()
		go func() { errCh <- imapServer.ListenAndServe() }()
	} else {
		logger.Warn("TLS no configurado: SMTPS(465) e IMAPS(993) deshabilitados (dev: Traefik termina TLS)")
	}

	logger.Info("Sytue Mail iniciado",
		"http", cfg.HTTPAddr, "mx", cfg.SMTPMXAddr, "submission", cfg.SMTPSubmissionAddr,
		"imaps", cfg.IMAPSAddr, "domain", cfg.PrimaryDomain, "hostname", cfg.Hostname)

	// Esperar a la señal de apagado o al primer error fatal de un servidor.
	select {
	case <-ctx.Done():
		logger.Info("señal de apagado recibida; cerrando")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("servidor terminó con error", "error", err)
			stop()
		}
	}

	// Apagado graceful de los servidores SMTP/IMAP (el HTTP lo hace su Run).
	smtpServers.Shutdown(cfg.ShutdownTimeout)
	_ = imapServer.Close()
	return nil
}

// serveSMTP arranca un listener SMTP y normaliza el error de cierre limpio.
func serveSMTP(s *gosmtp.Server, implicitTLS bool, logger *slog.Logger, name string) error {
	err := smtp.ListenAndServe(s, implicitTLS)
	if err != nil {
		logger.Error("servidor SMTP terminó", "listener", name, "error", err)
	}
	return err
}

// newSigner construye el firmante DKIM a partir de la configuración.
func newSigner(cfg config.Config) (*auth.Signer, error) {
	return auth.NewSigner(cfg.PrimaryDomain, cfg.DKIMSelector, cfg.DKIMPrivateKeyPEM)
}

// loadTLS carga el material TLS para los listeners de TLS implícito (465/993).
func loadTLS(cfg config.Config) (*tls.Config, error) {
	if !cfg.TLSConfigured() {
		return nil, nil //nolint:nilnil // contrato documentado: sin certificados ⇒ sin TLS implícito.
	}
	cert, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12, // STARTTLS legacy; TLS 1.3 preferido por negociación
	}, nil
}

// newSMIME construye el firmador S/MIME. Devuelve nil (sin error) cuando el
// despliegue no lo ha configurado: firmar es una mejora del correo, no un
// requisito para enviarlo.
func newSMIME(cfg config.Config, st *store.Store, logger *slog.Logger) (*smime.Service, error) {
	if !cfg.SMIMEEnabled {
		// Se dice. Apagarlo no rompe nada y por eso no se nota: el correo sigue
		// saliendo, solo que sin firma. Sin esta línea, la única forma de saber si
		// la variable llegó al pod es mandarse un correo y mirarle las tripas.
		logger.Info("smime: firma del correo saliente DESACTIVADA por configuración (SMIME_ENABLED=false)")
		return nil, nil //nolint:nilnil // desactivado a propósito por configuración.
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.SMIMEKeyEncKeyB64))
	if err != nil && cfg.SMIMEKeyEncKeyB64 != "" {
		return nil, fmt.Errorf("SYTUE_MAIL_KEY_ENCRYPTION_KEY no es base64 válido: %w", err)
	}
	cipher, err := keycrypto.New(key)
	if err != nil {
		return nil, fmt.Errorf("keycrypto: %w", err)
	}
	if cipher == nil || !cipher.Enabled() {
		logger.Warn("smime: sin SYTUE_MAIL_KEY_ENCRYPTION_KEY, el correo saliente NO se firmará")
		return nil, nil //nolint:nilnil // sin KEK no se emite material: es la degradación honesta.
	}
	return smime.NewService(st, cipher, logger), nil
}
