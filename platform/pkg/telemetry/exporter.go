// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// NopExporter descarta las señales (por defecto). Útil en tests o cuando la
// exportación se hace por otra vía (scrape de /metrics).
type NopExporter struct{}

func (NopExporter) ExportSpans(context.Context, []SpanData) error     { return nil }
func (NopExporter) ExportMetrics(context.Context, []MetricData) error { return nil }
func (NopExporter) Shutdown(context.Context) error                    { return nil }

// OTLPHTTPConfig configura el exportador OTLP/HTTP (protocolo JSON).
type OTLPHTTPConfig struct {
	// Endpoint base del colector OTLP (p. ej. http://otel-collector:4318).
	Endpoint string
	// Headers añade cabeceras (p. ej. Authorization al colector).
	Headers map[string]string
	// Timeout de cada envío (por defecto 10s).
	Timeout time.Duration
	// Resource identifica el origen (service.name, version, environment).
	Resource map[string]string
	// Client HTTP opcional (por defecto uno con el Timeout dado).
	Client *http.Client
}

// OTLPHTTPExporter exporta trazas y métricas vía OTLP/HTTP (JSON). Implementa el
// mapeo mínimo del esquema OTLP (resourceSpans/resourceMetrics) sin depender del
// SDK de OpenTelemetry, para no arrastrar dependencias fuera de la política de
// licencias. El colector estándar de OTel acepta este formato en :4318.
type OTLPHTTPExporter struct {
	cfg    OTLPHTTPConfig
	client *http.Client
}

// NewOTLPHTTPExporter construye el exportador OTLP/HTTP.
func NewOTLPHTTPExporter(cfg OTLPHTTPConfig) (*OTLPHTTPExporter, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("telemetry: endpoint OTLP obligatorio")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	return &OTLPHTTPExporter{cfg: cfg, client: client}, nil
}

// ExportSpans envía spans a /v1/traces en el esquema OTLP/JSON.
func (e *OTLPHTTPExporter) ExportSpans(ctx context.Context, spans []SpanData) error {
	if len(spans) == 0 {
		return nil
	}
	otlpSpans := make([]map[string]any, 0, len(spans))
	for _, s := range spans {
		span := map[string]any{
			"traceId":           s.TraceID.String(),
			"spanId":            s.SpanID.String(),
			"name":              s.Name,
			"kind":              1, // SPAN_KIND_INTERNAL
			"startTimeUnixNano": strconv.FormatInt(s.Start.UnixNano(), 10),
			"endTimeUnixNano":   strconv.FormatInt(s.End.UnixNano(), 10),
			"attributes":        otlpAttrs(s.Attributes),
		}
		if s.ParentID != (SpanID{}) {
			span["parentSpanId"] = s.ParentID.String()
		}
		if s.Err != nil {
			span["status"] = map[string]any{"code": 2, "message": s.Err.Error()} // STATUS_CODE_ERROR
		}
		otlpSpans = append(otlpSpans, span)
	}
	payload := map[string]any{
		"resourceSpans": []map[string]any{{
			"resource":   map[string]any{"attributes": otlpResource(e.cfg.Resource)},
			"scopeSpans": []map[string]any{{"scope": map[string]any{"name": "sytue-one"}, "spans": otlpSpans}},
		}},
	}
	return e.post(ctx, "/v1/traces", payload)
}

// ExportMetrics envía métricas a /v1/metrics en el esquema OTLP/JSON.
func (e *OTLPHTTPExporter) ExportMetrics(ctx context.Context, metrics []MetricData) error {
	if len(metrics) == 0 {
		return nil
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 10)
	otlpMetrics := make([]map[string]any, 0, len(metrics))
	for _, m := range metrics {
		switch m.Kind {
		case "counter":
			otlpMetrics = append(otlpMetrics, map[string]any{
				"name": m.Name,
				"sum": map[string]any{
					"aggregationTemporality": 2, // CUMULATIVE
					"isMonotonic":            true,
					"dataPoints": []map[string]any{{
						"asDouble":     m.Sum,
						"timeUnixNano": now,
						"attributes":   otlpAttrs(m.Attributes),
					}},
				},
			})
		case "histogram":
			counts := make([]uint64, 0, len(m.Buckets))
			bounds := make([]float64, 0, len(m.Buckets))
			var prev uint64
			for _, b := range m.Buckets {
				counts = append(counts, b.Count-prev)
				prev = b.Count
				bounds = append(bounds, b.LE)
			}
			counts = append(counts, m.Count-prev) // +Inf bucket
			otlpMetrics = append(otlpMetrics, map[string]any{
				"name": m.Name,
				"histogram": map[string]any{
					"aggregationTemporality": 2,
					"dataPoints": []map[string]any{{
						"count":          m.Count,
						"sum":            m.HistSum,
						"bucketCounts":   counts,
						"explicitBounds": bounds,
						"timeUnixNano":   now,
						"attributes":     otlpAttrs(m.Attributes),
					}},
				},
			})
		}
	}
	payload := map[string]any{
		"resourceMetrics": []map[string]any{{
			"resource":     map[string]any{"attributes": otlpResource(e.cfg.Resource)},
			"scopeMetrics": []map[string]any{{"scope": map[string]any{"name": "sytue-one"}, "metrics": otlpMetrics}},
		}},
	}
	return e.post(ctx, "/v1/metrics", payload)
}

// Shutdown es no-op (sin buffers internos que vaciar).
func (e *OTLPHTTPExporter) Shutdown(context.Context) error { return nil }

func (e *OTLPHTTPExporter) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telemetry: marshal OTLP: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.cfg.Endpoint+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telemetry: request OTLP: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range e.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("telemetry: envío OTLP: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("telemetry: OTLP %s devolvió %d", path, resp.StatusCode)
	}
	return nil
}

// otlpResource mapea el resource a atributos OTLP.
func otlpResource(res map[string]string) []map[string]any {
	out := make([]map[string]any, 0, len(res))
	for k, v := range res {
		if v == "" {
			continue
		}
		out = append(out, map[string]any{"key": k, "value": map[string]any{"stringValue": v}})
	}
	return out
}

// otlpAttrs mapea atributos internos al formato KeyValue de OTLP.
func otlpAttrs(attrs []Attr) []map[string]any {
	out := make([]map[string]any, 0, len(attrs))
	for _, a := range attrs {
		var val map[string]any
		switch v := a.Value.(type) {
		case string:
			val = map[string]any{"stringValue": v}
		case bool:
			val = map[string]any{"boolValue": v}
		case int:
			val = map[string]any{"intValue": strconv.Itoa(v)}
		case int64:
			val = map[string]any{"intValue": strconv.FormatInt(v, 10)}
		case float64:
			val = map[string]any{"doubleValue": v}
		default:
			val = map[string]any{"stringValue": valueString(a.Value)}
		}
		out = append(out, map[string]any{"key": a.Key, "value": val})
	}
	return out
}

// valueString formatea un valor de atributo como cadena estable.
func valueString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Variables estándar de OpenTelemetry para el destino. Se usan las oficiales y no
// unas propias para que cualquiera que ya sepa configurar un colector no tenga
// que aprenderse la nomenclatura de esta casa.
const (
	EnvOTLPEndpoint = "OTEL_EXPORTER_OTLP_ENDPOINT"
	EnvOTLPHeaders  = "OTEL_EXPORTER_OTLP_HEADERS"
)

// exporterDelEntorno construye el exportador OTLP si el entorno dice a dónde
// mandar, y devuelve el de descarte si no.
//
// Va aquí y no en cada servicio por lo mismo que en los SDK oficiales de
// OpenTelemetry: son quince binarios y el destino es una decisión de despliegue,
// no de código. Ninguno configuraba exportador, así que TODOS acumulaban métricas
// para nadie; con esto basta poner la variable en el chart.
//
// Un endpoint mal formado NO impide arrancar: se cae a descarte. Quedarse sin
// servicio porque el colector tiene una errata en la URL sería el peor cambio
// posible.
func exporterDelEntorno(cfg Config) Exporter {
	endpoint := strings.TrimSpace(os.Getenv(EnvOTLPEndpoint))
	if endpoint == "" {
		return NopExporter{}
	}
	exp, err := NewOTLPHTTPExporter(OTLPHTTPConfig{
		Endpoint: endpoint,
		Headers:  cabecerasOTLP(os.Getenv(EnvOTLPHeaders)),
		Resource: map[string]string{
			"service.name":           cfg.Service,
			"service.version":        cfg.Version,
			"deployment.environment": cfg.Env,
		},
	})
	if err != nil {
		return NopExporter{}
	}
	return exp
}

// cabecerasOTLP interpreta el formato de la especificación: `clave=valor` con
// comas. Sirve para el token del colector cuando lo hay.
func cabecerasOTLP(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := map[string]string{}
	for _, par := range strings.Split(raw, ",") {
		k, v, ok := strings.Cut(par, "=")
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if ok && k != "" {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
