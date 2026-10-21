// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package connectors

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// executePostgres conecta a un PostgreSQL EXTERNO y ejecuta un ÚNICO SELECT
// parametrizado en una transacción de SOLO-LECTURA con statement_timeout y tope
// de filas. Defensa en capas contra abuso:
//   - guardSelect: rechaza cualquier cosa que no sea un único SELECT/WITH.
//   - params posicionales ($1..$n): la inyección SQL es imposible (nunca se
//     concatena input al texto SQL).
//   - protocolo extendido de pgx (Parse/Bind/Execute): impide sentencias apiladas.
//   - transacción READ ONLY + default_transaction_read_only=on: cualquier DML/DDL
//     (incluido un CTE que escriba) se rechaza en tiempo de ejecución.
//   - statement_timeout + tope de filas + timeout de contexto: acotan trabajo.
//   - barrera SSRF (ssrfhttp): el host se resuelve y valida (IP pública) antes y
//     durante el dial; no se puede apuntar a un PostgreSQL/host interno.
func executePostgres(ctx context.Context, cfg PostgresConfig, password string, q PostgresQuery, lim Limits) (*Result, error) {
	if err := guardSelect(q.SQL); err != nil { // re-guard defensivo (config de confianza, coste nulo)
		return nil, err
	}
	// Barrera 1: resuelve el host y exige IP pública antes de intentar conectar.
	if err := ssrfhttp.ValidateHost(ctx, cfg.Host); err != nil {
		return nil, err
	}

	connCfg, err := buildConnConfig(cfg, password, lim.Timeout)
	if err != nil {
		return nil, err
	}

	conn, err := pgx.ConnectConfig(ctx, connCfg)
	if err != nil {
		return nil, fmt.Errorf("connectors: conexión PostgreSQL: %w", err)
	}
	// Cierre con contexto propio: el ctx de la petición puede estar ya cancelado.
	//nolint:contextcheck // deliberado (ver comentario): cerrar con el ctx cancelado dejaría la conexión sin liberar.
	defer func() { _ = conn.Close(context.Background()) }()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("connectors: iniciar transacción read-only: %w", err)
	}
	//nolint:contextcheck // deliberado: el rollback debe correr aunque el ctx de la petición esté cancelado.
	defer func() { _ = tx.Rollback(context.Background()) }() // solo-lectura: siempre rollback

	rows, err := tx.Query(ctx, q.SQL, q.Params...)
	if err != nil {
		return nil, fmt.Errorf("connectors: ejecutar consulta: %w", err)
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	if len(fds) > maxColumns {
		return nil, invalid("demasiadas columnas (máx. %d)", maxColumns)
	}
	cols := make([]Column, len(fds))
	for i, fd := range fds {
		cols[i] = Column{Name: fd.Name, Type: oidType(fd.DataTypeOID)}
	}

	out := make([][]any, 0, 128)
	for rows.Next() {
		if len(out) >= lim.MaxRows {
			break // tope de filas: dejamos de leer (cierra el portal)
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("connectors: leer valores: %w", err)
		}
		row := make([]any, len(vals))
		for i, v := range vals {
			row[i] = normalizePG(v)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("connectors: leer filas: %w", err)
	}
	return &Result{Columns: cols, Rows: out}, nil
}

// buildConnConfig construye el *pgx.ConnConfig hacia el PG externo. El DSN se
// arma con net/url (usuario/contraseña escapados por url.UserPassword → sin
// inyección en el DSN). El dialer SSRF-safe y los límites de sesión se fijan
// sobre el config, no en el DSN.
func buildConnConfig(cfg PostgresConfig, password string, timeout time.Duration) (*pgx.ConnConfig, error) {
	if cfg.Port == 0 {
		cfg.Port = 5432
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "prefer"
	}
	connectSecs := int(timeout / time.Second)
	if connectSecs < 1 {
		connectSecs = 1
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, password),
		Host:   net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		Path:   "/" + cfg.Database,
	}
	qv := url.Values{}
	qv.Set("sslmode", cfg.SSLMode)
	qv.Set("connect_timeout", strconv.Itoa(connectSecs))
	u.RawQuery = qv.Encode()

	connCfg, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, fmt.Errorf("connectors: DSN inválido: %w", err)
	}
	// Barrera 2 SSRF: el dialer re-resuelve y valida la IP en el connect.
	connCfg.DialFunc = ssrfhttp.SafeDialContext

	stmtMs := int(timeout / time.Millisecond)
	if stmtMs < 1000 {
		stmtMs = 1000
	}
	if connCfg.RuntimeParams == nil {
		connCfg.RuntimeParams = map[string]string{}
	}
	connCfg.RuntimeParams["statement_timeout"] = strconv.Itoa(stmtMs)
	connCfg.RuntimeParams["default_transaction_read_only"] = "on"
	connCfg.RuntimeParams["application_name"] = "sytue-docs-connector"
	return connCfg, nil
}

// -----------------------------------------------------------------------------
// Guard de SELECT único (solo-lectura).
// -----------------------------------------------------------------------------

// guardSelect rechaza todo lo que no sea un ÚNICO SELECT (o WITH ... SELECT) de
// solo lectura: permite UN ';' final opcional, rechaza ';' de nivel superior
// (sentencias apiladas) y exige que la primera palabra clave sea SELECT o WITH.
// El escaneo ignora ';' dentro de literales de comilla simple, identificadores
// entre comillas dobles y comentarios (línea y bloque anidado). Es la primera
// barrera; la transacción READ ONLY es el backstop en ejecución (un WITH que
// intente escribir se rechaza allí).
//
// ponytail: no interpreta dollar-quoting ($tag$…$tag$); un ';' dentro de un
// literal así se contaría y la consulta se rechazaría (dirección segura). Usar
// comillas estándar. Ampliar si aparece un caso real.
func guardSelect(sql string) error {
	s := strings.TrimSpace(sql)
	s = strings.TrimSuffix(s, ";") // un ';' final es admisible
	s = strings.TrimSpace(s)
	if s == "" {
		return invalid("SQL vacío")
	}
	kw, semicolons := analyzeSQL(s)
	if semicolons > 0 {
		return invalid("solo se permite una sentencia (sin ';' múltiples)")
	}
	if kw != "SELECT" && kw != "WITH" {
		return invalid("solo se permiten consultas SELECT de solo lectura")
	}
	return nil
}

// analyzeSQL recorre el SQL una vez y devuelve la primera palabra clave (en
// mayúsculas) y el número de ';' de nivel superior (fuera de cadenas/comentarios).
//
//nolint:gocognit,gocyclo // máquina de estados de UNA pasada sobre el SQL (cadenas, comentarios, $$...$$): trocearla parte el estado.
func analyzeSQL(sql string) (firstKeyword string, topSemicolons int) {
	const (
		stNormal = iota
		stLine   // -- comentario
		stBlock  // /* comentario */ (anidable)
		stSquote // 'cadena'
		stDquote // "identificador"
	)
	state := stNormal
	blockDepth := 0
	gotKeyword := false
	var kw strings.Builder

	rs := []rune(sql)
	finalize := func() {
		if !gotKeyword && kw.Len() > 0 {
			firstKeyword = kw.String()
			gotKeyword = true
		}
	}
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		var next rune
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		switch state {
		case stNormal:
			switch {
			case c == '-' && next == '-':
				finalize()
				state = stLine
				i++
			case c == '/' && next == '*':
				finalize()
				state = stBlock
				blockDepth = 1
				i++
			case c == '\'':
				finalize()
				state = stSquote
			case c == '"':
				finalize()
				state = stDquote
			case c == ';':
				finalize()
				topSemicolons++
			case isAlpha(c):
				if !gotKeyword {
					kw.WriteRune(toUpperASCII(c))
				}
			default:
				finalize()
			}
		case stLine:
			if c == '\n' {
				state = stNormal
			}
		case stBlock:
			if c == '/' && next == '*' {
				blockDepth++
				i++
			} else if c == '*' && next == '/' {
				blockDepth--
				i++
				if blockDepth == 0 {
					state = stNormal
				}
			}
		case stSquote:
			if c == '\'' {
				if next == '\'' { // '' escape
					i++
				} else {
					state = stNormal
				}
			}
		case stDquote:
			if c == '"' {
				if next == '"' { // "" escape
					i++
				} else {
					state = stNormal
				}
			}
		}
	}
	finalize()
	return firstKeyword, topSemicolons
}

func isAlpha(c rune) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func toUpperASCII(c rune) rune {
	if c >= 'a' && c <= 'z' {
		return c - 32
	}
	return c
}

// -----------------------------------------------------------------------------
// Mapeo de tipos y normalización de valores.
// -----------------------------------------------------------------------------

// oidType asigna un tipo lógico legible a partir del OID de PostgreSQL.
func oidType(oid uint32) string {
	switch oid {
	case pgtype.Int2OID, pgtype.Int4OID, pgtype.Int8OID,
		pgtype.Float4OID, pgtype.Float8OID, pgtype.NumericOID:
		return "number"
	case pgtype.BoolOID:
		return "boolean"
	case pgtype.TimestampOID, pgtype.TimestamptzOID:
		return "timestamp"
	case pgtype.DateOID:
		return "date"
	case pgtype.TimeOID:
		return "time"
	case pgtype.JSONOID, pgtype.JSONBOID:
		return "json"
	case pgtype.UUIDOID:
		return "uuid"
	case pgtype.ByteaOID:
		return "bytes"
	default:
		return "text"
	}
}

// normalizePG convierte un valor de pgx a un valor JSON-serializable: escalares
// tal cual, tiempos a RFC3339, bytea a texto/base64, y compuestos (numeric,
// arrays, json) a su JSON crudo.
func normalizePG(v any) any {
	switch x := v.(type) {
	case nil, bool, string, int16, int32, int64, float32, float64:
		return x
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case []byte:
		if utf8.Valid(x) {
			return string(x)
		}
		return base64.StdEncoding.EncodeToString(x)
	default:
		if b, err := json.Marshal(x); err == nil {
			return json.RawMessage(b)
		}
		return fmt.Sprintf("%v", x)
	}
}
