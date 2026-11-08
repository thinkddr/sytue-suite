// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"math"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
)

// La pareja en la web es apps/frontend/apps/calendar/src/components/asaMes.ts.
//
// EL ASA DEL MES DEL TELÉFONO, como datos: a qué posición va la hoja según lo que se
// desplazó el dedo y a qué velocidad. Sin widgets: lo pinta vistamescompacta.go. Los
// vectores de test (A1…) son los mismos, con el mismo nombre, a los dos lados; si se
// cambia un umbral aquí, se cambia allí.
//
// Tres posiciones, de abajo arriba, como la hoja del calendario de iOS:
//   - PosMes: la rejilla entera (24 de iniciales + 40 por semana);
//   - PosSemana: plegada a la semana del día elegido (24 + 40);
//   - PosAgenda: sin rejilla (0); debajo va la agenda.
//
// Arrastrar hacia ARRIBA pliega (mes → semana → agenda); hacia abajo, al revés. Al soltar,
// la más cercana a la altura alcanzada; con un lanzamiento (`ui.VelocidadRapida`, el
// mismo «golpe rápido» que el deslizar), la siguiente en el sentido del lanzamiento.

// PosicionAsa es dónde está la hoja. El valor es el `aria-valuenow` del asa de la web.
type PosicionAsa int

// Las tres, de abajo arriba.
const (
	PosMes PosicionAsa = iota
	PosSemana
	PosAgenda
)

// Posiciones van de la más alta (la hoja entera) a la más baja, como `POSICIONES`.
var Posiciones = [...]PosicionAsa{PosMes, PosSemana, PosAgenda}

const (
	// RecorridoToque es `RECORRIDO_TOQUE`: menos que esto no es un arrastre.
	RecorridoToque float32 = 6
	// VentanaVelocidad es `VENTANA_VELOCIDAD`: la velocidad al soltar se mide sobre los
	// últimos milisegundos del gesto.
	VentanaVelocidad = 100 * time.Millisecond
)

// AlturaHoja es `alturaHoja`: el alto de la hoja (sin el asa) con `filas` semanas.
func AlturaHoja(pos PosicionAsa, filas int) float32 {
	switch pos {
	case PosMes:
		return AltoDiasCompacta + float32(filas)*AltoSemanaCompacta
	case PosSemana:
		return AltoDiasCompacta + AltoSemanaCompacta
	case PosAgenda:
	}
	return 0
}

// AlturaArrastre es `alturaArrastre`: la de partida más lo recorrido, sin salirse.
func AlturaArrastre(desde PosicionAsa, dy float32, filas int) float32 {
	return min(max(AlturaHoja(desde, filas)+dy, 0), AlturaHoja(PosMes, filas))
}

// GeometriaHoja es `geometriaHoja`: `plegado` es lo que suben las semanas bajo las
// iniciales (de 0 en el mes a filaCursor·40 en la semana, en proporción) y `subida` lo que
// sube todo por debajo de la semana, hasta desaparecer en la agenda. Continuas las dos.
func GeometriaHoja(h float32, filas, filaCursor int) (plegado, subida float32) {
	mes, semana := AlturaHoja(PosMes, filas), AlturaHoja(PosSemana, filas)
	f := float32(1)
	if mes > semana {
		f = min(max((mes-h)/(mes-semana), 0), 1)
	}
	return float32(filaCursor) * AltoSemanaCompacta * f, max(0, semana-h)
}

// MuestraAsa es un punto del gesto: cuándo y a qué altura iba el dedo.
type MuestraAsa struct {
	T time.Duration
	Y float32
}

// VelocidadFinal es `velocidadFinal`, en px/ms (positiva hacia abajo): de la primera
// muestra de los últimos VentanaVelocidad a la última. Con menos de dos, o sin tiempo, 0.
func VelocidadFinal(m []MuestraAsa) float64 {
	if len(m) == 0 {
		return 0
	}
	ultima := m[len(m)-1]
	primera := ultima
	for _, x := range m {
		if x.T >= ultima.T-VentanaVelocidad {
			primera = x
			break
		}
	}
	dt := float64(ultima.T-primera.T) / float64(time.Millisecond)
	if dt <= 0 {
		return 0
	}
	return float64(ultima.Y-primera.Y) / dt
}

// PosicionAlSoltar es `posicionAlSoltar`: `dy` es lo recorrido (positivo hacia abajo) y
// `v` la de VelocidadFinal. Con menos de RecorridoToque se queda donde estaba.
func PosicionAlSoltar(desde PosicionAsa, dy float32, v float64, filas int) PosicionAsa {
	if abs32(dy) < RecorridoToque {
		return desde
	}
	h := AlturaArrastre(desde, dy, filas)
	if math.Abs(v) >= ui.VelocidadRapida {
		// Posiciones va de más alta a menos: hacia arriba se busca en ese orden, hacia
		// abajo en el contrario. Si no queda ninguna, la del extremo.
		if v < 0 {
			for _, p := range Posiciones {
				if AlturaHoja(p, filas) < h {
					return p
				}
			}
			return PosAgenda
		}
		for i := len(Posiciones) - 1; i >= 0; i-- {
			if AlturaHoja(Posiciones[i], filas) > h {
				return Posiciones[i]
			}
		}
		return PosMes
	}
	// La más cercana; a la misma distancia, la más próxima a la de partida.
	mejor := desde
	for _, p := range Posiciones {
		d, dm := abs32(AlturaHoja(p, filas)-h), abs32(AlturaHoja(mejor, filas)-h)
		if d < dm || (d == dm && absInt(int(p-desde)) < absInt(int(mejor-desde))) {
			mejor = p
		}
	}
	return mejor
}

// PosicionAlTocar es `posicionAlTocar`: mes ↔ semana; desde la agenda, a la semana.
func PosicionAlTocar(p PosicionAsa) PosicionAsa {
	if p == PosSemana {
		return PosMes
	}
	return PosSemana
}

// PosicionTrasPaso es `posicionTrasPaso`: +1 hacia la agenda (↑), −1 hacia el mes (↓).
func PosicionTrasPaso(p PosicionAsa, paso int) PosicionAsa {
	return PosicionAsa(min(max(int(p)+paso, int(PosMes)), int(PosAgenda)))
}

// PosicionDeVista es `posicionDeVista`; el día no tiene asa (false).
func PosicionDeVista(v Vista) (PosicionAsa, bool) {
	switch v {
	case VistaMes:
		return PosMes, true
	case VistaSemana:
		return PosSemana, true
	case VistaAgenda:
		return PosAgenda, true
	case VistaDia:
	}
	return PosMes, false
}

// VistaDePosicion es `vistaDePosicion`: el selector del cajón y el asa dicen lo mismo.
func VistaDePosicion(p PosicionAsa) Vista {
	switch p {
	case PosSemana:
		return VistaSemana
	case PosAgenda:
		return VistaAgenda
	case PosMes:
	}
	return VistaMes
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
