// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package diapositivas

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/media"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

var (
	ana = service.Quien{Sub: "ana", Tenant: "acme", Nombre: "Ana"}
	bea = service.Quien{Sub: "bea", Tenant: "acme", Nombre: "Bea", Admin: true}
	zoe = service.Quien{Sub: "zoe", Tenant: "globex", Nombre: "Zoe"}
)

func montar(t *testing.T) (context.Context, *Servicio, *store.Store) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	svc := service.New(service.Deps{Store: st, Firmante: media.New("clave-de-prueba")})
	return ctx, New(h.Pool, svc, nil), st
}

func directo(ctx context.Context, t *testing.T, st *store.Store, vis domain.Visibility) domain.Video {
	t.Helper()
	v, err := st.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Kind: domain.KindLive, Visibility: vis, Title: "Asamblea", LiveSource: domain.LiveSourceWebRTC})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMigracionesYRLSDeLasPresentaciones(t *testing.T) {
	pgtest.ExigirRLSForzada(t, pgtest.New(t, db.Migrations, db.MigrationsDir), nil)
}

func TestPresentarUnDirecto(t *testing.T) {
	ctx, s, st := montar(t)
	v := directo(ctx, t, st, domain.VisibilityOrg)

	// Sin presentación todavía.
	if _, hay, err := s.Ver(ctx, ana, v.ID); err != nil || hay {
		t.Fatalf("antes de presentar: %v %v", hay, err)
	}

	// La primera llamada la abre; pasar a la misma diapositiva no añade capítulo.
	e, err := s.Avanzar(ctx, ana, v.ID, Posicion{Index: 0, Total: 3, Title: "Portada", Ref: "docs:abc"})
	if err != nil || e.Index != 0 || e.Total != 3 || e.StartedBy != "ana" || e.Ref != "docs:abc" || len(e.Marks) != 1 {
		t.Fatalf("primera diapositiva: %+v %v", e, err)
	}
	if e, err = s.Avanzar(ctx, ana, v.ID, Posicion{Index: 0, Total: 3, Title: "Portada"}); err != nil || len(e.Marks) != 1 {
		t.Fatalf("la misma diapositiva no es un capítulo: %+v %v", e, err)
	}
	if e, err = s.Avanzar(ctx, ana, v.ID, Posicion{Index: 1, Total: 3, Title: "Cuentas"}); err != nil || e.Index != 1 || e.Title != "Cuentas" || e.Ref != "docs:abc" || len(e.Marks) != 2 {
		t.Fatalf("segunda diapositiva (el origen se conserva): %+v %v", e, err)
	}
	// Retroceder también es un capítulo, y el orden es el de pasar.
	if e, err = s.Avanzar(ctx, ana, v.ID, Posicion{Index: 0, Total: 3, Title: "Portada"}); err != nil || len(e.Marks) != 3 || e.Marks[2].Index != 0 || e.Marks[1].Index != 1 {
		t.Fatalf("retroceder: %+v %v", e, err)
	}

	// Quien ve el directo la lee; quien no lo ve, no sabe ni que existe.
	if got, hay, err := s.Ver(ctx, ana, v.ID); err != nil || !hay || got.Index != 0 || len(got.Marks) != 3 {
		t.Fatalf("Ver: %+v %v %v", got, hay, err)
	}
	if _, _, err := s.Ver(ctx, zoe, v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("otra organización: %v", err)
	}
	if _, _, err := s.Ver(ctx, service.Nadie, v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("sin sesión en un directo de la organización: %v", err)
	}
}

func TestSoloElDuenoAvanzaYAlgunoMiraSiEsPublico(t *testing.T) {
	ctx, s, st := montar(t)
	v := directo(ctx, t, st, domain.VisibilityPublic)
	p := Posicion{Index: 0, Total: 2}

	// Un administrador de la organización puede terminar el directo pero no presentar por el dueño.
	for nombre, q := range map[string]service.Quien{"administrador": bea, "otra organización": zoe, "sin sesión": service.Nadie} {
		if _, err := s.Avanzar(ctx, q, v.ID, p); !errors.Is(err, domain.ErrProhibido) && !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("%s no debería poder avanzar: %v", nombre, err)
		}
	}
	if _, err := s.Avanzar(ctx, ana, v.ID, p); err != nil {
		t.Fatal(err)
	}
	// Público: se lee sin sesión mientras el directo esté en emisión.
	if _, err := st.CambiarEstado(ctx, "acme", v.ID, store.Estado{A: domain.StatusLive}); err != nil {
		t.Fatal(err)
	}
	if got, hay, err := s.Ver(ctx, service.Nadie, v.ID); err != nil || !hay || got.Total != 2 {
		t.Fatalf("lectura pública: %+v %v %v", got, hay, err)
	}
	// Los capítulos cuentan desde que el directo empezó.
	time.Sleep(20 * time.Millisecond)
	e, err := s.Avanzar(ctx, ana, v.ID, Posicion{Index: 1, Total: 2, Title: "Fin"})
	if err != nil || len(e.Marks) != 2 || e.Marks[1].AtMS <= 0 {
		t.Fatalf("el capítulo en directo lleva su instante: %+v %v", e, err)
	}
	// Terminado, la presentación se queda como está: ni se mueve ni se pierde.
	if _, err := st.CambiarEstado(ctx, "acme", v.ID, store.Estado{A: domain.StatusEnded}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Avanzar(ctx, ana, v.ID, Posicion{Index: 0, Total: 2}); !errors.Is(err, domain.ErrConflicto) {
		t.Fatalf("avanzar tras terminar: %v", err)
	}
	if got, hay, err := s.Ver(ctx, ana, v.ID); err != nil || !hay || got.Index != 1 || len(got.Marks) != 2 {
		t.Fatalf("tras terminar: %+v %v %v", got, hay, err)
	}
}

func TestSoloUnDirectoLlevaPresentacion(t *testing.T) {
	ctx, s, st := montar(t)
	v, err := st.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", OwnerName: "Ana", Kind: domain.KindUpload, Visibility: domain.VisibilityOrg, Title: "Subido"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Avanzar(ctx, ana, v.ID, Posicion{Index: 0, Total: 1}); !errors.Is(err, domain.ErrConflicto) {
		t.Fatalf("un vídeo subido no se presenta: %v", err)
	}
}
