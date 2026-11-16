// SPDX-License-Identifier: Apache-2.0
package calendario

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

type transportVida func(*http.Request) (*http.Response, error)

func (f transportVida) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRetirarPresentadorCancelaYEsperaLaPeticionOriginal(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	transport := transportVida(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(canceled)
		<-release
		return nil, req.Context().Err()
	})
	parent, cancel := context.WithCancel(context.Background())
	p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC}
	a := NuevaAppConVida(parent, p, api.NewCalendars("https://calendar.invalid", nil, &http.Client{Transport: transport}))
	ui := make(chan func(), 2)
	a.despachar = func(fn func()) { ui <- fn }
	t.Cleanup(func() {
		cancel()
		releaseOnce.Do(func() { close(release) })
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := a.Join(ctx); err != nil {
			t.Errorf("quedó trabajo del presentador: %v", err)
		}
	})
	stale := false
	a.enUI(func() { stale = true })
	queued := <-ui
	a.escribir(func(ctx context.Context) error {
		_, err := a.Cal.List(ctx)
		return err
	}, func(error) { stale = true })
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("la petición no arrancó")
	}
	a.Cerrar()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("la petición no recibió la cancelación de identidad")
	}
	short, stop := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer stop()
	if err := a.Join(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Join no esperó al trabajo admitido: %v", err)
	}
	queued()
	if stale {
		t.Fatal("el callback encolado de la identidad retirada se ejecutó")
	}
	releaseOnce.Do(func() { close(release) })
	ctx, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	if err := a.Join(ctx); err != nil {
		t.Fatalf("Join no drenó la petición cancelada: %v", err)
	}
	select {
	case <-ui:
		t.Fatal("el resultado tardío se envió a la interfaz")
	default:
	}
}

func TestRelojSeDetieneAlRetirarLaIdentidad(t *testing.T) {
	p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC}
	a := NuevaAppConVida(context.Background(), p, nil)
	a.Reloj()
	a.Reloj()
	a.Cerrar()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Join(ctx); err != nil {
		t.Fatalf("el reloj no se detuvo: %v", err)
	}
}

// Drain must cancel the admitted work itself, even before the UI calls Cerrar.
// The transport deliberately waits after seeing cancellation so that an early
// return from Drain cannot masquerade as completed cleanup.
func TestDrainCancelaContextoAdmitidoYEsperaElTrabajo(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	transport := transportVida(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(canceled)
		<-release
		return nil, req.Context().Err()
	})
	p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC}
	a := NuevaAppConVida(context.Background(), p, api.NewCalendars("https://calendar.invalid", nil, &http.Client{Transport: transport}))
	a.despachar = func(func()) {}
	t.Cleanup(func() {
		a.Cerrar()
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Join(ctx); err != nil {
			t.Errorf("quedó trabajo del presentador: %v", err)
		}
	})
	a.escribir(func(ctx context.Context) error {
		_, err := a.Cal.List(ctx)
		return err
	}, func(error) {})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("la petición no arrancó")
	}
	drained := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		drained <- a.trabajos.Drain(ctx)
	}()
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("Drain no canceló el contexto usado por la petición")
	}
	if a.vida.Err() != nil {
		t.Fatal("el escenario canceló la vida del presentador antes de Drain")
	}
	select {
	case err := <-drained:
		t.Fatalf("Drain no esperó a que terminara el transporte: %v", err)
	default:
	}
	releaseOnce.Do(func() { close(release) })
	if err := <-drained; err != nil {
		t.Fatalf("Drain no terminó tras liberar el transporte: %v", err)
	}
}

func TestDrainDirectoDetieneReloj(t *testing.T) {
	p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC}
	a := NuevaAppConVida(context.Background(), p, nil)
	t.Cleanup(a.Cerrar)
	a.Reloj()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.trabajos.Drain(ctx); err != nil {
		t.Fatalf("Drain no detuvo el reloj: %v", err)
	}
	if a.vida.Err() != nil {
		t.Fatal("el reloj se detuvo solo al cancelar la vida del presentador")
	}
}

func TestJoinInvalidaCallbackEnColaYPermiteReintentar(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	transport := transportVida(func(req *http.Request) (*http.Response, error) {
		close(started)
		<-req.Context().Done()
		close(canceled)
		<-release
		return nil, req.Context().Err()
	})
	p := &Pantalla{T: traductor(t), Cursor: time.Now(), Zona: time.UTC}
	a := NuevaAppConVida(context.Background(), p, api.NewCalendars("https://calendar.invalid", nil, &http.Client{Transport: transport}))
	ui := make(chan func(), 1)
	a.despachar = func(fn func()) { ui <- fn }
	t.Cleanup(func() {
		a.Cerrar()
		releaseOnce.Do(func() { close(release) })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.Join(ctx); err != nil {
			t.Errorf("quedó trabajo del presentador: %v", err)
		}
	})
	stale := false
	a.enUI(func() { stale = true })
	queued := <-ui
	a.escribir(func(ctx context.Context) error {
		_, err := a.Cal.List(ctx)
		return err
	}, func(error) { stale = true })
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("la petición no arrancó")
	}
	short, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if err := a.Join(short); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Join no esperó al transporte bloqueado: %v", err)
	}
	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("Join no canceló la petición")
	}
	queued()
	if stale || a.vida.Err() == nil || a.lanzar(func(context.Context) {}) {
		t.Fatal("Join dejó viva la cola o permitió trabajo nuevo")
	}
	releaseOnce.Do(func() { close(release) })
	ctx, finish := context.WithTimeout(context.Background(), 5*time.Second)
	defer finish()
	if err := a.Join(ctx); err != nil {
		t.Fatalf("no se pudo reintentar Join: %v", err)
	}
}
