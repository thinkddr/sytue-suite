// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// Las dos cosas que `canvas.Text` no sabe hacer y que la web da por supuestas en toda
// lista y todo párrafo: recortar con puntos suspensivos y partir en líneas.
//
// El `letter-spacing` es la tercera y NO está aquí: lo resuelve `Versal`, de
// `piezas.go`, que llegó con el cromo de navegación. Se importa.

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// Repartir parte un texto en las líneas que caben en `cabe`, como hace el navegador
// con un párrafo: se corta por espacios y una palabra que no cabe sola ocupa su línea
// y se sale (`overflow-wrap` no está puesto en los párrafos de la suite salvo en los
// cuatro escalones de titular grande).
//
// Es codicioso, que es exactamente lo que hace el algoritmo de línea de CSS: no
// reparte el sobrante como haría un compositor de tipos.
func Repartir(texto string, e tema.Escalon, p Peso, cabe float32) []string {
	palabras := strings.Fields(texto)
	if len(palabras) == 0 {
		return nil
	}
	// Un ancho que todavía no se sabe NO es un ancho de cero. Con `cabe <= 0` cada
	// palabra se pasaba de largo y salía una por línea: un párrafo de veinte palabras
	// se comía veinte renglones de pantalla, sin error y sin aviso. Lo que hace el
	// navegador con una caja sin restringir es no partir, y eso es lo que se devuelve;
	// el llamante repartirá de verdad cuando sepa cuánto cabe.
	if cabe <= 0 {
		return []string{strings.Join(palabras, " ")}
	}
	lineas := []string{palabras[0]}
	for _, w := range palabras[1:] {
		i := len(lineas) - 1
		cand := lineas[i] + " " + w
		if Medir(cand, e, p).Width <= cabe {
			lineas[i] = cand
			continue
		}
		lineas = append(lineas, w)
	}
	return lineas
}

// Truncar es el `truncate` de la web (`text-overflow: ellipsis`): recorta el texto
// con puntos suspensivos hasta que quepa en `cabe`, y devuelve lo que ocupa.
//
// Hay que escribirlo porque `canvas.Text` no trunca: quien lo hace en Fyne es
// `widget.Label`, y los widgets de serie están fuera por la regla 1 del paquete. La
// búsqueda es BINARIA sobre runas y no lineal porque cada tanteo es una llamada al
// conformador: un asunto de correo largo son decenas de llamadas por fila, y las
// filas son cinco mil.
//
// `completo` es el texto sin recortar, y hay que pasarlo siempre: una fila de
// `widget.List` se recicla, y truncar sobre lo ya truncado va comiéndose el texto
// una letra por reciclaje.
func Truncar(t *canvas.Text, completo string, e tema.Escalon, p Peso, cabe float32) float32 {
	return TruncarCara(t, completo, e, p.cara(), cabe)
}

// TruncarCara es Truncar con una cara que no es un PESO: la cursiva de `italic`, que
// en la web es un estilo y aquí es otro fichero de Figtree. Se mide con la misma cara
// con la que se pinta, por lo mismo que Medir.
func TruncarCara(t *canvas.Text, completo string, e tema.Escalon, cara fyne.Resource, cabe float32) float32 {
	var ancho float32
	t.Text, ancho = truncado(completo, e, cara, cabe)
	return ancho
}

// recorta es Truncar sin el `canvas.Text`: lo que cabe de `s` en `ancho`, con puntos
// suspensivos si ha habido que cortar. Es para quien no tiene un texto a mano —una
// línea de un párrafo, el dato de un menú—.
//
// Hasta el 2026-09-23 era OTRA búsqueda, lineal y con un borde distinto: con ancho
// cero devolvía el texto ENTERO, que se salía de su caja porque Fyne no recorta, donde
// el navegador no enseña nada.
func recorta(s string, e tema.Escalon, p Peso, ancho float32) string {
	r, _ := truncado(s, e, p.cara(), ancho)
	return r
}

// truncado es la búsqueda de los tres: el texto que cabe y lo que ocupa.
func truncado(completo string, e tema.Escalon, cara fyne.Resource, cabe float32) (string, float32) {
	medir := func(s string) float32 {
		tam, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, e.Tam, fyne.TextStyle{}, cara)
		return tam.Width
	}
	entero := medir(completo)
	if entero <= cabe {
		return completo, entero
	}
	if cabe <= 0 {
		return "", 0
	}
	runas := []rune(completo)
	bajo, alto := 0, len(runas)
	ancho := float32(0)
	for bajo < alto {
		medio := (bajo + alto + 1) / 2
		cand := string(runas[:medio]) + "…"
		if w := medir(cand); w <= cabe {
			bajo, ancho = medio, w
			continue
		}
		alto = medio - 1
	}
	r := string(runas[:bajo]) + "…"
	if bajo == 0 {
		ancho = medir(r)
	}
	return r, ancho
}

// Rotulo es un texto suelto que ocupa su CAJA DE LÍNEA de CSS, no la de la fuente.
//
// Se movió aquí desde `apps/mail` el 2026-09-22: Sytue Rooms necesitaba el mismo
// texto suelto en seis sitios y una segunda copia es exactamente lo que
// `docs/CONVENTIONS.md` §13.2 cuenta que acaba divergiendo.
//
// Existe porque un `canvas.Text` a pelo declara 14,70 de alto donde `text-base` ocupa
// 19,25, y cuando va dentro de algo que CENTRA a sus hijos —las acciones de
// `ui.CabeceraApp`, por ejemplo— esos 4,55 de diferencia mueven el texto medio píxel
// arriba respecto a la web. Con el rótulo, «17 sin leer» cae en y = 69,5, que es lo
// medido en producción.
type Rotulo struct {
	widget.BaseWidget
	Texto   string
	Escalon tema.Escalon
	Peso    Peso
	Token   string
}

var _ fyne.Widget = (*Rotulo)(nil)

// NuevoRotulo crea el texto con su escalón, su peso y su token de color.
func NuevoRotulo(texto string, e tema.Escalon, p Peso, token string) *Rotulo {
	r := &Rotulo{Texto: texto, Escalon: e, Peso: p, Token: token}
	r.ExtendBaseWidget(r)
	return r
}

// MedidaRotulo es el ancho del texto por la caja de línea. Es la única cuenta.
func MedidaRotulo(r *Rotulo) fyne.Size {
	return fyne.NewSize(Medir(r.Texto, r.Escalon, r.Peso).Width, r.Escalon.Linea)
}

// AccessibilityLabel es el propio texto.
func (r *Rotulo) AccessibilityLabel() string { return r.Texto }

// AccessibilityRole implementa fyne.Accessible.
func (r *Rotulo) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleText }

// CreateRenderer compone el único texto.
func (r *Rotulo) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	t := Texto(r.Texto, r.Escalon, r.Peso, tema.Color(r.Token, v))
	return &rotuloRenderer{r: r, t: t}
}

type rotuloRenderer struct {
	r *Rotulo
	t *canvas.Text
}

func (x *rotuloRenderer) MinSize() fyne.Size           { return MedidaRotulo(x.r) }
func (x *rotuloRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{x.t} }
func (x *rotuloRenderer) Destroy()                     {}

func (x *rotuloRenderer) Layout(fyne.Size) {
	ColocarTexto(x.t, x.r.Escalon, 0, 0)
}

func (x *rotuloRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	x.t.Text = x.r.Texto
	x.t.Color = tema.Color(x.r.Token, v)
	x.t.FontSource = x.r.Peso.Cara()
	x.t.Refresh()
	x.Layout(x.r.Size())
}

// Lineas reparte el texto del bloque en las líneas que caben en `cabe`, respetando los
// saltos de línea del propio texto (los `<br>`).
func Lineas(texto string, e tema.Escalon, peso Peso, cabe float32) []string {
	var fuera []string
	for _, trozo := range strings.Split(texto, "\n") {
		if partes := Repartir(trozo, e, peso, cabe); len(partes) > 0 {
			fuera = append(fuera, partes...)
		} else {
			fuera = append(fuera, "")
		}
	}
	return fuera
}
