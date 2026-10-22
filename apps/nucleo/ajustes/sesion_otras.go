// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !android

package ajustes

import "os"

func estadoPlataforma(e Estado) Estado            { return e }
func abrirCerrojoSesion() (*os.File, bool, error) { return nil, false, nil }
