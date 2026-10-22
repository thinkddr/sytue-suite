// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// servidorAvisos hace de GNOME Shell o de dunst: apunta lo que le piden y devuelve el
// número del aviso.
//
// Con cerrojo: la conexión del servidor lo llama desde su rutina, y el viaje por el bus
// no es una sincronización que el detector de carreras vea.
type servidorAvisos struct {
	mu       sync.Mutex
	acciones []string
	hints    map[string]dbus.Variant
}

func (s *servidorAvisos) Notify(_ string, _ uint32, _, _, _ string, acciones []string,
	hints map[string]dbus.Variant, _ int32,
) (uint32, *dbus.Error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acciones, s.hints = acciones, hints
	return 7, nil
}

func (s *servidorAvisos) pedido() ([]string, map[string]dbus.Variant) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acciones, s.hints
}

// TestPulsarElAvisoLlevaADondeDice es el guardián del clic en Linux: el aviso pide la
// acción «default» —sin ella el servidor no avisa del clic— y, cuando llega
// `ActionInvoked` con su número, se hace lo que el aviso dice; un aviso cerrado sin
// pulsar no hace nada. VISTO EN ROJO sin «default» en la lista y sin la rutina que
// escucha.
func TestPulsarElAvisoLlevaADondeDice(t *testing.T) {
	servidor := busPropio(t)
	falso := &servidorAvisos{}
	if err := servidor.Export(falso, avisosRuta, avisosNombre); err != nil {
		t.Fatal(err)
	}
	if _, err := servidor.RequestName(avisosNombre, 0); err != nil {
		t.Fatal(err)
	}
	app, err := dbus.Connect(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	pulsado := make(chan struct{}, 2)
	if err := avisarEn(app, "eu.sytue.mail", "Sytue Mail", "Correo nuevo", "De Ada", func() { pulsado <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	acciones, hints := falso.pedido()
	if !slices.Contains(acciones, "default") {
		t.Fatalf("el aviso pide las acciones %q: sin «default» pulsarlo no hace nada", acciones)
	}
	if e, _ := hints["desktop-entry"].Value().(string); e != "eu.sytue.mail" {
		t.Errorf("desktop-entry = %q; el servidor saca de ahí el icono", e)
	}

	// Cerrado sin pulsar (motivo 2: lo descartó la persona): nada.
	if err := servidor.Emit(avisosRuta, avisosNombre+".NotificationClosed", uint32(7), uint32(2)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pulsado:
		t.Fatal("un aviso descartado ha abierto su destino")
	case <-time.After(200 * time.Millisecond):
	}

	// Otro, y esta vez se pulsa.
	if err := avisarEn(app, "eu.sytue.mail", "Sytue Mail", "Correo nuevo", "De Ada", func() { pulsado <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := servidor.Emit(avisosRuta, avisosNombre+".ActionInvoked", uint32(7), "default"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-pulsado:
	case <-time.After(2 * time.Second):
		t.Fatal("pulsar el aviso no ha llevado a ninguna parte")
	}
}
