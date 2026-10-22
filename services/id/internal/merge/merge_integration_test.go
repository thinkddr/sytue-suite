// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración del MOTOR DE FUSIÓN contra PostgreSQL real (pgtest).
//
// Se ejercita sobre el esquema de Sytue Analytics, que es el caso difícil de la
// suite y el que estuvo roto: tiene RLS forzada como todos, pero además su tabla
// de eventos está PARTICIONADA POR DÍA, que es donde el motor se equivocaba de
// relación sin decir nada.
//
// Lo que aquí importa es que el fallo característico de esta operación es MUDO.
// Bajo FORCE ROW LEVEL SECURITY las políticas se aplican también al dueño de la
// tabla, así que una transacción sin `app.merge='on'` sencillamente NO VE las
// filas del origen: el `UPDATE ... SET tenant = destino` afecta a cero filas, no
// devuelve error, y la fusión se declara completada habiendo movido nada. Por eso
// el pool de pgtest conecta con un rol NOSUPERUSER + NOBYPASSRLS: con un
// superusuario estos tests pasarían aunque no existiera ninguna política, y no
// demostrarían absolutamente nada.
package merge

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	analyticsdb "git.sytue.eu/sytue/one/services/analytics/db"
)

const (
	origen  = "org-absorbida"
	destino = "org-receptora"
)

// entorno levanta el esquema de analytics y siembra las dos organizaciones.
//
// La siembra va por el DSN PRIVILEGIADO (superusuario del contenedor) a
// propósito: insertar como la aplicación exigiría fijar `app.tenant` y, en los
// eventos, el flag del colector. Lo que se prueba aquí es el MOVIMIENTO, no la
// escritura de negocio; el movimiento sí corre con el rol de aplicación.
func entorno(t *testing.T) (*pgtest.Harness, *pg.Pool, context.Context) {
	t.Helper()
	h := pgtest.New(t, analyticsdb.Migrations, analyticsdb.MigrationsDir)
	ctx := context.Background()

	admin := h.PoolSinRLS(t)

	// Dos sitios, uno por organización. `host` es UNIQUE GLOBAL, así que dos
	// organizaciones nunca pueden chocar aquí: es la razón por la que esta base no
	// necesita gancho de desambiguación.
	for _, s := range []struct{ tenant, host string }{
		{origen, "absorbida.example"},
		{destino, "receptora.example"},
	} {
		if _, err := admin.Exec(ctx,
			`INSERT INTO analytics_sites (tenant, host) VALUES ($1, $2)`, s.tenant, s.host); err != nil {
			t.Fatalf("sembrar sitio %s: %v", s.host, err)
		}
	}

	// Eventos en la tabla PARTICIONADA (la partición de hoy la creó la migración
	// inicial con analytics_ensure_partitions) y un agregado diario. Cantidades
	// distintas por organización para que un movimiento en la dirección equivocada
	// se note en el recuento.
	const evento = `
		INSERT INTO analytics_events
		    (tenant, site_id, ts, visitor, name, path, country, browser, os, device)
		SELECT $1, s.id, now(), '\x000102030405060708090a0b0c0d0e0f'::bytea,
		       'pageview', '/', 'ES', 'firefox', 'linux', 'desktop'
		FROM analytics_sites s WHERE s.tenant = $1`
	for i := 0; i < 3; i++ {
		if _, err := admin.Exec(ctx, evento, origen); err != nil {
			t.Fatalf("sembrar evento del origen: %v", err)
		}
	}
	if _, err := admin.Exec(ctx, evento, destino); err != nil {
		t.Fatalf("sembrar evento del destino: %v", err)
	}
	const diario = `
		INSERT INTO analytics_daily (tenant, site_id, day, pageviews)
		SELECT $1, s.id, current_date, $2 FROM analytics_sites s WHERE s.tenant = $1`
	if _, err := admin.Exec(ctx, diario, origen, 10); err != nil {
		t.Fatalf("sembrar diario del origen: %v", err)
	}
	if _, err := admin.Exec(ctx, diario, destino, 20); err != nil {
		t.Fatalf("sembrar diario del destino: %v", err)
	}
	return h, admin, ctx
}

// cuenta filas de una tabla con un tenant dado, saltándose la RLS (rol
// privilegiado): la comprobación tiene que poder ver AMBAS organizaciones.
func cuenta(t *testing.T, ctx context.Context, admin *pg.Pool, tabla, tenant string) int {
	t.Helper()
	var n int
	// La tabla es una constante del propio test, no entrada de usuario.
	if err := admin.QueryRow(ctx,
		`SELECT count(*) FROM `+tabla+` WHERE tenant = $1`, tenant).Scan(&n); err != nil {
		t.Fatalf("contar %s de %s: %v", tabla, tenant, err)
	}
	return n
}

// TestMoveServiceMueveAnalyticsIncluidaLaTablaParticionada: la fusión reetiqueta
// de verdad, y lo hace por la tabla MADRE.
//
// Sytue Analytics no estaba en `merge.Services` ni tenía política de fusión: al
// absorber una organización, sus sitios y todo su histórico se quedaban con el
// tenant viejo. El cliente entraba en la organización destino y sus analíticas
// habían desaparecido, sin error en ninguna parte.
func TestMoveServiceMueveAnalyticsIncluidaLaTablaParticionada(t *testing.T) {
	h, admin, ctx := entorno(t)

	res, err := MoveService(ctx, h.Pool, origen, destino)
	if err != nil {
		t.Fatalf("MoveService: %v", err)
	}

	// 1 sitio + 3 eventos + 1 diario = 5. Si saliera 0 sin error, ese es
	// exactamente el fallo mudo que la política de fusión existe para impedir.
	if res.Total != 5 {
		t.Errorf("filas movidas = %d, quiero 5; detalle: %v", res.Total, res.Rows)
	}
	// La MADRE, no las particiones: `analytics_events` y no `analytics_events_...`.
	// Si aquí apareciese una partición diaria, la fusión estaría funcionando por
	// casualidad y se rompería con la partición que se cree mañana.
	if got := res.Rows["analytics_events"]; got != 3 {
		t.Errorf("eventos movidos por la tabla madre = %d, quiero 3; detalle: %v", got, res.Rows)
	}
	for tabla := range res.Rows {
		if strings.HasPrefix(tabla, "analytics_events_") {
			t.Errorf("se movió la partición %q directamente: el motor debe ir por la tabla madre", tabla)
		}
	}

	// Nada del origen sobrevive, y lo del destino sigue intacto: el UPDATE no
	// arrastró filas ajenas.
	for _, tabla := range []string{"analytics_sites", "analytics_events", "analytics_daily"} {
		if n := cuenta(t, ctx, admin, tabla, origen); n != 0 {
			t.Errorf("%s conserva %d filas del origen tras la fusión", tabla, n)
		}
	}
	if n := cuenta(t, ctx, admin, "analytics_events", destino); n != 4 {
		t.Errorf("eventos del destino = %d, quiero 4 (1 suyo + 3 absorbidos)", n)
	}
	if n := cuenta(t, ctx, admin, "analytics_sites", destino); n != 2 {
		t.Errorf("sitios del destino = %d, quiero 2", n)
	}

	// El agregado del destino no se sobrescribió: son dos filas con site_id
	// distinto, que es lo que hace que esta base no tenga colisiones.
	var vistas []int
	rows, err := admin.Query(ctx,
		`SELECT pageviews FROM analytics_daily WHERE tenant = $1 ORDER BY pageviews`, destino)
	if err != nil {
		t.Fatalf("leer agregados del destino: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			t.Fatalf("escanear agregado: %v", err)
		}
		vistas = append(vistas, v)
	}
	if len(vistas) != 2 || vistas[0] != 10 || vistas[1] != 20 {
		t.Errorf("agregados del destino = %v, quiero [10 20]: la fusión no debe fundir ni perder filas", vistas)
	}
}

// TestMoveServiceEsIdempotente: la fusión se reintenta, y un segundo pase no
// puede duplicar ni perder nada.
//
// Es la propiedad que sostiene toda la reanudación: si el proceso muere después
// de mover analytics pero antes de anotarlo, al reanudar se vuelve a intentar. Lo
// que lo hace seguro es el criterio `WHERE tenant = origen`, que tras el primer
// pase no encuentra ya ninguna fila.
func TestMoveServiceEsIdempotente(t *testing.T) {
	h, admin, ctx := entorno(t)

	primera, err := MoveService(ctx, h.Pool, origen, destino)
	if err != nil {
		t.Fatalf("MoveService (1ª): %v", err)
	}
	segunda, err := MoveService(ctx, h.Pool, origen, destino)
	if err != nil {
		t.Fatalf("MoveService (2ª): %v", err)
	}

	if primera.Total == 0 {
		t.Fatal("la primera pasada no movió nada: el test no prueba idempotencia")
	}
	if segunda.Total != 0 {
		t.Errorf("la segunda pasada movió %d filas, quiero 0; detalle: %v",
			segunda.Total, segunda.Rows)
	}
	// Y el total sigue siendo el de siempre: ni duplicados ni pérdidas.
	if n := cuenta(t, ctx, admin, "analytics_events", destino); n != 4 {
		t.Errorf("eventos del destino tras dos pasadas = %d, quiero 4", n)
	}
}

// TestMoveServiceAbortaSiFaltaLaPoliticaDeFusion: sin política, se PARA en vez de
// mentir.
//
// Este es el test que justifica todo el preflight. Se retira la política de una
// tabla y se comprueba que el motor se niega a seguir: sin esa comprobación, el
// UPDATE sobre esa tabla afectaría a cero filas SIN ERROR —RLS forzada no ve las
// filas del origen— y la fusión terminaría en verde dejando los datos de esa
// tabla en la organización absorbida.
func TestMoveServiceAbortaSiFaltaLaPoliticaDeFusion(t *testing.T) {
	h, admin, ctx := entorno(t)

	if _, err := admin.Exec(ctx,
		`DROP POLICY analytics_daily_platform_merge ON analytics_daily`); err != nil {
		t.Fatalf("retirar la política: %v", err)
	}

	_, err := MoveService(ctx, h.Pool, origen, destino)
	if err == nil {
		t.Fatal("MoveService dio por buena una base sin política de fusión: " +
			"habría dejado analytics_daily en la organización absorbida sin decir nada")
	}
	if !strings.Contains(err.Error(), "analytics_daily") {
		t.Errorf("el error no nombra la tabla culpable, y sin eso no se puede arreglar: %v", err)
	}

	// Y no movió NADA: la transacción entera se deshace, no medio servicio.
	if n := cuenta(t, ctx, admin, "analytics_sites", origen); n != 1 {
		t.Errorf("el origen quedó a medias: %d sitios (quiero 1, intactos)", n)
	}
}

// TestPoliticaDeFusionNoAbreNadaFueraDeLaFusion: la política añadida para poder
// fusionar no puede convertirse en un agujero en el aislamiento diario.
//
// La política es PERMISSIVE, es decir, SUMA permisos a las demás. Si su condición
// estuviera mal escrita —o si alguien fijara `app.merge` en una transacción de
// negocio—, cualquier inquilino vería a los demás. Aquí se comprueba lo contrario
// desde fuera: una transacción normal con `app.tenant` del destino sigue sin ver
// ni una fila del origen.
func TestPoliticaDeFusionNoAbreNadaFueraDeLaFusion(t *testing.T) {
	h, _, ctx := entorno(t)

	var n int
	err := h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant', $1, true)`, destino); err != nil {
			return err
		}
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM analytics_sites WHERE tenant = $1`, origen).Scan(&n)
	})
	if err != nil {
		t.Fatalf("consulta de negocio: %v", err)
	}
	if n != 0 {
		t.Errorf("una transacción normal del destino ve %d sitios del origen: "+
			"la política de fusión está abriendo permisos fuera de la fusión", n)
	}
}

// TestLeftoversInVeLaFilaQueLLegoTARDE: el verificador encuentra lo que el
// movimiento no pudo ver.
//
// Este es el escenario que motiva la verificación y no lo cubre ningún test de
// MoveService: mientras se fusiona, el origen SIGUE VIVO. Sus usuarios escriben
// hasta que se mueve la identidad, y una fila insertada después de que su tabla
// pasara por el UPDATE se queda con el tenant viejo. MoveService no falla —hizo
// lo que le tocaba con lo que había— así que sin este recuento la fusión se
// declara completada con datos de un cliente detrás.
func TestLeftoversInVeLaFilaQueLlegoTarde(t *testing.T) {
	h, admin, ctx := entorno(t)

	if _, err := MoveService(ctx, h.Pool, origen, destino); err != nil {
		t.Fatalf("MoveService: %v", err)
	}
	// Recién movido no queda nada: es la línea base, y sin ella un verificador
	// que devolviera siempre vacío pasaría la segunda mitad del test.
	restos, err := LeftoversIn(ctx, h.Pool, "analytics", origen)
	if err != nil {
		t.Fatalf("LeftoversIn tras mover: %v", err)
	}
	if len(restos) != 0 {
		t.Fatalf("tras mover dice que quedan restos: %+v", restos)
	}

	// El rezagado: un sitio del origen que entra DESPUÉS del movimiento.
	if _, err := admin.Exec(ctx,
		`INSERT INTO analytics_sites (tenant, host) VALUES ($1, $2)`,
		origen, "llego-tarde.example"); err != nil {
		t.Fatalf("sembrar el rezagado: %v", err)
	}

	restos, err = LeftoversIn(ctx, h.Pool, "analytics", origen)
	if err != nil {
		t.Fatalf("LeftoversIn: %v", err)
	}
	if len(restos) != 1 {
		t.Fatalf("restos = %+v, quiero exactamente uno (analytics_sites)", restos)
	}
	if restos[0].Table != "analytics_sites" || restos[0].Rows != 1 || restos[0].Service != "analytics" {
		t.Errorf("resto = %+v, quiero {analytics analytics_sites 1}", restos[0])
	}
}

// Y lo cuenta a TRAVÉS de la RLS, que es lo único que hace útil al verificador.
//
// Bajo FORCE ROW LEVEL SECURITY una transacción sin `app.merge` no ve las filas
// del origen y el recuento sale cero. Un verificador así no es que no compruebe
// nada: es que CONFIRMA la fusión, convirtiendo «no lo he mirado» en «está
// limpio». Aquí se cuenta lo mismo con y sin la variable, y los números tienen
// que discrepar; si coincidieran, el pool estaría saltándose la RLS y ninguno de
// estos tests significaría nada.
func TestLeftoversInCuentaAtravesDeLaRLS(t *testing.T) {
	h, _, ctx := entorno(t)

	restos, err := LeftoversIn(ctx, h.Pool, "analytics", origen)
	if err != nil {
		t.Fatalf("LeftoversIn: %v", err)
	}
	total := 0
	for _, r := range restos {
		total += r.Rows
	}
	// Sin mover nada, el origen tiene sus 5 filas sembradas.
	if total != 5 {
		t.Fatalf("con app.merge se ven %d filas del origen, quiero 5: %+v", total, restos)
	}

	// La misma cuenta en una transacción normal, sin app.merge.
	var ciego int
	err = h.Pool.InTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM analytics_sites WHERE tenant = $1`, origen).Scan(&ciego)
	})
	if err != nil {
		t.Fatalf("contar sin app.merge: %v", err)
	}
	if ciego != 0 {
		t.Fatalf("una transacción SIN app.merge ve %d filas del origen: el pool de pruebas "+
			"se está saltando la RLS y estos tests no demuestran nada", ciego)
	}
}
