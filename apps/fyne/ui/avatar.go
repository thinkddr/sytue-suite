// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Avatar.tsx

import (
	"image"
	"image/color"
	"math"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// TallaAvatar son las cinco tallas de Avatar.tsx. Son PÍXELES LITERALES en la web
// (`sizePx`), así que el rem de 14 no los toca.
type TallaAvatar uint8

// Las cinco tallas. El valor cero es `md`, como en la web.
const (
	AvatarMD TallaAvatar = iota
	AvatarXS
	AvatarSM
	AvatarLG
	AvatarXL
)

var pxAvatar = [...]float32{AvatarXS: 24, AvatarSM: 32, AvatarMD: 40, AvatarLG: 48, AvatarXL: 64}

// Presencia es el punto de estado de Avatar.tsx; el valor cero no pinta punto.
//
// No va solo en el color (WCAG 1.4.1): cada estado lleva su FORMA dentro del disco, en
// `surface` —en línea el disco lleno, ausente una media luna, no molestar una raya y
// desconectado un aro hueco—, con las mismas fracciones que `FORMA` de Avatar.tsx.
type Presencia uint8

// Los cuatro estados, más «sin punto».
const (
	SinPresencia Presencia = iota
	EnLinea
	Ausente
	NoMolestar
	Desconectado
)

var tokenPresencia = [...]string{
	EnLinea: "presence-online", Ausente: "presence-away",
	NoMolestar: "presence-dnd", Desconectado: "presence-offline",
}

// Token es el de la hoja para el punto de color de ese estado (`presence-online`…), o
// "" sin presencia. Es público porque el punto va también FUERA del avatar: en el
// disparador de «tu estado» de Rooms y en el menú de cuenta.
func (p Presencia) Token() string {
	if int(p) >= len(tokenPresencia) {
		return ""
	}
	return tokenPresencia[p]
}

// clavePresencia es el rótulo de cada presencia en el catálogo común.
var clavePresencia = [...]string{
	EnLinea: "presence.online", Ausente: "presence.away",
	NoMolestar: "presence.dnd", Desconectado: "presence.offline",
}

// Avatar es el de Avatar.tsx: el círculo de color con las iniciales o, con Foto, la
// foto recortada en círculo (`<img class="rounded-full object-cover">`).
//
// Lleva cáscara de widget porque se anuncia: en la web es un `role="img"` con el
// nombre de la persona como `aria-label`, y un `*fyne.Container` se anunciaría como
// «Container».
type Avatar struct {
	widget.BaseWidget
	Nombre    string
	Talla     TallaAvatar
	Presencia Presencia
	// Foto es la imagen de la persona. nil: las iniciales. Se recorta al cuadrado
	// del centro (`object-cover`) y al círculo aquí; quien la pone no hace nada.
	Foto image.Image
}

var _ fyne.Widget = (*Avatar)(nil)

// NuevoAvatar crea el avatar de talla md.
func NuevoAvatar(nombre string) *Avatar {
	a := &Avatar{Nombre: nombre}
	a.ExtendBaseWidget(a)
	return a
}

// Con devuelve el mismo avatar con otra talla.
func (a *Avatar) Con(t TallaAvatar) *Avatar { a.Talla = t; return a }

// MedidaAvatar es el cuadrado de la talla. Es la única cuenta.
func MedidaAvatar(t TallaAvatar) fyne.Size {
	p := pxAvatar[t]
	return fyne.NewSize(p, p)
}

// Iniciales es el `initials()` de Avatar.tsx, letra por letra: una palabra da sus dos
// primeras letras, varias dan la primera de la primera y la primera de la última.
func Iniciales(nombre string) string {
	partes := strings.Fields(nombre)
	switch len(partes) {
	case 0:
		return "?"
	case 1:
		r := []rune(partes[0])
		if len(r) > 2 {
			r = r[:2]
		}
		return strings.ToUpper(string(r))
	}
	primera, _ := utf8.DecodeRuneInString(partes[0])
	ultima, _ := utf8.DecodeRuneInString(partes[len(partes)-1])
	return strings.ToUpper(string(primera) + string(ultima))
}

// ColorAvatar es el `bgFor()` de Avatar.tsx: el mismo hash que los cursores y las marcas
// del control de cambios (`tokens.Persona`), para que una persona tenga el MISMO color en
// la web y en el escritorio.
func ColorAvatar(nombre string, v tema.Variante) color.NRGBA {
	return tema.Color(tokens.Persona(nombre), v)
}

// CreateRenderer construye el círculo, las iniciales y el punto de presencia.
func (a *Avatar) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	p := pxAvatar[a.Talla]
	circulo := canvas.NewRectangle(ColorAvatar(a.Nombre, v))
	circulo.CornerRadius = p / 2
	// `font-semibold` y `fontSize: Math.round(px * 0.4)`, que es un entero en la web.
	e := tema.Escalon{Tam: redondear(p * 0.4), Linea: redondear(p * 0.4)}
	r := &avatarRenderer{
		a: a, circulo: circulo,
		iniciales: Texto(Iniciales(a.Nombre), e, SemiNegrita, tema.Color("neutral-0", v)), // `text-white` en Avatar.tsx
		escalon:   e,
	}
	// `border-2` de `surface` alrededor del punto: dos rectángulos, como Caja. Se
	// construyen SIEMPRE, aunque ahora mismo no haya presencia, y se esconden: si
	// dependieran de la presencia del momento, una que llegue después —la primera que
	// contesta el Hub, que tarda lo que tarde la red— no tendría qué pintar. Es la otra
	// mitad del fallo de `Refresh`, y le pasaba justo al avatar de la barra, que es el
	// único que vive toda la sesión.
	r.aro = canvas.NewRectangle(tema.Color("surface", v))
	r.punto = canvas.NewRectangle(colorPresencia(a.Presencia, v))
	r.forma = canvas.NewRectangle(tema.Color("surface", v))
	// La foto nace SIEMPRE, escondida si no hay: por lo mismo que el punto.
	r.foto = canvas.NewImageFromImage(nil)
	r.foto.FillMode = canvas.ImageFillStretch
	r.foto.ScaleMode = canvas.ImageScaleSmooth
	r.objetos = []fyne.CanvasObject{circulo, r.iniciales, r.foto, r.aro, r.punto, r.forma}
	r.verPunto(a.Presencia)
	r.Refresh()
	return r
}

// RecorteCircular es `object-cover` y `rounded-full`: el cuadrado del centro de la
// imagen, con lo de fuera del círculo transparente y el borde suavizado (medio píxel
// de la imagen de origen, que el pintor luego escala).
func RecorteCircular(img image.Image) image.Image {
	b := img.Bounds()
	lado := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-lado)/2, b.Min.Y+(b.Dy()-lado)/2
	fuera := image.NewNRGBA(image.Rect(0, 0, lado, lado))
	radio := float64(lado) / 2
	for y := range lado {
		for x := range lado {
			dx, dy := float64(x)+0.5-radio, float64(y)+0.5-radio
			cobertura := min(max(radio-math.Sqrt(dx*dx+dy*dy)+0.5, 0), 1)
			if cobertura == 0 {
				continue
			}
			c, _ := color.NRGBAModel.Convert(img.At(x0+x, y0+y)).(color.NRGBA)
			c.A = uint8(float64(c.A)*cobertura + 0.5)
			fuera.SetNRGBA(x, y, c)
		}
	}
	return fuera
}

// colorPresencia es el token del punto. Sin presencia no hay token que pedir
// —`tokenPresencia[SinPresencia]` es "" y `tema.Color` entra en PÁNICO al pintar con un
// nombre desconocido—, así que el punto escondido se queda del color neutro.
func colorPresencia(p Presencia, v tema.Variante) color.NRGBA {
	if p == SinPresencia {
		p = Desconectado
	}
	return tema.Color(tokenPresencia[p], v)
}

func redondear(f float32) float32 {
	return float32(int(f + 0.5))
}

// AccessibilityLabel es el nombre de la persona, que es el `aria-label` de la web.
func (a *Avatar) AccessibilityLabel() string {
	if a.Presencia == SinPresencia {
		return a.Nombre
	}
	return a.Nombre + " · " + textoComun(clavePresencia[a.Presencia])
}

// AccessibilityRole implementa fyne.Accessible.
func (a *Avatar) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

type avatarRenderer struct {
	a          *Avatar
	circulo    *canvas.Rectangle
	iniciales  *canvas.Text
	foto       *canvas.Image
	fuente     image.Image // la Foto de la que sale `foto.Image`
	aro, punto *canvas.Rectangle
	// forma es la luna, la raya o el hueco del aro que distingue cada presencia.
	forma   *canvas.Rectangle
	escalon tema.Escalon
	objetos []fyne.CanvasObject
}

func (r *avatarRenderer) MinSize() fyne.Size           { return MedidaAvatar(r.a.Talla) }
func (r *avatarRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *avatarRenderer) Destroy()                     {}

// verPunto enseña o esconde el punto y su aro, que van siempre juntos, y la forma, que
// en línea no hay.
func (r *avatarRenderer) verPunto(p Presencia) {
	for _, o := range []*canvas.Rectangle{r.aro, r.punto, r.forma} {
		if p == SinPresencia || (o == r.forma && p == EnLinea) {
			o.Hide()
			continue
		}
		o.Show()
	}
}

// FormaPresencia es dónde va la forma de esa presencia dentro de un disco de lado `s`
// (posición relativa al disco, tamaño y radio de esquina); `ok` falso en línea, que es
// el disco lleno. Son las fracciones de `FORMA` en Avatar.tsx.
func FormaPresencia(p Presencia, s float32) (pos fyne.Position, tam fyne.Size, radio float32, ok bool) {
	switch p {
	case Ausente: // la luna: un disco del 70 % arriba a la derecha tapa el resto
		return fyne.NewPos(0.35*s, -0.05*s), fyne.NewSize(0.7*s, 0.7*s), 0.35 * s, true
	case NoMolestar: // la raya horizontal centrada
		h := max(1, 0.22*s)
		return fyne.NewPos(0.2*s, (s-h)/2), fyne.NewSize(0.6*s, h), 0, true
	case Desconectado: // el hueco del aro, la mitad del disco
		return fyne.NewPos(0.25*s, 0.25*s), fyne.NewSize(0.5*s, 0.5*s), 0.25 * s, true
	case SinPresencia, EnLinea:
	}
	return fyne.Position{}, fyne.Size{}, 0, false
}

func (r *avatarRenderer) Layout(t fyne.Size) {
	p := pxAvatar[r.a.Talla]
	r.circulo.Resize(fyne.NewSize(p, p))
	r.circulo.CornerRadius = p / 2
	r.foto.Resize(fyne.NewSize(p, p))
	ancho := Medir(r.iniciales.Text, r.escalon, SemiNegrita).Width
	ColocarTexto(r.iniciales, r.escalon, (p-ancho)/2, (p-r.escalon.Linea)/2)
	// `dot = Math.max(10, Math.round(px * 0.34))`, anclado a `-right-0.5 -bottom-0.5`,
	// que son 1,75 px FUERA de la caja por cada lado. El `border-2` va DENTRO de esos
	// `dot` (el `box-sizing: border-box` de la base de Tailwind): el aro mide `dot` y el
	// disco de color, `dot − 4`. Aquí se pintaba el disco a `dot` y el aro 4 más ancho.
	d := max(10, redondear(p*0.34))
	const fuera = 0.5 * 0.25 * tema.Rem
	x, y := p-d+fuera, p-d+fuera
	const borde = 2
	r.aro.Resize(fyne.NewSize(d, d))
	r.aro.CornerRadius = d / 2
	r.aro.Move(fyne.NewPos(x, y))
	s := d - 2*borde
	r.punto.Resize(fyne.NewSize(s, s))
	r.punto.CornerRadius = s / 2
	r.punto.Move(fyne.NewPos(x+borde, y+borde))
	if pos, tam, radio, ok := FormaPresencia(r.a.Presencia, s); ok {
		r.forma.Move(fyne.NewPos(x+borde+pos.X, y+borde+pos.Y))
		r.forma.Resize(tam)
		r.forma.CornerRadius = radio
	}
}

// Refresh repinta el círculo, las iniciales Y EL PUNTO.
//
// Lo del punto no estaba, y era un fallo que ningún guardián veía: `o.Refresh()` sobre
// un rectángulo lo vuelve a pintar con el color que YA tiene, así que cambiar
// `Presencia` no cambiaba nada. En una lista da igual —cada fila construye su avatar—,
// pero el de la barra de la suite vive toda la sesión: se ponía «No molestar» y el
// punto seguía gris hasta cerrar la ventana.
//
// La otra mitad era que el punto solo EXISTÍA si había presencia al construir el
// renderer, así que una app que arrancara sin sesión —las trece— no pintaba nada cuando
// el Hub contestaba. Ahora nace siempre y se esconde (ver CreateRenderer): aparecer y
// desaparecer es `Show`/`Hide`, no rehacer el renderer.
func (r *avatarRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.circulo.FillColor = ColorAvatar(r.a.Nombre, v)
	r.iniciales.Text = Iniciales(r.a.Nombre)
	r.iniciales.Color = tema.Color("neutral-0", v)
	r.punto.FillColor = colorPresencia(r.a.Presencia, v)
	r.aro.FillColor = tema.Color("surface", v)
	r.forma.FillColor = tema.Color("surface", v)
	r.verPunto(r.a.Presencia)
	if r.a.Foto != r.fuente {
		r.fuente, r.foto.Image = r.a.Foto, nil
		if r.a.Foto != nil {
			r.foto.Image = RecorteCircular(r.a.Foto)
		}
	}
	ver(r.foto, r.a.Foto != nil)
	ver(r.iniciales, r.a.Foto == nil)
	for _, o := range r.objetos {
		o.Refresh()
	}
	r.Layout(r.a.Size())
}
