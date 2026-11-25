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

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets"
	"git.sytue.eu/sytue/one/apps/nucleo/widgets/fuentes"
)

// VistaWidget binds a pending data request to this app and its adopted identity.
// It contains no token, identity, network client or configurable destination.
type VistaWidget = widgets.View

// esBusquedaAntigua reconoce la acción «Buscar» de los widgets de búsqueda global,
// retirados (ADR 0078, 2026-11-24: no aportaban). Uno que siga en una pantalla de
// inicio hasta que el sistema lo quite aún la puede mandar: abre la app y nada más,
// sin llegar a los receptores de rutas del producto.
func (s *Suite) esBusquedaAntigua(u *url.URL) bool {
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
	// privada es la Fuente de los productos que leen el repositorio privado que la app
	// tenga abierto (WidgetDesdeDrivePrivado, WidgetDesdeRegistrosPrivados); comunes, los
	// de su FuenteWidget.
	privada fuentes.Privada
	comunes fuentes.Comunes
}

// esWidgetPrivado: los productos cuyos datos viven, o pueden vivir, en el repositorio
// privado del dispositivo: el Drive en la bóveda o en Matrix (Drive y CertiPDF) y las
// apps de registros con la cuenta en Matrix. Con ellos ahí, su fuente del servidor está
// congelada.
func esWidgetPrivado(producto string) bool {
	return producto == "drive" || producto == "pdf" || fuentes.DeRegistros(producto)
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
	comunes := s.ComunesWidget(t)
	if esWidgetPrivado(s.productoWidget()) {
		// Lo que la app monte al abrir su repositorio privado y, sin él, la registrada: en
		// Drive y CertiPDF falla con fuentes.ErrSinDatosPrivados (se conserva lo publicado);
		// en una app de registros lee el servidor solo si la fuente de la cuenta sigue
		// `legacy`.
		s.widgets.mu.Lock()
		s.widgets.comunes = comunes
		s.widgets.mu.Unlock()
		s.widgets.privada.Respaldar(fabrica(comunes, s.URL, s.Sesion))
		return s.widgets.privada.Fuente()
	}
	return fabrica(comunes, s.URL, s.Sesion)
}

// WidgetDesdeDrivePrivado hace que el widget de Drive lea, mientras dure `vida`, el
// Drive privado que la app acaba de abrir (`datos`, api.Drive.WithPrivateData) y la cuota
// de la cuenta con `cuenta`, su cliente del servidor; y lo publica ya. Sin ello, la
// fuente del servidor de esa cuenta está congelada y lo nuevo no salía nunca (ADR 0078,
// enmienda del 2026-11-24).
func (s *Suite) WidgetDesdeDrivePrivado(vida context.Context, datos, cuenta *api.Drive) {
	s.montarWidgetPrivado(vida, func(c fuentes.Comunes) widgets.Fuente { return fuentes.Drive(c, datos, cuenta) })
}

// WidgetDesdePDFPrivado es WidgetDesdeDrivePrivado de CertiPDF: los PDF de su carpeta en
// el Drive privado (`archivos`, api.PDF.WithPrivateDrive) y las peticiones de firma del
// servicio (`firmas`).
func (s *Suite) WidgetDesdePDFPrivado(vida context.Context, archivos, firmas *api.PDF) {
	s.montarWidgetPrivado(vida, func(c fuentes.Comunes) widgets.Fuente { return fuentes.CertiPDF(c, archivos, firmas) })
}

// WidgetDesdeRegistrosPrivados es WidgetDesdeDrivePrivado de una app de registros
// (Calendar, Contacts, Notes, Photos, Bases, ERP, Cast y la ofimática) con la cuenta en
// Matrix: mientras dure `vida`, su widget lee `datos`, la fachada que la vista acaba de
// montar sobre el repositorio privado (el *api.Calendars, *api.Notes… de WithPrivateData),
// y lo que no es dato privado (la cuota de Cast) con `cuenta`, la sesión de esa cuenta.
// Sin ello leía la fuente del servidor, congelada desde que la cuenta pasó a Matrix
// (ADR 0078, enmienda del 2026-11-24). Una fachada que no es la de su producto no monta
// nada.
func (s *Suite) WidgetDesdeRegistrosPrivados(vida context.Context, cuenta api.Session, datos any) {
	if s.app == nil {
		return
	}
	producto := s.productoWidget()
	s.montarWidgetPrivado(vida, func(c fuentes.Comunes) widgets.Fuente {
		f, _ := fuentes.Privados(producto, c, datos, s.URL, cuenta)
		return f
	})
}

func (s *Suite) montarWidgetPrivado(vida context.Context, f func(fuentes.Comunes) widgets.Fuente) {
	if vida == nil || vida.Err() != nil || s.app == nil || !esWidgetPrivado(s.productoWidget()) {
		return
	}
	s.widgets.mu.Lock()
	comunes := s.widgets.comunes
	s.widgets.mu.Unlock()
	fuente := f(comunes)
	if fuente == nil {
		return
	}
	context.AfterFunc(vida, s.widgets.privada.Montar(fuente))
	s.AvisarWidget()
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

// AvisarEscritura dice que esta app acaba de escribir por una vía que no es HTTP: el
// Drive privado de Drive y de CertiPDF, en la bóveda o en Matrix (api.Drive.ConAviso,
// api.PDF.ConAviso). Cuenta como una escritura de ClienteConAviso, con su intervalo
// mínimo. Es seguro llamarlo sin publicador.
func (s *Suite) AvisarEscritura() { s.avisarEscrituraWidget() }

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
