// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package telemetry aporta observabilidad correlacionada (trazas + métricas +
// logs) a los servicios Go de Sytue One, sobre estándares abiertos:
//
//   - Contexto de traza W3C Trace Context (traceparent) propagable por HTTP.
//   - Spans con IDs de traza/span de 16/8 bytes (formato OTel/W3C).
//   - Métricas (contadores/histogramas) con atributos.
//   - Exportación OTLP (protobuf/JSON sobre HTTP) hacia el colector de la suite,
//     mediante un Exporter enchufable (ver OTLPHTTPExporter). Por defecto es
//     no-op (los datos quedan disponibles para el logger y /debug).
//
// Los IDs de traza se inyectan en el logger (trace_id/span_id) para correlacionar
// las tres señales sin acoplar el núcleo al SDK de OpenTelemetry (dependencia no
// permisiva de arrastre); la API es estable y compatible en intención con OTel.
package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Provider es la raíz de telemetría de un servicio. Construye tracers y meters
// y despacha las señales al Exporter configurado.
type Provider struct {
	service  string
	version  string
	env      string
	exporter Exporter
	mu       sync.Mutex
	metrics  []Metric
	// parar cierra el bucle de exportación periódica; una sola vez, porque
	// Shutdown puede llamarse dos veces (un defer y un apagado explícito) y
	// cerrar un canal cerrado es un panic.
	parar     chan struct{}
	pararUna  sync.Once
	exportada sync.WaitGroup
}

// Config configura el Provider.
type Config struct {
	// Service es el nombre del servicio (resource.service.name).
	Service string
	// Version es la versión del binario (resource.service.version).
	Version string
	// Env es el entorno de despliegue (deployment.environment).
	Env string
	// Exporter recibe spans y métricas. Si es nil, se usa un exporter no-op.
	Exporter Exporter
	// ExportInterval es cada cuánto se envían las métricas acumuladas. Con 0 se
	// usa DefaultExportInterval; con un valor negativo NO se exporta hasta el
	// cierre, que es lo que hay que pedir explícitamente en un test.
	//
	// ESTO NO EXISTÍA, y sin ello la instrumentación no servía para nada: las
	// métricas solo salían en Shutdown, o sea cuando el proceso ya se está
	// muriendo. Para decidir un escalado hacen falta antes, y de un pod vivo.
	ExportInterval time.Duration
}

// DefaultExportInterval es el periodo de exportación por defecto. Treinta
// segundos es el intervalo con el que un HPA consulta métricas: más lento haría
// escalar sobre datos viejos, y más rápido multiplica peticiones al colector sin
// que nadie las mire.
const DefaultExportInterval = 30 * time.Second

// Exporter recibe las señales de telemetría para enviarlas al colector.
// Implementaciones: OTLPHTTPExporter (OTLP/HTTP) o NopExporter.
type Exporter interface {
	// ExportSpans envía un lote de spans finalizados.
	ExportSpans(ctx context.Context, spans []SpanData) error
	// ExportMetrics envía un snapshot de métricas.
	ExportMetrics(ctx context.Context, metrics []MetricData) error
	// Shutdown libera recursos del exporter (flush pendiente).
	Shutdown(ctx context.Context) error
}

// NewProvider crea un Provider. El contexto controla el ciclo de vida.
func NewProvider(ctx context.Context, cfg Config) (*Provider, error) {
	exp := cfg.Exporter
	if exp == nil {
		exp = exporterDelEntorno(cfg)
	}
	p := &Provider{
		service:  cfg.Service,
		version:  cfg.Version,
		env:      cfg.Env,
		exporter: exp,
		parar:    make(chan struct{}),
	}

	intervalo := cfg.ExportInterval
	if intervalo == 0 {
		intervalo = DefaultExportInterval
	}
	// Con NopExporter no se monta el bucle: sería despertar un temporizador cada
	// treinta segundos, en cada servicio, para tirar el resultado a la basura.
	if intervalo > 0 && !esNoop(exp) {
		p.exportada.Add(1)
		// El ctx del proceso entra en el bucle: así el exportador se para por las
		// DOS vías que tiene un servicio para terminar —Shutdown, que cierra
		// `parar`, y la señal de apagado, que cancela este contexto—. Sin él,
		// una cancelación dejaba la goroutine dando vueltas hasta que alguien se
		// acordara de llamar a Shutdown.
		go p.exportarCada(ctx, intervalo)
	}
	return p, nil
}

// esNoop distingue el exporter de descarte. No es una comprobación cosmética:
// decide si se arranca una goroutine por servicio.
func esNoop(e Exporter) bool {
	_, ok := e.(NopExporter)
	return ok
}

// exportarCada envía las métricas acumuladas periódicamente hasta que se cierre
// el Provider.
//
// Un fallo de exportación NO detiene el bucle ni tumba el servicio: el colector
// puede estar reiniciándose, y perder una ventana de métricas es infinitamente
// preferible a que la telemetría se lleve por delante al servicio que observa.
func (p *Provider) exportarCada(ctx context.Context, intervalo time.Duration) {
	defer p.exportada.Done()
	t := time.NewTicker(intervalo)
	defer t.Stop()
	for {
		select {
		case <-p.parar:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			// El plazo de cada vuelta cuelga del ctx del proceso: si el servicio
			// se está apagando, el envío en curso no lo entretiene. El vaciado
			// FINAL no pasa por aquí —lo hace Shutdown con su propio plazo—.
			vuelta, cancel := context.WithTimeout(ctx, intervalo)
			_ = p.flushMetrics(vuelta)
			cancel()
		}
	}
}

// Resource describe el origen de las señales (OTel resource semantics).
func (p *Provider) Resource() map[string]string {
	return map[string]string{
		"service.name":           p.service,
		"service.version":        p.version,
		"deployment.environment": p.env,
	}
}

// Shutdown detiene la exportación periódica, vacía lo pendiente y cierra el
// exporter. Es idempotente: se llama desde un defer y a veces también a mano.
func (p *Provider) Shutdown(ctx context.Context) error {
	p.pararUna.Do(func() { close(p.parar) })
	// Se espera al bucle ANTES de vaciar: si siguiera vivo, su exportación y esta
	// competirían por el mismo snapshot y el colector recibiría el tramo dos veces.
	p.exportada.Wait()
	// Exportar métricas acumuladas antes de cerrar.
	if err := p.flushMetrics(ctx); err != nil {
		_ = p.exporter.Shutdown(ctx)
		return err
	}
	return p.exporter.Shutdown(ctx)
}

// Tracer devuelve un Tracer con nombre de instrumentación.
func (p *Provider) Tracer(name string) *Tracer {
	return &Tracer{provider: p, name: name}
}

// Meter devuelve un Meter con nombre de instrumentación.
func (p *Provider) Meter(name string) *Meter {
	return &Meter{provider: p, name: name}
}

// Attr es un par clave/valor adjuntable a spans y métricas.
type Attr struct {
	Key   string
	Value any
}

// String construye un Attr de cadena.
func String(k, v string) Attr { return Attr{Key: k, Value: v} }

// Int construye un Attr entero.
func Int(k string, v int) Attr { return Attr{Key: k, Value: v} }

// Bool construye un Attr booleano.
func Bool(k string, v bool) Attr { return Attr{Key: k, Value: v} }

// ---- Trazas ----

// TraceID es un identificador de traza W3C de 16 bytes.
type TraceID [16]byte

// SpanID es un identificador de span W3C de 8 bytes.
type SpanID [8]byte

// String devuelve la representación hex (32 chars) del TraceID.
func (t TraceID) String() string { return hex.EncodeToString(t[:]) }

// String devuelve la representación hex (16 chars) del SpanID.
func (s SpanID) String() string { return hex.EncodeToString(s[:]) }

// IsZero indica si el TraceID es el valor cero (sin traza).
func (t TraceID) IsZero() bool { return t == TraceID{} }

// SpanContext transporta la identidad de un span por el context.Context.
type SpanContext struct {
	TraceID TraceID
	SpanID  SpanID
	Sampled bool
	remote  bool
}

// IsValid indica si el SpanContext referencia una traza real.
func (sc SpanContext) IsValid() bool { return !sc.TraceID.IsZero() }

type ctxSpanKey struct{}

// ContextWithSpanContext inserta un SpanContext en el contexto.
func ContextWithSpanContext(ctx context.Context, sc SpanContext) context.Context {
	return context.WithValue(ctx, ctxSpanKey{}, sc)
}

// SpanContextFromContext extrae el SpanContext activo (o el cero si no hay).
func SpanContextFromContext(ctx context.Context) SpanContext {
	if ctx != nil {
		if sc, ok := ctx.Value(ctxSpanKey{}).(SpanContext); ok {
			return sc
		}
	}
	return SpanContext{}
}

// Tracer crea spans.
type Tracer struct {
	provider *Provider
	name     string
}

// SpanData es la representación exportable de un span finalizado.
type SpanData struct {
	Name       string
	TraceID    TraceID
	SpanID     SpanID
	ParentID   SpanID
	Start      time.Time
	End        time.Time
	Attributes []Attr
	Err        error
	Scope      string
}

// Span representa una unidad de trabajo trazada.
type Span struct {
	tracer   *Tracer
	name     string
	sc       SpanContext
	parentID SpanID
	start    time.Time
	attrs    []Attr
	ended    bool
	err      error
}

// Start abre un span. Hereda la traza del contexto si existe; si no, arranca una
// nueva. Devuelve el contexto con el nuevo SpanContext y el span.
func (t *Tracer) Start(ctx context.Context, name string, attrs ...Attr) (context.Context, *Span) {
	parent := SpanContextFromContext(ctx)
	sc := SpanContext{Sampled: true}
	if parent.IsValid() {
		sc.TraceID = parent.TraceID
	} else {
		sc.TraceID = newTraceID()
	}
	sc.SpanID = newSpanID()

	span := &Span{
		tracer:   t,
		name:     name,
		sc:       sc,
		parentID: parent.SpanID,
		start:    time.Now(),
		attrs:    attrs,
	}
	return ContextWithSpanContext(ctx, sc), span
}

// SpanContext devuelve el contexto de traza del span.
func (s *Span) SpanContext() SpanContext {
	if s == nil {
		return SpanContext{}
	}
	return s.sc
}

// SetAttributes añade atributos al span.
func (s *Span) SetAttributes(attrs ...Attr) {
	if s == nil {
		return
	}
	s.attrs = append(s.attrs, attrs...)
}

// RecordError marca el span con un error.
func (s *Span) RecordError(err error) {
	if s != nil {
		s.err = err
	}
}

// End cierra el span y lo entrega al exporter.
func (s *Span) End() {
	if s == nil || s.ended {
		return
	}
	s.ended = true
	data := SpanData{
		Name:       s.name,
		TraceID:    s.sc.TraceID,
		SpanID:     s.sc.SpanID,
		ParentID:   s.parentID,
		Start:      s.start,
		End:        time.Now(),
		Attributes: s.attrs,
		Err:        s.err,
		Scope:      s.tracer.name,
	}
	// Exportación best-effort, no bloqueante para el request path.
	_ = s.tracer.provider.exporter.ExportSpans(context.Background(), []SpanData{data})
}

func newTraceID() TraceID {
	var id TraceID
	_, _ = rand.Read(id[:])
	return id
}

func newSpanID() SpanID {
	var id SpanID
	_, _ = rand.Read(id[:])
	return id
}
