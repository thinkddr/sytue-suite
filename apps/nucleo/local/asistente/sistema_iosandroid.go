// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package asistente

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// La IA del sistema en el MÓVIL: Foundation Models en iOS y iPadOS 26 (sistema_ios.go, por
// un puente Swift) y Gemini Nano por ML Kit GenAI en Android (sistema_android.go, por un
// puente Java). Los dos son un modelo en el dispositivo que no saca nada del teléfono, los
// dos pueden no estar (teléfono sin el modelo, Apple Intelligence apagada, el modelo aún
// bajándose, la app sin el puente) y los dos contestan en segundos. Lo común está aquí,
// sin etiqueta de sistema, para probarlo en cualquier máquina con un puente de mentira.

// EstadoSistema es lo que dice el sistema del móvil de su modelo. El núcleo da el
// ESTADO y la pantalla pone el texto en su idioma (`local.App.NombreIA`), como con
// [Asistente.DestinoCon]: aquí solo queda el motivo en español de los errores y los
// registros.
type EstadoSistema int

// Los estados de la IA del sistema del móvil.
const (
	SistemaComprobando EstadoSistema = iota // aún no se ha preguntado
	SistemaLista                            // lista para contestar
	SistemaDescargable                      // Android: AICore la puede bajar (se le pide al usarla)
	SistemaDescargando                      // bajándose o preparándose
	SistemaNoSoportada                      // este dispositivo no la tiene
	SistemaApagada                          // iOS: Apple Intelligence apagada en Ajustes
	SistemaAntiguo                          // antes de iOS 26 o de Android 8
	SistemaSinPuente                        // la app no lleva el puente (framework o ML Kit)
	SistemaFallo                            // el sistema no contestó
)

// Estados son todos, en orden: los que la pantalla tiene que saber decir.
var Estados = []EstadoSistema{
	SistemaComprobando, SistemaLista, SistemaDescargable, SistemaDescargando,
	SistemaNoSoportada, SistemaApagada, SistemaAntiguo, SistemaSinPuente, SistemaFallo,
}

// ErrorSistema es [ErrSinIANativa] con el estado que lo explica: la pantalla lo saca con
// errors.As y lo dice en su idioma. Su texto, en español, es el de los registros.
type ErrorSistema struct{ Estado EstadoSistema }

func (e *ErrorSistema) Error() string { return ErrSinIANativa.Error() + ": " + e.Estado.motivo() }

// Unwrap es ErrSinIANativa.
func (e *ErrorSistema) Unwrap() error { return ErrSinIANativa }

// motivo es el texto en español del error y de los registros.
func (e EstadoSistema) motivo() string {
	switch e {
	case SistemaLista:
		return "lista"
	case SistemaDescargable:
		return "el modelo se puede descargar; se ha pedido al sistema que lo haga"
	case SistemaDescargando:
		return "el modelo se está descargando o preparando; prueba dentro de un rato"
	case SistemaNoSoportada:
		return "este dispositivo no tiene el modelo del sistema"
	case SistemaApagada:
		return "activa Apple Intelligence en Ajustes del sistema"
	case SistemaAntiguo:
		return "hace falta una versión más reciente del sistema"
	case SistemaSinPuente:
		return "esta compilación de la app no lleva el puente con el modelo del sistema"
	case SistemaFallo:
		return "el sistema no ha dicho si tiene modelo"
	default: // SistemaComprobando
		return "comprobando"
	}
}

// puenteNativo es la IA del sistema de un móvil. Las funciones las pone cada sistema;
// consultar y responder BLOQUEAN (cada puente con su plazo) y se llaman fuera de la
// interfaz.
type puenteNativo struct {
	nombre    string
	consultar func() EstadoSistema
	responder func(id int64, system, user string) (string, error)
	cancelar  func(id int64) // puede ser nil: la respuesta se abandona
	preparar  func()         // puede ser nil: pedir que se descargue el modelo
	// vigencia es cuánto vale lo consultado; 0, un minuto.
	vigencia time.Duration

	mu          sync.Mutex
	valor       EstadoSistema
	cuando      time.Time
	consultando bool
	ids         atomic.Int64
}

func (p *puenteNativo) vigente() bool {
	v := p.vigencia
	if v == 0 {
		v = time.Minute
	}
	return p.valor != SistemaComprobando && time.Since(p.cuando) < v
}

// estadoAhora pregunta al sistema si lo último que dijo ya no vale, y espera.
func (p *puenteNativo) estadoAhora() EstadoSistema {
	p.mu.Lock()
	if p.vigente() && p.valor == SistemaLista {
		v := p.valor
		p.mu.Unlock()
		return v
	}
	p.mu.Unlock()
	e := p.consultar()
	p.mu.Lock()
	p.valor, p.cuando = e, time.Now()
	p.mu.Unlock()
	return e
}

// estadoSinEsperar devuelve lo último que se sabe y, si es viejo, vuelve a preguntar en
// otra rutina. Es lo de Ajustes, que pinta en el hilo de la interfaz y no puede esperar
// a que el sistema conteste.
func (p *puenteNativo) estadoSinEsperar() EstadoSistema {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.vigente() && !p.consultando {
		p.consultando = true
		go func() {
			e := p.consultar()
			p.mu.Lock()
			p.valor, p.cuando, p.consultando = e, time.Now(), false
			p.mu.Unlock()
		}()
	}
	return p.valor
}

// completar pide la respuesta. Sin modelo, ErrSinIANativa con el motivo; con el
// contexto cancelado, deja de esperar y se lo dice al puente.
func (p *puenteNativo) completar(ctx context.Context, system, user string) (string, error) {
	switch e := p.estadoAhora(); e {
	case SistemaLista:
	case SistemaDescargable:
		if p.preparar != nil {
			go p.preparar()
		}
		return "", &ErrorSistema{e}
	default:
		return "", &ErrorSistema{e}
	}
	type resultado struct {
		texto string
		err   error
	}
	id := p.ids.Add(1)
	listo := make(chan resultado, 1)
	go func() {
		t, err := p.responder(id, system, user)
		listo <- resultado{t, err}
	}()
	select {
	case r := <-listo:
		if r.err != nil {
			// El modelo pudo dejar de estar (Apple Intelligence apagada a medias): que
			// la próxima vez se vuelva a preguntar.
			p.mu.Lock()
			p.valor = SistemaComprobando
			p.mu.Unlock()
			return "", fmt.Errorf("asistente: %s: %w", p.nombre, r.err)
		}
		t := strings.TrimSpace(r.texto)
		if t == "" {
			return "", fmt.Errorf("asistente: %s no devolvió texto", p.nombre)
		}
		return t, nil
	case <-ctx.Done():
		if p.cancelar != nil {
			p.cancelar(id)
		}
		return "", ctx.Err()
	}
}
