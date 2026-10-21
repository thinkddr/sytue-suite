// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de Sytue Rooms sobre PostgreSQL
// (pgx): salas, participantes y metadatos de grabación. Todo acceso pasa por
// transacciones con `app.tenant` fijado (pg.InTenantTx) para que RLS aísle por
// tenant. Consultas SIEMPRE parametrizadas (sin SQL dinámico), conforme a
// CONVENTIONS §10. Único datastore: PostgreSQL (sin Redis/Valkey).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/media/domain"
)

// Store agrupa el acceso a datos de Meet.
type Store struct {
	pool *pg.Pool
}

// New construye un Store sobre el pool dado.
func New(pool *pg.Pool) *Store { return &Store{pool: pool} }

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// uniqueViolation indica si el error es una violación de unicidad (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---------------------------------------------------------------------------
// Salas
// ---------------------------------------------------------------------------

const roomCols = `id, name, title, status, created_by, max_participants, empty_timeout_seconds,
	livekit_sid, metadata, locked, started_at, ended_at, created_at, updated_at, clave`

func scanRoom(row pgx.Row, t string) (domain.Room, error) {
	var r domain.Room
	var meta []byte
	if err := row.Scan(&r.ID, &r.Name, &r.Title, &r.Status, &r.CreatedBy,
		&r.MaxParticipants, &r.EmptyTimeoutSeconds, &r.LiveKitSID, &meta, &r.Locked,
		&r.StartedAt, &r.EndedAt, &r.CreatedAt, &r.UpdatedAt, &r.LinkKey); err != nil {
		return domain.Room{}, err
	}
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &r.Metadata)
	}
	r.Tenant = t
	return r, nil
}

// NewRoomRecord son los datos de creación de una sala.
type NewRoomRecord struct {
	Name                string
	Title               string
	CreatedBy           string
	MaxParticipants     int
	EmptyTimeoutSeconds int
	Metadata            map[string]any
}

// CreateRoom inserta una sala (estado scheduled). El nombre técnico es único por
// tenant. Devuelve ErrConflict si el nombre ya existe.
func (s *Store) CreateRoom(ctx context.Context, rec NewRoomRecord) (domain.Room, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidRoomName(rec.Name) || !domain.ValidTitle(rec.Title) {
		return domain.Room{}, domain.ErrInvalidName
	}
	meta := []byte("{}")
	if rec.Metadata != nil {
		b, err := json.Marshal(rec.Metadata)
		if err != nil {
			return domain.Room{}, fmt.Errorf("store: serializar metadata: %w", err)
		}
		meta = b
	}
	var r domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO rooms (tenant, name, title, status, created_by, max_participants, empty_timeout_seconds, metadata)
			 VALUES ($1, $2, $3, 'scheduled', $4, $5, $6, $7)
			 RETURNING `+roomCols,
			t.Slug, strings.TrimSpace(rec.Name), strings.TrimSpace(rec.Title), rec.CreatedBy,
			rec.MaxParticipants, rec.EmptyTimeoutSeconds, meta)
		var e error
		r, e = scanRoom(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	return r, err
}

// GetRoom devuelve una sala por ID.
func (s *Store) GetRoom(ctx context.Context, id uuid.UUID) (domain.Room, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE tenant=$1 AND id=$2`, t.Slug, id)
		var e error
		r, e = scanRoom(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Room{}, domain.ErrNotFound
	}
	return r, err
}

// GetRoomByName devuelve una sala por su nombre técnico de LiveKit.
func (s *Store) GetRoomByName(ctx context.Context, name string) (domain.Room, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT `+roomCols+` FROM rooms WHERE tenant=$1 AND (name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=rooms.tenant AND e.call_id=rooms.id AND e.livekit_room=$2))`, t.Slug, name)
		var e error
		r, e = scanRoom(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Room{}, domain.ErrNotFound
	}
	return r, err
}

// FindRoomBySource busca la sala viva (no finalizada) más reciente enlazada a un
// recurso de origen cross-app (evento de Calendar o canal de Chat) por su
// (kind, ref_id), leídos del enlace persistido en metadata->'source'. Devuelve
// found=false si no hay ninguna. La usa CreateRoomFromSource para ser idempotente
// por origen (una sala viva por evento/canal). Consulta parametrizada (los
// operadores JSONB ->/->> no interpolan valores en el SQL).
func (s *Store) FindRoomBySource(ctx context.Context, kind domain.SourceKind, refID string) (domain.Room, bool, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+roomCols+` FROM rooms
			 WHERE tenant=$1 AND metadata->'source'->>'kind'=$2 AND metadata->'source'->>'ref_id'=$3 AND status <> 'ended'
			 ORDER BY created_at DESC LIMIT 1`,
			t.Slug, string(kind), refID)
		var e error
		r, e = scanRoom(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Room{}, false, nil
	}
	if err != nil {
		return domain.Room{}, false, err
	}
	return r, true, nil
}

// ListRooms lista las salas del tenant, opcionalmente filtradas por estado, con
// paginación keyset por created_at descendente.
func (s *Store) ListRooms(ctx context.Context, status domain.RoomStatus, limit int, sujeto string) ([]domain.Room, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var rooms []domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Una reunión SUELTA (ni de un canal ni de un documento) solo sale a quien la
		// convocó o ya estuvo dentro: el resto entra con el enlace, no por la lista.
		// Las de un canal y las de un documento las filtra el servicio con su verja.
		rows, err := tx.Query(ctx, `SELECT `+roomCols+` FROM rooms
			WHERE tenant=$1 AND ($2 = '' OR status=$2)
			  AND (coalesce(metadata->'source'->>'kind', '') IN ('chat', 'document')
			       OR ($3 <> '' AND (created_by = $3
			           OR EXISTS (SELECT 1 FROM participants p
			                       WHERE p.tenant=$1 AND p.room_id = rooms.id AND p.identity = $3))))
			ORDER BY created_at DESC LIMIT $4`,
			t.Slug, string(status), sujeto, limit)
		if err != nil {
			return fmt.Errorf("store: listar salas: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRoom(rows, t.Slug)
			if err != nil {
				return err
			}
			rooms = append(rooms, r)
		}
		return rows.Err()
	})
	return rooms, err
}

// MarkRoomStarted marca una sala como activa al recibir room_started de LiveKit.
// Idempotente: no degrada una sala ya finalizada.
func (s *Store) MarkRoomStarted(ctx context.Context, name, livekitSID string, startedAt time.Time) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE rooms
			 SET status='active', livekit_sid=$3, started_at=COALESCE(started_at, $4), updated_at=now()
			 WHERE tenant=$1 AND (name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=rooms.tenant AND e.call_id=rooms.id AND e.livekit_room=$2)) AND status <> 'ended'`,
			t.Slug, name, livekitSID, startedAt.UTC())
		return err
	})
}

// MarkRoomEnded marca una sala como finalizada al recibir room_finished.
func (s *Store) MarkRoomEnded(ctx context.Context, name string, endedAt time.Time) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`UPDATE rooms SET status='ended', ended_at=$3, updated_at=now()
			 WHERE tenant=$1 AND (name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=rooms.tenant AND e.call_id=rooms.id AND e.livekit_room=$2)) AND status <> 'ended'`,
			t.Slug, name, endedAt.UTC()); err != nil {
			return err
		}
		// Al cerrar la sala, da por salidos a los participantes aún presentes.
		_, err := tx.Exec(ctx,
			`UPDATE participants p SET left_at=$3
			 FROM rooms r
			 WHERE p.tenant=$1 AND r.tenant=p.tenant AND p.room_id=r.id AND (r.name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=r.tenant AND e.call_id=r.id AND e.livekit_room=$2)) AND p.left_at IS NULL`,
			t.Slug, name, endedAt.UTC())
		return err
	})
}

// CloseRoom marca una sala como finalizada de forma explícita (acción de host).
func (s *Store) CloseRoom(ctx context.Context, id uuid.UUID) (domain.Room, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE rooms SET status='ended', ended_at=COALESCE(ended_at, now()), updated_at=now()
			 WHERE tenant=$1 AND id=$2 RETURNING `+roomCols,
			t.Slug, id)
		var e error
		r, e = scanRoom(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Room{}, domain.ErrNotFound
	}
	return r, err
}

// ---------------------------------------------------------------------------
// Participantes
// ---------------------------------------------------------------------------

const partCols = `id, room_id, identity, name, role, livekit_sid, joined_at, left_at`

func scanParticipant(row pgx.Row, t string) (domain.Participant, error) {
	var p domain.Participant
	if err := row.Scan(&p.ID, &p.RoomID, &p.Identity, &p.Name, &p.Role,
		&p.LiveKitSID, &p.JoinedAt, &p.LeftAt); err != nil {
		return domain.Participant{}, err
	}
	p.Tenant = t
	return p, nil
}

// SetRoomLocked abre o cierra la puerta de una sala.
//
// Devuelve la sala ya actualizada para que quien llama no tenga que releerla: lo
// que ve la interfaz es lo que quedó en la base, no lo que se pidió.
func (s *Store) SetRoomLocked(ctx context.Context, id uuid.UUID, locked bool) (domain.Room, error) {
	t := tenancy.MustFrom(ctx)
	var r domain.Room
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE rooms SET locked=$3, updated_at=now()
			 WHERE tenant=$1 AND id=$2 RETURNING `+roomCols,
			t.Slug, id, locked)
		var e error
		r, e = scanRoom(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Room{}, domain.ErrNotFound
	}
	return r, err
}

// ---------------------------------------------------------------------------
// Sala de espera
// ---------------------------------------------------------------------------

const admCols = `id, room_id, identity, name, requested_at, granted_at`

func scanAdmission(row pgx.Row, t string) (domain.Admission, error) {
	var a domain.Admission
	if err := row.Scan(&a.ID, &a.RoomID, &a.Identity, &a.Name,
		&a.RequestedAt, &a.GrantedAt); err != nil {
		return domain.Admission{}, err
	}
	a.Tenant = t
	return a, nil
}

// Knock anota que alguien llama a la puerta de una sala.
//
// Idempotente: llamar dos veces no crea dos filas ni reordena la cola —el
// `requested_at` NO se refresca, o quien insista subiría de puesto y quien
// esperó en silencio bajaría—. Si ya estaba admitido se devuelve tal cual, que
// es la respuesta correcta: puede pasar.
func (s *Store) Knock(ctx context.Context, roomID uuid.UUID, identity, name string) (domain.Admission, error) {
	t := tenancy.MustFrom(ctx)
	var a domain.Admission
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO room_admissions (tenant, room_id, identity, name)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (room_id, identity) DO UPDATE
			   SET name = COALESCE(NULLIF(EXCLUDED.name, ''), room_admissions.name)
			 RETURNING `+admCols, t.Slug, roomID, identity, name)
		var e error
		a, e = scanAdmission(row, t.Slug)
		return e
	})
	if err != nil {
		return domain.Admission{}, fmt.Errorf("meet: llamar a la puerta: %w", err)
	}
	return a, nil
}

// SetAdmission abre (o cierra) la puerta a una persona concreta.
//
// Denegar BORRA la fila en vez de marcarla: si se quedara, quien fue rechazado
// no podría volver a llamar nunca —el upsert encontraría su fila y no habría
// forma de distinguir «esperando» de «ya le dijeron que no»—, y una puerta que
// no se puede volver a tocar no es una sala de espera, es una lista negra.
func (s *Store) SetAdmission(ctx context.Context, roomID uuid.UUID, identity string, admitido bool) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if !admitido {
			_, err := tx.Exec(ctx,
				`DELETE FROM room_admissions WHERE tenant=$1 AND room_id=$2 AND identity=$3`,
				t.Slug, roomID, identity)
			return err
		}
		_, err := tx.Exec(ctx,
			`UPDATE room_admissions SET granted_at = COALESCE(granted_at, now())
			 WHERE tenant=$1 AND room_id=$2 AND identity=$3`,
			t.Slug, roomID, identity)
		return err
	})
}

// IsAdmitted responde si esa persona puede pasar aunque la sala esté cerrada.
func (s *Store) IsAdmitted(ctx context.Context, roomID uuid.UUID, identity string) (bool, error) {
	t := tenancy.MustFrom(ctx)
	var existe bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM room_admissions
			   WHERE tenant=$1 AND room_id=$2 AND identity=$3 AND granted_at IS NOT NULL)`,
			t.Slug, roomID, identity).Scan(&existe)
	})
	return existe, err
}

// ListKnocks devuelve la cola de la puerta: quién está esperando, por orden de
// llegada. Los ya admitidos no salen —no están esperando— y por eso el índice
// parcial cubre exactamente esta consulta.
func (s *Store) ListKnocks(ctx context.Context, roomID uuid.UUID) ([]domain.Admission, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Admission
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+admCols+` FROM room_admissions
			 WHERE tenant=$1 AND room_id=$2 AND granted_at IS NULL
			 ORDER BY requested_at`, t.Slug, roomID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			a, e := scanAdmission(rows, t.Slug)
			if e != nil {
				return e
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	return out, err
}

// RecordJoinInput son los datos de entrada de un participante.
type RecordJoinInput struct {
	RoomName   string
	Identity   string
	Name       string
	Role       domain.Role
	LiveKitSID string
	JoinedAt   time.Time
}

// RecordJoin registra (o reabre) la presencia de un participante en la sala
// identificada por su nombre. Resuelve el room_id por nombre dentro del tenant.
func (s *Store) RecordJoin(ctx context.Context, in RecordJoinInput) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var roomID uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM rooms WHERE tenant=$1 AND (name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=rooms.tenant AND e.call_id=rooms.id AND e.livekit_room=$2))`, t.Slug, in.RoomName).Scan(&roomID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		role := in.Role
		if !domain.ValidRole(role) {
			role = domain.RoleSpeaker
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO participants (tenant, room_id, identity, name, role, livekit_sid, joined_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)
			 ON CONFLICT (tenant, room_id, livekit_sid) DO UPDATE
			   SET left_at=NULL, name=EXCLUDED.name, role=EXCLUDED.role`,
			t.Slug, roomID, in.Identity, in.Name, string(role), in.LiveKitSID, in.JoinedAt.UTC())
		return err
	})
}

// RecordLeave marca la salida de un participante (por SID de LiveKit).
func (s *Store) RecordLeave(ctx context.Context, roomName, livekitSID string, leftAt time.Time) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`UPDATE participants p SET left_at=$4
			 FROM rooms r
			 WHERE p.tenant=$1 AND r.tenant=p.tenant AND p.room_id=r.id AND (r.name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=r.tenant AND e.call_id=r.id AND e.livekit_room=$2)) AND p.livekit_sid=$3 AND p.left_at IS NULL`,
			t.Slug, roomName, livekitSID, leftAt.UTC())
		return err
	})
}

// ListParticipants lista los participantes presentes de una sala.
func (s *Store) ListParticipants(ctx context.Context, roomID uuid.UUID, onlyPresent bool) ([]domain.Participant, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Participant
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		q := `SELECT ` + partCols + ` FROM participants WHERE tenant=$1 AND room_id=$2`
		if onlyPresent {
			q += ` AND left_at IS NULL`
		}
		q += ` ORDER BY joined_at`
		rows, err := tx.Query(ctx, q, t.Slug, roomID)
		if err != nil {
			return fmt.Errorf("store: listar participantes: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			p, err := scanParticipant(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Grabaciones (metadatos de egress)
// ---------------------------------------------------------------------------

const recCols = `id, room_id, egress_id, status, storage_key, size_bytes, duration_sec, started_at, ended_at`

func scanRecording(row pgx.Row, t string) (domain.Recording, error) {
	var r domain.Recording
	if err := row.Scan(&r.ID, &r.RoomID, &r.EgressID, &r.Status, &r.StorageKey,
		&r.SizeBytes, &r.DurationSec, &r.StartedAt, &r.EndedAt); err != nil {
		return domain.Recording{}, err
	}
	r.Tenant = t
	return r, nil
}

// UpsertRecording inserta o actualiza el metadato de una grabación por egress_id.
func (s *Store) UpsertRecording(ctx context.Context, roomName, egressID string, status domain.RecordingStatus, storageKey string, size, duration int64, ended *time.Time) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var roomID uuid.UUID
		if err := tx.QueryRow(ctx,
			`SELECT id FROM rooms WHERE tenant=$1 AND (name=$2 OR EXISTS(SELECT 1 FROM matrix_call_epochs e WHERE e.tenant=rooms.tenant AND e.call_id=rooms.id AND e.livekit_room=$2))`, t.Slug, roomName).Scan(&roomID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO recordings_meta (tenant, room_id, egress_id, status, storage_key, size_bytes, duration_sec, ended_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (tenant, egress_id) DO UPDATE
			   SET status=EXCLUDED.status,
			       storage_key=COALESCE(NULLIF(EXCLUDED.storage_key, ''), recordings_meta.storage_key),
			       size_bytes=GREATEST(recordings_meta.size_bytes, EXCLUDED.size_bytes),
			       duration_sec=GREATEST(recordings_meta.duration_sec, EXCLUDED.duration_sec),
			       ended_at=COALESCE(EXCLUDED.ended_at, recordings_meta.ended_at)`,
			t.Slug, roomID, egressID, string(status), storageKey, size, duration, ended)
		return err
	})
}

// ListRecordings lista los metadatos de grabación de una sala.
func (s *Store) ListRecordings(ctx context.Context, roomID uuid.UUID) ([]domain.Recording, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Recording
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+recCols+` FROM recordings_meta WHERE tenant=$1 AND room_id=$2 ORDER BY started_at DESC`,
			t.Slug, roomID)
		if err != nil {
			return fmt.Errorf("store: listar grabaciones: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanRecording(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}

// DeleteRecording borra la fila de una grabación. El FICHERO lo borra el
// servicio ANTES de llamar aquí: si se hiciera al revés, un fallo a mitad
// dejaría en el bucket un objeto que ya no aparece en ninguna consulta —nadie
// podría borrarlo porque nadie sabría que está—.
//
// Que no borre nada no es un error: la llamada se repite (un reintento, dos
// pestañas) y el resultado que importa —que no esté— ya se cumple.
func (s *Store) DeleteRecording(ctx context.Context, roomID, recordingID uuid.UUID) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`DELETE FROM recordings_meta WHERE tenant=$1 AND room_id=$2 AND id=$3`,
			t.Slug, roomID, recordingID); err != nil {
			return fmt.Errorf("store: borrar la grabación: %w", err)
		}
		return nil
	})
}
