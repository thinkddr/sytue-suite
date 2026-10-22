// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package escritorio

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// # El clic con la app cerrada, en GNOME
//
// `org.freedesktop.Notifications` devuelve la acción a la conexión que puso el aviso: si
// el proceso ya no vive, el clic no va a ninguna parte. GNOME Shell tiene además
// `org.gtk.Notifications`, que es de la APP y no del proceso: al pulsar, llama a
// `org.freedesktop.Application.ActivateAction` en el nombre de bus de la app
// (`eu.sytue.one`), y si nadie lo tiene, D-Bus arranca la app con su fichero `.service`.
// Se usa cuando está; si no, el portal de avisos (avisos_portal_linux.go), y si tampoco,
// el de freedesktop.
//
// La acción lleva el mismo [EnlaceDeAviso] que el toast de Windows —clave y destino—, así
// que llega a [Pulsado] por el mismo camino: con la app viva, la clave; recién arrancada,
// el destino.

const (
	gtkNombre   = "org.gtk.Notifications"
	gtkRuta     = dbus.ObjectPath("/org/gtk/Notifications")
	appInterfaz = "org.freedesktop.Application"
	// accionAviso es la acción de la app que sigue un aviso pulsado.
	accionAviso = "aviso"
)

// rutaDeApp es la ruta de objeto de la app por la convención de GApplication:
// `eu.sytue.one` → `/eu/sytue/one`.
func rutaDeApp(id string) dbus.ObjectPath {
	return dbus.ObjectPath("/" + strings.ReplaceAll(strings.ReplaceAll(id, ".", "/"), "-", "_"))
}

// aplicacion es `org.freedesktop.Application` de esta app: lo que GNOME llama al pulsar.
type aplicacion struct{}

func (aplicacion) Activate(map[string]dbus.Variant) *dbus.Error { return nil }

func (aplicacion) Open([]string, map[string]dbus.Variant) *dbus.Error { return nil }

func (aplicacion) ActivateAction(nombre string, params []dbus.Variant, _ map[string]dbus.Variant) *dbus.Error {
	if nombre != accionAviso || len(params) != 1 {
		return nil
	}
	if enlace, ok := params[0].Value().(string); ok {
		if u, err := url.Parse(enlace); err == nil {
			Pulsado(u)
		}
	}
	return nil
}

var (
	sirviendoMu sync.Mutex
	sirviendo   = map[*dbus.Conn]bool{}
)

// servirEn pone `org.freedesktop.Application` en la ruta de la app y se queda su nombre
// de bus, una vez por conexión. Si otra copia de la app ya lo tiene, no es un error: el
// clic irá a ella, que es la que puso el aviso.
func servirEn(conn *dbus.Conn, id string) error {
	sirviendoMu.Lock()
	defer sirviendoMu.Unlock()
	if sirviendo[conn] {
		return nil
	}
	if err := conn.Export(aplicacion{}, rutaDeApp(id), appInterfaz); err != nil {
		return err
	}
	if _, err := conn.RequestName(id, dbus.NameFlagDoNotQueue); err != nil {
		return err
	}
	sirviendo[conn] = true
	return nil
}

// hayGtk dice si hay quien atienda `org.gtk.Notifications`.
func hayGtk(conn *dbus.Conn) bool {
	var tiene bool
	err := conn.BusObject().Call("org.freedesktop.DBus.NameHasOwner", 0, gtkNombre).Store(&tiene)
	return err == nil && tiene
}

// avisarGtk pone el aviso en GNOME con la acción de seguirlo.
func avisarGtk(conn *dbus.Conn, a Aviso) error {
	if err := servirEn(conn, a.ID); err != nil {
		return err
	}
	clave := apuntar(a.AlPulsar)
	return conn.Object(gtkNombre, gtkRuta).Call(gtkNombre+".AddNotification", 0, a.ID, clave,
		map[string]dbus.Variant{
			"title":                 dbus.MakeVariant(a.Titulo),
			"body":                  dbus.MakeVariant(a.Cuerpo),
			"default-action":        dbus.MakeVariant("app." + accionAviso),
			"default-action-target": dbus.MakeVariant(EnlaceDeAviso(a.Esquema, clave, a.Destino)),
		}).Err
}

// recibirAvisos deja la app lista para que un aviso pulsado con ella cerrada la
// despierte: el fichero `.service` que D-Bus usa para arrancarla, y su nombre de bus
// cogido desde ya, que es a quien D-Bus entrega la acción que la despertó.
func recibirAvisos(id string) error {
	conn, err := dbus.SessionBus()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err == nil {
		exe, err = filepath.EvalSymlinks(exe)
	}
	if err != nil {
		return err
	}
	// Lo primero, antes que ninguna otra llamada a un portal por esta conexión: después,
	// el portal ya no deja decirle de qué app es.
	registrarEnPortal(conn, id)
	if cambiado, err := ponerServicio(id, exe); err != nil {
		return err
	} else if cambiado {
		// dbus-daemon y dbus-broker releen las carpetas de servicios con esto.
		_ = conn.BusObject().Call("org.freedesktop.DBus.ReloadConfig", 0).Err
	}
	return servirEn(conn, id)
}

// ponerServicio escribe `$XDG_DATA_HOME/dbus-1/services/<id>.service`, que es lo que D-Bus
// ejecuta para despertar la app. Arranca con [ArgBandeja]: en el Hub, seguir el aviso no
// tiene por qué abrir su ventana. Dice si ha cambiado algo.
func ponerServicio(id, exe string) (bool, error) {
	if strings.ContainsAny(id, "/\n") || strings.ContainsAny(exe, "\n") {
		return false, errors.New("escritorio: identificador o ejecutable no válidos para D-Bus")
	}
	// Por os.Root, abierto en XDG_DATA_HOME o en la casa: lo que se escribe no puede
	// salir de ahí, sea cual sea el id.
	base, carpeta := os.Getenv("XDG_DATA_HOME"), filepath.Join("dbus-1", "services")
	if base == "" {
		casa, err := os.UserHomeDir()
		if err != nil {
			return false, err
		}
		base, carpeta = casa, filepath.Join(".local", "share", carpeta)
	}
	raiz, err := os.OpenRoot(base)
	if err != nil {
		return false, err
	}
	defer raiz.Close()
	ruta := filepath.Join(carpeta, id+".service")
	citar := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	contenido := "[D-BUS Service]\nName=" + id + "\nExec=\"" + citar.Replace(exe) + "\" " + ArgBandeja + "\n"
	if viejo, err := raiz.ReadFile(ruta); err == nil && string(viejo) == contenido {
		return false, nil
	}
	if err := raiz.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		return false, err
	}
	// 0644: lo lee D-Bus, y no lleva nada secreto.
	return true, raiz.WriteFile(ruta, []byte(contenido), 0o644)
}
