// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !js && !android && !ios && !linux && !(darwin && tienda)

package auth

// En macOS y Windows el llavero es del sistema y siempre está: no hay fichero de repuesto.

func sinLlavero() bool { return false }

func cargarFichero(...string) (Tokens, int64, error) { return Tokens{}, 0, nil }

func guardarFichero(Tokens, ...string) (bool, error) { return false, nil }

func borrarFichero(...string) error { return nil }
