// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command chat es el binario de Sytue Rooms (mensajería en tiempo real:
// WebSocket para mensajes/typing/presencia + API REST de teams/canales/mensajes/
// hilos/reacciones; estado en PostgreSQL, fan-out durable por LISTEN/NOTIFY,
// fan-out efímero typing/presencia por Dragonfly; adjuntos vía Sytue Drive).
//
// Subcomandos:
//
//	chat              arranca el servidor HTTP/WebSocket (por defecto)
//	chat migrate      aplica las migraciones de base de datos y sale
//	chat healthcheck  sonda de liveness para el healthcheck de Docker
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	chatdb "git.sytue.eu/sytue/one/services/rooms/db"
	"git.sytue.eu/sytue/one/services/rooms/internal/config"
	chathttp "git.sytue.eu/sytue/one/services/rooms/internal/http"
	mediaconfig "git.sytue.eu/sytue/one/services/rooms/internal/media/config"
	mediahttp "git.sytue.eu/sytue/one/services/rooms/internal/media/http"
	mediaservice "git.sytue.eu/sytue/one/services/rooms/internal/media/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/service"
	"git.sytue.eu/sytue/one/services/rooms/internal/store"
	"git.sytue.eu/sytue/one/services/rooms/internal/ws"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "avisos-historicos":
			if err := runAvisosHistoricos(os.Args[2:]); err != nil {
				fmt.Fprintln(os.Stderr, "avisos-historicos:", err)
				os.Exit(1)
			}
			return
		case "healthcheck":
			os.Exit(runHealthcheck())
		case "migrate":
			if err := runMigrate(); err != nil {
				fmt.Fprintln(os.Stderr, "migrate:", err)
				os.Exit(1)
			}
			return
		case "seal-secrets":
			// Sella los secretos de firma heredados en claro (ver
			// store.SealPlaintextSecrets). Idempotente: se puede lanzar en cada
			// despliegue. Cruza organizaciones, así que es tarea de operación.
			if err := runSealSecrets(); err != nil {
				fmt.Fprintln(os.Stderr, "seal-secrets:", err)
				os.Exit(1)
			}
			return
		}
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

// run arranca el servidor completo.
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	logger := log.New(log.Options{
		Level: cfg.LogLevel, Format: cfg.LogFormat,
		Service: config.ServiceName, Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	// Telemetría (stub no-op por ahora).
	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Deja este Provider como el de la instrumentación transversal: el middleware
	// HTTP común mide con él sin que cada servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// Migraciones al arrancar (paridad dev/prod; idempotente).
	if err := pg.MigrateUp(chatdb.Migrations, chatdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	// PostgreSQL (único datastore: estado + cola/outbox + fan-out NOTIFY).
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar PostgreSQL: %w", err)
	}
	defer pool.Close()

	// Verificador de JWT de Sytue ID (JWKS).
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	// Dragonfly (pub/sub EFÍMERO): fan-out cross-réplica de typing/presencia.
	// Best-effort: sin DRAGONFLY_URL o con Dragonfly caído al arrancar, cliente
	// no-op => los efímeros quedan intra-réplica (comportamiento previo), sin
	// romper el servicio. Postgres sigue siendo la única fuente durable.
	// EL CLIENTE SE CONSERVA AUNQUE EL PING FALLE: la sonda de platform/pkg/cache
	// reconecta sola. Sustituirlo por un no-op, como se hacía antes, dejaba el
	// fan-out roto para el resto de la vida del proceso.
	ephemeral, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
	if err != nil {
		logger.Warn("Dragonfly no disponible al arrancar; typing/presencia intra-réplica hasta que reconecte", "error", err)
	}
	defer func() { _ = ephemeral.Close() }()

	// Cifrado en reposo de los secretos de FIRMA de las integraciones salientes.
	// En prod es obligatorio: sin él, quien lea la base puede firmar peticiones
	// haciéndose pasar por nosotros ante el endpoint del cliente, que es justo lo
	// que la firma existe para impedir. Fuera de prod se avisa y la creación de
	// integraciones con secreto falla (nunca se escribe en claro).
	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		return fmt.Errorf("keycrypto: %w", err)
	}
	if cfg.Env.IsProd() && !cipher.Enabled() {
		return errors.New("SYTUE_ROOMS_KEY_ENCRYPTION_KEY es obligatoria en prod (cifrado en reposo de los secretos de firma)")
	}
	if !cipher.Enabled() {
		logger.Warn("sin SYTUE_ROOMS_KEY_ENCRYPTION_KEY: no se podrán crear integraciones con secreto de firma")
	}

	// Capa de servicio + Hub WebSocket.
	st := service.NewStore(pool, cipher)
	drive := service.NewDriveClient(cfg.DriveURL)
	svc := service.New(cfg, st, service.NewPoolPublisher(pool), drive)
	// Personas de otras organizaciones: las encuentra Sytue ID (ADR 0040).
	if dir := service.NewDirectorioID(cfg.IDURL); dir != nil {
		svc.SetDirectorio(dir)
	}
	// El nombre visible de la organización que invita, en vez de su identificador.
	if n := personas.NuevosNombresOrgs(cfg.IDURL, cfg.IDNombresOrgsSecret); n != nil {
		svc.SetNombresOrgs(n)
	}
	// Miniaturas de las vistas previas: sin esto se regeneran en cada petición.
	svc.SetEfimero(ephemeral)

	// Web Push (Ola 18): enviador VAPID. Sin claves configuradas queda deshabilitado
	// (best-effort, como Dragonfly): el resto de Rooms funciona igual. Un par a medias
	// (solo pública o solo privada) sí es un error de config y aborta el arranque.
	pushSender, err := service.NewPushSender(cfg)
	if err != nil {
		return fmt.Errorf("web push: %w", err)
	}
	if pushSender != nil {
		svc.SetPushSender(pushSender)
		logger.Info("web push habilitado (VAPID configurado)")
	} else {
		logger.Info("web push deshabilitado (sin SYTUE_VAPID_PUBLIC/PRIVATE)")
	}

	// APNs: la app nativa de iOS solo se puede despertar por Apple, así que el aviso
	// que sale es VACÍO (ver service/apns.go). Sin credenciales queda deshabilitado.
	apnsSender, err := service.NewAPNSSender(cfg)
	if err != nil {
		return fmt.Errorf("apns: %w", err)
	}
	if apnsSender != nil {
		svc.SetAPNSSender(apnsSender)
		logger.Info("aviso a iOS habilitado (APNs, solo despertar)")
	} else {
		logger.Info("aviso a iOS deshabilitado (sin SYTUE_APNS_*)")
	}

	if cfg.HubBaseURL != "" && cfg.HubInternalSecret != "" {
		logger.Info("notificaciones a Sytue Hub habilitadas", "hub_url", cfg.HubBaseURL)
	}

	hub := ws.NewHub(ephemeral)
	if err := hub.StartRelay(ctx); err != nil {
		logger.Warn("relay efímero no disponible; typing/presencia solo intra-réplica", "error", err)
	}

	arrancarFanoutWS(ctx, pool, hub)

	// Purga periódica del outbox: chat publica un evento por mensaje/edición/
	// reacción y el fan-out por Subscriber NUNCA marca processed_at, así que sin
	// retención la tabla crece sin límite.
	go runOutboxPurge(ctx, pool)
	go runPurgaDenuncias(ctx, svc.Store())

	// Worker de mensajes programados (Ola 12): drena los vencidos (send_at<=now) y
	// los publica reusando el flujo de PostMessage. Cross-réplica con SKIP LOCKED
	// (sin doble envío); termina al cancelarse el contexto (apagado ordenado).
	if cfg.ScheduledScanInterval > 0 {
		go svc.RunScheduledWorker(ctx, cfg.ScheduledScanInterval)
	}

	// Webhooks SALIENTES (Ola 15): consumidor EXACTLY-ONCE del outbox. A diferencia
	// del Subscriber de fan-out WS (que corre en TODAS las réplicas y no marca
	// processed_at), este es un Dispatcher: FOR UPDATE SKIP LOCKED + processed_at ⇒
	// una sola réplica procesa cada mensaje y hace el POST SSRF-safe, con reintentos
	// acotados (MaxAttempts). No estorba al Subscriber (GetByID relee por id sin
	// mirar processed_at). BatchSize pequeño: el POST corre bajo el lock del lote.
	//
	// ponytail: el POST vive dentro de la tx del Dispatcher (sostiene el lock del
	// lote). Aceptable porque los salientes son pocos por tenant y solo los mensajes
	// que CASAN hacen POST (el resto es una consulta indexada que no bloquea). Si el
	// volumen de salientes creciera, encolar en una tabla propia y hacer el POST fuera
	// de la tx (patrón de la cola de Mail).
	// MessageCreatedHandler encadena los webhooks salientes (Ola 15) y el Web Push (Ola
	// 18) sobre el MISMO consumidor exactly-once: dos Dispatchers sobre el mismo canal se
	// repartirían los eventos (FOR UPDATE SKIP LOCKED) y cada mensaje solo recibiría UNO
	// de los dos efectos. El push es best-effort dentro del handler (no reintenta).
	outgoing := events.NewDispatcher(pool, svc.MessageCreatedHandler(), events.DispatcherConfig{
		Channels:    []events.Channel{service.EventChannel},
		BatchSize:   25,
		MaxAttempts: 5,
	})
	go func() {
		if err := outgoing.Run(ctx); err != nil {
			logger.Error("dispatcher de webhooks salientes / push detenido", "error", err)
		}
	}()

	// Limitador del endpoint público de webhooks entrantes (Ola 15): token-bucket
	// sobre Postgres (compartido entre réplicas), por token+IP.
	hookLimiter := pg.NewRateLimiter(pool, pg.RateLimiterConfig{
		PerSecond: cfg.HookRatePerSecond, Burst: cfg.HookRateBurst,
	})

	// --- Videollamada: el mismo proceso, la misma base de datos -----------------
	//
	// Mensajería y vídeo eran dos servicios con dos bases; para publicar la tarjeta de
	// «llamada finalizada» en un canal, el vídeo llamaba por HTTP a la mensajería con un
	// secreto compartido. Aquí es una llamada en proceso: no hay secreto que cablear en
	// dos sitios ni verja que falle cerrada si falta.
	mediaCfg, err := mediaconfig.Load()
	if err != nil {
		return fmt.Errorf("config de videollamada: %w", err)
	}
	signer := livekit.NewSigner(mediaCfg.LiveKitAPIKey, mediaCfg.LiveKitAPISecret)
	mediaSvc := mediaservice.New(
		mediaCfg,
		mediaservice.NewStore(pool),
		livekit.NewClient(mediaCfg.LiveKitHost, signer),
		signer,
		mediaservice.NewPoolPublisher(pool),
	)
	mediaSvc.SetMessagingNotifier(mediaservice.NewInProcessNotifier(svc))
	mediaSvc.SetEncryptionGateway(svc)
	mediaSvc.SetDocumentEncryptionReady(cfg.EncryptedDocumentCalls)
	svc.SetCallMediaTransport(mediaSvc)
	go svc.RunCallEncryptionExpiry(ctx)
	// «Llamar desde el documento»: la ACL de Sytue Docs decide quién entra a esas salas.
	if g := mediaservice.NewCalendarClient(mediaCfg.CalendarURL); g != nil {
		mediaSvc.SetCalendarGate(g)
	}
	if g := mediaservice.NewDocsClient(mediaCfg.DocsURL); g != nil {
		mediaSvc.SetDocumentGate(g)
	}
	// Descarga de grabaciones. Mismas credenciales que se le dan al egress: las
	// necesita de lectura además de escritura, porque quien firma la descarga es
	// este servicio. Si no hay destino configurado no se cablea nada y la ruta
	// responde «esta instalación no guarda grabaciones» en vez de romperse.
	if mediaCfg.PuedeGrabar() {
		host, ssl := servidorS3(mediaCfg.RecordingEndpoint)
		objetos, err := objectstore.New(objectstore.Config{
			Endpoint: host, Region: mediaCfg.RecordingRegion, UseSSL: ssl,
			AccessKey: mediaCfg.RecordingAccessKey, SecretKey: mediaCfg.RecordingSecretKey,
			Bucket: mediaCfg.RecordingBucket,
		})
		if err != nil {
			return fmt.Errorf("almacén de grabaciones: %w", err)
		}
		// Se comprueba que el bucket EXISTE antes de dar la grabación por
		// disponible. Un nombre bien escrito de un bucket que nadie creó pasa
		// todas las validaciones de config y solo falla al subir el fichero, o
		// sea después de haber grabado a la gente. Si no responde, el servicio
		// arranca igual —la mensajería y las llamadas no dependen de esto— pero
		// el botón de grabar dice que esta instalación no guarda grabaciones.
		if err := objetos.Healthz(ctx); err != nil {
			log.From(ctx).Warn("grabación deshabilitada: el bucket no responde",
				"bucket", mediaCfg.RecordingBucket, "error", err)
		} else {
			mediaSvc.SetObjectStore(objetos)
		}
	}

	enAnfitrion := chathttp.EnAnfitrion(svc)
	mediaDeps := mediahttp.Deps{
		Cfg:      mediaCfg,
		Service:  mediaSvc,
		Signer:   signer,
		Verifier: verifier,
		// La llamada de una conversación con gente de otra organización vive donde
		// vive la conversación.
		Anfitrion: func(r *http.Request, sala, canal uuid.UUID) (*http.Request, error) {
			return enAnfitrion(r, service.RefAnfitrion{Sala: sala, Canal: canal})
		},
	}

	router := chathttp.NewRouter(chathttp.Deps{
		Cfg:         cfg,
		Service:     svc,
		Verifier:    verifier,
		Hub:         hub,
		HookLimiter: hookLimiter,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: pool.Healthz},
			httpx.CheckerFunc{Label: "matrix", Fn: svc.MatrixReady},
		},
		MountExtra:       func(api chi.Router) { mediahttp.MountAPI(api, mediaDeps) },
		MountPublicExtra: func(r chi.Router) { mediahttp.MountPublic(r, mediaDeps) },
		// La cola de denuncias del operador: quién lo es lo dice Sytue ID.
		Operador: operador.SytueID(cfg.IDURL),
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
		// WebSocket de larga duración: sin límite de lectura ni de escritura.
		// SinTimeout y no 0, que es «el valor por defecto» (15 s / 30 s).
		WriteTimeout: httpx.SinTimeout,
		ReadTimeout:  httpx.SinTimeout,
	})

	logger.Info("Sytue Rooms arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "public_url", cfg.PublicURL,
		"livekit_url", mediaCfg.LiveKitURL)
	return srv.Run(ctx)
}

// arrancarFanoutWS entrega a los clientes WebSocket de ESTA réplica cada evento
// que se publica en el outbox, escuchando por LISTEN/NOTIFY.
//
// El broadcast de un mensaje a las salas WS es una entrega EFÍMERA por proceso (a
// los clientes conectados a esta réplica). DEBE hacerse con un Subscriber (todas
// las réplicas reciben cada NOTIFY), NO con el Dispatcher del outbox: el
// Dispatcher usa FOR UPDATE SKIP LOCKED + processed_at, de modo que con >1
// réplica solo UNA procesaría cada evento y los clientes de las demás nunca
// recibirían el mensaje. Cada réplica relee la fila del outbox por su ID (payload
// del NOTIFY) y entrega a sus clientes locales, sin marcar processed_at (eso es
// exclusivo del trabajo exactly-once a nivel de clúster).
//
// Vive fuera de run() para que el arranque quepa de un vistazo; el logger sale
// del contexto, como en runOutboxPurge.
func arrancarFanoutWS(ctx context.Context, pool *pg.Pool, hub *ws.Hub) {
	logger := log.From(ctx)
	fanout := service.NewFanoutHandler(hub)
	// El de la VIDEOLLAMADA va aparte porque publica en otro canal del outbox:
	// chat y medios viven en el mismo servicio desde la fusión, pero no comparten
	// canal de eventos. Sin esto, el aviso de «hay una llamada» solo llegaba
	// sondeando cada ~25 segundos desde el navegador.
	fanoutLlamadas := service.NewCallFanoutHandler(hub)
	subscriber := events.NewSubscriber(pool, func(nctx context.Context, canal events.Channel, n events.Notification) {
		ev, ok, err := events.GetByID(nctx, pool, n.ID)
		if err != nil {
			logger.Error("fan-out WS: no se pudo leer el evento del outbox", "id", n.ID, "error", err)
			return
		}
		if !ok {
			return // fila ausente (purgada): nada que entregar
		}
		manejar := fanout
		if canal == service.MediaEventChannel {
			manejar = fanoutLlamadas
		}
		if err := manejar(nctx, ev); err != nil {
			logger.Warn("fan-out WS: entrega fallida", "id", n.ID, "error", err)
		}
	}, service.EventChannel, service.MediaEventChannel)
	go func() {
		if err := subscriber.Run(ctx); err != nil {
			logger.Error("relay de eventos detenido", "error", err)
		}
	}()
}

// runOutboxPurge borra cada hora, por lotes, los eventos del outbox con más de
// 48 h (retención; usa events_outbox_created_idx). events_outbox no tiene RLS,
// por lo que basta un Exec directo del pool. Corre hasta cancelar el contexto.
func runOutboxPurge(ctx context.Context, pool *pg.Pool) {
	const batch = 5000
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var total int64
			for {
				tag, err := pool.Exec(ctx,
					`DELETE FROM events_outbox WHERE id IN (
					    SELECT id FROM events_outbox
					     WHERE created_at < now() - interval '48 hours'
					     ORDER BY created_at LIMIT $1)`, batch)
				if err != nil {
					log.From(ctx).Warn("purga de events_outbox falló", "error", err)
					break
				}
				total += tag.RowsAffected()
				if tag.RowsAffected() < batch {
					break
				}
			}
			if total > 0 {
				log.From(ctx).Info("events_outbox purgado", "count", total)
			}
		}
	}
}

// runPurgaDenuncias borra al arrancar y luego una vez al día las denuncias
// resueltas hace más de un año (store.RetencionDenuncias). Corre hasta cancelar
// el contexto.
func runPurgaDenuncias(ctx context.Context, st *store.Store) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		if n, err := st.PurgarDenuncias(ctx); err != nil {
			log.From(ctx).Warn("purga de denuncias falló", "error", err)
		} else if n > 0 {
			log.From(ctx).Info("denuncias resueltas purgadas por antigüedad", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runMigrate aplica las migraciones y sale (uso operativo / Job de despliegue).
func runMigrate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return pg.MigrateUp(chatdb.Migrations, chatdb.MigrationsDir, cfg.DatabaseURL)
}

// runSealSecrets cifra los secretos de firma que quedaron en claro de antes de
// que Rooms cifrara en reposo. Idempotente y de una pasada; sin KEK no hace nada
// y lo dice, porque sellar sin clave no significa nada.
func runSealSecrets() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cipher, err := keycrypto.New(cfg.KeyEncryptionKey)
	if err != nil {
		return fmt.Errorf("keycrypto: %w", err)
	}
	if !cipher.Enabled() {
		return errors.New("SYTUE_ROOMS_KEY_ENCRYPTION_KEY es obligatoria: sin ella no hay con qué sellar")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	webhooks, comandos, err := store.New(pool, cipher).SealPlaintextSecrets(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("secretos de firma sellados: %d saliente(s), %d comando(s)\n", webhooks, comandos)
	return nil
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/chat","healthcheck"]).
// Consulta /healthz en el puerto local y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8085"
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
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// servidorS3 parte el endpoint de las grabaciones en host y TLS. El egress lo
// quiere con esquema («https://s3.fr-par.scw.cloud») y minio-go lo quiere sin
// él; sin esta traducción el cliente intentaría resolver un host llamado
// «https» y la descarga fallaba solo en producción, donde el endpoint sí lleva
// esquema. Sin esquema se asume TLS: en la UE, un bucket en claro no es una
// opción por defecto.
func servidorS3(endpoint string) (string, bool) {
	switch {
	case strings.HasPrefix(endpoint, "https://"):
		return strings.TrimPrefix(endpoint, "https://"), true
	case strings.HasPrefix(endpoint, "http://"):
		return strings.TrimPrefix(endpoint, "http://"), false
	default:
		return endpoint, true
	}
}
