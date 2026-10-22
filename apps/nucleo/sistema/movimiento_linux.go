// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package sistema

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// menosMovimiento pregunta a los tres sitios donde un Linux lo dice, y basta con uno:
//
//   - GNOME: `enable-animations` a false, con gsettings.
//   - KDE Plasma: «Velocidad de las animaciones» al mínimo, que es
//     `AnimationDurationFactor=0` en `[KDE]` de kdeglobals.
//   - El portal: `reduced-motion` = 1 en `org.freedesktop.appearance`. Lo dan el de
//     KDE (que lo saca del mismo kdeglobals) y el de GNOME en sus versiones recientes;
//     sirve también a quien no tiene ni gsettings ni el fichero a mano.
func menosMovimiento() bool {
	if gnomeSinAnimaciones() || kdeSinAnimaciones() {
		return true
	}
	conn, err := dbus.SessionBus()
	return err == nil && portalPideMenosMovimiento(conn)
}

func gnomeSinAnimaciones() bool {
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	defer parar()
	salida, err := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "enable-animations").Output()
	return err == nil && strings.TrimSpace(string(salida)) == "false"
}

// kdeSinAnimaciones lee `AnimationDurationFactor` como lo lee KConfig: el kdeglobals
// de la persona manda, y si no lo dice, los de XDG_CONFIG_DIRS (lo que pone el
// administrador o la distribución), en su orden.
func kdeSinAnimaciones() bool {
	for _, dir := range carpetasDeConfiguracion() {
		if factor, ok := factorDeAnimacion(filepath.Join(dir, "kdeglobals")); ok {
			return factor == 0
		}
	}
	return false
}

func carpetasDeConfiguracion() []string {
	var dirs []string
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		dirs = append(dirs, d)
	} else if casa, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(casa, ".config"))
	}
	sistema := os.Getenv("XDG_CONFIG_DIRS")
	if sistema == "" {
		sistema = "/etc/xdg"
	}
	return append(dirs, filepath.SplitList(sistema)...)
}

// factorDeAnimacion saca `AnimationDurationFactor` del grupo `[KDE]` de un fichero de
// KConfig, y dice si lo había.
func factorDeAnimacion(ruta string) (float64, bool) {
	f, err := os.Open(ruta) //nolint:gosec // G304: kdeglobals de las carpetas XDG de quien ejecuta la app
	if err != nil {
		return 0, false
	}
	defer f.Close()
	grupo := ""
	for lector := bufio.NewScanner(f); lector.Scan(); {
		linea := strings.TrimSpace(lector.Text())
		if strings.HasPrefix(linea, "[") {
			grupo = linea
			continue
		}
		clave, valor, ok := strings.Cut(linea, "=")
		// `AnimationDurationFactor[$i]` es la misma clave, bloqueada por el administrador.
		clave = strings.TrimSuffix(strings.TrimSpace(clave), "[$i]")
		if !ok || grupo != "[KDE]" || clave != "AnimationDurationFactor" {
			continue
		}
		factor, err := strconv.ParseFloat(strings.TrimSpace(valor), 64)
		return factor, err == nil
	}
	return 0, false
}

// portalPideMenosMovimiento lee `reduced-motion` del portal de ajustes. Sin portal o sin
// la clave (versiones anteriores a que existiera), no pide nada.
func portalPideMenosMovimiento(conn *dbus.Conn) bool {
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	defer parar()
	var v dbus.Variant
	err := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop").
		CallWithContext(ctx, "org.freedesktop.portal.Settings.ReadOne", 0,
			"org.freedesktop.appearance", "reduced-motion").Store(&v)
	n, _ := v.Value().(uint32)
	return err == nil && n == 1
}
