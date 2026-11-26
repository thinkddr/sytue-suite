// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

type photosLocalFixture struct {
	privatePhotosFixture
	llamadas []string
}

func (p *photosLocalFixture) TimelineDesde(context.Context, string, int, bool) ([]Photo, string, error) {
	p.llamadas = append(p.llamadas, "desde")
	return []Photo{{ID: "a"}}, "", nil
}

func (p *photosLocalFixture) TimelineYears(context.Context, bool) ([]TimelineYear, error) {
	p.llamadas = append(p.llamadas, "años")
	return []TimelineYear{{Year: 2026, Count: 1}}, nil
}

func (p *photosLocalFixture) Memories(context.Context) ([]Memory, error) {
	p.llamadas = append(p.llamadas, "recuerdos")
	return nil, nil
}

func (p *photosLocalFixture) Duplicates(context.Context, int) ([][]Photo, error) {
	p.llamadas = append(p.llamadas, "repetidas")
	return nil, nil
}

func (p *photosLocalFixture) Nearby(context.Context, string, int) ([]Photo, error) {
	p.llamadas = append(p.llamadas, "cerca")
	return nil, nil
}

func (p *photosLocalFixture) Purgar(context.Context, string) (int, error) {
	p.llamadas = append(p.llamadas, "purgar")
	return 2, nil
}

func (p *photosLocalFixture) PonerPortada(context.Context, string, string, string) error {
	p.llamadas = append(p.llamadas, "portada")
	return nil
}

// Las rutas de Photos que calculaba el servicio se delegan en los datos privados que las
// cumplen (§5.7); sin ellas siguen fallando cerradas, sin tocar la red.
func TestPhotosDelegaLoQueCalculaElDispositivo(t *testing.T) {
	var peticiones atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { peticiones.Add(1) }))
	defer srv.Close()
	ctx := t.Context()

	datos := &photosLocalFixture{}
	p, err := NewPhotos(srv.URL, nil, nil).WithPrivateData(datos)
	if err != nil {
		t.Fatal(err)
	}
	if f, _, err := p.TimelineDesde(ctx, "2026", 10, false); err != nil || len(f) != 1 {
		t.Fatal(f, err)
	}
	if a, err := p.TimelineYears(ctx, false); err != nil || len(a) != 1 {
		t.Fatal(a, err)
	}
	_, _ = p.Memories(ctx)
	_, _ = p.Duplicates(ctx, 5)
	_, _ = p.Nearby(ctx, "a", 5)
	if n, err := p.Purgar(ctx, ""); err != nil || n != 2 {
		t.Fatal(n, err)
	}
	if err := p.PonerPortada(ctx, "al", "t", "a"); err != nil {
		t.Fatal(err)
	}
	if len(datos.llamadas) != 7 {
		t.Fatal("no se delegó todo:", datos.llamadas)
	}

	sin, err := NewPhotos(srv.URL, nil, nil).WithPrivateData(privatePhotosFixture{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := sin.TimelineDesde(ctx, "2026", 10, false); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("desde:", err)
	}
	if _, err := sin.TimelineYears(ctx, false); !errors.Is(err, ErrPrivatePhotosOperation) {
		t.Fatal("años:", err)
	}
	if _, err := sin.Memories(ctx); err == nil {
		t.Fatal("recuerdos sin camino local")
	}
	if _, err := sin.Duplicates(ctx, 5); err == nil {
		t.Fatal("repetidas sin camino local")
	}
	if _, err := sin.Nearby(ctx, "a", 5); err == nil {
		t.Fatal("cerca sin camino local")
	}
	if _, err := sin.Purgar(ctx, ""); err == nil {
		t.Fatal("purgar sin camino local")
	}
	if err := sin.PonerPortada(ctx, "al", "t", "a"); err == nil {
		t.Fatal("portada sin camino local")
	}
	if peticiones.Load() != 0 {
		t.Fatal("la fachada privada ha salido a la red", peticiones.Load())
	}
}

// AsistenteFunc cumple Asistente con una función, y `*IA` también lo cumple.
func TestAsistenteFunc(t *testing.T) {
	var a Asistente = AsistenteFunc(func(_ context.Context, q PeticionIA) (string, error) { return "hola " + q.Accion, nil })
	if s, err := a.Asistir(t.Context(), PeticionIA{Accion: "x"}); err != nil || s != "hola x" {
		t.Fatal(s, err)
	}
}
