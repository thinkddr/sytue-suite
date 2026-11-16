// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/iconos"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// Los bloques de las pantallas de asistente del Hub: una columna de piezas cuyo alto
// depende del ancho —párrafos que se reparten, filas que se parten en el móvil—.
//
// El alta y las pantallas del Hub reutilizan estas piezas. `fyne.Layout` no recibe
// el ancho en su `MinSize`, y aquí el alto depende del texto repartido.
type bloque interface {
	alto(ancho float32) float32
	// poner coloca el bloque con su esquina en (x, y) y ese ancho.
	poner(x, y, ancho float32)
	objetos() []fyne.CanvasObject
}

// columna apila bloques con un hueco (`flex flex-col gap-N`).
type columna struct {
	hueco float32
	hijos []bloque
}

func col(hueco float32, hijos ...bloque) *columna { return &columna{hueco: hueco, hijos: hijos} }

func (c *columna) alto(ancho float32) float32 {
	var h float32
	for i, b := range c.hijos {
		if i > 0 {
			h += c.hueco
		}
		h += b.alto(ancho)
	}
	return h
}

func (c *columna) poner(x, y, ancho float32) {
	for i, b := range c.hijos {
		if i > 0 {
			y += c.hueco
		}
		b.poner(x, y, ancho)
		y += b.alto(ancho)
	}
}

func (c *columna) objetos() []fyne.CanvasObject {
	var out []fyne.CanvasObject
	for _, b := range c.hijos {
		out = append(out, b.objetos()...)
	}
	return out
}

// parrafo es un `<p>` a todo el ancho.
type parrafo struct{ p *ui.Parrafo }

func (b parrafo) alto(ancho float32) float32   { return ui.AltoParrafo(b.p, ancho) }
func (b parrafo) poner(x, y, ancho float32)    { ponerParrafo(b.p, x, y, ancho, true) }
func (b parrafo) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.p} }

// texto es un párrafo de apoyo: letra normal en `text-muted`, que es como van todos los
// de estas pantallas.
func texto(t string, e tema.Escalon) parrafo {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	return parrafo{ui.NuevoParrafo(t, e, ui.Normal, "text-muted", v)}
}

// fijo es un objeto a su medida y pegado a la izquierda (`self-start`): un botón o un
// enlace no se estiran.
type fijo struct{ o fyne.CanvasObject }

func (b fijo) alto(float32) float32         { return b.o.MinSize().Height }
func (b fijo) poner(x, y, _ float32)        { ponerObjeto(b.o, x, y, b.o.MinSize(), true) }
func (b fijo) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.o} }

// ayuda es un `InlineHelp` a todo el ancho.
type ayuda struct{ a *ui.AyudaEnLinea }

func (b ayuda) alto(ancho float32) float32 { return ui.AltoAyudaEnLinea(b.a, ancho) }
func (b ayuda) poner(x, y, ancho float32) {
	ponerObjeto(b.a, x, y, ui.MedidaAyudaEnLinea(b.a, ancho), true)
}
func (b ayuda) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.a} }

func nuevaAyuda(tono ui.TonoAyuda, titulo, cuerpo string) ayuda {
	return ayuda{ui.NuevaAyudaEnLinea(cuerpo).ConTono(tono).ConTitulo(titulo)}
}

// tope acota el ancho de lo de dentro (`max-w-*`).
type tope struct {
	max float32
	b   bloque
}

func (t tope) alto(ancho float32) float32   { return t.b.alto(min(ancho, t.max)) }
func (t tope) poner(x, y, ancho float32)    { t.b.poner(x, y, min(ancho, t.max)) }
func (t tope) objetos() []fyne.CanvasObject { return t.b.objetos() }

// ---- Las piezas de `ui` dentro de la columna ----

// objetoB mete en la columna una pieza de `ui` cuyo alto depende del ancho (`ui.AltoA`):
// la `ui.SeccionAjustes` o el `ui.Resumen`. Aquí vivían su sección y su resumen; §2.8
// dice que la configuración guiada no se reimplementa por app, así que son los de `ui`.
type objetoB struct{ o fyne.CanvasObject }

func (b objetoB) alto(ancho float32) float32 { return ui.AltoA(b.o, ancho) }
func (b objetoB) poner(x, y, ancho float32) {
	ponerObjeto(b.o, x, y, fyne.NewSize(ancho, b.alto(ancho)), true)
}
func (b objetoB) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{b.o} }

// nuevaBSeccion es una `SettingsSection` dentro del flujo: la `ui.SeccionAjustes` con
// su título, la descripción opcional y la columna de bloques dentro.
func nuevaBSeccion(titulo, descripcion string, cuerpo bloque) objetoB {
	return objetoB{ui.NuevaSeccionAjustes(titulo, descripcion, deBloque(cuerpo))}
}

// colocado es lo de dentro de una `ui.SeccionAjustes` cuando lo coloca el Hub a mano:
// un widget cuyo alto a un ancho sale de la MISMA cuenta que lo coloca, así que la
// sección lo mide con `AltoA` y no pueden discrepar.
type colocado struct {
	widget.BaseWidget
	// colocar mide (y, con `mover`, coloca) lo de dentro a ese ancho desde (x, y) y
	// devuelve dónde acaba.
	colocar func(x, y, ancho float32, mover bool) float32
	// objetos son los de ahora: quien lo usa puede rehacerlos entre un pintado y otro.
	objetos func() []fyne.CanvasObject
}

var _ fyne.Widget = (*colocado)(nil)

func nuevoColocado(colocar func(x, y, ancho float32, mover bool) float32, objetos func() []fyne.CanvasObject) *colocado {
	c := &colocado{colocar: colocar, objetos: objetos}
	c.ExtendBaseWidget(c)
	return c
}

// deBloque es la columna de bloques de siempre hecha widget.
func deBloque(b bloque) *colocado {
	return nuevoColocado(func(x, y, ancho float32, mover bool) float32 {
		if mover {
			b.poner(x, y, ancho)
		}
		return y + b.alto(ancho)
	}, b.objetos)
}

// AltoPara es el alto a ese ancho, para `ui.AltoA`.
func (c *colocado) AltoPara(ancho float32) float32 { return c.colocar(0, 0, ancho, false) }

// CreateRenderer: lo coloca su `colocar`. Termina en Refresh.
func (c *colocado) CreateRenderer() fyne.WidgetRenderer {
	r := &colocadoRenderer{c: c}
	r.Refresh()
	return r
}

type colocadoRenderer struct{ c *colocado }

func (r *colocadoRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.c.AltoPara(r.c.Size().Width))
}
func (r *colocadoRenderer) Objects() []fyne.CanvasObject { return r.c.objetos() }
func (r *colocadoRenderer) Destroy()                     {}
func (r *colocadoRenderer) Layout(t fyne.Size) {
	if t.Width > 0 {
		r.c.colocar(0, 0, t.Width, true)
	}
}
func (r *colocadoRenderer) Refresh() { r.Layout(r.c.Size()) }

// smAjustada es el `text-sm leading-snug`: 1,375 del tamaño.
var smAjustada = tema.Escalon{Tam: tema.TextoSM.Tam, Linea: 1.375 * tema.TextoSM.Tam}

// ---- El resumen del último paso ----

// nuevoResumen es un `ui.Resumen` sin «Editar», como el del asistente de dispositivos.
func nuevoResumen(ventana float32, filas [][2]string) objetoB {
	fs := make([]ui.FilaResumen, len(filas))
	for i, f := range filas {
		fs[i] = ui.FilaResumen{Clave: f[0], Valor: f[1]}
	}
	r := ui.NuevoResumen(nil, fs...)
	r.Ventana = ventana
	return objetoB{r}
}

// ---- Lo plegado ----

// Detalles es el `<details>` de `ConnectDevices.tsx`: `border rounded-md bg-bg-subtle`,
// el resumen en `px-3 py-2 text-sm font-medium` y lo de dentro en `px-3 pt-1 pb-3`. Se
// abre y se cierra con el ratón, Intro o Espacio, como el del navegador.
//
// ponytail: el navegador pinta su triángulo de `list-item` delante del resumen; aquí va
// el chevron de la suite al mismo tamaño que el texto. Se cambia si alguien mide la
// diferencia y le importa.
type Detalles struct {
	ui.Pulsable
	Resumen string
	Abierto bool
	// AlCambiar avisa de que cambia de alto: la pantalla tiene que volver a colocarse.
	AlCambiar func()

	contenido bloque
}

var _ fyne.Widget = (*Detalles)(nil)

// Medidas del `<details>`.
const (
	MargenDetallesX   float32 = 3 * 0.25 * tema.Rem
	MargenDetallesY   float32 = 2 * 0.25 * tema.Rem
	MargenDentroArr   float32 = 1 * 0.25 * tema.Rem
	MargenDentroAbajo float32 = 3 * 0.25 * tema.Rem
	// HuecoMarcador es el espacio entre el marcador y el resumen.
	HuecoMarcador float32 = 0.25 * tema.Rem
)

func nuevosDetalles(resumen string, contenido bloque) *Detalles {
	d := &Detalles{Resumen: resumen, contenido: contenido}
	d.Nombre = resumen
	d.AlPulsar = func() {
		d.Abierto = !d.Abierto
		d.Refresh()
		if d.AlCambiar != nil {
			d.AlCambiar()
		}
	}
	d.ExtendBaseWidget(d)
	return d
}

// altoResumen es la franja que se pulsa: `py-2` y una línea de `text-sm`.
func altoResumen() float32 { return 2*MargenDetallesY + tema.TextoSM.Linea }

func (d *Detalles) alto(ancho float32) float32 {
	h := 2*tema.Borde + altoResumen()
	if d.Abierto {
		h += MargenDentroArr + d.contenido.alto(ancho-2*(tema.Borde+MargenDetallesX)) + MargenDentroAbajo
	}
	return h
}

func (d *Detalles) poner(x, y, ancho float32) {
	ponerObjeto(d, x, y, fyne.NewSize(ancho, d.alto(ancho)), true)
	if d.Abierto {
		d.contenido.poner(x+tema.Borde+MargenDetallesX, y+tema.Borde+altoResumen()+MargenDentroArr,
			ancho-2*(tema.Borde+MargenDetallesX))
	}
}

// objetos son la caja con su resumen y, si está abierto, lo de dentro, que va aparte
// para que sus botones se puedan pulsar.
func (d *Detalles) objetos() []fyne.CanvasObject {
	out := []fyne.CanvasObject{d}
	if d.Abierto {
		out = append(out, d.contenido.objetos()...)
	}
	return out
}

// CreateRenderer compone la caja, el marcador y el resumen.
func (d *Detalles) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &detallesRenderer{
		d:       d,
		aro:     ui.NuevoAro(tema.RadioMD, ui.ConHueco, v),
		caja:    ui.NuevaCaja(tema.Color("bg-subtle", v), tema.Color("border", v), tema.RadioMD),
		cerrado: iconos.Trazo(iconos.ChevronDerecha, tema.TextoSM.Tam, "text", v),
		abierto: iconos.Trazo(iconos.ChevronAbajo, tema.TextoSM.Tam, "text", v),
		texto:   ui.Texto(d.Resumen, tema.TextoSM, ui.Medio, tema.Color("text", v)),
	}
	r.objetos = append(r.aro.Objetos(), r.caja.Objetos()...)
	r.objetos = append(r.objetos, r.cerrado.Objeto(), r.abierto.Objeto(), r.texto)
	r.Refresh()
	return r
}

type detallesRenderer struct {
	d                *Detalles
	aro              *ui.Aro
	caja             *ui.Caja
	cerrado, abierto *iconos.Dibujo
	texto            *canvas.Text
	objetos          []fyne.CanvasObject
}

func (r *detallesRenderer) MinSize() fyne.Size           { return fyne.NewSize(0, altoResumen()+2*tema.Borde) }
func (r *detallesRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *detallesRenderer) Destroy()                     {}

func (r *detallesRenderer) Layout(t fyne.Size) {
	r.caja.Redimensionar(t)
	// El anillo va alrededor del resumen, que es lo que recibe el foco en la web.
	r.aro.Colocar(fyne.NewSize(t.Width, altoResumen()+2*tema.Borde))
	x, y := tema.Borde+MargenDetallesX, tema.Borde+MargenDetallesY
	lado := tema.TextoSM.Tam
	r.cerrado.Colocar(x, y+(tema.TextoSM.Linea-lado)/2, lado)
	r.abierto.Colocar(x, y+(tema.TextoSM.Linea-lado)/2, lado)
	ui.ColocarTexto(r.texto, tema.TextoSM, x+lado+HuecoMarcador, y)
}

func (r *detallesRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.cerrado.Ver(!r.d.Abierto)
	r.abierto.Ver(r.d.Abierto)
	r.texto.Text = r.d.Resumen
	r.texto.Refresh()
	r.aro.Pintar(v)
	r.aro.Ver(r.d.Enfocado())
	r.Layout(r.d.Size())
}

// ---- La fila con botón de copiar ----

// filaCopia mete la `ui.FilaCopia` (el `CopyField` de la web) en una columna de bloques.
// La fila vive en `ui` porque es de la suite; el envoltorio es del Hub porque `bloque`
// lo es.
type filaCopia struct{ *ui.FilaCopia }

func nuevaFilaCopia(rotulo, valor string, copiar func(string)) filaCopia {
	return filaCopia{ui.NuevaFilaCopia(rotulo, valor, copiar)}
}

func (f filaCopia) alto(ancho float32) float32 { return f.AltoPara(ancho) }

func (f filaCopia) poner(x, y, ancho float32) {
	ponerObjeto(f.FilaCopia, x, y, fyne.NewSize(ancho, f.AltoPara(ancho)), true)
}

func (f filaCopia) objetos() []fyne.CanvasObject { return []fyne.CanvasObject{f.FilaCopia} }
