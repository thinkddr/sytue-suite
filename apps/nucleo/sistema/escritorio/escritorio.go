// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package escritorio es lo del escritorio que el toolkit no da y que la suite necesita:
// el NÚMERO sobre el icono de la aplicación, saber si hay quien pinte la bandeja del
// sistema, y un aviso que al PULSARLO lleve a algún sitio. Qué número se pone y cuándo
// quedarse en la bandeja lo decide quien llama, en Go; aquí solo se ejecuta o se pregunta.
//
// # La insignia, destino a destino
//
//   - Linux: la señal `com.canonical.Unity.LauncherEntry.Update` por el bus de sesión, que
//     leen el dock de Ubuntu, Dash to Dock, Plasma y los lanzadores que siguen esa
//     convención. Probado contra un bus de verdad (escritorio_linux_test.go).
//   - macOS: `NSApp.dockTile.badgeLabel`. Compila; nadie lo ha visto en un Dock todavía.
//   - Windows: no hay número sobre el icono de la barra de tareas para una app de escritorio
//     —solo un icono superpuesto, `ITaskbarList3::SetOverlayIcon`, que pide dibujar el
//     número en un HICON—. ponytail: no se pone; se releva cuando haya un Windows donde verlo.
//   - Android: el lanzador pone el punto él solo mientras haya avisos en la bandeja.
//   - iOS: pide `UNUserNotificationCenter.setBadgeCount`, y aquí no hay Xcode para compilarlo.
package escritorio

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"sync"
)

// ErrNoDisponible es que este sistema no ofrece la operación aquí: quien llama cae en
// lo del toolkit.
var ErrNoDisponible = errors.New("escritorio: el sistema no ofrece esa operación")

// Aviso es lo que se enseña en el sistema y a dónde lleva pulsarlo.
type Aviso struct {
	// ID es el identificador de paquete (`eu.sytue.mail`), y Nombre el de la app tal
	// como se ve. Esquema es el de su `FyneApp.toml` (`sytue-mail`): por él vuelve el
	// clic en Windows y en Android.
	ID, Nombre, Esquema string
	Titulo, Cuerpo      string
	// Destino es a dónde lleva pulsarlo si la app que avisó ya no está viva: va escrito
	// en el propio aviso (el enlace en Windows y Android, el `userInfo` en macOS).
	Destino Destino
	// AlPulsar se llama en su propia rutina: quien toque la interfaz, que use `fyne.Do`.
	AlPulsar func()
}

// Destino es lo que un aviso lleva consigo para poder seguirlo sin el proceso que lo
// puso: un toast viejo del centro de actividades de Windows, un aviso del Centro de
// notificaciones de macOS o uno de Android pulsado después de cerrar la app.
type Destino struct {
	// Aviso es el identificador del aviso del Hub, para marcarlo leído.
	Aviso string
	// Origen es la clave del producto (`mail`) y Ruta la página de su web, con la
	// consulta (`/mail/inbox?m=42`). Sin host a propósito: lo pone quien lo sigue con
	// el dominio de la suite, así que un enlace escrito a mano no lleva a otra web.
	Origen, Ruta string
}

// DestinoDe saca el Destino de la dirección que trae un aviso del Hub, que puede ser la
// URL entera de la web o solo su ruta. Se queda con la ruta y la consulta.
func DestinoDe(aviso, origen, direccion string) Destino {
	u, err := url.Parse(direccion)
	if err != nil || direccion == "" {
		return Destino{Aviso: aviso, Origen: origen}
	}
	ruta := u.EscapedPath()
	if u.RawQuery != "" {
		ruta += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		ruta += "#" + u.EscapedFragment()
	}
	return Destino{Aviso: aviso, Origen: origen, Ruta: ruta}
}

// Avisar enseña un aviso del sistema que, al pulsarlo, llama a `AlPulsar`.
//
// Cada sistema entrega el clic por su camino:
//
//   - Linux: `org.gtk.Notifications` (GNOME) o el portal de avisos (Plasma) con la
//     acción `app.aviso`, y si no, `org.freedesktop.Notifications` con la acción
//     «default», que devuelve la señal `ActionInvoked` a esta conexión.
//   - macOS: el delegado de `UNUserNotificationCenter`, con el [EnlaceDeAviso] en su
//     `userInfo`. Solo con la app empaquetada: sin identificador de paquete el centro de
//     avisos no existe y devuelve ErrNoDisponible.
//   - Windows: un toast con `activationType="protocol"`, que al pulsarlo ABRE su
//     [EnlaceDeAviso]. Es lo único que una app de escritorio sin paquete ni activador COM
//     puede pedir, y ese enlace vuelve a la instancia viva por su enchufe, como cualquier
//     otro de la suite, hasta [Pulsado].
//   - Android: un PendingIntent que abre ese mismo [EnlaceDeAviso] en la app.
//
// En el resto (iOS) devuelve ErrNoDisponible y quien llama usa el aviso de Fyne, que no lleva
// clic.
//
// Si la app que avisó ya no está viva —se cerró, o Android mató el proceso—, pulsarlo la
// abre y el aviso lleva su [Destino]: [Pulsado] se lo pasa a quien se apuntó con
// [AlSeguir], que hace lo mismo que la campana.
//
// En Linux, con GNOME el aviso va por `org.gtk.Notifications`, y en Plasma (o donde haya
// un xdg-desktop-portal que atienda avisos) por `org.freedesktop.portal.Notification`:
// los dos son de la app y no del proceso, así que pulsarlo con la app cerrada la arranca
// D-Bus y le entrega el enlace ([RecibirAvisos]). Donde no hay ninguno de los dos (dunst,
// un portal que no sabe de qué app es el aviso), el clic llega solo mientras vive el
// proceso que avisó: freedesktop devuelve la acción a la conexión que puso el aviso.
func Avisar(a Aviso) error { return avisar(a) }

// RecibirAvisos deja la app `id` lista para que un aviso pulsado con ella cerrada la
// despierte. Solo hace algo en Linux (el `.service` de D-Bus y el nombre de bus de la
// app); se llama al arrancar, antes de que llegue nada.
func RecibirAvisos(id string) error {
	if id == "" {
		return nil
	}
	return recibirAvisos(id)
}

// El clic que vuelve por un ENLACE (Windows y Android) no puede
// llevar una función: lleva una clave al azar, y aquí se guarda qué hacer con ella.
var (
	clavesMu sync.Mutex
	claves   = map[string]func(){}
	orden    []string
)

// maxClaves es cuántos avisos vivos se recuerdan. Pulsar uno más viejo no hace nada;
// sin tope, una app abierta una semana acumularía una función por aviso.
const maxClaves = 64

// apuntar guarda `f` y devuelve la clave con la que se la reclama.
func apuntar(f func()) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	clave := hex.EncodeToString(b)
	clavesMu.Lock()
	defer clavesMu.Unlock()
	claves[clave] = f
	if orden = append(orden, clave); len(orden) > maxClaves {
		delete(claves, orden[0])
		orden = orden[1:]
	}
	return clave
}

// pulsar hace lo que dice el aviso de esa clave, una sola vez, y dice si la conocía.
func pulsar(clave string) bool {
	clavesMu.Lock()
	f, ok := claves[clave]
	delete(claves, clave)
	clavesMu.Unlock()
	if f != nil {
		go f()
	}
	return ok
}

// rutaAviso es la del enlace de un aviso pulsado. Empieza por `_` para que no la pise
// ninguna ruta de la web, que es de donde salen las de los receptores.
const rutaAviso = "/_aviso"

// EnlaceDeAviso es la URL que abre pulsar el aviso de esa clave, con su destino:
// `sytue-mail:/_aviso?clave=…&aviso=…&origen=mail&ruta=%2Fmail%2Finbox`.
func EnlaceDeAviso(esquema, clave string, d Destino) string {
	q := url.Values{"clave": {clave}}
	if d.Origen != "" {
		q.Set("aviso", d.Aviso)
		q.Set("origen", d.Origen)
		q.Set("ruta", d.Ruta)
	}
	return esquema + ":" + rutaAviso + "?" + q.Encode()
}

var (
	seguirMu  sync.Mutex
	seguidor  func(Destino)
	pendiente *Destino // llegó antes de que nadie se apuntara
)

// AlSeguir apunta quién lleva a su destino un aviso cuya app ya no estaba viva cuando se
// pulsó (`apps/fyne/sesion`, que tiene el Hub y la app). Se le llama en su propia rutina.
// Lo que llegó antes de apuntarse se le entrega en el acto: en macOS el clic con la app
// cerrada llega mientras arranca.
func AlSeguir(f func(Destino)) {
	seguirMu.Lock()
	seguidor = f
	d := pendiente
	pendiente = nil
	seguirMu.Unlock()
	if d != nil && f != nil {
		go f(*d)
	}
}

// Pulsado atiende el enlace de un aviso pulsado y dice si lo era. Quien recibe los
// enlaces de la app (`apps/fyne/sesion`) lo pregunta antes que nada: ese enlace no es de
// ninguna pantalla.
//
// Si la clave es de este proceso, hace lo que el aviso dijo al ponerse. Si no —un aviso
// de antes de reabrir la app—, sigue el [Destino] que lleva escrito. Ese destino lo puede
// escribir cualquiera que pueda abrir un enlace, así que solo se sigue si es una ruta de
// un producto de la suite: lo peor que lleva es a una página suya, como cualquier otro
// enlace de la app, y a marcar leído un aviso de quien ya ha entrado.
func Pulsado(u *url.URL) bool {
	if u == nil || u.Path != rutaAviso || !strings.HasPrefix(u.Scheme, "sytue") {
		return false
	}
	q := u.Query()
	if pulsar(q.Get("clave")) {
		return true
	}
	d := Destino{Aviso: q.Get("aviso"), Origen: q.Get("origen"), Ruta: q.Get("ruta")}
	if !destinoValido(d) {
		return true
	}
	seguirMu.Lock()
	f := seguidor
	if f == nil {
		pendiente = &d
	}
	seguirMu.Unlock()
	if f != nil {
		go f(d)
	}
	return true
}

// destinoValido: una clave de producto (letras y cifras) y una ruta que empieza por UNA
// barra. `//otro.host` sería una URL sin esquema que algún abridor tomaría por otro host.
func destinoValido(d Destino) bool {
	if d.Origen == "" || len(d.Origen) > 32 || strings.HasPrefix(d.Ruta, "//") ||
		(d.Ruta != "" && d.Ruta[0] != '/') {
		return false
	}
	for _, c := range d.Origen {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// PrepararAvisos deja la app lista para avisar antes del primer aviso. Solo hace algo en
// Android: crea el canal de avisos y, desde Android 13, pide el permiso
// POST_NOTIFICATIONS, sin el cual el sistema se calla los avisos sin dar ningún error.
// `nombre` es el de la app tal como se ve, que es como Android enseña el canal.
func PrepararAvisos(nombre string) error { return prepararAvisos(nombre) }

// Insignia deja `n` sobre el icono de la aplicación `id` (el identificador de paquete,
// `eu.sytue.mail`). 0 lo quita. Donde el sistema no lo ofrece, no hace nada.
func Insignia(id string, n int) error {
	if id == "" {
		return nil
	}
	return insignia(id, max(n, 0))
}

// HayBandeja dice si en este escritorio alguien pinta la bandeja del sistema. En macOS y
// en Windows, siempre; en Linux depende del escritorio, y sin ella una app que se esconde
// al cerrar la ventana se queda corriendo sin forma de volver a ella.
func HayBandeja() bool { return hayBandeja() }

// CederPrimerPlano deja que OTRO proceso se ponga delante. Windows solo se lo permite a
// quien tiene el primer plano: la copia que un enlace arranca lo tiene, se lo pasa a la
// instancia que ya corría y se va; sin cederlo, esa instancia recibía el enlace y se
// quedaba detrás (visto en Windows: «Ver estos ficheros en el equipo» de Drive abría la
// página del Hub sin traerlo). En los demás sistemas no hace nada.
func CederPrimerPlano() { cederPrimerPlano() }
