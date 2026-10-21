// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package sesion

// ImagenEnImagen fuera de Android no existe: el escritorio abre su propia ventana y en
// iOS el reproductor no es el del sistema.
var ImagenEnImagen imagenEnImagen = sinPiP{}

type sinPiP struct{}

func (sinPiP) Disponible() bool        { return false }
func (sinPiP) Activa() bool            { return false }
func (sinPiP) Preparar(int, int, bool) {}
func (sinPiP) Entrar(bool) bool        { return false }

// AlSucesoPiP no tiene nada que atender fuera de Android.
func AlSucesoPiP(func(que int)) {}
