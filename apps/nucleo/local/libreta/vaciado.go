// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package libreta

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// # Lo eliminado para siempre se va del disco
//
// «Eliminar para siempre» borra las filas del contacto, y la base (`secure_delete`)
// sobrescribe con ceros lo que ocupaban. Pero en modo WAL la página de antes sigue en el
// diario (`libreta.sqlite-wal`) hasta que se vuelca a la base y se vacía: eso se hace
// poco después de cada eliminación (vaciarPronto: vaciar la papelera son muchas seguidas
// y se juntan en una), sin esperar a cerrar la app; al cerrar, SQLite lo vacía solo. Si
// otra app está leyendo justo entonces, el diario no se puede vaciar del todo y se
// vuelve a intentar.

// EsperaVaciar es cuánto se espera tras eliminar para vaciar el diario; las pruebas la
// acortan.
var EsperaVaciar = 500 * time.Millisecond

// vaciado es el vaciado pendiente del diario.
type vaciado struct {
	mu      sync.Mutex
	reloj   *time.Timer
	parado  bool
	intento int
}

// vaciarPronto vacía el diario dentro de EsperaVaciar (una sola vez para varias
// eliminaciones seguidas).
func (x *Libreta) vaciarPronto(ctx context.Context) {
	ctx = context.WithoutCancel(ctx) // va después de la eliminación que lo pidió
	x.vaciado.mu.Lock()
	defer x.vaciado.mu.Unlock()
	if x.vaciado.parado {
		return
	}
	if x.vaciado.reloj != nil {
		x.vaciado.reloj.Stop()
	}
	x.vaciado.reloj = time.AfterFunc(EsperaVaciar, func() {
		if !x.vaciar(ctx) {
			x.reintentarVaciar(ctx)
		}
	})
}

// reintentarVaciar vuelve a intentarlo, unas pocas veces, si alguien leía.
func (x *Libreta) reintentarVaciar(ctx context.Context) {
	x.vaciado.mu.Lock()
	defer x.vaciado.mu.Unlock()
	if x.vaciado.parado || x.vaciado.intento >= 20 {
		x.vaciado.intento = 0
		return
	}
	x.vaciado.intento++
	x.vaciado.reloj = time.AfterFunc(EsperaVaciar, func() {
		if !x.vaciar(ctx) {
			x.reintentarVaciar(ctx)
		}
	})
}

func (v *vaciado) parar() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.parado = true
	if v.reloj != nil {
		v.reloj.Stop()
	}
}

// vaciar vuelca el diario a la base y lo deja vacío. Dice si lo consiguió del todo.
func (x *Libreta) vaciar(ctx context.Context) bool {
	if x.db == nil {
		return true
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	var ocupado, paginas, volcadas int
	if err := x.db.QueryRowContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&ocupado, &paginas, &volcadas); err != nil {
		slog.Debug("libreta: no se pudo vaciar el diario", "error", err)
		return false
	}
	return ocupado == 0
}
