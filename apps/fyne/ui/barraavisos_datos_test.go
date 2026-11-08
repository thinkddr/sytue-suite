// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

import (
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// TestLaSenalDeCambiosLlegaAlWidget: un evento creado en la web manda `datos` por el
// flujo de la campana, y la barra se lo pasa al widget de la app (Suite.AvisoDelHub,
// que filtra por producto: lo fija `sesion`). Sin producto no pasa nada, y no entra en
// la campana. VISTO EN ROJO sin el `case` de EventoDatos en alEstado (no llega nada) y
// sin `a.alDatos = s.AvisoDelHub` en nuevoAvisosBarra.
func TestLaSenalDeCambiosLlegaAlWidget(t *testing.T) {
	hub := &hubDeMentira{enviar: make(chan string, 4), conexion: make(chan struct{}, 4)}
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)

	s := suiteConSesion(t)
	s.Hub = api.NewHub(srv.URL, sesionDePrueba{}, nil)
	a := nuevoAvisosBarra(traductorPrueba(t), s)
	if a.alDatos == nil {
		t.Fatal("la barra de una suite no le pasa la señal de cambios a su widget")
	}
	var mu sync.Mutex
	var llegan []string
	a.alDatos = func(p string) {
		mu.Lock()
		llegan = append(llegan, p)
		mu.Unlock()
	}
	t.Cleanup(a.parar)
	a.sesionCambio(s.Estado())
	esperarConexion(t, hub)

	hub.enviar <- "event: datos\ndata: {}"
	hub.enviar <- "event: datos\ndata: {\"producto\":\"calendar\"}"
	esperar(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(llegan) > 0 })
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(llegan, []string{"calendar"}) {
		t.Fatalf("al widget le llega %v, quiero [calendar]", llegan)
	}
	if n := sinLeerDe(a); n != 0 {
		t.Fatalf("la señal de cambios cuenta como aviso: %d sin leer", n)
	}
}
