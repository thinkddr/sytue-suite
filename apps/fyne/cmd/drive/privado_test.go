// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"git.sytue.eu/sytue/one/apps/fyne/apps/drive"
	"git.sytue.eu/sytue/one/apps/fyne/catalogos"
	"git.sytue.eu/sytue/one/apps/fyne/cifrado"
	"git.sytue.eu/sytue/one/apps/fyne/tema"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/auth"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
	"git.sytue.eu/sytue/one/apps/nucleo/storage"
)

type repoPantalla struct {
	api.DriveData
	iniciar                           func(context.Context) error
	reanudar                          func(context.Context) error
	refrescar                         func(context.Context) error
	cerrados, reanudados, refrescados atomic.Int32
}

func (*repoPantalla) PrivateStorageScope() storage.Scope {
	return storage.Scope{Tenant: "org", Subject: "owner", Product: "drive"}
}

func (r *repoPantalla) Initialize(ctx context.Context) error {
	if r.iniciar != nil {
		return r.iniciar(ctx)
	}
	return ctx.Err()
}

func (r *repoPantalla) ResumeUploads(ctx context.Context) ([]api.File, error) {
	r.reanudados.Add(1)
	if r.reanudar != nil {
		return nil, r.reanudar(ctx)
	}
	return nil, ctx.Err()
}

func (r *repoPantalla) Refresh(ctx context.Context) error {
	r.refrescados.Add(1)
	if r.refrescar != nil {
		return r.refrescar(ctx)
	}
	return ctx.Err()
}
func (r *repoPantalla) Close() { r.cerrados.Add(1) }

type vistaDePrueba struct {
	identidad auth.IdentitySnapshot
	montajes  int
}

func controladorEn(t *testing.T) (*drivePrivado, *repoPantalla, chan func(), context.CancelFunc, *vistaDePrueba) {
	t.Helper()
	app := test.NewApp()
	app.Settings().SetTheme(tema.Sytue{})
	t.Cleanup(app.Quit)
	tr, err := i18n.New("es", i18n.Common(), catalogos.Drive())
	if err != nil {
		t.Fatal(err)
	}
	vista := &vistaDePrueba{identidad: auth.IdentitySnapshot{Issuer: "https://id.example", Client: "native", Subject: "owner", Organization: "org"}}
	queue := make(chan func(), 128)
	ctx, cancel := context.WithCancel(t.Context())
	repo := &repoPantalla{}
	d := &drivePrivado{panel: &cifrado.Panel{}, cuenta: vista.identidad, dentro: true, identidad: func() (auth.IdentitySnapshot, bool) { return vista.identidad, true }, despachar: func(fn func()) { queue <- fn }, avisar: func(error) {}, montar: func(context.Context, repositorioPrivado) (*drive.Pantalla, error) {
		vista.montajes++
		return drive.NuevaPantalla(tr, nil, nil, nil), nil
	}}
	d.instalar()
	d.ciclo = &cicloDrive{ctx: ctx, repo: repo}
	t.Cleanup(func() {
		cancel()
		if drain := d.retirar(); drain != nil {
			drain()
		}
	})
	return d, repo, queue, cancel, vista
}

func uiPaso(t *testing.T, queue chan func()) {
	t.Helper()
	select {
	case fn := <-queue:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("controller did not dispatch UI work")
	}
}

func TestDrivePrivadoNoMuestraDatosHastaConfirmarLaImportacion(t *testing.T) {
	d, repo, queue, _, vista := controladorEn(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	repo.iniciar = func(ctx context.Context) error {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	}
	d.preparar(d.ciclo.ctx)
	<-entered
	if vista.montajes != 0 || d.pantalla != nil {
		t.Fatal("partial import exposed a screen")
	}
	// Releasing the source is separate from the UI dispatcher and SDK transport.
	unblock()
	uiPaso(t, queue)
	if vista.montajes != 1 || d.pantalla == nil {
		t.Fatal("complete private import did not activate screen")
	}
}

func TestDrivePrivadoDescartaLaImportacionAnteriorYLasAccionesPendientes(t *testing.T) {
	d, repo, queue, cancel, vista := controladorEn(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	repo.iniciar = func(context.Context) error { close(entered); <-release; return nil }
	queued := 0
	d.cuandoLista(func(*drive.Pantalla) { queued++ })
	d.preparar(d.ciclo.ctx)
	<-entered
	cancel()
	drain := d.retirar()
	drained := make(chan struct{})
	go func() { drain(); close(drained) }()
	vista.identidad.Subject = "next-account"
	select {
	case <-drained:
		t.Fatal("repository teardown did not wait for initial import")
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	<-drained
	uiPaso(t, queue)
	if vista.montajes != 0 || queued != 0 || repo.cerrados.Load() != 1 || d.pantalla != nil {
		t.Fatal("stale import activated or retained the old presenter")
	}
}

func TestDrivePrivadoUnOrigenInvalidoNoActivaNiReanuda(t *testing.T) {
	d, repo, queue, _, vista := controladorEn(t)
	repo.iniciar = func(context.Context) error { return errors.New("invalid source") }
	d.preparar(d.ciclo.ctx)
	uiPaso(t, queue)
	if vista.montajes != 0 || repo.reanudados.Load() != 0 || d.pantalla != nil {
		t.Fatal("invalid import reached the app")
	}
}

func TestDrivePrivadoElMismoUsuarioNoRevalidaUnInicioDeSesionAnterior(t *testing.T) {
	d, repo, _, _, vista := controladorEn(t)
	d.ciclo.validar = func(context.Context) error { return auth.ErrAuthorizationChanged }
	called := false
	d.cuandoLista(func(*drive.Pantalla) { called = true })
	d.preparar(d.ciclo.ctx)
	if called || vista.montajes != 0 || repo.reanudados.Load() != 0 || d.ciclo.ocupado {
		t.Fatal("an expired persisted witness revived the same-account presenter")
	}
}

func TestDrivePrivadoUnaAccionPendientePuedeCerrarSinReanudar(t *testing.T) {
	d, repo, queue, cancel, _ := controladorEn(t)
	var drain func()
	d.cuandoLista(func(*drive.Pantalla) {
		cancel()
		drain = d.retirar()
	})
	d.preparar(d.ciclo.ctx)
	uiPaso(t, queue)
	if drain == nil || d.pantalla != nil || repo.reanudados.Load() != 0 {
		t.Fatal("queued close started a new job or retained the presenter")
	}
	drain()
	if repo.cerrados.Load() != 1 {
		t.Fatal("queued close did not drain its repository")
	}
}

func TestDrivePrivadoLaReanudacionNoBloqueaLaPantallaYSeDrenaAlCerrar(t *testing.T) {
	d, repo, queue, cancel, vista := controladorEn(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	repo.reanudar = func(context.Context) error { close(entered); <-release; return nil }
	d.preparar(d.ciclo.ctx)
	uiPaso(t, queue)
	<-entered
	if vista.montajes != 1 || d.pantalla == nil {
		t.Fatal("background uploads blocked the private screen")
	}
	cancel()
	drain := d.retirar()
	done := make(chan struct{})
	go func() { drain(); close(done) }()
	select {
	case <-done:
		t.Fatal("SDK cleanup could overtake an upload job")
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	<-done
	uiPaso(t, queue)
	if repo.cerrados.Load() != 1 || d.pantalla != nil {
		t.Fatal("late upload completion restored closed screen")
	}
}

func TestDrivePrivadoAgrupaRefrescosSinReproducirCallbacksViejos(t *testing.T) {
	d, repo, queue, _, _ := controladorEn(t)
	c := d.ciclo
	c.repo = repo
	entered := make(chan struct{}, 2)
	gates := [2]chan struct{}{make(chan struct{}), make(chan struct{})}
	var once [2]sync.Once
	unblock := func(n int) { once[n].Do(func() { close(gates[n]) }) }
	t.Cleanup(func() { unblock(0); unblock(1) })
	var calls atomic.Int32
	repo.refrescar = func(context.Context) error { n := calls.Add(1) - 1; entered <- struct{}{}; <-gates[n]; return nil }
	old, last := 0, 0
	d.refrescar(c, func() { old++ })
	<-entered
	d.refrescar(c, func() { last++ })
	unblock(0)
	uiPaso(t, queue)
	<-entered
	unblock(1)
	uiPaso(t, queue)
	if old != 0 || last != 1 || repo.refrescados.Load() != 2 {
		t.Fatal("refresh did not coalesce the pending view", old, last, repo.refrescados.Load())
	}
}

func TestDriveConsentimientoNoBloqueaUIYSeDrenaAntesDeRetirarSDK(t *testing.T) {
	d, repo, queue, cancel, _ := controladorEn(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	activated := 0
	d.autorizarAlmacen(d.ciclo.ctx, func(context.Context) error { close(entered); <-release; return nil }, func() { activated++ })
	<-entered
	if !d.ciclo.ocupado || activated != 0 {
		t.Fatal("Drive created its repository before storage consent")
	}
	d.autorizarAlmacen(d.ciclo.ctx, func(context.Context) error { t.Error("duplicate storage consent started"); return nil }, func() { activated++ })
	cancel()
	drain := d.retirar()
	drained := make(chan struct{})
	go func() { drain(); close(drained) }()
	select {
	case <-drained:
		t.Fatal("SDK teardown overtook pending storage consent")
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	<-drained
	uiPaso(t, queue)
	if activated != 0 || repo.cerrados.Load() != 1 {
		t.Fatal("retired storage consent activated a repository")
	}
}

func TestDriveConsentimientoRechazadoPermiteReintentarSinMontarRepositorio(t *testing.T) {
	d, _, queue, _, _ := controladorEn(t)
	denied := errors.New("consent denied")
	failures, activated := 0, 0
	d.avisar = func(err error) {
		if !errors.Is(err, denied) {
			t.Error("storage consent failure changed", err)
		}
		failures++
	}
	d.autorizarAlmacen(d.ciclo.ctx, func(context.Context) error { return denied }, func() { activated++ })
	uiPaso(t, queue)
	if d.ciclo.ocupado || failures != 1 || activated != 0 {
		t.Fatal("denied storage consent opened repository or blocked retry")
	}
	d.autorizarAlmacen(d.ciclo.ctx, func(context.Context) error { return nil }, func() { activated++ })
	uiPaso(t, queue)
	if d.ciclo.ocupado || activated != 1 {
		t.Fatal("accepted retry failed to open original storage lifetime")
	}
}
