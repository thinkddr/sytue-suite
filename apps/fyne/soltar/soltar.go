// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package soltar entrega a una ventana el TEXTO y los ENLACES que se sueltan sobre ella.
//
// Fyne solo entrega ficheros (`Window.SetOnDropped`), porque GLFW solo los acepta: su
// XDnD de X11 y su `wl_data_device` de Wayland piden `text/uri-list` y nada más, y en
// macOS y Windows se registran solo para ficheros. La web, en cambio, recibe
// `text/plain` y `text/uri-list` en cualquier `ondrop` (el cuaderno de Notepads crea
// con eso una nota de texto o de enlace).
//
// Por eso `apps/fyne/go.mod` reemplaza go-gl/glfw por un fork con un parche en su C
// (`apps/fyne/upstream/README.md`): una ventana con retrollamada de texto acepta
// también texto y URL que no son ficheros. Los ficheros NO pasan por aquí: siguen
// llegando por `SetOnDropped`, como siempre, y por eso esta API se combina con la de
// ficheros (`sesion.AceptarFicheros`, el soltar de Drive o de Photos) sin pisarla.
package soltar

import (
	"strings"

	"fyne.io/fyne/v2"
)

// Soltado es lo que se ha soltado sobre la ventana y no es un fichero. Solo uno de los
// dos campos viene lleno: Texto si se arrastró texto, URLs si se arrastró una lista de
// URI (un enlace del navegador, un marcador) sin ningún fichero dentro.
type Soltado struct {
	Texto string
	URLs  []string
	// Posicion es la del puntero al soltar, en coordenadas del lienzo de la ventana: las
	// mismas que el `fyne.Position` de `SetOnDropped`.
	Posicion fyne.Position
}

// ComoTexto es lo soltado como texto que escribir en un campo: el texto, o los enlaces,
// uno por línea, que es lo que escribe el navegador al soltar un enlace en un `textarea`.
func (s Soltado) ComoTexto() string {
	if s.Texto != "" {
		return s.Texto
	}
	return strings.Join(s.URLs, "\n")
}

// AlSoltar hace que `w` acepte texto y enlaces soltados y se los pase a `f`, en el hilo
// de la interfaz. Con nil deja de aceptarlos. Una ventana tiene una sola `f`: la última
// gana, como en `SetOnDropped`.
//
// Se llama desde el hilo de la interfaz: en `main`, antes de `ShowAndRun`, o dentro de
// `fyne.Do`.
func AlSoltar(w fyne.Window, f func(Soltado)) { alSoltar(w, f) }

// AlArrastrar avisa a `f` cuando un arrastre de FICHEROS entra en `w` (true) y cuando
// deja de estar encima (false): porque sale, porque se cancela o porque se suelta, y en
// ese caso antes de que los ficheros lleguen a `SetOnDropped`. Es el `dragover` y el
// `dragleave` con los que la web enciende «Suelta para añadir». Llega en el hilo de la
// interfaz y cada cambio una sola vez. Con nil deja de avisar; una ventana tiene una sola
// `f`, la última.
//
// Solo cuentan los ficheros: un texto o un enlace arrastrado no avisan, salvo en X11,
// donde una lista de URI se anuncia antes de saber si trae ficheros o enlaces.
//
// ponytail: en Wayland no avisa (el fork de GLFW no lo lleva ahí), ni en una ventana de
// Windows cuyo hilo ya estuviera en el apartamento multihilo de COM, ni en el móvil.
// Quien lo use no puede depender de él para aceptar lo soltado: solo para enseñarlo.
func AlArrastrar(w fyne.Window, f func(encima bool)) { alArrastrar(w, f) }
