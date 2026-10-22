// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// El aislamiento de Cast NO puede depender solo de la RLS.
//
// El resto de tests de este paquete conectan con el rol de la aplicación
// (NOSUPERUSER, NOBYPASSRLS), así que lo que demuestran es que las políticas
// aíslan. Estos conectan con el rol DUEÑO, que se las salta todas —como hace hoy
// el despliegue que entrega a los servicios el superusuario de arranque de
// PostgreSQL—, y exigen lo mismo: lo de otra organización no existe, y lo de una
// lista de personas no existe para quien no está en ella. Lo único que puede
// sostenerlo aquí es el SQL del store.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

// montarSinRLS da un store sobre una conexión que se salta la RLS.
func montarSinRLS(t *testing.T) *store.Store {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := h.PoolSinRLS(t)
	// La premisa: si este rol no se saltara la RLS, un verde aquí no diría nada
	// del SQL.
	var super, bypass bool
	if err := pool.QueryRow(ctx, `SELECT rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).Scan(&super, &bypass); err != nil {
		t.Fatal(err)
	}
	if !super && !bypass {
		t.Fatal("el rol dueño del harness no se salta la RLS: este test no probaría el SQL")
	}
	return store.New(pool)
}

func TestSinRLSOtraOrganizacionNoExiste(t *testing.T) {
	ctx := ctxCorto(t)
	s := montarSinRLS(t)
	v := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)
	subiendo := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusUploading)
	if _, err := s.RecibirOriginal(ctx, "acme", subiendo.ID, subiendo.ID.String()+"/original", 100); err != nil {
		t.Fatal(err)
	}
	if err := s.AnadirPersona(ctx, "acme", v.ID, domain.Persona{UserID: "bea", Name: "Bea"}, "ana"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Encolar(ctx, "acme", v.ID, domain.JobPoster, nil); err != nil {
		t.Fatal(err)
	}

	noExiste := func(que string, err error) {
		t.Helper()
		if !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("%s desde otra organización: %v, esperaba que no existiera", que, err)
		}
	}

	// Lecturas.
	_, err := s.EnOrganizacion(ctx, "globex", v.ID)
	noExiste("EnOrganizacion", err)
	for _, admin := range []bool{true, false} {
		vs, err := s.Biblioteca(ctx, "globex", "zoe", admin, store.Filtro{Limite: 10})
		if err != nil || len(vs) != 0 {
			t.Errorf("la biblioteca de globex (admin=%v) enseña %d vídeos de acme (%v)", admin, len(vs), err)
		}
	}
	if ps, err := s.Personas(ctx, "globex", v.ID); err != nil || len(ps) != 0 {
		t.Errorf("globex lee con quién se comparte un vídeo de acme: %v %v", ps, err)
	}
	if js, err := s.TrabajosDelVideo(ctx, "globex", v.ID); err != nil || len(js) != 0 {
		t.Errorf("globex lee los trabajos de un vídeo de acme: %d %v", len(js), err)
	}
	if n, err := s.BytesDeOrganizacion(ctx, "globex"); err != nil || n != 0 {
		t.Errorf("globex suma bytes de acme: %d %v", n, err)
	}

	// Escrituras.
	_, err = s.Actualizar(ctx, "globex", v.ID, domain.Cambios{Title: ptr("Robado")})
	noExiste("Actualizar", err)
	_, err = s.CambiarEstado(ctx, "globex", subiendo.ID, store.Estado{A: domain.StatusFailed})
	noExiste("CambiarEstado", err)
	_, err = s.Encolar(ctx, "globex", v.ID, domain.JobQualities, nil)
	noExiste("Encolar", err)
	_, err = s.RecibirOriginal(ctx, "globex", subiendo.ID, "x", 1)
	noExiste("RecibirOriginal", err)
	_, err = s.ElegirCalidades(ctx, "globex", v.ID, nil, nil)
	noExiste("ElegirCalidades", err)
	_, err = s.FijarPoster(ctx, "globex", v.ID, v.ID.String()+"/poster-propio")
	noExiste("FijarPoster", err)
	noExiste("AnadirPersona", s.AnadirPersona(ctx, "globex", v.ID, domain.Persona{UserID: "zoe"}, "zoe"))
	noExiste("QuitarPersona", s.QuitarPersona(ctx, "globex", v.ID, "bea"))
	_, err = s.Borrar(ctx, "globex", v.ID)
	noExiste("Borrar", err)
	if err := s.RegistrarVista(ctx, "globex", v.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Espectadores(ctx, "globex", v.ID, 7); err != nil {
		t.Fatal(err)
	}

	// Y nada de lo anterior tocó el vídeo de acme.
	queda, err := s.EnOrganizacion(ctx, "acme", v.ID)
	if err != nil {
		t.Fatalf("el vídeo de acme ya no está: %v", err)
	}
	if queda.Title != v.Title || queda.Views != 0 || queda.ViewersNow != 0 || queda.PosterKey != v.PosterKey {
		t.Errorf("globex cambió el vídeo de acme: título %q, vistas %d, espectadores %d, póster %q",
			queda.Title, queda.Views, queda.ViewersNow, queda.PosterKey)
	}
	if ps, err := s.Personas(ctx, "acme", v.ID); err != nil || len(ps) != 1 || ps[0].UserID != "bea" {
		t.Errorf("la lista de personas de acme cambió: %v %v", ps, err)
	}
}

func TestSinRLSLoPublicoEsSoloLoAbierto(t *testing.T) {
	ctx := ctxCorto(t)
	s := montarSinRLS(t)
	publico := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, domain.StatusReady)
	org := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)
	personas := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusReady)
	procesando := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, domain.StatusProcessing)
	borrado := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, domain.StatusReady)
	if _, err := s.Borrar(ctx, "acme", borrado.ID); err != nil {
		t.Fatal(err)
	}

	if got, err := s.Publico(ctx, publico.ID); err != nil || got.ID != publico.ID {
		t.Errorf("el público debería verse sin sesión: %v", err)
	}
	for _, v := range []domain.Video{org, personas, procesando, borrado} {
		if _, err := s.Publico(ctx, v.ID); !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("%s/%s (título %q) NO debería verse sin sesión: %v", v.Visibility, v.Status, v.Title, err)
		}
	}
	_, portada, err := s.Explore(ctx, nil, 10)
	if err != nil || len(portada) != 1 || portada[0].ID != publico.ID {
		t.Errorf("portada: %d vídeos (%v), esperaba solo el público listo", len(portada), err)
	}
	canal, err := s.Canal(ctx, "acme", nil, 10)
	if err != nil || len(canal) != 1 || canal[0].ID != publico.ID {
		t.Errorf("canal de acme: %d vídeos (%v), esperaba solo el público listo", len(canal), err)
	}
}

func TestSinRLSInvitadoSoloSiEstaEnLaLista(t *testing.T) {
	ctx := ctxCorto(t)
	s := montarSinRLS(t)
	personas := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusReady)
	org := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)
	procesando := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusProcessing)
	for _, v := range []domain.Video{personas, org, procesando} {
		if err := s.AnadirPersona(ctx, "acme", v.ID, domain.Persona{UserID: "bea", Name: "Bea"}, "ana"); err != nil {
			t.Fatal(err)
		}
	}

	if got, err := s.Invitado(ctx, "bea", personas.ID); err != nil || got.ID != personas.ID {
		t.Errorf("bea está en la lista y no lo ve: %v", err)
	}
	if _, err := s.Invitado(ctx, "carl", personas.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Errorf("carl no está en la lista y lo ve: %v", err)
	}
	// Estar en la lista solo abre lo que es `people` y ya se puede ver.
	for _, v := range []domain.Video{org, procesando} {
		if _, err := s.Invitado(ctx, "bea", v.ID); !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("la lectura de invitado enseña un vídeo %s/%s: %v", v.Visibility, v.Status, err)
		}
	}

	// «Compartidos conmigo»: de otras organizaciones, y solo donde está.
	vs, err := s.Compartidos(ctx, "bea", "globex", nil, 10)
	if err != nil || len(vs) != 1 || vs[0].ID != personas.ID {
		t.Errorf("compartidos con bea: %d vídeos (%v), esperaba solo el suyo", len(vs), err)
	}
	if vs, err := s.Compartidos(ctx, "carl", "globex", nil, 10); err != nil || len(vs) != 0 {
		t.Errorf("compartidos con carl, que no está en ninguna lista: %d vídeos (%v)", len(vs), err)
	}
	if vs, err := s.Compartidos(ctx, "bea", "acme", nil, 10); err != nil || len(vs) != 0 {
		t.Errorf("compartidos no debería repetir los de la propia organización: %d (%v)", len(vs), err)
	}
}
