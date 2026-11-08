// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// VistaWidget binds a pending data request to this app and its adopted identity.
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

// capturarWidget es la Vista de los publicadores: la sesión y la ÉPOCA adoptadas por
// esta app, nunca una lectura nueva del disco. Se llama al empezar cada ronda.
func (s *Suite) capturarWidget() VistaWidget {
	state := s.Estado()
	if !state.Dentro() || s.app == nil {
		return VistaWidget{}
	}
	return widgets.NewView(s.app.Metadata().ID, s.productoWidget(), widgets.Capture(state.EpocaSesion))
}

func (s *Suite) productoWidget() string {
	return strings.TrimPrefix(s.app.Metadata().Custom["esquema"], "sytue-")
}

// widgetsSuite es el publicador del widget de esta app y de qué avisos del Hub se
// entera. Una app tiene uno; sin lector nativo, ninguno.
type widgetsSuite struct {
	mu     sync.Mutex
	pub    *widgets.Publicador
	avisos []string
	prueba *widgets.OpcionesPublicador
}

// PublicadorWidget crea el publicador del widget de esta app con su Fuente y lo
// engancha a los disparadores comunes (ADR 0078, enmienda del 2026-11-06): entrar y
// salir, el bloqueo, la renovación, volver a primer plano, recuperar la red, las
// escrituras de los clientes de ClienteConAviso, los avisos del Hub de su producto (y
// de `otrosProductos`: One, de los de fuentes.ProductosDeHub), AvisarWidget, el
// latido y la medianoche. Devuelve nil sin lector (Windows, Linux, Apple sin WidgetKit).
// Se llama antes de Arrancar.
func (s *Suite) PublicadorWidget(f widgets.Fuente, otrosProductos ...string) *widgets.Publicador {
	if f == nil || s.app == nil || s.app.Metadata().ID == "" || !widgets.Disponible() {
		return nil
	}
	s.widgets.mu.Lock()
	defer s.widgets.mu.Unlock()
	o := widgets.OpcionesPublicador{}
	if s.widgets.prueba != nil {
		o = *s.widgets.prueba
	}
	o.Vista, o.Fuente = s.capturarWidget, f
	s.widgets.pub = widgets.NuevoPublicador(o)
	s.widgets.avisos = append([]string{s.productoWidget()}, otrosProductos...)
	return s.widgets.pub
}

// FuenteWidget es la Fuente del widget de esta app (fuentes.Registrada de su producto),
// con los Comunes de su traductor y la sesión y el apex de la suite: la misma que
// construye el refresco sin ventana de Android, que no corre el `main` (fondo_sinventana.go).
// nil para una app sin widget (la demo).
func (s *Suite) FuenteWidget(t *i18n.Translator) widgets.Fuente {
	if s.app == nil {
		return nil
	}
	fabrica, ok := fuentes.Registrada(s.productoWidget())
	if !ok {
		return nil
	}
	return fabrica(s.ComunesWidget(t), s.URL, s.Sesion)
}

// ProbarWidgetsCon es SOLO para tests: el reloj, el Programar y los plazos de los
// publicadores que cree esta suite después.
func (s *Suite) ProbarWidgetsCon(o widgets.OpcionesPublicador) {
	s.widgets.mu.Lock()
	defer s.widgets.mu.Unlock()
	s.widgets.prueba = &o
}

func (s *Suite) publicadorWidget() *widgets.Publicador {
	s.widgets.mu.Lock()
	defer s.widgets.mu.Unlock()
	return s.widgets.pub
}

// AvisarWidget dice que algo de esta app ha cambiado por una vía que no es una
// escritura propia: el vivo de Mail y de Rooms (SSE, WebSocket). Es seguro llamarlo
// sin publicador.
func (s *Suite) AvisarWidget() {
	if p := s.publicadorWidget(); p != nil {
		p.Avisar()
	}
}

// AvisoDelHub es la barra diciendo que ha llegado un aviso del Hub de `producto`, o
// su señal de cambios (`datos`: algo de ese producto cambió en el servidor).
//
// Del producto propio va con AvisarDelServidor: la señal de una escritura de esta
// misma app vuelve como eco y no debe saltarse el mínimo de AvisarEscritura.
func (s *Suite) AvisoDelHub(producto string) {
	s.relevar(producto) // en Android, One se lo pasa a la app cerrada (relevo.go)
	s.widgets.mu.Lock()
	p, suyo := s.widgets.pub, slices.Contains(s.widgets.avisos, producto)
	propio := suyo && len(s.widgets.avisos) > 0 && s.widgets.avisos[0] == producto
	s.widgets.mu.Unlock()
	switch {
	case p == nil || !suyo:
	case propio:
		p.AvisarDelServidor()
	default:
		p.Avisar()
	}
}

func (s *Suite) avisarEscrituraWidget() {
	if p := s.publicadorWidget(); p != nil {
		p.AvisarEscritura()
	}
}

// ClienteConAviso envuelve el cliente HTTP de los clientes `api` del PROPIO producto
// (nil: el de siempre): cada escritura con éxito avisa al widget, con el intervalo
// mínimo de AvisarEscritura. Los clientes de presencia, avisos, preferencias y de
// otros productos no se envuelven.
func (s *Suite) ClienteConAviso(base *http.Client) *http.Client {
	return api.ConAvisoDeEscritura(base, s.avisarEscrituraWidget)
}

// ComunesWidget es lo que las Fuentes necesitan de la app: su traductor (el de sus
// pantallas: «Sin título» se dice igual en el widget), la zona y la hora locales y el
// idioma de la suite.
func (s *Suite) ComunesWidget(t *i18n.Translator) fuentes.Comunes {
	var traducir func(string) string
	if t != nil {
		traducir = func(clave string) string { return t.T(clave, nil) }
	}
	return fuentes.Comunes{
		T:      traducir,
		Zona:   func() *time.Location { return time.Local },
		Ahora:  time.Now,
		Idioma: s.Idioma,
	}
}

func (s *Suite) iniciarWidgets(ctx context.Context) {
	// Sin widget nativo que lea (Windows, Linux, Apple sin WidgetKit) no hay nada que
	// avisar ni borrar en cada cambio de sesión.
	if s.app == nil || s.app.Metadata().ID == "" || !widgets.Disponible() {
		return
	}
	var mu sync.Mutex
	var generation, epoch, subject string
	var locked, initialized bool
	update := func(state Estado) {
		blocked := ajustes.Leer().Bloqueo
		mu.Lock()
		primero := !initialized
		otroQuien := !initialized || epoch != state.EpocaSesion || subject != state.Sujeto || locked != blocked
		renovada := !otroQuien && generation != state.GeneracionSesion
		generation, epoch, subject, locked, initialized = state.GeneracionSesion, state.EpocaSesion, state.Sujeto, blocked, true
		mu.Unlock()
		pub := s.publicadorWidget()
		switch {
		case otroQuien && (!state.Dentro() || blocked):
			if pub != nil {
				pub.Pausar()
			}
			// Fuera porque Abrir no pudo leer la sesión (iOS la lanza en frío para el
			// refresco con el teléfono bloqueado) no es saber que no hay nadie: lo
			// publicado se queda, con su antigüedad, y los lectores siguen comprobando la
			// época y el bloqueo. Lo borra quien sí lo sabe: salir, el bloqueo, otra
			// identidad.
			if primero && s.ilegibleAlAbrir && !blocked {
				return
			}
			// The provider checks lock/generation even when deleting fails.
			go func() { _ = widgets.ClearIf(ctx, state.EpocaSesion) }()
		case otroQuien:
			go widgets.Notify("")
			if pub != nil {
				pub.Reanudar()
			}
		case renovada && pub != nil:
			// La misma persona con otra marca: se reescribe lo último con la marca nueva,
			// sin red si nada ha cambiado desde entonces.
			pub.Republicar()
		}
	}
	// También sin escena: lanzada por iOS solo para refrescar el widget, la app sigue
	// a la sesión para pausar, borrar o reanudar (fondo.go).
	s.alCambiarSinEscena(update)
	s.AlVolver(func() {
		update(s.Estado())
		s.AvisarWidget()
	})
	// Al salir a la pantalla de inicio es cuando se mira el widget, y iOS suspende la
	// app segundos después: lo que el antirrebote o la ventana de escrituras tuvieran
	// aplazado se publica ya, con un plazo que cabe en esos segundos.
	s.AlFondo(func() {
		pub := s.publicadorWidget()
		if pub == nil {
			return
		}
		go func() {
			c, parar := context.WithTimeout(ctx, widgets.PlazoAlFondo)
			defer parar()
			_ = pub.PublicarPendiente(c)
		}()
	})
	update(s.Estado())
}
