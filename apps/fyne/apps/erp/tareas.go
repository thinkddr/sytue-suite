// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// # Tareas (`Tareas.tsx`)
//
// Lo que toca hacer, por fecha: las vencidas, las de hoy y las que vienen. Medido en el
// arnés (1280×800, 2026-09-23):
//
//	cabecera   título `text-lg`, frase `text-sm` con `max-w-prose` (463,08), y a la
//	           derecha «Mías · De todos» (114×24,75, botones `px-3 py-1`); `pb-4` y filete
//	grupos     `space-y-6`; título en versalitas, `space-y-2`, lista `rounded-lg border`
//	fila       50,75: `px-4 py-3`, resumen `text-sm font-medium` y debajo `text-xs`;
//	           a la derecha la fecha, «Subir», «Hecha» y «Descartar» con `gap-1.5`

// Medidas de Tareas.
const (
	// TareasGapGrupos es el `space-y-6` de la pantalla.
	TareasGapGrupos float32 = 6 * 0.25 * tema.Rem
	// FilaTareaPX y FilaTareaPY son el `px-4 py-3` de una fila.
	FilaTareaPX float32 = 4 * 0.25 * tema.Rem
	FilaTareaPY float32 = 3 * 0.25 * tema.Rem
	// HuecoAccionesTarea es el `gap-1.5` de la derecha.
	HuecoAccionesTarea float32 = 1.5 * 0.25 * tema.Rem
	// MiasPX y MiasPY son el `px-3 py-1` de «Mías» y «De todos».
	MiasPX float32 = 3 * 0.25 * tema.Rem
	MiasPY float32 = 1 * 0.25 * tema.Rem
	// VacioTareasIlustracion es el `size={96}` de la ilustración del calendario.
	VacioTareasIlustracion float32 = 96
)

// anchoProsa es `max-w-prose` (65ch) a `text-sm`: 541,640625, medido. No sale de medir el
// «0» aquí: el de la Figtree de Fyne es un 2,4 % más ancho que el del navegador (7,30
// contra 7,12) y daría 474,3.
//
// ponytail: un píxel de holgura al repartir. La frase de Tareas en castellano mide 463,67
// aquí y cabe en una línea en la web: es el error del conformador acumulado en 88 letras
// (≈0,007 por letra). Sin la holgura saltaba a dos líneas y la cabecera crecía 15,75.
// Chromium: el max-w-prose de13px alcanza541.640625; la frase natural ocupa529.953125.
func anchoProsa() float32 { return 541.640625 }

// HolguraReparto es ese píxel.
const HolguraReparto float32 = 1

// AltoFilaTarea es una fila: dos líneas (15,75 + 14) y el `py-3`. Medido: 50,75.
var AltoFilaTarea = tema.TextoSM.Linea + tema.TextoXS.Linea + 2*FilaTareaPY

// Tareas es el estado de la pantalla.
type Tareas struct {
	tr  *i18n.Translator
	loc i18n.Locale
	// Mias es el filtro de arriba: las de quien mira (por defecto) o las de todos.
	Mias bool
	// Lista son las pendientes; nil, cargando.
	Lista []api.ERPActividad
	// Ocupada es la tarea cuyo botón está esperando al servicio.
	Ocupada string
	// Ventana decide el reparto.
	Ventana float32
	// Hoy es la fecha de hoy (AAAA-MM-DD); se deja abierta para el guardián.
	Hoy string

	AlMias   func(bool)
	AlAbrir  func(modelo, id string)
	AlCerrar func(a api.ERPActividad, como string)
	AlSubir  func(a api.ERPActividad)
}

// NuevaTareas crea la pantalla con el filtro de «Mías».
func NuevaTareas(tr *i18n.Translator) *Tareas {
	return &Tareas{tr: tr, loc: tr.Locale(), Mias: true}
}

func (t *Tareas) t(clave string) string { return t.tr.T(clave, nil) }

// grupo es una de las tres secciones.
type grupo struct {
	titulo string
	filas  []api.ERPActividad
}

// Grupos reparte las tareas en vencidas, hoy y próximas, y deja fuera las vacías.
func (t *Tareas) Grupos() []grupo {
	hoy := t.Hoy
	if hoy == "" {
		hoy = HoyISO(time.Now())
	}
	gs := []grupo{{titulo: t.t("tasks.overdue")}, {titulo: t.t("tasks.today")}, {titulo: t.t("tasks.upcoming")}}
	for _, a := range t.Lista {
		switch {
		case a.Vence < hoy:
			gs[0].filas = append(gs[0].filas, a)
		case a.Vence == hoy:
			gs[1].filas = append(gs[1].filas, a)
		default:
			gs[2].filas = append(gs[2].filas, a)
		}
	}
	var out []grupo
	for _, g := range gs {
		if len(g.filas) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// Objeto arma la pantalla.
func (t *Tareas) Objeto(v tema.Variante) fyne.CanvasObject {
	ancho := anchoUtilDe(t.Ventana, AnchoContenidoLista)
	p := &pila{}
	cab, altoCab := t.cabecera(v, ancho)
	p.add(cab, altoCab, 0)
	switch {
	case t.Lista == nil:
		alto := float32(2*12*0.25*tema.Rem + 24)
		p.add(centrado(ui.NuevaRueda(24, "text-muted", v), alto), alto, TareasGapGrupos)
	case len(t.Grupos()) == 0:
		x := ui.NuevoVacio(t.t("tasks.allDone"), t.t("tasks.allDoneHint"))
		x.Ilustracion = iconos.Pintar(iconos.VacioCalendario, v).A(VacioTareasIlustracion).Objeto()
		alto := x.MedidaVacio(ancho)
		p.add(centrado(x, alto), alto, TareasGapGrupos)
	default:
		hoy := t.Hoy
		if hoy == "" {
			hoy = HoyISO(time.Now())
		}
		for _, g := range t.Grupos() {
			titulo := ui.NuevoVersal(g.titulo, tema.TextoXS, ui.SemiNegrita, tema.Color("text-subtle", v))
			p.add(titulo.Objeto(), tema.TextoXS.Linea, TareasGapGrupos)
			o, alto := t.lista(v, g.filas, hoy)
			p.add(o, alto, SeccionGap)
		}
	}
	o, alto := p.objeto()
	return enPagina(o, alto, AnchoContenidoLista, MargenLista)
}

// cabecera es el título, la frase y el conmutador de «Mías · De todos». En el móvil el
// conmutador baja (`flex-col sm:flex-row`).
func (t *Tareas) cabecera(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	titulo := ui.Texto(t.t("tasks.title"), tema.TextoLG, ui.SemiNegrita, tema.Color("text", v))
	frase := ui.NuevoParrafo(t.t("tasks.subtitle"), tema.TextoSM, ui.Normal, "text-muted", v)
	grupo := NuevoConmutadorMias(t.t("tasks.title"), t.t("tasks.mine"), t.t("tasks.all"), t.Mias, func(mias bool) {
		if t.AlMias != nil {
			t.AlMias(mias)
		}
	})
	g := grupo.MinSize()
	sm := t.Ventana == 0 || t.Ventana >= ui.CorteSM
	anchoTexto := ancho
	if sm {
		anchoTexto = max(ancho-g.Width-HuecoCabeceraFila, 0)
	}
	anchoFrase := min(anchoTexto, anchoProsa())
	altoFrase := ui.AltoParrafo(frase, anchoFrase+HolguraReparto)
	izq := tema.TextoLG.Linea + TituloMT + altoFrase
	alto := max(izq, g.Height)
	if !sm {
		alto = izq + HuecoCabeceraFila + g.Height
	}
	l := &cabeceraTareas{
		titulo: titulo, frase: frase, grupo: grupo, anchoFrase: anchoFrase, altoFrase: altoFrase, sm: sm,
		filete: canvas.NewRectangle(tema.Color("border", v)), altoIzq: izq,
	}
	alto += CabeceraPB + tema.Borde
	l.alto = alto
	return container.New(l, titulo, frase, grupo, l.filete), alto
}

// TituloMT es el `mt-1` de la frase y CabeceraPB el `pb-4` de la cabecera.
const (
	TituloMT   float32 = 1 * 0.25 * tema.Rem
	CabeceraPB float32 = 4 * 0.25 * tema.Rem
)

type cabeceraTareas struct {
	titulo                *canvas.Text
	frase                 *ui.Parrafo
	grupo                 *ConmutadorMias
	anchoFrase, altoFrase float32
	altoIzq, alto         float32
	sm                    bool
	filete                *canvas.Rectangle
}

func (c *cabeceraTareas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }
func (c *cabeceraTareas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(c.titulo, tema.TextoLG, 0, 0)
	c.frase.Move(fyne.NewPos(0, tema.TextoLG.Linea+TituloMT))
	c.frase.Resize(fyne.NewSize(c.anchoFrase+HolguraReparto, c.altoFrase))
	g := c.grupo.MinSize()
	c.grupo.Resize(g)
	if c.sm {
		c.grupo.Move(fyne.NewPos(t.Width-g.Width, 0))
	} else {
		c.grupo.Move(fyne.NewPos(0, c.altoIzq+HuecoCabeceraFila))
	}
	c.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	c.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
}

// lista es un grupo: `ul.rounded-lg.border.divide-y` con una fila por tarea.
func (t *Tareas) lista(v tema.Variante, filas []api.ERPActividad, hoy string) (fyne.CanvasObject, float32) {
	caja := ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), radioTarjeta())
	col := &columnaFilas{}
	var hijos []fyne.CanvasObject
	for i, a := range filas {
		o, alto := t.fila(v, a, hoy)
		col.filas = append(col.filas, bloque{o, alto, 0})
		hijos = append(hijos, o)
		if i > 0 {
			d := canvas.NewRectangle(tema.Color("border", v))
			col.filetes = append(col.filetes, d)
			hijos = append(hijos, d)
		}
	}
	lista := container.New(col, hijos...)
	return container.New(&enCaja{caja: caja, hijo: lista}, append(caja.Objetos(), lista)...), col.alto() + 2*tema.Borde
}

// fila es una tarea: resumen y, debajo, tipo · registro · responsable; a la derecha la
// fecha y las acciones. En el móvil las acciones bajan (`flex-col sm:flex-row`).
func (t *Tareas) fila(v tema.Variante, a api.ERPActividad, hoy string) (fyne.CanvasObject, float32) {
	act := a
	fecha := diaMes(t.loc, a.Vence)
	der := []pieza{piezaDe(ui.NuevaInsignia(fecha, tonoVence(a.Vence, hoy)), ui.MedidaInsignia(fecha))}
	ocupada := t.Ocupada == a.ID
	if a.Tipo == "subir_documento" && a.Registro != "" {
		s := botonXS(t.tr.T("docs.upload", nil), ui.Primario, iconos.Subir, func() {
			if t.AlSubir != nil {
				t.AlSubir(act)
			}
		})
		s.Cargando = ocupada
		// ponytail: sin selector de ficheros del sistema (el móvil, un Linux sin zenity
		// ni kdialog) el botón está y no abre nada.
		s.Inerte = t.AlSubir == nil
		der = append(der, piezaDe(s, s.MinSize()))
	}
	h := botonXS(t.t("tasks.done"), ui.Secundario, iconos.Tic, func() {
		if t.AlCerrar != nil {
			t.AlCerrar(act, "hecha")
		}
	})
	h.Cargando = ocupada
	d := ui.NuevoBoton(t.t("tasks.cancel"), func() {
		if t.AlCerrar != nil {
			t.AlCerrar(act, "cancelar")
		}
	}).Con(ui.Fantasma, ui.TallaXS)
	d.Inerte = ocupada
	der = append(der, piezaDe(h, h.MinSize()), piezaDe(d, d.MinSize()))

	resumen := ui.Texto(a.Resumen, tema.TextoSM, ui.Medio, tema.Color("text", v))
	// La segunda línea: el tipo, el registro (subrayado, abre su ficha) y, en «De todos»,
	// quién la tiene.
	sub := []pieza{textoPieza(RotuloTipo(a.Tipo, t.tr), tema.TextoXS, ui.Normal, "text-subtle", v)}
	if a.Registro != "" && a.Modelo != "" && a.Modelo != "obligacion" {
		sub = append(sub, textoPieza(" · ", tema.TextoXS, ui.Normal, "text-subtle", v))
		nombre := a.RegistroEtiqueta
		if nombre == "" {
			nombre = t.t("tasks.open")
		}
		en := ui.NuevoEnlace(nombre, tema.TextoXS, ui.Normal, "text-subtle", func() {
			if t.AlAbrir != nil {
				t.AlAbrir(act.Modelo, act.Registro)
			}
		})
		sub = append(sub, piezaDe(en, fyne.NewSize(ui.Medir(nombre, tema.TextoXS, ui.Normal).Width, tema.TextoXS.Linea)))
	}
	if !t.Mias && a.ResponsableNombre != "" {
		sub = append(sub, textoPieza(" · "+a.ResponsableNombre, tema.TextoXS, ui.Normal, "text-subtle", v))
	}
	pos := make([]fyne.Position, 0, len(sub))
	x := float32(0)
	for _, p := range sub {
		pos = append(pos, fyne.NewPos(x, 0))
		x += p.ancho
	}
	sm := t.Ventana == 0 || t.Ventana >= ui.CorteSM
	alto := AltoFilaTarea
	var anchoDer float32
	for i, p := range der {
		anchoDer += p.ancho
		if i > 0 {
			anchoDer += HuecoAccionesTarea
		}
	}
	if !sm {
		alto += HuecoCabecera + ui.LadoIconoSM
	}
	l := &filaTarea{
		resumen: resumen, sub: sub, pos: pos, der: der, anchoDer: anchoDer, sm: sm,
		fondo: canvas.NewRectangle(color.NRGBA{}),
	}
	objs := append([]fyne.CanvasObject{l.fondo, resumen}, objetosDe(sub, der)...)
	return container.New(l, objs...), alto
}

type filaTarea struct {
	fondo    *canvas.Rectangle
	resumen  *canvas.Text
	sub, der []pieza
	pos      []fyne.Position
	anchoDer float32
	sm       bool
}

func (f *filaTarea) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }
func (f *filaTarea) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.fondo.Resize(t)
	x0, y0 := FilaTareaPX, FilaTareaPY
	ui.ColocarTexto(f.resumen, tema.TextoSM, x0, y0)
	colocarPiezas(f.sub, f.pos, x0, y0+tema.TextoSM.Linea)
	xd, yd := t.Width-FilaTareaPX-f.anchoDer, (t.Height-ui.LadoIconoSM)/2
	if !f.sm {
		xd, yd = x0, y0+tema.TextoSM.Linea+tema.TextoXS.Linea+HuecoCabecera
	}
	for _, p := range f.der {
		colocarPieza(p, xd, yd+(ui.LadoIconoSM-p.alto)/2)
		xd += p.ancho + HuecoAccionesTarea
	}
}

// ConmutadorMias es el grupo «Mías · De todos»: dos botones pegados dentro de un
// perfil, el elegido en `bg-selected` y peso 600.
type ConmutadorMias struct {
	ui.Pulsable
	Mias, Todos string
	EnMias      bool
	AlElegir    func(mias bool)
}

// NuevoConmutadorMias crea el grupo. `nombre` es su `aria-label`.
func NuevoConmutadorMias(nombre, mias, todos string, enMias bool, alElegir func(bool)) *ConmutadorMias {
	c := &ConmutadorMias{Mias: mias, Todos: todos, EnMias: enMias, AlElegir: alElegir}
	c.Nombre = nombre
	c.ExtendBaseWidget(c)
	return c
}

func anchoMias(s string, activa bool) float32 {
	return ui.Medir(s, tema.TextoSM, pesoOpcion2(activa)).Width + 2*MiasPX
}

func pesoOpcion2(activa bool) ui.Peso {
	if activa {
		return ui.SemiNegrita
	}
	return ui.Normal
}

// MinSize son los dos botones con el borde. Medido: 114×24,75.
func (c *ConmutadorMias) MinSize() fyne.Size {
	c.ExtendBaseWidget(c)
	return fyne.NewSize(anchoMias(c.Mias, c.EnMias)+anchoMias(c.Todos, !c.EnMias)+2*tema.Borde,
		tema.TextoSM.Linea+2*MiasPY+2*tema.Borde)
}

// Tapped elige el botón que cae bajo el puntero.
func (c *ConmutadorMias) Tapped(e *fyne.PointEvent) {
	mias := e.Position.X < tema.Borde+anchoMias(c.Mias, c.EnMias)
	if c.AlElegir != nil && mias != c.EnMias {
		c.AlElegir(mias)
	}
}

// CreateRenderer construye el perfil y los dos rótulos.
func (c *ConmutadorMias) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &miasRenderer{
		c:     c,
		aro:   ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja:  ui.NuevaCaja(tema.Color("bg", v), tema.Color("border", v), tema.RadioMD),
		sel:   canvas.NewRectangle(tema.Color("selected", v)),
		mias:  ui.Texto(c.Mias, tema.TextoSM, pesoOpcion2(c.EnMias), tema.Color(tintaOpcion(c.EnMias, v), v)),
		todos: ui.Texto(c.Todos, tema.TextoSM, pesoOpcion2(!c.EnMias), tema.Color(tintaOpcion(!c.EnMias, v), v)),
	}
	r.Refresh()
	return r
}

type miasRenderer struct {
	c           *ConmutadorMias
	aro         *ui.Aro
	caja        *ui.Caja
	sel         *canvas.Rectangle
	mias, todos *canvas.Text
}

func (r *miasRenderer) MinSize() fyne.Size { return r.c.MinSize() }
func (r *miasRenderer) Objects() []fyne.CanvasObject {
	return append(append(r.aro.Objetos(), r.caja.Objetos()...), r.sel, r.mias, r.todos)
}
func (r *miasRenderer) Destroy() {}

func (r *miasRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	a1 := anchoMias(r.c.Mias, r.c.EnMias)
	a2 := anchoMias(r.c.Todos, !r.c.EnMias)
	alto := t.Height - 2*tema.Borde
	if r.c.EnMias {
		r.sel.Move(fyne.NewPos(tema.Borde, tema.Borde))
		r.sel.Resize(fyne.NewSize(a1, alto))
	} else {
		r.sel.Move(fyne.NewPos(tema.Borde+a1, tema.Borde))
		r.sel.Resize(fyne.NewSize(a2, alto))
	}
	y := tema.Borde + MiasPY
	ui.ColocarTexto(r.mias, tema.TextoSM, tema.Borde+MiasPX, y)
	ui.ColocarTexto(r.todos, tema.TextoSM, tema.Borde+a1+MiasPX, y)
}

func (r *miasRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Ver(r.c.Enfocado())
	r.aro.Pintar(v)
}
