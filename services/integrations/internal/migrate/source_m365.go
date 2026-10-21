// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
)

// m365 lee OneDrive/SharePoint y los contactos de un usuario por Microsoft Graph.
//
// Tiene DOS formas de autenticarse, y de ahí salen los dos alcances del asistente:
//
//	UN SOLO LOGIN (consentimiento delegado) — la persona pulsa «Iniciar sesión con
//	Microsoft», aprueba los permisos de lectura y se migra lo suyo. El access token lo
//	INYECTA la pasarela en Runtime.HTTP, así que aquí no hay token y todo cuelga de
//	`/me`.
//
//	TODA LA ORGANIZACIÓN (permisos de aplicación) — con el consentimiento del
//	administrador del inquilino, la app tiene permisos de aplicación y se lee la cuenta
//	de cada persona. Con auth de aplicación NO existe `/me`: todo cuelga de
//	/users/{upn}, de ahí que el trabajo lleve el usuario de origen.
type m365 struct {
	rt   connector.Runtime
	base string
	// token vacío ⇒ delegado: lo inyecta la pasarela y el sujeto es quien autorizó.
	token string
	// user vacío ⇒ delegado (`/me`); poblado ⇒ app-only sobre esa cuenta.
	user string
}

// newM365 prepara el origen. Con sourceUser vacío usa el consentimiento de quien
// conectó; con sourceUser mintea el token de aplicación de la instancia y trabaja
// sobre la cuenta de esa persona.
func newM365(ctx context.Context, rt connector.Runtime, inst connector.Instance, sourceUser string) (*m365, error) {
	base, err := graph.APIBase(inst)
	if err != nil {
		return nil, err
	}
	m := &m365{rt: rt, base: base}
	if strings.TrimSpace(sourceUser) == "" {
		if !inst.HasBearer {
			return nil, fmt.Errorf(
				"%w: esta conexión con Microsoft 365 todavía no está autorizada; termina el paso «Iniciar sesión con Microsoft» del asistente, o indica el usuario de origen si vas a migrar con permisos de aplicación", connector.ErrBadQuery,
			)
		}
		return m, nil // token vacío: lo inyecta la pasarela; sujeto = /me.
	}
	token, err := graph.AppToken(ctx, rt, inst)
	if err != nil {
		return nil, err
	}
	m.token, m.user = token, strings.TrimSpace(sourceUser)
	return m, nil
}

func (*m365) Label() string { return "Microsoft 365" }

// userRoot es el prefijo de todas las llamadas: `/me` con el consentimiento de la
// persona, `/users/{upn}` con permisos de aplicación.
func (m *m365) userRoot() string {
	if m.user == "" {
		return m.base + "/v1.0/me"
	}
	return m.base + "/v1.0/users/" + url.PathEscape(m.user)
}

// graphItem es lo justo de un driveItem para migrarlo. La presencia de "folder"
// distingue carpeta de fichero; downloadUrl es la URL pre-autenticada de descarga.
//
//nolint:tagliatelle // tags = contrato de Microsoft Graph (camelCase + @odata); snake rompería la integración.
type graphItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Folder *struct {
		ChildCount int `json:"childCount"`
	} `json:"folder"`
	File *struct {
		MimeType string `json:"mimeType"`
	} `json:"file"`
	LastModified string `json:"lastModifiedDateTime"`
	//nolint:tagliatelle // es la anotación literal que EMITE Microsoft Graph.
	DownloadURL string `json:"@microsoft.graph.downloadUrl"`
}

// ListChildren lista una carpeta de OneDrive siguiendo @odata.nextLink.
// folderID vacío = la raíz del drive del usuario.
func (m *m365) ListChildren(ctx context.Context, folderID string) ([]Item, error) {
	next := m.userRoot() + "/drive/root/children"
	if folderID != "" {
		next = m.userRoot() + "/drive/items/" + url.PathEscape(folderID) + "/children"
	}
	next += "?$top=200&$select=id,name,size,folder,file,lastModifiedDateTime"

	out := make([]Item, 0, 64)
	for page := 0; next != "" && page < maxPages; page++ {
		//nolint:tagliatelle // @odata.nextLink es el nombre literal que emite Graph.
		var resp struct {
			Value    []graphItem `json:"value"`
			NextLink string      `json:"@odata.nextLink"`
		}
		if err := getJSON(ctx, m.rt, next, m.token, &resp); err != nil {
			return nil, err
		}
		for _, gi := range resp.Value {
			out = append(out, gi.toItem())
		}
		if strings.TrimSpace(resp.NextLink) == "" {
			return out, nil
		}
		var err error
		if next, err = sameHostNext(next, resp.NextLink); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (gi graphItem) toItem() Item {
	it := Item{ID: gi.ID, Name: gi.Name, Size: gi.Size, IsFolder: gi.Folder != nil, DownloadURL: gi.DownloadURL}
	if gi.File != nil {
		it.MimeType = gi.File.MimeType
	}
	if t, err := time.Parse(time.RFC3339, gi.LastModified); err == nil {
		it.Modified = t
	}
	return it
}

// Open descarga el contenido de un driveItem. Graph publica una URL
// PRE-AUTENTICADA de vida corta (@microsoft.graph.downloadUrl) que se usa sin
// cabecera de auth; si no viene en el listado, se pide del propio item. Se evita
// a propósito /content: responde un 302 a otro host y la política de egress corta
// los redirects cross-host (filtrarían el token).
func (m *m365) Open(ctx context.Context, it Item) (io.ReadCloser, error) {
	dl := it.DownloadURL
	if dl == "" {
		var meta graphItem
		u := m.userRoot() + "/drive/items/" + url.PathEscape(it.ID) + "?$select=id,@microsoft.graph.downloadUrl"
		if err := getJSON(ctx, m.rt, u, m.token, &meta); err != nil {
			return nil, err
		}
		dl = meta.DownloadURL
	}
	if dl == "" {
		return nil, errors.New("microsoft graph no entregó una URL de descarga para el fichero")
	}
	return openStream(ctx, m.rt, dl, "") // pre-autenticada: sin Bearer.
}

// ListContacts lee la agenda personal del usuario (/users/{upn}/contacts).
func (m *m365) ListContacts(ctx context.Context, pageToken string) ([]Contact, string, error) {
	u := pageToken
	if u == "" {
		u = m.userRoot() + "/contacts?$top=100" +
			"&$select=id,displayName,givenName,surname,companyName,jobTitle,personalNotes,emailAddresses,businessPhones,mobilePhone"
	}
	//nolint:tagliatelle // los tags son el contrato de Microsoft Graph (camelCase + @odata).
	var resp struct {
		Value []struct {
			ID             string   `json:"id"`
			DisplayName    string   `json:"displayName"`
			GivenName      string   `json:"givenName"`
			Surname        string   `json:"surname"`
			CompanyName    string   `json:"companyName"`
			JobTitle       string   `json:"jobTitle"`
			PersonalNotes  string   `json:"personalNotes"`
			BusinessPhones []string `json:"businessPhones"`
			MobilePhone    string   `json:"mobilePhone"`
			EmailAddresses []struct {
				Address string `json:"address"`
			} `json:"emailAddresses"`
		} `json:"value"`
		NextLink string `json:"@odata.nextLink"`
	}
	if err := getJSON(ctx, m.rt, u, m.token, &resp); err != nil {
		return nil, "", err
	}

	out := make([]Contact, 0, len(resp.Value))
	for _, c := range resp.Value {
		mc := Contact{
			ID:            c.ID,
			FormattedName: firstNonEmpty(c.DisplayName, strings.TrimSpace(c.GivenName+" "+c.Surname), c.CompanyName),
			GivenName:     c.GivenName,
			FamilyName:    c.Surname,
			Organization:  c.CompanyName,
			Title:         c.JobTitle,
			Note:          c.PersonalNotes,
			Phones:        append([]string(nil), c.BusinessPhones...),
		}
		for _, e := range c.EmailAddresses {
			if strings.TrimSpace(e.Address) != "" {
				mc.Emails = append(mc.Emails, e.Address)
			}
		}
		if strings.TrimSpace(c.MobilePhone) != "" {
			mc.Phones = append(mc.Phones, c.MobilePhone)
		}
		out = append(out, mc)
	}

	next := ""
	if strings.TrimSpace(resp.NextLink) != "" {
		var err error
		if next, err = sameHostNext(u, resp.NextLink); err != nil {
			return nil, "", err
		}
	}
	return out, next, nil
}

// firstNonEmpty devuelve el primer valor no vacío (tras recortar espacios).
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

var _ Source = (*m365)(nil)
