// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build (darwin && !ios) || (linux && !android) || windows

package voz

import (
	"context"
	"errors"
	"os/exec"
	"sync"
)

// leerCon arranca la orden que lee y no la espera. Callar la mata y después corre
// `despues`, si lo hay (Speech Dispatcher sigue hablando aunque muera quien se lo pidió).
func leerCon(orden *exec.Cmd, alAcabar, despues func()) (func(), error) {
	if err := orden.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, ErrNoDisponible
		}
		return nil, err
	}
	go func() {
		_ = orden.Wait()
		if alAcabar != nil {
			alAcabar()
		}
	}()
	var una sync.Once
	return func() {
		una.Do(func() {
			_ = orden.Process.Kill()
			if despues != nil {
				despues()
			}
		})
	}, nil
}

// orden es una orden del sistema sin plazo: dura lo que dure la lectura.
func orden(nombre string, args ...string) *exec.Cmd {
	return exec.CommandContext(context.Background(), nombre, args...) //nolint:gosec // G204: órdenes fijas del sistema; el texto va por la entrada, no por la línea de órdenes
}
