// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// # El clic con la app cerrada, fuera de GNOME
//
// `org.freedesktop.portal.Notification` hace lo mismo que `org.gtk.Notifications` en
// GNOME: el aviso es de la APP, y pulsarlo llama a
// `org.freedesktop.Application.ActivateAction` en su nombre de bus (`eu.sytue.hub`),
// que D-Bus arranca con el `.service` si nadie lo tiene. En Plasma lo atiende
// plasmashell (libnotificationmanager, `Portal::invokeAction`), y solo si el portal le
// dice de qué app es el aviso: con el id vacío, el clic no hace nada.
//
// Una app de fuera de Flatpak consigue ese id de dos maneras:
//
//   - `org.freedesktop.host.portal.Registry.Register`, desde xdg-desktop-portal 1.19.4,
//     y solo si es lo primero que la conexión le pide a un portal ([registrarEnPortal]).
//   - Su unidad de systemd: quien la lanza desde el menú o al iniciar sesión la pone en
//     `app-<id>-<azar>.scope` o `app-<id>@<azar>.service`, y el portal lee el id de ahí.
//
// Si no hay ninguna de las dos —un xdg-desktop-portal viejo y la app lanzada desde una
// terminal, o despertada por el propio D-Bus—, el aviso va por freedesktop, que al menos
// devuelve el clic mientras la app siga viva.
//
// No se pone `DBusActivatable=true` en el .desktop: el portal no lo mira (llama al nombre
// de bus, y lo arranca el `.service`), y con él el menú abriría las apps llamando a
// `Activate` en vez de a su `Exec`.

const (
	// portalNombre y portalRuta, los del portal, están en atajo_linux.go.
	portalAvisos   = "org.freedesktop.portal.Notification"
	portalRegistro = "org.freedesktop.host.portal.Registry"
)

var (
	registradasMu sync.Mutex
	// registradas es el id con el que el portal conoce cada conexión, si se registró.
	registradas = map[*dbus.Conn]string{}
)

// registrarEnPortal le dice al portal de qué app es esta conexión. Que falle es lo
// normal en un xdg-desktop-portal anterior al registro: queda la unidad de systemd.
func registrarEnPortal(conn *dbus.Conn, id string) {
	err := conn.Object(portalNombre, portalRuta).Call(portalRegistro+".Register", 0, id, map[string]dbus.Variant{}).Err
	if err == nil {
		registradasMu.Lock()
		registradas[conn] = id
		registradasMu.Unlock()
	}
}

// portalConoceLaApp dice si el portal atiende avisos y sabe que esta conexión es la app
// `id`: sin lo segundo, el clic no volvería.
func portalConoceLaApp(conn *dbus.Conn, id string) bool {
	registradasMu.Lock()
	registrada := registradas[conn] == id
	registradasMu.Unlock()
	if !registrada {
		cgroup, err := os.ReadFile("/proc/self/cgroup")
		if err != nil || idDeUnidad(string(cgroup)) != id {
			return false
		}
	}
	_, err := conn.Object(portalNombre, portalRuta).GetProperty(portalAvisos + ".version")
	return err == nil
}

// Las dos formas de unidad de una app, tal como las lee xdg-desktop-portal
// (`_xdp_app_info_host_parse_app_id_from_unit_name`), de systemd.io/DESKTOP_ENVIRONMENTS:
// `app[-<lanzador>]-<id>-<azar>.scope|.slice` y `app[-<lanzador>]-<id>[@<azar>].service`.
var (
	unidadAmbito   = regexp.MustCompile(`^app-(?:[[:alnum:]]+-)?(.+?)(?:-[[:alnum:]]*)(?:\.scope|\.slice)$`)
	unidadServicio = regexp.MustCompile(`^app-(?:[[:alnum:]]+-)?(.+?)(?:@[[:alnum:]]*|-autostart)?\.service$`)
)

// idDeUnidad saca el id de app de la unidad de systemd en `/proc/self/cgroup` (la línea
// `0::` de cgroup v2), o "" si el proceso no está en la unidad de ninguna app.
func idDeUnidad(cgroup string) string {
	for linea := range strings.SplitSeq(cgroup, "\n") {
		ruta, ok := strings.CutPrefix(linea, "0::")
		if !ok {
			continue
		}
		unidad := ruta[strings.LastIndexByte(ruta, '/')+1:]
		m := unidadAmbito.FindStringSubmatch(unidad)
		if m == nil {
			m = unidadServicio.FindStringSubmatch(unidad)
		}
		if m == nil {
			return ""
		}
		return desescapar(m[1])
	}
	return ""
}

// desescapar deshace el `\x2d` con el que systemd escribe un guion dentro de un nombre.
func desescapar(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) && s[i+1] == 'x' {
			if n, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// avisarPortal pone el aviso con la acción de seguirlo, como en GNOME: el mismo
// [EnlaceDeAviso] vuelve a [Pulsado] por `ActivateAction`, viva o no la app.
func avisarPortal(conn *dbus.Conn, a Aviso) error {
	if err := servirEn(conn, a.ID); err != nil {
		return err
	}
	clave := apuntar(a.AlPulsar)
	return conn.Object(portalNombre, portalRuta).Call(portalAvisos+".AddNotification", 0, clave,
		map[string]dbus.Variant{
			"title":                 dbus.MakeVariant(a.Titulo),
			"body":                  dbus.MakeVariant(a.Cuerpo),
			"default-action":        dbus.MakeVariant("app." + accionAviso),
			"default-action-target": dbus.MakeVariant(EnlaceDeAviso(a.Esquema, clave, a.Destino)),
		}).Err
}
