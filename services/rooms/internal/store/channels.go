// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/rooms/internal/domain"
)

const channelCols = `id, team_id, slug, name, topic, purpose, kind, archived, created_by, created_at, updated_at, last_message_at`

func scanChannel(row pgx.Row, t string) (domain.Channel, error) {
	var c domain.Channel
	var kind string
	if err := row.Scan(&c.ID, &c.TeamID, &c.Slug, &c.Name, &c.Topic, &c.Purpose, &kind,
		&c.Archived, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.LastMessage); err != nil {
		return domain.Channel{}, err
	}
	c.Kind = domain.ChannelKind(kind)
	c.Tenant = t
	return c, nil
}

// NewChannelRecord son los datos para crear un canal.
type NewChannelRecord struct {
	TeamID    uuid.UUID
	Slug      string
	Name      string
	Topic     string
	Kind      domain.ChannelKind
	CreatedBy string
	// Members son subs a sembrar como miembros (canales privados/directos).
	Members []string
}

// CreateChannel crea un canal en un team (el actor debe ser miembro del team) y
// siembra a su creador y a los miembros indicados.
func (s *Store) CreateChannel(ctx context.Context, rec NewChannelRecord) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, rec.TeamID, rec.CreatedBy); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO channels (tenant, team_id, slug, name, topic, kind, created_by)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+channelCols,
			t.Slug, rec.TeamID, rec.Slug, strings.TrimSpace(rec.Name), rec.Topic,
			string(rec.Kind), rec.CreatedBy)
		var e error
		ch, e = scanChannel(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		if e != nil {
			return e
		}
		// Sembrar miembros (creador admin + el resto member).
		seed := map[string]domain.MemberRole{rec.CreatedBy: domain.RoleAdmin}
		for _, sub := range rec.Members {
			if sub != "" && sub != rec.CreatedBy {
				seed[sub] = domain.RoleMember
			}
		}
		for sub, role := range seed {
			if _, err := tx.Exec(ctx,
				`INSERT INTO channel_members (tenant, channel_id, subject, role)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
				t.Slug, ch.ID, sub, string(role)); err != nil {
				return err
			}
		}
		return nil
	})
	return ch, err
}

// defaultChannels son los canales que nacen con cada team y a los que se auto-une
// todo miembro nuevo (el equivalente al «Town Square»/«Off-Topic» de Mattermost).
// Sin ellos, el primer usuario de un tenant entraba a un producto vacío y sin salida.
//
// ponytail: la marca de «canal por defecto» es el SLUG —ya único por team y clave de
// idempotencia del INSERT—, no una columna nueva: cero migración. Si algún día la
// lista ha de ser configurable por tenant, ahí sí toca tabla.
var defaultChannels = [...]struct{ slug, name, topic string }{
	{"general", "General", "Canal general del equipo"},
	{"aleatorio", "Aleatorio", "Conversación informal, fuera de tema"},
}

// seedDefaultChannels crea los canales por defecto de un team recién creado, en la
// MISMA transacción. Idempotente (ON CONFLICT DO NOTHING sobre channels_unique_slug):
// si el equipo ya tiene un canal con ese slug, se respeta el existente.
//
// El creador no se marca admin de canal: requireChannelAdmin ya cae al rol del TEAM,
// así que un admin del equipo administra sus canales por defecto sin fila extra.
func seedDefaultChannels(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, creator string) error {
	for _, d := range defaultChannels {
		if _, err := tx.Exec(ctx,
			`INSERT INTO channels (tenant, team_id, slug, name, topic, kind, created_by)
			 VALUES ($1, $2, $3, $4, $5, 'public', $6)
			 ON CONFLICT (tenant, team_id, slug) WHERE deleted_at IS NULL DO NOTHING`,
			tenant, teamID, d.slug, d.name, d.topic, creator); err != nil {
			return err
		}
	}
	return nil
}

// joinDefaultChannels auto-une a un miembro del team a los canales por defecto VIVOS,
// en la MISMA transacción que su alta en el equipo. Idempotente. Si el team no los
// tiene (creado antes de esta ola, o archivados/renombrados por su equipo) no hace
// nada: no se resucita nada, solo se engancha a lo que existe.
func joinDefaultChannels(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, subject, name string) error {
	for _, d := range defaultChannels {
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
			 SELECT c.tenant, c.id, $3, $4, 'member' FROM channels c
			  WHERE c.tenant=$1 AND c.team_id=$2 AND c.slug=$5
			    AND c.kind='public' AND c.archived=false AND c.deleted_at IS NULL
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
			tenant, teamID, subject, name, d.slug); err != nil {
			return err
		}
	}
	return nil
}

// ListChannels lista la barra lateral del usuario en un team: los canales (de
// cualquier tipo) de los que es MIEMBRO. Los públicos a los que no pertenece no
// salen aquí —se encuentran con ListJoinableChannels—: antes salían todos, y un
// canal público del que alguien se había ido le volvía a aparecer al recargar.
// Los directos que el usuario ha borrado para sí (hidden_at, mig 0058) tampoco.
func (s *Store) ListChannels(ctx context.Context, teamID uuid.UUID, subject string, includeArchived bool) ([]domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, teamID, subject); err != nil {
			return err
		}
		// Nombre a mostrar: en canales directos (DM) es el del OTRO miembro (per-visor),
		// no el almacenado; lo resuelve un subselect a channel_members. El resto usa c.name.
		// El nombre a mostrar de un DM es el de los OTROS miembros (per-visor). Se
		// recogen sus nombres en un array (peers) con un solo agregado correlacionado
		// —igual coste que el subselect anterior, sin N+1— y el display se compone en
		// Go: para un 1:1 es el nombre del peer; para un grupo, «Ana, Beto, Caro».
		rows, err := tx.Query(ctx,
			`SELECT c.id, c.team_id, c.slug, c.name,
			        c.topic, c.purpose, c.kind, c.archived, c.created_by, c.created_at, c.updated_at, c.last_message_at,
			        COALESCE(mem.favorite, false), COALESCE(mem.notify_level, 'all'),
			        mem.category_id, COALESCE(mem.sort_order, 0),
			        COALESCE((
			             SELECT array_agg(COALESCE(NULLIF(cm2.name, ''), cm2.subject) ORDER BY cm2.name, cm2.subject)
			             FROM channel_members cm2
			             WHERE cm2.tenant=c.tenant AND cm2.channel_id=c.id AND cm2.subject <> $3
			               AND c.kind = 'direct'), '{}'::text[]),
			        COALESCE((
			             SELECT array_agg(cm3.subject ORDER BY cm3.name, cm3.subject)
			             FROM channel_members cm3
			             WHERE cm3.tenant=c.tenant AND cm3.channel_id=c.id AND cm3.subject <> $3
			               AND c.kind = 'direct'), '{}'::text[])
			 FROM channels c
			 JOIN channel_members mem
			   ON mem.tenant=c.tenant AND mem.channel_id=c.id AND mem.subject=$3
			 WHERE c.tenant=$1 AND c.team_id=$2 AND c.deleted_at IS NULL
			   AND ($4 OR c.archived = false)
			   AND mem.hidden_at IS NULL
			 ORDER BY c.last_message_at DESC NULLS LAST, lower(c.name)`,
			t.Slug, teamID, subject, includeArchived)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c domain.Channel
			var kind, notifyLevel string
			var peers, peerSubs []string
			if err := rows.Scan(&c.ID, &c.TeamID, &c.Slug, &c.Name, &c.Topic, &c.Purpose, &kind,
				&c.Archived, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt, &c.LastMessage,
				&c.Favorite, &notifyLevel, &c.CategoryID, &c.SortOrder, &peers, &peerSubs); err != nil {
				return err
			}
			c.Kind = domain.ChannelKind(kind)
			// notify_level es la fuente de verdad; Muted es su proyección (⟺ 'none').
			c.NotifyLevel = notifyLevel
			c.Muted = notifyLevel == domain.NotifyLevelNone
			c.Tenant = t.Slug
			if c.Kind == domain.ChannelDirect && len(peers) > 0 {
				c.Peers = peers
				c.PeerSubs = peerSubs // alineado con Peers (misma ORDER BY)
				c.Name = strings.Join(peers, ", ")
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// GetChannel devuelve un canal vivo accesible por el usuario.
func (s *Store) GetChannel(ctx context.Context, id uuid.UUID, subject string) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		var e error
		ch, e = getChannelTx(ctx, tx, t.Slug, id)
		if e != nil {
			return e
		}
		return requireChannelAccess(ctx, tx, t.Slug, ch, subject)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// getChannelTx lee un canal vivo por ID dentro de una transacción.
func getChannelTx(ctx context.Context, tx pgx.Tx, tenant string, id uuid.UUID) (domain.Channel, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+channelCols+` FROM channels WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL`, tenant, id)
	ch, err := scanChannel(row, tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// requireChannelAccess autoriza la lectura de un canal: miembro del team para
// públicos; miembro del canal Y del team para privados; miembro del canal para
// directos. Y en los tres, la persona de OTRA organización que se añadió a ESE
// canal (`externo`, ADR 0040): no está en ningún equipo de aquí, así que la marca
// sustituye al requisito del equipo y solo para ese canal.
//
// El doble requisito en PRIVADOS es la verja de la baja de equipo: antes bastaba
// la fila de channel_members, así que a quien se quitaba del team le quedaba
// lectura (y escritura) de todos sus canales privados entrando por el channelID.
// RemoveTeamMember además limpia esas filas; esto lo hace robusto aunque la
// limpieza falle o los datos deriven. Un miembro de canal privado es siempre
// miembro del team (se siembra al crear el canal y JoinChannel rechaza privados),
// así que no veta ningún acceso legítimo.
//
// Los DIRECTOS (DM) se dejan a propósito fuera: como en Mattermost/Slack, un DM es
// de ámbito de organización y no muere al salir de UN team — lo que corta el acceso
// a los DM es dar de baja la CUENTA en Sytue ID, no dejar un equipo.
func requireChannelAccess(ctx context.Context, tx pgx.Tx, tenant string, ch domain.Channel, subject string) error {
	if ch.Kind == domain.ChannelPublic {
		err := requireTeamMember(ctx, tx, tenant, ch.TeamID, subject)
		if !errors.Is(err, domain.ErrForbidden) {
			return err
		}
		if externo, merr := miembroDelCanal(ctx, tx, tenant, ch.ID, subject); merr != nil || !externo {
			return domain.ErrForbidden
		}
		return nil
	}
	externo, err := miembroDelCanal(ctx, tx, tenant, ch.ID, subject)
	if err != nil {
		return err
	}
	if ch.Kind == domain.ChannelPrivate && !externo && !domain.IsBotAuthor(subject) {
		return requireTeamMember(ctx, tx, tenant, ch.TeamID, subject)
	}
	return nil
}

// requireChannelMember falla con ErrForbidden si el usuario no es miembro del canal.
func requireChannelMember(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string) error {
	_, err := miembroDelCanal(ctx, tx, tenant, channelID, subject)
	return err
}

// miembroDelCanal comprueba la pertenencia al canal y dice si es de otra
// organización. ErrForbidden si no es miembro. Una sola consulta para las dos
// cosas: las verjas la hacen en cada lectura y cada mensaje.
func miembroDelCanal(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string) (bool, error) {
	var externo bool
	err := tx.QueryRow(ctx,
		`SELECT externo FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
		tenant, channelID, subject).Scan(&externo)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, domain.ErrForbidden
	}
	return externo, err
}

// requireWriteAccess autoriza publicar en un canal: el usuario debe ser miembro
// del canal y el canal no puede estar archivado. Para canales públicos, si el
// usuario aún no es miembro explícito pero pertenece al team, se le une (join on
// first post).
func requireWriteAccess(ctx context.Context, tx pgx.Tx, tenant string, ch domain.Channel, subject, name string) error {
	if ch.Archived {
		return domain.ErrConflict
	}
	externo, err := miembroDelCanal(ctx, tx, tenant, ch.ID, subject)
	if err != nil {
		if !errors.Is(err, domain.ErrForbidden) || ch.Kind != domain.ChannelPublic {
			return err
		}
		// Canal público: exigir pertenencia al team y auto-unir.
		if err := requireTeamMember(ctx, tx, tenant, ch.TeamID, subject); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, 'member')
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
			tenant, ch.ID, subject, name); err != nil {
			return err
		}
		// Ya se verificó el team en esta rama.
		return nil
	}
	// Publicar en un canal del TEAM (público o privado) exige seguir en el team: con
	// solo la fila de channel_members, a quien se quitaba del equipo le quedaba
	// escritura en sus canales. Los DM quedan fuera a propósito (ámbito de
	// organización, ver requireChannelAccess), las INTEGRACIONES también —un bot o
	// webhook es una identidad sintética (bot:/hook:) que por diseño no está en
	// team_members: su autorización es la membresía explícita del canal más su
	// token— y la persona de otra organización, por lo mismo que al leer.
	if ch.Kind != domain.ChannelDirect && !externo && !domain.IsBotAuthor(subject) {
		return requireTeamMember(ctx, tx, tenant, ch.TeamID, subject)
	}
	return nil
}

// UpdateChannelRecord son los cambios opcionales de un canal.
type UpdateChannelRecord struct {
	Name     *string
	Topic    *string
	Purpose  *string
	Archived *bool
}

// UpdateChannel renombra/retematiza/archiva un canal. Solo un admin del canal o
// del team puede hacerlo.
func (s *Store) UpdateChannel(ctx context.Context, id uuid.UUID, actor string, upd UpdateChannelRecord) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		cur, err := getChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, cur, actor); err != nil {
			return err
		}
		name := cur.Name
		topic := cur.Topic
		purpose := cur.Purpose
		archived := cur.Archived
		if upd.Name != nil {
			if !domain.ValidName(*upd.Name) {
				return domain.ErrInvalidName
			}
			name = strings.TrimSpace(*upd.Name)
		}
		if upd.Topic != nil {
			topic = *upd.Topic
		}
		if upd.Purpose != nil {
			purpose = *upd.Purpose
		}
		if upd.Archived != nil {
			archived = *upd.Archived
		}
		row := tx.QueryRow(ctx,
			`UPDATE channels SET name=$3, topic=$4, purpose=$5, archived=$6, updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL RETURNING `+channelCols,
			t.Slug, id, name, topic, purpose, archived)
		ch, err = scanChannel(row, t.Slug)
		if err != nil {
			return err
		}
		return s.revokeMatrixTx(ctx, tx, t.Slug, id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// requireChannelAdmin autoriza administrar un canal (admin del canal o del team).
func requireChannelAdmin(ctx context.Context, tx pgx.Tx, tenant string, ch domain.Channel, subject string) error {
	var role string
	err := tx.QueryRow(ctx,
		`SELECT role FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
		tenant, ch.ID, subject).Scan(&role)
	if err == nil && domain.MemberRole(role).CanManage() {
		return nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	tr, err := teamRole(ctx, tx, tenant, ch.TeamID, subject)
	if err != nil {
		return err
	}
	if !tr.CanManage() {
		return domain.ErrForbidden
	}
	return nil
}

// JoinChannel añade al usuario como miembro de un canal público de su team.
func (s *Store) JoinChannel(ctx context.Context, id uuid.UUID, subject, name string) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		cur, err := getChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if cur.Kind != domain.ChannelPublic {
			return domain.ErrForbidden
		}
		if err := requireTeamMember(ctx, tx, t.Slug, cur.TeamID, subject); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, 'member')
			 ON CONFLICT (tenant, channel_id, subject) DO UPDATE SET name=EXCLUDED.name`,
			t.Slug, id, subject, name); err != nil {
			return err
		}
		ch = cur
		return nil
	})
	return ch, err
}

// LeaveChannel elimina al usuario de la membresía de un canal.
func (s *Store) LeaveChannel(ctx context.Context, id uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		if err := lockMatrixChannelTx(ctx, tx, t.Slug, id); err != nil {
			return err
		}
		if err := ultimoAdminTx(ctx, tx, t.Slug, id, subject); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
			t.Slug, id, subject)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return s.revokeMatrixTx(ctx, tx, t.Slug, id)
	})
}

// ultimoAdminTx devuelve ErrLastAdmin (un ErrConflict) si subject es el ÚLTIMO admin de un canal
// PRIVADO y aún quedan otros miembros: si se fuera, el canal se quedaría sin nadie
// que lo administre desde dentro. Que nombre antes a otro admin, o que lo borre.
// En públicos y directos no aplica (un público lo administra el equipo; un
// directo no tiene roles).
func ultimoAdminTx(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject string) error {
	var ultimo bool
	err := tx.QueryRow(ctx,
		`SELECT cm.role = 'admin' AND c.kind = 'private'
		    AND NOT EXISTS (SELECT 1 FROM channel_members o
		                     WHERE o.tenant=cm.tenant AND o.channel_id=cm.channel_id
		                       AND o.subject<>cm.subject AND o.role='admin')
		    AND EXISTS (SELECT 1 FROM channel_members o
		                 WHERE o.tenant=cm.tenant AND o.channel_id=cm.channel_id
		                   AND o.subject<>cm.subject)
		   FROM channel_members cm
		   JOIN channels c ON c.tenant=cm.tenant AND c.id=cm.channel_id
		  WHERE cm.tenant=$1 AND cm.channel_id=$2 AND cm.subject=$3`,
		tenant, channelID, subject).Scan(&ultimo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no es miembro: quien llama ya contesta ErrNotFound
	}
	if err != nil {
		return err
	}
	if ultimo {
		return domain.ErrLastAdmin
	}
	return nil
}

// DeleteChannel borra (en blando: deleted_at) un canal público o privado. Solo un
// admin del canal o del equipo; los directos nunca —«borrar un chat» es
// HideDirect, que solo lo quita para quien lo pide—. Devuelve ErrNotFound si el
// canal no existe o ya estaba borrado.
//
// El cerrojo y la revocación de Matrix/llamadas van ANTES del UPDATE: los dos
// buscan el canal con deleted_at IS NULL y, después, ya no lo encontrarían.
func (s *Store) DeleteChannel(ctx context.Context, id uuid.UUID, actor string) error {
	t := tenancy.MustFrom(ctx)
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		if err := lockMatrixChannelTx(ctx, tx, t.Slug, id); err != nil {
			return err
		}
		cur, err := getChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if cur.Kind == domain.ChannelDirect {
			return domain.ErrForbidden
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, cur, actor); err != nil {
			return err
		}
		if err := s.revokeMatrixTx(ctx, tx, t.Slug, id); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`UPDATE channels SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL AND kind <> 'direct'`,
			t.Slug, id)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// HideDirect borra un mensaje directo SOLO para subject, como en WhatsApp: sale de
// su barra lateral (hidden_at) y su historial anterior deja de verse para él
// (cleared_before). Los demás conservan el suyo. Si alguien vuelve a escribir, el
// directo reaparece con solo lo nuevo (createMessageTx limpia hidden_at). Solo
// directos y solo un miembro: ErrForbidden en otro caso.
func (s *Store) HideDirect(ctx context.Context, id uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		cur, err := getChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if cur.Kind != domain.ChannelDirect {
			return domain.ErrForbidden
		}
		tag, err := tx.Exec(ctx,
			`UPDATE channel_members SET hidden_at=now(), cleared_before=now()
			 WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
			t.Slug, id, subject)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrForbidden
		}
		return nil
	})
}

// reabrirDirectoTx vuelve a poner a subject en un directo que ya existía: quita su
// hidden_at (si lo había borrado para sí) y le da de alta si se había ido. Sin
// esto, un directo del que alguien salió no se podía volver a abrir. cleared_before
// se conserva: lo borrado sigue borrado.
func reabrirDirectoTx(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID, subject, name string) error {
	_, err := tx.Exec(ctx,
		`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
		 VALUES ($1, $2, $3, $4, 'member')
		 ON CONFLICT (tenant, channel_id, subject) DO UPDATE SET hidden_at = NULL`,
		tenant, channelID, subject, name)
	return err
}

// ChannelMemberSubs devuelve los subs de los miembros de un canal (para fan-out
// de notificaciones de presencia/menciones).
func (s *Store) ChannelMemberSubs(ctx context.Context, id uuid.UUID) ([]string, error) {
	t := tenancy.MustFrom(ctx)
	var out []string
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT subject FROM channel_members WHERE tenant=$1 AND channel_id=$2`, t.Slug, id)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var sub string
			if err := rows.Scan(&sub); err != nil {
				return err
			}
			out = append(out, sub)
		}
		return rows.Err()
	})
	return out, err
}

// ---------------------------------------------------------------------------
// Miembros de canal (roster)
// ---------------------------------------------------------------------------
//
// El rol de canal (channel_members.role, admin|member) existía en el esquema y lo
// leía requireChannelAdmin, pero NADIE lo escribía: era inalcanzable. Y los miembros
// de un canal privado solo podían sembrarse en el INSERT de creación (JoinChannel
// rechaza los privados), así que un canal privado nacía congelado.
//
// Invariante que comparten las tres mutaciones: los DIRECTOS (DM) no tienen roster
// gestionable. Su identidad ES el conjunto de participantes (slug determinista, ver
// directSlugN), así que tocarlo rompería la idempotencia; y como requireChannelAdmin
// cae al rol del TEAM cuando no hay fila de canal, sin esta puerta un admin del equipo
// podría colarse en cualquier conversación privada. Para salir de un DM está el
// autoservicio (actor == subject, equivalente a LeaveChannel).

// ListChannelMembers lista los miembros de un canal, exigiendo ACCESO al canal
// (miembro del team en públicos; miembro del canal —y del team— en privados). Orden
// estable por nombre y sub, igual que ListTeamMembers.
//
// ponytail: sin paginación, como ListTeamMembers (el listado análogo del repo): un
// roster es acotado y el cliente lo pinta entero. Si un canal llegara a decenas de
// miles de miembros, keyset por (lower(name), subject) como en ListSaved.
func (s *Store) ListChannelMembers(ctx context.Context, channelID uuid.UUID, actor string) ([]domain.Member, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.Member
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT subject, name, role, joined_at, externo FROM channel_members
			 WHERE tenant=$1 AND channel_id=$2 ORDER BY lower(name), subject`,
			t.Slug, channelID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var m domain.Member
			var role string
			if err := rows.Scan(&m.Subject, &m.Name, &role, &m.JoinedAt, &m.External); err != nil {
				return err
			}
			m.Role = domain.MemberRole(role)
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// AddChannelMember añade a alguien al canal («Añadir personas»). Gate: admin del
// canal O del team (requireChannelAdmin). El añadido DEBE ser ya miembro del TEAM
// —ErrNotMember si no—: las verjas de lectura y escritura exigen el team además del
// canal (ver requireChannelAccess), así que su fila sería una membresía muerta que
// además ocuparía el roster. Si la petición no trae nombre, se usa el del equipo.
// Idempotente: re-añadir a un miembro refresca su nombre y conserva su rol.
func (s *Store) AddChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject, name string) (domain.Member, error) {
	return s.anadirAlCanal(ctx, channelID, actor, subject, name, false)
}

// AddExternalChannelMember añade al canal a una persona de OTRA organización
// (ADR 0040). Misma verja que AddChannelMember; lo que cambia es que no se le
// exige el equipo —no puede estar en uno de aquí— y que su fila lleva `externo`,
// que es lo que le abre las verjas de ESTE canal. `name` tiene que venir de Sytue
// ID, no del navegador: es como la verán todos los miembros. Si resulta que sí es
// del equipo, entra como cualquiera.
func (s *Store) AddExternalChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject, name string) (domain.Member, error) {
	return s.anadirAlCanal(ctx, channelID, actor, subject, name, true)
}

func (s *Store) anadirAlCanal(ctx context.Context, channelID uuid.UUID, actor, subject, name string, deFuera bool) (domain.Member, error) {
	t := tenancy.MustFrom(ctx)
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return domain.Member{}, domain.ErrInvalidInput
	}
	var m domain.Member
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if ch.Kind == domain.ChannelDirect {
			return domain.ErrForbidden
		}
		if ch.Archived {
			return domain.ErrConflict
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		// Pertenencia al team del añadido + su nombre a mostrar, en una sola consulta.
		teamName, err := teamMemberName(ctx, tx, t.Slug, ch.TeamID, subject)
		externo := false
		switch {
		case errors.Is(err, domain.ErrForbidden) && deFuera:
			externo = true
		case errors.Is(err, domain.ErrForbidden):
			return domain.ErrNotMember
		case err != nil:
			return err
		}
		if strings.TrimSpace(name) == "" {
			name = teamName
		}
		// De fuera y aún no está: se le INVITA (mig 0041). No es miembro hasta que acepte.
		if externo {
			if _, noEsta := miembroDelCanal(ctx, tx, t.Slug, ch.ID, subject); errors.Is(noEsta, domain.ErrForbidden) {
				m = domain.Member{Subject: subject, Name: name, Role: domain.RoleMember, External: true, Pending: true}
				return invitar(ctx, tx, t.Slug, ch.ID, subject, name, actor, nombreEnElCanal(ctx, tx, t.Slug, ch.ID, actor))
			}
		}
		var role string
		if err := tx.QueryRow(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role, externo)
			 VALUES ($1, $2, $3, $4, 'member', $5)
			 ON CONFLICT (tenant, channel_id, subject) DO UPDATE SET name=EXCLUDED.name
			 RETURNING subject, name, role, joined_at, externo`,
			t.Slug, channelID, subject, name, externo).Scan(&m.Subject, &m.Name, &role, &m.JoinedAt, &m.External); err != nil {
			return err
		}
		m.Role = domain.MemberRole(role)
		return nil
	})
	return m, err
}

// RemoveChannelMember saca a alguien del canal. Gate: admin del canal o del team; o
// uno mismo (autoservicio, equivalente a LeaveChannel). ErrNotFound si no era miembro.
func (s *Store) RemoveChannelMember(ctx context.Context, channelID uuid.UUID, actor, subject string) error {
	t := tenancy.MustFrom(ctx)
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return domain.ErrInvalidInput
	}
	return s.enMantenimientoDe(ctx, func(tx pgx.Tx, _ string) error {
		if err := lockMatrixChannelTx(ctx, tx, t.Slug, channelID); err != nil {
			return err
		}
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if actor != subject {
			if ch.Kind == domain.ChannelDirect {
				return domain.ErrForbidden
			}
			if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
				return err
			}
		} else if err := ultimoAdminTx(ctx, tx, t.Slug, channelID, subject); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM channel_members WHERE tenant=$1 AND channel_id=$2 AND subject=$3`,
			t.Slug, channelID, subject)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return s.revokeMatrixTx(ctx, tx, t.Slug, channelID)
	})
}

// SetChannelMemberRole fija el rol de un miembro del canal (admin|member): la única
// vía para que exista un «admin de canal». Gate: admin del canal o del team.
// ErrNotFound si el sujeto no es miembro del canal.
func (s *Store) SetChannelMemberRole(ctx context.Context, channelID uuid.UUID, actor, subject string, role domain.MemberRole) (domain.Member, error) {
	t := tenancy.MustFrom(ctx)
	subject = strings.TrimSpace(subject)
	if subject == "" || !domain.ValidMemberRole(role) {
		return domain.Member{}, domain.ErrInvalidInput
	}
	var m domain.Member
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		ch, err := getChannelTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		if ch.Kind == domain.ChannelDirect {
			return domain.ErrForbidden
		}
		if err := requireChannelAdmin(ctx, tx, t.Slug, ch, actor); err != nil {
			return err
		}
		var got string
		err = tx.QueryRow(ctx,
			`UPDATE channel_members SET role=$4
			 WHERE tenant=$1 AND channel_id=$2 AND subject=$3
			 RETURNING subject, name, role, joined_at`,
			t.Slug, channelID, subject, string(role)).Scan(&m.Subject, &m.Name, &got, &m.JoinedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		m.Role = domain.MemberRole(got)
		return nil
	})
	return m, err
}

// SetChannelPref fija las preferencias per-usuario de un canal: favorito, y la
// política de aviso (notifyLevel: 'all'|'mentions'|'none'). Punteros nil = sin cambio.
// notify_level es la fuente de verdad; el `muted` legado se traduce (true→'none',
// false→'all') y el atenuado del sidebar (Muted) se DERIVA como notify_level='none' en
// la misma fila, para que ningún lector tenga que reconciliar dos columnas. Requiere
// acceso al canal; para un canal público del que aún no eres miembro, marcar una
// preferencia crea la fila de membresía (como Mattermost). Devuelve el canal con
// Favorite/NotifyLevel/Muted resueltos.
func (s *Store) SetChannelPref(ctx context.Context, id uuid.UUID, subject, name string, favorite, muted *bool, notifyLevel *string) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	// Un notifyLevel explícito manda; si no, un `muted` legado se traduce a 'none'/'all'.
	var levelArg *string
	switch {
	case notifyLevel != nil:
		levelArg = notifyLevel
	case muted != nil:
		lvl := domain.NotifyLevelAll
		if *muted {
			lvl = domain.NotifyLevelNone
		}
		levelArg = &lvl
	}
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		cur, err := getChannelTx(ctx, tx, t.Slug, id)
		if err != nil {
			return err
		}
		if err := requireChannelAccess(ctx, tx, t.Slug, cur, subject); err != nil {
			return err
		}
		// Asegura la fila de membresía (público no-miembro que marca preferencia).
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
			 VALUES ($1, $2, $3, $4, 'member')
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
			t.Slug, id, subject, name); err != nil {
			return err
		}
		var fav bool
		var level string
		if err := tx.QueryRow(ctx,
			`UPDATE channel_members
			 SET favorite = COALESCE($4, favorite),
			     notify_level = COALESCE($5, notify_level),
			     muted = (COALESCE($5, notify_level) = 'none')
			 WHERE tenant=$1 AND channel_id=$2 AND subject=$3
			 RETURNING favorite, notify_level`,
			t.Slug, id, subject, favorite, levelArg).Scan(&fav, &level); err != nil {
			return err
		}
		cur.Favorite = fav
		cur.NotifyLevel = level
		cur.Muted = level == domain.NotifyLevelNone
		ch = cur
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// ListJoinableChannels lista canales PÚBLICOS del team a los que el usuario aún no
// pertenece (para «explorar canales»), filtrando por nombre (ILIKE) y con LIMIT.
// Requiere pertenecer al team. Coste acotado por LIMIT.
func (s *Store) ListJoinableChannels(ctx context.Context, teamID uuid.UUID, subject, query string, limit int) ([]domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	// Escapa los comodines ILIKE del usuario (ver likePattern, compartido con la
	// búsqueda de mensajes).
	pattern := likePattern(query)
	var out []domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := requireTeamMember(ctx, tx, t.Slug, teamID, subject); err != nil {
			return err
		}
		// ponytail: ILIKE + LIMIT basta para explorar; si un team acumula miles de
		// canales públicos, migrar a trigram (pg_trgm ya vive en la mig. 0002).
		rows, err := tx.Query(ctx,
			`SELECT `+prefixed("c", channelCols)+` FROM channels c
			 WHERE c.tenant=$1 AND c.team_id=$2 AND c.deleted_at IS NULL
			   AND c.archived=false AND c.kind='public'
			   AND NOT EXISTS (
			        SELECT 1 FROM channel_members m
			        WHERE m.tenant=c.tenant AND m.channel_id=c.id AND m.subject=$3)
			   AND ($4 = '' OR plegar(c.name) LIKE plegar($5))
			 ORDER BY lower(c.name) LIMIT $6`,
			t.Slug, teamID, subject, query, pattern, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanChannel(rows, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	return out, err
}

// directSlugN deriva el slug determinista de un DM (1:1 o de grupo): dm-<hash de
// TODOS los subjects ORDENADOS>. Ordenar hace la clave simétrica e independiente del
// orden: el MISMO conjunto reusa el mismo canal, lo abra quien lo abra y en cualquier
// orden. Generaliza directSlug — para dos subs produce idéntico hash, así que un DM
// 1:1 cae en el mismo canal por cualquiera de las dos vías (retrocompatible). 128 bits
// (32 hex) evitan colisiones y caben en el límite de 64 del slug; la unicidad la
// garantiza channels_unique_slug (tenant, team_id, slug).
func directSlugN(subs ...string) string {
	ordered := append([]string(nil), subs...)
	sort.Strings(ordered)
	sum := sha256.Sum256([]byte(strings.Join(ordered, "\x00")))
	return "dm-" + hex.EncodeToString(sum[:])[:32]
}

// directSlug es el slug de un DM 1:1 (el caso de dos participantes de directSlugN).
func directSlug(a, b string) string {
	return directSlugN(a, b)
}

// teamMemberName devuelve el nombre del miembro del team y confirma la pertenencia
// (ErrForbidden si no lo es).
func teamMemberName(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, subject string) (string, error) {
	var name string
	err := tx.QueryRow(ctx,
		`SELECT name FROM team_members WHERE tenant=$1 AND team_id=$2 AND subject=$3`,
		tenant, teamID, subject).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrForbidden
	}
	return name, err
}

// nombresDelGrupo recoge el nombre de cada participante de un DM de grupo (subs[0]
// es quien lo abre) y dice cuáles son de otra organización. Los del team, con su
// nombre del team; los de `externos`, con el que dio Sytue ID; cualquier otro,
// ErrForbidden. Quien lo abre tiene que ser del team: la conversación vive aquí.
func nombresDelGrupo(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, subs []string, externos map[string]string) ([]string, []bool, error) {
	names := make([]string, len(subs))
	deFuera := make([]bool, len(subs))
	for i, sub := range subs {
		n, err := teamMemberName(ctx, tx, tenant, teamID, sub)
		if nombre, ok := externos[sub]; ok && i > 0 && errors.Is(err, domain.ErrForbidden) {
			n, err, deFuera[i] = nombre, nil, true
		}
		if err != nil {
			return nil, nil, err
		}
		names[i] = n
	}
	return names, deFuera, nil
}

// getDirectBySlug lee el canal directo (DM) de un team por su slug determinista.
func getDirectBySlug(ctx context.Context, tx pgx.Tx, tenant string, teamID uuid.UUID, slug string) (domain.Channel, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+channelCols+` FROM channels
		 WHERE tenant=$1 AND team_id=$2 AND slug=$3 AND kind='direct' AND deleted_at IS NULL`,
		tenant, teamID, slug)
	return scanChannel(row, tenant)
}

// OpenDirect abre —idempotente— el canal directo (DM 1:1) entre el actor y otherSub
// dentro del team dado. Clave idempotente: el slug determinista directSlug, único por
// (tenant, team_id, slug), así que un par tiene un único DM por team. Ambos han de
// pertenecer al team; se siembran como miembros del canal. El nombre a mostrar es
// per-visor (el del OTRO miembro) y lo resuelve ListChannels.
//
// ponytail: DM acotado al team (el mismo par en dos teams tendría dos hilos);
// aceptable v1. Cross-team pediría una tabla de conversaciones fuera del team.
func (s *Store) OpenDirect(ctx context.Context, teamID uuid.UUID, actorSub, otherSub string) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	if otherSub == "" || otherSub == actorSub {
		return domain.Channel{}, domain.ErrInvalidInput
	}
	slug := directSlug(actorSub, otherSub)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		actorName, err := teamMemberName(ctx, tx, t.Slug, teamID, actorSub)
		if err != nil {
			return err
		}
		otherName, err := teamMemberName(ctx, tx, t.Slug, teamID, otherSub)
		if err != nil {
			return err
		}
		// Reusar si ya existe (idempotencia), devolviéndolo a la barra de quien lo abre.
		if existing, err := getDirectBySlug(ctx, tx, t.Slug, teamID, slug); err == nil {
			ch = existing
			return reabrirDirectoTx(ctx, tx, t.Slug, ch.ID, actorSub, actorName)
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// Crear. El name almacenado = otherName como respaldo no vacío (el display se
		// resuelve per-visor en ListChannels; la columna no queda en blanco).
		fallback := strings.TrimSpace(otherName)
		if fallback == "" {
			fallback = otherSub
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO channels (tenant, team_id, slug, name, kind)
			 VALUES ($1, $2, $3, $4, 'direct') RETURNING `+channelCols,
			t.Slug, teamID, slug, fallback)
		ch, err = scanChannel(row, t.Slug)
		if uniqueViolation(err) {
			// Carrera: otro proceso creó el mismo DM. Releer el ganador.
			ch, err = getDirectBySlug(ctx, tx, t.Slug, teamID, slug)
			return err
		}
		if err != nil {
			return err
		}
		// Sembrar ambos miembros con su nombre (para resolver el display del peer).
		for _, m := range [...]struct{ sub, name string }{{actorSub, actorName}, {otherSub, otherName}} {
			if _, err := tx.Exec(ctx,
				`INSERT INTO channel_members (tenant, channel_id, subject, name, role)
				 VALUES ($1, $2, $3, $4, 'member')
				 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
				t.Slug, ch.ID, m.sub, m.name); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// OpenGroupDirect abre —idempotente— un DM directo con N participantes (grupo, 3..8; o
// un 1:1 si otherSubs trae uno solo) entre el actor y otherSubs dentro del team. La
// clave idempotente es directSlugN sobre el CONJUNTO completo de subjects (actor
// incluido), único por (tenant, team_id, slug): el mismo grupo, en cualquier orden,
// reusa el mismo canal. Generaliza OpenDirect (dos subs producen el mismo slug, así que
// un 1:1 abierto por cualquiera de las dos vías cae en el mismo canal). Valida 2..8
// participantes totales, sin duplicados ni el actor repetido, y que todos pertenezcan
// al team (ErrForbidden si alguno no lo es). El display a mostrar lo resuelve
// ListChannels per-visor (los OTROS miembros).
//
// `externos` son las personas de OTRA organización del conjunto (sub → nombre que
// dio Sytue ID, ADR 0040): no están en el team y entran igual, con su fila
// `externo`. Quien abre la conversación sí tiene que ser del team: la conversación
// vive en SU organización.
func (s *Store) OpenGroupDirect(ctx context.Context, teamID uuid.UUID, actorSub string, otherSubs []string, externos map[string]string) (domain.Channel, error) {
	t := tenancy.MustFrom(ctx)
	// Conjunto = actor ∪ otros, sin vacíos ni duplicados (ni el actor repetido).
	seen := map[string]bool{actorSub: true}
	subs := []string{actorSub}
	for _, o := range otherSubs {
		o = strings.TrimSpace(o)
		if o == "" || seen[o] {
			return domain.Channel{}, domain.ErrInvalidInput
		}
		seen[o] = true
		subs = append(subs, o)
	}
	if len(subs) < 2 || len(subs) > 8 {
		return domain.Channel{}, domain.ErrInvalidInput
	}
	slug := directSlugN(subs...)
	var ch domain.Channel
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		// Todos deben pertenecer al team; recogemos sus nombres para sembrar la
		// membresía (el display per-visor lo resuelve ListChannels).
		names, deFuera, err := nombresDelGrupo(ctx, tx, t.Slug, teamID, subs, externos)
		if err != nil {
			return err
		}
		// Reusar si ya existe (idempotencia del conjunto), devolviéndolo a la barra
		// de quien lo abre (subs[0]).
		if existing, err := getDirectBySlug(ctx, tx, t.Slug, teamID, slug); err == nil {
			ch = existing
			return reabrirDirectoTx(ctx, tx, t.Slug, ch.ID, actorSub, names[0])
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		// name almacenado = respaldo no vacío (unión de nombres, cae al sub si falta);
		// el display real es per-visor en ListChannels.
		parts := make([]string, len(subs))
		for i := range subs {
			if strings.TrimSpace(names[i]) != "" {
				parts[i] = names[i]
			} else {
				parts[i] = subs[i]
			}
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO channels (tenant, team_id, slug, name, kind)
			 VALUES ($1, $2, $3, $4, 'direct') RETURNING `+channelCols,
			t.Slug, teamID, slug, strings.Join(parts, ", "))
		ch, err = scanChannel(row, t.Slug)
		if uniqueViolation(err) {
			// Carrera: otro proceso creó el mismo grupo. Releer el ganador.
			ch, err = getDirectBySlug(ctx, tx, t.Slug, teamID, slug)
			return err
		}
		if err != nil {
			return err
		}
		for i, fuera := range deFuera {
			if fuera {
				ch.Invitados = append(ch.Invitados, subs[i])
			}
		}
		return sembrarGrupo(ctx, tx, t.Slug, ch.ID, subs, names, deFuera)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Channel{}, domain.ErrNotFound
	}
	return ch, err
}

// sembrarGrupo da de alta a TODOS los del grupo con su nombre (para resolver el display).
// Los de otra organización no entran: se les invita, y entran al aceptar (mig 0041).
// subs[0] es quien abre la conversación.
func sembrarGrupo(ctx context.Context, tx pgx.Tx, tenant string, chID uuid.UUID, subs, names []string, deFuera []bool) error {
	for i, sub := range subs {
		if deFuera[i] {
			if err := invitar(ctx, tx, tenant, chID, sub, names[i], subs[0], names[0]); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO channel_members (tenant, channel_id, subject, name, role, externo)
			 VALUES ($1, $2, $3, $4, 'member', $5)
			 ON CONFLICT (tenant, channel_id, subject) DO NOTHING`,
			tenant, chID, sub, names[i], deFuera[i]); err != nil {
			return err
		}
	}
	return nil
}
