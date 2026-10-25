// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// DriveItem es el `driveItem` de Microsoft Graph con el subconjunto de facetas
// que consumen las librerías: la presencia de `folder` o `file` es lo que
// distingue carpeta de fichero.
type DriveItem struct {
	ID                   string           `json:"id"`
	Name                 string           `json:"name"`
	Size                 int64            `json:"size"`
	CreatedDateTime      string           `json:"createdDateTime,omitempty"`
	LastModifiedDateTime string           `json:"lastModifiedDateTime,omitempty"`
	Folder               *FolderFacet     `json:"folder,omitempty"`
	File                 *FileFacet       `json:"file,omitempty"`
	ParentReference      *ParentReference `json:"parentReference,omitempty"`
}

// FolderFacet marca un elemento como carpeta. ChildCount es un puntero porque el
// listado no siempre conoce el número de hijos: se omite antes que inventarlo.
type FolderFacet struct {
	ChildCount *int `json:"childCount,omitempty"`
}

// FileFacet marca un elemento como fichero.
type FileFacet struct {
	MimeType string  `json:"mimeType,omitempty"`
	Hashes   *Hashes `json:"hashes,omitempty"`
}

// Hashes lleva el checksum del contenido (Sytue calcula SHA-256).
type Hashes struct {
	Sha256Hash string `json:"sha256Hash,omitempty"`
}

// ParentReference referencia la carpeta contenedora.
type ParentReference struct {
	DriveID string `json:"driveId,omitempty"`
	ID      string `json:"id,omitempty"`
}

// DriveItemCollection es una colección OData de driveItem.
type DriveItemCollection struct {
	Value    []DriveItem `json:"value"`
	NextLink string      `json:"@odata.nextLink,omitempty"`
}

// Identity y IdentitySet describen al propietario del drive.
type Identity struct {
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// IdentitySet agrupa las identidades de una relación (aquí, el propietario).
type IdentitySet struct {
	User *Identity `json:"user,omitempty"`
}

// DriveQuota es la cuota del drive en el dialecto de Graph (bytes, no cadenas).
type DriveQuota struct {
	Total     int64  `json:"total"`
	Used      int64  `json:"used"`
	Remaining int64  `json:"remaining"`
	State     string `json:"state,omitempty"`
}

// Drive es el recurso `drive` de Graph.
type Drive struct {
	ID        string       `json:"id"`
	DriveType string       `json:"driveType"`
	Name      string       `json:"name"`
	Owner     *IdentitySet `json:"owner,omitempty"`
	Quota     *DriveQuota  `json:"quota,omitempty"`
	Root      *DriveItem   `json:"root,omitempty"`
}

// GraphFromFile traduce un fichero de Drive a driveItem.
func GraphFromFile(f domain.File) DriveItem {
	item := DriveItem{
		ID:                   f.ID.String(),
		Name:                 f.Name,
		Size:                 f.SizeBytes,
		CreatedDateTime:      rfc3339(f.CreatedAt),
		LastModifiedDateTime: rfc3339(f.UpdatedAt),
		File:                 &FileFacet{MimeType: mimeOr(f.ContentType)},
		ParentReference:      &ParentReference{ID: f.FolderID.String()},
	}
	if f.Checksum != "" {
		item.File.Hashes = &Hashes{Sha256Hash: f.Checksum}
	}
	return item
}

// GraphFromFolder traduce una carpeta a driveItem. childCount nil ⇒ se omite.
func GraphFromFolder(d domain.Folder, childCount *int) DriveItem {
	item := DriveItem{
		ID:                   d.ID.String(),
		Name:                 d.Name,
		CreatedDateTime:      rfc3339(d.CreatedAt),
		LastModifiedDateTime: rfc3339(d.UpdatedAt),
		Folder:               &FolderFacet{ChildCount: childCount},
	}
	if d.ParentID != nil {
		item.ParentReference = &ParentReference{ID: d.ParentID.String()}
	}
	return item
}

// NewDriveItemCollection arma la colección (value nunca nulo).
func NewDriveItemCollection(items []DriveItem, nextLink string) DriveItemCollection {
	if items == nil {
		items = []DriveItem{}
	}
	return DriveItemCollection{Value: items, NextLink: nextLink}
}

// NewGraphDrive arma el recurso drive: raíz + propietario + cuota (nil ⇒ se
// omite la cuota, no se inventa).
func NewGraphDrive(root domain.Folder, subject, displayName string, quota *domain.Quota) Drive {
	d := Drive{
		ID:        root.ID.String(),
		DriveType: "business",
		Name:      "Sytue Drive",
		Owner:     &IdentitySet{User: &Identity{ID: subject, DisplayName: displayName}},
	}
	item := GraphFromFolder(root, nil)
	d.Root = &item
	if quota != nil && quota.LimitBytes > 0 {
		d.Quota = &DriveQuota{
			Total:     quota.LimitBytes,
			Used:      quota.UsedBytes,
			Remaining: quota.Available(),
			State:     quotaState(*quota),
		}
	}
	return d
}

// quotaState traduce el uso al vocabulario de Graph (normal|nearing|critical|exceeded).
func quotaState(q domain.Quota) string {
	remaining := q.Available()
	switch {
	case remaining == 0:
		return "exceeded"
	case remaining <= q.LimitBytes/10:
		return "critical"
	case remaining <= q.LimitBytes/5:
		return "nearing"
	default:
		return "normal"
	}
}

// NextLink construye el `@odata.nextLink` ABSOLUTO que Graph exige: el cliente lo
// vuelve a pedir tal cual. skipToken vacío ⇒ "" (no hay más páginas).
func NextLink(baseURL, path string, top int, skipToken string) string {
	if skipToken == "" {
		return ""
	}
	v := url.Values{}
	if top > 0 {
		v.Set("$top", strconv.Itoa(top))
	}
	v.Set("$skiptoken", skipToken)
	return strings.TrimRight(baseURL, "/") + path + "?" + v.Encode()
}

// ---------------------------------------------------------------------------
// Errores en el dialecto de Graph
// ---------------------------------------------------------------------------

// GraphError es el sobre de error de Microsoft Graph.
type GraphError struct {
	Error GraphErrorBody `json:"error"`
}

// GraphErrorBody es el cuerpo del sobre: `code` es un símbolo, no un número.
type GraphErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewGraphError traduce un estado HTTP + mensaje al sobre de Graph.
func NewGraphError(status int, message string) GraphError {
	return GraphError{Error: GraphErrorBody{Code: graphCode(status), Message: message}}
}

// graphCode mapea el estado HTTP al código simbólico de Graph.
func graphCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalidRequest"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "accessDenied"
	case http.StatusNotFound:
		return "itemNotFound"
	case http.StatusConflict:
		return "nameAlreadyExists"
	case http.StatusRequestEntityTooLarge:
		return "entityTooLarge"
	case http.StatusUnsupportedMediaType:
		return "invalidRequest"
	case http.StatusTooManyRequests:
		return "activityLimitReached"
	case http.StatusNotImplemented:
		return "notImplemented"
	case http.StatusInsufficientStorage:
		return "quotaLimitReached"
	default:
		return "generalException"
	}
}
