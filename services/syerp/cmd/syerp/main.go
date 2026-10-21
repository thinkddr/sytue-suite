// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command syerp es el binario de SyERP (:8098): el ERP de la suite. API REST
// única para los modelos del catálogo (contactos, ventas, compras, inventario,
// fabricación, facturación, proyectos, RRHH, mantenimiento, soporte…) y libro
// diario por partida doble. Estado en PostgreSQL (única fuente de verdad); auth
// vía Sytue ID (JWT/JWKS).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/asistente"
	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/correo"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"

	syerpdb "git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/avisos"
	"git.sytue.eu/sytue/one/services/syerp/internal/catalogo"
	"git.sytue.eu/sytue/one/services/syerp/internal/conectores"
	"git.sytue.eu/sytue/one/services/syerp/internal/config"
	syerphttp "git.sytue.eu/sytue/one/services/syerp/internal/http"
	"git.sytue.eu/sytue/one/services/syerp/internal/obligaciones"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
	"git.sytue.eu/sytue/one/services/syerp/internal/verifactu"
	"git.sytue.eu/sytue/one/services/syerp/internal/web"
)

// version se inyecta en build vía -ldflags "-X main.version=...".
var version string

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
			// Sonda local del proceso usada por Docker/Compose.
			os.Exit(runHealthcheck())
		case "dbcheck":
			// Preflight del despliegue: credenciales y red contra la RDB sin
			// cargar OIDC, migrar ni arrancar un servidor.
			if err := runDBCheck(); err != nil {
				fmt.Fprintf(os.Stderr, "dbcheck: %v\n", err)
				os.Exit(1)
			}
			return
		case "catalogo":
			// Valida los manifiestos y dice qué hay. Sin base de datos y sin red:
			// es la comprobación que se corre tras tocar un fichero de módulo.
			if err := runCatalogo(); err != nil {
				fmt.Fprintf(os.Stderr, "catalogo: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "sytue-syerp: error fatal: %v\n", err)
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

func runCatalogo() error {
	cat, err := catalogo.Cargar()
	if err != nil {
		return err
	}
	modelos := 0
	for _, m := range cat.Modulos() {
		fmt.Printf("%-16s %s\n", m.Clave, nombresDe(m))
		modelos += len(m.Modelos)
	}
	fmt.Printf("\n%d módulos, %d modelos\n", len(cat.Modulos()), modelos)
	return nil
}

func nombresDe(m catalogo.Modulo) string {
	s := ""
	for i, mo := range m.Modelos {
		if i > 0 {
			s += ", "
		}
		s += mo.Clave
	}
	return s
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("cargar configuracion: %w", err)
	}
	if version != "" {
		cfg.Version = version
	}

	logger := log.New(log.Options{
		Level:   cfg.LogLevel,
		Format:  cfg.LogFormat,
		Service: config.ServiceName,
		Version: cfg.Version,
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx = log.Into(ctx, logger)

	tp, errTel := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if errTel != nil {
		return fmt.Errorf("telemetría: %w", errTel)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	telemetry.SetDefault(tp)

	// El catálogo se valida ANTES de tocar la base de datos o la red: un
	// manifiesto roto tiene que impedir el arranque, no aparecer como un
	// formulario sin rótulos media hora después.
	cat, err := catalogo.Cargar()
	if err != nil {
		return fmt.Errorf("catálogo de módulos: %w", err)
	}

	// --- PostgreSQL (única fuente de verdad) ---------------------------------
	pool, err := pg.Connect(ctx, pg.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DBMaxConns})
	if err != nil {
		return fmt.Errorf("conectar a PostgreSQL: %w", err)
	}
	defer pool.Close()

	if err := pg.MigrateUp(syerpdb.Migrations, syerpdb.MigrationsDir, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("aplicar migraciones: %w", err)
	}
	logger.Info("migraciones aplicadas")

	// Las obligaciones se validan contra el MISMO diccionario que los módulos:
	// un concepto sin sus cinco idiomas no arranca, igual que un campo sin rótulo.
	obl, err := obligaciones.Cargar(func(clave string) bool { return cat.Rotulo(clave, "en") != "" })
	if err != nil {
		return fmt.Errorf("calendario de obligaciones: %w", err)
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

	st := store.New(pool)
	st.Version = cfg.Version

	// VERI*FACTU: sin certificado no se envía nada y nadie lo puede activar, que es
	// mejor que emitir facturas con QR que nunca llegan a la AEAT.
	var remitente *verifactu.Cliente
	if len(cfg.VerifactuCertPEM) > 0 {
		if remitente, err = verifactu.NuevoCliente(cfg.VerifactuCertPEM, cfg.VerifactuKeyPEM, cfg.VerifactuProduccion, cfg.VerifactuSello); err != nil {
			return fmt.Errorf("VERI*FACTU: %w", err)
		}
		go remitirVerifactu(ctx, st, remitente, logger)
	} else {
		logger.Info("VERI*FACTU desactivado: falta el certificado (VERIFACTU_CERT_PEM, VERIFACTU_KEY_PEM)")
	}

	// Sellado de los secretos de firma de webhooks. Sin clave queda deshabilitado
	// (nil) y configurar un webhook se rechaza: guardar en claro un secreto con el
	// que se pueden falsificar cobros no es una alternativa.
	cifra, err := keycrypto.New(cfg.SecretEncKey)
	if err != nil {
		return fmt.Errorf("clave de sellado: %w", err)
	}
	if !cifra.Enabled() {
		logger.Warn("SECRET_ENC_KEY ausente: no se podrán configurar webhooks de Stripe")
	}

	// Avisos por correo de las obligaciones: cada media hora, sin la sesión de nadie.
	go avisos.Ejecutar(ctx, avisos.Deps{
		Store: st, Obligaciones: obl, Log: logger,
		Correo: correo.Config{
			Addr: cfg.SMTPAddr, From: cfg.SMTPFrom, User: cfg.SMTPUser, Password: cfg.SMTPPassword,
			TLSServerName: cfg.SMTPTLSServerName,
		},
		Enlace: strings.TrimRight(cfg.AppURL, "/") + "/?vista=obligaciones",
	})

	router := syerphttp.NewRouter(syerphttp.Deps{
		Cfg:          cfg,
		Store:        st,
		Catalogo:     cat,
		Verifier:     verifier,
		Cifra:        cifra,
		Pasarela:     conectores.NewPasarela(cfg.IntegrationsURL),
		Obligaciones: obl,
		Verifactu:    remitente,
		// 95 s: la ruta de proponer informes tiene 100 (ver syerphttp.plazoPropuesta);
		// el resto de rutas cortan antes, a los 30, por su propio plazo.
		IA:  asistente.Nuevo(cfg.AIURL, 95*time.Second),
		Web: web.Nuevo(),
		ReadyCheckers: []httpx.Checker{
			httpx.CheckerFunc{Label: "postgres", Fn: st.Healthz},
		},
	})

	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	modelos := 0
	for _, m := range cat.Modulos() {
		modelos += len(m.Modelos)
	}
	logger.Info("sytue-syerp iniciado",
		"addr", cfg.HTTPAddr, "public_url", cfg.PublicURL, "env", string(cfg.Env),
		"modulos", len(cat.Modulos()), "modelos", modelos)
	if err := srv.Run(ctx); err != nil {
		return fmt.Errorf("servidor HTTP: %w", err)
	}
	logger.Info("sytue-syerp detenido limpiamente")
	return nil
}

// remitirVerifactu envía lo pendiente de cada organización. Mira cada 30 s, pero
// quien decide si toca es la base: guarda el tiempo de espera que marca la AEAT
// para todas las réplicas.
func remitirVerifactu(ctx context.Context, st *store.Store, c *verifactu.Cliente, logger *slog.Logger) {
	tic := time.NewTicker(30 * time.Second)
	defer tic.Stop()
	for {
		tenants, err := st.TenantsConVerifactuPendiente(ctx)
		if err != nil {
			logger.Warn("VERI*FACTU: no se pudo mirar qué hay pendiente", "error", err)
		}
		for _, t := range tenants {
			tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: t})
			lote, err := st.ReclamarLoteVerifactu(tctx)
			if err != nil || len(lote.IDs) == 0 {
				if err != nil {
					logger.Warn("VERI*FACTU: no se pudo preparar el envío", "tenant", t, "error", err)
				}
				continue
			}
			resp, errEnvio := c.Enviar(tctx, lote.Obligado, lote.XML)
			if errEnvio != nil {
				logger.Warn("VERI*FACTU: envío fallido", "tenant", t, "registros", len(lote.IDs), "error", errEnvio)
			} else {
				logger.Info("VERI*FACTU: enviado", "tenant", t, "registros", len(lote.IDs), "estado", resp.Estado, "csv", resp.CSV)
			}
			if err := st.GuardarRespuestaVerifactu(tctx, lote, resp, errEnvio); err != nil {
				logger.Error("VERI*FACTU: no se pudo guardar la respuesta", "tenant", t, "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tic.C:
		}
	}
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
