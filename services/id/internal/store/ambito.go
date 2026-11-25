// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

// EL ÁMBITO DECLARADO de una petición, y quien se lo hace cumplir a la base
// (migración 0127).
//
// Sytue ID no puede llevar la RLS de los servicios de datos —sin tenant no se ve
// nada— porque es quien decide el tenant: el inicio de sesión, el token endpoint,
// SCIM o la consola de plataforma consultan sin organización por diseño. Así que la
// base deja pasar lo que llega SIN ámbito y confina lo que llega CON él. El ámbito lo
// pone el servidor HTTP en el contexto de la petición (la organización activa del
// token en la API de administración, la persona del token en las rutas de seguridad
// de su cuenta), y este fichero lo traslada a `app.org_id`, `app.tenant` y
// `app.user_id` en CADA sentencia, también en las que no van en transacción.
//
// Lo que se confina no depende de recordar nada en los 300 métodos del store: todos
// pasan por DB() o por InTx, y los dos lo aplican.

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// Ambito es lo que una petición deja ver a la base. Un campo vacío no confina ese eje.
type Ambito struct {
	// OrgID y Tenant son la MISMA organización (uuid y slug): las tablas de la bóveda
	// y del perfil de Apple la guardan por slug, el resto por uuid. Van siempre juntos.
	OrgID  string
	Tenant string
	// UserID confina a las filas de una persona.
	UserID string
}

func (a Ambito) vacio() bool { return a.OrgID == "" && a.Tenant == "" && a.UserID == "" }

type claveAmbito struct{}

// ConAmbitoDeOrganizacion confina lo que se haga con ctx a una organización. Un uuid o
// un slug mal formado no se corrige ni se ignora: deja el ámbito inválido y la primera
// consulta falla (fail-closed), porque ignorarlo sería servir sin confinar.
func ConAmbitoDeOrganizacion(ctx context.Context, orgID, slug string) context.Context {
	a, _ := AmbitoDe(ctx)
	a.OrgID, a.Tenant = orgID, slug
	return context.WithValue(ctx, claveAmbito{}, a)
}

// ConAmbitoDePersona confina lo que se haga con ctx a las filas de una persona.
func ConAmbitoDePersona(ctx context.Context, userID string) context.Context {
	a, _ := AmbitoDe(ctx)
	a.UserID = userID
	return context.WithValue(ctx, claveAmbito{}, a)
}

// AmbitoDe devuelve el ámbito declarado en ctx, si hay alguno.
func AmbitoDe(ctx context.Context) (Ambito, bool) {
	a, ok := ctx.Value(claveAmbito{}).(Ambito)
	return a, ok && !a.vacio()
}

var errAmbitoInvalido = errors.New("store: ámbito de RLS inválido")

func (a Ambito) validar() error {
	if a.OrgID != "" {
		if _, err := uuid.Parse(a.OrgID); err != nil {
			return fmt.Errorf("%w: org_id", errAmbitoInvalido)
		}
	}
	if a.UserID != "" {
		if _, err := uuid.Parse(a.UserID); err != nil {
			return fmt.Errorf("%w: user_id", errAmbitoInvalido)
		}
	}
	if a.Tenant != "" && !tenancy.ValidSlug(a.Tenant) {
		return fmt.Errorf("%w: tenant", errAmbitoInvalido)
	}
	// La organización se declara por los dos lados o por ninguno: con solo uno, las
	// tablas que la guardan por el otro quedarían sin confinar sin que nada lo dijera.
	if (a.OrgID == "") != (a.Tenant == "") {
		return fmt.Errorf("%w: organización a medias", errAmbitoInvalido)
	}
	return nil
}

// fijarAmbito escribe el ámbito en la transacción con SET LOCAL (set_config(…, true)):
// muere con ella y no se lo lleva la siguiente que reutilice la conexión, también
// detrás de PgBouncer en modo transacción. Los valores van como parámetros.
func fijarAmbito(ctx context.Context, q pg.Querier, a Ambito) error {
	if err := a.validar(); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `SELECT set_config('app.org_id', $1, true),
		set_config('app.tenant', $2, true), set_config('app.user_id', $3, true)`,
		a.OrgID, a.Tenant, a.UserID)
	if err != nil {
		return fmt.Errorf("store: fijar ámbito: %w", err)
	}
	return nil
}

// conAmbito es el pg.Querier que devuelve DB(): sin ámbito en el contexto delega tal
// cual en el pool (los flujos globales no pagan nada); con ámbito, cada sentencia
// suelta va en su propia transacción con el ámbito fijado, porque una variable LOCAL
// fuera de una transacción no confinaría nada.
type conAmbito struct{ db pg.Database }

func (c conAmbito) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	a, ok := AmbitoDe(ctx)
	if !ok {
		return c.db.Exec(ctx, sql, args...)
	}
	var tag pgconn.CommandTag
	err := c.db.InTx(ctx, func(tx pgx.Tx) error {
		if err := fijarAmbito(ctx, tx, a); err != nil {
			return err
		}
		var err error
		tag, err = tx.Exec(ctx, sql, args...)
		return err
	})
	return tag, err
}

func (c conAmbito) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	a, ok := AmbitoDe(ctx)
	if !ok {
		return c.db.QueryRow(ctx, sql, args...)
	}
	return filaConAmbito{ctx: ctx, db: c.db, ambito: a, sql: sql, args: args}
}

// Query con ámbito LEE la respuesta entera dentro de la transacción y la devuelve en
// memoria: las filas no pueden sobrevivir al COMMIT que cierra el ámbito. Las rutas
// confinadas son las de administración y las de la cuenta, que paginan; un listado
// sin límite de verdad no pasa por aquí.
func (c conAmbito) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	a, ok := AmbitoDe(ctx)
	if !ok {
		return c.db.Query(ctx, sql, args...)
	}
	var out *filasEnMemoria
	err := c.db.InTx(ctx, func(tx pgx.Tx) error {
		if err := fijarAmbito(ctx, tx, a); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		out, err = leerFilas(rows)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// filaConAmbito aplaza la consulta al Scan, que es cuando pgx la ejecuta también en
// el camino normal: así el error (incluido pgx.ErrNoRows, sin envolver) sale donde el
// llamante lo espera.
type filaConAmbito struct {
	ctx    context.Context
	db     pg.Database
	ambito Ambito
	sql    string
	args   []any
}

func (f filaConAmbito) Scan(dest ...any) error {
	return f.db.InTx(f.ctx, func(tx pgx.Tx) error {
		if err := fijarAmbito(f.ctx, tx, f.ambito); err != nil {
			return err
		}
		return tx.QueryRow(f.ctx, f.sql, f.args...).Scan(dest...)
	})
}

// filasEnMemoria implementa pgx.Rows sobre una respuesta ya leída. Escanea con un
// pgtype.Map propio: el de la conexión vuelve al pool con ella y no admite uso
// concurrente. El servicio no registra tipos propios, así que el mapa por defecto es
// el mismo que tendría la conexión.
type filasEnMemoria struct {
	campos  []pgconn.FieldDescription
	filas   [][][]byte
	tag     pgconn.CommandTag
	mapa    *pgtype.Map
	actual  int
	err     error
	cerrada bool
}

func leerFilas(rows pgx.Rows) (*filasEnMemoria, error) {
	defer rows.Close()
	out := &filasEnMemoria{
		campos: append([]pgconn.FieldDescription(nil), rows.FieldDescriptions()...),
		mapa:   pgtype.NewMap(),
		actual: -1,
	}
	for rows.Next() {
		crudos := rows.RawValues()
		fila := make([][]byte, len(crudos))
		for i, v := range crudos {
			if v != nil {
				fila[i] = append([]byte{}, v...)
			}
		}
		out.filas = append(out.filas, fila)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.tag = rows.CommandTag()
	return out, nil
}

func (r *filasEnMemoria) Close()                                       { r.cerrada = true }
func (r *filasEnMemoria) Err() error                                   { return r.err }
func (r *filasEnMemoria) CommandTag() pgconn.CommandTag                { return r.tag }
func (r *filasEnMemoria) FieldDescriptions() []pgconn.FieldDescription { return r.campos }
func (r *filasEnMemoria) Conn() *pgx.Conn                              { return nil }
func (r *filasEnMemoria) TypeMap() *pgtype.Map                         { return r.mapa }

func (r *filasEnMemoria) Next() bool {
	if r.cerrada || r.err != nil || r.actual+1 >= len(r.filas) {
		r.cerrada = true
		return false
	}
	r.actual++
	return true
}

func (r *filasEnMemoria) RawValues() [][]byte {
	if r.actual < 0 || r.actual >= len(r.filas) {
		return nil
	}
	return r.filas[r.actual]
}

func (r *filasEnMemoria) Scan(dest ...any) error {
	if len(dest) == 1 {
		if rs, ok := dest[0].(pgx.RowScanner); ok {
			return r.fallo(rs.ScanRow(r))
		}
	}
	return r.fallo(pgx.ScanRow(r.mapa, r.campos, r.RawValues(), dest...))
}

// Values reproduce lo que hace pgx con una fila: decodifica por OID y, si no conoce
// el tipo, entrega el texto o una copia de los bytes.
func (r *filasEnMemoria) Values() ([]any, error) {
	crudos := r.RawValues()
	if crudos == nil {
		return nil, r.fallo(errors.New("store: Values sin fila actual"))
	}
	valores := make([]any, 0, len(r.campos))
	for i, fd := range r.campos {
		buf := crudos[i]
		switch dt, ok := r.mapa.TypeForOID(fd.DataTypeOID); {
		case buf == nil:
			valores = append(valores, nil)
		case ok:
			v, err := dt.Codec.DecodeValue(r.mapa, fd.DataTypeOID, fd.Format, buf)
			if err != nil {
				return nil, r.fallo(err)
			}
			valores = append(valores, v)
		case fd.Format == pgx.TextFormatCode:
			valores = append(valores, string(buf))
		default:
			valores = append(valores, append([]byte{}, buf...))
		}
	}
	return valores, nil
}

// fallo deja el error en las filas, como pgx: el llamante que solo mira rows.Err()
// tras el bucle también lo ve.
func (r *filasEnMemoria) fallo(err error) error {
	if err != nil && r.err == nil {
		r.err = err
	}
	return err
}
