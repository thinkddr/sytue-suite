// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package domain define los tipos y reglas de negocio de Sytue Drive,
// independientes de transporte (HTTP/WebDAV) y de almacenamiento (PG/S3).
//
// Clean-room: modelo propio sobre el estándar WebDAV (RFC 4918) y semántica de
// sistemas de archivos; sin código de terceros AGPL.
package domain

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Errores de dominio. La capa HTTP los traduce a *httpx.Error.
var (
	ErrNotFound      = errors.New("drive: recurso no encontrado")
	ErrConflict      = errors.New("drive: conflicto de nombres")
	ErrInvalidName   = errors.New("drive: nombre inválido")
	ErrQuotaExceeded = errors.New("drive: cuota de almacenamiento superada")
	ErrInvalidParent = errors.New("drive: carpeta destino inválida")
	ErrShareExpired  = errors.New("drive: enlace de compartición expirado o revocado")
	ErrPermission    = errors.New("drive: permiso insuficiente")
	// ErrPasswordRequired indica que el enlace está protegido y falta (o no
	// coincide) la prueba de contraseña: la ruta pública debe mostrar el prompt.
	ErrPasswordRequired = errors.New("drive: el enlace requiere contraseña")
	// ErrWrongPassword indica que la contraseña enviada al enlace no coincide.
	ErrWrongPassword = errors.New("drive: contraseña incorrecta")
	ErrCycle         = errors.New("drive: mover una carpeta dentro de sí misma")
	ErrRootImmutable = errors.New("drive: la carpeta raíz no se puede modificar")
	// ErrBatchTooLarge indica que una operación en lote (o el árbol de un ZIP)
	// supera el límite razonable de elementos; el llamador debe acotar la selección.
	ErrBatchTooLarge = errors.New("drive: selección demasiado grande")
	// ErrLabelBlocksSharing indica que la ETIQUETA DE SENSIBILIDAD del fichero
	// prohíbe compartirlo fuera de la organización: no se puede crear un enlace
	// público para él. La capa HTTP lo traduce a 403 con el motivo, para que la UI
	// pueda decir POR QUÉ (y con qué etiqueta) en vez de un «no puedes» seco.
	ErrLabelBlocksSharing = errors.New("drive: la etiqueta de sensibilidad impide compartir fuera de la organización")
	// ErrUnsupportedMedia indica que no se puede generar una miniatura del fichero:
	// no es un formato ráster soportado (JPEG/PNG/GIF), está corrupto, o sus
	// dimensiones/bytes exceden el límite (guarda anti bomba de descompresión). La
	// capa HTTP lo traduce a 415.
	ErrUnsupportedMedia = errors.New("drive: tipo de medio no soportado")
)

// Folder es una carpeta del árbol de un tenant.
type Folder struct {
	ID        uuid.UUID  `json:"id"`
	Tenant    string     `json:"-"`
	ParentID  *uuid.UUID `json:"parent_id,omitempty"`
	Name      string     `json:"name"`
	IsRoot    bool       `json:"is_root"`
	IsGroup   bool       `json:"is_group"`
	OwnerSub  string     `json:"owner_sub,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// DeletedAt es cuándo se envió a la papelera (nil = viva). Solo viaja en los
	// listados de papelera: con la retención del servicio permite decir cuánto le
	// queda antes de la auto-purga.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// File es el metadato de un fichero; el contenido vive en S3 (StorageKey).
type File struct {
	ID          uuid.UUID `json:"id"`
	Tenant      string    `json:"-"`
	FolderID    uuid.UUID `json:"folder_id"`
	Name        string    `json:"name"`
	StorageKey  string    `json:"-"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	Checksum    string    `json:"checksum,omitempty"`
	OwnerSub    string    `json:"owner_sub,omitempty"`
	// ModifiedBy es el sub de quien subió el contenido ACTUAL (el creador original
	// vive en OwnerSub). Se propaga como autor al archivar una versión.
	ModifiedBy string    `json:"modified_by,omitempty"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Label es el SNAPSHOT de la etiqueta de sensibilidad del fichero (nil = sin
	// etiquetar). No es una FK a Sytue ID: se copió al etiquetar y sobrevive a los
	// cambios de la taxonomía (ver mig 0013).
	Label *Label `json:"label,omitempty"`
	// DeletedAt es cuándo se envió a la papelera (nil = vivo). Ver Folder.DeletedAt.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}

// Label es la etiqueta de sensibilidad tal y como estaba EN EL MOMENTO de etiquetar
// el fichero (paridad Purview / Google Drive labels). La taxonomía viva es de Sytue
// ID; esto es la copia que Drive usa para decidir sin salir a la red.
type Label struct {
	// ID es el uuid de la etiqueta en Sytue ID (referencia informativa, no FK).
	ID   string `json:"id"`
	Name string `json:"name"`
	// Rank es el nivel de sensibilidad relativo (mayor = más sensible).
	Rank int `json:"rank"`
	// BlockExternalShare: mientras el fichero lleve esta etiqueta NO admite enlaces
	// públicos. Viaja en el snapshot a propósito (fail-closed: aflojar la taxonomía
	// no afloja sola la restricción de lo ya etiquetado).
	BlockExternalShare bool `json:"block_external_share"`
}

// FileVersion es una versión archivada del contenido de un fichero: referencia el
// blob de esa versión (StorageKey, nunca serializado) más los metadatos para
// listarla/descargarla. IsCurrent marca la versión viva (la que sirve el fichero
// actual, sintetizada desde la fila de files; las archivadas son IsCurrent=false).
type FileVersion struct {
	FileID      uuid.UUID `json:"file_id"`
	Tenant      string    `json:"-"`
	Version     int       `json:"version"`
	StorageKey  string    `json:"-"`
	SizeBytes   int64     `json:"size_bytes"`
	ContentType string    `json:"content_type"`
	Checksum    string    `json:"checksum,omitempty"`
	AuthorSub   string    `json:"author_sub,omitempty"`
	IsCurrent   bool      `json:"is_current"`
	CreatedAt   time.Time `json:"created_at"`
}

// ShareKind es el tipo de compartición.
type ShareKind string

const (
	ShareLink  ShareKind = "link"
	ShareUser  ShareKind = "user"
	ShareGroup ShareKind = "group"
)

// Permission es el nivel de acceso concedido por un share.
type Permission string

const (
	PermRead  Permission = "read"
	PermWrite Permission = "write"
)

// Share es una concesión de acceso a un fichero o carpeta.
type Share struct {
	ID         uuid.UUID  `json:"id"`
	Tenant     string     `json:"-"`
	FileID     *uuid.UUID `json:"file_id,omitempty"`
	FolderID   *uuid.UUID `json:"folder_id,omitempty"`
	Kind       ShareKind  `json:"kind"`
	Subject    string     `json:"subject,omitempty"`
	Permission Permission `json:"permission"`
	// Token solo se devuelve al crear un enlace; nunca se lista de vuelta.
	Token string `json:"token,omitempty"`
	// HasPassword indica si el enlace está protegido por contraseña. El hash NUNCA
	// se serializa; solo este booleano viaja al cliente (para pintar el candado).
	HasPassword bool       `json:"has_password,omitempty"`
	CreatedBy   string     `json:"created_by,omitempty"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Quota describe el uso/límite de almacenamiento de un tenant.
//
// La cuota del plan es UNA para toda la suite y Drive es su dueño: UsedBytes es
// lo que ocupa Drive MÁS lo que las otras apps han declarado (ExternalUsage, mig
// 0026). Quien compara con LimitBytes —Available, HasRoomFor, la reserva del
// store, el pre-chequeo del presign, el aviso del 90 %— ya cuenta lo externo sin
// saber que existe.
type Quota struct {
	Tenant     string `json:"tenant"`
	LimitBytes int64  `json:"limit_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
	// ExternalUsage es lo que ocupa cada otra app de la suite en esta cuota, por
	// clave de app («cast»). UsedBytes YA lo incluye: es un desglose, no un sumando
	// pendiente. Vacío si ninguna ha declarado nada.
	ExternalUsage map[string]int64 `json:"external_usage,omitempty"`
	UpdatedAt     time.Time        `json:"updated_at"`
	// La ampliación es una vista de lectura: nunca modifica la cuota facturada
	// ni las reservas del almacén de Sytue en PostgreSQL.
	BaseLimitBytes         int64  `json:"base_limit_bytes"`
	BaseUsedBytes          int64  `json:"base_used_bytes"`
	ConnectedStorage       bool   `json:"-"`
	StorageTotalBytes      *int64 `json:"-"`
	StorageUsedBytes       *int64 `json:"-"`
	StorageAvailableBytes  *int64 `json:"-"`
	StorageCapacityPartial bool   `json:"-"`
	capacityCombined       bool
}

// WithStorageExtension suma la capacidad del almacén conectado sin convertir
// un dato ausente en cero. Se conserva el libre que informa el proveedor: una
// cuenta de equipo puede tener un límite individual menor que total-usado.
func (q Quota) WithStorageExtension(total, used, available *int64) Quota {
	if q.ConnectedStorage {
		q.LimitBytes, q.UsedBytes = q.BaseLimitBytes, q.BaseUsedBytes
	}
	q.BaseLimitBytes, q.BaseUsedBytes = q.LimitBytes, q.UsedBytes
	q.ConnectedStorage, q.capacityCombined = true, false
	q.StorageCapacityPartial = false
	q.StorageTotalBytes, q.StorageUsedBytes, q.StorageAvailableBytes = total, used, available
	for _, n := range []*int64{total, used, available} {
		if n != nil && *n < 0 {
			q.StorageTotalBytes, q.StorageUsedBytes, q.StorageAvailableBytes = nil, nil, nil
			q.StorageCapacityPartial = true
			return q
		}
	}
	if total != nil && available != nil && *available > *total {
		q.StorageTotalBytes, q.StorageUsedBytes, q.StorageAvailableBytes = nil, nil, nil
		q.StorageCapacityPartial = true
		return q
	}
	if total == nil || used == nil || available == nil {
		q.StorageCapacityPartial = true
		return q
	}
	if q.LimitBytes < 0 || q.UsedBytes < 0 ||
		*total > math.MaxInt64-q.LimitBytes || *used > math.MaxInt64-q.UsedBytes {
		q.StorageTotalBytes, q.StorageUsedBytes, q.StorageAvailableBytes = nil, nil, nil
		q.StorageCapacityPartial = true
		return q
	}
	q.LimitBytes += *total
	q.UsedBytes += *used
	q.capacityCombined = true
	return q
}

// Available devuelve los bytes libres (nunca negativo).
func (q Quota) Available() int64 {
	if q.capacityCombined {
		base := q.BaseLimitBytes - q.BaseUsedBytes
		if base < 0 {
			base = 0
		}
		return base + *q.StorageAvailableBytes
	}
	if q.UsedBytes >= q.LimitBytes {
		return 0
	}
	return q.LimitBytes - q.UsedBytes
}

// HasRoomFor indica si caben n bytes adicionales.
func (q Quota) HasRoomFor(n int64) bool { return n >= 0 && n <= q.Available() }

// ExternalBytes es la suma de lo declarado por las otras apps.
func (q Quota) ExternalBytes() int64 {
	var n int64
	for _, b := range q.ExternalUsage {
		n += b
	}
	return n
}

// DriveBytes es lo que ocupa Drive solo: UsedBytes sin lo externo. Es el
// `usageInDrive` de la API de Google y el desglose del medidor.
func (q Quota) DriveBytes() int64 {
	used := q.UsedBytes
	if q.ConnectedStorage {
		// El uso total del proveedor puede incluir datos ajenos a Drive.
		used = q.BaseUsedBytes
	}
	if n := used - q.ExternalBytes(); n > 0 {
		return n
	}
	return 0
}

// ValidName valida un nombre de fichero/carpeta: no vacío, sin separadores de
// ruta ni control, longitud razonable. (RFC 4918 §5.2 trata segmentos de ruta.)
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 255 {
		return false
	}
	if name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return false
	}
	for _, r := range name {
		if r < 0x20 { // caracteres de control
			return false
		}
	}
	return true
}

// ValidPermission valida un valor de permiso.
func ValidPermission(p Permission) bool { return p == PermRead || p == PermWrite }

// ValidShareKind valida un tipo de share.
func ValidShareKind(k ShareKind) bool {
	return k == ShareLink || k == ShareUser || k == ShareGroup
}
