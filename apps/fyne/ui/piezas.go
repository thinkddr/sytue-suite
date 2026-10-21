// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Las piezas pequeñas que comparten el menú, las pestañas y las migas: el rótulo en
// versalitas con su tracking, el contador de no leídos y el galón.

// Versalitas es el `text-transform: uppercase` de la hoja.
func Versalitas(s string) string { return strings.ToUpper(s) }

// TrackingOverline es el `letter-spacing: 0.06em` de `sytue-overline` y de las
// pestañas, en EM. A los 10,5 px del overline son 0,63 px por letra, que es el número
// que se mide en producción.
//
// La hoja lo puso a propósito: «a 12px y en negrita, las versalitas se apelmazan en
// rótulos como USUARIOS o FACTURAS si no se les da aire». Sin él, un rótulo de ocho
// letras sale 5 px más estrecho y se lee apretado.
const TrackingOverline float32 = 0.06

// Versal es un texto en versalitas con espaciado entre letras, que `canvas.Text` no
// sabe hacer: Fyne no expone `letter-spacing`, así que cada letra va en su propio
// `canvas.Text` y la posición la ponemos nosotros.
//
// El ancho de cada letra se mide sobre el PREFIJO de la cadena y no letra a letra, para
// conservar el kerning que el conformador aplica entre pares; al resultado se le suma
// el tracking, exactamente como hace el navegador, que lo añade DESPUÉS de cada
// carácter, el último incluido.
//
// ponytail: un objeto de lienzo por letra. Es lo que cuesta el tracking, y se paga en
// rótulos de una decena de letras que se construyen una vez. Para un párrafo esto no
// vale: ahí haría falta un `canvas.Text` de verdad con espaciado, que es un cambio en
// Fyne.
type Versal struct {
	letras   []*canvas.Text
	x        []float32 // desplazamiento de cada letra desde el origen
	ancho    float32
	escalon  tema.Escalon
	tracking float32
}

// NuevoVersal construye el rótulo ya en mayúsculas.
func NuevoVersal(s string, e tema.Escalon, p Peso, c color.NRGBA) *Versal {
	return NuevoVersalCon(s, e, p, c, TrackingOverline)
}

// TrackingWider es el `tracking-wider` de Tailwind (0,05 em), el de los rótulos en
// versalitas que la web escribe a mano con `uppercase` en vez de con `sytue-overline`.
const TrackingWider float32 = 0.05

// NuevoVersalCon es el mismo rótulo con otro `letter-spacing`, en em: los `<h3>` del ERP
// llevan `tracking-wide` (0,025) y su «Etapa:» `tracking-wider` (0,05).
func NuevoVersalCon(s string, e tema.Escalon, p Peso, c color.NRGBA, em float32) *Versal {
	return NuevoEspaciado(Versalitas(s), e, p, c, em)
}

// NuevoEspaciado es el texto con `letter-spacing` y SIN pasarlo a mayúsculas: la
// cabecera de la tabla del ERP lleva `tracking-wider` y se pinta en caja mixta
// («Vencimiento», medido 84,63 con su botón y no los 96 de «VENCIMIENTO»).
func NuevoEspaciado(s string, e tema.Escalon, p Peso, c color.NRGBA, em float32) *Versal {
	v := &Versal{escalon: e, tracking: em * e.Tam}
	r := []rune(s)
	for i, letra := range r {
		t := Texto(string(letra), e, p, c)
		v.letras = append(v.letras, t)
		v.x = append(v.x, Medir(string(r[:i]), e, p).Width+float32(i)*v.tracking)
	}
	v.ancho = Medir(string(r), e, p).Width + float32(len(r))*v.tracking
	return v
}

// MedirVersal es el ancho que va a ocupar el rótulo, con su tracking. Es la cuenta que
// usan las medidas públicas de los widgets que lo llevan.
func MedirVersal(s string, e tema.Escalon, p Peso) float32 {
	return MedirVersalCon(s, e, p, TrackingOverline)
}

// MedirVersalCon es MedirVersal con otro tracking, en em.
func MedirVersalCon(s string, e tema.Escalon, p Peso, em float32) float32 {
	return MedirEspaciado(Versalitas(s), e, p, em)
}

// MedirEspaciado es lo que ocupa NuevoEspaciado.
func MedirEspaciado(s string, e tema.Escalon, p Peso, em float32) float32 {
	return Medir(s, e, p).Width + float32(len([]rune(s)))*em*e.Tam
}

// Objetos son las letras, para el `Objects()` del renderer.
func (v *Versal) Objetos() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(v.letras))
	for i, t := range v.letras {
		objs[i] = t
	}
	return objs
}

// Ancho es lo que ocupa.
func (v *Versal) Ancho() float32 { return v.ancho }

// Colocar pone la esquina superior izquierda de la CAJA DE LÍNEA en (x, y), con cada
// glifo centrado dentro como hace el navegador.
func (v *Versal) Colocar(x, y float32) {
	for i, t := range v.letras {
		ColocarTexto(t, v.escalon, x+v.x[i], y)
	}
}

// Ver lo enseña o lo esconde. Un rótulo que el layout no coloca porque su columna no
// cabe hay que esconderlo: si no, sus letras se quedan donde nacieron, en (0, 0), y se
// pintan todas amontonadas en la esquina.
func (v *Versal) Ver(si bool) {
	for _, t := range v.letras {
		if si {
			t.Show()
		} else {
			t.Hide()
		}
	}
}

// Objeto es el rótulo como objeto colocable: mide su caja de línea y, si le dan más
// alto, va centrado en él.
//
// ponytail: Drive (`enVersal`) y Photos (`VersalRepartido`) llevan aún su envoltorio;
// Mail y ERP ya usan este.
func (v *Versal) Objeto() fyne.CanvasObject {
	return container.New(versalColocado{v}, v.Objetos()...)
}

type versalColocado struct{ v *Versal }

func (c versalColocado) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(c.v.ancho, c.v.escalon.Linea)
}

func (c versalColocado) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	c.v.Colocar(0, (t.Height-c.v.escalon.Linea)/2)
}

// Pintar cambia el color de todas las letras.
func (v *Versal) Pintar(c color.NRGBA) {
	for _, t := range v.letras {
		t.Color = c
		t.Refresh()
	}
}

// Contador es `<Badge count>`: el círculo de no leídos del menú lateral. `h-[18px]
// min-w-[18px] px-1 text-xs`, píldora, con el relleno del ROL primario y SU rótulo
// (`bg-primary text-on-primary`), que conmutan juntos: el color con blanco en claro y
// su tono claro con rótulo oscuro en oscuro.
//
// Fue el escalón 600 fijo con `on-primary` encima, y en oscuro —donde `on-primary` es
// casi negro— se quedaba en 4,1:1. Un relleno fijo no puede llevar un rótulo que conmuta.
type Contador struct {
	widget.BaseWidget

	Texto string
	// SobreCromo es el contador DENTRO del carril: el primario de allí es blanco y su
	// rótulo, del color del carril (`sidebar-primary` y `sidebar-on-primary`). Lo pone la
	// fila del menú.
	SobreCromo bool
}

// tokens son el relleno y el rótulo del contador.
func (c *Contador) tokens() (relleno, rotulo string) {
	if c.SobreCromo {
		return "sidebar-primary", "sidebar-on-primary"
	}
	return "primary", "on-primary"
}

var _ fyne.Widget = (*Contador)(nil)

// NuevoContador crea el contador con su número ya formateado.
func NuevoContador(texto string) *Contador {
	c := &Contador{Texto: texto}
	c.ExtendBaseWidget(c)
	return c
}

// AccessibilityLabel: el número se lee tal cual; el contexto lo pone la fila.
func (c *Contador) AccessibilityLabel() string { return c.Texto }

// AccessibilityRole: es texto, no un control.
func (c *Contador) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// MedidaContador es el `h-[18px] min-w-[18px] px-1`: 18 de alto, y de ancho lo que
// pida el número con su relleno, nunca menos de 18 —que es lo que lo mantiene redondo
// con un solo dígito—.
func MedidaContador(texto string) fyne.Size {
	// `tabular-nums`: «12» mide 20,219 en producción; en proporcional salían 17,41 y el
	// suelo de 18 se lo comía.
	ancho := MedirTabular(texto, tema.TextoXS, SemiNegrita) + 2*tema.Espacio(1)
	return fyne.NewSize(max(ancho, AltoContador), AltoContador)
}

// CreateRenderer construye la píldora y su número.
func (c *Contador) CreateRenderer() fyne.WidgetRenderer {
	r := &contadorRenderer{
		c:    c,
		caja: NuevaCaja(color.NRGBA{}, color.NRGBA{}, AltoContador/2), // `rounded-pill`
	}
	r.Refresh()
	return r
}

type contadorRenderer struct {
	c     *Contador
	caja  *Caja
	texto *TextoTabular
	de    string // el texto con el que se hizo `texto`
}

func (r *contadorRenderer) MinSize() fyne.Size { return MedidaContador(r.c.Texto) }

func (r *contadorRenderer) Objects() []fyne.CanvasObject {
	return append(r.caja.Objetos(), r.texto.Objetos()...)
}

func (r *contadorRenderer) Destroy() {}

// Layout centra el número. `leading-none` con `items-center`: el glifo va en el medio
// de los 18, no en el medio de una caja de línea de 14.
func (r *contadorRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	linea := tema.Escalon{Tam: tema.TextoXS.Tam, Linea: tema.TextoXS.Tam} // leading-none
	r.texto.Colocar((t.Width-r.texto.Ancho())/2, (t.Height-linea.Linea)/2, linea)
}

// Refresh repinta con los tokens del tema actual.
func (r *contadorRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	relleno, rotulo := r.c.tokens()
	r.caja.Pintar(tema.Color(relleno, v), color.NRGBA{})
	if r.texto == nil || r.de != r.c.Texto {
		r.texto, r.de = NuevoTextoTabular(r.c.Texto, tema.TextoXS, SemiNegrita, tema.Color(rotulo, v)), r.c.Texto
		r.Layout(r.c.Size())
	}
	r.texto.Tenir(tema.Color(rotulo, v))
}
