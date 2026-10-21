// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package editor

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"git.sytue.eu/sytue/one/apps/fyne/peticion"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// La franja del editor tiene salida (petición del dueño, 2026-09-25): quien no tiene
// permiso ve «Pedir acceso de edición»; enviada, «Petición enviada»; y el dueño ve quién
// se lo pide con «Dar acceso» y «Rechazar». Un documento que el lienzo no entendió no
// ofrece pedir nada: el permiso no lo arreglaría.
func TestLaFranjaDelEditorOfreceSalida(t *testing.T) {
	marco := func(c *Contexto) *Marco {
		m := NuevoMarco(c, Genericas(), nil)
		m.Resize(fyne.NewSize(1200, 800))
		return m
	}
	pedir := func() {}
	if m := marco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, SoloLectura: true, Peticion: peticion.Estado{Pedir: pedir}}); m.franja == nil || m.franja.Accion == nil {
		t.Fatal("sin permiso, la franja no ofrece pedir acceso")
	}
	if m := marco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, SoloLectura: true, Peticion: peticion.Estado{Pedir: pedir, Pedida: true}}); m.franja.Nota == "" {
		t.Error("con la petición enviada, la franja no lo dice")
	}
	if m := marco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, SoloLectura: true, MotivoSoloLectura: "editor.readOnly"}); m.franja.Accion != nil {
		t.Error("un documento no entendido ofrece pedir permiso, que no lo arregla")
	}
	m := marco(&Contexto{Tipo: Documento, Estado: Listo, Zoom: 100, Peticion: peticion.Estado{Quien: "Ana", Conceder: pedir, Rechazar: pedir}})
	if m.franja == nil || m.franja.Accion == nil || m.franja.Secundaria == nil {
		t.Error("el dueño no ve quién le pide editar, o no puede concederlo")
	}
}

// El desbloqueo en vivo: el canal manda `role` y el editor se vuelve a abrir, sin que
// nadie recargue. Un `role` que sigue sin dejar escribir no reabre nada.
func TestElRolNuevoReabreElDocumento(t *testing.T) {
	sigue := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = c.CloseNow() }()
		var hola map[string]any
		_ = wsjson.Read(r.Context(), c, &hola)
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "role", "role": "commenter"})
		<-sigue
		_ = wsjson.Write(r.Context(), c, map[string]any{"type": "role", "role": "editor"})
		<-r.Context().Done()
	}))
	defer srv.Close()
	reabierto := 0
	escucharRol(api.NewDocs(srv.URL, nil, srv.Client()), "d1", func() { reabierto++ })
	defer DejarDeEscucharRol()
	// El `fyne.Do` de la rutina del canal lo ejecuta el driver de pruebas en el siguiente
	// punto de sincronía del test: estos `DoAndWait` vacíos.
	esperar := func(d time.Duration) {
		for fin := time.Now().Add(d); reabierto == 0 && time.Now().Before(fin); time.Sleep(10 * time.Millisecond) {
			fyne.DoAndWait(func() {})
		}
	}
	esperar(300 * time.Millisecond)
	if reabierto != 0 {
		t.Fatal("se reabrió con el rol de commenter, que no deja escribir")
	}
	close(sigue)
	esperar(5 * time.Second)
	if reabierto != 1 {
		t.Fatalf("llegó el rol de editor y el documento se reabrió %d veces", reabierto)
	}
}
