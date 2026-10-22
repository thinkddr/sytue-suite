// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// sytueSource lee el contenido de Sytue para LLEVÁRSELO a otro sitio.
//
// Es la mitad que faltaba: el mismo motor que trae contenido lo lleva, con las
// mismas garantías. Y es deliberado que exista —no una concesión a regañadientes—
// porque una plataforma de la que no puedes salir no es soberana: es otra jaula,
// solo que con la bandera cambiada. Si alguien decide irse, que se vaya con todo
// y en un clic; lo que retiene a un cliente es que el producto sea mejor, no que
// sus datos estén secuestrados.
//
// Implementa Source, así que los recorridos de drive y contactos son los MISMOS
// que ya existen: no hay una segunda implementación del árbol ni de la paginación.
type sytueSource struct {
	drive    string
	contacts string
	token    TokenSource
	hc       *http.Client
}

// NewSytueSource construye el origen. token mintea el Bearer de la persona de la
// que se exporta; hc es opcional.
func NewSytueSource(driveBase, contactsBase string, token TokenSource, hc *http.Client) Source {
	if hc == nil {
		hc = &http.Client{Timeout: 0} // el corte lo pone el contexto del trabajo.
	}
	return &sytueSource{
		drive:    strings.TrimRight(driveBase, "/"),
		contacts: strings.TrimRight(contactsBase, "/"),
		token:    token,
		hc:       hc,
	}
}

func (*sytueSource) Label() string { return "Sytue" }

// ListChildren lista una carpeta de Sytue Drive; folderID vacío = la raíz.
//
// La paginación de ficheros se agota AQUÍ: el contrato de Source es «dame los
// hijos de esta carpeta», y devolver solo la primera página dejaría fuera, sin
// decir nada, todo lo que pase de un tamaño de página.
func (s *sytueSource) ListChildren(ctx context.Context, folderID string) ([]Item, error) {
	if folderID == "" {
		var root struct {
			ID string `json:"id"`
		}
		if err := s.get(ctx, s.drive+"/api/v1/folders/root", &root); err != nil {
			return nil, err
		}
		folderID = root.ID
	}
	type driveFile struct {
		ID          string    `json:"id"`
		Name        string    `json:"name"`
		SizeBytes   int64     `json:"size_bytes"`
		ContentType string    `json:"content_type"`
		UpdatedAt   time.Time `json:"updated_at"`
	}
	var first struct {
		Folders []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"folders"`
		Files      []driveFile `json:"files"`
		NextCursor string      `json:"next_cursor"`
	}
	base := s.drive + "/api/v1/folders/" + url.PathEscape(folderID)
	if err := s.get(ctx, base+"/children", &first); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(first.Folders)+len(first.Files))
	for _, f := range first.Folders {
		items = append(items, Item{ID: f.ID, Name: f.Name, IsFolder: true})
	}
	appendFiles := func(files []driveFile) {
		for _, f := range files {
			items = append(items, Item{
				ID: f.ID, Name: f.Name, Size: f.SizeBytes,
				MimeType: f.ContentType, Modified: f.UpdatedAt,
				DownloadURL: s.drive + "/api/v1/files/" + url.PathEscape(f.ID) + "/content",
			})
		}
	}
	appendFiles(first.Files)

	cursor := first.NextCursor
	for cursor != "" {
		var page struct {
			Files      []driveFile `json:"files"`
			NextCursor string      `json:"next_cursor"`
		}
		if err := s.get(ctx, base+"/files?cursor="+url.QueryEscape(cursor), &page); err != nil {
			return nil, err
		}
		appendFiles(page.Files)
		if page.NextCursor == cursor { // el destino no avanza: parar antes que girar en redondo.
			break
		}
		cursor = page.NextCursor
	}
	return items, nil
}

// Open abre el contenido de un fichero de Sytue Drive.
func (s *sytueSource) Open(ctx context.Context, it Item) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, it.DownloadURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	token, err := s.token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := s.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("migrate: descargar %q de Sytue Drive: %w", it.Name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("migrate: Sytue Drive devolvió HTTP %d al abrir %q", resp.StatusCode, it.Name)
	}
	return resp.Body, nil
}

// ListContacts devuelve una página de contactos de Sytue Contacts. El cursor es
// el desplazamiento, tal y como lo pagina el servicio.
func (s *sytueSource) ListContacts(ctx context.Context, pageToken string) ([]Contact, string, error) {
	u := s.contacts + "/api/v1/contacts/all?limit=200"
	if pageToken != "" {
		u += "&offset=" + url.QueryEscape(pageToken)
	}
	var page struct {
		Contacts []struct {
			ID            string `json:"id"`
			FormattedName string `json:"formatted_name"`
			GivenName     string `json:"given_name"`
			FamilyName    string `json:"family_name"`
			Organization  string `json:"organization"`
			Title         string `json:"title"`
			Note          string `json:"note"`
			Emails        []struct {
				Email string `json:"email"`
			} `json:"emails"`
			Phones []struct {
				Phone string `json:"phone"`
			} `json:"phones"`
		} `json:"contacts"`
		NextOffset int `json:"next_offset"`
	}
	if err := s.get(ctx, u, &page); err != nil {
		return nil, "", err
	}
	out := make([]Contact, 0, len(page.Contacts))
	for _, c := range page.Contacts {
		contact := Contact{
			ID: c.ID, FormattedName: c.FormattedName, GivenName: c.GivenName,
			FamilyName: c.FamilyName, Organization: c.Organization, Title: c.Title, Note: c.Note,
		}
		for _, e := range c.Emails {
			contact.Emails = append(contact.Emails, e.Email)
		}
		for _, p := range c.Phones {
			contact.Phones = append(contact.Phones, p.Phone)
		}
		out = append(out, contact)
	}
	if page.NextOffset == 0 {
		return out, "", nil
	}
	return out, strconv.Itoa(page.NextOffset), nil
}

// get emite un GET autenticado a la suite y decodifica la respuesta.
func (s *sytueSource) get(ctx context.Context, rawURL string, dst any) error {
	token, err := s.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	resp, err := s.hc.Do(req)
	if err != nil {
		return fmt.Errorf("migrate: %s: %w", redactPath(rawURL), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &suiteError{status: resp.StatusCode, msg: strings.TrimSpace(string(msg)), service: "Sytue"}
	}
	return json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(dst)
}

var _ Source = (*sytueSource)(nil)
