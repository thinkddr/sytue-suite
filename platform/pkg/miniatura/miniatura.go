// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package miniatura mantiene la API de los servicios. El motor y su turno de
// memoria viven en el núcleo para compartirlos con los dispositivos autorizados.
package miniatura

import (
	"image"

	shared "git.sytue.eu/sytue/one/apps/nucleo/miniatura"
)

var ErrMedioNoSoportado = shared.ErrMedioNoSoportado

const (
	MaxPixeles  = shared.MaxPixeles
	MaxBytes    = shared.MaxBytes
	CalidadJPEG = shared.CalidadJPEG
)

func Coste(cfg image.Config) int64 {
	return shared.Coste(cfg)
}

func Cabe(cfg image.Config) bool {
	return shared.Cabe(cfg)
}

func Turno() func() {
	return shared.Turno()
}

// Cabecera es image.DecodeConfig con el modelo de color que de verdad cuesta
// decodificar (ver apps/nucleo/miniatura.Cabecera).
func Cabecera(datos []byte) (image.Config, string, error) {
	return shared.Cabecera(datos)
}

func Decodificar(datos []byte) (image.Image, error) {
	return shared.Decodificar(datos)
}

func Render(datos []byte, maxLado int, tipoSalida string) ([]byte, error) {
	return shared.Render(datos, maxLado, tipoSalida)
}

func Reducir(src image.Image, maxLado int) *image.NRGBA {
	return shared.Reducir(src, maxLado)
}

func Cascada(src image.Image, lados []int) map[int]*image.NRGBA {
	return shared.Cascada(src, lados)
}

func Dimensiones(sw, sh, maxLado int) (int, int) {
	return shared.Dimensiones(sw, sh, maxLado)
}

func TipoSalida(contentType string) (string, bool) {
	return shared.TipoSalida(contentType)
}
