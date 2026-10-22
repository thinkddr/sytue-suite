// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !(linux || darwin || windows) || android || ios

package escritorio

func atajo(string, string, Combinacion, func()) (func(), error) { return nil, ErrNoDisponible }
