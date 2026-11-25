// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

// El estado de carga y el de error de `EditorApp.tsx` (packages/office/src/editor).

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las medidas del esqueleto de carga de `EditorApp.tsx`, una a una. Los anchos en
// píxeles son `style={{ width }}` literales en la web; lo demás son clases de Tailwind.
var (
	// AnchosPestanasEsqueleto son las cinco pestañas: `[48, 64, 56, 72, 60]`, `h-3`.
	AnchosPestanasEsqueleto = []float32{48, 64, 56, 72, 60}
	// AnchosGruposEsqueleto son los cuatro grupos de la banda: `[120, 96, 140, 88]`, `h-10`.
	AnchosGruposEsqueleto = []float32{120, 96, 140, 88}
	// LineasFolioEsqueleto son las ocho líneas del folio, a `90 - (i % 3) * 15` %.
	LineasFolioEsqueleto = 8
)

// Clases del esqueleto. La cabecera es `px-3 gap-2` con un `h-6 w-6`, un `h-4 w-40` y,
// a la derecha, un `h-6 w-16` y otro `h-6 w-6`; las pestañas `px-3 gap-3`; la banda
// `px-3 gap-2`; el carril `py-2 gap-1` con tres `h-8 w-8`.
var (
	MargenEsqueleto        = tema.Espacio(3)
	HuecoEsqueleto         = tema.Espacio(2)
	HuecoPestanasEsqueleto = tema.Espacio(3)
	AltoPestanaEsqueleto   = tema.Espacio(3)
	AltoGrupoEsqueleto     = tema.Espacio(10)
	LadoMarcaEsqueleto     = tema.Espacio(6)
	AnchoTituloEsqueleto   = tema.Espacio(40)
	AltoTituloEsqueleto    = tema.Espacio(4)
	AnchoChipEsqueleto     = tema.Espacio(16)
	LadoCarrilEsqueleto    = tema.Espacio(8)
)

// El folio de carga: `mx-auto mt-8 w-[min(70%,720px)] rounded-sm p-12`, con un título
// `h-6 w-1/2 mb-6` y las líneas `h-3 mb-3`.
var (
	FraccionFolioEsqueleto           = float32(0.7)
	AnchoMaxFolioEsqueleto   float32 = 720
	ArribaFolioEsqueleto             = tema.Espacio(8)
	RellenoFolioEsqueleto            = tema.Espacio(12)
	AltoTituloFolioEsqueleto         = tema.Espacio(6)
	BajoTituloFolioEsqueleto         = tema.Espacio(6)
	AltoLineaFolioEsqueleto          = tema.Espacio(3)
	BajoLineaFolioEsqueleto          = tema.Espacio(3)
)

// Los colores locales de `editor.css` que el esqueleto comparte con el lienzo: el papel
// es blanco en los dos temas y la mesa tiene su propio gris. No son tokens de la suite.
var (
	// PapelEditor es `--editor-page-bg`.
	PapelEditor = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	// MesaClara y MesaOscura son `--editor-canvas-bg` en los dos temas.
	MesaClara  = color.NRGBA{R: 0xE9, G: 0xE6, B: 0xDD, A: 0xFF}
	MesaOscura = color.NRGBA{R: 0x18, G: 0x1A, B: 0x1C, A: 0xFF}
)

// ColorMesa es `--editor-canvas-bg` en esa variante.
func ColorMesa(v tema.Variante) color.NRGBA {
	if v == tema.Oscuro {
		return MesaOscura
	}
	return MesaClara
}

// AnchoFolioEsqueleto es `w-[min(70%,720px)]` sobre la mesa de ese ancho.
func AnchoFolioEsqueleto(mesa float32) float32 {
	return min(mesa*FraccionFolioEsqueleto, AnchoMaxFolioEsqueleto)
}

// FraccionLineaEsqueleto es el ancho de la línea `i` del folio: `90 - (i % 3) * 15` %.
func FraccionLineaEsqueleto(i int) float32 { return float32(90-(i%3)*15) / 100 }

// AltoFolioEsqueleto es lo que mide el folio de carga: su relleno, el título y las líneas.
func AltoFolioEsqueleto() float32 {
	return 2*RellenoFolioEsqueleto + AltoTituloFolioEsqueleto + BajoTituloFolioEsqueleto +
		float32(LineasFolioEsqueleto)*(AltoLineaFolioEsqueleto+BajoLineaFolioEsqueleto)
}

// encoger reparte los anchos en una fila flex de `disponible`: si no caben, cada bloque
// cede en proporción a su ancho, que es lo que hace `flex-shrink: 1` con bloques sin
// contenido. Los huecos no ceden.
func encoger(anchos []float32, hueco, disponible float32) []float32 {
	out := make([]float32, len(anchos))
	copy(out, anchos)
	suma := float32(0)
	for _, w := range anchos {
		suma += w
	}
	libre := disponible - hueco*float32(max(len(anchos)-1, 0))
	if suma <= 0 || libre >= suma {
		return out
	}
	f := max(libre, 0) / suma
	for i := range out {
		out[i] *= f
	}
	return out
}

// EsqueletoEditor es el estado «cargando» del editor: el cromo entero en bloques
// (cabecera, pestañas, banda, carril, folio y barra de estado), como `EditorApp.tsx`.
// Se percibe más rápido que un «Abriendo…» y no salta al montar la interfaz real,
// porque cada banda mide lo mismo que la de verdad.
type EsqueletoEditor struct {
	widget.BaseWidget

	ctx *Contexto
	r   *esqueletoEditorRenderer // el último renderer, para Parar
}

var (
	_ fyne.Widget     = (*EsqueletoEditor)(nil)
	_ fyne.Accessible = (*EsqueletoEditor)(nil)
)

// NuevoEsqueletoEditor crea el esqueleto del editor.
func NuevoEsqueletoEditor(c *Contexto) *EsqueletoEditor {
	e := &EsqueletoEditor{ctx: c}
	e.ExtendBaseWidget(e)
	return e
}

// AccessibilityLabel es el `<span class="sr-only">` de la web: «Abriendo…». Los
// bloques no dicen nada (son `aria-hidden`).
func (e *EsqueletoEditor) AccessibilityLabel() string { return e.ctx.Traducir("editor.opening") }

// AccessibilityRole: `role="status"` no existe en Fyne 2.8; el texto es lo que llega.
func (e *EsqueletoEditor) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// MinSize es el de las bandas fijas, como el marco.
func (e *EsqueletoEditor) MinSize() fyne.Size {
	e.ExtendBaseWidget(e)
	return fyne.NewSize(AnchoCarril, AltoCabecera+AltoCinta()+AltoEstado)
}

// bloque es un `rounded-sm bg-bg-subtle` de la web, con su pulso.
func bloque(ancho, alto float32) *ui.Esqueleto {
	b := ui.NuevoEsqueleto(ancho, alto)
	b.Radio = tema.RadioSM
	return b
}

// CreateRenderer construye los bloques una vez; Layout los coloca.
func (e *EsqueletoEditor) CreateRenderer() fyne.WidgetRenderer {
	r := &esqueletoEditorRenderer{
		e:      e,
		fondo:  canvas.NewRectangle(color.NRGBA{}),
		mesa:   canvas.NewRectangle(color.NRGBA{}),
		papel:  canvas.NewRectangle(PapelEditor),
		sombra: ui.NuevaSombraCon(CapasSombraPagina, tema.RadioSM),
		marca:  bloque(LadoMarcaEsqueleto, LadoMarcaEsqueleto),
		titulo: bloque(AnchoTituloEsqueleto, AltoTituloEsqueleto),
		chip:   bloque(AnchoChipEsqueleto, LadoMarcaEsqueleto),
		cuenta: bloque(LadoMarcaEsqueleto, LadoMarcaEsqueleto),
		cabeza: bloque(0, AltoTituloFolioEsqueleto),
	}
	for range 5 {
		r.lineas = append(r.lineas, canvas.NewRectangle(color.NRGBA{}))
	}
	for _, w := range AnchosPestanasEsqueleto {
		r.pestanas = append(r.pestanas, bloque(w, AltoPestanaEsqueleto))
	}
	for _, w := range AnchosGruposEsqueleto {
		r.grupos = append(r.grupos, bloque(w, AltoGrupoEsqueleto))
	}
	for range 3 {
		r.carril = append(r.carril, bloque(LadoCarrilEsqueleto, LadoCarrilEsqueleto))
	}
	for range LineasFolioEsqueleto {
		r.texto = append(r.texto, bloque(0, AltoLineaFolioEsqueleto))
	}
	r.papel.CornerRadius = tema.RadioSM
	r.Refresh()
	e.r = r
	return r
}

// Parar detiene el pulso de todos los bloques. Lo llama el marco al quitar el
// esqueleto, sin esperar a que Fyne destruya el renderer.
func (e *EsqueletoEditor) Parar() {
	if e.r != nil {
		e.r.Destroy()
	}
}

type esqueletoEditorRenderer struct {
	e *EsqueletoEditor

	fondo, mesa, papel *canvas.Rectangle
	sombra             *ui.Sombra
	// lineas son los bordes: bajo la cabecera, las pestañas y la banda, a la derecha
	// del carril y encima de la barra de estado.
	lineas                   []*canvas.Rectangle
	marca, titulo, chip      *ui.Esqueleto
	cuenta, cabeza           *ui.Esqueleto
	pestanas, grupos, carril []*ui.Esqueleto
	texto                    []*ui.Esqueleto
	variante                 tema.Variante
	objetos                  []fyne.CanvasObject
}

func (r *esqueletoEditorRenderer) MinSize() fyne.Size { return r.e.MinSize() }
func (r *esqueletoEditorRenderer) Destroy() {
	for _, b := range r.bloques() {
		b.Parar()
	}
}

func (r *esqueletoEditorRenderer) bloques() []*ui.Esqueleto {
	out := []*ui.Esqueleto{r.marca, r.titulo, r.chip, r.cuenta, r.cabeza}
	out = append(out, r.pestanas...)
	out = append(out, r.grupos...)
	out = append(out, r.carril...)
	return append(out, r.texto...)
}

func (r *esqueletoEditorRenderer) Objects() []fyne.CanvasObject {
	if r.objetos == nil {
		r.objetos = []fyne.CanvasObject{r.fondo, r.mesa, r.sombra.Objeto(), r.papel}
		for _, l := range r.lineas {
			r.objetos = append(r.objetos, l)
		}
		for _, b := range r.bloques() {
			r.objetos = append(r.objetos, b)
		}
	}
	return r.objetos
}

// fila coloca bloques en fila desde `x`, centrados en vertical en la banda que empieza
// en `y` y mide `alto`.
func fila(bs []*ui.Esqueleto, anchos []float32, x, y, alto, hueco float32) {
	for i, b := range bs {
		h := ui.MedidaEsqueleto(b).Height
		b.Move(fyne.NewPos(x, y+(alto-h)/2))
		b.Resize(fyne.NewSize(anchos[i], h))
		x += anchos[i] + hueco
	}
}

func (r *esqueletoEditorRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	borde := tema.Borde
	util := max(t.Width-2*MargenEsqueleto, 0)

	// Cabecera: marca y título a la izquierda, chip y cuenta a la derecha (`ml-auto`).
	y := float32(0)
	alto := AltoCabecera - borde
	fila([]*ui.Esqueleto{r.marca, r.titulo}, encoger([]float32{LadoMarcaEsqueleto, AnchoTituloEsqueleto}, HuecoEsqueleto, util),
		MargenEsqueleto, y, alto, HuecoEsqueleto)
	derecha := encoger([]float32{AnchoChipEsqueleto, LadoMarcaEsqueleto}, HuecoEsqueleto, util)
	fila([]*ui.Esqueleto{r.chip, r.cuenta}, derecha,
		t.Width-MargenEsqueleto-derecha[0]-derecha[1]-HuecoEsqueleto, y, alto, HuecoEsqueleto)
	r.raya(0, fyne.NewPos(0, y+alto), fyne.NewSize(t.Width, borde))
	y += AltoCabecera

	// Pestañas.
	alto = AltoDeTira() - borde
	fila(r.pestanas, encoger(AnchosPestanasEsqueleto, HuecoPestanasEsqueleto, util), MargenEsqueleto, y, alto, HuecoPestanasEsqueleto)
	r.raya(1, fyne.NewPos(0, y+alto), fyne.NewSize(t.Width, borde))
	y += AltoDeTira()

	// Banda de grupos.
	alto = AltoBanda - borde
	fila(r.grupos, encoger(AnchosGruposEsqueleto, HuecoEsqueleto, util), MargenEsqueleto, y, alto, HuecoEsqueleto)
	r.raya(2, fyne.NewPos(0, y+alto), fyne.NewSize(t.Width, borde))
	y += AltoBanda

	// Carril y mesa, hasta la barra de estado.
	abajo := max(t.Height-AltoEstado, y)
	cuerpo := abajo - y
	yc := y + RellenoCarril
	for _, b := range r.carril {
		b.Move(fyne.NewPos((AnchoCarril-borde-LadoCarrilEsqueleto)/2, yc))
		b.Resize(fyne.NewSize(LadoCarrilEsqueleto, LadoCarrilEsqueleto))
		yc += LadoCarrilEsqueleto + HuecoCarril
	}
	r.raya(3, fyne.NewPos(AnchoCarril-borde, y), fyne.NewSize(borde, cuerpo))

	mesaX := AnchoCarril
	mesaAncho := max(t.Width-mesaX, 0)
	r.mesa.Move(fyne.NewPos(mesaX, y))
	r.mesa.Resize(fyne.NewSize(mesaAncho, cuerpo))

	folio := AnchoFolioEsqueleto(mesaAncho)
	pos := fyne.NewPos(mesaX+(mesaAncho-folio)/2, y+ArribaFolioEsqueleto)
	// La mesa es `overflow-hidden`: en una ventana baja el folio se corta en la barra de
	// estado y lo que queda por debajo no se pinta.
	tam := fyne.NewSize(folio, min(AltoFolioEsqueleto(), max(abajo-pos.Y, 0)))
	mostrar(r.papel, tam.Height > 0)
	mostrar(r.sombra.Objeto(), tam.Height > 0)
	r.papel.Move(pos)
	r.papel.Resize(tam)
	r.sombra.Ajustar(pos, tam, r.variante)
	dentro := max(folio-2*RellenoFolioEsqueleto, 0)
	x := pos.X + RellenoFolioEsqueleto
	yt := pos.Y + RellenoFolioEsqueleto
	r.cabeza.Move(fyne.NewPos(x, yt))
	r.cabeza.Resize(fyne.NewSize(dentro/2, AltoTituloFolioEsqueleto))
	mostrar(r.cabeza, yt+AltoTituloFolioEsqueleto <= abajo)
	yt += AltoTituloFolioEsqueleto + BajoTituloFolioEsqueleto
	for i, b := range r.texto {
		b.Move(fyne.NewPos(x, yt))
		b.Resize(fyne.NewSize(dentro*FraccionLineaEsqueleto(i), AltoLineaFolioEsqueleto))
		mostrar(b, yt+AltoLineaFolioEsqueleto <= abajo)
		yt += AltoLineaFolioEsqueleto + BajoLineaFolioEsqueleto
	}

	r.raya(4, fyne.NewPos(0, abajo), fyne.NewSize(t.Width, borde))
}

// mostrar enseña o esconde `o` según `si`.
func mostrar(o fyne.CanvasObject, si bool) {
	if si {
		o.Show()
	} else {
		o.Hide()
	}
}

// raya coloca el borde `i`.
func (r *esqueletoEditorRenderer) raya(i int, p fyne.Position, s fyne.Size) {
	r.lineas[i].Move(p)
	r.lineas[i].Resize(s)
}

func (r *esqueletoEditorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.variante = v
	r.fondo.FillColor = tema.Color("surface", v)
	r.mesa.FillColor = ColorMesa(v)
	borde := tema.Color("border", v)
	for _, l := range r.lineas {
		l.FillColor = borde
		l.Refresh()
	}
	r.fondo.Refresh()
	r.mesa.Refresh()
	r.papel.Refresh()
	for _, b := range r.bloques() {
		b.Refresh()
	}
	r.Layout(r.e.Size())
}

// PantallaFallo es el estado de error del editor, como `EditorApp.tsx` con
// `status === 'error'`: el editor entero cede el sitio a un EmptyState de tono error
// (el aviso en `danger`) centrado sobre `surface`, con «Reintentar» —que vuelve a pedir
// el documento— y «Volver». Cada botón solo sale si hay a dónde llevar: un fichero
// suelto que no se pudo leer no tiene nada que reintentar.
func PantallaFallo(c *Contexto) fyne.CanvasObject {
	e := ui.NuevoVacio(c.Traducir("editor.error.title"), c.Traducir("editor.error.description"))
	e.Tono = ui.VacioError
	var acciones []fyne.CanvasObject
	if c.Reintentar != nil {
		acciones = append(acciones, ui.NuevoBoton(c.Traducir("editor.error.retry"), c.Reintentar).Con(ui.Primario, ui.TallaSM))
	}
	if c.Volver != nil {
		acciones = append(acciones, ui.NuevoBoton(c.Traducir("editor.error.back"), c.Volver).Con(ui.Secundario, ui.TallaSM))
	}
	if len(acciones) > 0 {
		e.Accion = acciones[0]
	}
	if len(acciones) > 1 {
		e.AccionSecundaria = acciones[1]
	}
	return container.NewStack(ui.NuevoLiso("surface"), e)
}
