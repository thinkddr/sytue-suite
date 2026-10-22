// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/internal/busprueba"
)

// busPropio es [busprueba.Nuevo]: un bus de sesión solo para el test.
func busPropio(t *testing.T) *dbus.Conn { t.Helper(); return busprueba.Nuevo(t) }

// TestLaInsigniaLlegaAlLanzador es el guardián del número en Linux: la señal tiene que
// nombrar el .desktop de la app y llevar la cuenta, y con 0 tiene que apagarse, no
// enseñar un «0». Se ha visto en rojo nombrando la app sin «.desktop» y con
// count-visible siempre a true.
func TestLaInsigniaLlegaAlLanzador(t *testing.T) {
	lanzador := busPropio(t)
	if err := lanzador.AddMatchSignal(dbus.WithMatchInterface("com.canonical.Unity.LauncherEntry")); err != nil {
		t.Fatal(err)
	}
	senales := make(chan *dbus.Signal, 4)
	lanzador.Signal(senales)

	for _, c := range []struct {
		n       int
		visible bool
	}{{7, true}, {0, false}} {
		if err := Insignia("eu.sytue.mail", c.n); err != nil {
			t.Fatal(err)
		}
		select {
		case s := <-senales:
			if app, _ := s.Body[0].(string); app != "application://eu.sytue.mail.desktop" {
				t.Errorf("la señal nombra %q; el lanzador busca el .desktop", app)
			}
			props, _ := s.Body[1].(map[string]dbus.Variant)
			if n, _ := props["count"].Value().(int64); n != int64(c.n) {
				t.Errorf("count = %d; quiero %d", n, c.n)
			}
			if v, _ := props["count-visible"].Value().(bool); v != c.visible {
				t.Errorf("con %d, count-visible = %v", c.n, v)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("el lanzador no recibió ninguna señal")
		}
	}
}

// TestSinQuienPinteLaBandejaNoHayBandeja: en un GNOME sin la extensión de la bandeja,
// cerrar la ventana del Hub tiene que cerrarlo, no dejarlo corriendo invisible. Se ha
// visto en rojo devolviendo true sin mirar la respuesta.
func TestSinQuienPinteLaBandejaNoHayBandeja(t *testing.T) {
	escritorio := busPropio(t)
	app, err := dbus.Connect(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if hayBandejaEn(app) {
		t.Fatal("sin StatusNotifierWatcher en el bus no hay bandeja")
	}
	if _, err := escritorio.RequestName("org.kde.StatusNotifierWatcher", 0); err != nil {
		t.Fatal(err)
	}
	if !hayBandejaEn(app) {
		t.Fatal("con el StatusNotifierWatcher en el bus sí hay bandeja")
	}
}
