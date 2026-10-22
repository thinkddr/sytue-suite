// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/docs/db"
	"git.sytue.eu/sytue/one/services/docs/internal/domain"
	"git.sytue.eu/sytue/one/services/docs/internal/store"
)

// enUnMes es una caducidad válida bajo cualquier techo razonable: desde el techo de
// caducidad, un enlace sin fecha solo vale en una organización «sin techo».
func enUnMes() *time.Time {
	v := time.Now().Add(30 * 24 * time.Hour)
	return &v
}

const dia = 24 * time.Hour

// cerca: ¿está `t` a menos de dos minutos de `de`?
func cerca(t *time.Time, de time.Time) bool {
	return t != nil && t.Sub(de).Abs() < 2*time.Minute
}

type enlacesDePrueba struct {
	t    *testing.T
	st   *store.Store
	raiz *pgx.Conn // superusuario: para envejecer enlaces y correr el bloque de la migración
	doc  map[string]string
}

func nuevosEnlacesDePrueba(t *testing.T) *enlacesDePrueba {
	t.Helper()
	h := harnessComoEnProduccion(t)
	raiz, err := pgx.Connect(context.Background(), h.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raiz.Close(context.Background()) })
	e := &enlacesDePrueba{t: t, st: store.New(h.Pool), raiz: raiz, doc: map[string]string{}}
	for _, org := range []string{"acme", "globex"} {
		d, _, err := e.st.CreateDocument(tenantCtx(org), domain.CreateDocumentParams{Kind: domain.KindDocument, Title: "T", OwnerSub: "owner"})
		if err != nil {
			t.Fatal(err)
		}
		e.doc[org] = d.ID
	}
	return e
}

// crear deja un enlace con esa caducidad, creado hace `edad`. Va por SQL y no por el
// almacén: son enlaces «de antes», y el almacén ya no deja crear los que incumplen el techo.
func (e *enlacesDePrueba) crear(org, token string, caduca *time.Time, edad time.Duration) string {
	e.t.Helper()
	var id string
	if err := e.raiz.QueryRow(context.Background(), `
		INSERT INTO doc_share_links (document_id, tenant, token_hash, role, created_by, expires_at, created_at)
		VALUES ($1, $2, $3, 'viewer', 'owner', $4, now() - make_interval(secs => $5))
		RETURNING id::text`,
		e.doc[org], org, domain.HashToken(token), caduca, edad.Seconds()).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *enlacesDePrueba) enlace(org, id string) domain.ShareLink {
	e.t.Helper()
	ls, err := e.st.ListShareLinks(tenantCtx(org), e.doc[org])
	if err != nil {
		e.t.Fatal(err)
	}
	for _, l := range ls {
		if l.ID == id {
			return l
		}
	}
	e.t.Fatalf("no está el enlace %s", id)
	return domain.ShareLink{}
}

func (e *enlacesDePrueba) resuelve(token string) bool {
	e.t.Helper()
	_, err := e.st.ResolveShareToken(context.Background(), domain.HashToken(token))
	_, errCanje := e.st.EnlaceParaCanjear(context.Background(), domain.HashToken(token))
	if (err == nil) != (errCanje == nil) {
		e.t.Fatalf("resolver (%v) y canjear (%v) no dicen lo mismo de %s", err, errCanje, token)
	}
	if err != nil && !errors.Is(err, domain.ErrShareExpired) {
		e.t.Fatal(err)
	}
	return err == nil
}

// El techo de caducidad de la organización, contra PostgreSQL y con las funciones en
// manos del rol de la aplicación: el techo por defecto, «sin techo» como única puerta
// de los enlaces sin caducidad, el ajuste de los enlaces que ya existían (avisan, no se
// rompen) y el aislamiento entre organizaciones.
func TestTechoDeCaducidadDeEnlaces(t *testing.T) {
	e := nuevosEnlacesDePrueba(t)
	acme, globex := tenantCtx("acme"), tenantCtx("globex")
	ahora := time.Now()
	en := func(d time.Duration) *time.Time { v := ahora.Add(d); return &v }

	if p, err := e.st.PoliticaEnlaces(acme); err != nil || p.MaxDias != domain.TechoEnlacesPorDefecto || !p.PorDefecto {
		t.Fatalf("política por defecto: %+v %v", p, err)
	}

	sinFecha := e.crear("acme", "sin-fecha", nil, 0)
	lejos := e.crear("acme", "lejos", en(730*dia), 0)
	corto := e.crear("acme", "corto", en(3*dia), 0)
	viejo := e.crear("acme", "viejo", nil, 400*dia)
	revocado := e.crear("acme", "revocado", nil, 0)
	if err := e.st.RevokeShareLink(acme, e.doc["acme"], revocado); err != nil {
		t.Fatal(err)
	}
	ajeno := e.crear("globex", "ajeno", nil, 0)

	// Al comprobar: un enlace sin caducidad no vale mientras la organización tenga techo,
	// aunque la fila exista (una carrera, un dato heredado).
	if e.resuelve("sin-fecha") {
		t.Fatal("un enlace sin caducidad se resuelve con el techo por defecto")
	}
	if !e.resuelve("corto") {
		t.Fatal("un enlace con caducidad dentro del techo no se resuelve")
	}

	// El almacén no deja crear lo que incumple el techo, lo llame quien lo llame.
	for nombre, caduca := range map[string]*time.Time{"sin fecha": nil, "más allá del techo": en(400 * dia)} {
		if _, err := e.st.CreateShareLink(acme, e.doc["acme"], domain.HashToken("no-"+nombre), "", "owner", domain.RoleViewer, caduca); !errors.Is(err, domain.ErrCaducidadExcedida) {
			t.Errorf("crear %s con techo por defecto: %v", nombre, err)
		}
	}
	if _, err := e.st.CreateShareLink(acme, e.doc["acme"], domain.HashToken("si"), "", "owner", domain.RoleViewer, en(364*dia)); err != nil {
		t.Errorf("crear dentro del techo: %v", err)
	}

	// Rango del techo.
	for _, malo := range []int{-1, domain.TechoEnlacesMaximo + 1} {
		if _, err := e.st.GuardarPoliticaEnlaces(acme, malo, "admin"); !errors.Is(err, domain.ErrTechoInvalido) {
			t.Errorf("techo %d: %v", malo, err)
		}
	}

	// «Sin techo» (decisión explícita): los enlaces sin caducidad valen y no se toca ninguno.
	if n, err := e.st.GuardarPoliticaEnlaces(acme, 0, "admin"); err != nil || n != 0 {
		t.Fatalf("sin techo: %d %v", n, err)
	}
	if p, _ := e.st.PoliticaEnlaces(acme); p.MaxDias != 0 || p.PorDefecto {
		t.Fatalf("tras quitar el techo: %+v", p)
	}
	if !e.resuelve("sin-fecha") || !e.resuelve("viejo") {
		t.Fatal("sin techo, un enlace sin caducidad no se resuelve")
	}
	// ... pero solo en ESA organización.
	if e.resuelve("ajeno") {
		t.Fatal("el «sin techo» de acme abrió un enlace sin caducidad de globex")
	}

	// Techo de 7 días: los vivos que no lo cumplen pasan a caducar; ninguno se borra.
	n, err := e.st.GuardarPoliticaEnlaces(acme, 7, "admin")
	if err != nil || n != 4 {
		t.Fatalf("ajustados = %d (se esperaban sin-fecha, lejos, viejo y el de 364 días): %v", n, err)
	}
	for _, c := range []struct {
		nombre, id string
		hasta      time.Time
	}{
		{"sin fecha, recién creado", sinFecha, ahora.Add(7 * dia)},
		{"más allá del techo", lejos, ahora.Add(7 * dia)},
		// Creado hace 400 días: su techo ya pasó, así que caduca tras la gracia (7 < 30).
		{"viejo sin fecha", viejo, ahora.Add(7 * dia)},
	} {
		l := e.enlace("acme", c.id)
		if !cerca(l.ExpiresAt, c.hasta) || l.AdjustedAt == nil || l.RevokedAt != nil {
			t.Errorf("%s: caduca %v (se esperaba ~%v), ajustado %v, revocado %v", c.nombre, l.ExpiresAt, c.hasta, l.AdjustedAt, l.RevokedAt)
		}
	}
	if l := e.enlace("acme", corto); !cerca(l.ExpiresAt, ahora.Add(3*dia)) || l.AdjustedAt != nil {
		t.Errorf("un enlace que ya cumplía se tocó: %+v", l)
	}
	if l := e.enlace("acme", revocado); l.ExpiresAt != nil || l.AdjustedAt != nil {
		t.Errorf("un enlace revocado se tocó: %+v", l)
	}
	if l := e.enlace("globex", ajeno); l.ExpiresAt != nil || l.AdjustedAt != nil {
		t.Errorf("la política de acme tocó un enlace de globex: %+v", l)
	}
	if p, _ := e.st.PoliticaEnlaces(globex); !p.PorDefecto {
		t.Errorf("la política de acme se ve desde globex: %+v", p)
	}
	// Siguen sirviendo hasta su nueva fecha: avisan, no se rompen.
	for _, tk := range []string{"sin-fecha", "lejos", "viejo", "corto"} {
		if !e.resuelve(tk) {
			t.Errorf("%s dejó de servir al ajustarlo", tk)
		}
	}

	// La gracia: con un techo de 365, un enlace de hace 400 días no muere hoy sino en 30.
	antiguo := e.crear("globex", "antiguo", en(3000*dia), 400*dia)
	if n, err := e.st.GuardarPoliticaEnlaces(globex, 365, "admin"); err != nil || n != 2 {
		t.Fatalf("globex: ajustados = %d: %v", n, err)
	}
	if l := e.enlace("globex", antiguo); !cerca(l.ExpiresAt, ahora.Add(domain.GraciaEnlacesDias*dia)) {
		t.Errorf("gracia: caduca %v", l.ExpiresAt)
	}
	if l := e.enlace("globex", ajeno); !cerca(l.ExpiresAt, ahora.Add(365*dia)) {
		t.Errorf("recién creado con techo 365: caduca %v", l.ExpiresAt)
	}
	// Subir el techo no alarga lo ya ajustado.
	if n, err := e.st.GuardarPoliticaEnlaces(globex, 3650, "admin"); err != nil || n != 0 {
		t.Fatalf("subir el techo ajustó %d enlaces: %v", n, err)
	}
}

// El bloque de la migración 0024 que adapta los enlaces que ya existían, ejecutado tal
// cual está escrito en el fichero sobre enlaces «de antes»: los que no cumplen el techo
// por defecto pasan a caducar (nunca antes de 30 días), marcados; los demás no se tocan.
func TestLaMigracionDelTechoNoRompeLosEnlacesQueYaExistian(t *testing.T) {
	e := nuevosEnlacesDePrueba(t)
	ahora := time.Now()
	en := func(d time.Duration) *time.Time { v := ahora.Add(d); return &v }

	viejo := e.crear("acme", "viejo", nil, 400*dia)
	reciente := e.crear("acme", "reciente", nil, 10*dia)
	lejos := e.crear("globex", "lejos", en(3000*dia), 0)
	cumple := e.crear("acme", "cumple", en(20*dia), 0)
	caducado := e.crear("acme", "caducado", en(time.Second), 100*dia)
	revocado := e.crear("acme", "revocado", nil, 0)
	if err := e.st.RevokeShareLink(tenantCtx("acme"), e.doc["acme"], revocado); err != nil {
		t.Fatal(err)
	}

	sql, err := fs.ReadFile(db.Migrations, db.MigrationsDir+"/0024_techo_de_caducidad_de_enlaces.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	i := strings.Index(string(sql), "DO $$")
	j := strings.Index(string(sql), "END $$;")
	if i < 0 || j < i {
		t.Fatal("la migración 0024 ya no tiene el bloque que adapta los enlaces existentes")
	}
	// Con el rol de la APLICACIÓN (sin BYPASSRLS), que es quien migra en producción.
	if err := pgx.BeginFunc(context.Background(), e.raiz, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), "SET LOCAL ROLE sytue_app"); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), string(sql)[i:j+len("END $$;")])
		return err
	}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		nombre, org, id string
		hasta           *time.Time
		ajustado        bool
	}{
		{"sin fecha de hace 400 días: gracia de 30", "acme", viejo, en(30 * dia), true},
		{"sin fecha de hace 10 días: su techo", "acme", reciente, en(355 * dia), true},
		{"con fecha más allá del techo, de otra organización", "globex", lejos, en(365 * dia), true},
		{"ya cumplía", "acme", cumple, en(20 * dia), false},
		{"ya caducado", "acme", caducado, en(time.Second), false},
		{"revocado", "acme", revocado, nil, false},
	} {
		l := e.enlace(c.org, c.id)
		if (c.hasta == nil) != (l.ExpiresAt == nil) || (c.hasta != nil && !cerca(l.ExpiresAt, *c.hasta)) || (l.AdjustedAt != nil) != c.ajustado {
			t.Errorf("%s: caduca %v (se esperaba %v), ajustado %v", c.nombre, l.ExpiresAt, c.hasta, l.AdjustedAt)
		}
	}
}
