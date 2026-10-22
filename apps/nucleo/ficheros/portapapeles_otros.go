// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build js || (!darwin && !linux && !windows && !android && !ios)

package ficheros

func copiarImagen(string) error { return ErrNoDisponible }
