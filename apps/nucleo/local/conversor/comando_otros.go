// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !windows

package conversor

import "os/exec"

// ocultarVentana no hace nada fuera de Windows: no hay ventana de consola que esconder.
func ocultarVentana(*exec.Cmd) {}
