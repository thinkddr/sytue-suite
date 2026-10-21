// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/BulkActionBar.tsx

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// BarraDeLote es `BulkActionBar.tsx`: la franja que aparece al marcar varios elementos.
//
// Es de la suite y no de una app —la piden Mail, Drive y Contactos— y sus dos claves
// (`ui.bulk.region` y `ui.bulk.clear`) ya estaban en el catálogo COMÚN antes que este
// widget, que es la prueba escrita de dónde tocaba ponerla.
//
// # Las dos cosas que el componente compartido arregló, y que aquí están
//
//  1. **Una sola parada de tabulación.** Las tres barras de la web prometían
//     `role="toolbar"` y no lo cumplían: cada botón era su propia parada, así que salir
//     de una barra de seis acciones costaba seis tabulaciones. Aquí el foco ROTA con
//     las flechas (y con Inicio y Fin) y el tabulador sale de la barra de una vez. Se
//     apoya en `Boton.EnElRecorrido`, que es el «roving tabindex» de la APG; el porqué
//     de implementarlo con `fyne.Disableable` está en `boton.go`.
//  2. **Escape vacía la selección**, que es lo primero que se prueba y lo que no hacía
//     ninguna de las tres.
//
// ponytail: Escape solo llega si el foco está DENTRO de la barra. La web lo escucha en
// el documento —porque el foco se queda en la LISTA, que es donde se acaba de marcar— y
// Fyne no tiene nada equivalente: `SetOnTypedKey` es una sola ranura global y solo
// dispara cuando no hay nada enfocado. Mientras no haya un repartidor de teclas de la
// suite, la lista que tenga la barra encima llama a `Escapar()` desde su propio
// `TypedKey`; es una línea por pantalla y está dicho en la documentación del método.
//
// El recuento en palabras lo pasa quien llama: «3 seleccionados» tiene formas de plural
// distintas por idioma Y por sustantivo (correos, archivos, contactos), y esa clave ya
// existe en el catálogo de cada app.
type BarraDeLote struct {
	widget.BaseWidget

	// Recuento es el rótulo YA pluralizado por la app: «3 archivos seleccionados».
	Recuento string
	// Rotulo es el nombre accesible de la barra (`ui.bulk.region`).
	Rotulo string
	// RotuloQuitar es el del aspa (`ui.bulk.clear`).
	RotuloQuitar string
	// Acciones son los botones, en el orden en que se ven. Los construye la app: aquí
	// no se sabe ni qué hacen ni cómo se llaman.
	Acciones []*Boton
	// AlQuitar vacía la selección. Lo disparan el aspa y Escape.
	AlQuitar func()
	// Canalon SUSTITUYE al `px-3` del componente cuando la app pone el suyo: Drive le
	// pasa `px-4 sm:px-6` desde fuera, y en Tailwind la última clase gana, no se suma.
	// Cero: el `px-3` de `BulkActionBar.tsx`.
	Canalon float32

	quitar *Boton
	activo int
}

var (
	_ fyne.Widget     = (*BarraDeLote)(nil)
	_ fyne.Accessible = (*BarraDeLote)(nil)
)

// Las medidas de la barra: `flex flex-wrap items-center gap-1.5 border-b px-3 py-2`.
var (
	huecoLote    = tema.Espacio(1.5) // gap-1.5
	rellenoLoteX = tema.Espacio(3)   // px-3
	rellenoLoteY = tema.Espacio(2)   // py-2
)

// AltoBarraLote es el alto de escritorio. En táctil los botones crecen: se mide con
// MedidaBarraDeLoteEn en vez de reservar aquí un alto calculado antes de conocerlos.
var AltoBarraLote = 2*rellenoLoteY + medidas[TallaSM].alto + tema.Borde

// NuevaBarraDeLote crea la barra. `recuento` llega ya pluralizado.
func NuevaBarraDeLote(recuento, rotulo, rotuloQuitar string, acciones []*Boton, alQuitar func()) *BarraDeLote {
	b := &BarraDeLote{
		Recuento: recuento, Rotulo: rotulo, RotuloQuitar: rotuloQuitar,
		Acciones: acciones, AlQuitar: alQuitar,
	}
	b.ExtendBaseWidget(b)
	return b
}

// Con pone el canalón de la app y devuelve la barra, para encadenarla.
func (b *BarraDeLote) Con(canalon float32) *BarraDeLote {
	b.Canalon = canalon
	return b
}

// AltoPara mide la envoltura con el ancho que va a recibir, también antes del
// primer Layout. Size().Width todavía puede ser cero o el de la ventana anterior.
func (b *BarraDeLote) AltoPara(ancho float32) float32 {
	return MedidaBarraDeLoteEn(b, ancho).Height
}

// AccessibilityLabel es el nombre de la barra: una barra de herramientas sin nombre se
// anuncia como «barra de herramientas» a secas.
func (b *BarraDeLote) AccessibilityLabel() string { return b.Rotulo }

// AccessibilityRole: agrupa; lo que se pulsa son sus botones. Fyne 2.8 no tiene rol de
// barra de herramientas.
func (b *BarraDeLote) AccessibilityRole() fyne.AccessibleRole { return fyne.AccessibleRoleContainer }

// MedidaBarraDeLote es lo que ocupa en una línea: todo el ancho que le den, por su alto.
func MedidaBarraDeLote() fyne.Size {
	return fyne.NewSize(0, 2*rellenoLoteY+SueloTactil(medidas[TallaSM].alto)+tema.Borde)
}

func altoFilaLote(b *BarraDeLote) float32 {
	alto := SueloTactil(medidas[TallaSM].alto)
	for _, a := range b.Acciones {
		if a.Visible() {
			alto = max(alto, a.MinSize().Height)
		}
	}
	return alto
}

// MedidaBarraDeLoteEn es lo que ocupa con ese ancho: con `flex-wrap`, si el recuento y
// las acciones no caben en una línea, bajan a las que hagan falta, a `gap-1.5`. En el
// panel de 279 de Sytue Contacts sus cinco acciones ocupan tres. Con ancho 0, una.
//
// EN TÁCTIL no hay envoltura: una sola línea que se desplaza de lado, como la
// web en móvil. La envoltura es la respuesta de escritorio a un panel estrecho
// —enseña todo lo que hay sin gesto ninguno—; en un teléfono apilar dos o tres
// líneas de botones se come la lista que hay debajo, que es justo lo que se
// vino a marcar.
func MedidaBarraDeLoteEn(b *BarraDeLote, ancho float32) fyne.Size {
	if EsTactil() {
		return fyne.NewSize(ancho, 2*rellenoLoteY+altoFilaLote(b)+tema.Borde)
	}
	n := len(lineasLote(b, ancho))
	return fyne.NewSize(ancho, 2*rellenoLoteY+float32(n)*altoFilaLote(b)+float32(n-1)*huecoLote+tema.Borde)
}

// mrRecuento es el `mr-1` del recuento.
var mrRecuento = tema.Espacio(1)

// lineasLote reparte en líneas, como el navegador con `flex-wrap`, los índices de los
// elementos de la barra: el 0 es el recuento (con su `mr-1`), después las acciones y el
// aspa. El `flex-1` de detrás del recuento no ocupa (su base es 0): solo empuja lo que
// quepa en la primera línea hacia la derecha.
func lineasLote(b *BarraDeLote, ancho float32) [][]int {
	anchos := make([]float32, 0, len(b.Acciones)+2)
	anchos = append(anchos, Medir(b.Recuento, tema.TextoSM, Medio).Width+mrRecuento)
	for _, a := range b.Acciones {
		anchos = append(anchos, a.MinSize().Width)
	}
	anchos = append(anchos, ladoSoloIcono(TallaSM)) // el aspa
	canalon := b.Canalon
	if canalon <= 0 {
		canalon = rellenoLoteX
	}
	cabe := ancho - 2*canalon
	var lineas [][]int
	linea := make([]int, 0, len(anchos))
	usado := float32(0)
	for i, w := range anchos {
		// El `flex-1` cuenta como un elemento más: su hueco va aunque no mida nada.
		hueco := huecoLote
		if i == 1 {
			hueco = 2 * huecoLote
		}
		if len(linea) > 0 && ancho > 0 && usado+hueco+w > cabe {
			lineas, linea, usado = append(lineas, linea), make([]int, 0, len(anchos)), 0
		} else if len(linea) > 0 {
			usado += hueco
		}
		linea, usado = append(linea, i), usado+w
	}
	return append(lineas, linea)
}

// Escapar vacía la selección. Es público porque en la web esto lo escucha el DOCUMENTO
// —el foco está en la lista, no en la barra— y Fyne no tiene dónde: la pantalla que
// pinte la barra tiene que llamarlo desde el `TypedKey` de su lista.
func (b *BarraDeLote) Escapar() {
	if b.AlQuitar != nil {
		b.AlQuitar()
	}
}

// controles son los botones de la barra en orden de tabulación: las acciones y, al
// final, el aspa. Es el mismo orden en que se pintan.
func (b *BarraDeLote) controles() []*Boton {
	lista := make([]*Boton, 0, len(b.Acciones)+1)
	lista = append(lista, b.Acciones...)
	if b.quitar != nil {
		lista = append(lista, b.quitar)
	}
	return lista
}

// Activar marca cuál de los controles es LA parada de tabulación de la barra. Los demás
// se apartan del recorrido y solo se alcanzan con las flechas.
func (b *BarraDeLote) Activar(i int) {
	lista := b.controles()
	if len(lista) == 0 {
		return
	}
	b.activo = (i + len(lista)) % len(lista)
	for j, c := range lista {
		c.EnElRecorrido(j == b.activo)
	}
}

// mover es la rotación de las flechas: circular, como en un `toolbar` de la APG.
func (b *BarraDeLote) mover(desde, delta int, extremo bool) {
	lista := b.controles()
	if len(lista) == 0 {
		return
	}
	destino := desde + delta
	if extremo {
		destino = 0
		if delta > 0 {
			destino = len(lista) - 1
		}
	}
	destino = (destino + len(lista)) % len(lista)
	// El orden importa: `Activar` mete al destino en el recorrido y el gestor de foco
	// se NIEGA a enfocar lo que está fuera de él.
	b.Activar(destino)
	if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
		enfocar(c, lista[destino])
	}
}

// CreateRenderer compone la franja y ata el foco rotatorio.
func (b *BarraDeLote) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	b.quitar = NuevoBoton("", b.AlQuitar).Con(Fantasma, TallaSM)
	b.quitar.SoloIcono = true
	b.quitar.Icono = iconos.Cerrar
	b.quitar.TamIcono = 16 // el `<Icon name="close" size={16} />` de la web
	b.quitar.Rotulo = b.RotuloQuitar

	r := &barraLoteRenderer{
		b:      b,
		fondo:  canvas.NewRectangle(tema.Color("bg-subtle", v)),
		filete: canvas.NewRectangle(tema.Color("border", v)),
		texto:  Texto(b.Recuento, tema.TextoSM, Medio, tema.Color("text", v)),
	}
	r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.texto}
	// EN TÁCTIL los controles van dentro de UNA tira deslizable —la misma pieza
	// de la barra de selección de Mail (seleccion.go)—: lo que no cabe se
	// recorta y se alcanza arrastrando, y enfocar un control escondido la
	// desliza hasta él (lo hace NuevaTiraDeslizable).
	if EsTactil() {
		controles := b.controles()
		dentro := make([]fyne.CanvasObject, 0, 1+len(controles))
		dentro = append(dentro, r.texto)
		for i, c := range controles {
			c.AlMover = func(delta int, extremo bool) { b.mover(i, delta, extremo) }
			c.AlEscapar = b.Escapar
			dentro = append(dentro, c)
		}
		r.tira = NuevaTiraDeslizable(altoFilaLote(b), huecoLote, dentro...)
		r.tira.AnchoPropio = true
		r.objetos = []fyne.CanvasObject{r.fondo, r.filete, r.tira}
		b.Activar(0)
		r.Refresh()
		return r
	}
	for i, c := range b.controles() {
		c.AlMover = func(delta int, extremo bool) { b.mover(i, delta, extremo) }
		c.AlEscapar = b.Escapar
		r.objetos = append(r.objetos, c)
	}
	b.Activar(0)
	r.Refresh()
	return r
}

type barraLoteRenderer struct {
	b             *BarraDeLote
	fondo, filete *canvas.Rectangle
	texto         *canvas.Text
	// tira es la fila deslizable de táctil; nil en escritorio, donde los
	// controles cuelgan directo de la barra.
	tira    *TiraDeslizable
	objetos []fyne.CanvasObject
}

func (r *barraLoteRenderer) MinSize() fyne.Size {
	return MedidaBarraDeLoteEn(r.b, r.b.Size().Width)
}
func (r *barraLoteRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *barraLoteRenderer) Destroy()                     {}

// Layout reparte como el `flex`: el recuento a la izquierda, un `flex-1` de por medio y
// las acciones empujadas a la derecha con el aspa la última.
func (r *barraLoteRenderer) Layout(t fyne.Size) {
	r.fondo.Move(fyne.NewPos(0, 0))
	r.fondo.Resize(t)
	r.filete.Move(fyne.NewPos(0, t.Height-tema.Borde))
	r.filete.Resize(fyne.NewSize(t.Width, tema.Borde))

	canalon := r.b.Canalon
	if canalon <= 0 {
		canalon = rellenoLoteX
	}
	linea := t.Height - tema.Borde
	centrar := func(o fyne.CanvasObject, x float32) float32 {
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (linea-m.Height)/2))
		o.Resize(m)
		return m.Width
	}
	if r.tira != nil {
		r.tira.Move(fyne.NewPos(canalon, rellenoLoteY))
		r.tira.Resize(fyne.NewSize(max(t.Width-2*canalon, 0), max(linea-2*rellenoLoteY, 0)))
		return
	}
	lineas := lineasLote(r.b, t.Width)
	if len(lineas) == 1 {
		centrar(r.texto, canalon)
		derecha := t.Width - canalon
		lista := r.b.controles()
		for i := len(lista) - 1; i >= 0; i-- {
			derecha -= centrar(lista[i], derecha-lista[i].MinSize().Width) + huecoLote
		}
		return
	}
	// Varias líneas: en la primera, lo que va detrás del `flex-1` se pega a la
	// derecha; en las demás no hay `flex-1` y todo va a la izquierda.
	lista := r.b.controles()
	alto := altoFilaLote(r.b)
	for n, l := range lineas {
		y := rellenoLoteY + float32(n)*(alto+huecoLote)
		colocar := func(o fyne.CanvasObject, x float32) {
			m := o.MinSize()
			o.Move(fyne.NewPos(x, y+(alto-m.Height)/2))
			o.Resize(m)
		}
		if n == 0 {
			colocar(r.texto, canalon)
			derecha := t.Width - canalon
			for k := len(l) - 1; k >= 1; k-- {
				c := lista[l[k]-1]
				derecha -= c.MinSize().Width
				colocar(c, derecha)
				derecha -= huecoLote
			}
			continue
		}
		x := canalon
		for _, i := range l {
			c := lista[i-1]
			colocar(c, x)
			x += c.MinSize().Width + huecoLote
		}
	}
}

// Refresh repinta la franja con los tokens del tema actual.
func (r *barraLoteRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.fondo.FillColor = tema.Color("bg-subtle", v)
	r.fondo.Refresh()
	r.filete.FillColor = tema.Color("border", v)
	r.filete.Refresh()
	r.texto.Text = r.b.Recuento
	r.texto.Color = tema.Color("text", v)
	r.texto.Refresh()
	r.Layout(r.b.Size())
}
