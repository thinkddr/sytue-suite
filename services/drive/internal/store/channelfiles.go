// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Adjuntos de canal de Sytue Rooms: persistencia del ÁMBITO DE CANAL (mig 0014).
// El contenido vive en las MISMAS tablas folders/files marcado con channel_id, y el
// AISLAMIENTO lo impone la RLS vía la GUC `app.channel` que fija inChannelTx —el
// mismo patrón que las unidades de equipo (`app.team_drive`, teamdrive.go)—. Un
// fichero de canal es INVISIBLE desde el ámbito personal, así que ninguna de las
// rutas existentes de Drive (raíz, hijos, búsqueda, recientes, favoritos, papelera,
// ZIP, WebDAV, compat Google/Graph, shares, enlace público) lo expone: no hace falta
// tocar ni una de sus consultas.
//
// QUIÉN puede leerlo NO se decide aquí: la verja de membresía del canal vive en el
// servicio (service/channelfiles.go), que la consulta a Sytue Rooms en cada petición.
package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// channelScopeSQL repite EN LA CONSULTA el ámbito de canal que ya impone la política
// de la mig 0014 (cinturón y tirantes, como el predicado `tenant=$1` junto a la RLS
// por tenant). Casa la columna con la GUC de la transacción: NULL con ámbito personal
// o de unidad de equipo, el canal en curso bajo inChannelTx. Se usa en las consultas
// que reciben un uuid del cliente (una carpeta, un fichero), donde acertar el id no
// debe bastar para leer un adjunto de un canal ajeno.
const channelScopeSQL = `channel_id IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid`

// ambitoSQL repite EN LA CONSULTA las políticas enteras de folders/files: la de
// ámbito (`*_scope_isolation`, mig 0010 y 0014) y la de mantenimiento (mig 0009),
// que se suman con OR. Lee las mismas variables de la transacción —vacías en «Mi
// unidad», la unidad bajo inTeamDriveTx, el canal bajo inChannelTx—, así que con la
// RLS activa no cambia nada: dice lo mismo dos veces.
//
// Existe para cuando la RLS NO se evalúa. PostgreSQL no aplica ninguna política a
// un superusuario ni a un rol con BYPASSRLS, y entonces lo único que separa «Mi
// unidad» de una unidad de equipo o de un canal privado es lo que diga el SQL.
// Va en TODA consulta de este paquete que entra en folders/files por organización
// (`WHERE tenant=$1 AND …`): por identificador, por carpeta o enumerando. Las
// ramas recursivas que solo descienden o suben desde una semilla ya acotada no lo
// repiten, y las dos comprobaciones de «¿sigue alguien usando este blob?» no lo
// llevan A PROPÓSITO (ver blobReferenced).
const ambitoSQL = `(current_setting('app.maintenance', true) = 'on'
	OR (team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
	AND ` + channelScopeSQL + `
	AND (team_drive_id IS NOT NULL OR channel_id IS NOT NULL OR owner_sub = coalesce(current_setting('app.owner_sub', true), ''))))`

// ambitoF, ambitoFi y ambitoFo son ambitoSQL para las consultas que nombran la
// tabla con alias (`f`, `fi`, `fo`). Escritas a mano y no compuestas: son tres.
const (
	ambitoF = `(current_setting('app.maintenance', true) = 'on'
	OR (f.team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
	AND f.channel_id IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
	AND (f.team_drive_id IS NOT NULL OR f.channel_id IS NOT NULL OR f.owner_sub = coalesce(current_setting('app.owner_sub', true), ''))))`
	ambitoFi = `(current_setting('app.maintenance', true) = 'on'
	OR (fi.team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
	AND fi.channel_id IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
	AND (fi.team_drive_id IS NOT NULL OR fi.channel_id IS NOT NULL OR fi.owner_sub = coalesce(current_setting('app.owner_sub', true), ''))))`
	ambitoFo = `(current_setting('app.maintenance', true) = 'on'
	OR (fo.team_drive_id IS NOT DISTINCT FROM nullif(current_setting('app.team_drive', true), '')::uuid
	AND fo.channel_id IS NOT DISTINCT FROM nullif(current_setting('app.channel', true), '')::uuid
	AND (fo.team_drive_id IS NOT NULL OR fo.channel_id IS NOT NULL OR fo.owner_sub = coalesce(current_setting('app.owner_sub', true), ''))))`
)

// inChannelTx abre una tx con `app.tenant` (RLS por tenant) y ADEMÁS `app.channel`
// fijado al id del canal (LOCAL: revierte al terminar la tx, sin fugas entre
// transacciones de la misma conexión del pool). Bajo esta tx, folders/files ven SOLO
// el contenido de ESE canal (política de la mig 0014): un fichero personal, de otra
// unidad o de OTRO canal simplemente no existe.
func (s *Store) inChannelTx(ctx context.Context, channelID uuid.UUID, fn func(tx pgx.Tx) error) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.channel', $1, true)", channelID.String()); err != nil {
			return fmt.Errorf("store: fijar app.channel: %w", err)
		}
		return fn(tx)
	})
}

// ensureChannelFolderTx devuelve (creándola si hace falta) la carpeta del canal
// DENTRO de la tx de ámbito. Es una raíz DESLIGADA del árbol personal (parent_id
// NULL, is_root false) igual que la raíz de una unidad de equipo, y su nombre es el
// uuid del canal: único entre las carpetas sin padre y sin filtrar el NOMBRE del
// canal (que sería una fuga de metadatos de un privado) a quien liste la BD.
func ensureChannelFolderTx(ctx context.Context, tx pgx.Tx, tenant string, channelID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx,
		`SELECT id FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND channel_id=$2 AND deleted_at IS NULL`,
		tenant, channelID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, fmt.Errorf("store: leer carpeta de canal: %w", err)
	}
	// Carrera entre dos primeras subidas del mismo canal: el índice único parcial
	// (tenant, channel_id) la resuelve; DO UPDATE no-op devuelve SIEMPRE la fila
	// ganadora, así que no hace falta reintentar el SELECT.
	err = tx.QueryRow(ctx,
		`INSERT INTO folders (tenant, parent_id, name, is_root, channel_id)
		 VALUES ($1, NULL, $2, false, $3)
		 ON CONFLICT (tenant, channel_id) WHERE channel_id IS NOT NULL AND deleted_at IS NULL
		 DO UPDATE SET updated_at = folders.updated_at
		 RETURNING id`,
		tenant, channelID.String(), channelID).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("store: crear carpeta de canal: %w", err)
	}
	return id, nil
}

// CreateChannelFile registra el metadato de un adjunto RECIÉN subido al canal:
// carpeta del canal (get-or-create), cuota reservada contra el tenant y fila marcada
// con channel_id (el WITH CHECK de la política lo exige).
//
// A diferencia de «Mi unidad», un nombre repetido NO sustituye al fichero existente
// (que en Rooms cambiaría el contenido de un adjunto YA publicado en un mensaje
// antiguo): se desambigua con «nombre (2)» reutilizando uniqueCopyName. Cada adjunto
// es, por tanto, un artefacto inmutable del mensaje que lo publicó.
func (s *Store) CreateChannelFile(ctx context.Context, channelID uuid.UUID, rec NewFileRecord) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(rec.Name) {
		return domain.File{}, domain.ErrInvalidName
	}
	if rec.ContentType == "" {
		rec.ContentType = "application/octet-stream"
	}
	var f domain.File
	err := s.inChannelTx(ctx, channelID, func(tx pgx.Tx) error {
		folderID, err := ensureChannelFolderTx(ctx, tx, t.Slug, channelID)
		if err != nil {
			return err
		}
		name, err := uniqueChannelName(ctx, tx, t.Slug, folderID, strings.TrimSpace(rec.Name))
		if err != nil {
			return err
		}
		if err := reserveQuota(ctx, tx, t.Slug, bytesEnCuota(rec.StorageKey, rec.SizeBytes)); err != nil {
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO files (tenant, folder_id, name, storage_key, size_bytes, content_type, checksum, owner_sub, modified_by, channel_id)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9) RETURNING `+fileCols,
			t.Slug, folderID, name, rec.StorageKey, rec.SizeBytes, rec.ContentType,
			rec.Checksum, rec.OwnerSub, channelID)
		var e error
		f, e = scanFile(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	return f, err
}

// uniqueChannelName devuelve un nombre libre en la carpeta del canal («foto.png»,
// «foto.png (2)», …) sondeando con el mismo helper que las copias.
func uniqueChannelName(ctx context.Context, tx pgx.Tx, tenant string, folderID uuid.UUID, name string) (string, error) {
	return uniqueCopyName(name, func(cand string) (bool, error) {
		var ex bool
		err := tx.QueryRow(ctx,
			`SELECT true FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND lower(name)=lower($3) AND deleted_at IS NULL`,
			tenant, folderID, cand).Scan(&ex)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return ex, err
	})
}

// ChannelFile lee un fichero VIVO del ámbito de ESE canal. Deny-by-default: un
// fichero personal, de otra unidad o de otro canal NO existe aquí (ErrNotFound, sin
// filtrar su existencia) porque la RLS de ámbito lo excluye — no basta con acertar
// el uuid del fichero, hay que acertar también el canal al que pertenece.
// CopiarAdjuntoAMiUnidad es «Guardar en mi Drive»: copia un adjunto del canal a una
// carpeta de «Mi unidad». CRUZA DE ÁMBITO A PROPÓSITO, y es la única función del
// paquete que lo hace: lee el origen en el ámbito del canal (con el canal escrito
// en la consulta) y, en la misma transacción, vuelve al ámbito personal para
// insertar la copia. Quien llama ya ha comprobado la membresía del canal.
//
// Antes esto pasaba por CopyFile, que lee el origen en el ámbito personal: solo
// encontraba el adjunto porque la base no evaluaba la RLS.
func (s *Store) CopiarAdjuntoAMiUnidad(ctx context.Context, channelID, fileID, destFolderID uuid.UUID, ownerSub string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var out domain.File
	err := s.inChannelTx(ctx, channelID, func(tx pgx.Tx) error {
		src, err := scanFile(tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND id=$2 AND deleted_at IS NULL AND channel_id=$3`,
			t.Slug, fileID, channelID), t.Slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrNotFound
		}
		if err != nil {
			return err
		}
		// De aquí en adelante, «Mi unidad»: la copia nace en el ámbito personal.
		personalOwner := OwnerFrom(ctx)
		if personalOwner == "" {
			personalOwner = ownerSub
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.channel', '', true), set_config('app.owner_sub', $1, true)", personalOwner); err != nil {
			return fmt.Errorf("store: volver al ámbito personal: %w", err)
		}
		out, err = insertarCopia(WithOwner(ctx, personalOwner), tx, t.Slug, src, destFolderID, ownerSub)
		return err
	})
	return out, err
}

func (s *Store) ChannelFile(ctx context.Context, channelID, fileID uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inChannelTx(ctx, channelID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL AND channel_id=$3`,
			t.Slug, fileID, channelID)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}
