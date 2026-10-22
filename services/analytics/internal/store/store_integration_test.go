// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de Sytue Analytics contra un PostgreSQL REAL (pgtest, que
// conecta con un rol SIN BYPASSRLS: las políticas se evalúan de verdad).
//
// Lo que demuestran, y que ningún test unitario puede demostrar:
//
//   - un inquilino no ve NADA de otro, ni el crudo ni los agregados;
//   - el colector puede INSERTAR eventos y NO PUEDE LEERLOS (ADR 0026 §4: el que
//     escribe no puede leer), que es la mitad del argumento del radio de
//     explosión;
//   - sin flag de sesión no se ve nada (deny-by-default);
//   - las particiones se crean y se retiran con el privilegio que tiene el
//     servicio, que no es dueño de la tabla;
//   - el rollup produce los números que el panel lee, incluidas las visitas
//     reconstruidas por hueco de 30 minutos.
package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/analytics/db"
	"git.sytue.eu/sytue/one/services/analytics/internal/domain"
	"git.sytue.eu/sytue/one/services/analytics/internal/store"
)

func nuevoHarness(t *testing.T) (*pgtest.Harness, *store.Store) {
	t.Helper()
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	return h, store.New(h.Pool)
}

func ctxTenant(slug string) context.Context {
	return tenancy.Into(context.Background(), tenancy.Tenant{Slug: slug})
}

// insertaEvento escribe una fila de crudo COMO LO HACE EL COLECTOR: SQL directo
// bajo app.ingest='on', sin pasar por el store, para que lo único que pueda
// dejar pasar (o no) la escritura sea la política de la migración.
func insertaEvento(t *testing.T, h *pgtest.Harness, tenant, siteID string, ts time.Time, visitor []byte, name, path string) {
	t.Helper()
	ctx := context.Background()
	err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.ingest', 'on', true)"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO analytics_events
			    (tenant, site_id, ts, visitor, name, path, country, browser, os, device)
			VALUES ($1, $2, $3, $4, $5, $6, 'ES', 'Chrome', 'macOS', 'desktop')`,
			tenant, siteID, ts, visitor, name, path)
		return err
	})
	if err != nil {
		t.Fatalf("insertar evento: %v", err)
	}
}

func creaSitio(t *testing.T, st *store.Store, tenant, host string) store.Site {
	t.Helper()
	s, err := st.CreateSite(ctxTenant(tenant), host, nil)
	if err != nil {
		t.Fatalf("crear sitio %s: %v", host, err)
	}
	return s
}

// TestRLSAislaInquilinos: un inquilino no ve los eventos ni los agregados de
// otro. Sin política, la consulta devolvería las filas de los dos.
func TestRLSAislaInquilinos(t *testing.T) {
	h, st := nuevoHarness(t)
	ctx := context.Background()

	sitioA := creaSitio(t, st, "acme", "acme.ejemplo.com")
	sitioB := creaSitio(t, st, "globex", "globex.ejemplo.com")

	ayer := time.Now().UTC().Add(-2 * time.Hour)
	insertaEvento(t, h, "acme", sitioA.ID, ayer, make([]byte, 16), "pageview", "/a")
	insertaEvento(t, h, "globex", sitioB.ID, ayer, make([]byte, 16), "pageview", "/b")

	if err := st.Rollup(ctx, time.Now(), 100); err != nil {
		t.Fatalf("rollup: %v", err)
	}

	// Sitios.
	sitiosA, err := st.ListSites(ctxTenant("acme"))
	if err != nil {
		t.Fatalf("listar sitios de acme: %v", err)
	}
	if len(sitiosA) != 1 || sitiosA[0].Host != "acme.ejemplo.com" {
		t.Fatalf("acme ve %d sitios: %+v", len(sitiosA), sitiosA)
	}

	// Agregados: acme no ve NADA del sitio de globex, ni sabiendo su id.
	tot, err := st.Summary(ctxTenant("acme"), sitioB.ID, ayer.AddDate(0, 0, -1), time.Now())
	if err != nil {
		t.Fatalf("totales cruzados: %v", err)
	}
	if tot.Pageviews != 0 || tot.Days != 0 {
		t.Fatalf("acme ve los agregados de globex: %+v", tot)
	}
	// Y sí ve los suyos (si no, el test anterior pasaría con una base vacía).
	tot, err = st.Summary(ctxTenant("acme"), sitioA.ID, ayer.AddDate(0, 0, -1), time.Now())
	if err != nil {
		t.Fatalf("totales propios: %v", err)
	}
	if tot.Pageviews != 1 || tot.Visitors != 1 {
		t.Fatalf("acme no ve sus propios agregados: %+v", tot)
	}

	// Crudo: cuenta sin ningún filtro, con solo app.tenant fijado. Lo único que
	// puede esconder filas aquí es la política.
	if n := cuenta(t, h, "analytics_events", "app.tenant", "acme"); n != 1 {
		t.Fatalf("con app.tenant=acme se ven %d eventos, debía ser 1 (el suyo)", n)
	}
	if n := cuenta(t, h, "analytics_daily_breakdown", "app.tenant", "globex"); n == 0 {
		t.Fatal("globex no ve su propio desglose")
	}
}

// TestElColectorNoPuedeLeer: con app.ingest se INSERTA pero no se SELECCIONA.
// Es la mitad del argumento del radio de explosión del ADR §5.
func TestElColectorNoPuedeLeer(t *testing.T) {
	h, st := nuevoHarness(t)
	sitio := creaSitio(t, st, "acme", "acme.ejemplo.com")
	insertaEvento(t, h, "acme", sitio.ID, time.Now().UTC(), make([]byte, 16), "pageview", "/a")

	if n := cuenta(t, h, "analytics_events", "app.ingest", "on"); n != 0 {
		t.Fatalf("con app.ingest se leen %d eventos: el colector podría leer el crudo de todos", n)
	}
	// Pero sí puede resolver el sitio por su hostname (es lo único que necesita).
	if _, err := st.SiteByHost(context.Background(), "acme.ejemplo.com"); err != nil {
		t.Fatalf("el colector debe poder resolver el sitio: %v", err)
	}
}

// TestDenyByDefault: sin ningún flag de sesión, no se ve una sola fila.
func TestDenyByDefault(t *testing.T) {
	h, st := nuevoHarness(t)
	sitio := creaSitio(t, st, "acme", "acme.ejemplo.com")
	insertaEvento(t, h, "acme", sitio.ID, time.Now().UTC(), make([]byte, 16), "pageview", "/a")

	for _, tabla := range []string{"analytics_sites", "analytics_events", "analytics_salts", "analytics_daily"} {
		if n := cuenta(t, h, tabla, "", ""); n != 0 {
			t.Errorf("sin flags se ven %d filas de %s", n, tabla)
		}
	}
	// La sal no es visible ni con app.tenant: no es de nadie, es del servicio.
	if n := cuenta(t, h, "analytics_salts", "app.tenant", "acme"); n != 0 {
		t.Errorf("un inquilino ve %d filas de sal", n)
	}
}

// TestParticionesYRetencion: el servicio, que NO es dueño de la tabla, puede
// crear y retirar particiones vía las funciones SECURITY DEFINER.
func TestParticionesYRetencion(t *testing.T) {
	_, st := nuevoHarness(t)
	ctx := context.Background()

	if err := st.EnsurePartitions(ctx, 7); err != nil {
		t.Fatalf("crear particiones: %v", err)
	}
	// keep_days=2 conserva hoy y ayer; la migración creó de ayer en adelante, así
	// que no hay ninguna caducada todavía.
	n, err := st.ExpirePartitions(ctx, 2)
	if err != nil {
		t.Fatalf("purgar particiones: %v", err)
	}
	if n != 0 {
		t.Fatalf("no debía caducar ninguna partición, cayeron %d", n)
	}
}

// TestRollupCuentaVisitasYRebotes: la reconstrucción de visitas (mismo visitor,
// hueco < 30 min) es del rollup, no del navegador. No hay id de sesión.
func TestRollupCuentaVisitasYRebotes(t *testing.T) {
	h, st := nuevoHarness(t)
	ctx := context.Background()
	sitio := creaSitio(t, st, "acme", "acme.ejemplo.com")

	// Los cuatro eventos tienen que caer en el MISMO día UTC: el rollup agrega por
	// DÍA y `Summary` suma los días del rango, así que un visitante con eventos a
	// caballo de la medianoche cuenta como único en CADA uno de los dos días —lo
	// correcto, porque sin HLL no se pueden deduplicar únicos entre días—. Con
	// `ahora - 2h` a secas el test pasaba 22 horas al día y fallaba entre las
	// 00:00 y las 02:00 UTC, que es justo cuando corre la tanda nocturna:
	// «visitantes = 3, se esperaban 2» sin que nadie hubiera tocado la analítica.
	//
	// Anclarlo a un día ya cerrado —mediodía de ayer— arregla eso y rompe lo otro:
	// `Rollup` solo agrega los días que toca su ventana `[ahora-3h, ahora+1h)`, y
	// pasadas las 03:00 UTC ayer ya no entra. El test dejaría de ver sus propios
	// eventos 21 horas al día en vez de 2.
	//
	// Así que se queda dentro de la ventana y solo se recula cuando el tramo de 47
	// minutos fuera a cruzar la medianoche. Comprobado sobre los 1440 minutos del
	// día: ninguno cruza y ninguno se sale de la ventana del rollup.
	base := time.Now().UTC().Add(-2 * time.Hour)
	if fin := base.Add(47 * time.Minute); fin.YearDay() != base.YearDay() {
		y, m, d := fin.Date()
		base = time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Add(-time.Hour)
	}
	v1 := domain.Visitor([]byte("0123456789abcdef0123456789abcdef"), sitio.ID, "203.0.113.1", "chrome")
	v2 := domain.Visitor([]byte("0123456789abcdef0123456789abcdef"), sitio.ID, "203.0.113.2", "chrome")

	// v1: dos páginas seguidas (una visita, sin rebote) y otra 45 min después
	// (segunda visita, de una sola página: rebote).
	insertaEvento(t, h, "acme", sitio.ID, base, v1, "pageview", "/")
	insertaEvento(t, h, "acme", sitio.ID, base.Add(2*time.Minute), v1, "pageview", "/precios")
	insertaEvento(t, h, "acme", sitio.ID, base.Add(47*time.Minute), v1, "pageview", "/")
	// v2: una sola página (una visita, rebote).
	insertaEvento(t, h, "acme", sitio.ID, base.Add(time.Minute), v2, "pageview", "/")

	if err := st.Rollup(ctx, time.Now(), 100); err != nil {
		t.Fatalf("rollup: %v", err)
	}

	hoy := time.Now().UTC()
	tot, err := st.Summary(ctxTenant("acme"), sitio.ID, hoy.AddDate(0, 0, -1), hoy)
	if err != nil {
		t.Fatalf("totales: %v", err)
	}
	if tot.Pageviews != 4 {
		t.Errorf("páginas vistas = %d, se esperaban 4", tot.Pageviews)
	}
	if tot.Visitors != 2 {
		t.Errorf("visitantes = %d, se esperaban 2", tot.Visitors)
	}
	if tot.Visits != 3 {
		t.Errorf("visitas = %d, se esperaban 3 (el hueco de 45 min abre una nueva)", tot.Visits)
	}
	if tot.Bounces != 2 {
		t.Errorf("rebotes = %d, se esperaban 2", tot.Bounces)
	}

	rutas, err := st.Breakdown(ctxTenant("acme"), sitio.ID, "path", hoy.AddDate(0, 0, -1), hoy, 10)
	if err != nil {
		t.Fatalf("desglose: %v", err)
	}
	if len(rutas) != 2 || rutas[0].Value != "/" || rutas[0].Pageviews != 3 {
		t.Fatalf("desglose por ruta inesperado: %+v", rutas)
	}

	// Idempotente: repetir el rollup no duplica nada.
	if err := st.Rollup(ctx, time.Now(), 100); err != nil {
		t.Fatalf("segundo rollup: %v", err)
	}
	tot2, err := st.Summary(ctxTenant("acme"), sitio.ID, hoy.AddDate(0, 0, -1), hoy)
	if err != nil {
		t.Fatalf("totales tras repetir: %v", err)
	}
	if tot2 != tot {
		t.Fatalf("el rollup no es idempotente: %+v vs %+v", tot2, tot)
	}
}

// TestElCajonDeOtras comprueba que el top-N acota la cardinalidad y que lo que
// se sale del top va a «(otras)» en vez de desaparecer.
func TestElCajonDeOtras(t *testing.T) {
	h, st := nuevoHarness(t)
	ctx := context.Background()
	sitio := creaSitio(t, st, "acme", "acme.ejemplo.com")

	base := time.Now().UTC().Add(-time.Hour)
	visitante := make([]byte, 16)
	for i := range 5 {
		// La ruta /popular se lleva más vistas que las demás.
		insertaEvento(t, h, "acme", sitio.ID, base.Add(time.Duration(i)*time.Second), visitante, "pageview", "/popular")
	}
	for i := range 4 {
		insertaEvento(t, h, "acme", sitio.ID, base.Add(time.Duration(10+i)*time.Second), visitante, "pageview",
			"/rara-"+time.Duration(i).String())
	}

	if err := st.Rollup(ctx, time.Now(), 1); err != nil { // top-1
		t.Fatalf("rollup: %v", err)
	}
	hoy := time.Now().UTC()
	rutas, err := st.Breakdown(ctxTenant("acme"), sitio.ID, "path", hoy.AddDate(0, 0, -1), hoy, 10)
	if err != nil {
		t.Fatalf("desglose: %v", err)
	}
	if len(rutas) != 2 {
		t.Fatalf("con top-1 se esperaban 2 filas (la top y «(otras)»): %+v", rutas)
	}
	if rutas[0].Value != "/popular" || rutas[0].Pageviews != 5 {
		t.Errorf("la fila top es %+v", rutas[0])
	}
	if rutas[1].Value != "(otras)" || rutas[1].Pageviews != 4 {
		t.Errorf("el cajón es %+v, se esperaban 4 vistas", rutas[1])
	}
}

// cuenta devuelve cuántas filas de una tabla ve una transacción con la GUC dada
// (o sin ninguna). Sin filtro en el WHERE: el recuento ES lo que deja ver la RLS.
func cuenta(t *testing.T, h *pgtest.Harness, tabla, guc, valor string) int {
	t.Helper()
	ctx := context.Background()
	var n int
	err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if guc != "" {
			if _, err := tx.Exec(ctx, "SELECT set_config($1, $2, true)", guc, valor); err != nil {
				return err
			}
		}
		return tx.QueryRow(ctx, "SELECT count(*) FROM "+tabla).Scan(&n) //nolint:gosec // G202: `tabla` es una constante del propio test.
	})
	if err != nil {
		t.Fatalf("contar %s (%s=%s): %v", tabla, guc, valor, err)
	}
	return n
}

// Guarda de compilación: el store debe seguir sirviendo el flag de mantenimiento
// que usan el rollup y la purga.
var _ = pg.MaintenanceSettingKey
