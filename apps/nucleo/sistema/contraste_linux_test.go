// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package sistema

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"

	"git.sytue.eu/sytue/one/apps/nucleo/internal/busprueba"
)

// TestMasContrasteEsElAltoContrasteDeGNOME: con `high-contrast` a true —un gsettings
// fingido que lo dice, y solo para esa clave— se pide más contraste; con false o sin
// gsettings, no.
func TestMasContrasteEsElAltoContrasteDeGNOME(t *testing.T) {
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent")
	gsettings := func(respuesta string) {
		t.Helper()
		dir := t.TempDir()
		guion := "#!/bin/sh\n[ \"$2\" = org.gnome.desktop.a11y.interface ] && [ \"$3\" = high-contrast ] && echo " + respuesta + "\n"
		if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte(guion), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
			t.Fatal(err)
		}
		t.Setenv("PATH", dir)
	}
	gsettings("true")
	if !masContraste() {
		t.Error("con el alto contraste de GNOME no se pide más contraste")
	}
	gsettings("false")
	if masContraste() {
		t.Error("con el alto contraste apagado se pide más contraste")
	}
	t.Setenv("PATH", t.TempDir())
	if masContraste() {
		t.Error("sin gsettings se pide más contraste")
	}
}

// portalDeContraste hace de xdg-desktop-portal con su `contrast`.
type portalDeContraste struct{ valor uint32 }

func (p portalDeContraste) ReadOne(espacio, clave string) (dbus.Variant, *dbus.Error) {
	if espacio != "org.freedesktop.appearance" || clave != "contrast" {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.portal.Error.NotFound", nil)
	}
	return dbus.MakeVariant(p.valor), nil
}

// TestMasContrasteEsElAjusteDelPortal: `contrast` = 1 pide más contraste y 0 no. Y no se
// confunde con `reduced-motion`, que es otra clave del mismo espacio.
func TestMasContrasteEsElAjusteDelPortal(t *testing.T) {
	escritorio := busprueba.Nuevo(t)
	app, err := dbus.Connect(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if portalPideMasContraste(app) {
		t.Fatal("sin portal se pide más contraste")
	}
	ruta := dbus.ObjectPath("/org/freedesktop/portal/desktop")
	if _, err := escritorio.RequestName("org.freedesktop.portal.Desktop", 0); err != nil {
		t.Fatal(err)
	}
	for _, valor := range []uint32{1, 0} {
		if err := escritorio.Export(portalDeContraste{valor}, ruta, "org.freedesktop.portal.Settings"); err != nil {
			t.Fatal(err)
		}
		if got := portalPideMasContraste(app); got != (valor == 1) {
			t.Errorf("contrast = %d: pide más contraste = %v", valor, got)
		}
		if portalPideMenosMovimiento(app) {
			t.Errorf("contrast = %d: se lee como menos movimiento", valor)
		}
	}
}
