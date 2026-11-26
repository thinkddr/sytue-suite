// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// # La vuelta por el esquema de la app (móvil)
//
// En el escritorio, el navegador vuelve con el código a `http://127.0.0.1:<puerto>/oauth`,
// donde la app escucha mientras dura el alta (RFC 8252 §7.3). En iOS y Android eso no
// sirve: la app pasa a segundo plano mientras la persona está en el navegador, y el
// sistema la duerme (iOS) o puede hacerlo (Android). Allí la vuelta es por el esquema
// propio de la app (RFC 8252 §7.1, PLAN-EDICION-LOCAL §5.10): `redirect_uri` es
// `sytueoffice-<producto>:/oauth`, el sistema reabre la app con ese enlace y la cáscara
// se lo da a [Vuelta.Recibir] antes que a nadie (`l.AlAbrirEnlace` no lo ve).
//
// Un enlace del esquema lo puede abrir cualquiera (una web, otra app), así que:
//
//   - solo se acepta la vuelta de un alta EN MARCHA, por su `state` (32 bytes al azar);
//     un enlace con otro `state`, o sin alta en marcha, se descarta y no hace nada;
//   - cada `state` vale UNA vez: la segunda vuelta con el mismo se descarta;
//   - el alta caduca (por defecto a los 10 minutos): una vuelta tardía se descarta;
//   - el código solo sirve con el verificador PKCE (S256) que no sale de la app, y el
//     servidor de tokens lo comprueba al canjearlo.
//
// Google y Microsoft exigen registrar ese `redirect_uri` en el cliente OAuth (lo hace el
// dueño del cliente, PLAN §11): Microsoft Entra lo admite en «Aplicaciones móviles y de
// escritorio»; Google solo admite en sus clientes de iOS y Android el esquema del
// identificador del paquete, así que con Google la vuelta por esquema necesita un cliente
// propio de cada app y ese esquema registrado en el paquete (pendiente).

// RutaVuelta es la ruta del enlace de vuelta: `<esquema>:/oauth`.
const RutaVuelta = "/oauth"

// CaducidadVuelta es cuánto espera un alta a que vuelva el navegador.
const CaducidadVuelta = 10 * time.Minute

// Vuelta recoge, por el esquema de la app, la vuelta del navegador de las altas OAuth en
// marcha.
type Vuelta struct {
	// Redireccion es el `redirect_uri` que se manda: `<esquema>:/oauth`.
	Redireccion string
	// Caducidad es cuánto espera un alta; cero, CaducidadVuelta.
	Caducidad time.Duration

	esquema string
	mu      sync.Mutex
	esperas map[string]chan url.Values
}

// NuevaVuelta prepara la vuelta por el esquema de la app (`sytueoffice-docs`).
func NuevaVuelta(esquema string) *Vuelta {
	esquema = strings.ToLower(strings.TrimSuffix(esquema, ":"))
	return &Vuelta{Redireccion: esquema + ":" + RutaVuelta, esquema: esquema, esperas: map[string]chan url.Values{}}
}

// ErrVueltaCaducada: el navegador no ha vuelto a tiempo.
var ErrVueltaCaducada = errors.New("nube: el navegador no ha vuelto a tiempo; vuelve a intentarlo")

// esSuya dice si u es un enlace de vuelta de este esquema: `<esquema>:/oauth?…` (o
// `<esquema>://oauth?…`, que algunos navegadores escriben así).
func (v *Vuelta) esSuya(u *url.URL) bool {
	if u == nil || !strings.EqualFold(u.Scheme, v.esquema) {
		return false
	}
	return (u.Host == "" && u.Path == RutaVuelta) || (u.Host == strings.TrimPrefix(RutaVuelta, "/") && (u.Path == "" || u.Path == "/"))
}

// Recibir da a la vuelta un enlace con el que el sistema ha abierto la app. Devuelve true
// si era un enlace de vuelta de OAuth (aunque no sea de ningún alta en marcha: se
// descarta), y entonces no es un enlace para la app.
func (v *Vuelta) Recibir(u *url.URL) bool {
	if !v.esSuya(u) {
		return false
	}
	q := u.Query()
	estado := q.Get("state")
	v.mu.Lock()
	llega, ok := v.esperas[estado]
	if ok {
		// Un solo uso: lo que llegue después con el mismo state, fuera.
		delete(v.esperas, estado)
	}
	v.mu.Unlock()
	if !ok || estado == "" {
		slog.Info("nube: vuelta de OAuth sin alta en marcha; se descarta")
		return true
	}
	llega <- q
	return true
}

// esperar apunta un alta con su state, llama a abrir (el navegador) y espera la vuelta,
// hasta que llega, caduca o se cancela ctx.
func (v *Vuelta) esperar(ctx context.Context, estado string, abrir func() error) (url.Values, error) {
	llega := make(chan url.Values, 1)
	v.mu.Lock()
	v.esperas[estado] = llega
	v.mu.Unlock()
	defer func() {
		v.mu.Lock()
		delete(v.esperas, estado)
		v.mu.Unlock()
	}()
	if err := abrir(); err != nil {
		return nil, fmt.Errorf("nube: no se pudo abrir el navegador: %w", err)
	}
	caduca := v.Caducidad
	if caduca <= 0 {
		caduca = CaducidadVuelta
	}
	reloj := time.NewTimer(caduca)
	defer reloj.Stop()
	select {
	case q := <-llega:
		return q, nil
	case <-reloj.C:
		return nil, ErrVueltaCaducada
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pendientes es cuántas altas esperan su vuelta (para las pruebas).
func (v *Vuelta) pendientes() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.esperas)
}

// vueltaDeLaApp es la vuelta por esquema de esta app, si la hay (en el móvil).
var vueltaDeLaApp atomic.Pointer[Vuelta]

// UsarVuelta hace que las altas OAuth de esta app (la nube, Calendar, Contacts, Mail y
// Rooms: todas van por OAuth.Conectar) vuelvan por el esquema de la app en vez de por
// 127.0.0.1. La pone la cáscara en iOS y Android; nil la quita.
func UsarVuelta(v *Vuelta) { vueltaDeLaApp.Store(v) }

// VueltaDeLaApp es la que puso UsarVuelta, o nil.
func VueltaDeLaApp() *Vuelta { return vueltaDeLaApp.Load() }
