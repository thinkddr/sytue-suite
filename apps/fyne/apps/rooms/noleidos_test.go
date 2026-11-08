// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package rooms

import (
	"context"
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestTotalNoLeidos es el gemelo de lib/totalNoLeidos.test.ts: suman todos los canales
// menos los silenciados.
func TestTotalNoLeidos(t *testing.T) {
	cs := []Canal{
		{ID: "a", SinLeer: 3},
		{ID: "dm", Clase: Directo, SinLeer: 2},
		{ID: "ruido", SinLeer: 40, Silenciado: true, Nivel: AvisarNada},
		{ID: "menciones", SinLeer: 1, Nivel: AvisarMenciones},
		{ID: "leido"},
	}
	if n := TotalNoLeidos(cs); n != 6 {
		t.Errorf("el total es %d y son 6", n)
	}
	if n := TotalNoLeidos(nil); n != 0 {
		t.Errorf("sin canales el total es %d", n)
	}
}

// TestElIconoLlevaLosNoLeidos: la sala dice el total al arrancar —sin el canal abierto,
// que se está leyendo— y otra vez cuando cambia al leer el resto, pero no lo repite si
// no cambia.
func TestElIconoLlevaLosNoLeidos(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	var dichos []int
	sa.AlCambiarNoLeidos = func(n int) { dichos = append(dichos, n) }
	sa.Arrancar(context.Background())
	if len(dichos) == 0 || dichos[len(dichos)-1] != 4 {
		t.Fatalf("al arrancar el icono dice %v; quiero 4 (los del directo)", dichos)
	}
	antes := len(dichos)
	sa.pintarCanales()
	if len(dichos) != antes {
		t.Errorf("repintar sin cambios volvió a decirlo: %v", dichos)
	}
	for _, c := range sa.P.Lateral.Canales {
		if c.ID == "d1" {
			sa.Elegir(c)
		}
	}
	if dichos[len(dichos)-1] != 0 {
		t.Errorf("leído el directo el icono dice %v; quiero 0", dichos)
	}
}

// TestElIconoCuentaElCanalAbiertoEnSegundoPlano es el gemelo del caso de
// totalNoLeidos.test.ts: con la app en segundo plano, lo que llega al canal abierto no
// lo lee nadie y suma al icono; lo propio no; al volver, se quita.
func TestElIconoCuentaElCanalAbiertoEnSegundoPlano(t *testing.T) {
	s := servicioConDos()
	sa, _ := salaDePrueba(t, s)
	var dichos []int
	sa.AlCambiarNoLeidos = func(n int) { dichos = append(dichos, n) }
	sa.Arrancar(context.Background())
	base := dichos[len(dichos)-1]
	llega := func(id, autor string) {
		carga, _ := json.Marshal(api.RoomsMessage{ID: id, ChannelID: sa.abierto, AuthorSub: autor, Body: "hola"})
		sa.alLlegar(api.RoomsEvent{Type: "message", ChannelID: sa.abierto, Subject: autor, Payload: carga})
	}

	llega("m0", "otra") // a la vista: se está leyendo
	if got := dichos[len(dichos)-1]; got != base {
		t.Fatalf("con la app delante el icono pasó de %d a %d", base, got)
	}
	sa.EnSegundoPlano()
	llega("m1", "otra")
	llega("m2", "otra")
	llega("m2", "otra") // la misma trama otra vez (reconexión): no suma
	llega("m3", sa.Yo)
	if got := dichos[len(dichos)-1]; got != base+2 {
		t.Fatalf("en segundo plano el icono dice %d; quiero %d", got, base+2)
	}
	sa.EnPrimerPlano()
	if got := dichos[len(dichos)-1]; got != base {
		t.Errorf("al volver el icono dice %d; quiero %d", got, base)
	}
}

// TestSumaNoVisto es el gemelo de `sumaNoVisto`.
func TestSumaNoVisto(t *testing.T) {
	raiz := "r"
	ajeno := api.RoomsMessage{AuthorSub: "otra"}
	if !SumaNoVisto(true, ajeno, "yo", Canal{}) {
		t.Error("en segundo plano un mensaje ajeno no suma")
	}
	if SumaNoVisto(false, ajeno, "yo", Canal{}) ||
		SumaNoVisto(true, api.RoomsMessage{AuthorSub: "yo"}, "yo", Canal{}) ||
		SumaNoVisto(true, api.RoomsMessage{AuthorSub: "otra", ParentID: &raiz}, "yo", Canal{}) ||
		SumaNoVisto(true, ajeno, "yo", Canal{Silenciado: true}) ||
		SumaNoVisto(true, ajeno, "yo", Canal{Nivel: AvisarNada}) {
		t.Error("suman los que no deben: a la vista, propios, de hilo, silenciados o con avisos a nada")
	}
}
