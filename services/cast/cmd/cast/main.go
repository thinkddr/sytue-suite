// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command cast es el binario de Sytue Cast (:8099): vídeo bajo demanda y directo,
// con portada pública, biblioteca por organización, vídeos para personas
// concretas, comentarios y chat. Estado en PostgreSQL (única fuente de verdad),
// ficheros en S3, auth vía Sytue ID (JWT/JWKS).
//
// El mismo binario, con el subcomando `transcoder`, es el trabajador que
// convierte los originales (CAST-ENCARGO §7): otra imagen con ffmpeg, mismo código.
//
// `cast encolar-calidades [N]` encola, en todas las organizaciones, el trabajo que
// da la escalera WebM completa (1080p si el original llega, 720p y 360p) a los
// vídeos de antes de ella, que solo tenían 720p: como mucho N (100 por defecto),
// los más recientes primero. Cuesta CPU del transcodificador y espacio de cada
// organización. El barrido hace lo mismo poco a poco (CAST_SWEEP_BACKFILL_QUALITIES);
// esto queda para empujar a mano un lote grande.
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/livekit"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/operador"
	"git.sytue.eu/sytue/one/platform/pkg/personas"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	castdb "git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/almacen"
	"git.sytue.eu/sytue/one/services/cast/internal/config"
	"git.sytue.eu/sytue/one/services/cast/internal/cuota"
	"git.sytue.eu/sytue/one/services/cast/internal/diapositivas"
	"git.sytue.eu/sytue/one/services/cast/internal/directo"
	casthttp "git.sytue.eu/sytue/one/services/cast/internal/http"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/social"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
	"git.sytue.eu/sytue/one/services/cast/internal/transcoder"
)

// version se inyecta en build vía -ldflags "-X main.version=...".
var version string

func main() {
	modo := ""
	if len(os.Args) > 1 {
		modo = os.Args[1]
	}
	switch modo {
	case "healthcheck":
		// Sonda local del proceso usada por Docker/Compose.
		os.Exit(runHealthcheck())
	case "transcoder-healthcheck":
		// Sonda del trabajador: no escucha en ningún puerto, late en un fichero.
		dir := os.Getenv("TRANSCODER_TMPDIR")
		if dir == "" {
			dir = os.TempDir()
		}
		if err := transcoder.ComprobarLatido(filepath.Join(dir, transcoder.NombreLatido), transcoder.EdadMaximaLatido, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "transcoder-healthcheck: %v\n", err)
			os.Exit(1)
		}
		return
	case "encolar-calidades":
		if err := runEncolarCalidades(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "encolar-calidades: %v\n", err)
			os.Exit(1)
		}
		return
	case "dbcheck":
		// Preflight del despliegue: verifica credenciales y red contra la RDB
		// sin cargar OIDC, migrar ni arrancar un servidor.
		if err := runDBCheck(); err != nil {
			fmt.Fprintf(os.Stderr, "dbcheck: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := run(modo == "transcoder"); err != nil {
		fmt.Fprintf(os.Stderr, "sytue-cast: error fatal: %v\n", err)
		os.Exit(1)
	}
}

func runDBCheck() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL está vacía")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	pool.Close()
	return nil
}

// runEncolarCalidades implementa `cast encolar-calidades [N]`.
func runEncolarCalidades(args []string) error {
	limite := 100
	if len(args) > 0 {
		n, err := strconv.Atoi(args[0])
		if err != nil || n <= 0 {
			return fmt.Errorf("el límite ha de ser un entero positivo, no %q", args[0])
		}
		limite = n
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL está vacía")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pg.Connect(ctx, pg.Config{DSN: dsn, MaxConns: 1})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()
	n, err := store.New(pool).EncolarCalidadesPendientes(ctx, limite)
	if err != nil {
		return err
	}
	fmt.Printf("encolados %d trabajos de calidades\n", n)
	return nil
}

// run arranca el servicio HTTP o, con `esTranscoder`, el trabajador. Comparten
// configuración, base, almacén y servicio: solo cambia quién escucha.
func run(esTranscoder bool) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cargar configuracion: %w", err)
	}
	if version != "" {
		cfg.Version = version
	}
	nombre := config.ServiceName
	if esTranscoder {
		nombre += "-transcoder"
	}

	logger := log.New(log.Options{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		Service: nombre,
		Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, errTel := telemetry.NewProvider(ctx, telemetry.Config{Service: nombre, Version: cfg.Version})
	if errTel != nil {
		return fmt.Errorf("telemetría: %w", errTel)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	telemetry.SetDefault(tp)

	// --- PostgreSQL (única fuente de verdad) ---------------------------------
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()

	// Las migraciones las aplica el servicio; el trabajador espera a que estén
	// (arrancan a la vez y solo uno debe migrar).
	if !esTranscoder {
		if err := pg.MigrateUp(castdb.Migrations, castdb.MigrationsDir, cfg.DatabaseURL); err != nil {
			return fmt.Errorf("aplicar migraciones: %w", err)
		}
		logger.Info("migraciones aplicadas")
	}

	// --- S3: el bucket de Cast ---------------------------------------------
	s3cfg := cfg.S3
	s3cfg.Remote, s3cfg.LocalTenants = cfg.S3Remote, cfg.S3LocalTenants
	objetos, err := objectstore.New(s3cfg)
	if err != nil {
		return fmt.Errorf("almacén de objetos: %w", err)
	}

	st := store.New(pool)
	cuotaSvc, driveSvc := driveDe(cfg)
	svc := service.New(service.Deps{
		Store:      st,
		Directorio: directorioDe(cfg),
		Objetos:    almacen.New(objetos),
		Cuota:      cuotaSvc,
		Drive:      driveSvc,
		Firmante:   media.New(cfg.MediaSigningKey),
		Logger:     logger,
	})

	if esTranscoder {
		logger.Info("sytue-cast-transcoder iniciado", "env", string(cfg.Env), "threads", cfg.TranscoderThreads)
		err := transcoder.Run(ctx, transcoder.Deps{Pool: pool, Objetos: objetos, Store: st, Servicio: svc, Cfg: cfg, Logger: logger})
		if err != nil && !errors.Is(err, context.Canceled) {
			return fmt.Errorf("transcodificador: %w", err)
		}
		logger.Info("sytue-cast-transcoder detenido limpiamente")
		return nil
	}

	// --- Verificador de tokens de Sytue ID (JWKS) ----------------------------
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("inicializar verificador OIDC: %w", err)
	}

	// La clave de las denuncias sin sesión (el reto ALTCHA y el troceo de la IP) se
	// deriva de la de /media: un secreto menos que repartir.
	claveDenuncias := hmac.New(sha256.New, []byte(cfg.MediaSigningKey))
	claveDenuncias.Write([]byte("sytue-cast-reports"))
	secretoDenuncias := claveDenuncias.Sum(nil)
	hub := hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
	soc := social.New(pool, svc.AccesoSocial()).ConModeracion(social.Moderacion{
		Hub: hub, PublicURL: cfg.PublicURL, Secreto: secretoDenuncias,
	})
	// Cast corre con varias réplicas: lo que el chat difunde en una (mensajes,
	// borrados, bloqueos recién puestos) llega a las conexiones de las demás.
	soc.Relevar(ctx)
	// El chat de un directo se retira a los 30 días, en todas las organizaciones.
	// En segundo plano y con el contexto del servidor: se apaga con él.
	if cfg.ChatCleanupInterval > 0 {
		go soc.BucleDeLimpieza(ctx, cfg.ChatCleanupInterval)
	}

	// El barrido: borrados pasada la gracia, subidas atascadas y el relleno de
	// calidades de los vídeos de antes. Aquí y no en el transcodificador
	// (service/barrido.go); las réplicas se turnan con un cerrojo.
	if cfg.SweepInterval > 0 {
		go svc.BucleDeBarrido(ctx, cfg.SweepInterval, service.Barrido{
			Gracia: cfg.SweepDeletedGrace, Subida: cfg.SweepStaleUpload,
			Calidades: cfg.SweepBackfillQualities,
		})
	}

	// El directo (§6) solo si hay SFU; sin él, el router monta la implementación
	// nula. El sondeo de respaldo se apaga con el servidor.
	dir := directoDe(cfg, pool, st, svc, soc, logger)
	if dir != nil {
		go dir.BucleDeSondeo(ctx)
	}

	router := casthttp.NewRouter(casthttp.Deps{
		Cfg:      cfg,
		Store:    st,
		Servicio: svc,
		Social:   soc,
		Directo:  casthttp.NuevoDirecto(dir),
		// «Presentar con Cast»: la posición de la presentación de un directo.
		Diapositivas: diapositivas.New(pool, svc, logger),
		Verifier:     verifier,
		// Las soluciones ya usadas, en la base: una solución vale una vez en todas
		// las réplicas, no una vez en cada una (migración 0012).
		Altcha: altcha.New(secretoDenuncias, altcha.ConRepeticiones(altcha.EnTabla(pool, "altcha_used"))),
		// La cola de denuncias del operador: quién lo es lo dice Sytue ID.
		Operador: operador.SytueID(cfg.IDURL),
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
			httpx.CheckerFunc{Label: "s3", Fn: objetos.Healthz},
		},
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:    cfg.HTTPAddr,
		Handler: router,
		// Sin los 15/30 s por defecto: una subida de 8 GiB y la descarga de un
		// original tardan lo que tardan, y el WebSocket del chat vive horas. Las
		// rutas REST llevan su propio tope de 30 s por grupo (server.go).
		ReadTimeout:     httpx.SinTimeout,
		WriteTimeout:    httpx.SinTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info("sytue-cast iniciado",
		"addr", cfg.HTTPAddr, "public_url", cfg.PublicURL, "env", string(cfg.Env))
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	// Las purgas y declaraciones de cuota en curso terminan antes de salir.
	svc.Esperar()
	logger.Info("sytue-cast detenido limpiamente")
	return nil
}

// directorioDe es Sytue ID para «@ana». nil sin URL: no se comparte con nadie.
func directorioDe(cfg config.Config) personas.Directorio {
	if d := personas.NewDirectorioID(cfg.IDURL); d != nil {
		return d
	}
	return nil
}

// directoDe es el directo sobre LiveKit. nil sin LIVEKIT_URL: sin directo.
func directoDe(cfg config.Config, pool *pg.Pool, st *store.Store, svc *service.Servicio, soc *social.Servicio, logger *slog.Logger) *directo.Servicio {
	if cfg.LiveKitURL == "" {
		return nil
	}
	firmante := livekit.NewSigner(cfg.LiveKitAPIKey, cfg.LiveKitAPISecret)
	return directo.New(directo.Deps{
		Pool: pool, Store: st, Servicio: svc,
		LiveKit:  livekit.NewClient(cfg.LiveKitHost, firmante),
		Firmante: firmante,
		Hub:      hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret),
		Bloquean: soc.QuienesBloquean,
		Cfg: directo.Config{
			LiveKitURL:    cfg.LiveKitURL,
			IngestRTMPURL: cfg.IngestRTMPURL,
			PublicURL:     cfg.PublicURL,
			Grabacion: livekit.DestinoS3{
				AccessKey: cfg.RecordingAccessKey, Secret: cfg.RecordingSecretKey, Region: cfg.RecordingRegion,
				Bucket: cfg.RecordingBucket, Endpoint: cfg.RecordingEndpoint, ForcePathStyle: true,
			},
			PollInterval: cfg.LiveKitPollInterval,
		},
		Logger: logger,
	})
}

// driveDe es Drive para la cuota y para «Subir a Cast». nil sin URL: sin cuota y
// sin subir desde Drive. Las interfaces se devuelven a nil a mano: un *Drive nil
// dentro de una interfaz no sería nil y el servicio lo usaría.
func driveDe(cfg config.Config) (service.Cuota, service.Drive) {
	if c := cuota.New(cfg.DriveURL, cfg.InternalSecret); c != nil {
		return c, c
	}
	return nil, nil
}

// runHealthcheck implementa el subcomando "healthcheck": pide /healthz al puerto
// local y devuelve 0 si responde 200, 1 en caso contrario.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = config.DefaultPort
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/healthz", http.NoBody)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: estado %d\n", resp.StatusCode)
		return 1
	}
	return 0
}
