// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	privateHub "git.sytue.eu/sytue/one/apps/nucleo/hub"
	"git.sytue.eu/sytue/one/apps/nucleo/ingress"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

// receptorDoble guarda un aviso en la primera vuelta y espera hasta que lo paren.
type receptorDoble struct{ parado atomic.Bool }

func (r *receptorDoble) Run(ctx context.Context, changed func(ingress.PollResult, error)) error {
	changed(ingress.PollResult{Attempted: 1, Completed: 1}, nil)
	<-ctx.Done()
	r.parado.Store(true)
	return ctx.Err()
}

// El receptor de la vista recarga la bandeja al guardar un aviso y se para al
// desmontar (cerrar sesión o cambiar de cuenta): Join espera a que termine.
func TestElReceptorRecargaYSeParaAlDesmontar(t *testing.T) {
	receptor := &receptorDoble{}
	recargas := make(chan struct{}, 1)
	stop, join := runHubReceiver(t.Context(), receptor, func() { recargas <- struct{}{} })
	select {
	case <-recargas:
	case <-time.After(5 * time.Second):
		t.Fatal("guardar un aviso no recargó la bandeja")
	}
	stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := join(ctx); err != nil || !receptor.parado.Load() {
		t.Fatal("desmontar no paró el receptor", err)
	}
}

// Sin receptor (el plan no lo creó) montar no arranca nada ni bloquea al desmontar.
func TestSinReceptorNoHayNadaQueParar(t *testing.T) {
	receivers := &hubReceivers{}
	repo := &privateHub.Repository{}
	stop, join := runHubReceiver(t.Context(), receivers.get(repo), nil)
	stop()
	if err := join(t.Context()); err != nil {
		t.Fatal(err)
	}
	receivers.put(repo, &privateHub.Receiver{})
	if receivers.get(repo) == nil {
		t.Fatal("no se encuentra el receptor guardado")
	}
	receivers.drop(repo)
	if receivers.get(repo) != nil {
		t.Fatal("retirar el plan no soltó el receptor")
	}
}

func TestLaEpocaDeLaFuente(t *testing.T) {
	legacy := storage.AppMigrationState{LegacyMigrationState: storage.LegacyMigrationState{Mode: "legacy"}}
	congelada := storage.AppMigrationState{LegacyMigrationState: storage.LegacyMigrationState{Mode: "frozen", Epoch: "0b5c7c2e-1f7a-4d43-8d0e-5a3c9b1e2f40"}}
	if hubEpoch(legacy) != "" || hubEpoch(congelada) != congelada.Epoch {
		t.Fatal(hubEpoch(legacy), hubEpoch(congelada))
	}
}
