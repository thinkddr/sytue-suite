// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package service orquesta la lógica de aplicación de Sytue Drive: combina el
// store (metadatos en PostgreSQL) con el objectstore (blobs en S3) y publica
// eventos de dominio (outbox sobre PostgreSQL). Mantiene la coherencia entre
// ambos almacenes (p. ej. liberar el blob al borrar el metadato).
package service

import (
	"archive/zip"
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/events"
	"git.sytue.eu/sytue/one/platform/pkg/hubnotify"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/peticiones"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/config"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// eventChannel es el canal lógico de eventos de Drive (events.Channel).
const eventChannel events.Channel = "drive"

// dataStore es la superficie mínima de persistencia que consume el Service. La
// satisface *store.Store (producción) y un fake en tests, permitiendo ejercitar
// la lógica sin PostgreSQL. No cambia la firma pública de New.
type dataStore interface {
	// Almacén de terceros (mig 0023): lo que el NAS por túnel necesita del store.
	ExternalStorage(ctx context.Context) (store.ExternalStorage, error)
	SaveExternalStorage(ctx context.Context, e store.ExternalStorage, actor string) (store.ExternalStorage, error)
	MarkExternalStorageChecked(ctx context.Context, ok bool, msg string) error
	EnsureQuota(ctx context.Context, defaultLimit int64) (domain.Quota, error)
	EnsureRoot(ctx context.Context) (domain.Folder, error)
	EnsureBase(ctx context.Context, limit int64, fijar bool) (domain.Folder, domain.Quota, error)
	GetQuota(ctx context.Context) (domain.Quota, error)
	GetFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error)
	FolderPath(ctx context.Context, id uuid.UUID) (string, error)
	ListChildren(ctx context.Context, folderID uuid.UUID) ([]domain.Folder, []domain.File, error)
	ListSubfolders(ctx context.Context, folderID uuid.UUID) ([]domain.Folder, error)
	ResolveUnder(ctx context.Context, root uuid.UUID, segs []string) (*domain.Folder, *domain.File, error)
	CreateFolder(ctx context.Context, parentID uuid.UUID, name, ownerSub string, isGroup bool) (domain.Folder, error)
	RenameFolder(ctx context.Context, id uuid.UUID, name string) (domain.Folder, error)
	MoveFolder(ctx context.Context, id, newParentID uuid.UUID) (domain.Folder, error)
	DeleteFolder(ctx context.Context, id uuid.UUID) ([]store.DeletedFile, error)
	GetFile(ctx context.Context, id uuid.UUID) (domain.File, error)
	GetFileByStorageKey(ctx context.Context, key string) (domain.File, error)
	// Reservas de las subidas directas (mig 0030, FILE-08).
	ReservarSubida(ctx context.Context, clave string, tamano int64, caduca time.Time, conCuota bool) error
	SubidaFinalizada(ctx context.Context, clave string) error
	ReclamarSubidasCaducadas(ctx context.Context, antes time.Time, limite int) ([]store.SubidaCaducada, error)
	CreateFile(ctx context.Context, rec store.NewFileRecord) (domain.File, error)
	RenameFile(ctx context.Context, id uuid.UUID, name string) (domain.File, error)
	MoveFile(ctx context.Context, id, newFolderID uuid.UUID) (domain.File, error)
	SetFileLabel(ctx context.Context, id uuid.UUID, label domain.Label) (domain.File, error)
	DeleteFile(ctx context.Context, id uuid.UUID) (store.DeletedFile, error)
	CopyFile(ctx context.Context, srcID, destFolderID uuid.UUID, ownerSub string) (domain.File, error)
	CopiaParaDuenya(ctx context.Context, srcID, destFolderID uuid.UUID, duenya, editora string) (domain.File, bool, error)
	CopyFolder(ctx context.Context, srcID uuid.UUID, destParentID *uuid.UUID, ownerSub string) (domain.Folder, error)
	ListTrash(ctx context.Context) ([]domain.Folder, []domain.File, error)
	RestoreFile(ctx context.Context, id uuid.UUID) (domain.File, error)
	// FicheroEnCualquierEstado ve también los ficheros en la papelera (docs.go).
	FicheroEnCualquierEstado(ctx context.Context, id uuid.UUID) (domain.File, error)
	// SujetoRetenido: hay una retención legal activa sobre esa persona (docs.go).
	SujetoRetenido(ctx context.Context, sub string) (bool, error)
	RestoreFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error)
	PurgeFile(ctx context.Context, id uuid.UUID) (store.DeletedFile, error)
	PurgeFolder(ctx context.Context, id uuid.UUID) ([]store.DeletedFile, error)
	ClaimAndPurgeExpiredTrash(ctx context.Context, cutoff time.Time, limit int) (int, []store.PurgedBlob, error)
	ListFilesPage(ctx context.Context, folderID uuid.UUID, after store.FileCursor, limit int) (store.FilePage, error)
	GetFileByName(ctx context.Context, folderID uuid.UUID, name string) (domain.File, error)
	SearchFiles(ctx context.Context, query string, limit int) ([]store.Entry, error)
	ListRecent(ctx context.Context, after store.RecentCursor, limit int) ([]store.Entry, *store.RecentCursor, error)
	ListFavorites(ctx context.Context, subject string) ([]store.Entry, error)
	SetFavorite(ctx context.Context, subject string, fileID, folderID *uuid.UUID, on bool) error
	SetQuotaLimit(ctx context.Context, limit int64) (domain.Quota, error)
	CreateShare(ctx context.Context, rec store.NewShareRecord) (domain.Share, error)
	ListSharesForFile(ctx context.Context, fileID uuid.UUID) ([]domain.Share, error)
	ListSharesForFolder(ctx context.Context, folderID uuid.UUID) ([]domain.Share, error)
	RevokeShare(ctx context.Context, id uuid.UUID) error
	ResolveLink(ctx context.Context, token string) (domain.Share, string, string, error)
	ListSharedWithMe(ctx context.Context, subject string, groups []string) ([]store.SharedEntry, error)
	ShareForResource(ctx context.Context, subject string, groups []string, resourceID uuid.UUID) (domain.Share, error)
	// «Pedir acceso de edición» (peticiones.go, mig 0025).
	PedirEdicion(ctx context.Context, recurso uuid.UUID, p peticiones.Peticion) (bool, error)
	PeticionesEdicion(ctx context.Context, recurso uuid.UUID, sub string) ([]peticiones.Peticion, error)
	ResolverPeticionEdicion(ctx context.Context, recurso uuid.UUID, sub, sujeto string, concede bool) error
	PedidasPor(ctx context.Context, sub string, recursos []uuid.UUID) (map[uuid.UUID]bool, error)
	ListVersions(ctx context.Context, fileID uuid.UUID, before, limit int) ([]domain.FileVersion, *int, error)
	GetVersion(ctx context.Context, fileID uuid.UUID, version int) (domain.FileVersion, error)
	RestoreVersion(ctx context.Context, fileID uuid.UUID, version int, actorSub string) (domain.File, error)
	BatchMove(ctx context.Context, fileIDs, folderIDs []uuid.UUID, targetFolderID uuid.UUID) (store.BatchResult, error)
	BatchTrash(ctx context.Context, fileIDs, folderIDs []uuid.UUID) ([]store.DeletedFile, int, error)
	CollectZip(ctx context.Context, fileIDs, folderIDs []uuid.UUID, maxFiles int) ([]store.ZipEntry, error)
}

// blobStore importa el contrato compartido de almacenamiento.
type blobStore = objectstore.BlobStore

// Service implementa los casos de uso de Drive.
type Service struct {
	cfg   config.Config
	store dataStore
	// team es el store CONCRETO para las operaciones por ÁMBITO: unidades de equipo
	// (teamdrive.go) y adjuntos de canal de Rooms (channelfiles.go), que fijan una
	// GUC de RLS en su transacción.
	// No pasa por la interfaz dataStore (cuyo único fin es el fake
	// de los tests unitarios del camino personal): las unidades se prueban por
	// integración (store real) y con el fake del SERVICIO en la capa HTTP, así que
	// no ensucian dataStore ni su fake. Nil en tests unitarios que no las ejercitan.
	team  *store.Store
	blobs blobStore
	// router es el mismo blobs, con su tipo: es quien sabe si el tenant tiene
	// almacén de terceros. nil en los tests que inyectan un fake.
	router *blobRouter
	pub    EventPublisher
	// chat resuelve la MEMBRESÍA de un canal de Sytue Rooms para gatear sus adjuntos
	// (ámbito de canal, channelfiles.go). nil ⇒ la verja no está configurada y los
	// adjuntos de canal NO se sirven (fail-closed).
	chat ChannelAuthorizer
	// hubNotifier emite notificaciones a Sytue Hub para compartición de ficheros/carpetas.
	hubNotifier *hubnotify.Client
}

// Store es la superficie de persistencia que consume el servicio (la satisface
// *store.Store). Se declara aquí para test/desacople.
type Store = store.Store

// Entry es una entrada plana (fichero o carpeta con su ruta resuelta) de
// «Recientes»/«Favoritos». Alias del tipo del store para que la capa HTTP la
// nombre sin importar el paquete store (mismo patrón que Store arriba).
type Entry = store.Entry

// SharedEntry es un recurso compartido CON el usuario (nodo raíz de un share
// kind=user|group) con su permiso y quién lo compartió. Alias del tipo del store.
type SharedEntry = store.SharedEntry

// BatchResult cuenta los elementos afectados por una operación en lote (alias del
// tipo del store para que la capa HTTP lo nombre sin importar el paquete store).
type BatchResult = store.BatchResult

// ZipEntry es un fichero a incluir en un ZIP (alias del tipo del store).
type ZipEntry = store.ZipEntry

// NewStore construye el store de Drive sobre el pool de PostgreSQL.
func NewStore(pool *pg.Pool) *Store { return store.New(pool) }

// EventPublisher publica eventos de dominio (outbox). La implementación real es
// events.PublishTx sobre el *pg.Pool.
type EventPublisher interface {
	Publish(ctx context.Context, ev events.Event) error
}

// New construye el servicio. Envuelve el objectstore en blobStat para dotarlo del
// HEAD (Stat) que finalize necesita y que el objectstore no expone (ver presign.go).
// `chat` es la verja de membresía de canal para los adjuntos de Rooms; nil deja esa
// superficie cerrada (fail-closed) sin afectar al resto de Drive.
func New(cfg config.Config, st *Store, blobs *objectstore.Client, pub EventPublisher, chat ChannelAuthorizer) *Service {
	router := newBlobRouter(blobStat{blobs}, st, cfg.ExternalStorageKey, newIntegrationsClient(cfg.IntegrationsURL, cfg.IntegrationsInternalSecret))
	router.tunnel = newTunnelClient(cfg.TunnelAPIURL, cfg.TunnelAPIKey, cfg.TunnelOrg, cfg.TunnelDomainID, cfg.TunnelDomain, cfg.TunnelEndpoint)
	var notifier *hubnotify.Client
	if cfg.HubBaseURL != "" && cfg.HubInternalSecret != "" {
		notifier = hubnotify.New(cfg.HubBaseURL, cfg.HubInternalSecret)
	}
	return &Service{cfg: cfg, store: st, team: st, blobs: router, router: router, pub: pub, chat: chat, hubNotifier: notifier}
}

// SetHubNotifier configura o reemplaza el cliente de notificaciones de Hub (útil en tests).
func (s *Service) SetHubNotifier(notifier *hubnotify.Client) {
	s.hubNotifier = notifier
}

// ExternalStorageEnabled indica si el tenant del contexto tiene conectado y
// abrible su almacén S3 de terceros: entonces los blobs nuevos van allí.
func (s *Service) ExternalStorageEnabled(ctx context.Context) bool {
	t, err := tenancy.From(ctx)
	return err == nil && s.router != nil && s.router.Enabled(ctx, t)
}

// ExternalInstances lista las instancias de Google Drive y OneDrive del tenant
// (conectadas en Sytue Admin → Aplicaciones). Sin conectores, lista vacía.
func (s *Service) ExternalInstances(ctx context.Context) (ExternalAccounts, error) {
	t, err := tenancy.From(ctx)
	if err != nil || s.router == nil || s.router.conn == nil {
		return ExternalAccounts{Items: []ExternalInstance{}, Providers: map[string]bool{}}, err
	}
	return s.router.conn.Instances(ctx, t)
}

// ProbeExternal comprueba que un almacén de terceros responde con esa
// configuración y esa clave, sin guardarlos. Devuelve el error legible o "".
func (s *Service) ProbeExternal(ctx context.Context, cfg store.ExternalStorage, secret string) string {
	t, err := tenancy.From(ctx)
	if err != nil || s.router == nil {
		return "no disponible"
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, err := s.router.abrir(t, cfg, secret)
	if err != nil {
		return err.Error()
	}
	if p, ok := b.(interface{ Probar(context.Context) error }); ok {
		if err := p.Probar(ctx); err != nil {
			return err.Error()
		}
		return ""
	}
	// S3: el cliente del objectstore comprueba el bucket.
	if bs, ok := b.(blobStat); ok {
		if err := bs.Healthz(ctx); err != nil {
			return err.Error()
		}
	}
	return ""
}

// ForgetExternalStorage olvida la caché del almacén de terceros de un tenant
// (tras conectarlo o desconectarlo).
func (s *Service) ForgetExternalStorage(tenant string) {
	if s.router != nil {
		s.router.Forget(tenant)
	}
}

// claveDeBlob es la clave S3 de un blob nuevo: con el prefijo de terceros si el
// tenant tiene bucket propio, para que el enrutador lo mande allí y la cuota lo
// ignore (store.EsExterno).
func (s *Service) claveDeBlob(ctx context.Context, t tenancy.Tenant, folderID uuid.UUID, name string) (string, error) {
	key := blobKey(folderID, name)
	if s.router != nil {
		external, err := s.router.externo(ctx, t)
		if err != nil {
			return "", errors.Join(ErrExternalStorageOff, err)
		}
		if external != nil {
			return store.ExternalPrefix + key, nil
		}
	}
	return key, nil
}

// publish emite un evento de dominio sin romper la operación si falla (best
// effort; el outbox/relay garantiza la entrega cuando se publica en la tx de
// negocio — aquí lo hacemos tras commitear el metadato).
func (s *Service) publish(ctx context.Context, eventType string, payload any) {
	if s.pub == nil {
		return
	}
	t, _ := tenancy.From(ctx)
	ev, err := events.NewEvent(eventChannel, eventType, t.Slug, payload)
	if err != nil {
		log.From(ctx).Warn("drive: construir evento", "type", eventType, "error", err)
		return
	}
	if err := s.pub.Publish(ctx, ev); err != nil {
		log.From(ctx).Warn("drive: publicar evento", "type", eventType, "error", err)
	}
}

// EnsureTenant inicializa raíz y cuota del tenant (idempotente). Se llama al
// primer acceso autenticado del tenant.
func (s *Service) EnsureTenant(ctx context.Context) (domain.Folder, domain.Quota, error) {
	// Una transacción, no tres: eran cuota, raíz y ADEMÁS releer la cuota con
	// `GetQuota`, que es justo lo que `EnsureQuota` acababa de devolver y se
	// tiraba. Crear la raíz no toca los bytes usados, así que la cuota que sale
	// de la primera consulta es la misma que salía de la tercera.
	limite, fijar := s.limiteDeCuota(ctx)
	return s.store.EnsureBase(ctx, limite, fijar)
}

// limiteDeCuota decide de dónde sale el límite de almacenamiento del tenant.
//
// En una cuenta personal lo dicta el PLAN, y viene en el access token que Sytue
// ID emite (`storage_bytes`): 5 GiB en la gratuita, 1 TiB pagando. Se aplica
// como límite FIJO, que manda sobre la fila: es lo que hace que subir de plan, o
// dejar de pagar, se note en la siguiente petición sin que nadie tenga que
// avisar a Drive. Una cuenta personal no tiene administrador que ajuste su cuota
// a mano, así que no hay nada que este valor pueda pisar.
//
// En una organización el token no trae cuota y se usa el valor por defecto del
// servicio, que solo vale para un tenant nuevo: lo que un administrador haya
// fijado después con SetQuotaLimit se respeta.
func (s *Service) limiteDeCuota(ctx context.Context) (int64, bool) {
	if c, ok := authn.FromContext(ctx); ok && c.StorageBytes > 0 {
		return c.StorageBytes, true
	}
	return s.cfg.DefaultQuotaBytes, false
}

// Root devuelve la carpeta raíz del tenant (creándola si hace falta).
func (s *Service) Root(ctx context.Context) (domain.Folder, error) {
	// Dos transacciones eran una de más, y esto lo paga CADA resolución de ruta
	// de WebDAV: un PROPFIND, un GET y un PUT pasan por aquí antes de mirar nada.
	limite, fijar := s.limiteDeCuota(ctx)
	root, _, err := s.store.EnsureBase(ctx, limite, fijar)
	return root, err
}

// GetFolder delega en el store.
func (s *Service) GetFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error) {
	return s.store.GetFolder(ctx, id)
}

// ListChildren delega en el store.
func (s *Service) ListChildren(ctx context.Context, id uuid.UUID) ([]domain.Folder, []domain.File, error) {
	return s.store.ListChildren(ctx, id)
}

// ListSubfolders lista solo las subcarpetas de una carpeta; los ficheros se
// piden aparte con ListFilesPage (keyset).
func (s *Service) ListSubfolders(ctx context.Context, id uuid.UUID) ([]domain.Folder, error) {
	return s.store.ListSubfolders(ctx, id)
}

// CreateFolder crea una subcarpeta y emite un evento.
func (s *Service) CreateFolder(ctx context.Context, parentID uuid.UUID, name, ownerSub string, isGroup bool) (domain.Folder, error) {
	f, err := s.store.CreateFolder(ctx, parentID, name, ownerSub, isGroup)
	if err != nil {
		return domain.Folder{}, err
	}
	s.publish(ctx, "drive.folder.created", map[string]any{"folder_id": f.ID, "name": f.Name})
	return f, nil
}

// RenameFolder renombra una carpeta.
func (s *Service) RenameFolder(ctx context.Context, id uuid.UUID, name string) (domain.Folder, error) {
	return s.store.RenameFolder(ctx, id, name)
}

// MoveFolder mueve una carpeta.
func (s *Service) MoveFolder(ctx context.Context, id, newParent uuid.UUID) (domain.Folder, error) {
	return s.store.MoveFolder(ctx, id, newParent)
}

// DeleteFolder envía una carpeta (y su subárbol) a la papelera. NO purga los
// blobs: se conservan para poder restaurar; el borrado definitivo es PurgeFolder.
func (s *Service) DeleteFolder(ctx context.Context, id uuid.UUID) error {
	deleted, err := s.store.DeleteFolder(ctx, id)
	if err != nil {
		return err
	}
	s.publish(ctx, "drive.folder.deleted", map[string]any{"folder_id": id, "files": len(deleted)})
	return nil
}

// GetFile delega en el store.
func (s *Service) GetFile(ctx context.Context, id uuid.UUID) (domain.File, error) {
	return s.store.GetFile(ctx, id)
}

// FilePath devuelve la RUTA absoluta de un fichero («/a/b/informe.pdf»).
//
// El explorador navega por ruta, no por id, así que sin esto un enlace canónico
// /files/{id} —el que compone la búsqueda federada del Hub y «Compartir a Rooms»— no
// lleva a ninguna parte. La verja es GetFile: si quien pide no puede ver el fichero, falla
// ahí y no se llega a resolver ninguna ruta.
func (s *Service) FilePath(ctx context.Context, id uuid.UUID) (string, error) {
	f, err := s.store.GetFile(ctx, id)
	if err != nil {
		return "", err
	}
	dir, err := s.store.FolderPath(ctx, f.FolderID)
	if err != nil {
		return "", err
	}
	return store.JoinFilePath(dir, f.Name), nil
}

// UploadFile sube el contenido a S3 y registra el metadato, reservando cuota.
// El tamaño se conoce tras leer (se usa un buffer limitado por MaxUploadBytes).
func (s *Service) UploadFile(ctx context.Context, folderID uuid.UUID, name, contentType, ownerSub string, body io.Reader) (domain.File, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.File{}, err
	}
	if !domain.ValidName(name) {
		return domain.File{}, domain.ErrInvalidName
	}
	// Garantiza la fila de cuota del tenant antes de reservar (defensivo: la
	// reserva de cuota en el store requiere que exista).
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.File{}, err
	}

	// Clave S3 estable e impredecible (no expone nombres del usuario en S3).
	// Subida en streaming con tamaño+checksum acotados (compartida con la subida a
	// un canal de Rooms; ver putBlob en channelfiles.go).
	key, err := s.claveDeBlob(ctx, t, folderID, name)
	if err != nil {
		return domain.File{}, err
	}
	storageKey, size, checksum, err := s.putBlob(ctx, t, key, name, contentType, ownerSub, body)
	if err != nil {
		return domain.File{}, err
	}
	storageKey = s.compartirBlob(ctx, t, storageKey, checksum)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	rec := store.NewFileRecord{
		FolderID:    folderID,
		Name:        name,
		StorageKey:  storageKey,
		SizeBytes:   size,
		ContentType: contentType,
		Checksum:    checksum,
		OwnerSub:    ownerSub,
	}
	f, err := s.store.CreateFile(ctx, rec)
	if err != nil {
		// Compensación: si el metadato no entra (cuota/conflicto), suelta el blob.
		s.soltarBlob(ctx, t, storageKey)
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.created", map[string]any{"file_id": f.ID, "name": f.Name, "size": f.SizeBytes})
	s.avisarCuota(ctx, ownerSub, f.SizeBytes)
	return f, nil
}

// OpenFileContent abre el contenido de un fichero desde S3. El llamador cierra.
func (s *Service) OpenFileContent(ctx context.Context, id uuid.UUID) (domain.File, io.ReadCloser, error) {
	f, err := s.store.GetFile(ctx, id)
	if err != nil {
		return domain.File{}, nil, err
	}
	t, _ := tenancy.From(ctx)
	rc, err := s.blobs.Get(ctx, t, f.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return domain.File{}, nil, domain.ErrNotFound
		}
		return domain.File{}, nil, fmt.Errorf("service: abrir blob: %w", err)
	}
	return f, rc, nil
}

// PresignDownload genera una URL prefirmada de descarga directa desde S3.
func (s *Service) PresignDownload(ctx context.Context, id uuid.UUID) (*url.URL, error) {
	f, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	t, _ := tenancy.From(ctx)
	return s.blobs.PresignGet(ctx, t, f.StorageKey, s.cfg.PresignTTL)
}

// RenameFile renombra un fichero.
func (s *Service) RenameFile(ctx context.Context, id uuid.UUID, name string) (domain.File, error) {
	return s.store.RenameFile(ctx, id, name)
}

// MoveFile mueve un fichero.
func (s *Service) MoveFile(ctx context.Context, id, newFolder uuid.UUID) (domain.File, error) {
	return s.store.MoveFile(ctx, id, newFolder)
}

// SetFileLabel pone (o quita, con label.ID vacío) la etiqueta de sensibilidad de un
// fichero. `label` YA viene resuelta contra la taxonomía viva de Sytue ID por la capa
// HTTP: aquí solo se guarda el snapshot (mig 0013), así que nadie puede inventarse una
// etiqueta —ni relajar su política— desde el cuerpo de la petición.
//
// GATEADO por permiso de ESCRITURA sobre el fichero, igual que RestoreVersion/CopyFile:
// un viewer recibe ErrPermission (deny-by-default). El acceso al fichero en sí lo gatea
// RLS por tenant (uno ajeno no existe → ErrNotFound).
func (s *Service) SetFileLabel(ctx context.Context, id uuid.UUID, label domain.Label, perm domain.Permission) (domain.File, error) {
	if perm != domain.PermWrite {
		return domain.File{}, domain.ErrPermission
	}
	f, err := s.store.SetFileLabel(ctx, id, label)
	if err != nil {
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.labeled", map[string]any{
		"file_id": f.ID, "label_id": label.ID, "label_name": label.Name,
	})
	return f, nil
}

// DeleteFile envía un fichero a la papelera. NO purga su blob: se conserva para
// poder restaurar; el borrado definitivo (blob incluido) es PurgeFile.
func (s *Service) DeleteFile(ctx context.Context, id uuid.UUID) error {
	if _, err := s.store.DeleteFile(ctx, id); err != nil {
		return err
	}
	s.publish(ctx, "drive.file.deleted", map[string]any{"file_id": id})
	return nil
}

// ---------------------------------------------------------------------------
// Copia server-side (por referencia al blob, sin duplicar bytes)
// ---------------------------------------------------------------------------

// CopyFile hace «una copia» de un fichero EN EL SERVIDOR: una fila de metadatos
// nueva que referencia el MISMO blob (sin descargar/resubir; O(1) en bytes),
// «Copia de X» única en el destino. destFolderID vacío (uuid.Nil) = la misma
// carpeta del origen (paridad Drive: la copia aparece junto al original). GATEADO
// por permiso de ESCRITURA sobre el destino (copiar es crear allí), como
// RestoreVersion: un viewer recibe ErrPermission (deny-by-default). El acceso al
// ORIGEN lo gatea RLS (GetFile / el store: un fichero ajeno no existe → ErrNotFound).
// La cuota se reserva por bytes lógicos (política existente de CreateFile).
func (s *Service) CopyFile(ctx context.Context, srcID, destFolderID uuid.UUID, actorSub string, perm domain.Permission) (domain.File, error) {
	if perm != domain.PermWrite {
		return domain.File{}, domain.ErrPermission
	}
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.File{}, err
	}
	if destFolderID == uuid.Nil {
		src, err := s.store.GetFile(ctx, srcID) // RLS gatea el origen y da la carpeta por defecto
		if err != nil {
			return domain.File{}, err
		}
		destFolderID = src.FolderID
	}
	f, err := s.store.CopyFile(ctx, srcID, destFolderID, actorSub)
	if err != nil {
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.copied", map[string]any{
		"file_id": f.ID, "source_id": srcID, "name": f.Name, "size": f.SizeBytes,
	})
	return f, nil
}

// CopyFolder copia una carpeta y su subárbol EN EL SERVIDOR (recursivo, cada
// fichero referencia su MISMO blob; sin duplicar bytes). destParentID nil = el
// padre del origen (copia hermana «Copia de X»). Mismo gating que CopyFile:
// ESCRITURA (un viewer recibe ErrPermission); RLS gatea el acceso al origen y el
// aislamiento por tenant. La cuota se reserva por los bytes lógicos del subárbol.
func (s *Service) CopyFolder(ctx context.Context, srcID uuid.UUID, destParentID *uuid.UUID, actorSub string, perm domain.Permission) (domain.Folder, error) {
	if perm != domain.PermWrite {
		return domain.Folder{}, domain.ErrPermission
	}
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.Folder{}, err
	}
	f, err := s.store.CopyFolder(ctx, srcID, destParentID, actorSub)
	if err != nil {
		return domain.Folder{}, err
	}
	s.publish(ctx, "drive.folder.copied", map[string]any{
		"folder_id": f.ID, "source_id": srcID, "name": f.Name,
	})
	return f, nil
}

// ---------------------------------------------------------------------------
// Papelera: listar, restaurar y purgar (borrado definitivo)
// ---------------------------------------------------------------------------

// ListTrash lista los elementos en la papelera del tenant (borrados de nivel
// superior). Delega en el store.
func (s *Service) ListTrash(ctx context.Context) ([]domain.Folder, []domain.File, error) {
	return s.store.ListTrash(ctx)
}

// RestoreFile devuelve un fichero de la papelera a su carpeta original.
func (s *Service) RestoreFile(ctx context.Context, id uuid.UUID) (domain.File, error) {
	f, err := s.store.RestoreFile(ctx, id)
	if err != nil {
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.restored", map[string]any{"file_id": f.ID})
	return f, nil
}

// RestoreFolder devuelve una carpeta (y su subárbol) de la papelera.
func (s *Service) RestoreFolder(ctx context.Context, id uuid.UUID) (domain.Folder, error) {
	f, err := s.store.RestoreFolder(ctx, id)
	if err != nil {
		return domain.Folder{}, err
	}
	s.publish(ctx, "drive.folder.restored", map[string]any{"folder_id": f.ID})
	return f, nil
}

// PurgeFile elimina definitivamente un fichero de la papelera y borra su blob en
// S3 (best effort: si el borrado del blob falla, el metadato ya no existe y un GC
// puede limpiar el huérfano más tarde).
func (s *Service) PurgeFile(ctx context.Context, id uuid.UUID) error {
	df, err := s.store.PurgeFile(ctx, id)
	if err != nil {
		return err
	}
	// Solo borra el blob si NINGUNA copia (que comparte el mismo blob por referencia)
	// ni versión lo referencia ya; si no, se conserva (borrarlo perdería datos de la
	// copia superviviente). El store fija BlobOrphaned tras purgar la fila.
	if df.BlobOrphaned {
		t, _ := tenancy.From(ctx)
		s.soltarBlob(ctx, t, df.StorageKey)
	}
	s.publish(ctx, "drive.file.purged", map[string]any{"file_id": id})
	return nil
}

// PurgeFolder elimina definitivamente una carpeta (y su subárbol) de la papelera
// y borra sus blobs en S3.
func (s *Service) PurgeFolder(ctx context.Context, id uuid.UUID) error {
	deleted, err := s.store.PurgeFolder(ctx, id)
	if err != nil {
		return err
	}
	t, _ := tenancy.From(ctx)
	for _, df := range deleted {
		// Conserva los blobs que una copia superviviente (misma clave por referencia)
		// aún necesita; el store marca BlobOrphaned solo los que ya no referencia nadie.
		if !df.BlobOrphaned {
			continue
		}
		s.soltarBlob(ctx, t, df.StorageKey)
	}
	s.publish(ctx, "drive.folder.purged", map[string]any{"folder_id": id, "files": len(deleted)})
	return nil
}

// ---------------------------------------------------------------------------
// Operaciones en lote (mover/papelera N) y ZIP en streaming
// ---------------------------------------------------------------------------

// maxZipFiles acota el nº de ficheros que puede reunir un ZIP de selección/carpeta
// (defensa contra árboles gigantes). ponytail: cota por NÚMERO de ficheros; añadir
// una cota por tamaño total (sumar ZipEntry.Size) si algún tenant la necesita.
const maxZipFiles = 5000

// BatchMove reubica N ficheros y N carpetas bajo targetFolderID en UNA operación
// (delega en el store: `id = ANY`, sin N+1) y emite un evento resumen. Gateado por
// ítem vía RLS (lo ajeno/inexistente/borrado se omite; el resultado cuenta lo
// realmente movido). Errores: ErrInvalidParent (destino inválido), ErrCycle
// (destino dentro de una carpeta movida), ErrConflict (choque de nombre, atómico).
func (s *Service) BatchMove(ctx context.Context, fileIDs, folderIDs []uuid.UUID, targetFolderID uuid.UUID) (BatchResult, error) {
	res, err := s.store.BatchMove(ctx, fileIDs, folderIDs, targetFolderID)
	if err != nil {
		return BatchResult{}, err
	}
	s.publish(ctx, "drive.batch.moved", map[string]any{
		"files": res.Files, "folders": res.Folders, "target": targetFolderID,
	})
	return res, nil
}

// BatchTrash envía a la papelera N ficheros y N subárboles de carpeta en UNA
// operación (soft delete; NO purga blobs ni libera cuota, como DeleteFile/
// DeleteFolder) y emite un evento resumen. Gateado por ítem vía RLS.
func (s *Service) BatchTrash(ctx context.Context, fileIDs, folderIDs []uuid.UUID) (BatchResult, error) {
	deleted, folders, err := s.store.BatchTrash(ctx, fileIDs, folderIDs)
	if err != nil {
		return BatchResult{}, err
	}
	s.publish(ctx, "drive.batch.trashed", map[string]any{"files": len(deleted), "folders": folders})
	return BatchResult{Files: len(deleted), Folders: folders}, nil
}

// ZipEntries reúne los ficheros accesibles a incluir en un ZIP de la selección
// (carpetas → su subárbol; ficheros sueltos → la raíz), gateado por ítem
// (deny-by-default: RLS + vivo; lo ajeno/borrado no se incluye). Falla con
// ErrBatchTooLarge si el árbol supera el límite. Se llama ANTES de escribir
// cabeceras para que el error salga como HTTP y no como un ZIP truncado.
func (s *Service) ZipEntries(ctx context.Context, fileIDs, folderIDs []uuid.UUID) ([]ZipEntry, error) {
	return s.store.CollectZip(ctx, fileIDs, folderIDs, maxZipFiles)
}

// StreamZip escribe un ZIP sobre w en STREAMING: por cada entrada abre su blob en
// S3 y lo copia directo al zip.Writer (que a su vez escribe en w), sin bufferizar el
// ZIP ni los blobs en memoria/disco. Memoria O(1) en el tamaño total del contenido
// (solo el índice central —nombres— queda en memoria hasta Close, inherente al
// formato). Cada ruta se sanea contra path traversal antes de escribirse. Si un
// blob falla a mitad, devuelve error (las cabeceras ya se enviaron: el llamador
// registra y aborta; el ZIP del cliente queda truncado).
func (s *Service) StreamZip(ctx context.Context, w io.Writer, entries []ZipEntry) error {
	t, err := tenancy.From(ctx)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(w)
	for _, e := range entries {
		name := sanitizeZipPath(e.Path)
		if name == "" {
			continue // sin ningún segmento válido: se omite (defensivo)
		}
		rc, err := s.blobs.Get(ctx, t, e.StorageKey)
		if err != nil {
			_ = zw.Close()
			return fmt.Errorf("service: abrir blob para zip: %w", err)
		}
		fw, err := zw.Create(name) // Deflate por defecto; escribe en streaming
		if err != nil {
			_ = rc.Close()
			_ = zw.Close()
			return fmt.Errorf("service: crear entrada zip: %w", err)
		}
		if _, err := io.Copy(fw, rc); err != nil {
			_ = rc.Close()
			_ = zw.Close()
			return fmt.Errorf("service: copiar blob a zip: %w", err)
		}
		_ = rc.Close()
	}
	return zw.Close()
}

// sanitizeZipPath convierte una ruta relativa (segmentos separados por '/') en el
// nombre de una entrada de ZIP SEGURA contra path traversal: normaliza '\'→'/',
// descarta segmentos vacíos, '.' y '..' (nunca escapa hacia arriba ni es absoluta) y
// neutraliza caracteres de control restantes. Los nombres ya pasan domain.ValidName
// (sin '/'/'..'/control) al crearse; esto es defensa en profundidad sobre datos
// heredados. Devuelve "" si no queda ningún segmento válido.
func sanitizeZipPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	segs := strings.Split(p, "/")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		seg = strings.TrimSpace(seg)
		if seg == "" || seg == "." || seg == ".." {
			continue
		}
		seg = strings.Map(func(r rune) rune {
			if r < 0x20 || r == 0x7f {
				return '_'
			}
			return r
		}, seg)
		out = append(out, seg)
	}
	return strings.Join(out, "/")
}

// ---------------------------------------------------------------------------
// Historial de versiones
// ---------------------------------------------------------------------------

// VersionPage es una página del historial de versiones de un fichero con el token
// de continuación keyset ya codificado (opaco) para la siguiente petición ("" si
// no hay más). La primera página incluye la versión ACTUAL como cabeza.
type VersionPage struct {
	Versions   []domain.FileVersion
	NextCursor string
}

// currentVersion sintetiza la entrada de historial de la versión VIVA a partir de
// la fila del fichero (no vive en file_versions; la sirve files). Su autor es
// quien subió el contenido actual (ModifiedBy) y su fecha, la última modificación.
func currentVersion(f domain.File) domain.FileVersion {
	return domain.FileVersion{
		FileID:      f.ID,
		Version:     f.Version,
		StorageKey:  f.StorageKey,
		SizeBytes:   f.SizeBytes,
		ContentType: f.ContentType,
		Checksum:    f.Checksum,
		AuthorSub:   f.ModifiedBy,
		IsCurrent:   true,
		CreatedAt:   f.UpdatedAt,
	}
}

// FileVersions lista el historial de un fichero, más nueva primero, con
// paginación keyset. La primera página (cursor vacío) encabeza con la versión ACTUAL.
//
// GetFile se llama SIEMPRE, no solo en la primera página. Era la verja de acceso del
// historial y estaba dentro del `if before == 0`, así que bastaba pedir la página dos
// —con un cursor cualquiera— para saltársela entera y listar las versiones de un
// fichero que no se puede abrir. La autorización descansaba en la RLS de
// `file_versions`, que solo aísla por TENANT: dentro de la organización no separa nada.
func (s *Service) FileVersions(ctx context.Context, fileID uuid.UUID, cursor string, limit int) (VersionPage, error) {
	before, err := decodeVersionCursor(cursor)
	if err != nil {
		return VersionPage{}, err
	}
	f, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return VersionPage{}, err
	}
	var out VersionPage
	if before == 0 {
		out.Versions = append(out.Versions, currentVersion(f))
	}
	prev, next, err := s.store.ListVersions(ctx, fileID, before, limit)
	if err != nil {
		return VersionPage{}, err
	}
	out.Versions = append(out.Versions, prev...)
	if next != nil {
		out.NextCursor = strconv.Itoa(*next)
	}
	return out, nil
}

// OpenVersionContent abre el blob de una versión archivada para descargarla. El
// llamador cierra.
//
// La verja es GetFile sobre el fichero PADRE, no la RLS de `file_versions`: esa política
// aísla por TENANT y nada más, así que quien conociera un id de fichero y un número de
// versión podía descargarse el blob de una versión de un fichero al que no tiene acceso
// —un adjunto de canal, el fichero de otra persona— sin pasar por ninguna comprobación.
// Es el mismo patrón que el salto de paginación de FileVersions: la autorización
// descansaba en la fila equivocada.
func (s *Service) OpenVersionContent(ctx context.Context, fileID uuid.UUID, version int) (domain.FileVersion, io.ReadCloser, error) {
	if _, err := s.store.GetFile(ctx, fileID); err != nil {
		return domain.FileVersion{}, nil, err
	}
	v, err := s.store.GetVersion(ctx, fileID, version)
	if err != nil {
		return domain.FileVersion{}, nil, err
	}
	t, _ := tenancy.From(ctx)
	rc, err := s.blobs.Get(ctx, t, v.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return domain.FileVersion{}, nil, domain.ErrNotFound
		}
		return domain.FileVersion{}, nil, fmt.Errorf("service: abrir blob de versión: %w", err)
	}
	return v, rc, nil
}

// RestoreVersion hace ACTUAL una versión archivada (no destructivo: la actual se
// archiva y la objetivo revive como versión nueva). GATEADO por permiso de
// escritura: solo un editor restaura; un viewer recibe ErrPermission
// (deny-by-default, como «Compartido conmigo»). RLS cubre «sin acceso» (ErrNotFound).
func (s *Service) RestoreVersion(ctx context.Context, fileID uuid.UUID, version int, actorSub string, perm domain.Permission) (domain.File, error) {
	if perm != domain.PermWrite {
		return domain.File{}, domain.ErrPermission
	}
	f, err := s.store.RestoreVersion(ctx, fileID, version, actorSub)
	if err != nil {
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.version.restored", map[string]any{
		"file_id": f.ID, "restored": version, "version": f.Version,
	})
	return f, nil
}

// decodeVersionCursor invierte el cursor keyset del historial (el número de la
// última versión de la página anterior, en decimal). Vacío = primera página;
// corrupto = entrada inválida (no un fallo de servidor). ponytail: cursor en
// claro (un nº de versión no es secreto) en vez del base64/JSON de los otros
// listados; el cliente lo trata igual como token opaco que reenvía.
func decodeVersionCursor(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(token)
	if err != nil || n <= 0 {
		return 0, domain.ErrInvalidName
	}
	return n, nil
}

// Search busca ficheros por nombre.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]Entry, error) {
	return s.store.SearchFiles(ctx, query, limit)
}

// FilePage es una página de ficheros de una carpeta con el token de continuación
// ya codificado (opaco) para la siguiente petición ("" si no hay más).
type FilePage struct {
	Files      []domain.File
	NextCursor string
}

// ListFilesPage lista los ficheros de una carpeta con paginación keyset. El
// cursor es un token opaco devuelto por la página anterior (vacío = inicio).
func (s *Service) ListFilesPage(ctx context.Context, folderID uuid.UUID, cursor string, limit int) (FilePage, error) {
	after, err := decodeFileCursor(cursor)
	if err != nil {
		return FilePage{}, err
	}
	page, err := s.store.ListFilesPage(ctx, folderID, after, limit)
	if err != nil {
		return FilePage{}, err
	}
	out := FilePage{Files: page.Files}
	if page.Next != nil {
		out.NextCursor = encodeFileCursor(*page.Next)
	}
	return out, nil
}

// encodeFileCursor serializa un cursor keyset a un token opaco URL-safe. El
// formato es interno (JSON base64); el cliente lo trata como una cadena opaca.
func encodeFileCursor(c store.FileCursor) string {
	b, err := json.Marshal(cursorWire{N: c.NameLower, I: c.ID.String()})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeFileCursor invierte encodeFileCursor. Un token vacío es la primera
// página; un token corrupto es una entrada inválida (no un fallo de servidor).
func decodeFileCursor(token string) (store.FileCursor, error) {
	if token == "" {
		return store.FileCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return store.FileCursor{}, domain.ErrInvalidName
	}
	var w cursorWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return store.FileCursor{}, domain.ErrInvalidName
	}
	id, err := uuid.Parse(w.I)
	if err != nil {
		return store.FileCursor{}, domain.ErrInvalidName
	}
	return store.FileCursor{NameLower: w.N, ID: id}, nil
}

// cursorWire es la forma serializada de un FileCursor (claves cortas).
type cursorWire struct {
	N string `json:"n"`
	I string `json:"i"`
}

// ---------------------------------------------------------------------------
// Recientes y Favoritos
// ---------------------------------------------------------------------------

// RecentPage es una página de «Recientes» con el token de continuación keyset ya
// codificado (opaco) para la siguiente petición ("" si no hay más).
type RecentPage struct {
	Entries    []Entry
	NextCursor string
}

// Recent lista los ficheros del tenant por reciente-primero (updated_at DESC),
// con paginación keyset. El cursor es un token opaco de la página anterior.
func (s *Service) Recent(ctx context.Context, cursor string, limit int) (RecentPage, error) {
	after, err := decodeRecentCursor(cursor)
	if err != nil {
		return RecentPage{}, err
	}
	entries, next, err := s.store.ListRecent(ctx, after, limit)
	if err != nil {
		return RecentPage{}, err
	}
	out := RecentPage{Entries: entries}
	if next != nil {
		out.NextCursor = encodeRecentCursor(*next)
	}
	return out, nil
}

// Favorites lista los favoritos vivos de un usuario (carpetas y ficheros).
func (s *Service) Favorites(ctx context.Context, subject string) ([]Entry, error) {
	return s.store.ListFavorites(ctx, subject)
}

// SetFavorite marca/desmarca un fichero o carpeta como favorito del usuario.
// Exige exactamente un objetivo (fichero XOR carpeta).
func (s *Service) SetFavorite(ctx context.Context, subject string, fileID, folderID *uuid.UUID, on bool) error {
	if (fileID == nil) == (folderID == nil) {
		return domain.ErrInvalidName
	}
	return s.store.SetFavorite(ctx, subject, fileID, folderID, on)
}

// encodeRecentCursor serializa un RecentCursor a un token opaco URL-safe (JSON
// base64; el cliente lo trata como una cadena opaca).
func encodeRecentCursor(c store.RecentCursor) string {
	b, err := json.Marshal(recentWire{U: c.UpdatedAt, I: c.ID.String()})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// decodeRecentCursor invierte encodeRecentCursor. Token vacío = primera página;
// token corrupto = entrada inválida (no un fallo de servidor).
func decodeRecentCursor(token string) (store.RecentCursor, error) {
	if token == "" {
		return store.RecentCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return store.RecentCursor{}, domain.ErrInvalidName
	}
	var w recentWire
	if err := json.Unmarshal(raw, &w); err != nil {
		return store.RecentCursor{}, domain.ErrInvalidName
	}
	id, err := uuid.Parse(w.I)
	if err != nil {
		return store.RecentCursor{}, domain.ErrInvalidName
	}
	return store.RecentCursor{UpdatedAt: w.U, ID: id}, nil
}

// recentWire es la forma serializada de un RecentCursor (claves cortas).
type recentWire struct {
	U time.Time `json:"u"`
	I string    `json:"i"`
}

// Quota devuelve la cuota del tenant (inicializándola si hace falta).
func (s *Service) Quota(ctx context.Context) (domain.Quota, error) {
	return s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes)
}

// SetQuotaLimit fija el límite de cuota (acción de admin).
func (s *Service) SetQuotaLimit(ctx context.Context, limit int64) (domain.Quota, error) {
	return s.store.SetQuotaLimit(ctx, limit)
}

// CreateShareInput son los datos de entrada para compartir.
type CreateShareInput struct {
	FileID     *uuid.UUID
	FolderID   *uuid.UUID
	Kind       domain.ShareKind
	Subject    string
	Permission domain.Permission
	// Password protege un enlace público (solo kind=link). Vacío = sin contraseña.
	Password  string
	ExpiresAt *time.Time
	CreatedBy string
}

// CreateShare crea una compartición. Para enlaces genera un token seguro.
//
// ENFORCEMENT de la etiqueta de sensibilidad: si el fichero lleva una etiqueta con
// BlockExternalShare, NO se crea el enlace público (ErrLabelBlocksSharing → 403 con el
// motivo). Es el ÚNICO punto por el que nace un share kind=link (la capa HTTP propia,
// la de compatibilidad Google/Graph y WebDAV pasan todas por aquí), así que la puerta
// se cierra una vez y para todos los llamantes. Compartir con personas o grupos de la
// organización sigue funcionando igual: lo que se bloquea es la salida FUERA.
func (s *Service) CreateShare(ctx context.Context, in CreateShareInput) (domain.Share, error) {
	if (in.FileID == nil) == (in.FolderID == nil) {
		return domain.Share{}, domain.ErrInvalidName
	}
	if in.Kind == domain.ShareLink && in.FileID != nil {
		f, err := s.store.GetFile(ctx, *in.FileID)
		if err != nil {
			return domain.Share{}, err
		}
		if f.Label != nil && f.Label.BlockExternalShare {
			return domain.Share{}, fmt.Errorf("%w (%s)", domain.ErrLabelBlocksSharing, f.Label.Name)
		}
	}
	if in.Permission == "" {
		in.Permission = domain.PermRead
	}
	rec := store.NewShareRecord{
		FileID:     in.FileID,
		FolderID:   in.FolderID,
		Kind:       in.Kind,
		Subject:    in.Subject,
		Permission: in.Permission,
		CreatedBy:  in.CreatedBy,
		ExpiresAt:  in.ExpiresAt,
	}
	if in.Kind == domain.ShareLink {
		tok, err := secureToken()
		if err != nil {
			return domain.Share{}, err
		}
		rec.Token = tok
		// La contraseña solo tiene sentido en enlaces públicos; se guarda hasheada
		// (Argon2id), nunca en claro. En user/group se ignora.
		if in.Password != "" {
			hash, err := domain.HashPassword(in.Password)
			if err != nil {
				return domain.Share{}, err
			}
			rec.PasswordHash = hash
		}
	}
	sh, err := s.store.CreateShare(ctx, rec)
	if err != nil {
		return domain.Share{}, err
	}
	s.publish(ctx, "drive.share.created", map[string]any{"share_id": sh.ID, "kind": sh.Kind})

	s.emitShareNotification(ctx, in, sh)
	return sh, nil
}

func (s *Service) emitShareNotification(ctx context.Context, in CreateShareInput, sh domain.Share) {
	if s.hubNotifier == nil || sh.Kind != domain.ShareUser || sh.Subject == "" {
		return
	}
	targetName := "Elemento"
	nodo := ""
	if in.FileID != nil {
		nodo = in.FileID.String()
	} else if in.FolderID != nil {
		nodo = in.FolderID.String()
	}
	if in.FileID != nil {
		if f, err := s.store.GetFile(ctx, *in.FileID); err == nil && f.Name != "" {
			targetName = f.Name
		}
	} else if in.FolderID != nil {
		if f, err := s.store.GetFolder(ctx, *in.FolderID); err == nil && f.Name != "" {
			targetName = f.Name
		}
	}
	// Quién comparte, por su nombre: `CreatedBy` es su `sub`, y el aviso decía
	// «0b6a4c8e-… ha compartido…». Sin nombre, el título de respaldo no lo nombra y
	// sin `actor` los clientes usan ese respaldo.
	actor := ""
	if c, ok := authn.FromContext(ctx); ok {
		actor = cmp.Or(strings.TrimSpace(c.Name), c.Email)
	}
	t, _ := tenancy.From(ctx)
	s.hubNotifier.SendAsync(ctx, avisoDeCompartido(s.cfg.PublicURL, t.Slug, sh.Subject, nodo, targetName, actor))
}

// avisoDeCompartido es el aviso de «compartido contigo»: el mismo en vivo que en la
// reconstrucción de avisos (AvisosHistoricos). Sin `actor`, el título no nombra a nadie.
func avisoDeCompartido(publicURL, tenant, para, nodo, targetName, actor string) hubnotify.Notification {
	return hubnotify.Compartido(tenant, para, "drive", "drive.share.created", targetName, actor,
		hubnotify.EnlaceCompartido(publicURL, tenant, nodo))
}

// ListFileShares lista comparticiones de un fichero.
func (s *Service) ListFileShares(ctx context.Context, id uuid.UUID) ([]domain.Share, error) {
	return s.store.ListSharesForFile(ctx, id)
}

// ListFolderShares lista comparticiones de una carpeta.
func (s *Service) ListFolderShares(ctx context.Context, id uuid.UUID) ([]domain.Share, error) {
	return s.store.ListSharesForFolder(ctx, id)
}

// RevokeShare revoca una compartición.
func (s *Service) RevokeShare(ctx context.Context, id uuid.UUID) error {
	return s.store.RevokeShare(ctx, id)
}

// PublicNode es el resultado de navegar un enlace público: o una carpeta con su
// listado (IsDir) o un fichero con su contenido abierto (Body; el llamador cierra).
type PublicNode struct {
	IsDir      bool
	Folder     domain.Folder   // carpeta actual (IsDir)
	Subfolders []domain.Folder // subcarpetas (IsDir)
	Files      []domain.File   // ficheros (IsDir), primera página
	More       bool            // hay más ficheros de los listados (IsDir)
	File       domain.File     // fichero resuelto (!IsDir)
	Body       io.ReadCloser   // contenido del fichero (!IsDir); cerrar tras usar
}

// publicListLimit acota los ficheros listados por página de carpeta pública.
// ponytail: la página pública no ofrece "cargar más"; los primeros N bastan para
// el caso común. Si un enlace a carpeta con >N ficheros lo pide, paginar keyset.
const publicListLimit = 500

// publicProof deriva una prueba-capacidad por-enlace de su hash de contraseña.
// Es SHA-256(hash): el visitante solo la obtiene tras acertar la contraseña
// (PublicVerify), no es reversible al hash ni a la contraseña, y viaja de una
// petición a otra por cookie sin necesitar un secreto de servicio. En el camino
// común (cada request de un enlace protegido) solo cuesta un SHA-256, no un Argon2.
// ponytail: sin secreto global; si se añade uno, subir a HMAC(secreto, share.id).
func publicProof(passwordHash string) string {
	sum := sha256.Sum256([]byte(passwordHash))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// PublicResolve resuelve un token de enlace y aplica la verja de contraseña.
// Devuelve el share y su tenant. Errores: ErrShareExpired (token inválido o
// caducado), ErrPasswordRequired (protegido y la prueba falta o no coincide).
func (s *Service) PublicResolve(ctx context.Context, token, proof string) (domain.Share, string, error) {
	sh, tenantSlug, hash, err := s.store.ResolveLink(ctx, token)
	if err != nil {
		return domain.Share{}, "", err
	}
	if hash != "" {
		want := publicProof(hash)
		if subtle.ConstantTimeCompare([]byte(proof), []byte(want)) != 1 {
			return domain.Share{}, "", domain.ErrPasswordRequired
		}
	}
	return sh, tenantSlug, nil
}

// PublicVerify comprueba una contraseña contra un enlace protegido y devuelve la
// prueba (valor de cookie) que concede el acceso posterior. Para un enlace sin
// contraseña devuelve "" sin error. Errores: ErrShareExpired, ErrWrongPassword.
func (s *Service) PublicVerify(ctx context.Context, token, password string) (string, error) {
	_, _, hash, err := s.store.ResolveLink(ctx, token)
	if err != nil {
		return "", err
	}
	if hash == "" {
		return "", nil // no protegido: no hace falta prueba
	}
	ok, err := domain.VerifyPassword(password, hash)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", domain.ErrWrongPassword
	}
	return publicProof(hash), nil
}

// PublicBrowse navega dentro de un share ya resuelto (verja de contraseña ya
// superada). subpath "" = raíz del share. Si resuelve a carpeta, devuelve su
// listado (subcarpetas + primera página de ficheros); si a fichero, lo abre
// (Body). La resolución camina por NOMBRE desde la raíz del share, así que no
// puede escapar del subárbol compartido hacia arriba.
func (s *Service) PublicBrowse(ctx context.Context, share domain.Share, tenantSlug, subpath string) (PublicNode, error) {
	t, err := tenancy.New(tenantSlug)
	if err != nil {
		return PublicNode{}, err
	}
	tctx := tenancy.Into(ctx, t)
	segs := splitPath(subpath)

	// Enlace a un único fichero: no admite subruta; devuelve el fichero.
	if share.FileID != nil {
		if len(segs) != 0 {
			return PublicNode{}, domain.ErrNotFound
		}
		f, err := s.store.GetFile(tctx, *share.FileID)
		if err != nil {
			return PublicNode{}, err
		}
		return s.publicFileNode(tctx, t, f)
	}
	if share.FolderID == nil {
		return PublicNode{}, domain.ErrNotFound
	}

	// Enlace a carpeta: el descenso por la subruta lo hace el store en UNA
	// transacción. Antes iba tramo a tramo —una consulta por segmento, cada una
	// trayendo TODAS las subcarpetas del padre para quedarse con una—, y esto es
	// una ruta PÚBLICA: la paga cualquiera que abra un enlace compartido, y el
	// coste crecía con lo hondo que estuviera la carpeta.
	curID := *share.FolderID
	// El resolutor ya devuelve la carpeta ENTERA, así que cuando hay subruta no
	// hace falta volver a pedirla abajo: es una transacción menos.
	var destino *domain.Folder
	if len(segs) > 0 {
		carpeta, fichero, err := s.store.ResolveUnder(tctx, curID, segs)
		if err != nil {
			return PublicNode{}, err
		}
		// LA CARPETA MANDA, y no es lo mismo que en `ResolvePath`: el recorrido de
		// antes miraba las subcarpetas primero y solo caía al fichero si no había
		// ninguna con ese nombre. Se conserva tal cual.
		switch {
		case carpeta != nil:
			curID = carpeta.ID
			destino = carpeta
		case fichero != nil:
			return s.publicFileNode(tctx, t, *fichero)
		default:
			return PublicNode{}, domain.ErrNotFound
		}
	}

	// curID es la carpeta destino: devuelve su listado. Solo se pide si no venía
	// ya resuelta (enlace a la raíz del compartido, sin subruta).
	var folder domain.Folder
	if destino != nil {
		folder = *destino
	} else {
		f, err := s.store.GetFolder(tctx, curID)
		if err != nil {
			return PublicNode{}, err
		}
		folder = f
	}
	subs, err := s.store.ListSubfolders(tctx, curID)
	if err != nil {
		return PublicNode{}, err
	}
	page, err := s.store.ListFilesPage(tctx, curID, store.FileCursor{}, publicListLimit)
	if err != nil {
		return PublicNode{}, err
	}
	return PublicNode{
		IsDir:      true,
		Folder:     folder,
		Subfolders: subs,
		Files:      withoutBlockedByLabel(page.Files),
		More:       page.Next != nil,
	}, nil
}

// withoutBlockedByLabel quita del listado público los ficheros cuya etiqueta prohíbe
// la compartición externa: dentro de una carpeta compartida por enlace no se anuncian
// (y publicFileNode tampoco los sirve si alguien adivina el nombre).
func withoutBlockedByLabel(files []domain.File) []domain.File {
	out := files[:0]
	for _, f := range files {
		if f.Label != nil && f.Label.BlockExternalShare {
			continue
		}
		out = append(out, f)
	}
	return out
}

// publicFileNode abre el blob de un fichero para servirlo por el enlace público.
func (s *Service) publicFileNode(ctx context.Context, t tenancy.Tenant, f domain.File) (PublicNode, error) {
	// Verja de etiqueta AL SERVIR, no solo al crear el enlace: cierra los dos huecos
	// que dejaría comprobar solo en la creación —un fichero etiquetado DESPUÉS de
	// crearse su enlace, y un fichero etiquetado dentro de una carpeta compartida por
	// enlace—. Se responde 404 (no 403): para quien mira desde fuera, el fichero
	// simplemente no está en el enlace (el listado tampoco lo muestra).
	if f.Label != nil && f.Label.BlockExternalShare {
		return PublicNode{}, domain.ErrNotFound
	}
	rc, err := s.blobs.Get(ctx, t, f.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return PublicNode{}, domain.ErrNotFound
		}
		return PublicNode{}, err
	}
	return PublicNode{File: f, Body: rc}, nil
}

// ---------------------------------------------------------------------------
// Compartido conmigo + acceso por share (kind=user|group)
// ---------------------------------------------------------------------------

// SharedWithMe lista los recursos compartidos con el usuario (por su sub o sus
// grupos del JWT) vía shares kind=user|group vigentes.
func (s *Service) SharedWithMe(ctx context.Context, subject string, groups []string) ([]SharedEntry, error) {
	return s.store.ListSharedWithMe(ctx, subject, groups)
}

// SharedBrowse navega un recurso compartido CON el usuario, aplicando la VERJA de
// acceso: el usuario debe ser destinatario de un share vigente sobre el recurso
// raíz `rootID` (kind=user con su sub, o kind=group con uno de sus grupos); si no,
// ErrNotFound (deny-by-default, sin filtrar la existencia). subpath "" = raíz del
// share. Reutiliza PublicBrowse (mismo recorrido por-nombre acotado al subárbol
// compartido, que no puede escapar hacia arriba) y devuelve además el permiso
// concedido (viewer/editor) para que el cliente oculte la escritura a los viewers.
func (s *Service) SharedBrowse(ctx context.Context, subject string, groups []string, rootID uuid.UUID, subpath string) (PublicNode, domain.Permission, error) {
	sh, err := s.store.ShareForResource(ctx, subject, groups, rootID)
	if err != nil {
		return PublicNode{}, "", err
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return PublicNode{}, "", err
	}
	node, err := s.PublicBrowse(ctx, sh, t.Slug, subpath)
	return node, sh.Permission, err
}

// SharedUpload sube un fichero DENTRO de una carpeta compartida con el usuario,
// exigiendo permiso de escritura (editor): un viewer recibe ErrPermission. La
// carpeta destino se resuelve caminando subpath por nombre desde la raíz del share
// (no puede escapar del subárbol compartido).
func (s *Service) SharedUpload(ctx context.Context, subject string, groups []string, rootID uuid.UUID, subpath, name, contentType, ownerSub string, body io.Reader) (domain.File, error) {
	sh, err := s.store.ShareForResource(ctx, subject, groups, rootID)
	if err != nil {
		return domain.File{}, err
	}
	if sh.Permission != domain.PermWrite {
		return domain.File{}, domain.ErrPermission
	}
	if sh.FolderID == nil {
		return domain.File{}, domain.ErrPermission // solo se sube dentro de una carpeta compartida
	}
	destID, err := s.resolveSharedFolder(ctx, *sh.FolderID, subpath)
	if err != nil {
		return domain.File{}, err
	}
	return s.UploadFile(ctx, destID, name, contentType, ownerSub, body)
}

// resolveSharedFolder camina subpath (solo carpetas) por nombre desde rootID y
// devuelve la carpeta destino. Como PublicBrowse, solo desciende: no puede escapar
// hacia arriba del subárbol compartido.
func (s *Service) resolveSharedFolder(ctx context.Context, rootID uuid.UUID, subpath string) (uuid.UUID, error) {
	cur := rootID
	for _, seg := range splitPath(subpath) {
		subs, err := s.store.ListSubfolders(ctx, cur)
		if err != nil {
			return uuid.Nil, err
		}
		var next *uuid.UUID
		for i := range subs {
			if strings.EqualFold(subs[i].Name, seg) {
				id := subs[i].ID
				next = &id
				break
			}
		}
		if next == nil {
			return uuid.Nil, domain.ErrNotFound
		}
		cur = *next
	}
	return cur, nil
}

// blobKey deriva una clave S3 estable y opaca para un fichero nuevo. No incluye
// el prefijo de tenant (lo añade objectstore). Estructura: files/<folder>/<rand>.
func blobKey(folderID uuid.UUID, _ string) string {
	r := make([]byte, 16)
	_, _ = rand.Read(r)
	return fmt.Sprintf("files/%s/%s", folderID.String(), hex.EncodeToString(r))
}

// secureToken genera un token URL-safe de 256 bits para enlaces públicos.
func secureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("service: generar token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// countingReader cuenta los bytes leídos a través de él.
type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
