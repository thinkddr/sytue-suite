// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// ---------------------------------------------------------------------------
// Sytue DB Studio — introspección, lectura de filas y consola SQL del Postgres
// gestionado de un proyecto.
//
// El control-plane abre una conexión EFÍMERA a la base de datos DEL CLIENTE (la
// que aprovisiona KubeProvisioner.provisionPostgres) usando el DSN cifrado en
// managed_services. Nada de pool: una conexión por petición que se cierra
// SIEMPRE, para que una consola SQL abierta no consuma slots del Postgres del
// cliente ni del control-plane.
//
// El SQL lo escribe el cliente sobre SU propia base de datos: no se filtra por
// palabras clave (un "prohibido DROP" es a la vez inútil —hay mil formas de
// escribirlo— y molesto: es su BD). Lo que sí se acota es el daño a la
// PLATAFORMA: statement_timeout corto, timeout de conexión, tope duro de filas
// y de bytes de respuesta, y cierre garantizado de la conexión.
// ---------------------------------------------------------------------------

const (
	// dbStudioStatementTimeout: ninguna consulta del Studio puede tener ocupado
	// un worker del Postgres del cliente más que esto. Se aplica como GUC en el
	// paquete de arranque, así que cubre TODAS las sentencias de la conexión
	// (incluida la SQL arbitraria del editor).
	dbStudioStatementTimeout = 8 * time.Second
	// dbStudioConnectTimeout: si el Postgres del proyecto no está (pod caído,
	// service sin endpoints) se falla rápido en vez de colgar el handler.
	dbStudioConnectTimeout = 5 * time.Second
	// dbStudioMaxRows / dbStudioMaxBytes: topes DUROS de la respuesta. Un
	// `SELECT * FROM tabla_de_50M` no puede convertirse en un OOM del
	// control-plane ni en un JSON de gigabytes hacia el navegador.
	dbStudioMaxRows  = 500
	dbStudioMaxBytes = 1 << 20 // 1 MiB de datos aproximados
	// dbStudioMaxTables / dbStudioMaxColumns acotan la introspección de un
	// esquema grande (un catálogo con miles de tablas no debe tumbar la vista).
	dbStudioMaxTables  = 200
	dbStudioMaxColumns = 5000
	// dbStudioMaxSQLLen: la consola es para consultas escritas a mano; un cuerpo
	// enorme se rechaza antes de tocar la BD.
	dbStudioMaxSQLLen = 64 * 1024
	// dbStudioRowsDefault / dbStudioRowsMax acotan ?limit= del explorador.
	dbStudioRowsDefault = 50
	dbStudioRowsMax     = 200
	// dbStudioExposedSchema es el esquema que PostgREST publica en la API REST
	// automática (PGRST_DB_SCHEMA en el Deployment que crea el provisioner). Es
	// la fuente de verdad de `apiExposed`: no se inventa por tabla.
	dbStudioExposedSchema = "public"
	// dbStudioMask es la máscara de credenciales. El control-plane NUNCA
	// devuelve la contraseña ni el secreto JWT en claro por API.
	dbStudioMask = "••••"
)

// DbColumn describe una columna introspeccionada (nombres JSON en camelCase:
// son el contrato exacto que valida el dashboard, no se pueden renombrar).
type DbColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
	// nolint:tagliatelle // camelCase publicado: lo consume el DB Studio del
	// dashboard (cloud-dashboard/src/lib/data.ts). Renombrarlo a snake_case es un
	// cambio de contrato con su propio despliegue, no un arreglo de lint.
	IsPrimaryKey bool   `json:"isPrimaryKey"`
	References   string `json:"references,omitempty"`
	Default      string `json:"default,omitempty"`
}

// DbTable es una tabla del Postgres del proyecto con sus columnas.
type DbTable struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	// RowCount es la ESTIMACIÓN del planificador (pg_class.reltuples), no un
	// count(*): en el listado hay N tablas y un conteo exacto por tabla es un
	// escaneo completo cada una (no escala y se comería el statement_timeout).
	// El conteo EXACTO sí se hace al abrir una tabla concreta (DbTableRows).
	RowCount   int64      `json:"rowCount"`
	Columns    []DbColumn `json:"columns"`
	APIExposed bool       `json:"apiExposed"`
}

// DbRows son las primeras filas de una tabla.
type DbRows struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	TotalRows int64            `json:"totalRows"`
}

// SQLResult es el resultado de una consulta de la consola. Los errores de SQL
// del usuario NO son errores HTTP: viajan en Error para pintarse en el editor.
type SQLResult struct {
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
	RowCount  int              `json:"rowCount"`
	ElapsedMs int64            `json:"elapsedMs"`
	Notice    string           `json:"notice,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// DbAPIInfo describe la API REST automática (PostgREST) del proyecto. Todas las
// credenciales van ENMASCARADAS: este endpoint es de consulta, no de reparto de
// secretos (las claves reales se inyectan en los despliegues y se gestionan en
// Secrets).
type DbAPIInfo struct {
	RestURL           string `json:"restUrl"`
	AnonKeyPreview    string `json:"anonKeyPreview"`
	ServiceKeyPreview string `json:"serviceKeyPreview"`
	ConnectionString  string `json:"connectionString"`
}

// dbStudioTarget es el Postgres gestionado de un proyecto ya resuelto: servicio
// (para metadatos no sensibles) + credenciales descifradas.
type dbStudioTarget struct {
	svc     domain.ManagedService
	dsn     string
	restURL string
}

// resolveDbTarget localiza el servicio postgres READY del proyecto y descifra su
// DSN. Aislamiento: ListServices corre bajo RLS con el tenant del contexto, así
// que un projectID de otro tenant devuelve vacío → ErrNotFound (nunca se llega a
// pedir credenciales). ResolveServiceCreds se acota además por tenant explícito.
func (s *Service) resolveDbTarget(ctx context.Context, projectID uuid.UUID) (dbStudioTarget, error) {
	svcs, err := s.store.ListServices(ctx, projectID)
	if err != nil {
		return dbStudioTarget{}, err
	}
	var pg domain.ManagedService
	found := false
	// El de PRODUCCIÓN: es el que explora el panel, y es el único cuyas
	// credenciales devuelve la consulta de abajo. Un proyecto puede tener además
	// un Postgres para sus vistas previas, y elegir «el primero» mezclaría la
	// ficha de uno con la contraseña del otro.
	for _, svc := range svcs {
		if svc.Kind == domain.ServicePostgres && svc.Status == domain.ServiceReady && svc.Target != domain.TargetPreview {
			pg, found = svc, true
			break
		}
	}
	if !found {
		return dbStudioTarget{}, fmt.Errorf("%w: el proyecto no tiene un Sytue Postgres listo", domain.ErrNotFound)
	}
	// Sin store concreto o sin cifrador (binarios/tests que no los cablean) no
	// hay forma REAL de obtener el DSN: se responde 404 en vez de inventar datos.
	if s.pstore == nil || s.sealer == nil {
		return dbStudioTarget{}, domain.ErrNotFound
	}
	t := tenancy.MustFrom(ctx)
	encs, err := s.pstore.ResolveServiceCreds(ctx, t.Slug, projectID, domain.TargetProduction)
	if err != nil {
		return dbStudioTarget{}, err
	}
	enc, ok := encs[string(domain.ServicePostgres)]
	if !ok || len(enc) == 0 {
		return dbStudioTarget{}, fmt.Errorf("%w: el servicio postgres no tiene credenciales", domain.ErrNotFound)
	}
	plain, err := s.sealer.Open(enc)
	if err != nil {
		return dbStudioTarget{}, fmt.Errorf("db studio: descifrar credenciales: %w", err)
	}
	var creds map[string]string
	if err := json.Unmarshal(plain, &creds); err != nil {
		return dbStudioTarget{}, fmt.Errorf("db studio: credenciales ilegibles: %w", err)
	}
	dsn := creds["DATABASE_URL"]
	if dsn == "" {
		return dbStudioTarget{}, fmt.Errorf("%w: sin DATABASE_URL en las credenciales", domain.ErrNotFound)
	}
	restURL := pg.Config["rest_url"]
	if restURL == "" {
		restURL = creds["POSTGREST_URL"]
	}
	return dbStudioTarget{svc: pg, dsn: dsn, restURL: restURL}, nil
}

// dbStudioConnConfig construye la configuración de conexión con los límites de
// la plataforma ya puestos EN EL ARRANQUE (no como sentencias SET aparte, que
// costarían un round-trip y que el SQL del usuario podría cambiar en la misma
// sesión).
func dbStudioConnConfig(dsn string) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db studio: DSN inválido: %w", err)
	}
	cfg.ConnectTimeout = dbStudioConnectTimeout
	if cfg.RuntimeParams == nil {
		cfg.RuntimeParams = map[string]string{}
	}
	cfg.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", dbStudioStatementTimeout.Milliseconds())
	// Una transacción abierta por un editor SQL que el usuario deja a medias
	// bloquea vacuum y locks en la BD del cliente: se corta igual de corto.
	cfg.RuntimeParams["idle_in_transaction_session_timeout"] = fmt.Sprintf("%d", dbStudioStatementTimeout.Milliseconds())
	// Identifica la sesión en pg_stat_activity del cliente (soporte/auditoría).
	cfg.RuntimeParams["application_name"] = "sytue-db-studio"
	return cfg, nil
}

// dbStudioConnect abre una conexión EFÍMERA al Postgres del proyecto. El
// llamante DEBE cerrarla siempre (defer): no hay pool, cada petición del Studio
// abre y cierra la suya.
func dbStudioConnect(ctx context.Context, dsn string) (*pgx.Conn, error) {
	cfg, err := dbStudioConnConfig(dsn)
	if err != nil {
		return nil, err
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("db studio: conectar con el Postgres del proyecto: %w", err)
	}
	return conn, nil
}

// tablesQuery lista tablas ordinarias y particionadas de los esquemas del
// usuario. reltuples es la estimación del planificador (-1 = nunca analizada →
// 0). Sin identificadores dinámicos: solo catálogo.
const tablesQuery = `
SELECT n.nspname, c.relname, GREATEST(c.reltuples, 0)::bigint
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg\_%'
ORDER BY n.nspname, c.relname
LIMIT $1`

// columnsQuery trae las columnas de esos mismos esquemas en UNA consulta (nada
// de N+1 por tabla): tipo formateado, nulabilidad, pertenencia a la PK, defecto
// y la FK simple a la que apunta.
const columnsQuery = `
SELECT n.nspname, c.relname, a.attname,
       format_type(a.atttypid, a.atttypmod),
       NOT a.attnotnull,
       COALESCE(pk.yes, false),
       COALESCE(pg_get_expr(d.adbin, d.adrelid), ''),
       COALESCE(fk.ref, '')
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
LEFT JOIN pg_attrdef d ON d.adrelid = c.oid AND d.adnum = a.attnum
LEFT JOIN LATERAL (
  SELECT true AS yes FROM pg_index i
  WHERE i.indrelid = c.oid AND i.indisprimary AND a.attnum = ANY (i.indkey)
  LIMIT 1
) pk ON true
LEFT JOIN LATERAL (
  SELECT fc.relname || '.' || fa.attname AS ref
  FROM pg_constraint con
  JOIN pg_class fc ON fc.oid = con.confrelid
  JOIN pg_attribute fa ON fa.attrelid = con.confrelid AND fa.attnum = con.confkey[1]
  WHERE con.conrelid = c.oid AND con.contype = 'f' AND con.conkey[1] = a.attnum
  LIMIT 1
) fk ON true
WHERE c.relkind IN ('r', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
  AND n.nspname NOT LIKE 'pg\_%'
ORDER BY n.nspname, c.relname, a.attnum
LIMIT $1`

// DbTables introspecciona el esquema del Postgres del proyecto.
func (s *Service) DbTables(ctx context.Context, projectID uuid.UUID) ([]DbTable, error) {
	target, err := s.resolveDbTarget(ctx, projectID)
	if err != nil {
		return nil, err
	}
	conn, err := dbStudioConnect(ctx, target.dsn)
	if err != nil {
		return nil, err
	}
	defer conn.Close(context.WithoutCancel(ctx))

	out := make([]DbTable, 0, 16)
	idx := map[string]int{} // "esquema.tabla" → posición en out
	rows, err := conn.Query(ctx, tablesQuery, dbStudioMaxTables)
	if err != nil {
		return nil, fmt.Errorf("db studio: listar tablas: %w", err)
	}
	for rows.Next() {
		var t DbTable
		if err := rows.Scan(&t.Schema, &t.Name, &t.RowCount); err != nil {
			rows.Close()
			return nil, err
		}
		t.Columns = []DbColumn{}
		t.APIExposed = t.Schema == dbStudioExposedSchema
		idx[t.Schema+"."+t.Name] = len(out)
		out = append(out, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db studio: listar tablas: %w", err)
	}

	crows, err := conn.Query(ctx, columnsQuery, dbStudioMaxColumns)
	if err != nil {
		return nil, fmt.Errorf("db studio: introspeccionar columnas: %w", err)
	}
	defer crows.Close()
	for crows.Next() {
		var schema, table string
		var col DbColumn
		if err := crows.Scan(&schema, &table, &col.Name, &col.Type, &col.Nullable,
			&col.IsPrimaryKey, &col.Default, &col.References); err != nil {
			return nil, err
		}
		i, ok := idx[schema+"."+table]
		if !ok {
			continue // tabla fuera del tope de dbStudioMaxTables
		}
		out[i].Columns = append(out[i].Columns, col)
	}
	if err := crows.Err(); err != nil {
		return nil, fmt.Errorf("db studio: introspeccionar columnas: %w", err)
	}
	return out, nil
}

// DbTableRows devuelve las primeras filas de una tabla. `table` llega del
// cliente como "esquema.nombre": se valida contra el CATÁLOGO (la tabla debe
// existir) y el identificador se compone con pgx.Identifier.Sanitize — jamás por
// concatenación del texto del cliente.
func (s *Service) DbTableRows(ctx context.Context, projectID uuid.UUID, table string, limit int) (DbRows, error) {
	schema, name, err := parseQualifiedTable(table)
	if err != nil {
		return DbRows{}, err
	}
	if limit <= 0 {
		limit = dbStudioRowsDefault
	}
	if limit > dbStudioRowsMax {
		limit = dbStudioRowsMax
	}
	target, err := s.resolveDbTarget(ctx, projectID)
	if err != nil {
		return DbRows{}, err
	}
	conn, err := dbStudioConnect(ctx, target.dsn)
	if err != nil {
		return DbRows{}, err
	}
	defer conn.Close(context.WithoutCancel(ctx))

	// La tabla debe existir en el catálogo (parámetros, no interpolación). De
	// paso trae la estimación de filas, que sirve de red si el conteo exacto no
	// cabe en el statement_timeout.
	var estimate int64
	err = conn.QueryRow(ctx, `
		SELECT GREATEST(c.reltuples, 0)::bigint
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r', 'p') AND n.nspname = $1 AND c.relname = $2`,
		schema, name).Scan(&estimate)
	if err != nil {
		return DbRows{}, fmt.Errorf("%w: la tabla %s.%s no existe", domain.ErrNotFound, schema, name)
	}

	ident := pgx.Identifier{schema, name}.Sanitize()
	rows, err := conn.Query(ctx, "SELECT * FROM "+ident+" LIMIT $1", limit)
	if err != nil {
		return DbRows{}, fmt.Errorf("db studio: leer filas: %w", err)
	}
	cols, recs, err := collectRows(rows, limit)
	if err != nil {
		return DbRows{}, fmt.Errorf("db studio: leer filas: %w", err)
	}

	// Conteo EXACTO de la tabla abierta (es lo que el usuario ve como "de N
	// filas"). Si no cabe en el statement_timeout se cae a la estimación: la
	// alternativa sería fallar la vista entera por un contador.
	total := estimate
	var exact int64
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+ident).Scan(&exact); err == nil {
		total = exact
	}
	return DbRows{Columns: cols, Rows: recs, TotalRows: total}, nil
}

// RunDbQuery ejecuta SQL del usuario contra SU base de datos. Devuelve error de
// Go solo para fallos de plataforma (proyecto sin Postgres, conexión); un SQL
// mal escrito o cancelado por timeout vuelve como SQLResult.Error para pintarse
// en el editor.
func (s *Service) RunDbQuery(ctx context.Context, projectID uuid.UUID, sql string) (SQLResult, error) {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return SQLResult{}, fmt.Errorf("%w: consulta vacía", domain.ErrInvalidInput)
	}
	if len(sql) > dbStudioMaxSQLLen {
		return SQLResult{}, fmt.Errorf("%w: consulta demasiado larga", domain.ErrInvalidInput)
	}
	target, err := s.resolveDbTarget(ctx, projectID)
	if err != nil {
		return SQLResult{}, err
	}
	conn, err := dbStudioConnect(ctx, target.dsn)
	if err != nil {
		return SQLResult{}, err
	}
	defer conn.Close(context.WithoutCancel(ctx))

	// Auditoría: ejecutar SQL arbitrario sobre los datos del cliente es una
	// acción sensible; queda registrada (sin el texto completo: puede llevar
	// datos personales en un literal).
	s.audit(ctx, "db.query", projectID.String(), map[string]any{"sql_len": len(sql)})

	start := time.Now()
	rows, err := conn.Query(ctx, sql)
	if err != nil {
		return SQLResult{
			Columns: []string{}, Rows: []map[string]any{},
			ElapsedMs: time.Since(start).Milliseconds(), Error: sqlErrorMessage(err),
		}, nil
	}
	cols, recs, cerr := collectRows(rows, dbStudioMaxRows)
	elapsed := time.Since(start).Milliseconds()
	if cerr != nil {
		return SQLResult{
			Columns: []string{}, Rows: []map[string]any{},
			ElapsedMs: elapsed, Error: sqlErrorMessage(cerr),
		}, nil
	}
	res := SQLResult{Columns: cols, Rows: recs, RowCount: len(recs), ElapsedMs: elapsed}
	// Sentencias sin filas (INSERT/UPDATE/DDL): la etiqueta de comando ("UPDATE
	// 3") es la única respuesta útil. Solo está disponible tras cerrar rows.
	if len(cols) == 0 {
		tag := rows.CommandTag()
		res.Notice = tag.String()
		res.RowCount = int(tag.RowsAffected())
	} else if len(recs) == dbStudioMaxRows {
		res.Notice = fmt.Sprintf("Resultado truncado a %d filas.", dbStudioMaxRows)
	}
	return res, nil
}

// DbAPIInfo devuelve el endpoint REST automático y las credenciales SIEMPRE
// enmascaradas.
func (s *Service) DbAPIInfo(ctx context.Context, projectID uuid.UUID) (DbAPIInfo, error) {
	target, err := s.resolveDbTarget(ctx, projectID)
	if err != nil {
		return DbAPIInfo{}, err
	}
	// Las claves anon/service son JWT HS256 firmados con el secreto del proyecto
	// (PGRST_JWT_SECRET). Firmarlas aquí y devolverlas por un GET sería repartir
	// credenciales de acceso total a los datos del cliente desde una pantalla de
	// consulta: se muestra solo la máscara y el dashboard enlaza a Secrets, donde
	// se gestionan de verdad.
	return DbAPIInfo{
		RestURL:           target.restURL,
		AnonKeyPreview:    "JWT HS256 " + dbStudioMask,
		ServiceKeyPreview: "JWT HS256 " + dbStudioMask,
		ConnectionString:  maskDSN(target.dsn),
	}, nil
}

// DbConnectionString devuelve la cadena de conexión COMPLETA, con contraseña.
//
// Hasta ahora la única forma de tocar la base era el editor SQL del panel: la
// cadena se enseñaba enmascarada y la contraseña solo existía dentro del clúster,
// inyectada en la app. Eso deja fuera todo lo demás —psql, un cliente gráfico, un
// `pg_dump` propio, una migración desde el portátil— y convierte en imposible algo
// tan normal como mirar la base con la herramienta de siempre.
//
// Es la MISMA postura que el secreto del webhook: son datos del cliente, no de la
// plataforma, así que se pueden ver; lo que se controla es quién y con qué rastro.
// Por eso va con admin/owner, solo humanos (un token de API no se lleva la
// contraseña de la base) y queda escrito en la auditoría.
func (s *Service) DbConnectionString(ctx context.Context, projectID uuid.UUID) (string, error) {
	target, err := s.resolveDbTarget(ctx, projectID)
	if err != nil {
		return "", err
	}
	s.audit(ctx, "db.credentials.reveal", projectID.String(), map[string]any{"service_id": target.svc.ID})
	return target.dsn, nil
}

// ---------------------------------------------------------------------------
// Ayudantes
// ---------------------------------------------------------------------------

// parseQualifiedTable parte "esquema.tabla" (lo que manda el dashboard). Sin
// punto se asume el esquema público. No sanea: eso lo hacen la comprobación
// contra el catálogo y pgx.Identifier.Sanitize; aquí solo se acota la longitud
// (NAMEDATALEN de Postgres es 63) y se rechaza lo vacío.
func parseQualifiedTable(table string) (string, string, error) {
	table = strings.TrimSpace(table)
	schema, name := dbStudioExposedSchema, table
	if s, n, ok := strings.Cut(table, "."); ok {
		schema, name = s, n
	}
	if schema == "" || name == "" || len(schema) > 63 || len(name) > 63 {
		return "", "", fmt.Errorf("%w: nombre de tabla inválido", domain.ErrInvalidInput)
	}
	return schema, name, nil
}

// collectRows materializa un resultado con los topes de la plataforma: corta en
// maxRows o al superar dbStudioMaxBytes aproximados, y cierra rows SIEMPRE.
//
// ponytail: al cortar antes de tiempo, pgx DRENA el resto del resultado en
// Close() (no lo cancela). El techo lo pone el statement_timeout de la conexión,
// que cubre también la transmisión, así que un `SELECT *` de una tabla enorme se
// corta en 8 s server-side. Si algún día hiciera falta cortar antes: enviar un
// CancelRequest a la conexión en vez de cerrar.
func collectRows(rows pgx.Rows, maxRows int) ([]string, []map[string]any, error) {
	defer rows.Close()
	fds := rows.FieldDescriptions()
	cols := make([]string, 0, len(fds))
	for _, fd := range fds {
		cols = append(cols, fd.Name)
	}
	out := make([]map[string]any, 0, 32)
	size := 0
	for len(out) < maxRows && size < dbStudioMaxBytes && rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, nil, err
		}
		rec := make(map[string]any, len(cols))
		for i, v := range vals {
			if i >= len(cols) {
				break
			}
			rec[cols[i]] = jsonSafeValue(v)
			size += approxValueSize(v)
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return cols, out, nil
}

// jsonSafeValue normaliza los valores que pgx decodifica a formas que el
// navegador entiende: uuid llega como [16]byte y bytea como []byte, y ambos se
// serializarían como listas de números.
func jsonSafeValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case [16]byte:
		return uuid.UUID(t).String()
	case []byte:
		if utf8.Valid(t) {
			return string(t)
		}
		return `\x` + hex.EncodeToString(t)
	default:
		return v
	}
}

// approxValueSize estima el peso de un valor para el tope de respuesta. Es
// deliberadamente barato: solo tiene que impedir respuestas gigantes.
func approxValueSize(v any) int {
	switch t := v.(type) {
	case string:
		return len(t)
	case []byte:
		return len(t)
	case nil:
		return 1
	default:
		return 16
	}
}

// maskDSN devuelve la cadena de conexión con la contraseña sustituida por la
// máscara. Se reconstruye a mano en vez de con url.String() para que los
// bullets no salgan percent-encoded.
func maskDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil || u.Host == "" {
		return dbStudioMask // ante la duda, no se filtra nada
	}
	user := ""
	if u.User != nil {
		user = u.User.Username() + ":" + dbStudioMask + "@"
	}
	out := u.Scheme + "://" + user + u.Host + u.Path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out
}

// sqlErrorMessage traduce a un mensaje para el editor. El texto del error de
// Postgres es del propio cliente sobre su BD (no filtra nada de la plataforma),
// salvo el caso del timeout, que se explica como límite de Sytue porque lo es.
func sqlErrorMessage(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "statement timeout") || strings.Contains(msg, "57014") {
		return fmt.Sprintf("La consulta superó el límite de %s y se canceló.", dbStudioStatementTimeout)
	}
	return msg
}
