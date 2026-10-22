// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import "sort"

// # Los pasos de una diapositiva
//
// Es la parte PURA de `slides/anim.ts`: qué cajas se ven en cada clic de la
// presentación y en qué orden. No mueve nada —el movimiento es de quien pinta—; decide
// qué hay en pantalla, que es lo que no puede equivocarse: una caja con entrada que
// sale desde el principio le chafa el remate a quien presenta.
//
// Se porta tal cual, con sus nombres de campo: el mismo documento se presenta en la
// web y aquí, y dos lecturas distintas de `trigger` serían dos presentaciones
// distintas del mismo mazo.

// Categoria es `SlideAnimCategory`. Se DERIVA del tipo; no se guarda.
type Categoria uint8

// Las cuatro, en el orden de ANIM_CATEGORY.
const (
	Entrada Categoria = iota
	Enfasis
	Salida
	Movimiento
)

// categoriaAnim es ANIM_CATEGORY: la categoría de cada efecto y, a la vez, el conjunto
// de tipos VÁLIDOS. Un tipo que no está aquí se descarta al leer, como en la web.
var categoriaAnim = map[string]Categoria{
	"appear": Entrada, "fade": Entrada, "fly": Entrada, "zoom": Entrada,
	"pulse": Enfasis, "spin": Enfasis, "grow-shrink": Enfasis,
	"disappear": Salida, "fade-out": Salida, "fly-out": Salida, "zoom-out": Salida,
	"motion-right": Movimiento, "motion-left": Movimiento, "motion-up": Movimiento,
	"motion-down": Movimiento, "motion-up-right": Movimiento, "motion-up-left": Movimiento,
	"motion-down-right": Movimiento, "motion-down-left": Movimiento, "motion-loop": Movimiento,
	"motion-arc": Movimiento, "motion-custom": Movimiento,
}

// disparadores válidos (TRIGGERS).
var disparadores = map[string]bool{"click": true, "with": true, "after": true}

// escalonMS es STAGGER_MS: el escalón entre animaciones 'after' dentro de un paso.
const escalonMS = 260

// AnimsDe es `boxAnims`: la secuencia saneada de una caja. Migra el `anim` único del
// modelo viejo y descarta los efectos que no existen. No renumera: eso es global
// (NormalizarOrden).
func AnimsDe(c Caja) []Anim {
	crudas := c.Anims
	if crudas == nil && c.Anim != nil {
		crudas = []Anim{*c.Anim}
	}
	var fuera []Anim
	for _, a := range crudas {
		if _, ok := categoriaAnim[a.Tipo]; !ok {
			continue
		}
		limpia := Anim{Tipo: a.Tipo, Orden: a.Orden}
		if disparadores[a.Disparo] {
			limpia.Disparo = a.Disparo
		}
		// Los puntos solo valen en la trayectoria personalizada; las demás derivan su
		// geometría del tipo.
		if a.Tipo == "motion-custom" {
			limpia.Puntos = a.Puntos
		}
		fuera = append(fuera, limpia)
	}
	return fuera
}

// conAnims es `withAnims`: escribe la secuencia y quita el `anim` viejo; vacía = sin
// campo, que es el JSON limpio que escribe la web.
func conAnims(c Caja, anims []Anim) Caja {
	c.Anim = nil
	c.Anims = nil
	if len(anims) > 0 {
		c.Anims = anims
	}
	return c
}

type entradaAnim struct {
	caja   string
	indice int
	anim   Anim
}

// entradasDe aplana las animaciones de la diapositiva, ordenadas por `order` (estable,
// como el `sort` de JavaScript).
func entradasDe(cajas []Caja) []entradaAnim {
	var fuera []entradaAnim
	for _, c := range cajas {
		for i, a := range AnimsDe(c) {
			fuera = append(fuera, entradaAnim{caja: c.ID, indice: i, anim: a})
		}
	}
	sort.SliceStable(fuera, func(i, j int) bool { return fuera[i].anim.Orden < fuera[j].anim.Orden })
	return fuera
}

// NormalizarOrden es `normalizeAnimOrders`: renumera `order` a 1..N en toda la
// diapositiva y migra el modelo viejo. Una caja sin animaciones se queda como estaba
// (solo se le limpia el campo viejo, si lo traía).
func NormalizarOrden(cajas []Caja) []Caja {
	rango := map[entradaClave]int{}
	for i, e := range entradasDe(cajas) {
		rango[entradaClave{e.caja, e.indice}] = i + 1
	}
	fuera := make([]Caja, len(cajas))
	for i, c := range cajas {
		anims := AnimsDe(c)
		if len(anims) == 0 {
			if c.Anim != nil || c.Anims != nil {
				c = conAnims(c, nil)
			}
			fuera[i] = c
			continue
		}
		for k := range anims {
			anims[k].Orden = float64(rango[entradaClave{c.ID, k}])
		}
		fuera[i] = conAnims(c, anims)
	}
	return fuera
}

type entradaClave struct {
	caja   string
	indice int
}

// Planeada es `PlannedAnim`: una animación colocada en el tiempo de la presentación.
type Planeada struct {
	Caja      string
	Indice    int
	Anim      Anim
	Categoria Categoria
	// Paso es el clic, 1..N, en que se dispara.
	Paso int
	// RetardoMS es el escalón de las 'after' dentro de su paso.
	RetardoMS int
}

// Plan es `slidePlan`: la secuencia de la diapositiva con el paso y el retardo de cada
// animación. La primera siempre abre paso, diga lo que diga su disparador.
func Plan(d Diapo) []Planeada {
	entradas := entradasDe(d.Cajas)
	fuera := make([]Planeada, len(entradas))
	paso, tras := 0, 0
	for i, e := range entradas {
		disparo := e.anim.Disparo
		if disparo == "" {
			disparo = "click"
		}
		switch {
		case i == 0 || disparo == "click":
			paso++
			tras = 0
		case disparo == "after":
			tras++
		}
		// 'with' hereda el paso y el retardo de la anterior: van juntas.
		fuera[i] = Planeada{
			Caja: e.caja, Indice: e.indice, Anim: e.anim,
			Categoria: categoriaAnim[e.anim.Tipo], Paso: paso, RetardoMS: tras * escalonMS,
		}
	}
	return fuera
}

// Pasos es `animStepCount`: cuántos clics tiene la diapositiva (0 sin animaciones).
func Pasos(d Diapo) int {
	p := Plan(d)
	if len(p) == 0 {
		return 0
	}
	return p[len(p)-1].Paso
}

// PorCaja es `planByBox`: las animaciones planeadas de cada caja, en orden.
func PorCaja(plan []Planeada) map[string][]Planeada {
	m := map[string][]Planeada{}
	for _, p := range plan {
		m[p.Caja] = append(m[p.Caja], p)
	}
	return m
}

// PresenteEn es `boxPresentAtStep`: si la caja se ve al terminar el paso `paso`. Sin
// entrada se ve desde el principio; manda su última entrada o salida disparada; el
// énfasis no cambia nada.
func PresenteEn(suyas []Planeada, paso int) bool {
	presente := true
	for _, p := range suyas {
		if p.Categoria == Entrada {
			presente = false
			break
		}
	}
	for _, p := range suyas {
		if p.Paso > paso {
			break // ordenadas por `order`, así que el paso no decrece
		}
		switch p.Categoria {
		case Entrada:
			presente = true
		case Salida:
			presente = false
		case Enfasis, Movimiento:
		}
	}
	return presente
}

// Avanzar es `presentAdvance`: la siguiente posición (diapositiva, paso) al avanzar
// (dir 1) o retroceder (dir −1). Avanzar revela el paso siguiente y, agotados, salta a
// la próxima en su paso 0; retroceder oculta el último paso o vuelve a la anterior YA
// construida. Con topes en los dos extremos.
func Avanzar(diapos []Diapo, diapo, paso, dir int) (int, int) {
	ultima := len(diapos) - 1
	if ultima < 0 {
		return 0, 0
	}
	cur := min(max(diapo, 0), ultima)
	pasos := Pasos(diapos[cur])
	s := min(max(paso, 0), pasos)
	if dir > 0 {
		switch {
		case s < pasos:
			return cur, s + 1
		case cur < ultima:
			return cur + 1, 0
		}
		return cur, s
	}
	switch {
	case s > 0:
		return cur, s - 1
	case cur > 0:
		return cur - 1, Pasos(diapos[cur-1])
	}
	return cur, 0
}

// ---- trayectorias de movimiento ----

// Las trayectorias de `anim.ts`: una polilínea de DESFASES en % del lienzo desde la
// posición de la caja (el primer punto es siempre {0, 0}).
const (
	movRecto    = 30 // MOTION_STRAIGHT
	movDiagonal = 21 // MOTION_DIAG
	muestrasArc = 12 // ARC_SAMPLES
	maxPuntos   = 64 // MOTION_MAX_POINTS
)

// PuntosMovimiento es `motionPoints`: los vértices de la trayectoria de ese tipo; la
// personalizada, con sus puntos saneados. Un tipo que no es de movimiento, solo el
// origen.
func PuntosMovimiento(tipo string, propios []Punto) []Punto {
	recta := func(dx, dy float64) []Punto { return []Punto{{}, {DX: dx, DY: dy}} }
	switch tipo {
	case "motion-right":
		return recta(movRecto, 0)
	case "motion-left":
		return recta(-movRecto, 0)
	case "motion-up":
		return recta(0, -movRecto)
	case "motion-down":
		return recta(0, movRecto)
	case "motion-up-right":
		return recta(movDiagonal, -movDiagonal)
	case "motion-up-left":
		return recta(-movDiagonal, -movDiagonal)
	case "motion-down-right":
		return recta(movDiagonal, movDiagonal)
	case "motion-down-left":
		return recta(-movDiagonal, movDiagonal)
	case "motion-loop":
		return []Punto{{}, {DX: 30}, {DX: 30, DY: 22}, {DY: 22}, {}}
	case "motion-arc":
		// La Bézier cuadrática {0,0} → (20, 26) → (40, 0), en 12 tramos.
		pts := make([]Punto, 0, muestrasArc+1)
		for i := range muestrasArc + 1 {
			t := float64(i) / muestrasArc
			pts = append(pts, Punto{DX: 2*(1-t)*t*20 + t*t*40, DY: 2 * (1 - t) * t * 26})
		}
		return pts
	case "motion-custom":
		return sanearPuntos(propios)
	}
	return []Punto{{}}
}

// sanearPuntos es `sanitizeMotionPoints`: hasta 64 puntos topados a ±100 %, el primero
// en el origen y al menos dos (si no, la diagonal de arranque).
func sanearPuntos(crudos []Punto) []Punto {
	if len(crudos) < 2 {
		return []Punto{{}, {DX: 25, DY: 15}}
	}
	pts := make([]Punto, 0, min(len(crudos), maxPuntos))
	for _, p := range crudos[:min(len(crudos), maxPuntos)] {
		pts = append(pts, Punto{DX: min(max(p.DX, -100), 100), DY: min(max(p.DY, -100), 100)})
	}
	pts[0] = Punto{}
	return pts
}

// FinMovimiento es `motionEnd`: el desfase del último vértice.
func FinMovimiento(a Anim) Punto {
	pts := PuntosMovimiento(a.Tipo, a.Puntos)
	return pts[len(pts)-1]
}

// DesfaseReposo es `motionRestOffset`: la suma de los finales de las trayectorias de la
// caja YA disparadas en el paso `paso`, que es donde se queda en los siguientes.
//
// ponytail: suma simple, como la web: dos trayectorias del mismo paso cuentan las dos.
func DesfaseReposo(suyas []Planeada, paso int) Punto {
	var d Punto
	for _, p := range suyas {
		if p.Paso > paso {
			break
		}
		if p.Categoria == Movimiento {
			f := FinMovimiento(p.Anim)
			d.DX, d.DY = d.DX+f.DX, d.DY+f.DY
		}
	}
	return d
}
