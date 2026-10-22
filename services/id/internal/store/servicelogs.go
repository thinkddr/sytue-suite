// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	plog "git.sytue.eu/sytue/one/platform/pkg/log"
)

// Registro de avisos y errores de los servicios (mig 0049).
//
// La pantalla de plataforma no enseña líneas sueltas: enseña PROBLEMAS. Mil
// veces el mismo error no son mil cosas que mirar, es una sola que lleva pasando
// mil veces, y esa diferencia es lo que separa un panel útil de un volcado que
// nadie abre dos veces. Por eso lo que se consulta agrupa por servicio y mensaje
// y cuenta, en vez de paginar la tabla cruda.

// LogEntry es una línea tal cual se guardó. Se usa para el detalle de un grupo.
type LogEntry struct {
	ID         int64
	OccurredAt time.Time
	Level      string
	Service    string
	Message    string
	Attrs      map[string]string
}

// LogGroup es un problema: el mismo mensaje del mismo servicio, con cuántas
// veces ha pasado y desde cuándo.
type LogGroup struct {
	Level   string
	Service string
	Message string
	Count   int
	First   time.Time
	Last    time.Time
	// Attrs del ejemplar más reciente: sirve para enseñar un caso concreto sin
	// tener que pedir el detalle.
	Sample map[string]string
}

// LogStats son los números de cabecera de la pantalla.
type LogStats struct {
	Errors24h int
	Warns24h  int
	Errors1h  int
	Services  int
	// PorHora son las últimas 24 horas, de la más antigua a la más reciente, con
	// el recuento de errores de cada una. Es lo que dibuja la tendencia: un
	// número suelto no distingue «mil errores ahora» de «mil repartidos ayer».
	PorHora []HourBucket
}

// HourBucket es una hora del histograma.
type HourBucket struct {
	Hour   time.Time
	Errors int
	Warns  int
}

// WriteServiceLogs implementa plog.Sink: guarda un lote de entradas capturadas.
//
// Un solo INSERT con todas las filas: el capturador ya agrupa en lotes de cien
// justamente para que una tormenta de errores no se convierta en cien viajes a
// la base — y una tormenta de errores es exactamente cuando la base peor está.
func (s *Store) WriteServiceLogs(ctx context.Context, batch []plog.Entry) error {
	if len(batch) == 0 {
		return nil
	}
	const sql = `INSERT INTO service_logs (occurred_at, level, service, message, attrs)
		SELECT * FROM unnest($1::timestamptz[], $2::text[], $3::text[], $4::text[], $5::jsonb[])`
	n := len(batch)
	cuando := make([]time.Time, n)
	nivel := make([]string, n)
	servicio := make([]string, n)
	mensaje := make([]string, n)
	attrs := make([][]byte, n)
	for i, e := range batch {
		cuando[i] = e.At
		nivel[i] = e.Level
		servicio[i] = e.Service
		// Un mensaje enorme (un volcado pegado por error) no puede hacer crecer
		// la tabla sin límite ni romper el INSERT del lote entero.
		mensaje[i] = recortar(e.Message, 500)
		b, err := json.Marshal(e.Attrs)
		if err != nil {
			b = []byte(`{}`)
		}
		attrs[i] = b
	}
	_, err := s.DB().Exec(ctx, sql, cuando, nivel, servicio, mensaje, attrs)
	return errf("WriteServiceLogs", err)
}

func recortar(s string, tope int) string {
	if len(s) <= tope {
		return s
	}
	return s[:tope] + "…"
}

// ListLogGroups devuelve los problemas del periodo, el más reciente primero.
//
// `desde` acota la ventana; `nivel` y `servicio` filtran (vacío = todos) y `q`
// busca en el mensaje. El agrupado se hace en SQL y no en Go porque la gracia es
// no traerse las mil líneas para contarlas aquí.
func (s *Store) ListLogGroups(ctx context.Context, desde time.Time, nivel, servicio, q string, limit int) ([]LogGroup, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{desde, limit}
	filtros := ""
	if nivel != "" {
		args = append(args, strings.ToUpper(nivel))
		filtros += " AND level = $" + itoa(len(args))
	}
	if servicio != "" {
		args = append(args, servicio)
		filtros += " AND service = $" + itoa(len(args))
	}
	if term := strings.TrimSpace(q); term != "" {
		args = append(args, "%"+escapeLike(term)+"%")
		filtros += ` AND plegar(message) LIKE plegar($` + itoa(len(args)) + `) ESCAPE '\'`
	}
	sql := `SELECT level, service, message, count(*), min(occurred_at), max(occurred_at),
			(array_agg(attrs ORDER BY occurred_at DESC))[1]
		FROM service_logs
		WHERE occurred_at >= $1` + filtros + `
		GROUP BY level, service, message
		ORDER BY max(occurred_at) DESC
		LIMIT $2`
	rows, err := s.DB().Query(ctx, sql, args...)
	if err != nil {
		return nil, errf("ListLogGroups", err)
	}
	defer rows.Close()
	out := make([]LogGroup, 0, limit)
	for rows.Next() {
		var g LogGroup
		var muestra []byte
		if err := rows.Scan(&g.Level, &g.Service, &g.Message, &g.Count,
			&g.First, &g.Last, &muestra); err != nil {
			return nil, errf("ListLogGroups.scan", err)
		}
		_ = json.Unmarshal(muestra, &g.Sample)
		out = append(out, g)
	}
	return out, errf("ListLogGroups.iter", rows.Err())
}

// LogStatsSince calcula los números de cabecera y el histograma por hora.
//
// Todo en DOS consultas y no en cinco: la pantalla se refresca sola cada pocos
// segundos, y multiplicar viajes a la base por cada tarjeta es como se convierte
// un panel de diagnóstico en parte del problema que diagnostica.
func (s *Store) LogStatsSince(ctx context.Context, ahora time.Time) (LogStats, error) {
	var st LogStats
	const resumen = `SELECT
		count(*) FILTER (WHERE level = 'ERROR' AND occurred_at >= $1::timestamptz - interval '24 hours'),
		count(*) FILTER (WHERE level = 'WARN'  AND occurred_at >= $1::timestamptz - interval '24 hours'),
		count(*) FILTER (WHERE level = 'ERROR' AND occurred_at >= $1::timestamptz - interval '1 hour'),
		count(DISTINCT service) FILTER (WHERE level = 'ERROR' AND occurred_at >= $1::timestamptz - interval '24 hours')
		FROM service_logs WHERE occurred_at >= $1::timestamptz - interval '24 hours'`
	if err := s.DB().QueryRow(ctx, resumen, ahora).
		Scan(&st.Errors24h, &st.Warns24h, &st.Errors1h, &st.Services); err != nil {
		return st, errf("LogStatsSince", err)
	}

	// generate_series y no un GROUP BY a secas: una hora SIN errores tiene que
	// aparecer como cero. Si solo salen las horas con datos, el gráfico junta dos
	// picos separados por un día de calma y parece una crisis continua.
	const serie = `SELECT h, coalesce(e.errores, 0), coalesce(e.avisos, 0)
		FROM generate_series(date_trunc('hour', $1::timestamptz) - interval '23 hours',
		                     date_trunc('hour', $1::timestamptz), interval '1 hour') AS h
		LEFT JOIN (
			SELECT date_trunc('hour', occurred_at) AS hora,
			       count(*) FILTER (WHERE level = 'ERROR') AS errores,
			       count(*) FILTER (WHERE level = 'WARN')  AS avisos
			FROM service_logs WHERE occurred_at >= $1::timestamptz - interval '24 hours'
			GROUP BY 1
		) e ON e.hora = h
		ORDER BY h`
	rows, err := s.DB().Query(ctx, serie, ahora)
	if err != nil {
		return st, errf("LogStatsSince.serie", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b HourBucket
		if err := rows.Scan(&b.Hour, &b.Errors, &b.Warns); err != nil {
			return st, errf("LogStatsSince.scan", err)
		}
		st.PorHora = append(st.PorHora, b)
	}
	return st, errf("LogStatsSince.iter", rows.Err())
}

// PurgeServiceLogs borra lo más viejo que `antes`. Lo llama la purga periódica
// del servicio: este registro es diagnóstico reciente, no un archivo histórico,
// y sin poda crecería sin límite igual que crecían las sesiones antes de la
// suya.
func (s *Store) PurgeServiceLogs(ctx context.Context, antes time.Time) (int64, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM service_logs WHERE occurred_at < $1`, antes)
	if err != nil {
		return 0, errf("PurgeServiceLogs", err)
	}
	return tag.RowsAffected(), nil
}
