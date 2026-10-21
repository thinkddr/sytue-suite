// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command docs es el binario del servicio Sytue Docs/Sheets/Slides (puerto 8083
// en dev): ofimática colaborativa Sytue-native sobre Go + PostgreSQL + S3.
// Edición en tiempo real por WebSocket con CRDT (clean-room), versionado,
// permisos deny-by-default y exports OOXML/ODF. Identidad vía Sytue ID (OIDC).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/callaccess"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	docsdb "git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/collab"
	"git.sytue.eu/sytue/one/services/docs/internal/config"
	"git.sytue.eu/sytue/one/services/docs/internal/connectors"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/drive"
	dochttp "git.sytue.eu/sytue/one/services/docs/internal/http"
	"git.sytue.eu/sytue/one/services/docs/internal/ocr"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
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
	if len(os.Args) > 1 && os.Args[1] == "reconciliar-papelera" {
		if err := runReconciliarPapelera(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "reconciliar-papelera:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		slog.Error("sytue-docs: arranque fallido", "error", err)
		os.Exit(1)
	}
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/docs","healthcheck"]).
// Consulta /healthz en el puerto local y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
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
		Level: cfg.Base.LogLevel, Format: cfg.Base.LogFormat,
		Service: cfg.Base.ServiceName, Version: cfg.Base.Version,
	})
	slog.SetDefault(logger)
	if cfg.SelloURL != "" && cfg.SelloAnclas == nil {
		// Se arranca igual —un sello sin autenticar sigue fechando la firma—, pero
		// sobre HTTP plano un intermediario podría colar uno fabricado (ADR 0051).
		logger.Warn("sello de tiempo SIN anclas: los tokens de la autoridad no se autentican",
			"url", cfg.SelloURL)
	}

	// Contexto raíz cancelado por SIGINT/SIGTERM (apagado graceful).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	// Telemetría. Este servicio no la tenía: sin ella no hay forma de saber
	// cuántas peticiones atiende ni con qué latencia, que es lo que tiene que
	// decidir cuándo escalar.
	tp, errTel := telemetry.NewProvider(ctx, telemetry.Config{Service: cfg.Base.ServiceName, Version: cfg.Base.Version})
	if errTel != nil {
		return fmt.Errorf("telemetría: %w", errTel)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	// Provider de la instrumentación transversal: el middleware HTTP común mide
	// con él sin que el servicio tenga que pasárselo a su router.
	telemetry.SetDefault(tp)

	// PostgreSQL: único datastore (estado, ops CRDT, versiones, permisos, outbox).
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	// Migraciones embebidas.
	if err := pg.MigrateUp(docsdb.Migrations, docsdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return err
	}
	logger.Info("migraciones aplicadas")

	// Object Storage S3 (blobs/exports). Opcional: si no hay credenciales, el
	// servicio arranca sin exports a S3 (snapshots solo inline).
	var objects *objectstore.Client
	if cfg.S3Configured() {
		objects, err = objectstore.New(objectstore.Config{
			Endpoint: cfg.S3Endpoint, Region: cfg.S3Region,
			AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey,
			UseSSL: cfg.S3UseSSL, Bucket: cfg.S3Bucket,
			Remote: cfg.S3Remote, LocalTenants: cfg.S3LocalTenants,
		})
		if err != nil {
			return err
		}
		if cfg.Base.Env != "prod" {
			if err := objects.EnsureBucket(ctx, cfg.S3Region); err != nil {
				logger.Warn("no se pudo asegurar el bucket S3", "error", err)
			}
		}
	} else {
		logger.Warn("S3 no configurado: exports/snapshots grandes deshabilitados")
	}

	// Verificador OIDC de Sytue ID (JWKS con caché/rotación).
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer: cfg.OIDCIssuer, JWKSURL: cfg.OIDCJWKSURL, Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return err
	}

	st := store.New(pool)
	// Cómo se sostiene el aislamiento por organización, decidido UNA vez y con
	// pruebas: o se asume el rol de aplicación, o se comprueba que la RLS forzada
	// basta sobre un rol que no puede eludirla. Si ninguna de las dos cosas se
	// cumple, esto devuelve error y el servicio NO arranca: servir con las
	// políticas desactivadas sería peor que no servir.
	//
	// Va aquí, tras las migraciones y antes de tocar la base para nada más.
	modo, err := st.ResolveAppRole(ctx)
	if err != nil {
		return fmt.Errorf("aislamiento por organización: %w", err)
	}
	logger.Info("aislamiento por organización", "modo", modo)
	identitySource := callaccess.New(cfg.IDBaseURL, cfg.IDOrgsSecret)
	if cfg.EncryptedDocumentCalls && identitySource == nil {
		return fmt.Errorf("encrypted document calls require the private ID identity origin and credential")
	}
	if identitySource != nil {
		st.SetCallIdentitySource(identitySource)
	}
	callRevoker := callaccess.New(cfg.RoomsURL, cfg.RoomsCallAccessSecret)
	if cfg.EncryptedDocumentCalls && callRevoker == nil {
		return fmt.Errorf("encrypted document calls require a valid internal Rooms origin and secret")
	}
	var accessRevoker store.CallAccessRevoker
	if callRevoker != nil {
		accessRevoker = callRevoker
	}
	if err := st.ConfigureCallAccess(ctx, cfg.EncryptedDocumentCalls, accessRevoker); err != nil {
		return fmt.Errorf("encrypted document call access: %w", err)
	}
	svc := domain.NewService(st)

	// Dragonfly (pub/sub EFÍMERO) para colaboración de Docs.
	// Failsafe: sin DRAGONFLY_URL o con Dragonfly caído, el cliente queda
	// deshabilitado (no-op) y se degrada a Postgres (como antes).
	// El cliente se conserva aunque el Ping falle: la sonda de platform/pkg/cache
	// reconecta y la colaboración vuelve sola.
	ephemeral, err := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.DragonflyPassword)
	if err != nil {
		logger.Warn("Dragonfly no disponible al arrancar; docs collab con fallback Postgres hasta que reconecte", "error", err)
	}
	defer func() { _ = ephemeral.Close() }()

	hub := collab.NewHub(ephemeral)

	// Conector de datos externos (Power Query para Sheets). El Sealer cifra las
	// credenciales at-rest; sin SECRET_ENC_KEY queda deshabilitado (nil) y las
	// conexiones con credenciales se rechazan (el servicio arranca igual). El
	// Executor abre red saliente por la barrera SSRF con topes de filas/bytes/tiempo.
	sealer, err := connectors.NewSealer(cfg.SecretEncKey)
	if err != nil {
		return err
	}
	if !sealer.Enabled() {
		logger.Warn("conectores: SECRET_ENC_KEY ausente; las conexiones con credenciales se rechazarán")
	}
	executor := connectors.NewExecutor(connectors.Limits{}, cfg.IntegrationsURL).ConBases(cfg.BasesURL)

	handler := dochttp.New(dochttp.Deps{
		Service: svc, Store: st, Hub: hub, Objects: objects,
		Verifier: verifier, SnapshotInlineLimit: cfg.SnapshotInlineLimit,
		Sealer: sealer, Executor: executor,
		PublicURL: cfg.PublicURL, WopiHosts: cfg.WopiHosts,
		// Sytue OCR. `Nuevo` devuelve nil con URL vacía, y eso es lo que hace que
		// la interfaz no ofrezca reconocer texto en vez de ofrecerlo y fallar.
		OCR: ocr.Nuevo(cfg.OCRURL, cfg.OCRTimeout),
		// Sytue Drive: la unidad donde vive el fichero de cada documento nuevo.
		// `Nuevo` devuelve nil con URL vacía y entonces los documentos se crean
		// solo en Docs — invisibles en la unidad, que es el estado que esta ola
		// viene a cerrar, así que vacío significa despliegue a medias.
		Drive: drive.Nuevo(cfg.DriveURL, cfg.DriveTimeout).ConSecretoInterno(cfg.DriveInternalSecret),
		// Avisos del centro de notificaciones (compartir documento). `New`
		// devuelve nil sin URL y todo sigue funcionando sin avisos.
		HubNotifier: hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret),
		// «Te piden editar»: el correo al dueño. Sin remitente no sale ninguno.
		IDBaseURL:    cfg.IDBaseURL,
		IDOrgsSecret: cfg.IDOrgsSecret,
		Correo: correo.Config{
			Addr: cfg.SMTPAddr, From: cfg.SMTPFrom, User: cfg.SMTPUser, Password: cfg.SMTPPassword,
			TLSServerName: cfg.SMTPTLSServerName,
		},
		// Sello de tiempo de las firmas. Con URL vacía se firma sin sello y la
		// interfaz lo dice, que es mejor que no dejar firmar.
		Sello: domain.OpcionesSello{
			URL:      cfg.SelloURL,
			Politica: cfg.SelloPolitica,
			Usuario:  cfg.SelloUsuario,
			Clave:    cfg.SelloClave,
			Timeout:  cfg.SelloTimeout,
			Anclas:   cfg.SelloAnclas,
		},
		FirmaAnclas:    cfg.FirmaAnclas,
		Env:            cfg.Base.Env,
		SecretoInterno: cfg.DriveInternalSecret,
	})

	// Router base + sondas + rutas del servicio.
	// El tope del router acompaña a la entrada PDF más grande: sin él manda el
	// defecto de 1 MiB y `domain.MaxPDFBytes` (100 MiB) no se alcanzaba nunca.
	// Ver la nota de services/drive.
	r := httpx.NewRouter(httpx.RouterOptions{
		Logger:         logger,
		RequestTimeout: 30 * time.Second,
		MaxBodyBytes:   domain.MaxPDFBytes + (1 << 20),
	})
	checkers := []httpx.Checker{
		httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
	}
	if objects != nil {
		checkers = append(checkers, httpx.CheckerFunc{Label: "objectstore", Fn: objects.Healthz})
	}
	httpx.MountHealth(r, cfg.Base.Version, 3*time.Second, checkers...)
	handler.Mount(r)

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr: cfg.Base.HTTPAddr, Handler: r, ShutdownTimeout: cfg.Base.ShutdownTimeout,
	})

	// Fan-out WS cross-réplica por LISTEN/NOTIFY (mismo patrón que chat): el hub
	// de colaboración solo difunde a los clientes de ESTE proceso, así que cada
	// réplica escucha el NOTIFY del outbox (LISTEN es broadcast a todas), relee
	// las ops del lote y las entrega a SUS clientes WS locales; el conjunto
	// converge. Se usa Subscriber y NO el Dispatcher: la entrega WS es efímera y
	// por réplica (el Dispatcher, con SKIP LOCKED + processed_at, entregaría cada
	// evento a UNA sola réplica). La réplica de origen ya difundió en línea al
	// aplicar las ops y volverá a difundir aquí al recibir su propio NOTIFY: el
	// duplicado es inocuo porque el CRDT es idempotente y conmutativo
	// (domain/crdt.go, State.Apply) y los clientes deduplican por seq.
	subscriber := events.NewSubscriber(pool, func(nctx context.Context, _ events.Channel, n events.Notification) {
		if n.Type != store.EventDocUpdated {
			return
		}
		if ephemeral != nil && ephemeral.Enabled() {
			return // ya difundido en tiempo real vía pub/sub de Dragonfly
		}
		fanOutDocEvent(nctx, n, pool, hub, st, logger)
	}, store.EventChannel)
	go func() {
		if err := subscriber.Run(ctx); err != nil {
			logger.Error("fan-out colab: subscriber detenido", "error", err)
		}
	}()

	// Purga periódica del outbox en una goroutine ligada al contexto.
	go runOutboxPurge(ctx, st)
	if handler.PapeleraCompartida() {
		go runPapeleraCaducada(ctx, handler)
	} else {
		// Sin la papelera compartida no se sabría si hay retención legal: la papelera
		// de Docs no caduca. Se dice una vez, no cada hora.
		logger.Warn("papelera de Docs sin caducidad: falta SYTUE_DOCS_DRIVE_URL o SYTUE_DOCS_DRIVE_INTERNAL_SECRET")
	}

	logger.Info("sytue-docs listo", "addr", cfg.Base.HTTPAddr, "env", cfg.Base.Env)
	return srv.Run(ctx)
}

// fanOutDocEvent relee las ops del lote de un evento docs.doc.updated y las
// difunde a los clientes WS de ESTA réplica interesados en el documento. Es el
// fallback cross-réplica por LISTEN/NOTIFY cuando Dragonfly no está disponible.
func fanOutDocEvent(nctx context.Context, n events.Notification, pool *pg.Pool, hub *collab.Hub, st *store.Store, logger *slog.Logger) {
	ev, ok, err := events.GetByID(nctx, pool, n.ID)
	if err != nil {
		logger.Error("fan-out colab: no se pudo leer el evento del outbox", "id", n.ID, "error", err)
		return
	}
	if !ok || ev.Tenant == "" {
		return // fila purgada o sin tenant: nada que entregar
	}
	docID, since, limit, err := decodeDocUpdated(ev.Payload)
	if err != nil {
		logger.Warn("fan-out colab: payload de evento inválido", "id", n.ID, "error", err)
		return
	}
	// Sin clientes locales en la sala de ese documento, no releer ops: evita una
	// consulta por evento en cada réplica sin interesados.
	if len(hub.Presence(docID)) == 0 {
		return
	}
	t, err := tenancy.New(ev.Tenant)
	if err != nil {
		logger.Warn("fan-out colab: tenant inválido en el evento", "id", n.ID, "error", err)
		return
	}
	ops, err := st.OpsSince(tenancy.Into(nctx, t), docID, since, limit)
	if err != nil {
		logger.Warn("fan-out colab: no se pudieron releer las ops", "doc", docID, "error", err)
		return
	}
	hub.PublishOps(nctx, docID, ops, nil)
}

// decodeDocUpdated extrae de un payload docs.doc.updated el rango de ops del
// lote como argumentos para OpsSince: since = base_seq (seq previa al lote) y
// limit = tamaño del lote. Eventos antiguos sin base_seq (réplicas previas
// durante un despliegue rodante) degradan a la última op del lote; los clientes
// completan por replay/poll.
// ponytail: OpsSince recorta limit a 5000; un lote mayor (restore gigante) se
// difunde parcial y los clientes convergen por replay.
func decodeDocUpdated(payload []byte) (docID string, since int64, limit int, err error) {
	var p struct {
		DocumentID string `json:"document_id"`
		Seq        int64  `json:"seq"`
		BaseSeq    *int64 `json:"base_seq"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", 0, 0, err
	}
	if p.DocumentID == "" || p.Seq <= 0 {
		return "", 0, 0, fmt.Errorf("payload %s incompleto: %s", store.EventDocUpdated, payload)
	}
	since = p.Seq - 1
	if p.BaseSeq != nil && *p.BaseSeq >= 0 && *p.BaseSeq < p.Seq {
		since = *p.BaseSeq
	}
	return p.DocumentID, since, int(p.Seq - since), nil
}

// runPapeleraCaducada borra cada hora lo que lleva más de 30 días en la papelera de
// Docs (dochttp.RetencionPapelera), con la retención legal y el fichero de Drive
// resueltos por Drive. La primera vuelta, al arrancar.
func runPapeleraCaducada(ctx context.Context, h *dochttp.Handler) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if c, err := h.PurgarPapeleraCaducada(ctx); err != nil {
			log.From(ctx).Warn("purga de la papelera caducada", "error", err)
		} else if c != (dochttp.Caducidad{}) {
			log.From(ctx).Info("papelera caducada", "purgados", c.Purgados, "retenidos", c.Retenidos,
				"vivos", c.Vivos, "errores", c.Errores)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// runOutboxPurge purga periódicamente los eventos antiguos de events_outbox
// (retención 48h) y las firmas preparadas que caducaron sin completarse. El fan-out de docs es efímero (NOTIFY + lectura por id):
// nadie marca processed_at, así que sin purga la tabla crece sin límite. Se
// ejecuta por lotes hasta que se cancela el contexto.
func runOutboxPurge(ctx context.Context, st *store.Store) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if n, err := st.PurgeOutboxBefore(ctx, time.Now().Add(-48*time.Hour)); err != nil {
				log.From(ctx).Warn("purga de outbox falló", "error", err)
			} else if n > 0 {
				log.From(ctx).Info("eventos de outbox purgados", "count", n)
			}
			// Las firmas preparadas y nunca completadas (ADR 0051): cada una guarda
			// la cadena del certificado de alguien, y no tiene por qué quedarse.
			if n, err := st.PurgarFirmasCaducadas(ctx); err != nil {
				log.From(ctx).Warn("purga de firmas preparadas falló", "error", err)
			} else if n > 0 {
				log.From(ctx).Info("firmas preparadas caducadas purgadas", "count", n)
			}
		}
	}
}
