// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/CopyField.tsx

import (
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// FilaCopia es el `CopyField` de @sytue/ui: el rótulo (`sytue-meta`, con su ancho desde
// `sm:` y la línea entera por debajo), el valor en monoespaciada (`font-mono text-xs
// break-all`) y «Copiar» (`Button ghost sm`, inerte sin valor), en una caja `border
// rounded-md bg-surface px-3 py-2` con `gap-2`.
//
// Vivía en el Hub (`apps/hub/bloques.go`) mientras solo la usaban sus pantallas; la web
// la ha sacado a @sytue/ui porque había seis versiones que ya divergían, y su espejo
// viene aquí por la misma regla (CONVENTIONS §13.2).
//
// Como en la web, el aviso es del propio botón: pasa a «Copiado» un momento, con el
// ancho de la palabra más larga reservado para que la fila no salte.
type FilaCopia struct {
	widget.BaseWidget
	Rotulo, Valor string
	// Ventana decide el `sm:`; cero es ancha.
	Ventana float32
	// RotuloSM es el ancho del rótulo desde `sm:`: `RotuloCopiaSM` (`labelWidth="lg"`,
	// el de por defecto, también con cero) o `RotuloCopiaCorto` (`labelWidth="sm"`).
	RotuloSM float32
	// Aviso es cuánto dura «Copiado» en el botón; cero, los 2 s de la web.
	Aviso time.Duration

	boton  *Boton
	copiar func(string)

	mu      sync.Mutex
	copiado bool
	vuelta  int // cada pulsación invalida la vuelta a «Copiar» de la anterior
}

var _ fyne.Widget = (*FilaCopia)(nil)

// Medidas del `CopyField`.
const (
	MargenCopiaX  float32 = 3 * 0.25 * tema.Rem
	MargenCopiaY  float32 = 2 * 0.25 * tema.Rem
	HuecoCopia    float32 = 2 * 0.25 * tema.Rem
	RotuloCopiaSM float32 = 52 * 0.25 * tema.Rem // `sm:w-52`, `labelWidth="lg"`
	// RotuloCopiaCorto es `sm:w-24` (`labelWidth="sm"`): rótulos de una palabra, como
	// los tres campos de un registro DNS.
	RotuloCopiaCorto float32 = 24 * 0.25 * tema.Rem
)

// NuevaFilaCopia crea la fila. `copiar` es quien escribe en el portapapeles: lo sabe la
// app, que tiene la ventana.
func NuevaFilaCopia(rotulo, valor string, copiar func(string)) *FilaCopia {
	f := &FilaCopia{Rotulo: rotulo, Valor: valor, copiar: copiar}
	f.boton = NuevoBoton(textoComun("ui.copy.copy"), f.pulsar).Con(Fantasma, TallaSM)
	f.boton.Inerte = valor == ""
	f.ExtendBaseWidget(f)
	return f
}

// Boton es el botón de copiar, para quien lo tenga que pulsar en una prueba.
func (f *FilaCopia) Boton() *Boton { return f.boton }

// Copiado dice si el botón está diciendo «Copiado».
func (f *FilaCopia) Copiado() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.copiado
}

func (f *FilaCopia) pulsar() {
	if f.Valor == "" {
		return
	}
	if f.copiar != nil {
		f.copiar(f.Valor)
	}
	aviso := f.Aviso
	if aviso <= 0 {
		aviso = 2 * time.Second
	}
	f.mu.Lock()
	f.copiado = true
	f.vuelta++
	esta := f.vuelta
	f.mu.Unlock()
	f.Refresh()
	time.AfterFunc(aviso, func() {
		fyne.Do(func() {
			f.mu.Lock()
			if f.vuelta != esta {
				f.mu.Unlock()
				return
			}
			f.copiado = false
			f.mu.Unlock()
			f.Refresh()
		})
	})
}

// anchoBoton es el del rótulo más largo de los dos: las dos palabras ocupan la misma
// celda en la web y el botón no cambia de ancho al copiar.
func anchoBoton() float32 {
	return max(MedidaBoton(textoComun("ui.copy.copy"), TallaSM, 0).Width,
		MedidaBoton(textoComun("ui.copy.copied"), TallaSM, 0).Width)
}

func (f *FilaCopia) medidaBoton() fyne.Size {
	f.boton.Ancho = anchoBoton()
	return f.boton.MinSize()
}

func (f *FilaCopia) ancha() bool { return f.Ventana == 0 || f.Ventana >= CorteSM }

// valorVisible es lo que se escribe: la web pone una raya si no hay valor.
func (f *FilaCopia) valorVisible() string {
	if f.Valor == "" {
		return "—"
	}
	return f.Valor
}

// geometria reparte la fila de ese ancho: el ancho del rótulo, el del valor y dónde
// empieza la línea del valor (0 si va en la misma que el rótulo).
func (f *FilaCopia) geometria(ancho float32) (rotulo, valor, bajada float32) {
	interior := max(ancho-2*(tema.Borde+MargenCopiaX), 0)
	boton := f.medidaBoton().Width
	if f.ancha() {
		rotulo = RotuloCopiaSM
		if f.RotuloSM > 0 {
			rotulo = f.RotuloSM
		}
		rotulo = min(rotulo, interior)
		return rotulo, max(interior-rotulo-boton-2*HuecoCopia, 0), 0
	}
	// Por debajo de `sm:` el rótulo es `w-full`: ocupa su línea y el resto baja.
	rotulo = interior
	return rotulo, max(interior-boton-HuecoCopia, 0), float32(len(Repartir(f.Rotulo, tema.TextoXS, Normal, rotulo)))*tema.TextoXS.Linea + HuecoCopia
}

// lineasValor parte el valor en cualquier carácter (`break-all`).
func (f *FilaCopia) lineasValor(ancho float32) []string {
	return partirMono(f.valorVisible(), tema.TextoXS.Tam, ancho)
}

// AltoPara es el alto de la fila a ese ancho: el rótulo y el valor se reparten, así que
// depende de él (y `AltoA` lo usa).
func (f *FilaCopia) AltoPara(ancho float32) float32 {
	rotulo, valor, bajada := f.geometria(ancho)
	altoRotulo := float32(len(Repartir(f.Rotulo, tema.TextoXS, Normal, rotulo))) * tema.TextoXS.Linea
	fila := max(float32(len(f.lineasValor(valor)))*tema.TextoXS.Linea, f.medidaBoton().Height)
	if bajada == 0 {
		fila = max(fila, altoRotulo)
	}
	return 2*(tema.Borde+MargenCopiaY) + bajada + fila
}

// partirMono parte un texto monoespaciado donde se acabe el ancho, carácter a
// carácter, que es lo que hace `break-all` con una URL.
func partirMono(s string, tam, ancho float32) []string {
	var lineas []string
	var actual []rune
	for _, c := range s {
		prueba := string(append(actual, c))
		if len(actual) > 0 && fyne.MeasureText(prueba, tam, fyne.TextStyle{Monospace: true}).Width > ancho {
			lineas = append(lineas, string(actual))
			actual = actual[:0]
		}
		actual = append(actual, c)
	}
	return append(lineas, string(actual))
}

// CreateRenderer compone la caja, el rótulo, el valor y el botón.
func (f *FilaCopia) CreateRenderer() fyne.WidgetRenderer {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r := &filaCopiaRenderer{
		f:      f,
		caja:   NuevaCaja(tema.Color("surface", v), tema.Color("border", v), tema.RadioMD),
		rotulo: NuevoParrafo(f.Rotulo, tema.TextoXS, Normal, "text-subtle", v),
	}
	r.Refresh()
	return r
}

type filaCopiaRenderer struct {
	f       *FilaCopia
	caja    *Caja
	rotulo  *Parrafo
	valor   []*canvas.Text
	objetos []fyne.CanvasObject
}

func (r *filaCopiaRenderer) MinSize() fyne.Size {
	return fyne.NewSize(0, r.f.AltoPara(r.f.Size().Width))
}
func (r *filaCopiaRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filaCopiaRenderer) Destroy()                     {}

func colocarFilaCopia(o fyne.CanvasObject, x, y float32, t fyne.Size) {
	o.Move(fyne.NewPos(x, y))
	o.Resize(t)
}

func (r *filaCopiaRenderer) Layout(t fyne.Size) {
	f := r.f
	r.caja.Redimensionar(t)
	anchoRotulo, anchoValor, bajada := f.geometria(t.Width)
	lineas := f.lineasValor(anchoValor)
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	if len(lineas) != len(r.valor) {
		r.valor = nil
		for range lineas {
			// La cara de Figtree que pone `Texto` gana al estilo: sin quitarla, el valor
			// sale en Figtree aunque diga monoespaciada.
			c := Texto("", tema.TextoXS, Normal, tema.Color("text", v))
			c.FontSource, c.TextStyle.Monospace = nil, true
			r.valor = append(r.valor, c)
		}
		r.objetos = append(r.caja.Objetos(), r.rotulo, f.boton)
		for _, c := range r.valor {
			r.objetos = append(r.objetos, c)
		}
	}
	x := tema.Borde + MargenCopiaX
	arriba := tema.Borde + MargenCopiaY
	altoRotulo := AltoParrafo(r.rotulo, anchoRotulo)
	altoValor := float32(len(lineas)) * tema.TextoXS.Linea
	boton := f.medidaBoton()
	fila := max(altoValor, boton.Height)
	yRotulo := arriba
	if bajada == 0 {
		fila = max(fila, altoRotulo)
		yRotulo += (fila - altoRotulo) / 2
	}
	colocarFilaCopia(r.rotulo, x, yRotulo, fyne.NewSize(anchoRotulo, altoRotulo))
	vy := arriba + bajada + (fila-altoValor)/2
	vx := x
	if bajada == 0 {
		vx += anchoRotulo + HuecoCopia
	}
	for i, c := range r.valor {
		c.Text = lineas[i]
		c.TextSize = tema.TextoXS.Tam
		c.Refresh()
		ColocarTexto(c, tema.TextoXS, vx, vy+float32(i)*tema.TextoXS.Linea)
	}
	colocarFilaCopia(f.boton, vx+anchoValor+HuecoCopia, arriba+bajada+(fila-boton.Height)/2, boton)
}

func (r *filaCopiaRenderer) Refresh() {
	f := r.f
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	r.caja.Pintar(tema.Color("surface", v), tema.Color("border", v))
	r.rotulo.Texto = f.Rotulo
	r.rotulo.Refresh()
	texto := textoComun("ui.copy.copy")
	if f.Copiado() {
		texto = textoComun("ui.copy.copied")
	}
	// Inerte antes de repintar: un valor que llega después (la cuenta aún cargando)
	// tiene que despertar el botón en este mismo Refresh, no en el siguiente.
	inerte := f.Valor == ""
	if f.boton.Texto != texto || f.boton.Inerte != inerte {
		f.boton.Texto, f.boton.Inerte = texto, inerte
		f.boton.Refresh()
	}
	r.valor = nil // fuerza el reparto: puede haber cambiado el valor
	r.Layout(f.Size())
}
