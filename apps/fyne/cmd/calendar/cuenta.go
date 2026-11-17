// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"time"
)

// pedirDatosCuenta belongs to one session generation. Its caller cancels vida
// on identity change; queued UI callbacks recheck it when they execute.
func pedirDatosCuenta(vida context.Context, vigente func() bool, correo func(context.Context) (string, error), gestor func(context.Context) (bool, error),
	despachar func(func()), aplicarCorreo func(string), aplicarGestor func(bool),
) {
	go func() {
		ctx, cancelar := context.WithTimeout(vida, 15*time.Second)
		defer cancelar()
		if valor, err := correo(ctx); err == nil {
			despacharVigente(vida, despachar, func() {
				if vigente() {
					aplicarCorreo(valor)
				}
			})
		}
		if ctx.Err() != nil {
			return
		}
		if valor, err := gestor(ctx); err == nil {
			despacharVigente(vida, despachar, func() {
				if vigente() {
					aplicarGestor(valor)
				}
			})
		}
	}()
}

func despacharVigente(vida context.Context, despachar func(func()), aplicar func()) {
	if vida.Err() != nil {
		return
	}
	despachar(func() {
		if vida.Err() == nil {
			aplicar()
		}
	})
}
