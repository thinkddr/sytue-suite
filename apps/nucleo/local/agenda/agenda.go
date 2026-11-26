// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package agenda sincroniza Calendar y Contacts de Sytue Office con las cuentas de la
// persona: CalDAV y CardDAV (iCloud con contraseña de aplicación, Fastmail, Nextcloud,
// Sytue, cualquier servidor DAV), Google Calendar y Google Contacts, y Microsoft 365 u
// Outlook (PLAN-EDICION-LOCAL §5.10, §6.11; A-10, A-11, A-12, K-05, K-06; D-16, D-18).
//
// # Cómo
//
// Cada calendario o libreta de una cuenta es uno de ESTE equipo con un origen: se lee y
// se escribe en local, siempre, con red o sin ella, y un [Sincronizador] por cuenta y
// servicio reconcilia:
//
//   - **bajada** incremental con el mecanismo del proveedor (`sync-collection` o
//     `getctag` y ETag en DAV; `syncToken` en Google; `delta` en Microsoft Graph). En la
//     primera bajada de un calendario solo entran los eventos desde hace un año
//     ([Sincronizador.Ventana]); los anteriores, si cambian;
//   - **subida**: lo que cambió aquí desde la última vez se detecta comparando cada
//     objeto con la huella de lo que se sincronizó. Lo que no sube (sin red, el servidor
//     caído) queda en la cola de pendientes del estado y se reintenta con espera
//     creciente;
//   - **conflictos**: el servidor manda en lo que no se tocó aquí. Si los dos cambiaron
//     lo mismo, el del servidor queda en su sitio y lo de aquí se guarda como un evento
//     o contacto nuevo («(copia en conflicto)»), que se sube después, y se avisa. Nunca
//     se pierde lo escrito en este equipo: una escritura del sincronizador solo se hace
//     si el objeto sigue como se leyó.
//
// El estado de cada sincronizador (fichas, ETag, huellas, pendientes) es un JSON legible
// en la carpeta de datos de la app (`<Datos>/<app>/agenda/`).
//
// # Cuándo
//
// [Motor] los lanza al abrir, al volver a primer plano, cada pocos minutos con la app
// abierta y a mano («Sincronizar ahora»). Toda la red sale por el portero (`local/red`)
// y solo hacia los servidores de las cuentas dadas de alta ([Registro]).
package agenda

import (
	"context"
	"errors"
	"time"
)

// Servicio es lo que se sincroniza.
type Servicio string

// Servicios.
const (
	Calendario Servicio = "calendario"
	Contactos  Servicio = "contactos"
)

// Errores que un [Remoto] devuelve y el sincronizador distingue.
var (
	// ErrConflicto: el objeto cambió en el servidor desde la ETag que se mandó.
	ErrConflicto = errors.New("agenda: el objeto cambió en el servidor")
	// ErrNoEsta: el objeto o la colección ya no existe en el servidor.
	ErrNoEsta = errors.New("agenda: no existe en el servidor")
	// ErrFichaCaducada: el servidor no reconoce la ficha; hay que bajar todo otra vez.
	ErrFichaCaducada = errors.New("agenda: la ficha de sincronización caducó")
	// ErrAcceso: el servidor rechazó la contraseña o el token (la cuenta pide volver a
	// entrar) o el permiso para escribir.
	ErrAcceso = errors.New("agenda: el servidor rechazó el acceso")
	// ErrNoSoportado: el proveedor no sabe guardar eso (p. ej. un tipo de repetición).
	ErrNoSoportado = errors.New("agenda: el proveedor no admite este cambio")
)

// Coleccion es un calendario o una libreta del servidor.
type Coleccion struct {
	ID          string // el del proveedor (la URL en DAV)
	Nombre      string
	Color       string // «#RRGGBB» o ""
	SoloLectura bool
	// Planifica: el servidor manda él las invitaciones de lo que se guarda (CalDAV con
	// `calendar-auto-schedule`, Google, Graph). Si no, se mandan por correo (A-11).
	Planifica bool
	// Extra es lo que el proveedor necesita volver a ver (la ficha DAV de la colección).
	Extra map[string]string
}

// Objeto es un evento (VCALENDAR con un UID, maestro y excepciones) o un contacto
// (vCard) en el servidor.
type Objeto struct {
	ID    string // el del proveedor
	ETag  string
	Datos []byte // vacío si solo se conoce la ETag
}

// Cambios es lo que cambió en una colección desde una ficha.
type Cambios struct {
	Puestos  []Objeto // nuevos o cambiados, con datos
	Fuera    []Objeto // existen pero no se bajan (antes de la ventana)
	Borrados []string
	// Completo: Vistos es la colección entera; lo conocido que no está, se borró.
	Completo bool
	Vistos   []string
	Ficha    string
}

// Remoto es un proveedor (una cuenta y un servicio).
type Remoto interface {
	Colecciones(ctx context.Context) ([]Coleccion, error)
	// Cambios baja lo que cambió desde ficha ("" la primera vez). etagDe dice la ETag
	// conocida de un objeto. desde (no cero) acota la primera bajada de un calendario.
	Cambios(ctx context.Context, col Coleccion, ficha string, etagDe func(id string) (string, bool), desde time.Time) (Cambios, error)
	Leer(ctx context.Context, col Coleccion, id string) (Objeto, error)
	// Crear sube un objeto nuevo; devuelve su ID y su ETag y, si el proveedor le añadió
	// algo que hay que tener aquí (el enlace de la reunión que creó), sus Datos.
	Crear(ctx context.Context, col Coleccion, uid string, datos []byte) (Objeto, error)
	// Cambiar lo reemplaza si sigue en obj.ETag (si no, ErrConflicto); los Datos que
	// devuelva, como en Crear.
	Cambiar(ctx context.Context, col Coleccion, obj Objeto, datos []byte) (Objeto, error)
	// Borrar lo quita si sigue en etag. Si ya no estaba, nil.
	Borrar(ctx context.Context, col Coleccion, id, etag string) error
}

// ObjetoLocal es un evento o un contacto de este equipo.
type ObjetoLocal struct {
	ID, UID, Datos string
}

// ErrCambioLocal: el objeto cambió aquí mientras se sincronizaba; no se pisa.
var ErrCambioLocal = errors.New("agenda: el objeto cambió en este equipo")

// Local es el almacén de la app (Calendar o Contacts).
type Local interface {
	Crear(ctx context.Context, nombre, color string, soloLectura bool) (string, error)
	Actualizar(ctx context.Context, id, nombre, color string, soloLectura bool) error
	Existe(ctx context.Context, id string) (bool, error)
	Objetos(ctx context.Context, coleccion string) ([]ObjetoLocal, error)
	// Poner crea (id "") o reemplaza un objeto si sigue con los datos esperado ("" no lo
	// mira); si cambió, ErrCambioLocal.
	Poner(ctx context.Context, coleccion, id, esperado, datos string) (ObjetoLocal, error)
	Borrar(ctx context.Context, id, esperado string) error
	// Copia hace de datos un objeto nuevo (UID nuevo, nombre marcado): la copia de lo
	// escrito aquí cuando choca con un cambio del servidor.
	Copia(datos, marca string) (string, error)
}
