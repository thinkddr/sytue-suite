// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package lector

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Caducar hace como si ya hubiera pasado RefrescoLibreta desde la última lectura.
func Caducar(b *Libreta) {
	b.mu.Lock()
	b.visto = time.Time{}
	b.mu.Unlock()
}

// EsperarRelectura espera a que acabe la relectura en curso, si hay una.
func EsperarRelectura(b *Libreta) {
	b.mu.Lock()
	leyendo := b.leyendo
	b.mu.Unlock()
	if leyendo != nil {
		<-leyendo
	}
}

// FrenarLectura hace que, durante la prueba, cada lectura de la libreta entera espere a
// que se cierre suelta, y cuenta cuántas empiezan.
func FrenarLectura(t *testing.T, suelta <-chan struct{}) *atomic.Int32 {
	t.Helper()
	antes := leerTodo
	var n atomic.Int32
	leerTodo = func(l *Lector, ctx context.Context) ([]api.Contact, error) {
		n.Add(1)
		<-suelta
		return antes(l, ctx)
	}
	t.Cleanup(func() { leerTodo = antes })
	return &n
}
