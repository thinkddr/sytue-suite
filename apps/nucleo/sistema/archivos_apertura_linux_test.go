// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package sistema

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
)

type datosAperturaArchivosPrueba struct {
	mu        sync.Mutex
	acceso    ajustes.AccesoDrive
	dueno     string
	abiertos  []string
	estrictos []bool
}

func prepararDatosAperturaArchivos(t *testing.T) (*datosAperturaArchivosPrueba, *auth.Session, context.Context) {
	t.Helper()
	s := auth.NewSession(auth.Tokens{Issuer: "https://id.a.example", Cliente: "native", Subject: "ana", Organizacion: "empresa", Access: "synthetic-a", Refresh: "synthetic-refresh", Expiry: time.Now().Add(time.Hour)}, nil, nil)
	i := s.SnapshotIdentity().IdentidadDrive()
	d := &datosAperturaArchivosPrueba{acceso: ajustes.AccesoDrive{Version: 1, Identidad: i, Revision: strings.Repeat("a", 32), Activo: true}, dueno: i}
	l, h, a := leerAccesoAperturaArchivos, hayCredencialAperturaArchivos, abrirDominioAperturaArchivos
	t.Cleanup(func() {
		leerAccesoAperturaArchivos, hayCredencialAperturaArchivos, abrirDominioAperturaArchivos = l, h, a
	})
	leerAccesoAperturaArchivos = func() (ajustes.AccesoDrive, error) { d.mu.Lock(); defer d.mu.Unlock(); return d.acceso, nil }
	hayCredencialAperturaArchivos = func(esperada string) (bool, error) { d.mu.Lock(); defer d.mu.Unlock(); return d.dueno == esperada, nil }
	abrirDominioAperturaArchivos = func(dominio string, estricto bool) error {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.abiertos, d.estrictos = append(d.abiertos, dominio), append(d.estrictos, estricto)
		return nil
	}
	ctx := s.WithAuthorizationIntent(context.Background())
	return d, s, ctx
}

func validarIntencionArchivos(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i := auth.RequestAuthorizationIntent(ctx)
	if i == nil || !i.Current() {
		return auth.ErrAuthorizationChanged
	}
	return nil
}

func TestAperturaArchivosRechazaCambioDuranteResolucionInterna(t *testing.T) {
	for _, cambio := range []string{"sesion", "sesion-aba", "sesion-llavero", "proveedor", "llavero"} {
		t.Run(cambio, func(t *testing.T) {
			d, s, ctx := prepararDatosAperturaArchivos(t)
			i := s.SnapshotIdentity().IdentidadDrive()
			entrando, liberar := make(chan struct{}), make(chan struct{})
			var once sync.Once
			original := leerAccesoAperturaArchivos
			originalLlavero := hayCredencialAperturaArchivos
			var primera sync.Once
			if cambio == "sesion-llavero" {
				hayCredencialAperturaArchivos = func(esperada string) (bool, error) {
					primera.Do(func() { close(entrando); <-liberar })
					return originalLlavero(esperada)
				}
			} else {
				leerAccesoAperturaArchivos = func() (ajustes.AccesoDrive, error) {
					primera.Do(func() { close(entrando); <-liberar })
					return original()
				}
			}
			done := make(chan error, 1)
			terminada := make(chan struct{})
			t.Cleanup(func() { once.Do(func() { close(liberar) }); <-terminada })
			go func() {
				defer close(terminada)
				a, err := PrepararAperturaArchivos(ctx, i)
				if err == nil {
					err = a.Abrir(ctx, validarIntencionArchivos)
				}
				done <- err
			}()
			select {
			case <-entrando:
			case <-time.After(time.Second):
				t.Fatal("no se bloqueó el resolver real del SDK")
			}
			switch cambio {
			case "sesion", "sesion-aba", "sesion-llavero":
				s.Sincronizar(auth.Tokens{Issuer: "https://id.a.example", Cliente: "native", Subject: "bea", Organizacion: "otra", Access: "synthetic-b", Refresh: "synthetic-b", Expiry: time.Now().Add(time.Hour)})
				if cambio == "sesion-aba" {
					s.Sincronizar(auth.Tokens{Issuer: "https://id.a.example", Cliente: "native", Subject: "ana", Organizacion: "empresa", Access: "synthetic-a2", Refresh: "synthetic-a2", Expiry: time.Now().Add(time.Hour)})
				}
			case "proveedor":
				d.mu.Lock()
				d.acceso.Identidad, d.dueno = strings.Repeat("b", 64), strings.Repeat("b", 64)
				d.mu.Unlock()
			case "llavero":
				// Una marca aún en A no autoriza la clave real de B.
				d.mu.Lock()
				d.dueno = strings.Repeat("b", 64)
				d.mu.Unlock()
			}
			once.Do(func() { close(liberar) })
			select {
			case err := <-done:
				d.mu.Lock()
				n := len(d.abiertos)
				d.mu.Unlock()
				if err == nil || n != 0 {
					t.Fatal("el resolver abrió otra cuenta con la intención anterior")
				}
			case <-time.After(time.Second):
				t.Fatal("la resolución descartada no terminó")
			}
		})
	}
}

func TestAperturaArchivosEntregaSoloDominioCapturadoSinSelectorGlobal(t *testing.T) {
	d, s, ctx := prepararDatosAperturaArchivos(t)
	a, err := PrepararAperturaArchivos(ctx, s.SnapshotIdentity().IdentidadDrive())
	if err != nil {
		t.Fatal(err)
	}
	dominioA := "sytue-drive." + d.acceso.Identidad + "." + d.acceso.Revision
	// Simula el cambio justo después de la última lectura. Una apertura de A ya
	// validada puede terminar, pero nunca puede volver a resolver un dominio de B.
	err = a.Abrir(ctx, func(ctx context.Context) error {
		if err := validarIntencionArchivos(ctx); err != nil {
			return err
		}
		d.mu.Lock()
		d.acceso.Identidad, d.dueno = strings.Repeat("b", 64), strings.Repeat("b", 64)
		d.mu.Unlock()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.abiertos) != 1 || d.abiertos[0] != dominioA || !d.estrictos[0] {
		t.Fatal("aplicar reconsultó el dominio global o permitió el selector sin dueño")
	}
}

func TestAperturaArchivosRevalidaRevisionYPropietarioAntesDeAbrir(t *testing.T) {
	for _, cambio := range []string{"revision", "llavero", "cerrado", "cancelado"} {
		t.Run(cambio, func(t *testing.T) {
			d, s, padre := prepararDatosAperturaArchivos(t)
			ctx, cancelar := context.WithCancel(padre)
			defer cancelar()
			a, err := PrepararAperturaArchivos(ctx, s.SnapshotIdentity().IdentidadDrive())
			if err != nil {
				t.Fatal(err)
			}
			d.mu.Lock()
			switch cambio {
			case "revision":
				d.acceso.Revision = strings.Repeat("b", 32)
			case "llavero":
				d.dueno = strings.Repeat("b", 64)
			case "cerrado":
				d.acceso.Activo = false
			case "cancelado":
				cancelar()
			}
			d.mu.Unlock()
			err = a.Abrir(ctx, validarIntencionArchivos)
			d.mu.Lock()
			n := len(d.abiertos)
			d.mu.Unlock()
			if err == nil || n != 0 {
				t.Fatal("se abrió un dominio retirado o de otra autorización")
			}
			if cambio == "cancelado" && !errors.Is(err, context.Canceled) {
				t.Fatal("no se conservó la cancelación")
			}
		})
	}
}
