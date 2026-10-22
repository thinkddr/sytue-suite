// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Adjuntos de canal de Sytue Rooms: la VERJA. Un fichero subido a un mensaje de un
// canal (incluido un canal PRIVADO o un DM) pertenece al CANAL, no a «Mi unidad»,
// y solo lo puede leer quien sea MIEMBRO del canal EN EL MOMENTO de la petición.
//
// Modelo de permiso (Mattermost/Slack: se evalúa en cada petición contra el roster
// vivo, NO se replica una ACL en Drive):
//
//   - Fuente de verdad: channel_members de services/rooms, consultada aquí en cada
//     petición vía ChannelAuthorizer. Salir del canal —o del equipo, para un
//     privado— revoca la lectura AL INSTANTE, sin ventana ni trabajo de limpieza:
//     es justo la clase de fallo que el arreglo de la baja de equipo (46b9fde) tuvo
//     que corregir por replicar el estado en dos sitios.
//   - Canal público: lo lee cualquier miembro del equipo; privado: sus miembros (que
//     además siguen en el equipo); DM: sus participantes. Exactamente la misma verja
//     que leer el MENSAJE, así que un adjunto nunca es más visible que su mensaje.
//   - Canal archivado: sus miembros siguen leyendo los adjuntos (archivar corta la
//     ESCRITURA, no la lectura), como en Mattermost.
//   - Deny-by-default y sin filtrar existencia: no-miembro, fichero de OTRO canal o
//     fichero inexistente ⇒ ErrNotFound (404), nunca 403.
//   - Fail-closed: si Sytue Rooms no responde (o no hay secreto interno configurado),
//     NO se sirve el adjunto.
//
// Los dos tipos de adjunto NO son iguales, y se tratan distinto a propósito:
//
//  1. SUBIDO desde el compositor ⇒ ámbito de canal (channel_id, mig 0014). Es
//     confidencial del canal: invisible en el Drive de cualquiera, aunque sea admin.
//  2. ADJUNTADO POR REFERENCIA desde el propio Drive del usuario (selector «Archivo
//     de Sytue Drive») ⇒ NO se mueve ni se copia: sigue siendo el MISMO fichero de
//     Drive con SUS permisos («Mi unidad» privada por owner_sub desde la mig 0032),
//     de modo que las versiones y la cuota no se bifurcan. Leerlo A TRAVÉS de Rooms
//     exige además membresía del canal.
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

// ChannelAuthorizer resuelve si un sujeto puede LEER un canal de Sytue Rooms. La
// implementación real es *ChatClient (chat.go), que pregunta a services/rooms con el
// secreto interno compartido. nil = puede; domain.ErrNotFound = no puede o el canal
// no existe (sin distinguir: no se filtra la existencia).
type ChannelAuthorizer interface {
	CanAccessChannel(ctx context.Context, channelID uuid.UUID, subject string) error
}

// ChannelFile aplica la verja y devuelve los metadatos del adjunto (nombre/tipo/
// tamaño, que es lo que Rooms denormaliza en el mensaje al publicarlo). Primero
// MEMBRESÍA del canal (sin ella nada existe), después el fichero en el ámbito de ESE
// canal; si ahí no está, cae al ámbito personal para el adjunto por REFERENCIA (y
// para los adjuntos heredados de antes de la mig 0014, que siguen en «Mi unidad»).
// Es la ÚNICA verja: la comparten los metadatos, el contenido y la miniatura.
func (s *Service) ChannelFile(ctx context.Context, channelID, fileID uuid.UUID, subject string) (domain.File, error) {
	if s.chat == nil {
		// Sin verja configurada no se sirve un adjunto de canal: fail-closed.
		return domain.File{}, domain.ErrNotFound
	}
	if subject == "" || channelID == uuid.Nil || fileID == uuid.Nil {
		return domain.File{}, domain.ErrNotFound
	}
	if err := s.chat.CanAccessChannel(ctx, channelID, subject); err != nil {
		return domain.File{}, err
	}
	f, err := s.team.ChannelFile(ctx, channelID, fileID)
	if err == nil {
		return f, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.File{}, err
	}
	// Ámbito personal: fichero adjuntado por referencia o adjunto heredado.
	if store.OwnerFrom(ctx) == "" {
		ctx = store.WithOwner(ctx, subject)
	}
	return s.store.GetFile(ctx, fileID)
}

// ChannelFileContent abre el contenido del adjunto desde S3 (el llamador cierra).
func (s *Service) ChannelFileContent(ctx context.Context, channelID, fileID uuid.UUID, subject string) (domain.File, io.ReadCloser, error) {
	f, err := s.ChannelFile(ctx, channelID, fileID, subject)
	if err != nil {
		return domain.File{}, nil, err
	}
	t, _ := tenancy.From(ctx)
	rc, err := s.blobs.Get(ctx, t, f.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return domain.File{}, nil, domain.ErrNotFound
		}
		return domain.File{}, nil, fmt.Errorf("service: abrir blob de adjunto: %w", err)
	}
	return f, rc, nil
}

// ChannelThumbnail sirve la miniatura del adjunto (misma generación perezosa y
// caché que la de «Mi unidad»; aquí solo cambia la verja de acceso).
func (s *Service) ChannelThumbnail(ctx context.Context, channelID, fileID uuid.UUID, subject string) (io.ReadCloser, string, string, error) {
	f, err := s.ChannelFile(ctx, channelID, fileID, subject)
	if err != nil {
		return nil, "", "", err
	}
	return s.thumbnailOf(ctx, f)
}

// ChannelUploadFile sube un adjunto AL CANAL: exige membresía, guarda el blob en S3
// y registra el metadato en el ámbito del canal (carpeta propia + channel_id), de
// modo que nace invisible para el resto de la organización. Es el camino del
// compositor de Rooms (arrastrar, pegar, «Subir archivo»).
func (s *Service) ChannelUploadFile(ctx context.Context, channelID uuid.UUID, subject, name, contentType string, body io.Reader) (domain.File, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.File{}, err
	}
	if !domain.ValidName(name) {
		return domain.File{}, domain.ErrInvalidName
	}
	if s.chat == nil || subject == "" || channelID == uuid.Nil {
		return domain.File{}, domain.ErrNotFound
	}
	// Verja ANTES de gastar cuota, ancho de banda y un objeto en S3.
	if err := s.chat.CanAccessChannel(ctx, channelID, subject); err != nil {
		return domain.File{}, err
	}
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.File{}, err
	}

	storageKey, size, checksum, err := s.putBlob(ctx, t, blobKey(channelID, name), name, contentType, subject, body)
	if err != nil {
		return domain.File{}, err
	}
	storageKey = s.compartirBlob(ctx, t, storageKey, checksum)
	f, err := s.team.CreateChannelFile(ctx, channelID, store.NewFileRecord{
		Name:        name,
		StorageKey:  storageKey,
		SizeBytes:   size,
		ContentType: contentType,
		Checksum:    checksum,
		OwnerSub:    subject,
	})
	if err != nil {
		// Compensación: si el metadato no entra (cuota/conflicto), suelta el blob.
		s.soltarBlob(ctx, t, storageKey)
		return domain.File{}, err
	}
	// Evento SIN nombre de fichero ni canal: el bus lo consumen relays de toda la
	// suite y el nombre de un adjunto de un privado es dato del canal.
	s.publish(ctx, "drive.channel_file.created", map[string]any{"file_id": f.ID, "size": f.SizeBytes})
	return f, nil
}

// putBlob sube el cuerpo a S3 bajo `key` calculando tamaño y checksum en streaming,
// acotado por MaxUploadBytes (si se pasa, borra el objeto y devuelve
// ErrQuotaExceeded). Lo comparten la subida a «Mi unidad» y la de un canal.
func (s *Service) putBlob(ctx context.Context, t tenancy.Tenant, key, name, contentType, ownerSub string, body io.Reader) (string, int64, string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	limited := io.LimitReader(body, s.cfg.MaxUploadBytes+1)
	hasher := sha256.New()
	counter := &countingReader{r: io.TeeReader(limited, hasher)}
	// minio requiere el tamaño; como llega por stream sin Content-Length fiable,
	// usamos -1 para subida en multipart con tamaño desconocido. El -1 obliga a
	// acotar el buffer de parte en objectstore (ver tamanoDeParte): sin eso minio
	// reserva ~524 MiB por parte para cubrir el peor caso y el pod muere por OOM
	// subiendo un fichero de kilobytes.
	if err := s.blobs.Put(ctx, t, key, counter, -1, objectstore.PutOptions{
		ContentType: contentType,
		Metadata:    map[string]string{"original-name": name, "owner": ownerSub},
	}); err != nil {
		return "", 0, "", fmt.Errorf("service: subir blob: %w", err)
	}
	if counter.n > s.cfg.MaxUploadBytes {
		_ = s.blobs.Delete(ctx, t, key)
		return "", 0, "", domain.ErrQuotaExceeded
	}
	return key, counter.n, hex.EncodeToString(hasher.Sum(nil)), nil
}

// CopyChannelFileToMyDrive copia un adjunto de canal al espacio del usuario («Guardar en
// mi Drive»): el equivalente del «Save to Drive» de Slack y el «Copiar a» de Teams.
//
// La AUTORIZACIÓN es la misma verja de siempre —membresía del canal, vía ChannelFile—, y
// no la del ámbito personal: el adjunto vive en la carpeta del canal, invisible para el
// resto de la organización, así que un permiso de «Mi unidad» no diría nada útil sobre
// él. Quien puede ver el mensaje puede guardarse el fichero; quien no está en el canal
// recibe ErrNotFound sin distinguir si existe.
//
// La copia NO duplica bytes: `store.CopyFile` referencia el MISMO blob (igual que la
// copia dentro de Drive). Lo que se crea es un metadato nuevo en el ámbito del usuario,
// de modo que el fichero sobreviva a que el mensaje se borre o le echen del canal — que
// es justo el sentido de guardárselo.
func (s *Service) CopyChannelFileToMyDrive(ctx context.Context, channelID, fileID uuid.UUID, subject string) (domain.File, error) {
	if store.OwnerFrom(ctx) == "" {
		ctx = store.WithOwner(ctx, subject)
	}
	// Verja de canal: la ÚNICA que autoriza esto.
	if _, err := s.ChannelFile(ctx, channelID, fileID, subject); err != nil {
		return domain.File{}, err
	}
	// La cuota se comprueba aunque el blob se comparta: lo que se contabiliza son los
	// bytes lógicos del usuario, no el almacenamiento físico.
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return domain.File{}, err
	}
	root, err := s.store.EnsureRoot(ctx)
	if err != nil {
		return domain.File{}, err
	}
	// Un adjunto subido al canal vive en el ámbito del canal: se copia cruzando de
	// ámbito a propósito. Si no está ahí es un fichero de «Mi unidad» adjuntado por
	// referencia (o uno heredado de antes de la mig 0014), y se copia como cualquiera.
	var f domain.File
	err = domain.ErrNotFound
	if s.team != nil {
		f, err = s.team.CopiarAdjuntoAMiUnidad(ctx, channelID, fileID, root.ID, subject)
	}
	if errors.Is(err, domain.ErrNotFound) {
		f, err = s.store.CopyFile(ctx, fileID, root.ID, subject)
	}
	if err != nil {
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.copied", map[string]any{
		"file_id": f.ID, "source_id": fileID, "name": f.Name, "size": f.SizeBytes,
		"from_channel": channelID,
	})
	return f, nil
}
