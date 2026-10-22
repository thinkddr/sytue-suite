// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// portalAtajos hace de xdg-desktop-portal con GlobalShortcuts.
type portalAtajos struct {
	conn     *dbus.Conn
	mu       sync.Mutex
	app      string
	sesion   dbus.ObjectPath
	atado    string // el `preferred_trigger` que llegó
	pedidos  int
	registro bool
}

func (p *portalAtajos) pedido(remitente dbus.Sender, datos map[string]dbus.Variant) dbus.ObjectPath {
	p.mu.Lock()
	p.pedidos++
	n := p.pedidos
	p.mu.Unlock()
	quien := strings.ReplaceAll(strings.TrimPrefix(string(remitente), ":"), ".", "_")
	ruta := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + quien + "/p" + string(rune('0'+n)))
	go func() {
		time.Sleep(20 * time.Millisecond)
		_ = p.conn.Emit(ruta, "org.freedesktop.portal.Request.Response", uint32(0), datos)
	}()
	return ruta
}

func (p *portalAtajos) Register(app string, _ map[string]dbus.Variant) *dbus.Error {
	p.mu.Lock()
	p.app, p.registro = app, true
	p.mu.Unlock()
	return nil
}

func (p *portalAtajos) CreateSession(remitente dbus.Sender, _ map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	p.sesion = "/org/freedesktop/portal/desktop/session/1/s1"
	s := p.sesion
	p.mu.Unlock()
	// Como string, que es como lo mandan los portales de la primera versión.
	return p.pedido(remitente, map[string]dbus.Variant{"session_handle": dbus.MakeVariant(string(s))}), nil
}

func (p *portalAtajos) BindShortcuts(remitente dbus.Sender, _ dbus.ObjectPath, atajos []struct {
	ID       string
	Opciones map[string]dbus.Variant
}, _ string, _ map[string]dbus.Variant,
) (dbus.ObjectPath, *dbus.Error) {
	p.mu.Lock()
	if len(atajos) == 1 && atajos[0].ID == atajoID {
		p.atado, _ = atajos[0].Opciones["preferred_trigger"].Value().(string)
	}
	p.mu.Unlock()
	return p.pedido(remitente, map[string]dbus.Variant{}), nil
}

// TestElAtajoSePideAlPortalYLlega es el guardián del atajo global en Linux: la app se
// registra con su identificador, pide Ctrl+Alt+Espacio y, cuando el portal manda
// `Activated` con su atajo, se llama a quien lo pidió; otro atajo no. VISTO EN ROJO sin
// la rutina de `Activated` y con el modificador en el formato de Gtk (`<Control>`).
func TestElAtajoSePideAlPortalYLlega(t *testing.T) {
	escritorio := busPropio(t)
	p := &portalAtajos{conn: escritorio}
	if err := escritorio.Export(p, portalRuta, atajosIface); err != nil {
		t.Fatal(err)
	}
	if err := escritorio.Export(p, portalRuta, "org.freedesktop.host.portal.Registry"); err != nil {
		t.Fatal(err)
	}
	if _, err := escritorio.RequestName(portalNombre, 0); err != nil {
		t.Fatal(err)
	}

	pulsado := make(chan struct{}, 2)
	soltar, err := atajo("eu.sytue.one", "Buscar en Sytue One", CombinacionDe("linux"), func() { pulsado <- struct{}{} })
	if err != nil {
		t.Fatalf("Atajo: %v", err)
	}
	defer soltar()

	p.mu.Lock()
	app, atado, sesion := p.app, p.atado, p.sesion
	p.mu.Unlock()
	if app != "eu.sytue.one" {
		t.Errorf("la app se registró como %q", app)
	}
	if atado != "CTRL+ALT+space" {
		t.Errorf("el portal recibió %q; el formato de XDG es CTRL+ALT+space", atado)
	}

	activar := func(id string) {
		if err := escritorio.Emit(portalRuta, atajosIface+".Activated", sesion, id, uint64(1), map[string]dbus.Variant{}); err != nil {
			t.Fatal(err)
		}
	}
	activar("otro")
	select {
	case <-pulsado:
		t.Fatal("un atajo que no es el de la suite ha abierto el buscador")
	case <-time.After(200 * time.Millisecond):
	}
	activar(atajoID)
	select {
	case <-pulsado:
	case <-time.After(2 * time.Second):
		t.Fatal("pulsar el atajo no ha llegado a la app")
	}
}

// TestLaCombinacionDeCadaSistema: la de macOS no puede ser ninguna de las cuatro que
// el sistema trae puestas con Espacio.
func TestLaCombinacionDeCadaSistema(t *testing.T) {
	if got := CombinacionDe("windows").Portal(); got != "CTRL+ALT+space" {
		t.Errorf("Windows: %q", got)
	}
	mac := CombinacionDe("darwin")
	for _, ocupada := range []Combinacion{
		{Cmd: true, Tecla: "space"},
		{Ctrl: true, Tecla: "space"},
		{Ctrl: true, Alt: true, Tecla: "space"},
		{Alt: true, Cmd: true, Tecla: "space"},
	} {
		if mac == ocupada {
			t.Errorf("en macOS %+v ya la usa el sistema", ocupada)
		}
	}
}
