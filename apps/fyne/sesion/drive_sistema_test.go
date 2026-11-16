// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/nube"
)

func TestDriveSistemaArrancaSinConfiguracionYRetiraAntesDeCambiarCuenta(t *testing.T) {
	var mu sync.Mutex
	var hechos []string
	apuntar := func(s string) { mu.Lock(); hechos = append(hechos, s); mu.Unlock() }
	a := estadoDriveSistema{auth.IdentitySnapshot{Issuer: "https://id.a.eu", Client: "desktop", Subject: "ana", Organization: "acme"}, true}
	b := a
	b.identidad.Organization = "beta"
	actual := a
	c := &controladorDriveSistema{
		leer: func() estadoDriveSistema { return actual },
		conectar: func(ctx context.Context, e estadoDriveSistema) error {
			apuntar("+" + e.identidad.Organization)
			<-ctx.Done()
			apuntar("-" + e.identidad.Organization)
			return ctx.Err()
		},
		retirar: func(context.Context, estadoDriveSistema) error { apuntar("fuera"); return nil },
	}
	esperar := func(n int) {
		t.Helper()
		for fin := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			mu.Lock()
			l := len(hechos)
			mu.Unlock()
			if l >= n {
				return
			}
			if time.Now().After(fin) {
				t.Fatal("no terminó la transición de Drive")
			}
		}
	}
	ctx, parar := context.WithCancel(context.Background())
	defer parar()
	c.seguir(ctx)
	esperar(1)    // sin carpeta previamente conectada ni clic
	c.seguir(ctx) // tema/notificación de la misma cuenta no duplica conexión
	actual = b
	c.seguir(ctx)
	esperar(3)
	actual.habilitado = false
	c.seguir(ctx)
	esperar(5)
	actual = b
	c.seguir(ctx)
	esperar(6)
	actual = estadoDriveSistema{}
	c.seguir(ctx)
	esperar(8)
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(hechos, []string{"+acme", "-acme", "+beta", "-beta", "fuera", "+beta", "-beta", "fuera"}) {
		t.Fatalf("orden: %v", hechos)
	}
}

func TestDriveSistemaReintentaAlVolverTrasFallo(t *testing.T) {
	n := 0
	c := &controladorDriveSistema{
		leer: func() estadoDriveSistema {
			return estadoDriveSistema{auth.IdentitySnapshot{Subject: "ana", Organization: "acme"}, true}
		},
		conectar: func(context.Context, estadoDriveSistema) error { n++; return errors.New("sin red") },
	}
	ctx := context.Background()
	for intento := 1; intento <= 2; intento++ {
		c.seguir(ctx)
		for fin := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			c.mu.Lock()
			finito := c.cancelar == nil
			c.mu.Unlock()
			if finito {
				break
			}
			if time.Now().After(fin) {
				t.Fatal("no liberó el intento")
			}
		}
	}
	if n != 2 {
		t.Fatalf("intentos: %d", n)
	}
}

func TestLaFuenteDriveSistemaNuncaAdoptaElTokenDeOtraCuenta(t *testing.T) {
	s := auth.NewSession(auth.Tokens{Access: "token-a", Refresh: "refresh-a", Expiry: time.Now().Add(time.Hour), Issuer: "https://id.a.eu", Cliente: "desktop", Subject: "ana", Organizacion: "acme"}, nil, nil)
	f := sesionDriveSistema{sesion: s, identidad: s.SnapshotIdentity()}
	token, alcance, err := f.AccessTokenScoped(t.Context())
	if err != nil || token != "token-a" || alcance == nil || !alcance.Current("Bearer "+token) {
		t.Fatalf("token propio: %q,%v", token, err)
	}
	s.Sincronizar(auth.Tokens{Access: "token-b", Refresh: "refresh-b", Expiry: time.Now().Add(time.Hour), Issuer: "https://id.a.eu", Cliente: "desktop", Subject: "ana", Organizacion: "beta"})
	if _, err := f.AccessToken(context.Background()); !errors.Is(err, nube.ErrOtraCuenta) {
		t.Fatalf("adoptó la organización nueva: %v", err)
	}
	if alcance.Current("Bearer " + token) {
		t.Fatal("la autorización capturada sobrevivió al cambio de cuenta")
	}
}

func TestDriveSistemaInicioYRetiradaDeCredencialSiguenLaSesion(t *testing.T) {
	e := estadoDriveSistema{auth.IdentitySnapshot{Subject: "ana", Organization: "acme"}, true}
	activa, conexiones := false, 0
	var inicios []bool
	c := &controladorDriveSistema{
		leer:       func() estadoDriveSistema { return e },
		inicio:     func(e estadoDriveSistema) error { inicios = append(inicios, e.dentro()); return nil },
		conectar:   func(context.Context, estadoDriveSistema) error { activa = true; conexiones++; return nil },
		conservada: func(estadoDriveSistema) bool { return activa },
		retirar:    func(context.Context, estadoDriveSistema) error { activa = false; return nil },
	}
	esperar := func() {
		t.Helper()
		for fin := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			c.mu.Lock()
			hecho := c.cancelar == nil
			c.mu.Unlock()
			if hecho {
				return
			}
			if time.Now().After(fin) {
				t.Fatal("no terminó la conexión")
			}
		}
	}
	ctx := context.Background()
	c.seguir(ctx)
	esperar()
	c.seguir(ctx) // renovar el token o cambiar el tema no repite el inicio del SO
	if conexiones != 1 || len(inicios) != 1 || !inicios[0] {
		t.Fatal("no configuró el inicio automático por defecto")
	}
	activa = false // otra app cerró y volvió a abrir la misma cuenta entre dos avisos
	c.seguir(ctx)
	esperar()
	if conexiones != 2 {
		t.Fatal("conservó un resultado viejo tras retirar la credencial")
	}
	e.habilitado = false
	c.seguir(ctx)
	e = estadoDriveSistema{}
	c.seguir(ctx)
	if !slices.Equal(inicios, []bool{true, true, false, false}) {
		t.Fatalf("inicio del SO: %v", inicios)
	}
}
