// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
)

// ConfigGitHubAPIBase apunta la API de GitHub a otra raíz: GitHub Enterprise
// Server (https://ghe.acme.com/api/v3) y los servidores simulados de los tests.
const ConfigGitHubAPIBase = "github_api_base"

const (
	defaultGitHubAPIBase = "https://api.github.com"
	// githubAPIVersion fija la versión de la API REST: sin ella GitHub sirve «la
	// última», que es exactamente lo que no quiere una integración que debe seguir
	// funcionando dentro de dos años.
	githubAPIVersion = "2022-11-28"
	// secretKeyBearer es la clave convencional del vault donde el canje del
	// consentimiento deja el access token (== store.SecretKeyBearer). Se declara
	// aquí para no acoplar el runner a la capa de persistencia.
	secretKeyBearer = "bearer" //nolint:gosec // nombre de la clave, no la credencial
	// gistPrefix marca los ids del árbol sintético de gists ("gist:<id>").
	gistPrefix = "gist:"
	// reposPerPage es el tamaño de página de los listados (el máximo de GitHub).
	reposPerPage = 100
)

// github lee la cuenta de GitHub que autorizó la conexión: sus repositorios (los
// propios y los de las organizaciones de las que forma parte) y sus gists.
//
// El token del consentimiento NO se usa aquí para firmar peticiones: lo inyecta la
// pasarela en Runtime.HTTP como cualquier otro egress. La copia que sí lo necesita
// en claro es la de repositorios —la forja de destino clona el origen y sin
// credencial no ve un repositorio privado—, y para eso está cloneToken.
type github struct {
	rt   connector.Runtime
	base string
	// cloneToken es la credencial que se entrega a Sytue Git para clonar. Vive en
	// memoria durante el trabajo y jamás se registra: la auditoría de egress redacta
	// Authorization, y este valor solo viaja en el cuerpo de la petición a la forja.
	cloneToken string
}

// newGitHub construye el origen. Exige que la conexión esté hecha por
// CONSENTIMIENTO: GitHub publica flujo OAuth 2.0, así que pedir un token pegado a
// mano sería el último recurso de un proveedor que no lo ofrece, y no es el caso.
func newGitHub(rt connector.Runtime, inst connector.Instance) (*github, error) {
	if !inst.HasBearer {
		return nil, fmt.Errorf(
			"%w: esta conexión con GitHub todavía no está autorizada; termina el paso «Conectar con GitHub» del asistente", connector.ErrBadQuery,
		)
	}
	base, err := baseURLOr(inst.Config[ConfigGitHubAPIBase], defaultGitHubAPIBase)
	if err != nil {
		return nil, err
	}
	return &github{rt: rt, base: base, cloneToken: inst.Secrets[secretKeyBearer]}, nil
}

func (*github) Label() string { return "GitHub" }

// apiHeaders son las cabeceras que GitHub espera de un cliente de su API REST: el
// media type versionado y la versión explícita de la API.
func apiHeaders() http.Header {
	return http.Header{
		"Accept":               []string{"application/vnd.github+json"},
		"X-Github-Api-Version": []string{githubAPIVersion},
	}
}

// get es un GET a la API de GitHub con sus cabeceras. El token no viaja aquí: lo
// inyecta la pasarela en Runtime.HTTP (el driver nunca ve la credencial).
func (g *github) get(ctx context.Context, path string, dst any) error {
	return getJSONWith(ctx, g.rt, g.base+path, "", apiHeaders(), dst)
}

// --- Repositorios ------------------------------------------------------------

// ListRepos devuelve una página de los repositorios que la cuenta autorizada
// alcanza: los suyos, aquellos en los que colabora y los de sus organizaciones.
// Es literalmente «todo lo que tiene» quien se conectó; lo que su token no vea, no
// existe para la migración.
func (g *github) ListRepos(ctx context.Context, cursor string) ([]Repo, string, error) {
	page := pageOr1(cursor)
	q := url.Values{}
	q.Set("per_page", strconv.Itoa(reposPerPage))
	q.Set("page", strconv.Itoa(page))
	q.Set("affiliation", "owner,collaborator,organization_member")
	q.Set("sort", "full_name") // orden estable: el recorrido debe ser reproducible.

	//nolint:tagliatelle // los tags son el CONTRATO de la API REST de GitHub.
	var resp []struct {
		ID       int64  `json:"id"`
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		Owner    struct {
			Login string `json:"login"`
			Type  string `json:"type"` // "User" | "Organization"
		} `json:"owner"`
		CloneURL    string `json:"clone_url"`
		Description string `json:"description"`
		Private     bool   `json:"private"`
		Archived    bool   `json:"archived"`
		Size        int64  `json:"size"`
	}
	if err := g.get(ctx, "/user/repos?"+q.Encode(), &resp); err != nil {
		return nil, "", err
	}

	out := make([]Repo, 0, len(resp))
	for _, r := range resp {
		out = append(out, Repo{
			ID:          "repo:" + strconv.FormatInt(r.ID, 10),
			Owner:       r.Owner.Login,
			Name:        r.Name,
			FullName:    r.FullName,
			CloneURL:    r.CloneURL,
			Description: r.Description,
			Private:     r.Private,
			Archived:    r.Archived,
			OwnerIsOrg:  r.Owner.Type == "Organization",
			AuthToken:   g.cloneToken,
			// Empty se deja sin poblar A PROPÓSITO: GitHub no publica un indicador de
			// «sin commits» y deducirlo de size==0 marcaría como vacíos repositorios
			// recién creados cuyo tamaño aún no ha recalculado. Saltarse un repositorio
			// real es peor que intentarlo: si de verdad está vacío, la forja lo dice y
			// se registra como omisión con ese motivo (ErrUnimportable).
		})
	}
	// Página incompleta ⇒ era la última. GitHub pagina por Link, pero el número de
	// página es cursor suficiente aquí y sobrevive a una reanudación (el Link de una
	// respuesta vieja, no).
	if len(resp) < reposPerPage {
		return out, "", nil
	}
	return out, strconv.Itoa(page + 1), nil
}

// --- Gists -------------------------------------------------------------------

// ListChildren presenta los gists como un árbol de carpetas para que aterricen en
// Sytue Drive: la raíz lista un «directorio» por gist y cada uno, sus ficheros.
func (g *github) ListChildren(ctx context.Context, folderID string) ([]Item, error) {
	if folderID == "" {
		return g.listGists(ctx)
	}
	return g.gistFiles(ctx, strings.TrimPrefix(folderID, gistPrefix))
}

// listGists recorre TODOS los gists de la cuenta (paginación interna acotada).
func (g *github) listGists(ctx context.Context) ([]Item, error) {
	out := make([]Item, 0, 64)
	for page := 1; page <= maxPages; page++ {
		q := url.Values{}
		q.Set("per_page", strconv.Itoa(reposPerPage))
		q.Set("page", strconv.Itoa(page))

		//nolint:tagliatelle // contrato de la API REST de GitHub.
		var resp []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
			Public      bool   `json:"public"`
			Files       map[string]struct {
				Filename string `json:"filename"`
			} `json:"files"`
		}
		if err := g.get(ctx, "/gists?"+q.Encode(), &resp); err != nil {
			return nil, err
		}
		for _, gi := range resp {
			out = append(out, Item{
				ID:       gistPrefix + gi.ID,
				Name:     gistFolderName(gi.ID, gi.Description, gi.Files),
				IsFolder: true,
			})
		}
		if len(resp) < reposPerPage {
			return out, nil
		}
	}
	return out, nil
}

// gistFiles lista los ficheros de un gist. El contenido no se trae aquí: lo pide
// Open al copiarlo, para no cargar en memoria lo que no se va a subir todavía.
func (g *github) gistFiles(ctx context.Context, gistID string) ([]Item, error) {
	gi, err := g.gist(ctx, gistID)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(gi.Files))
	for name, f := range gi.Files {
		filename := firstNonEmpty(f.Filename, name)
		it := Item{
			ID:          gistPrefix + gistID + "/" + filename,
			Name:        safeFileName(filename),
			Size:        f.Size,
			MimeType:    firstNonEmpty(f.Type, "text/plain"),
			DownloadURL: f.RawURL,
		}
		out = append(out, it)
	}
	return out, nil
}

// gistDoc es la forma de un gist con sus ficheros ya resueltos.
type gistDoc struct {
	Files map[string]struct {
		Filename  string `json:"filename"`
		Type      string `json:"type"`
		RawURL    string `json:"raw_url"`
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
		Content   string `json:"content"`
	} `json:"files"`
}

// gist trae un gist completo (ficheros con contenido si no están truncados).
//
//nolint:tagliatelle // contrato de la API REST de GitHub.
func (g *github) gist(ctx context.Context, gistID string) (gistDoc, error) {
	var doc gistDoc
	err := g.get(ctx, "/gists/"+url.PathEscape(gistID), &doc)
	return doc, err
}

// Open devuelve el contenido de un fichero de un gist.
//
// ponytail: se vuelve a pedir el gist por fichero en vez de cachear el que listó
// ListChildren. Un gist son unos pocos ficheros pequeños y el listado y la copia
// no ocurren a la vez; cachearlo sería estado vivo por ahorrar una petición.
func (g *github) Open(ctx context.Context, it Item) (io.ReadCloser, error) {
	gistID, filename, ok := splitGistFileID(it.ID)
	if !ok {
		return nil, fmt.Errorf("migrate: identificador de fichero de gist inesperado: %q", it.ID)
	}
	doc, err := g.gist(ctx, gistID)
	if err != nil {
		return nil, err
	}
	for name, f := range doc.Files {
		if firstNonEmpty(f.Filename, name) != filename {
			continue
		}
		if f.Truncated || f.Content == "" {
			// Fichero grande: GitHub no lo sirve en el JSON y hay que ir al blob crudo.
			return openStream(ctx, g.rt, f.RawURL, "")
		}
		return io.NopCloser(strings.NewReader(f.Content)), nil
	}
	return nil, fmt.Errorf("migrate: el gist %s ya no tiene el fichero %q", gistID, filename)
}

// ListContacts existe porque el contrato Source lo pide; GitHub no tiene agenda.
// Nadie debería llegar aquí: MigrationSource.Supports rechaza «contacts» para
// GitHub al crear el trabajo.
func (*github) ListContacts(context.Context, string) ([]Contact, string, error) {
	return nil, "", fmt.Errorf("migrate: GitHub no tiene agenda de contactos que migrar")
}

// --- Helpers -----------------------------------------------------------------

// gistFolderName da nombre a la carpeta de un gist: su descripción, o el nombre de
// su primer fichero si no la tiene. Lleva SIEMPRE el id corto porque las
// descripciones se repiten y dos carpetas indistinguibles en Drive no ayudan a
// nadie.
func gistFolderName(id, description string, files map[string]struct {
	Filename string `json:"filename"`
},
) string {
	name := strings.TrimSpace(description)
	if name == "" {
		for k, f := range files {
			name = firstNonEmpty(f.Filename, k)
			break // mapa: cualquiera vale como etiqueta; el id corto lo desambigua.
		}
	}
	if name == "" {
		name = "gist"
	}
	short := id
	if len(short) > 7 {
		short = short[:7]
	}
	return safeFileName(name) + " (" + short + ")"
}

// splitGistFileID parte "gist:<id>/<fichero>" en sus dos mitades.
func splitGistFileID(id string) (gistID, filename string, ok bool) {
	rest, found := strings.CutPrefix(id, gistPrefix)
	if !found {
		return "", "", false
	}
	gistID, filename, found = strings.Cut(rest, "/")
	if !found || gistID == "" || filename == "" {
		return "", "", false
	}
	return gistID, filename, true
}

// safeFileName deja un nombre utilizable como carpeta o fichero en Sytue Drive:
// sin separadores de ruta ni saltos de línea, y acotado. El nombre viene de un
// tercero, así que se sanea aquí y no se confía en que el destino lo haga.
func safeFileName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 0x20 {
			return '-'
		}
		return r
	}, strings.TrimSpace(s))
	if len(s) > 120 {
		s = s[:120]
	}
	if s == "" || s == "." || s == ".." {
		return "sin-nombre"
	}
	return s
}

// pageOr1 lee el cursor de paginación (número de página) con 1 por defecto.
func pageOr1(cursor string) int {
	n, err := strconv.Atoi(strings.TrimSpace(cursor))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

var (
	_ Source     = (*github)(nil)
	_ RepoSource = (*github)(nil)
)
