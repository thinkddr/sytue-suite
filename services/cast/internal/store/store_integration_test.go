// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Cast contra un PostgreSQL real (platform/pkg/pgtest).
// Verifican lo que no puede fallar: que las migraciones aplican con su RLS forzada,
// que las dos lecturas que cruzan organizaciones (pública e invitada) enseñan
// exactamente lo que dicen y nada más, y que el ciclo de vida no se salta pasos.
package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/cast/db"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/store"
)

func ctxCorto(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func montar(t *testing.T) *store.Store {
	t.Helper()
	s, _ := montarConHarness(t)
	return s
}

// montarConHarness da además el harness, para lo que el rol de la aplicación no
// puede hacer por diseño (adelantar el reloj de la cola).
func montarConHarness(t *testing.T) (*store.Store, *pgtest.Harness) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	return store.New(h.Pool), h
}

// comoSuperusuario ejecuta SQL saltándose la RLS (rol dueño de la base).
func comoSuperusuario(ctx context.Context, t *testing.T, h *pgtest.Harness, sql string, args ...any) {
	t.Helper()
	conn, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	if _, err := conn.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

// listo crea un vídeo y lo lleva a `ready` (o lo deja como nazca si `st` es vacío).
func listo(ctx context.Context, t *testing.T, s *store.Store, tenant, owner string, vis domain.Visibility, st domain.Status) domain.Video {
	t.Helper()
	v, err := s.Crear(ctx, store.Nuevo{Tenant: tenant, OwnerSub: owner, OwnerName: "Persona " + owner, Kind: domain.KindUpload, Visibility: vis, Title: "Vídeo " + string(vis)})
	if err != nil {
		t.Fatalf("Crear: %v", err)
	}
	if st == "" || st == domain.StatusUploading {
		return v
	}
	pasos := []domain.Status{domain.StatusProcessing}
	if st != domain.StatusProcessing {
		pasos = append(pasos, st)
	}
	for _, paso := range pasos {
		if v, err = s.CambiarEstado(ctx, tenant, v.ID, store.Estado{A: paso}); err != nil {
			t.Fatalf("CambiarEstado %s: %v", paso, err)
		}
	}
	return v
}

func TestMigracionesYRLS(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	// Todas las tablas de Cast llevan organización: ninguna se queda fuera.
	pgtest.ExigirRLSForzada(t, h, nil)
}

// El limitador compartido de /public, /media y /ws funciona sobre la base de
// Cast con el rol de la aplicación. Sin la migración 0004 falla, y el middleware
// (fail-closed) dejaba todo lo anónimo en 503.
func TestLimitadorCompartido(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	l := pg.NewRateLimiter(s.DB(), pg.RateLimiterConfig{PerSecond: 1, Burst: 1})
	r, err := l.Allow(ctx, "ip:203.0.113.7")
	if err != nil || !r.Allowed {
		t.Fatalf("primera petición: %+v %v", r, err)
	}
	if r, err := l.Allow(ctx, "ip:203.0.113.7"); err != nil || r.Allowed {
		t.Fatalf("la segunda, sin fichas, pasó: %+v %v", r, err)
	}
}

func TestOtraOrganizacionNoVeNada(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	v := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)

	if _, err := s.EnOrganizacion(ctx, "globex", v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("globex leyó un vídeo de acme: %v", err)
	}
	vs, err := s.Biblioteca(ctx, "globex", "zoe", true, store.Filtro{Limite: 10})
	if err != nil || len(vs) != 0 {
		t.Fatalf("la biblioteca de globex enseña %d vídeos de acme (%v)", len(vs), err)
	}
	if _, err := s.Actualizar(ctx, "globex", v.ID, domain.Cambios{Title: ptr("Robado")}); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("globex editó un vídeo de acme: %v", err)
	}
}

// La política pública enseña public y unlisted ya visibles, y nada más.
func TestPoliticaPublica(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	publico := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, domain.StatusReady)
	oculto := listo(ctx, t, s, "acme", "ana", domain.VisibilityUnlisted, domain.StatusReady)
	org := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)
	personas := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusReady)
	procesando := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, domain.StatusProcessing)
	otraOrg := listo(ctx, t, s, "globex", "zoe", domain.VisibilityPublic, domain.StatusReady)

	for _, v := range []domain.Video{publico, oculto, otraOrg} {
		if got, err := s.Publico(ctx, v.ID); err != nil || got.ID != v.ID {
			t.Errorf("%s debería verse sin sesión: %v", v.Visibility, err)
		}
	}
	for _, v := range []domain.Video{org, personas, procesando} {
		if _, err := s.Publico(ctx, v.ID); !errors.Is(err, domain.ErrNoExiste) {
			t.Errorf("%s/%s NO debería verse sin sesión: %v", v.Visibility, v.Status, err)
		}
	}

	// La portada: solo `public`, de todas las organizaciones, y el canal filtra.
	directos, portada, err := s.Explore(ctx, nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(directos) != 0 {
		t.Fatalf("no hay directos y la portada enseña %d", len(directos))
	}
	if len(portada) != 2 || !contiene(portada, publico.ID) || !contiene(portada, otraOrg.ID) {
		t.Fatalf("portada: %d vídeos, esperaba los dos públicos", len(portada))
	}
	canal, err := s.Canal(ctx, "globex", nil, 10)
	if err != nil || len(canal) != 1 || canal[0].ID != otraOrg.ID {
		t.Fatalf("canal de globex: %v %v", canal, err)
	}
	// Un directo público en curso va delante, aparte, solo en la primera página.
	live, err := s.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", Kind: domain.KindLive, Visibility: domain.VisibilityPublic, Title: "Directo", LiveSource: domain.LiveSourceRTMP, LiveRoom: "cast-acme-x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CambiarEstado(ctx, "acme", live.ID, store.Estado{A: domain.StatusLive}); err != nil {
		t.Fatal(err)
	}
	directos, portada, err = s.Explore(ctx, nil, 10)
	if err != nil || len(directos) != 1 || directos[0].ID != live.ID || len(portada) != 2 {
		t.Fatalf("con un directo: directos=%d portada=%d err=%v", len(directos), len(portada), err)
	}
	directos, _, err = s.Explore(ctx, &store.Cursor{Fecha: portada[0].CreatedAt, ID: portada[0].ID}, 10)
	if err != nil || len(directos) != 0 {
		t.Fatalf("la segunda página no repite los directos: %d %v", len(directos), err)
	}
	// /media lee por id sin organización, pero nunca lo que aún se sube.
	if _, err := s.PorID(ctx, publico.ID); err != nil {
		t.Fatalf("PorID: %v", err)
	}
	subiendo := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, "")
	if _, err := s.PorID(ctx, subiendo.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("PorID de un vídeo en uploading: %v", err)
	}

	// Borrado blando: desaparece de lo público al instante.
	if borrado, err := s.Borrar(ctx, "acme", publico.ID); err != nil || borrado.ID != publico.ID {
		t.Fatal(err)
	}
	if _, err := s.Publico(ctx, publico.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatal("un vídeo borrado sigue viéndose sin sesión")
	}
}

// La política de invitado enseña a cada persona SUS vídeos `people`, de cualquier
// organización, y su fila de la lista; ni los de otros ni la lista entera.
func TestPoliticaDeInvitado(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	v := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusReady)
	if err := s.AnadirPersona(ctx, "acme", v.ID, domain.Persona{UserID: "dana", Name: "Dana", Username: "dana"}, "ana"); err != nil {
		t.Fatal(err)
	}

	if got, err := s.Invitado(ctx, "dana", v.ID); err != nil || got.ID != v.ID {
		t.Fatalf("dana (globex) debería ver el vídeo que le compartieron: %v", err)
	}
	if _, err := s.Invitado(ctx, "zoe", v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("zoe no está en la lista y lo ve: %v", err)
	}
	ok, err := s.EsInvitado(ctx, "dana", v.ID)
	if err != nil || !ok {
		t.Fatalf("EsInvitado(dana)=%v %v", ok, err)
	}
	if ok, _ := s.EsInvitado(ctx, "zoe", v.ID); ok {
		t.Fatal("EsInvitado(zoe) debería ser falso")
	}

	comp, err := s.Compartidos(ctx, "dana", "globex", nil, 10)
	if err != nil || len(comp) != 1 || comp[0].ID != v.ID {
		t.Fatalf("Compartidos(dana): %v %v", comp, err)
	}
	// Los de su propia organización no se repiten aquí: salen en su biblioteca.
	comp, err = s.Compartidos(ctx, "dana", "acme", nil, 10)
	if err != nil || len(comp) != 0 {
		t.Fatalf("Compartidos(dana, acme) debería estar vacío: %v %v", comp, err)
	}

	// Si el dueño lo pasa a `org`, la persona de fuera deja de verlo.
	vis := domain.VisibilityOrg
	if _, err := s.Actualizar(ctx, "acme", v.ID, domain.Cambios{Visibility: &vis}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Invitado(ctx, "dana", v.ID); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatal("dana sigue viendo un vídeo que ya no es `people`")
	}

	// Y la lista es del dueño: quitar a alguien lo saca de verdad.
	if err := s.QuitarPersona(ctx, "acme", v.ID, "dana"); err != nil {
		t.Fatal(err)
	}
	if err := s.QuitarPersona(ctx, "acme", v.ID, "dana"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("quitar dos veces: %v", err)
	}
}

// La biblioteca aplica la regla de dentro en SQL: lo propio siempre, lo visible de
// los demás según visibilidad, y `people` solo si se está en la lista.
func TestBiblioteca(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	mioEnProceso := listo(ctx, t, s, "acme", "bea", domain.VisibilityOrg, domain.StatusProcessing)
	ajenoEnProceso := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, domain.StatusProcessing)
	org := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, domain.StatusReady)
	personasSinBea := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusReady)
	personasConBea := listo(ctx, t, s, "acme", "ana", domain.VisibilityPeople, domain.StatusReady)
	if err := s.AnadirPersona(ctx, "acme", personasConBea.ID, domain.Persona{UserID: "bea", Name: "Bea"}, "ana"); err != nil {
		t.Fatal(err)
	}

	vs, err := s.Biblioteca(ctx, "acme", "bea", false, store.Filtro{Limite: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, esperado := range []uuid.UUID{mioEnProceso.ID, org.ID, personasConBea.ID} {
		if !contiene(vs, esperado) {
			t.Errorf("a Bea le falta %s", esperado)
		}
	}
	for _, prohibido := range []uuid.UUID{ajenoEnProceso.ID, personasSinBea.ID} {
		if contiene(vs, prohibido) {
			t.Errorf("Bea ve %s y no debería", prohibido)
		}
	}
	// Quien administra lo ve todo.
	todo, err := s.Biblioteca(ctx, "acme", "root", true, store.Filtro{Limite: 10})
	if err != nil || len(todo) != 5 {
		t.Fatalf("admin ve %d de 5 (%v)", len(todo), err)
	}
	// «Mis vídeos» y paginación: un límite de 2 devuelve 2 y el cursor sigue.
	mios, err := s.Biblioteca(ctx, "acme", "ana", false, store.Filtro{OwnerSub: "ana", Limite: 2})
	if err != nil || len(mios) != 2 {
		t.Fatalf("mios: %d (%v)", len(mios), err)
	}
	resto, err := s.Biblioteca(ctx, "acme", "ana", false, store.Filtro{OwnerSub: "ana", Desde: &store.Cursor{Fecha: mios[1].CreatedAt, ID: mios[1].ID}, Limite: 10})
	if err != nil || len(resto) != 2 {
		t.Fatalf("resto tras el cursor: %d (%v)", len(resto), err)
	}
}

func TestCicloDeVida(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	v := listo(ctx, t, s, "acme", "ana", domain.VisibilityPublic, "")
	if v.Status != domain.StatusUploading || v.PublishedAt != nil {
		t.Fatalf("nace %s publicado=%v", v.Status, v.PublishedAt)
	}
	// Saltarse `processing` no vale.
	if _, err := s.CambiarEstado(ctx, "acme", v.ID, store.Estado{A: domain.StatusReady}); !errors.Is(err, domain.ErrConflicto) {
		t.Fatalf("uploading → ready: %v", err)
	}
	if _, err := s.CambiarEstado(ctx, "acme", v.ID, store.Estado{A: domain.StatusProcessing}); err != nil {
		t.Fatal(err)
	}
	dur := int64(90_000)
	v, err := s.CambiarEstado(ctx, "acme", v.ID, store.Estado{A: domain.StatusReady, DurationMS: &dur, HlsKey: ptr("x/hls/master.m3u8"), PosterKey: ptr("x/poster.jpg")})
	if err != nil {
		t.Fatal(err)
	}
	if v.PublishedAt == nil || v.DurationMS == nil || *v.DurationMS != dur || v.PosterKey == "" {
		t.Fatalf("ready sin publicar/duración/póster: %+v", v)
	}
	// Un fallo guarda el motivo; volver a procesar lo limpia.
	live, err := s.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", Kind: domain.KindLive, Visibility: domain.VisibilityOrg, Title: "Directo"})
	if err != nil || live.Status != domain.StatusScheduled {
		t.Fatalf("un directo nace scheduled: %v %v", live.Status, err)
	}
	live, err = s.CambiarEstado(ctx, "acme", live.ID, store.Estado{A: domain.StatusLive})
	if err != nil || live.LiveStartedAt == nil || live.PublishedAt == nil {
		t.Fatalf("live sin marcas: %+v %v", live, err)
	}
	live, err = s.CambiarEstado(ctx, "acme", live.ID, store.Estado{A: domain.StatusEnded})
	if err != nil || live.LiveEndedAt == nil {
		t.Fatalf("ended sin marca: %+v %v", live, err)
	}
	if _, err := s.CambiarEstado(ctx, "acme", live.ID, store.Estado{A: domain.StatusProcessing}); err != nil {
		t.Fatalf("un directo acabado se procesa como grabación: %v", err)
	}
	fallo, err := s.CambiarEstado(ctx, "acme", live.ID, store.Estado{A: domain.StatusFailed, Failure: ptr("ffmpeg: códec")})
	if err != nil || fallo.Failure == "" {
		t.Fatalf("failed sin motivo: %+v %v", fallo, err)
	}
	otra, err := s.CambiarEstado(ctx, "acme", live.ID, store.Estado{A: domain.StatusProcessing})
	if err != nil || otra.Failure != "" {
		t.Fatalf("reintentar debería limpiar el motivo: %+v %v", otra, err)
	}
}

func TestLosCheckDelEsquemaSonEntradaInvalida(t *testing.T) {
	ctx := ctxCorto(t)
	s := montar(t)
	_, err := s.Crear(ctx, store.Nuevo{Tenant: "acme", OwnerSub: "ana", Kind: domain.KindUpload, Visibility: domain.VisibilityOrg, Title: ""})
	if !errors.Is(err, domain.ErrEntradaInvalida) {
		t.Fatalf("título vacío: %v", err)
	}
}

func ptr[T any](v T) *T { return &v }

func contiene(vs []domain.Video, id uuid.UUID) bool {
	for _, v := range vs {
		if v.ID == id {
			return true
		}
	}
	return false
}

// La cola: el servicio encola en la organización, el transcodificador reclama en
// mantenimiento (SKIP LOCKED), y terminar o fallar mueve el vídeo en la misma
// transacción. Tres reintentos con espera creciente; el cuarto fallo deja el
// vídeo en failed.
func TestColaDeTranscodificacion(t *testing.T) {
	ctx := ctxCorto(t)
	s, h := montarConHarness(t)
	// Adelanta el reloj de los reintentos: lo que espera se puede reclamar ya.
	yaToca := func() {
		comoSuperusuario(ctx, t, h, `UPDATE transcode_jobs SET next_attempt_at = now() WHERE status = 'pending'`)
	}
	v := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, "")
	if _, err := s.Encolar(ctx, "acme", v.ID, domain.JobKind("dance"), nil); !errors.Is(err, domain.ErrEntradaInvalida) {
		t.Fatalf("clase desconocida: %v", err)
	}
	// La subida registra el original y encola en una transacción.
	v, err := s.RecibirOriginal(ctx, "acme", v.ID, v.ID.String()+"/original", 1024)
	if err != nil || v.Status != domain.StatusProcessing || v.OriginalKey == "" || v.BytesTotal != 1024 {
		t.Fatalf("RecibirOriginal: %+v %v", v, err)
	}
	trabajos, err := s.TrabajosDelVideo(ctx, "acme", v.ID)
	if err != nil || len(trabajos) != 1 || trabajos[0].Kind != domain.JobUpload || string(trabajos[0].Payload) != "{}" {
		t.Fatalf("tras la subida: %+v %v", trabajos, err)
	}
	// Encolar otra vez lo mismo no duplica: devuelve el activo.
	otra, err := s.Encolar(ctx, "acme", v.ID, domain.JobUpload, nil)
	if err != nil || otra.ID != trabajos[0].ID {
		t.Fatalf("encolar dos veces duplicó el trabajo: %v %v", otra.ID, err)
	}
	j := trabajos[0]
	if _, err := s.Reclamar(ctx, "otra-replica"); err != nil {
		t.Fatalf("Reclamar: %v", err)
	}
	if _, err := s.Reclamar(ctx, "otra-replica"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("no debería quedar nada que reclamar: %v", err)
	}
	vj, err := s.VideoDelTrabajo(ctx, j)
	if err != nil || vj.ID != v.ID {
		t.Fatalf("VideoDelTrabajo: %+v %v", vj, err)
	}
	// Falla tres veces: vuelve a pending CON ESPERA y el vídeo sigue processing.
	for i := 1; i <= 3; i++ {
		reintenta, err := s.Fallar(ctx, j.ID, "ffmpeg: fallo")
		if err != nil || !reintenta {
			t.Fatalf("fallo %d: reintenta=%v err=%v", i, reintenta, err)
		}
		if _, err := s.Reclamar(ctx, "replica"); !errors.Is(err, domain.ErrNoExiste) {
			t.Fatalf("tras el fallo %d se reclamó sin esperar: %v", i, err)
		}
		yaToca()
		if _, err := s.Reclamar(ctx, "replica"); err != nil {
			t.Fatalf("reclamar tras la espera del fallo %d: %v", i, err)
		}
	}
	vv, _ := s.EnOrganizacion(ctx, "acme", v.ID)
	if vv.Status != domain.StatusProcessing {
		t.Fatalf("tras tres fallos el vídeo debería seguir processing: %s", vv.Status)
	}
	reintenta, err := s.Fallar(ctx, j.ID, "ffmpeg: códec")
	if err != nil || reintenta {
		t.Fatalf("cuarto fallo: reintenta=%v err=%v", reintenta, err)
	}
	vv, _ = s.EnOrganizacion(ctx, "acme", v.ID)
	if vv.Status != domain.StatusFailed || vv.Failure != "ffmpeg: códec" {
		t.Fatalf("el cuarto fallo deja el vídeo failed con motivo: %s %q", vv.Status, vv.Failure)
	}

	// Reintentar de verdad: nuevo trabajo, se termina, y el vídeo queda ready con derivadas.
	j2, err := s.Encolar(ctx, "acme", v.ID, domain.JobUpload, nil)
	if err != nil || j2.ID == j.ID {
		t.Fatalf("reencolar tras un fallo definitivo: %v", err)
	}
	if _, err := s.Reclamar(ctx, "replica"); err != nil {
		t.Fatal(err)
	}
	pre := v.ID.String() + "/"
	dur, w, hgt, total := int64(2000), int32(1280), int32(720), int64(4096)
	listoV, err := s.Terminar(ctx, j2.ID, store.Estado{DurationMS: &dur, Width: &w, Height: &hgt, HlsKey: ptr(pre + "hls/master.m3u8"), WebmKey: ptr(pre + "webm/720p.webm"), PosterKey: ptr(pre + "poster.jpg"), BytesTotal: &total})
	if err != nil || listoV.Status != domain.StatusReady || listoV.Failure != "" || listoV.HlsKey == "" || listoV.BytesTotal != total || listoV.Width == nil {
		t.Fatalf("Terminar: %+v %v", listoV, err)
	}
	if _, err := s.Terminar(ctx, j2.ID, store.Estado{}); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("terminar dos veces: %v", err)
	}

	// Un póster NO saca el vídeo de la vista: sigue ready mientras se cuece, y si
	// falla del todo el vídeo no pasa a failed.
	jp, err := s.Encolar(ctx, "acme", v.ID, domain.JobPoster, []byte(`{"at_ms": 1000}`))
	if err != nil {
		t.Fatal(err)
	}
	if vv, _ := s.EnOrganizacion(ctx, "acme", v.ID); vv.Status != domain.StatusReady {
		t.Fatalf("pedir un póster sacó el vídeo de ready: %s", vv.Status)
	}
	for i := 0; i < 4; i++ {
		yaToca()
		if _, err := s.Reclamar(ctx, "replica"); err != nil {
			t.Fatalf("reclamar póster %d: %v", i, err)
		}
		if _, err := s.Fallar(ctx, jp.ID, "sin fotograma"); err != nil {
			t.Fatal(err)
		}
	}
	if vv, _ := s.EnOrganizacion(ctx, "acme", v.ID); vv.Status != domain.StatusReady || vv.Failure != "" {
		t.Fatalf("un póster fallido tocó el vídeo: %s %q", vv.Status, vv.Failure)
	}
	jp2, _ := s.Encolar(ctx, "acme", v.ID, domain.JobPoster, nil)
	if _, err := s.Reclamar(ctx, "replica"); err != nil {
		t.Fatal(err)
	}
	conPoster, err := s.Terminar(ctx, jp2.ID, store.Estado{PosterKey: ptr(pre + "poster-2.jpg")})
	if err != nil || conPoster.Status != domain.StatusReady || conPoster.PosterKey != pre+"poster-2.jpg" {
		t.Fatalf("póster terminado: %+v %v", conPoster, err)
	}

	// Lo que ocupa la organización es lo que se declara a Drive.
	n, err := s.BytesDeOrganizacion(ctx, "acme")
	if err != nil || n != total {
		t.Fatalf("BytesDeOrganizacion: %d %v", n, err)
	}
	// Borrar cancela lo que quedaba en cola y deja de contar.
	if _, err := s.Encolar(ctx, "acme", v.ID, domain.JobPoster, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Borrar(ctx, "acme", v.ID); err != nil {
		t.Fatal(err)
	}
	yaToca()
	if _, err := s.Reclamar(ctx, "replica"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("un trabajo de un vídeo borrado sigue en la cola: %v", err)
	}
	if n, _ := s.BytesDeOrganizacion(ctx, "acme"); n != 0 {
		t.Fatalf("borrado y sigue contando: %d", n)
	}
}

// Un trabajo en running cuya réplica murió vuelve a la cola pasado domain.Huerfano,
// y terminar un trabajo de un vídeo borrado cierra el trabajo sin fallar.
func TestColaHuerfanosYVideoBorrado(t *testing.T) {
	ctx := ctxCorto(t)
	s, h := montarConHarness(t)
	v := listo(ctx, t, s, "acme", "ana", domain.VisibilityOrg, "")
	if _, err := s.RecibirOriginal(ctx, "acme", v.ID, v.ID.String()+"/original", 10); err != nil {
		t.Fatal(err)
	}
	j, err := s.Reclamar(ctx, "replica-muerta")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Reclamar(ctx, "replica-viva"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("un trabajo recién cogido no es huérfano: %v", err)
	}
	comoSuperusuario(ctx, t, h, `UPDATE transcode_jobs SET claimed_at = now() - interval '4 hours' WHERE id = $1`, j.ID)
	j2, err := s.Reclamar(ctx, "replica-viva")
	if err != nil || j2.ID != j.ID || j2.ClaimedBy != "replica-viva" || j2.Attempts != 2 {
		t.Fatalf("el huérfano no volvió a la cola: %+v %v", j2, err)
	}
	if _, err := s.Borrar(ctx, "acme", v.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Terminar(ctx, j2.ID, store.Estado{}); !errors.Is(err, store.ErrVideoCambio) {
		t.Fatalf("terminar el trabajo de un vídeo borrado: %v", err)
	}
	if _, err := s.Reclamar(ctx, "replica-viva"); !errors.Is(err, domain.ErrNoExiste) {
		t.Fatalf("el trabajo cerrado no debería volver: %v", err)
	}
}
