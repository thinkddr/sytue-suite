// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// Recálculo de los campos de fórmula (domain/formula.go). El valor de una fórmula se
// GUARDA en el registro, así que hay que rehacerlo cuando cambia aquello de lo que
// depende:
//
//   - los demás campos y los enlaces del propio registro: en la misma escritura;
//   - un campo de un registro ENLAZADO (los agregados `{enlace.campo}`): se recalculan
//     los registros que enlazan con él, y los que dependan de esos, en la misma
//     transacción mientras quepan en `presupuestoRecalculo`;
//   - cuando no caben —o al crear o cambiar una fórmula en una tabla grande—, se anota
//     la TABLA en `base_recalc` y `RecalcularPendientes` la recorre por tramos.
//
// Un recálculo no sube `rev` ni se audita: no es una edición de nadie, y subir la versión
// haría fallar por conflicto a quien está editando otro campo del mismo registro.

// presupuestoRecalculo son los registros que una petición recalcula dentro de su
// transacción. Variable para que el test lo baje.
var presupuestoRecalculo = 500

const tramoRecalculo = 500

type recalculo struct {
	tx      pgx.Tx
	a       domain.Actor
	base    string
	esquema map[string][]domain.Campo // tabla → campos, de toda la base
	quedan  int
	// parches son los valores de fórmula recién calculados, por registro.
	parches map[string]map[string]json.RawMessage
}

// nuevoRecalculo prepara el recálculo de una base, o nil si la base no tiene ninguna
// fórmula (lo normal: una consulta por índice y nada más).
func nuevoRecalculo(ctx context.Context, tx pgx.Tx, a domain.Actor, base string) (*recalculo, error) {
	var hay bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM base_fields WHERE tenant = $1 AND base_id = $2 AND kind = 'formula')`, a.Tenant, base).Scan(&hay); err != nil || !hay {
		return nil, err
	}
	esquema, err := esquemaDeBase(ctx, tx, a, base)
	if err != nil {
		return nil, err
	}
	return &recalculo{tx: tx, a: a, base: base, esquema: esquema, quedan: presupuestoRecalculo, parches: map[string]map[string]json.RawMessage{}}, nil
}

// esquemaDeBase lee los campos de todas las tablas de una base, por tabla y en su orden.
func esquemaDeBase(ctx context.Context, tx pgx.Tx, a domain.Actor, base string) (map[string][]domain.Campo, error) {
	rows, err := tx.Query(ctx, `
		SELECT id::text, table_id::text, name, kind, options, coalesce(link_table_id::text, ''), position
		  FROM base_fields WHERE tenant = $1 AND base_id = $2 ORDER BY table_id, position, created_at, id LIMIT $3`,
		a.Tenant, base, domain.MaxTablasPorBase*domain.MaxCamposPorTabla)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]domain.Campo{}
	for rows.Next() {
		c, err := leerCampo(rows)
		if err != nil {
			return nil, err
		}
		out[c.TableID] = append(out[c.TableID], c)
	}
	return out, rows.Err()
}

// dependientesDe son los registros que enlazan con `registro`, por tabla (como mucho
// presupuestoRecalculo+1: con más, la lista no sirve y se encola la tabla).
func dependientesDe(ctx context.Context, tx pgx.Tx, a domain.Actor, registro string) (map[string][]string, error) {
	out := map[string][]string{}
	rows, err := tx.Query(ctx, `SELECT DISTINCT from_table_id::text, from_record::text FROM base_links WHERE tenant = $1 AND to_record = $2 LIMIT $3`,
		a.Tenant, registro, presupuestoRecalculo+1)
	if err != nil {
		return nil, err
	}
	var t, id string
	_, err = pgx.ForEachRow(rows, []any{&t, &id}, func() error {
		out[t] = append(out[t], id)
		return nil
	})
	return out, err
}

// recalcularDependientes rehace las fórmulas de los registros que enlazaban con uno que
// se acaba de borrar. `cortada` = eran más de los que se listaron: la tabla entera.
func recalcularDependientes(ctx context.Context, tx pgx.Tx, a domain.Actor, base string, dependientes map[string][]string, cortada bool) error {
	if len(dependientes) == 0 {
		return nil
	}
	r, err := nuevoRecalculo(ctx, tx, a, base)
	if r == nil || err != nil {
		return err
	}
	for t, ids := range dependientes {
		if cortada {
			err = r.encolar(ctx, t)
		} else {
			err = r.recalcular(ctx, t, ids, 0)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// trasEscribir recalcula lo que depende de los registros `ids` de `tabla`, recién
// escritos, y devuelve los valores de fórmula nuevos de esos registros.
func trasEscribir(ctx context.Context, tx pgx.Tx, a domain.Actor, base, tabla string, ids []string) (map[string]map[string]json.RawMessage, error) {
	r, err := nuevoRecalculo(ctx, tx, a, base)
	if r == nil || err != nil {
		return nil, err
	}
	return r.parches, r.recalcular(ctx, tabla, ids, 0)
}

// recalcular rehace las fórmulas de los registros `ids` de `tabla` y sigue con los
// registros de otras tablas que agregan sobre ellos. El esquema no tiene ciclos (se
// rechazan al guardar la fórmula), así que termina; `nivel` es solo un cinturón.
func (r *recalculo) recalcular(ctx context.Context, tabla string, ids []string, nivel int) error {
	if len(ids) == 0 {
		return nil
	}
	if len(ids) > r.quedan || nivel > domain.MaxTablasPorBase {
		return r.encolar(ctx, tabla)
	}
	r.quedan -= len(ids)
	if err := r.calcular(ctx, tabla, ids); err != nil {
		return err
	}
	// Quién agrega sobre esta tabla: fórmulas de otra tabla S con `{enlace.campo}` donde
	// el enlace (de S) apunta aquí.
	for s, campos := range r.esquema {
		var enlaces []string
		for _, ref := range refsDeAgregado(campos) {
			if i := slices.IndexFunc(campos, func(c domain.Campo) bool { return c.ID == ref.Enlace }); i >= 0 && campos[i].LinkTableID == tabla {
				enlaces = append(enlaces, ref.Enlace)
			}
		}
		if len(enlaces) == 0 {
			continue
		}
		deps, err := textos(ctx, r.tx, `
			SELECT DISTINCT from_record::text FROM base_links
			 WHERE tenant = $1 AND field_id = ANY($2::uuid[]) AND to_record = ANY($3::uuid[]) LIMIT $4`,
			r.a.Tenant, enlaces, ids, r.quedan+1)
		if err != nil {
			return err
		}
		if err := r.recalcular(ctx, s, deps, nivel+1); err != nil {
			return err
		}
	}
	return nil
}

// refsDeAgregado son las referencias `{enlace.campo}` de las fórmulas de una tabla.
func refsDeAgregado(campos []domain.Campo) []domain.RefFormula {
	var out []domain.RefFormula
	for _, c := range campos {
		if c.Kind != domain.TipoFormula {
			continue
		}
		f, err := domain.AnalizarFormula(c.Formula)
		if err != nil {
			continue
		}
		for _, ref := range f.Refs {
			if ref.Enlace != "" && !slices.Contains(out, ref) {
				out = append(out, ref)
			}
		}
	}
	return out
}

// calcular evalúa las fórmulas de los registros `ids` de `tabla` y guarda las que cambian.
func (r *recalculo) calcular(ctx context.Context, tabla string, ids []string) error {
	campos := r.esquema[tabla]
	if !slices.ContainsFunc(campos, func(c domain.Campo) bool { return c.Kind == domain.TipoFormula }) {
		return nil
	}
	cuenta, err := r.cuentaDeEnlaces(ctx, ids)
	if err != nil {
		return err
	}
	agregados, err := r.valoresEnlazados(ctx, campos, ids)
	if err != nil {
		return err
	}
	type fila struct {
		id   string
		data map[string]json.RawMessage
	}
	rows, err := r.tx.Query(ctx, `SELECT id::text, data FROM base_records WHERE tenant = $1 AND table_id = $2 AND id = ANY($3::uuid[]) FOR NO KEY UPDATE`, r.a.Tenant, tabla, ids)
	if err != nil {
		return err
	}
	filas, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (f fila, err error) {
		var raw []byte
		if err = row.Scan(&f.id, &raw); err == nil {
			_ = json.Unmarshal(raw, &f.data)
		}
		return f, err
	})
	if err != nil {
		return err
	}
	for _, f := range filas {
		nuevos := domain.EvaluarFormulas(campos, f.data, cuenta[f.id], agregados[f.id])
		r.parches[f.id] = nuevos
		if err := r.guardar(ctx, f.id, f.data, nuevos); err != nil {
			return err
		}
	}
	return nil
}

// cuentaDeEnlaces da, por registro y campo de enlace, cuántos registros enlaza.
func (r *recalculo) cuentaDeEnlaces(ctx context.Context, ids []string) (map[string]map[string]int, error) {
	out := map[string]map[string]int{}
	rows, err := r.tx.Query(ctx, `
		SELECT from_record::text, field_id::text, count(*) FROM base_links
		 WHERE tenant = $1 AND from_record = ANY($2::uuid[]) GROUP BY 1, 2`, r.a.Tenant, ids)
	if err != nil {
		return nil, err
	}
	var reg, campo string
	var n int
	_, err = pgx.ForEachRow(rows, []any{&reg, &campo, &n}, func() error {
		if out[reg] == nil {
			out[reg] = map[string]int{}
		}
		out[reg][campo] = n
		return nil
	})
	return out, err
}

// valoresEnlazados da, por registro, los valores que pide cada agregado `{enlace.campo}`.
// Solo el campo que se agrega, no el registro enlazado entero: 500 registros con 100
// enlaces de 60 KB no caben en memoria.
func (r *recalculo) valoresEnlazados(ctx context.Context, campos []domain.Campo, ids []string) (map[string]map[domain.RefFormula][]json.RawMessage, error) {
	out := map[string]map[domain.RefFormula][]json.RawMessage{}
	for _, ref := range refsDeAgregado(campos) {
		rows, err := r.tx.Query(ctx, `
			SELECT l.from_record::text, d.data -> $4::text
			  FROM base_links l JOIN base_records d ON d.tenant = l.tenant AND d.id = l.to_record
			 WHERE l.tenant = $1 AND l.field_id = $2 AND l.from_record = ANY($3::uuid[])
			 ORDER BY l.from_record, l.to_record LIMIT $5`,
			r.a.Tenant, ref.Enlace, ids, ref.Campo, len(ids)*domain.MaxEnlaces)
		if err != nil {
			return nil, err
		}
		var reg string
		var v []byte
		if _, err := pgx.ForEachRow(rows, []any{&reg, &v}, func() error {
			if out[reg] == nil {
				out[reg] = map[domain.RefFormula][]json.RawMessage{}
			}
			out[reg][ref] = append(out[reg][ref], slices.Clone(v))
			return nil
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// guardar escribe en el registro los valores de fórmula que han cambiado.
func (r *recalculo) guardar(ctx context.Context, id string, data, nuevos map[string]json.RawMessage) error {
	poner, quitar := map[string]json.RawMessage{}, []string{}
	for campo, v := range nuevos {
		switch {
		case v == nil && data[campo] != nil:
			quitar = append(quitar, campo)
		case v != nil && !bytes.Equal(canonico(data[campo]), v):
			poner[campo] = v
		}
	}
	if len(poner) == 0 && len(quitar) == 0 {
		return nil
	}
	// ponytail: si con los resultados el registro no cabe (CHECK de 64 KiB), sus
	// fórmulas quedan vacías; el techo de datos propios (60 KB) deja 5 KB para ellas.
	if len(jsonDe(data))+len(jsonDe(poner)) > 65000 {
		poner = map[string]json.RawMessage{}
		for campo := range nuevos {
			quitar = append(quitar, campo)
			nuevos[campo] = nil
		}
	}
	_, err := r.tx.Exec(ctx, `UPDATE base_records SET data = (data - $3::text[]) || $4::jsonb WHERE id = $1 AND tenant = $2`,
		id, r.a.Tenant, quitar, jsonDe(poner))
	return err
}

// canonico reescribe un valor guardado como lo escribiría json.Marshal, para comparar
// (jsonb devuelve «1000000000000000» donde Go escribe «1e+15»).
func canonico(raw json.RawMessage) []byte {
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return nil
	}
	b, _ := json.Marshal(v)
	return b
}

// encolar anota que la tabla entera tiene que recalcularse. Pedirlo otra vez reinicia
// el recorrido: lo que ya se había recalculado puede haber quedado viejo.
func (r *recalculo) encolar(ctx context.Context, tabla string) error {
	_, err := r.tx.Exec(ctx, `
		INSERT INTO base_recalc (tenant, table_id, base_id) VALUES ($1, $2, $3)
		ON CONFLICT (tenant, table_id) DO UPDATE SET desde_t = NULL, desde_id = NULL, requested_at = now()`,
		r.a.Tenant, tabla, r.base)
	return err
}

// tablaEntera recalcula todos los registros de una tabla: dentro de la transacción si
// caben en el presupuesto, o por la cola.
func (r *recalculo) tablaEntera(ctx context.Context, tabla string) error {
	ids, err := textos(ctx, r.tx, `SELECT id::text FROM base_records WHERE tenant = $1 AND table_id = $2 LIMIT $3`, r.a.Tenant, tabla, r.quedan+1)
	if err != nil {
		return err
	}
	return r.recalcular(ctx, tabla, ids, 0)
}

func textos(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// comprobarFormula valida la fórmula de `campo` (nuevo si no tiene ID) contra el esquema
// de la base: referencias a campos de su tabla o de las tablas que enlaza, y sin ciclos
// —ni entre campos de la misma tabla ni dando la vuelta por otras tablas—.
func comprobarFormula(esquema map[string][]domain.Campo, campo domain.Campo) error {
	f, err := domain.AnalizarFormula(campo.Formula)
	if err != nil {
		return err
	}
	if err := f.Comprobar(esquema[campo.TableID], func(t string) []domain.Campo { return esquema[t] }); err != nil {
		return err
	}
	if campo.ID == "" {
		return nil // un campo nuevo no lo referencia nadie: no puede cerrar un ciclo
	}
	deps := map[string][]string{} // campo de fórmula → campos de los que depende
	for _, campos := range esquema {
		for _, c := range campos {
			if c.ID == campo.ID {
				c.Formula = campo.Formula
			}
			if c.Kind != domain.TipoFormula {
				continue
			}
			if g, err := domain.AnalizarFormula(c.Formula); err == nil {
				for _, ref := range g.Refs {
					deps[c.ID] = append(deps[c.ID], ref.Campo)
				}
			}
		}
	}
	visto := map[string]bool{}
	var llega func(id string) bool
	llega = func(id string) bool {
		if id == campo.ID {
			return true
		}
		if visto[id] {
			return false
		}
		visto[id] = true
		return slices.ContainsFunc(deps[id], llega)
	}
	if slices.ContainsFunc(deps[campo.ID], llega) {
		return fmt.Errorf("%w: fórmula: referencia circular", domain.ErrInvalido)
	}
	return nil
}

// CambiarFormula cambia la expresión de un campo de fórmula (propietario) y recalcula
// la tabla. El tipo del resultado no cambia, como no cambia el de ningún campo.
func (s *Store) CambiarFormula(ctx context.Context, a domain.Actor, campo, expr string) error {
	return s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := tablaDeCampo(ctx, tx, a, campo)
		if err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccEsquema); err != nil {
			return err
		}
		esquema, err := esquemaDeBase(ctx, tx, a, base)
		if err != nil {
			return err
		}
		i := slices.IndexFunc(esquema[tabla], func(c domain.Campo) bool { return c.ID == campo })
		if i < 0 || esquema[tabla][i].Kind != domain.TipoFormula {
			return fmt.Errorf("%w: no es un campo de fórmula", domain.ErrInvalido)
		}
		in, err := domain.ValidarEntradaCampo(domain.EntradaCampo{Name: esquema[tabla][i].Name, Kind: domain.TipoFormula, Formula: expr, Result: esquema[tabla][i].Result})
		if err != nil {
			return err
		}
		antes := esquema[tabla][i].Formula
		esquema[tabla][i].Formula = in.Formula
		if err := comprobarFormula(esquema, esquema[tabla][i]); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE base_fields SET options = options || jsonb_build_object('formula', $3::text) WHERE id = $1 AND tenant = $2`, campo, a.Tenant, in.Formula); err != nil {
			return err
		}
		r := &recalculo{tx: tx, a: a, base: base, esquema: esquema, quedan: presupuestoRecalculo, parches: map[string]map[string]json.RawMessage{}}
		if err := r.tablaEntera(ctx, tabla); err != nil {
			return err
		}
		return auditar(ctx, tx, a, base, &tabla, nil, &campo, "field.formula", jsonDe(antes), jsonDe(in.Formula))
	})
}

// RecalcularPendientes recorre un tramo de cada tabla anotada en `base_recalc` y devuelve
// cuántos tramos hizo (0 = no queda nada). Cada tramo es una transacción en la
// organización de la tabla; con varias réplicas, SKIP LOCKED hace que no se pisen.
func (s *Store) RecalcularPendientes(ctx context.Context) (int, error) {
	type tarea struct{ tenant, tabla string }
	var tareas []tarea
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('app.bases_publico', 'on', true)`); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT tenant, table_id::text FROM base_recalc ORDER BY requested_at LIMIT 20`)
		if err != nil {
			return err
		}
		tareas, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (t tarea, err error) {
			return t, row.Scan(&t.tenant, &t.tabla)
		})
		return err
	})
	hechos := 0
	for _, t := range tareas {
		a := domain.Actor{Sub: "system:recalc", Tenant: t.tenant}
		e := s.en(ctx, a, func(tx pgx.Tx) error {
			hecho, err := tramoDeRecalculo(ctx, tx, a, t.tabla)
			if hecho {
				hechos++
			}
			return err
		})
		err = errors.Join(err, e)
	}
	return hechos, err
}

// tramoDeRecalculo recalcula el tramo siguiente de una tabla de la cola y avanza (o
// termina) su fila. false = la lleva otra réplica, o ya no está.
func tramoDeRecalculo(ctx context.Context, tx pgx.Tx, a domain.Actor, tabla string) (bool, error) {
	var base string
	var desdeT *time.Time
	var desdeID *string
	if err := tx.QueryRow(ctx, `SELECT base_id::text, desde_t, desde_id::text FROM base_recalc WHERE tenant = $1 AND table_id = $2 FOR UPDATE SKIP LOCKED`,
		a.Tenant, tabla).Scan(&base, &desdeT, &desdeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	rows, err := tx.Query(ctx, `
		SELECT id::text, created_at FROM base_records
		 WHERE tenant = $1 AND table_id = $2 AND ($3::timestamptz IS NULL OR (created_at, id) > ($3::timestamptz, $4::uuid))
		 ORDER BY created_at, id LIMIT $5`, a.Tenant, tabla, desdeT, desdeID, tramoRecalculo)
	if err != nil {
		return false, err
	}
	var ids []string
	var id string
	var ultimo time.Time
	if _, err := pgx.ForEachRow(rows, []any{&id, &ultimo}, func() error {
		ids = append(ids, id)
		return nil
	}); err != nil {
		return false, err
	}
	r, err := nuevoRecalculo(ctx, tx, a, base)
	if err != nil {
		return false, err
	}
	if r != nil && len(ids) > 0 {
		// El tramo cabe siempre; lo que dependa de él tiene el presupuesto de una petición.
		r.quedan += len(ids)
		if err := r.recalcular(ctx, tabla, ids, 0); err != nil {
			return false, err
		}
	}
	if r == nil || len(ids) < tramoRecalculo {
		_, err = tx.Exec(ctx, `DELETE FROM base_recalc WHERE tenant = $1 AND table_id = $2`, a.Tenant, tabla)
		return true, err
	}
	_, err = tx.Exec(ctx, `UPDATE base_recalc SET desde_t = $3, desde_id = $4 WHERE tenant = $1 AND table_id = $2`, a.Tenant, tabla, ultimo, ids[len(ids)-1])
	return true, err
}

// Bucle llama a `paso` cada `cada` hasta que el contexto se cancela, y lo repite sin
// esperar mientras haga trabajo. Lo usan la cola de recálculos y la purga de adjuntos.
//
// ponytail: sondeo; LISTEN/NOTIFY si la espera de unos segundos molesta.
func Bucle(ctx context.Context, cada time.Duration, paso func(context.Context) (int, error), alFallar func(error)) {
	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		for ctx.Err() == nil {
			n, err := paso(ctx)
			if err != nil && alFallar != nil {
				alFallar(err)
			}
			if n == 0 || err != nil {
				break
			}
		}
	}
}
