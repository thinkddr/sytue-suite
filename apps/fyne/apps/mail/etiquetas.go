// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # «Etiquetas» (`LabelsModal.tsx`)
//
// El gestor de etiquetas: qué es una etiqueta, las tuyas —renombrar al salir del campo,
// recolorear con un clic, borrar con confirmación en la propia fila— y una nueva, con
// su nombre (sin repetir) y su color. Todo optimista, con aviso si el servidor dice que
// no, y el lateral se refresca con cada cambio.

// GestorEtiquetas es lo que el diálogo le pide al servidor. Lo cumple *api.Mail.
type GestorEtiquetas interface {
	CreateLabel(ctx context.Context, nombre, color string) (api.MailLabel, error)
	UpdateLabel(ctx context.Context, id, nombre, color string) error
	DeleteLabel(ctx context.Context, id string) error
}

var _ GestorEtiquetas = (*api.Mail)(nil)

// ColoresEtiqueta son los tokens de `LABEL_COLOR_TOKENS`, en su orden.
var ColoresEtiqueta = []string{"slate", "brass", "sage", "clay", "blue", "plum", "teal", "rose"}

// Las medidas de la hoja: la muestra es `h-6 w-6 rounded-full border-2` con el tic de
// 14 en la elegida y `gap-1.5`; la fila de la lista, `flex items-center gap-3 px-3
// py-2.5` con el punto de `h-3 w-3`.
const (
	ladoMuestraEtiqueta float32 = 6 * 0.25 * tema.Rem
	huecoMuestras       float32 = 1.5 * 0.25 * tema.Rem
	puntoFilaEtiqueta   float32 = 3 * 0.25 * tema.Rem
	margenXFilaEtiqueta float32 = 3 * 0.25 * tema.Rem
	margenYFilaEtiqueta float32 = 2.5 * 0.25 * tema.Rem
	huecoFilaEtiqueta   float32 = 3 * 0.25 * tema.Rem
	huecoSeccionesEtiq  float32 = 6 * 0.25 * tema.Rem
	huecoDentroSeccion  float32 = 3 * 0.25 * tema.Rem
	aireSeccionNueva    float32 = 5 * 0.25 * tema.Rem
)

// muestraEtiqueta es un color del selector: redonda, con el borde de 2 en `text` si es
// la elegida y el tic blanco de 14 dentro.
type muestraEtiqueta struct {
	ui.Pulsable
	token   string
	Elegida bool
}

func nuevaMuestraEtiqueta(token, nombre string, alPulsar func()) *muestraEtiqueta {
	m := &muestraEtiqueta{token: token}
	m.Nombre, m.AlPulsar = nombre, alPulsar
	m.ExtendBaseWidget(m)
	return m
}

// celdaMuestraEtiqueta es la caja de una muestra: el punto de 24 y, con el dedo, un
// blanco de 44 con el punto en su centro y sin hueco entre blancos (la web: el
// `after:-inset-2.5` y `gap-5`, el mismo paso de 44).
func celdaMuestraEtiqueta() float32 {
	if ui.EsTactil() {
		return ui.SueloTactilBarra
	}
	return ladoMuestraEtiqueta
}

func huecoMuestrasEtiqueta() float32 {
	if ui.EsTactil() {
		return 0
	}
	return huecoMuestras
}

func (m *muestraEtiqueta) MinSize() fyne.Size {
	return fyne.NewSize(celdaMuestraEtiqueta(), celdaMuestraEtiqueta())
}

func (m *muestraEtiqueta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &muestraRenderer{
		m:      m,
		aro:    ui.NuevoAroCon("focus-ring", ladoMuestraEtiqueta/2, ui.ConHueco, v),
		fuera:  canvas.NewCircle(color.NRGBA{}),
		dentro: canvas.NewCircle(ColorEtiqueta(m.token, v)),
		tic:    iconos.TrazoConTinta(iconos.Tic, 14, tema.Color("neutral-0", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.fuera, r.dentro, r.tic.Objeto())
	r.Refresh()
	return r
}

type muestraRenderer struct {
	m             *muestraEtiqueta
	aro           *ui.Aro
	fuera, dentro *canvas.Circle
	tic           *iconos.Dibujo
	objetos       []fyne.CanvasObject
}

func (r *muestraRenderer) MinSize() fyne.Size           { return r.m.MinSize() }
func (r *muestraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *muestraRenderer) Destroy()                     {}
func (r *muestraRenderer) Layout(celda fyne.Size) {
	// El punto es de 24 en el centro de su celda (44 con el dedo).
	o := fyne.NewPos((celda.Width-ladoMuestraEtiqueta)/2, (celda.Height-ladoMuestraEtiqueta)/2)
	t := fyne.NewSize(ladoMuestraEtiqueta, ladoMuestraEtiqueta)
	r.aro.ColocarEn(o, t)
	r.fuera.Move(o)
	r.fuera.Resize(t)
	// `border-2`: el color va por dentro de los dos píxeles del borde.
	r.dentro.Move(o.AddXY(2, 2))
	r.dentro.Resize(fyne.NewSize(t.Width-4, t.Height-4))
	r.tic.Colocar(o.X+(t.Width-14)/2, o.Y+(t.Height-14)/2, 14)
}

func (r *muestraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	c := ColorEtiqueta(r.m.token, v)
	r.fuera.FillColor = c // `border-transparent` deja ver el fondo del botón, que es el color
	if r.m.Elegida {
		r.fuera.FillColor = tema.Color("text", v)
	}
	r.dentro.FillColor = c
	r.tic.Ver(r.m.Elegida)
	r.aro.Pintar(v)
	r.aro.Ver(r.m.Enfocado())
	for _, o := range r.objetos {
		o.Refresh()
	}
}

// selectorColor es `ColorPicker`: las ocho muestras, una elegida.
func selectorColor(t *i18n.Translator, elegido string, alElegir func(string)) *fyne.Container {
	objs := make([]fyne.CanvasObject, 0, len(ColoresEtiqueta))
	for _, token := range ColoresEtiqueta {
		m := nuevaMuestraEtiqueta(token, t.T("colour."+token, nil), func() { alElegir(token) })
		m.Elegida = token == elegido
		objs = append(objs, m)
	}
	return container.New(flujo{hueco: huecoMuestrasEtiqueta()}, objs...)
}

// ---- El diálogo ----

// gestorEtiquetas es el estado del diálogo abierto.
type gestorEtiquetas struct {
	b        *Buzon
	d        *ui.Dialogo
	caja     *fyne.Container
	ancho    float32
	lista    []api.MailLabel
	estado   Estado
	borrando string
	nombre   *ui.Entrada
	color    string
	creando  bool
}

// verEtiquetas abre «Etiquetas».
func (b *Buzon) verEtiquetas() {
	lienzo := b.P.Lienzo()
	if lienzo == nil {
		return
	}
	t := b.P.T
	g := &gestorEtiquetas{b: b, ancho: AnchoCuerpoDialogo(ui.DialogoLG, lienzo.Size().Width), color: ColoresEtiqueta[0]}
	g.nombre = ui.NuevaEntrada(t.T("labels.namePlaceholder", nil))
	g.nombre.Ancho = g.ancho
	g.nombre.AlCambiar(func(string) { g.pintar() })
	g.nombre.AlTeclear = func(k *fyne.KeyEvent) bool {
		if k.Name == fyne.KeyReturn || k.Name == fyne.KeyEnter {
			g.crear()
			return true
		}
		return false
	}
	g.caja = container.New(ui.Columna{Hueco: huecoSeccionesEtiq})
	cerrar := ui.NuevoBoton(t.T("common.close", nil), nil).Con(ui.Secundario, ui.TallaMD)
	g.d = ui.NuevoDialogo(t.T("labels.title", nil), g.caja).Con(ui.DialogoLG, cerrar)
	cerrar.AlPulsar = g.d.Cerrar
	g.d.RotuloCerrar = t.T("ui.close", nil)
	g.d.Mostrar(lienzo)
	g.cargar()
}

func (g *gestorEtiquetas) cargar() {
	g.estado = Cargando
	g.pintar()
	pedir(g.b, g.b.S.Labels, func(xs []api.MailLabel, err error) {
		if err != nil {
			g.estado = Error
		} else {
			g.lista, g.estado = xs, Listo
		}
		g.pintar()
	})
}

// cambiado es `onChanged`: el lateral cuenta las etiquetas de nuevo.
func (g *gestorEtiquetas) cambiado() { g.b.cargarEtiquetas() }

// duplicada dice si el nombre escrito ya lo tiene otra etiqueta (sin mayúsculas).
func (g *gestorEtiquetas) duplicada() bool {
	n := strings.TrimSpace(g.nombre.Texto())
	for _, l := range g.lista {
		if n != "" && strings.EqualFold(l.Name, n) {
			return true
		}
	}
	return false
}

// seccion es `SettingsSection plain`: el título en `sytue-h4`, la descripción en
// `text-sm text-muted leading-relaxed` y el contenido a `gap-3`.
func seccion(titulo, descripcion string, ancho float32, dentro ...fyne.CanvasObject) fyne.CanvasObject {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	cabeza := container.New(ui.Columna{Hueco: tema.Espacio(1)},
		ui.NuevoRotulo(titulo, tema.TextoLG, ui.SemiNegrita, "text"),
		ui.NuevoParrafo(descripcion, tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}, ui.Normal, "text-muted", v).A(ancho))
	return container.New(ui.Columna{Hueco: huecoDentroSeccion}, append([]fyne.CanvasObject{cabeza}, dentro...)...)
}

// pintar rehace el cuerpo con el estado de ahora.
func (g *gestorEtiquetas) pintar() {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	ayuda := ui.NuevaAyudaEnLinea(t.T("labels.whatBody", nil)).ConTono(ui.AyudaInfo).ConTitulo(t.T("labels.whatTitle", nil))

	var tuyas fyne.CanvasObject
	switch {
	case g.estado == Cargando:
		rueda := ui.NuevaRueda(16, "text-subtle", v)
		rueda.Rotulo = t.T("labels.loading", nil)
		tuyas = container.New(ui.Fila{Hueco: tema.Espacio(2)}, rueda,
			ui.NuevoRotulo(t.T("common.loading", nil), tema.TextoSM, ui.Normal, "text-subtle"))
	case g.estado == Error:
		tuyas = container.New(ui.Columna{Hueco: tema.Espacio(2)},
			ui.NuevoRotulo(t.T("labels.loadError", nil), tema.TextoSM, ui.Normal, "text"),
			ui.NuevoBoton(t.T("common.retry", nil), g.cargar).Con(ui.Secundario, ui.TallaSM))
	case len(g.lista) == 0:
		vacio := ui.NuevoVacio(t.T("labels.emptyTitle", nil), t.T("labels.emptyDescription", nil))
		vacio.Ilustracion = iconos.Trazo(iconos.Cubo, 32, "text-subtle", v).Objeto()
		vacio.Accion = ui.NuevoBoton(t.T("labels.emptyAction", nil), func() {
			if c := g.b.P.Lienzo(); c != nil {
				c.Focus(g.nombre)
			}
		}).Con(ui.Secundario, ui.TallaMD)
		tuyas = vacio
	default:
		filas := make([]fyne.CanvasObject, 0, len(g.lista))
		for _, l := range g.lista {
			filas = append(filas, g.fila(l))
		}
		tuyas = enCajaConFiletes(filas)
	}

	dup := g.duplicada()
	pista := t.T("labels.nameHint", nil)
	err := ""
	if dup {
		err, pista = t.T("labels.nameDuplicate", nil), ""
	}
	g.nombre.Invalida = dup
	g.nombre.Refresh()
	anadir := ui.NuevoBoton(t.T("labels.addLabel", nil), g.crear)
	anadir.Icono, anadir.Cargando = iconos.Mas, g.creando
	anadir.Inerte = strings.TrimSpace(g.nombre.Texto()) == "" || dup
	nueva := seccion(t.T("labels.newTitle", nil), t.T("labels.newDescription", nil), g.ancho,
		ui.GrupoCampo(t.T("common.name", nil), false, g.nombre, pista, err),
		ui.GrupoCampo(t.T("labels.colour", nil), false, selectorColor(t, g.color, func(c string) { g.color = c; g.pintar() }),
			t.T("labels.colourHint", nil), ""),
		container.New(&extremos{}, canvas.NewRectangle(color.NRGBA{}), anadir))

	g.caja.Objects = []fyne.CanvasObject{
		ayuda,
		seccion(t.T("labels.yoursTitle", nil), t.T("labels.yoursDescription", nil), g.ancho, tuyas),
		container.New(ui.Columna{Hueco: aireSeccionNueva}, canvasFilete(v), nueva),
	}
	g.caja.Refresh()
	if g.d != nil {
		g.d.Recolocar() // el diálogo se vuelve a medir con su contenido nuevo
	}
}

// fila es `LabelRow`: el punto, el nombre editable, el color y borrar (que pregunta en
// la propia fila).
func (g *gestorEtiquetas) fila(l api.MailLabel) fyne.CanvasObject {
	t := g.b.P.T
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	punto := canvas.NewCircle(ColorEtiqueta(l.Color, v))
	punto.Resize(fyne.NewSize(puntoFilaEtiqueta, puntoFilaEtiqueta))
	marco := container.NewWithoutLayout(punto)
	marco.Resize(punto.Size())
	nombre := ui.NuevaEntrada("")
	nombre.PonerTexto(l.Name)
	nombre.AlEnfocar = func(si bool) {
		if !si {
			g.renombrar(l, strings.TrimSpace(nombre.Texto()))
		}
	}
	colores := selectorColor(t, l.Color, func(c string) { g.guardar(l, l.Name, c) })
	var fin fyne.CanvasObject
	if g.borrando == l.ID {
		si := ui.NuevoBoton(t.T("common.yes", nil), func() { g.borrar(l) }).Con(ui.Peligro, ui.TallaSM)
		no := ui.NuevoBoton(t.T("common.no", nil), func() { g.borrando = ""; g.pintar() }).Con(ui.Secundario, ui.TallaSM)
		fin = container.New(ui.Fila{Hueco: tema.Espacio(1.5)},
			ui.NuevoRotulo(t.T("labels.confirmDelete", nil), tema.TextoXS, ui.Normal, "text-muted"), si, no)
	} else {
		x := ui.NuevoBoton(t.T("labels.deleteLabel", i18n.Vars{"label": l.Name}), func() { g.borrando = l.ID; g.pintar() }).
			Con(ui.Fantasma, ui.TallaSM)
		x.SoloIcono, x.Icono, x.TamIcono = true, iconos.Cerrar, 16
		fin = x
	}
	tel := enTelefono(g.b)
	return container.New(ui.Margen{Izq: margenXFilaEtiqueta, Der: margenXFilaEtiqueta, Arr: margenYFilaEtiqueta, Abj: margenYFilaEtiqueta},
		container.New(&filaEtiqueta{lado: puntoFilaEtiqueta, coloresAbajo: tel, finAbajo: tel && g.borrando == l.ID, ancho: g.ancho - 2*margenXFilaEtiqueta}, marco, nombre, colores, fin))
}

// filaEtiqueta reparte `flex items-center gap-3`: el punto, el nombre `flex-1`, el
// selector y el final, centrados en vertical.
//
// La usa también la fila de «Carpetas», que es la misma con el glifo de 16 en vez del
// punto y el recuento en vez del selector: `lado` es el ancho de lo primero.
//
// En el teléfono (`max-md:flex-wrap`) se parte: `coloresAbajo` lleva el selector a su
// propia línea, debajo del nombre y con su sangría (`max-md:order-last basis-full
// pl-6`), y `finAbajo` la confirmación de borrar (`max-md:basis-full justify-end`). En
// una línea, el nombre se quedaba en 26 px y desaparecía.
type filaEtiqueta struct {
	lado                   float32
	coloresAbajo, finAbajo bool
	// ancho es el de la fila en el diálogo, para el mínimo con los colores abajo (el
	// flujo se parte según el ancho y `MinSize` no lo recibe).
	ancho float32
}

// partes son los altos de la línea de arriba, la de la confirmación y la de los colores.
func (f filaEtiqueta) partes(objs []fyne.CanvasObject, ancho float32) (arriba, medio, abajo float32) {
	nombre, colores, fin := objs[1], objs[2], objs[3]
	arriba = nombre.MinSize().Height
	if !f.finAbajo {
		arriba = max(arriba, fin.MinSize().Height)
	} else {
		medio = fin.MinSize().Height
	}
	if f.coloresAbajo {
		abajo = ui.AltoA(colores, max(ancho-f.lado-huecoFilaEtiqueta, 0))
	} else {
		arriba = max(arriba, colores.MinSize().Height)
	}
	return arriba, medio, abajo
}

func (f filaEtiqueta) alto(objs []fyne.CanvasObject, ancho float32) float32 {
	a, m, b := f.partes(objs, ancho)
	h := a
	for _, x := range []float32{m, b} {
		if x > 0 {
			h += huecoFilaEtiqueta + x
		}
	}
	return h
}

// AltoCon: con los colores abajo, el alto depende del ancho (el flujo se parte).
func (f filaEtiqueta) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	return f.alto(objs, ancho)
}

func (f filaEtiqueta) MinSize(objs []fyne.CanvasObject) fyne.Size {
	if f.coloresAbajo || f.finAbajo {
		return fyne.NewSize(0, f.alto(objs, f.ancho))
	}
	h := float32(0)
	for _, o := range objs[1:] {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(0, h)
}

func (f filaEtiqueta) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if f.coloresAbajo || f.finAbajo {
		f.partida(objs, t)
		return
	}
	punto, nombre, colores, fin := objs[0], objs[1], objs[2], objs[3]
	centro := func(h float32) float32 { return (t.Height - h) / 2 }
	punto.Move(fyne.NewPos(0, centro(f.lado)))
	punto.Resize(fyne.NewSize(f.lado, f.lado))
	mf := fin.MinSize()
	fin.Move(fyne.NewPos(t.Width-mf.Width, centro(mf.Height)))
	fin.Resize(mf)
	mc := colores.MinSize()
	cx := t.Width - mf.Width - huecoFilaEtiqueta - mc.Width
	colores.Move(fyne.NewPos(cx, centro(mc.Height)))
	colores.Resize(mc)
	x := f.lado + huecoFilaEtiqueta
	hn := nombre.MinSize().Height
	nombre.Move(fyne.NewPos(x, centro(hn)))
	nombre.Resize(fyne.NewSize(max(cx-huecoFilaEtiqueta-x, 0), hn))
}

// enTelefono es el `max-md:` de la web: el lienzo por debajo de 768.
func enTelefono(b *Buzon) bool {
	l := b.P.Lienzo()
	return l != nil && l.Size().Width > 0 && l.Size().Width < ui.Estrecho
}

// partida es la fila del teléfono: punto, nombre y (si cabe) borrar o el recuento
// arriba; la confirmación y los colores, cada uno en su línea.
func (f filaEtiqueta) partida(objs []fyne.CanvasObject, t fyne.Size) {
	punto, nombre, colores, fin := objs[0], objs[1], objs[2], objs[3]
	arriba, medio, abajo := f.partes(objs, t.Width)
	centro := func(h float32) float32 { return (arriba - h) / 2 }
	punto.Move(fyne.NewPos(0, centro(f.lado)))
	punto.Resize(fyne.NewSize(f.lado, f.lado))
	derecha := t.Width
	if !f.finAbajo {
		mf := fin.MinSize()
		fin.Move(fyne.NewPos(t.Width-mf.Width, centro(mf.Height)))
		fin.Resize(mf)
		derecha -= mf.Width + huecoFilaEtiqueta
	}
	if !f.coloresAbajo {
		mc := colores.MinSize()
		colores.Move(fyne.NewPos(derecha-mc.Width, centro(mc.Height)))
		colores.Resize(mc)
		derecha -= mc.Width + huecoFilaEtiqueta
	}
	x := f.lado + huecoFilaEtiqueta
	hn := nombre.MinSize().Height
	nombre.Move(fyne.NewPos(x, centro(hn)))
	nombre.Resize(fyne.NewSize(max(derecha-x, 0), hn))
	y := arriba
	if f.finAbajo {
		y += huecoFilaEtiqueta
		mf := fin.MinSize()
		fin.Move(fyne.NewPos(t.Width-mf.Width, y))
		fin.Resize(mf)
		y += medio
	}
	if f.coloresAbajo {
		y += huecoFilaEtiqueta
		colores.Move(fyne.NewPos(x, y))
		colores.Resize(fyne.NewSize(max(t.Width-x, 0), abajo))
	}
}

func (g *gestorEtiquetas) crear() {
	nombre := strings.TrimSpace(g.nombre.Texto())
	if nombre == "" || g.duplicada() || g.creando {
		return
	}
	g.creando = true
	g.pintar()
	tono := g.color
	pedir(g.b, func(ctx context.Context) (api.MailLabel, error) {
		return g.b.S.CreateLabel(ctx, nombre, tono)
	}, func(_ api.MailLabel, err error) {
		g.creando = false
		if err != nil {
			g.pintar()
			g.b.avisar(ui.AvisoPeligro, "labels.createFailed", nil)
			return
		}
		g.nombre.PonerTexto("")
		g.cambiado()
		g.b.avisar(ui.AvisoExito, "labels.created", nil)
		g.cargar()
	})
}

// renombrar es `commitName`: vacío o igual, se deja como estaba.
func (g *gestorEtiquetas) renombrar(l api.MailLabel, nombre string) {
	if nombre == "" || nombre == l.Name {
		return
	}
	g.guardar(l, nombre, l.Color)
}

// guardar manda nombre y color JUNTOS: el servicio sustituye, y mandar uno borraría el
// otro. Optimista, y de vuelta si falla.
func (g *gestorEtiquetas) guardar(l api.MailLabel, nombre, tono string) {
	antes := append([]api.MailLabel(nil), g.lista...)
	for i := range g.lista {
		if g.lista[i].ID == l.ID {
			g.lista[i].Name, g.lista[i].Color = nombre, tono
		}
	}
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.UpdateLabel(ctx, l.ID, nombre, tono)
	}, func(_ struct{}, err error) {
		if err != nil {
			g.lista = antes
			g.pintar()
			g.b.avisar(ui.AvisoPeligro, "labels.saveFailed", nil)
			return
		}
		g.cambiado()
	})
}

func (g *gestorEtiquetas) borrar(l api.MailLabel) {
	antes := append([]api.MailLabel(nil), g.lista...)
	g.borrando = ""
	for i := range g.lista {
		if g.lista[i].ID == l.ID {
			g.lista = append(g.lista[:i:i], g.lista[i+1:]...)
			break
		}
	}
	g.pintar()
	pedir(g.b, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, g.b.S.DeleteLabel(ctx, l.ID)
	}, func(_ struct{}, err error) {
		if err != nil {
			g.lista = antes
			g.pintar()
			g.b.avisar(ui.AvisoPeligro, "labels.deleteFailed", nil)
			return
		}
		g.cambiado()
	})
}
