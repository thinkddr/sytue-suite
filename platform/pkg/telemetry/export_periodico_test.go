// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package telemetry

import (
	"context"
	"sync"
	"testing"
	"time"
)

// espia cuenta exportaciones. Con mutex porque el bucle periódico corre en su
// propia goroutine y el test lee desde la suya.
type espia struct {
	mu     sync.Mutex
	rondas int
	puntos int
}

func (e *espia) ExportSpans(context.Context, []SpanData) error { return nil }

func (e *espia) ExportMetrics(_ context.Context, m []MetricData) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rondas++
	e.puntos += len(m)
	return nil
}

func (e *espia) Shutdown(context.Context) error { return nil }

func (e *espia) leer() (int, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.rondas, e.puntos
}

// Las métricas salen del proceso ESTANDO VIVO, no al morir.
//
// Es la diferencia entre servir para escalar y no servir para nada. Hasta el
// 2026-08-11 `flushMetrics` solo se llamaba desde Shutdown: un pod exportaba sus
// cifras justo cuando lo estaban matando, que es cuando ya no hay decisión que
// tomar con ellas.
func TestLasMetricasSalenSinEsperarAlCierre(t *testing.T) {
	e := &espia{}
	p, err := NewProvider(t.Context(), Config{
		Service: "prueba", Exporter: e, ExportInterval: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	t.Cleanup(func() { _ = p.Shutdown(context.Background()) })

	p.Meter("x").Counter("peticiones").Add(t.Context(), 1)

	plazo := time.After(2 * time.Second)
	for {
		if rondas, _ := e.leer(); rondas > 0 {
			return // exportó sin que nadie cerrara nada
		}
		select {
		case <-plazo:
			t.Fatal("dos segundos sin exportar: el bucle periódico no está funcionando")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// Con exporter de descarte NO se monta el bucle: sería despertar un temporizador
// cada treinta segundos, en cada servicio, para tirar el trabajo a la basura.
func TestSinExporterNoSeMontaElBucle(t *testing.T) {
	p, err := NewProvider(t.Context(), Config{Service: "prueba"})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if p.parar == nil {
		t.Fatal("el canal de parada debe existir aunque no haya bucle")
	}
	// Si se hubiera montado, este Wait se quedaría colgado hasta el close.
	hecho := make(chan struct{})
	go func() { p.exportada.Wait(); close(hecho) }()
	select {
	case <-hecho:
	case <-time.After(time.Second):
		t.Fatal("hay un bucle de exportación corriendo con NopExporter")
	}
	_ = p.Shutdown(context.Background())
}

// Shutdown dos veces no panica. Pasa de verdad: un `defer` en main y un apagado
// explícito en la ruta de error, y cerrar un canal ya cerrado es un panic que
// solo aparece el día que algo falla.
func TestCerrarDosVecesNoPanica(t *testing.T) {
	p, err := NewProvider(t.Context(), Config{
		Service: "prueba", Exporter: &espia{}, ExportInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("primer cierre: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("segundo cierre: %v", err)
	}
}

// Sin la variable de entorno, descarte; con ella, exportador real.
//
// Es lo que hace que baste una variable en el chart para encender la telemetría
// de los quince binarios, sin tocar código.
func TestElDestinoSaleDelEntorno(t *testing.T) {
	t.Setenv(EnvOTLPEndpoint, "")
	if e := exporterDelEntorno(Config{Service: "x"}); !esNoop(e) {
		t.Errorf("sin endpoint debería ser descarte, es %T", e)
	}
	t.Setenv(EnvOTLPEndpoint, "http://colector:4318")
	if e := exporterDelEntorno(Config{Service: "x"}); esNoop(e) {
		t.Error("con endpoint debería construir el exportador OTLP")
	}
}

// Un endpoint ilegible NO impide arrancar: cae a descarte. Quedarse sin servicio
// porque el colector tiene una errata en la URL sería el peor cambio posible.
func TestUnEndpointRotoNoTumbaElServicio(t *testing.T) {
	t.Setenv(EnvOTLPEndpoint, "://esto-no-es-una-url")
	p, err := NewProvider(t.Context(), Config{Service: "x"})
	if err != nil {
		t.Fatalf("el provider no debe fallar por un endpoint roto: %v", err)
	}
	_ = p.Shutdown(t.Context())
}

// Las cabeceras siguen el formato de la especificación: clave=valor con comas.
func TestLasCabecerasSiguenLaEspecificacion(t *testing.T) {
	got := cabecerasOTLP(" Authorization=Bearer abc , X-Scope-OrgID=sytue ")
	if got["Authorization"] != "Bearer abc" || got["X-Scope-OrgID"] != "sytue" {
		t.Errorf("cabeceras mal interpretadas: %v", got)
	}
	if cabecerasOTLP("") != nil || cabecerasOTLP("basura-sin-igual") != nil {
		t.Error("una cadena vacía o sin pares debe dar nil, no un mapa vacío")
	}
}
