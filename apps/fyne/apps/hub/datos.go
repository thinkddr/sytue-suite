// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// paridad-web: apps/frontend/apps/portal/src/pages/datos.astro

package hub

import (
	"context"
	"io"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/ficheros"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// «Tus datos»: `TusDatos.tsx` dentro de `datos.astro`. Descargar una copia de TODO lo
// de la persona (RGPD art. 20): una decisión —el formato de las listas— y un botón.
//
// Medido en producción a 1280×800 (`medidas/portal/datos.ancho.json`), con la topbar ya
// descontada (−56):
//
//	título «Tus datos»          x = 213, y = 21     24,5/31,5 · 700
//	entrada                     y = 57,75           12,25/19,91 · ancho 588
//	«Descargar una copia»       y = 98,66           854 × 473,5, sombra `shadow-sm`
//	  cuerpo                    y = 150,41          11,38/18,48 · tres líneas en 588
//	  «Formato de las listas»   y = 219,86          11,38/15,75 · 600
//	  ayuda                     y = 239,11          10,5/14
//	  tres opciones             y = 263,61          588 × 59,64 con `gap-2`
//	  «Descargar copia»         y = 470,53          119 × 38,5
//	  alcance                   y = 523,03          10,5/17,06 · dos líneas
//	«Llevarlo a otro servicio»  y = 593,16          854 × 129,97
//	  «Ir a Migraciones»        y = 692,38          82 × 15,75 · 600 · primary-text
//
// Lo que NO porta, a propósito o por falta de pieza:
//
//   - Con varias organizaciones se elige de cuál es la copia, y elegir otra es ENTRAR en
//     ella: `sesion.Suite.CambiarOrganizacion` y, al volver, la descarga empieza sola,
//     que es el `?descargar=<formato>` de la web.
//   - ponytail: al empezar, la web dice «Tu navegador la sigue guardando aunque cierres
//     esta página», que aquí sería falso. Va el titular solo y, al acabar, dónde ha
//     quedado el fichero; la frase propia de la app pide una clave nueva en el
//     catálogo de la web.

// Medidas propias de «Tus datos», de las clases del JSX.
const (
	// MargenEntradaAjustes es el `mt-1.5` entre el título y la entrada.
	MargenEntradaAjustes float32 = 1.5 * 0.25 * tema.Rem
	// MargenLeyenda es el `mb-1` de la leyenda y de la ayuda del `fieldset`.
	MargenLeyenda float32 = 1 * 0.25 * tema.Rem
	// HuecoOpciones es el `gap-2` del `fieldset`.
	HuecoOpciones float32 = 2 * 0.25 * tema.Rem
	// HuecoMover es el `gap-3` de la sección «Llevarlo a otro servicio».
	HuecoMover float32 = 3 * 0.25 * tema.Rem
)

// EstadoDescarga es por dónde va la copia pedida.
type EstadoDescarga uint8

// Los cuatro.
const (
	SinDescarga EstadoDescarga = iota
	Descargando
	Descargada
	DescargaFallida
)

// Datos es la pantalla `/datos`.
type Datos struct {
	pagina

	T *i18n.Translator
	// Formato es el de las listas; el cero vale CSV, que es lo que elige la web.
	Formato  api.HubFormato
	Descarga EstadoDescarga
	// Guardada es dónde ha quedado la copia, cuando ha acabado.
	Guardada string
	// Orgs son mis organizaciones; con dos o más se elige de cuál es la copia.
	Orgs []api.Organizacion
	// Org es la elegida; vacía, la activa.
	Org string

	AlDescargar func(api.HubFormato)
	// AlEntrarYDescargar entra en otra organización y pide allí la copia.
	AlEntrarYDescargar func(org string, f api.HubFormato)
	AlMigrar           func()
}

// orgElegida es la que está marcada: la pedida, o la activa si no se ha tocado.
func (d *Datos) orgElegida() string {
	if d.Org != "" {
		return d.Org
	}
	for _, o := range d.Orgs {
		if o.Activa {
			return o.ID
		}
	}
	return ""
}

// otraOrg es la elegida si NO es la activa: entonces el botón entra en ella.
func (d *Datos) otraOrg() string {
	for _, o := range d.Orgs {
		if o.ID == d.orgElegida() && !o.Activa {
			return o.ID
		}
	}
	return ""
}

var _ fyne.Widget = (*Datos)(nil)

// NuevosDatos crea la pantalla.
func NuevosDatos(t *i18n.Translator) *Datos {
	d := &Datos{T: t, Formato: api.HubCSV}
	d.ExtendBaseWidget(d)
	return d
}

// Resize recuerda el ancho: el alto depende de él.
func (d *Datos) Resize(s fyne.Size) {
	if d.recordar(s) {
		d.BaseWidget.Resize(s)
		d.Refresh()
		return
	}
	d.BaseWidget.Resize(s)
}

// AccessibilityRole: la pantalla agrupa.
func (d *Datos) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// claveFormato es la descripción de cada formato en el catálogo.
var claveFormato = map[api.HubFormato]string{
	api.HubCSV: "data.export.csv", api.HubJSON: "data.export.json", api.HubXML: "data.export.xml",
}

// CreateRenderer construye la pantalla. Las tres opciones, el botón y el enlace se
// crean UNA vez: rehacerlos al elegir le quitaría el foco a quien usa el teclado.
func (d *Datos) CreateRenderer() fyne.WidgetRenderer {
	t := d.T
	r := &datosRenderer{d: d, fondo: ui.NuevoFondoSuite(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))}
	for _, f := range api.HubFormatos {
		// El título es el nombre del formato, que no se traduce («CSV»): la constante
		// del SDK va en minúsculas porque es lo que viaja.
		o := ui.NuevaTarjetaEleccion(strings.ToUpper(string(f)), t.T(claveFormato[f], nil), func() {
			d.Formato = f
			d.Refresh()
		})
		o.Densa = true
		r.opciones = append(r.opciones, o)
	}
	r.accion = NuevaAccion(t.T("data.export.action", nil), tema.TextoSM, func() {
		if d.AlDescargar != nil {
			d.AlDescargar(d.formato())
		}
	})
	// Con otra organización elegida, la acción es un `Button variant="primary"` y no el
	// enlace: primero hay que entrar en ella.
	r.entrar = ui.NuevoBoton(t.T("data.export.action", nil), func() {
		if org := d.otraOrg(); org != "" && d.AlEntrarYDescargar != nil {
			d.AlEntrarYDescargar(org, d.formato())
		}
	}).Con(ui.Primario, ui.TallaMD)
	r.migrar = ui.NuevoEnlace(t.T("data.move.action", nil), tema.TextoSM, ui.SemiNegrita, "primary-text", func() {
		if d.AlMigrar != nil {
			d.AlMigrar()
		}
	})
	r.rehacer()
	return r
}

func (d *Datos) formato() api.HubFormato {
	if d.Formato == "" {
		return api.HubCSV
	}
	return d.Formato
}

type datosRenderer struct {
	d *Datos

	fondo            *ui.FondoSuite
	titulo, entrada  *ui.Parrafo
	copia, mover     *seccion
	cuerpo, leyenda  *ui.Parrafo
	ayudaFormato     *ui.Parrafo
	opciones         []*ui.TarjetaEleccion
	leyendaOrg       *ui.Parrafo
	ayudaOrg         *ui.Parrafo
	opcionesOrg      []*ui.TarjetaEleccion
	orgsDe           string // de qué lista están hechas las opciones de organización
	accion           *Accion
	entrar           *ui.Boton
	fallo            *ui.Parrafo
	empezada         *ui.AyudaEnLinea
	alcance, cuerpo2 *ui.Parrafo
	migrar           *ui.Enlace

	objetos []fyne.CanvasObject
}

func (r *datosRenderer) rehacer() {
	d, t := r.d, r.d.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := func(clave string, e tema.Escalon, peso ui.Peso, token string) *ui.Parrafo {
		return ui.NuevoParrafo(t.T(clave, nil), e, peso, token, v)
	}
	r.fondo.PonerVariante(v)
	r.titulo = p("data.title", ui.EscalonTitulo(2), ui.PesoTitulo(2), "text")
	r.entrada = p("data.lead", baseRelajada, ui.Normal, "text-muted")
	r.copia = nuevaSeccion(v, t.T("data.export.section", nil))
	r.cuerpo = p("data.export.body", smRelajada, ui.Normal, "text-muted")
	r.leyenda = p("data.export.formatLegend", tema.TextoSM, ui.SemiNegrita, "text")
	r.ayudaFormato = p("data.export.formatHelp", tema.TextoXS, ui.Normal, "text-muted")
	for i, o := range r.opciones {
		o.Elegida = api.HubFormatos[i] == d.formato()
		o.Refresh()
	}
	r.rehacerOrgs(v)
	r.fallo, r.empezada = nil, nil
	switch d.Descarga {
	case DescargaFallida:
		r.fallo = p("ob.error.generic", tema.TextoSM, ui.Normal, "danger-text")
	case Descargando, Descargada:
		r.empezada = ui.NuevaAyudaEnLinea("").ConTono(ui.AyudaExito).ConTitulo(t.T("data.export.startedTitle", nil))
		if d.Descarga == Descargada {
			r.empezada.Cuerpo = d.Guardada
		}
	case SinDescarga:
	}
	r.alcance = p("data.export.scope", xsRelajada, ui.Normal, "text-muted")
	r.mover = nuevaSeccion(v, t.T("data.move.section", nil))
	r.cuerpo2 = p("data.move.body", smRelajada, ui.Normal, "text-muted")

	r.objetos = []fyne.CanvasObject{r.fondo, r.titulo, r.entrada}
	r.objetos = append(r.objetos, r.copia.objetos()...)
	r.objetos = append(r.objetos, r.cuerpo)
	if r.hayOrgs() {
		r.objetos = append(r.objetos, r.leyendaOrg, r.ayudaOrg)
		for _, o := range r.opcionesOrg {
			r.objetos = append(r.objetos, o)
		}
	}
	r.objetos = append(r.objetos, r.leyenda, r.ayudaFormato)
	for _, o := range r.opciones {
		r.objetos = append(r.objetos, o)
	}
	r.objetos = append(r.objetos, r.botonDescarga())
	if r.fallo != nil {
		r.objetos = append(r.objetos, r.fallo)
	}
	if r.empezada != nil {
		r.objetos = append(r.objetos, r.empezada)
	}
	r.objetos = append(r.objetos, r.alcance)
	r.objetos = append(r.objetos, r.mover.objetos()...)
	r.objetos = append(r.objetos, r.cuerpo2, r.migrar)
}

func (r *datosRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, r.alto(r.d.Ancho())) }
func (r *datosRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *datosRenderer) Destroy()                     {}

// Refresh rehace la pantalla (ver el comentario del lanzador).
func (r *datosRenderer) Refresh() {
	r.rehacer()
	r.Layout(r.d.Size())
	canvas.Refresh(r.d)
}

// alto es lo que ocupa la pantalla entera. `Layout` recorre lo mismo.
func (r *datosRenderer) alto(ventana float32) float32 {
	return r.colocar(ventana, false)
}

func (r *datosRenderer) Layout(t fyne.Size) {
	if t.Width <= 0 {
		return
	}
	r.fondo.Resize(fyne.NewSize(t.Width, max(t.Height, r.alto(t.Width))))
	r.colocar(t.Width, true)
}

// colocar recorre la pantalla de arriba abajo; con `mover` a false solo mide, y así
// `alto` y `Layout` no pueden discrepar.
func (r *datosRenderer) colocar(ventana float32, mover bool) float32 {
	x, ancho := CajaAjustes(ventana)
	y := ponerParrafo(r.titulo, x, MargenAjustesY, ancho, mover) + MargenEntradaAjustes
	y = ponerParrafo(r.entrada, x, y, min(ancho, AnchoIntro), mover) + MargenAjustesY
	y = ponerSeccion(r.copia, x, y, ancho, mover, r.cuerpoCopia) + MargenAjustesY
	y = ponerSeccion(r.mover, x, y, ancho, mover, r.cuerpoMover)
	return y + MargenAjustesY
}

// hayOrgs: el `fieldset` de organizaciones sale con dos o más, como en la web.
func (r *datosRenderer) hayOrgs() bool { return len(r.d.Orgs) > 1 }

// rehacerOrgs rehace las opciones de organización SOLO si ha cambiado la lista:
// rehacerlas al elegir le quitaría el foco a quien usa el teclado.
func (r *datosRenderer) rehacerOrgs(v tema.Variante) {
	d, t := r.d, r.d.T
	r.leyendaOrg = ui.NuevoParrafo(t.T("ob.sum.org", nil), tema.TextoSM, ui.SemiNegrita, "text", v)
	r.ayudaOrg = ui.NuevoParrafo(t.T("data.export.orgHelp", nil), tema.TextoXS, ui.Normal, "text-muted", v)
	var clave strings.Builder
	for _, o := range d.Orgs {
		clave.WriteString(o.ID + "\x00" + o.Nombre + "\x00")
	}
	if clave.String() != r.orgsDe {
		r.orgsDe, r.opcionesOrg = clave.String(), nil
		for _, o := range d.Orgs {
			c := ui.NuevaTarjetaEleccion(o.Nombre, "", func() {
				d.Org = o.ID
				d.Refresh()
			})
			c.Densa = true
			r.opcionesOrg = append(r.opcionesOrg, c)
		}
	}
	for i, c := range r.opcionesOrg {
		c.Elegida = d.Orgs[i].ID == d.orgElegida()
		c.Refresh()
	}
}

// botonDescarga es el que toca: el enlace de siempre, o el botón que entra en otra.
func (r *datosRenderer) botonDescarga() fyne.CanvasObject {
	if r.d.otraOrg() != "" {
		return r.entrar
	}
	return r.accion
}

// ponerOpciones pone un `fieldset` de tarjetas de elección: leyenda, ayuda y opciones.
func ponerOpciones(leyenda, ayuda *ui.Parrafo, opciones []*ui.TarjetaEleccion, x, y, col float32, mover bool) float32 {
	y = ponerParrafo(leyenda, x, y, col, mover) + MargenLeyenda
	y = ponerParrafo(ayuda, x, y, col, mover) + MargenLeyenda + HuecoOpciones
	for i, o := range opciones {
		if i > 0 {
			y += HuecoOpciones
		}
		o.Ancho = col
		y = ponerObjeto(o, x, y, fyne.NewSize(col, ui.AltoTarjetaEleccion(o, col)), mover)
	}
	return y
}

// cuerpoCopia es «Descargar una copia»: todo en una columna de `max-w-2xl`.
func (r *datosRenderer) cuerpoCopia(x, y, col float32, mover bool) float32 {
	y = ponerParrafo(r.cuerpo, x, y, col, mover) + HuecoSeccion
	if r.hayOrgs() {
		y = ponerOpciones(r.leyendaOrg, r.ayudaOrg, r.opcionesOrg, x, y, col, mover) + HuecoSeccion
	}
	y = ponerOpciones(r.leyenda, r.ayudaFormato, r.opciones, x, y, col, mover)
	if b := r.botonDescarga(); b == r.entrar {
		y = ponerObjeto(b, x, y+HuecoSeccion, b.MinSize(), mover) + HuecoSeccion
	} else {
		y = ponerObjeto(r.accion, x, y+HuecoSeccion, MedidaAccion(r.accion), mover) + HuecoSeccion
	}
	if r.fallo != nil {
		y = ponerParrafo(r.fallo, x, y, col, mover) + HuecoSeccion
	}
	if r.empezada != nil {
		y = ponerObjeto(r.empezada, x, y, ui.MedidaAyudaEnLinea(r.empezada, col), mover) + HuecoSeccion
	}
	return ponerParrafo(r.alcance, x, y, col, mover)
}

// cuerpoMover es «Llevarlo a otro servicio»: el párrafo y el enlace, con `gap-3`.
func (r *datosRenderer) cuerpoMover(x, y, col float32, mover bool) float32 {
	y = ponerParrafo(r.cuerpo2, x, y, col, mover) + HuecoMover
	return ponerObjeto(r.migrar, x, y, ui.MedidaEnlace(r.migrar), mover)
}

// ---- La descarga ----

// Exportador es lo que la copia le pide al Hub. Lo cumple `*api.Hub`.
type Exportador interface {
	Export(ctx context.Context, f api.HubFormato) (io.ReadCloser, string, error)
}

var _ Exportador = (*api.Hub)(nil)

// GuardarCopia pide la copia y la escribe en `carpeta` según llega, sin pisar nada: la
// copia puede pesar gigas y no cabe en memoria. Devuelve dónde ha quedado. Es lo que en
// la web hace el gestor de descargas del navegador.
func GuardarCopia(ctx context.Context, e Exportador, f api.HubFormato, carpeta string) (string, error) {
	cuerpo, nombre, err := e.Export(ctx, f)
	if err != nil {
		return "", err
	}
	defer func() { _ = cuerpo.Close() }()
	return ficheros.Guardar(carpeta, nombre, cuerpo)
}
