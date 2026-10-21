// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"sync"

	"github.com/godbus/dbus/v5"
)

// El aviso de Linux va por `org.freedesktop.Notifications` a mano, y no por el
// `SendNotification` de Fyne, por una sola razón: Fyne no pide la acción «default», y
// sin ella pulsar el aviso no hace nada. Con ella, el servidor de avisos (GNOME, Plasma,
// dunst…) manda `ActionInvoked` al pulsarlo, y la suite lleva a donde el aviso dice.

const (
	avisosNombre = "org.freedesktop.Notifications"
	avisosRuta   = dbus.ObjectPath("/org/freedesktop/Notifications")
)

var (
	avisosMu sync.Mutex
	// alPulsarAviso es qué hacer con cada aviso vivo, por el número que le dio el
	// servidor. Se borra al cerrarse, pulsado o no.
	alPulsarAviso = map[uint32]func(){}
	// escuchando son las conexiones que ya tienen su rutina de señales.
	escuchando = map[*dbus.Conn]bool{}
)

func avisar(a Aviso) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	return avisarPor(conn, a)
}

// avisarPor elige el camino. Los dos primeros devuelven el clic aunque la app se haya
// cerrado; el último, solo mientras vive el proceso que avisó.
//
//  1. GNOME: `org.gtk.Notifications` (avisos_gtk_linux.go).
//  2. El portal de avisos, si sabe de qué app es esta conexión: Plasma, y cualquier
//     escritorio con un xdg-desktop-portal que los atienda (avisos_portal_linux.go).
//  3. `org.freedesktop.Notifications`, que tiene todo escritorio con avisos.
func avisarPor(conn *dbus.Conn, a Aviso) error {
	if a.ID != "" && a.Esquema != "" {
		if hayGtk(conn) {
			return avisarGtk(conn, a)
		}
		if portalConoceLaApp(conn, a.ID) {
			return avisarPortal(conn, a)
		}
	}
	return avisarEn(conn, a.ID, a.Nombre, a.Titulo, a.Cuerpo, a.AlPulsar)
}

func avisarEn(conn *dbus.Conn, id, nombre, titulo, cuerpo string, alPulsar func()) error {
	if err := escucharAvisos(conn); err != nil {
		return err
	}
	// `desktop-entry` es lo que usa el servidor para el icono y para agrupar: el .desktop
	// que `fyne package` llama como el identificador.
	var n uint32
	err := conn.Object(avisosNombre, avisosRuta).Call(avisosNombre+".Notify", 0,
		nombre, uint32(0), "", titulo, cuerpo, []string{"default", nombre},
		map[string]dbus.Variant{"desktop-entry": dbus.MakeVariant(id)}, int32(-1)).Store(&n)
	if err != nil {
		return err
	}
	if alPulsar != nil {
		avisosMu.Lock()
		alPulsarAviso[n] = alPulsar
		avisosMu.Unlock()
	}
	return nil
}

// escucharAvisos se suscribe UNA vez por conexión a las dos señales que importan.
func escucharAvisos(conn *dbus.Conn) error {
	avisosMu.Lock()
	defer avisosMu.Unlock()
	if escuchando[conn] {
		return nil
	}
	for _, miembro := range []string{"ActionInvoked", "NotificationClosed"} {
		if err := conn.AddMatchSignal(dbus.WithMatchInterface(avisosNombre), dbus.WithMatchMember(miembro)); err != nil {
			return err
		}
	}
	senales := make(chan *dbus.Signal, 16)
	conn.Signal(senales)
	escuchando[conn] = true
	go func() {
		for s := range senales {
			atenderAviso(s)
		}
	}()
	return nil
}

// atenderAviso lleva a cabo un clic, o se olvida de un aviso cerrado.
func atenderAviso(s *dbus.Signal) {
	if s == nil || s.Path != avisosRuta || len(s.Body) < 2 {
		return
	}
	n, _ := s.Body[0].(uint32)
	avisosMu.Lock()
	f := alPulsarAviso[n]
	delete(alPulsarAviso, n)
	avisosMu.Unlock()
	if accion, _ := s.Body[1].(string); s.Name == avisosNombre+".ActionInvoked" && accion == "default" && f != nil {
		f()
	}
}
