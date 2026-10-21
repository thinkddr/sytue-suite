// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/syphotos/internal/domain"
)

// foto compone una entrada de la lista que agrupa `agruparParecidas`: una fecha
// de galería y un vector.
func foto(minuto int, v ...float32) conVector {
	t := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC).Add(time.Duration(minuto) * time.Minute)
	return conVector{a: domain.Activo{SubidaEn: t}, v: v}
}

// El caso que justifica la función: una ráfaga de tres casi idénticas.
func TestAgruparParecidas_juntaLaRafaga(t *testing.T) {
	g := agruparParecidas([]conVector{
		foto(0, 1, 0, 0),
		foto(-1, 1, 0.01, 0),
		foto(-2, 1, 0, 0.01),
	}, 50)
	if len(g) != 1 {
		t.Fatalf("grupos: %d, se esperaba 1", len(g))
	}
	if len(g[0].Fotos) != 3 {
		t.Fatalf("la ráfaga son 3 fotos, salieron %d", len(g[0].Fotos))
	}
}

// LA VENTANA ES LO QUE SEPARA UNA RÁFAGA DE UNA COINCIDENCIA. Dos fotos
// idénticas tomadas con horas de diferencia no son un duplicado: son la misma
// pared fotografiada dos días distintos.
func TestAgruparParecidas_fueraDeLaVentanaNoAgrupa(t *testing.T) {
	lejos := -int(VentanaDeRafaga/time.Minute) - 1
	g := agruparParecidas([]conVector{
		foto(0, 1, 0, 0),
		foto(lejos, 1, 0, 0),
	}, 50)
	if len(g) != 0 {
		t.Fatalf("no debería agrupar nada: %d grupos", len(g))
	}
}

// Parecidas de cerca, pero por debajo del umbral: no son la misma foto.
func TestAgruparParecidas_pordebajoDelUmbral(t *testing.T) {
	g := agruparParecidas([]conVector{
		foto(0, 1, 0, 0),
		foto(-1, 0, 1, 0),
	}, 50)
	if len(g) != 0 {
		t.Fatalf("vectores ortogonales no son duplicados: %d grupos", len(g))
	}
}

// Una foto sola nunca es un grupo: `len(grupo) > 1`.
func TestAgruparParecidas_unaSolaNoEsGrupo(t *testing.T) {
	if g := agruparParecidas([]conVector{foto(0, 1, 0, 0)}, 50); len(g) != 0 {
		t.Fatalf("grupos: %d", len(g))
	}
	if g := agruparParecidas(nil, 50); len(g) != 0 {
		t.Fatalf("una lista vacía no da grupos: %d", len(g))
	}
}

// Vectores de distinta longitud no se comparan: son de modelos distintos y el
// parecido no significaría nada.
func TestAgruparParecidas_distintaDimensionSeSalta(t *testing.T) {
	g := agruparParecidas([]conVector{
		foto(0, 1, 0, 0),
		foto(-1, 1, 0),
	}, 50)
	if len(g) != 0 {
		t.Fatalf("no se comparan modelos distintos: %d grupos", len(g))
	}
}

// El límite corta, y corta en grupos y no en fotas sueltas.
func TestAgruparParecidas_respetaElLimite(t *testing.T) {
	// Cuatro parejas, cada una en su propia ventana.
	lista := make([]conVector, 0, 8)
	for i := range 4 {
		base := -i * (int(VentanaDeRafaga/time.Minute) + 5)
		lista = append(lista, foto(base, 1, float32(i), 0), foto(base-1, 1, float32(i), 0))
	}
	if g := agruparParecidas(lista, 2); len(g) != 2 {
		t.Fatalf("el límite de 2 dejó %d grupos", len(g))
	}
}
