// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package tienda

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/pages/db"
	"git.sytue.eu/sytue/one/services/pages/internal/domain"
	"git.sytue.eu/sytue/one/services/pages/internal/ir"
	"git.sytue.eu/sytue/one/services/pages/internal/plantillas"
	"git.sytue.eu/sytue/one/services/pages/internal/store"
)

// publicarConFormulario deja en vivo la plantilla «producto», que trae un formulario, y
// devuelve su identificador y sus campos.
func publicarConFormulario(t *testing.T, super *pg.Pool, tenant string, sitio uuid.UUID) (string, []ir.CampoFormulario) {
	t.Helper()
	ctx := context.Background()
	p, err := plantillas.ObtenerPlantilla("producto")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p.IR)
	if err != nil {
		t.Fatal(err)
	}
	var formID string
	var campos []ir.CampoFormulario
	for id, n := range ir.Formularios(&p.IR) {
		if formID == "" || id < formID {
			formID, campos = id, n.Fields
		}
	}
	if formID == "" {
		t.Fatal("la plantilla «producto» ya no trae formulario: elige otra para el test")
	}
	var version uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO site_versions (tenant, site_id, kind, ir, draft_rev, created_by)
		VALUES ($1, $2, 'publish', $3, 1, 'sub') RETURNING id`, tenant, sitio, raw).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `INSERT INTO publications (tenant, site_id, kind, number, version_id, files, bytes, renderer_version, created_by)
		VALUES ($1, $2, 'live', 1, $3, 1, 1, 'test', 'sub')`, tenant, sitio, version); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `UPDATE sites SET published_number = 1 WHERE id = $1 AND tenant = $2`, sitio, tenant); err != nil {
		t.Fatal(err)
	}
	return formID, campos
}

// valoresValidos rellena cada campo con un valor de su tipo.
func valoresValidos(campos []ir.CampoFormulario) map[string]any {
	v := map[string]any{}
	for _, c := range campos {
		switch c.Kind {
		case "checkbox":
			v[c.ID] = true
		case "number":
			n := 1.0
			if c.Min != nil {
				n = float64(*c.Min)
			}
			v[c.ID] = n
		case "select":
			v[c.ID] = 0.0
		case "date":
			v[c.ID] = "2026-10-16"
		case "time":
			v[c.ID] = "10:00"
		case "email":
			v[c.ID] = "luis.formulario@example.test"
		default:
			v[c.ID] = "Luis"
		}
	}
	return v
}

// WEB-07. Un envío de formulario solo entra si el formulario está encendido y existe en
// la versión publicada, y solo con sus campos, de su tipo y su largo; pasado el tope
// diario, se guarda pero no avisa.
func TestUnFormularioSoloAceptaLoQuePublico(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web', 'Web', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	formID, campos := publicarConFormulario(t, super, org, sitio)
	if _, err := super.Exec(ctx, `INSERT INTO form_settings (site_id, tenant, form_id, enabled, notify)
		VALUES ($1, $2, $3, true, '[{"email":"aviso@example.test"}]')`, sitio, org, formID); err != nil {
		t.Fatal(err)
	}
	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil)
	enviar := func(campos map[string]any) error {
		return svc.EnviarFormularioPublico(ctx, org, sitio, formID, campos, true, "es")
	}
	contar := func(sql string) int {
		t.Helper()
		var n int
		if err := super.QueryRow(ctx, sql, sitio).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	guardados := func() int { return contar(`SELECT count(*) FROM form_submissions WHERE site_id = $1`) }

	conIntruso := valoresValidos(campos)
	conIntruso["intruso"] = strings.Repeat("x", 1000)
	if err := enviar(conIntruso); !errors.Is(err, domain.ErrBorradorInvalido) {
		t.Fatalf("un campo que no es del formulario: %v; quiero rechazo", err)
	}
	largo := valoresValidos(campos)
	for _, c := range campos {
		if c.Kind == "text" || c.Kind == "textarea" || c.Kind == "email" || c.Kind == "tel" {
			largo[c.ID] = strings.Repeat("x", 6000)
			break
		}
	}
	if err := enviar(largo); !errors.Is(err, domain.ErrBorradorInvalido) {
		t.Fatalf("un campo más largo de lo que admite: %v; quiero rechazo", err)
	}
	if guardados() != 0 {
		t.Fatal("se guardó un envío rechazado")
	}
	if err := enviar(valoresValidos(campos)); err != nil {
		t.Fatalf("un envío válido: %v", err)
	}

	// Tope de avisos del día: el envío entra, el aviso no.
	if _, err := super.Exec(ctx, `INSERT INTO events_outbox (id, channel, type, tenant, payload)
		SELECT gen_random_uuid(), 'pages', 'pages.form.submitted', $1, jsonb_build_object('site_id', $2::text)
		  FROM generate_series(1, 200)`, org, sitio); err != nil {
		t.Fatal(err)
	}
	if err := enviar(valoresValidos(campos)); err != nil || guardados() != 2 {
		t.Fatalf("pasado el tope: err=%v y %d guardados; quiero el envío guardado", err, guardados())
	}
	if n := contar(`SELECT count(*) FROM events_outbox WHERE payload->>'site_id' = $1::text`); n != 201 {
		t.Fatalf("avisos del sitio: %d; quiero 201 (el del primer envío y los 200 sembrados)", n)
	}

	// Apagado, el formulario no existe.
	if _, err := super.Exec(ctx, `UPDATE form_settings SET enabled = false WHERE site_id = $1`, sitio); err != nil {
		t.Fatal(err)
	}
	if err := enviar(valoresValidos(campos)); !errors.Is(err, domain.ErrFormularioNoExiste) {
		t.Fatalf("formulario apagado: %v; quiero ErrFormularioNoExiste", err)
	}
}

// Un formulario publicado sin fila previa en form_settings (p. ej. sembrado antes de
// WEB-07) sigue existiendo: su definición pública trae los campos y el consentimiento de
// la versión en vivo, y acepta envíos.
func TestUnFormularioPublicadoSeVeYSeEnviaSinConfigurarlo(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web', 'Web', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	formID, campos := publicarConFormulario(t, super, org, sitio)
	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil)

	def, err := svc.ObtenerFormularioPublico(ctx, org, sitio, formID)
	if err != nil {
		t.Fatalf("definición de un formulario publicado: %v", err)
	}
	// Leída como la lee el navegador.
	crudo, _ := json.Marshal(def)
	var leida struct {
		Fields []struct {
			ID string `json:"id"`
		} `json:"fields"`
		ConsentText map[string]string `json:"consent_text"`
	}
	if err := json.Unmarshal(crudo, &leida); err != nil || len(campos) == 0 || len(leida.Fields) != len(campos) || leida.Fields[0].ID != campos[0].ID || len(leida.ConsentText) == 0 {
		t.Fatalf("definición %s; quiero los %d campos y el consentimiento publicados", crudo, len(campos))
	}
	if _, err := svc.ObtenerFormularioPublico(ctx, org, sitio, "noexiste00"); !errors.Is(err, domain.ErrFormularioNoExiste) {
		t.Fatalf("un formulario que no está publicado: %v; quiero ErrFormularioNoExiste", err)
	}
	if err := svc.EnviarFormularioPublico(ctx, org, sitio, formID, valoresValidos(campos), true, "es"); err != nil {
		t.Fatalf("envío a un formulario publicado sin configurar: %v", err)
	}

	// Apagado a mano, deja de existir también para el GET.
	if _, err := super.Exec(ctx, `INSERT INTO form_settings (site_id, tenant, form_id, enabled) VALUES ($1, $2, $3, false)`, sitio, org, formID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObtenerFormularioPublico(ctx, org, sitio, formID); !errors.Is(err, domain.ErrFormularioNoExiste) {
		t.Fatalf("formulario apagado: %v; quiero ErrFormularioNoExiste", err)
	}
}

// WEB-07. El IR publicado se cachea en memoria por sitio y versión (5 min, §10.6) para no
// decodificar el JSONB en cada envío, y al publicar una versión nueva se invalida en el acto.
func TestCacheDeFormularioPublicadoPorSitioYVersion(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web', 'Web', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}
	formID, campos := publicarConFormulario(t, super, org, sitio)
	svc := Nuevo(store.New(super), nil, "https://web.example.test", "sites.example.test", nil, nil)

	// Primera consulta: calienta la caché de (sitio, versión 1).
	if _, err := svc.ObtenerFormularioPublico(ctx, org, sitio, formID); err != nil {
		t.Fatalf("primer lectura del formulario publicado: %v", err)
	}

	// Si estropeamos el JSONB de la versión 1 en BD sin cambiar published_number,
	// la caché de 5 min responde sin releer ni volver a decodificar site_versions.ir.
	if _, err := super.Exec(ctx, `UPDATE site_versions SET ir = '{}'::jsonb WHERE site_id = $1`, sitio); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnviarFormularioPublico(ctx, org, sitio, formID, valoresValidos(campos), true, "es"); err != nil {
		t.Fatalf("el segundo envío dentro de los 5 min debía servirse de la caché de la versión publicada: %v", err)
	}

	// En cuanto cambia la versión publicada (published_number = 2, con el IR vacío),
	// la caché de la versión 1 deja de aplicar inmediatamente.
	var v2 uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO site_versions (tenant, site_id, kind, ir, draft_rev, created_by)
		VALUES ($1, $2, 'publish', '{}'::jsonb, 2, 'sub') RETURNING id`, org, sitio).Scan(&v2); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `INSERT INTO publications (tenant, site_id, kind, number, version_id, files, bytes, renderer_version, created_by)
		VALUES ($1, $2, 'live', 2, $3, 1, 1, 'test', 'sub')`, org, sitio, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := super.Exec(ctx, `UPDATE sites SET published_number = 2 WHERE id = $1 AND tenant = $2`, sitio, org); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ObtenerFormularioPublico(ctx, org, sitio, formID); !errors.Is(err, domain.ErrFormularioNoExiste) {
		t.Fatalf("tras publicar la versión 2 sin formulario: %v; quiero ErrFormularioNoExiste", err)
	}
}

// WEB-07. Al publicar un sitio en vivo (CrearPublicacionLive / Rollback), se crean las
// filas por defecto en form_settings (enabled=true, notify=[]) para los formularios de la
// versión publicada, sin pisar la configuración existente (incluidos los desactivados con
// enabled=false o con destinatarios ya configurados).
func TestPublicarCreaFormSettingsPorDefectoSinPisarLosExistentes(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := context.Background()
	super := h.PoolSinRLS(t)
	const org = "org"
	tctx := tenancy.Into(ctx, tenancy.Tenant{Slug: org})

	var sitio uuid.UUID
	if err := super.QueryRow(ctx, `INSERT INTO sites (tenant, slug, name, draft, created_by) VALUES ($1, 'web', 'Web', '{}', 'sub') RETURNING id`, org).Scan(&sitio); err != nil {
		t.Fatal(err)
	}

	p, err := plantillas.ObtenerPlantilla("producto")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p.IR)
	if err != nil {
		t.Fatal(err)
	}
	var formID string
	for id := range ir.Formularios(&p.IR) {
		if formID == "" || id < formID {
			formID = id
		}
	}
	if formID == "" {
		t.Fatal("la plantilla «producto» no trae formulario")
	}

	st := store.New(h.Pool)
	v1, err := st.CrearVersionConTipo(tctx, sitio, "v1", "publish", raw, 1, "sub")
	if err != nil {
		t.Fatalf("crear versión 1: %v", err)
	}
	if _, err := st.CrearPublicacionLive(tctx, sitio, v1, 1, 100, "1.0", nil, "sub", nil); err != nil {
		t.Fatalf("publicar versión 1: %v", err)
	}

	var enabled bool
	var notify string
	if err := super.QueryRow(ctx, `SELECT enabled, notify::text FROM form_settings WHERE site_id = $1 AND tenant = $2 AND form_id = $3`, sitio, org, formID).Scan(&enabled, &notify); err != nil {
		t.Fatalf("al publicar debía crearse la fila por defecto en form_settings: %v", err)
	}
	if !enabled || notify != "[]" {
		t.Fatalf("fila inicial en form_settings: enabled=%v notify=%s; quiero true y []", enabled, notify)
	}

	// Si la organización apaga el formulario y configura un correo de aviso, volver a
	// publicar no pisa ni enabled=false ni notify.
	if _, err := super.Exec(ctx, `UPDATE form_settings SET enabled = false, notify = '[{"email":"aviso@example.test"}]'::jsonb WHERE site_id = $1 AND tenant = $2 AND form_id = $3`, sitio, org, formID); err != nil {
		t.Fatal(err)
	}
	v2, err := st.CrearVersionConTipo(tctx, sitio, "v2", "publish", raw, 2, "sub")
	if err != nil {
		t.Fatalf("crear versión 2: %v", err)
	}
	if _, err := st.CrearPublicacionLive(tctx, sitio, v2, 1, 100, "1.0", nil, "sub", nil); err != nil {
		t.Fatalf("publicar versión 2: %v", err)
	}
	if err := super.QueryRow(ctx, `SELECT enabled, notify::text FROM form_settings WHERE site_id = $1 AND tenant = $2 AND form_id = $3`, sitio, org, formID).Scan(&enabled, &notify); err != nil {
		t.Fatal(err)
	}
	if enabled || !strings.Contains(notify, "aviso@example.test") {
		t.Fatalf("la segunda publicación pisó form_settings: enabled=%v notify=%s", enabled, notify)
	}
}

