// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package sesion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

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

// TestCambioDeOrganizacionEnVivoCambiaLaCarpetaDeDriveSinReiniciarElHub (FYS-04): en una
// sesión viva, cambiar entre organizaciones con CambiarOrganizacion para primero la
// conexión anterior y monta la carpeta «Sytue Drive – <organización>» de la nueva sin
// reiniciar el Hub; al volver a una organización previa reutiliza su carpeta ya marcada.
func TestCambioDeOrganizacionEnVivoCambiaLaCarpetaDeDriveSinReiniciarElHub(t *testing.T) {
	aparte(t)
	casa, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	a := test.NewApp()
	defer a.Quit()
	s := Abrir(a, "sytue.eu")
	doble := &organizadorDoble{}
	s.orgs = doble

	tokensDe := func(org string) auth.Tokens {
		return auth.Tokens{
			Issuer:       "https://id.sytue.eu",
			Cliente:      auth.ClientID,
			Subject:      "ana",
			Organizacion: org,
			Access:       "tok-" + org,
			Refresh:      "ref-" + org,
			Expiry:       time.Now().Add(time.Hour),
		}
	}

	var (
		mu       sync.Mutex
		eventos  []string
		fuentes  []sesionDriveSistema
		abiertas = make(chan string, 8)
	)
	apuntar := func(ev string) {
		mu.Lock()
		eventos = append(eventos, ev)
		mu.Unlock()
	}
	c := &controladorDriveSistema{
		leer: func() estadoDriveSistema { return estadoDriveSistema{s.Sesion.SnapshotIdentity(), s.DriveEnSistema()} },
		conectar: func(ctx context.Context, e estadoDriveSistema) error {
			return conectarCarpetaDriveWindows(ctx, s, e, func(ctx context.Context, dir string, fuente sesionDriveSistema) error {
				base := filepath.Base(dir)
				mu.Lock()
				fuentes = append(fuentes, fuente)
				mu.Unlock()
				apuntar("+" + base)
				abiertas <- dir
				<-ctx.Done()
				apuntar("-" + base)
				return ctx.Err()
			})
		},
		retirar: func(_ context.Context, anterior estadoDriveSistema) error {
			apuntar("fuera:" + anterior.identidad.Organization)
			return nil
		},
	}
	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	s.AlCambiar(func(Estado) { go c.seguir(ctx) })

	esperarCarpeta := func(quiero string) {
		t.Helper()
		select {
		case dir := <-abiertas:
			if dir != quiero {
				t.Fatalf("carpeta conectada = %q; quiero %q", dir, quiero)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no se conectó la carpeta %q", quiero)
		}
	}

	// 1. Primera organización (acme): monta «Sytue Drive – acme».
	if err := s.Sesion.AdoptarCon(ctx, tokensDe("acme")); err != nil {
		t.Fatal(err)
	}
	s.aplicar(s.visto)
	dirAcme := filepath.Join(casa, "Sytue Drive – acme")
	esperarCarpeta(dirAcme)
	if got, err := prepararCarpetaDriveWindows(ctx, s, s.Sesion.SnapshotIdentity()); err != nil || got != dirAcme {
		t.Fatalf("prepararCarpetaDriveWindows(acme) = %q, %v; quiero %q", got, err, dirAcme)
	}

	// 2. Cambio en vivo a «beta» sin reiniciar el Hub: se detiene «Sytue Drive – acme» y se
	// conecta «Sytue Drive – beta». La fuente capturada para «acme» queda invalidada.
	s.reentrar = func(ctx context.Context) error {
		if err := s.Sesion.AdoptarCon(ctx, tokensDe("beta")); err != nil {
			return err
		}
		s.aplicar(s.visto)
		return nil
	}
	if err := s.CambiarOrganizacion(ctx, "beta"); err != nil {
		t.Fatal(err)
	}
	dirBeta := filepath.Join(casa, "Sytue Drive – beta")
	esperarCarpeta(dirBeta)
	mu.Lock()
	primeraFuente := fuentes[0]
	mu.Unlock()
	if _, err := primeraFuente.AccessToken(ctx); !errors.Is(err, nube.ErrOtraCuenta) {
		t.Fatalf("la fuente de acme siguió dando token tras cambiar a beta: %v", err)
	}
	if got, err := prepararCarpetaDriveWindows(ctx, s, s.Sesion.SnapshotIdentity()); err != nil || got != dirBeta {
		t.Fatalf("prepararCarpetaDriveWindows(beta) = %q, %v; quiero %q", got, err, dirBeta)
	}

	// 3. Cambio en vivo de vuelta a «acme»: reutiliza «Sytue Drive – acme» sin crear « (2)».
	s.reentrar = func(ctx context.Context) error {
		if err := s.Sesion.AdoptarCon(ctx, tokensDe("acme")); err != nil {
			return err
		}
		s.aplicar(s.visto)
		return nil
	}
	if err := s.CambiarOrganizacion(ctx, "acme"); err != nil {
		t.Fatal(err)
	}
	esperarCarpeta(dirAcme)

	mu.Lock()
	gotEventos := slices.Clone(eventos)
	mu.Unlock()
	quieroEventos := []string{
		"+Sytue Drive – acme",
		"-Sytue Drive – acme",
		"+Sytue Drive – beta",
		"-Sytue Drive – beta",
		"+Sytue Drive – acme",
	}
	if !slices.Equal(gotEventos, quieroEventos) {
		t.Fatalf("transiciones de carpetas = %v; quiero %v", gotEventos, quieroEventos)
	}
}
