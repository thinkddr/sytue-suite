// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

package store

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// recorrer pagina de `limite` en `limite` hasta que no hay cursor y devuelve los ids.
func (e *entorno) recorrer(vista string, limite int, entreMedias func(pag PaginaRegistros)) []string {
	e.t.Helper()
	var ids []string
	cur := ""
	for i := 0; i < 1000; i++ {
		pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, vista, limite, cur)
		e.ok(err)
		if len(pag.Records) > limite {
			e.t.Fatalf("página de %d con límite %d", len(pag.Records), limite)
		}
		for _, r := range pag.Records {
			ids = append(ids, r.ID)
		}
		if entreMedias != nil {
			entreMedias(pag)
		}
		if cur = pag.NextCursor; cur == "" {
			return ids
		}
	}
	e.t.Fatal("la paginación no termina")
	return nil
}

// Paginar por cursor entrega lo MISMO y en el mismo orden que leerlo de una vez, sin
// repetir ni saltarse nada, con cualquier orden de vista: empates, vacíos (van al
// final), números, textos, casillas y direcciones mezcladas.
func TestPaginaPorCursor(t *testing.T) {
	e := nuevo(t)
	chk, err := e.s.CrearCampo(e.ctx, ana, e.tabl.ID, domain.EntradaCampo{Name: "Hecho", Kind: domain.TipoCasilla})
	e.ok(err)
	for i := 0; i < 37; i++ {
		d := map[string]json.RawMessage{}
		if i%5 != 0 {
			d[e.num.ID] = json.RawMessage(fmt.Sprint(i % 7)) // empates y huecos
		}
		if i%4 != 0 {
			d[e.txt.ID] = json.RawMessage(fmt.Sprintf(`"n%d"`, i%3))
		}
		if i%3 != 0 {
			d[chk.ID] = json.RawMessage(fmt.Sprint(i%2 == 0))
		}
		e.registro(ana, e.tabl.ID, d)
	}
	vistas := map[string]domain.ConfigVista{
		"sin orden":                   {},
		"número desc":                 {Sort: []domain.Orden{{Field: e.num.ID, Dir: "desc"}}},
		"texto, número desc":          {Sort: []domain.Orden{{Field: e.txt.ID, Dir: "asc"}, {Field: e.num.ID, Dir: "desc"}}},
		"casilla desc, texto, número": {Sort: []domain.Orden{{Field: chk.ID, Dir: "desc"}, {Field: e.txt.ID, Dir: "desc"}, {Field: e.num.ID, Dir: "asc"}}},
		"con filtro":                  {Filters: []domain.Filtro{{Field: e.num.ID, Op: "gte", Value: json.RawMessage(`2`)}}, Sort: []domain.Orden{{Field: e.num.ID, Dir: "asc"}}},
	}
	var cursorDeOtra string
	for nombre, cfg := range vistas {
		v, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, nombre, domain.VistaRejilla, cfg)
		e.ok(err)
		entera := e.recorrer(v.ID, 500, nil)
		for _, limite := range []int{1, 4, 36, 37} {
			if got := e.recorrer(v.ID, limite, nil); !slices.Equal(got, entera) {
				t.Errorf("%s, de %d en %d: %d registros, y no en el orden de la lectura entera (%d)", nombre, limite, limite, len(got), len(entera))
			}
		}
		if nombre == "número desc" {
			pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, v.ID, 5, "")
			e.ok(err)
			cursorDeOtra = pag.NextCursor
		}
	}
	if cursorDeOtra == "" {
		t.Fatal("la primera página de 5 no trae cursor")
	}
	// El cursor es de UN orden: con otro (aquí, sin vista) se rechaza, no se reinterpreta.
	_, err = e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, "", 5, cursorDeOtra)
	esperar(t, err, domain.ErrInvalido, "cursor de otro orden")
	// Y no es una credencial: quien no puede leer la tabla no lee con un cursor bueno.
	_, err = e.s.ListarRegistros(e.ctx, bea, e.tabl.ID, "", 5, cursorDeOtra)
	esperar(t, err, domain.ErrNoExiste, "cursor en manos de quien no tiene acceso")
	_, err = e.s.ListarRegistros(e.ctx, gus, e.tabl.ID, "", 5, cursorDeOtra)
	esperar(t, err, domain.ErrNoExiste, "cursor en otra organización")
}

// Estable aunque la tabla cambie entre dos páginas: borrar justo el registro en el que
// se quedó el cursor no rompe la lectura ni repite (un OFFSET sí se habría corrido).
func TestCursorEstableAnteCambios(t *testing.T) {
	e := nuevo(t)
	for i := 0; i < 10; i++ {
		e.registro(ana, e.tabl.ID, dato(e.num.ID, fmt.Sprint(i)))
	}
	todos := e.recorrer("", 500, nil)
	borrado := false
	got := e.recorrer("", 3, func(pag PaginaRegistros) {
		if !borrado {
			borrado = true
			e.ok(e.s.BorrarRegistro(e.ctx, ana, pag.Records[2].ID, false))
			e.ok(e.s.BorrarRegistro(e.ctx, ana, pag.Records[0].ID, false))
		}
	})
	if !slices.Equal(got, todos) {
		t.Errorf("tras borrar entre páginas: %v, esperado %v", got, todos)
	}
}

// El contador de registros lo lleva la base de datos por todas las vías, y el techo se
// comprueba contra él.
func TestContadorDeRegistrosYTecho(t *testing.T) {
	e := nuevo(t)
	cuenta := func(tabla string) (n int64) {
		e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
				return err
			}
			return tx.QueryRow(e.ctx, `SELECT record_count FROM base_tables WHERE id = $1`, tabla).Scan(&n)
		}))
		return n
	}
	a := e.registro(ana, e.tabl.ID, nil)
	e.registro(ana, e.tabl.ID, nil)
	e.registro(ana, e.tabl.ID, nil)
	e.ok(e.s.BorrarRegistro(e.ctx, ana, a.ID, false))
	if n := cuenta(e.tabl.ID); n != 2 {
		t.Errorf("contador tras 3 altas y 1 baja: %d", n)
	}
	csv, err := domain.LeerCSV(strings.NewReader("a,b\n1,2\n3,4\n5,6\n"))
	e.ok(err)
	imp, _, err := e.s.ImportarTabla(e.ctx, ana, e.base.ID, "Importada", csv)
	e.ok(err)
	if n := cuenta(imp.ID); n != 3 {
		t.Errorf("contador tras importar 3 filas: %d", n)
	}
	// En el techo, no entra ni uno más.
	e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
			return err
		}
		_, err := tx.Exec(e.ctx, `UPDATE base_tables SET record_count = $2 WHERE id = $1`, e.tabl.ID, domain.MaxRegistrosPorTabla)
		return err
	}))
	_, err = e.s.CrearRegistro(e.ctx, ana, e.tabl.ID, nil, nil)
	esperar(t, err, domain.ErrLimite, "alta con la tabla en el techo")
}

// TestMedirPaginacion mide, con una tabla de 200.000 registros, lo que cuesta la primera
// página y una profunda con el OFFSET de antes y con el cursor de ahora. Solo corre si
// se pide (BASES_MEDIR=1): no afirma tiempos, los imprime para la PR.
func TestMedirPaginacion(t *testing.T) {
	if os.Getenv("BASES_MEDIR") == "" {
		t.Skip("BASES_MEDIR=1 para medir")
	}
	const total = 200000
	e := nuevo(t)
	e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
			return err
		}
		_, err := tx.Exec(e.ctx, `
			INSERT INTO base_records (tenant, base_id, table_id, data, created_by, updated_by, created_at)
			SELECT 'acme', $1, $2, jsonb_build_object($3::text, 'cliente ' || g, $4::text, (g * 7919) % 100003), 'ana', 'ana', now() + g * interval '1 millisecond'
			  FROM generate_series(1, $5::int) g`, e.base.ID, e.tabl.ID, e.txt.ID, e.num.ID, total)
		return err
	}))
	// Las estadísticas, como las tendría producción (autovacuum): las recoge el dueño de
	// la tabla, no el rol de la aplicación, al que ANALYZE ignora con un aviso.
	dueno, err := pgx.Connect(e.ctx, e.h.DSN)
	e.ok(err)
	_, err = dueno.Exec(e.ctx, `ANALYZE base_records`)
	e.ok(err)
	e.ok(dueno.Close(e.ctx))
	vista, err := e.s.CrearVista(e.ctx, ana, e.tabl.ID, "Por importe", domain.VistaRejilla, domain.ConfigVista{Sort: []domain.Orden{{Field: e.num.ID, Dir: "desc"}}})
	e.ok(err)
	campos := []domain.Campo{e.txt, e.num}
	medir := func(nombre string, fn func()) {
		fn() // en caliente
		var mejor time.Duration
		for i := 0; i < 5; i++ {
			t0 := time.Now()
			fn()
			if d := time.Since(t0); mejor == 0 || d < mejor {
				mejor = d
			}
		}
		t.Logf("MEDIDA %-46s %8.1f ms", nombre, float64(mejor.Microseconds())/1000)
	}
	for _, caso := range []struct {
		nombre string
		vista  string
		cfg    domain.ConfigVista
	}{{"orden por defecto", "", domain.ConfigVista{}}, {"vista ordenada por número desc", vista.ID, vista.Config}} {
		// ANTES: la consulta con OFFSET, tal como era.
		conOffset := func(desp int) (ultimo domain.Registro) {
			e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
				if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
					return err
				}
				q := construirConsulta(campos, caso.cfg)
				args := append([]any{"acme", e.tabl.ID}, q.args...)
				args = append(args, 100, desp)
				rows, err := tx.Query(e.ctx, fmt.Sprintf(`SELECT r.id::text, r.data, r.created_at FROM base_records r
					WHERE r.tenant = $1 AND r.table_id = $2%s ORDER BY %s LIMIT $%d OFFSET $%d`, q.where, q.order, len(args)-1, len(args)), args...)
				if err != nil {
					return err
				}
				defer rows.Close()
				for rows.Next() {
					var data []byte
					if err := rows.Scan(&ultimo.ID, &data, &ultimo.CreatedAt); err != nil {
						return err
					}
					_ = json.Unmarshal(data, &ultimo.Data)
				}
				return rows.Err()
			}))
			return ultimo
		}
		// El plan de la primera página, para la PR: qué índice la sostiene.
		e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
				return err
			}
			q := construirConsulta(campos, caso.cfg)
			args := append([]any{"acme", e.tabl.ID}, q.args...)
			rows, err := tx.Query(e.ctx, fmt.Sprintf(`EXPLAIN (ANALYZE, BUFFERS OFF, TIMING OFF) SELECT r.id::text, r.data FROM base_records r
				WHERE r.tenant = $1 AND r.table_id = $2%s ORDER BY %s LIMIT 101`, q.where, q.order), args...)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var l string
				if err := rows.Scan(&l); err != nil {
					return err
				}
				t.Logf("PLAN %s: %s", caso.nombre, l)
			}
			return rows.Err()
		}))
		q := construirConsulta(campos, caso.cfg)
		profundo := q.cursorDe(conOffset(total - 200)) // el cursor que deja en la penúltima página
		medir("ANTES  "+caso.nombre+", primera página", func() { conOffset(0) })
		medir("ANTES  "+caso.nombre+", página profunda", func() { conOffset(total - 100) })
		medir("AHORA  "+caso.nombre+", primera página", func() {
			_, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, caso.vista, 100, "")
			e.ok(err)
		})
		medir("AHORA  "+caso.nombre+", página profunda", func() {
			pag, err := e.s.ListarRegistros(e.ctx, ana, e.tabl.ID, caso.vista, 100, profundo)
			e.ok(err)
			if len(pag.Records) != 100 || pag.NextCursor != "" {
				t.Fatalf("la página profunda no es la última: %d, cursor %q", len(pag.Records), pag.NextCursor)
			}
		})
	}
	medir("ANTES  alta: count(*) de la tabla", func() {
		e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
				return err
			}
			var n int
			return tx.QueryRow(e.ctx, `SELECT count(*) FROM (SELECT 1 FROM base_records WHERE tenant = $1 AND table_id = $2 LIMIT $3) x`, "acme", e.tabl.ID, domain.MaxRegistrosPorTabla).Scan(&n)
		}))
	})
	medir("AHORA  alta: contador", func() {
		e.ok(e.h.Pool.InTx(e.ctx, func(tx pgx.Tx) error {
			if err := pg.SetTenant(e.ctx, tx, "acme"); err != nil {
				return err
			}
			return hayHueco(e.ctx, tx, "acme", e.tabl.ID)
		}))
	})
}
