// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
)

// Nombres de las series. Se siguen las convenciones de OpenTelemetry
// (`http.server.*`) porque el exportador habla OTLP: cambiar el nombre aquí
// significa cambiarlo en cualquier panel o regla de escalado que ya lo use.
const (
	metricRequests = "http.server.requests"
	metricDuration = "http.server.duration"
)

// rutaDesconocida etiqueta lo que no casó con ninguna ruta registrada. Sin esto
// habría que elegir entre no medir los 404 —que son justo los que interesan
// cuando alguien escanea— o meter la URL cruda como atributo, que es la forma
// clásica de reventar la cardinalidad: un atacante pidiendo /aaa1, /aaa2… crea
// una serie por petición hasta tumbar al recolector.
const rutaDesconocida = "desconocida"

// Metrics cuenta las peticiones HTTP y mide su latencia con el Meter de la casa.
//
// POR QUÉ ESTO NO TRAE prometheus/client_golang. La maquinaria ya está escrita
// —`telemetry.Meter` con Counter e Histogram, atributos y exportador OTLP— y
// llevaba sin instrumentar nada: cero llamadas a Counter/Histogram en toda la
// suite. Añadir una dependencia para lo que ya hay hecho sería ensanchar la
// superficie de suministro a cambio de nada.
//
// LA ETIQUETA DE RUTA ES EL PATRÓN, NUNCA LA URL. `chi.RouteContext(r).
// RoutePattern()` devuelve `/api/v1/orgs/{id}` y no `/api/v1/orgs/9f3b…`. Es la
// diferencia entre una serie por endpoint y una serie por identificador, o sea
// entre unas decenas de series y tantas como filas tenga la base.
//
// El patrón SOLO existe después de que chi resuelva, así que se lee al volver de
// `next.ServeHTTP` y no antes.
func Metrics(m *telemetry.Meter) func(http.Handler) http.Handler {
	if m == nil {
		// Sin Meter, el middleware se quita de en medio. Un servicio sin
		// telemetría configurada debe seguir sirviendo, no arrancar a medias.
		return func(next http.Handler) http.Handler { return next }
	}
	peticiones := m.Counter(metricRequests)
	duracion := m.Histogram(metricDuration)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			inicio := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			// En segundos: es la unidad de DefaultBoundaries y la que espera
			// OTLP. Medirlo en milisegundos metería todo en el último cubo.
			transcurrido := time.Since(inicio).Seconds()
			// Capacidad 3: método y ruta ahora, y el código de estado que el
			// contador añade justo debajo. Reservarla evita que el `append` de
			// la tercera copie el respaldo en CADA petición servida.
			attrs := make([]telemetry.Attr, 0, 3)
			attrs = append(attrs,
				telemetry.String("http.request.method", r.Method),
				telemetry.String("http.route", rutaDe(r)),
			)
			ctx := r.Context()
			// El código de estado va SOLO en el contador. En el histograma
			// multiplicaría las series por cada código visto sin responder a
			// ninguna pregunta que no responda ya el contador.
			peticiones.Add(ctx, 1, append(attrs,
				telemetry.String("http.response.status_code", strconv.Itoa(rec.status)))...)
			duracion.Record(ctx, transcurrido, attrs...)
		})
	}
}

// rutaDe devuelve el patrón de la ruta que atendió la petición.
func rutaDe(r *http.Request) string {
	if rc := chi.RouteContext(r.Context()); rc != nil {
		if p := rc.RoutePattern(); p != "" {
			return p
		}
	}
	return rutaDesconocida
}
