// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

func TestRecomiendaLaMejorRutaSana(t *testing.T) {
	// Con varias sanas gana la de mayor intención, no la primera que llegó.
	got := RecommendHealthPath([]HealthProbeResult{
		{Path: "/", Status: 200, Healthy: true},
		{Path: "/api/health", Status: 200, Healthy: true},
		{Path: "/healthz", Status: 200, Healthy: true},
	})
	if got != "/healthz" {
		t.Errorf("recomendó %q; /healthz es la que el programador escribió para esto", got)
	}
}

func TestLaRaizSoloSiNoHayNadaMejor(t *testing.T) {
	got := RecommendHealthPath([]HealthProbeResult{
		{Path: "/healthz", Status: 404},
		{Path: "/health", Status: 404},
		{Path: "/", Status: 200, Healthy: true},
	})
	if got != "/" {
		t.Errorf("recomendó %q; con solo la raíz sana hay que proponerla", got)
	}
	if !IsWeakHealthPath(got) {
		t.Error("la raíz tiene que marcarse como señal débil: responde aunque la app esté rota por dentro")
	}
	if IsWeakHealthPath("/healthz") {
		t.Error("/healthz no es una señal débil")
	}
}

// EL CASO QUE PROTEGE LA APP: si no responde nada, callarse. Proponer una ruta que
// la app no sirve haría que la sonda la matara en bucle.
func TestSinNingunaSanaNoSeInventaNada(t *testing.T) {
	if got := RecommendHealthPath([]HealthProbeResult{
		{Path: "/healthz", Status: 404},
		{Path: "/", Status: 0},
	}); got != "" {
		t.Errorf("recomendó %q sin ninguna ruta sana: eso reiniciaría la app en bucle", got)
	}
	if got := RecommendHealthPath(nil); got != "" {
		t.Errorf("sin resultados no puede haber recomendación, salió %q", got)
	}
}

// Una respuesta que no está en el catálogo no puede colarse como recomendación:
// solo se propone lo que se sondeó a propósito.
func TestSoloSeRecomiendaDelCatalogo(t *testing.T) {
	if got := RecommendHealthPath([]HealthProbeResult{
		{Path: "/algo-raro", Status: 200, Healthy: true},
	}); got != "" {
		t.Errorf("recomendó %q, que no es del catálogo", got)
	}
}

func TestCatalogoOrdenadoPorIntencion(t *testing.T) {
	if len(HealthPathCandidates) == 0 {
		t.Fatal("el catálogo no puede estar vacío")
	}
	if HealthPathCandidates[0] != "/healthz" {
		t.Errorf("la primera preferencia debe ser /healthz, es %q", HealthPathCandidates[0])
	}
	if last := HealthPathCandidates[len(HealthPathCandidates)-1]; last != "/" {
		t.Errorf("la raíz debe ir la ÚLTIMA por ser la peor señal, va %q", last)
	}
	seen := map[string]bool{}
	for _, c := range HealthPathCandidates {
		if seen[c] {
			t.Errorf("candidato duplicado %q: sondearlo dos veces es una petición de más a la app", c)
		}
		seen[c] = true
		if err := ValidateHealthPath(c); err != nil {
			t.Errorf("candidato %q inválido para el propio campo: %v", c, err)
		}
	}
}
