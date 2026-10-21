// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(linux || darwin || windows) || android || ios || (darwin && tienda)

package escritorio

import "context"

// En el móvil las apps se actualizan por su tienda, y en la Mac App Store también
// (etiqueta `tienda`, ver tienda.go): ahí el montaje del DMG ni se compila.
func instalar(context.Context, string) error { return ErrNoDisponible }

func hermanasDe(string) []Hermana { return nil }

func instalarHermana(context.Context, string, string) error { return ErrNoDisponible }
