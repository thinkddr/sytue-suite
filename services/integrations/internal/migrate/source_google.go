// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
)

// Claves de Instance.Config que permiten apuntar las APIs de Google a otra raíz
// (nubes soberanas y los servidores simulados de los tests).
const (
	ConfigGoogleDriveBase  = "google_drive_base"
	ConfigGooglePeopleBase = "google_people_base"
)

const (
	defaultGoogleDriveBase  = "https://www.googleapis.com"
	defaultGooglePeopleBase = "https://people.googleapis.com"

	// Scopes de SOLO LECTURA: la migración lee del origen, jamás escribe en él.
	//nolint:gosec // G101: identificadores de scope OAuth públicos, no credenciales.
	scopeDriveRead = "https://www.googleapis.com/auth/drive.readonly"
	//nolint:gosec // G101: identificador de scope OAuth público, no una credencial.
	scopeContactsRead = "https://www.googleapis.com/auth/contacts.readonly"

	// folderMime es el tipo MIME con el que Google marca una carpeta.
	folderMime = "application/vnd.google-apps.folder"
	// nativePrefix marca los formatos NATIVOS de Google (Docs, Sheets, Slides…):
	// no tienen bytes descargables, hay que EXPORTARLOS.
	nativePrefix = "application/vnd.google-apps."
)

// exportAs describe a qué se convierte un documento nativo de Google y adónde va.
type exportAs struct {
	Mime string // formato de exportación que se le pide a Google
	Ext  string // extensión con la que viaja (Docs detecta el formato por ella)
	// ToDocs distingue lo que se abre y se EDITA en Sytue Docs de lo que es un
	// fichero normal: un dibujo exportado a PNG es una imagen, y su sitio es Drive.
	ToDocs bool
	// Skip explica por qué un nativo NO se puede traer, cuando es el caso.
	Skip string
}

// googleExports es el mapa de conversión de los formatos nativos. Lo que no está
// aquí no tiene equivalente honesto: un formulario o un script no son documentos
// que se puedan abrir en otra suite, y fingir que sí acabaría en un fichero vacío.
var googleExports = map[string]exportAs{
	"application/vnd.google-apps.document": {
		Mime: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Ext:  ".docx", ToDocs: true,
	},
	"application/vnd.google-apps.spreadsheet": {
		Mime: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		Ext:  ".xlsx", ToDocs: true,
	},
	"application/vnd.google-apps.presentation": {
		Mime: "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		Ext:  ".pptx", ToDocs: true,
	},
	// Un dibujo sí tiene una representación fiel: una imagen. Va a Drive.
	"application/vnd.google-apps.drawing": {Mime: "image/png", Ext: ".png"},
	// Estos no viajan, y el informe dice por qué.
	"application/vnd.google-apps.form": {
		Skip: "es un formulario de Google: sus respuestas y su lógica no tienen equivalente que importar (la hoja de respuestas, si la hay, sí se migra aparte)",
	},
	"application/vnd.google-apps.script": {
		Skip: "es un script de Apps Script: es código de la plataforma de Google, no un documento",
	},
	"application/vnd.google-apps.site": {
		Skip: "es un sitio de Google Sites: no hay documento equivalente al que convertirlo",
	},
	"application/vnd.google-apps.map": {
		Skip: "es un mapa de My Maps: no hay documento equivalente al que convertirlo",
	},
	"application/vnd.google-apps.shortcut": {
		Skip: "es un acceso directo a otro fichero, no contenido propio: el fichero original se migra por su cuenta",
	},
}

// google lee Google Drive (v3) y la agenda (People API) del usuario de origen.
//
// Tiene DOS formas de autenticarse, y de ahí salen los dos alcances que ofrece el
// asistente:
//
//	UN SOLO LOGIN (consentimiento) — la persona pulsa «Conectar con Google», aprueba
//	los permisos de lectura y se migra lo suyo. El access token lo INYECTA la
//	pasarela en Runtime.HTTP, así que aquí no hay token: `token` va vacío. Google no
//	permite leer el contenido de OTRAS personas con un token de usuario, y por eso
//	este camino migra exactamente la cuenta de quien autorizó.
//
//	TODA LA ORGANIZACIÓN (delegación) — con una cuenta de servicio con delegación a
//	nivel de dominio se impersona a cada persona del Workspace. Es el camino que
//	pide un usuario de origen concreto y mintea un token por cada uno.
type google struct {
	rt         connector.Runtime
	driveBase  string
	peopleBase string
	// gmailAPIBase y gcalAPIBase son las raíces de Gmail y Calendar
	// (configurables para nubes soberanas y para los tests).
	gmailAPIBase string
	gcalAPIBase  string
	// Raíces de Google Cloud y Firebase (dominio 'cloud'), con el proyecto y las
	// regiones de las que migrar. Google no tiene una API única de plataforma:
	// cada producto vive en su propio host.
	gcpRunBase       string
	gcpStorageBase   string
	gcpSecretBase    string
	gcpSchedulerBase string
	gcpSQLBase       string
	gcpFirestoreBase string
	gcpHostingBase   string
	gcpProject       string
	gcpRegions       string
	token            string
}

// newGoogle prepara el origen. Con sourceUser vacío usa el consentimiento de quien
// conectó (un login y nada más); con sourceUser mintea el token impersonándolo con
// el scope que necesita el dominio (drive.readonly o contacts.readonly).
func newGoogle(ctx context.Context, rt connector.Runtime, inst connector.Instance, sourceUser, scope string) (*google, error) {
	driveBase, err := baseURLOr(inst.Config[ConfigGoogleDriveBase], defaultGoogleDriveBase)
	if err != nil {
		return nil, err
	}
	peopleBase, err := baseURLOr(inst.Config[ConfigGooglePeopleBase], defaultGooglePeopleBase)
	if err != nil {
		return nil, err
	}
	g := &google{
		rt: rt, driveBase: driveBase, peopleBase: peopleBase,
		gmailAPIBase: inst.Config[ConfigGmailBase],
		gcalAPIBase:  inst.Config[ConfigGCalBase],

		gcpRunBase:       inst.Config[ConfigGCPRunBase],
		gcpStorageBase:   inst.Config[ConfigGCPStorageBase],
		gcpSecretBase:    inst.Config[ConfigGCPSecretBase],
		gcpSchedulerBase: inst.Config[ConfigGCPSchedulerBase],
		gcpSQLBase:       inst.Config[ConfigGCPSQLBase],
		gcpFirestoreBase: inst.Config[ConfigGCPFirestoreBase],
		gcpHostingBase:   inst.Config[ConfigGCPHostingBase],
		gcpProject:       inst.Config[ConfigGCPProject],
		gcpRegions:       inst.Config[ConfigGCPRegions],
	}
	if strings.TrimSpace(sourceUser) == "" {
		if !inst.HasBearer {
			return nil, fmt.Errorf(
				"%w: esta conexión con Google todavía no está autorizada; termina el paso «Conectar con Google» del asistente, o indica el usuario de origen si vas a migrar con delegación", connector.ErrBadQuery,
			)
		}
		return g, nil // token vacío: lo inyecta la pasarela.
	}
	token, err := gws.UserToken(ctx, rt, inst, sourceUser, scope)
	if err != nil {
		return nil, err
	}
	g.token = token
	return g, nil
}

func (*google) Label() string { return "Google Workspace" }

// ListChildren lista los hijos de una carpeta de Drive (v3). folderID vacío = la
// raíz ('root'). Los ficheros nativos se devuelven YA marcados como omitidos con
// su motivo: honestidad por construcción, el runner no los intenta.
func (g *google) ListChildren(ctx context.Context, folderID string) ([]Item, error) {
	parent := folderID
	if parent == "" {
		parent = "root"
	}
	q := url.Values{}
	q.Set("q", "'"+strings.ReplaceAll(parent, "'", `\'`)+"' in parents and trashed = false")
	q.Set("fields", "nextPageToken,files(id,name,mimeType,size,modifiedTime)")
	q.Set("pageSize", "200")
	q.Set("orderBy", "name") // orden estable: el recorrido debe ser reproducible.

	out := make([]Item, 0, 64)
	pageToken := ""
	for page := 0; page < maxPages; page++ {
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		//nolint:tagliatelle // los tags son el CONTRATO de Google Drive v3 (camelCase):
		// renombrarlos a snake rompe la integración. Mismo criterio que internal/driver.
		var resp struct {
			Files []struct {
				ID           string `json:"id"`
				Name         string `json:"name"`
				MimeType     string `json:"mimeType"`
				Size         string `json:"size"` // Drive v3 emite int64 como string.
				ModifiedTime string `json:"modifiedTime"`
			} `json:"files"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := getJSON(ctx, g.rt, g.driveBase+"/drive/v3/files?"+q.Encode(), g.token, &resp); err != nil {
			return nil, err
		}
		for _, f := range resp.Files {
			it := Item{ID: f.ID, Name: f.Name, MimeType: f.MimeType, IsFolder: f.MimeType == folderMime}
			it.Size = parseInt64(f.Size)
			if t, err := time.Parse(time.RFC3339, f.ModifiedTime); err == nil {
				it.Modified = t
			}
			if !it.IsFolder && strings.HasPrefix(f.MimeType, nativePrefix) {
				g.prepareExport(&it, f.MimeType)
			}
			out = append(out, it)
		}
		if resp.NextPageToken == "" {
			return out, nil
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

// prepareExport decide qué hacer con un documento nativo: a qué formato se
// exporta, con qué extensión viaja y si su destino es Sytue Docs o Drive.
func (g *google) prepareExport(it *Item, nativeMime string) {
	spec, known := googleExports[nativeMime]
	switch {
	case !known:
		it.SkipReason = "documento nativo de Google (" + nativeMime +
			"): no hay formato al que exportarlo sin perder lo que lo hace ese documento"
		return
	case spec.Skip != "":
		it.SkipReason = spec.Skip
		return
	}
	// La URL de exportación ES la de descarga de este item: Open no necesita saber
	// nada más. Google exporta el documento al vuelo con el formato pedido.
	q := url.Values{}
	q.Set("mimeType", spec.Mime)
	it.DownloadURL = g.driveBase + "/drive/v3/files/" + url.PathEscape(it.ID) + "/export?" + q.Encode()
	it.MimeType = spec.Mime
	it.Native = spec.ToDocs
	if !strings.EqualFold(path.Ext(it.Name), spec.Ext) {
		it.Name += spec.Ext
	}
	// El tamaño del original no dice nada del exportado (Google no lo publica), y
	// contar bytes que no son los que viajan sería mentir en el informe.
	it.Size = 0
}

// Open descarga los bytes de un fichero de Drive: los binarios por alt=media y
// los nativos por la URL de EXPORTACIÓN que preparó ListChildren.
func (g *google) Open(ctx context.Context, it Item) (io.ReadCloser, error) {
	u := it.DownloadURL
	if u == "" {
		u = g.driveBase + "/drive/v3/files/" + url.PathEscape(it.ID) + "?alt=media"
	}
	body, err := openStream(ctx, g.rt, u, g.token)
	if err != nil && isExportTooLarge(err) {
		// Google se niega a exportar por encima de 10 MB. Reintentarlo daría siempre
		// lo mismo, así que se registra como omisión con su motivo.
		return nil, fmt.Errorf(
			"%w: Google no exporta documentos de más de 10 MB; descárgalo a mano desde Drive", ErrUnimportable,
		)
	}
	return body, err
}

// isExportTooLarge reconoce el rechazo de Google al exportar un documento
// demasiado grande (403 con `exportSizeLimitExceeded`).
func isExportTooLarge(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "exportSizeLimitExceeded") ||
		strings.Contains(msg, "This file is too large to be exported")
}

// ListContacts lee la agenda del usuario impersonado (People API, connections de
// "me"). pageToken es el cursor nativo del API.
func (g *google) ListContacts(ctx context.Context, pageToken string) ([]Contact, string, error) {
	q := url.Values{}
	q.Set("personFields", "names,emailAddresses,phoneNumbers,organizations,biographies")
	q.Set("pageSize", "200")
	if pageToken != "" {
		q.Set("pageToken", pageToken)
	}
	//nolint:tagliatelle // los tags son el contrato de la People API de Google (camelCase).
	var resp struct {
		Connections []struct {
			ResourceName string `json:"resourceName"`
			Names        []struct {
				DisplayName string `json:"displayName"`
				GivenName   string `json:"givenName"`
				FamilyName  string `json:"familyName"`
			} `json:"names"`
			EmailAddresses []struct {
				Value string `json:"value"`
			} `json:"emailAddresses"`
			PhoneNumbers []struct {
				Value string `json:"value"`
			} `json:"phoneNumbers"`
			Organizations []struct {
				Name  string `json:"name"`
				Title string `json:"title"`
			} `json:"organizations"`
			Biographies []struct {
				Value string `json:"value"`
			} `json:"biographies"`
		} `json:"connections"`
		NextPageToken string `json:"nextPageToken"`
	}
	if err := getJSON(ctx, g.rt, g.peopleBase+"/v1/people/me/connections?"+q.Encode(), g.token, &resp); err != nil {
		return nil, "", err
	}

	out := make([]Contact, 0, len(resp.Connections))
	for _, p := range resp.Connections {
		c := Contact{ID: p.ResourceName}
		if len(p.Names) > 0 {
			c.FormattedName = p.Names[0].DisplayName
			c.GivenName, c.FamilyName = p.Names[0].GivenName, p.Names[0].FamilyName
		}
		if len(p.Organizations) > 0 {
			c.Organization, c.Title = p.Organizations[0].Name, p.Organizations[0].Title
		}
		if len(p.Biographies) > 0 {
			c.Note = p.Biographies[0].Value
		}
		for _, e := range p.EmailAddresses {
			if strings.TrimSpace(e.Value) != "" {
				c.Emails = append(c.Emails, e.Value)
			}
		}
		for _, ph := range p.PhoneNumbers {
			if strings.TrimSpace(ph.Value) != "" {
				c.Phones = append(c.Phones, ph.Value)
			}
		}
		c.FormattedName = firstNonEmpty(c.FormattedName, strings.TrimSpace(c.GivenName+" "+c.FamilyName), c.Organization)
		out = append(out, c)
	}
	return out, resp.NextPageToken, nil
}

// parseInt64 lee el tamaño que Drive v3 emite como string ("" ⇒ 0).
func parseInt64(s string) int64 {
	var n int64
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

var _ Source = (*google)(nil)
