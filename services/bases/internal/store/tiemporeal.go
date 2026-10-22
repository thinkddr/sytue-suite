// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/services/bases/internal/domain"
)

// COEDICIÓN EN TIEMPO REAL (ADR 0068, bases-2 C)
//
// Tres piezas, las tres dentro de la regla de la casa (una petición, una transacción, y
// el permiso resuelto EN ella):
//
//   - Novedades: una tanda del flujo de cambios de una base para UNA persona. Cada tanda
//     vuelve a resolver su ACL, así que un permiso retirado corta el flujo en la tanda
//     siguiente, y cada cambio se filtra por la tabla que esa persona puede leer.
//   - EscribirCelda: una operación de celda con identificador (idempotente) y «último
//     escritor gana»: no falla porque otra persona haya tocado OTRA celda del registro, y
//     si pisa un valor que quien escribe no había visto, lo dice.
//   - Presencia: quién está y en qué celda, autorizada por el servicio en cada latido.

// Novedades es una tanda del flujo.
type Novedades struct {
	Cambios []domain.Cambio
	// Cursor es hasta dónde queda leído: el `id` del último evento entregado.
	Cursor domain.Cursor
	// Atras: quien pregunta se había quedado demasiado atrás; ha de recargar (`resync`).
	Atras     bool
	Presentes []domain.Presente
	RolBase   domain.Rol
}

// Novedades lee los cambios de `base` posteriores a `desde` que `a` puede ver, y quién
// está presente. `desde` nil = empezar AHORA (lo anterior lo carga el cliente por la API);
// «ahora» es la transacción más antigua aún en curso, así que puede contar de nuevo algún
// cambio de justo antes —el cliente vuelve a pedirlo, sin daño—, pero no se salta ninguno.
// ErrNoExiste si la base no existe o `a` ya no tiene ningún acceso: el flujo se cierra.
func (s *Store) Novedades(ctx context.Context, a domain.Actor, base string, desde *domain.Cursor) (Novedades, error) {
	var out Novedades
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		owner, acl, err := cargarACL(ctx, tx, a, base)
		if err != nil {
			return err
		}
		roles := map[string]domain.Rol{}
		rolEnTabla := func(tabla string) domain.Rol {
			r, visto := roles[tabla]
			if !visto {
				r, _ = domain.ResolverRol(owner, acl, a, tabla)
				roles[tabla] = r
			}
			return r
		}
		out.RolBase = rolEnTabla("")
		if out.RolBase == "" && len(acl) == 0 {
			return domain.ErrNoExiste
		}
		// Solo se entregan cambios de transacciones anteriores a la más antigua aún en
		// curso: ninguna fila puede aparecer después por detrás del cursor (migración 0021).
		var horizonte int64
		if err := tx.QueryRow(ctx, `SELECT pg_snapshot_xmin(pg_current_snapshot())::text::bigint`).Scan(&horizonte); err != nil {
			return err
		}
		if desde == nil {
			out.Cursor = domain.Cursor{Xact: horizonte}
		} else {
			out.Cursor = *desde
			if err := leerCambios(ctx, tx, a, base, horizonte, rolEnTabla, &out); err != nil {
				return err
			}
		}
		return leerPresentes(ctx, tx, a, base, rolEnTabla, &out)
	})
	return out, err
}

func leerCambios(ctx context.Context, tx pgx.Tx, a domain.Actor, base string, horizonte int64, rolEnTabla func(string) domain.Rol, out *Novedades) error {
	rows, err := tx.Query(ctx, `
		SELECT xact, id, action, coalesce(table_id::text, ''), coalesce(record_id::text, ''), coalesce(field_id::text, ''), actor, coalesce(op_id::text, '')
		  FROM base_audit
		 WHERE tenant = $1 AND base_id = $2 AND (xact, id) > ($3::bigint, $4::bigint) AND xact < $5
		 ORDER BY xact, id LIMIT $6`, a.Tenant, base, out.Cursor.Xact, out.Cursor.ID, horizonte, domain.MaxCambiosPorTanda+1)
	if err != nil {
		return err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var c domain.Cursor
		var accion, tabla, registro, campo, actor, op string
		if err := rows.Scan(&c.Xact, &c.ID, &accion, &tabla, &registro, &campo, &actor, &op); err != nil {
			return err
		}
		n++
		if n > domain.MaxCambiosPorTanda {
			// Demasiado atrás: no se le va a contar todo. Que recargue y siga desde ahora.
			out.Cambios, out.Atras, out.Cursor = nil, true, domain.Cursor{Xact: horizonte}
			return nil
		}
		out.Cursor = c
		if ev, ok := domain.CambioParaElFlujo(accion, tabla, registro, campo, actor, op, rolEnTabla(""), rolEnTabla(tabla)); ok {
			ev.Cursor = c.String()
			out.Cambios = append(out.Cambios, ev)
		}
	}
	return rows.Err()
}

func leerPresentes(ctx context.Context, tx pgx.Tx, a domain.Actor, base string, rolEnTabla func(string) domain.Rol, out *Novedades) error {
	rows, err := tx.Query(ctx, `
		SELECT conn, sub, name, coalesce(table_id::text, ''), coalesce(record_id::text, ''), coalesce(field_id::text, '')
		  FROM base_presence
		 WHERE tenant = $1 AND base_id = $2 AND seen > now() - make_interval(secs => $3)
		 ORDER BY conn LIMIT $4`, a.Tenant, base, domain.VidaPresencia.Seconds(), domain.MaxPresentes)
	if err != nil {
		return err
	}
	defer rows.Close()
	out.Presentes = []domain.Presente{}
	for rows.Next() {
		var p domain.Presente
		if err := rows.Scan(&p.Conn, &p.Sub, &p.Name, &p.TableID, &p.RecordID, &p.FieldID); err != nil {
			return err
		}
		// En qué celda está alguien solo lo ve quien puede leer esa tabla; quien no,
		// sabe que está en la base y nada más.
		if p.TableID != "" && !domain.Permite(rolEnTabla(p.TableID), domain.AccLeer) {
			p.TableID, p.RecordID, p.FieldID = "", "", ""
		}
		out.Presentes = append(out.Presentes, p)
	}
	return rows.Err()
}

// Latir registra o renueva la presencia de una pestaña de `a` en la base (y, si la
// dice, en una celda de una tabla que puede leer). El `sub` y el nombre los pone el
// servicio; una pestaña no se puede hacer pasar por la de otra persona.
func (s *Store) Latir(ctx context.Context, a domain.Actor, base string, p domain.Presente) error {
	if !domain.ConexionValida(p.Conn) || len([]rune(p.Name)) > 200 {
		return domain.ErrInvalido
	}
	return s.en(ctx, a, func(tx pgx.Tx) error {
		if p.TableID != "" {
			b, err := baseDeTabla(ctx, tx, a, p.TableID)
			if err != nil || b != base {
				return domain.ErrNoExiste
			}
		}
		if err := autorizar(ctx, tx, a, base, p.TableID, domain.AccLeer); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM base_presence WHERE tenant = $1 AND base_id = $2 AND seen < now() - make_interval(secs => $3)`,
			a.Tenant, base, domain.VidaPresencia.Seconds()); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM base_presence WHERE tenant = $1 AND base_id = $2 AND conn <> $3`, a.Tenant, base, p.Conn).Scan(&n); err != nil {
			return err
		}
		if n >= domain.MaxPresentes {
			return domain.ErrLimite
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO base_presence (tenant, base_id, conn, sub, name, table_id, record_id, field_id)
			VALUES ($1, $2, $3, $4, $5, $6::uuid, $7::uuid, $8::uuid)
			ON CONFLICT (base_id, conn) DO UPDATE
			   SET name = EXCLUDED.name, table_id = EXCLUDED.table_id, record_id = EXCLUDED.record_id,
			       field_id = EXCLUDED.field_id, seen = now()
			 WHERE base_presence.sub = EXCLUDED.sub AND base_presence.tenant = EXCLUDED.tenant`,
			a.Tenant, base, p.Conn, a.Sub, p.Name, ptr(p.TableID), ptr(p.RecordID), ptr(p.FieldID))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrInvalido // esa pestaña es de otra persona
		}
		return nil
	})
}

// Irse quita la presencia de una pestaña de `a` (al cerrar el flujo). Sin acceso ya no
// hay nada que quitar: la fila caduca sola.
func (s *Store) Irse(ctx context.Context, a domain.Actor, base, conn string) error {
	if !domain.ConexionValida(conn) {
		return nil
	}
	return s.en(ctx, a, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `DELETE FROM base_presence WHERE tenant = $1 AND base_id = $2 AND conn = $3 AND sub = $4`, a.Tenant, base, conn, a.Sub)
		return err
	})
}

// registroPorID lee un registro con sus enlaces. Quien llama ya ha autorizado.
func registroPorID(ctx context.Context, tx pgx.Tx, a domain.Actor, registro, tabla string) (domain.Registro, error) {
	r := domain.Registro{ID: registro, TableID: tabla, Links: map[string][]string{}}
	var data []byte
	if err := errNoFilas(tx.QueryRow(ctx, `
		SELECT data, rev, created_by, updated_by, created_at, updated_at FROM base_records WHERE id = $1 AND tenant = $2`, registro, a.Tenant).
		Scan(&data, &r.Rev, &r.CreatedBy, &r.UpdatedBy, &r.CreatedAt, &r.UpdatedAt)); err != nil {
		return r, err
	}
	if err := json.Unmarshal(data, &r.Data); err != nil || r.Data == nil {
		r.Data = map[string]json.RawMessage{}
	}
	regs := []domain.Registro{r}
	err := cargarEnlaces(ctx, tx, a, regs, []string{registro})
	return regs[0], err
}

// ObtenerRegistro devuelve UN registro (lector): lo que pide una pantalla abierta cuando
// el flujo le dice que ese registro ha cambiado.
func (s *Store) ObtenerRegistro(ctx context.Context, a domain.Actor, registro string) (domain.Registro, error) {
	var r domain.Registro
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := registroDe(ctx, tx, a, registro)
		if err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccLeer); err != nil {
			return err
		}
		r, err = registroPorID(ctx, tx, a, registro, tabla)
		return err
	})
	return r, err
}

// Celda es una operación de celda: el valor nuevo de UN campo de un registro.
type Celda struct {
	FieldID string
	// Value es el valor de un campo que no es de enlace (null lo vacía); Links, los ids
	// de un campo de enlace. Se usa el que corresponde al tipo del campo.
	Value json.RawMessage
	Links []string
	// Expected es lo que quien escribe veía en la celda. Si ya no es eso, alguien la
	// cambió entretanto: se escribe igual (último escritor gana) y se avisa. nil = no se compara.
	Expected json.RawMessage
	// OpID identifica la operación (UUID del cliente): repetirla no la aplica dos veces.
	OpID string
}

// ResultadoCelda es lo que contesta una operación de celda.
type ResultadoCelda struct {
	Record domain.Registro `json:"record"`
	// Conflict: se ha pisado un valor que quien escribe no había visto.
	Conflict bool `json:"conflict"`
	// Replayed: esta operación ya se había aplicado; no se ha vuelto a escribir.
	Replayed bool `json:"replayed"`
}

// EscribirCelda aplica una operación de celda (editor). No pide la versión del registro:
// dos personas en celdas distintas del mismo registro no se estorban, y en la misma
// celda gana quien escribe después, sabiendo que ha pisado algo.
func (s *Store) EscribirCelda(ctx context.Context, a domain.Actor, registro string, c Celda) (ResultadoCelda, error) {
	var out ResultadoCelda
	err := s.en(ctx, a, func(tx pgx.Tx) error {
		tabla, base, err := registroDe(ctx, tx, a, registro)
		if err != nil {
			return err
		}
		if err := autorizar(ctx, tx, a, base, tabla, domain.AccEscribirReg); err != nil {
			return err
		}
		var actual []byte
		if err := errNoFilas(tx.QueryRow(ctx, `SELECT data FROM base_records WHERE id = $1 AND tenant = $2 FOR UPDATE`, registro, a.Tenant).Scan(&actual)); err != nil {
			return err
		}
		// Con el registro ya bloqueado: una repetición que llegue a la vez espera aquí y ve la primera.
		if out.Replayed, err = opYaAplicada(ctx, tx, a, c.OpID, registro); err != nil {
			return err
		}
		if !out.Replayed {
			if out.Conflict, err = aplicarCelda(ctx, tx, a, base, tabla, registro, actual, c); err != nil {
				return err
			}
			// Las fórmulas que dependen de la celda se recalculan como en cualquier otra escritura.
			if _, err := trasEscribir(ctx, tx, a, base, tabla, []string{registro}); err != nil {
				return err
			}
		}
		out.Record, err = registroPorID(ctx, tx, a, registro, tabla)
		return err
	})
	return out, err
}

// opYaAplicada dice si la operación `op` ya dejó su fila en la traza. Un identificador
// que ya se usó para OTRO registro no es una repetición: es una entrada no válida.
func opYaAplicada(ctx context.Context, tx pgx.Tx, a domain.Actor, op, registro string) (bool, error) {
	if op == "" {
		return false, nil
	}
	var de string
	err := tx.QueryRow(ctx, `SELECT coalesce(record_id::text, '') FROM base_audit WHERE tenant = $1 AND op_id = $2`, a.Tenant, op).Scan(&de)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return false, nil
	case err != nil:
		return false, err
	case de != registro:
		return false, fmt.Errorf("%w: identificador de operación ya usado", domain.ErrInvalido)
	}
	return true, nil
}

// aplicarCelda escribe la celda y su fila de traza (con el identificador de la
// operación). Devuelve si había un valor distinto del que quien escribe esperaba.
func aplicarCelda(ctx context.Context, tx pgx.Tx, a domain.Actor, base, tabla, registro string, actual []byte, c Celda) (bool, error) {
	campos, err := camposDe(ctx, tx, a, tabla)
	if err != nil {
		return false, err
	}
	i := slices.IndexFunc(campos, func(x domain.Campo) bool { return x.ID == c.FieldID })
	if i < 0 {
		return false, fmt.Errorf("%w: campo desconocido", domain.ErrInvalido)
	}
	var antes, despues json.RawMessage
	accion := "record.update"
	if campos[i].Kind == domain.TipoRelacion {
		accion = "record.link"
		previos, err := guardarEnlaces(ctx, tx, a, campos, tabla, registro, map[string][]string{c.FieldID: c.Links})
		if err != nil {
			return false, err
		}
		nuevos, _ := domain.ValidarEnlaces(c.Links)
		slices.Sort(nuevos)
		if slices.Equal(previos[c.FieldID], nuevos) {
			return false, nil // nada que escribir
		}
		antes, despues = jsonDe(previos[c.FieldID]), jsonDe(nuevos)
		if _, err := tx.Exec(ctx, `UPDATE base_records SET rev = rev + 1, updated_by = $3, updated_at = now() WHERE id = $1 AND tenant = $2`, registro, a.Tenant, a.Sub); err != nil {
			return false, err
		}
	} else {
		parcial, err := domain.ValidarParcial(campos, map[string]json.RawMessage{c.FieldID: c.Value})
		if err != nil {
			return false, err
		}
		merged, cambios := fusionar(actual, parcial)
		if len(cambios) == 0 {
			return false, nil
		}
		if err := domain.TamanoDatos(merged); err != nil {
			return false, err
		}
		antes, despues = cambios[0].antes, cambios[0].despues
		if _, err := tx.Exec(ctx, `UPDATE base_records SET data = $3::jsonb, rev = rev + 1, updated_by = $4, updated_at = now() WHERE id = $1 AND tenant = $2`,
			registro, a.Tenant, jsonDe(merged), a.Sub); err != nil {
			return false, err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO base_audit (tenant, base_id, table_id, record_id, field_id, actor, action, before, after, op_id)
		VALUES ($1, $2, $3::uuid, $4::uuid, $5::uuid, $6, $7, $8::jsonb, $9::jsonb, $10::uuid)`,
		a.Tenant, base, tabla, registro, c.FieldID, a.Sub, accion, nilSiVacio(domain.Acotar(antes)), nilSiVacio(domain.Acotar(despues)), ptr(c.OpID))
	if err != nil {
		return false, fmt.Errorf("auditoría: %w", err)
	}
	return c.Expected != nil && !mismoJSON(antes, c.Expected), nil
}

// mismoJSON compara dos valores JSON por su contenido (vacío y `null` son lo mismo, y
// en una lista de enlaces no cuenta el orden).
func mismoJSON(x, y json.RawMessage) bool {
	var vx, vy any
	if len(x) > 0 && json.Unmarshal(x, &vx) != nil {
		return false
	}
	if len(y) > 0 && json.Unmarshal(y, &vy) != nil {
		return false
	}
	lx, okx := vx.([]any)
	ly, oky := vy.([]any)
	if okx && oky {
		orden := func(p, q any) int { return cmp.Compare(fmt.Sprint(p), fmt.Sprint(q)) }
		lx, ly = slices.Clone(lx), slices.Clone(ly)
		slices.SortFunc(lx, orden)
		slices.SortFunc(ly, orden)
		return reflect.DeepEqual(lx, ly)
	}
	if (okx && len(lx) == 0 && vy == nil) || (oky && len(ly) == 0 && vx == nil) {
		return true // sin enlaces y nulo
	}
	return reflect.DeepEqual(vx, vy)
}
