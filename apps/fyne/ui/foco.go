// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Aro es el anillo de foco CANÓNICO de la suite, el mismo y único que define
// `packages/ui/src/styles.css` fuera de toda `@layer`.
//
// Hay dos formas y las dos están aquí porque las dos existen en la hoja:
//
//   - `.sytue-focus-ring` — `box-shadow: 0 0 0 2px var(--sytue-focus-ring)`. Va pegado
//     al control. Es el de los campos, las casillas y el conmutador.
//   - `.sytue-focus-ring-inset` — `box-shadow: 0 0 0 1px var(--sytue-surface),
//     0 0 0 3px var(--sytue-focus-ring)`. Abre un pelo del color de la superficie antes
//     del anillo, o el anillo se funde con el relleno de un botón sólido. Es el de los
//     botones.
//
// # Tres cosas que hay que saber para no equivocarse al usarlo
//
//  1. **El aro NO ocupa sitio.** `box-shadow` no participa en el layout de CSS, así que
//     el aro se pinta FUERA de la caja del control y la medida declarada del widget no
//     cambia al enfocarlo. Por eso `Colocar` mueve los rectángulos a coordenadas
//     negativas: Fyne no recorta los objetos de un renderer a la caja del widget, y eso
//     está comprobado en `TestElAroSePintaFueraDeLaCaja`.
//
//  2. **El radio crece con el hueco.** Una sombra con `spread` sobre una caja redondeada
//     agranda el radio en lo mismo que el `spread`; si se deja el radio del control, las
//     esquinas del aro salen más cerradas que las de la caja y se ve un aro con picos.
//
//  3. **Relleno o abierto.** El aro nace RELLENO: el anillo es un rectángulo lleno
//     que el control tapa con su caja opaca. Un control SIN caja opaca en reposo —un
//     enlace, una miga, una pestaña, una fila del menú, un botón fantasma— no lo tapa,
//     y el aro relleno le pintaba el centro entero del color del anillo: medido el
//     2026-09-22, siete widgets del catálogo enfocados eran un bloque azul (o blanco,
//     sobre el menú de tinta) con el rótulo encima. Ese control pide `Abierto()`, que
//     pinta solo la franja de fuera, como `box-shadow`.
//
//     El abierto NO es un trazo de `canvas.Rectangle`, que es lo que fue hasta el
//     2026-10-05: los dos pintores de Fyne no trazan igual. El de software pinta el
//     `StrokeWidth` a caballo del contorno y el de GL por DENTRO de la caja, así que el
//     aro salía en su sitio en los tests y un píxel más adentro en la ventana de verdad,
//     a ×1 y a ×2 (medido con `scripts/ventana-real.sh --aros`: centrado en 0 y no en −1;
//     el del botón fantasma, con hueco, dos píxeles dentro del botón). Ahora son cuatro
//     imágenes —la franja de arriba, la de abajo y los dos lados— pintadas aquí píxel a
//     píxel: una imagen la tratan igual los dos pintores, y sale exacta a cualquier
//     escala. Lo vigila el muestrario de `pantalla.Aros`, en los tests y en la ventana.
//
//     El RELLENO se queda como está: cae en su sitio en los dos pintores. A ×1 el de GL
//     le suaviza el borde y sale al 84 % —3,70:1 el peor, el botón principal en claro;
//     100 % a ×2—, que pasa de 3:1 (decidido el 2026-10-05, CONVENTIONS §13.6).
type Aro struct {
	anillo, hueco *canvas.Rectangle
	// franjas son el aro ABIERTO: arriba, abajo, izquierda y derecha. Sin ellas, relleno.
	franjas []*canvas.Raster
	sep     float32
	token   string
	abierto bool
	// Lo que pintan las franjas: las esquinas de la caja, los dos colores, y dónde está
	// y cuánto mide la caja.
	arriba, abajo float32
	tinta, fondo  color.NRGBA
	pos           fyne.Position
	caja          fyne.Size
	visible       bool
}

// SepAro es el hueco entre el control y el anillo. Son las dos formas de la hoja: sin
// hueco para lo que va sobre la superficie, y con un pelo para lo que va sobre relleno.
const (
	SinHueco  float32 = 0
	ConHueco  float32 = tema.Borde
	grosorAro         = tema.AnilloFoco
)

// NuevoAro construye el aro de un control con ese radio de esquina. Nace oculto: un aro
// siempre visible es peor que no tenerlo.
func NuevoAro(radio, sep float32, v tema.Variante) *Aro {
	return NuevoAroCon("focus-ring", radio, sep, v)
}

// NuevoAroCon es el mismo aro con OTRO token de color, y existe porque la hoja usa dos:
// sobre el menú de tinta el azul de `focus-ring` no llega a 3:1 contra el ítem activo, y
// ahí Sidebar.tsx pide `chrome-focus`, que es blanco
// (`.sytue-focus-ring-chrome`).
func NuevoAroCon(token string, radio, sep float32, v tema.Variante) *Aro {
	a := &Aro{
		anillo: canvas.NewRectangle(tema.Color(token, v)), sep: sep, token: token,
		arriba: radio, abajo: radio, tinta: tema.Color(token, v), fondo: tema.Color("surface", v),
	}
	a.anillo.CornerRadius = RadioExtendido(radio, sep+grosorAro)
	a.anillo.Hide()
	if sep > 0 {
		a.hueco = canvas.NewRectangle(a.fondo)
		a.hueco.CornerRadius = RadioExtendido(radio, sep)
		a.hueco.Hide()
	}
	return a
}

// Abierto deja el centro del aro VACÍO: en vez de un rectángulo relleno, las cuatro
// franjas de fuera de la caja. Es para el control que no tapa el centro con una caja
// opaca (ver el punto 3 del tipo). Se llama al construirlo.
func (a *Aro) Abierto() *Aro {
	if a.abierto {
		return a
	}
	a.abierto, a.anillo, a.hueco = true, nil, nil
	for i := range 4 {
		f := canvas.NewRaster(func(w, h int) image.Image { return a.pintarFranja(i, w, h) })
		f.ScaleMode = canvas.ImageScalePixels
		f.Hide()
		a.franjas = append(a.franjas, f)
	}
	return a
}

// ConEsquinas da al aro una esquina ARRIBA y otra ABAJO, para la caja que no las tiene
// iguales: la zona de escribir de Notes es `rounded-b-md` y, enfocada, arriba toma los 4
// del aro global de theme.css. Cada esquina se extiende como la del constructor.
func (a *Aro) ConEsquinas(arriba, abajo float32) *Aro {
	a.arriba, a.abajo = arriba, abajo
	if a.abierto {
		return a
	}
	esquinas(a.anillo, arriba, abajo, a.sep+grosorAro)
	if a.hueco != nil {
		esquinas(a.hueco, arriba, abajo, a.sep)
	}
	return a
}

func esquinas(r *canvas.Rectangle, arriba, abajo, s float32) {
	r.TopLeftCornerRadius, r.TopRightCornerRadius = RadioExtendido(arriba, s), RadioExtendido(arriba, s)
	r.BottomLeftCornerRadius, r.BottomRightCornerRadius = RadioExtendido(abajo, s), RadioExtendido(abajo, s)
}

// franja es dónde cae cada una de las cuatro respecto a la caja, y lo que mide. La de
// arriba y la de abajo llevan las esquinas enteras; las de los lados, el tramo recto que
// queda entre ellas.
func (a *Aro) franja(i int) (fyne.Position, fyne.Size) {
	m := a.Margen()
	tope := min(a.caja.Width, a.caja.Height) / 2
	techo := func(r float32) float32 { return float32(math.Ceil(float64(min(r, tope)))) }
	arriba, abajo := techo(a.arriba), techo(a.abajo)
	lado := max(a.caja.Height-arriba-abajo, 0)
	switch i {
	case 0:
		return fyne.NewPos(-m, -m), fyne.NewSize(a.caja.Width+2*m, m+arriba)
	case 1:
		return fyne.NewPos(-m, a.caja.Height-abajo), fyne.NewSize(a.caja.Width+2*m, abajo+m)
	case 2:
		return fyne.NewPos(-m, arriba), fyne.NewSize(m, lado)
	}
	return fyne.NewPos(a.caja.Width, arriba), fyne.NewSize(m, lado)
}

// pintarFranja pinta una de las cuatro a la resolución que pida el pintor: el anillo, de
// `sep` a `sep`+2 de la caja, y con hueco el pelo de superficie que lo separa de ella.
// Es `box-shadow: 0 0 0 1px superficie, 0 0 0 3px anillo`, sin el centro.
func (a *Aro) pintarFranja(i, w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	p, t := a.franja(i)
	if w == 0 || h == 0 || t.Width <= 0 || t.Height <= 0 {
		return img
	}
	ex, ey := float64(w)/float64(t.Width), float64(h)/float64(t.Height)
	for py := range h {
		y := float64(p.Y) + (float64(py)+0.5)/ey
		for px := range w {
			x := float64(p.X) + (float64(px)+0.5)/ex
			// La cobertura de un píxel: cuánto de él cae dentro del contorno agrandado `s`.
			dentro := func(s float32) float64 { return min(max(0.5-a.distancia(x, y, float64(s))*ex, 0), 1) }
			medio := dentro(a.sep)
			anillo, hueco := dentro(a.Margen())-medio, 0.0
			if a.sep > 0 {
				hueco = medio - dentro(0)
			}
			if total := anillo + hueco; total > 0 {
				mezcla := func(t, f uint8) uint8 { return uint8((float64(t)*anillo+float64(f)*hueco)/total + 0.5) }
				img.SetNRGBA(px, py, color.NRGBA{
					R: mezcla(a.tinta.R, a.fondo.R), G: mezcla(a.tinta.G, a.fondo.G), B: mezcla(a.tinta.B, a.fondo.B),
					A: uint8(float64(a.tinta.A)*anillo + float64(a.fondo.A)*hueco + 0.5),
				})
			}
		}
	}
	return img
}

// distancia es la que hay, con signo, de un punto al contorno de la caja agrandada `s`
// por cada lado, con la esquina que el navegador le pone a un `box-shadow` de ese
// `spread` (RadioExtendido). Negativa, dentro.
func (a *Aro) distancia(x, y, s float64) float64 {
	w, h := float64(a.caja.Width), float64(a.caja.Height)
	r := a.abajo
	if y < h/2 {
		r = a.arriba
	}
	rr := float64(RadioExtendido(min(r, min(a.caja.Width, a.caja.Height)/2), float32(s)))
	dx, dy := math.Abs(x-w/2)-(w/2+s-rr), math.Abs(y-h/2)-(h/2+s-rr)
	if dx > 0 && dy > 0 {
		return math.Hypot(dx, dy) - rr
	}
	return max(dx, dy) - rr
}

// Antes monta la lista de `Objects()` de un renderer con el aro DELANTE de todo lo
// demás, que es la única colocación correcta: el aro se pinta primero y la caja opaca
// del control le tapa la parte que se solapa.
//
// Existe porque el orden no se puede dejar a la memoria. En la tira de miniaturas de
// Sytue Diapos el aro se añadió al FINAL —que es lo natural cuando se va apilando— y
// el anillo, que es un rectángulo RELLENO más grande que el control, tapó la
// diapositiva entera de negro. Pasó los treinta guardianes del paquete y se vio
// mirando el PNG.
//
//	r.objetos = r.aro.Antes(r.caja.Objetos()...)   // así
//	r.objetos = append(r.objetos, r.aro.Objetos()...) // así NO
func (a *Aro) Antes(resto ...fyne.CanvasObject) []fyne.CanvasObject {
	return append(a.Objetos(), resto...)
}

// Objetos son los rectángulos del aro, en el orden en que se pintan. Es el escalón de
// abajo: lo normal es `Antes`, que no deja elegir mal el orden.
func (a *Aro) Objetos() []fyne.CanvasObject {
	if a.abierto {
		out := make([]fyne.CanvasObject, len(a.franjas))
		for i, f := range a.franjas {
			out[i] = f
		}
		return out
	}
	if a.hueco == nil {
		return []fyne.CanvasObject{a.anillo}
	}
	return []fyne.CanvasObject{a.anillo, a.hueco}
}

// Colocar pone el aro alrededor de una caja de `tam` que empieza en (0,0).
func (a *Aro) Colocar(tam fyne.Size) { a.ColocarEn(fyne.NewPos(0, 0), tam) }

// ColocarEn pone el aro alrededor de una caja que no empieza en el origen. Es lo que
// necesita una casilla, donde lo que se enfoca es el cuadradito de 14 y no la fila
// entera con su etiqueta: en la web el aro lo lleva el `<input>`, no el `<label>`.
func (a *Aro) ColocarEn(pos fyne.Position, tam fyne.Size) {
	a.pos, a.caja = pos, tam
	if a.abierto {
		for i, f := range a.franjas {
			p, t := a.franja(i)
			f.Move(pos.Add(p))
			f.Resize(t)
		}
		a.Ver(a.visible) // una franja sin tamaño no se enseña
		return
	}
	a.colocar(a.anillo, pos, tam, a.Margen())
	if a.hueco != nil {
		a.colocar(a.hueco, pos, tam, a.sep)
	}
}

// colocar pone un rectángulo que sobresale `m` de la caja por cada lado.
func (a *Aro) colocar(r *canvas.Rectangle, pos fyne.Position, tam fyne.Size, m float32) {
	r.Move(fyne.NewPos(pos.X-m, pos.Y-m))
	r.Resize(fyne.NewSize(tam.Width+2*m, tam.Height+2*m))
}

// Margen es lo que el aro se sale de la caja por cada lado. No se reserva en el layout
// —`box-shadow` tampoco lo hace—, pero hace falta para encuadrar una captura.
func (a *Aro) Margen() float32 { return a.sep + grosorAro }

// Ver enseña o esconde el aro. Es lo que llama un control en FocusGained/FocusLost.
func (a *Aro) Ver(visible bool) {
	a.visible = visible
	for _, o := range a.Objetos() {
		if visible && (!a.abierto || (o.Size().Width > 0 && o.Size().Height > 0)) {
			o.Show()
		} else {
			o.Hide()
		}
	}
}

// CambiarRadio adapta el aro a otra esquina del control sin reconstruirlo: el radio de
// un botón o de una fila del menú es de la FORMA, y la forma se cambia con la app
// abierta.
func (a *Aro) CambiarRadio(radio float32) {
	if a.abierto {
		if a.arriba != radio || a.abajo != radio {
			a.arriba, a.abajo = radio, radio
			a.ColocarEn(a.pos, a.caja) // las franjas se reparten según la esquina
			a.repintar()
		}
		return
	}
	a.arriba, a.abajo = radio, radio
	a.anillo.CornerRadius = RadioExtendido(radio, a.sep+grosorAro)
	if a.hueco != nil {
		a.hueco.CornerRadius = RadioExtendido(radio, a.sep)
	}
}

// repintar rehace las franjas: su imagen se guarda hasta que algo de lo que pinta cambia.
func (a *Aro) repintar() {
	for _, f := range a.franjas {
		f.Refresh()
	}
}

// CambiarToken cambia el color del anillo sin reconstruirlo: es para el control que vive
// a ratos sobre el carril y a ratos fuera (los de la barra, que por debajo de 768 es
// carril entera). Se ve en el siguiente `Pintar`.
func (a *Aro) CambiarToken(token string) { a.token = token }

// Pintar recolorea el aro al cambiar de tema, sin reconstruirlo.
func (a *Aro) Pintar(v tema.Variante) {
	tinta, fondo := tema.Color(a.token, v), tema.Color("surface", v)
	cambia := tinta != a.tinta || fondo != a.fondo
	a.tinta, a.fondo = tinta, fondo
	if a.abierto {
		if cambia {
			a.repintar()
		}
		return
	}
	a.anillo.FillColor = tinta
	a.anillo.Refresh()
	if a.hueco != nil {
		a.hueco.FillColor = fondo
		a.hueco.Refresh()
	}
}

// RadioExtendido es la esquina de un `box-shadow` de `spread` s sobre una caja de esquina r,
// como la calcula el navegador (CSS Backgrounds 3, §7.1.1): r + s, salvo que r sea menor que
// s, y entonces s se multiplica antes por 1 + (r/s − 1)³. Con r = 0 da 0: el aro de una caja
// sin redondeo es un CUADRADO, que es lo que pedían los `NuevoAro(0, …)` de las pestañas,
// Calcus, Diapos y el ERP. Antes salía una esquina de 2 alrededor de un cuadrado.
func RadioExtendido(r, s float32) float32 {
	if s <= 0 {
		return r
	}
	if r >= s {
		return r + s
	}
	k := r/s - 1
	return r + s*(1+k*k*k)
}
