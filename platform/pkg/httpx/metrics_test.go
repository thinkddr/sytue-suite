// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package httpx

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/telemetry"
)

// capturador retiene lo que el Provider exporta al cerrarse. Es la única vía de
// leer las series desde fuera del paquete telemetry, y es la buena: comprueba lo
// que de verdad sale hacia el colector, no un estado interno.
type capturador struct{ metricas []telemetry.MetricData }

func (c *capturador) ExportSpans(context.Context, []telemetry.SpanData) error { return nil }

func (c *capturador) ExportMetrics(_ context.Context, m []telemetry.MetricData) error {
	c.metricas = append(c.metricas, m...)
	return nil
}

func (c *capturador) Shutdown(context.Context) error { return nil }

// medido monta un router con el middleware y devuelve una función que cierra el
// Provider y entrega las series de la métrica pedida, con sus atributos.
func medido(t *testing.T, rutas func(chi.Router)) (*chi.Mux, func(nombre string) []map[string]string) {
	t.Helper()
	capt := &capturador{}
	tp, err := telemetry.NewProvider(t.Context(), telemetry.Config{
		Service: "prueba", Version: "v0", Exporter: capt,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	r := chi.NewRouter()
	r.Use(Metrics(tp.Meter("httpx")))
	rutas(r)

	cerrado := false
	return r, func(nombre string) []map[string]string {
		if !cerrado {
			if err := tp.Shutdown(context.Background()); err != nil {
				t.Fatalf("shutdown: %v", err)
			}
			cerrado = true
		}
		var out []map[string]string
		for _, m := range capt.metricas {
			if m.Name != nombre {
				continue
			}
			attrs := map[string]string{}
			for _, a := range m.Attributes {
				attrs[a.Key] = fmt.Sprint(a.Value)
			}
			out = append(out, attrs)
		}
		return out
	}
}

// LA PRUEBA QUE JUSTIFICA EL MIDDLEWARE: mil identificadores distintos producen
// UNA sola serie, no mil.
//
// Es el fallo que hunde a los recolectores y que no da la cara en desarrollo,
// donde nadie pide mil URLs. Con la ruta cruda como atributo, cada identificador
// crea su propia serie y la memoria crece con las filas de la base; con el patrón
// de chi, `/orgs/{id}` es siempre la misma etiqueta.
func TestLaRutaEtiquetaElPatronYNoElIdentificador(t *testing.T) {
	r, series := medido(t, func(r chi.Router) {
		r.Get("/orgs/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	})

	for i := 0; i < 1000; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/orgs/org-"+strconv.Itoa(i), http.NoBody))
		if rec.Code != http.StatusOK {
			t.Fatalf("petición %d: código %d", i, rec.Code)
		}
	}

	s := series(metricRequests)
	if len(s) != 1 {
		// Se enseñan TRES y no todas: cuando esto falla de verdad hay mil series,
		// y volcarlas son ochenta kilobytes de salida que esconden el mensaje. Con
		// tres se ve la forma del problema, que es lo que hace falta.
		t.Fatalf("mil identificadores produjeron %d series, quiero 1; muestra: %v",
			len(s), s[:min(3, len(s))])
	}
	if got := s[0]["http.route"]; got != "/orgs/{id}" {
		t.Errorf("http.route = %q, quiero el patrón «/orgs/{id}»", got)
	}
}

// Una ruta que no existe se cuenta, pero SIN meter la URL en la etiqueta.
//
// Las dos mitades importan. Si no se contara, un escaneo no aparecería en
// ninguna métrica; si se etiquetara con la URL, ese mismo escaneo crearía una
// serie por intento, que es peor que no medir.
func TestUn404SeCuentaSinFiltrarLaURL(t *testing.T) {
	r, series := medido(t, func(r chi.Router) {
		r.Get("/existe", func(w http.ResponseWriter, _ *http.Request) {})
	})

	for _, url := range []string{"/no-existe-1", "/no-existe-2", "/no-existe-3"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, http.NoBody))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: código %d, quiero 404", url, rec.Code)
		}
	}

	s := series(metricRequests)
	if len(s) != 1 {
		t.Errorf("tres URLs inexistentes produjeron %d series, quiero 1: %v", len(s), s)
	}
	for _, serie := range s {
		if ruta := serie["http.route"]; ruta != rutaDesconocida {
			t.Errorf("una serie etiquetó la ruta como %q: eso es una URL cruda", ruta)
		}
	}
}

// El código de estado va en el contador y NO en el histograma: si estuviera en
// los dos, cada código nuevo duplicaría las series de latencia sin responder
// nada que el contador no responda ya.
func TestElCodigoDeEstadoNoEntraEnElHistograma(t *testing.T) {
	r, series := medido(t, func(r chi.Router) {
		r.Get("/x", func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Query().Get("falla") != "" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		})
	})

	for _, u := range []string{"/x", "/x?falla=1"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, u, http.NoBody))
	}

	if n := len(series(metricRequests)); n != 2 {
		t.Errorf("el contador tiene %d series, quiero 2 (una por código)", n)
	}
	if n := len(series(metricDuration)); n != 1 {
		t.Errorf("el histograma tiene %d series, quiero 1 (sin código de estado)", n)
	}
}

// Sin Meter el middleware desaparece: un servicio sin telemetría configurada
// tiene que seguir sirviendo, no arrancar a medias ni panicar en la primera
// petición.
func TestSinMeterElMiddlewareNoEstorba(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Metrics(nil))
	r.Get("/x", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", http.NoBody))
	if rec.Code != http.StatusTeapot {
		t.Errorf("código %d, quiero 418: el middleware nulo alteró la respuesta", rec.Code)
	}
}
