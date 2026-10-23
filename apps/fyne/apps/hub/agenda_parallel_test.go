// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"context"
	"testing"
	"time"
)

type blockedAgenda struct {
	started chan string
	release chan struct{}
}

func (a *blockedAgenda) Hoy(context.Context, time.Time, time.Time) ([]EventoDeHoy, error) {
	a.started <- "calendar"
	<-a.release
	return nil, nil
}

func (a *blockedAgenda) SinLeer(context.Context) (int, error) {
	a.started <- "mail"
	<-a.release
	return 7, nil
}

func TestAgendaIndependentReadsOverlap(t *testing.T) {
	a := &blockedAgenda{started: make(chan string, 2), release: make(chan struct{})}
	v := NuevaVista(traductor(t), nil, time.Now()).ConAgenda(a)
	done := make(chan struct{})
	go func() { v.cargarAgenda(); close(done) }()
	for range 2 {
		select {
		case <-a.started:
		case <-time.After(time.Second):
			t.Error("one service prevented the other independent request from starting")
		}
	}
	close(a.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("agenda did not finish after both services were released")
	}
}
