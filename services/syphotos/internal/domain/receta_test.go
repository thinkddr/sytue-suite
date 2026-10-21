// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"errors"
	"testing"
)

// LA HUELLA SOSTIENE LA CACHÉ DE DERIVADAS: con ella se nombran los ficheros
// generados. Si dos recetas iguales dan huellas distintas, se recalcula y se
// guarda dos veces lo mismo; si dos recetas DISTINTAS dan la misma, se sirve una
// foto por otra — que es el fallo grave, y silencioso.
func TestHuella_estableEntreEjecuciones(t *testing.T) {
	r := Receta{Version: 1, Pasos: []Paso{
		{Tipo: PasoExposicion, Params: map[string]float64{"valor": 0.5}},
		{Tipo: PasoContraste, Params: map[string]float64{"valor": -0.25}},
	}}
	a, err := r.Huella()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := r.Huella()
	if a != b {
		t.Errorf("la misma receta dio %q y %q", a, b)
	}
	if len(a) != 12 {
		t.Errorf("huella de %d caracteres, se esperaban 12: %q", len(a), a)
	}
}

// El orden de los pasos ES la receta: recortar y luego girar no da lo mismo que
// girar y luego recortar, así que no pueden compartir huella.
func TestHuella_elOrdenCuenta(t *testing.T) {
	giro := Paso{Tipo: PasoGiro, Params: map[string]float64{"grados": 90}}
	recorte := Paso{Tipo: PasoRecorte, Params: map[string]float64{
		"x": 0, "y": 0, "ancho": 0.5, "alto": 0.5,
	}}

	uno, _ := Receta{Version: 1, Pasos: []Paso{giro, recorte}}.Huella()
	otro, _ := Receta{Version: 1, Pasos: []Paso{recorte, giro}}.Huella()
	if uno == otro {
		t.Error("dos órdenes distintos dieron la misma huella: se serviría una foto por otra")
	}
}

// El ruido del último bit de un flotante no puede provocar un recálculo: los
// deslizadores de la interfaz no tienen esa resolución, y sin normalizar cada
// arrastre generaría una derivada nueva.
func TestHuella_normalizaElRuidoDeLosFlotantes(t *testing.T) {
	a, _ := Receta{Version: 1, Pasos: []Paso{
		{Tipo: PasoExposicion, Params: map[string]float64{"valor": 0.5}},
	}}.Huella()
	b, _ := Receta{Version: 1, Pasos: []Paso{
		{Tipo: PasoExposicion, Params: map[string]float64{"valor": 0.50000000000000001}},
	}}.Huella()
	if a != b {
		t.Errorf("0.5 y 0.50000000000000001 dieron huellas distintas (%q vs %q)", a, b)
	}
	// Pero una diferencia REAL sí tiene que verse.
	c, _ := Receta{Version: 1, Pasos: []Paso{
		{Tipo: PasoExposicion, Params: map[string]float64{"valor": 0.51}},
	}}.Huella()
	if a == c {
		t.Error("0.5 y 0.51 comparten huella: un ajuste visible no se recalcularía")
	}
}

// La receta vacía no es un caso especial: el original entra por el mismo camino
// que cualquier edición, así que el almacén no necesita una rama aparte.
func TestRecetaVacia_esValidaYTieneHuella(t *testing.T) {
	r := RecetaVacia()
	if err := r.Valida(); err != nil {
		t.Fatalf("la receta vacía no valida: %v", err)
	}
	h, err := r.Huella()
	if err != nil || h == "" {
		t.Errorf("la receta vacía tiene que tener huella; salió %q, %v", h, err)
	}
}

// LO QUE HAY QUE RECHAZAR. Cliente y servidor interpretarían distinto un valor
// que no se entiende, y ahí es donde lo exportado deja de parecerse a lo visto.
func TestValida_rechazaLoQueNoSePuedeInterpretar(t *testing.T) {
	casos := []struct {
		nombre string
		receta Receta
	}{
		{"paso desconocido", Receta{Version: 1, Pasos: []Paso{{Tipo: "psicodelia"}}}},
		{"parámetro fuera de rango", Receta{Version: 1, Pasos: []Paso{
			{Tipo: PasoExposicion, Params: map[string]float64{"valor": 42}},
		}}},
		{"parámetro que no existe", Receta{Version: 1, Pasos: []Paso{
			{Tipo: PasoExposicion, Params: map[string]float64{"valor": 0, "gamma": 1}},
		}}},
		{"falta un parámetro", Receta{Version: 1, Pasos: []Paso{{Tipo: PasoExposicion}}}},
		{"espejo sin eje", Receta{Version: 1, Pasos: []Paso{{Tipo: PasoEspejo}}}},
		{"recorte que se sale", Receta{Version: 1, Pasos: []Paso{
			{Tipo: PasoRecorte, Params: map[string]float64{
				"x": 0.8, "y": 0, "ancho": 0.5, "alto": 0.5,
			}},
		}}},
		{"recorte de lado cero", Receta{Version: 1, Pasos: []Paso{
			{Tipo: PasoRecorte, Params: map[string]float64{
				"x": 0, "y": 0, "ancho": 0, "alto": 0.5,
			}},
		}}},
		{"versión del futuro", Receta{Version: VersionReceta + 1}},
		{"versión cero", Receta{Version: 0}},
	}
	for _, c := range casos {
		if err := c.receta.Valida(); err == nil {
			t.Errorf("%s: se aceptó y no debería", c.nombre)
		} else if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%s: error %v; se esperaba que envolviera ErrInvalidInput", c.nombre, err)
		}
	}
}

// NaN e infinito no son «fuera de rango»: toda comparación con NaN es falsa, así
// que sin comprobarlos aparte se colarían hasta el motor de píxeles.
func TestValida_niNaNniInfinito(t *testing.T) {
	nan := Receta{Version: 1, Pasos: []Paso{
		{Tipo: PasoExposicion, Params: map[string]float64{"valor": nanFloat()}},
	}}
	if err := nan.Valida(); err == nil {
		t.Error("NaN se coló por la comprobación de rango")
	}
	inf := Receta{Version: 1, Pasos: []Paso{
		{Tipo: PasoContraste, Params: map[string]float64{"valor": infFloat()}},
	}}
	if err := inf.Valida(); err == nil {
		t.Error("infinito se coló por la comprobación de rango")
	}
}

func nanFloat() float64 { z := 0.0; return z / z }
func infFloat() float64 { z := 0.0; return 1 / z }

// Una receta la escribe la interfaz paso a paso: mil pasos significa que algo
// está en bucle, y sin tope ese bucle se guarda y se reproduce en cada render.
func TestValida_topeDePasos(t *testing.T) {
	pasos := make([]Paso, 257)
	for i := range pasos {
		pasos[i] = Paso{Tipo: PasoContraste, Params: map[string]float64{"valor": 0}}
	}
	if err := (Receta{Version: 1, Pasos: pasos}).Valida(); err == nil {
		t.Error("se aceptaron 257 pasos")
	}
}

// Y lo que SÍ tiene que pasar, que es la otra mitad: una receta normal completa.
func TestValida_aceptaUnaEdicionCorriente(t *testing.T) {
	r := Receta{Version: VersionReceta, Pasos: []Paso{
		{Tipo: PasoRecorte, Params: map[string]float64{
			"x": 0.1, "y": 0.1, "ancho": 0.8, "alto": 0.8,
		}},
		{Tipo: PasoGiro, Params: map[string]float64{"grados": 90}},
		{Tipo: PasoEspejo, Eje: "h"},
		{Tipo: PasoExposicion, Params: map[string]float64{"valor": -0.3}},
		{Tipo: PasoSaturacion, Params: map[string]float64{"valor": 0.2}},
		{Tipo: PasoTemperatura, Params: map[string]float64{"valor": 0.15}},
	}}
	if err := r.Valida(); err != nil {
		t.Errorf("una edición corriente fue rechazada: %v", err)
	}
}
