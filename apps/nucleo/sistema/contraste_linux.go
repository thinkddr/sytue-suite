// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package sistema

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// masContraste pregunta a los dos sitios donde un Linux lo dice, y basta con uno:
//
//   - GNOME: «Alto contraste» de Accesibilidad, que es `high-contrast` a true en
//     `org.gnome.desktop.a11y.interface`, con gsettings.
//   - El portal: `contrast` = 1 en `org.freedesktop.appearance`. Es lo que Firefox y
//     Chromium leen para `prefers-contrast: more`, y sirve a quien no tiene gsettings.
func masContraste() bool {
	if gnomeConAltoContraste() {
		return true
	}
	conn, err := dbus.SessionBus()
	return err == nil && portalPideMasContraste(conn)
}

func gnomeConAltoContraste() bool {
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	defer parar()
	salida, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.a11y.interface", "high-contrast").Output()
	return err == nil && strings.TrimSpace(string(salida)) == "true"
}

// portalPideMasContraste lee `contrast` del portal de ajustes: 0 es sin preferencia y 1,
// más contraste. Sin portal o sin la clave, no pide nada.
func portalPideMasContraste(conn *dbus.Conn) bool {
	return portalApariencia(conn, "contrast") == 1
}
