// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package nube

import "context"

func conectar(context.Context, string, Fuente) error { return ErrNoDisponible }

func registrada(string) bool { return false }

func desconectar(string) error { return ErrNoDisponible }
