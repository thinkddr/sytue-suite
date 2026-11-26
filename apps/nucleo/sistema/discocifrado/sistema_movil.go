// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android || ios

package discocifrado

import (
	"context"
	"os/exec"
	"runtime"
)

func deSistema(context.Context, string) Resultado { return deMovil(runtime.GOOS) }

func ocultarVentana(*exec.Cmd) {}
