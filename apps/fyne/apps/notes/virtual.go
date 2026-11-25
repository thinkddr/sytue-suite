// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// # Solo se montan las tarjetas que se ven
//
// Un cuaderno en columnas o en lista tenía TODAS sus tarjetas en el árbol: repintar uno
// de 1.000 notas eran ~30 ms aunque se reutilizaran, porque se colocaban, se medían y se
// recorrían las mil, y la memoria crecía con el cuaderno (medido el 2026-11-24,
// docs/roadmap.md). `columnaVirtual` es `columnaTarjetas` con las tarjetas REPARTIDAS
// todas —cada una con su alto, que la tarjeta recuerda por ancho aunque no esté montada
// (MedidaTarjetaNota)— y COLGADAS solo las que caen en el desplazador, más una vista por
// arriba y otra por abajo para que el tabulador y la rueda tengan adónde ir. Al desplazar
// entran las que llegan (Lienzo.alDesplazar) y las que salen se sueltan: su renderer lo
// tira Fyne cuando caduca.
//
// Las que se escriben, se renombran, se arrastran o tienen el foco van SIEMPRE colgadas:
// soltarlas les quitaría el foco y lo escrito (`Tarjeta.fija`).
//
// El mínimo de la columna es el de todas, así que el desplazador mide lo mismo que antes
// y la barra no cambia; y el ancho al que se mide, el de la última vez que se colocó
// (`Lienzo.anchos`), como lo medían las tarjetas con su tamaño de entonces.

// claveColumna es qué columna del lienzo es: la de cada columna de la vista en columnas,
// la de la lista o la del árbol que no llena el panel. Con ella se recuerda su ancho.
type claveColumna int

const (
	columnaLista claveColumna = -1 - iota
	columnaArbol
)

// columnaVirtual es el layout de la columna (ver arriba). c es su contenedor, cuyos
// objetos son las tarjetas colgadas.
type columnaVirtual struct {
	hueco float32
	// destino es la columna de la vista en columnas: vacía sigue siendo un destino
	// visible y alcanzable para el asa (16 de alto como mínimo).
	destino bool
	l       *Lienzo
	clave   claveColumna
	todas   []*Tarjeta
	c       *fyne.Container
	// ancho es el del último Layout (o el recordado, antes de él); ys y altos, dónde
	// cae y cuánto mide cada tarjeta a ese ancho.
	ancho     float32
	ys, altos []float32
}

var _ fyne.Layout = (*columnaVirtual)(nil)

// nuevaColumnaVirtual arma la columna con esas tarjetas, en ese orden.
func (l *Lienzo) nuevaColumnaVirtual(clave claveColumna, hueco float32, destino bool, tarjetas []*Tarjeta) *fyne.Container {
	cv := &columnaVirtual{hueco: hueco, destino: destino, l: l, clave: clave, todas: tarjetas, ancho: l.anchos[clave]}
	cv.c = container.NewWithoutLayout()
	cv.c.Layout = cv
	l.virtuales = append(l.virtuales, cv)
	if cv.ancho > 0 {
		// Lo que se colgaría al ancho de la última vez: el primer pintado de la columna
		// nueva no espera a su Layout para tener tarjetas.
		cv.medir(cv.ancho)
		cv.montar()
	} else {
		// Sin ancho todavía no se sabe qué se ve: todas, como antes, hasta el Layout.
		cv.c.Objects = objetosDe(tarjetas)
	}
	return cv.c
}

// anchoDe es el ancho al que va una tarjeta en una columna de ese ancho: las filas de
// una sublista entran `1.75rem` por nivel (Tarjeta.sangriaNivel).
func anchoDe(t *Tarjeta, ancho float32) (x, w float32) {
	x = min(t.sangriaNivel(), ancho/2)
	return x, ancho - x
}

// MinSize es el de la columna entera, como `ui.Columna`: cada tarjeta a su ancho.
func (cv *columnaVirtual) MinSize([]fyne.CanvasObject) fyne.Size {
	var ancho, alto float32
	for i, t := range cv.todas {
		w := t.Size().Width
		if cv.ancho > 0 {
			_, w = anchoDe(t, cv.ancho)
		}
		m := MedidaTarjetaNota(t, w)
		if i > 0 {
			alto += cv.hueco
		}
		alto += m.Height
		ancho = max(ancho, m.Width)
	}
	if cv.destino {
		alto = max(alto, tema.Espacio(16))
	}
	return fyne.NewSize(ancho, alto)
}

// Layout reparte todas las tarjetas a ese ancho y coloca las que se ven.
func (cv *columnaVirtual) Layout(_ []fyne.CanvasObject, t fyne.Size) {
	cv.ancho = t.Width
	if cv.l.anchos == nil {
		cv.l.anchos = map[claveColumna]float32{}
	}
	cv.l.anchos[cv.clave] = t.Width
	cv.medir(t.Width)
	cv.montar()
	cv.colocar()
}

// medir apunta dónde cae y cuánto mide cada tarjeta en una columna de ese ancho: el alto
// que pide A SU ANCHO (ver columnaTarjetas).
func (cv *columnaVirtual) medir(ancho float32) {
	cv.ys, cv.altos = cv.ys[:0], cv.altos[:0]
	var y float32
	for _, t := range cv.todas {
		_, w := anchoDe(t, ancho)
		alto := altoDe(t, w)
		cv.ys, cv.altos = append(cv.ys, y), append(cv.altos, alto)
		y += alto + cv.hueco
	}
}

// montar cuelga las tarjetas que caen en la vista (y las fijas) y suelta las demás.
// Devuelve si ha cambiado algo.
func (cv *columnaVirtual) montar() bool {
	if len(cv.ys) != len(cv.todas) {
		return false
	}
	desde, hasta, todo := cv.vista()
	objs := make([]fyne.CanvasObject, 0, len(cv.c.Objects)+1)
	for i, t := range cv.todas {
		dentro := todo || (cv.ys[i]+cv.altos[i] >= desde && cv.ys[i] <= hasta) || t.fija()
		t.fuera = !dentro
		if dentro {
			objs = append(objs, t)
		}
	}
	if slices.Equal(objs, cv.c.Objects) {
		return false
	}
	cv.c.Objects = objs
	return true
}

// colocar pone cada tarjeta colgada en su sitio.
func (cv *columnaVirtual) colocar() {
	if len(cv.ys) != len(cv.todas) {
		return
	}
	for i, t := range cv.todas {
		if t.fuera {
			continue
		}
		x, w := anchoDe(t, cv.ancho)
		t.Move(fyne.NewPos(x, cv.ys[i]))
		t.Resize(fyne.NewSize(w, cv.altos[i]))
	}
}

// alDesplazar cuelga lo que entra en la vista y suelta lo que sale.
func (cv *columnaVirtual) alDesplazar() {
	if cv.montar() {
		cv.colocar()
		cv.c.Refresh()
	}
}

// vista es lo que se ve de la columna, en sus coordenadas y con una vista de más por
// cada lado; todo, si la columna no está en el desplazador del lienzo (entonces no hay
// manera de saber qué se ve, y se cuelga entera, como antes).
func (cv *columnaVirtual) vista() (desde, hasta float32, todo bool) {
	s := cv.l.desplazador
	if s == nil || s.Content == nil {
		return 0, 0, true
	}
	arriba, ok := dentroDe(s.Content, cv.c)
	if !ok {
		return 0, 0, true
	}
	alto := s.Size().Height
	if alto <= 0 {
		alto = ventana([]fyne.CanvasObject{cv.c}).Height
	}
	y := s.Offset.Y - arriba.Y
	return y - alto, y + 2*alto, false
}

// dentroDe es dónde está o dentro de raíz, bajando solo por contenedores, que es todo lo
// que hay entre el contenido del desplazador y una columna del lienzo.
func dentroDe(raiz, o fyne.CanvasObject) (fyne.Position, bool) {
	if raiz == o {
		return fyne.Position{}, true
	}
	c, ok := raiz.(*fyne.Container)
	if !ok {
		return fyne.Position{}, false
	}
	for _, h := range c.Objects {
		if p, ok := dentroDe(h, o); ok {
			return h.Position().Add(p), true
		}
	}
	return fyne.Position{}, false
}

// alDesplazar es lo que hace el lienzo al moverse su desplazador: cada columna cuelga lo
// que entra en la vista.
func (l *Lienzo) alDesplazar() {
	for _, cv := range l.virtuales {
		cv.alDesplazar()
	}
}

// fija dice si la tarjeta va colgada aunque no se vea: soltarla le quitaría el foco, lo
// escrito o el arrastre.
func (t *Tarjeta) fija() bool {
	return t.Editor != nil || t.Renombrar != nil || t.arrastrando || t.enfocado ||
		(t.mas != nil && t.mas.enfoque) || (t.render != nil && t.render.asa != nil && t.render.asa.enfocada)
}
