// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"bufio"
	"context"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/cloud/internal/domain"
)

// maxRuntimeLogChars acota UNA línea guardada. Lo que escribe el contenedor lo
// decide CÓDIGO DEL CLIENTE: sin tope, un volcado de un JSON de 50 MB sin saltos
// de línea entraría entero en una fila. Se trunca (se ve fea pero se ve) en vez
// de descartar la línea o dejar crecer la tabla sin límite.
const maxRuntimeLogChars = 8 * 1024

// RuntimeLogFilter acota una búsqueda de logs. Todos los campos son opcionales;
// el proyecto y la organización NO van aquí (los fija el llamante autenticado).
type RuntimeLogFilter struct {
	// Query casa por SUBCADENA case-insensitive, que es como se busca en un log
	// («timeout», «/api/v1/pay», un id de petición) y lo mismo que ya hace el
	// filtro de los logs de build (service.filterLogLines).
	Query string
	Since *time.Time // ts >= Since
	Until *time.Time // ts <  Until
}

// RuntimeLogPage es una página de logs con el cursor para pedir la siguiente.
type RuntimeLogPage struct {
	Lines []domain.LogLine
	// Next es el cursor de continuación; nil si no quedan más resultados.
	Next *PageCursor
}

// AppendRuntimeLogs guarda un lote de líneas de un deployment y adelanta su
// corte de recolección. Cross-tenant: lo llama el recolector de fondo, que pasa
// el tenant de la fila reclamada y opera bajo RLS con él fijado (pg.SetTenant),
// nunca con BYPASSRLS.
//
// Todo el lote entra en UNA sentencia (unnest de dos arrays): un INSERT por línea
// haría un round-trip por línea de log, que es el orden de magnitud equivocado.
//
// El corte se adelanta al ts de la ÚLTIMA línea guardada, no al reloj: la lectura
// del runtime devuelve líneas escritas después de reclamar el arriendo, y si el
// corte se quedara en el instante de la reclamación esas líneas volverían a
// leerse —y a guardarse— en la pasada siguiente. GREATEST evita retroceder si el
// lote llega desordenado o tarde. Todo en la MISMA transacción que el INSERT: si
// este falla, el corte no avanza y las líneas se reintentan enteras.
func (s *Store) AppendRuntimeLogs(ctx context.Context, tenant string, projectID, deploymentID uuid.UUID, lines []domain.LogLine) (int64, error) {
	if len(lines) == 0 {
		return 0, nil
	}
	ts := make([]time.Time, 0, len(lines))
	text := make([]string, 0, len(lines))
	maxTS := lines[0].TS
	for _, l := range lines {
		ts = append(ts, l.TS)
		text = append(text, sanitizeLogText(l.Text))
		if l.TS.After(maxTS) {
			maxTS = l.TS
		}
	}
	var n int64
	err := s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO runtime_logs (tenant, project_id, deployment_id, ts, line)
			SELECT $1, $2, $3, u.ts, u.line
			FROM unnest($4::timestamptz[], $5::text[]) AS u(ts, line)`,
			tenant, projectID, deploymentID, ts, text)
		if err != nil {
			return fmtErr("guardar logs de runtime", err)
		}
		n = tag.RowsAffected()
		if _, err := tx.Exec(ctx, `
			UPDATE deployments SET logs_collected_at = GREATEST(logs_collected_at, $3)
			WHERE tenant = $1 AND id = $2`,
			tenant, deploymentID, maxTS); err != nil {
			return fmtErr("avanzar corte de logs", err)
		}
		return nil
	})
	return n, err
}

// sanitizeLogText deja la línea en algo que PostgreSQL pueda ALMACENAR. No es
// cosmética: `text` no admite el byte NUL ni secuencias UTF-8 inválidas, y la
// salida de un contenedor es binario arbitrario (un `cat` de un fichero, una
// traza con bytes crudos). Sin esto, UNA línea envenenada abortaría el INSERT del
// lote entero y se perderían todos los logs de esa pasada.
//
// El truncado va ANTES de validar el UTF-8 a propósito: cortar por bytes puede
// partir un carácter multibyte, y ToValidUTF8 se lleva ese resto roto.
func sanitizeLogText(s string) string {
	if len(s) > maxRuntimeLogChars {
		s = s[:maxRuntimeLogChars]
	}
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	return strings.ToValidUTF8(s, "")
}

const runtimeLogSelect = `SELECT id, deployment_id, ts, line FROM runtime_logs`

// runtimeLogWhere construye el WHERE parametrizado (tenant + proyecto + filtros +
// keyset) y sus args. El predicado de tenant es EXPLÍCITO además de RLS (defensa
// en profundidad, igual que el resto de listados del store).
func runtimeLogWhere(tenant string, projectID uuid.UUID, f RuntimeLogFilter, after PageCursor) (string, []any) {
	args := []any{}
	ph := func(v any) string {
		args = append(args, v)
		return "$" + strconv.Itoa(len(args))
	}
	var b strings.Builder
	b.WriteString(` WHERE tenant = ` + ph(tenant) + ` AND project_id = ` + ph(projectID))
	if f.Query != "" {
		// escapeLike (ver audit.go): sin él, buscar «100%» devolvería la tabla.
		b.WriteString(` AND plegar(line) LIKE plegar(` + ph("%"+escapeLike(f.Query)+"%") + `) ESCAPE '\'`)
	}
	if f.Since != nil {
		b.WriteString(` AND ts >= ` + ph(*f.Since))
	}
	if f.Until != nil {
		b.WriteString(` AND ts < ` + ph(*f.Until))
	}
	if !after.IsZero() {
		// Keyset, NUNCA OFFSET: el predicado compuesto recorre
		// runtime_logs_search_idx y su coste no crece con la profundidad.
		b.WriteString(` AND (ts, id) < (` + ph(after.CreatedAt) + `, ` + ph(after.ID) + `)`)
	}
	return b.String(), args
}

// SearchRuntimeLogs devuelve una página de logs del proyecto (más recientes
// primero) filtrada por texto y rango de tiempo, con paginación keyset por
// (ts, id) DESC. Pide limit+1 para saber si hay página siguiente sin un COUNT.
//
// ponytail: el texto se busca con plegar() (sin acentos ni mayúsculas), sin índice de texto. Lo que acota el
// coste es el rango de tiempo + la retención de 14 días (el índice sirve tenant+
// proyecto+ts y el filtro se aplica sobre esa ventana). Si un tenant con volumen
// necesita buscar sobre la ventana entera, el salto es un GIN pg_trgm sobre
// `line` —como ya hacen mail/drive/docs— a cambio de encarecer cada INSERT.
func (s *Store) SearchRuntimeLogs(ctx context.Context, projectID uuid.UUID, f RuntimeLogFilter, after PageCursor, limit int) (RuntimeLogPage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var page RuntimeLogPage
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		where, args := runtimeLogWhere(t.Slug, projectID, f, after)
		args = append(args, limit+1)
		rows, err := tx.Query(ctx,
			runtimeLogSelect+where+` ORDER BY ts DESC, id DESC LIMIT $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return fmtErr("buscar logs de runtime", err)
		}
		defer rows.Close()
		for rows.Next() {
			var l domain.LogLine
			if err := rows.Scan(&l.ID, &l.DeploymentID, &l.TS, &l.Text); err != nil {
				return err
			}
			page.Lines = append(page.Lines, l)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(page.Lines) > limit {
			last := page.Lines[limit-1]
			page.Lines = page.Lines[:limit]
			page.Next = &PageCursor{CreatedAt: last.TS, ID: last.ID}
		}
		return nil
	})
	if err != nil {
		return RuntimeLogPage{}, err
	}
	return page, nil
}

// runtimeLogExportCap acota la descarga: tope duro de líneas por fichero, por el
// mismo motivo que el del CSV de auditoría —una petición no puede convertirse en
// un volcado ilimitado que ate una conexión y la memoria del proceso—.
const runtimeLogExportCap = 200_000

// ExportRuntimeLogs escribe el conjunto filtrado como texto plano (una línea por
// línea de log, con su instante delante), en orden CRONOLÓGICO.
//
// Cronológico y no al revés: la búsqueda de la pantalla enseña lo más reciente
// primero porque se está mirando qué acaba de pasar, pero un fichero descargado
// se lee de arriba abajo —y es lo que espera cualquier herramienta a la que se
// le eche encima—.
func (s *Store) ExportRuntimeLogs(ctx context.Context, projectID uuid.UUID, f RuntimeLogFilter, w io.Writer) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		where, args := runtimeLogWhere(t.Slug, projectID, f, PageCursor{})
		args = append(args, runtimeLogExportCap)
		rows, err := tx.Query(ctx,
			runtimeLogSelect+where+` ORDER BY ts ASC, id ASC LIMIT $`+strconv.Itoa(len(args)), args...)
		if err != nil {
			return fmtErr("exportar logs de runtime", err)
		}
		defer rows.Close()
		bw := bufio.NewWriter(w)
		for rows.Next() {
			var id, depID uuid.UUID
			var ts time.Time
			var line string
			if err := rows.Scan(&id, &depID, &ts, &line); err != nil {
				return fmtErr("leer línea de log", err)
			}
			if _, err := bw.WriteString(ts.UTC().Format(time.RFC3339Nano) + " " + line + "\n"); err != nil {
				return err
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		return bw.Flush()
	})
}

// LogCollectTarget es un deployment reclamado por el recolector de logs.
type LogCollectTarget struct {
	ID        uuid.UUID
	Tenant    string
	ProjectID uuid.UUID
	// Since es el valor ANTERIOR de logs_collected_at: el corte desde el que hay
	// que pedirle logs al runtime. Cero = nunca recolectado (primera vez).
	Since time.Time
}

// ClaimLogCollection reclama hasta `limit` deployments vivos a los que toca
// recolectar logs (los que llevan más tiempo sin reclamar primero) y los
// ARRIENDA adelantando logs_collect_claimed_at a `now`, de forma que otra
// réplica (o el siguiente tick) no vuelva a tomarlos hasta pasada `due`.
//
// FOR UPDATE SKIP LOCKED reparte el trabajo entre réplicas sin duplicarlo, igual
// que el prober de uptime. Cross-tenant acotado: transacción de mantenimiento
// (pg.InMaintenanceTx), jamás un rol con BYPASSRLS.
//
// El RETURNING lee logs_collected_at de la SUBCONSULTA (alias s), no de la fila
// actualizada: hace falta el valor VIEJO, que es justo el corte de la lectura.
func (s *Store) ClaimLogCollection(ctx context.Context, now time.Time, due time.Duration, limit int) ([]LogCollectTarget, error) {
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var out []LogCollectTarget
	err := s.pool.InMaintenanceTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE deployments d SET logs_collect_claimed_at = $1
			FROM (
			    SELECT id, logs_collected_at FROM deployments
			    WHERE status = 'ready'
			      AND (logs_collect_claimed_at IS NULL OR logs_collect_claimed_at < $2)
			    ORDER BY logs_collect_claimed_at NULLS FIRST
			    FOR UPDATE SKIP LOCKED
			    LIMIT $3
			) s
			WHERE d.id = s.id
			RETURNING d.id, d.tenant, d.project_id, s.logs_collected_at`,
			now, now.Add(-due), limit)
		if err != nil {
			return fmtErr("reclamar recolección de logs", err)
		}
		defer rows.Close()
		for rows.Next() {
			var tgt LogCollectTarget
			var since *time.Time
			if err := rows.Scan(&tgt.ID, &tgt.Tenant, &tgt.ProjectID, &since); err != nil {
				return err
			}
			if since != nil {
				tgt.Since = *since
			}
			out = append(out, tgt)
		}
		return rows.Err()
	})
	return out, err
}
