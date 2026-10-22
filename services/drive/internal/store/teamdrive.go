// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Unidades de equipo (team/shared drives): persistencia. El contenido vive en las
// MISMAS tablas folders/files marcado con team_drive_id; el AISLAMIENTO lo impone
// la RLS vía la GUC `app.team_drive` (ver migración 0010), que fija inTeamDriveTx.
// Así los métodos de contenido reutilizan los núcleos de tx del store personal
// (createFileTx, listFilesPageTx, purgeFileTx…) sin duplicar lógica: el ámbito lo
// pone la política, no la consulta.
package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// inTeamDriveTx abre una tx con `app.tenant` (RLS por tenant) y ADEMÁS
// `app.team_drive` fijado al id de la unidad (LOCAL: revierte al terminar la tx,
// sin fugas entre transacciones de la misma conexión del pool). Bajo esta tx, las
// consultas sobre folders/files ven SOLO el contenido de esa unidad (la RLS de la
// migración 0010). Las tablas team_drives/team_drive_members no dependen de
// `app.team_drive` (solo de `app.tenant`), así que también son legibles aquí.
func (s *Store) inTeamDriveTx(ctx context.Context, driveID uuid.UUID, fn func(tx pgx.Tx) error) error {
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT set_config('app.team_drive', $1, true)", driveID.String()); err != nil {
			return fmt.Errorf("store: fijar app.team_drive: %w", err)
		}
		return fn(tx)
	})
}

// ---------------------------------------------------------------------------
// Unidades y membresía (tablas team_drives / team_drive_members, por tenant)
// ---------------------------------------------------------------------------

const teamDriveCols = `id, name, root_folder_id, created_by, created_at, updated_at, archived_at`

func scanTeamDrive(row pgx.Row, t string) (domain.TeamDrive, error) {
	var d domain.TeamDrive
	if err := row.Scan(&d.ID, &d.Name, &d.RootFolderID, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ArchivedAt); err != nil {
		return domain.TeamDrive{}, err
	}
	d.Tenant = t
	return d, nil
}

// TeamDriveMembership es una unidad de equipo junto con el rol del usuario que la
// consulta (para «mis unidades de equipo»).
type TeamDriveMembership struct {
	domain.TeamDrive
	Role domain.TeamRole
}

// CreateTeamDrive crea una unidad de equipo: su carpeta RAÍZ propia (desligada del
// árbol personal: parent_id NULL, is_root false, marcada con team_drive_id), la fila
// team_drives y al creador como manager. Todo en UNA tx bajo el ámbito de la nueva
// unidad (inTeamDriveTx) para que la RLS admita insertar la raíz con su team_drive_id.
func (s *Store) CreateTeamDrive(ctx context.Context, name, createdBy string) (domain.TeamDrive, error) {
	t := tenancy.MustFrom(ctx)
	if name == "" || len(name) > 255 {
		return domain.TeamDrive{}, domain.ErrInvalidName
	}
	if createdBy == "" {
		return domain.TeamDrive{}, domain.ErrPermission
	}
	driveID := uuid.New()
	rootID := uuid.New()
	var d domain.TeamDrive
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		// Carpeta raíz de la unidad. name = id de la raíz: único entre las carpetas
		// con parent_id NULL (evita choque con el índice de hermanos) y nunca se
		// muestra (la navegación usa rutas relativas + el nombre de la unidad).
		if _, err := tx.Exec(ctx,
			`INSERT INTO folders (tenant, id, parent_id, name, is_root, team_drive_id)
			 VALUES ($1, $2, NULL, $3, false, $4)`,
			t.Slug, rootID, rootID.String(), driveID); err != nil {
			return fmt.Errorf("store: crear raíz de unidad: %w", err)
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO team_drives (id, tenant, name, root_folder_id, created_by)
			 VALUES ($1, $2, $3, $4, $5) RETURNING `+teamDriveCols,
			driveID, t.Slug, name, rootID, createdBy)
		var e error
		d, e = scanTeamDrive(row, t.Slug)
		if e != nil {
			return e
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO team_drive_members (tenant, drive_id, user_subject, role)
			 VALUES ($1, $2, $3, 'manager')`,
			t.Slug, driveID, createdBy); err != nil {
			return fmt.Errorf("store: alta del creador como manager: %w", err)
		}
		return nil
	})
	return d, err
}

// ListTeamDrivesForUser lista las unidades de las que el usuario es MIEMBRO, con su
// rol, orden alfabético. Recorrido indexado por (tenant, user_subject); sin N+1.
func (s *Store) ListTeamDrivesForUser(ctx context.Context, subject string) ([]TeamDriveMembership, error) {
	t := tenancy.MustFrom(ctx)
	var out []TeamDriveMembership
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT d.id, d.name, d.root_folder_id, d.created_by, d.created_at, d.updated_at, d.archived_at, m.role
			 FROM team_drives d
			 JOIN team_drive_members m ON m.drive_id = d.id AND m.tenant = d.tenant
			 WHERE d.tenant = $1 AND m.user_subject = $2
			 ORDER BY lower(d.name), d.id`, t.Slug, subject)
		if err != nil {
			return fmt.Errorf("store: listar unidades del usuario: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var m TeamDriveMembership
			if err := rows.Scan(&m.ID, &m.Name, &m.RootFolderID, &m.CreatedBy,
				&m.CreatedAt, &m.UpdatedAt, &m.ArchivedAt, &m.Role); err != nil {
				return err
			}
			m.Tenant = t.Slug
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// TeamDriveAccess es la VERJA de acceso (espeja ShareForResource): devuelve la
// unidad y el ROL del usuario si es MIEMBRO; si no lo es (o la unidad no existe),
// ErrNotFound —deny-by-default, sin distinguir «no existe» de «no eres miembro»
// para no filtrar la existencia—. La usa cada operación antes de actuar.
func (s *Store) TeamDriveAccess(ctx context.Context, driveID uuid.UUID, subject string) (domain.TeamDrive, domain.TeamRole, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.TeamDrive
	var role domain.TeamRole
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT d.id, d.name, d.root_folder_id, d.created_by, d.created_at, d.updated_at, d.archived_at, m.role
			 FROM team_drives d
			 JOIN team_drive_members m ON m.drive_id = d.id AND m.tenant = d.tenant
			 WHERE d.tenant = $1 AND d.id = $2 AND m.user_subject = $3`,
			t.Slug, driveID, subject)
		return row.Scan(&d.ID, &d.Name, &d.RootFolderID, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt, &d.ArchivedAt, &role)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TeamDrive{}, "", domain.ErrNotFound
	}
	if err != nil {
		return domain.TeamDrive{}, "", err
	}
	d.Tenant = t.Slug
	return d, role, nil
}

// RenameTeamDrive cambia el nombre de la unidad (ajuste; solo manager, gateado en
// el servicio).
func (s *Store) RenameTeamDrive(ctx context.Context, driveID uuid.UUID, name string) (domain.TeamDrive, error) {
	t := tenancy.MustFrom(ctx)
	if name == "" || len(name) > 255 {
		return domain.TeamDrive{}, domain.ErrInvalidName
	}
	var d domain.TeamDrive
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE team_drives SET name = $3, updated_at = now()
			 WHERE tenant = $1 AND id = $2 RETURNING `+teamDriveCols,
			t.Slug, driveID, name)
		var e error
		d, e = scanTeamDrive(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TeamDrive{}, domain.ErrNotFound
	}
	return d, err
}

// SetTeamDriveArchived archiva (archived=true) o restaura (false) una unidad
// (ajuste; solo manager, gateado en el servicio). Marca organizativa: no toca el
// contenido ni la membresía. Idempotente al valor pedido.
func (s *Store) SetTeamDriveArchived(ctx context.Context, driveID uuid.UUID, archived bool) (domain.TeamDrive, error) {
	t := tenancy.MustFrom(ctx)
	var d domain.TeamDrive
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE team_drives
			 SET archived_at = CASE WHEN $3 THEN now() ELSE NULL END, updated_at = now()
			 WHERE tenant = $1 AND id = $2 RETURNING `+teamDriveCols,
			t.Slug, driveID, archived)
		var e error
		d, e = scanTeamDrive(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TeamDrive{}, domain.ErrNotFound
	}
	return d, err
}

const teamMemberCols = `user_subject, role, created_at, updated_at`

func scanTeamMember(row pgx.Row, driveID uuid.UUID, t string) (domain.TeamMember, error) {
	var m domain.TeamMember
	if err := row.Scan(&m.Subject, &m.Role, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return domain.TeamMember{}, err
	}
	m.DriveID = driveID
	m.Tenant = t
	return m, nil
}

// ListTeamDriveMembers lista los miembros de una unidad (para el admin de miembros;
// solo manager, gateado en el servicio).
func (s *Store) ListTeamDriveMembers(ctx context.Context, driveID uuid.UUID) ([]domain.TeamMember, error) {
	t := tenancy.MustFrom(ctx)
	var out []domain.TeamMember
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT `+teamMemberCols+` FROM team_drive_members
			 WHERE tenant = $1 AND drive_id = $2 ORDER BY user_subject`, t.Slug, driveID)
		if err != nil {
			return fmt.Errorf("store: listar miembros: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			m, err := scanTeamMember(rows, driveID, t.Slug)
			if err != nil {
				return err
			}
			out = append(out, m)
		}
		return rows.Err()
	})
	return out, err
}

// PutTeamDriveMember añade un miembro o cambia su rol (upsert). Solo manager
// (gateado en el servicio). Al degradar a un manager comprueba que quede otro
// (ErrLastManager) para no dejar la unidad sin gestor.
func (s *Store) PutTeamDriveMember(ctx context.Context, driveID uuid.UUID, subject string, role domain.TeamRole) (domain.TeamMember, error) {
	t := tenancy.MustFrom(ctx)
	if subject == "" || !domain.ValidTeamRole(role) {
		return domain.TeamMember{}, domain.ErrInvalidName
	}
	var m domain.TeamMember
	err := s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if role != domain.TeamManager {
			if err := ensureAnotherManagerTx(ctx, tx, t.Slug, driveID, subject); err != nil {
				return err
			}
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO team_drive_members (tenant, drive_id, user_subject, role)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (drive_id, user_subject)
			 DO UPDATE SET role = EXCLUDED.role, updated_at = now()
			 RETURNING `+teamMemberCols, t.Slug, driveID, subject, string(role))
		var e error
		m, e = scanTeamMember(row, driveID, t.Slug)
		return e
	})
	return m, err
}

// RemoveTeamDriveMember quita a un miembro (solo manager). El contenido de la
// unidad NO se toca (continuidad): sus ficheros pertenecen a la unidad, no al
// usuario. Comprueba que no sea el último manager (ErrLastManager).
func (s *Store) RemoveTeamDriveMember(ctx context.Context, driveID uuid.UUID, subject string) error {
	t := tenancy.MustFrom(ctx)
	return s.pool.InTenantTx(ctx, func(tx pgx.Tx) error {
		if err := ensureAnotherManagerTx(ctx, tx, t.Slug, driveID, subject); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM team_drive_members WHERE tenant = $1 AND drive_id = $2 AND user_subject = $3`,
			t.Slug, driveID, subject)
		if err != nil {
			return fmt.Errorf("store: quitar miembro: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrNotFound
		}
		return nil
	})
}

// ensureAnotherManagerTx falla con ErrLastManager si `subject` es manager y no hay
// ningún OTRO manager en la unidad (quitarlo/degradarlo la dejaría sin gestor). Si
// `subject` no es manager (o no existe), no bloquea.
func ensureAnotherManagerTx(ctx context.Context, tx pgx.Tx, tenant string, driveID uuid.UUID, subject string) error {
	var otherManagers int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM team_drive_members
		 WHERE tenant = $1 AND drive_id = $2 AND role = 'manager' AND user_subject <> $3`,
		tenant, driveID, subject).Scan(&otherManagers); err != nil {
		return fmt.Errorf("store: contar gestores: %w", err)
	}
	if otherManagers > 0 {
		return nil
	}
	// Sin otros managers: solo bloquea si el sujeto ES el manager actual.
	var isManager bool
	err := tx.QueryRow(ctx,
		`SELECT role = 'manager' FROM team_drive_members
		 WHERE tenant = $1 AND drive_id = $2 AND user_subject = $3`,
		tenant, driveID, subject).Scan(&isManager)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if isManager {
		return domain.ErrLastManager
	}
	return nil
}

// ---------------------------------------------------------------------------
// Contenido dentro de una unidad (RLS por ámbito; reutiliza los núcleos de tx)
// ---------------------------------------------------------------------------

// TeamListSubfolders lista las subcarpetas vivas de una carpeta de la unidad.
func (s *Store) TeamListSubfolders(ctx context.Context, driveID, folderID uuid.UUID) ([]domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var folders []domain.Folder
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		folders, e = listSubfoldersTx(ctx, tx, t.Slug, folderID)
		return e
	})
	return folders, err
}

// TeamGetFolder devuelve una carpeta viva de la unidad por id.
func (s *Store) TeamGetFolder(ctx context.Context, driveID, folderID uuid.UUID) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.Folder
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+folderCols+` FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, folderID)
		var e error
		f, e = scanFolder(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return f, err
}

// TeamListFilesPage lista los ficheros de una carpeta de la unidad (keyset).
func (s *Store) TeamListFilesPage(ctx context.Context, driveID, folderID uuid.UUID, after FileCursor, limit int) (FilePage, error) {
	t := tenancy.MustFrom(ctx)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var page FilePage
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		page, e = listFilesPageTx(ctx, tx, t.Slug, folderID, after, limit)
		return e
	})
	if err != nil {
		return FilePage{}, err
	}
	return page, nil
}

// TeamGetFileByName resuelve un fichero por nombre dentro de una carpeta de la
// unidad (para descender por ruta en la navegación, como PublicBrowse).
func (s *Store) TeamGetFileByName(ctx context.Context, driveID, folderID uuid.UUID, name string) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files
			 WHERE tenant=$1 AND `+ambitoSQL+` AND folder_id=$2 AND lower(name)=lower($3) AND deleted_at IS NULL`,
			t.Slug, folderID, name)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// TeamGetFile devuelve un fichero vivo de la unidad por id (para descargar).
func (s *Store) TeamGetFile(ctx context.Context, driveID, fileID uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`SELECT `+fileCols+` FROM files WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, fileID)
		var e error
		f, e = scanFile(row, t.Slug)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// TeamCreateFolder crea una subcarpeta dentro de la unidad. El team_drive_id lo
// hereda del padre vía la RLS/consulta del ámbito (createFolder no lo fija; la raíz
// y sus descendientes comparten ámbito), pero para la carpeta nueva lo fijamos
// explícito para satisfacer el WITH CHECK de la política.
func (s *Store) TeamCreateFolder(ctx context.Context, driveID, parentID uuid.UUID, name, ownerSub string) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(name) {
		return domain.Folder{}, domain.ErrInvalidName
	}
	var f domain.Folder
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		// Padre debe existir y estar vivo DENTRO de la unidad (RLS acota el ámbito).
		var ok bool
		if err := tx.QueryRow(ctx,
			`SELECT true FROM folders WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL`,
			t.Slug, parentID).Scan(&ok); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return domain.ErrInvalidParent
			}
			return err
		}
		row := tx.QueryRow(ctx,
			`INSERT INTO folders (tenant, parent_id, name, owner_sub, team_drive_id)
			 VALUES ($1, $2, $3, $4, $5) RETURNING `+folderCols,
			t.Slug, parentID, name, ownerSub, driveID)
		var e error
		f, e = scanFolder(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	return f, err
}

// TeamCreateFile inserta el metadato de un fichero subido a una unidad, con la
// MISMA lógica de cuota/versión/conflicto que «Mi unidad» (createFileTx). La cuota
// se reserva contra el TENANT (la organización): el contenido de la unidad cuenta
// contra la cuota de la org, no de ningún usuario. team_drive_id se fija explícito
// para el WITH CHECK del ámbito.
func (s *Store) TeamCreateFile(ctx context.Context, driveID uuid.UUID, rec NewFileRecord) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	if !domain.ValidName(rec.Name) {
		return domain.File{}, domain.ErrInvalidName
	}
	if rec.ContentType == "" {
		rec.ContentType = "application/octet-stream"
	}
	var f domain.File
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		f, e = createFileTx(ctx, tx, t.Slug, &driveID, rec)
		return e
	})
	return f, err
}

// TeamDeleteFolder envía a la papelera una carpeta (y su subárbol) de la unidad.
func (s *Store) TeamDeleteFolder(ctx context.Context, driveID, folderID uuid.UUID) ([]DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var deleted []DeletedFile
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		deleted, e = softDeleteFolderTx(ctx, tx, t.Slug, folderID)
		return e
	})
	return deleted, err
}

// TeamDeleteFile envía a la papelera un fichero de la unidad.
func (s *Store) TeamDeleteFile(ctx context.Context, driveID, fileID uuid.UUID) (DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var df DeletedFile
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE files SET deleted_at=now(), updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NULL
			 RETURNING id, storage_key, size_bytes`, t.Slug, fileID)
		return row.Scan(&df.ID, &df.StorageKey, &df.SizeBytes)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedFile{}, domain.ErrNotFound
	}
	return df, err
}

// TeamListTrash lista los elementos de nivel superior en la papelera de la unidad.
func (s *Store) TeamListTrash(ctx context.Context, driveID uuid.UUID) ([]domain.Folder, []domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var folders []domain.Folder
	var files []domain.File
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		folders, files, e = listTrashTx(ctx, tx, t.Slug)
		return e
	})
	return folders, files, err
}

// TeamRestoreFile devuelve un fichero de la papelera de la unidad.
func (s *Store) TeamRestoreFile(ctx context.Context, driveID, fileID uuid.UUID) (domain.File, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.File
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE files SET deleted_at=NULL, updated_at=now()
			 WHERE tenant=$1 AND `+ambitoSQL+` AND id=$2 AND deleted_at IS NOT NULL RETURNING `+fileCols, t.Slug, fileID)
		var e error
		f, e = scanFile(row, t.Slug)
		if uniqueViolation(e) {
			return domain.ErrConflict
		}
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.File{}, domain.ErrNotFound
	}
	return f, err
}

// TeamRestoreFolder devuelve una carpeta (y su subárbol) de la papelera de la unidad.
func (s *Store) TeamRestoreFolder(ctx context.Context, driveID, folderID uuid.UUID) (domain.Folder, error) {
	t := tenancy.MustFrom(ctx)
	var f domain.Folder
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		f, e = restoreFolderTx(ctx, tx, t.Slug, folderID)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Folder{}, domain.ErrNotFound
	}
	return f, err
}

// TeamPurgeFile borra DEFINITIVAMENTE un fichero de la papelera de la unidad y
// libera cuota (reutiliza purgeFileTx).
func (s *Store) TeamPurgeFile(ctx context.Context, driveID, fileID uuid.UUID) (DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var df DeletedFile
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		df, e = purgeFileTx(ctx, tx, t.Slug, fileID)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedFile{}, domain.ErrNotFound
	}
	return df, err
}

// TeamPurgeFolder borra DEFINITIVAMENTE una carpeta y su subárbol de la papelera de
// la unidad (reutiliza purgeFolderTx).
func (s *Store) TeamPurgeFolder(ctx context.Context, driveID, folderID uuid.UUID) ([]DeletedFile, error) {
	t := tenancy.MustFrom(ctx)
	var deleted []DeletedFile
	err := s.inTeamDriveTx(ctx, driveID, func(tx pgx.Tx) error {
		var e error
		deleted, e = purgeFolderTx(ctx, tx, t.Slug, folderID)
		return e
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return deleted, err
}
