// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux

package sesion

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/documentos"
)

func tokensAperturaDrive() auth.Tokens {
	return auth.Tokens{Issuer: "https://id.a.example", Cliente: "native", Subject: "ana", Organizacion: "empresa", Access: "synthetic-a", Refresh: "synthetic-refresh", Expiry: time.Now().Add(time.Hour)}
}

func TestAperturaDriveConIntencionNoAdoptaOtraCuentaNiInstancia(t *testing.T) {
	a := auth.NewSession(tokensAperturaDrive(), nil, nil)
	ctx := a.WithAuthorizationIntent(context.Background())
	s := &Suite{Sesion: a}
	i, err := s.prepararAperturaDrive(ctx)
	if err != nil || i != auth.RequestAuthorizationIntent(ctx).Identity() {
		t.Fatal("no se usa la identidad capturada al mostrar la cuenta")
	}
	b := auth.NewSession(tokensAperturaDrive(), nil, nil) // Misma identidad, otra instancia.
	if _, err := (&Suite{Sesion: b}).prepararAperturaDrive(ctx); !errors.Is(err, auth.ErrAuthorizationChanged) {
		t.Fatal("la apertura adoptó otra instancia de Session")
	}
	tokens := tokensAperturaDrive()
	tokens.Subject = "bea"
	a.Sincronizar(tokens)
	if err := s.AbrirDriveEnSistemaCon(ctx); !errors.Is(err, auth.ErrAuthorizationChanged) {
		t.Fatal("la intención anterior llegó a una apertura de otra cuenta")
	}
}

func TestAperturaDriveRevalidaLaIntencionTrasEsperasAntesDeAbrir(t *testing.T) {
	for _, cambio := range []string{"cancelar", "cuenta", "servidor", "organizacion"} {
		t.Run(cambio, func(t *testing.T) {
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			tokens := tokensAperturaDrive()
			s := &Suite{Sesion: auth.NewSession(tokens, nil, nil)}
			ctx, cancelar := context.WithCancel(s.Sesion.WithAuthorizationIntent(context.Background()))
			defer cancelar()
			if _, err := s.prepararAperturaDrive(ctx); err != nil {
				t.Fatal(err)
			}
			esperando, liberar := make(chan struct{}), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(liberar) }) })
			var aperturas atomic.Int32
			done := make(chan error, 1)
			go func() {
				close(esperando)
				<-liberar // Representa la preparación del proveedor o del explorador.
				done <- s.aplicarAperturaDrive(ctx, func() error { aperturas.Add(1); return nil })
			}()
			<-esperando
			switch cambio {
			case "cancelar":
				cancelar()
			case "cuenta":
				tokens.Subject = "bea"
				s.Sesion.Sincronizar(tokens)
			case "servidor":
				tokens.Issuer = "https://id.b.example"
				s.Sesion.Sincronizar(tokens)
			case "organizacion":
				tokens.Organizacion = "otra"
				s.Sesion.Sincronizar(tokens)
			}
			once.Do(func() { close(liberar) })
			select {
			case err := <-done:
				if (!errors.Is(err, context.Canceled) && !errors.Is(err, auth.ErrAuthorizationChanged)) || aperturas.Load() != 0 {
					t.Fatal("la apertura atravesó un cambio de intención durante una espera")
				}
			case <-time.After(time.Second):
				t.Fatal("no terminó la apertura descartada")
			}
		})
	}
}

func TestAperturaDriveNoRehabilitaLaPreferenciaDesactivada(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s := &Suite{Sesion: auth.NewSession(tokensAperturaDrive(), nil, nil)}
	ctx := s.Sesion.WithAuthorizationIntent(context.Background())
	if _, err := s.prepararAperturaDrive(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ajustes.Cambiar(func(a *ajustes.Ajustes) { a.DriveSistemaDesactivado = true }); err != nil {
		t.Fatal(err)
	}
	aperturas := 0
	if err := s.aplicarAperturaDrive(ctx, func() error { aperturas++; return nil }); !errors.Is(err, documentos.ErrNoDisponible) || aperturas != 0 {
		t.Fatal("abrir reactivó una ubicación que se acababa de desactivar")
	}
	if err := s.AbrirDriveEnSistemaCon(ctx); !errors.Is(err, documentos.ErrNoDisponible) {
		t.Fatal("el adaptador no respeta la preferencia desactivada")
	}
	if !ajustes.Leer().DriveSistemaDesactivado {
		t.Fatal("la apertura cambió la preferencia de la persona")
	}
}
