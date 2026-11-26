// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package chat es Rooms en Sytue Office (apps/fyne/PLAN-EDICION-LOCAL.md §6.14): un
// CLIENTE de los servicios de mensajería que la persona configura, sin salas propias ni
// servidor de Sytue.
//
// El Agregador cumple la interfaz de la pantalla (`rooms.Servicio`) juntando varios
// proveedores; cada uno es una cuenta o un espacio:
//
//   - Notas, el proveedor local: canales y mensajes en claro, en JSON, bajo
//     `<Datos>/Datos/rooms/`. No habla con la red y está siempre, así que la app abre y
//     sirve sin configurar nada (canales de notas para uno mismo).
//   - Matrix, una cuenta de un servidor Matrix (Sytue Rooms online es Matrix) SIN cifrado
//     de extremo a extremo: lee y escribe en las salas sin cifrar; las cifradas se ven en
//     la lista pero no se pueden leer ni escribir desde esta edición.
//
// Los identificadores que ve la pantalla llevan delante la cuenta: un canal es
// `<cuenta>|<canal>` y un mensaje `<cuenta>|<canal>|<mensaje>`, así que cada llamada sabe
// a qué proveedor ir sin preguntar a nadie.
package chat

import (
	"context"
	"errors"
	"io"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Yo es el `sub` de quien usa el equipo en todas las cuentas: cada proveedor pone Yo en
// los mensajes propios, y la pantalla decide con él qué se puede editar o borrar.
const Yo = "yo"

// EventoCanales es lo que entrega Escuchar cuando la cuenta tiene conversaciones que no
// estaban en su lista (un chat de Teams o un espacio de Google Chat abierto en otro
// sitio). No llega a la pantalla: el Agregador llama a AlCambiarCanales, que vuelve a
// pedir la lista.
const EventoCanales = "office_channels_changed"

// ErrNoDisponible es lo que devuelve una función que el proveedor no tiene.
var ErrNoDisponible = errors.New("chat: no disponible en Sytue Office")

// ErrSalaCifrada: la sala va cifrada de extremo a extremo y esta edición no la lee.
var ErrSalaCifrada = errors.New("chat: esta sala está cifrada de extremo a extremo; Sytue Office no puede leerla ni escribir en ella")

// Proveedor es lo mínimo que da una cuenta. Los identificadores de canal y de mensaje son
// los PROPIOS del proveedor, sin el prefijo de la cuenta: eso lo pone el Agregador.
type Proveedor interface {
	// Rotulo es lo que ve la persona: «Notas», «@ana:matrix.org»…
	Rotulo() string
	// Canales son los canales con lo pendiente de cada uno.
	Canales(ctx context.Context) ([]api.RoomsChannel, []api.RoomsUnread, error)
	// Mensajes es una página EN ORDEN DE LECTURA (el más viejo primero) de los
	// anteriores a antes; antes cero es la más reciente. Sin las respuestas de hilo.
	Mensajes(ctx context.Context, canal string, antes time.Time, limite int) ([]api.RoomsMessage, error)
	// Enviar publica texto en canal; con raiz, como respuesta en su hilo.
	Enviar(ctx context.Context, canal, raiz, texto string) (api.RoomsMessage, error)
	MarcarLeido(ctx context.Context, canal string) error
	// Escuchar entrega lo que llega (tipos de `api.RoomsEvent`) hasta que el contexto
	// muera o la conexión caiga, y vuelve con el porqué.
	Escuchar(ctx context.Context, alLlegar func(api.RoomsEvent)) error
}

// Capacidades opcionales: lo que un proveedor no cumple, el Agregador lo contesta con
// ErrNoDisponible.

// Editor edita y borra mensajes.
type Editor interface {
	Editar(ctx context.Context, canal, mensaje, texto string) (api.RoomsMessage, error)
	Borrar(ctx context.Context, canal, mensaje string) error
}

// Reaccionador pone y quita reacciones.
type Reaccionador interface {
	Reaccionar(ctx context.Context, canal, mensaje, emoji string, poner bool) error
}

// Hilos da un hilo entero.
type Hilos interface {
	Hilo(ctx context.Context, canal, raiz string) (api.RoomsThread, error)
}

// Creador crea canales.
type Creador interface {
	CrearCanal(ctx context.Context, nombre, asunto string, privado bool) (api.RoomsChannel, error)
}

// Gestor cambia, archiva, borra y abandona canales.
type Gestor interface {
	CambiarCanal(ctx context.Context, canal string, c api.RoomsChannelPatch) (api.RoomsChannel, error)
	BorrarCanal(ctx context.Context, canal string) error
	Archivados(ctx context.Context) ([]api.RoomsChannel, error)
}

// Abandonador sale de un canal.
type Abandonador interface {
	Salir(ctx context.Context, canal string) error
}

// Miembros lista quién está en un canal.
type Miembros interface {
	Miembros(ctx context.Context, canal string) ([]api.RoomsMember, error)
}

// Buscador busca en los mensajes que tiene a mano.
type Buscador interface {
	Buscar(ctx context.Context, q string, limite int) ([]api.RoomsMessage, error)
}

// Directos abre (o reutiliza) una conversación directa con esas personas, por sus
// identificadores propios de la cuenta.
type Directos interface {
	AbrirDirecto(ctx context.Context, personas []string) (api.RoomsChannel, error)
}

// Personas es la gente que la cuenta conoce y la búsqueda de alguien por su
// identificador, su correo o su nombre.
type Personas interface {
	Gente(ctx context.Context) ([]api.RoomsMember, error)
	Resolver(ctx context.Context, q string) (api.RoomsPerson, error)
}

// Invitador mete (o invita) a alguien en un canal.
type Invitador interface {
	Anadir(ctx context.Context, canal, persona string) (api.RoomsMember, error)
}

// Explorador busca canales a los que unirse y se une.
type Explorador interface {
	Explorar(ctx context.Context, q string) ([]api.RoomsChannel, error)
	Unirse(ctx context.Context, canal string) (api.RoomsChannel, error)
}

// Fichero es un adjunto por mandar: lo que eligió la persona, ya copiado aparte.
type Fichero struct {
	Nombre, Tipo string
	Tamano       int64
	Abrir        func() (io.ReadCloser, error)
}

// Adjuntos manda ficheros con un mensaje y trae los de los mensajes. fichero es el
// `FileID` de un `api.RoomsAttachment` que dio la propia cuenta.
type Adjuntos interface {
	EnviarFicheros(ctx context.Context, canal, raiz, texto string, fs []Fichero) (api.RoomsMessage, error)
	Bajar(ctx context.Context, canal, fichero string, miniatura bool) (io.ReadCloser, error)
}

// HilosPropios son los hilos que la cuenta tiene a mano (en su caché o en disco) con
// respuestas: el Agregador se queda con los que escribió quien mira y los que sigue.
type HilosPropios interface {
	MisHilos(ctx context.Context) ([]api.RoomsThreadSummary, error)
}
