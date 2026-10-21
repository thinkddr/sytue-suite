// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

const (
	// Scopes de ESCRITURA para el camino de salida. drive.file es el mínimo que
	// permite crear: da acceso SOLO a lo que crea esta aplicación, no a todo el
	// Drive de la persona. Pedir `drive` a secas sería pedir permiso para leer y
	// borrar treinta años de ficheros ajenos para poder escribir uno.
	//nolint:gosec // G101: identificadores de scope OAuth públicos, no credenciales.
	scopeDriveWrite = "https://www.googleapis.com/auth/drive.file"
	//nolint:gosec // G101: idem.
	scopeContactsWrite = "https://www.googleapis.com/auth/contacts"
	// googleUploadPath es el endpoint de subida con metadatos (multipart).
	googleUploadPath = "/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true"
	// googleFolderMime es el tipo con el que Google representa una carpeta.
	googleFolderMime = "application/vnd.google-apps.folder"
)

// googleDest escribe en Google Workspace: es el destino del camino de SALIDA.
//
// El egress sale por Runtime.HTTP como el resto del tráfico a terceros —misma
// barrera SSRF, mismo límite de ritmo, misma auditoría—: que la migración vaya
// «hacia fuera» no la exime de la política de egreso, la hace más importante.
type googleDest struct {
	rt         connector.Runtime
	driveBase  string
	peopleBase string
	gmailBase  string
	calBase    string
	token      string
}

// NewGoogleDestination construye el destino de salida hacia Google.
func NewGoogleDestination(rt connector.Runtime, inst connector.Instance, token string) (Destination, error) {
	driveBase, err := baseURLOr(inst.Config[ConfigGoogleDriveBase], defaultGoogleDriveBase)
	if err != nil {
		return nil, err
	}
	peopleBase, err := baseURLOr(inst.Config[ConfigGooglePeopleBase], defaultGooglePeopleBase)
	if err != nil {
		return nil, err
	}
	gmailBase, err := baseURLOr(inst.Config[ConfigGmailBase], defaultGmailBase)
	if err != nil {
		return nil, err
	}
	calBase, err := baseURLOr(inst.Config[ConfigGCalBase], defaultGCalBase)
	if err != nil {
		return nil, err
	}
	return &googleDest{
		rt: rt, driveBase: driveBase, peopleBase: peopleBase, gmailBase: gmailBase, calBase: calBase, token: token,
	}, nil
}

// Root devuelve la carpeta donde aterriza todo. No es la raíz del Drive de la
// persona: es una carpeta PROPIA («Sytue»), porque con `drive.file` la
// aplicación solo puede tocar lo que ella misma crea, y porque volcar una suite
// entera sobre la raíz de alguien es la forma más rápida de arruinar su Drive.
func (g *googleDest) Root(ctx context.Context) (string, error) {
	// Reanudar un trabajo NO puede crear una segunda carpeta «Sytue»: Drive
	// admite nombres repetidos, así que un corte de red dejaría dos árboles a
	// medias y nadie sabría cuál es cuál. Con `drive.file` la búsqueda solo ve lo
	// que esta aplicación creó, que es exactamente lo que hay que encontrar.
	q := url.Values{}
	q.Set("q", "name = 'Sytue' and mimeType = '"+googleFolderMime+"' and trashed = false")
	q.Set("fields", "files(id)")
	q.Set("pageSize", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.driveBase+"/drive/v3/files?"+q.Encode(), http.NoBody)
	if err != nil {
		return "", err
	}
	var found struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := g.do(ctx, req, &found); err == nil && len(found.Files) > 0 {
		return found.Files[0].ID, nil
	}
	return g.createFile(ctx, "", "Sytue", googleFolderMime, nil, "")
}

// CreateFolder crea una subcarpeta dentro de la carpeta indicada.
func (g *googleDest) CreateFolder(ctx context.Context, parentID, name string) (string, error) {
	return g.createFile(ctx, parentID, name, googleFolderMime, nil, "")
}

// UploadFile sube el contenido a una carpeta de Google Drive EN STREAMING.
func (g *googleDest) UploadFile(ctx context.Context, folderID, name, contentType string, body io.Reader) (string, error) {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return g.createFile(ctx, folderID, name, contentType, body, contentType)
}

// createFile crea una entrada en Drive: carpeta (sin cuerpo) o fichero.
//
// La subida va en multipart y por un io.Pipe: el fichero NUNCA se carga entero
// en memoria. Un buzón o un Drive de verdad tiene ficheros de gigabytes y son
// miles; construir el cuerpo en un buffer mataría el proceso en el primer vídeo.
func (g *googleDest) createFile(ctx context.Context, parentID, name, mimeType string, body io.Reader, contentType string) (string, error) {
	metadata := map[string]any{"name": name}
	if mimeType == googleFolderMime {
		metadata["mimeType"] = googleFolderMime
	}
	if parentID != "" {
		metadata["parents"] = []string{parentID}
	}
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	cuerpo, tipo := relacionado(metaJSON, contentType, body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.driveBase+googleUploadPath, cuerpo)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", tipo)
	var created struct {
		ID string `json:"id"`
	}
	if err := g.do(ctx, req, &created); err != nil {
		return "", err
	}
	if created.ID == "" {
		return "", fmt.Errorf("migrate: Google no devolvió el id de %q", name)
	}
	return created.ID, nil
}

// relacionado arma en streaming un multipart/related de Google —metadatos JSON y,
// si hay, el contenido—, que es como se sube a Drive y como se importa a Gmail.
func relacionado(meta []byte, contentType string, body io.Reader) (io.Reader, string) {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": []string{"application/json; charset=UTF-8"}})
		if err == nil {
			_, err = part.Write(meta)
		}
		if err == nil && body != nil {
			var media io.Writer
			if media, err = mw.CreatePart(textproto.MIMEHeader{"Content-Type": []string{contentType}}); err == nil {
				_, err = io.Copy(media, body)
			}
		}
		if err == nil {
			err = mw.Close()
		}
		_ = pw.CloseWithError(err) // con err nil, cierre normal.
	}()
	return pr, "multipart/related; boundary=" + mw.Boundary()
}

// AddressBook no tiene equivalente en Google: los contactos de una persona son
// UNA lista, sin libretas. Se devuelve un identificador simbólico para cumplir el
// contrato del runner sin inventar una jerarquía que allí no existe.
func (g *googleDest) AddressBook(context.Context) (string, error) { return "google:contacts", nil }

// CreateContact crea un contacto en la agenda de Google (People API).
func (g *googleDest) CreateContact(ctx context.Context, _ string, c Contact) (string, error) {
	person := map[string]any{
		"names": []map[string]any{{
			"givenName":   c.GivenName,
			"familyName":  c.FamilyName,
			"displayName": c.FormattedName,
		}},
	}
	if c.Organization != "" || c.Title != "" {
		person["organizations"] = []map[string]any{{"name": c.Organization, "title": c.Title}}
	}
	if c.Note != "" {
		person["biographies"] = []map[string]any{{"value": c.Note, "contentType": "TEXT_PLAIN"}}
	}
	if len(c.Emails) > 0 {
		emails := make([]map[string]any, 0, len(c.Emails))
		for _, e := range c.Emails {
			emails = append(emails, map[string]any{"value": e})
		}
		person["emailAddresses"] = emails
	}
	if len(c.Phones) > 0 {
		phones := make([]map[string]any, 0, len(c.Phones))
		for _, p := range c.Phones {
			phones = append(phones, map[string]any{"value": p})
		}
		person["phoneNumbers"] = phones
	}

	raw, err := json.Marshal(person)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.peopleBase+"/v1/people:createContact", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	//nolint:tagliatelle // contrato de la People API de Google (camelCase).
	var created struct {
		ResourceName string `json:"resourceName"`
	}
	if err := g.do(ctx, req, &created); err != nil {
		return "", err
	}
	return created.ResourceName, nil
}

// do emite la petición por el egress de la pasarela y traduce el error.
func (g *googleDest) do(ctx context.Context, req *http.Request, dst any) error {
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := g.rt.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: escribir en Google: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf(
			"migrate: Google rechazó la escritura (HTTP %d): la autorización de salida no incluye permiso para crear; "+
				"vuelve a conectar la cuenta de destino aceptando los permisos de escritura: %w", resp.StatusCode, ErrSinPermiso,
		)
	case resp.StatusCode == http.StatusRequestEntityTooLarge:
		return fmt.Errorf("%w: Google no acepta un fichero de este tamaño", ErrUnimportable)
	case resp.StatusCode == http.StatusBadRequest:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%w: Google no acepta esta entrada: %s", ErrUnimportable, strings.TrimSpace(string(msg)))
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Google"}
	}
	if dst == nil {
		return nil
	}
	// El ctx viaja en la petición; aquí solo se decodifica.
	_ = ctx
	return json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst)
}

var _ Destination = (*googleDest)(nil)
