// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type captureExporter struct {
	spans   []SpanData
	metrics []MetricData
}

func (c *captureExporter) ExportSpans(_ context.Context, s []SpanData) error {
	c.spans = append(c.spans, s...)
	return nil
}

func (c *captureExporter) ExportMetrics(_ context.Context, m []MetricData) error {
	c.metrics = append(c.metrics, m...)
	return nil
}
func (c *captureExporter) Shutdown(context.Context) error { return nil }

func TestSpanExportAndParenting(t *testing.T) {
	exp := &captureExporter{}
	p, _ := NewProvider(context.Background(), Config{Service: "svc", Exporter: exp})
	tr := p.Tracer("t")

	ctx, parent := tr.Start(context.Background(), "parent")
	_, child := tr.Start(ctx, "child")
	child.End()
	parent.End()

	if len(exp.spans) != 2 {
		t.Fatalf("esperaba 2 spans, got %d", len(exp.spans))
	}
	// child (primero en cerrar) comparte traceID y apunta al parent.
	c := exp.spans[0]
	if c.TraceID != parent.SpanContext().TraceID {
		t.Error("child no hereda traceID del parent")
	}
	if c.ParentID != parent.SpanContext().SpanID {
		t.Error("child no referencia al parent")
	}
}

func TestTraceParentRoundTrip(t *testing.T) {
	p, _ := NewProvider(context.Background(), Config{Service: "svc"})
	_, span := p.Tracer("t").Start(context.Background(), "op")
	ctx := ContextWithSpanContext(context.Background(), span.SpanContext())

	h := http.Header{}
	InjectTraceParent(ctx, h)
	tp := h.Get(HeaderTraceParent)
	if tp == "" {
		t.Fatal("traceparent no inyectado")
	}
	extracted := ExtractTraceParent(h)
	if !extracted.IsValid() || extracted.TraceID != span.SpanContext().TraceID {
		t.Fatalf("extracción incorrecta: %+v", extracted)
	}
}

func TestExtractTraceParentInvalid(t *testing.T) {
	h := http.Header{}
	h.Set(HeaderTraceParent, "garbage")
	if ExtractTraceParent(h).IsValid() {
		t.Fatal("traceparent inválido no debe validar")
	}
}

func TestMetricsSnapshot(t *testing.T) {
	exp := &captureExporter{}
	p, _ := NewProvider(context.Background(), Config{Service: "svc", Exporter: exp})
	m := p.Meter("t")
	c := m.Counter("reqs")
	c.Add(context.Background(), 3, String("route", "/x"))
	c.Add(context.Background(), 2, String("route", "/x"))
	h := m.Histogram("latency")
	h.Record(context.Background(), 0.15)

	data := p.Collect()
	var sawCounter, sawHist bool
	for _, d := range data {
		if d.Kind == "counter" && d.Name == "reqs" {
			sawCounter = true
			if d.Sum != 5 {
				t.Errorf("suma contador = %v, want 5", d.Sum)
			}
		}
		if d.Kind == "histogram" && d.Name == "latency" {
			sawHist = true
			if d.Count != 1 {
				t.Errorf("count histograma = %d", d.Count)
			}
		}
	}
	if !sawCounter || !sawHist {
		t.Fatal("faltan métricas en snapshot")
	}

	if err := p.FlushMetrics(context.Background()); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if len(exp.metrics) == 0 {
		t.Fatal("exporter no recibió métricas")
	}
}

func TestPprofRequiresGuard(t *testing.T) {
	mux := http.NewServeMux()
	if err := RegisterPprof(mux, PprofConfig{}); err == nil {
		t.Fatal("sin Guard debe fallar (deny-by-default)")
	}
}

func TestPprofGuardBlocks(t *testing.T) {
	mux := http.NewServeMux()
	guard := StaticTokenGuard("X-Debug-Token", "s3cr3t")
	if err := RegisterPprof(mux, PprofConfig{Guard: guard}); err != nil {
		t.Fatalf("registro: %v", err)
	}

	// Sin token: 403.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", http.NoBody))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sin token debe ser 403, got %d", rec.Code)
	}
	// Con token: pasa al handler de pprof (200).
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", http.NoBody)
	req.Header.Set("X-Debug-Token", "s3cr3t")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("con token válido debe pasar, got %d", rec.Code)
	}
}

// TestElSpanNoLlevaLaURL: el nombre y `http.route` son el patrón de la ruta. La URL
// lleva secretos (el token de un enlace público) que no pueden ir a las trazas.
func TestElSpanNoLlevaLaURL(t *testing.T) {
	exp := &captureExporter{}
	p, _ := NewProvider(context.Background(), Config{Service: "svc", Exporter: exp})
	r := chi.NewRouter()
	r.Use(p.Middleware)
	r.Get("/public/{token}", func(http.ResponseWriter, *http.Request) {})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/public/secreto-del-enlace", http.NoBody))
	if len(exp.spans) != 1 || exp.spans[0].Name != "GET /public/{token}" {
		t.Fatalf("spans = %+v", exp.spans)
	}
	for _, a := range exp.spans[0].Attributes {
		if strings.Contains(fmt.Sprint(a), "secreto") {
			t.Errorf("atributo con la URL: %+v", a)
		}
	}
}
