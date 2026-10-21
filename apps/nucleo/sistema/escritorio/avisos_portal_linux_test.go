// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// TestElIdDeLaAppSaleDeSuUnidad: las formas de unidad con las que Plasma, GNOME y
// systemd lanzan una app, y las que no son de ninguna. VISTO EN ROJO sin desescapar el
// `\x2d` y sin mirar la forma `.service`.
func TestElIdDeLaAppSaleDeSuUnidad(t *testing.T) {
	base := "12:pids:/\n0::/user.slice/user-1000.slice/user@1000.service/app.slice/"
	for unidad, id := range map[string]string{
		"app-eu.sytue.hub-1234.scope":                "eu.sytue.hub",
		"app-gnome-eu.sytue.hub-99.scope":            "eu.sytue.hub",
		"app-eu.sytue.hub@0f1e2d.service":            "eu.sytue.hub",
		"app-eu.sytue.hub-autostart.service":         "eu.sytue.hub",
		"app-eu.sytue.cert\\x2dpdf-77.scope":         "eu.sytue.cert-pdf",
		"dbus.service":                               "",
		"session-2.scope":                            "",
		"app-org.kde.konsole-5c1a.scope/tab-1.scope": "",
	} {
		if got := idDeUnidad(base + unidad + "\n"); got != id {
			t.Errorf("%s: id %q, se esperaba %q", unidad, got, id)
		}
	}
	if got := idDeUnidad("0::/\n"); got != "" {
		t.Errorf("sin unidad: id %q", got)
	}
}

// portalFalso hace de xdg-desktop-portal: el registro, la versión del portal de avisos
// (solo si `avisos`) y los avisos que le ponen.
type portalFalso struct {
	avisos bool
	mu     sync.Mutex
	id     string
	aviso  map[string]dbus.Variant
}

func (p *portalFalso) Register(string, map[string]dbus.Variant) *dbus.Error { return nil }

func (p *portalFalso) AddNotification(id string, aviso map[string]dbus.Variant) *dbus.Error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.id, p.aviso = id, aviso
	return nil
}

func (p *portalFalso) Get(iface, nombre string) (dbus.Variant, *dbus.Error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.avisos && iface == portalAvisos && nombre == "version" {
		return dbus.MakeVariant(uint32(2)), nil
	}
	return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", nil)
}

func (p *portalFalso) puesto() (string, map[string]dbus.Variant) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.id, p.aviso
}

// TestEnPlasmaElAvisoVaPorElPortalYVuelveALaApp es el guardián del clic con la app
// cerrada fuera de GNOME: con un portal que atiende avisos y que sabe de qué app es la
// conexión, el aviso lleva `app.aviso` con su enlace, y lo que hace plasmashell al
// pulsarlo —ActivateAction en el NOMBRE de la app— llega a su destino. Sin el portal de
// avisos, o sin que sepa de qué app es, va por freedesktop. VISTO EN ROJO sin
// registrarse en el portal y con el portal por delante de la comprobación del id.
func TestEnPlasmaElAvisoVaPorElPortalYVuelveALaApp(t *testing.T) {
	escritorio := busPropio(t)
	portal := &portalFalso{avisos: true}
	for _, iface := range []string{portalAvisos, portalRegistro, "org.freedesktop.DBus.Properties"} {
		if err := escritorio.Export(portal, portalRuta, iface); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := escritorio.RequestName(portalNombre, 0); err != nil {
		t.Fatal(err)
	}
	freedesktop := &servidorAvisos{}
	if err := escritorio.Export(freedesktop, avisosRuta, avisosNombre); err != nil {
		t.Fatal(err)
	}
	if _, err := escritorio.RequestName(avisosNombre, 0); err != nil {
		t.Fatal(err)
	}
	app, err := dbus.Connect(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()

	const id = "eu.sytue.pruebasportal"
	pulsado := make(chan struct{}, 1)
	aviso := Aviso{
		ID: id, Nombre: "Pruebas", Esquema: "sytue-pruebas", Titulo: "Correo nuevo", Cuerpo: "De Ada",
		Destino:  Destino{Aviso: "a1", Origen: "mail", Ruta: "/mail/inbox"},
		AlPulsar: func() { pulsado <- struct{}{} },
	}

	// Sin registrarse (y el test no corre en la unidad de ninguna app): freedesktop.
	if err := avisarPor(app, aviso); err != nil {
		t.Fatal(err)
	}
	if clave, _ := portal.puesto(); clave != "" {
		t.Fatal("el aviso ha ido al portal sin que el portal sepa de qué app es: el clic no volvería")
	}
	if acciones, _ := freedesktop.pedido(); len(acciones) == 0 {
		t.Fatal("sin portal que sepa de la app, el aviso no ha ido por freedesktop")
	}

	registrarEnPortal(app, id)
	if err := avisarPor(app, aviso); err != nil {
		t.Fatal(err)
	}
	clave, puesto := portal.puesto()
	if clave == "" || puesto["default-action"].Value() != "app."+accionAviso {
		t.Fatalf("aviso %q con acción %v", clave, puesto["default-action"])
	}
	enlace, _ := puesto["default-action-target"].Value().(string)
	if !strings.HasPrefix(enlace, "sytue-pruebas:/_aviso?") || !strings.Contains(enlace, "clave="+clave) || !strings.Contains(enlace, "origen=mail") {
		t.Fatalf("el enlace del aviso es %q", enlace)
	}

	// plasmashell «pulsa»: ActivateAction en el nombre de la app, con el enlace.
	if err := escritorio.Object(id, rutaDeApp(id)).Call(appInterfaz+".ActivateAction", 0,
		accionAviso, []dbus.Variant{dbus.MakeVariant(enlace)}, map[string]dbus.Variant{}).Err; err != nil {
		t.Fatal(err)
	}
	select {
	case <-pulsado:
	case <-time.After(2 * time.Second):
		t.Fatal("pulsar el aviso del portal no ha llevado a ninguna parte")
	}

	// Un portal sin avisos (el de wlroots, p. ej.): freedesktop, aunque esté registrada.
	portal.mu.Lock()
	portal.avisos, portal.id = false, ""
	portal.mu.Unlock()
	if err := avisarPor(app, aviso); err != nil {
		t.Fatal(err)
	}
	if clave, _ := portal.puesto(); clave != "" {
		t.Fatal("el aviso ha ido a un portal que no atiende avisos")
	}
}
