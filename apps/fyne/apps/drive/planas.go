// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package drive

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// # La fila de las vistas planas
//
// «Recientes», «Destacados», «Compartido conmigo» y «Documentos» no usan la fila del
// explorador: no tienen columnas ni casilla. Son el `<li>` que FileBrowser.tsx repite en
// las cuatro —`group border-border/60 hover:bg-bg-subtle flex items-center gap-3
// border-b px-3 py-2`— con la tesela, el nombre y su línea de meta debajo, y a la
// derecha lo de cada vista: la estrella y el «⋯», el permiso y la descarga, o la hora.
//
// Es UN widget y no un contenedor con botones dentro por lo mismo que `ui.FilaDrive`:
// Fyne da el puntero al Hoverable más profundo, así que al pasar de la fila a uno de sus
// botones la fila recibía `MouseOut` y perdía su `hover:bg-bg-subtle` justo donde la web
// lo mantiene. Aquí la fila sabe en qué zona está el puntero y pinta las dos cosas.
//
// En la web solo el NOMBRE abre (es un `<button>` con `hover:underline`), y una carpeta
// de «Recientes» o «Destacados» no se abre desde ahí: su nombre es texto. Se respeta.

// Medidas de la fila plana.
const (
	// LadoBotonFila es el `h-8 w-8` de los botones del final.
	LadoBotonFila float32 = 8 * 0.25 * tema.Rem
	// HuecoFilaPlana es el `gap-3` entre las piezas de la fila.
	HuecoFilaPlana float32 = 3 * 0.25 * tema.Rem
	// LadoGlifoBotonFila es el `size={16}` de los glifos de esos botones.
	LadoGlifoBotonFila float32 = 16
)

// BotonFila es uno de los botones del final de una fila plana. No es un widget: es una
// ZONA de la fila (ver el comentario de arriba).
type BotonFila struct {
	// Cuerpo es un glifo propio de Drive; si está vacío se pinta Icono, del catálogo.
	Cuerpo string
	Icono  iconos.Icono
	// Texto hace de la zona un botón con rótulo (`ghost sm`: `h-8 px-3 text-sm
	// font-semibold`) en vez de un glifo.
	Texto string
	// Tinta es el token en reposo y TintaEncima el de al pasar por encima (vacío: la
	// misma). La estrella marcada va en `primary-text` y la apagada en `text-subtle`.
	Tinta, TintaEncima string
	// FondoEncima es el velo al pasar por encima; vacío, `bg-subtle`.
	FondoEncima string
	// Lado es la caja (cero: `h-8 w-8`) y LadoGlifo el dibujo (cero: 16). Las filas de
	// una unidad de equipo usan `p-1.5` alrededor de un glifo de 18.
	Lado, LadoGlifo float32
	// SoloAlPasar es `opacity-0 group-hover:opacity-100 pointer-coarse:opacity-100`.
	SoloAlPasar bool
	// Rotulo es el `aria-label`.
	Rotulo string
	Inerte bool
	// AlPulsar recibe la caja del botón, de la que se cuelga un menú.
	AlPulsar func(disparador fyne.CanvasObject)
}

func (b BotonFila) lado() float32 {
	if b.Lado > 0 {
		return b.Lado
	}
	return LadoBotonFila
}

func (b BotonFila) ladoGlifo() float32 {
	if b.LadoGlifo > 0 {
		return b.LadoGlifo
	}
	return LadoGlifoBotonFila
}

// medida es lo que ocupa la zona.
func (b BotonFila) medida() fyne.Size {
	if b.Texto != "" {
		return fyne.NewSize(ui.Medir(b.Texto, tema.TextoSM, ui.SemiNegrita).Width+2*tema.Espacio(3), LadoBotonFila)
	}
	return fyne.NewSize(b.lado(), b.lado())
}

// Insignia es un distintivo de píldora: `rounded-full border px-2 py-0.5 text-xs
// font-medium`, con un glifo de 12 delante si lo lleva. El `PermBadge` de «Compartido
// conmigo», y el rol y el «Archivada» de las unidades de equipo.
type Insignia struct {
	Texto string
	// Glifo es un cuerpo de `glyphs.tsx` (el lápiz, el candado); vacío, sin glifo.
	Glifo string
	// Acento es `border-primary/40 text-primary-text`. Si no, `border-border` con la
	// Tinta dada (vacía: `text-muted`).
	Acento bool
	Tinta  string
}

// Medidas de la insignia.
const (
	margenXInsignia   float32 = 2 * 0.25 * tema.Rem
	margenYInsignia   float32 = 0.5 * 0.25 * tema.Rem
	ladoLapizInsignia float32 = 12
	huecoInsignia     float32 = 1 * 0.25 * tema.Rem
)

// MedidaInsignia es lo que ocupa: el texto con su relleno y su borde, y el glifo.
func MedidaInsignia(i *Insignia) fyne.Size {
	ancho := ui.Medir(i.Texto, tema.TextoXS, ui.Medio).Width + 2*margenXInsignia + 2*tema.Borde
	if i.Glifo != "" {
		ancho += ladoLapizInsignia + huecoInsignia
	}
	return fyne.NewSize(ancho, tema.TextoXS.Linea+2*margenYInsignia+2*tema.Borde)
}

// tintas son el borde y la tinta de la insignia.
func (i *Insignia) tintas(v tema.Variante) (borde, tinta color.NRGBA) {
	if i.Acento {
		borde = tema.Color("primary", v)
		borde.A = uint8(float32(borde.A)*0.4 + 0.5)
		return borde, tema.Color("primary-text", v)
	}
	t := i.Tinta
	if t == "" {
		t = "text-muted"
	}
	return tema.Color("border", v), tema.Color(t, v)
}

// Dónde va el filete de una fila plana.
const (
	FileteAbajo  = iota // `border-b`: las listas sueltas
	FileteArriba        // `divide-y` dentro de una caja con borde: entre filas
	SinFilete
)

// FilaPlana es la fila de las vistas planas.
type FilaPlana struct {
	widget.BaseWidget

	Nombre string
	// Extension y Carpeta deciden la tesela de 36. Glifo la sustituye: un documento de
	// Docs lleva el glifo de su tipo, de 20 y en `text-muted`, sin tesela.
	Extension string
	Carpeta   bool
	Glifo     *iconos.Dibujo
	// LadoGlifo es el del dibujo (cero: 20), CajaGlifo la caja que reserva (cero: el
	// mismo lado; `h-9 w-9` son 31,5) y TintaGlifo su token (vacío: `text-muted`).
	LadoGlifo, CajaGlifo float32
	TintaGlifo           string
	// Meta es la línea de debajo ya escrita. En las vistas de ficheros es `sytue-meta`
	// (`text-xs text-subtle`, con `mt-0.5`); en «Documentos», `text-sm text-muted` y sin
	// separación. Lo dice MetaDocs.
	Meta     string
	MetaDocs bool
	// MetaParte deja que la meta pase de línea (`block`, sin `truncate`): lo que puede
	// hacer quien tiene un rol en una unidad de equipo no cabe en un móvil. Hace falta
	// AnchoDe, el ancho al que se va a colocar la fila, porque el alto depende de él y
	// `MinSize` no lo recibe.
	MetaParte bool
	AnchoDe   func() float32
	// Derecha es un texto al final (`text-sm text-muted`): la hora de un documento.
	Derecha   string
	Insignias []*Insignia
	Botones   []BotonFila
	// RellenoY es el `py-*` (cero: `py-2`) y Filete dónde va la línea.
	RellenoY float32
	Filete   int
	// Grueso es el puntero grueso: los botones «al pasar» se ven siempre.
	Grueso bool
	// AlAbrir hace del nombre un enlace; nil, el nombre es texto.
	AlAbrir func()

	encima   bool
	zona     int // -1 fuera, 0 el nombre, 1… el botón i-1
	enfocado bool
	r        *filaPlanaRenderer
}

var (
	_ fyne.Widget       = (*FilaPlana)(nil)
	_ fyne.Tappable     = (*FilaPlana)(nil)
	_ fyne.Focusable    = (*FilaPlana)(nil)
	_ desktop.Hoverable = (*FilaPlana)(nil)
)

// NuevaFilaPlana crea la fila.
func NuevaFilaPlana(nombre string) *FilaPlana {
	f := &FilaPlana{Nombre: nombre, zona: -1}
	f.ExtendBaseWidget(f)
	return f
}

// LadoGlifoDoc es el `size={20}` del glifo de tipo de un documento de Docs.
const LadoGlifoDoc float32 = 20

func (f *FilaPlana) ladoGlifo() float32 {
	if f.LadoGlifo > 0 {
		return f.LadoGlifo
	}
	return LadoGlifoDoc
}

func (f *FilaPlana) cajaGlifo() float32 { return max(f.CajaGlifo, f.ladoGlifo()) }

func (f *FilaPlana) rellenoY() float32 {
	if f.RellenoY > 0 {
		return f.RellenoY
	}
	return ui.AltoFila
}

// AltoFilaPlana es lo que mide: el relleno a cada lado de lo más alto —la tesela de 36,
// o el nombre con su línea de meta— y el filete. 51 en las vistas de ficheros y 50 en
// «Documentos», donde el glifo es de 20 y la línea de debajo es de `text-sm`.
func (f *FilaPlana) AltoFilaPlana() float32 {
	alto := 2*f.rellenoY() + f.altoContenido()
	if f.Filete != SinFilete {
		alto += tema.Borde
	}
	return alto
}

func (f *FilaPlana) altoContenido() float32 {
	texto := tema.TextoBase.Linea
	if f.Meta != "" {
		texto += float32(f.lineasMeta()) * f.escalonMeta().Linea
		texto += f.separaMeta()
	}
	if f.Glifo != nil {
		return max(texto, f.cajaGlifo())
	}
	return max(texto, ui.TeselaDrive)
}

// lineasMeta es cuántas líneas ocupa la meta: una, salvo que parta y no quepa.
func (f *FilaPlana) lineasMeta() int {
	if !f.MetaParte || f.AnchoDe == nil {
		return 1
	}
	_, cabe := f.cabeTexto(f.AnchoDe())
	return max(len(ui.Repartir(f.Meta, f.escalonMeta(), ui.Normal, cabe)), 1)
}

// cabeTexto es dónde empieza el nombre y cuánto ancho le queda a la columna del texto
// en una fila de `ancho`: lo que dejan el glifo por la izquierda y, por la derecha, los
// botones, las insignias y el texto del final.
func (f *FilaPlana) cabeTexto(ancho float32) (x, cabe float32) {
	x = ui.MargenFila + ui.TeselaDrive + HuecoFilaPlana
	if f.Glifo != nil {
		x = ui.MargenFila + f.cajaGlifo() + HuecoFilaPlana
	}
	derecha := ancho - ui.MargenFila
	for _, b := range f.Botones {
		derecha -= b.medida().Width + HuecoFilaPlana
	}
	for _, i := range f.Insignias {
		derecha -= MedidaInsignia(i).Width + HuecoFilaPlana
	}
	if f.Derecha != "" {
		derecha -= ui.Medir(f.Derecha, tema.TextoSM, ui.Normal).Width + HuecoFilaPlana
	}
	return x, max(derecha-x, 0)
}

func (f *FilaPlana) escalonMeta() tema.Escalon {
	if f.MetaDocs {
		return tema.TextoSM
	}
	return tema.TextoXS
}

func (f *FilaPlana) separaMeta() float32 {
	if f.MetaDocs {
		return 0
	}
	return SeparaMeta
}

// MinSize es el alto de la hoja; el ancho lo pone la lista.
func (f *FilaPlana) MinSize() fyne.Size {
	f.ExtendBaseWidget(f)
	return fyne.NewSize(2*ui.MargenFila+ui.TeselaDrive, f.AltoFilaPlana())
}

// zonaEn dice qué hay bajo un punto: el nombre, un botón o nada.
func (f *FilaPlana) zonaEn(p fyne.Position) int {
	if f.r == nil {
		return -1
	}
	for i, z := range f.r.zonas {
		o, t := z.Position(), z.Size()
		if p.X >= o.X && p.X < o.X+t.Width && p.Y >= o.Y && p.Y < o.Y+t.Height {
			return i + 1
		}
	}
	n := f.r.nombreCaja
	if f.AlAbrir != nil && p.X >= n.Position.X && p.X < n.Position.X+n.Size.Width &&
		p.Y >= n.Position.Y && p.Y < n.Position.Y+n.Size.Height {
		return 0
	}
	return -1
}

// verBoton dice si un botón se ve ahora.
func (f *FilaPlana) verBoton(b BotonFila) bool { return !b.SoloAlPasar || f.encima || f.Grueso }

// Tapped abre si se pulsa el nombre, o dispara el botón que haya debajo.
func (f *FilaPlana) Tapped(e *fyne.PointEvent) {
	if e == nil {
		return
	}
	switch z := f.zonaEn(e.Position); {
	case z == 0:
		f.AlAbrir()
	case z > 0:
		b := f.Botones[z-1]
		if !b.Inerte && b.AlPulsar != nil && f.verBoton(b) {
			b.AlPulsar(f.r.zonas[z-1])
		}
	}
}

// MouseIn enciende el hover de la fila y el de la zona.
func (f *FilaPlana) MouseIn(e *desktop.MouseEvent) {
	f.encima, f.zona = true, f.zonaEn(e.Position)
	f.Refresh()
}

// MouseMoved mueve el resalte entre el nombre y los botones.
func (f *FilaPlana) MouseMoved(e *desktop.MouseEvent) {
	if z := f.zonaEn(e.Position); z != f.zona {
		f.zona = z
		f.Refresh()
	}
}

// MouseOut lo apaga todo.
func (f *FilaPlana) MouseOut() { f.encima, f.zona = false, -1; f.Refresh() }

// FocusGained, FocusLost, TypedRune y TypedKey: con el foco, Intro y Espacio abren,
// que es lo que hace el `<button>` del nombre.
func (f *FilaPlana) FocusGained() { f.enfocado = true; f.Refresh() }

// FocusLost apaga el anillo.
func (f *FilaPlana) FocusLost() { f.enfocado = false; f.Refresh() }

// TypedRune abre con Espacio.
func (f *FilaPlana) TypedRune(r rune) {
	if r == ' ' && f.AlAbrir != nil {
		f.AlAbrir()
	}
}

// TypedKey abre con Intro.
func (f *FilaPlana) TypedKey(e *fyne.KeyEvent) {
	if ui.SubirEscape(f, e) { // Escape y «atrás» son del diálogo de encima
		return
	}
	if (e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter) && f.AlAbrir != nil {
		f.AlAbrir()
	}
}

// AccessibilityLabel es el nombre.
func (f *FilaPlana) AccessibilityLabel() string { return f.Nombre }

// AccessibilityRole: se abre como un botón.
func (f *FilaPlana) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleButton }

// CreateRenderer compone la fila. Solo primitivas y dibujos.
func (f *FilaPlana) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaPlanaRenderer{
		f:         f,
		velo:      canvas.NewRectangle(color.NRGBA{}),
		linea:     canvas.NewRectangle(conAlfaBorde(v)),
		nombre:    ui.Texto(f.Nombre, tema.TextoBase, ui.Medio, tema.Color("text", v)),
		subrayado: canvas.NewRectangle(tema.Color("text", v)),
		aro:       ui.NuevoAro(tema.RadioMD, ui.SinHueco, v),
	}
	r.objetos = append(r.aro.Objetos(), r.velo, r.linea)
	if f.Glifo != nil {
		r.objetos = append(r.objetos, f.Glifo.Objeto())
	} else {
		r.tesela = ui.NuevaTeselaTipo(f.Extension, f.Carpeta, ui.TeselaDrive)
		r.objetos = append(r.objetos, r.tesela.Objetos()...)
	}
	r.objetos = append(r.objetos, r.nombre, r.subrayado)
	if f.Meta != "" {
		tinta := "text-subtle"
		if f.MetaDocs {
			tinta = "text-muted"
		}
		if f.MetaParte {
			r.metaParte = ui.NuevoParrafo(f.Meta, f.escalonMeta(), ui.Normal, tinta, v)
			r.objetos = append(r.objetos, r.metaParte)
		} else {
			r.meta = ui.Texto(f.Meta, f.escalonMeta(), ui.Normal, tema.Color(tinta, v))
			r.objetos = append(r.objetos, r.meta)
		}
	}
	if f.Derecha != "" {
		r.derecha = ui.Texto(f.Derecha, tema.TextoSM, ui.Normal, tema.Color("text-muted", v))
		r.objetos = append(r.objetos, r.derecha)
	}
	for _, ins := range f.Insignias {
		p := nuevaPildora(ins, v)
		r.insignias = append(r.insignias, p)
		r.objetos = append(r.objetos, p.objetos()...)
	}
	for _, b := range f.Botones {
		zona := canvas.NewRectangle(color.NRGBA{})
		zona.CornerRadius = tema.RadioMD
		r.zonas = append(r.zonas, zona)
		r.objetos = append(r.objetos, zona)
		if b.Texto != "" {
			t := ui.Texto(b.Texto, tema.TextoSM, ui.SemiNegrita, tema.Color("text", v))
			r.glifos, r.rotulos = append(r.glifos, nil), append(r.rotulos, t)
			r.objetos = append(r.objetos, t)
			continue
		}
		var g *iconos.Dibujo
		if b.Cuerpo != "" {
			g = glifoCon(b.Cuerpo, b.ladoGlifo(), tema.Color(b.Tinta, v))
		} else {
			g = iconos.Trazo(b.Icono, b.ladoGlifo(), b.Tinta, v)
		}
		r.glifos, r.rotulos = append(r.glifos, g), append(r.rotulos, nil)
		r.objetos = append(r.objetos, g.Objeto())
	}
	f.r = r
	r.Refresh()
	return r
}

// pildora es una insignia montada: su caja, su glifo y su texto.
type pildora struct {
	i     *Insignia
	caja  *ui.Caja
	glifo *iconos.Dibujo
	texto *canvas.Text
}

func nuevaPildora(i *Insignia, v tema.Variante) *pildora {
	borde, tinta := i.tintas(v)
	p := &pildora{
		i: i, caja: ui.NuevaCaja(color.NRGBA{}, borde, tema.RadioPill),
		texto: ui.Texto(i.Texto, tema.TextoXS, ui.Medio, tinta),
	}
	if i.Glifo != "" {
		p.glifo = glifoCon(i.Glifo, ladoLapizInsignia, tinta)
	}
	return p
}

func (p *pildora) objetos() []fyne.CanvasObject {
	out := append([]fyne.CanvasObject{}, p.caja.Objetos()...)
	if p.glifo != nil {
		out = append(out, p.glifo.Objeto())
	}
	return append(out, p.texto)
}

func (p *pildora) colocar(o fyne.Position) {
	m := MedidaInsignia(p.i)
	ui.MoverCaja(p.caja, o, m)
	x := o.X + tema.Borde + margenXInsignia
	if p.glifo != nil {
		p.glifo.Colocar(x, o.Y+(m.Height-ladoLapizInsignia)/2, ladoLapizInsignia)
		x += ladoLapizInsignia + huecoInsignia
	}
	ui.ColocarTexto(p.texto, tema.TextoXS, x, o.Y+tema.Borde+margenYInsignia)
}

func (p *pildora) pintar(v tema.Variante) {
	borde, tinta := p.i.tintas(v)
	p.caja.Pintar(color.NRGBA{}, borde)
	p.texto.Color = tinta
	if p.glifo != nil {
		p.glifo.TenirCon(tinta)
	}
}

type filaPlanaRenderer struct {
	f                      *FilaPlana
	velo, linea, subrayado *canvas.Rectangle
	aro                    *ui.Aro
	tesela                 *ui.TeselaTipo
	nombre, meta, derecha  *canvas.Text
	metaParte              *ui.Parrafo
	insignias              []*pildora
	zonas                  []*canvas.Rectangle
	glifos                 []*iconos.Dibujo
	rotulos                []*canvas.Text
	objetos                []fyne.CanvasObject
	nombreCaja             struct {
		Position fyne.Position
		Size     fyne.Size
	}
}

func (r *filaPlanaRenderer) MinSize() fyne.Size           { return r.f.MinSize() }
func (r *filaPlanaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaPlanaRenderer) Destroy()                     {}

func (r *filaPlanaRenderer) Layout(t fyne.Size) {
	f := r.f
	r.aro.Colocar(t)
	r.velo.Move(fyne.NewPos(0, 0))
	r.velo.Resize(t)
	arriba, contenido := float32(0), t.Height
	switch f.Filete {
	case FileteAbajo:
		contenido -= tema.Borde
		r.linea.Move(fyne.NewPos(0, contenido))
	case FileteArriba:
		arriba = tema.Borde
		r.linea.Move(fyne.NewPos(0, 0))
	}
	r.linea.Resize(fyne.NewSize(t.Width, tema.Borde))
	r.linea.Hidden = f.Filete == SinFilete
	centro := func(alto float32) float32 { return arriba + (contenido-arriba-alto)/2 }

	x := ui.MargenFila
	if f.Glifo != nil {
		caja := f.cajaGlifo()
		f.Glifo.Colocar(x+(caja-f.ladoGlifo())/2, centro(f.ladoGlifo()), f.ladoGlifo())
		x += caja + HuecoFilaPlana
	} else {
		r.tesela.Colocar(x, centro(ui.TeselaDrive))
		x += ui.TeselaDrive + HuecoFilaPlana
	}

	derecha := t.Width - ui.MargenFila
	for i := len(r.zonas) - 1; i >= 0; i-- {
		b := f.Botones[i]
		m := b.medida()
		o := fyne.NewPos(derecha-m.Width, centro(m.Height))
		r.zonas[i].Move(o)
		r.zonas[i].Resize(m)
		if g := r.glifos[i]; g != nil {
			g.Colocar(o.X+(m.Width-b.ladoGlifo())/2, o.Y+(m.Height-b.ladoGlifo())/2, b.ladoGlifo())
		} else {
			ui.ColocarTexto(r.rotulos[i], tema.TextoSM, o.X+tema.Espacio(3), o.Y+(m.Height-tema.TextoSM.Linea)/2)
		}
		derecha -= m.Width + HuecoFilaPlana
	}
	for i := len(r.insignias) - 1; i >= 0; i-- {
		m := MedidaInsignia(r.insignias[i].i)
		r.insignias[i].colocar(fyne.NewPos(derecha-m.Width, centro(m.Height)))
		derecha -= m.Width + HuecoFilaPlana
	}
	if r.derecha != nil {
		ancho := ui.Medir(f.Derecha, tema.TextoSM, ui.Normal).Width
		ui.ColocarTexto(r.derecha, tema.TextoSM, derecha-ancho, centro(tema.TextoSM.Linea))
		derecha -= ancho + HuecoFilaPlana
	}

	cabe := max(derecha-x, 0)
	alto := tema.TextoBase.Linea
	lineas := 1
	if r.metaParte != nil {
		lineas = max(len(ui.Repartir(f.Meta, f.escalonMeta(), ui.Normal, cabe)), 1)
	}
	if r.meta != nil || r.metaParte != nil {
		alto += float32(lineas)*f.escalonMeta().Linea + f.separaMeta()
	}
	y := centro(alto)
	anchoNombre := ui.Truncar(r.nombre, f.Nombre, tema.TextoBase, ui.Medio, cabe)
	ui.ColocarTexto(r.nombre, tema.TextoBase, x, y)
	r.nombreCaja.Position, r.nombreCaja.Size = fyne.NewPos(x, y), fyne.NewSize(anchoNombre, tema.TextoBase.Linea)
	// `hover:underline`: el subrayado del navegador cae un poco por debajo de la línea
	// base; aquí va al pie de la caja de la fuente, a 1 px.
	r.subrayado.Move(fyne.NewPos(x, y+(tema.TextoBase.Linea+tema.TextoBase.Tam)/2+1))
	r.subrayado.Resize(fyne.NewSize(anchoNombre, tema.Borde))
	if r.meta != nil {
		ui.Truncar(r.meta, f.Meta, f.escalonMeta(), ui.Normal, cabe)
		ui.ColocarTexto(r.meta, f.escalonMeta(), x, y+tema.TextoBase.Linea+f.separaMeta())
	}
	if r.metaParte != nil {
		r.metaParte.Move(fyne.NewPos(x, y+tema.TextoBase.Linea+f.separaMeta()))
		r.metaParte.Resize(fyne.NewSize(cabe, float32(lineas)*f.escalonMeta().Linea))
	}
}

func (r *filaPlanaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.velo.FillColor = color.NRGBA{}
	if f.encima {
		r.velo.FillColor = tema.Color("bg-subtle", v)
	}
	r.linea.FillColor = conAlfaBorde(v)
	r.nombre.Color = tema.Color("text", v)
	r.subrayado.FillColor = tema.Color("text", v)
	r.subrayado.Hidden = f.zona != 0
	if r.tesela != nil {
		r.tesela.Poner(f.Extension, f.Carpeta, v)
	}
	if f.Glifo != nil {
		tinta := f.TintaGlifo
		if tinta == "" {
			tinta = "text-muted"
		}
		f.Glifo.Tenir(tinta, v)
	}
	for _, p := range r.insignias {
		p.pintar(v)
	}
	for i, b := range f.Botones {
		tinta := b.Tinta
		if tinta == "" {
			tinta = "text"
		}
		r.zonas[i].FillColor = color.NRGBA{}
		if f.zona == i+1 && !b.Inerte && b.Texto == "" {
			fondo := b.FondoEncima
			if fondo == "" {
				fondo = "bg-subtle"
			}
			r.zonas[i].FillColor = tema.Color(fondo, v)
			if b.TintaEncima != "" {
				tinta = b.TintaEncima
			}
		}
		c := tema.Color(tinta, v)
		if b.Inerte {
			c.A /= 2 // `disabled:opacity-50`
		}
		ver := f.verBoton(b)
		r.zonas[i].Hidden = !ver
		if g := r.glifos[i]; g != nil {
			g.TenirCon(c)
			g.Ver(ver)
		} else {
			r.rotulos[i].Color = c
			r.rotulos[i].Hidden = !ver
		}
	}
	r.aro.Pintar(v)
	r.aro.Ver(f.enfocado)
	for _, o := range r.objetos {
		o.Refresh()
	}
	if s := f.Size(); s.Width > 0 {
		r.Layout(s)
	}
}
