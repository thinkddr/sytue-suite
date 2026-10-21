// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Las piezas de una PÁGINA de Notepads, con la referencia de Twos (styles/app.css): una
// columna de lectura para el día, un cuaderno en lista y las páginas de Tareas, Fijadas,
// Buscar, Archivo, Papelera y Compartido conmigo; el título escrito una vez y en grande,
// filas sin caja y los controles en voz baja.

// filaCentro pone a sus hijos en fila, centrados en vertical (`flex items-center gap-*`).
type filaCentro struct{ hueco float32 }

func (f filaCentro) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	n := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		if n > 0 {
			ancho += f.hueco
		}
		ancho, alto, n = ancho+m.Width, max(alto, m.Height), n+1
	}
	return fyne.NewSize(ancho, alto)
}

func (f filaCentro) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	var x float32
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		m := o.MinSize()
		o.Move(fyne.NewPos(x, (t.Height-m.Height)/2))
		o.Resize(m)
		x += m.Width + f.hueco
	}
}

// AltoSegmento es el `min-h-7` de `.notes-seg-item`.
var AltoSegmento = tema.Espacio(7)

// segmento es una opción del segmentado de Notepads (`.notes-seg-item`): texto en voz
// baja y, la elegida, con un fondo suave. No es el Boton relleno: aquí no se ejecuta
// nada, se elige qué se mira. `cuenta` es el número que la acompaña («Todas 11»), o "".
func segmento(rotulo, cuenta string, elegido bool, v tema.Variante, accion func()) *Opcion {
	tinta, peso := "text-muted", ui.Normal
	if elegido {
		tinta, peso = "text", ui.Medio
	}
	piezas := []fyne.CanvasObject{lineaTok(rotulo, tema.TextoSM, peso, tinta, v)}
	nombre := rotulo
	if cuenta != "" {
		piezas = append(piezas, lineaTok(cuenta, tema.TextoXS, ui.Normal, "text-subtle", v))
		nombre += " " + cuenta
	}
	o := NuevaOpcion(nombre, container.New(filaCentro{hueco: tema.Espacio(1.5)}, piezas...), accion)
	o.PX, o.PY = tema.Espacio(2), (AltoSegmento-tema.TextoSM.Linea)/2
	o.Superficie = "bg"
	o.Activa, o.FondoActivo = elegido, tema.Color("bg-subtle", v)
	return o
}

// ladoAro es el `size-3.5` del aro de progreso.
var ladoAro = tema.Espacio(3.5)

// arco es la ruta SVG de un arco de `fraccion` de vuelta, desde arriba y en el sentido del
// reloj, en la rejilla de 24 de los iconos. Dos radios hacen el grosor de `stroke-width
// 2.5` sobre 16 de la web, que el trazo fijo de los iconos (2 sobre 24) dejaría en la mitad.
func arco(fraccion float64) string {
	d := ""
	for _, r := range []float64{9.4, 8} {
		if fraccion >= 0.999 {
			d += fmt.Sprintf("M12 %.2fa%.2f %.2f 0 1 1 0 %.2fa%.2f %.2f 0 1 1 0 %.2f", 12-r, r, r, 2*r, r, r, -2*r)
			continue
		}
		a := 2 * math.Pi * fraccion
		largo := 0
		if fraccion > 0.5 {
			largo = 1
		}
		d += fmt.Sprintf("M12 %.2fA%.2f %.2f 0 %d 1 %.2f %.2f", 12-r, r, r, largo, 12+r*math.Sin(a), 12-r*math.Cos(a))
	}
	return d
}

// aroProgreso es el progreso de las tareas (`.notes-ring`): un aro de 14 px y «2/5».
func aroProgreso(hechas, total int, v tema.Variante) fyne.CanvasObject {
	capas := []fyne.CanvasObject{iconos.TrazoDeRuta(arco(1), ladoAro, tema.Color("border", v)).Objeto()}
	if hechas > 0 {
		capas = append(capas, iconos.TrazoDeRuta(arco(float64(hechas)/float64(total)), ladoAro, tema.Color("primary", v)).Objeto())
	}
	aro := container.New(&lado{l: ladoAro}, capas...)
	return container.New(filaCentro{hueco: tema.Espacio(1.5)}, aro,
		lineaTok(strconv.Itoa(hechas)+"/"+strconv.Itoa(total), tema.TextoXS, ui.Normal, "text-subtle", v))
}

// lado es una caja cuadrada con sus hijos apilados dentro.
type lado struct{ l float32 }

func (c *lado) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(c.l, c.l) }
func (c *lado) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(fyne.NewSize(c.l, c.l))
	}
}

// escalonTitulo es `text-2xl sm:text-3xl`: en el teléfono el título no se come media pantalla.
func (p *Pantalla) escalonTitulo() tema.Escalon {
	if p.anchoVentana() >= ui.CorteSM {
		return tema.Texto3XL
	}
	return tema.Texto2XL
}

// tituloPagina es el título de una página (`.notes-page-title`): grande, una sola vez.
func (p *Pantalla) tituloPagina(s string, v tema.Variante) fyne.CanvasObject {
	return ui.NuevoParrafo(s, p.escalonTitulo(), ui.SemiNegrita, "text", v)
}

// cabeceraPagina es el título a la izquierda y, a la derecha, lo que la página ofrezca:
// las acciones de un cuaderno en lista o el segmentado de Tareas (`.notes-page-head`).
func (p *Pantalla) cabeceraPagina(titulo string, derecha fyne.CanvasObject, interior float32, v tema.Variante) fyne.CanvasObject {
	if derecha == nil {
		return ui.AlAnchoFluido(interior, p.tituloPagina(titulo, v))
	}
	// En el teléfono lo de la derecha baja a su línea (`flex-wrap`): el título no cabe a
	// su lado y, partido en lo que le dejaba, se pisaba con las flechas.
	if p.anchoVentana() < ui.CorteSM {
		return ui.PilaBloques(
			ui.Bloque{Obj: ui.AlAnchoFluido(interior, p.tituloPagina(titulo, v)), Margen: tema.Espacio(1)},
			ui.Bloque{Obj: container.New(ui.FilaAuto{Derecha: 1}, derecha)})
	}
	// `min-w-0` del título: parte en líneas en lo que deja lo de la derecha.
	resto := max(interior-derecha.MinSize().Width-tema.Espacio(3), 1)
	return container.New(ui.FilaAuto{Hueco: tema.Espacio(3), Derecha: 1, Elastico: 1},
		ui.AlAnchoFluido(resto, p.tituloPagina(titulo, v)), derecha)
}

// reserva guarda el alto de su hijo aunque esté oculto: la pista de la captura aparece al
// entrar en el campo sin empujar lo de debajo (en la web va en absoluto sobre el hueco).
type reserva struct{ alto float32 }

func (r reserva) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(1, r.alto) }
func (r reserva) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(t)
	}
}

// anchoApp es el ancho de la ventana de la app, o el de escritorio si aún no hay ninguna.
func anchoApp() float32 {
	for _, w := range fyne.CurrentApp().Driver().AllWindows() {
		if a := w.Canvas().Size().Width; a > 0 {
			return a
		}
	}
	return 1280
}

// columnaPagina es `.notes-col .notes-page`: la columna de lectura centrada, con su
// `px-4 sm:px-6`, `pt-8` y `pb-10`.
type columnaPagina struct{}

func (columnaPagina) margenX(ancho float32) float32 {
	if ancho >= ui.CorteSM-tema.AnchoLateral {
		return tema.Espacio(6)
	}
	return tema.Espacio(4)
}

func (c columnaPagina) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := objs[0].MinSize()
	return fyne.NewSize(m.Width+2*tema.Espacio(4), m.Height+tema.Espacio(8)+tema.Espacio(10))
}

func (c columnaPagina) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	ancho := min(t.Width, AnchoLectura+2*tema.Espacio(6))
	mx := c.margenX(t.Width)
	objs[0].Move(fyne.NewPos((t.Width-ancho)/2+mx, tema.Espacio(8)))
	objs[0].Resize(fyne.NewSize(max(ancho-2*mx, 1), max(t.Height-tema.Espacio(8), 0)))
}

// paginaDe arma una página de Notepads: el título grande, lo que lleve a su derecha (el
// segmentado de Tareas, «Vaciar la papelera») y el cuerpo, en la columna de lectura.
func paginaDe(titulo string, derecha, cuerpo fyne.CanvasObject, v tema.Variante) fyne.CanvasObject {
	e := tema.Texto3XL
	if anchoApp() < ui.CorteSM {
		e = tema.Texto2XL
	}
	cab := []fyne.CanvasObject{lineaTok(titulo, e, ui.SemiNegrita, "text", v)}
	if derecha != nil {
		cab = append(cab, derecha)
	}
	return container.New(columnaPagina{}, container.New(ui.Columna{Hueco: tema.Espacio(5)},
		container.New(&filaElastica{hueco: tema.Espacio(3)}, cab...), cuerpo))
}

// fraseVacia es una página sin nada: una frase en voz baja, sin ilustración.
func fraseVacia(s string, v tema.Variante) fyne.CanvasObject {
	return ui.NuevoParrafo(s, tema.TextoBase, ui.Normal, "text-subtle", v)
}

// filasDePagina apila las filas de una página (`.notes-rows`): un píxel entre ellas y
// sangradas 8 px hacia fuera, para que su texto caiga en el borde de la columna.
func filasDePagina(filas ...fyne.CanvasObject) fyne.CanvasObject {
	return container.New(ui.Margen{Izq: -tema.Espacio(2), Der: -tema.Espacio(2)}, container.New(ui.Columna{Hueco: 1}, filas...))
}

// celdaFila es la columna fija del glifo de una fila (`.notes-stream-glyph`, `w-5`).
func celdaFila(dentro fyne.CanvasObject) fyne.CanvasObject {
	return container.New(&celdaGlifo{lado: ColumnaGlifoFila}, dentro)
}

// glifoDeHit es lo que abre la fila de un resultado: el glifo de una tarea o de un
// enlace, y la viñeta para lo demás.
func glifoDeHit(tipo string, v tema.Variante) fyne.CanvasObject {
	if tipo == "todo" || tipo == "link" {
		if d := GlifoDeTipo(tipo); d != "" {
			return celdaFila(Glifo(d, 14, "text-subtle", v))
		}
	}
	return celdaFila(punto(VinetaFila, tema.Color("text-subtle", v)))
}

// rotuloGrupo es el rótulo de un grupo dentro de una página (`.notes-group`).
func rotuloGrupo(s string, v tema.Variante) fyne.CanvasObject {
	return lineaTok(strings.ToUpper(s), tema.TextoXS, ui.Medio, "text-subtle", v)
}
