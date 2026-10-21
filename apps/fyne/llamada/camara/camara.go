// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package camara es la captura de imagen de la llamada nativa: la cámara y la pantalla
// del sistema, entregadas como fotogramas 4:2:0 (`*image.YCbCr`), que es lo que el
// codificador VP8 de `llamada` come sin convertir.
//
// Una implementación por sistema, cada una con su API de siempre:
//
//   - macOS e iOS: AVFoundation (camara_darwin.m).
//   - Linux: V4L2 por ioctl, en Go (camara_linux.go).
//   - Windows: Media Foundation (camara_windows.go).
//   - Android: Camera2, con su clase Java (eu.sytue.nativo.Camara).
//
// Y la pantalla, donde el sistema deja: ScreenCaptureKit en macOS, GDI en Windows y
// XGetImage en X11 (pantalla_*.go).
//
// Las fuentes se LEEN, no empujan: la llamada pide el último fotograma a su ritmo. Así
// ninguna API del sistema llama a Go desde sus hilos, y una fuente que va más deprisa
// que el codificador no llena ninguna cola: se pierden los fotogramas intermedios, que
// es lo que se quiere en una llamada.
package camara

import (
	"errors"
	"image"
)

// Fuente es una cámara (o una pantalla) abierta.
type Fuente interface {
	// Leer devuelve el último fotograma si hay uno NUEVO desde la lectura anterior.
	Leer() (*image.YCbCr, bool)
	// Cerrar la suelta (apaga la luz de la cámara). Se puede llamar más de una vez.
	Cerrar()
}

// Dispositivo es una cámara que se puede elegir.
type Dispositivo struct {
	ID, Nombre string
}

// Pedido es lo que se le pide a la cámara. El sistema da lo más parecido que tenga.
type Pedido struct {
	// ID es el dispositivo; vacío, el de por omisión.
	ID          string
	Ancho, Alto int
	Fotogramas  int
}

// PedidoPorOmision es 640×360 a 15 fps: la capa media de la simulcast de la web, que es
// la que una tesela pide. Una cámara que no la tenga da la más cercana.
var PedidoPorOmision = Pedido{Ancho: 640, Alto: 360, Fotogramas: 15}

var (
	// ErrSinCamara es no tener cámara, o no tener implementación en este sistema.
	ErrSinCamara = errors.New("camara: no hay cámara")
	// ErrSinPantalla es no poder compartir la pantalla en este sistema (móvil, Wayland).
	ErrSinPantalla = errors.New("camara: este sistema no deja compartir la pantalla desde aquí")
	// ErrSinPermiso es el permiso negado por el sistema o por la persona.
	ErrSinPermiso = errors.New("camara: sin permiso para usar la cámara")
)

// Abrir abre la cámara pedida.
func Abrir(p Pedido) (Fuente, error) { return abrir(p) }

// TextoAvisoPantalla es el texto, ya traducido, del aviso fijo que Android enseña mientras
// se comparte la pantalla. Lo pone quien monta la app.
var TextoAvisoPantalla = "Sytue Rooms"

// AbrirPantalla empieza a capturar la pantalla principal. Cada Leer hace una captura: la
// llamada las pide a 5 por segundo.
func AbrirPantalla() (Fuente, error) { return abrirPantalla() }

// Dispositivos son las cámaras que hay.
func Dispositivos() []Dispositivo { return dispositivos() }

// I420 crea la imagen 4:2:0 de un tamaño, con los lados pares que pide VP8.
func I420(ancho, alto int) *image.YCbCr {
	return image.NewYCbCr(image.Rect(0, 0, ancho&^1, alto&^1), image.YCbCrSubsampleRatio420)
}
