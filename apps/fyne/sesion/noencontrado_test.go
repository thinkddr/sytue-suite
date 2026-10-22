// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func enlace(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// nada comprueba que en un rato prudencial no se ha enseñado la página.
func nada(t *testing.T, c <-chan string, que string) {
	t.Helper()
	if _, ok := recibir(c, 200*time.Millisecond); ok {
		t.Errorf("%s ha enseñado la página «no encontrada»", que)
	}
}

// TestLoQueNoEstaEnsenaLaPaginaNoEncontrada son las tres situaciones de la 404 nativa y
// las dos que NO lo son. Antes las tres acababan en silencio (el inicio, o nada) o en el
// navegador, que diría lo mismo. VISTO EN ROJO quitando cada una de las tres ramas: el
// caso correspondiente no llega en dos segundos.
func TestLoQueNoEstaEnsenaLaPaginaNoEncontrada(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	espia := nuevaEspia(a)
	s.app = espia
	vista := make(chan string, 8)
	s.AlNoEncontrar(func() { vista <- "404" })
	estado := http.StatusNotFound
	s.AlAbrirFichero("slides", func(context.Context, api.Apertura) error {
		return &api.Error{Status: estado}
	})

	// 1. El servicio dice que el fichero no está: la página, y NO el navegador.
	s.recibirURL(enlace(t, "sytue-slides:/open-from-drive?file=borrado"))
	esperar(t, vista)
	espia.nadaEnLaWeb(t)

	// 2. Una ruta que la app no reconoce.
	s.recibirURL(enlace(t, "sytue-slides:/esto-no-existe"))
	esperar(t, vista)

	// Una ruta que la app atiende por su cuenta (RutaPropia) tampoco lo es.
	s.RutaPropia(func(u *url.URL) bool { return u.Path == "/edit" })
	s.recibirURL(enlace(t, "sytue-slides:/edit?id=p1"))
	nada(t, vista, "una ruta propia de la app")

	// La raíz es «ábrete», no un destino.
	s.recibirURL(enlace(t, "sytue-slides:/"))
	nada(t, vista, "la raíz de la app")

	// Un 403 NO es «no encontrado»: sigue a la web, que distingue «sin permiso».
	estado = http.StatusForbidden
	s.recibirURL(enlace(t, "sytue-slides:/open-from-drive?file=ajeno"))
	esperar(t, espia.llega)
	nada(t, vista, "un 403")
}

// TestUnEnlaceSinReceptorEsUnDestinoQueNoHay: una app que no atiende enlaces (Mail,
// Notepads, Calendar…) recibe uno de la suite, de un aviso o de otra app. Antes de
// arrancar se guarda —su receptor puede no haberse apuntado todavía—; después, ya no
// va a recogerlo nadie.
func TestUnEnlaceSinReceptorEsUnDestinoQueNoHay(t *testing.T) {
	aparte(t)
	a := test.NewApp()
	defer a.Quit()

	s := Abrir(a, "sytue.eu")
	vista := make(chan string, 4)
	s.AlNoEncontrar(func() { vista <- "404" })

	s.recibirURL(enlace(t, "sytue-notes:/notebooks/n-borrado"))
	nada(t, vista, "un enlace antes de arrancar")

	s.mu.Lock()
	s.arrancada = true
	s.mu.Unlock()
	s.recibirURL(enlace(t, "sytue-notes:/notebooks/otro"))
	esperar(t, vista)
	s.recibirURL(enlace(t, "sytue-notes:"))
	nada(t, vista, "la raíz de la app")
}
