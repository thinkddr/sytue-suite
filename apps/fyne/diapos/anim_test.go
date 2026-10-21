// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package diapos

import (
	"slices"
	"testing"
)

func cajaAnim(id string, anims ...Anim) Caja { return Caja{ID: id, W: 10, H: 10, Anims: anims} }

// TestElPlanEsElDeLaWeb repite los vectores de `test/slides-anim.test.ts`: los mismos
// pasos, las mismas categorías y los mismos retardos. Si la web cambia de criterio,
// esto se queda atrás a propósito y alguien tiene que mirar las dos.
func TestElPlanEsElDeLaWeb(t *testing.T) {
	d := Diapo{ID: "s", Cajas: []Caja{cajaAnim("a",
		Anim{Tipo: "fade", Orden: 1},
		Anim{Tipo: "pulse", Orden: 2, Disparo: "with"},
		Anim{Tipo: "fade-out", Orden: 3, Disparo: "after"},
		Anim{Tipo: "zoom", Orden: 4, Disparo: "click"},
	)}}
	plan := Plan(d)
	pasos, retardos := make([]int, 0, len(plan)), make([]int, 0, len(plan))
	cats := make([]Categoria, 0, len(plan))
	for _, p := range plan {
		pasos, retardos, cats = append(pasos, p.Paso), append(retardos, p.RetardoMS), append(cats, p.Categoria)
	}
	if !slices.Equal(pasos, []int{1, 1, 1, 2}) || !slices.Equal(retardos, []int{0, 0, 260, 0}) ||
		!slices.Equal(cats, []Categoria{Entrada, Enfasis, Salida, Entrada}) {
		t.Fatalf("plan distinto del de la web: pasos %v, retardos %v, categorías %v", pasos, retardos, cats)
	}
	if Pasos(d) != 2 {
		t.Errorf("Pasos = %d, la web dice 2", Pasos(d))
	}
	// La primera abre paso aunque diga «after».
	solo := Diapo{Cajas: []Caja{cajaAnim("a", Anim{Tipo: "fade", Orden: 1, Disparo: "after"})}}
	if Pasos(solo) != 1 {
		t.Errorf("la primera animación tiene que abrir el paso 1")
	}
}

// TestLaPresenciaEsLaDeLaWeb: entrada revela, salida oculta, énfasis no cambia nada, y
// una caja sin entrada se ve desde el principio.
//
// VISTO EN ROJO (2026-09-22, en el frontal): arrancando `presente` en true siempre, la
// caja con entrada salía en el paso 0 y el test lo decía. Deshecho.
func TestLaPresenciaEsLaDeLaWeb(t *testing.T) {
	d := Diapo{Cajas: []Caja{
		cajaAnim("a", Anim{Tipo: "fade", Orden: 1}, Anim{Tipo: "fade-out", Orden: 2, Disparo: "click"}),
		cajaAnim("b", Anim{Tipo: "pulse", Orden: 3}),
		{ID: "c"},
	}}
	por := PorCaja(Plan(d))
	for paso, quiere := range []bool{false, true, false} {
		if got := PresenteEn(por["a"], paso); got != quiere {
			t.Errorf("caja con entrada y salida en el paso %d: presente %v, la web %v", paso, got, quiere)
		}
	}
	if !PresenteEn(por["b"], 0) || !PresenteEn(por["c"], 0) {
		t.Error("una caja sin entrada tiene que verse desde el paso 0")
	}
}

// TestAvanzarRecorreLosPasosYLasDiapositivas es la máquina de `presentAdvance`.
func TestAvanzarRecorreLosPasosYLasDiapositivas(t *testing.T) {
	diapos := []Diapo{
		{Cajas: []Caja{cajaAnim("a", Anim{Tipo: "fade", Orden: 1}), cajaAnim("b", Anim{Tipo: "fade", Orden: 2})}},
		{ID: "sin anims"},
	}
	type pos struct{ d, p int }
	camino := make([]pos, 1, 5)
	d, p := 0, 0
	for range 4 {
		d, p = Avanzar(diapos, d, p, 1)
		camino = append(camino, pos{d, p})
	}
	quiere := []pos{{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 0}}
	if !slices.Equal(camino, quiere) {
		t.Fatalf("avanzando: %v, la web %v", camino, quiere)
	}
	// Retroceder desde la segunda vuelve a la primera YA construida.
	if d, p := Avanzar(diapos, 1, 0, -1); d != 0 || p != 2 {
		t.Errorf("retroceder desde (1,0) da (%d,%d), la web (0,2)", d, p)
	}
	if d, p := Avanzar(diapos, 0, 0, -1); d != 0 || p != 0 {
		t.Errorf("retroceder desde el principio tiene que quedarse en (0,0), no (%d,%d)", d, p)
	}
}

// TestNormalizarRenumeraYMigra: órdenes 1..N en toda la diapositiva, el `anim` viejo
// pasa a `anims` y una caja sin animaciones se queda sin los dos campos.
func TestNormalizarRenumeraYMigra(t *testing.T) {
	vieja := Anim{Tipo: "fly", Orden: 7}
	cajas := NormalizarOrden([]Caja{
		{ID: "a", Anims: []Anim{{Tipo: "fade", Orden: 10}}},
		{ID: "b", Anim: &vieja},
		{ID: "c", Anims: []Anim{{Tipo: "inventado", Orden: 1}}},
	})
	if cajas[0].Anims[0].Orden != 2 || cajas[1].Anims[0].Orden != 1 || cajas[1].Anim != nil {
		t.Errorf("no renumera ni migra como la web: %+v %+v", cajas[0].Anims, cajas[1])
	}
	if cajas[2].Anims != nil {
		t.Errorf("un efecto que no existe se descarta, y la caja queda sin campo: %+v", cajas[2].Anims)
	}
}
