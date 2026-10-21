// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"net/url"
	"strings"
	"sync"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
)

// VistaWidget binds a pending data request to this app and its adopted session.
// It contains no token, identity, network client or configurable destination.
type VistaWidget = widgets.View

// AlBuscar recibe la acción transversal, también si llegó al arrancar antes de
// montar la barra. No cuenta como un receptor de rutas propias del producto.
func (s *Suite) AlBuscar(f func()) {
	s.mu.Lock()
	s.alBuscar = f
	pendiente := s.busquedaPendiente && f != nil
	if pendiente {
		s.busquedaPendiente = false
	}
	s.mu.Unlock()
	if pendiente {
		fyne.Do(f)
	}
}

func (s *Suite) recibirBusqueda(u *url.URL) bool {
	if !s.EsBusqueda(u) {
		return false
	}
	s.mu.Lock()
	f := s.alBuscar
	if f == nil {
		s.busquedaPendiente = true
	}
	s.mu.Unlock()
	if f != nil {
		fyne.Do(f)
	}
	return true
}

// EsBusqueda reconoce sólo la acción local de esta app. No admite texto ni
// destinos del exterior: el widget abre el campo y la persona escribe después.
func (s *Suite) EsBusqueda(u *url.URL) bool {
	if s.app == nil || u == nil {
		return false
	}
	esquema := s.app.Metadata().Custom["esquema"]
	if !api.EsProducto(strings.TrimPrefix(esquema, "sytue-")) || !strings.HasPrefix(esquema, "sytue-") ||
		u.Scheme != esquema || u.Host != "" || u.User != nil || u.Opaque != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return false
	}
	q, err := url.ParseQuery(u.RawQuery)
	return err == nil && len(q) == 1 && len(q["action"]) == 1 && q.Get("action") == "search"
}

// CapturarWidget must be called BEFORE requesting the data whose counters will
// be published. A response from an older account cannot adopt a newer marker.
func (s *Suite) CapturarWidget() VistaWidget {
	state := s.Estado()
	if !state.Dentro() || s.app == nil {
		return VistaWidget{}
	}
	metadata := s.app.Metadata()
	return widgets.NewView(metadata.ID, strings.TrimPrefix(metadata.Custom["esquema"], "sytue-"), widgets.Capture(state.GeneracionSesion))
}

func (s *Suite) iniciarWidgets(ctx context.Context) {
	// Sin widget nativo que lea (Windows, Linux, Apple sin WidgetKit) no hay nada que
	// avisar ni borrar en cada cambio de sesión.
	if s.app == nil || s.app.Metadata().ID == "" || !widgets.Disponible() {
		return
	}
	var mu sync.Mutex
	var generation, subject string
	var locked, initialized bool
	update := func(state Estado) {
		blocked := ajustes.Leer().Bloqueo
		mu.Lock()
		changed := !initialized || generation != state.GeneracionSesion || subject != state.Sujeto || locked != blocked
		generation, subject, locked, initialized = state.GeneracionSesion, state.Sujeto, blocked, true
		mu.Unlock()
		if !changed {
			return
		}
		go func() {
			if !state.Dentro() || blocked {
				// The provider checks lock/generation even when deleting fails.
				_ = widgets.ClearIf(ctx, state.GeneracionSesion)
			} else {
				widgets.Notify("")
			}
		}()
	}
	s.AlCambiar(update)
	s.AlVolver(func() { update(s.Estado()) })
	update(s.Estado())
}
