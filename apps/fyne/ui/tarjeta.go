// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Card.tsx

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Medidas de Card.tsx. El cuerpo lleva `px-4 py-4`; la cabecera, `px-4 py-3` y una
// línea de un píxel del token `border-subtle` debajo.
var (
	margenTarjeta  = tema.Espacio(4)
	margenCabecera = tema.Espacio(3)
)

// escalonCabecera es el del título, CardTitle: `sytue-h4`, con la escala de la forma (y
// su peso, `PesoTitulo(4)`).
func escalonCabecera() tema.Escalon { return EscalonTitulo(4) }

// TonoTarjeta es el color de recurso de Card.tsx; el valor vacío elige blue.
type TonoTarjeta string

// Los nueve tonos de recurso y la superficie neutral.
const (
	TarjetaAzul    TonoTarjeta = "blue"
	TarjetaVioleta TonoTarjeta = "violet"
	TarjetaMenta   TonoTarjeta = "mint"
	TarjetaRosa    TonoTarjeta = "rose"
	TarjetaAmbar   TonoTarjeta = "amber"
	TarjetaCian    TonoTarjeta = "cyan"
	TarjetaCoral   TonoTarjeta = "coral"
	TarjetaLima    TonoTarjeta = "lime"
	TarjetaPizarra TonoTarjeta = "slate"
	TarjetaNeutra  TonoTarjeta = "neutral"
)

// TonosTarjeta devuelve el orden de CARD_TONES para distribuir recursos como el
// lanzador web. Devuelve una copia para que un producto no cambie la paleta global.
func TonosTarjeta() [9]TonoTarjeta {
	return [9]TonoTarjeta{
		TarjetaAzul, TarjetaVioleta, TarjetaMenta, TarjetaRosa,
		TarjetaAmbar, TarjetaCian, TarjetaCoral, TarjetaLima, TarjetaPizarra,
	}
}

// Tarjeta es Card.tsx: superficie BLANCA que se despega del lienzo con sombra suave,
// `rounded-lg`, y una cabecera opcional separada por una línea. El color de grupo de un
// recurso no tiñe la tarjeta —fue un fondo pastel hasta el rediseño—: va en su icono,
// sobre relleno sólido (`TonoSolido`), o en la tarjeta entera con `Solida`.
//
// Es el segundo widget del patrón y se escribió después del botón a propósito: enseña
// el caso que el botón no tiene, que es contener a otro `CanvasObject` sin que su
// `MinSize` se lo coma. Ver el comentario de ui.go.
type Tarjeta struct {
	widget.BaseWidget
	// Titulo, vacío, quita la cabecera y su línea.
	Titulo string
	// Cuerpo es lo que va dentro, con el `px-4 py-4` alrededor. Puede ser nil.
	Cuerpo fyne.CanvasObject
	// Tono es el color de grupo del recurso. Solo pinta la tarjeta con `Solida`; vacío
	// equivale a TarjetaAzul.
	Tono TonoTarjeta
	// Solida usa el acabado `solid` y su tinta de contraste. El cuerpo conserva
	// sus colores explícitos: quien lo construye puede usar ColorTexto.
	Solida bool
	// Plana quita la sombra. Es el `raised` de Card.tsx AL REVÉS, y a propósito: allí
	// el defecto es `raised = true`, así que invertir el nombre hace que el valor cero
	// de Go sea el de la web. Once sitios la piden plana; el borde hairline ya separa
	// la tarjeta del fondo, y en tema oscuro la sombra es negro al 6 % sobre grafito.
	Plana bool
	// SinRelleno cuelga el cuerpo sin el `px-4 py-4` de CardBody: el `<dl>` de la
	// ficha de Sytue Contacts va DIRECTAMENTE dentro de `<Card>`, y cada fila lleva su
	// propio `px-6 py-3`. Con el relleno, esas filas salían 14 px más adentro.
	SinRelleno bool
}

var _ fyne.Widget = (*Tarjeta)(nil)

// NuevaTarjeta crea la tarjeta.
func NuevaTarjeta(titulo string, cuerpo fyne.CanvasObject) *Tarjeta {
	t := &Tarjeta{Titulo: titulo, Cuerpo: cuerpo}
	t.ExtendBaseWidget(t)
	return t
}

// AltoCabecera es lo que ocupa la cabecera de una tarjeta con título, línea incluida.
// Con el título vacío es 0.
func AltoCabecera(titulo string) float32 {
	if titulo == "" {
		return 0
	}
	return escalonCabecera().Linea + 2*margenCabecera + tema.Borde
}

// MedidaTarjeta es lo que va a ocupar la tarjeta. Como en el botón, es la ÚNICA cuenta:
// el renderer la llama y el guardián compara contra ella y contra los píxeles.
//
// La SOMBRA no está aquí, y es deliberado: `box-shadow` no participa en el layout de
// CSS, así que una tarjeta mide lo mismo con sombra y sin ella. La sombra se sale de la
// caja —5,5 px por lado con `shadow-sm`— y quien tiene que ceder es un padre que
// recorte (`IsClip`), no la medida.
func MedidaTarjeta(titulo string, cuerpo fyne.CanvasObject) fyne.Size {
	return medidaTarjeta(titulo, cuerpo, margenTarjeta)
}

func medidaTarjeta(titulo string, cuerpo fyne.CanvasObject, margen float32) fyne.Size {
	interior := fyne.NewSize(0, 0)
	if cuerpo != nil {
		interior = cuerpo.MinSize()
	}
	return fyne.NewSize(
		interior.Width+2*margen+2*tema.Borde,
		interior.Height+2*margen+AltoCabecera(titulo)+2*tema.Borde,
	)
}

// margen es el relleno del cuerpo: el de CardBody, o ninguno.
func (t *Tarjeta) margen() float32 {
	if t.SinRelleno {
		return 0
	}
	return margenTarjeta
}

func (t *Tarjeta) tono() TonoTarjeta {
	switch t.Tono {
	case TarjetaAzul, TarjetaVioleta, TarjetaMenta, TarjetaRosa, TarjetaAmbar,
		TarjetaCian, TarjetaCoral, TarjetaLima, TarjetaPizarra, TarjetaNeutra:
		return t.Tono
	default:
		return TarjetaAzul
	}
}

func (t *Tarjeta) rolFondo() string {
	tono := t.tono()
	if t.Solida || t.dePleno() {
		if tono == TarjetaNeutra {
			tono = TarjetaPizarra
		}
		return "card-solid-" + string(tono)
	}
	return "surface"
}

// dePleno dice si la tarjeta es una TARJETA DE RECURSO de color pleno: en la forma
// Marco lo es toda la que diga su tono (`.sytue-card-tone` de appearance.css, que
// Card.tsx pone cuando recibe `tone`). Sin tono es un panel, blanco en las dos formas.
func (t *Tarjeta) dePleno() bool { return t.Tono != "" && !t.Solida && tema.EsMarco() }

// rolBorde es la línea de la tarjeta: sobre el blanco con sombra, la suave —la sombra
// ya dibuja el canto—; la sólida conserva `border-border`.
func (t *Tarjeta) rolBorde() string {
	if t.Solida {
		return "border"
	}
	return "border-subtle"
}

// borde es la línea ya compuesta. La tarjeta de color pleno no la lleva
// (`border-color: transparent`).
func (t *Tarjeta) borde(v tema.Variante) color.NRGBA {
	if t.dePleno() {
		return color.NRGBA{}
	}
	return BordeSobre(t.rolBorde(), t.rolFondo(), v)
}

// radioTarjeta es `rounded-card`: de la forma (16 en Marco, 14 en Carril).
func radioTarjeta() float32 { return tema.Forma("radius-card") }

// ColorTexto devuelve la tinta del título y de los cuerpos que quieran seguir el
// acabado de la tarjeta. Fyne no hereda el color de texto de su contenedor.
func (t *Tarjeta) ColorTexto(v tema.Variante) color.NRGBA {
	if t.Solida || t.dePleno() {
		tono := t.tono()
		if tono == TarjetaNeutra {
			tono = TarjetaPizarra
		}
		return tema.Color("on-card-solid-"+string(tono), v)
	}
	return tema.Color("text", v)
}

// La línea de la cabecera va encima del relleno ya pintado. En una superficie
// translúcida conserva la tinta cruda para no componer dos veces ese relleno.
func (t *Tarjeta) colorFilete(v tema.Variante) color.NRGBA {
	fondo := t.rolFondo()
	if tema.Color(fondo, v).A != 0xff {
		return tokens.Crudo("border-subtle", v)
	}
	return BordeSobre("border-subtle", fondo, v)
}

// CreateRenderer construye la geometría. Solo primitivas y widgets de este paquete.
func (t *Tarjeta) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := t.rolFondo()
	r := &tarjetaRenderer{
		t:    t,
		caja: NuevaCaja(tema.Color(fondo, v), t.borde(v), radioTarjeta()).ConTrazo(),
	}
	if !t.Plana {
		// `shadow-[var(--shadow-sm)]` con el radio de `rounded-lg`. Va de HERMANA y la
		// primera de la lista, como el diálogo y el menú: dentro de la caja la
		// recortaría el propio panel.
		r.sombra = NuevaSombra(tema.SombraSM, radioTarjeta()).SinInterior()
		r.objetos = append(r.objetos, r.sombra.Objeto())
	}
	r.objetos = append(r.objetos, r.caja.Objetos()...)
	if t.Titulo != "" {
		r.titulo = Texto(t.Titulo, escalonCabecera(), PesoTitulo(4), t.ColorTexto(v))
		r.linea = canvas.NewRectangle(t.colorFilete(v))
		r.objetos = append(r.objetos, r.titulo, r.linea)
	}
	if t.Cuerpo != nil {
		r.objetos = append(r.objetos, t.Cuerpo)
	}
	return r
}

type tarjetaRenderer struct {
	t       *Tarjeta
	sombra  *Sombra
	caja    *Caja
	titulo  *canvas.Text
	linea   *canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *tarjetaRenderer) MinSize() fyne.Size {
	return medidaTarjeta(r.t.Titulo, r.t.Cuerpo, r.t.margen())
}

func (r *tarjetaRenderer) Objects() []fyne.CanvasObject { return r.objetos }

func (r *tarjetaRenderer) Destroy() {}

// Layout coloca cabecera, línea y cuerpo. Todo en píxeles enteros: ver la regla 3.
func (r *tarjetaRenderer) Layout(t fyne.Size) {
	if r.sombra != nil {
		r.sombra.Ajustar(fyne.NewPos(0, 0), t, tema.De(fyne.CurrentApp().Settings().ThemeVariant()))
	}
	r.caja.Redimensionar(t)
	alto := AltoCabecera(r.t.Titulo)
	if r.titulo != nil {
		ColocarTexto(r.titulo, escalonCabecera(), tema.Borde+margenTarjeta, tema.Borde+margenCabecera)
		r.linea.Move(fyne.NewPos(tema.Borde, alto))
		r.linea.Resize(fyne.NewSize(t.Width-2*tema.Borde, tema.Borde))
	}
	if r.t.Cuerpo != nil {
		m := r.t.margen()
		r.t.Cuerpo.Move(fyne.NewPos(tema.Borde+m, alto+tema.Borde+m))
		r.t.Cuerpo.Resize(fyne.NewSize(
			t.Width-2*(tema.Borde+m),
			t.Height-alto-2*(tema.Borde+m),
		))
	}
}

// Refresh repinta con los colores del tema actual.
func (r *tarjetaRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	fondo := r.t.rolFondo()
	// El radio es de la forma, que puede cambiar con la app abierta.
	r.caja.CambiarRadio(radioTarjeta())
	if r.sombra != nil {
		r.sombra.CambiarRadio(radioTarjeta())
		r.sombra.Ajustar(fyne.NewPos(0, 0), r.t.Size(), v)
	}
	r.caja.Pintar(tema.Color(fondo, v), r.t.borde(v))
	if r.titulo != nil {
		r.titulo.Text = r.t.Titulo
		r.titulo.Color = r.t.ColorTexto(v)
		r.titulo.Refresh()
		r.linea.FillColor = r.t.colorFilete(v)
		r.linea.Refresh()
	}
}

// TonoSolido son los dos colores de `CardIcon`: el relleno SÓLIDO del color de grupo y
// la tinta del glifo que va encima (`bg-card-solid-<tono> text-on-card-solid-<tono>`).
// Es donde vive ahora el color de un recurso: en su icono, no en el fondo de la tarjeta.
func TonoSolido(tono TonoTarjeta, v tema.Variante) (relleno, tinta color.NRGBA) {
	t := &Tarjeta{Tono: tono, Solida: true}
	return tema.Color(t.rolFondo(), v), t.ColorTexto(v)
}
