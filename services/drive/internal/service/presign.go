// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Subida directa a S3 con URL prefirmada (descarga la subida de ficheros grandes
// del app-server). Dos pasos:
//
//	PresignUpload  comprueba permiso (acceso a la carpeta destino, RLS) y cuota
//	               ANTES de firmar, reserva una storage_key única e impredecible y
//	               devuelve una URL PUT prefirmada acotada (clave+método+caducidad)
//	               + un handle firmado (HMAC) que encapsula lo autorizado.
//	FinalizeUpload verifica el handle, que el objeto llegó a S3 y su tamaño REAL
//	               (HEAD; no confía en el cliente), registra el metadato reservando
//	               cuota atómicamente y ata el blob. Idempotente.
//
// Seguridad (doble barrera): la firma S3 ata la URL a esa clave/método (el cliente
// no puede subir a otra clave ni sobreescribir otra); el handle HMAC ata la clave,
// carpeta, nombre, tamaño y tenant para finalize (el cliente no puede forjarlos).
// El tamaño y la existencia se comprueban en S3 en finalize, no se creen al cliente.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"

	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/objectstore"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/store"
)

// Errores propios de la finalización de subidas directas (los mapea la capa HTTP).
var (
	// ErrPresignHandleInvalid: el handle no verifica (firma inválida) o caducó.
	ErrPresignHandleInvalid = errors.New("service: handle de subida inválido o caducado")
	// ErrUploadNotFound: no hay objeto en S3 para la clave reservada (la subida
	// directa no llegó a completarse).
	ErrUploadNotFound = errors.New("service: la subida directa no llegó a S3")
	// ErrUploadSizeMismatch: el tamaño real en S3 no casa con lo autorizado.
	ErrUploadSizeMismatch = errors.New("service: el tamaño subido no coincide con lo autorizado")
)

// sufijoFirme nombra la copia de una subida directa que el cliente ya no puede
// tocar (ver FinalizeUpload). Va pegado a la clave reservada para que un reintento
// de finalize la encuentre sin guardar nada más.
const sufijoFirme = ".firme"

// blobStat añade HEAD (tamaño/existencia) sobre *objectstore.Client, que no expone
// StatObject. Reusa Get —que ya hace obj.Stat() y traduce objectstore.ErrNotFound—
// y minio cachea el ObjectInfo, así que la segunda lectura no repite red ni descarga
// el cuerpo. Vive aquí (no en platform/pkg) porque el objectstore queda fuera de
// este cambio; el prefijo de tenant lo sigue aplicando el objectstore en Get.
type blobStat struct{ *objectstore.Client }

// Stat devuelve el tamaño del objeto (bytes). objectstore.ErrNotFound si no existe.
func (b blobStat) Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error) {
	rc, err := b.Get(ctx, t, key)
	if err != nil {
		return 0, err // incluye objectstore.ErrNotFound si el objeto no llegó
	}
	defer func() { _ = rc.Close() }()
	o, ok := rc.(interface {
		Stat() (minio.ObjectInfo, error)
	})
	if !ok {
		return 0, fmt.Errorf("service: el blob no expone Stat")
	}
	info, err := o.Stat()
	if err != nil {
		return 0, fmt.Errorf("service: stat blob: %w", err)
	}
	return info.Size, nil
}

var _ blobStore = blobStat{}

// PresignResult es la respuesta de PresignUpload: la URL PUT prefirmada (a la que
// el cliente sube el fichero directamente), el handle firmado para finalize y
// cuándo caduca la URL (el PUT debe completarse antes).
type PresignResult struct {
	URL       string
	Handle    string
	ExpiresAt time.Time
}

// uploadHandle es la carga firmada del handle: encapsula TODO lo autorizado en el
// presign para que finalize no dependa de datos del cliente. Va firmada (HMAC) y
// codificada en el handle; el cliente la trata como opaca.
type uploadHandle struct {
	Tenant      string `json:"t"`
	FolderID    string `json:"f"`
	Name        string `json:"n"`
	StorageKey  string `json:"k"`
	Size        int64  `json:"s"`
	ContentType string `json:"c"`
	Owner       string `json:"o"`
	Expires     int64  `json:"e"` // unix seconds
}

// handleKey deriva la clave HMAC del handle a partir del secreto S3 (server-only,
// estable y compartido entre réplicas vía env), con separación de dominio para no
// reutilizar el secreto en crudo.
// ponytail: reusa el secreto S3 como raíz HMAC; si algún día se quiere rotar
// independientemente, introducir PRESIGN_SIGNING_KEY en config.
func (s *Service) handleKey() []byte {
	sum := sha256.Sum256([]byte("sytue-drive/presign-finalize/v1\x00" + s.cfg.S3SecretKey))
	return sum[:]
}

// signHandle serializa y firma (HMAC-SHA256) la carga: base64url(payload).base64url(sig).
func (s *Service) signHandle(h uploadHandle) (string, error) {
	payload, err := json.Marshal(h)
	if err != nil {
		return "", fmt.Errorf("service: serializar handle: %w", err)
	}
	mac := hmac.New(sha256.New, s.handleKey())
	mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

// verifyHandle valida la firma (tiempo constante) y decodifica la carga. NO
// comprueba caducidad ni tenant (eso lo hace FinalizeUpload con el contexto).
func (s *Service) verifyHandle(handle string) (uploadHandle, error) {
	var h uploadHandle
	dot := strings.IndexByte(handle, '.')
	if dot <= 0 || dot >= len(handle)-1 {
		return h, ErrPresignHandleInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(handle[:dot])
	if err != nil {
		return h, ErrPresignHandleInvalid
	}
	sig, err := base64.RawURLEncoding.DecodeString(handle[dot+1:])
	if err != nil {
		return h, ErrPresignHandleInvalid
	}
	mac := hmac.New(sha256.New, s.handleKey())
	mac.Write(payload)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return h, ErrPresignHandleInvalid
	}
	if err := json.Unmarshal(payload, &h); err != nil {
		return h, ErrPresignHandleInvalid
	}
	return h, nil
}

// PresignUpload autoriza una subida directa a S3. Comprueba permiso (resuelve la
// carpeta destino desde la ruta: misma verja RLS que la subida por WebDAV; ajeno o
// inexistente → ErrNotFound) y cuota ANTES de firmar. Reserva una storage_key
// única e impredecible (files/<carpeta>/<rand>) y firma un handle con lo
// autorizado. Los bytes NO pasan por el app-server. El pre-chequeo de cuota es
// blando; la reserva dura y atómica la hace finalize (CreateFile → reserveQuota).
func (s *Service) PresignUpload(ctx context.Context, path string, size int64, contentType, ownerSub string) (PresignResult, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return PresignResult{}, err
	}
	if size <= 0 || size > s.cfg.MaxUploadBytes {
		// Tamaño fuera de rango: ni firmes (no cabría). 400 por ruta/param inválido.
		return PresignResult{}, domain.ErrInvalidName
	}
	// Resuelve carpeta + nombre desde la ruta (permiso = acceso RLS a la carpeta).
	folderID, name, err := s.resolveParent(ctx, path)
	if err != nil {
		return PresignResult{}, err
	}
	if !domain.ValidName(name) {
		return PresignResult{}, domain.ErrInvalidName
	}
	// Pre-chequeo de cuota (blando): evita firmar subidas que no cabrían.
	if _, err := s.store.EnsureQuota(ctx, s.cfg.DefaultQuotaBytes); err != nil {
		return PresignResult{}, err
	}
	storageKey, err := s.claveDeBlob(ctx, t, folderID, name)
	if err != nil {
		return PresignResult{}, err
	}
	now := time.Now()
	urlExp := now.Add(s.cfg.PresignTTL)
	handleExp := now.Add(2 * s.cfg.PresignTTL)
	// La reserva vive tanto como el handle (2 * PresignTTL): lo pendiente cuenta
	// contra la cuota hasta que se finalice o expire el handle, y el recolector no
	// barre la clave mientras el handle siga siendo válido (FILE-08).
	if err := s.store.ReservarSubida(ctx, storageKey, size, handleExp, !store.EsExterno(storageKey)); err != nil {
		if errors.Is(err, store.ErrDemasiadasSubidas) {
			return PresignResult{}, domain.ErrQuotaExceeded
		}
		return PresignResult{}, err
	}

	if contentType == "" {
		contentType = "application/octet-stream"
	}
	u, err := s.blobs.PresignPut(ctx, t, storageKey, s.cfg.PresignTTL)
	if err != nil {
		return PresignResult{}, fmt.Errorf("service: presign put: %w", err)
	}

	// La URL caduca en PresignTTL (el PUT debe terminar antes). El handle vive el
	// doble: da margen para llamar a finalize tras una subida larga.
	handle, err := s.signHandle(uploadHandle{
		Tenant:      t.Slug,
		FolderID:    folderID.String(),
		Name:        name,
		StorageKey:  storageKey,
		Size:        size,
		ContentType: contentType,
		Owner:       ownerSub,
		Expires:     handleExp.Unix(),
	})
	if err != nil {
		return PresignResult{}, err
	}
	return PresignResult{URL: u.String(), Handle: handle, ExpiresAt: urlExp}, nil
}

// FinalizeUpload confirma una subida directa. Verifica el handle (firma+caducidad),
// que se emitió para ESTE tenant (aislamiento), que el objeto llegó a S3 y su
// tamaño REAL (HEAD, sin confiar en el cliente), y registra el metadato reservando
// cuota atómicamente. Idempotente: si el blob ya está registrado (reintento),
// devuelve el fichero existente sin recrear. Si la subida no llegó o el tamaño no
// casa, limpia el objeto no conforme y devuelve error.
func (s *Service) FinalizeUpload(ctx context.Context, handle string) (domain.File, error) {
	t, err := tenancy.From(ctx)
	if err != nil {
		return domain.File{}, err
	}
	h, err := s.verifyHandle(handle)
	if err != nil {
		return domain.File{}, err
	}
	// Aislamiento por tenant: el handle se firmó para un tenant concreto. Un handle
	// de otro tenant (o el actual manipulado) no finaliza aquí (deny-by-default).
	if h.Tenant != t.Slug {
		return domain.File{}, domain.ErrPermission
	}
	if time.Now().Unix() > h.Expires {
		return domain.File{}, ErrPresignHandleInvalid
	}
	folderID, err := uuid.Parse(h.FolderID)
	if err != nil {
		return domain.File{}, ErrPresignHandleInvalid
	}

	// Idempotencia: si un fichero VIVO ya referencia esta storage_key (reintento de
	// finalize del MISMO handle), devuélvelo sin recrear ni duplicar versión.
	// Con la deduplicación la fila guarda la clave COMPARTIDA y la propia se borra
	// al compartir, así que un reintento cae al HEAD de abajo y recibe «no llegó»,
	// el mismo 404 que un handle caducado: sin duplicar nada.
	// Si no se pudo deduplicar, la fila guarda la copia puesta a salvo (ver abajo).
	firme := h.StorageKey + sufijoFirme
	for _, clave := range []string{h.StorageKey, firme} {
		if existing, err := s.store.GetFileByStorageKey(ctx, clave); err == nil {
			return existing, nil
		} else if !errors.Is(err, domain.ErrNotFound) {
			return domain.File{}, err
		}
	}

	// Verificación server-side y copia puesta a salvo (FILE-04, FILE-08).
	actual, err := s.asegurarSubidaDirecta(ctx, t, h, firme)
	if err != nil {
		return domain.File{}, err
	}

	// Los bytes no pasaron por aquí: el hash se calcula leyendo el objeto una
	// vez con tope h.Size. Es lo que permite deduplicarlo entre organizaciones y,
	// de paso, que una subida directa lleve checksum como las demás sin que un
	// flujo mayor que h.Size (p. ej. metadato gzip forjado) se cuele. Si no se
	// puede leer por otra causa, se registra sin hash y sin compartir.
	storageKey, checksum := firme, ""
	if hash, err := s.hashDe(ctx, t, firme, h.Size); err == nil {
		checksum = hash
		storageKey = s.compartirBlob(ctx, t, firme, hash)
	} else if errors.Is(err, ErrUploadSizeMismatch) {
		_ = s.blobs.Delete(ctx, t, firme)
		return domain.File{}, ErrUploadSizeMismatch
	}
	f, err := s.store.CreateFile(ctx, store.NewFileRecord{
		FolderID:    folderID,
		Name:        h.Name,
		StorageKey:  storageKey,
		SizeBytes:   actual,
		ContentType: h.ContentType,
		Checksum:    checksum,
		OwnerSub:    h.Owner,
	})
	if err != nil {
		s.soltarBlob(ctx, t, storageKey) // compensa: cuota/conflicto → suelta el blob
		return domain.File{}, err
	}
	s.publish(ctx, "drive.file.created", map[string]any{"file_id": f.ID, "name": f.Name, "size": f.SizeBytes})
	s.avisarCuota(ctx, h.Owner, f.SizeBytes)
	return f, nil
}

// asegurarSubidaDirecta comprueba el objeto subido, consume la reserva de un solo
// uso, lo copia a su clave `.firme` fuera del alcance de la URL prefirmada y vuelve
// a medir la copia (FILE-04, FILE-08).
func (s *Service) asegurarSubidaDirecta(ctx context.Context, t tenancy.Tenant, h uploadHandle, firme string) (int64, error) {
	actual, err := s.blobs.Stat(ctx, t, h.StorageKey)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return 0, ErrUploadNotFound
		}
		// Un fallo pasajero del almacén no es motivo para tirar lo subido: el
		// cliente puede reintentar con el mismo handle mientras valga.
		return 0, err
	}
	if actual != h.Size || actual > s.cfg.MaxUploadBytes {
		_ = s.blobs.Delete(ctx, t, h.StorageKey) // limpia el objeto no conforme
		s.cerrarReserva(ctx, h.StorageKey)
		return 0, ErrUploadSizeMismatch
	}
	if err := s.store.SubidaFinalizada(ctx, h.StorageKey); err != nil {
		if errors.Is(err, store.ErrSubidaYaFinalizada) {
			_ = s.blobs.Delete(ctx, t, h.StorageKey)
			return 0, ErrPresignHandleInvalid
		}
		log.From(ctx).Warn("drive: marcar la subida directa como finalizada", "key", h.StorageKey, "error", err)
	}
	if err := s.blobs.Copiar(ctx, t, h.StorageKey, firme); err != nil {
		return 0, fmt.Errorf("service: poner a salvo la subida directa: %w", err)
	}
	if err := s.blobs.Delete(ctx, t, h.StorageKey); err != nil {
		log.From(ctx).Warn("drive: borrar la subida directa tras ponerla a salvo", "key", h.StorageKey, "error", err)
	}
	actual, err = s.blobs.Stat(ctx, t, firme)
	if err != nil {
		_ = s.blobs.Delete(ctx, t, firme)
		return 0, fmt.Errorf("service: medir la subida puesta a salvo: %w", err)
	}
	if actual != h.Size || actual > s.cfg.MaxUploadBytes {
		_ = s.blobs.Delete(ctx, t, firme)
		return 0, ErrUploadSizeMismatch
	}
	return actual, nil
}

// cerrarReserva saca la subida de lo pendiente (FILE-08). Si falla, solo queda
// contando como pendiente hasta que caduque la URL.
func (s *Service) cerrarReserva(ctx context.Context, clave string) {
	if err := s.store.SubidaFinalizada(ctx, clave); err != nil && !errors.Is(err, store.ErrSubidaYaFinalizada) {
		log.From(ctx).Warn("drive: marcar la subida directa como finalizada", "key", clave, "error", err)
	}
}
