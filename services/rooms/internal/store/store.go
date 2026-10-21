// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package store implementa la persistencia de Sytue Rooms sobre PostgreSQL (pgx).
// Todo acceso pasa por transacciones con `app.tenant` fijado (pg.InTenantTx)
// para que RLS aísle por tenant. Consultas SIEMPRE parametrizadas (sin SQL
// dinámico), conforme a CONVENTIONS §10.
package store

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

// Store agrupa el acceso a datos de Chat.
type Store struct {
	pool *pg.Pool
	// cipher cifra en reposo los secretos de FIRMA de las integraciones salientes.
	// Nil = sin KEK: entonces crear una integración con secreto FALLA (ErrNoCipher)
	// en vez de escribirlo en claro. Ver sealSecret/openSecret.
	cipher               *keycrypto.Cipher
	matrixGate           MatrixGate
	callMatrix           CallMatrixGate
	callMedia            CallMediaTransport
	documentCallsEnabled bool
}

// New construye un Store sobre el pool dado, con el cifrador de secretos en
// reposo (puede ser nil en desarrollo: ver Store.cipher).
func New(pool *pg.Pool, cipher *keycrypto.Cipher) *Store {
	return &Store{pool: pool, cipher: cipher}
}

// ErrNoCipher indica que se pidió guardar un secreto sin KEK configurada. Se
// falla CERRADO a propósito: un secreto de firma en claro en la base deja que
// cualquiera que la lea firme peticiones haciéndose pasar por nosotros, que es
// justo lo que la firma existe para impedir.
var ErrNoCipher = errors.New("store: falta SYTUE_ROOMS_KEY_ENCRYPTION_KEY para cifrar el secreto en reposo")

// sealSecret cifra un secreto para guardarlo. Un secreto vacío se queda vacío
// (no hay nada que proteger y así la columna sigue distinguiendo «sin firma»).
func (s *Store) sealSecret(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	if !s.cipher.Enabled() {
		return "", ErrNoCipher
	}
	return s.cipher.Encrypt(plain)
}

// openSecret descifra un secreto leído. Los valores heredados EN CLARO —los
// escritos antes de que esto existiera— no llevan el prefijo `encv1:` y
// keycrypto los devuelve tal cual, así que las integraciones ya creadas siguen
// firmando mientras se rotan.
func (s *Store) openSecret(stored string) (string, error) {
	if stored == "" || !s.cipher.Enabled() {
		return stored, nil
	}
	return s.cipher.Decrypt(stored)
}

// Healthz delega en el pool (para sondas /readyz).
func (s *Store) Healthz(ctx context.Context) error { return s.pool.Healthz(ctx) }

// uniqueViolation indica si el error es una violación de unicidad (23505).
func uniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ---------------------------------------------------------------------------
// Teams
// ---------------------------------------------------------------------------

const teamCols = `id, slug, name, description, created_by, created_at, updated_at`

// scanTeam lee las columnas de teamCols. `extra` recibe los destinos de las columnas que
// una consulta concreta añada DESPUÉS (hoy solo ListTeams, con m.role): así una columna
// de más no obliga a un segundo scanner que luego se olvide de crecer cuando crezca
// teamCols — que es como se cuelan los desajustes entre columnas y descriptores.
func scanTeam(row pgx.Row, t string, extra ...any) (domain.Team, error) {
	var tm domain.Team
	dst := append([]any{
		&tm.ID, &tm.Slug, &tm.Name, &tm.Description,
		&tm.CreatedBy, &tm.CreatedAt, &tm.UpdatedAt,
	}, extra...)
	if err := row.Scan(dst...); err != nil {
		return domain.Team{}, err
	}
	tm.Tenant = t
	return tm, nil
}

// NewTeamRecord son los datos para crear un team.
type NewTeamRecord struct {
	Slug        string
	Name        string
	Description string
	CreatedBy   string
	CreatorName string
}

// CreateTeam inserta un team y añade a su creador como admin, atómicamente.
func (s *Store) CreateTeam(ctx context.Context, rec NewTeamRecord) (domain.Team, error) {
	t := tenancy.MustFrom(ctx)
	var tm domain.Team
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO teams (tenant, slug, name, description, created_by)
			 VALUES ($1, $2, $3, $4, $5) RETURNING `+teamCols,
			t.Slug, rec.Slug, strings.TrimSpace(rec.Name), rec.Description, rec.CreatedBy)
		var e error
		tm, e = scanTeam(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		if e != nil {
			return e
		}
		if _, e = tx.Exec(ctx,
			`INSERT INTO team_members (tenant, team_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, 'admin')`,
			t.Slug, tm.ID, rec.CreatedBy, rec.CreatorName); e != nil {
			return e
		}
		// Canales por defecto en la MISMA transacción: un team recién creado sin
		// ningún canal es un callejón sin salida (ver seedDefaultChannels).
		if e := seedDefaultChannels(ctx, tx, t.Slug, tm.ID, rec.CreatedBy); e != nil {
			return e
		}
		return joinDefaultChannels(ctx, tx, t.Slug, tm.ID, rec.CreatedBy, rec.CreatorName)
	})
	return tm, err
}

// EnsureDefaultTeamMembership devuelve el team por defecto del tenant (slug
// "general"), creándolo si no existe, y garantiza la pertenencia del usuario.
// Carrera-seguro: el INSERT usa el índice único parcial de slug (ON CONFLICT
// DO NOTHING) y ante empate se relee la fila ganadora. El creador entra como
// admin; quien se incorpora después, como member.
func (s *Store) EnsureDefaultTeamMembership(ctx context.Context, sub, name string) (domain.Team, bool, error) {
	t := tenancy.MustFrom(ctx)
	var tm domain.Team
	var created bool
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`INSERT INTO teams (tenant, slug, name, description, created_by)
			 VALUES ($1, 'general', 'General', 'Espacio por defecto del equipo', $2)
			 ON CONFLICT (tenant, slug) WHERE deleted_at IS NULL DO NOTHING
			 RETURNING `+teamCols,
			t.Slug, sub)
		var e error
		tm, e = scanTeam(row, t.Slug)
		switch {
		case e == nil:
			created = true
		case errors.Is(e, pgx.ErrNoRows):
			// Ya existía (u otro proceso ganó la carrera): usar la fila vigente.
			row = tx.QueryRow(ctx,
				`SELECT `+teamCols+` FROM teams
				 WHERE tenant=$1 AND slug='general' AND deleted_at IS NULL`, t.Slug)
			if tm, e = scanTeam(row, t.Slug); e != nil {
				return e
			}
		default:
			return e
		}
		role := "member"
		if created {
			role = "admin"
		}
		if _, e = tx.Exec(ctx,
			`INSERT INTO team_members (tenant, team_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, team_id, subject) DO NOTHING`,
			t.Slug, tm.ID, sub, name, role); e != nil {
			return e
		}
		// Los canales por defecto se siembran SOLO al crear el team: en uno ya
		// existente no se resucita lo que su equipo haya archivado o borrado.
		if created {
			if e := seedDefaultChannels(ctx, tx, t.Slug, tm.ID, sub); e != nil {
				return e
			}
		}
		return joinDefaultChannels(ctx, tx, t.Slug, tm.ID, sub, name)
	})
	return tm, created, err
}

// ListTeams lista los teams de los que el usuario es miembro.
func (s *Store) ListTeams(ctx context.Context, subject string) ([]domain.Team, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Team
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// m.role sale GRATIS: la unión con team_members ya está ahí para filtrar por
		// pertenencia, así que devolver el rol de quien pregunta no cuesta ni una
		// consulta más ni un índice nuevo.
		rows, err := tx.Query(ctx,
			`SELECT `+prefixed("tm", teamCols)+`, m.role FROM teams tm
			 JOIN team_members m ON m.tenant = tm.tenant AND m.team_id = tm.id
			 WHERE tm.tenant = $1 AND m.subject = $2 AND tm.deleted_at IS NULL
			 ORDER BY lower(tm.name)`, t.Slug, subject)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var role string
			tm, err := scanTeam(rows, t.Slug, &role)
			if err != nil {
				return err
			}
			tm.Role = role
			out = append(out, tm)
		}
		return rows.Err()
	})
	return out, err
}

// GetTeam devuelve un team vivo por ID, exigiendo que el usuario sea miembro.
func (s *Store) GetTeam(ctx context.Context, id uuid.UUID, subject string) (domain.Team, error) {
	t := tenancy.MustFrom(ctx)
	var tm domain.Team
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, id, subject); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`SELECT `+teamCols+` FROM teams WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, t.Slug, id)
		var e error
		tm, e = scanTeam(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Team{}, domain.ErrNotFound
	}
	return tm, err
}

// ---------------------------------------------------------------------------
// Membresía (teams y canales)
// ---------------------------------------------------------------------------

// requireTeamMember falla con ErrForbidden si el usuario no pertenece al team.
func requireTeamMember(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, subject string) error {
	var role string
	err := tx.QueryRow(ctx,
		`SELECT role FROM team_members WHERE tenant=$1 AND team_id=$2 AND subject=$3`,
		tenant, teamID, subject).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrForbidden
	}
	return err
}

// teamRole devuelve el rol del usuario en el team (o ErrForbidden).
func teamRole(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, subject string) (domain.MemberRole, error) {
	var role string
	err := tx.QueryRow(ctx,
		`SELECT role FROM team_members WHERE tenant=$1 AND team_id=$2 AND subject=$3`,
		tenant, teamID, subject).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrForbidden
	}
	if err != nil {
		return "", err
	}
	return domain.MemberRole(role), nil
}

// AddTeamMember añade (o reactiva) un miembro a un team. Solo un admin puede.
func (s *Store) AddTeamMember(ctx context.Context, teamID uuid.UUID, actor, subject, name string, role domain.MemberRole) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		r, err := teamRole(ctx, tx, t.Slug, teamID, actor)
		if err != nil {
			return err
		}
		if !r.CanManage() {
			return domain.ErrForbidden
		}
		if _, err = tx.Exec(ctx,
			`INSERT INTO team_members (tenant, team_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (tenant, team_id, subject) DO UPDATE SET role=EXCLUDED.role, name=EXCLUDED.name`,
			t.Slug, teamID, subject, name, string(role)); err != nil {
			return err
		}
		// Auto-alta en los canales por defecto en la MISMA transacción: sin esto el
		// recién incorporado entraba al equipo sin ningún canal propio.
		return joinDefaultChannels(ctx, tx, t.Slug, teamID, subject, name)
	})
}

// RemoveTeamMember elimina a un miembro de un team (admin, o uno mismo).
func (s *Store) RemoveTeamMember(ctx context.Context, teamID uuid.UUID, actor, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		if actor != subject {
			r, err := teamRole(ctx, tx, t.Slug, teamID, actor)
			if err != nil {
				return err
			}
			if !r.CanManage() {
				return domain.ErrForbidden
			}
		}
		ids, err := s.lockTeamMatrixChannelsTx(ctx, tx, t.Slug, teamID)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM team_members WHERE tenant=$1 AND team_id=$2 AND subject=$3`,
			t.Slug, teamID, subject)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		// Salir del team ARRASTRA la membresía de sus canales, en la MISMA
		// transacción: si no, quedaban filas de channel_members vivas y el afectado
		// conservaba acceso a los canales privados del team entrando por el
		// channelID (las verjas de channels.go exigen ahora también el team, pero
		// esta limpieza es la raíz: sin ella el usuario seguiría viendo los canales
		// listados y ocupando el roster).
		// Los DIRECTOS quedan fuera a propósito: un DM es de ámbito de organización
		// y no muere al dejar UN team (igual que en Mattermost/Slack); lo que corta
		// los DM es dar de baja la cuenta en Sytue ID.
		if _, err := tx.Exec(ctx,
			`DELETE FROM channel_members cm
			  USING channels ch
			  WHERE cm.tenant=$1 AND cm.subject=$2
			    AND ch.tenant=cm.tenant AND ch.id=cm.channel_id
			    AND ch.team_id=$3 AND ch.kind <> 'direct'`,
			t.Slug, subject, teamID); err != nil {
			return err
		}
		for _, id := range ids {
			if err := s.revokeMatrixTx(ctx, tx, t.Slug, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListTeamMembers lista los miembros de un team (exige pertenencia).
func (s *Store) ListTeamMembers(ctx context.Context, teamID uuid.UUID, actor string) ([]domain.Member, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Member
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, teamID, actor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT subject, name, role, joined_at FROM team_members
			 WHERE tenant=$1 AND team_id=$2 ORDER BY lower(name), subject`, t.Slug, teamID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m domain.Member
			var role string
			if err := rows.Scan(&m.Subject, &m.Name, &role, &m.JoinedAt); err != nil {
				return err
			}
			m.Role = domain.MemberRole(role)
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// prefixed añade un alias de tabla a una lista de columnas separadas por coma.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ", ")
	for i, p := range parts {
		parts[i] = alias + "." + p
	}
	return strings.Join(parts, ", ")
}
