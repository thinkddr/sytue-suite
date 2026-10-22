// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command ocr es el binario de Sytue OCR: el microservicio que reconoce el texto
// de una página escaneada para Sytue PDF. Motor Tesseract 5 (Apache-2.0),
// autoalojado y sin ninguna llamada saliente — ver ADR-0034.
//
// Es STATELESS y SIN SECRETOS: no usa base de datos, no escribe en disco más que
// un fichero temporal por página, y el modelo viaja dentro de la imagen.
//
// Subcomandos:
//
//	ocr              arranca el servidor HTTP (por defecto)
//	ocr healthcheck  sonda de liveness para el healthcheck de Docker
package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	"git.sytue.eu/sytue/one/services/ocr/internal/config"
	ocrhttp "git.sytue.eu/sytue/one/services/ocr/internal/http"
	"git.sytue.eu/sytue/one/services/ocr/internal/tesseract"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

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

	tp, err := telemetry.NewProvider(ctx, telemetry.Config{Service: config.ServiceName, Version: cfg.Version})
	if err != nil {
		return fmt.Errorf("telemetría: %w", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
	telemetry.SetDefault(tp)

	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	// El motor se exige AL ARRANCAR y su ausencia es FATAL, al revés que la clave
	// de Sytue AI. La diferencia importa: allí la clave la pone quien despliega y
	// puede faltar legítimamente; aquí el motor va DENTRO de la imagen, así que
	// si no está es que la imagen está mal construida. Arrancar «sano» con un
	// servicio que no puede hacer su único trabajo es cómo se cuela a producción
	// una funcionalidad apagada.
	motor, err := tesseract.Nuevo(ctx)
	if err != nil {
		return fmt.Errorf("motor de OCR: %w", err)
	}
	motor.TiempoMaximo = cfg.Timeout
	logger.Info("motor de OCR listo", "idiomas", motor.Idiomas())

	router := ocrhttp.NewRouter(ocrhttp.Deps{Cfg: cfg, Verifier: verifier, Motor: motor})

	// El reconocimiento tarda: el plazo de escritura tiene que superar el del
	// motor o la respuesta se cortaría a medias justo con las páginas difíciles.
	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		WriteTimeout:    cfg.Timeout + 60*time.Second,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info("Sytue OCR arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "concurrentes", cfg.MaxConcurrentes)
	return srv.Run(ctx)
}

// runHealthcheck es la sonda usada por Docker.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8096"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://127.0.0.1:"+port+"/healthz", http.NoBody)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: estado", res.StatusCode)
		return 1
	}
	return 0
}
