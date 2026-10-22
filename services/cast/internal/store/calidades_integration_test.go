// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"errors"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func TestCalidadesElegidasYSuTrabajo(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)

	// Al crear se guarda la elección; nada = NULL (la recomendada).
	v, err := s.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Kind: domain.KindUpload, Visibility: domain.VisibilityOrg, Title: "Con calidades", Qualities: []int32{720, 360}})
	if err != nil || !slices.Equal(v.Qualities, []int32{720, 360}) {
		t.Fatalf("Crear con calidades: %v %v", v.Qualities, err)
	}
	sin := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)
	if sin.Qualities != nil || sin.HlsQualities != nil || sin.WebmQualities != nil {
		t.Fatalf("sin elección, NULL: %v %v %v", sin.Qualities, sin.HlsQualities, sin.WebmQualities)
	}

	// El CHECK de la columna es entrada inválida, no un 500.
	if _, err := s.ElegirCalidades(ctx, "acme", sin.ID, []int32{1, 2, 3, 4, 5}, nil); !errors.Is(err, domain.ErrEntradaInvalida) {
		t.Fatalf("cinco calidades: %v", err)
	}

	// Elegir y encolar van juntos; si la función dice que no, no se encola.
	if _, err := s.ElegirCalidades(ctx, "acme", sin.ID, []int32{720}, func(domain.Video) bool { return false }); err != nil {
		t.Fatal(err)
	}
	if js, _ := s.TrabajosDelVideo(ctx, "acme", sin.ID); len(js) != 0 {
		t.Fatalf("no se pidió trabajo y se encoló: %+v", js)
	}
	elegido, err := s.ElegirCalidades(ctx, "acme", sin.ID, []int32{720}, func(domain.Video) bool { return true })
	if err != nil || !slices.Equal(elegido.Qualities, []int32{720}) {
		t.Fatalf("ElegirCalidades: %v %v", elegido.Qualities, err)
	}
	js, _ := s.TrabajosDelVideo(ctx, "acme", sin.ID)
	if len(js) != 1 || js[0].Kind != domain.JobQualities {
		t.Fatalf("trabajo de calidades: %+v", js)
	}
	j, err := s.Reclamar(ctx, "replica")
	if err != nil || j.ID != js[0].ID {
		t.Fatalf("Reclamar: %v %v", j.ID, err)
	}

	// Mientras se cuece, alguien vuelve a cambiarla: el trabajo en curso es de la
	// elección anterior, así que al terminar se encola otra pasada.
	if _, err := s.ElegirCalidades(ctx, "acme", sin.ID, []int32{720, 360}, func(domain.Video) bool { return true }); err != nil {
		t.Fatal(err)
	}
	total := int64(4242)
	hecho, err := s.Terminar(ctx, j.ID, store.Estado{
		WebmQualities: []int32{720}, HlsQualities: []int32{720}, BytesTotal: &total, Elegidas: []int32{720},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hecho.Status != domain.StatusReady || !slices.Equal(hecho.WebmQualities, []int32{720}) || !slices.Equal(hecho.HlsQualities, []int32{720}) || hecho.BytesTotal != 4242 {
		t.Fatalf("Terminar de calidades no cambia el estado y anota lo producido: %+v", hecho)
	}
	js, _ = s.TrabajosDelVideo(ctx, "acme", sin.ID)
	pendientes := 0
	for _, x := range js {
		if x.Kind == domain.JobQualities && x.Status == domain.JobPending {
			pendientes++
		}
	}
	if pendientes != 1 {
		t.Fatalf("la elección cambió a medias: otra pasada pendiente; trabajos %+v", js)
	}

	// Si no cambió, no se encola nada más.
	j2, err := s.Reclamar(ctx, "replica")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Terminar(ctx, j2.ID, store.Estado{WebmQualities: []int32{720, 360}, Elegidas: []int32{720, 360}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reclamar(ctx, "replica"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("sin cambio no hay otra pasada: %v", err)
	}
}
