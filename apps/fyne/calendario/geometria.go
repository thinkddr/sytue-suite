// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"image/color"
	"math"
	"sort"
	"time"
)

// La geometría PURA de las rejillas: `multiDayLayout.ts` (las barras de varios días del
// mes y de la franja «todo el día») y `timeLayout.ts` (los bloques de la rejilla
// horaria). Entran instantes ya resueltos a la zona de visualización y sale geometría:
// índices, columnas, filas y minutos. La conversión a píxeles la hace quien pinta.
//
// Es un port línea a línea, con sus mismos casos de prueba en `geometria_test.go`: si
// la web cambia una regla, el test de allí y el de aquí se ponen rojos a la vez.

// Barra es `Bar<T>`: un elemento acotado a los índices [Desde, Hasta] INCLUSIVOS de un
// array de días consecutivos.
type Barra[T any] struct {
	Item         T
	Desde, Hasta int
	// Fila es la de apilado (0 arriba): dos barras que se solapan nunca la comparten.
	Fila int
}

// Tramo es `Segment<T>`: una barra recortada a UNA semana, lista para pintar.
type Tramo[T any] struct {
	Barra[T]
	// Col es la columna (0..porSemana−1) donde arranca dentro de su semana, y Ancho
	// cuántas ocupa.
	Col, Ancho int
	// SigueIzq: empezó en una semana anterior (borde recto y SIN texto). SigueDer:
	// sigue en la siguiente (borde recto).
	SigueIzq, SigueDer bool
}

// deltaDias es `dayDelta`: días naturales de `a` a `b`, a salvo del cambio de hora
// porque redondea al día.
func deltaDias(a, b time.Time, loc *time.Location) int {
	return int(math.Round(inicioDelDia(b, loc).Sub(inicioDelDia(a, loc)).Hours() / 24))
}

// Tramar es `barSpan`: acota [inicio, fin) a `dias` y lo devuelve en índices
// INCLUSIVOS. El fin es EXCLUSIVO, como en iCalendar (un «todo el día» de tres días
// lleva DTEND el cuarto): el último día cubierto es el del instante justo anterior.
// Sin fin, o con fin ≤ inicio, cubre solo el día de su inicio. ok=false si no solapa.
func Tramar(dias []time.Time, inicio, fin time.Time, loc *time.Location) (desde, hasta int, ok bool) {
	if len(dias) == 0 {
		return 0, 0, false
	}
	ultimoIdx := len(dias) - 1
	ultimo := inicio
	if !fin.IsZero() && fin.After(inicio) {
		ultimo = fin.Add(-time.Millisecond)
	}
	s := deltaDias(dias[0], inicio, loc)
	e := deltaDias(dias[0], ultimo, loc)
	if e < 0 || s > ultimoIdx {
		return 0, 0, false
	}
	s, e = max(s, 0), min(e, ultimoIdx)
	if e < s {
		return 0, 0, false
	}
	return s, e, true
}

// Apilar es `assignRows`: fila de apilado por «first-fit» voraz. Orden ESTABLE —por
// inicio, luego la más larga arriba, luego el de entrada—, así que el mismo conjunto
// da siempre el mismo apilado.
//
// ponytail: O(n·R) tras ordenar; R son las filas de una semana, siempre pocas.
func Apilar[T any](barras []Barra[T]) []Barra[T] {
	orden := make([]int, len(barras))
	for i := range orden {
		orden[i] = i
	}
	sort.SliceStable(orden, func(i, j int) bool {
		a, b := barras[orden[i]], barras[orden[j]]
		if a.Desde != b.Desde {
			return a.Desde < b.Desde
		}
		return b.Hasta-b.Desde < a.Hasta-a.Desde
	})
	var finFila []int // finFila[f] = último índice ocupado en la fila f
	out := make([]Barra[T], 0, len(barras))
	for _, i := range orden {
		b := barras[i]
		f := 0
		for f < len(finFila) && finFila[f] >= b.Desde {
			f++
		}
		if f == len(finFila) {
			finFila = append(finFila, 0)
		}
		finFila[f] = b.Hasta
		b.Fila = f
		out = append(out, b)
	}
	return out
}

// Semanas es `sliceWeeks`: recorta cada barra colocada a sus semanas y la pasa a
// coordenadas de columna, con los cortes de continuación en los saltos de semana.
func Semanas[T any](colocadas []Barra[T], cuantas, porSemana int) [][]Tramo[T] {
	semanas := make([][]Tramo[T], cuantas)
	for _, b := range colocadas {
		for w := max(0, b.Desde/porSemana); w <= b.Hasta/porSemana && w < cuantas; w++ {
			ini, fin := w*porSemana, w*porSemana+porSemana-1
			s, e := max(b.Desde, ini), min(b.Hasta, fin)
			semanas[w] = append(semanas[w], Tramo[T]{
				Barra: b, Col: s - ini, Ancho: e - s + 1,
				SigueIzq: b.Desde < ini, SigueDer: b.Hasta > fin,
			})
		}
	}
	return semanas
}

// Limitar es `limitWeekRows`: se ven las barras de las `maxFilas` primeras filas y las
// demás se cuentan, por columna, para el «+N más» de cada día.
func Limitar[T any](tramos []Tramo[T], maxFilas, porSemana int) (visibles []Tramo[T], ocultasPorCol []int) {
	ocultasPorCol = make([]int, porSemana)
	for _, t := range tramos {
		if t.Fila < maxFilas {
			visibles = append(visibles, t)
			continue
		}
		for c := t.Col; c < t.Col+t.Ancho && c < porSemana; c++ {
			ocultasPorCol[c]++
		}
	}
	return visibles, ocultasPorCol
}

// Filas es `rowCount`: cuántas filas de barras ocupa una semana.
func Filas[T any](tramos []Tramo[T]) int {
	n := 0
	for _, t := range tramos {
		n = max(n, t.Fila+1)
	}
	return n
}

// canal es el paso de sRGB a lineal de la luminancia relativa de WCAG.
func canal(c uint8) float64 {
	s := float64(c) / 255
	if s <= 0.03928 {
		return s / 12.92
	}
	return math.Pow((s+0.055)/1.055, 2.4)
}

// Luminancia es `luminance`: la relativa de WCAG, de 0 (negro) a 1 (blanco).
func Luminancia(c color.NRGBA) float64 {
	return 0.2126*canal(c.R) + 0.7152*canal(c.G) + 0.0722*canal(c.B)
}

// Los dos textos posibles sobre una barra de color: `#ffffff` y `#111827`.
var (
	TintaClaraBarra  = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	TintaOscuraBarra = color.NRGBA{R: 0x11, G: 0x18, B: 0x27, A: 0xff}
)

// TintaSobre es `barTextColor`: blanco o tinta oscura, la de MÁS contraste sobre el
// relleno. El color del calendario lo elige la persona, así que el texto de la barra no
// puede depender de él: con una de las dos siempre pasa de 4,5:1.
func TintaSobre(relleno color.NRGBA) color.NRGBA {
	l := Luminancia(relleno)
	if 1.05/(l+0.05) >= (l+0.05)/0.05 {
		return TintaClaraBarra
	}
	return TintaOscuraBarra
}

// ---- La rejilla horaria (`timeLayout.ts`) ----

const (
	// MinutosDia es `DAY_MINUTES`: la rejilla cubre siempre [0, 1440).
	MinutosDia = 24 * 60
	// PasoIman es `SNAP_MIN`: el arrastre se imanta a 15 minutos.
	PasoIman = 15
	// MinutosMinimos es `MIN_EVENT_MIN`: lo menos que ocupa un bloque.
	MinutosMinimos = 20
	// minutosPorDefecto es `DEFAULT_EVENT_MIN`, lo que dura un evento sin fin.
	minutosPorDefecto = 30
)

// Intervalo son minutos desde las 00:00 de un día.
type Intervalo struct{ Desde, Hasta int }

// MinutosDelDia es `dayMinutes`: acota [inicio, fin) al día y lo pasa a minutos desde
// medianoche. Lo que cruza a un día vecino se recorta a [0, 1440]. Sin fin, o con fin
// ≤ inicio, dura media hora para que se vea y se pueda pulsar. ok=false si no solapa.
func MinutosDelDia(inicio, fin, dia time.Time, loc *time.Location) (Intervalo, bool) {
	d0 := inicioDelDia(dia, loc)
	d1 := d0.Add(MinutosDia * time.Minute)
	if fin.IsZero() || !fin.After(inicio) {
		fin = inicio.Add(minutosPorDefecto * time.Minute)
	}
	if !inicio.Before(d1) || !fin.After(d0) {
		return Intervalo{}, false
	}
	a := max(0, int(math.Round(inicio.Sub(d0).Minutes())))
	b := min(MinutosDia, int(math.Round(fin.Sub(d0).Minutes())))
	if b <= a {
		return Intervalo{}, false
	}
	return Intervalo{a, b}, true
}

// Imantar es `snapMin`: redondea al paso y acota al día. `Math.round` de JS redondea
// los medios HACIA ARRIBA, también en negativo; `math.Floor(x+0.5)` es lo mismo.
func Imantar(minuto float64, paso int) int {
	s := int(math.Floor(minuto/float64(paso)+0.5)) * paso
	return max(0, min(MinutosDia, s))
}

// Bloque es un evento de la rejilla horaria ya repartido: su intervalo y su columna
// dentro del grupo de los que se solapan con él.
type Bloque[T any] struct {
	Item T
	Intervalo
	Col, Cols int
}

// Columnas es `layoutTimed`: agrupa los intervalos solapados en racimos y, dentro de
// cada uno, da a cada evento la primera columna libre. Los disjuntos ocupan el ancho
// entero. Sale por inicio (y a igualdad, el que acaba más tarde primero).
//
// ponytail: O(n²) por día, como en la web; n son los eventos de una jornada.
func Columnas[T any](items []Bloque[T]) []Bloque[T] {
	orden := append([]Bloque[T](nil), items...)
	sort.SliceStable(orden, func(i, j int) bool {
		if orden[i].Desde != orden[j].Desde {
			return orden[i].Desde < orden[j].Desde
		}
		return orden[i].Hasta > orden[j].Hasta
	})
	out := make([]Bloque[T], 0, len(orden))
	racimo := make([]Bloque[T], 0, len(orden))
	finRacimo := math.MinInt
	cerrar := func() {
		cols := 0
		for _, e := range racimo {
			cols = max(cols, e.Col+1)
		}
		for i := range racimo {
			racimo[i].Cols = cols
		}
		out = append(out, racimo...)
		racimo, finRacimo = nil, math.MinInt
	}
	for _, it := range orden {
		if it.Desde >= finRacimo {
			cerrar()
		}
		finCol := map[int]int{}
		for _, e := range racimo {
			if f, ok := finCol[e.Col]; !ok || e.Hasta > f {
				finCol[e.Col] = e.Hasta
			}
		}
		col := 0
		for {
			f, ok := finCol[col]
			if !ok || f <= it.Desde {
				break
			}
			col++
		}
		it.Col = col
		racimo = append(racimo, it)
		finRacimo = max(finRacimo, it.Hasta)
	}
	cerrar()
	return out
}
