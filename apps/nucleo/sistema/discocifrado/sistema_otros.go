// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !linux && !darwin && !windows

package discocifrado

import (
	"context"
	"os/exec"
)

// En los demás sistemas (FreeBSD…) no se sabe.
func deSistema(context.Context, string) Resultado { return noSeSabe }

func ocultarVentana(*exec.Cmd) {}
