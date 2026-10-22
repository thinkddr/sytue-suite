// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command pages es el binario de Sytue Web (:8100), el constructor de webs y
// tiendas de las organizaciones. Un binario, varios modos (PAGES-ENCARGO §2):
//
//	pages                          la API (sin argumentos)
//	pages sitios                   el servidor de las webs publicadas, sin base de datos
//	pages healthcheck              sonda local del proceso (Docker, compose)
//	pages dbcheck                  preflight del despliegue contra la base
//	pages mapa                     regenera una vez el mapa host → sitio
//	pages suspender <slug> <motivo>  suspende una web por abuso
//	pages reactivar <slug>         levanta la suspensión (no republica)
//
// Cada área cablea lo suyo en su BLOQUE MARCADO (`// --- tienda ---`…): quien la
// sustituye toca solo su bloque, y el choque al rebasar es trivial.
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
	"strings"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/altcha"
	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	pagesdb "git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/analitica"
	"git.sytue.eu/sytue/one/services/pages/internal/catalogo"
	"git.sytue.eu/sytue/one/services/pages/internal/config"
	"git.sytue.eu/sytue/one/services/pages/internal/dominios"
	pageshttp "git.sytue.eu/sytue/one/services/pages/internal/http"
	"git.sytue.eu/sytue/one/services/pages/internal/ia"
	"git.sytue.eu/sytue/one/services/pages/internal/mapa"
	"git.sytue.eu/sytue/one/services/pages/internal/publicar"
	"git.sytue.eu/sytue/one/services/pages/internal/sitios"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
	"git.sytue.eu/sytue/one/services/pages/internal/tienda"
)

// version se inyecta en build vía -ldflags "-X main.version=...".
var version string

func main() {
	modo, args := "", []string(nil)
	if len(os.Args) > 1 {
		modo, args = os.Args[1], os.Args[2:]
	}
	var err error
	switch modo {
	case "healthcheck":
		os.Exit(runHealthcheck())
	case "dbcheck":
		err = runDBCheck()
	case "":
		err = runAPI()
	case "sitios":
		err = runSitios()
	case "mapa", "suspender", "reactivar":
		err = runOperacion(modo, args)
	default:
		err = fmt.Errorf("subcomando desconocido %q (sitios, healthcheck, dbcheck, mapa, suspender, reactivar)", modo)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "sytue-pages %s: %v\n", modo, err)
		os.Exit(1)
	}
}

// runAPI arranca la API.
func runAPI() error {
	cfg, err := config.Cargar()
	if err != nil {
		return fmt.Errorf("cargar configuración: %w", err)
	}
	ctx, logger, cerrar, err := arrancar(cfg.Base, config.ServiceName)
	if err != nil {
		return err
	}
	defer cerrar()

	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()
	if err := pg.MigrateUp(pagesdb.Migrations, pagesdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	objetos, err := objectstore.New(cfg.S3)
	if err != nil {
		return fmt.Errorf("almacén de objetos: %w", err)
	}
	st := store.New(pool)

	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("inicializar verificador OIDC: %w", err)
	}

	cat, err := catalogo.CargarPorDefecto()
	if err != nil {
		return fmt.Errorf("cargar catálogo: %w", err)
	}

	catalogoCtx, cancelarCatalogo := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelarCatalogo()
	if err := cat.Materializar(catalogoCtx, objetos); err != nil {
		return fmt.Errorf("materializar catálogo: %w", err)
	}

	d := pageshttp.Deps{
		Cfg: cfg, Store: st, Verifier: verifier, Objetos: objetos, Catalogo: cat,
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
			httpx.CheckerFunc{Label: "s3", Fn: objetos.Healthz},
		},
	}

	// --- servicio ---
	d.Publicar = publicar.Nuevo()
	// --- sitios ---
	d.Mapa = mapa.NuevoEscritor(st, objetos, cfg.SitesDomain)
	d.Lector = mapa.NuevoLector(objetos, cfg.MapaRefresco)
	d.Lector.IniciarBucle(ctx)
	var hubClient *hubnotify.Client
	if cfg.HubBaseURL != "" {
		hubClient = hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
	}
	d.Dominios = dominios.Nuevo(st, cfg, d.Mapa, hubClient, logger)
	d.Dominios.IniciarBarrido(ctx)
	d.Analitica = analitica.Nuevo(cfg.AnalyticsURL, cfg.AnalyticsSecret)
	// --- altcha ---
	var altchaVerifier *altcha.Verifier
	if len(cfg.SecretEncKey) > 0 {
		mac := hmac.New(sha256.New, cfg.SecretEncKey)
		mac.Write([]byte("sytue-pages-altcha"))
		altchaKey := mac.Sum(nil)
		altchaVerifier = altcha.New(altchaKey, altcha.ConRepeticiones(st.RegistrarAltcha))
	} else {
		altchaVerifier = altcha.New([]byte("dev-altcha-secret-key-32-bytes!"), altcha.ConRepeticiones(st.RegistrarAltcha))
	}
	d.Altcha = altchaVerifier
	// --- ia & seguridad ---
	var keyCipher *keycrypto.Cipher
	if len(cfg.SecretEncKey) > 0 {
		keyCipher, err = keycrypto.New(cfg.SecretEncKey)
		if err != nil {
			return fmt.Errorf("inicializar keycrypto: %w", err)
		}
	}
	// --- tienda ---
	d.Tienda = tienda.Nuevo(st, keyCipher, cfg.PublicURL, cfg.SitesDomain, hubClient, altchaVerifier).
		ConSyERP(cfg.SyERPURL, cfg.SyERPPedidosSecret)
	go d.Tienda.IniciarBarrido(ctx)
	// Los avisos del outbox: correo por el relé y campana del Hub.
	go func() {
		if err := d.Tienda.Despachar(ctx, cfg.Correo); err != nil {
			logger.Error("despachador de avisos de la tienda", "error", err)
		}
	}()
	if !cfg.Correo.Activo() {
		logger.Warn("sin SMTP_FROM: los correos de la tienda no salen y se reintentarán")
	}
	// --- ia ---
	asistenteCliente := asistente.Nuevo(cfg.AIURL, 60*time.Second)
	d.IA = ia.Nuevo(cfg, st, asistenteCliente, keyCipher, objetos, cat, hubClient, logger)
	d.IA.IniciarBarrido(ctx)
	// --- fin de los bloques ---

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:    cfg.HTTPAddr,
		Handler: pageshttp.NewRouter(d),
		// Plazos por defecto (15/30 s): los grupos de subidas, publicaciones y zip
		// los amplían por petición a 5 min (internal/http, conPlazo).
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
	logger.Info("sytue-pages iniciado", "addr", cfg.HTTPAddr, "public_url", cfg.PublicURL,
		"sites_domain", cfg.SitesDomain, "env", string(cfg.Env))
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("sytue-pages detenido limpiamente")
	return nil
}

// runSitios arranca el servidor de las webs publicadas: sin base, sin OIDC y sin
// más secreto que el de S3.
func runSitios() error {
	cfg, err := config.CargarSitios()
	if err != nil {
		return fmt.Errorf("cargar configuración: %w", err)
	}
	ctx, logger, cerrar, err := arrancar(cfg.Base, config.ServiceName+"-sitios")
	if err != nil {
		return err
	}
	defer cerrar()

	objetos, err := objectstore.New(cfg.S3)
	if err != nil {
		return fmt.Errorf("almacén de objetos: %w", err)
	}
	lector := mapa.NuevoLector(objetos, cfg.MapaRefresco)
	lector.IniciarBucle(ctx)

	// --- sitios ---
	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         sitios.NuevoRouter(cfg.Version, cfg, lector, objetos),
		ShutdownTimeout: cfg.ShutdownTimeout,
	})
	// --- fin de los bloques ---
	logger.Info("sytue-pages-sitios iniciado", "addr", cfg.HTTPAddr, "sites_domain", cfg.SitesDomain)
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	return nil
}

// runOperacion son los subcomandos de operación, que se lanzan con `kubectl exec`
// en un pod de la API (tienen su entorno completo).
func runOperacion(modo string, args []string) error {
	uso := map[string]string{"mapa": "pages mapa", "suspender": "pages suspender <slug> <motivo>", "reactivar": "pages reactivar <slug>"}
	switch {
	case modo == "mapa" && len(args) != 0,
		modo == "suspender" && (len(args) < 2 || strings.TrimSpace(strings.Join(args[1:], " ")) == ""),
		modo == "reactivar" && len(args) != 1:
		return fmt.Errorf("uso: %s", uso[modo])
	}
	cfg, err := config.Cargar()
	if err != nil {
		return fmt.Errorf("cargar configuración: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: 1})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()
	st := store.New(pool)

	switch modo {
	case "suspender":
		err = st.Suspender(ctx, args[0], strings.Join(args[1:], " "))
	case "reactivar":
		err = st.Reactivar(ctx, args[0])
	}
	if err != nil {
		return err
	}
	objetos, err := objectstore.New(cfg.S3)
	if err != nil {
		return fmt.Errorf("almacén de objetos: %w", err)
	}
	// --- sitios ---
	escritor := mapa.NuevoEscritor(st, objetos, cfg.SitesDomain)
	// --- fin de los bloques ---
	if err := escritor.Regenerar(ctx); err != nil {
		return fmt.Errorf("regenerar el mapa: %w", err)
	}
	fmt.Println("hecho")
	return nil
}

// arrancar prepara lo común a los dos servidores: logger, señales y telemetría.
func arrancar(base pcfg.Base, nombre string) (context.Context, *slog.Logger, func(), error) {
	if version != "" {
		base.Version = version
	}
	logger := log.New(log.Options{Level: base.LogLevel, Format: base.LogFormat, Service: nombre, Version: base.Version})
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	ctx = log.Into(ctx, logger)
	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: nombre, Version: base.Version})
	if err != nil {
		stop()
		return nil, nil, nil, fmt.Errorf("telemetría: %w", err)
	}
	telemetry.SetDefault(tp)
	return ctx, logger, func() { _ = tp.Shutdown(context.Background()); stop() }, nil
}

func runDBCheck() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL está vacía")
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

// runHealthcheck pide /healthz al puerto local: 0 si responde 200.
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
