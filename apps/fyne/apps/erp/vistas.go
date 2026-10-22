// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package erp

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # Las otras dos formas de la lista: tarjetas y, en móvil, filas de resumen
//
// Medido en el arnés del frontal (2026-09-23):
//
//	grupo de vista  135,8 × 29,5: borde, `p-0.5` y dos botones de 24 (`min-h-[24px]`)
//	tarjeta         277,67 × 175,75 a tres columnas (`p-4 rounded-lg border`)
//	fila de móvil   348 × 70 (dos líneas de campos) o 56 (una), `px-2 py-3`

// Medidas del grupo de vista (`role="group" aria-label={list.viewOptions}`).
const (
	// GrupoVistaP es el `p-0.5` del grupo.
	GrupoVistaP float32 = 0.5 * 0.25 * tema.Rem
	// OpcionVistaPX es el `px-2.5` de cada botón y HuecoOpcionVista su `gap-1`.
	OpcionVistaPX    float32 = 2.5 * 0.25 * tema.Rem
	HuecoOpcionVista float32 = 1 * 0.25 * tema.Rem
	// IconoOpcionVista es el `size={13}` de su icono.
	IconoOpcionVista float32 = 13
	// RadioOpcionVista es el `rounded` (0,25 rem).
	RadioOpcionVista float32 = 0.25 * tema.Rem
)

// AltoGrupoVista es el grupo entero: dos bordes, dos `p-0.5` y el botón de 24. Medido:
// 29,5.
var AltoGrupoVista = ui.LadoIconoSM + 2*GrupoVistaP + 2*tema.Borde

// GrupoVista es el selector Tabla/Tarjetas de la cabecera. Dos botones con
// `aria-pressed` dentro de una caja perfilada: el elegido lleva `bg-selected` y peso
// 600, el otro va apagado.
type GrupoVista struct {
	ui.Pulsable
	Tabla, Tarjetas string
	EnTarjetas      bool
	AlElegir        func(tarjetas bool)
}

// NuevoGrupoVista crea el grupo. `nombre` es su `aria-label`.
func NuevoGrupoVista(nombre, tabla, tarjetas string, enTarjetas bool, alElegir func(bool)) *GrupoVista {
	g := &GrupoVista{Tabla: tabla, Tarjetas: tarjetas, EnTarjetas: enTarjetas, AlElegir: alElegir}
	g.Nombre = nombre
	g.ExtendBaseWidget(g)
	return g
}

func pesoOpcion(activa bool) ui.Peso {
	if activa {
		return ui.SemiNegrita
	}
	return ui.Medio
}

// anchoOpcion es un botón: `px-2.5`, icono de 13, `gap-1` y rótulo.
func anchoOpcion(rotulo string, activa bool) float32 {
	return 2*OpcionVistaPX + IconoOpcionVista + HuecoOpcionVista + ui.Medir(rotulo, tema.TextoXS, pesoOpcion(activa)).Width
}

// MinSize es el grupo con sus dos botones. Medido: 59,03 + 71,27 dentro de 135,8.
func (g *GrupoVista) MinSize() fyne.Size {
	g.ExtendBaseWidget(g)
	ancho := anchoOpcion(g.Tabla, !g.EnTarjetas) + anchoOpcion(g.Tarjetas, g.EnTarjetas)
	return fyne.NewSize(ancho+2*GrupoVistaP+2*tema.Borde, AltoGrupoVista)
}

// Tapped elige el botón que cae bajo el puntero.
func (g *GrupoVista) Tapped(e *fyne.PointEvent) {
	tarjetas := e.Position.X > tema.Borde+GrupoVistaP+anchoOpcion(g.Tabla, !g.EnTarjetas)
	if g.AlElegir != nil && tarjetas != g.EnTarjetas {
		g.AlElegir(tarjetas)
	}
}

// CreateRenderer construye la caja y los dos botones.
func (g *GrupoVista) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &grupoVistaRenderer{
		g:    g,
		aro:  ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
		caja: ui.NuevaCaja(tema.Color("surface-raised", v), tema.Color("border", v), tema.RadioMD),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	for i, rot := range []string{g.Tabla, g.Tarjetas} {
		activa := (i == 1) == g.EnTarjetas
		ic := iconos.Tabla
		if i == 1 {
			ic = iconos.Rejilla
		}
		op := &opcionVista{
			sombra: ui.NuevaSombra(tema.SombraXS, RadioOpcionVista),
			fondo:  ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, RadioOpcionVista),
			icono:  iconos.Trazo(ic, IconoOpcionVista, tintaOpcion(activa, v), v),
			texto:  ui.Texto(rot, tema.TextoXS, pesoOpcion(activa), tema.Color(tintaOpcion(activa, v), v)),
			rotulo: rot, activa: activa,
		}
		if activa {
			r.objetos = append(r.objetos, op.sombra.Objeto())
		}
		r.objetos = append(r.objetos, op.fondo.Objetos()...)
		r.objetos = append(r.objetos, op.icono.Objeto(), op.texto)
		r.ops = append(r.ops, op)
	}
	r.Refresh()
	return r
}

func tintaOpcion(activa bool, _ tema.Variante) string {
	if activa {
		return "text"
	}
	return "text-muted"
}

type opcionVista struct {
	sombra *ui.Sombra
	fondo  *ui.Caja
	icono  *iconos.Dibujo
	texto  *canvas.Text
	rotulo string
	activa bool
}

type grupoVistaRenderer struct {
	g       *GrupoVista
	aro     *ui.Aro
	caja    *ui.Caja
	ops     []*opcionVista
	objetos []fyne.CanvasObject
}

func (r *grupoVistaRenderer) MinSize() fyne.Size           { return r.g.MinSize() }
func (r *grupoVistaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *grupoVistaRenderer) Destroy()                     {}

func (r *grupoVistaRenderer) Layout(t fyne.Size) {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	x := tema.Borde + GrupoVistaP
	y := tema.Borde + GrupoVistaP
	for _, op := range r.ops {
		ancho := anchoOpcion(op.rotulo, op.activa)
		tam := fyne.NewSize(ancho, ui.LadoIconoSM)
		op.sombra.Ajustar(fyne.NewPos(x, y), tam, v)
		ui.MoverCaja(op.fondo, fyne.NewPos(x, y), tam)
		op.icono.Colocar(x+OpcionVistaPX, y+(ui.LadoIconoSM-IconoOpcionVista)/2, IconoOpcionVista)
		ui.ColocarTexto(op.texto, tema.TextoXS, x+OpcionVistaPX+IconoOpcionVista+HuecoOpcionVista,
			y+(ui.LadoIconoSM-tema.TextoXS.Linea)/2)
		x += ancho
	}
}

func (r *grupoVistaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.aro.Ver(r.g.Enfocado())
	r.aro.Pintar(v)
	for _, op := range r.ops {
		fondo := color.NRGBA{}
		if op.activa {
			fondo = tema.Color("selected", v)
		}
		op.fondo.Pintar(fondo, color.NRGBA{})
	}
}

// --- Las filas de resumen del móvil -------------------------------------------

// Medidas de una fila de móvil: `px-2 py-3`, la primera línea de 17,5 (la insignia) y
// cada línea de campos de 14 con su `mt-1`.
const (
	FilaMovilPX   float32 = 2 * 0.25 * tema.Rem
	FilaMovilPY   float32 = 3 * 0.25 * tema.Rem
	FilaMovilMT   float32 = 1 * 0.25 * tema.Rem
	HuecoCamposMv float32 = 3 * 0.25 * tema.Rem // el `gap-x-3` entre campos
)

// par es «Cliente: Ferretería Lobo»: el rótulo apagado y el valor en `font-medium`.
type par struct{ rotulo, valor string }

// pares son los campos de la lista de una fila (ni `ref` ni `estado`), sin los vacíos.
func (l *Lista) pares(r Registro, tope int) []par {
	var out []par
	for _, clave := range l.Modelo.Lista {
		if clave == "ref" || clave == "estado" {
			continue
		}
		for _, c := range l.Modelo.Campos {
			if c.Clave != clave {
				continue
			}
			if v := Celda(c, r.Data[clave], l.loc, l.Pagina.Etiquetas); v != SinValor {
				out = append(out, par{c.Etiqueta + ":", v})
			}
		}
		if tope > 0 && len(out) == tope {
			break
		}
	}
	return out
}

// filasMovil es la lista en `md:hidden`: una fila pulsable por registro con su nombre,
// su estado y sus campos, separadas por el filete de `divide-y`.
func (l *Lista) filasMovil(v tema.Variante, ancho float32) (fyne.CanvasObject, float32) {
	lay := &columnaFilas{}
	var hijos []fyne.CanvasObject
	for i, r := range l.registros() {
		reg := r
		f := NuevaFilaERP(NombreDe(r), func() {
			if l.AlAbrir != nil {
				l.AlAbrir(reg)
			}
		})
		fila := &filaMovil{fondo: f}
		objs := []fyne.CanvasObject{f}
		fila.nombre = ui.Texto(NombreDe(r), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
		fila.completo = NombreDe(r)
		objs = append(objs, fila.nombre)
		if r.Estado != "" {
			t := RotuloEstado(l.Modelo, r.Estado)
			fila.insignia = ui.NuevaInsignia(t, tonosEstado[Tono(r.Estado)])
			objs = append(objs, fila.insignia)
		}
		var ps []pieza
		for _, p := range l.pares(r, 0) {
			ps = append(ps, parPieza(p, v))
		}
		for _, p := range ps {
			objs = append(objs, p.obj)
		}
		fila.campos = ps
		anchos, altos := medidasDe(ps)
		fila.pos, fila.tam = Envolver(anchos, altos, HuecoCamposMv, 0, max(ancho-2*FilaMovilPX, 0))
		alto := 2*FilaMovilPY + ui.MedidaInsignia("·").Height
		if len(ps) > 0 {
			alto += FilaMovilMT + fila.tam.Height
		}
		c := container.New(fila, objs...)
		lay.filas = append(lay.filas, bloque{c, alto, 0})
		hijos = append(hijos, c)
		if i > 0 {
			d := canvas.NewRectangle(tema.Color("border-subtle", v))
			lay.filetes = append(lay.filetes, d)
			hijos = append(hijos, d)
		}
	}
	return container.New(lay, hijos...), lay.alto()
}

// parPieza arma «Rótulo: valor» como un contenedor de dos textos.
func parPieza(p par, v tema.Variante) pieza {
	rot := ui.Texto(p.rotulo+" ", tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
	val := ui.Texto(p.valor, tema.TextoXS, ui.Medio, tema.Color("text", v))
	ar := ui.Medir(p.rotulo+" ", tema.TextoXS, ui.Normal).Width
	av := ui.Medir(p.valor, tema.TextoXS, ui.Medio).Width
	c := container.New(&dosTextos{a: rot, b: val, xb: ar}, rot, val)
	return pieza{c, ar + av, tema.TextoXS.Linea, false}
}

// dosTextos pone dos textos de la misma línea uno detrás de otro.
type dosTextos struct {
	a, b *canvas.Text
	xb   float32
}

func (d *dosTextos) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(0, tema.TextoXS.Linea)
}

func (d *dosTextos) Layout(_ []fyne.CanvasObject, _ fyne.Size) {
	ui.ColocarTexto(d.a, tema.TextoXS, 0, 0)
	ui.ColocarTexto(d.b, tema.TextoXS, d.xb, 0)
}

// filaMovil coloca una fila de móvil.
type filaMovil struct {
	fondo    *FilaERP
	nombre   *canvas.Text
	completo string
	insignia *ui.Insignia
	campos   []pieza
	pos      []fyne.Position
	tam      fyne.Size
}

func (f *filaMovil) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (f *filaMovil) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	f.fondo.Move(fyne.NewPos(0, 0))
	f.fondo.Resize(t)
	linea := ui.MedidaInsignia("·").Height
	der := t.Width - FilaMovilPX
	if f.insignia != nil {
		m := ui.MedidaInsignia(f.insignia.Texto)
		f.insignia.Resize(m)
		f.insignia.Move(fyne.NewPos(der-m.Width, FilaMovilPY))
		der -= m.Width + HuecoCabecera
	}
	ui.Truncar(f.nombre, f.completo, tema.TextoSM, ui.SemiNegrita, der-FilaMovilPX)
	ui.ColocarTexto(f.nombre, tema.TextoSM, FilaMovilPX, FilaMovilPY+(linea-tema.TextoSM.Linea)/2)
	colocarPiezas(f.campos, f.pos, FilaMovilPX, FilaMovilPY+linea+FilaMovilMT)
}

// columnaFilas apila bloques de alto conocido con un filete de un píxel entre ellos.
type columnaFilas struct {
	filas   []bloque
	filetes []*canvas.Rectangle
}

func (c *columnaFilas) alto() float32 {
	var a float32
	for i, f := range c.filas {
		a += f.alto
		if i > 0 {
			a += tema.Borde
		}
	}
	return a
}

func (c *columnaFilas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto()) }

func (c *columnaFilas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	var y float32
	for i, f := range c.filas {
		if i > 0 {
			c.filetes[i-1].Move(fyne.NewPos(0, y))
			c.filetes[i-1].Resize(fyne.NewSize(t.Width, tema.Borde))
			y += tema.Borde
		}
		f.obj.Move(fyne.NewPos(0, y))
		f.obj.Resize(fyne.NewSize(t.Width, f.alto))
		y += f.alto
	}
}

// --- Las tarjetas -------------------------------------------------------------

// Medidas de una tarjeta de registro (`Card p-4`, ver el comentario del fichero).
const (
	TarjetaRegP       float32 = 4 * 0.25 * tema.Rem
	TarjetaRegMB      float32 = 2 * 0.25 * tema.Rem // el `mb-2` de la cabecera y del título
	TarjetaCamposMB   float32 = 4 * 0.25 * tema.Rem // el `mb-4` de los campos
	TarjetaCamposGap  float32 = 1 * 0.25 * tema.Rem // su `space-y-1`
	TarjetaPiePT      float32 = 3 * 0.25 * tema.Rem // el `pt-3` del pie, tras su filete
	HuecoTarjetaRefCs float32 = 2 * 0.25 * tema.Rem // el `gap-2` entre casilla y referencia
)

// altoTarjeta es lo que ocupa una tarjeta con `n` campos: `p-4`, la cabecera de 17,5
// con su `mb-2`, el título con el suyo, los campos con su `space-y-1` y `mb-4`, y el
// pie con su filete, su `pt-3` y el botón `xs`. Medido: 175,75 con tres, 158,25 con dos.
func altoTarjeta(n int) float32 {
	alto := 2*TarjetaRegP + 2*tema.Borde + ui.MedidaInsignia("·").Height + TarjetaRegMB +
		tema.TextoSM.Linea + TarjetaRegMB + TarjetaCamposMB + tema.Borde + TarjetaPiePT + ui.LadoIconoSM
	if n > 0 {
		alto += float32(n)*tema.TextoXS.Linea + float32(n-1)*TarjetaCamposGap
	}
	return alto
}

// CortesTarjetasReg es `grid gap-3 sm:grid-cols-2 lg:grid-cols-3`.
var CortesTarjetasReg = ui.Cortes{1, 1, 2, 2, 3, 3}

// tarjetas es la rejilla de la vista de tarjetas. Las filas de la rejilla miden lo que
// su tarjeta más alta (`align-items: stretch`).
func (l *Lista) tarjetas(v tema.Variante) (fyne.CanvasObject, float32) {
	cols := columnasTarjetas(l.Ventana)
	rs := l.registros()
	lay := &rejillaTarjetas{cols: cols}
	hijos := make([]fyne.CanvasObject, 0, len(rs))
	for i, r := range rs {
		t := l.tarjeta(v, r)
		hijos = append(hijos, t)
		lay.tarjetas = append(lay.tarjetas, t)
		lay.altos = append(lay.altos, altoTarjeta(len(l.pares(r, 3))))
		_ = i
	}
	return container.New(lay, hijos...), lay.alto()
}

// columnasTarjetas son las columnas que caben con esta ventana: los cortes `sm` y `lg`
// miden la VENTANA con el rem de 16 (640 y 1024).
func columnasTarjetas(ventana float32) int {
	switch {
	case ventana == 0 || ventana >= 1024:
		return 3
	case ventana >= ui.CorteSM:
		return 2
	}
	return 1
}

// rejillaTarjetas reparte las tarjetas en columnas iguales con `gap-3`.
type rejillaTarjetas struct {
	cols     int
	tarjetas []fyne.CanvasObject
	altos    []float32
}

func (g *rejillaTarjetas) filas() []float32 {
	var out []float32
	for i := 0; i < len(g.altos); i += g.cols {
		fila := float32(0)
		for j := i; j < min(i+g.cols, len(g.altos)); j++ {
			fila = max(fila, g.altos[j])
		}
		out = append(out, fila)
	}
	return out
}

func (g *rejillaTarjetas) alto() float32 {
	var a float32
	for i, f := range g.filas() {
		a += f
		if i > 0 {
			a += HuecoTarjetas
		}
	}
	return a
}

func (g *rejillaTarjetas) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, g.alto()) }

func (g *rejillaTarjetas) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ancho := (t.Width - float32(g.cols-1)*HuecoTarjetas) / float32(g.cols)
	filas := g.filas()
	var y float32
	for i, tj := range g.tarjetas {
		f, c := i/g.cols, i%g.cols
		if c == 0 && f > 0 {
			y += filas[f-1] + HuecoTarjetas
		}
		tj.Move(fyne.NewPos(float32(c)*(ancho+HuecoTarjetas), y))
		tj.Resize(fyne.NewSize(ancho, filas[f]))
	}
}

// tarjeta es una tarjeta de registro: casilla y referencia con el estado, el nombre,
// hasta tres campos y, al pie, «Duplicar» y «Abrir». Se pulsa entera.
func (l *Lista) tarjeta(v tema.Variante, r Registro) fyne.CanvasObject {
	abrir := func() {
		if l.AlAbrir != nil {
			l.AlAbrir(r)
		}
	}
	fondo := NuevaTarjetaReg(NombreDe(r), abrir)
	tj := &tarjetaReg{fondo: fondo}
	objs := []fyne.CanvasObject{fondo}
	cs := NuevaCasillaNativa(NombreDe(r), l.Marcadas[r.ID], func() {
		if l.AlMarcar != nil {
			l.AlMarcar(r.ID)
		}
	})
	tj.casilla = cs
	ref := r.Ref
	if ref == "" {
		ref = SinValor
	}
	tj.ref = textoCelda(ref, celdaERP{mono: true, escalon: tema.TextoXS, tinta: "text-muted"}, v)
	objs = append(objs, cs, tj.ref)
	if r.Estado != "" {
		tj.insignia = ui.NuevaInsignia(RotuloEstado(l.Modelo, r.Estado), tonosEstado[Tono(r.Estado)])
		objs = append(objs, tj.insignia)
	}
	tj.nombre = ui.Texto(NombreDe(r), tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
	tj.completo = NombreDe(r)
	objs = append(objs, tj.nombre)
	for _, p := range l.pares(r, 3) {
		rot := ui.Texto(p.rotulo, tema.TextoXS, ui.Normal, tema.Color("text-subtle", v))
		val := ui.Texto(p.valor, tema.TextoXS, ui.Normal, tema.Color("text", v))
		tj.campos = append(tj.campos, [2]*canvas.Text{rot, val})
		objs = append(objs, rot, val)
	}
	tj.filete = canvas.NewRectangle(tema.Color("border", v))
	objs = append(objs, tj.filete)
	if l.AlDuplicar != nil {
		tj.dup = ui.NuevoBoton(l.t("action.duplicate"), func() { l.AlDuplicar(r) }).Con(ui.Fantasma, ui.TallaXS)
		objs = append(objs, tj.dup)
	}
	tj.abrir = ui.NuevoBoton(l.t("action.open"), abrir).Con(ui.Secundario, ui.TallaXS)
	objs = append(objs, tj.abrir)
	return container.New(tj, objs...)
}

// tarjetaReg coloca una tarjeta de registro.
type tarjetaReg struct {
	fondo      *TarjetaReg
	casilla    *CasillaNativa
	ref        *canvas.Text
	insignia   *ui.Insignia
	nombre     *canvas.Text
	completo   string
	campos     [][2]*canvas.Text
	filete     *canvas.Rectangle
	dup, abrir *ui.Boton
}

func (t *tarjetaReg) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, 0) }

func (t *tarjetaReg) Layout(_ []fyne.CanvasObject, tam fyne.Size) {
	t.fondo.Move(fyne.NewPos(0, 0))
	t.fondo.Resize(tam)
	x0 := tema.Borde + TarjetaRegP
	util := tam.Width - 2*x0
	y := tema.Borde + TarjetaRegP
	cab := ui.MedidaInsignia("·").Height
	t.casilla.Resize(fyne.NewSize(LadoCasillaNativa, LadoCasillaNativa))
	t.casilla.Move(fyne.NewPos(x0, y+(cab-LadoCasillaNativa)/2))
	ui.ColocarTexto(t.ref, tema.TextoXS, x0+LadoCasillaNativa+HuecoTarjetaRefCs, y+(cab-tema.TextoXS.Linea)/2)
	if t.insignia != nil {
		m := ui.MedidaInsignia(t.insignia.Texto)
		t.insignia.Resize(m)
		t.insignia.Move(fyne.NewPos(x0+util-m.Width, y))
	}
	y += cab + TarjetaRegMB
	ui.Truncar(t.nombre, t.completo, tema.TextoSM, ui.SemiNegrita, util)
	ui.ColocarTexto(t.nombre, tema.TextoSM, x0, y)
	y += tema.TextoSM.Linea + TarjetaRegMB
	for i, c := range t.campos {
		if i > 0 {
			y += TarjetaCamposGap
		}
		ui.ColocarTexto(c[0], tema.TextoXS, x0, y)
		ancho := ui.Medir(c[1].Text, tema.TextoXS, ui.Normal).Width
		ui.ColocarTexto(c[1], tema.TextoXS, x0+util-ancho, y)
		y += tema.TextoXS.Linea
	}
	// El pie va ABAJO (`mt-auto`): en una fila de la rejilla todas miden lo que la más
	// alta, y los botones quedan alineados.
	pieY := tam.Height - tema.Borde - TarjetaRegP - ui.LadoIconoSM
	t.filete.Move(fyne.NewPos(x0, pieY-TarjetaPiePT-tema.Borde))
	t.filete.Resize(fyne.NewSize(util, tema.Borde))
	if t.dup != nil {
		m := t.dup.MinSize()
		t.dup.Resize(m)
		t.dup.Move(fyne.NewPos(x0, pieY))
	}
	m := t.abrir.MinSize()
	t.abrir.Resize(m)
	t.abrir.Move(fyne.NewPos(x0+util-m.Width, pieY))
}

// TarjetaReg es el fondo pulsable de una tarjeta: `rounded-lg border bg-surface` y el
// borde a `border-strong` con el puntero encima.
type TarjetaReg struct {
	ui.Pulsable
}

// NuevaTarjetaReg crea el fondo de una tarjeta de registro.
func NuevaTarjetaReg(nombre string, alPulsar func()) *TarjetaReg {
	t := &TarjetaReg{}
	t.Nombre, t.AlPulsar = nombre, alPulsar
	t.ExtendBaseWidget(t)
	return t
}

// MinSize: la tarjeta la mide su rejilla.
func (t *TarjetaReg) MinSize() fyne.Size {
	t.ExtendBaseWidget(t)
	return fyne.NewSize(0, 0)
}

// CreateRenderer construye la caja y el aro.
func (t *TarjetaReg) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &tarjetaRegRenderer{
		t:    t,
		aro:  ui.NuevoAro(tema.RadioLG, ui.SinHueco, v),
		caja: ui.NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioLG),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.Refresh()
	return r
}

type tarjetaRegRenderer struct {
	t       *TarjetaReg
	aro     *ui.Aro
	caja    *ui.Caja
	objetos []fyne.CanvasObject
}

func (r *tarjetaRegRenderer) MinSize() fyne.Size           { return r.t.MinSize() }
func (r *tarjetaRegRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *tarjetaRegRenderer) Destroy()                     {}

func (r *tarjetaRegRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
}

func (r *tarjetaRegRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde := tema.Color("border", v)
	if r.t.Encima() {
		borde = tema.Color("border-strong", v)
	}
	r.caja.Pintar(tema.Color("surface", v), borde)
	r.aro.Ver(r.t.Enfocado())
	r.aro.Pintar(v)
}
