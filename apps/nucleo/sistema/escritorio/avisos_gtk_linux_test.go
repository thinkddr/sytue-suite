// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// gnomeFalso hace de GNOME Shell: apunta el aviso que le ponen.
type gnomeFalso struct {
	mu    sync.Mutex
	app   string
	aviso map[string]dbus.Variant
}

func (g *gnomeFalso) AddNotification(app, _ string, aviso map[string]dbus.Variant) *dbus.Error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.app, g.aviso = app, aviso
	return nil
}

func (g *gnomeFalso) puesto() (string, map[string]dbus.Variant) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.app, g.aviso
}

// TestEnGNOMEElAvisoVuelveALaAppYNoAlProceso: con `org.gtk.Notifications` el aviso lleva
// la acción `app.aviso` con su enlace, y pulsarlo es que GNOME llame a ActivateAction en
// el NOMBRE de la app: llega a quien se lo haya quedado, sea el proceso que avisó o uno
// que D-Bus acaba de arrancar. Y el `.service` que D-Bus usa para arrancarla lleva el
// ejecutable citado y el argumento de la bandeja. VISTO EN ROJO sin quedarse el nombre
// de bus y con el enlace fuera del `default-action-target`.
func TestEnGNOMEElAvisoVuelveALaAppYNoAlProceso(t *testing.T) {
	shell := busPropio(t)
	g := &gnomeFalso{}
	if err := shell.Export(g, gtkRuta, gtkNombre); err != nil {
		t.Fatal(err)
	}
	if _, err := shell.RequestName(gtkNombre, 0); err != nil {
		t.Fatal(err)
	}
	app, err := dbus.Connect(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if !hayGtk(app) {
		t.Fatal("no se ve org.gtk.Notifications")
	}
	pulsado := make(chan struct{}, 1)
	const id = "eu.sytue.pruebas"
	if err := avisarGtk(app, Aviso{
		ID: id, Esquema: "sytue-pruebas", Titulo: "Correo nuevo", Cuerpo: "De Ada",
		Destino:  Destino{Aviso: "a1", Origen: "mail", Ruta: "/mail/inbox"},
		AlPulsar: func() { pulsado <- struct{}{} },
	}); err != nil {
		t.Fatal(err)
	}
	quien, aviso := g.puesto()
	if quien != id || aviso["default-action"].Value() != "app."+accionAviso {
		t.Fatalf("aviso de %q con acción %v", quien, aviso["default-action"])
	}
	enlace, _ := aviso["default-action-target"].Value().(string)
	if !strings.HasPrefix(enlace, "sytue-pruebas:/_aviso?") || !strings.Contains(enlace, "origen=mail") {
		t.Fatalf("el enlace del aviso es %q", enlace)
	}

	// GNOME «pulsa»: llama al NOMBRE de la app, no a la conexión que puso el aviso.
	if err := shell.Object(id, rutaDeApp(id)).Call(appInterfaz+".ActivateAction", 0,
		accionAviso, []dbus.Variant{dbus.MakeVariant(enlace)}, map[string]dbus.Variant{}).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-pulsado:
	case <-time.After(2 * time.Second):
		t.Fatal("pulsar el aviso en GNOME no ha llevado a ninguna parte")
	}

	casa := t.TempDir()
	t.Setenv("XDG_DATA_HOME", casa)
	if cambiado, err := ponerServicio(id, `/opt/Sytue "One"/sytue-hub`); err != nil || !cambiado {
		t.Fatalf("ponerServicio: %v, %v", cambiado, err)
	}
	datos, _ := os.ReadFile(filepath.Join(casa, "dbus-1", "services", id+".service"))
	if string(datos) != "[D-BUS Service]\nName="+id+"\nExec=\"/opt/Sytue \\\"One\\\"/sytue-hub\" --bandeja\n" {
		t.Errorf("el .service es\n%s", datos)
	}
	if cambiado, _ := ponerServicio(id, `/opt/Sytue "One"/sytue-hub`); cambiado {
		t.Error("el mismo .service se reescribe en cada arranque")
	}
}
