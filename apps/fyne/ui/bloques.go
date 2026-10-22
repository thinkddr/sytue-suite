// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// Las piezas de maquetación de una PÁGINA de la web: la pila de bloques con su margen
// debajo (`mb-4`, `mb-6`…), el alto de un objeto A UN ANCHO y la fila con `ml-auto` y
// `flex-wrap`.
//
// Nacieron en `apps/photos` y Sytue Cast las importaba de allí (2026-09-28): una pieza
// que dos productos comparten no es de ninguno, así que vive aquí (CONVENTIONS §13.2).
// La pila de bloques se llama `PilaBloques` y no `Pila` porque `Pila` ya es el
// `flex-col` con un hijo elástico de este paquete; el nombre sigue a `PilaAvisos`:
// una pila, y de qué.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// ---- La pila de bloques de una página ----

// Bloque es un hijo de una página: el objeto y el margen que la web le pone DEBAJO
// (`mb-4`, `mb-6`, `mb-8`). No es un hueco constante: la web no usa uno.
type Bloque struct {
	Obj    fyne.CanvasObject
	Margen float32
	// Alto es el alto del bloque a un ancho dado, para lo que depende del ancho —una
	// rejilla, sobre todo—. nil: manda su `MinSize`.
	//
	// Hace falta porque `fyne.Layout.MinSize` NO recibe el ancho: una rejilla
	// preguntada sin él contesta con el de UNA columna, y la página saldría con el
	// alto de una tesela por muchas filas que tenga.
	Alto func(ancho float32) float32
}

// pila coloca los bloques en columna, cada uno a todo el ancho.
type pila struct{ b []Bloque }

var _ fyne.Layout = pila{}

func (p pila) alto(i int, o fyne.CanvasObject, ancho float32) float32 {
	if p.b[i].Alto != nil {
		return p.b[i].Alto(ancho)
	}
	return AltoA(o, ancho)
}

// conAlto lo cumplen los layouts cuyo alto depende del ancho: la pila, el recuadro, la
// fila con icono. Es lo que `fyne.Layout.MinSize` no puede decir, porque no recibe el
// ancho.
type conAlto interface {
	AltoCon(objs []fyne.CanvasObject, ancho float32) float32
}

// AltoA es el alto de un objeto a un ancho dado: el de un párrafo que se reparte, el de
// una rejilla, el de un estado vacío o el de cualquier layout que sepa contarlo. Para lo
// demás, su mínimo.
func AltoA(o fyne.CanvasObject, ancho float32) float32 {
	switch x := o.(type) {
	case *Parrafo:
		return AltoParrafo(x, ancho)
	case *Vacio:
		return x.MedidaVacio(ancho)
	case interface{ AltoPara(float32) float32 }:
		return x.AltoPara(ancho)
	case *CabeceraApp:
		x.Ancho = ancho // con `flex-wrap`, el alto depende del ancho
		return MedidaCabeceraApp(x).Height
	case *fyne.Container:
		switch l := x.Layout.(type) {
		case conAlto:
			return l.AltoCon(x.Objects, ancho)
		case Rejilla:
			return l.MinSizeCon(x.Objects, ancho).Height
		}
	}
	return o.MinSize().Height
}

// AlAncho envuelve un objeto para que su mínimo sea su alto A ESE ANCHO: es lo que
// necesita un desplazamiento vertical, que pregunta el mínimo y no el ancho.
func AlAncho(ancho float32, o fyne.CanvasObject) *fyne.Container {
	return container.New(alAncho(ancho), o)
}

// AlAnchoFluido calcula la altura al ancho disponible sin convertir ese ancho en
// un mínimo de ventana. Permite que un desplazador vuelva a encoger al redimensionar.
func AlAnchoFluido(ancho float32, o fyne.CanvasObject) *fyne.Container {
	return container.New(anchoFluido{alAncho(ancho)}, o)
}

type anchoFluido struct{ alAncho }

func (a anchoFluido) MinSize(objs []fyne.CanvasObject) fyne.Size {
	m := a.alAncho.MinSize(objs)
	m.Width = 0
	return m
}

type alAncho float32

func (a alAncho) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(float32(a), AltoA(objs[0], float32(a)))
}

func (a alAncho) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(t)
}

// AltoCon es el alto de la pila a un ancho dado, que es la cuenta de verdad.
func (p pila) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	alto := float32(0)
	for i, o := range objs {
		alto += p.alto(i, o, ancho) + p.b[i].Margen
	}
	return alto
}

func (p pila) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho := float32(0)
	for _, o := range objs {
		ancho = max(ancho, o.MinSize().Width)
	}
	return fyne.NewSize(ancho, p.AltoCon(objs, ancho))
}

var _ conAlto = pila{}

func (p pila) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	y := float32(0)
	for i, o := range objs {
		h := p.alto(i, o, t.Width)
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(t.Width, h))
		y += h + p.b[i].Margen
	}
}

// PilaBloques arma un contenedor con esos bloques. Los nil se saltan: son las ramas que en la
// web devuelven `null`.
func PilaBloques(bloques ...Bloque) *fyne.Container {
	objs := make([]fyne.CanvasObject, 0, len(bloques))
	usados := make([]Bloque, 0, len(bloques))
	for _, b := range bloques {
		if b.Obj == nil {
			continue
		}
		objs = append(objs, b.Obj)
		usados = append(usados, b)
	}
	return container.New(pila{usados}, objs...)
}

// ---- Fila con `ml-auto` ----

// FilaAuto es la fila de cabecera de estas páginas: los primeros hijos a la izquierda y
// el resto empujados a la derecha, que es el `ml-auto` de la web.
//
// Con Partir es además `flex-wrap`: lo que no cabe baja a la fila siguiente, a SaltoFila
// de la anterior (el `gap-2` y, si los hijos lo llevan, su `mb-4`). El `ml-auto` empuja
// a la derecha los de SU fila. Un hijo más ancho que la fila se queda solo en ella y
// encoge hasta el ancho (el `min-w-0` de la nav). Y Elastico es el hijo que crece hasta
// llenar su fila (`flex-1` con base 0: el buscador en un teléfono), contado desde 1.
type FilaAuto struct {
	Hueco float32
	// Derecha es el índice del primer hijo empujado a la derecha; con len(objs) no
	// hay ninguno.
	Derecha int
	// AlturaFila fija el alto de la fila; con 0, el del hijo más alto.
	AlturaFila float32

	Partir    bool
	SaltoFila float32
	Elastico  int
	// AlFinal alinea todas las filas al final, también después de partir: las
	// acciones de un diálogo conservan su alineación al bajar a otra fila.
	AlFinal bool
}

var _ fyne.Layout = FilaAuto{}

// MinSize suma anchos y huecos, y toma el alto del más alto. Partida, el ancho es el del
// hijo más ancho —puede encoger— y el alto, el de una fila: el de verdad depende del
// ancho y lo da AltoCon.
func (f FilaAuto) MinSize(objs []fyne.CanvasObject) fyne.Size {
	ancho, alto := float32(0), f.AlturaFila
	for i, o := range objs {
		m := o.MinSize()
		switch {
		case f.Partir:
			ancho = max(ancho, m.Width)
		case i > 0:
			ancho += f.Hueco + m.Width
		default:
			ancho += m.Width
		}
		alto = max(alto, m.Height)
	}
	if f.Partir {
		ancho = 0
	}
	return fyne.NewSize(ancho, alto)
}

// base es el ancho con el que un hijo entra en el reparto: el suyo, o 0 si es elástico.
func (f FilaAuto) base(i int, o fyne.CanvasObject) float32 {
	if f.Partir && i+1 == f.Elastico {
		return 0
	}
	return o.MinSize().Width
}

// filas reparte los hijos en filas a ese ancho: el índice del primero de cada una.
func (f FilaAuto) filas(objs []fyne.CanvasObject, ancho float32) []int {
	cortes := []int{0}
	x := float32(0)
	for i, o := range objs {
		w := f.base(i, o)
		if i > cortes[len(cortes)-1] && x+f.Hueco+w > ancho {
			cortes, x = append(cortes, i), w
			continue
		}
		if i > cortes[len(cortes)-1] {
			x += f.Hueco
		}
		x += w
	}
	return cortes
}

// anchosFila es lo que se lleva cada hijo de la fila [desde, hasta) a ese ancho: el
// suyo, el elástico lo que sobre y el que va solo, como mucho el ancho.
func (f FilaAuto) anchosFila(objs []fyne.CanvasObject, desde, hasta int, ancho float32) []float32 {
	anchos := make([]float32, hasta-desde)
	total := f.Hueco * float32(max(hasta-desde-1, 0))
	for i := desde; i < hasta; i++ {
		anchos[i-desde] = f.base(i, objs[i])
		total += anchos[i-desde]
	}
	if e := f.Elastico - 1; f.Partir && e >= desde && e < hasta {
		anchos[e-desde] += max(ancho-total, 0)
	}
	if f.Partir && hasta-desde == 1 {
		anchos[0] = min(anchos[0], ancho) // `min-w-0`: el que no cabe solo, encoge
	}
	return anchos
}

// altoFila es el del hijo más alto de la fila [desde, hasta), cada uno a su ancho: un
// párrafo elástico que se reparte en varias líneas hace la fila más alta.
func (f FilaAuto) altoFila(objs []fyne.CanvasObject, desde, hasta int, ancho float32) float32 {
	alto := f.AlturaFila
	for i, w := range f.anchosFila(objs, desde, hasta, ancho) {
		alto = max(alto, f.altoHijo(objs[desde+i], w))
	}
	return alto
}

// altoHijo es el alto de un hijo a su ancho. Solo partida: sin partir, cada hijo mide
// lo que dice su mínimo, que es lo que hacía siempre esta fila.
func (f FilaAuto) altoHijo(o fyne.CanvasObject, ancho float32) float32 {
	if f.Partir {
		return AltoA(o, ancho)
	}
	return o.MinSize().Height
}

// AltoCon es el alto a ese ancho: el de sus filas más los saltos entre ellas.
func (f FilaAuto) AltoCon(objs []fyne.CanvasObject, ancho float32) float32 {
	if !f.Partir || len(objs) == 0 {
		return f.MinSize(objs).Height
	}
	cortes := f.filas(objs, ancho)
	alto := float32(0)
	for i, c := range cortes {
		fin := len(objs)
		if i+1 < len(cortes) {
			fin = cortes[i+1]
		}
		if i > 0 {
			alto += f.SaltoFila
		}
		alto += f.altoFila(objs, c, fin, ancho)
	}
	return alto
}

// Layout coloca los de la izquierda desde el margen y los de la derecha pegados al
// final, en su orden; partida, fila a fila.
func (f FilaAuto) Layout(objs []fyne.CanvasObject, t fyne.Size) {
	if !f.Partir {
		f.colocarFila(objs, 0, len(objs), 0, t.Width, t.Height)
		return
	}
	cortes := f.filas(objs, t.Width)
	y := float32(0)
	for i, c := range cortes {
		fin := len(objs)
		if i+1 < len(cortes) {
			fin = cortes[i+1]
		}
		h := f.altoFila(objs, c, fin, t.Width)
		f.colocarFila(objs, c, fin, y, t.Width, h)
		y += h + f.SaltoFila
	}
}

// colocarFila coloca los hijos [desde, hasta) en una fila de ese ancho y alto a la
// altura y, centrados en vertical.
func (f FilaAuto) colocarFila(objs []fyne.CanvasObject, desde, hasta int, y, ancho, alto float32) {
	anchos := f.anchosFila(objs, desde, hasta, ancho)
	colocar := func(i int, x float32) float32 {
		o, w := objs[i], anchos[i-desde]
		h := f.altoHijo(o, w)
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos(x, y+(alto-h)/2))
		return x + w + f.Hueco
	}
	x := float32(0)
	// El `ml-auto` empuja solo en SU fila: las de después vuelven a empezar a la izquierda.
	partir := hasta
	if f.AlFinal {
		partir = desde
	} else if f.Derecha >= desde && f.Derecha < hasta {
		partir = f.Derecha
	}
	for i := desde; i < partir; i++ {
		x = colocar(i, x)
	}
	if partir >= hasta {
		return
	}
	derecha := f.Hueco * float32(hasta-partir-1)
	for i := partir; i < hasta; i++ {
		derecha += anchos[i-desde]
	}
	x = max(ancho-derecha, x)
	for i := partir; i < hasta; i++ {
		x = colocar(i, x)
	}
}
