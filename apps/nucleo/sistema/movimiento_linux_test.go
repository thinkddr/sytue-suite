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

// TestMenosMovimientoEsLaPreferenciaDeGNOME: con `enable-animations` a false —un gsettings
// fingido que lo dice— se pide menos movimiento; sin gsettings, no.
func TestMenosMovimientoEsLaPreferenciaDeGNOME(t *testing.T) {
	sinKDE(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gsettings"), []byte("#!/bin/sh\necho false\n"), 0o755); err != nil { //nolint:gosec // G306: tiene que poder ejecutarse
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if !menosMovimiento() {
		t.Error("con las animaciones apagadas no se pide menos movimiento")
	}
	t.Setenv("PATH", t.TempDir())
	if menosMovimiento() {
		t.Error("sin gsettings se pide menos movimiento")
	}
}

// sinKDE deja las carpetas de configuración vacías y sin bus.
func sinKDE(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_DIRS", t.TempDir())
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/nonexistent")
}

// TestMenosMovimientoEsLaVelocidadDeAnimacionDeKDE: el deslizador de Plasma al mínimo
// escribe `AnimationDurationFactor=0` en `[KDE]`; la misma clave en otro grupo no cuenta,
// el fichero de la persona manda sobre el del sistema, y el del sistema vale si la
// persona no dice nada. VISTO EN ROJO sin comprobar el grupo y leyendo solo el de la
// persona.
func TestMenosMovimientoEsLaVelocidadDeAnimacionDeKDE(t *testing.T) {
	sinKDE(t)
	t.Setenv("PATH", t.TempDir())
	escribir := func(dir, contenido string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "kdeglobals"), []byte(contenido), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	casa, sistema := os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_CONFIG_DIRS")
	casos := []struct {
		nombre, casa, sistema string
		menos                 bool
	}{
		{"al mínimo", "[General]\nfoo=1\n\n[KDE]\nAnimationDurationFactor=0\n", "", true},
		{"a medias", "[KDE]\nAnimationDurationFactor=0.5\n", "", false},
		{"en otro grupo", "[General]\nAnimationDurationFactor=0\n", "", false},
		{"bloqueada por el administrador", "", "[KDE]\nAnimationDurationFactor[$i]=0\n", true},
		{"la persona manda", "[KDE]\nAnimationDurationFactor=1\n", "[KDE]\nAnimationDurationFactor=0\n", false},
		{"sin fichero", "", "", false},
	}
	for _, c := range casos {
		escribir(casa, c.casa)
		escribir(sistema, c.sistema)
		if got := menosMovimiento(); got != c.menos {
			t.Errorf("%s: menosMovimiento() = %v", c.nombre, got)
		}
	}
}

// portalFalso hace de xdg-desktop-portal con su `reduced-motion`.
type portalFalso struct{ valor uint32 }

func (p portalFalso) ReadOne(espacio, clave string) (dbus.Variant, *dbus.Error) {
	if espacio != "org.freedesktop.appearance" || clave != "reduced-motion" {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.portal.Error.NotFound", nil)
	}
	return dbus.MakeVariant(p.valor), nil
}

// TestMenosMovimientoEsElAjusteDelPortal: `reduced-motion` = 1 pide menos movimiento, 0
// no. VISTO EN ROJO leyendo la clave `contrast`.
func TestMenosMovimientoEsElAjusteDelPortal(t *testing.T) {
	escritorio := busprueba.Nuevo(t)
	app, err := dbus.Connect(os.Getenv("DBUS_SESSION_BUS_ADDRESS"))
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	if portalPideMenosMovimiento(app) {
		t.Fatal("sin portal se pide menos movimiento")
	}
	ruta := dbus.ObjectPath("/org/freedesktop/portal/desktop")
	if _, err := escritorio.RequestName("org.freedesktop.portal.Desktop", 0); err != nil {
		t.Fatal(err)
	}
	for _, valor := range []uint32{1, 0} {
		if err := escritorio.Export(portalFalso{valor}, ruta, "org.freedesktop.portal.Settings"); err != nil {
			t.Fatal(err)
		}
		if got := portalPideMenosMovimiento(app); got != (valor == 1) {
			t.Errorf("reduced-motion = %d: pide menos movimiento = %v", valor, got)
		}
	}
}
