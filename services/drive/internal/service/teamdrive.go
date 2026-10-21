// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Casos de uso de las UNIDADES DE EQUIPO (team/shared drives). La VERJA de acceso
// es deny-by-default y por ROL: cada operación resuelve primero la membresía del
// llamante con store.TeamDriveAccess (un NO-miembro recibe ErrNotFound, sin filtrar
// la existencia — igual que ShareForResource en «Compartido conmigo») y comprueba
// que su rol alcance el privilegio requerido (viewer<editor<manager); si no,
// ErrPermission. El AISLAMIENTO del contenido lo impone además la RLS por ámbito
// (app.team_drive) dentro del store; aquí solo se decide QUIÉN puede hacer QUÉ.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// TeamDriveMembership es una unidad de equipo con el rol del usuario que la
// consulta («mis unidades de equipo»). Alias del tipo del store para que la capa
// HTTP lo nombre sin importar el paquete store (mismo patrón que Entry/SharedEntry).
type TeamDriveMembership = store.TeamDriveMembership

// TeamListing es el listado de una carpeta DENTRO de una unidad de equipo: las
// subcarpetas y una página keyset de ficheros, más el ROL del llamante (para que el
// cliente oculte la escritura a los viewers) y el cursor de continuación.
type TeamListing struct {
	Subfolders []domain.Folder
	Files      []domain.File
	NextCursor string
	Role       domain.TeamRole
}

// teamAccess resuelve la unidad y aplica la verja: el llamante debe ser MIEMBRO
// (deny-by-default: si no, ErrNotFound) y su rol alcanzar `need` (si no,
// ErrPermission). Devuelve la unidad (con su carpeta raíz) para las operaciones.
func (s *Service) teamAccess(ctx context.Context, driveID uuid.UUID, subject string, need domain.TeamRole) (domain.TeamDrive, error) {
	drive, role, err := s.team.TeamDriveAccess(ctx, driveID, subject)
	if err != nil {
		return domain.TeamDrive{}, err // ErrNotFound para un no-miembro (sin fuga)
	}
	if !role.AtLeast(need) {
		return domain.TeamDrive{}, domain.ErrPermission
	}
	return drive, nil
}

// CreateTeamDrive crea una unidad de equipo con el creador como manager. Cualquier
// miembro autenticado del tenant puede crearla (como crear una carpeta en «Mi
// unidad»); la propiedad es de la ORG, no del creador.
func (s *Service) CreateTeamDrive(ctx context.Context, name, createdBy string) (domain.TeamDrive, error) {
	d, err := s.team.CreateTeamDrive(ctx, name, createdBy)
	if err != nil {
		return domain.TeamDrive{}, err
	}
	s.publish(ctx, "drive.team_drive.created", map[string]any{"team_drive_id": d.ID, "name": d.Name})
	return d, nil
}

// ListTeamDrives lista las unidades de las que el usuario es miembro, con su rol.
func (s *Service) ListTeamDrives(ctx context.Context, subject string) ([]TeamDriveMembership, error) {
	return s.team.ListTeamDrivesForUser(ctx, subject)
}

// GetTeamDrive devuelve una unidad y el ROL del usuario si es miembro; si no,
// ErrNotFound (deny-by-default).
func (s *Service) GetTeamDrive(ctx context.Context, driveID uuid.UUID, subject string) (domain.TeamDrive, domain.TeamRole, error) {
	return s.team.TeamDriveAccess(ctx, driveID, subject)
}

// RenameTeamDrive renombra una unidad (ajuste; solo manager).
func (s *Service) RenameTeamDrive(ctx context.Context, driveID uuid.UUID, subject, name string) (domain.TeamDrive, error) {
	if _, err := s.teamAccess(ctx, driveID, subject, domain.TeamManager); err != nil {
		return domain.TeamDrive{}, err
	}
	d, err := s.team.RenameTeamDrive(ctx, driveID, name)
	if err != nil {
		return domain.TeamDrive{}, err
	}
	s.publish(ctx, "drive.team_drive.renamed", map[string]any{"team_drive_id": d.ID, "name": d.Name})
	return d, nil
}

// SetTeamDriveArchived archiva o restaura una unidad (ajuste; solo manager).
func (s *Service) SetTeamDriveArchived(ctx context.Context, driveID uuid.UUID, subject string, archived bool) (domain.TeamDrive, error) {
	if _, err := s.teamAccess(ctx, driveID, subject, domain.TeamManager); err != nil {
		return domain.TeamDrive{}, err
	}
	d, err := s.team.SetTeamDriveArchived(ctx, driveID, archived)
	if err != nil {
		return domain.TeamDrive{}, err
	}
	s.publish(ctx, "drive.team_drive.archived", map[string]any{"team_drive_id": d.ID, "archived": archived})
	return d, nil
}

// ListTeamDriveMembers lista los miembros de una unidad (admin de miembros; solo
// manager). Un no-miembro/rol insuficiente no ve la lista (deny-by-default).
func (s *Service) ListTeamDriveMembers(ctx context.Context, driveID uuid.UUID, subject string) ([]domain.TeamMember, error) {
	if _, err := s.teamAccess(ctx, driveID, subject, domain.TeamManager); err != nil {
		return nil, err
	}
	return s.team.ListTeamDriveMembers(ctx, driveID)
}

// PutTeamDriveMember añade un miembro o cambia su rol (solo manager). El store
// impide dejar la unidad sin gestor (ErrLastManager).
func (s *Service) PutTeamDriveMember(ctx context.Context, driveID uuid.UUID, actorSub, subject string, role domain.TeamRole) (domain.TeamMember, error) {
	if _, err := s.teamAccess(ctx, driveID, actorSub, domain.TeamManager); err != nil {
		return domain.TeamMember{}, err
	}
	if !domain.ValidTeamRole(role) {
		return domain.TeamMember{}, domain.ErrInvalidName
	}
	m, err := s.team.PutTeamDriveMember(ctx, driveID, subject, role)
	if err != nil {
		return domain.TeamMember{}, err
	}
	s.publish(ctx, "drive.team_drive.member_set", map[string]any{"team_drive_id": driveID, "subject": subject, "role": string(role)})
	return m, nil
}

// RemoveTeamDriveMember quita a un miembro (solo manager). El contenido permanece
// (continuidad). No permite quitar al último manager (ErrLastManager).
func (s *Service) RemoveTeamDriveMember(ctx context.Context, driveID uuid.UUID, actorSub, subject string) error {
	if _, err := s.teamAccess(ctx, driveID, actorSub, domain.TeamManager); err != nil {
		return err
	}
	if err := s.team.RemoveTeamDriveMember(ctx, driveID, subject); err != nil {
		return err
	}
	s.publish(ctx, "drive.team_drive.member_removed", map[string]any{"team_drive_id": driveID, "subject": subject})
	return nil
}

// ---------------------------------------------------------------------------
// Contenido dentro de una unidad (reutiliza los núcleos de tx del store personal;
// el ámbito lo impone la RLS vía app.team_drive dentro de cada método Team*)
// ---------------------------------------------------------------------------

// teamFolderOrRoot devuelve folderID, o la raíz de la unidad si es uuid.Nil (la
// raíz nunca se expone al cliente: navegar «/» de una unidad opera sobre ella).
func teamFolderOrRoot(drive domain.TeamDrive, folderID uuid.UUID) uuid.UUID {
	if folderID == uuid.Nil {
		return drive.RootFolderID
	}
	return folderID
}

// TeamListChildren lista subcarpetas + una página de ficheros de una carpeta de la
// unidad (folderID vacío = raíz). Cualquier miembro (viewer+) puede leer.
func (s *Service) TeamListChildren(ctx context.Context, driveID uuid.UUID, subject string, folderID uuid.UUID, cursor string, limit int) (TeamListing, error) {
	drive, role, err := s.team.TeamDriveAccess(ctx, driveID, subject)
	if err != nil {
		return TeamListing{}, err
	}
	target := teamFolderOrRoot(drive, folderID)
	after, err := decodeFileCursor(cursor)
	if err != nil {
		return TeamListing{}, err
	}
	subs, err := s.team.TeamListSubfolders(ctx, driveID, target)
	if err != nil {
		return TeamListing{}, err
	}
	page, err := s.team.TeamListFilesPage(ctx, driveID, target, after, limit)
	if err != nil {
		return TeamListing{}, err
	}
	out := TeamListing{Subfolders: subs, Files: page.Files, Role: role}
	if page.Next != nil {
		out.NextCursor = encodeFileCursor(*page.Next)
	}
	return out, nil
}

// TeamCreateFolder crea una subcarpeta dentro de la unidad (parentID vacío = raíz).
// Requiere permiso de escritura (editor+).
func (s *Service) TeamCreateFolder(ctx context.Context, driveID uuid.UUID, subject string, parentID uuid.UUID, name string) (domain.Folder, error) {
	drive, err := s.teamAccess(ctx, driveID, subject, domain.TeamEditor)
	if err != nil {
		return domain.Folder{}, err
	}
	f, err := s.team.TeamCreateFolder(ctx, driveID, teamFolderOrRoot(drive, parentID), name, subject)
	if err != nil {
		return domain.Folder{}, err
	}
	s.publish(ctx, "drive.team_drive.folder_created", map[string]any{"team_drive_id": driveID, "folder_id": f.ID, "name": f.Name})
	return f, nil
}

// TeamUploadFile sube el contenido a S3 y registra el metadato dentro de la unidad
// (folderID vacío = raíz), con la MISMA lógica de cuota/versión que «Mi unidad»
// (createFileTx). Requiere escritura (editor+). La cuota se reserva contra el tenant.
func (s *Service) TeamUploadFile(ctx context.Context, driveID uuid.UUID, subject string, folderID uuid.UUID, name, contentType string, body io.Reader) (domain.File, error) {
	drive, err := s.teamAccess(ctx, driveID, subject, domain.TeamEditor)
	if err != nil {
		return domain.File{}, err
	}
	if !domain.ValidName(name) {
		return domain.File{}, domain.ErrInvalidName
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.File{}, err
	}
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.File{}, err
	}
	target := teamFolderOrRoot(drive, folderID)
	storageKey := blobKey(target, name)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	limited := io.LimitReader(body, s.cfg.MaxUploadBytes+1)
	hasher := sha256.New()
	counter := &countingReader{r: io.TeeReader(limited, hasher)}
	if err := s.blobs.Put(ctx, t, storageKey, counter, -1, objectstore.PutOptions{
		ContentType: contentType,
		Metadata:    map[string]string{"original-name": name, "owner": subject},
	}); err != nil {
		return domain.File{}, fmt.Errorf("service: subir blob de unidad: %w", err)
	}
	if counter.n > s.cfg.MaxUploadBytes {
		_ = s.blobs.Delete(ctx, t, storageKey)
		return domain.File{}, domain.ErrQuotaExceeded
	}
	checksum := hex.EncodeToString(hasher.Sum(nil))
	storageKey = s.compartirBlob(ctx, t, storageKey, checksum)
	f, err := s.team.TeamCreateFile(ctx, driveID, store.NewFileRecord{
		FolderID:    target,
		Name:        name,
		StorageKey:  storageKey,
		SizeBytes:   counter.n,
		ContentType: contentType,
		Checksum:    checksum,
		OwnerSub:    subject,
	})
	if err != nil {
		s.soltarBlob(ctx, t, storageKey) // compensación: metadato no entró (cuota/conflicto)
		return domain.File{}, err
	}
	s.publish(ctx, "drive.team_drive.file_created", map[string]any{"team_drive_id": driveID, "file_id": f.ID, "name": f.Name, "size": f.SizeBytes})
	return f, nil
}

// TeamOpenFileContent abre el blob de un fichero de la unidad para descargarlo. El
// llamador cierra. Cualquier miembro (viewer+) puede descargar.
func (s *Service) TeamOpenFileContent(ctx context.Context, driveID uuid.UUID, subject string, fileID uuid.UUID) (domain.File, io.ReadCloser, error) {
	if _, err := s.teamAccess(ctx, driveID, subject, domain.TeamViewer); err != nil {
		return domain.File{}, nil, err
	}
	f, err := s.team.TeamGetFile(ctx, driveID, fileID)
	if err != nil {
		return domain.File{}, nil, err
	}
	t, _ := tenancy.From(ctx)
	rc, err := s.blobs.Get(ctx, t, f.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return domain.File{}, nil, domain.ErrNotFound
		}
		return domain.File{}, nil, fmt.Errorf("service: abrir blob de unidad: %w", err)
	}
	return f, rc, nil
}

// TeamDeleteFolder envía a la papelera de la unidad una carpeta y su subárbol
// (editor+). Los blobs se conservan hasta la purga.
func (s *Service) TeamDeleteFolder(ctx context.Context, driveID uuid.UUID, subject string, folderID uuid.UUID) error {
	if _, err := s.teamAccess(ctx, driveID, subject, domain.TeamEditor); err != nil {
		return err
	}
	deleted, err := s.team.TeamDeleteFolder(ctx, driveID, folderID)
	if err != nil {
		return err
	}
	s.publish(ctx, "drive.team_drive.folder_deleted", map[string]any{"team_drive_id": driveID, "folder_id": folderID, "files": len(deleted)})
	return nil
}

// TeamDeleteFile envía a la papelera de la unidad un fichero (editor+).
func (s *Service) TeamDeleteFile(ctx context.Context, driveID uuid.UUID, subject string, fileID uuid.UUID) error {
	if _, err := s.teamAccess(ctx, driveID, subject, domain.TeamEditor); err != nil {
		return err
	}
	if _, err := s.team.TeamDeleteFile(ctx, driveID, fileID); err != nil {
		return err
	}
	s.publish(ctx, "drive.team_drive.file_deleted", map[string]any{"team_drive_id": driveID, "file_id": fileID})
	return nil
}
