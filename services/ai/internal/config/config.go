// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package config carga la configuración tipada de Sytue AI desde el entorno del
// proceso (12-factor). Reutiliza el cargador común de platform/pkg/config.
//
// SEGURIDAD: la clave del proveedor LLM (AI_API_KEY) vive SOLO en el servidor,
// leída del entorno. NUNCA se registra ni se devuelve al cliente (ver LogValue).
// Es OPCIONAL en el arranque: sin ella el servicio arranca sano pero /assist
// responde 503 (ai_not_configured), de modo que el compose de dev no entra en
// bucle de reinicio cuando no hay clave. El resto de la validación (proveedor,
// base URL, modelo) sí es estricta porque no depende de secretos.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	pcfg "git.sytue.eu/sytue/one/platform/pkg/config"
)

// ServiceName es el nombre lógico del servicio (atributo de logs/telemetría).
const ServiceName = "sytue-ai"

// Version la inyecta el linker en build (-ldflags "-X .../config.Version=..."),
// y sirve de default cuando la variable de entorno VERSION no está definida.
var Version = "dev"

// Proveedores LLM soportados.
const (
	ProviderScaleway     = "scaleway"      // Generative APIs (Francia, datos en la UE; por defecto).
	ProviderOpenAICompat = "openai-compat" // endpoint OpenAI-compatible (modelo soberano/self-hosted).
)

// Config es la configuración completa de Sytue AI.
type Config struct {
	pcfg.Base

	// PublicURL es la URL pública de cara al usuario (https://ai.sytue.eu).
	PublicURL string

	// OIDC configura la validación de tokens de Sytue ID (JWKS). El BFF reenvía
	// el access token de la sesión; se valida firma/issuer/audiencia/expiración.
	OIDCIssuer   string
	OIDCJWKSURL  string
	OIDCAudience string
	// OIDCAcceptedAudiences: la suite comparte sesión (cookie única), así que el
	// token puede traer la audiencia de OTRO app de la suite (p. ej. sytue-docs).
	// Se acepta si su aud contiene ALGUNA (allowlist first-party).
	OIDCAcceptedAudiences []string

	// --- Proveedor LLM -------------------------------------------------------

	// Provider selecciona el backend: scaleway | openai-compat.
	Provider string
	// APIKey es la clave del proveedor. SERVER-ONLY: nunca se loguea ni se
	// devuelve. Opcional en arranque (sin ella, /assist responde 503).
	APIKey string
	// Model es el identificador de modelo. Configurable por env.
	Model string
	// BaseURL es la raíz del proveedor (Scaleway: https://api.scaleway.ai;
	// openai-compat: la URL del endpoint soberano/self-hosted).
	BaseURL string
	// MaxTokens es el tope de tokens de salida por respuesta.
	MaxTokens int
	// Timeout es el plazo máximo de la llamada al proveedor.
	Timeout time.Duration

	// --- Modelo de calidad (modo "calidad", asíncrono) -----------------------
	//
	// DOS MODELOS Y NO UNO, porque miden distinto. Medido el 2026-09-09 contra
	// Scaleway con tres repeticiones por tarea y sin tope de salida:
	//
	//              interactivo (qwen3.8-27b)  gemma-4-26b
	//   latencia   0,6 s (mediana)        14,0 s
	//   coste      0,59 €/10.000          5,66 €/10.000
	//   fórmula    mal 3 de 3             bien 3 de 3
	//   revelado   +1 paso (el doble)     +0,5/+0,1/+0,3 (acertado)
	//
	// Gemma razona antes de responder y por eso acierta donde el pequeño falla;
	// también por eso tarda catorce segundos, que en un panel en línea es un
	// usuario que ya lo ha escrito a mano. Así que el rápido atiende lo
	// interactivo y el bueno lo que se puede esperar.

	// ModelCalidad es el modelo del modo "calidad". Vacío = no hay segundo
	// modelo y "calidad" cae al modelo interactivo (degradación honesta).
	ModelCalidad string
	// MaxTokensCalidad es el tope de salida del modelo de calidad. MUY superior
	// al interactivo A PROPÓSITO: los tokens de razonamiento salen por aquí y se
	// facturan como salida. Con 1024 —el tope del interactivo— gemma-4 se queda
	// a media frase y devuelve la respuesta VACÍA, que es un 502 sin causa
	// visible. Medido: mediana 692, la fórmula de Calcus pasa de 2000.
	MaxTokensCalidad int
	// TimeoutCalidad es el plazo del modelo de calidad. El del interactivo se
	// queda corto: 14 s de mediana y 34 s en el peor caso medido.
	TimeoutCalidad time.Duration
	// EsfuerzoCalidad es el `reasoning_effort` por defecto del modelo de
	// calidad: none | low | medium | high.
	//
	// `high` Y NO EL DEFECTO DEL MODELO, que es contraintuitivo y por eso queda
	// escrito. Medido el 2026-09-09, tres repeticiones sobre fórmula, extracción
	// y receta de revelado:
	//
	//   none     0,76 €/10.000   1,0 s   bien en texto, MAL en visión
	//   low      6,13 €/10.000   9,9 s   7 de 9
	//   medium   6,21 €/10.000   9,9 s   9 de 9
	//   high     5,43 €/10.000   8,1 s   9 de 9
	//   sin fijar 6,07 €/10.000  8,8 s   9 de 9
	//
	// `high` gana al defecto en las tres columnas: razona mejor y para antes,
	// mientras que `low` y `medium` divagan. Quien llame puede pedir `none` por
	// petición cuando sepa que su tarea es de texto: son ocho veces menos.
	EsfuerzoCalidad string

	// --- Audio y vectores ----------------------------------------------------

	// ModelAudio transcribe (whisper-large-v3). Vacío => /transcribe da 503.
	// No entra en el presupuesto: se factura por minuto de audio (0,003 €), no
	// por token, y una reunión de una hora cuesta 18 céntimos.
	ModelAudio string
	// ModelEmbeddings vectoriza (qwen3-embedding-8b), para la búsqueda
	// semántica. Vacío => sin embeddings.
	ModelEmbeddings string

	// --- Presupuesto por usuario ---------------------------------------------

	// PresupuestoMensual es el tope BLANDO en euros por usuario y mes sobre el
	// modelo de calidad. Al agotarlo NO se corta: se sirve con el modelo rápido.
	// <= 0 => sin tope.
	PresupuestoMensual float64
	// Tarifas es el precio por modelo en €/millón de tokens, "modelo:entrada:salida"
	// separado por comas. Se declara y no se codifica porque los precios del
	// proveedor cambian sin avisar, y una tarifa vieja no falla: cobra mal.
	Tarifas string
	// DragonflyURL es la caché donde vive el contador del presupuesto. Vacía =>
	// no se cuenta y no hay tope (se degrada ABIERTO a propósito).
	DragonflyURL string
	// CachePassword autentica contra la caché. La variable se llama
	// DRAGONFLY_PASSWORD, como en los otros siete servicios que usan la misma
	// caché: el gate del despliegue cuenta que los ocho la reciban con ese
	// nombre, y un nombre propio aquí lo dejaría ciego para este servicio.
	CachePassword string

	// --- Límites de la frontera de confianza ---------------------------------

	// MaxInstructionBytes acota la instrucción (orden del usuario).
	MaxInstructionBytes int
	// MaxContextBytes acota el contexto (texto del documento, tratado como datos).
	MaxContextBytes int

	// --- Integración con Sytue Web (§9.1) ---------------------------------

	// PagesSecret abre /internal/v1/assist y /internal/v1/models. Sin secreto
	// no se monta la ruta interna (httpx.SoloDesdeDentro).
	PagesSecret string
	// TimeoutPages es el plazo por llamada para la generación de Pages.
	TimeoutPages time.Duration
	// MaxTokensTope es el tope máximo de tokens de salida permitidos para Pages.
	MaxTokensTope int
	// ModelCodigo es el modelo preferido para tareas de código.
	ModelCodigo string

	// --- Rate limit por usuario/tenant ---------------------------------------

	// RateLimitPerMinute es la tasa sostenida de peticiones por usuario/tenant.
	RateLimitPerMinute int
	// RateLimitBurst es la ráfaga permitida por usuario/tenant.
	RateLimitBurst int
}

// LogValue implementa slog.LogValuer para que registrar la Config NUNCA filtre
// la API key. Solo expone campos no sensibles (proveedor, modelo, base URL,
// límites) y si la clave está presente (booleano), jamás su valor.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("provider", c.Provider),
		slog.String("model", c.Model),
		slog.String("base_url", c.BaseURL),
		slog.Bool("api_key_set", c.APIKey != ""),
		slog.Int("max_tokens", c.MaxTokens),
		slog.String("model_calidad", c.ModelCalidad),
		slog.Int("max_tokens_calidad", c.MaxTokensCalidad),
		slog.Bool("pages_secret_set", c.PagesSecret != ""),
		slog.String("model_codigo", c.ModelCodigo),
	)
}

// defaultModel devuelve el modelo por defecto según el proveedor. El modelo
// Mistral Small servido por Scaleway equilibra latencia/coste y mantiene la
// inferencia en centros de datos europeos. Para openai-compat lo fija el operador.
func defaultModel(provider string) string {
	if provider == ProviderScaleway {
		// Preferencia declarada (2026-09-19): qwen3.8-27b por delante de
		// mistral-small. Todo serverless de Generative APIs: nada de GPU.
		return "qwen3.8-27b"
	}
	return ""
}

// defaultModelCalidad devuelve el modelo del modo "calidad". Solo para Scaleway:
// en un endpoint soberano cualquiera no se puede suponer qué modelos sirve, así
// que allí hay que declararlo con AI_MODEL_CALIDAD o no habrá segundo modelo.
func defaultModelCalidad(provider string) string {
	if provider == ProviderScaleway {
		return "gemma-4-26b-a4b-it"
	}
	return ""
}

// defaultModelAudio y defaultModelEmbeddings: los especializados. Solo para
// Scaleway, por lo mismo que el de calidad.
func defaultModelAudio(provider string) string {
	if provider == ProviderScaleway {
		return "whisper-large-v3"
	}
	return ""
}

func defaultModelEmbeddings(provider string) string {
	if provider == ProviderScaleway {
		return "qwen3-embedding-8b"
	}
	return ""
}

// tarifasPorDefecto son los precios publicados de Scaleway consultados el
// 2026-09-09, en €/millón de tokens (entrada:salida). Solo están los modelos que
// este servicio puede usar; uno que no aparezca se sirve pero no se cobra.
const tarifasPorDefecto = "mistral-small-3.2-24b-instruct-2506:0.15:0.35," +
	"gemma-4-26b-a4b-it:0.25:0.50," +
	"qwen3-embedding-8b:0.10:0," +
	"qwen3.8-27b:0.20:0.40," +
	"qwen3.5-397b-a17b:0.60:1.80"

// ParsearTarifas convierte "modelo:entrada:salida,..." en un mapa. Una entrada
// mal escrita se DESCARTA con su motivo en vez de tumbar el arranque: sin tarifa
// el modelo se sirve igual y no se cobra, que es preferible a quedarse sin
// asistente por una coma.
func ParsearTarifas(raw string) (map[string][2]float64, []error) {
	out := make(map[string][2]float64)
	var errs []error
	for _, entrada := range strings.Split(raw, ",") {
		entrada = strings.TrimSpace(entrada)
		if entrada == "" {
			continue
		}
		partes := strings.Split(entrada, ":")
		if len(partes) != 3 {
			errs = append(errs, fmt.Errorf("tarifa %q: se esperaba modelo:entrada:salida", entrada))
			continue
		}
		ent, err1 := strconv.ParseFloat(partes[1], 64)
		sal, err2 := strconv.ParseFloat(partes[2], 64)
		if err1 != nil || err2 != nil {
			errs = append(errs, fmt.Errorf("tarifa %q: precios no numéricos", entrada))
			continue
		}
		out[strings.TrimSpace(partes[0])] = [2]float64{ent, sal}
	}
	return out, errs
}

// defaultBaseURL devuelve la base URL por defecto según el proveedor.
func defaultBaseURL(provider string) string {
	if provider == ProviderScaleway {
		return "https://api.scaleway.ai"
	}
	return ""
}

// Load construye la configuración leyendo el entorno. Devuelve todos los
// errores de validación juntos.
func Load() (Config, error) {
	base, err := pcfg.LoadBase("", ServiceName)
	if err != nil {
		return Config{}, err
	}
	if _, ok := os.LookupEnv("VERSION"); !ok {
		base.Version = Version
	}

	l := pcfg.NewLoader("")

	provider := strings.ToLower(strings.TrimSpace(l.GetDefault("AI_PROVIDER", ProviderScaleway)))

	cfg := Config{
		Base:      base,
		PublicURL: l.GetDefault("PUBLIC_URL", "https://ai.sytue.localhost"),

		OIDCIssuer:            l.GetDefault("OIDC_ISSUER", "https://id.sytue.localhost"),
		OIDCJWKSURL:           l.Get("OIDC_JWKS_URL"),
		OIDCAudience:          l.GetDefault("OIDC_AUDIENCE", "sytue-ai"),
		OIDCAcceptedAudiences: l.GetCSVDefault("OIDC_ACCEPTED_AUDIENCES", pcfg.FirstPartyAudiences("sytue-ai")),

		Provider:  provider,
		APIKey:    l.Get("AI_API_KEY"), // server-only; opcional en arranque.
		Model:     l.GetDefault("AI_MODEL", defaultModel(provider)),
		BaseURL:   strings.TrimRight(l.GetDefault("AI_BASE_URL", defaultBaseURL(provider)), "/"),
		MaxTokens: l.GetIntDefault("AI_MAX_TOKENS", 1024),
		Timeout:   l.GetDurationDefault("AI_TIMEOUT", 60*time.Second),

		ModelCalidad:     l.GetDefault("AI_MODEL_CALIDAD", defaultModelCalidad(provider)),
		MaxTokensCalidad: l.GetIntDefault("AI_MAX_TOKENS_CALIDAD", 4096),
		TimeoutCalidad:   l.GetDurationDefault("AI_TIMEOUT_CALIDAD", 120*time.Second),
		EsfuerzoCalidad:  strings.ToLower(strings.TrimSpace(l.GetDefault("AI_ESFUERZO_CALIDAD", "high"))),

		ModelAudio:      l.GetDefault("AI_MODEL_AUDIO", defaultModelAudio(provider)),
		ModelEmbeddings: l.GetDefault("AI_MODEL_EMBEDDINGS", defaultModelEmbeddings(provider)),

		PresupuestoMensual: l.GetFloatDefault("AI_PRESUPUESTO_MENSUAL_EUR", 1.50),
		Tarifas:            l.GetDefault("AI_TARIFAS", tarifasPorDefecto),
		DragonflyURL:       l.Get("DRAGONFLY_URL"),
		CachePassword:      l.Get("DRAGONFLY_PASSWORD"),

		MaxInstructionBytes: l.GetIntDefault("AI_MAX_INSTRUCTION_BYTES", 8*1024),
		MaxContextBytes:     l.GetIntDefault("AI_MAX_CONTEXT_BYTES", 256*1024),

		PagesSecret:   l.SecretoOpcional(base.Env, "AI_PAGES_SECRET", pcfg.DevAIPagesSecret),
		TimeoutPages:  l.GetDurationDefault("AI_TIMEOUT_PAGES", 300*time.Second),
		MaxTokensTope: l.GetIntDefault("AI_MAX_TOKENS_TOPE", 16384),
		ModelCodigo:   l.GetDefault("AI_MODEL_CODIGO", "qwen3.5-397b-a17b"), // ponytail: sin medir

		RateLimitPerMinute: l.GetIntDefault("AI_RATE_LIMIT_PER_MIN", 20),
		RateLimitBurst:     l.GetIntDefault("AI_RATE_LIMIT_BURST", 5),
	}

	// PORT (compose) tiene prioridad sobre HTTP_ADDR para fijar la escucha.
	if port := l.Get("PORT"); port != "" {
		cfg.HTTPAddr = ":" + port
	} else if cfg.HTTPAddr == "" || cfg.HTTPAddr == ":8080" {
		cfg.HTTPAddr = ":8084"
	}

	if err := l.Err(); err != nil {
		return Config{}, err
	}

	if cfg.OIDCJWKSURL == "" {
		cfg.OIDCJWKSURL = cfg.OIDCIssuer + "/.well-known/jwks.json"
	}

	// Validación estructural del proveedor (NO depende de secretos): fallar
	// rápido ante una mala configuración de despliegue.
	switch cfg.Provider {
	case ProviderScaleway, ProviderOpenAICompat:
	default:
		return Config{}, fmt.Errorf("AI_PROVIDER inválido: %q (scaleway|openai-compat)", cfg.Provider)
	}
	if cfg.BaseURL == "" {
		return Config{}, fmt.Errorf("AI_BASE_URL es obligatorio para el proveedor %q", cfg.Provider)
	}
	if cfg.Model == "" {
		return Config{}, fmt.Errorf("AI_MODEL es obligatorio para el proveedor %q", cfg.Provider)
	}
	if cfg.MaxTokens <= 0 {
		return Config{}, fmt.Errorf("AI_MAX_TOKENS debe ser > 0")
	}
	// El modelo de calidad es OPCIONAL: sin él, "calidad" cae al interactivo. Lo
	// que no se admite es declararlo con un tope que lo dejaría truncando.
	if cfg.ModelCalidad != "" && cfg.MaxTokensCalidad <= 0 {
		return Config{}, fmt.Errorf("AI_MAX_TOKENS_CALIDAD debe ser > 0")
	}
	switch cfg.EsfuerzoCalidad {
	case "", "none", "low", "medium", "high":
	default:
		return Config{}, fmt.Errorf("AI_ESFUERZO_CALIDAD inválido: %q (none|low|medium|high)", cfg.EsfuerzoCalidad)
	}
	if cfg.MaxContextBytes <= 0 || cfg.MaxInstructionBytes <= 0 {
		return Config{}, fmt.Errorf("los límites de tamaño deben ser > 0")
	}
	if cfg.TimeoutPages <= 0 {
		return Config{}, fmt.Errorf("AI_TIMEOUT_PAGES debe ser > 0")
	}
	if cfg.MaxTokensTope <= 0 {
		return Config{}, fmt.Errorf("AI_MAX_TOKENS_TOPE debe ser > 0")
	}

	return cfg, nil
}
