// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ui

// Espejo de `enTandas` de `@sytue/ui` (packages/ui/src/lib/enTandas.ts).

import "sync"

// # Lo que el servicio no hace en lote
//
// Deshacer un envío a la papelera de varias cosas, o vaciarla: los servicios de la suite
// restauran y purgan POR ID, y cada llamada pasa por su propia autorización, así que lo
// que se niega se queda donde estaba y se cuenta aparte. Lo usan Drive y Fotos; vive aquí
// porque la plomería se importa, no se copia.

// EnParalelo es cuántas llamadas van a la vez (`EN_PARALELO` de la web): las justas para
// no esperar en fila ni saturar el servicio con cien peticiones de golpe.
const EnParalelo = 4

// EnTandas hace `hacer` con cada elemento, de `enParalelo` en `enParalelo`, y separa lo que
// salió de lo que no. Un fallo no corta el resto: lo que se pudo, se hizo; lo que no, se
// dice. Bloquea hasta que terminan todas.
func EnTandas[T any](items []T, hacer func(T) error, enParalelo int) (hechos, fallidos []T) {
	if enParalelo < 1 {
		enParalelo = 1
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	cupo := make(chan struct{}, enParalelo)
	for _, x := range items {
		cupo <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() { <-cupo; wg.Done() }()
			err := hacer(x)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fallidos = append(fallidos, x)
			} else {
				hechos = append(hechos, x)
			}
		}()
	}
	wg.Wait()
	return hechos, fallidos
}
