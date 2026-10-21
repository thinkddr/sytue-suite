// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package httpx aporta el andamiaje HTTP común a todos los servicios Go de
// Sytue One: router chi, middlewares (request id, logging estructurado,
// recovery), respuestas/errores JSON canónicos y sondas /healthz y /readyz.
//
// # Errores de API traducibles (CONVENTIONS §13.1)
//
// El producto habla cinco idiomas y eso incluye los mensajes de error. La
// traducción se hace EN EL CLIENTE, no aquí: el servidor manda un código
// estable y los datos, y quien pinta la frase elige el idioma.
//
//	{"error":{"code":"unknown_instance_size",
//	          "message":"Unknown instance size \"xl\".",
//	          "params":{"size":"xl"}},
//	 "request_id":"…"}
//
// Por qué en el cliente y no en el servidor: el catálogo de los cinco idiomas
// ya existe en el frontend para la interfaz; duplicarlo en Go daría dos fuentes
// de verdad y obligaría a desplegar un servicio para corregir una tilde en
// italiano. Además el servidor no siempre sabe el idioma mejor que el cliente
// (usuario con Accept-Language: en navegando la interfaz en español).
//
// Las tres piezas del contrato:
//
//   - Code: la clave de traducción. Es contrato de API, como una ruta: se
//     añaden códigos, no se renombran. Un código por motivo distinguible.
//   - Params: los datos que interpola la frase, estructurados y crudos (sin
//     preformatear), para que el cliente componga y formatee en su idioma.
//   - Message: el respaldo en INGLÉS base, con marcadores "{param}" que se
//     sustituyen al serializar. Existe para curl, CLIs viejos y logs; un
//     cliente con catálogo lo ignora.
//
// Para el texto que GENERA el servidor (correos, sugeridor de nombres) sí hace
// falta saber el idioma: httpx.LanguageFromRequest(r) negocia Accept-Language
// con el inglés como base.
//
// # Cómo migra un servicio (mecánico, error a error)
//
//  1. Localiza el error con texto español, típicamente
//     fmt.Errorf("%w: tamaño de instancia desconocido %q", ErrValidation, size)
//     en internal/domain, y su traducción a HTTP en toAPIError.
//  2. Dale un código estable y un mensaje inglés con marcadores, como var de
//     paquete:
//     var ErrUnknownInstanceSize = httpx.ErrBadRequest.
//     WithCode("unknown_instance_size").
//     WithMessage(`Unknown instance size "{size}".`)
//  3. En el punto de fallo, adjunta los datos en vez de interpolarlos en el
//     texto: ErrUnknownInstanceSize.WithParams(map[string]any{"size": size}).
//     Si el dominio no debe importar httpx, deja el error centinela del dominio
//     y haz el WithParams en toAPIError, que ya conoce el valor.
//  4. Añade code + frase inglesa al catálogo del cliente y sus cuatro
//     traducciones, en el mismo commit (§13.1).
//
// Los mensajes españoles que queden vivos no rompen nada: siguen saliendo por
// "message" hasta que su ola los migre.
package httpx

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
)

// RouterOptions configura el router base.
type RouterOptions struct {
	// Logger es el logger base del servicio.
	Logger *slog.Logger
	// RequestTimeout aplica un plazo a cada petición (0 = sin plazo).
	RequestTimeout time.Duration
	// RequestTimeoutFor permite ampliar el plazo de una operación concreta.
	// Un valor no positivo conserva RequestTimeout; no desactiva su protección.
	RequestTimeoutFor func(*http.Request) time.Duration
	// MaxBodyBytes limita el tamaño del cuerpo de cada petición (0 = 1 MiB).
	MaxBodyBytes int64
	// Security configura las cabeceras de seguridad. Si es nil, se usa
	// DefaultSecurityHeaders() (endurecido por defecto, deny-by-default).
	Security *SecurityHeadersConfig
	// CORS habilita CORS restrictivo si se define (allowlist explícita).
	CORS *CORSConfig
	// RateLimit habilita el rate-limiting global si se define.
	RateLimit *RateLimitConfig
	// CSRF habilita la protección CSRF double-submit si se define.
	CSRF *CSRFConfig
	// Meter instrumenta peticiones y latencia. Si es nil se recurre al Provider
	// por defecto del servicio ([telemetry.SetDefault]), y si tampoco lo hay no
	// se mide nada: la telemetría no puede ser un requisito para servir.
	//
	// Conviene fijarlo a mano en los tests, para no medir contra el global.
	Meter *telemetry.Meter
}

// NewRouter construye un *chi.Mux con la cadena de middlewares SEGURA por
// defecto de Sytue One, en orden (deny-by-default, defensa en profundidad):
//
//	request id -> logging -> [métricas] -> recovery -> security headers ->
//	[CORS] -> body-limit -> [rate-limit] -> [CSRF] -> [timeout]
//
// Las cabeceras de seguridad se aplican SIEMPRE (endurecidas si no se configuran);
// CORS/rate-limit/CSRF son opcionales pero recomendados en superficies con cookies.
//
// NewRouter monta además sondas /healthz y /readyz por defecto (liveness 200 fijo;
// readiness sin dependencias => 200). Esto cumple dos objetivos de §0.7 (defensa
// en profundidad): (1) provee las probes que esperan los charts de Helm aunque el
// servicio no llame a MountHealth, y (2) GARANTIZA que chi construye su cadena de
// handler (chi v5 salta TODOS los middlewares en un mux SIN rutas: un 404 no
// pasaría por RequestID/SecurityHeaders/Recoverer). Al haber siempre >=1 ruta, los
// 404 atraviesan la cadena completa (request-id + envelope de error canónico).
//
// MountHealth es idempotente frente a esto: chi sobreescribe el handler de una
// ruta ya registrada, así que un servicio que llame MountHealth (con versión y
// checkers reales) simplemente reemplaza estas sondas por defecto, sin panic ni
// duplicados.
func NewRouter(opts RouterOptions) *chi.Mux {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	r := chi.NewRouter()
	r.Use(RequestID)
	r.Use(RequestLogger(opts.Logger))
	// Las métricas van POR FUERA de todo lo que rechaza peticiones, y es
	// deliberado: así cuentan los 429 del rate-limit y los 500 que fabrica
	// Recoverer al capturar un panic. Medir por dentro dejaría fuera de la
	// estadística justo el tráfico que dice que el servicio está en apuros, que
	// es el que tiene que disparar el escalado.
	medidor := opts.Meter
	if medidor == nil {
		medidor = telemetry.DefaultMeter("http")
	}
	r.Use(Metrics(medidor))
	r.Use(Recoverer)

	sec := DefaultSecurityHeaders()
	if opts.Security != nil {
		sec = *opts.Security
	}
	r.Use(SecurityHeaders(sec))

	if opts.CORS != nil {
		r.Use(CORS(*opts.CORS))
	}

	r.Use(MaxBodyBytes(opts.MaxBodyBytes))

	if opts.RateLimit != nil {
		r.Use(RateLimit(*opts.RateLimit))
	}
	if opts.CSRF != nil {
		r.Use(CSRF(*opts.CSRF))
	}
	if opts.RequestTimeout > 0 || opts.RequestTimeoutFor != nil {
		r.Use(requestTimeout(opts.RequestTimeout, opts.RequestTimeoutFor))
	}
	r.NotFound(func(w http.ResponseWriter, req *http.Request) { Fail(w, req, ErrNotFound) })
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		Fail(w, req, NewError(http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed."))
	})

	// Sondas por defecto: liveness siempre 200; readiness sin dependencias 200.
	// Registrar al menos una ruta obliga a chi a construir la cadena de handler,
	// de modo que incluso los 404 pasen por todos los middlewares (ver doc de
	// NewRouter). MountHealth sobreescribe estas sondas si el servicio aporta
	// versión y checkers reales (chi reemplaza el handler de una ruta existente).
	MountHealth(r, "", 0)
	return r
}

func requestTimeout(fallback time.Duration, resolve func(*http.Request) time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeout := fallback
			if resolve != nil {
				if specific := resolve(r); specific > 0 {
					timeout = specific
				}
			}
			if timeout > 0 {
				middleware.Timeout(timeout)(next).ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// EnableWebDAVMethods registra en chi los verbos WebDAV (CalDAV/CardDAV) para que
// r.Handle/r.Mount puedan enrutarlos: chi solo conoce los métodos HTTP estándar, y
// sin esto un PROPFIND/REPORT cae en MethodNotAllowed (405). RegisterMethod añade
// el verbo y lo incluye en mALL; es idempotente. DEBE llamarse ANTES de montar las
// rutas DAV (típicamente desde un init() del paquete del servidor DAV).
func EnableWebDAVMethods() {
	for _, m := range []string{"PROPFIND", "PROPPATCH", "MKCOL", "MKCALENDAR", "REPORT", "COPY", "MOVE", "LOCK", "UNLOCK"} {
		chi.RegisterMethod(m)
	}
}

// Checker es una sonda de readiness para una dependencia (DB, S3, ...).
// Debe devolver nil si la dependencia está sana.
type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// CheckerFunc adapta una función a Checker.
type CheckerFunc struct {
	Label string
	Fn    func(ctx context.Context) error
}

func (c CheckerFunc) Name() string                    { return c.Label }
func (c CheckerFunc) Check(ctx context.Context) error { return c.Fn(ctx) }

// buildCommit es el commit exacto con el que se construyó ESTA imagen.
//
// Se lee del entorno una sola vez, y con un nombre SIN prefijo de servicio a
// propósito: es un dato del build, igual para los once binarios, y darle un
// nombre por servicio obligaría a acordarse de once sitios. El Dockerfile de
// cada uno lo hornea como ENV desde su `--build-arg COMMIT`.
//
// No va en ldflags porque eso metería una entrada distinta en cada build y
// rompería la reproducibilidad byte a byte del binario, que es la razón por la
// que la versión ya se inyectaba por entorno.
var buildCommit = os.Getenv("SYTUE_BUILD_COMMIT")

// HealthHandler responde la sonda de liveness: el proceso está vivo.
// No comprueba dependencias (no debe reiniciar el pod por una DB caída).
//
// Devuelve también QUÉ está vivo: la versión y el commit. Es lo que consulta la
// consola de plataforma para poder decir qué hay desplegado de verdad, y lo
// primero que se pregunta cuando algo se comporta como no debería.
func HealthHandler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out := map[string]any{"status": "ok"}
		// La versión y el commit SOLO hacia dentro. Medido contra producción el
		// 2026-08-13: `curl https://drive.sytue.eu/healthz` devolvía sin
		// autenticación de ningún tipo `{"status":"ok","version":"v2.2.1",
		// "commit":"ee717df9…"}`. Decirle a cualquiera qué versión exacta corre es
		// regalarle la lista de fallos conocidos que puede probar, y el commit
		// además señala el código concreto. `ingressroutes.Internal` ya declaraba
		// /healthz como ruta que no debe publicarse; el Ingress catch-all de cada
		// superficie la publicaba igual, así que la defensa se pone aquí.
		//
		// Quien pregunta de dentro sí lo ve, y es quien lo necesita: el kubelet
		// llama a la IP del Pod y Sytue ID pregunta por la URL interna del
		// componente (platform_versions.go), sin pasar por Traefik. Traefik es
		// quien añade X-Forwarded-For, así que su presencia distingue «viene de
		// internet» de «viene de dentro» sin mantener ninguna lista de redes.
		if r.Header.Get("X-Forwarded-For") == "" {
			out["version"] = version
			if buildCommit != "" {
				out["commit"] = buildCommit
			}
		}
		JSON(w, r, http.StatusOK, out)
	}
}

// ReadyHandler responde la sonda de readiness: comprueba dependencias en
// paralelo. 200 si todas sanas, 503 con detalle por dependencia si alguna falla.
func ReadyHandler(checkTimeout time.Duration, checkers ...Checker) http.HandlerFunc {
	if checkTimeout <= 0 {
		checkTimeout = 3 * time.Second
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), checkTimeout)
		defer cancel()

		type result struct {
			name string
			err  error
		}
		results := make(chan result, len(checkers))
		for _, c := range checkers {
			go func(c Checker) {
				results <- result{name: c.Name(), err: c.Check(ctx)}
			}(c)
		}

		deps := make(map[string]string, len(checkers))
		ready := true
		for range checkers {
			res := <-results
			if res.err != nil {
				ready = false
				deps[res.name] = "error: " + res.err.Error()
			} else {
				deps[res.name] = "ok"
			}
		}

		status := http.StatusOK
		state := "ready"
		if !ready {
			status = http.StatusServiceUnavailable
			state = "not_ready"
		}
		JSON(w, r, status, map[string]any{"status": state, "dependencies": deps})
	}
}

// MountHealth registra /healthz y /readyz en el router.
func MountHealth(r chi.Router, version string, checkTimeout time.Duration, checkers ...Checker) {
	r.Get("/healthz", HealthHandler(version))
	r.Get("/readyz", ReadyHandler(checkTimeout, checkers...))
}
