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

// Las piezas propias del ERP y los layouts de la lista. Nada de aquí es un widget
// genérico: lo que existe en `apps/fyne/ui` se importa.

// Pildora es un botón del filtro por estado: silenciosa, y la activa rellena.
//
// Es propia del ERP y no `ui.Boton`: la de la suite más parecida —`Boton` con
// `TallaXS`— mide 24,5 y esta 21, va en `rounded-control` y su activa usa `bg-selected`
// con peso 600, no la variante secundaria. Medido: h 21, `px-3 py-1`, `text-xs`.
type Pildora struct {
	ui.Pulsable

	// Rotulo es el estado, ya traducido, con su recuento entre paréntesis si lo hay.
	Rotulo string
	// Activa es `aria-pressed`.
	Activa bool
	// Ligera es la de los ámbitos de Obligaciones: `px-2.5`, peso normal en reposo y,
	// al pasar, solo la tinta (`hover:text-text`), sin fondo.
	Ligera bool
}

var _ fyne.Widget = (*Pildora)(nil)

// NuevaPildora crea una píldora de filtro.
func NuevaPildora(rotulo string, activa bool, alPulsar func()) *Pildora {
	p := &Pildora{Rotulo: rotulo, Activa: activa}
	p.Nombre, p.AlPulsar = rotulo, alPulsar
	p.ExtendBaseWidget(p)
	return p
}

// AccessibilityLabel es el rótulo.
func (p *Pildora) AccessibilityLabel() string { return p.Rotulo }

func (p *Pildora) peso() ui.Peso {
	if p.Activa {
		return ui.SemiNegrita // font-semibold
	}
	if p.Ligera {
		return ui.Normal
	}
	return ui.Medio // font-medium
}

func (p *Pildora) px() float32 {
	if p.Ligera {
		return PildoraObligPX
	}
	return PildoraPX
}

// MedidaPildora es el rótulo con su `px-3`, por los 21 de alto.
func MedidaPildora(p *Pildora) fyne.Size {
	return fyne.NewSize(ui.Medir(p.Rotulo, tema.TextoXS, p.peso()).Width+2*p.px(), AltoPildora)
}

// CreateRenderer construye fondo y rótulo.
func (p *Pildora) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pildoraRenderer{
		p:      p,
		aro:    ui.NuevoAro(tema.Forma("radius-control"), ui.SinHueco, v).Abierto(),
		caja:   ui.NuevaCaja(color.NRGBA{}, color.NRGBA{}, tema.Forma("radius-control")),
		rotulo: ui.Texto(p.Rotulo, tema.TextoXS, p.peso(), tema.Color("text-muted", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.rotulo)
	r.Refresh()
	return r
}

type pildoraRenderer struct {
	p       *Pildora
	aro     *ui.Aro
	caja    *ui.Caja
	rotulo  *canvas.Text
	objetos []fyne.CanvasObject
}

func (r *pildoraRenderer) MinSize() fyne.Size           { return MedidaPildora(r.p) }
func (r *pildoraRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pildoraRenderer) Destroy()                     {}

func (r *pildoraRenderer) Layout(t fyne.Size) {
	r.aro.Colocar(t)
	r.caja.Redimensionar(t)
	ui.Truncar(r.rotulo, r.p.Rotulo, tema.TextoXS, r.p.peso(), t.Width-2*r.p.px())
	ui.ColocarTexto(r.rotulo, tema.TextoXS, r.p.px(), (t.Height-tema.TextoXS.Linea)/2)
}

func (r *pildoraRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo, tinta := color.NRGBA{}, tema.Color("text-muted", v)
	switch {
	case r.p.Activa:
		fondo, tinta = tema.Color("selected", v), tema.Color("text", v)
	case r.p.Encima() && r.p.Ligera: // hover:text-text
		tinta = tema.Color("text", v)
	case r.p.Encima(): // hover:bg-bg-subtle hover:text-text
		fondo, tinta = tema.Color("bg-subtle", v), tema.Color("text", v)
	}
	r.aro.Ver(r.p.Enfocado())
	r.aro.Pintar(v)
	r.caja.Pintar(fondo, color.NRGBA{})
	r.rotulo.Color, r.rotulo.FontSource = tinta, r.p.peso().Cara()
	r.rotulo.Refresh()
}

// --- Los layouts de la lista ------------------------------------------------

// pieza es una cosa colocada en una fila que salta: su objeto y lo que mide. `texto` es
// un `canvas.Text`, que se coloca por su caja de línea y no por su tamaño.
type pieza struct {
	obj         fyne.CanvasObject
	ancho, alto float32
	texto       bool
}

func piezaDe(o fyne.CanvasObject, m fyne.Size) pieza { return pieza{o, m.Width, m.Height, false} }

// Envolver es `flex flex-wrap items-center` con `gap-x`/`gap-y`: las piezas en fila y,
// la que no cabe en `ancho`, a la siguiente; cada fila con la altura de su pieza más
// alta y las demás centradas en ella. Devuelve dónde va cada una y lo que ocupa todo.
func Envolver(anchos, altos []float32, hx, hy, ancho float32) (pos []fyne.Position, ocupa fyne.Size) {
	pos = make([]fyne.Position, len(anchos))
	var y, x, fila float32
	inicio := 0
	cerrar := func(hasta int) {
		for j := inicio; j < hasta; j++ {
			pos[j].Y = y + (fila-altos[j])/2
		}
		y += fila
	}
	for i := range anchos {
		if i > inicio && x+hx+anchos[i] > ancho {
			cerrar(i)
			y += hy
			x, fila, inicio = 0, 0, i
		} else if i > inicio {
			x += hx
		}
		pos[i].X = x
		x += anchos[i]
		ocupa.Width = max(ocupa.Width, x)
		fila = max(fila, altos[i])
	}
	if len(anchos) > 0 {
		cerrar(len(anchos))
	}
	ocupa.Height = y
	return pos, ocupa
}

func medidasDe(ps []pieza) (anchos, altos []float32) {
	for _, p := range ps {
		anchos, altos = append(anchos, p.ancho), append(altos, p.alto)
	}
	return anchos, altos
}

// colocarPiezas pone las piezas en (x0, y0) + su posición.
func colocarPiezas(ps []pieza, pos []fyne.Position, x0, y0 float32) {
	for i, p := range ps {
		if t, ok := p.obj.(*canvas.Text); ok && p.texto {
			ui.ColocarTexto(t, escalonDe(t), x0+pos[i].X, y0+pos[i].Y)
			continue
		}
		p.obj.Resize(fyne.NewSize(p.ancho, p.alto))
		p.obj.Move(fyne.NewPos(x0+pos[i].X, y0+pos[i].Y))
	}
}

// escalonDe es el escalón de un texto por su tamaño: los de aquí son todos de la escala.
func escalonDe(t *canvas.Text) tema.Escalon {
	for _, e := range []tema.Escalon{tema.TextoXS, tema.TextoSM, tema.TextoBase, tema.TextoLG, tema.TextoXL} {
		if e.Tam == t.TextSize {
			return e
		}
	}
	return tema.TextoBase
}

// pagina coloca los bloques de la lista, uno debajo de otro con el `space-y-4`, dentro
// del `max-w-5xl` centrado de `main`; la banda de aviso va fuera de él, con su `mx-6
// mt-4` sobre `main`.
type pagina struct {
	banda   fyne.CanvasObject
	bloques []bloque
}

var _ fyne.Layout = (*pagina)(nil)

// reparto calcula dónde empieza el contenido y cuánto mide: `mx-auto max-w-5xl px-6`.
func (p *pagina) reparto(ancho float32) (izq, util float32) {
	caja := min(ancho, AnchoContenidoLista)
	return (ancho-caja)/2 + MargenLista, max(caja-2*MargenLista, 0)
}

func (p *pagina) alto() float32 {
	alto := 2 * MargenLista
	if p.banda != nil {
		alto += AvisoMT + AltoBandaAviso
	}
	for i, b := range p.bloques {
		alto += b.alto
		if i > 0 {
			alto += SeparaBloques
		}
	}
	return alto
}

func (p *pagina) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, p.alto()) }

func (p *pagina) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if len(objs) > 0 {
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(t)
	}
	izq, util := p.reparto(t.Width)
	y := float32(0)
	if p.banda != nil {
		p.banda.Move(fyne.NewPos(MargenLista, AvisoMT))
		p.banda.Resize(fyne.NewSize(max(t.Width-2*MargenLista, 0), AltoBandaAviso))
		y = AvisoMT + AltoBandaAviso
	}
	y += MargenLista
	for i, b := range p.bloques {
		if i > 0 {
			y += SeparaBloques
		}
		b.obj.Move(fyne.NewPos(izq, y))
		b.obj.Resize(fyne.NewSize(util, b.alto))
		y += b.alto
	}
}

// banda es el aviso rojo: caja redonda y una línea de `text-sm`.
//
// Guarda el texto ENTERO aparte del `canvas.Text`, y no es una comodidad: `ui.Truncar`
// quiere siempre el original. Pasarle lo que ya hay pintado trunca sobre lo truncado, y
// al repintar se va comiendo letras.
type banda struct {
	caja     *ui.Caja
	texto    *canvas.Text
	completo string
}

func (b *banda) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, AltoBandaAviso) }

func (b *banda) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	b.caja.Redimensionar(t)
	ui.Truncar(b.texto, b.completo, tema.TextoSM, ui.Normal, t.Width-2*AvisoPX)
	ui.ColocarTexto(b.texto, tema.TextoSM, AvisoPX, (t.Height-tema.TextoSM.Linea)/2)
}

// cabeceraLista coloca el título con sus insignias (`izq`) y la barra (`der`): en una
// fila con la barra pegada a la derecha si caben, y si no, la barra debajo. Dentro de
// cada lado las piezas saltan también (`flex-wrap gap-2`).
type cabeceraLista struct {
	izq, der []pieza
	filete   *canvas.Rectangle
	// Calculado por `colocar`.
	posI, posD   []fyne.Position
	tamI, tamD   fyne.Size
	dxD, dyD, dy float32
	alto         float32
}

var _ fyne.Layout = (*cabeceraLista)(nil)

// colocar hace el reparto para `ancho`. Se llama al construir, porque el alto de la
// cabecera cambia con él y la página tiene que saberlo antes de colocar lo de debajo.
func (c *cabeceraLista) colocar(ancho float32) {
	ai, hi := medidasDe(c.izq)
	ad, hd := medidasDe(c.der)
	_, largoI := Envolver(ai, hi, HuecoCabecera, HuecoCabecera, 1e9)
	_, largoD := Envolver(ad, hd, HuecoCabecera, HuecoCabecera, 1e9)
	if largoI.Width+HuecoCabeceraFila+largoD.Width <= ancho {
		c.posI, c.tamI = Envolver(ai, hi, HuecoCabecera, HuecoCabecera, ancho)
		c.posD, c.tamD = Envolver(ad, hd, HuecoCabecera, HuecoCabecera, ancho)
		fila := max(c.tamI.Height, c.tamD.Height)
		c.dy = (fila - c.tamI.Height) / 2
		c.dxD, c.dyD = ancho-c.tamD.Width, (fila-c.tamD.Height)/2
		c.alto = fila + PieCabecera + tema.Borde
		return
	}
	c.posI, c.tamI = Envolver(ai, hi, HuecoCabecera, HuecoCabecera, ancho)
	c.posD, c.tamD = Envolver(ad, hd, HuecoCabecera, HuecoCabecera, ancho)
	c.dy, c.dxD, c.dyD = 0, 0, c.tamI.Height+HuecoCabeceraFila
	c.alto = c.dyD + c.tamD.Height + PieCabecera + tema.Borde
}

func (c *cabeceraLista) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, c.alto) }

func (c *cabeceraLista) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	c.filete.Resize(fyne.NewSize(t.Width, tema.Borde))
	colocarPiezas(c.izq, c.posI, 0, c.dy)
	colocarPiezas(c.der, c.posD, c.dxD, c.dyD)
}

// barraLote coloca la barra de acciones en lote: la cuenta y «Deseleccionar» a la
// izquierda y las acciones a la derecha (`justify-between`), saltando como la cabecera.
type barraLote struct {
	caja     *ui.Caja
	sombra   *ui.Sombra
	v        tema.Variante
	izq, der []pieza
	// juntos son las primeras piezas de `der` que van pegadas con `gap-1` (el rótulo y
	// el selector de estado).
	juntos     int
	posI, posD []fyne.Position
	tamI, tamD fyne.Size
	dxD, dyD   float32
	dyI        float32
	alto       float32
}

// HuecoLoteJunto es el `gap-1` entre «Cambiar estado:» y su selector.
const HuecoLoteJunto float32 = 1 * 0.25 * tema.Rem

func (b *barraLote) colocar(ancho float32) {
	util := max(ancho-2*LotePX-2*tema.Borde, 0)
	ai, hi := medidasDe(b.izq)
	ad, hd := medidasDe(b.der)
	// El rótulo y el selector son UN hijo del flex (`flex items-center gap-1`).
	if b.juntos == 2 && len(ad) >= 2 {
		ad = append([]float32{ad[0] + HuecoLoteJunto + ad[1]}, ad[2:]...)
		hd = append([]float32{max(hd[0], hd[1])}, hd[2:]...)
	}
	b.posI, b.tamI = Envolver(ai, hi, HuecoCabecera, HuecoCabecera, util)
	posD, tamD := Envolver(ad, hd, HuecoCabecera, HuecoCabecera, util)
	if b.juntos == 2 && len(b.der) >= 2 {
		// Deshace la pieza doble: el selector va detrás de su rótulo.
		p0 := posD[0]
		sel := fyne.NewPos(p0.X+b.der[0].ancho+HuecoLoteJunto, p0.Y)
		rot := fyne.NewPos(p0.X, p0.Y)
		alto := max(b.der[0].alto, b.der[1].alto)
		rot.Y += (alto - b.der[0].alto) / 2
		sel.Y += (alto - b.der[1].alto) / 2
		posD = append([]fyne.Position{rot, sel}, posD[1:]...)
	}
	b.posD, b.tamD = posD, tamD
	if b.tamI.Width+HuecoCabeceraFila+b.tamD.Width <= util {
		fila := max(b.tamI.Height, b.tamD.Height)
		b.dyI, b.dxD, b.dyD = (fila-b.tamI.Height)/2, util-b.tamD.Width, (fila-b.tamD.Height)/2
		b.alto = fila + 2*LotePY + 2*tema.Borde
		return
	}
	b.dyI, b.dxD, b.dyD = 0, 0, b.tamI.Height+HuecoCabeceraFila
	b.alto = b.dyD + b.tamD.Height + 2*LotePY + 2*tema.Borde
}

func (b *barraLote) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, b.alto) }

func (b *barraLote) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	b.sombra.Ajustar(fyne.NewPos(0, 0), t, b.v)
	b.caja.Redimensionar(t)
	x0, y0 := tema.Borde+LotePX, tema.Borde+LotePY
	colocarPiezas(b.izq, b.posI, x0, y0+b.dyI)
	colocarPiezas(b.der, b.posD, x0+b.dxD, y0+b.dyD)
}

// filaFiltros coloca el rótulo y las píldoras con `gap-1`, la papelera con su `ml-2`
// de separación, y «Restablecer vista» a la derecha (`justify-between gap-2`).
type filaFiltros struct {
	piezas []pieza
	// papelera es el índice de la píldora de la papelera, que va apartada; 0 si no
	// hay estados y es la única.
	papelera int
	// limpiar es «Restablecer vista». Nil sin filtros.
	limpiar *ui.Boton
	pos     []fyne.Position
	grupo   fyne.Size
	xL, yL  float32
	alto    float32
}

var _ fyne.Layout = (*filaFiltros)(nil)

// HuecoPildoras es el `gap-1` entre píldoras y SeparaPapelera el `ml-2` que la aparta
// de los estados.
const (
	HuecoPildoras   float32 = 1 * 0.25 * tema.Rem
	SeparaPapelera  float32 = 2 * 0.25 * tema.Rem
	MargenRotuloFil float32 = 1 * 0.25 * tema.Rem // el `mr-1` del rótulo
	HuecoFiltros    float32 = 2 * 0.25 * tema.Rem // el `gap-2` de la fila
)

func (f *filaFiltros) colocar(ancho float32) {
	anchos, altos := medidasDe(f.piezas)
	if f.papelera > 0 {
		// El `ml-2` va en un `<span>` que envuelve la píldora: ocupa como parte de ella.
		anchos[f.papelera] += SeparaPapelera
	}
	f.pos, f.grupo = Envolver(anchos, altos, HuecoPildoras, HuecoPildoras, ancho)
	if f.papelera > 0 {
		f.pos[f.papelera].X += SeparaPapelera
	}
	f.alto = f.grupo.Height
	if f.limpiar == nil {
		return
	}
	m := f.limpiar.MinSize()
	if f.grupo.Width+HuecoFiltros+m.Width <= ancho {
		fila := max(f.grupo.Height, m.Height)
		for i := range f.pos {
			f.pos[i].Y += (fila - f.grupo.Height) / 2
		}
		f.xL, f.yL, f.alto = ancho-m.Width, (fila-m.Height)/2, fila
		return
	}
	f.xL, f.yL = 0, f.grupo.Height+HuecoFiltros
	f.alto = f.yL + m.Height
}

func (f *filaFiltros) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, f.alto) }

func (f *filaFiltros) Layout(_ []fyne.CanvasObject, _ fyne.Size) {
	colocarPiezas(f.piezas, f.pos, 0, 0)
	if f.limpiar != nil {
		m := f.limpiar.MinSize()
		f.limpiar.Resize(m)
		f.limpiar.Move(fyne.NewPos(f.xL, f.yL))
	}
}

// pieLista coloca «N a la vista» y «Ver más» con `gap-3`.
type pieLista struct {
	texto *canvas.Text
	// completo es «N a la vista» sin recortar.
	completo string
	mas      *ui.Boton
	alto     float32
}

var _ fyne.Layout = (*pieLista)(nil)

// HuecoPie es el `gap-3` del pie.
const HuecoPie float32 = 3 * 0.25 * tema.Rem

func (p *pieLista) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, p.alto) }

func (p *pieLista) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	ui.ColocarTexto(p.texto, tema.TextoXS, 0, (p.alto-tema.TextoXS.Linea)/2)
	if p.mas == nil {
		return
	}
	x := ui.Medir(p.completo, tema.TextoXS, ui.Normal).Width + HuecoPie
	m := p.mas.MinSize()
	p.mas.Resize(m)
	p.mas.Move(fyne.NewPos(x, (p.alto-m.Height)/2))
}

// --- El lienzo y el texto que se pulsa -------------------------------------------

// lienzo coloca cada objeto donde se calculó al armar la vista. Las vistas del ERP se
// rehacen cuando cambia la ventana (`vigia`), así que las posiciones no dependen de lo
// que llegue a Layout.
type lienzo struct {
	puestos []puesto
	alto    float32
}

type puesto struct {
	o          fyne.CanvasObject
	x, y, w, h float32
	// texto va por su caja de línea y no por su tamaño.
	texto *canvas.Text
}

func (l *lienzo) poner(o fyne.CanvasObject, x, y, w, h float32) {
	l.puestos = append(l.puestos, puesto{o: o, x: x, y: y, w: w, h: h})
}

func (l *lienzo) texto(t *canvas.Text, x, y float32) {
	l.puestos = append(l.puestos, puesto{o: t, x: x, y: y, texto: t})
}

func (l *lienzo) caja(c *ui.Caja, w, h float32) {
	l.poner(container.New(&soloCaja{c}, c.Objetos()...), 0, 0, w, h)
}

// soloCaja es una `ui.Caja` suelta, colocada como cualquier otro objeto.
type soloCaja struct{ c *ui.Caja }

func (s *soloCaja) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.Size{} }

func (s *soloCaja) Layout(_ []fyne.CanvasObject, t fyne.Size) { s.c.Redimensionar(t) }

func (l *lienzo) pieza(p pieza, x, y float32) {
	if t, ok := p.obj.(*canvas.Text); ok && p.texto {
		l.texto(t, x, y)
		return
	}
	l.poner(p.obj, x, y, p.ancho, p.alto)
}

func (l *lienzo) piezas(ps []pieza, pos []fyne.Position, x0, y0 float32) {
	for i, p := range ps {
		l.pieza(p, x0+pos[i].X, y0+pos[i].Y)
	}
}

func (l *lienzo) objeto(alto float32) (fyne.CanvasObject, float32) {
	l.alto = alto
	objs := make([]fyne.CanvasObject, 0, len(l.puestos))
	for _, p := range l.puestos {
		objs = append(objs, p.o)
	}
	return container.New(l, objs...), alto
}

func (l *lienzo) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(0, l.alto) }

func (l *lienzo) Layout([]fyne.CanvasObject, fyne.Size) {
	for _, p := range l.puestos {
		if p.texto != nil {
			ui.ColocarTexto(p.texto, escalonDe(p.texto), p.x, p.y)
			continue
		}
		p.o.Resize(fyne.NewSize(p.w, p.h))
		p.o.Move(fyne.NewPos(p.x, p.y))
	}
}

// Marca es el triángulo de un `<summary>`.
type Marca uint8

// Las marcas: ninguna, cerrada (▸) y abierta (▾).
const (
	SinMarca Marca = iota
	MarcaCerrada
	MarcaAbierta
)

// AnchoMarca es lo que ocupa el `::marker` del `<summary>`, medido en Chrome: 11,11 a
// 10,5 px y 12,05 a 11,375 (1,058 em).
const AnchoMarca float32 = 1.0583

// Pulsador es un texto que se pulsa y cambia de tinta al pasar: el `<summary>` de un
// `<details>` (con su triángulo), «Abrir el portal» (con su flecha) y las flechas del
// año (solo el icono). Sin fondo: lo que cambia es la tinta, `text-muted` →
// `hover:text-text`.
type Pulsador struct {
	ui.Pulsable

	Texto   string
	Escalon tema.Escalon
	Peso    ui.Peso
	// Token y TokenEncima son la tinta en reposo y al pasar; vacíos, `text-muted` y `text`.
	Token, TokenEncima string
	Marca              Marca
	// Icono va detrás del texto con `gap-1`, o solo.
	Icono     iconos.Icono
	LadoIcono float32
	PX, PY    float32
	Radio     float32
	// Punteado es `underline decoration-dotted` y Subrayado el `underline` de siempre.
	Punteado, Subrayado bool
}

var _ fyne.Widget = (*Pulsador)(nil)

// NuevoPulsador crea el texto pulsable.
func NuevoPulsador(texto string, e tema.Escalon, p ui.Peso, alPulsar func()) *Pulsador {
	x := &Pulsador{Texto: texto, Escalon: e, Peso: p, LadoIcono: 14}
	x.Nombre, x.AlPulsar = texto, alPulsar
	x.ExtendBaseWidget(x)
	return x
}

// AccessibilityLabel es el texto, o el nombre si solo lleva icono.
func (p *Pulsador) AccessibilityLabel() string { return p.Nombre }

func (p *Pulsador) anchoMarca() float32 {
	if p.Marca == SinMarca {
		return 0
	}
	return AnchoMarca * p.Escalon.Tam
}

// MinSize es el contenido con su relleno.
func (p *Pulsador) MinSize() fyne.Size {
	p.ExtendBaseWidget(p)
	w := 2*p.PX + p.anchoMarca()
	h := float32(0)
	if p.Texto != "" {
		w += ui.Medir(p.Texto, p.Escalon, p.Peso).Width
		h = p.Escalon.Linea
	}
	if p.Icono != iconos.Ninguno {
		if p.Texto != "" {
			w += HuecoUno
		}
		w += p.LadoIcono
		h = max(h, p.LadoIcono)
	}
	return fyne.NewSize(w, h+2*p.PY)
}

// rutasMarca son los dos triángulos en la rejilla de 24 de los iconos, medidos en
// Chrome a 8 aumentos y metidos una unidad hacia dentro por el trazo de 2.
var rutasMarca = map[Marca]string{
	MarcaCerrada: "M1 4.6 1 17.1 11.3 10.9Z",
	MarcaAbierta: "M1.7 6.1 14.3 6.1 8 15.9Z",
}

// CreateRenderer arma el anillo, el triángulo, el texto, el icono y el punteado.
func (p *Pulsador) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pulsadorRenderer{p: p, aro: ui.NuevoAro(max(p.Radio, 0), ui.SinHueco, v).Abierto()}
	tinta := tema.Color(p.token(), v)
	r.objetos = r.aro.Objetos()
	if p.Marca != SinMarca {
		r.marca = iconos.TrazoDeRutaRelleno(rutasMarca[p.Marca], p.Escalon.Tam, tinta)
		r.objetos = append(r.objetos, r.marca.Objeto())
	}
	if p.Texto != "" {
		r.texto = ui.Texto(p.Texto, p.Escalon, p.Peso, tinta)
		r.objetos = append(r.objetos, r.texto)
	}
	if p.Icono != iconos.Ninguno {
		r.icono = iconos.TrazoConTinta(p.Icono, p.LadoIcono, tinta)
		r.objetos = append(r.objetos, r.icono.Objeto())
	}
	if p.Punteado && p.Texto != "" {
		n := int(ui.Medir(p.Texto, p.Escalon, p.Peso).Width / 2)
		for range n {
			d := canvas.NewRectangle(tinta)
			r.puntos = append(r.puntos, d)
			r.objetos = append(r.objetos, d)
		}
	}
	if p.Subrayado && p.Texto != "" {
		r.raya = canvas.NewRectangle(tinta)
		r.objetos = append(r.objetos, r.raya)
	}
	r.Refresh()
	return r
}

func (p *Pulsador) token() string {
	if p.Encima() {
		if p.TokenEncima != "" {
			return p.TokenEncima
		}
		return "text"
	}
	if p.Token != "" {
		return p.Token
	}
	return "text-muted"
}

type pulsadorRenderer struct {
	p       *Pulsador
	aro     *ui.Aro
	marca   *iconos.Dibujo
	texto   *canvas.Text
	icono   *iconos.Dibujo
	puntos  []*canvas.Rectangle
	raya    *canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *pulsadorRenderer) MinSize() fyne.Size           { return r.p.MinSize() }
func (r *pulsadorRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pulsadorRenderer) Destroy()                     {}

func (r *pulsadorRenderer) Layout(t fyne.Size) {
	p := r.p
	r.aro.Colocar(t)
	x, e := p.PX, p.Escalon
	alto := t.Height - 2*p.PY
	if r.marca != nil {
		r.marca.Colocar(x, p.PY+(e.Linea-e.Tam)/2, e.Tam)
		x += p.anchoMarca()
	}
	if r.texto != nil {
		y := p.PY + (alto-e.Linea)/2
		ui.ColocarTexto(r.texto, e, x, y)
		// La línea base de Figtree: media interlínea y 0,95 em de ascendente; el
		// subrayado, 0,1 em por debajo.
		base := y + (e.Linea-1.2*e.Tam)/2 + 0.95*e.Tam + 0.1*e.Tam
		for i, d := range r.puntos {
			d.Move(fyne.NewPos(x+float32(2*i), base))
			d.Resize(fyne.NewSize(1, 1))
		}
		if r.raya != nil {
			r.raya.Move(fyne.NewPos(x, base))
			r.raya.Resize(fyne.NewSize(ui.Medir(p.Texto, e, p.Peso).Width, 1))
		}
		x += ui.Medir(p.Texto, e, p.Peso).Width + HuecoUno
	}
	if r.icono != nil {
		r.icono.Colocar(x, p.PY+(alto-p.LadoIcono)/2, p.LadoIcono)
	}
}

func (r *pulsadorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	tinta := tema.Color(r.p.token(), v)
	if r.marca != nil {
		r.marca.TenirCon(tinta)
	}
	if r.texto != nil {
		r.texto.Color = tinta
		r.texto.Refresh()
	}
	if r.icono != nil {
		r.icono.TenirCon(tinta)
	}
	for _, d := range append(r.puntos, r.raya) {
		if d != nil {
			d.FillColor = tinta
			d.Refresh()
		}
	}
	r.aro.Ver(r.p.Enfocado())
	r.aro.Pintar(v)
}
