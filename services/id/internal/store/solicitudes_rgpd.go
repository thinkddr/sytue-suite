// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// --- Registro de solicitudes RGPD de una organización (mig 0130) -------------
//
// La organización es la responsable del tratamiento: es a ella a quien una
// persona pide acceso, rectificación o supresión, y quien tiene que poder
// demostrar que contestó en plazo (art. 12.3) y cómo (art. 5.2). Este registro
// es esa prueba. No guarda lo que se entregó ni lo que se borró: solo qué se
// pidió, quién lo atendió, cuándo y con qué resultado.

// Tipos de solicitud (arts. 15 a 21).
const (
	SolicitudAcceso         = "access"
	SolicitudRectificacion  = "rectification"
	SolicitudSupresion      = "erasure"
	SolicitudLimitacion     = "restriction"
	SolicitudPortabilidad   = "portability"
	SolicitudOposicion      = "objection"
	EstadoSolicitudRecibida = "received"
	EstadoSolicitudEnCurso  = "in_progress"
	EstadoSolicitudResuelta = "completed"
	EstadoSolicitudDenegada = "rejected"
)

// TiposSolicitudRGPD y CanalesSolicitudRGPD son los valores que admite la base.
var (
	TiposSolicitudRGPD   = []string{SolicitudAcceso, SolicitudRectificacion, SolicitudSupresion, SolicitudLimitacion, SolicitudPortabilidad, SolicitudOposicion}
	CanalesSolicitudRGPD = []string{"email", "web", "letter", "phone", "in_person", "other"}
)

// PlazoRespuestaRGPD es el mes del art. 12.3; ProrrogaRGPD, los dos meses más que
// admite cuando la solicitud es compleja o son muchas.
const (
	PlazoRespuestaMesesRGPD = 1
	ProrrogaMesesRGPD       = 2
	// RetencionSolicitudesRGPD es cuánto se guarda una solicitud cerrada: los tres
	// años en que prescriben las infracciones muy graves (art. 72 LOPDGDD), que es
	// el tiempo durante el que puede hacer falta demostrar que se atendió.
	RetencionSolicitudesRGPD = 3 * 365 * 24 * time.Hour
)

// Errores del registro.
var (
	// ErrSolicitudCerrada: una solicitud resuelta o denegada ya no cambia.
	ErrSolicitudCerrada = errors.New("la solicitud ya está cerrada")
	// ErrProrrogaNoAdmitida: ya se prorrogó, o el plazo ya venció (la prórroga se
	// comunica dentro del primer mes, art. 12.3).
	ErrProrrogaNoAdmitida = errors.New("la solicitud no admite prórroga")
	// ErrSupresionNoAdmitida: la solicitud no es de supresión de una cuenta de la
	// organización, o ya se ejecutó.
	ErrSupresionNoAdmitida = errors.New("la solicitud no admite supresión de cuenta")
	// ErrCuentaDeFuera: la cuenta se usa fuera de la organización (otra
	// organización, o su cuenta personal). Esa parte no es suya: la persona la
	// borra ella misma o se lo pide a quien corresponda.
	ErrCuentaDeFuera = errors.New("la cuenta se usa fuera de esta organización")
)

// SolicitudRGPD es una fila del registro.
type SolicitudRGPD struct {
	ID                 string     `json:"id"`
	OrgID              string     `json:"-"`
	SubjectUserID      string     `json:"subject_user_id,omitempty"`
	SubjectEmail       string     `json:"subject_email"`
	SubjectName        string     `json:"subject_name"`
	Kind               string     `json:"kind"`
	Channel            string     `json:"channel"`
	ReceivedAt         time.Time  `json:"received_at"`
	DueAt              time.Time  `json:"due_at"`
	Extended           bool       `json:"extended"`
	ExtensionReason    string     `json:"extension_reason,omitempty"`
	IdentityVerified   bool       `json:"identity_verified"`
	IdentityMethod     string     `json:"identity_method,omitempty"`
	Status             string     `json:"status"`
	Resolution         string     `json:"resolution,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	ErasureStartedAt   *time.Time `json:"erasure_started_at,omitempty"`
	ErasureCompletedAt *time.Time `json:"erasure_completed_at,omitempty"`
	CreatedBy          string     `json:"created_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ClosedAt           *time.Time `json:"closed_at,omitempty"`
}

// EventoSolicitudRGPD es una línea de la historia de una solicitud.
type EventoSolicitudRGPD struct {
	ID      int64           `json:"id"`
	At      time.Time       `json:"at"`
	ActorID string          `json:"actor_id,omitempty"`
	Action  string          `json:"action"`
	Detail  json.RawMessage `json:"detail,omitempty"`
}

const columnasSolicitudRGPD = `id::text, org_id::text, coalesce(subject_user_id::text, ''), subject_email, subject_name,
	kind, channel, received_at, due_at, extended, extension_reason, identity_verified, identity_method,
	status, resolution, notes, erasure_started_at, erasure_completed_at, coalesce(created_by::text, ''),
	created_at, updated_at, closed_at`

func escanearSolicitud(r escaneable) (SolicitudRGPD, error) {
	var s SolicitudRGPD
	err := r.Scan(&s.ID, &s.OrgID, &s.SubjectUserID, &s.SubjectEmail, &s.SubjectName,
		&s.Kind, &s.Channel, &s.ReceivedAt, &s.DueAt, &s.Extended, &s.ExtensionReason, &s.IdentityVerified, &s.IdentityMethod,
		&s.Status, &s.Resolution, &s.Notes, &s.ErasureStartedAt, &s.ErasureCompletedAt, &s.CreatedBy,
		&s.CreatedAt, &s.UpdatedAt, &s.ClosedAt)
	return s, err
}

// CrearSolicitudRGPD registra una solicitud y su primer evento en la misma
// transacción. El plazo se calcula desde la recepción, no desde el registro: es
// la fecha que cuenta para el art. 12.3. SubjectUserID, si viene, tiene que ser
// de la organización (lo comprueba quien llama).
func (s *Store) CrearSolicitudRGPD(ctx context.Context, in SolicitudRGPD, actorID string) (SolicitudRGPD, error) {
	var out SolicitudRGPD
	err := s.InTx(ctx, func(q pg.Querier) error {
		row := q.QueryRow(ctx, `
			INSERT INTO dsr_requests (org_id, subject_user_id, subject_email, subject_name, kind, channel,
			    received_at, due_at, identity_verified, identity_method, notes, created_by)
			VALUES ($1, $2::uuid, $3, $4, $5, $6, $7::timestamptz, $7::timestamptz + make_interval(months => $8::int),
			        $9, $10, $11, $12::uuid)
			RETURNING `+columnasSolicitudRGPD,
			in.OrgID, nilSiVacio(in.SubjectUserID), strings.ToLower(strings.TrimSpace(in.SubjectEmail)), in.SubjectName,
			in.Kind, in.Channel, in.ReceivedAt, PlazoRespuestaMesesRGPD, in.IdentityVerified, in.IdentityMethod,
			in.Notes, nilSiVacio(actorID))
		var err error
		if out, err = escanearSolicitud(row); err != nil {
			return errf("CrearSolicitudRGPD", err)
		}
		return eventoSolicitud(ctx, q, out.ID, actorID, "created", map[string]any{"kind": out.Kind, "channel": out.Channel})
	})
	return out, err
}

// FiltroSolicitudesRGPD acota el listado.
type FiltroSolicitudesRGPD struct {
	Estado   string // "" = todos; "open" = las no cerradas
	Tipo     string
	Limite   int
	Vencidas bool // solo las abiertas con el plazo vencido
}

// ListarSolicitudesRGPD devuelve el registro de la organización, lo más reciente
// primero. Tope de 500: un registro con más solicitudes vivas pide un filtro.
func (s *Store) ListarSolicitudesRGPD(ctx context.Context, orgID string, f FiltroSolicitudesRGPD) ([]SolicitudRGPD, error) {
	if f.Limite <= 0 || f.Limite > 500 {
		f.Limite = 200
	}
	rows, err := s.DB().Query(ctx, `SELECT `+columnasSolicitudRGPD+` FROM dsr_requests
		WHERE org_id = $1
		  AND ($2 = '' OR ($2 = 'open' AND closed_at IS NULL) OR status = $2)
		  AND ($3 = '' OR kind = $3)
		  AND (NOT $4 OR (closed_at IS NULL AND due_at < now()))
		ORDER BY received_at DESC, id DESC LIMIT $5`, orgID, f.Estado, f.Tipo, f.Vencidas, f.Limite)
	if err != nil {
		return nil, errf("ListarSolicitudesRGPD", err)
	}
	defer rows.Close()
	out := make([]SolicitudRGPD, 0, 16)
	for rows.Next() {
		sol, err := escanearSolicitud(rows)
		if err != nil {
			return nil, errf("ListarSolicitudesRGPD.scan", err)
		}
		out = append(out, sol)
	}
	return out, errf("ListarSolicitudesRGPD.rows", rows.Err())
}

// SolicitudRGPDConHistoria devuelve una solicitud de la organización con su
// historia. domain.ErrNotFound si no es de ella (sin fuga entre organizaciones).
func (s *Store) SolicitudRGPDConHistoria(ctx context.Context, orgID, id string) (SolicitudRGPD, []EventoSolicitudRGPD, error) {
	sol, err := escanearSolicitud(s.DB().QueryRow(ctx, `SELECT `+columnasSolicitudRGPD+` FROM dsr_requests
		WHERE id = $1::uuid AND org_id = $2`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return sol, nil, domain.ErrNotFound
	}
	if err != nil {
		return sol, nil, errf("SolicitudRGPDConHistoria", err)
	}
	rows, err := s.DB().Query(ctx, `SELECT id, at, coalesce(actor_id::text, ''), action, detail
		FROM dsr_request_events WHERE request_id = $1 ORDER BY id`, id)
	if err != nil {
		return sol, nil, errf("SolicitudRGPDConHistoria.events", err)
	}
	defer rows.Close()
	var eventos []EventoSolicitudRGPD
	for rows.Next() {
		var e EventoSolicitudRGPD
		var detail []byte
		if err := rows.Scan(&e.ID, &e.At, &e.ActorID, &e.Action, &detail); err != nil {
			return sol, nil, errf("SolicitudRGPDConHistoria.scan", err)
		}
		e.Detail = detail
		eventos = append(eventos, e)
	}
	return sol, eventos, errf("SolicitudRGPDConHistoria.rows", rows.Err())
}

// CambiosSolicitudRGPD son los campos que se pueden tocar de una solicitud
// abierta. nil = no se toca.
type CambiosSolicitudRGPD struct {
	Status           *string
	Resolution       *string
	Notes            *string
	IdentityVerified *bool
	IdentityMethod   *string
}

// ActualizarSolicitudRGPD aplica cambios a una solicitud ABIERTA y deja un evento
// con lo que cambió (sin el texto, que ya está en la fila). Cerrarla
// (completed/rejected) exige una resolución escrita: es lo que se le contesta a
// la persona. Una cerrada ya no cambia.
func (s *Store) ActualizarSolicitudRGPD(ctx context.Context, orgID, id string, c CambiosSolicitudRGPD, actorID string) (SolicitudRGPD, error) {
	var out SolicitudRGPD
	err := s.InTx(ctx, func(q pg.Querier) error {
		actual, err := solicitudParaCambiar(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		nueva := actual
		cambios := map[string]any{}
		if c.Status != nil && *c.Status != actual.Status {
			nueva.Status = *c.Status
			cambios["status"] = *c.Status
		}
		if c.Resolution != nil && *c.Resolution != actual.Resolution {
			nueva.Resolution = *c.Resolution
			cambios["resolution"] = true
		}
		if c.Notes != nil && *c.Notes != actual.Notes {
			nueva.Notes = *c.Notes
			cambios["notes"] = true
		}
		if c.IdentityVerified != nil && *c.IdentityVerified != actual.IdentityVerified {
			nueva.IdentityVerified = *c.IdentityVerified
			cambios["identity_verified"] = *c.IdentityVerified
		}
		if c.IdentityMethod != nil && *c.IdentityMethod != actual.IdentityMethod {
			nueva.IdentityMethod = *c.IdentityMethod
			cambios["identity_method"] = true
		}
		cierra := nueva.Status == EstadoSolicitudResuelta || nueva.Status == EstadoSolicitudDenegada
		if cierra && strings.TrimSpace(nueva.Resolution) == "" {
			return domain.ErrInvalidInput
		}
		// Una supresión de cuenta no se da por resuelta antes de que el barrido
		// la haya terminado: lo que se le dice a la persona tiene que ser verdad.
		if nueva.Status == EstadoSolicitudResuelta && actual.ErasureStartedAt != nil && actual.ErasureCompletedAt == nil {
			return ErrSupresionNoAdmitida
		}
		if len(cambios) == 0 {
			out = actual
			return nil
		}
		row := q.QueryRow(ctx, `UPDATE dsr_requests SET status = $3, resolution = $4, notes = $5,
			    identity_verified = $6, identity_method = $7, updated_at = now(),
			    closed_at = CASE WHEN $8 THEN now() ELSE NULL END
			WHERE id = $1::uuid AND org_id = $2 RETURNING `+columnasSolicitudRGPD,
			id, orgID, nueva.Status, nueva.Resolution, nueva.Notes, nueva.IdentityVerified, nueva.IdentityMethod, cierra)
		if out, err = escanearSolicitud(row); err != nil {
			return errf("ActualizarSolicitudRGPD", err)
		}
		return eventoSolicitud(ctx, q, id, actorID, "updated", cambios)
	})
	return out, err
}

// ProrrogarSolicitudRGPD alarga dos meses el plazo de una solicitud abierta, una
// sola vez y antes de que venza (art. 12.3: la prórroga se comunica dentro del
// primer mes, con sus motivos).
func (s *Store) ProrrogarSolicitudRGPD(ctx context.Context, orgID, id, motivo, actorID string) (SolicitudRGPD, error) {
	var out SolicitudRGPD
	err := s.InTx(ctx, func(q pg.Querier) error {
		actual, err := solicitudParaCambiar(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if actual.Extended || !actual.DueAt.After(time.Now()) {
			return ErrProrrogaNoAdmitida
		}
		row := q.QueryRow(ctx, `UPDATE dsr_requests SET extended = true, extension_reason = $3,
			    due_at = due_at + make_interval(months => $4::int), updated_at = now()
			WHERE id = $1::uuid AND org_id = $2 RETURNING `+columnasSolicitudRGPD, id, orgID, motivo, ProrrogaMesesRGPD)
		if out, err = escanearSolicitud(row); err != nil {
			return errf("ProrrogarSolicitudRGPD", err)
		}
		return eventoSolicitud(ctx, q, id, actorID, "extended", map[string]any{"due_at": out.DueAt.UTC().Format(time.RFC3339)})
	})
	return out, err
}

// solicitudParaCambiar lee y BLOQUEA una solicitud abierta de la organización.
func solicitudParaCambiar(ctx context.Context, q pg.Querier, orgID, id string) (SolicitudRGPD, error) {
	sol, err := escanearSolicitud(q.QueryRow(ctx, `SELECT `+columnasSolicitudRGPD+` FROM dsr_requests
		WHERE id = $1::uuid AND org_id = $2 FOR UPDATE`, id, orgID))
	if errors.Is(err, pgx.ErrNoRows) {
		return sol, domain.ErrNotFound
	}
	if err != nil {
		return sol, errf("solicitudParaCambiar", err)
	}
	if sol.ClosedAt != nil {
		return sol, ErrSolicitudCerrada
	}
	return sol, nil
}

// IniciarSupresionRGPD ejecuta la SUPRESIÓN FÍSICA de la cuenta de una solicitud
// de supresión: la da de baja (si no lo estaba), borra en la misma transacción
// todo lo que sirve para entrar y la marca para que el barrido de bajas la
// anonimice sin esperar los treinta días y sin que nadie pueda restaurarla.
//
// Solo si la cuenta es SOLO de esta organización: si es miembro de otra, o tiene
// su cuenta personal, esa parte no es de quien administra esta y la supresión se
// niega (ErrCuentaDeFuera). Y no puede dejar una organización sin quien la
// administre (ErrUnicoAdministrador), con el mismo bloqueo que la baja propia.
func (s *Store) IniciarSupresionRGPD(ctx context.Context, orgID, id, actorID string) (SolicitudRGPD, error) {
	var out SolicitudRGPD
	err := s.InTx(ctx, func(q pg.Querier) error {
		sol, err := solicitudParaCambiar(ctx, q, orgID, id)
		if err != nil {
			return err
		}
		if sol.Kind != SolicitudSupresion || sol.SubjectUserID == "" || sol.ErasureStartedAt != nil {
			return ErrSupresionNoAdmitida
		}
		// Sigue siendo de la organización (quitarla de ella no debe abrir la puerta
		// a borrar la cuenta de otro), y no lo es de nadie más.
		var aqui, fuera bool
		if err := q.QueryRow(ctx, `SELECT
			    EXISTS (SELECT 1 FROM memberships WHERE user_id = $1 AND org_id = $2),
			    EXISTS (SELECT 1 FROM memberships WHERE user_id = $1 AND org_id <> $2)`,
			sol.SubjectUserID, orgID).Scan(&aqui, &fuera); err != nil {
			return errf("IniciarSupresionRGPD.miembro", err)
		}
		if !aqui {
			return domain.ErrNotFound
		}
		if fuera {
			return ErrCuentaDeFuera
		}
		bloquean, err := s.OrgsSinOtroAdministradorBloqueando(ctx, q, sol.SubjectUserID)
		if err != nil {
			return err
		}
		if len(bloquean) > 0 {
			return ErrUnicoAdministrador
		}
		if _, err := s.CloseUser(ctx, q, sol.SubjectUserID, actorID, "rgpd:"+sol.ID); err != nil {
			return err
		}
		for _, sql := range credencialesSQL {
			if _, err := q.Exec(ctx, sql, sol.SubjectUserID); err != nil {
				return errf("IniciarSupresionRGPD.credenciales", err)
			}
		}
		// personal_data_erased_at a NULL: lo que los otros servicios guardan de la
		// persona se borra otra vez aunque una baja anterior ya lo hubiera hecho.
		if _, err := q.Exec(ctx, `UPDATE users SET erasure_request_id = $2::uuid, personal_data_erased_at = NULL
			WHERE id = $1 AND status = 'deleted' AND purged_at IS NULL`, sol.SubjectUserID, sol.ID); err != nil {
			return errf("IniciarSupresionRGPD.marca", err)
		}
		row := q.QueryRow(ctx, `UPDATE dsr_requests SET erasure_started_at = now(), updated_at = now(),
			    status = CASE WHEN status = 'received' THEN 'in_progress' ELSE status END
			WHERE id = $1::uuid AND org_id = $2 RETURNING `+columnasSolicitudRGPD, id, orgID)
		if out, err = escanearSolicitud(row); err != nil {
			return errf("IniciarSupresionRGPD", err)
		}
		return eventoSolicitud(ctx, q, id, actorID, "erasure.started", map[string]any{"user_id": sol.SubjectUserID})
	})
	return out, err
}

// CompletarSupresionRGPD deja escrito en la solicitud que el barrido terminó la
// supresión física de la cuenta (ficha anonimizada, membresías fuera y lo que los
// demás servicios guardaban de la persona, borrado). Idempotente.
func (s *Store) CompletarSupresionRGPD(ctx context.Context, q pg.Querier, solicitudID string) error {
	if q == nil {
		q = s.DB()
	}
	tag, err := q.Exec(ctx, `UPDATE dsr_requests SET erasure_completed_at = now(), updated_at = now()
		WHERE id = $1::uuid AND erasure_started_at IS NOT NULL AND erasure_completed_at IS NULL`, solicitudID)
	if err != nil {
		return errf("CompletarSupresionRGPD", err)
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return eventoSolicitud(ctx, q, solicitudID, "", "erasure.completed", map[string]any{})
}

// PurgarSolicitudesRGPDVencidas borra las solicitudes cerradas hace más de
// RetencionSolicitudesRGPD, con su historia.
func (s *Store) PurgarSolicitudesRGPDVencidas(ctx context.Context) (int64, error) {
	tag, err := s.DB().Exec(ctx, `DELETE FROM dsr_requests WHERE closed_at < $1`, time.Now().Add(-RetencionSolicitudesRGPD))
	if err != nil {
		return 0, errf("PurgarSolicitudesRGPDVencidas", err)
	}
	return tag.RowsAffected(), nil
}

func eventoSolicitud(ctx context.Context, q pg.Querier, id, actorID, accion string, detalle map[string]any) error {
	raw, err := json.Marshal(detalle)
	if err != nil {
		return errf("eventoSolicitud", err)
	}
	_, err = q.Exec(ctx, `INSERT INTO dsr_request_events (request_id, actor_id, action, detail) VALUES ($1::uuid, $2::uuid, $3, $4)`,
		id, nilSiVacio(actorID), accion, raw)
	return errf("eventoSolicitud", err)
}
