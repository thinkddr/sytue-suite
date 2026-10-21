// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (linux || darwin || windows) && !android && !ios && !wasm && !test_web_driver

package soltar

import (
	"maps"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"

	"github.com/go-gl/glfw/v3.4/glfw"
)

// La retrollamada del fork de GLFW es UNA para todas sus ventanas, porque Fyne no deja
// ver su `*glfw.Window`: aquí se reconoce la ventana por su identificador nativo, el
// mismo que da `RunNative`.
//
// ponytail: con la retrollamada puesta, TODAS las ventanas del proceso aceptan texto,
// aunque solo algunas lo usen; la que no lo pidió lo recibe y lo tira. Hoy cada app
// tiene una ventana, salvo la de presentar de Diapos. Si molesta, el fork puede dar la
// retrollamada por ventana y buscar la `*glfw.Window` por su identificador nativo.
var (
	mu        sync.Mutex
	ventanas  = map[fyne.Window]func(Soltado){}
	arrastres = map[fyne.Window]func(bool){}
)

func alSoltar(w fyne.Window, f func(Soltado)) {
	mu.Lock()
	if f == nil {
		delete(ventanas, w)
	} else {
		ventanas[w] = f
	}
	ninguna := len(ventanas) == 0
	mu.Unlock()
	if ninguna {
		glfw.SetDropTextCallback(nil)
		return
	}
	glfw.SetDropTextCallback(recibir)
}

// recibir corre dentro de `glfw.PollEvents`, en el hilo de la interfaz, igual que la
// retrollamada de ficheros de Fyne.
func recibir(gw *glfw.Window, x, y float64, texto string, urls []string) {
	mu.Lock()
	copia := maps.Clone(ventanas)
	mu.Unlock()
	for w, f := range copia {
		if !mismaVentana(w, gw) {
			continue
		}
		// Las mismas cuentas que hace Fyne con el puntero (`processMouseMoved`): la
		// coordenada entera de GLFW entre la escala del lienzo.
		e := w.Canvas().Scale()
		f(Soltado{Texto: texto, URLs: urls, Posicion: fyne.NewPos(float32(int(x))/e, float32(int(y))/e)})
		return
	}
}

func alArrastrar(w fyne.Window, f func(bool)) {
	mu.Lock()
	if f == nil {
		delete(arrastres, w)
	} else {
		arrastres[w] = f
	}
	ninguna := len(arrastres) == 0
	mu.Unlock()
	if ninguna {
		glfw.SetDragCallback(nil)
		return
	}
	glfw.SetDragCallback(recibirArrastre)
}

// recibirArrastre corre, como recibir, dentro de `glfw.PollEvents`.
func recibirArrastre(gw *glfw.Window, encima bool) {
	mu.Lock()
	copia := maps.Clone(arrastres)
	mu.Unlock()
	for w, f := range copia {
		if mismaVentana(w, gw) {
			f(encima)
			return
		}
	}
}

func mismaVentana(w fyne.Window, gw *glfw.Window) (igual bool) {
	nw, ok := w.(driver.NativeWindow)
	if !ok {
		return false
	}
	nw.RunNative(func(c any) { igual = mismoContexto(c, gw) })
	return igual
}
