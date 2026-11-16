// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build !darwin && !ios

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

// TestCambioDeOrganizacionEnVivoCambiaLaCarpetaDeDriveSinReiniciarElHub (FYS-04): en una
// sesión viva, cambiar entre organizaciones con CambiarOrganizacion para primero la
// conexión anterior y monta la carpeta «Sytue Drive – <organización>» de la nueva sin
// reiniciar el Hub; al volver a una organización previa reutiliza su carpeta ya marcada.
//
// La carpeta local con el nombre de la organización es el flujo de Windows/Linux
// (`prepararCarpetaDriveWindows`); en Apple, `abrirDriveEnSistema` abre por
// «Archivos»/Finder, que no crea carpeta con ese nombre.
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
