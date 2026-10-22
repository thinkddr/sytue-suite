// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// Contrapartida de BrowseChannelsModal.tsx dentro de la pantalla principal declarada
// en pantalla.go.
//
// «Explorar canales»: diálogo `md` con la caja de buscar (espera 250 ms) y la lista de
// los canales públicos a los que aún no se pertenece, cada uno con «Unirse». Pie con
// «Cerrar». Estados: la rueda de 22 mientras carga, el error con reintento, y el vacío
// —«no hay canales» o «ninguno coincide», con su ilustración—.

// Medidas de la HOJA: la lista es `flex-col gap-1`, cada fila `flex items-center gap-3
// rounded-md px-2 py-2` con la marca de 28, el nombre `text-base font-medium`, el
// propósito (o el tema) en `text-xs text-text-muted` y el botón `sm` secundario.
var (
	HuecoListaExplorar          = tema.Espacio(1)
	CanalonFilaExplorar         = tema.Espacio(2)
	RellenoFilaExplorar         = tema.Espacio(2)
	HuecoFilaExplorar           = tema.Espacio(3)
	LadoRuedaExplorar   float32 = 22
	AireCargaExplorar           = tema.Espacio(10)
	AireErrorExplorar           = tema.Espacio(8)
)

// Explorar es el cuerpo del diálogo.
type Explorar struct {
	widget.BaseWidget

	T      *i18n.Translator
	Estado Estado
	// Canales son los resultados; Uniendo, el que está en camino.
	Canales []Canal
	Uniendo string

	AlBuscar     func(string)
	AlUnirse     func(Canal)
	AlReintentar func()

	caja    *ui.Entrada
	cuerpo  fyne.CanvasObject
	objetos []fyne.CanvasObject
}

var _ fyne.Widget = (*Explorar)(nil)

// NuevoExplorar crea el cuerpo, cargando.
func NuevoExplorar(t *i18n.Translator) *Explorar {
	e := &Explorar{T: t, Estado: Cargando}
	e.ExtendBaseWidget(e)
	e.caja = ui.NuevaEntrada(t.T("browse.searchPlaceholder", nil))
	e.caja.Icono = iconos.Buscar
	e.caja.AlCambiar(func(s string) {
		if e.AlBuscar != nil {
			e.AlBuscar(s)
		}
	})
	return e
}

// Caja es la de buscar, que recibe el foco.
func (e *Explorar) Caja() *ui.Entrada { return e.caja }

func (e *Explorar) construir(v tema.Variante) fyne.CanvasObject {
	t := e.T
	switch e.Estado {
	case Cargando:
		return container.NewCenter(container.New(ui.Columna{},
			hueco(AireCargaExplorar), ui.NuevaRueda(LadoRuedaExplorar, "text-subtle", v), hueco(AireCargaExplorar)))
	case Error:
		return container.NewCenter(container.New(ui.Columna{Hueco: tema.Espacio(2)},
			hueco(AireErrorExplorar-tema.Espacio(2)),
			container.NewCenter(ui.NuevoRotulo(t.T("browse.loadFailed", nil), tema.TextoSM, ui.Normal, "text-muted")),
			container.NewCenter(ui.NuevoBoton(t.T("action.retry", nil), e.AlReintentar).Con(ui.Secundario, ui.TallaSM)),
			hueco(AireErrorExplorar-tema.Espacio(2))))
	case Listo, Vacio, SinConexion, SinPermiso:
	}
	if len(e.Canales) == 0 {
		titulo, ilus := t.T("browse.empty", nil), iconos.VacioChat
		if strings.TrimSpace(e.caja.Texto()) != "" {
			titulo, ilus = t.T("browse.emptyQuery", nil), iconos.VacioBusqueda
		}
		vac := ui.NuevoVacio(titulo, "")
		vac.Ilustracion = iconos.Pintar(ilus, v).A(LadoIlustracionBusqueda).Objeto()
		return vac
	}
	filas := make([]fyne.CanvasObject, 0, len(e.Canales))
	for _, c := range e.Canales {
		f := NuevaFilaExplorar(t, c, e.Uniendo == c.ID)
		f.AlUnirse = e.AlUnirse
		filas = append(filas, f)
	}
	// ponytail: la lista de la web tiene tope (`max-h-[46dvh]`) y desplaza dentro;
	// aquí crece con sus filas y desplaza el cuerpo del diálogo, que lo hace solo.
	return container.New(ui.Columna{Hueco: HuecoListaExplorar}, filas...)
}

// CreateRenderer monta la caja y el cuerpo.
func (e *Explorar) CreateRenderer() fyne.WidgetRenderer {
	r := &explorarRenderer{e: e}
	r.Refresh()
	return r
}

type explorarRenderer struct{ e *Explorar }

func (r *explorarRenderer) MinSize() fyne.Size {
	m := r.e.cuerpo.MinSize()
	return fyne.NewSize(m.Width, r.e.caja.MinSize().Height+tema.Espacio(3)+m.Height)
}
func (r *explorarRenderer) Objects() []fyne.CanvasObject { return r.e.objetos }
func (r *explorarRenderer) Destroy()                     {}

func (r *explorarRenderer) Layout(t fyne.Size) {
	e := r.e
	mc := e.caja.MinSize()
	e.caja.Resize(fyne.NewSize(t.Width, mc.Height))
	y := mc.Height + tema.Espacio(3) // `gap-3`
	e.cuerpo.Move(fyne.NewPos(0, y))
	e.cuerpo.Resize(fyne.NewSize(t.Width, max(t.Height-y, e.cuerpo.MinSize().Height)))
}

func (r *explorarRenderer) Refresh() {
	e := r.e
	e.cuerpo = e.construir(tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	e.objetos = []fyne.CanvasObject{e.caja, e.cuerpo}
	r.Layout(e.Size())
}

// FilaExplorar es un canal al que unirse.
type FilaExplorar struct {
	widget.BaseWidget

	Canal    Canal
	Uniendo  bool
	AlUnirse func(Canal)

	encima  bool
	fondo   *canvas.Rectangle
	marca   *MarcaCanal
	nombre  *canvas.Text
	detalle *canvas.Text
	unirse  *ui.Boton
	objetos []fyne.CanvasObject
}

var _ desktop.Hoverable = (*FilaExplorar)(nil)

// NuevaFilaExplorar crea la fila.
func NuevaFilaExplorar(t *i18n.Translator, c Canal, uniendo bool) *FilaExplorar {
	f := &FilaExplorar{Canal: c, Uniendo: uniendo}
	f.ExtendBaseWidget(f)
	f.unirse = ui.NuevoBoton(t.T("action.join", nil), func() {
		if f.AlUnirse != nil {
			f.AlUnirse(f.Canal)
		}
	}).Con(ui.Secundario, ui.TallaSM)
	f.unirse.Cargando = uniendo
	return f
}

// MouseIn es el `hover:bg-bg-subtle` de la fila.
func (f *FilaExplorar) MouseIn(*desktop.MouseEvent) { f.encima = true; f.Refresh() }

// MouseMoved no hace nada.
func (f *FilaExplorar) MouseMoved(*desktop.MouseEvent) {}

// MouseOut devuelve la fila al reposo.
func (f *FilaExplorar) MouseOut() { f.encima = false; f.Refresh() }

// AccessibilityLabel es el nombre del canal.
func (f *FilaExplorar) AccessibilityLabel() string { return f.Canal.Nombre }

// AccessibilityRole implementa fyne.Accessible.
func (f *FilaExplorar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// detalleDe es el propósito del canal, o su tema si aún no tiene propósito.
func (f *FilaExplorar) detalleDe() string {
	if f.Canal.Proposito != "" {
		return f.Canal.Proposito
	}
	return f.Canal.Tema
}

// MedidaFilaExplorar es lo que ocupa: manda la marca de 28, o el nombre con su
// detalle debajo si lo hay (19,25 + 14).
func MedidaFilaExplorar(conDetalle bool) fyne.Size {
	texto := tema.TextoBase.Linea
	if conDetalle {
		texto += tema.TextoXS.Linea
	}
	return fyne.NewSize(0, 2*RellenoFilaExplorar+max(LadoMarcaCanal, texto, ui.MedidaBoton("", ui.TallaSM, 0).Height))
}

// CreateRenderer compone la fila.
func (f *FilaExplorar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo = canvas.NewRectangle(color.NRGBA{})
	f.fondo.CornerRadius = tema.RadioMD
	f.marca = NuevaMarcaCanalPapel()
	f.nombre = ui.Texto(f.Canal.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v))
	f.detalle = ui.Texto(f.detalleDe(), tema.TextoXS, ui.Normal, tema.Color("text-muted", v))
	f.objetos = []fyne.CanvasObject{f.fondo, f.marca, f.nombre, f.detalle, f.unirse}
	r := &filaExplorarRenderer{f: f}
	r.Refresh()
	return r
}

type filaExplorarRenderer struct{ f *FilaExplorar }

func (r *filaExplorarRenderer) MinSize() fyne.Size {
	return MedidaFilaExplorar(r.f.detalleDe() != "")
}
func (r *filaExplorarRenderer) Objects() []fyne.CanvasObject { return r.f.objetos }
func (r *filaExplorarRenderer) Destroy()                     {}

func (r *filaExplorarRenderer) Layout(t fyne.Size) {
	f := r.f
	f.fondo.Resize(t)
	x := CanalonFilaExplorar
	f.marca.Move(fyne.NewPos(x, (t.Height-LadoMarcaCanal)/2))
	f.marca.Resize(fyne.NewSize(LadoMarcaCanal, LadoMarcaCanal))
	x += LadoMarcaCanal + HuecoFilaExplorar
	mb := f.unirse.MinSize()
	xb := t.Width - CanalonFilaExplorar - mb.Width
	f.unirse.Move(fyne.NewPos(xb, (t.Height-mb.Height)/2))
	f.unirse.Resize(mb)
	cabe := max(xb-HuecoFilaExplorar-x, 0)
	alto := tema.TextoBase.Linea
	detalle := f.detalleDe()
	if detalle != "" {
		alto += tema.TextoXS.Linea
	}
	y := (t.Height - alto) / 2
	ui.Truncar(f.nombre, f.Canal.Nombre, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(f.nombre, tema.TextoBase, x, y)
	if detalle == "" {
		f.detalle.Hide()
		return
	}
	f.detalle.Show()
	ui.Truncar(f.detalle, detalle, tema.TextoXS, ui.Normal, cabe)
	ui.ColocarTexto(f.detalle, tema.TextoXS, x, y+tema.TextoBase.Linea)
}

func (r *filaExplorarRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	f.fondo.FillColor = color.NRGBA{}
	if f.encima {
		f.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	f.fondo.Refresh()
	r.Layout(f.Size())
}
