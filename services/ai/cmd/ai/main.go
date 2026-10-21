// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command ai es el binario de Sytue AI: el microservicio que hace funcionar el
// asistente del editor. Atiende POST /api/v1/assist (autenticado por JWT de
// Sytue ID) y delega en un proveedor LLM configurable (Scaleway/Mistral por
// defecto, o un endpoint OpenAI-compatible soberano). Es STATELESS: no usa base
// de datos ni almacenamiento; la API key vive solo en el entorno del servidor.
//
// Subcomandos:
//
//	ai              arranca el servidor HTTP (por defecto)
//	ai healthcheck  sonda de liveness para el healthcheck de Docker
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/cache"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/telemetry"

	"git.sytue.eu/sytue/one/services/ai/internal/config"
	aihttp "git.sytue.eu/sytue/one/services/ai/internal/http"
	"git.sytue.eu/sytue/one/services/ai/internal/presupuesto"
	"git.sytue.eu/sytue/one/services/ai/internal/provider"
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

	// Verificador de JWT de Sytue ID (JWKS). Valida la sesión que reenvía el BFF.
	verifier, err := authn.NewVerifier(ctx, authn.VerifierConfig{
		Issuer:    cfg.OIDCIssuer,
		JWKSURL:   cfg.OIDCJWKSURL,
		Audiences: cfg.OIDCAcceptedAudiences,
	})
	if err != nil {
		return fmt.Errorf("verificador OIDC: %w", err)
	}

	// Proveedor LLM. Sin API key, el servicio arranca SANO pero /assist responde
	// 503 (así el compose de dev no entra en bucle de reinicio sin clave).
	prov, perr := provider.New(provider.Config{
		Provider:  cfg.Provider,
		APIKey:    cfg.APIKey,
		Model:     cfg.Model,
		BaseURL:   cfg.BaseURL,
		MaxTokens: cfg.MaxTokens,
		Timeout:   cfg.Timeout,
	})
	switch {
	case errors.Is(perr, provider.ErrNotConfigured):
		logger.Warn("proveedor de IA sin API key: /assist responderá 503 hasta configurar AI_API_KEY",
			"provider", cfg.Provider)
		prov = nil
	case perr != nil:
		return fmt.Errorf("proveedor de IA: %w", perr)
	}

	// Segundo proveedor, el del modo "calidad". Es el MISMO backend con otro
	// modelo, otro tope de salida y otro plazo: no hay una segunda credencial ni
	// un segundo despliegue. Si no está configurado, `calidad` cae al de arriba.
	var provCalidad provider.Provider
	if prov != nil && cfg.ModelCalidad != "" {
		provCalidad, perr = provider.New(provider.Config{
			Provider:  cfg.Provider,
			APIKey:    cfg.APIKey,
			Model:     cfg.ModelCalidad,
			BaseURL:   cfg.BaseURL,
			MaxTokens: cfg.MaxTokensCalidad,
			Esfuerzo:  cfg.EsfuerzoCalidad,
			Timeout:   cfg.TimeoutCalidad,
		})
		if perr != nil {
			return fmt.Errorf("proveedor de IA (calidad): %w", perr)
		}
		// `tope_salida` y no `max_tokens`: el redactor del logger tapa esa
		// clave por si acaso y la deja en [REDACTED]. Esta línea existe
		// precisamente para poder ver ese número cuando algo salga truncado.
		logger.Info("modo calidad disponible", "model", cfg.ModelCalidad,
			"tope_salida", cfg.MaxTokensCalidad, "esfuerzo", cfg.EsfuerzoCalidad,
			"timeout", cfg.TimeoutCalidad)
	}

	// Transcripción de audio (whisper). Otro modelo y otro endpoint; no entra en
	// el presupuesto porque se factura por minuto de audio, no por token.
	var transcriptor provider.Transcriptor
	if prov != nil && cfg.ModelAudio != "" {
		transcriptor, perr = provider.NuevoTranscriptor(provider.Config{
			Provider: cfg.Provider, APIKey: cfg.APIKey, Model: cfg.ModelAudio,
			BaseURL: cfg.BaseURL, Timeout: cfg.TimeoutCalidad,
		})
		if perr != nil {
			return fmt.Errorf("transcriptor: %w", perr)
		}
		logger.Info("transcripción disponible", "model", cfg.ModelAudio)
	}

	// Vectores de texto (qwen3-embedding-8b, serverless). Es la pieza que
	// retira la GPU de lote de Sytue Photos: descripción → vector, sin GPU.
	var embebedor provider.Embebedor
	if prov != nil && cfg.ModelEmbeddings != "" {
		embebedor, perr = provider.NuevoEmbebedor(provider.Config{
			Provider: cfg.Provider, APIKey: cfg.APIKey, Model: cfg.ModelEmbeddings,
			BaseURL: cfg.BaseURL, Timeout: cfg.TimeoutCalidad,
		})
		if perr != nil {
			return fmt.Errorf("embebedor: %w", perr)
		}
		logger.Info("embeddings disponible", "model", cfg.ModelEmbeddings)
	}

	// LA CACHÉ ES OPCIONAL Y SU AUSENCIA SE DICE. Sin ella no hay contador y por
	// tanto no hay tope: el modelo caro queda sin freno por usuario. Se degrada
	// abierto a propósito —quedarse sin asistente por una pieza opcional es
	// peor—, pero el operador tiene que poder saberlo, y por eso este aviso.
	var contador *presupuesto.Contador
	if cfg.DragonflyURL == "" {
		if cfg.PresupuestoMensual > 0 {
			logger.Warn("sin DRAGONFLY_URL no hay contador de presupuesto: el modelo de calidad queda SIN tope por usuario",
				"tope_configurado_eur", cfg.PresupuestoMensual)
		}
	} else {
		cli, cerr := cache.NewConAuth(ctx, cfg.DragonflyURL, cfg.CachePassword)
		if cerr != nil && cli == nil {
			return fmt.Errorf("caché del presupuesto: %w", cerr)
		}
		if cerr != nil {
			// New devuelve el cliente aunque el Ping falle: la sonda lo recupera.
			logger.Warn("caché del presupuesto arrancó degradada", "error", cerr)
		}
		defer func() { _ = cli.Close() }()
		contador = presupuesto.Nuevo(cli, cfg.PresupuestoMensual)
		logger.Info("presupuesto por usuario activo", "tope_mensual_eur", cfg.PresupuestoMensual)
	}

	// Tarifas: una entrada mal escrita se descarta con su motivo, no tumba el
	// arranque. Un modelo sin tarifa se sirve igual y no se cobra.
	crudas, errsTarifa := config.ParsearTarifas(cfg.Tarifas)
	for _, e := range errsTarifa {
		logger.Warn("tarifa descartada", "error", e)
	}
	tarifas := make(map[string]presupuesto.Precio, len(crudas))
	for modelo, pr := range crudas {
		tarifas[modelo] = presupuesto.Precio{Entrada: pr[0], Salida: pr[1]}
	}
	if _, hay := tarifas[cfg.ModelCalidad]; !hay && cfg.ModelCalidad != "" && cfg.PresupuestoMensual > 0 {
		// El caso que hace inútil el tope sin que nadie lo note: hay
		// presupuesto, hay modelo caro, y no hay con qué cobrarle.
		logger.Warn("el modelo de calidad no tiene tarifa: su gasto NO se cobrará al presupuesto",
			"model", cfg.ModelCalidad)
	}

	// Rate-limit por usuario/tenant, en memoria (token bucket).
	// ponytail: límite POR RÉPLICA (in-memory). Para un tope global entre
	// réplicas, sustituir por un RateLimiter compartido (p. ej. pg.RateLimiter).
	limiter := httpx.NewTokenBucket(float64(cfg.RateLimitPerMinute)/60.0, cfg.RateLimitBurst)

	router := aihttp.NewRouter(aihttp.Deps{
		Cfg:             cfg,
		Verifier:        verifier,
		Provider:        prov,
		ProviderCalidad: provCalidad,
		Transcriptor:    transcriptor,
		Embebedor:       embebedor,
		Presupuesto:     contador,
		Tarifas:         tarifas,
		Limiter:         limiter,
	})

	// La inferencia puede tardar: el WriteTimeout del servidor debe superar el
	// plazo de la llamada al proveedor, o cortaría la respuesta a medias.
	srv := httpx.NewServer(httpx.ServerConfig{
		Addr:            cfg.HTTPAddr,
		Handler:         router,
		WriteTimeout:    max(cfg.Timeout, cfg.TimeoutCalidad, cfg.TimeoutPages) + 30*time.Second,
		ShutdownTimeout: cfg.ShutdownTimeout,
	})

	logger.Info("Sytue AI arrancando",
		"addr", cfg.HTTPAddr, "env", string(cfg.Env), "config", cfg)
	return srv.Run(ctx)
}

// runHealthcheck es la sonda usada por Docker (CMD ["/app/ai","healthcheck"]).
// Consulta /healthz en el puerto local y devuelve 0 si está sano.
func runHealthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
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
