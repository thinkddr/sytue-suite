// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js

package sesion

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/offline"
)

// ErrSoloLectura es lo que recibe quien intenta escribir con `SYTUE_SOLO_LECTURA`.
var ErrSoloLectura = errors.New("sesion: solo lectura (SYTUE_SOLO_LECTURA), la petición no se envía")

// soloLectura corta ANTES de la red todo lo que no sea leer. Es para las tiradas de
// ventana real (`scripts/ventana-real.sh`), que recorren las apps con la cuenta de
// auditoría contra producción pulsando en coordenadas fijas: el 2026-10-05 una
// coordenada desplazada cayó en «Nueva nota» y el recorrido creaba, por diseño, un
// documento de plantilla en cada editor. Con esto un clic puede acabar en cualquier
// botón y no se escribe nada.
//
// Deja pasar el POST al `/token` del IdP: el canje y la renovación de la sesión, sin los
// que no hay ni lectura.
//
// ponytail: solo HTTP. Un WebSocket ya abierto (el chat de Rooms, la edición de un
// documento) sigue pudiendo escribir, pero para eso hace falta TECLEAR, no un clic; si
// el recorrido llega a escribir dentro de un canal o de un documento, se corta aquí la
// petición de `Upgrade`.
type soloLectura struct{ base http.RoundTripper }

func (s soloLectura) RoundTrip(r *http.Request) (*http.Response, error) {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return s.base.RoundTrip(r)
	case http.MethodPost:
		if strings.HasSuffix(r.URL.Path, "/token") {
			return s.base.RoundTrip(r)
		}
	}
	return nil, ErrSoloLectura
}

var unaVezSoloLectura sync.Once

// ponerSoloLectura envuelve el transporte por defecto, que es por donde salen todos los
// clientes de la suite: los de `api` (http.DefaultClient), el de la caché «red primero»
// y el de la sesión. Lo llama Abrir.
func ponerSoloLectura() {
	if os.Getenv("SYTUE_SOLO_LECTURA") == "" {
		return
	}
	unaVezSoloLectura.Do(func() {
		http.DefaultTransport = soloLectura{http.DefaultTransport}
		slog.Warn("SYTUE_SOLO_LECTURA: esta app no envía ninguna escritura")
	})
}

// Conexion arma el cliente HTTP de una app con la caché «red primero» de la suite y el
// monitor que ella alimenta. Sin monitor la pantalla nunca sabe que no hay red: se queda
// en «no se pudo cargar» y deja intentar mutaciones.
//
// Nació en `cmd/drive` y sube aquí porque es de todas las apps, como la sesión: lo único
// que cambia de una a otra es el producto y qué rutas NO se guardan (`privadas`). Lo que
// se guarda es lo que la web guarda (las reglas de `worker.ts`), por persona y en la
// caché del sistema. En Drive, el CONTENIDO de los ficheros, las miniaturas y los ZIP no:
// son descargas, no listados, y guardarlos sería copiar la unidad entera sin que nadie
// lo pida.
func (s *Suite) Conexion(producto string, privadas ...string) (*offline.Monitor, *http.Client) {
	red := &offline.Monitor{}
	t := &offline.Transport{
		Base:          api.Transporte, // con plazo (FYS-11)
		Owner:         s.dueñoCache,
		Product:       producto,
		Monitor:       red,
		Private:       privadas,
		CacheProducts: api.Productos(),
		Scope:         s.scopeCache,
	}
	var err error
	t.Store, err = Almacen()
	if err != nil {
		// Sin almacén no hay caché ni, por tanto, dueño al que guardarla: el transporte
		// deja pasar todo tal cual y el monitor se queda en «hay red».
		slog.Warn("sin caché para trabajar sin conexión", "producto", producto, "error", err)
		t.Owner = nil
	}
	return red, &http.Client{Transport: t}
}

// scopeCache rejects the previous session before the cross-app watcher adopts it.
// It contains no token and is checked only around fallback/persistence, not per byte.
func (s *Suite) scopeCache() string {
	s.mu.Lock()
	generation := s.generacionSesion
	s.mu.Unlock()
	if generation == "" || ajustes.Leer().Sesion != generation {
		return ""
	}
	return generation
}

// Almacen abre el almacén local de la suite, en la caché del sistema: el mismo que
// usa la caché «red primero» de Conexion, y donde una app guarda lo que no puede
// perder sin red (los borradores y la cola de capturas de Notepads).
func Almacen() (*offline.Store, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	return offline.Open(filepath.Join(dir, "sytue", "offline"))
}

// vaciarAlmacen quita del disco, al cerrar sesión, TODO lo que las apps guardaron para
// trabajar sin conexión, de todas las cuentas que hayan pasado por este equipo: la caché
// de respuestas y también la cola y los borradores de Notepads, de los que ya se ha
// avisado antes de salir (NAT-08, ver notepads.go). Es lo que hace la web.
func (s *Suite) vaciarAlmacen() {
	alm, err := Almacen()
	if err == nil {
		err = alm.Clear()
	}
	if err != nil {
		slog.Warn("sesion: la caché sin conexión no se ha podido vaciar al cerrar sesión", "error", err)
	}
}
