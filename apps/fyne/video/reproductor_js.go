// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js

package video

import (
	"errors"
	"image"
	"io"
	"time"

	"git.sytue.eu/sytue/one/apps/fyne/llamada"
)

// Reproductor en `js` existe solo para que `make fyne-cmd-check` tipe Sytue Cast sin
// CGO (libvpx y libopus no compilan a wasm). Nuevo siempre falla, así que ningún otro
// método llega a llamarse.
type Reproductor struct {
	AlFotograma func(img image.Image, t time.Duration)
	AlCambiar   func()
}

// Nuevo en `js` no tiene descodificador con el que reproducir.
func Nuevo(io.ReadSeeker, llamada.Dispositivo) (*Reproductor, error) {
	return nil, errors.New("video: sin reproductor en js")
}

func (r *Reproductor) Arrancar()                 {}
func (r *Reproductor) Duracion() time.Duration   { return 0 }
func (r *Reproductor) Tamano() (ancho, alto int) { return 0, 0 }
func (r *Reproductor) Posicion() time.Duration   { return 0 }
func (r *Reproductor) Pausado() bool             { return true }
func (r *Reproductor) Terminado() bool           { return false }
func (r *Reproductor) Err() error                { return nil }
func (r *Reproductor) Reproducir()               {}
func (r *Reproductor) Pausar()                   {}
func (r *Reproductor) Alternar()                 {}
func (r *Reproductor) Buscar(time.Duration)      {}
func (r *Reproductor) Volumen(float64)           {}
func (r *Reproductor) Silenciar(bool)            {}
func (r *Reproductor) Silenciado() bool          { return false }
func (r *Reproductor) Cerrar()                   {}
