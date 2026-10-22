// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package compat

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/drive/internal/domain"
)

// GoogleFile es el recurso `files` de la API de Google Drive v3, con el
// subconjunto de campos que las librerías cliente leen de verdad. `size` viaja
// como cadena (int64 en JSON string) igual que en Google.
type GoogleFile struct {
	Kind         string   `json:"kind"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	MimeType     string   `json:"mimeType"`
	Size         string   `json:"size,omitempty"`
	CreatedTime  string   `json:"createdTime,omitempty"`
	ModifiedTime string   `json:"modifiedTime,omitempty"`
	Parents      []string `json:"parents,omitempty"`
	Trashed      bool     `json:"trashed"`
}

// GoogleFileList es la respuesta de files.list.
type GoogleFileList struct {
	Kind             string       `json:"kind"`
	IncompleteSearch bool         `json:"incompleteSearch"`
	NextPageToken    string       `json:"nextPageToken,omitempty"`
	Files            []GoogleFile `json:"files"`
}

// GoogleUser es el usuario autenticado tal y como lo describe about.get.
type GoogleUser struct {
	Kind         string `json:"kind"`
	DisplayName  string `json:"displayName,omitempty"`
	EmailAddress string `json:"emailAddress,omitempty"`
	PermissionID string `json:"permissionId,omitempty"`
	Me           bool   `json:"me"`
}

// GoogleStorageQuota es la cuota de almacenamiento (bytes como cadena).
type GoogleStorageQuota struct {
	Limit        string `json:"limit,omitempty"`
	Usage        string `json:"usage,omitempty"`
	UsageInDrive string `json:"usageInDrive,omitempty"`
}

// GoogleAbout es la respuesta de about.get.
type GoogleAbout struct {
	Kind         string              `json:"kind"`
	User         GoogleUser          `json:"user"`
	StorageQuota *GoogleStorageQuota `json:"storageQuota,omitempty"`
}

// GoogleFromFile traduce un fichero de Drive al recurso de Google.
func GoogleFromFile(f domain.File) GoogleFile {
	return GoogleFile{
		Kind:         "drive#file",
		ID:           f.ID.String(),
		Name:         f.Name,
		MimeType:     mimeOr(f.ContentType),
		Size:         strconv.FormatInt(f.SizeBytes, 10),
		CreatedTime:  rfc3339(f.CreatedAt),
		ModifiedTime: rfc3339(f.UpdatedAt),
		Parents:      []string{f.FolderID.String()},
	}
}

// GoogleFromFolder traduce una carpeta. El mimeType de carpeta es el contrato
// que usan las librerías para distinguirla de un fichero.
func GoogleFromFolder(d domain.Folder) GoogleFile {
	g := GoogleFile{
		Kind:         "drive#file",
		ID:           d.ID.String(),
		Name:         d.Name,
		MimeType:     FolderMIME,
		CreatedTime:  rfc3339(d.CreatedAt),
		ModifiedTime: rfc3339(d.UpdatedAt),
	}
	if d.ParentID != nil {
		g.Parents = []string{d.ParentID.String()}
	}
	return g
}

// GoogleNode traduce una entrada de un listado transversal (p. ej. «Recientes»),
// que no trae carpeta padre: `parents` se OMITE en vez de inventarlo.
func GoogleNode(id uuid.UUID, name string, isDir bool, size int64, contentType string, modified time.Time) GoogleFile {
	g := GoogleFile{
		Kind:         "drive#file",
		ID:           id.String(),
		Name:         name,
		MimeType:     FolderMIME,
		ModifiedTime: rfc3339(modified),
	}
	if !isDir {
		g.MimeType = mimeOr(contentType)
		g.Size = strconv.FormatInt(size, 10)
	}
	return g
}

// NewGoogleFileList arma la respuesta de listado (files nunca nulo: las
// librerías iteran el array sin comprobarlo).
func NewGoogleFileList(files []GoogleFile, nextPageToken string) GoogleFileList {
	if files == nil {
		files = []GoogleFile{}
	}
	return GoogleFileList{Kind: "drive#fileList", Files: files, NextPageToken: nextPageToken}
}

// NewGoogleAbout arma about.get. quota nil ⇒ el campo storageQuota se OMITE (no
// se inventa una cuota que el servicio no conoce).
func NewGoogleAbout(subject, displayName, email string, quota *domain.Quota) GoogleAbout {
	a := GoogleAbout{
		Kind: "drive#about",
		User: GoogleUser{
			Kind:         "drive#user",
			DisplayName:  displayName,
			EmailAddress: email,
			PermissionID: subject,
			Me:           true,
		},
	}
	if quota != nil && quota.LimitBytes > 0 {
		// `usage` es el total del plan (Drive más lo que declaran las otras apps de
		// la suite, mig 0026); `usageInDrive`, solo lo de Drive: es exactamente la
		// distinción que hace Google entre la cuota de la cuenta y la de Drive.
		a.StorageQuota = &GoogleStorageQuota{
			Limit:        strconv.FormatInt(quota.LimitBytes, 10),
			Usage:        strconv.FormatInt(quota.UsedBytes, 10),
			UsageInDrive: strconv.FormatInt(quota.DriveBytes(), 10),
		}
	}
	return a
}

// ---------------------------------------------------------------------------
// Errores en el dialecto de Google
// ---------------------------------------------------------------------------

// GoogleError es el sobre de error de las APIs de Google.
type GoogleError struct {
	Error GoogleErrorBody `json:"error"`
}

// GoogleErrorBody es el cuerpo del sobre de error.
type GoogleErrorBody struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Errors  []GoogleErrorItem `json:"errors"`
}

// GoogleErrorItem es el detalle por error que leen los clientes para decidir
// reintentos (reason).
type GoogleErrorItem struct {
	Domain  string `json:"domain"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// NewGoogleError traduce un estado HTTP + mensaje al sobre de Google.
func NewGoogleError(status int, message string) GoogleError {
	return GoogleError{Error: GoogleErrorBody{
		Code:    status,
		Message: message,
		Errors: []GoogleErrorItem{{
			Domain:  "global",
			Reason:  googleReason(status),
			Message: message,
		}},
	}}
}

// googleReason mapea el estado HTTP al `reason` que usan las librerías de Google.
func googleReason(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "badRequest"
	case http.StatusUnauthorized:
		return "authError"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "notFound"
	case http.StatusConflict:
		return "duplicate"
	case http.StatusRequestEntityTooLarge:
		return "uploadTooLarge"
	case http.StatusUnsupportedMediaType:
		return "badContent"
	case http.StatusTooManyRequests:
		return "rateLimitExceeded"
	case http.StatusNotImplemented:
		return "notImplemented"
	case http.StatusInsufficientStorage:
		return "storageQuotaExceeded"
	default:
		return "backendError"
	}
}

// ---------------------------------------------------------------------------
// Parámetro `q` de files.list
// ---------------------------------------------------------------------------

// ErrUnsupportedQuery indica un filtro de `q` fuera del subconjunto soportado.
// El handler lo traduce a 400 con el término exacto: nunca se ignora en silencio
// (devolver resultados sin aplicar el filtro sería peor que fallar).
var ErrUnsupportedQuery = errors.New("compat: filtro no soportado en q")

// Query es el subconjunto de `q` que soporta la emulación:
//
//	'<id>' in parents      → hijos de esa carpeta ('root' = raíz del tenant)
//	name = '<nombre>'      → nombre exacto (sin distinguir mayúsculas, igual que
//	                         la unicidad de nombres nativa)
//	mimeType = '<tipo>'    → tipo exacto ('…apps.folder' = carpetas)
//	mimeType != '<tipo>'   → tipo distinto
//	trashed = true|false   → papelera
//
// … combinados con `and`. Cualquier otro término (contains, owners, or,
// paréntesis, …) es ErrUnsupportedQuery.
type Query struct {
	ParentID   string
	Name       string
	MimeType   string
	MimeNot    bool
	Trashed    bool
	TrashedSet bool
}

// ParseQuery interpreta el parámetro `q`. Vacío ⇒ sin filtros.
func ParseQuery(q string) (Query, error) {
	var out Query
	if strings.TrimSpace(q) == "" {
		return out, nil
	}
	for _, term := range splitTerms(q) {
		if err := parseTerm(term, &out); err != nil {
			return Query{}, err
		}
	}
	return out, nil
}

// FoldersOnly indica que el filtro solo puede satisfacerlo una carpeta (permite
// ahorrarse el listado de ficheros).
func (q Query) FoldersOnly() bool { return q.MimeType == FolderMIME && !q.MimeNot }

// FilesOnly indica que ninguna carpeta puede satisfacer el filtro.
func (q Query) FilesOnly() bool {
	if q.MimeType == "" {
		return false
	}
	if q.MimeNot {
		return q.MimeType == FolderMIME
	}
	return q.MimeType != FolderMIME
}

// Matches aplica los filtros de nombre y tipo a un elemento ya traducido. El
// filtro de padre y el de papelera los resuelve el handler eligiendo la consulta
// (no se pueden aplicar sobre el resultado).
func (q Query) Matches(f GoogleFile) bool {
	if q.Name != "" && !strings.EqualFold(f.Name, q.Name) {
		return false
	}
	if q.MimeType != "" && (f.MimeType == q.MimeType) == q.MimeNot {
		return false
	}
	return true
}

// splitTerms parte la expresión por el operador `and` de nivel superior,
// respetando las cadenas entre comillas simples (con \' como escape).
func splitTerms(q string) []string {
	var terms []string
	start, inQuote := 0, false
	for i := 0; i < len(q); i++ {
		switch q[i] {
		case '\\':
			if inQuote {
				i++ // salta el carácter escapado
			}
		case '\'':
			inQuote = !inQuote
		case 'a', 'A':
			if inQuote || !isWordAt(q, i, "and") {
				continue
			}
			terms = append(terms, strings.TrimSpace(q[start:i]))
			start = i + len("and")
			i += len("and") - 1
		}
	}
	return append(terms, strings.TrimSpace(q[start:]))
}

// parseTerm interpreta un término suelto y lo acumula en out.
func parseTerm(term string, out *Query) error {
	if i := wordIndex(term, "in"); i >= 0 {
		field := strings.TrimSpace(term[i+len("in"):])
		if field != "parents" {
			return unsupported(term)
		}
		id, err := unquote(strings.TrimSpace(term[:i]))
		if err != nil {
			return unsupported(term)
		}
		out.ParentID = id
		return nil
	}

	field, op, value, ok := splitOperator(term)
	if !ok {
		return unsupported(term)
	}
	switch {
	case strings.EqualFold(field, "name"):
		if op != "=" {
			return unsupported(term)
		}
		v, err := unquote(value)
		if err != nil {
			return unsupported(term)
		}
		out.Name = v
	case strings.EqualFold(field, "mimeType"):
		v, err := unquote(value)
		if err != nil {
			return unsupported(term)
		}
		out.MimeType, out.MimeNot = v, op == "!="
	case strings.EqualFold(field, "trashed"):
		if op != "=" {
			return unsupported(term)
		}
		b, err := strconv.ParseBool(value)
		if err != nil {
			return unsupported(term)
		}
		out.Trashed, out.TrashedSet = b, true
	default:
		return unsupported(term)
	}
	return nil
}

// splitOperator parte `campo op valor` por el primer `=` o `!=` fuera de comillas.
func splitOperator(term string) (field, op, value string, ok bool) {
	for i := 0; i < len(term); i++ {
		switch term[i] {
		case '\\':
			i++
		case '\'':
			// Una comilla antes del operador significa que el término no tiene la
			// forma `campo op valor` (p. ej. `'x' in owners`): no es soportado.
			return "", "", "", false
		case '!':
			if i+1 < len(term) && term[i+1] == '=' {
				return strings.TrimSpace(term[:i]), "!=", strings.TrimSpace(term[i+2:]), true
			}
		case '=':
			return strings.TrimSpace(term[:i]), "=", strings.TrimSpace(term[i+1:]), true
		}
	}
	return "", "", "", false
}

// unquote quita las comillas simples de un literal de `q` y deshace \' y \\.
func unquote(s string) (string, error) {
	if len(s) < 2 || s[0] != '\'' || s[len(s)-1] != '\'' {
		return "", ErrUnsupportedQuery
	}
	var b strings.Builder
	body := s[1 : len(s)-1]
	for i := 0; i < len(body); i++ {
		if body[i] == '\\' && i+1 < len(body) {
			i++
			b.WriteByte(body[i])
			continue
		}
		if body[i] == '\'' { // comilla sin escapar dentro del literal
			return "", ErrUnsupportedQuery
		}
		b.WriteByte(body[i])
	}
	return b.String(), nil
}

// wordIndex localiza la palabra completa w fuera de comillas (-1 si no está).
func wordIndex(s, w string) int {
	inQuote := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if inQuote {
				i++
			}
		case '\'':
			inQuote = !inQuote
		default:
			if !inQuote && isWordAt(s, i, w) {
				return i
			}
		}
	}
	return -1
}

// isWordAt indica si en s[i:] empieza la palabra w delimitada por espacios.
func isWordAt(s string, i int, w string) bool {
	if i+len(w) > len(s) || !strings.EqualFold(s[i:i+len(w)], w) {
		return false
	}
	if i > 0 && !isSpace(s[i-1]) {
		return false
	}
	return i+len(w) < len(s) && isSpace(s[i+len(w)])
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// unsupported construye el error con el término exacto que el cliente debe quitar.
func unsupported(term string) error {
	return fmt.Errorf("%w: %q (soportados: '<id>' in parents, name = '…', mimeType = '…' o != '…', trashed = true|false, unidos por `and`)",
		ErrUnsupportedQuery, strings.TrimSpace(term))
}
