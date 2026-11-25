// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// # Lo que vive en el repositorio privado
//
// Drive (y con él CertiPDF, que guarda en su carpeta «PDF») guarda los datos de la cuenta
// en el repositorio privado del dispositivo: la bóveda directa o Matrix (ADR 0086). La
// fuente del servidor de esa cuenta está congelada desde que pasó allí, así que una
// Fuente que la lea enseña lo de antes de pasar y nunca lo nuevo: era el widget de Drive
// «que no cambia». La Fuente de esos productos lee el repositorio que la app tiene ABIERTO
// (Privada.Montar, al montar la vista) y, sin él —un arranque en segundo plano de iOS o el
// refresco sin ventana de Android, que no tienen la clave—, falla con ErrSinDatosPrivados:
// el Publicador conserva lo último que se publicó desde el repositorio, con su «Actualizado
// hace X», y nunca enseña lo congelado del servidor (ADR 0078, enmienda del 2026-11-24).

// ErrSinDatosPrivados: los datos del widget están en el repositorio privado y la app no lo
// tiene abierto. No es un rechazo de la sesión: lo publicado se queda.
var ErrSinDatosPrivados = errors.New("fuentes: el repositorio privado del widget no está abierto")

// sinDatosPrivados es la Fuente registrada de un producto privado: la del refresco sin
// ventana, que no puede abrir el repositorio.
func sinDatosPrivados() widgets.Fuente {
	return func(context.Context) (widgets.Detail, error) { return widgets.Detail{}, ErrSinDatosPrivados }
}

// Privada es la Fuente de un producto cuyos datos viven en el repositorio privado: lee del
// que la app tenga montado y, sin ninguno, de su respaldo (Respaldar) o, sin respaldo,
// falla con ErrSinDatosPrivados. Es segura entre rutinas; el valor cero vale.
type Privada struct {
	mu       sync.Mutex
	actual   widgets.Fuente
	respaldo widgets.Fuente
	turno    uint64
}

// Fuente es la que se le da al Publicador (sesion.Suite.PublicadorWidget). Lo que lee
// del repositorio montado sale marcado SoloConLaApp: ya rancio, el lector dice que se
// actualiza con la app abierta.
func (p *Privada) Fuente() widgets.Fuente {
	return func(ctx context.Context) (widgets.Detail, error) {
		p.mu.Lock()
		f, montada := p.actual, p.actual != nil
		if f == nil {
			f = p.respaldo
		}
		p.mu.Unlock()
		if f == nil {
			return widgets.Detail{}, ErrSinDatosPrivados
		}
		d, err := f(ctx)
		if err == nil && montada {
			d.SoloConLaApp = true
		}
		return d, err
	}
}

// Respaldar pone la Fuente que se lee mientras no haya repositorio montado: la registrada
// del producto (Registrada). En Drive y CertiPDF es la que falla con ErrSinDatosPrivados;
// en una app de registros, la que lee el servidor solo si la fuente de la cuenta sigue
// `legacy` (registros.go).
func (p *Privada) Respaldar(f widgets.Fuente) {
	p.mu.Lock()
	p.respaldo = f
	p.mu.Unlock()
}

// Montar pone la Fuente sobre el repositorio que la app acaba de abrir y devuelve cómo
// quitarla cuando lo cierre (otra cuenta, cerrar sesión, el bloqueo). Quitar una que ya
// sustituyó otra más nueva no quita la nueva.
func (p *Privada) Montar(f widgets.Fuente) (soltar func()) {
	p.mu.Lock()
	p.turno++
	turno := p.turno
	p.actual = f
	p.mu.Unlock()
	return func() {
		p.mu.Lock()
		if p.turno == turno {
			p.actual = nil
		}
		p.mu.Unlock()
	}
}
