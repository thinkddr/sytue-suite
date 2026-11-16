// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// paridad-web: apps/frontend/packages/ui/src/components/Skeleton.tsx

import (
	"image/color"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"git.sytue.eu/sytue/one/apps/fyne/tema"
)

// DelanteroEsqueleto es el `lead` de SkeletonRows: lo que va delante del texto.
type DelanteroEsqueleto int

const (
	// SinDelantero es `lead="none"`.
	SinDelantero DelanteroEsqueleto = iota
	// DelanteroAvatar es `lead="avatar"`: el círculo de `Avatar size="sm"`.
	DelanteroAvatar
	// DelanteroTesela es `lead="tile"`: la tesela de tipo de Drive.
	DelanteroTesela
)

// LadoDelanteroEsqueleto es el `LADO_DELANTERO` de la web: 32 el avatar y 36 la tesela.
func LadoDelanteroEsqueleto(d DelanteroEsqueleto) float32 {
	switch d {
	case DelanteroAvatar:
		return MedidaAvatar(AvatarSM).Width
	case DelanteroTesela:
		return TeselaDrive
	case SinDelantero:
	}
	return 0
}

// Las rayas de cada fila, en rem de la suite: el título de `0.85rem`, el subtítulo de
// `0.7rem` con el `gap-1.5` entre los dos, y la meta de `0.7rem` por defecto separada por
// `gap-8`.
const (
	AltoTituloEsqueleto float32 = 0.85 * tema.Rem
	AltoSubEsqueleto    float32 = 0.7 * tema.Rem
	huecoLineasEsq      float32 = 1.5 * 0.25 * tema.Rem
	huecoMetaEsq        float32 = 8 * 0.25 * tema.Rem
)

// AltoFilaEsqueleto es `altoFilaEsqueleto` de la web: `py-2` arriba y abajo más lo más
// alto de sus hijos —la caja de línea de `text-base`, más la de `text-sm` con dos líneas,
// o la pieza delantera si es más alta—. El filete, si lo hay, va DENTRO de ese alto, como
// el `border-b` de un `<li>` con `box-sizing: border-box`.
func AltoFilaEsqueleto(d DelanteroEsqueleto, lineas int) float32 {
	texto := tema.TextoBase.Linea
	if lineas >= 2 {
		texto += tema.TextoSM.Linea
	}
	return 2*AltoFila + max(texto, LadoDelanteroEsqueleto(d))
}

// MetaEsqueleto es una columna de la derecha (`SkeletonRowsMeta`). Oculta es la columna
// que la web esconde por debajo de un corte (`hidden sm:block`): quien la monta sabe el
// ancho de su ventana.
type MetaEsqueleto struct {
	Ancho, Alto float32
	Oculta      bool
}

// RayaEsqueleto es una línea de texto de una fila con receta propia (RecetaEsqueleto).
type RayaEsqueleto struct {
	// Alto es el `height` de la raya.
	Alto float32
	// Ancho es un `width` fijo (`width="7rem"`), acotado a lo que deje la fila; gana a
	// Partes.
	Ancho float32
	// Partes son los `width="NN%"` de la columna de texto, una por fila y en ciclo: las
	// rayas no miden todas lo mismo. Vacío es todo el ancho.
	Partes []float32
	// Cola es un bloque a la derecha EN LA MISMA LÍNEA, centrado con la raya: la meta de
	// la fila de Mail (`justify-between gap-3`). La raya encoge antes que pisarla.
	Cola fyne.Size
	// Radio es el `rounded-*` de la raya; cero es el `rounded-md` de Skeleton.tsx. Una
	// raya a todo el alto de la fila hace de fila entera: los cuadernos del menú de
	// Notes son `radius-nav`, como el destino que van a ser.
	Radio float32
}

// RecetaEsqueleto es la geometría de las filas de una lista que no es la de `ListItem`:
// la bandeja de Mail, la libreta de Contactos, el historial de Rooms, las filas y el
// árbol de Notes o la lista del día de Calendar. Cada una copia la fila real a la que
// sustituye —sin eso el layout salta al llegar los datos—; lo común (un único latido, un
// único «Cargando», el filete y el reparto) lo pone FilasEsqueleto.
type RecetaEsqueleto struct {
	// Alto es lo que mide cada fila, con su filete; Hueco, el aire entre dos filas (el
	// `gap` o el `mt` de la columna); Encima, el de antes de la primera.
	Alto, Hueco, Encima float32
	// Izq y Der son el relleno horizontal de la fila (su `px`), por dentro del Margen.
	Izq, Der float32
	// Delantero es la medida de la pieza delantera; con cero, la de LadoDelanteroEsqueleto.
	// La forma la dice el DelanteroEsqueleto de las filas: círculo el avatar y caja
	// `rounded-md` la tesela. HuecoDelantero es el `gap` hasta el texto.
	Delantero      fyne.Size
	HuecoDelantero float32
	// Rayas son las líneas de texto, de arriba abajo, separadas por HuecoRayas.
	Rayas      []RayaEsqueleto
	HuecoRayas float32
	// Arriba es `items-start`: el delantero y las rayas cuelgan de Relleno (el `pt` de la
	// fila), bajados BajaDelantero y BajaRayas. Sin él, los dos se centran en la fila.
	Arriba                            bool
	Relleno, BajaDelantero, BajaRayas float32
	// Sangrias es lo que entra cada fila por la izquierda (las ramas de un árbol), una
	// por fila; las que faltan, cero.
	Sangrias []float32
}

// altoRayas es lo que mide la columna de texto de la receta.
func (r *RecetaEsqueleto) altoRayas() float32 {
	var h float32
	for i, l := range r.Rayas {
		if i > 0 {
			h += r.HuecoRayas
		}
		h += l.Alto
	}
	return h
}

// FilasEsqueleto es `SkeletonRows`: las filas de carga de una lista, ya montadas.
//
// Cada app se hacía las suyas —Drive tenía tres, con tres geometrías—, todas con un
// `Esqueleto` por bloque, o sea una animación por bloque. Aquí late la lista entera A UNA,
// con una sola animación que tiñe todos los rectángulos, que es además lo que se ve en la
// web: todos los `.sytue-skeleton` arrancan juntos y pulsan en fase.
//
// Para el lector de pantalla es un único «Cargando» (`role="status"` una vez, §3.18): los
// bloques son rectángulos de lienzo y no llegan a él.
type FilasEsqueleto struct {
	widget.BaseWidget

	Filas     int
	Delantero DelanteroEsqueleto
	// Lineas es 1 (título) o 2 (título y subtítulo).
	Lineas int
	Meta   []MetaEsqueleto
	// Margen es el relleno horizontal del contenedor (el `px-3` de una lista), por fuera
	// del `px-3` de cada fila.
	Margen float32
	// Filete es el `border-b` de cada fila (`rowClass`).
	Filete bool
	// SinUltimoFilete es el `last:border-b-0`: dentro de una caja con borde, el filete
	// de la última fila se sumaría al de la caja.
	SinUltimoFilete bool
	// Rotulo es el `label`; vacío es «Cargando» (`ui.loading`).
	Rotulo string
	// Receta cambia la geometría de `SkeletonRows` por la de una lista propia; con ella,
	// Lineas y Meta no se usan (ConReceta).
	Receta *RecetaEsqueleto
	// Tinte es el token de los bloques; vacío es `bg-subtle`. Sobre el cromo oscuro del
	// menú es `sidebar-hover-bg` (SobreCromo), como el Esqueleto suelto.
	Tinte string

	latidoEsqueleto
}

// latidoEsqueleto es el ÚNICO latido de un grupo de bloques de carga: una animación que
// lleva la fase y tiñe todos sus rectángulos a la vez. Lo comparten FilasEsqueleto y
// BloquesEsqueleto.
type latidoEsqueleto struct {
	mu   sync.Mutex
	fase float32
	anim *fyne.Animation
}

// latir lanza el latido; `tenir` repinta los bloques en cada fotograma. Con «menos
// movimiento» no late: los bloques se quedan opacos, como el Esqueleto suelto.
func (l *latidoEsqueleto) latir(tenir func()) {
	if MenosMovimiento() {
		return
	}
	a := fyne.NewAnimation(DuracionPulso, func(p float32) {
		l.mu.Lock()
		l.fase = p
		l.mu.Unlock()
		tenir()
	})
	a.Curve = fyne.AnimationLinear
	a.RepeatCount = fyne.AnimationRepeatForever
	l.mu.Lock()
	l.anim = a
	l.mu.Unlock()
	a.Start()
}

// Parar detiene el latido. Lo llama el renderer al destruirse.
func (l *latidoEsqueleto) Parar() {
	l.mu.Lock()
	a := l.anim
	l.anim = nil
	l.mu.Unlock()
	if a != nil {
		a.Stop()
	}
}

// tinta es el relleno de los bloques en el instante del latido.
func (l *latidoEsqueleto) tinta(tinte string) color.NRGBA {
	l.mu.Lock()
	fase := l.fase
	l.mu.Unlock()
	return tintaEsqueleto(tinte, fase)
}

var (
	_ fyne.Widget     = (*FilasEsqueleto)(nil)
	_ fyne.Accessible = (*FilasEsqueleto)(nil)
)

// NuevasFilasEsqueleto crea `filas` filas de carga de `lineas` líneas.
func NuevasFilasEsqueleto(filas int, d DelanteroEsqueleto, lineas int) *FilasEsqueleto {
	f := &FilasEsqueleto{Filas: filas, Delantero: d, Lineas: lineas}
	f.ExtendBaseWidget(f)
	return f
}

// ConReceta les da la geometría de una lista propia y las devuelve, para encadenarlo.
func (f *FilasEsqueleto) ConReceta(r RecetaEsqueleto) *FilasEsqueleto {
	f.Receta = &r
	f.Lineas, f.Meta = len(r.Rayas), nil
	return f
}

// ConRotulo cambia el «Cargando» por el de la lista («Cargando mensajes…»), ya
// traducido, y las devuelve.
func (f *FilasEsqueleto) ConRotulo(rotulo string) *FilasEsqueleto {
	f.Rotulo = rotulo
	return f
}

// SobreCromo tiñe los bloques para el carril oscuro y las devuelve.
func (f *FilasEsqueleto) SobreCromo() *FilasEsqueleto {
	f.Tinte = "sidebar-hover-bg"
	return f
}

// AccessibilityLabel es la región `role="status"`: se dice una vez para toda la lista.
func (f *FilasEsqueleto) AccessibilityLabel() string {
	if f.Rotulo != "" {
		return f.Rotulo
	}
	return textoComun("ui.loading")
}

// AccessibilityRole: como la Rueda, Fyne no tiene «status» y lo que llega es el texto.
func (f *FilasEsqueleto) AccessibilityRole() fyne.AccessibleRole {
	return fyne.AccessibleRoleText
}

// parteTitulo y parteSub son los anchos de la web (`38 + (i*11)%42` y `50 + (i*17)%35`
// por ciento): iguales, el esqueleto parece una tabla vacía en vez de algo que llega.
func parteTitulo(i int) float32 { return float32(38+(i*11)%42) / 100 }
func parteSub(i int) float32    { return float32(50+(i*17)%35) / 100 }

// filaEsq son los bloques de una fila; los nil no se pintan.
type filaEsq struct {
	delantero, titulo, sub, filete *canvas.Rectangle
	meta                           []*canvas.Rectangle
	// rayas y colas son las de una receta (una cola por raya, nil sin ella).
	rayas, colas []*canvas.Rectangle
}

// CreateRenderer monta los rectángulos y arranca el latido.
func (f *FilasEsqueleto) CreateRenderer() fyne.WidgetRenderer {
	r := &filasEsqueletoRenderer{f: f}
	nuevo := func() *canvas.Rectangle {
		b := canvas.NewRectangle(color.NRGBA{})
		r.bloques = append(r.bloques, b)
		r.objetos = append(r.objetos, b)
		return b
	}
	for range max(f.Filas, 0) {
		var fila filaEsq
		if f.Delantero != SinDelantero {
			fila.delantero = nuevo()
		}
		if f.Receta != nil {
			for _, l := range f.Receta.Rayas {
				fila.rayas = append(fila.rayas, nuevo())
				var cola *canvas.Rectangle
				if l.Cola.Width > 0 {
					cola = nuevo()
				}
				fila.colas = append(fila.colas, cola)
			}
		} else {
			fila.titulo = nuevo()
			if f.Lineas >= 2 {
				fila.sub = nuevo()
			}
			for range f.Meta {
				fila.meta = append(fila.meta, nuevo())
			}
		}
		if f.Filete && (!f.SinUltimoFilete || len(r.filas) != f.Filas-1) {
			// El filete es borde, no esqueleto: no late.
			fila.filete = canvas.NewRectangle(color.NRGBA{})
			r.objetos = append(r.objetos, fila.filete)
		}
		r.filas = append(r.filas, fila)
	}
	f.latir(r.tenir)
	r.Refresh()
	return r
}

type filasEsqueletoRenderer struct {
	f       *FilasEsqueleto
	filas   []filaEsq
	bloques []*canvas.Rectangle
	objetos []fyne.CanvasObject
}

func (r *filasEsqueletoRenderer) alto() float32 {
	if r.f.Receta != nil {
		return r.f.Receta.Alto
	}
	return AltoFilaEsqueleto(r.f.Delantero, r.f.Lineas)
}

// paso es lo que hay de una fila a la siguiente y encima lo de antes de la primera.
func (r *filasEsqueletoRenderer) paso() (paso, encima float32) {
	if rc := r.f.Receta; rc != nil {
		return rc.Alto + rc.Hueco, rc.Encima
	}
	return r.alto(), 0
}

func (r *filasEsqueletoRenderer) MinSize() fyne.Size {
	n := float32(len(r.filas))
	if n == 0 {
		return fyne.NewSize(0, 0)
	}
	paso, encima := r.paso()
	return fyne.NewSize(0, encima+n*paso-(paso-r.alto()))
}

func (r *filasEsqueletoRenderer) Objects() []fyne.CanvasObject { return r.objetos }
func (r *filasEsqueletoRenderer) Destroy()                     { r.f.Parar() }

// ponerBloque coloca un bloque del esqueleto; un ancho negativo se queda en cero.
func ponerBloque(b *canvas.Rectangle, x, y, w, h float32) {
	b.Move(fyne.NewPos(x, y))
	b.Resize(fyne.NewSize(max(w, 0), h))
}

func (r *filasEsqueletoRenderer) Layout(t fyne.Size) {
	if r.f.Receta != nil {
		r.layoutReceta(t)
		return
	}
	alto := r.alto()
	izq := r.f.Margen + MargenFila
	der := t.Width - r.f.Margen - MargenFila
	lado := LadoDelanteroEsqueleto(r.f.Delantero)
	for i, fila := range r.filas {
		y := float32(i) * alto
		caja := alto
		if fila.filete != nil {
			caja -= tema.Borde
			ponerBloque(fila.filete, r.f.Margen, y+caja, t.Width-2*r.f.Margen, tema.Borde)
		}
		centro := func(h float32) float32 { return y + (caja-h)/2 }
		x := izq
		if fila.delantero != nil {
			ponerBloque(fila.delantero, x, centro(lado), lado, lado)
			fila.delantero.CornerRadius = tema.RadioMD
			if r.f.Delantero == DelanteroAvatar {
				fila.delantero.CornerRadius = lado / 2
			}
			x += lado + HuecoFila
		}
		finTexto := r.colocarMeta(fila, der, centro)
		util := max(finTexto-x, 0)
		texto := AltoTituloEsqueleto
		if fila.sub != nil {
			texto += huecoLineasEsq + AltoSubEsqueleto
		}
		ty := centro(texto)
		ponerBloque(fila.titulo, x, ty, float32(int(util*parteTitulo(i)+0.5)), AltoTituloEsqueleto)
		fila.titulo.CornerRadius = tema.RadioMD
		if fila.sub != nil {
			ponerBloque(fila.sub, x, ty+AltoTituloEsqueleto+huecoLineasEsq, float32(int(util*parteSub(i)+0.5)), AltoSubEsqueleto)
			fila.sub.CornerRadius = tema.RadioMD
		}
	}
}

// layoutReceta reparte las filas con la geometría de una lista propia.
func (r *filasEsqueletoRenderer) layoutReceta(t fyne.Size) {
	rc := r.f.Receta
	paso, encima := r.paso()
	lado := rc.Delantero
	if lado.Width <= 0 || lado.Height <= 0 {
		l := LadoDelanteroEsqueleto(r.f.Delantero)
		lado = fyne.NewSize(l, l)
	}
	der := t.Width - r.f.Margen - rc.Der
	texto := rc.altoRayas()
	for i, fila := range r.filas {
		y := encima + float32(i)*paso
		caja := rc.Alto
		if fila.filete != nil {
			caja -= tema.Borde
			ponerBloque(fila.filete, r.f.Margen, y+caja, t.Width-2*r.f.Margen, tema.Borde)
		}
		x := r.f.Margen + rc.Izq
		if i < len(rc.Sangrias) {
			x += rc.Sangrias[i]
		}
		if fila.delantero != nil {
			dy := y + (caja-lado.Height)/2
			if rc.Arriba {
				dy = y + rc.Relleno + rc.BajaDelantero
			}
			ponerBloque(fila.delantero, x, dy, lado.Width, lado.Height)
			fila.delantero.CornerRadius = tema.RadioMD
			if r.f.Delantero == DelanteroAvatar {
				fila.delantero.CornerRadius = min(lado.Width, lado.Height) / 2
			}
			x += lado.Width + rc.HuecoDelantero
		}
		util := max(der-x, 0)
		ty := y + (caja-texto)/2
		if rc.Arriba {
			ty = y + rc.Relleno + rc.BajaRayas
		}
		for j, l := range rc.Rayas {
			ancho := util
			switch {
			case l.Ancho > 0:
				ancho = min(l.Ancho, util)
			case len(l.Partes) > 0:
				ancho = float32(int(util*l.Partes[i%len(l.Partes)] + 0.5))
			}
			if cola := fila.colas[j]; cola != nil {
				ancho = min(ancho, max(util-l.Cola.Width-rc.HuecoDelantero, 0))
				ponerBloque(cola, der-l.Cola.Width, ty+(l.Alto-l.Cola.Height)/2, l.Cola.Width, l.Cola.Height)
				cola.CornerRadius = tema.RadioMD
			}
			ponerBloque(fila.rayas[j], x, ty, ancho, l.Alto)
			fila.rayas[j].CornerRadius = tema.RadioMD
			if l.Radio > 0 {
				fila.rayas[j].CornerRadius = l.Radio
			}
			ty += l.Alto + rc.HuecoRayas
		}
	}
}

// colocarMeta pone la meta de una fila de derecha a izquierda (la oculta no ocupa
// sitio) y devuelve dónde acaba el texto.
func (r *filasEsqueletoRenderer) colocarMeta(fila filaEsq, der float32, centro func(float32) float32) float32 {
	xm := der
	hayMeta := false
	for j := len(fila.meta) - 1; j >= 0; j-- {
		m, b := r.f.Meta[j], fila.meta[j]
		b.CornerRadius = tema.RadioMD
		if m.Oculta {
			b.Hide()
			continue
		}
		b.Show()
		h := m.Alto
		if h <= 0 {
			h = AltoSubEsqueleto
		}
		if hayMeta {
			xm -= huecoMetaEsq
		}
		xm -= m.Ancho
		ponerBloque(b, xm, centro(h), m.Ancho, h)
		hayMeta = true
	}
	if !hayMeta {
		return der
	}
	return xm - HuecoFila
}

// tenir pinta todos los bloques con el instante del latido.
func (r *filasEsqueletoRenderer) tenir() {
	c := r.f.tinta(r.f.Tinte)
	for _, b := range r.bloques {
		b.FillColor = c
		b.Refresh()
	}
}

// Refresh vuelve a tenir (el tema puede haber cambiado) y recoloca.
func (r *filasEsqueletoRenderer) Refresh() {
	v := tema.De(fyne.CurrentApp().Settings().ThemeVariant())
	for _, fila := range r.filas {
		if fila.filete != nil {
			fila.filete.FillColor = tema.Color("border", v)
			fila.filete.Refresh()
		}
	}
	r.tenir()
	r.Layout(r.f.Size())
}
