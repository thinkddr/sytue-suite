// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package log

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// Captura de avisos y errores para poder ENSEÑARLOS.
//
// Los servicios escriben JSON a stdout y ahí muere: sin un agregador, la única
// forma de saber qué está fallando en producción es abrir una terminal contra
// el clúster, y los logs de un pod que se reinició ya no existen. Eso convierte
// «¿qué está roto?» en una pregunta que solo puede responder quien tiene
// kubeconfig, y solo mientras el pod siga vivo.
//
// Se capturan SOLO warn y error. El info es el 99 % del volumen y no responde
// esa pregunta; guardarlo sería pagar almacenamiento para no leerlo nunca.
//
// Tres decisiones que gobiernan esto:
//
//   - NUNCA BLOQUEA. La cola tiene tope y, si se llena, se descarta y se cuenta
//     lo descartado. Un servicio que se frena porque no puede escribir sus
//     propios logs convierte un incidente en una caída, y justo cuando más
//     logs se están generando.
//   - Va DESPUÉS de la redacción (el redactor de este paquete envuelve a este
//     handler), así que lo que se guarda ya viene sin secretos ni PII.
//   - El sumidero se conecta MÁS TARDE que el logger, porque la base de datos
//     todavía no existe cuando arranca el logging. De ahí SinkRef.
//
// Las etiquetas JSON son el formato de cable del recolector remoto
// (collector.go): lo que viaja al IdP y lo que este acepta es ESTA estructura.
type Entry struct {
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Service string    `json:"service"`
	Message string    `json:"message"`
	// Attrs son los atributos del registro, ya redactados y aplanados a texto:
	// se guardan para poder enseñar el detalle sin arrastrar tipos de Go.
	Attrs map[string]string `json:"attrs,omitempty"`
}

// Sink persiste un lote de entradas. Lo implementa la capa de persistencia de
// cada servicio.
type Sink interface {
	Write(ctx context.Context, batch []Entry) error
}

// SinkRef es un sumidero que se puede enchufar después de construir el logger.
//
// El orden de arranque manda: primero el logger —para poder registrar los
// fallos del propio arranque— y solo después la base de datos. Sin esta
// indirección habría que elegir entre no capturar el arranque o construir el
// logger sin poder registrar por qué no se pudo abrir la base.
type SinkRef struct{ v atomic.Pointer[Sink] }

// Set conecta el sumidero. Hasta que se llame, todo lo capturado se descarta.
func (r *SinkRef) Set(s Sink) {
	if s != nil {
		r.v.Store(&s)
	}
}

func (r *SinkRef) get() Sink {
	if p := r.v.Load(); p != nil {
		return *p
	}
	return nil
}

// tamaños de la cola y del lote. Cien entradas por INSERT es lo que hace que
// una ráfaga de errores no se convierta en cien viajes a la base; dos mil de
// cola aguantan varios segundos de tormenta antes de empezar a descartar.
const (
	colaMax = 2000
	loteMax = 100
	// volcado periódico: sin él, un error suelto se quedaría en el buffer hasta
	// que llegaran otros noventa y nueve, y la pantalla no lo vería en minutos.
	intervaloVolcado = 2 * time.Second
)

type captureHandler struct {
	inner   slog.Handler
	attrs   []slog.Attr
	grupo   string
	estado  *captureEstado
	service string
}

type captureEstado struct {
	ref       *SinkRef
	cola      chan Entry
	desechado atomic.Int64
	cerrar    sync.Once
	hecho     chan struct{}
}

// NewCaptureHandler envuelve un handler capturando warn y error hacia el
// sumidero. Devuelve el handler y una función de parada que vuelca lo pendiente.
func NewCaptureHandler(inner slog.Handler, ref *SinkRef, service string) (slog.Handler, func(context.Context)) {
	est := &captureEstado{ref: ref, cola: make(chan Entry, colaMax), hecho: make(chan struct{})}
	go est.bucle()
	h := &captureHandler{inner: inner, estado: est, service: service}
	return h, est.parar
}

func (h *captureHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *captureHandler) Handle(ctx context.Context, r slog.Record) error {
	// Lo capturado no puede cambiar lo que se escribe en stdout: primero el
	// camino normal, y la captura como efecto secundario que puede fallar sin
	// que nadie se entere.
	if r.Level >= slog.LevelWarn {
		h.encolar(r)
	}
	return h.inner.Handle(ctx, r)
}

func (h *captureHandler) encolar(r slog.Record) {
	attrs := make(map[string]string, r.NumAttrs()+len(h.attrs))
	for _, a := range h.attrs {
		attrs[a.Key] = fmt.Sprint(a.Value.Any())
	}
	r.Attrs(func(a slog.Attr) bool {
		clave := a.Key
		if h.grupo != "" {
			clave = h.grupo + "." + clave
		}
		attrs[clave] = fmt.Sprint(a.Value.Any())
		return true
	})
	e := Entry{
		At:      r.Time,
		Level:   r.Level.String(),
		Service: h.service,
		Message: r.Message,
		Attrs:   attrs,
	}
	select {
	case h.estado.cola <- e:
	default:
		// Cola llena: se descarta y se cuenta. Perder líneas es peor que perder
		// el servicio, y el contador sale por su propio log para que se note.
		h.estado.desechado.Add(1)
	}
}

func (h *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	nuevos := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	nuevos = append(nuevos, h.attrs...)
	nuevos = append(nuevos, attrs...)
	return &captureHandler{
		inner: h.inner.WithAttrs(attrs), attrs: nuevos,
		grupo: h.grupo, estado: h.estado, service: h.service,
	}
}

func (h *captureHandler) WithGroup(name string) slog.Handler {
	return &captureHandler{
		inner: h.inner.WithGroup(name), attrs: h.attrs,
		grupo: name, estado: h.estado, service: h.service,
	}
}

func (e *captureEstado) bucle() {
	defer close(e.hecho)
	tick := time.NewTicker(intervaloVolcado)
	defer tick.Stop()
	lote := make([]Entry, 0, loteMax)
	for {
		select {
		case entrada, abierto := <-e.cola:
			if !abierto {
				e.volcar(lote)
				return
			}
			lote = append(lote, entrada)
			if len(lote) >= loteMax {
				e.volcar(lote)
				lote = lote[:0]
			}
		case <-tick.C:
			if len(lote) > 0 {
				e.volcar(lote)
				lote = lote[:0]
			}
		}
	}
}

func (e *captureEstado) volcar(lote []Entry) {
	if len(lote) == 0 {
		return
	}
	s := e.ref.get()
	if s == nil {
		return // todavía no hay base: se pierde, y es lo correcto al arrancar
	}
	// Contexto propio y acotado: heredar el de la petición que generó el error
	// haría que el registro se perdiera justo cuando esa petición se cancela,
	// que es cuando más falta hace.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	copia := make([]Entry, len(lote))
	copy(copia, lote)
	// Un fallo al guardar NO se registra con slog: sería un error que genera
	// otro error que intenta guardarse, y así hasta llenar la cola.
	_ = s.Write(ctx, copia)
}

func (e *captureEstado) parar(ctx context.Context) {
	e.cerrar.Do(func() { close(e.cola) })
	select {
	case <-e.hecho:
	case <-ctx.Done():
	}
}

// Descartados devuelve cuántas entradas se perdieron por cola llena. Lo publica
// el propio servicio como métrica: si crece, o hay una tormenta de errores o el
// sumidero no da abasto, y las dos cosas importan.
func (e *captureEstado) Descartados() int64 { return e.desechado.Load() }
