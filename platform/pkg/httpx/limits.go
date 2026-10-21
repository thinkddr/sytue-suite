// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// DefaultMaxBodyBytes es el límite por defecto del cuerpo de petición (1 MiB).
// Coincide con el límite de DecodeJSON; para subidas de ficheros los servicios
// (p. ej. Drive) montan un límite mayor en sus rutas de upload.
const DefaultMaxBodyBytes int64 = 1 << 20

// MaxBodyBytes limita el tamaño del cuerpo de cada petición mediante
// http.MaxBytesReader. Es una segunda barrera además de la de DecodeJSON, que
// protege también a handlers que leen el cuerpo directamente.
func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSConfig configura una política CORS restrictiva (allowlist explícita).
// deny-by-default: sin orígenes permitidos no se emite ninguna cabecera CORS.
type CORSConfig struct {
	// AllowedOrigins es la allowlist exacta de orígenes (esquema+host+puerto).
	// No se admite "*" junto a credenciales: si se necesita "*", desactivar
	// AllowCredentials. El valor "*" solo se recomienda para APIs públicas sin cookies.
	AllowedOrigins []string
	// AllowedMethods (por defecto GET, POST, PUT, PATCH, DELETE, OPTIONS).
	AllowedMethods []string
	// AllowedHeaders (por defecto Authorization, Content-Type, X-Request-Id, X-CSRF-Token).
	AllowedHeaders []string
	// ExposedHeaders se exponen al cliente (por defecto X-Request-Id).
	ExposedHeaders []string
	// AllowCredentials permite cookies/credenciales cross-origin.
	AllowCredentials bool
	// MaxAgeSeconds cachea la respuesta preflight (por defecto 600).
	MaxAgeSeconds int
}

func (c CORSConfig) withDefaults() CORSConfig {
	if len(c.AllowedMethods) == 0 {
		c.AllowedMethods = []string{
			http.MethodGet, http.MethodPost, http.MethodPut,
			http.MethodPatch, http.MethodDelete, http.MethodOptions,
			// QUERY lleva cuerpo, así que NO es una petición simple: el navegador
			// hace preflight y, si no está en esta lista, ni llega a lanzarla. Va
			// aquí y no en cada servicio porque olvidarlo no da un error claro,
			// da un fallo de CORS en el navegador de quien lo use.
			MethodQuery,
		}
	}
	if len(c.AllowedHeaders) == 0 {
		c.AllowedHeaders = []string{"Authorization", "Content-Type", HeaderRequestID, HeaderCSRFToken}
	}
	if len(c.ExposedHeaders) == 0 {
		c.ExposedHeaders = []string{HeaderRequestID}
	}
	if c.MaxAgeSeconds == 0 {
		c.MaxAgeSeconds = 600
	}
	return c
}

// CORS es un middleware CORS restrictivo con allowlist explícita de orígenes.
// Responde 204 a las preflight (OPTIONS con Access-Control-Request-Method).
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	cfg = cfg.withDefaults()
	methods := strings.Join(cfg.AllowedMethods, ", ")
	reqHeaders := strings.Join(cfg.AllowedHeaders, ", ")
	expose := strings.Join(cfg.ExposedHeaders, ", ")
	maxAge := strconv.Itoa(cfg.MaxAgeSeconds)
	allowAll := false
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				// Petición same-origin o no navegador: sin cabeceras CORS.
				next.ServeHTTP(w, r)
				return
			}

			allowed := allowAll || originAllowed(origin, cfg.AllowedOrigins)
			if allowed {
				h := w.Header()
				// Con "*" y sin credenciales, se refleja "*"; en cualquier otro
				// caso se refleja el origen concreto y se marca Vary.
				if allowAll && !cfg.AllowCredentials {
					h.Set("Access-Control-Allow-Origin", "*")
				} else {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Add("Vary", "Origin")
				}
				if cfg.AllowCredentials && !allowAll {
					h.Set("Access-Control-Allow-Credentials", "true")
				}
				h.Set("Access-Control-Expose-Headers", expose)
			}

			// Preflight.
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				if allowed {
					h := w.Header()
					h.Set("Access-Control-Allow-Methods", methods)
					h.Set("Access-Control-Allow-Headers", reqHeaders)
					h.Set("Access-Control-Max-Age", maxAge)
					h.Add("Vary", "Access-Control-Request-Method")
					h.Add("Vary", "Access-Control-Request-Headers")
				}
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowed(origin string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(a, origin) {
			return true
		}
		// Comodín de subdominio: "https://*.sytue.eu" cubre UNA etiqueta de ese
		// apex con el mismo esquema —`corporate.sytue.eu` sí, `x.apps.sytue.eu`
		// no—. No cubre el apex desnudo (añádelo aparte).
		//
		// UNA SOLA ETIQUETA, Y ES LA DIFERENCIA ENTRE PODER USAR ESTO O NO. Antes
		// casaba con cualquier profundidad, así que `https://*.sytue.eu` habría
		// dado CORS con credenciales a `loquesea.apps.sytue.eu`: las aplicaciones
		// que los clientes alojan en Sytue Cloud, o sea código de terceros
		// llamando al Hub con la sesión de quien lo visite. Por eso el comodín se
		// evitaba y las allowlists se escribían a mano, y por eso al dominio del
		// tenant —donde de verdad corre la suite tras el login— no llegaba CORS y
		// las notificaciones, la presencia y el conmutador fallaban en silencio.
		//
		// Acotado a una etiqueta, el comodín expresa lo que se quería decir: los
		// subdominios DIRECTOS del apex, que son nuestros.
		if i := strings.Index(a, "://*."); i != -1 {
			scheme, suffix := a[:i+3], a[i+5:] // "https://", "sytue.eu" (sin "*.")
			lo := strings.ToLower(origin)
			if strings.HasPrefix(lo, strings.ToLower(scheme)) {
				host := lo[len(scheme):]
				etiqueta, resto, hayPunto := strings.Cut(host, ".")
				if hayPunto && etiqueta != "" && strings.EqualFold(resto, suffix) {
					return true
				}
			}
		}
	}
	return false
}
