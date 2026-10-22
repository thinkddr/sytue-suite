// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package aihttp implementa la superficie HTTP de Sytue AI: un único endpoint
// autenticado POST /api/v1/assist que atiende al asistente del editor.
//
// Contrato (alineado con el BFF packages/office/src/pages/api/ai.ts): recibe
// { action, instruction, context, kind } con Authorization: Bearer <sesión> y
// responde { text } (o un error). El BFF apunta SYTUE_AI_URL a este endpoint.
package aihttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/ai/internal/config"
	"git.sytue.eu/sytue/one/services/ai/internal/presupuesto"
	"git.sytue.eu/sytue/one/services/ai/internal/provider"
)

// errNotConfigured se devuelve cuando el servicio no tiene proveedor (sin API
// key). 503 con el mismo código que espera el BFF ("ai_not_configured").
// Modos de atención. Se validan como una lista cerrada: un modo mal escrito
// debe decirlo, no caer en silencio al rápido y dejar a alguien preguntándose
// por qué su extracción de facturas sigue fallando.
const (
	modoInteractivo = "interactivo"
	modoCalidad     = "calidad"
)

var errNotConfigured = httpx.NewError(http.StatusServiceUnavailable, "ai_not_configured",
	"El asistente de IA no está configurado.")

// Deps son las dependencias del router de Sytue AI.
type Deps struct {
	Cfg      config.Config
	Verifier *authn.Verifier
	// Provider es el backend LLM interactivo. Puede ser nil (sin API key):
	// /assist responde 503.
	Provider provider.Provider
	// ProviderCalidad atiende `"modo":"calidad"`. Puede ser nil: entonces
	// "calidad" se sirve con el interactivo, que responde peor pero responde.
	ProviderCalidad provider.Provider
	// Limiter aplica el rate-limit por usuario/tenant.
	Limiter httpx.RateLimiter
	// Presupuesto lleva el gasto mensual por usuario en el modelo caro. Puede
	// ser nil: entonces no hay tope.
	Presupuesto Presupuestador
	// Tarifas es el precio por modelo (€/millón de tokens). Un modelo que no
	// esté aquí se sirve igual pero NO se cobra al presupuesto, y se dice en el
	// log: es mejor que inventarse una tarifa.
	Tarifas map[string]presupuesto.Precio
	// Transcriptor atiende /transcribe (whisper). Puede ser nil: 503.
	Transcriptor provider.Transcriptor
	// Embebedor atiende /embeddings (qwen3-embedding-8b, serverless). Puede
	// ser nil: 503. Es la pieza que retira la GPU de lote de Sytue Photos.
	Embebedor provider.Embebedor
}

// NewRouter construye el router completo de Sytue AI.
func NewRouter(d Deps) http.Handler {
	// El plazo de la petición debe superar el de la llamada al proveedor para
	// que sea el handler (con un 504 legible) quien gane, no el timeout del
	// router. Se toma el MAYOR de los dos modos: con el del interactivo, una
	// petición en modo calidad moriría a los 75 s con un error del router —sin
	// pista de qué la mató— en vez del 504 que sí explica lo que pasó.
	plazo := d.Cfg.Timeout
	if d.Cfg.TimeoutCalidad > plazo {
		plazo = d.Cfg.TimeoutCalidad
	}
	if d.Cfg.TimeoutPages > plazo {
		plazo = d.Cfg.TimeoutPages
	}
	r := httpx.NewRouter(httpx.RouterOptions{RequestTimeout: plazo + 15*time.Second})
	httpx.MountHealth(r, d.Cfg.Version, 3*time.Second)

	h := &handlers{
		cfg: d.Cfg, provider: d.Provider, calidad: d.ProviderCalidad,
		presupuesto: d.Presupuesto, tarifas: d.Tarifas, transcriptor: d.Transcriptor,
		embebedor: d.Embebedor,
	}

	r.Route("/api/v1", func(api chi.Router) {
		// 1) Autenticación JWT de Sytue ID (valida la sesión que reenvía el BFF).
		api.Use(d.Verifier.RequireAuth)
		// 2) Un token de bootstrap para pagar nunca llega al proveedor de IA.
		api.Use(d.Verifier.RequireTenant)
		// 3) Rate-limit POR USUARIO/TENANT (después de auth, para leer los claims).
		if d.Limiter != nil {
			api.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: d.Limiter, KeyFunc: rateLimitKey}))
		}
		api.Post("/assist", h.assist)
		// Transcripción de audio. NO pasa por el presupuesto del modelo caro:
		// whisper se factura por minuto de audio (0,003 €) y no por token, así
		// que una hora de reunión cuesta 18 céntimos. Lo que lo acota es el rate
		// limit y el tope de tamaño del cuerpo, no un contador de euros.
		api.Post("/transcribe", h.transcribe)
		// Vectores de TEXTO (qwen3-embedding-8b, serverless). La búsqueda
		// semántica —y la vectorización de Sytue Photos tras retirar la GPU
		// de lote— pasa por aquí: descripción de la foto → vector.
		api.Post("/embeddings", h.embeddings)
	})

	if d.Cfg.PagesSecret != "" {
		r.Route("/internal/v1", func(interna chi.Router) {
			interna.Use(httpx.SoloDesdeDentro(d.Cfg.PagesSecret))
			interna.Use(verificarIdentidadAfirmada)
			if d.Limiter != nil {
				interna.Use(httpx.RateLimit(httpx.RateLimitConfig{Limiter: d.Limiter, KeyFunc: rateLimitKeyAfirmada}))
			}
			interna.Post("/assist", h.assistPages)
			interna.Post("/models", h.modelsPages)
		})
	}

	return r
}

type handlers struct {
	cfg          config.Config
	provider     provider.Provider
	calidad      provider.Provider
	presupuesto  Presupuestador
	tarifas      map[string]presupuesto.Precio
	transcriptor provider.Transcriptor
	embebedor    provider.Embebedor
}

// embeddingsRequest son los textos a vectorizar. El modelo NO se elige aquí:
// es el qwen3-embedding-8b de la configuración del servicio — un solo espacio
// de vectores por despliegue, o los parecidos salen inventados.
type embeddingsRequest struct {
	// Entradas: hasta 64 textos de 8 KiB cada uno. Suficiente para un lote de
	// descripciones de Sytue Photos y acotado contra peticiones kilométricas.
	Entradas []string `json:"inputs"`
}

// embeddings convierte TEXTO en vectores con el modelo de embeddings
// serverless. Sustituye a la GPU de lote: el vector de una foto sale de su
// descripción (que escribe la visión de gemma-4 en /assist), no de los
// píxeles — el catálogo serverless no trae ningún codificador de imagen y
// este camino entero va por Generative APIs.
func (h *handlers) embeddings(w http.ResponseWriter, r *http.Request) {
	if h.embebedor == nil {
		httpx.Fail(w, r, errNotConfigured)
		return
	}
	var req embeddingsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if len(req.Entradas) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("inputs is required."))
		return
	}
	if len(req.Entradas) > 64 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many inputs (64 maximum)."))
		return
	}
	for _, e := range req.Entradas {
		if len(e) > 8*1024 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Input too long (8 KiB maximum per input)."))
			return
		}
	}
	vectores, tokens, err := h.embebedor.Embeder(r.Context(), req.Entradas)
	if err != nil {
		log.From(r.Context()).Warn("embeddings falló", "error", err.Error())
		httpx.Fail(w, r, httpx.ErrInternal.WithMessage("embedding failed."))
		return
	}
	_ = tokens // el embeddings es de céntimos por millón y sin salida; no hay
	// presupuesto por usuario que contabilizar aquí todavía.
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"model":      h.embebedor.Modelo(),
		"embeddings": vectores,
		"tokens":     tokens,
	})
}

// rateLimitKey deriva la clave de límite de los claims (tenant + usuario). Se
// ejecuta tras RequireAuth, así que los claims están presentes; si por lo que
// fuera faltaran, cae a la IP para no dejar la ruta sin límite.
func rateLimitKey(r *http.Request) string {
	if c, ok := authn.FromContext(r.Context()); ok && c.Subject != "" {
		return "u:" + c.Tenant + ":" + c.Subject
	}
	return "ip:" + clientIP(r)
}

// clientIP extrae una IP de cliente razonable para el fallback del rate-limit.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	return r.RemoteAddr
}

// Presupuestador es lo que el handler necesita del contador de gasto.
//
// INTERFAZ Y NO EL TIPO CONCRETO por una razón concreta: el contador de verdad
// se degrada ABIERTO cuando no hay caché —permite siempre—, así que con el tipo
// concreto no hay forma de escribir el test del caso que importa, que es el
// usuario que ya agotó su presupuesto. Lo implementa *presupuesto.Contador.
type Presupuestador interface {
	// Permite dice si el usuario puede usar el modelo caro, y cuánto lleva.
	Permite(ctx context.Context, tenant, usuario string) (bool, float64)
	// Cobrar suma el coste de una petición ya servida y devuelve el total.
	Cobrar(ctx context.Context, tenant, usuario string, euros float64) float64
}

// quienEs saca tenant y usuario de los claims. Se ejecuta tras RequireAuth, así
// que están; el respaldo por IP existe para que un fallo de claims no deje el
// presupuesto SIN clave —todas las peticiones compartirían contador y el primer
// usuario del mes agotaría el de todos—.
func quienEs(r *http.Request) (tenant, usuario string) {
	if c, ok := authn.FromContext(r.Context()); ok && c.Subject != "" {
		return c.Tenant, c.Subject
	}
	return "sin-tenant", "ip:" + clientIP(r)
}

// cobrar apunta al presupuesto del usuario lo que costó la petición y devuelve
// el total del mes. Devuelve 0 si el modelo no tiene tarifa declarada.
func (h *handlers) cobrar(ctx context.Context, tenant, usuario string, prov provider.Provider, res provider.Resultado) float64 {
	if h.presupuesto == nil || res.TokensEntrada+res.TokensSalida == 0 {
		return 0
	}
	tarifa, hay := h.tarifas[prov.Modelo()]
	if !hay {
		log.From(ctx).Warn("modelo sin tarifa declarada: la peticion no se cobra al presupuesto",
			"modelo", prov.Modelo())
		return 0
	}
	return h.presupuesto.Cobrar(ctx, tenant, usuario, tarifa.Coste(res.TokensEntrada, res.TokensSalida))
}

// elegir devuelve el proveedor y el plazo del modo pedido.
//
// EL MODO LO DECIDE QUIEN LLAMA, no el servicio a partir de la acción. Un panel
// en línea sabe que su usuario está mirando el cursor; un trabajo de fondo sabe
// que no hay nadie esperando. Adivinarlo aquí desde `action` significaría que
// añadir una acción nueva cambia en silencio cuánto tarda la existente.
//
// Si no hay modelo de calidad configurado, "calidad" NO falla: usa el
// interactivo. Vale más una respuesta peor que un 503 por una variable de
// entorno que nadie puso.
func (h *handlers) elegir(modo string) (provider.Provider, time.Duration) {
	if modo == modoCalidad && h.calidad != nil {
		return h.calidad, h.cfg.TimeoutCalidad
	}
	return h.provider, h.cfg.Timeout
}

// assistRequest es el cuerpo esperado. Coincide con { action, instruction,
// context, kind } que compone el panel y reenvía el BFF.
type assistRequest struct {
	Action      string `json:"action"`
	Instruction string `json:"instruction"`
	Context     string `json:"context"`
	Kind        string `json:"kind"`
	// Modo elige el modelo: "" o "interactivo" (rápido, por defecto) y
	// "calidad" (acierta más, tarda del orden de quince segundos).
	Modo string `json:"modo,omitempty"`
	// Esfuerzo pisa el `reasoning_effort` del modo: none | low | medium | high.
	// Lo pone quien llama porque depende de la TAREA y no del despliegue: en
	// texto, `none` acierta igual por la octava parte del coste; en visión se
	// deja la exposición y hace falta `medium` o más. Medido.
	Esfuerzo string `json:"esfuerzo,omitempty"`
	// Imagen en base64, para las tareas de visión (OCR de un escaneo, receta de
	// revelado de una foto). Vacía => petición solo de texto.
	Imagen string `json:"imagen,omitempty"`
	// ImagenTipo es el MIME de Imagen. Por defecto image/png.
	ImagenTipo string `json:"imagen_tipo,omitempty"`
}

// validate aplica los límites de la frontera de confianza (el cuerpo llega del
// navegador vía el BFF).
func (req assistRequest) validate(cfg config.Config) error {
	if strings.TrimSpace(req.Instruction) == "" {
		return httpx.ErrBadRequest.WithMessage("An instruction is required.")
	}
	if len(req.Instruction) > cfg.MaxInstructionBytes {
		return httpx.ErrBadRequest.WithMessage("The instruction is too long.")
	}
	if len(req.Context) > cfg.MaxContextBytes {
		return httpx.ErrBadRequest.WithMessage("The document context is too large.")
	}
	if len(req.Action) > 64 || len(req.Kind) > 64 {
		return httpx.ErrBadRequest.WithMessage("Invalid request parameters.")
	}
	switch req.Modo {
	case "", modoInteractivo, modoCalidad:
	default:
		return httpx.ErrBadRequest.WithMessage("Unknown mode.")
	}
	// Lista cerrada, y con los MISMOS valores que acepta el proveedor: uno
	// inventado sale de aquí con un 400 legible en vez de con el 502 opaco que
	// devolvería el servidor de inferencia.
	switch req.Esfuerzo {
	case "", "none", "low", "medium", "high":
	default:
		return httpx.ErrBadRequest.WithMessage("Unknown reasoning effort.")
	}
	// La imagen viaja en base64 dentro del JSON, así que ocupa un tercio más de
	// lo que pesa. Se acota con el mismo tope que el contexto.
	if len(req.Imagen) > cfg.MaxContextBytes {
		return httpx.ErrBadRequest.WithMessage("The image is too large.")
	}
	return nil
}

// assist atiende el asistente del editor: valida, construye un prompt seguro
// (contexto como datos) y delega en el proveedor. Responde { text }.
func (h *handlers) assist(w http.ResponseWriter, r *http.Request) {
	if h.provider == nil {
		httpx.Fail(w, r, errNotConfigured)
		return
	}

	var req assistRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := req.validate(h.cfg); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	// EL PRESUPUESTO SE MIRA ANTES DE ELEGIR EL MODELO, que es lo que hace que
	// agotarlo NO sea un error: el usuario que se pasó del tope sigue teniendo
	// asistente, solo que con el modelo rápido.
	tenant, usuario := quienEs(r)
	modo := req.Modo
	degradado := false
	// `h.presupuesto != nil` NO ES DEFENSA DE ADORNO: sin caché configurada no
	// hay contador, y `Presupuestador` es una interfaz —llamar a un método sobre
	// una interfaz nil revienta, al contrario que sobre el puntero, que sí tenía
	// guarda de receptor nil—. Sin contador no hay tope, que es la degradación
	// abierta que se quiere.
	if modo == modoCalidad && h.presupuesto != nil {
		if permite, _ := h.presupuesto.Permite(r.Context(), tenant, usuario); !permite {
			modo, degradado = modoInteractivo, true
		}
	}

	prov, plazo := h.elegir(modo)

	// El deadline del proveedor es el límite principal de la inferencia.
	ctx, cancel := context.WithTimeout(r.Context(), plazo)
	defer cancel()

	user := provider.BuildUserMessage(provider.Request{
		Action:      req.Action,
		Instruction: req.Instruction,
		Context:     req.Context,
		Kind:        req.Kind,
	})

	opts := provider.Opciones{Esfuerzo: req.Esfuerzo}
	if req.Imagen != "" {
		datos, decErr := base64.StdEncoding.DecodeString(req.Imagen)
		if decErr != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The image is not valid base64."))
			return
		}
		tipo := req.ImagenTipo
		if tipo == "" {
			tipo = "image/png"
		}
		opts.Imagen = &provider.Imagen{Datos: datos, Tipo: tipo}
	}

	start := time.Now()
	res, err := prov.Complete(ctx, provider.SystemPrompt, user, opts)
	elapsed := time.Since(start)

	// SE COBRA AUNQUE LA PETICIÓN FALLE: los tokens ya se gastaron. Un truncado
	// consume el tope entero de salida, que es el caso más caro que hay, y no
	// cobrarlo dejaría un bucle de peticiones fallidas gastando gratis.
	gastado := h.cobrar(r.Context(), tenant, usuario, prov, res)

	// Log de auditoría SIN PII ni contenido del documento (RGPD): solo metadatos.
	logger := log.From(r.Context())
	if err != nil {
		// No se filtra la causa (podría venir del proveedor) más allá de logging server-side.
		logger.Warn("assist falló", "provider", prov.Name(), "modelo", prov.Modelo(),
			"modo", modoDe(modo), "degradado", degradado, "action", req.Action,
			"kind", req.Kind, "ms", elapsed.Milliseconds(), "error", err)
		httpx.Fail(w, r, mapProviderError(err))
		return
	}
	// Sin PII ni contenido del documento (RGPD): solo metadatos y lo que cuesta.
	logger.Info("assist", "provider", prov.Name(), "modelo", prov.Modelo(),
		"modo", modoDe(modo), "degradado", degradado, "action", req.Action,
		"kind", req.Kind, "ms", elapsed.Milliseconds(),
		"tokens_entrada", res.TokensEntrada, "tokens_salida", res.TokensSalida,
		"gastado_mes_eur", gastado)

	// La respuesta dice CON QUÉ se atendió y si hubo degradación. El cliente
	// puede seguir leyendo solo `text` —lo que hace hoy el panel del editor—,
	// pero con esto puede además avisar de que el presupuesto del mes se agotó,
	// que es la diferencia entre «hoy responde peor» y «algo va mal».
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"text":      res.Texto,
		"modelo":    prov.Modelo(),
		"degradado": degradado,
	})
}

// modoDe normaliza el modo para el log: el vacío es el interactivo, y en una
// métrica «» y «interactivo» no deben salir como dos cosas distintas.
func modoDe(m string) string {
	if m == "" {
		return modoInteractivo
	}
	return m
}

// mapProviderError traduce los errores del proveedor a errores de API. El BFF
// colapsa cualquier no-200 a "ai_failed"; los códigos aquí son para observabilidad.
func mapProviderError(err error) error {
	switch {
	case errors.Is(err, provider.ErrNotConfigured):
		return errNotConfigured
	case errors.Is(err, context.DeadlineExceeded):
		return httpx.NewError(http.StatusGatewayTimeout, "ai_timeout", "The assistant took too long to answer.")
	case errors.Is(err, provider.ErrRefused):
		return httpx.NewError(http.StatusUnprocessableEntity, "ai_refused", "El asistente no pudo atender esta solicitud.")
	case errors.Is(err, provider.ErrTruncated):
		// 502 como el resto para el cliente, pero con código propio: es lo que
		// distingue «hay que subir AI_MAX_TOKENS» de «el proveedor falla».
		return httpx.NewError(http.StatusBadGateway, "ai_truncated",
			"The assistant ran out of room to answer.").Wrap(err)
	default:
		// ErrEmpty, errores de red o estados no-2xx del proveedor.
		return httpx.NewError(http.StatusBadGateway, "ai_failed", "The assistant is unavailable right now.").Wrap(err)
	}
}

// transcribirTodo pasa los trozos por el modelo EN ORDEN y une el resultado.
//
// Uno detrás de otro y no en paralelo: la conversación tiene un orden y quien
// lee el acta lo necesita. En paralelo se tarda menos y se devuelve un texto
// desordenado, que no vale para nada.
//
// Si un trozo falla, falla la transcripción entera y se dice cuál. Devolver una
// hora de reunión con un hueco silencioso en medio, sin avisar, es peor que no
// devolver nada: nadie sabría que le falta un trozo.
func transcribirTodo(ctx context.Context, t provider.Transcriptor, audios [][]byte, nombre, idioma string) (string, error) {
	partes := make([]string, 0, len(audios))
	for i, a := range audios {
		texto, err := t.Transcribir(ctx, a, nombreDeTrozo(nombre, i, len(audios)), idioma)
		if err != nil {
			return "", fmt.Errorf("trozo %d de %d: %w", i+1, len(audios), err)
		}
		if s := strings.TrimSpace(texto); s != "" {
			partes = append(partes, s)
		}
	}
	return strings.Join(partes, "\n\n"), nil
}

// nombreDeTrozo mantiene la extensión, que es de donde el proveedor deduce el
// contenedor. Sin ella, un `.ogg` llamado `audio-2` se rechaza con un 400 que no
// dice que el problema sea el formato.
func nombreDeTrozo(nombre string, i, total int) string {
	if total <= 1 || nombre == "" {
		return nombre
	}
	ext := path.Ext(nombre)
	return fmt.Sprintf("%s-%d%s", strings.TrimSuffix(nombre, ext), i+1, ext)
}

func bytesDe(audios [][]byte) int {
	n := 0
	for _, a := range audios {
		n += len(a)
	}
	return n
}

// --- Transcripción de audio (whisper) ----------------------------------------

// transcribeRequest lleva el audio en base64. Es coherente con /assist, que ya
// recibe la imagen así: un solo formato de cuerpo para toda la API en vez de
// JSON aquí y multipart allá.
type transcribeRequest struct {
	Audio string `json:"audio"`
	// Nombre del fichero original. El proveedor lo usa para deducir el
	// contenedor (webm, mp3, wav): sin él acierta casi siempre, pero cuando
	// falla devuelve un 400 que no dice que el problema sea el formato.
	Nombre string `json:"nombre,omitempty"`
	// Idioma en ISO-639-1 ("es"). Vacío => lo detecta el modelo.
	Idioma string `json:"idioma,omitempty"`
	// Audios son los TROZOS de una grabación larga, en orden, cada uno en base64.
	//
	// POR QUÉ HACE FALTA: una reunión de una hora no cabe en una petición —el
	// fichero se lee entero en memoria y el modelo tiene su propio tope—, así que
	// quien graba la trocea y manda los pedazos. Aquí se transcriben en ORDEN y
	// se unen: hacerlo en paralelo iría más rápido y devolvería la conversación
	// desordenada, que no sirve para nada.
	//
	// Si viene, manda sobre `Audio`.
	Audios []string `json:"audios,omitempty"`
}

// transcribe convierte audio en texto con whisper-large-v3.
func (h *handlers) transcribe(w http.ResponseWriter, r *http.Request) {
	if h.transcriptor == nil {
		httpx.Fail(w, r, errNotConfigured)
		return
	}
	var req transcribeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	trozos := req.Audios
	if len(trozos) == 0 && strings.TrimSpace(req.Audio) != "" {
		trozos = []string{req.Audio}
	}
	if len(trozos) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Audio is required."))
		return
	}
	// Tope de trozos: una reunión de ocho horas en pedazos de cinco minutos son
	// noventa y seis. Cien deja margen y evita que una petición encadene mil
	// llamadas al modelo con el reloj del servicio corriendo.
	if len(trozos) > 100 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Too many audio chunks (100 maximum)."))
		return
	}
	if len(req.Idioma) > 8 || len(req.Nombre) > 256 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid request parameters."))
		return
	}
	audios := make([][]byte, 0, len(trozos))
	for i, t := range trozos {
		b, err := base64.StdEncoding.DecodeString(t)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				fmt.Sprintf("Audio chunk %d is not valid base64.", i+1)))
			return
		}
		if len(b) == 0 {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
				fmt.Sprintf("Audio chunk %d is empty.", i+1)))
			return
		}
		audios = append(audios, b)
	}

	// El plazo del modo calidad y no el interactivo: transcribir una reunión
	// larga tarda, y es una tarea de fondo por definición —nadie mira el cursor
	// mientras se genera el acta—.
	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.TimeoutCalidad)
	defer cancel()

	start := time.Now()
	texto, err := transcribirTodo(ctx, h.transcriptor, audios, req.Nombre, req.Idioma)
	elapsed := time.Since(start)

	logger := log.From(r.Context())
	if err != nil {
		logger.Warn("transcribe falló", "modelo", h.transcriptor.Modelo(),
			"bytes", bytesDe(audios), "trozos", len(audios), "ms", elapsed.Milliseconds(), "error", err)
		httpx.Fail(w, r, mapProviderError(err))
		return
	}
	// Sin PII: el tamaño y el tiempo, nunca lo que se dijo.
	logger.Info("transcribe", "modelo", h.transcriptor.Modelo(),
		"bytes", bytesDe(audios), "trozos", len(audios), "ms", elapsed.Milliseconds())
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"text":   texto,
		"modelo": h.transcriptor.Modelo(),
	})
}

const (
	headerTenant = "X-Sytue-Tenant"
	headerSub    = "X-Sytue-Sub"
)

type identidadAfirmadaKey struct{}

type IdentidadAfirmada struct {
	Tenant string
	Sub    string
}

func esCaracterValido(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z':
		return true
	case c >= 'A' && c <= 'Z':
		return true
	case c >= '0' && c <= '9':
		return true
	case c == '-' || c == '_' || c == '.' || c == '@' || c == ':':
		return true
	default:
		return false
	}
}

func validarIdentificador(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !esCaracterValido(s[i]) {
			return false
		}
	}
	return true
}

func verificarIdentidadAfirmada(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := strings.TrimSpace(r.Header.Get(headerTenant))
		sub := strings.TrimSpace(r.Header.Get(headerSub))
		if !validarIdentificador(tenant) || !validarIdentificador(sub) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("X-Sytue-Tenant and X-Sytue-Sub headers are required with valid format."))
			return
		}
		ctx := context.WithValue(r.Context(), identidadAfirmadaKey{}, IdentidadAfirmada{Tenant: tenant, Sub: sub})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func rateLimitKeyAfirmada(r *http.Request) string {
	if id, ok := r.Context().Value(identidadAfirmadaKey{}).(IdentidadAfirmada); ok {
		return id.Tenant + ":" + id.Sub
	}
	return r.RemoteAddr
}

type assistPagesRequest struct {
	Action      string          `json:"action"`
	Instruction string          `json:"instruction"`
	Context     string          `json:"context"`
	Kind        string          `json:"kind"`
	Modo        string          `json:"modo,omitempty"`
	Esfuerzo    string          `json:"esfuerzo,omitempty"`
	Sistema     string          `json:"sistema,omitempty"`
	Esquema     json.RawMessage `json:"esquema,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	BYOK        *provider.BYOK  `json:"byok,omitempty"`
}

func (req assistPagesRequest) validate(cfg config.Config) error {
	if err := req.validarInstruccionYContexto(cfg); err != nil {
		return err
	}
	if err := req.validarParametros(cfg); err != nil {
		return err
	}
	if err := req.validarEsquema(); err != nil {
		return err
	}
	return req.validarBYOK()
}

func (req assistPagesRequest) validarInstruccionYContexto(cfg config.Config) error {
	if strings.TrimSpace(req.Instruction) == "" {
		return httpx.ErrBadRequest.WithMessage("An instruction is required.")
	}
	maxInstr := cfg.MaxInstructionBytes
	if req.Sistema == "pages" {
		maxInstr = 32 * 1024 // ≤ 32 KiB para sistema=pages (§9.1)
	}
	if len(req.Instruction) > maxInstr {
		return httpx.ErrBadRequest.WithMessage("The instruction is too long.")
	}
	if len(req.Context) > cfg.MaxContextBytes {
		return httpx.ErrBadRequest.WithMessage("The document context is too large.")
	}
	return nil
}

func (req assistPagesRequest) validarParametros(cfg config.Config) error {
	if len(req.Action) > 64 || len(req.Kind) > 64 {
		return httpx.ErrBadRequest.WithMessage("Invalid request parameters.")
	}
	if req.Sistema != "" && req.Sistema != "pages" {
		return httpx.ErrBadRequest.WithMessage("Unknown system.")
	}
	if req.MaxTokens < 0 || req.MaxTokens > cfg.MaxTokensTope {
		return httpx.ErrBadRequest.WithMessage("Invalid max_tokens value.")
	}
	switch req.Esfuerzo {
	case "", "none", "low", "medium", "high":
		return nil
	default:
		return httpx.ErrBadRequest.WithMessage("Unknown reasoning effort.")
	}
}

func (req assistPagesRequest) validarEsquema() error {
	if len(req.Esquema) > 64*1024 {
		return httpx.ErrBadRequest.WithMessage("The schema is too large.")
	}
	if len(req.Esquema) > 0 {
		trimmed := bytes.TrimSpace(req.Esquema)
		if len(trimmed) == 0 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' || !json.Valid(trimmed) {
			return httpx.ErrBadRequest.WithMessage("The schema is not valid JSON.")
		}
	}
	return nil
}

func (req assistPagesRequest) validarBYOK() error {
	if req.BYOK == nil {
		return nil
	}
	p := strings.ToLower(strings.TrimSpace(req.BYOK.Proveedor))
	if _, ok := provider.BaseURLs[p]; !ok {
		return httpx.ErrBadRequest.WithMessage("Invalid BYOK provider.")
	}
	if strings.TrimSpace(req.BYOK.Clave) == "" {
		return httpx.ErrBadRequest.WithMessage("BYOK key is required.")
	}
	if strings.TrimSpace(req.BYOK.Modelo) == "" {
		return httpx.ErrBadRequest.WithMessage("BYOK model is required.")
	}
	return nil
}

func (h *handlers) assistPages(w http.ResponseWriter, r *http.Request) {
	id, _ := r.Context().Value(identidadAfirmadaKey{}).(IdentidadAfirmada)

	var req assistPagesRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := req.validate(h.cfg); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	var prov provider.Provider
	var err error
	esBYOK := req.BYOK != nil

	if esBYOK {
		prov, err = provider.NewFromBYOK(*req.BYOK, h.cfg.TimeoutPages, nil)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Failed to initialize BYOK provider."))
			return
		}
	} else {
		prov = h.provider
		if prov == nil {
			httpx.Fail(w, r, errNotConfigured)
			return
		}
	}

	systemPrompt := provider.SystemPrompt
	if req.Sistema == "pages" {
		systemPrompt = provider.SystemPromptPages
	}

	user := provider.BuildUserMessage(provider.Request{
		Action:      req.Action,
		Instruction: req.Instruction,
		Context:     req.Context,
		Kind:        req.Kind,
	})

	opts := provider.Opciones{
		Esfuerzo:  req.Esfuerzo,
		Esquema:   req.Esquema,
		MaxTokens: req.MaxTokens,
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.cfg.TimeoutPages)
	defer cancel()

	start := time.Now()
	res, err := prov.Complete(ctx, systemPrompt, user, opts)
	elapsed := time.Since(start)

	var gastado float64
	if !esBYOK {
		gastado = h.cobrar(r.Context(), id.Tenant, id.Sub, prov, res)
	}

	logger := log.From(r.Context())
	if err != nil {
		logger.Warn("assist pages falló",
			"tenant", id.Tenant, "sub", id.Sub, "byok", esBYOK,
			"provider", prov.Name(), "modelo", prov.Modelo(),
			"ms", elapsed.Milliseconds(), "error", err)
		httpx.Fail(w, r, mapProviderError(err))
		return
	}

	logger.Info("assist pages",
		"tenant", id.Tenant, "sub", id.Sub, "byok", esBYOK,
		"provider", prov.Name(), "modelo", prov.Modelo(),
		"ms", elapsed.Milliseconds(),
		"tokens_entrada", res.TokensEntrada, "tokens_salida", res.TokensSalida,
		"gastado_mes_eur", gastado)

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"text":           res.Texto,
		"modelo":         prov.Modelo(),
		"degradado":      false,
		"tokens_entrada": res.TokensEntrada,
		"tokens_salida":  res.TokensSalida,
	})
}

type modelsPagesRequest struct {
	BYOK struct {
		Proveedor string `json:"proveedor"`
		Clave     string `json:"clave"`
	} `json:"byok"`
}

func (h *handlers) modelsPages(w http.ResponseWriter, r *http.Request) {
	id, _ := r.Context().Value(identidadAfirmadaKey{}).(IdentidadAfirmada)

	var req modelsPagesRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	prov := strings.ToLower(strings.TrimSpace(req.BYOK.Proveedor))
	if _, ok := provider.BaseURLs[prov]; !ok {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid BYOK provider."))
		return
	}
	if strings.TrimSpace(req.BYOK.Clave) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("BYOK key is required."))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	modelos, err := provider.ListModels(ctx, nil, prov, req.BYOK.Clave)
	logger := log.From(r.Context())
	if err != nil {
		logger.Warn("models pages falló", "tenant", id.Tenant, "sub", id.Sub,
			"proveedor", prov, "error", err)
		httpx.Fail(w, r, httpx.NewError(http.StatusBadGateway, "models_failed", "Failed to retrieve models from provider.").Wrap(err))
		return
	}

	logger.Info("models pages", "tenant", id.Tenant, "sub", id.Sub, "proveedor", prov, "count", len(modelos))
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"modelos": modelos,
	})
}
