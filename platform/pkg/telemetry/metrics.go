// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// Meter crea instrumentos de métrica ligados a un Provider.
type Meter struct {
	provider *Provider
	name     string
}

// Metric es la interfaz común de un instrumento agregable.
type Metric interface {
	snapshot() []MetricData
}

// MetricData es la representación exportable de un punto de métrica.
type MetricData struct {
	Name       string
	Scope      string
	Kind       string // "counter" | "histogram"
	Attributes []Attr
	// Counter
	Sum float64
	// Histogram
	Count   uint64
	HistSum float64
	Buckets []HistogramBucket
}

// HistogramBucket es un cubo acumulativo (le, count) de un histograma.
type HistogramBucket struct {
	LE    float64
	Count uint64
}

// Counter es un contador monotónico con desglose por conjunto de atributos.
type Counter struct {
	name  string
	scope string
	mu    sync.Mutex
	vals  map[string]*counterSeries
}

type counterSeries struct {
	attrs []Attr
	sum   float64
}

// Counter crea (o reutiliza) un contador con nombre y lo registra en el Provider.
func (m *Meter) Counter(name string) *Counter {
	c := &Counter{name: name, scope: m.name, vals: make(map[string]*counterSeries)}
	m.provider.register(c)
	return c
}

// Add incrementa el contador para el conjunto de atributos dado.
func (c *Counter) Add(_ context.Context, delta int64, attrs ...Attr) {
	if delta < 0 {
		return // monotónico
	}
	key := attrKey(attrs)
	c.mu.Lock()
	s, ok := c.vals[key]
	if !ok {
		s = &counterSeries{attrs: attrs}
		c.vals[key] = s
	}
	s.sum += float64(delta)
	c.mu.Unlock()
}

func (c *Counter) snapshot() []MetricData {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]MetricData, 0, len(c.vals))
	for _, s := range c.vals {
		out = append(out, MetricData{
			Name: c.name, Scope: c.scope, Kind: "counter",
			Attributes: s.attrs, Sum: s.sum,
		})
	}
	return out
}

// DefaultBoundaries son los límites (segundos) por defecto de los histogramas de
// latencia, alineados con el SLO p95 < 200 ms de la suite.
var DefaultBoundaries = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.2, 0.5, 1, 2.5, 5, 10}

// Histogram registra distribuciones (latencias, tamaños) con cubos acumulativos.
type Histogram struct {
	name   string
	scope  string
	bounds []float64
	mu     sync.Mutex
	series map[string]*histSeries
}

type histSeries struct {
	attrs  []Attr
	counts []uint64 // len(bounds)+1 (último = +Inf)
	sum    float64
	total  uint64
}

// Histogram crea un histograma con los límites por defecto.
func (m *Meter) Histogram(name string) *Histogram {
	return m.HistogramWithBounds(name, DefaultBoundaries)
}

// HistogramWithBounds crea un histograma con límites personalizados (ascendentes).
func (m *Meter) HistogramWithBounds(name string, bounds []float64) *Histogram {
	b := append([]float64(nil), bounds...)
	sort.Float64s(b)
	h := &Histogram{name: name, scope: m.name, bounds: b, series: make(map[string]*histSeries)}
	m.provider.register(h)
	return h
}

// Record observa un valor para el conjunto de atributos dado.
func (h *Histogram) Record(_ context.Context, v float64, attrs ...Attr) {
	key := attrKey(attrs)
	h.mu.Lock()
	s, ok := h.series[key]
	if !ok {
		s = &histSeries{attrs: attrs, counts: make([]uint64, len(h.bounds)+1)}
		h.series[key] = s
	}
	idx := sort.SearchFloat64s(h.bounds, v)
	s.counts[idx]++
	s.sum += v
	s.total++
	h.mu.Unlock()
}

func (h *Histogram) snapshot() []MetricData {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]MetricData, 0, len(h.series))
	for _, s := range h.series {
		buckets := make([]HistogramBucket, 0, len(h.bounds))
		var cumulative uint64
		for i, b := range h.bounds {
			cumulative += s.counts[i]
			buckets = append(buckets, HistogramBucket{LE: b, Count: cumulative})
		}
		out = append(out, MetricData{
			Name: h.name, Scope: h.scope, Kind: "histogram",
			Attributes: s.attrs, Count: s.total, HistSum: s.sum, Buckets: buckets,
		})
	}
	return out
}

// register añade un instrumento al Provider para su exportación.
func (p *Provider) register(m Metric) {
	p.mu.Lock()
	p.metrics = append(p.metrics, m)
	p.mu.Unlock()
}

// Collect toma un snapshot de todas las métricas registradas.
func (p *Provider) Collect() []MetricData {
	p.mu.Lock()
	instruments := append([]Metric(nil), p.metrics...)
	p.mu.Unlock()
	var out []MetricData
	for _, m := range instruments {
		out = append(out, m.snapshot()...)
	}
	return out
}

// flushMetrics exporta un snapshot de métricas al colector.
func (p *Provider) flushMetrics(ctx context.Context) error {
	data := p.Collect()
	if len(data) == 0 {
		return nil
	}
	return p.exporter.ExportMetrics(ctx, data)
}

// FlushMetrics exporta un snapshot bajo demanda (p. ej. desde un scrape/timer).
func (p *Provider) FlushMetrics(ctx context.Context) error { return p.flushMetrics(ctx) }

// attrKey deriva una clave estable de un conjunto de atributos (para agregar).
func attrKey(attrs []Attr) string {
	if len(attrs) == 0 {
		return ""
	}
	pairs := make([]string, len(attrs))
	for i, a := range attrs {
		pairs[i] = a.Key + "=" + valueString(a.Value)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

// Provider por defecto para instrumentación transversal.
//
// Existe por lo mismo que [SetLogInjector], que es el precedente de este paquete:
// hay puntos de instrumentación —el middleware HTTP común, sin ir más lejos— que
// viven en código compartido por los catorce servicios y a los que hacer llegar el
// Provider explícitamente obligaría a tocar tres sitios por servicio para lo que
// siempre es el mismo objeto. Un servicio tiene UN Provider, no varios.
//
// No es un sustituto de pasarlo a mano: quien pueda hacerlo, mejor, y los tests
// deben usar siempre el suyo para no depender de este.
var (
	defaultMu       sync.RWMutex
	defaultProvider *Provider
)

// SetDefault fija el Provider que usará la instrumentación transversal. Se llama
// una vez, al arrancar el servicio, junto a NewProvider.
func SetDefault(p *Provider) {
	defaultMu.Lock()
	defaultProvider = p
	defaultMu.Unlock()
}

// DefaultMeter devuelve un Meter del Provider por defecto, o nil si no se ha
// fijado ninguno. Devolver nil y no un Meter vacío es deliberado: quien lo use
// tiene que decidir explícitamente qué hacer sin telemetría, y la respuesta
// correcta siempre es seguir funcionando.
func DefaultMeter(name string) *Meter {
	defaultMu.RLock()
	p := defaultProvider
	defaultMu.RUnlock()
	if p == nil {
		return nil
	}
	return p.Meter(name)
}
