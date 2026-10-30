// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package diapositivas es «Presentar con Cast» (Sytue Diapos): un directo puede llevar
// una presentación. Quien presenta avisa de cada cambio de diapositiva y los asistentes
// leen en cuál va; la grabación del directo conserva cada cambio como un capítulo, con
// el instante en que se pasó.
//
// Solo viaja la POSICIÓN: el número y el título de la diapositiva. Ni el contenido de la
// presentación ni las notas del ponente salen de Diapos (el cuerpo de la petición no
// tiene dónde llevarlas y el servicio rechaza campos que no conoce). Quién puede mirar lo
// decide `internal/acceso` por el mismo camino que la reproducción; quién puede avanzar,
// solo el dueño del directo: un administrador puede terminarlo pero no presentar en su
// nombre, como con el token de emisor.
package diapositivas

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/cast/internal/domain"
	"git.sytue.eu/sytue/one/services/cast/internal/service"
)

// Topes: los mismos que los CHECK de la migración 0008.
const (
	MaxDiapositivas = 2000
	MaxMarcas       = 2000
	// MaxTitulo es lo que se guarda del título de una diapositiva (runas): se recorta, no
	// se rechaza, porque sale del contenido de quien presenta y no debe pararle.
	MaxTitulo = 120
	// maxRef es el tope de la referencia al origen.
	maxRef = 200
)

var reRef = regexp.MustCompile(`^[a-z][a-z0-9-]{0,19}:[A-Za-z0-9_-]{1,100}$`)

// Posicion es lo que dice quien presenta: dónde va.
type Posicion struct {
	// Index es la diapositiva (desde 0) y Total cuántas tiene la presentación.
	Index, Total int
	// Title es el título de la diapositiva ("" si no tiene).
	Title string
	// Ref es de dónde sale la presentación, p. ej. `docs:<id>` ("" si no se dice).
	Ref string
}

// Marca es un capítulo de la grabación: la diapositiva a la que se pasó y cuándo.
type Marca struct {
	AtMS  int64
	Index int
	Title string
}

// Estado es la presentación de un directo: dónde va y sus capítulos.
type Estado struct {
	Ref       string
	Index     int
	Total     int
	Title     string
	StartedBy string
	StartedAt time.Time
	UpdatedAt time.Time
	Marks     []Marca
}

// Servicio son los casos de uso de las presentaciones.
type Servicio struct {
	pool   *pg.Pool
	svc    *service.Servicio
	logger *slog.Logger
}

// New construye el servicio sobre el pool y el servicio de vídeos (de él sale el ACL).
func New(pool *pg.Pool, svc *service.Servicio, logger *slog.Logger) *Servicio {
	if logger == nil {
		logger = slog.Default()
	}
	return &Servicio{pool: pool, svc: svc, logger: logger}
}

func (s *Servicio) enTenant(ctx context.Context, tenant string, fn func(tx pgx.Tx) error) error {
	return s.pool.InTx(ctx, func(tx pgx.Tx) error {
		if err := pg.SetTenant(ctx, tx, tenant); err != nil {
			return err
		}
		return fn(tx)
	})
}

// titulo deja el título en una línea, sin controles y de a lo más MaxTitulo runas.
func titulo(raw string) string {
	t := strings.Join(strings.FieldsFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
	if utf8.RuneCountInString(t) > MaxTitulo {
		t = string([]rune(t)[:MaxTitulo])
	}
	return t
}

// Validar comprueba una posición y la deja normalizada.
func (p Posicion) Validar() (Posicion, error) {
	if p.Total < 1 || p.Total > MaxDiapositivas {
		return p, fmt.Errorf("%w: la presentación tiene que tener entre 1 y %d diapositivas", domain.ErrEntradaInvalida, MaxDiapositivas)
	}
	if p.Index < 0 || p.Index >= p.Total {
		return p, fmt.Errorf("%w: la diapositiva %d no existe en una presentación de %d", domain.ErrEntradaInvalida, p.Index, p.Total)
	}
	if p.Ref != "" && (len(p.Ref) > maxRef || !reRef.MatchString(p.Ref)) {
		return p, fmt.Errorf("%w: referencia de origen ilegible", domain.ErrEntradaInvalida)
	}
	p.Title = titulo(p.Title)
	return p, nil
}

// Avanzar apunta la diapositiva en la que va el directo. Solo el dueño, y solo mientras
// el directo está programado o en emisión; la primera llamada abre la presentación (y
// deja constancia en el registro de quién la lanzó). Pasar a la misma diapositiva no
// añade capítulo.
func (s *Servicio) Avanzar(ctx context.Context, quien service.Quien, id uuid.UUID, p Posicion) (Estado, error) {
	p, err := p.Validar()
	if err != nil {
		return Estado{}, err
	}
	if quien.Anonimo || quien.Sub == "" || quien.Tenant == "" {
		return Estado{}, domain.ErrProhibido
	}
	v, _, err := s.svc.Resolver(ctx, quien, id)
	if err != nil {
		return Estado{}, err
	}
	if v.Tenant != quien.Tenant || v.OwnerSub != quien.Sub {
		return Estado{}, domain.ErrProhibido
	}
	if v.Kind != domain.KindLive || (v.Status != domain.StatusScheduled && v.Status != domain.StatusLive) {
		return Estado{}, fmt.Errorf("%w: el directo está %s", domain.ErrConflicto, v.Status)
	}
	var nueva bool
	err = s.enTenant(ctx, v.Tenant, func(tx pgx.Tx) error {
		// El upsert bloquea la fila: dos avisos a la vez se ordenan y los capítulos no chocan.
		var previo int
		if err := tx.QueryRow(ctx, `
			INSERT INTO live_presentations (video_id, tenant, source_ref, total, slide_index, slide_title, started_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (video_id) DO UPDATE
			   SET total = EXCLUDED.total, slide_index = EXCLUDED.slide_index,
			       slide_title = EXCLUDED.slide_title, updated_at = now(),
			       source_ref = CASE WHEN EXCLUDED.source_ref <> '' THEN EXCLUDED.source_ref ELSE live_presentations.source_ref END
			RETURNING (xmax = 0)::int`,
			v.ID, v.Tenant, p.Ref, p.Total, p.Index, p.Title, quien.Sub).Scan(&previo); err != nil {
			return fmt.Errorf("cast: apuntar la diapositiva: %w", err)
		}
		nueva = previo == 1
		var ultima *int
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*), (SELECT idx FROM slide_marks WHERE video_id = $1 AND tenant = $2 ORDER BY seq DESC LIMIT 1) FROM slide_marks WHERE video_id = $1 AND tenant = $2`, v.ID, v.Tenant).Scan(&n, &ultima); err != nil {
			return fmt.Errorf("cast: leer los capítulos: %w", err)
		}
		if n >= MaxMarcas || (ultima != nil && *ultima == p.Index) {
			return nil
		}
		// Los capítulos cuentan desde que el directo empezó; antes de emitir, 0.
		var desdeMS int64
		if v.LiveStartedAt != nil {
			desdeMS = max(0, time.Since(*v.LiveStartedAt).Milliseconds())
		}
		if _, err := tx.Exec(ctx, `INSERT INTO slide_marks (video_id, tenant, seq, at_ms, idx, title) VALUES ($1, $2, $3, $4, $5, $6)`,
			v.ID, v.Tenant, n+1, desdeMS, p.Index, p.Title); err != nil {
			return fmt.Errorf("cast: apuntar el capítulo: %w", err)
		}
		return nil
	})
	if err != nil {
		return Estado{}, err
	}
	if nueva {
		// La huella de la presentación: quién, en qué directo y de dónde. No se registra cada
		// cambio de diapositiva, que está en `slide_marks`.
		s.logger.InfoContext(ctx, "cast: presentación iniciada", "tenant", v.Tenant, "video", v.ID, "by", quien.Sub, "ref", logsafe.Text(p.Ref), "total", p.Total)
	}
	est, _, err := s.leer(ctx, v.Tenant, v.ID)
	return est, err
}

// Ver es la presentación del directo para quien pueda verlo (también sin sesión, si es
// público); false si no lleva ninguna.
func (s *Servicio) Ver(ctx context.Context, quien service.Quien, id uuid.UUID) (Estado, bool, error) {
	v, _, err := s.svc.Resolver(ctx, quien, id)
	if err != nil {
		return Estado{}, false, err
	}
	return s.leer(ctx, v.Tenant, v.ID)
}

func (s *Servicio) leer(ctx context.Context, tenant string, id uuid.UUID) (Estado, bool, error) {
	var e Estado
	hay := false
	err := s.enTenant(ctx, tenant, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT source_ref, slide_index, total, slide_title, started_by, started_at, updated_at
			  FROM live_presentations WHERE video_id = $1 AND tenant = $2`, id, tenant).
			Scan(&e.Ref, &e.Index, &e.Total, &e.Title, &e.StartedBy, &e.StartedAt, &e.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cast: leer la presentación: %w", err)
		}
		hay = true
		rows, err := tx.Query(ctx, `SELECT at_ms, idx, title FROM slide_marks WHERE video_id = $1 AND tenant = $2 ORDER BY seq`, id, tenant)
		if err != nil {
			return fmt.Errorf("cast: leer los capítulos: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m Marca
			if err := rows.Scan(&m.AtMS, &m.Index, &m.Title); err != nil {
				return fmt.Errorf("cast: leer un capítulo: %w", err)
			}
			e.Marks = append(e.Marks, m)
		}
		return rows.Err()
	})
	return e, hay, err
}
