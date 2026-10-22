// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// # «No encontramos esta página», también en nativo
//
// Un enlace llega a una app por tres caminos —el esquema, un fichero, la instancia que ya
// estaba abierta— y dos veces de cada tres acababa en silencio cuando apuntaba a algo que
// no hay: la app se abría en su inicio, o se quedaba como estaba, sin decir nada. La web
// enseña su 404; aquí se enseña la misma página, que pinta `ui.PaginaNoEncontrada`.
//
// Sesion no pinta —`ui` depende de `sesion`, no al revés—: quien la pinta se apunta con
// [Suite.AlNoEncontrar], y es la barra de la suite, que ya está en cada `main`. Las tres
// situaciones en las que se enseña:
//
//  1. Un enlace de la suite que NINGÚN receptor de esta app atiende, una vez arrancada.
//  2. Una apertura de fichero con una ruta que la app no reconoce.
//  3. Una apertura cuyo recurso el servicio contesta con 404: el fichero se borró.
//
// Un 403 no es esto —es «sin permiso», y la web lo distingue—, y lo que la app nativa
// no sabe abrir sigue yendo a la web, como antes.

// AlNoEncontrar registra cómo enseña esta app su página «no encontrada». Se llama en el
// hilo de la interfaz.
func (s *Suite) AlNoEncontrar(f func()) {
	s.mu.Lock()
	s.noEncontrado = f
	s.mu.Unlock()
}

// RutaPropia apunta rutas que la app atiende con su propio [Suite.AlAbrirURL] —Diapos
// abre `/edit?id=…` del servicio— para que [Suite.AlAbrirFichero], que solo sabe de
// aperturas de fichero, no las tome por un destino que no existe.
func (s *Suite) RutaPropia(f func(*url.URL) bool) {
	s.mu.Lock()
	s.propias = append(s.propias, f)
	s.mu.Unlock()
}

func (s *Suite) esPropia(u *url.URL) bool {
	s.mu.Lock()
	propias := append([]func(*url.URL) bool{}, s.propias...)
	s.mu.Unlock()
	for _, f := range propias {
		if f(u) {
			return true
		}
	}
	return false
}

// NoEncontrado enseña la página «no encontrada» y dice si había quién la enseñara. Sin
// nadie apuntado (un binario sin la barra de la suite) no hace nada.
func (s *Suite) NoEncontrado() bool {
	s.mu.Lock()
	f := s.noEncontrado
	s.mu.Unlock()
	if f == nil {
		return false
	}
	fyne.Do(f)
	return true
}

// esRaiz dice si un enlace es solo «ábrete»: la raíz de la app, sin destino. Eso no es
// un destino que no exista.
func esRaiz(u *url.URL) bool {
	return strings.Trim(u.Path, "/") == "" && strings.Trim(u.Opaque, "/") == ""
}

// noEsta dice si un fallo es que el servicio NO TIENE lo pedido (404), que es lo único
// que lleva a la página «no encontrada». Un 403 es otra cosa, y un fallo de red, otra.
func noEsta(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}
