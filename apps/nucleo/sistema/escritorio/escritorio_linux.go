// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import "github.com/godbus/dbus/v5"

// insignia emite LauncherEntry.Update. Va por la conexión COMPARTIDA del proceso y no por
// una propia a propósito: Plasma borra el número cuando desaparece quien lo emitió, así
// que una conexión que se cierra al volver dejaría la insignia puesta un instante.
//
// La app se nombra por su .desktop, que `fyne package` llama como el identificador.
func insignia(id string, n int) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err // sin bus de sesión no hay lanzador que lo lea; quien llama lo ignora
	}
	return conn.Emit("/eu/sytue/insignia", "com.canonical.Unity.LauncherEntry.Update",
		"application://"+id+".desktop", map[string]dbus.Variant{
			"count":         dbus.MakeVariant(int64(n)),
			"count-visible": dbus.MakeVariant(n > 0),
		})
}

// hayBandeja pregunta si está el StatusNotifierWatcher, que es quien recoge los iconos de
// la bandeja. Lo ponen Plasma, XFCE y la extensión AppIndicator del GNOME de Ubuntu; el
// GNOME de serie, no.
func hayBandeja() bool {
	conn, err := dbus.SessionBus()
	return err == nil && hayBandejaEn(conn)
}

func hayBandejaEn(conn *dbus.Conn) bool {
	var tiene bool
	err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, "org.kde.StatusNotifierWatcher").Store(&tiene)
	return err == nil && tiene
}
