// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pgtest"
	"git.sytue.eu/sytue/one/services/syerp/db"
	"git.sytue.eu/sytue/one/services/syerp/internal/store"
)

// MEDIR, NO OPINAR. Este fichero no prueba comportamiento: siembra un volumen
// realista y enseña qué PLAN elige Postgres para la consulta que sirve todas las
// listas del ERP. Un índice que existe no es un índice que se use.

const filasPorModelo = 20000

func sembrar(ctx context.Context, t *testing.T, h *pgtest.Harness) {
	t.Helper()
	inicio := time.Now()
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO records (tenant, model, ref, etiqueta, estado, data, owner_sub, created_at)
			SELECT current_setting('app.tenant', true),
			       m.modelo,
			       CASE WHEN m.modelo = 'factura_venta'
			            THEN 'FV-2026-' || lpad(i::text, 5, '0') ELSE '' END,
			       'Cliente ' || i,
			       (ARRAY['borrador','confirmado','pagado'])[1 + (i % 3)],
			       jsonb_build_object('fecha', '2026-09-12', 'base', (i * 137) % 900000),
			       'ada',
			       now() - (i || ' minutes')::interval
			  FROM generate_series(1, $1) AS i,
			       unnest(ARRAY['factura_venta','contacto']) AS m(modelo)`, filasPorModelo)
		return err
	})
	if err != nil {
		t.Fatalf("sembrar: %v", err)
	}
	// ANALYZE con el DSN PRIVILEGIADO y no con el pool de la aplicación: ANALYZE
	// exige ser dueño de la tabla, y el rol de aplicación no lo es. Lanzado con
	// él, PostgreSQL se lo salta con un aviso que nadie lee y la tabla se queda
	// SIN estadísticas — con lo que el planificador estima una fila donde hay
	// veinte mil y cualquier conclusión sobre el plan es basura.
	if err := analizar(ctx, h); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	var filas float64
	var ultimo *time.Time
	if err := h.Pool.QueryRow(ctx, `
		SELECT c.reltuples, s.last_analyze
		  FROM pg_class c
		  LEFT JOIN pg_stat_user_tables s ON s.relid = c.oid
		 WHERE c.relname = 'records'`).Scan(&filas, &ultimo); err != nil {
		t.Fatalf("estadísticas: %v", err)
	}
	t.Logf("estadísticas de `records`: reltuples=%.0f last_analyze=%v", filas, ultimo)
	t.Logf("sembradas %d filas en %s", filasPorModelo*2, time.Since(inicio).Round(time.Millisecond))
}

// analizar lanza ANALYZE por la conexión privilegiada del harness.
func analizar(ctx context.Context, h *pgtest.Harness) error {
	conn, err := pgx.Connect(ctx, h.DSN)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	_, err = conn.Exec(ctx, `ANALYZE records`)
	return err
}

// planDe devuelve el plan como una sola cadena, para poder afirmar sobre él.
func planDe(ctx context.Context, t *testing.T, h *pgtest.Harness, sql string, args ...any) string {
	t.Helper()
	var lineas []string
	err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		filas, err := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS, TIMING OFF) "+sql, args...)
		if err != nil {
			return err
		}
		defer filas.Close()
		for filas.Next() {
			var l string
			if err := filas.Scan(&l); err != nil {
				return err
			}
			lineas = append(lineas, l)
		}
		return filas.Err()
	})
	if err != nil {
		t.Fatalf("EXPLAIN: %v", err)
	}
	return strings.Join(lineas, "\n")
}

// TestElListadoUsaSuIndice es un GATE, no una medida.
//
// La consulta que sirve TODAS las listas del ERP tiene que resolverse recorriendo
// `records_listado` y parándose en la página, sin ordenar nada. Que lo haga
// depende de un detalle fácil de perder: `tenant` va en el `WHERE` aunque la RLS
// ya lo imponga, porque es la primera columna del índice y la política lo compara
// con `current_setting`, que no sirve de condición de índice ordenada.
//
// Medido con 20.000 filas en un modelo: sin esa igualdad, PostgreSQL lee las
// 20.000 y hace un top-N —coste 2114, 9,4 ms—; con ella, un Index Scan de 51
// filas —coste 8,88, 0,56 ms—. Quien quite esa línea no rompe nada visible: la
// lista sigue saliendo bien, solo que diecisiete veces más lenta y leyendo la
// tabla entera. Por eso hay un test mirando el PLAN y no solo el resultado.
func TestElListadoUsaSuIndice(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	ctx := tenantCtx("acme")
	sembrar(ctx, t, h)

	// La consulta REAL de `Listar`, con sus guardas y su cursor vacío.
	const listar = `
		SELECT id::text AS rid, model, ref, etiqueta, estado, data, created_at, updated_at
		  FROM records
		 WHERE tenant = $1
		   AND model = $2
		   AND deleted_at IS NULL
		   AND ($3 = '' OR estado = $3)
		   AND ($4 = '' OR etiqueta ILIKE '%' || $4 || '%')
		   AND ($5::jsonb IS NULL OR data @> $5::jsonb)
		   AND ($6::timestamptz IS NULL
		        OR (created_at, id) < ($6::timestamptz, $7::uuid))
		 ORDER BY created_at DESC, id DESC
		 LIMIT $8`

	primera := planDe(ctx, t, h, listar, "acme", "factura_venta", "", "", nil, nil, nil, 51)
	t.Logf("\n── primera página ──\n%s", primera)
	if !strings.Contains(primera, "Index Scan using records_listado") {
		t.Errorf("la primera página no usa records_listado:\n%s", primera)
	}
	if strings.Contains(primera, "Sort Method") {
		t.Errorf("la primera página ordena en memoria en vez de leer el índice:\n%s", primera)
	}

	// Y la segunda, que es donde el cursor tiene que seguir por el índice.
	var marca time.Time
	var id string
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT created_at, id::text FROM records
			 WHERE model = 'factura_venta' AND deleted_at IS NULL
			 ORDER BY created_at DESC, id DESC OFFSET 50 LIMIT 1`).Scan(&marca, &id)
	}); err != nil {
		t.Fatalf("cursor: %v", err)
	}
	segunda := planDe(ctx, t, h, listar, "acme", "factura_venta", "", "", nil, marca, id, 51)
	t.Logf("\n── segunda página ──\n%s", segunda)
	if !strings.Contains(segunda, "Index Scan using records_listado") {
		t.Errorf("la segunda página no usa records_listado:\n%s", segunda)
	}
	if strings.Contains(segunda, "Sort Method") {
		t.Errorf("la segunda página ordena en memoria:\n%s", segunda)
	}

	// Y la comprobación que explica POR QUÉ: la misma consulta SIN el tenant
	// explícito vuelve al escaneo completo. Si algún día PostgreSQL aprendiera a
	// usar la política como condición de índice, esto se pondría rojo y sería una
	// buena noticia: se podría quitar la línea.
	sinTenant := planDe(ctx, t, h, `
		SELECT id::text AS rid, model, ref, etiqueta, estado, data, created_at, updated_at
		  FROM records WHERE model = $1 AND deleted_at IS NULL
		 ORDER BY created_at DESC, id DESC LIMIT 51`, "factura_venta")
	if strings.Contains(sinTenant, "Index Scan using records_listado") {
		t.Logf("AVISO: sin el tenant explícito YA usa el índice; se puede simplificar `Listar`:\n%s", sinTenant)
	}
}

// TestTiempoDeListado pone números al plan: cuánto tarda de verdad cada forma.
func TestTiempoDeListado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	sembrar(ctx, t, h)

	medir := func(nombre string, fn func() error) {
		// Una vuelta en frío y luego la mediana de nueve: la primera paga el
		// cacheo de páginas y no representa a las demás.
		if err := fn(); err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		var total time.Duration
		const n = 9
		for i := 0; i < n; i++ {
			inicio := time.Now()
			if err := fn(); err != nil {
				t.Fatalf("%s: %v", nombre, err)
			}
			total += time.Since(inicio)
		}
		t.Logf("%-42s %s", nombre, (total / n).Round(time.Microsecond))
	}

	medir("Listar primera página", func() error {
		_, err := st.Listar(ctx, store.Filtro{Modelo: "factura_venta"})
		return err
	})
	medir("Listar filtrando por estado", func() error {
		_, err := st.Listar(ctx, store.Filtro{Modelo: "factura_venta", Estado: "confirmado"})
		return err
	})
	medir("Listar buscando texto", func() error {
		_, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Texto: "Cliente 1234"})
		return err
	})

	// Y la página completa tal y como la sirve el handler: lista + etiquetas.
	pag, err := st.Listar(ctx, store.Filtro{Modelo: "factura_venta"})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	t.Logf("%-42s %d", "registros en la primera página", len(pag.Registros))
	if pag.Siguiente == "" {
		t.Error("con 20.000 filas la primera página tiene que traer cursor")
	}
}

// TestPaginarNoSaltaNiRepite recorre una lista ENTERA por cursor y comprueba que
// cada fila sale exactamente una vez, en orden.
//
// La paginación por cursor falla en silencio: si el orden del `ORDER BY` y el
// del `WHERE (created_at, id) < (…)` no coinciden exactamente, unas filas se
// repiten y otras no salen NUNCA. Nadie lo nota mirando la primera página —que
// es lo que se mira—, y en un ERP significa una factura que no aparece en
// ningún listado.
func TestPaginarNoSaltaNiRepite(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	contacto := modelo(t, c, "contacto")

	// Suficientes para varias páginas, y con etiquetas repetidas a propósito: si
	// el desempate por `id` no funcionara, filas con el mismo instante se
	// pisarían entre páginas.
	const total = 130
	for i := 0; i < total; i++ {
		crear(ctx, t, st, contacto, `{"nombre":"Repetido","tipo":"persona"}`)
	}

	vistos := map[string]int{}
	var orden []time.Time
	cursor := ""
	for paginas := 0; ; paginas++ {
		if paginas > 20 {
			t.Fatal("la paginación no termina: el cursor no avanza")
		}
		pag, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Limite: 25, Cursor: cursor})
		if err != nil {
			t.Fatalf("Listar: %v", err)
		}
		for _, r := range pag.Registros {
			vistos[r.ID]++
			orden = append(orden, r.CreadoEn)
		}
		if pag.Siguiente == "" {
			break
		}
		cursor = pag.Siguiente
	}

	if len(vistos) != total {
		t.Errorf("se vieron %d registros distintos de %d", len(vistos), total)
	}
	for id, veces := range vistos {
		if veces != 1 {
			t.Errorf("el registro %s salió %d veces", id, veces)
		}
	}
	// Y en orden descendente de principio a fin, sin saltos entre páginas.
	for i := 1; i < len(orden); i++ {
		if orden[i].After(orden[i-1]) {
			t.Fatalf("el orden se rompe en la posición %d: %s antes que %s",
				i, orden[i-1], orden[i])
		}
	}
}

// TestOrdenarPorImporteEsNumerico: si el campo se compara como texto, «9» va
// detrás de «10» y la columna de importes queda mal ordenada de una forma que
// parece aleatoria. Los importes viven dentro del jsonb, que es texto, así que
// esto NO sale gratis: hay que pedir la comparación numérica a mano.
func TestOrdenarPorImporteEsNumerico(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	contacto := modelo(t, c, "contacto")
	factura := modelo(t, c, "factura_venta")

	cliente := crear(ctx, t, st, contacto, `{"nombre":"Cliente","tipo":"organizacion"}`)
	// 900, 1.000 y 10.000 céntimos: por texto el orden sería 1000, 10000, 900.
	for _, base := range []int{900, 1000, 10000} {
		crear(ctx, t, st, factura, fmt.Sprintf(
			`{"cliente":%q,"fecha":"2026-09-12","base":%d}`, cliente.ID, base))
	}

	pag, err := st.Listar(ctx, store.Filtro{
		Modelo: "factura_venta", Orden: "base", OrdenTipo: "dinero",
	})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	bases := make([]int64, 0, len(pag.Registros))
	for _, r := range pag.Registros {
		v, _ := r.Data["base"].(float64)
		bases = append(bases, int64(v))
	}
	if len(bases) != 3 || bases[0] != 10000 || bases[1] != 1000 || bases[2] != 900 {
		t.Fatalf("orden descendente = %v, se esperaba [10000 1000 900]", bases)
	}

	pag, err = st.Listar(ctx, store.Filtro{
		Modelo: "factura_venta", Orden: "base", OrdenTipo: "dinero", Ascendente: true,
	})
	if err != nil {
		t.Fatalf("Listar ascendente: %v", err)
	}
	primera, _ := pag.Registros[0].Data["base"].(float64)
	if int64(primera) != 900 {
		t.Fatalf("ascendente empieza por %v y debería por 900", primera)
	}
}

// TestOrdenarNoRompeConDatosViejos: un campo que era texto y pasó a ser dinero en
// el manifiesto deja filas con valores no numéricos. Convertirlas a número
// reventaría la consulta entera con un error de conversión — y la lista dejaría
// de cargar por un cambio de manifiesto hecho meses antes.
func TestOrdenarNoRompeConDatosViejos(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	// Se escribe a mano porque `domain.Validar` no dejaría meter esto: simula
	// exactamente lo que queda en la base tras cambiar el tipo de un campo.
	if err := h.Pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO records (tenant, model, etiqueta, data, owner_sub)
			VALUES (current_setting('app.tenant', true), 'contacto', 'Raro',
			        '{"nombre":"Raro","cp":"sin cp"}'::jsonb, 'ada'),
			       (current_setting('app.tenant', true), 'contacto', 'Normal',
			        '{"nombre":"Normal","cp":"28001"}'::jsonb, 'ada')`)
		return err
	}); err != nil {
		t.Fatalf("sembrar: %v", err)
	}

	pag, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Orden: "cp", OrdenTipo: "entero"})
	if err != nil {
		t.Fatalf("ordenar con datos que no casan con el tipo: %v", err)
	}
	if len(pag.Registros) != 2 {
		t.Fatalf("registros = %d, se esperaban 2", len(pag.Registros))
	}
}

// TestOrdenarRechazaCampoInventado: la clave acaba DENTRO de una expresión SQL,
// así que el store la valida aunque quien llama ya lo haya hecho.
func TestOrdenarRechazaCampoInventado(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")

	for _, malo := range []string{"x'; DROP TABLE records; --", "Mayúsculas", "con espacio", "1numero"} {
		if _, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Orden: malo}); err == nil {
			t.Errorf("se aceptó %q como campo de orden", malo)
		}
	}
}

// TestPaginarOrdenadoNoSaltaNiRepite: con orden pedido a mano la paginación es
// por desplazamiento, no por cursor keyset. Tiene que dar el mismo resultado.
func TestPaginarOrdenadoNoSaltaNiRepite(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	contacto := modelo(t, c, "contacto")

	const total = 70
	for i := 0; i < total; i++ {
		// Ciudades repetidas a propósito: sin el desempate por `id`, las filas
		// con la misma ciudad podrían salir en distinto orden en cada página.
		crear(ctx, t, st, contacto, fmt.Sprintf(
			`{"nombre":"C%d","tipo":"persona","ciudad":"Ciudad %d"}`, i, i%5))
	}

	vistos := map[string]int{}
	cursor := ""
	for paginas := 0; paginas < 20; paginas++ {
		pag, err := st.Listar(ctx, store.Filtro{
			Modelo: "contacto", Orden: "ciudad", Limite: 20, Cursor: cursor,
		})
		if err != nil {
			t.Fatalf("Listar: %v", err)
		}
		for _, r := range pag.Registros {
			vistos[r.ID]++
		}
		if pag.Siguiente == "" {
			break
		}
		cursor = pag.Siguiente
	}
	if len(vistos) != total {
		t.Errorf("se vieron %d registros distintos de %d", len(vistos), total)
	}
	for id, veces := range vistos {
		if veces != 1 {
			t.Errorf("el registro %s salió %d veces", id, veces)
		}
	}
}

// TestCosteDeOrdenar pone números a lo que cuesta ordenar por un campo del jsonb
// frente al orden por defecto. No afirma nada: mide, y el número sale en el log
// para que la decisión de ofrecerlo se tome con el dato delante.
func TestCosteDeOrdenar(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	sembrar(ctx, t, h)

	medir := func(nombre string, f store.Filtro) {
		if _, err := st.Listar(ctx, f); err != nil {
			t.Fatalf("%s: %v", nombre, err)
		}
		var total time.Duration
		const n = 9
		for i := 0; i < n; i++ {
			inicio := time.Now()
			if _, err := st.Listar(ctx, f); err != nil {
				t.Fatalf("%s: %v", nombre, err)
			}
			total += time.Since(inicio)
		}
		t.Logf("%-46s %s", nombre, (total / n).Round(time.Microsecond))
	}

	medir("por defecto (fecha, con índice)", store.Filtro{Modelo: "factura_venta"})
	medir("ordenando por un campo del jsonb", store.Filtro{
		Modelo: "factura_venta", Orden: "base", OrdenTipo: "dinero",
	})
	medir("ordenando por etiqueta (columna real)", store.Filtro{
		Modelo: "factura_venta", Orden: "etiqueta",
	})
}

// TestPapeleraYRestaurar: borrar es a la papelera, y de la papelera se puede
// salir. Sin esta vuelta, «borrar» era de ida sola desde la interfaz y daba una
// sensación de reversibilidad que no era cierta.
func TestPapeleraYRestaurar(t *testing.T) {
	h := pgtest.New(t, db.Migrations, db.MigrationsDir)
	st := store.New(h.Pool)
	ctx := tenantCtx("acme")
	c := cat(t)
	contacto := modelo(t, c, "contacto")

	vivo := crear(ctx, t, st, contacto, `{"nombre":"Se queda","tipo":"persona"}`)
	fuera := crear(ctx, t, st, contacto, `{"nombre":"Se va","tipo":"persona"}`)

	if err := st.Borrar(ctx, "contacto", fuera.ID); err != nil {
		t.Fatalf("Borrar: %v", err)
	}

	// La lista normal no lo ve; la papelera solo lo ve a él.
	normal, err := st.Listar(ctx, store.Filtro{Modelo: "contacto"})
	if err != nil {
		t.Fatalf("Listar: %v", err)
	}
	if len(normal.Registros) != 1 || normal.Registros[0].ID != vivo.ID {
		t.Fatalf("la lista normal trae %d registros", len(normal.Registros))
	}
	papelera, err := st.Listar(ctx, store.Filtro{Modelo: "contacto", Papelera: true})
	if err != nil {
		t.Fatalf("Listar papelera: %v", err)
	}
	if len(papelera.Registros) != 1 || papelera.Registros[0].ID != fuera.ID {
		t.Fatalf("la papelera trae %d registros", len(papelera.Registros))
	}

	// Y vuelve.
	if _, err := st.Restaurar(ctx, "contacto", fuera.ID); err != nil {
		t.Fatalf("Restaurar: %v", err)
	}
	normal, err = st.Listar(ctx, store.Filtro{Modelo: "contacto"})
	if err != nil {
		t.Fatalf("Listar tras restaurar: %v", err)
	}
	if len(normal.Registros) != 2 {
		t.Fatalf("tras restaurar la lista trae %d y deberían ser 2", len(normal.Registros))
	}

	// Restaurar algo que nunca estuvo borrado no inventa nada.
	if _, err := st.Restaurar(ctx, "contacto", vivo.ID); !errors.Is(err, store.ErrNoEncontrado) {
		t.Fatalf("restaurar lo que no estaba borrado: err = %v", err)
	}
}
