// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && !ios

package sistema

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func menosMovimiento() bool {
	ctx, parar := context.WithTimeout(context.Background(), 2*time.Second)
	defer parar()
	salida, err := exec.CommandContext(ctx, "defaults", "read", "com.apple.universalaccess", "reduceMotion").Output()
	return err == nil && strings.TrimSpace(string(salida)) == "1"
}
