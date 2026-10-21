// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"image/color"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/buscar"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// En la web: apps/frontend/apps/rooms/src/components/ForwardMessageModal.tsx,
// apps/frontend/apps/rooms/src/lib/forward.ts y, de MessageView.tsx, «Guardar en Notes»
// y el diálogo de «Visto por».

// CuerpoReenviado es `forwardedBody`: el original citado línea a línea, atribuido a su
// autor y con el enlace al mensaje real, que es quien sigue pasando por el permiso del
// canal.
func CuerpoReenviado(cuerpo, autor, enlace string) string {
	citado := "> _(sin texto)_"
	if t := strings.TrimSpace(cuerpo); t != "" {
		lineas := strings.Split(t, "\n")
		for i, l := range lineas {
			lineas[i] = "> " + l
		}
		citado = strings.Join(lineas, "\n")
	}
	return "Reenviado de **" + autor + "**:\n" + citado + "\n\n" + enlace
}

// GuardarEnNotes es «Guardar en Sytue Notepads»: abre Notepads con una nota nueva YA
// VINCULADA al mensaje. No se copia el texto: viaja la REFERENCIA, así que quien abra la
// nota sigue pasando por el permiso del canal. En nativo es el esquema de Notepads, y
// si no está instalada, su web.
func (sa *Sala) GuardarEnNotes(m Mensaje) {
	rotulo := []rune(strings.TrimSpace(m.Cuerpo))
	ruta := "/?link=room_message:" + url.QueryEscape(m.ID) + "&label=" + url.QueryEscape(string(rotulo[:min(len(rotulo), 120)]))
	if sa.Abridor == nil {
		return
	}
	if err := api.AbrirEn(sa.Abridor, sa.Apex, "notes", sa.org(), ruta); err != nil {
		sa.P.Avisar(ui.AvisoPeligro, sa.P.T.T("error.loadFailed", nil))
	}
}

// ---- «Reenviar» ----

// AbrirReenviar es `ForwardMessageModal`: el original arriba, la caja para filtrar y la
// lista de canales y directos; elegir uno publica la cita y cierra.
func (sa *Sala) AbrirReenviar(m Mensaje) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	t := sa.P.T
	r := NuevoReenviar(t, m, sa.P.Lateral.Canales)
	d := ui.NuevoDialogo(t.T("forward.title", nil), r).Con(ui.DialogoSM, nil)
	d.RotuloCerrar, d.Foco = t.T("action.close", nil), r.Caja()
	cuerpo := CuerpoReenviado(m.Cuerpo, m.Autor.Nombre, sa.enlaceA(m))
	r.AlElegir = func(c Canal) {
		r.Enviando = c.ID
		r.Refresh()
		pedir(sa, func(ctx context.Context) (api.RoomsMessage, error) { return sa.S.Post(ctx, c.ID, cuerpo) },
			func(_ api.RoomsMessage, err error) {
				r.Enviando = ""
				r.Refresh()
				if err != nil {
					sa.P.Avisar(ui.AvisoPeligro, t.T("forward.failed", nil))
					return
				}
				destino := "#" + c.Nombre
				if c.Clase == Directo {
					destino = c.Nombre
				}
				sa.P.Avisos.Notificar(ui.NuevoAviso(ui.AvisoExito, t.T("forward.done", nil), destino))
				d.Cerrar()
				if c.ID == sa.abierto {
					sa.recargarMensajes()
				}
			})
	}
	sa.reenviar = r
	d.Mostrar(lienzo)
}

// Reenviar es el cuerpo del diálogo: `flex flex-col gap-3` con la vista previa, la caja y
// la lista.
type Reenviar struct {
	widget.BaseWidget

	T        *i18n.Translator
	Mensaje  Mensaje
	Canales  []Canal
	AlElegir func(Canal)
	// Enviando es el canal al que se está reenviando: su fila lleva la rueda y las demás
	// se apagan.
	Enviando string

	caja *ui.Entrada
	col  *fyne.Container
}

// NuevoReenviar crea el cuerpo.
func NuevoReenviar(t *i18n.Translator, m Mensaje, canales []Canal) *Reenviar {
	r := &Reenviar{T: t, Mensaje: m, Canales: canales}
	r.caja = ui.NuevaEntrada(t.T("search.channelOrPerson", nil))
	r.caja.Icono = iconos.Buscar
	r.caja.AlCambiar(func(string) { r.Refresh() })
	r.ExtendBaseWidget(r)
	return r
}

// Caja es la de filtrar, que es donde va el foco al abrir.
func (r *Reenviar) Caja() *ui.Entrada { return r.caja }

// Candidatos son los canales cuyo nombre contiene lo escrito.
func (r *Reenviar) Candidatos() []Canal {
	q := buscar.Plegar(strings.TrimSpace(r.caja.Texto()))
	var out []Canal
	for _, c := range r.Canales {
		if q == "" || strings.Contains(buscar.Plegar(c.Nombre), q) {
			out = append(out, c)
		}
	}
	return out
}

// AccessibilityLabel es el del diálogo.
func (r *Reenviar) AccessibilityLabel() string { return r.T.T("forward.title", nil) }

// AccessibilityRole: un contenedor.
func (r *Reenviar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// AltoMaxReenviar es el `max-h-[46dvh]` de la lista, contra la ventana de 800 de la
// medida de escritorio.
//
// ponytail: fijo a los 800 de referencia; en una ventana más baja la lista no encoge.
var AltoMaxReenviar float32 = 0.46 * 800

func (r *Reenviar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	m := r.Mensaje
	previa := strings.TrimSpace(m.Cuerpo)
	if previa == "" {
		previa = r.T.T("forward.noText", nil)
	}
	autor := ui.NuevoRotulo(m.Autor.Nombre, tema.TextoXS, ui.SemiNegrita, "text-muted")
	texto := ui.NuevoParrafo(previa, tema.TextoSM, ui.Normal, "text", v)
	texto.Lineas = 2
	caja := ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD)
	vista := container.NewStack(append(caja.Objetos(),
		container.New(ui.Margen{Izq: tema.Espacio(3), Arr: tema.Espacio(2), Der: tema.Espacio(3), Abj: tema.Espacio(2)},
			container.New(ui.Columna{Hueco: tema.Espacio(0.5)}, autor, texto)))...)
	r.col = container.New(ui.Columna{Hueco: tema.Espacio(0.5)})
	lista := container.NewVScroll(r.col)
	r.pintar()
	return widget.NewSimpleRenderer(container.New(ui.Columna{Hueco: tema.Espacio(3)}, vista, r.caja,
		container.New(altoMax(AltoMaxReenviar), lista)))
}

// pintar rehace las filas con lo escrito.
func (r *Reenviar) pintar() {
	if r.col == nil {
		return
	}
	r.col.Objects = r.col.Objects[:0]
	cs := r.Candidatos()
	if len(cs) == 0 {
		clave := "forward.empty"
		if strings.TrimSpace(r.caja.Texto()) != "" {
			clave = "forward.emptyQuery"
		}
		r.col.Objects = append(r.col.Objects, container.New(ui.Margen{Arr: tema.Espacio(8), Abj: tema.Espacio(8)},
			container.NewCenter(ui.NuevoRotulo(r.T.T(clave, nil), tema.TextoSM, ui.Normal, "text-muted"))))
	}
	for _, c := range cs {
		r.col.Objects = append(r.col.Objects, NuevaFilaDestino(c, r.Enviando, r.AlElegir))
	}
	r.col.Refresh()
}

// Refresh vuelve a filtrar.
func (r *Reenviar) Refresh() {
	r.pintar()
	r.BaseWidget.Refresh()
}

// altoMax es un `max-h-*`: el hijo mide lo que pida, hasta ese tope.
type altoMax float32

func (a altoMax) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	if s, ok := objs[0].(*container.Scroll); ok {
		m = s.Content.MinSize()
	}
	return fyne.NewSize(m.Width, min(m.Height, float32(a)))
}

func (a altoMax) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.Position{})
	objs[0].Resize(t)
}

// FilaDestino es un canal de la lista: `flex items-center gap-3 rounded-md px-2 py-2`, el
// avatar `sm` de un directo o la almohadilla de un canal, el nombre en `text-base
// font-medium` y la rueda del que se está enviando.
type FilaDestino struct {
	ui.Pulsable
	Canal    Canal
	Enviando string
	// AlfaInerte es la opacidad apagada: el `disabled:opacity-60` de «Reenviar» o el 50
	// de «Compartir en Rooms». Cero es 0,6.
	AlfaInerte float32
}

// NuevaFilaDestino crea la fila; con otro enviándose, se apaga (`disabled:opacity-60`).
func NuevaFilaDestino(c Canal, enviando string, al func(Canal)) *FilaDestino {
	f := &FilaDestino{Canal: c, Enviando: enviando}
	f.Nombre = c.Nombre
	f.Inerte = enviando != ""
	f.AlPulsar = func() {
		if al != nil {
			al(c)
		}
	}
	f.ExtendBaseWidget(f)
	return f
}

var _ desktop.Hoverable = (*FilaDestino)(nil)

// AltoFilaDestino manda el avatar `sm` (32) con su `py-2`.
func AltoFilaDestino() float32 { return 2*tema.Espacio(2) + ui.MedidaAvatar(ui.AvatarSM).Height }

func (f *FilaDestino) MinSize() fyne.Size { return fyne.NewSize(0, AltoFilaDestino()) }

func (f *FilaDestino) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaDestinoRenderer{f: f, fondo: canvas.NewRectangle(color.NRGBA{})}
	r.fondo.CornerRadius = tema.RadioMD
	if f.Canal.Clase == Directo {
		r.marca = ui.NuevoAvatar(f.Canal.Nombre).Con(ui.AvatarSM)
	} else {
		r.marca = NuevaAlmohadilla("text-muted")
	}
	r.nombre = ui.Texto(f.Canal.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v))
	r.objs = []fyne.CanvasObject{r.fondo, r.marca, r.nombre}
	if f.Enviando == f.Canal.ID {
		r.rueda = ui.NuevaRueda(iconos.TallaSM, "text-muted", v)
		r.objs = append(r.objs, r.rueda)
	}
	return r
}

type filaDestinoRenderer struct {
	f      *FilaDestino
	fondo  *canvas.Rectangle
	marca  fyne.CanvasObject
	nombre *canvas.Text
	rueda  fyne.CanvasObject
	objs   []fyne.CanvasObject
}

func (r *filaDestinoRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaDestinoRenderer) Objects() []fyne.CanvasObject { return r.objs }
func (r *filaDestinoRenderer) Destroy()                     {}

func (r *filaDestinoRenderer) Layout(t fyne.Size) {
	r.fondo.Resize(t)
	x := tema.Espacio(2)
	m := r.marca.MinSize()
	r.marca.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
	r.marca.Resize(m)
	x += m.Width + tema.Espacio(3)
	derecha := t.Width - tema.Espacio(2)
	if r.rueda != nil {
		derecha -= iconos.TallaSM + tema.Espacio(3)
		r.rueda.Move(fyne.NewPos(derecha+tema.Espacio(3), (t.Height-iconos.TallaSM)/2))
		r.rueda.Resize(fyne.NewSize(iconos.TallaSM, iconos.TallaSM))
	}
	ui.Truncar(r.nombre, r.f.Canal.Nombre, tema.TextoBase, ui.Medio, max(derecha-x, 0))
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, (t.Height-tema.TextoBase.Linea)/2)
}

func (r *filaDestinoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = color.NRGBA{}
	if r.f.Encima() || r.f.Enfocado() {
		r.fondo.FillColor = tema.Color("bg-subtle", v)
	}
	r.fondo.Refresh()
	tinta := tema.Color("text", v)
	if r.f.Inerte {
		alfa := r.f.AlfaInerte
		if alfa == 0 {
			alfa = 0.6
		}
		tinta.A = uint8(float32(tinta.A) * alfa)
	}
	r.nombre.Color = tinta
	r.nombre.Refresh()
	r.Layout(r.f.Size())
}

// ---- «Visto por» ----

// AbrirLectores es el diálogo `sm` de «Visto por»: la rueda mientras llega, «nadie» o
// «N de M personas» con la lista de quién y cuándo. Un fallo enseña el vacío: decir «no
// se pudo» en algo que se abre de un menú es ruido, y volver a abrirlo reintenta.
func (sa *Sala) AbrirLectores(m Mensaje) {
	lienzo := fyne.CurrentApp().Driver().CanvasForObject(sa.P.Objeto())
	if lienzo == nil {
		return
	}
	t := sa.P.T
	cuerpo := container.NewStack(container.New(altoFijo(tema.Espacio(20)),
		container.NewCenter(ui.NuevaRueda(18, "text-muted", tema.De(fyne.CurrentApp().Settings().ThemeVariant())))))
	var d *ui.Dialogo
	cerrar := ui.NuevoBoton(t.T("action.close", nil), func() { d.Cerrar() }).Con(ui.Primario, ui.TallaMD)
	d = ui.NuevoDialogo(t.T("message.readersTitle", nil), cuerpo).Con(ui.DialogoSM, cerrar)
	d.RotuloCerrar = t.T("action.close", nil)
	sa.pregunta = d
	d.Mostrar(lienzo)
	type lectura struct {
		ls    []api.RoomsReader
		total int
	}
	pedir(sa, func(ctx context.Context) (lectura, error) {
		ls, n, err := sa.S.Readers(ctx, m.ID)
		return lectura{ls, n}, err
	}, func(l lectura, err error) {
		if err != nil {
			l = lectura{}
		}
		cuerpo.Objects = []fyne.CanvasObject{ListaLectores(t, sa.P.Locale, l.ls, l.total)}
		cuerpo.Refresh()
		d.Recolocar()
	})
}

// ListaLectores es el cuerpo ya con datos.
func ListaLectores(t *i18n.Translator, l i18n.Locale, ls []api.RoomsReader, total int) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if len(ls) == 0 {
		return ui.NuevoParrafo(t.T("message.seenByNobody", nil), tema.TextoSM, ui.Normal, "text-muted", v)
	}
	filas := make([]fyne.CanvasObject, 0, 2*len(ls))
	for i, r := range ls {
		if i > 0 {
			pelo := canvas.NewRectangle(tokenConAlfa("border", 0.6, v))
			pelo.SetMinSize(fyne.NewSize(0, tema.Borde))
			filas = append(filas, pelo)
		}
		filas = append(filas, container.New(ui.Margen{Izq: tema.Espacio(3), Arr: tema.Espacio(2), Der: tema.Espacio(3), Abj: tema.Espacio(2)},
			container.NewBorder(nil, nil, nil,
				ui.NuevoRotulo(i18n.FormatDateTime(l, r.ReadAt.Local()), escalonMeta, ui.Normal, "text-subtle"),
				ui.NuevoRotulo(nombreDe(r.Name, r.Subject), tema.TextoSM, ui.Normal, "text"))))
	}
	caja := ui.NuevaCaja(color.NRGBA{}, tema.Color("border", v), tema.RadioMD)
	lista := container.NewStack(append(caja.Objetos(), container.New(ui.Margen{Arr: tema.Borde, Abj: tema.Borde},
		container.New(ui.Columna{}, filas...)))...)
	cuenta := ui.NuevoRotulo(t.T("message.seenByCount", i18n.Vars{"count": len(ls), "total": total}), escalonMeta, ui.Normal, "text-subtle")
	return container.New(ui.Columna{Hueco: tema.Espacio(2)}, cuenta, lista)
}

// tokenConAlfa es un `border-border/60`: el token con esa opacidad.
func tokenConAlfa(token string, a float32, v tema.Variante) color.NRGBA {
	c := tema.Color(token, v)
	c.A = uint8(float32(c.A) * a)
	return c
}

// altoFijo es un `h-*`: el hijo a ese alto.
type altoFijo float32

func (a altoFijo) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(objs[0].MinSize().Width, float32(a))
}

func (a altoFijo) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.Position{})
	objs[0].Resize(t)
}
