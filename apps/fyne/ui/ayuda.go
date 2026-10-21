// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las dos piezas de `FieldHint.tsx`, que es la capa de AYUDA de la suite: la línea
// bajo un campo y el bloque de contexto dentro del flujo.
//
// Están aquí y no en un formulario concreto porque las pide TODO formulario, y porque
// Sytue Drive ya se había montado la suya —`ayudaEnLinea`, en `apps/drive/piezas.go`,
// con su `// ponytail:` pidiendo esto—.

// TonoAyuda son los cinco tonos de `HintTone`. El cero es `neutral`, como allí.
//
// No es `Tono`, el de la insignia: esa tiene SIETE y dos de ellas —primaria y
// acento— no existen aquí. Son dos conjuntos distintos en la web y lo siguen siendo
// aquí; juntarlos daría un `AyudaAcento` que ninguna hoja sabe pintar.
type TonoAyuda uint8

// Los cinco tonos.
const (
	AyudaNeutra TonoAyuda = iota
	AyudaInfo
	AyudaExito
	AyudaAviso
	AyudaPeligro
)

// iconoTono es el mapa `toneIcon` de la web.
var iconoTono = [...]iconos.Icono{
	AyudaNeutra:  iconos.Informacion,
	AyudaInfo:    iconos.Informacion,
	AyudaExito:   iconos.Tic,
	AyudaAviso:   iconos.Aviso,
	AyudaPeligro: iconos.Peligro,
}

// tintaTono es el mapa `hintText`.
var tintaTono = [...]string{
	AyudaNeutra:  "text-muted",
	AyudaInfo:    "info-text",
	AyudaExito:   "success-text",
	AyudaAviso:   "warning-text",
	AyudaPeligro: "danger-text",
}

// superficieTono es el mapa `helpSurface`: borde y fondo del bloque.
var superficieTono = [...][2]string{
	AyudaNeutra:  {"border", "bg-subtle"},
	AyudaInfo:    {"info", "info-bg"},
	AyudaExito:   {"success", "success-bg"},
	AyudaAviso:   {"warning", "warning-bg"},
	AyudaPeligro: {"danger", "danger-bg"},
}

// Medidas de las dos piezas, de `FieldHint.tsx`.
const (
	// RellenoAyuda es el `p-3` del bloque y HuecoAyuda su `gap-3`.
	RellenoAyuda float32 = 3 * 0.25 * tema.Rem
	HuecoAyuda   float32 = 3 * 0.25 * tema.Rem
	// GlifoAyuda es el `<Icon size={16}>` del bloque, y GlifoPista el `size={14}` de
	// la línea.
	GlifoAyuda float32 = 16
	GlifoPista float32 = 14
	// DesplazaGlifoAyuda es el `mt-0.5` del icono del bloque, y DesplazaGlifoPista el
	// `mt-px` del de la línea.
	DesplazaGlifoAyuda float32 = 0.5 * 0.25 * tema.Rem
	DesplazaGlifoPista float32 = 1
	// HuecoPista es el `gap-1.5` de la línea.
	HuecoPista float32 = 1.5 * 0.25 * tema.Rem
	// MargenTitulo es el `mt-1` entre el titular y el cuerpo.
	MargenTitulo float32 = 1 * 0.25 * tema.Rem
)

// Los dos interlineados que usa esta familia y que no están en la escala: el cuerpo del
// bloque es `text-sm leading-relaxed` (1,625) y la línea es `text-xs leading-snug`
// (1,375). La escala de `tema` trae el interlineado por defecto de cada escalón, no
// estos.
var (
	relajado = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.625 * tema.TextoSM.Tam}
	ajustado = tema.Escalon{Tam: tema.TextoXS.Tam, Linea: 1.375 * tema.TextoXS.Tam}
)

// AyudaEnLinea es el `InlineHelp`: el bloque de contexto dentro del flujo —«qué
// necesitas a mano», el resultado de una prueba, un aviso—. Sobrio: borde y fondo tenue
// del tono, sin degradados ni sombras.
type AyudaEnLinea struct {
	widget.BaseWidget

	Tono TonoAyuda
	// Titulo es el titular breve. Vacío: solo cuerpo.
	Titulo string
	// Cuerpo es el texto, que se reparte en líneas al ancho que haya.
	Cuerpo string
	// SinIcono lo quita, que es el `hideIcon` de la web.
	SinIcono bool

	caja   *Caja
	icono  *iconos.Dibujo
	titulo *Parrafo
	cuerpo *Parrafo
}

var (
	_ fyne.Widget     = (*AyudaEnLinea)(nil)
	_ fyne.Accessible = (*AyudaEnLinea)(nil)
)

// NuevaAyudaEnLinea crea el bloque neutro con su cuerpo.
func NuevaAyudaEnLinea(cuerpo string) *AyudaEnLinea {
	a := &AyudaEnLinea{Cuerpo: cuerpo}
	a.ExtendBaseWidget(a)
	return a
}

// ConTono devuelve el mismo bloque con otro tono, para encadenarlo.
func (a *AyudaEnLinea) ConTono(t TonoAyuda) *AyudaEnLinea {
	a.Tono = t
	return a
}

// ConTitulo le pone titular.
func (a *AyudaEnLinea) ConTitulo(t string) *AyudaEnLinea {
	a.Titulo = t
	return a
}

// AccessibilityLabel es lo que hay escrito: el titular y el cuerpo.
func (a *AyudaEnLinea) AccessibilityLabel() string {
	if a.Titulo == "" {
		return a.Cuerpo
	}
	return a.Titulo + ". " + a.Cuerpo
}

// AccessibilityRole: es texto. La web le pone `role="alert"` en peligro y `status` en
// aviso; Fyne 2.8 no tiene ninguno de los dos y no hay región viva a la que anunciar.
//
// ponytail: el techo es del toolkit, no de esta pieza. Está en los huecos medidos de
// `docs/roadmap.md` junto al resto de lo anunciable.
func (a *AyudaEnLinea) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// AltoAyudaEnLinea es lo que ocupa a un ancho dado: el borde y el `p-3` de arriba y
// abajo, y las líneas que salgan del titular y del cuerpo con su `mt-1` de por medio.
// Medido en el arnés del ERP (2026-09-23): una línea de cuerpo con icono, 41,48 —el
// `rounded-md border p-3` suma su borde—.
func AltoAyudaEnLinea(a *AyudaEnLinea, ancho float32) float32 {
	cabe := a.cabe(ancho)
	alto := float32(0)
	if a.Titulo != "" {
		alto += float32(len(Repartir(a.Titulo, relajado, SemiNegrita, cabe))) * relajado.Linea
	}
	if a.Cuerpo != "" {
		if alto > 0 {
			alto += MargenTitulo
		}
		alto += float32(len(Repartir(a.Cuerpo, relajado, Normal, cabe))) * relajado.Linea
	}
	return 2*tema.Borde + 2*RellenoAyuda + max(alto, GlifoAyuda)
}

// cabe es el ancho útil del texto: el bloque menos su relleno y, si lo lleva, el icono
// con su hueco.
func (a *AyudaEnLinea) cabe(ancho float32) float32 {
	util := ancho - 2*tema.Borde - 2*RellenoAyuda
	if !a.SinIcono {
		util -= GlifoAyuda + HuecoAyuda
	}
	return max(util, 0)
}

// MedidaAyudaEnLinea es la caja del bloque a ese ancho.
func MedidaAyudaEnLinea(a *AyudaEnLinea, ancho float32) fyne.Size {
	return fyne.NewSize(ancho, AltoAyudaEnLinea(a, ancho))
}

// CreateRenderer compone el bloque.
func (a *AyudaEnLinea) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	borde, fondo := superficieTono[a.Tono][0], superficieTono[a.Tono][1]
	a.caja = NuevaCaja(tema.Color(fondo, v), tema.Color(borde, v), tema.RadioMD)

	r := &ayudaRenderer{a: a}
	r.objetos = a.caja.Objetos()
	if !a.SinIcono {
		// El icono del tono neutro va en `text-subtle`, no en la tinta del tono: lo
		// dice la clase de la web, y es lo que evita un informativo azul en un bloque
		// que no informa de nada.
		tinte := tintaTono[a.Tono]
		if a.Tono == AyudaNeutra {
			tinte = "text-subtle"
		}
		a.icono = iconos.Trazo(iconoTono[a.Tono], GlifoAyuda, tinte, v)
		r.objetos = append(r.objetos, a.icono.Objeto())
	}
	if a.Titulo != "" {
		tinte := "text"
		if a.Tono != AyudaNeutra {
			tinte = tintaTono[a.Tono]
		}
		a.titulo = NuevoParrafo(a.Titulo, relajado, SemiNegrita, tinte, v)
		r.objetos = append(r.objetos, a.titulo)
	}
	if a.Cuerpo != "" {
		a.cuerpo = NuevoParrafo(a.Cuerpo, relajado, Normal, "text-muted", v)
		r.objetos = append(r.objetos, a.cuerpo)
	}
	r.Refresh()
	return r
}

type ayudaRenderer struct {
	a       *AyudaEnLinea
	objetos []fyne.CanvasObject
}

func (r *ayudaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, AltoAyudaEnLinea(r.a, r.a.Size().Width))
}

func (r *ayudaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *ayudaRenderer) Destroy()                     {}

func (r *ayudaRenderer) Layout(t fyne.Size) {
	a := r.a
	a.caja.Redimensionar(t)
	x := tema.Borde + RellenoAyuda
	if a.icono != nil {
		a.icono.Colocar(x, tema.Borde+RellenoAyuda+DesplazaGlifoAyuda, GlifoAyuda)
		x += GlifoAyuda + HuecoAyuda
	}
	cabe := a.cabe(t.Width)
	y := tema.Borde + RellenoAyuda
	coloca := func(p *Parrafo) {
		if p == nil {
			return
		}
		p.Move(fyne.NewPos(x, y))
		p.Resize(fyne.NewSize(cabe, AltoParrafo(p, cabe)))
		y += AltoParrafo(p, cabe)
	}
	coloca(a.titulo)
	if a.titulo != nil && a.cuerpo != nil {
		y += MargenTitulo
	}
	coloca(a.cuerpo)
}

func (r *ayudaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	s := superficieTono[r.a.Tono]
	r.a.caja.Pintar(tema.Color(s[1], v), tema.Color(s[0], v))
	r.Layout(r.a.Size())
}

// PistaCampo es el `FieldHint`: la línea bajo un campo que dice qué es y dónde
// encontrarlo, y que cambia a estado en vivo —ok o error— sin gritar: mismo sitio,
// mismo tamaño, solo cambian el icono y el color.
//
// De serie es una LÍNEA y no se reparte: el texto de una pista suele caber en el ancho
// de su campo. Con Ancho se reparte a ese ancho, que es lo que hace el `<p>` de la web
// con `min-w-0` cuando no cabe (las pistas de la autorespuesta de Mail van a dos
// renglones).
type PistaCampo struct {
	widget.BaseWidget

	Tono  TonoAyuda
	Texto string
	// SinIcono lo quita. Sin él, el icono se pinta salvo en el tono neutro, que es lo
	// que hace `showIcon` en la web.
	SinIcono bool
	// Ancho es el de la caja en la que va, icono incluido. Con 0, una sola línea.
	Ancho float32

	icono  *iconos.Dibujo
	rotulo *Parrafo
}

var (
	_ fyne.Widget     = (*PistaCampo)(nil)
	_ fyne.Accessible = (*PistaCampo)(nil)
)

// NuevaPistaCampo crea la pista neutra.
func NuevaPistaCampo(texto string) *PistaCampo {
	p := &PistaCampo{Texto: texto}
	p.ExtendBaseWidget(p)
	return p
}

// ConTono devuelve la misma pista con otro tono, para encadenarla.
func (p *PistaCampo) ConTono(t TonoAyuda) *PistaCampo {
	p.Tono = t
	return p
}

// AccessibilityLabel es el texto de la pista. En la web va enlazada al control con
// `aria-describedby`; Fyne no tiene cómo enlazar dos widgets, así que al menos se lee.
func (p *PistaCampo) AccessibilityLabel() string { return p.Texto }

// AccessibilityRole: es texto.
func (p *PistaCampo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// conIcono es `showIcon`: el neutro no lo lleva, porque una ayuda que no es estado no
// tiene nada que señalar.
func (p *PistaCampo) conIcono() bool { return !p.SinIcono && p.Tono != AyudaNeutra }

// MedidaPistaCampo es lo que ocupa: la caja de línea de `text-xs leading-snug`, y de
// ancho el texto más el icono con su hueco.
func MedidaPistaCampo(p *PistaCampo) fyne.Size {
	icono := float32(0)
	if p.conIcono() {
		icono = GlifoPista + HuecoPista
	}
	if p.Ancho > 0 {
		n := max(len(Repartir(p.Texto, ajustado, Normal, p.Ancho-icono)), 1)
		return fyne.NewSize(p.Ancho, max(float32(n)*ajustado.Linea, GlifoPista))
	}
	return fyne.NewSize(Medir(p.Texto, ajustado, Normal).Width+icono, max(ajustado.Linea, GlifoPista))
}

// CreateRenderer compone la línea.
func (p *PistaCampo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &pistaCampoRenderer{p: p}
	if p.conIcono() {
		p.icono = iconos.Trazo(iconoTono[p.Tono], GlifoPista, tintaTono[p.Tono], v)
		r.objetos = append(r.objetos, p.icono.Objeto())
	}
	p.rotulo = NuevoParrafo(p.Texto, ajustado, Normal, tintaTono[p.Tono], v)
	r.objetos = append(r.objetos, p.rotulo)
	r.Refresh()
	return r
}

type pistaCampoRenderer struct {
	p       *PistaCampo
	objetos []fyne.CanvasObject
}

func (r *pistaCampoRenderer) MinSize() fyne.Size           { return MedidaPistaCampo(r.p) }
func (r *pistaCampoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *pistaCampoRenderer) Destroy()                     {}

// Layout: `items-start`, así que el icono se apoya arriba con su `mt-px` y no centrado.
func (r *pistaCampoRenderer) Layout(t fyne.Size) {
	x := float32(0)
	if r.p.icono != nil {
		r.p.icono.Colocar(0, DesplazaGlifoPista, GlifoPista)
		x = GlifoPista + HuecoPista
	}
	r.p.rotulo.Move(fyne.NewPos(x, 0))
	alto := ajustado.Linea
	if r.p.Ancho > 0 {
		alto = AltoParrafo(r.p.rotulo, max(t.Width-x, 0))
	}
	r.p.rotulo.Resize(fyne.NewSize(max(t.Width-x, 0), alto))
}

func (r *pistaCampoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.p.rotulo.Texto = r.p.Texto
	r.p.rotulo.Tinta = tema.Color(tintaTono[r.p.Tono], v)
	r.p.rotulo.Refresh()
	if r.p.icono != nil {
		r.p.icono.Tenir(tintaTono[r.p.Tono], v)
	}
	r.Layout(r.p.Size())
}
