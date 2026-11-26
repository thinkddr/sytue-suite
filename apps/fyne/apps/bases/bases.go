// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package bases es Sytue Bases nativa: las tablas relacionales colaborativas del
// ADR 0068, con las mismas pantallas que la web (`apps/frontend/apps/bases`).
//
// Es ESTADO y no vista, como el ERP: la pantalla cambia sus campos y llama a Repintar,
// que vuelve a armar el árbol con las piezas de `ui`. El permiso lo decide siempre el
// servicio; el rol que llega solo decide qué se PINTA.
package bases

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Traductor es el catálogo de Bases sobre el común de la suite.
func Traductor(l i18n.Locale) (*i18n.Translator, error) {
	return i18n.New(l, i18n.Common(), catalogos.Bases())
}

// Pantalla es Sytue Bases entera: el carril, «Mis bases» y la base abierta.
type Pantalla struct {
	// conPie: los totales de la vista están encendidos (resumen.go). Es de la persona y
	// dura lo que la ventana, como el resto de lo que se elige al mirar.
	conPie  bool
	tr      *i18n.Translator
	svc     *api.Bases
	ventana fyne.Window

	// «Mis bases» (lista.go).
	Lista    []api.BasesBase
	Cargando bool
	Fallo    string
	creando  bool
	// El campo de «Crear base» es el MISMO objeto entre repintados: lo escrito y el foco
	// no pueden perderse porque llegue la lista. El botón es el del último repintado.
	nombre *ui.Entrada
	crear  *ui.Boton

	// buscar es el campo de búsqueda de la lista, el MISMO entre repintados.
	buscar *ui.Entrada

	// abierta es la base que se está mirando; nil, la lista.
	abierta *Base

	// Org es la organización de la sesión: el principal de «toda la organización» al
	// compartir. Nil, se manda vacío y decide el servicio.
	Org func() string
	// Yo es el `sub` de quien mira: en la actividad, sus entradas son «Tú». Nil, nadie
	// lo es.
	Yo func() string
	// URLPublica compone la dirección web de una ruta pública de Bases (`/f/<token>`, el
	// formulario): lleva el host y el prefijo, y nunca la organización.
	URLPublica func(ruta string) string
	// Guardar deja un fichero donde el sistema guarda las descargas y devuelve su ruta.
	// Nil: no se ofrece exportar.
	Guardar func(nombre string, contenido io.Reader) (string, error)
	// Elegir abre el selector de ficheros del sistema y pasa lo elegido a `listo`, en el
	// hilo de la interfaz (cancelar no llama a nada). Nil: no se ofrece subir adjuntos
	// ni importar un CSV con el selector; soltarlos sobre la ventana sigue valiendo.
	Elegir func(listo func(rutas []string))
	// ficha es la ficha de registro abierta, si la hay: lo que se suelta va a ella.
	ficha *fichaRegistro

	// Plazo es cuánto se espera a una petición.
	Plazo time.Duration
	raiz  *fyne.Container
	// scroll es el desplazador del contenido. Es el MISMO entre repintados de una misma
	// pantalla: repintar tras guardar una celda no puede devolver la rejilla arriba.
	scroll  *container.Scroll
	enLista bool
	// tam es el tamaño de la ventana: de él sale dónde parten las filas de botones.
	tam fyne.Size
	// fondo lanza lo que habla con la red. Es `go f()`; los guardianes lo cambian por
	// `f()` para que la carga ocurra en su rutina y en orden.
	fondo func(func())

	// Tiempo real (tiemporeal.go). conn es esta ventana ante el servicio: la presencia y
	// el flujo la llevan, y con ella se descuenta a sí misma de quienes están. vivo lanza
	// el flujo y el latido, que no vuelven hasta cerrar la base: es `go f()` también en
	// los guardianes, que no pueden quedarse dentro. cadaLatido y esperaRecarga los
	// acortan las pruebas.
	conn          string
	vivo          func(func())
	cadaLatido    time.Duration
	esperaRecarga time.Duration
	// comentando es el panel de comentarios abierto, si lo hay: un comentario de otra
	// persona en ese registro lo vuelve a cargar.
	comentando *panelComentarios

	// SinServidor es la edición local (Sytue Office): una persona y ningún servicio
	// detrás. No se ofrece compartir ni la actividad, y no se abre el tiempo real, que
	// sin servidor solo diría «se ha perdido la conexión». Falso, como siempre.
	SinServidor bool
	// SinFormularios no ofrece la vista de formulario (sus enlaces públicos piden un
	// servicio que los reciba): ni al crear una vista ni, en una que ya lo sea, el panel
	// de enlaces. Falso, como siempre (B-05).
	SinFormularios bool
	// ActividadLocal ofrece la actividad de la base aunque no haya servidor: la que
	// anota el propio dispositivo (B-06). Solo cuenta con SinServidor.
	ActividadLocal bool
}

// NuevaPantalla monta la aplicación sobre su ventana. `svc` nil la deja pintarse sin red.
func NuevaPantalla(tr *i18n.Translator, svc *api.Bases, w fyne.Window) *Pantalla {
	p := &Pantalla{
		tr: tr, svc: svc, ventana: w,
		Cargando: true,
		Plazo:    30 * time.Second,
		fondo:    func(f func()) { go f() },
		vivo:     func(f func()) { go f() },
		conn:     idAleatorio(),

		cadaLatido:    cadaLatido,
		esperaRecarga: esperaRecarga,
		tam:           fyne.NewSize(1280, 800),
	}
	p.nombre = ui.NuevaEntrada("")
	p.nombre.Rotulo = p.t("bases.newName")
	p.nombre.AlCambiar(func(string) { p.ajustarCrear() })
	// La búsqueda filtra la lista que ya está entera: repintar basta.
	p.buscar = ui.NuevaEntrada(p.t("bases.search"))
	p.buscar.Rotulo, p.buscar.Icono, p.buscar.Busqueda = p.t("bases.search"), iconos.Buscar, true
	p.buscar.AlCambiar(func(string) { p.Repintar() })
	p.nombre.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name != fyne.KeyReturn && k.Name != fyne.KeyEnter {
			return false
		}
		p.crearBase()
		return true
	}
	return p
}

func (p *Pantalla) t(clave string, vars ...i18n.Vars) string {
	var v i18n.Vars
	if len(vars) > 0 {
		v = vars[0]
	}
	return p.tr.T(clave, v)
}

func (p *Pantalla) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.Plazo)
}

// enHilo ejecuta algo en la rutina de la interfaz (obligatorio desde Fyne 2.6).
func enHilo(f func()) { fyne.Do(f) }

// Arrancar pide la lista de bases. Se llama cuando ya hay sesión.
func (p *Pantalla) Arrancar() {
	if p.ventana != nil {
		ui.AlAtras(p.ventana.Canvas(), p.atras)
		p.engancharTeclas(p.ventana.Canvas())
	}
	p.cargarLista()
}

// Objeto es el armazón con el carril y la pantalla que toque.
func (p *Pantalla) Objeto() fyne.CanvasObject {
	p.raiz = container.New(&vigia{p: p}, p.marco())
	return p.raiz
}

// Repintar vuelve a armar el contenido con el estado de ahora. El armazón y el
// desplazador solo se rehacen al pasar de la lista a una base o al revés.
func (p *Pantalla) Repintar() {
	if p.raiz == nil {
		return
	}
	if p.scroll == nil || p.enLista != (p.abierta == nil) {
		p.raiz.Objects[0] = p.marco()
		p.raiz.Refresh()
		return
	}
	p.scroll.Content = p.contenido()
	p.scroll.Refresh()
}

func (p *Pantalla) contenido() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if p.abierta != nil {
		return p.objetoBase(v)
	}
	return p.objetoLista(v)
}

// anchoContenido es el ancho que le queda a una pantalla dentro de su `p-6`.
func (p *Pantalla) anchoContenido() float32 {
	ancho := ui.AnchoHoja(p.tam.Width, ui.AnchoMenu(false), 0) - 2*tema.Espacio(6)
	return max(ancho, 0)
}

// marco es Marco.tsx: el carril con «Mis bases» y el contenido, que se desplaza.
func (p *Pantalla) marco() fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p.enLista = p.abierta == nil
	p.scroll = container.NewVScroll(p.contenido())
	refresco := ui.RefrescarAlTirar(p.scroll, &ui.RefrescoLista{
		Arriba: func() bool { return p.scroll.Offset.Y <= 0.5 },
		Puede: func() bool {
			return p.svc != nil && p.abierta == nil && !p.Cargando && !p.creando && p.nombre.Texto() == ""
		},
		Actualizar: p.cargarLista,
	})
	arm := ui.NuevoArmazon(refresco)
	arm.Titulo = p.t("nav.title")
	tinta := "sidebar-fg-muted"
	if p.enLista {
		tinta = "sidebar-fg"
	}
	fila := ui.NuevaFilaLateral(p.t("nav.bases"), iconos.Trazo(iconos.Tabla, ui.GlifoMenu, tinta, v).Objeto(), p.volverALista)
	fila.Activa = p.enLista
	arm.Menu = ui.NuevoLateral(p.t("ui.mainNav"), ui.NuevaSeccionLateral(p.t("nav.title"), fila))
	return arm
}

// vigia ocupa la ventana entera con sus hijos y repinta cuando cambia de ancho: las
// filas de botones parten (`flex-wrap`) según lo que quepa.
type vigia struct{ p *Pantalla }

func (v *vigia) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

func (v *vigia) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
	if t.Width > 0 && t.Width != v.p.tam.Width {
		v.p.tam = t
		fyne.Do(v.p.Repintar)
	}
	v.p.tam = t
}

// volverALista es el «Todas las bases» de la web: deja la base y vuelve a pedir la lista.
func (p *Pantalla) volverALista() {
	p.dejarDeSeguir(p.abierta)
	p.abierta = nil
	p.cargarLista()
}

// mensajeDeError es `mensajeDeError` de lib/api.ts: el mensaje traducido de un fallo de
// la API, nunca el texto crudo del servidor.
func (p *Pantalla) mensajeDeError(err error) string {
	var e *api.Error
	if errors.As(err, &e) {
		switch e.Status {
		case http.StatusUnauthorized:
			return p.t("err.session")
		case http.StatusForbidden:
			return p.t("err.forbidden")
		case http.StatusNotFound:
			return p.t("err.notFound")
		}
		switch e.Code() {
		case "rev_conflict":
			return p.t("err.conflict")
		case "not_a_member":
			return p.t("err.notMember")
		case "is_member":
			return p.t("err.isMember")
		case "referenced":
			return p.t("err.referenced")
		case "limit_reached":
			return p.t("err.limit")
		}
		if e.Status == http.StatusUnprocessableEntity || e.Status == http.StatusBadRequest {
			return p.t("err.invalid")
		}
	}
	return p.t("err.generic")
}

// puedeEsquema dice si el rol deja cambiar el esquema y la propia base: solo el
// propietario. Es para pintar (lib/valores.ts); autoriza el servicio.
func puedeEsquema(rol string) bool { return rol == api.BasesOwner }

// alerta es el `<p role="alert" class="text-danger-text text-sm">` de las dos pantallas.
func alerta(texto string, v tema.Variante) fyne.CanvasObject {
	return ui.NuevoParrafo(texto, tema.TextoSM, ui.Normal, "danger-text", v)
}

// acotado es `mx-auto max-w-* p-6`: un hijo a todo el ancho hasta Max, centrado y con su
// margen. Max cero no acota.
type acotado struct{ Max, Margen float32 }

func (a acotado) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*a.Margen, m.Height+2*a.Margen)
}

func (a acotado) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ancho := t.Width - 2*a.Margen
	if a.Max > 0 {
		ancho = min(ancho, a.Max)
	}
	ancho = max(ancho, 0)
	objs[0].Move(fyne.NewPos((t.Width-ancho)/2, a.Margen))
	objs[0].Resize(fyne.NewSize(ancho, max(objs[0].MinSize().Height, t.Height-2*a.Margen)))
}

// extremos es `flex items-end gap-*` con el PRIMER hijo en `flex-1`: él se lleva lo que
// sobra y los demás van a su ancho, todos apoyados abajo. Con Centro, `items-center`.
type extremos struct {
	Hueco  float32
	Centro bool
}

func (e extremos) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		m := o.MinSize()
		if i > 0 {
			ancho += e.Hueco + m.Width
		}
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

func (e extremos) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	x := t.Width
	for i := len(objs) - 1; i >= 0; i-- {
		m := objs[i].MinSize()
		ancho := m.Width
		if i == 0 {
			ancho = max(x, 0)
		}
		y := t.Height - m.Height
		if e.Centro {
			y /= 2
		}
		objs[i].Move(fyne.NewPos(x-ancho, y))
		objs[i].Resize(fyne.NewSize(ancho, m.Height))
		x -= ancho + e.Hueco
	}
}
