// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ui son los widgets PROPIOS de la suite para Fyne.
//
// # El patrón. Léelo entero antes de escribir el segundo widget
//
// Fyne no está pensado para paridad a píxel: trae un juego de widgets con tema propio y
// escalado por densidad. Llegar al píxel con `widget.Button` y `theme.Size()` no se
// puede, y no por falta de maña. Lo que sí se puede —medido, no supuesto— es bajar al
// `fyne.CanvasObject` y gobernar la geometría nosotros. Eso es este paquete.
//
// Seis reglas. Las cuatro primeras salen de medir Fyne v2.8.1 el 2026-09-21 con el
// pintor de software; la quinta y la sexta, de cómo se rompe lo que no se mide.
//
//  1. **Solo primitivas.** Un widget de la suite se compone de `canvas.Rectangle`,
//     `canvas.Text`, `canvas.Image`, `canvas.Line` y otros widgets de ESTE paquete.
//     Nunca `widget.Button`, `widget.Label` ni `container.NewVBox`: cada uno trae su
//     propio relleno sacado del tema y su propio redondeo, y entonces la medida final
//     la decide Fyne.
//
//  2. **La medida se declara una vez y es pública.** Cada widget tiene una función
//     `MedidaX(...) fyne.Size` que devuelve exactamente lo que el widget va a ocupar.
//     El `MinSize()` del renderer la llama a ella; el guardián compara contra ella Y
//     contra los píxeles pintados. Calcular la medida en dos sitios es cómo se acaba
//     con un widget que dice 32 y pinta 33.
//
//  3. **Los decimales de la web se escriben tal cual y NO se redondean.** Fyne
//     cuantiza al píxel de DISPOSITIVO, así que lo que a ×1 sale medio píxel largo, a
//     ×2 —Retina, HiDPI, cualquier teléfono— sale exacto: 31,5 son 63 filas clavadas.
//     Redondear en nuestro código congela a ×1 un error que la pantalla arregla sola.
//     Los guardianes miden a ×2, que es donde se ve si la cuenta es correcta. Detalle
//     en `tema/medidas.go`.
//
//  4. **El borde no es un trazo.** `canvas.Rectangle.StrokeWidth = 1` pinta el trazo
//     CENTRADO en el contorno, así que reparte su cobertura entre dos filas y sale a
//     media fuerza: medido, (240,242,243) donde la web pone (227,230,232) — el mismo
//     síntoma que tuvo Gio por otra causa. Un borde es un rectángulo RELLENO del color
//     del borde con el radio pedido, y encima el relleno a un píxel por dentro. Eso es
//     `Caja`, y hay que usarla.
//
//     Y hay otra razón, medida el 2026-10-05: ese «centrado» es del pintor de SOFTWARE,
//     el de los tests. El de GL pinta el trazo por DENTRO de la caja, así que un trazo
//     sale en un sitio en el guardián y en otro en la ventana. Le pasó al aro de foco
//     abierto (ver `Aro`): lo que se traza no lo protege ningún test.
//
//  5. **Colores por token, nunca por `fyne.Theme`.** `tema.Color("border-strong", v)`.
//     El mapa de Fyne tiene cuarenta nombres y la hoja ciento cincuenta.
//
//  6. **Un widget nuevo llega con su guardián**, y el guardián compara con el número
//     MEDIDO en producción, no con lo que calcula el propio widget. Un test que compara
//     la implementación consigo misma pasa siempre. Ver `boton_test.go`, que es el
//     ejemplo a copiar: mide la caja declarada, la caja pintada y el color del píxel
//     del borde.
//
//  7. **`CreateRenderer` termina llamando a `r.Refresh()`.** Fyne NO llama al `Refresh`
//     del renderer antes del primer pintado: llama a `Layout` y pinta lo que haya. Un
//     renderer que nazca a medias —un color todavía a cero, un hijo que solo se
//     materializa al repintar— sale INVISIBLE, y se arregla solo en cuanto algo toque
//     la pantalla, que es lo que hace que no se note en una sesión de desarrollo.
//
//     No es teórico: le pasó a DOS widgets del catálogo a la vez. `Pestana` nacía con
//     el rótulo en `color.NRGBA{}` y pintaba **0 píxeles** de tinta recién montada
//     (1 623 tras un `Refresh`). Y `Entrada` no pintaba su MARCA DE AGUA —2 324 píxeles
//     con ella y sin ella, ni uno del rótulo— porque `widget.Entry` solo la materializa
//     en su propio `Refresh`; eso es todo campo de búsqueda de la suite.
//
//     Lo vigila `TestNingunWidgetNaceInvisible`, que monta cada widget del catálogo,
//     lo pinta SIN repintar y compara con lo que pinta después de un `Refresh`.
//
// # El techo tipográfico, y por qué ya no está
//
// El conformador que usan Fyne y Gio (github.com/go-text/typesetting) escalaba la
// fuente a `ceil(tamaño)` antes de colocar un glifo, así que los tamaños fraccionarios
// de la escala de la web no existían: `text-base` (12,25 px) se conformaba como 13 y
// cada rótulo salía un +6,12 % más ancho, a ×1 igual que a ×2. Lo quita un `replace` a
// un fork de una línea; está explicado en `apps/fyne/go.mod` y en el ADR 0048, y lo
// vigila `TestElConformadorNoRedondea`.
//
// Lo que queda por debajo es el redondeo de harfbuzz a 1/64 px por glifo —medido, 0,23 %
// en el peor caso— y que el pintor pone el origen de cada tirada en un píxel de
// dispositivo entero.
package ui

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/tokens"
)

// Peso es el `font-weight` de la web. Fyne solo distingue negrita en `fyne.TextStyle`,
// así que los pesos intermedios se piden por `FontSource`, que es lo que hacen Texto y
// Medir.
type Peso uint8

// Los cuatro pesos que usa la suite (`font-normal`, `font-medium`, `font-semibold`,
// `font-bold`), y dos CARAS que no son un peso pero se piden igual: `italic` y
// `font-mono`, las dos en su peso normal.
//
// ponytail: van en Peso y no en un eje aparte porque lo único que decide un peso es qué
// fichero de letra se usa (Peso.Cara), y medir, repartir, truncar y pintar ya pasan por
// ahí: con dos constantes más, `ui.Texto`, `ui.Parrafo` y `ui.Repartir` las admiten sin
// tocarlos. Si un día hace falta negrita cursiva o mono en negrita, se parte en dos ejes.
const (
	Normal Peso = iota
	Medio
	SemiNegrita
	Negrita
	// Cursiva es `italic` (Figtree Italic).
	Cursiva
	// Mono es `font-mono` (JetBrains Mono).
	Mono
	// ExtraNegrita es el 800 (`font-extrabold`): el peso de los títulos de la hoja en la
	// forma Marco. Va al final para no mover los valores de los demás.
	ExtraNegrita
)

// Cara es el recurso de fuente de este peso. Es pública porque una fila que se
// RECICLA cambia de peso sin rehacer el `canvas.Text` —leído a no leído, cincuenta
// veces por pantalla en una bandeja—, y quien lo hace vive fuera de este paquete.
func (p Peso) Cara() fyne.Resource { return p.cara() }

func (p Peso) cara() fyne.Resource {
	switch p {
	case Medio:
		return tema.Media
	case SemiNegrita:
		return tema.SemiNegra
	case Negrita:
		return tema.Negra
	case ExtraNegrita:
		return tema.ExtraNegra
	case Cursiva:
		return tema.Cursiva
	case Mono:
		return tema.Mono
	case Normal:
	}
	return tema.Regular
}

// Medir devuelve la CAJA CSS de un texto: el ancho que el conformador le da con esa
// cara y ese tamaño, y de alto la caja de línea del escalón.
//
// Las dos mitades tienen su motivo:
//
//   - El ancho no se pide a `fyne.MeasureText`, que mide siempre con la fuente del tema
//     —la Regular—: un rótulo semi-negrita es más ancho, y medir con una cara y pintar
//     con otra es cómo se recorta la última letra de un botón.
//   - El alto NO es el que devuelve Fyne. `canvas.Text` contesta con la caja de línea
//     de la FUENTE —1,2 em—: `text-base` (12,25 px) da 14,70. La que ocupa sitio en el
//     layout de la web es la caja de línea de CSS —19,25 para el texto raíz—, y es la
//     que hay que usar. El glifo se centra dentro de ella: ver ColocarTexto.
func Medir(texto string, e tema.Escalon, p Peso) fyne.Size {
	return MedirConCara(texto, e, p.cara())
}

// MedirConCara mide con la misma cara explícita que se utilizará al pintar.
func MedirConCara(texto string, e tema.Escalon, cara fyne.Resource) fyne.Size {
	// Sin texto no hay nada que conformar, y así una medida puede ser una variable de
	// paquete: `AltoKPI` del ERP mide una insignia vacía ANTES de que exista la app, y el
	// binario se caía al arrancar.
	if texto == "" {
		return fyne.NewSize(0, e.Linea)
	}
	tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(texto, e.Tam, fyne.TextStyle{}, cara)
	return fyne.NewSize(tam.Width, e.Linea)
}

// Texto crea el `canvas.Text` de la suite: cara de Figtree explícita y tamaño de la
// escala. Se mide con Medir y los dos usan la MISMA cara.
func Texto(s string, e tema.Escalon, p Peso, c color.Color) *canvas.Text {
	return TextoConCara(s, e, p.cara(), c)
}

// TextoConCara crea un texto con una cara de marca o de reserva ya resuelta.
func TextoConCara(s string, e tema.Escalon, cara fyne.Resource, c color.Color) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = e.Tam
	t.FontSource = cara
	return t
}

// ColocarTexto pone un texto con su esquina superior izquierda en (x, y) DE SU CAJA DE
// LÍNEA, con la línea base donde la pone Chromium.
//
// # Dónde pone Chromium la línea base, medido
//
// Centrar la caja de la fuente (1,2 em en Figtree) en la caja de línea —lo que se hacía
// hasta el 2026-09-23— es la cuenta de CSS sobre el papel, pero no la del navegador ni la
// del pintor de Fyne, y la tinta caía de 1 a 1,5 px más abajo que en la web en seis
// escalones de seis (medido en el frontal, Chromium con `--font-render-hinting=none`, a
// ×2). Son dos redondeos por cada lado:
//
//   - Blink redondea el ascendente y el descendente de la fuente a píxeles ENTEROS, y la
//     media interlínea hacia abajo: línea base = arriba + ⌊(línea − (⌊A⌉ + ⌊D⌉)) / 2⌋ + ⌊A⌉.
//     Con esa cuenta caen EXACTAS las seis medidas (`TestLaLineaBaseEsLaDeChromium`).
//   - El pintor de Fyne (los dos: el de software y el de GL comparten `DrawString`)
//     pone la línea base en ⌈y·escala⌉ + ⌈ascendente·escala⌉: dos techos, hasta dos
//     píxeles de dispositivo de más.
//
// Así que aquí se calcula la línea base de Blink y se coloca el texto para que los dos
// techos caigan justo en ella a ×2 —Retina, HiDPI, cualquier teléfono y los guardianes—.
// A ×1 queda a lo sumo un píxel.
//
// ponytail: a ×1 y a ×3 puede quedar un píxel de dispositivo; lo exacto a toda escala es
// que el pintor del fork ponga la línea base en ⌊(y + ascendente)·escala⌉, y entonces
// aquí sobra el ⌈2·A⌉/2.
func ColocarTexto(t *canvas.Text, e tema.Escalon, x, y float32) {
	caja := t.MinSize() // la de la FUENTE, que es la que Fyne va a pintar
	t.Move(fyne.NewPos(x, y+desfaseTexto(t.TextSize, e.Linea, t.FontSource, t.TextStyle)))
	t.Resize(caja)
}

// LineaBase es dónde cae la línea base de un texto de ese escalón y ese peso DENTRO de su
// caja de línea, contando desde arriba, tal como la pinta ColocarTexto. Es lo que necesita
// un `items-baseline`: alinear dos textos de escalones distintos por su línea base.
func LineaBase(e tema.Escalon, p Peso) float32 {
	return LineaBaseConCara(e, p.cara())
}

// LineaBaseConCara alinea una reserva con la línea base de la cara principal.
func LineaBaseConCara(e tema.Escalon, cara fyne.Resource) float32 {
	return lineaBaseBlink(e.Tam, e.Linea, cara, fyne.TextStyle{})
}

// DesfaseTexto es cuánto baja ColocarTexto el `canvas.Text` desde el borde de arriba de
// su caja de línea de alto `linea`. Lo necesita quien recupera la caja de línea de un
// texto ya colocado para compararla con la web: la caja es la posición menos esto.
func DesfaseTexto(t *canvas.Text, linea float32) float32 {
	return desfaseTexto(t.TextSize, linea, t.FontSource, t.TextStyle)
}

// lineaBaseBlink es la línea base de Blink dentro de la caja de línea (ver ColocarTexto).
func lineaBaseBlink(tam, linea float32, cara fyne.Resource, estilo fyne.TextStyle) float32 {
	caja, base := fyne.CurrentApp().Driver().RenderedTextSize("H", tam, estilo, cara)
	a := float32(math.Round(float64(base)))
	d := float32(math.Round(float64(caja.Height - base)))
	if linea < a+d {
		return lineaBaseApretada(linea, base, caja.Height-base)
	}
	return float32(math.Floor(float64(linea-(a+d))/2)) + a
}

// lineaBaseApretada es la cuenta de Blink cuando el medio interlineado es NEGATIVO (una
// caja de línea más baja que la fuente: `leading-none`). Ahí la redondez de la subida y
// la bajada decide el píxel, y Blink la hace a ×2 (zoom por DSF) y, en Linux con
// posicionamiento subpíxel, si la bajada se redondeó hacia abajo le quita una unidad a
// la subida para que la cola de las letras no se corte (`SimpleFontData::PlatformInit`).
// Medido contra la web en la insignia y el contador de `text-xs`: sin esto caían un píxel
// de dispositivo más arriba.
func lineaBaseApretada(linea, sube, baja float32) float32 {
	s := math.Round(float64(2 * sube))
	b := math.Round(float64(2 * baja))
	if b < float64(2*baja) && s >= 1 {
		s, b = s-1, b+1
	}
	return float32(math.Floor((float64(2*linea)-(s+b))/2)+s) / 2
}

// desfaseTexto es cuánto hay que bajar el `canvas.Text` desde el borde de arriba de su caja
// de línea para que el pintor ponga su línea base en la de Blink a ×2.
func desfaseTexto(tam, linea float32, cara fyne.Resource, estilo fyne.TextStyle) float32 {
	_, base := fyne.CurrentApp().Driver().RenderedTextSize("H", tam, estilo, cara)
	// El pintor pone la línea base ⌈2·base⌉ píxeles por debajo del borde de arriba del
	// texto, así que el texto va ⌈2·base⌉/2 por encima de la línea base que se quiere.
	return lineaBaseBlink(tam, linea, cara, estilo) - float32(math.Ceil(float64(2*base)))/2
}

// Caja es el fondo de un control: un relleno con su radio y, si lleva, un borde de un
// píxel por fuera.
//
// Son DOS rectángulos y no uno con `StrokeWidth` porque el trazo de Fyne va centrado en
// el contorno y sale a media fuerza (ver la regla 4 del paquete). El de fuera es del
// color del borde; el de dentro, del relleno, un píxel más pequeño por cada lado.
//
// Sin borde (`borde` con alfa 0) el rectángulo exterior no se pinta, así que una caja
// sin borde cuesta lo mismo que antes.
//
// Un relleno TRANSLÚCIDO con borde puede usar ConTrazo para dejar ver lo que hay
// detrás. El modo original de dos rectángulos necesita precomponerlo antes:
// el rectángulo exterior opaco cubre también el interior de la caja.
type Caja struct {
	fuera, dentro  *canvas.Rectangle
	conBorde       bool
	trazo          bool
	grosor         float32
	relleno, borde color.NRGBA
	radio          float32
	tamano         fyne.Size
}

// NuevaCaja construye la caja. `radio` es el de la esquina exterior; la interior lleva
// uno menos, que es lo que hace el navegador con `border-radius` y un borde de 1px.
func NuevaCaja(relleno, borde color.NRGBA, radio float32) *Caja {
	c := &Caja{
		fuera:    canvas.NewRectangle(bordeSobre(borde, relleno)),
		dentro:   canvas.NewRectangle(relleno),
		conBorde: borde.A != 0,
		grosor:   tema.Borde,
		relleno:  relleno,
		borde:    borde,
		radio:    radio,
	}
	c.fuera.CornerRadius = radio
	c.dentro.CornerRadius = radio
	if c.conBorde {
		c.dentro.CornerRadius = max(radio-tema.Borde, 0)
	}
	return c
}

// ConBorde conserva el contrato del puente Astro: ancho CSS medido, incluso
// fraccionario. Los controles de la suite mantienen un píxel por defecto.
func (c *Caja) ConBorde(grosor float32) *Caja {
	c.grosor = max(grosor, 0)
	c.dentro.CornerRadius = max(c.radio-c.grosor, 0)
	if c.grosor == 0 {
		c.conBorde = false
	}
	c.Pintar(c.relleno, c.borde)
	c.Redimensionar(c.tamano)
	return c
}

// ConTrazo evita un fondo opaco debajo del relleno translúcido. El borde ocupa
// el mismo píxel interior, con su eje a medio píxel: no amplía el layout ni los
// radios exteriores. Los rellenos opacos conservan los dos rectángulos originales.
func (c *Caja) ConTrazo() *Caja {
	if c.trazo {
		return c
	}
	c.trazo = true
	c.Pintar(c.relleno, c.borde)
	return c
}

// CambiarRadio adapta las esquinas sin reconstruir los objetos ni mover la caja.
// El borde sigue dentro: su radio interior es uno menos que el exterior.
func (c *Caja) CambiarRadio(radio float32) {
	if c.radio == radio {
		return
	}
	c.radio = radio
	c.fuera.CornerRadius = radio
	c.dentro.CornerRadius = radio
	if c.conBorde {
		c.dentro.CornerRadius = max(radio-c.grosor, 0)
		if c.trazo && c.relleno.A != 0xff {
			c.fuera.CornerRadius = max(radio-c.grosor/2, 0)
		}
	}
	c.fuera.Refresh()
	c.dentro.Refresh()
}

// Objetos son los rectángulos de la caja, en el orden en que se pintan. Va al principio
// de los `Objects()` del renderer.
func (c *Caja) Objetos() []fyne.CanvasObject {
	if !c.conBorde {
		return []fyne.CanvasObject{c.dentro}
	}
	return []fyne.CanvasObject{c.fuera, c.dentro}
}

// Redimensionar coloca la caja en (0,0) con el tamaño pedido. Un tamaño negativo —un
// padre que aún no ha repartido, un hueco que resta más de lo que hay— se queda en cero:
// el rasterizador de las esquinas entra en pánico con un lado negativo (visto en Docus
// con −2).
func (c *Caja) Redimensionar(t fyne.Size) {
	t = fyne.NewSize(max(t.Width, 0), max(t.Height, 0))
	c.tamano = t
	if c.trazo && c.conBorde && c.relleno.A != 0xff {
		c.fuera.Move(fyne.NewPos(c.grosor/2, c.grosor/2))
		c.fuera.Resize(fyne.NewSize(max(t.Width-c.grosor, 0), max(t.Height-c.grosor, 0)))
	} else {
		if c.trazo {
			c.fuera.Move(fyne.NewPos(0, 0))
		}
		c.fuera.Resize(t)
	}
	if !c.conBorde {
		c.dentro.Move(fyne.NewPos(0, 0))
		c.dentro.Resize(t)
		return
	}
	c.dentro.Move(fyne.NewPos(c.grosor, c.grosor))
	// Nunca negativo: una caja con borde que todavía mide 0 —un widget recién creado al
	// que aún no le ha llegado el layout— dejaba el relleno en −2 × −2, y el pintor de
	// software de Fyne entra en pánico al rasterizar un rectángulo redondeado así.
	c.dentro.Resize(fyne.NewSize(max(t.Width-2*c.grosor, 0), max(t.Height-2*c.grosor, 0)))
}

// Pintar cambia los colores sin reconstruir nada: es lo que llama un renderer en su
// Refresh cuando el puntero entra o sale.
//
// Lo que NO cambia es si la caja lleva borde o no: eso lo decide la VARIANTE del
// control y no su estado —un botón secundario siempre lo lleva, uno fantasma nunca—,
// así que se fija al construirla. Si algún día hace falta un control que gane borde al
// pasar por encima, se reconstruye la caja, no se parchea esto: el rectángulo de fuera
// solo está en `Objetos()` cuando se declaró con borde.
func (c *Caja) Pintar(relleno, borde color.NRGBA) {
	c.relleno, c.borde = relleno, borde
	c.dentro.FillColor = relleno
	c.dentro.Refresh()
	if c.conBorde {
		filete := bordeSobre(borde, relleno)
		if c.trazo && relleno.A != 0xff {
			c.fuera.FillColor = color.Transparent
			c.fuera.StrokeColor = filete
			c.fuera.StrokeWidth = c.grosor
			c.fuera.CornerRadius = max(c.radio-c.grosor/2, 0)
		} else {
			c.fuera.FillColor = filete
			if c.trazo {
				c.fuera.StrokeColor = color.Transparent
				c.fuera.StrokeWidth = 0
				c.fuera.CornerRadius = c.radio
			}
		}
		if c.trazo {
			c.Redimensionar(c.tamano)
		}
		c.fuera.Refresh()
	}
}

// bordeSobre compone el borde de la hoja sobre el relleno OPACO de la propia caja, que es
// lo que hace el navegador (`background-clip: border-box`: el `rgba(tinta, .12)` del borde
// se ve sobre el fondo del elemento). `tema.Color` da los bordes aplanados sobre
// `surface`, y en una caja de otro relleno eso no acierta: medido el 2026-09-27 en oscuro,
// el borde de los menús y de las tarjetas sobre `surface-raised` salía #2d3b42 y la web
// pinta #32434b (Δ 9). Con relleno translúcido conserva el alfa; un borde ya
// compuesto queda como viene.
func bordeSobre(borde, relleno color.NRGBA) color.NRGBA {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if crudo, ok := tokens.BordeCrudo(borde, v); ok {
		if relleno.A != 0xff {
			return tokens.SobreConAlfa(crudo, relleno)
		}
		return tokens.SobreSRGB(crudo, relleno)
	}
	return borde
}

// Fila y Columna son los `fyne.Layout` de la suite: hijos uno detrás de otro con un
// hueco constante, cada uno a su MinSize.
//
// Son propios y no `layout.NewHBoxLayout()` porque el de serie usa
// `theme.Padding()` como hueco —que el ajuste de tamaño de letra del sistema mueve— y
// reparte el sobrante entre los hijos. Aquí el hueco es el que se pide, en píxeles, y
// nadie se estira: la web tampoco lo hace.
type Fila struct{ Hueco float32 }

var _ fyne.Layout = Fila{}

// MinSize suma los anchos más los huecos, y toma el alto del más alto.
func (f Fila) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		if i > 0 {
			ancho += f.Hueco
		}
		ancho += m.Width
		alto = max(alto, m.Height)
	}
	return fyne.NewSize(ancho, alto)
}

// Layout coloca los hijos de izquierda a derecha, en píxeles enteros.
func (f Fila) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	var x float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(x, 0))
		o.Resize(m)
		x += m.Width + f.Hueco
	}
}

// Columna es Fila en vertical.
//
// OJO con el valor cero: por defecto ESTIRA a los hijos al ancho de la columna. Es
// `items-stretch`, el defecto de CSS para un `flex-col`, y es lo que quiere una lista
// de filas. No es lo que quiere una miniatura: en Sytue Diapos una de 128 salió a 145
// y su contenido quedó descentrado. Para eso está `Encaja`.
type Columna struct {
	Hueco float32
	// Encaja es `items-center`: cada hijo se queda con su ancho mínimo y va centrado
	// en la columna, en vez de estirarse.
	Encaja bool
}

var _ fyne.Layout = Columna{}

// MinSize suma los altos más los huecos, y toma el ancho del más ancho.
func (c Columna) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		if i > 0 {
			alto += c.Hueco
		}
		alto += m.Height
		ancho = max(ancho, m.Width)
	}
	return fyne.NewSize(ancho, alto)
}

// AltoCon cuenta el reparto de sus hijos al ancho disponible. El mínimo de un
// párrafo todavía sin colocar no sabe cuántas líneas tendrá en un teléfono.
func (c Columna) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	var alto float32
	visibles := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		if visibles > 0 {
			alto += c.Hueco
		}
		if c.Encaja {
			alto += o.MinSize().Height
		} else {
			alto += AltoA(o, ancho)
		}
		visibles++
	}
	return alto
}

// Layout coloca los hijos de arriba abajo, en píxeles enteros: estirados al ancho de
// la columna, o con el suyo y centrados si `Encaja`.
func (c Columna) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var y float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		if c.Encaja {
			o.Move(fyne.NewPos((t.Width-m.Width)/2, y))
			o.Resize(m)
		} else {
			m.Height = AltoA(o, t.Width)
			o.Move(fyne.NewPos(0, y))
			o.Resize(fyne.NewSize(t.Width, m.Height))
		}
		y += m.Height + c.Hueco
	}
}

// Pila es `flex h-full flex-col` con UN hijo en `flex-1`: los demás a su alto con el
// ancho asignado, y el elástico se lleva lo que sobra.
//
// Es la disposición de toda pantalla de la suite —cabecera fija, cuerpo que crece, pie
// fijo—, así que vive aquí y no en una app. Nació en `apps/drive`, donde sigue
// referenciada por su nombre viejo; `apps/photos` tenía además su versión horizontal
// (`filaConCampoElastico`), que es lo que pasa cuando una pieza compartida se queda
// dentro de un producto.
//
// Es propia y no `layout.NewBorderLayout` porque el de serie separa los hijos con
// `theme.Padding()` —7 px con este tema— y entre una cabecera y su lista no hay hueco.
type Pila struct {
	// Elastico es el índice del hijo que crece. Fuera de rango: no crece ninguno, y la
	// pila mide lo que suman sus hijos.
	Elastico int
}

var _ fyne.Layout = Pila{}

// MinSize suma los altos y toma el ancho del más ancho.
func (p Pila) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		ancho = max(ancho, m.Width)
		alto += m.Height
	}
	return fyne.NewSize(ancho, alto)
}

// Layout reparte: los fijos a su alto con el ancho real y el elástico a lo que quede,
// nunca negativo. Medir antes de redimensionarlos evita usar la envoltura anterior.
func (p Pila) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var fijo float32
	for i, o := range objs {
		if i != p.Elastico && o.Visible() {
			fijo += AltoA(o, t.Width)
		}
	}
	var y float32
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		alto := max(t.Height-fijo, 0)
		if i != p.Elastico {
			alto = AltoA(o, t.Width)
		}
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, alto))
		y += alto
	}
}
