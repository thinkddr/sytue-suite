// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package calendario

import (
	"math"
	"testing"
	"time"
)

// VECTORES ESPEJO del asa del mes del teléfono. Los mismos, con el mismo nombre, están en
// la web (apps/frontend/apps/calendar/src/components/asaMes.test.ts): si uno cambia aquí,
// cambia allí. Con 5 semanas la hoja mide 224 en el mes, 64 en la semana y 0 en la agenda;
// con 6, 264.

func TestPosicionAlSoltar(t *testing.T) {
	casos := []struct {
		id    string
		desde PosicionAsa
		dy    float32
		v     float64
		filas int
		want  PosicionAsa
	}{
		{"A1 sin-moverse", PosMes, -3, 0, 5, PosMes},
		{"A1b poco-pero-arrastre", PosMes, -7, 0, 5, PosMes},
		{"A1c temblor-rapido", PosMes, -5, -1, 5, PosMes},
		{"A2 sube-poco-lento", PosMes, -60, 0, 5, PosMes},
		{"A3 sube-pasada-la-mitad", PosMes, -100, 0, 5, PosSemana},
		{"A4 lanzamiento-corto-arriba", PosMes, -20, -0.8, 5, PosSemana},
		{"A5 sube-casi-todo-lento", PosMes, -200, 0, 5, PosAgenda},
		{"A5f lanzamiento-pasada-la-semana", PosMes, -170, -0.6, 5, PosAgenda},
		{"A6 semana-lanzada-arriba", PosSemana, -10, -0.6, 5, PosAgenda},
		{"A7 agenda-baja-lento", PosAgenda, 40, 0, 5, PosSemana},
		{"A8 agenda-lanzada-abajo", PosAgenda, 15, 0.9, 5, PosSemana},
		{"A9 semana-lanzada-abajo", PosSemana, 12, 0.7, 5, PosMes},
		{"A10 mes-lanzado-abajo", PosMes, 20, 1, 5, PosMes},
		{"A11 agenda-lanzada-arriba", PosAgenda, -20, -1, 5, PosAgenda},
		{"A12 lanzamiento-contrario", PosMes, -120, 0.7, 5, PosMes},
		{"A13 seis-semanas", PosMes, -130, 0, 6, PosSemana},
		{"A14 empate-se-queda", PosMes, -80, 0, 5, PosMes},
		{"A15 lento-por-debajo-del-lanzamiento", PosSemana, -20, -0.49, 5, PosSemana},
	}
	for _, c := range casos {
		if got := PosicionAlSoltar(c.desde, c.dy, c.v, c.filas); got != c.want {
			t.Errorf("%s: %d y es %d", c.id, got, c.want)
		}
	}
}

func TestAlturasDelAsa(t *testing.T) {
	// B1 alturas-de-cada-posicion
	if a, b, c, d := AlturaHoja(PosMes, 5), AlturaHoja(PosSemana, 5), AlturaHoja(PosAgenda, 5), AlturaHoja(PosMes, 6); a != 224 || b != 64 || c != 0 || d != 264 {
		t.Errorf("B1: %v %v %v %v", a, b, c, d)
	}
	// B2 el-arrastre-no-se-sale
	if a, b, c := AlturaArrastre(PosMes, 50, 5), AlturaArrastre(PosAgenda, -30, 5), AlturaArrastre(PosSemana, -10, 5); a != 224 || b != 0 || c != 54 {
		t.Errorf("B2: %v %v %v", a, b, c)
	}
	// B3 geometria-continua
	for _, c := range []struct {
		h              float32
		filas, cursor  int
		plegado, subir float32
	}{{224, 5, 2, 0, 0}, {144, 5, 2, 40, 0}, {64, 5, 2, 80, 0}, {32, 5, 2, 80, 32}, {0, 5, 0, 0, 64}} {
		if p, s := GeometriaHoja(c.h, c.filas, c.cursor); p != c.plegado || s != c.subir {
			t.Errorf("B3 h=%v: plegado %v subida %v; quiero %v %v", c.h, p, s, c.plegado, c.subir)
		}
	}
}

func TestVelocidadFinal(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	// C1 ultimos-100-ms
	m := []MuestraAsa{{ms(0), 0}, {ms(40), -10}, {ms(100), -40}, {ms(140), -80}}
	if v := VelocidadFinal(m); math.Abs(v+0.7) > 1e-6 {
		t.Errorf("C1: %v y es -0,7", v)
	}
	// C2 una-muestra-o-ninguna
	if a, b, c := VelocidadFinal(nil), VelocidadFinal([]MuestraAsa{{ms(5), 9}}), VelocidadFinal([]MuestraAsa{{ms(5), 9}, {ms(5), 30}}); a != 0 || b != 0 || c != 0 {
		t.Errorf("C2: %v %v %v", a, b, c)
	}
}

func TestToqueTecladoYVistasDelAsa(t *testing.T) {
	// D1 toque
	if a, b, c := PosicionAlTocar(PosMes), PosicionAlTocar(PosSemana), PosicionAlTocar(PosAgenda); a != PosSemana || b != PosMes || c != PosSemana {
		t.Errorf("D1: %d %d %d", a, b, c)
	}
	// D2 paso
	for _, c := range []struct {
		desde PosicionAsa
		paso  int
		want  PosicionAsa
	}{{PosMes, 1, PosSemana}, {PosSemana, 1, PosAgenda}, {PosAgenda, 1, PosAgenda}, {PosMes, -1, PosMes}, {PosAgenda, -2, PosMes}} {
		if got := PosicionTrasPaso(c.desde, c.paso); got != c.want {
			t.Errorf("D2 %d%+d: %d y es %d", c.desde, c.paso, got, c.want)
		}
	}
	// D3 vistas
	if _, hay := PosicionDeVista(VistaDia); hay {
		t.Error("D3: el día no tiene asa")
	}
	for _, p := range Posiciones {
		if got, hay := PosicionDeVista(VistaDePosicion(p)); !hay || got != p {
			t.Errorf("D3: %d va y vuelve como %d", p, got)
		}
	}
}
