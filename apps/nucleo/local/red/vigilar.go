// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package red

import (
	"net/http"
	"sync/atomic"
)

// Intento es una conexión que alguien quiso abrir por el portero o por [Denegar], se
// permitiera o no: lo que ve [VigilarDePrueba].
type Intento struct {
	// Proposito con que se pidió; vacío si llegó a [Denegar] (un cliente que no pasa por
	// el portero: `http.DefaultTransport`, `api.Transporte`).
	Proposito Proposito
	// Red es «tcp» al marcar, o el esquema de la petición («https»).
	Red string
	// Direccion es `host:puerto` al marcar, o el host de la petición.
	Direccion string
}

var vigia atomic.Pointer[func(Intento)]

// VigilarDePrueba hace que f vea cada [Intento] de este proceso —por [Portero.Cliente],
// [Portero.Transporte], [Portero.Marcar], [Portero.Puntual] o [Denegar]— ANTES de que el
// portero decida si lo deja salir, y devuelve cómo dejar de vigilar. Es SOLO para la
// prueba de arranque de las apps de Sytue Office (`apps/fyne/local/prueba`): una app
// recién instalada no intenta ni una conexión (R-RED, TestRecienInstaladaNoAbreNingunaConexion).
// Ningún otro fichero que no sea de pruebas lo llama (TestLasPuertasDePruebaSoloEnPruebas).
func VigilarDePrueba(f func(Intento)) (dejar func()) {
	vigia.Store(&f)
	return func() { vigia.CompareAndSwap(&f, nil) }
}

func avisar(pr Proposito, red, direccion string) {
	if f := vigia.Load(); f != nil {
		(*f)(Intento{Proposito: pr, Red: red, Direccion: direccion})
	}
}

func avisarPeticion(pr Proposito, r *http.Request) {
	if r != nil && r.URL != nil {
		avisar(pr, r.URL.Scheme, r.URL.Host)
	}
}
