// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strings"
	"time"
)

// GoogleDrive habla con la API v3 de Google Drive.
type GoogleDrive struct {
	Token func(context.Context) (string, error)
	HTTP  *http.Client
	// API y Subidas se cambian en pruebas.
	API, Subidas string
}

const (
	apiGoogle     = "https://www.googleapis.com/drive/v3"
	subidasGoogle = "https://www.googleapis.com/upload/drive/v3"
)

// Los documentos nativos de Google no tienen bytes: se exportan al formato abierto.
var exportarGoogle = map[string][2]string{
	"application/vnd.google-apps.document":     {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
	"application/vnd.google-apps.spreadsheet":  {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
	"application/vnd.google-apps.presentation": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
}

func (g *GoogleDrive) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	t, err := g.Token(ctx)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t)
	return g.HTTP.Do(req.WithContext(ctx))
}

// Listar pide los hijos de la carpeta ("" es «Mi unidad»).
func (g *GoogleDrive) Listar(ctx context.Context, carpeta string) ([]Elemento, error) {
	id := cmp.Or(carpeta, "root")
	var out []Elemento
	pagina := ""
	for {
		q := url.Values{
			"q":        {fmt.Sprintf("'%s' in parents and trashed = false", strings.ReplaceAll(id, "'", `\'`))},
			"fields":   {"nextPageToken,files(id,name,mimeType,size,modifiedTime,createdTime,version,md5Checksum)"},
			"pageSize": {"200"},
		}
		if pagina != "" {
			q.Set("pageToken", pagina)
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cmp.Or(g.API, apiGoogle)+"/files?"+q.Encode(), http.NoBody)
		resp, err := g.do(ctx, req)
		if err != nil {
			return nil, err
		}
		var r struct {
			Next  string `json:"nextPageToken"` //nolint:tagliatelle // contrato de la API de Google Drive v3
			Files []struct {
				ID, Name, MimeType, Size, Version, Md5Checksum string
				ModifiedTime, CreatedTime                      time.Time
			} `json:"files"`
		}
		if resp.StatusCode != http.StatusOK {
			err := errorDeEstado("Google Drive", resp)
			_ = resp.Body.Close()
			return nil, err
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, f := range r.Files {
			e := Elemento{
				ID: f.ID, Nombre: f.Name, Tipo: f.MimeType, Modificado: f.ModifiedTime, Creado: f.CreatedTime, Version: f.Version,
				Carpeta: f.MimeType == carpetaGoogle, Estable: f.ID,
			}
			_, _ = fmt.Sscan(f.Size, &e.Tamano) // un documento de Google no tiene tamaño: se queda en 0
			if f.Md5Checksum != "" {
				e.Huella = "md5:" + strings.ToLower(f.Md5Checksum)
			}
			out = append(out, e)
		}
		if r.Next == "" {
			break
		}
		pagina = r.Next
	}
	ordenar(out)
	return out, nil
}

// Descargar baja el contenido, o exporta un documento de Google a OOXML.
func (g *GoogleDrive) Descargar(ctx context.Context, e Elemento) ([]byte, string, error) {
	nombre := e.Nombre
	destino := cmp.Or(g.API, apiGoogle) + "/files/" + url.PathEscape(e.ID) + "?alt=media"
	if exp, ok := exportarGoogle[e.Tipo]; ok {
		destino = cmp.Or(g.API, apiGoogle) + "/files/" + url.PathEscape(e.ID) + "/export?mimeType=" + url.QueryEscape(exp[0])
		if !strings.HasSuffix(strings.ToLower(nombre), exp[1]) {
			nombre += exp[1]
		}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, destino, http.NoBody)
	resp, err := g.do(ctx, req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errorDeEstado("Google Drive", resp)
	}
	datos, err := leerAcotado(resp.Body)
	return datos, nombre, err
}

// Subir reemplaza el fichero con ese nombre en la carpeta, o lo crea. Si hay varios que se
// llaman así (Google lo admite), el primero de [Ordenar]: el más antiguo. Para sustituir
// uno concreto, [GoogleDrive.Reemplazar].
func (g *GoogleDrive) Subir(ctx context.Context, carpeta, nombre string, datos []byte) (Elemento, error) {
	hijos, err := g.Listar(ctx, carpeta)
	if err != nil {
		return Elemento{}, err
	}
	if i := slices.IndexFunc(hijos, func(e Elemento) bool { return !e.Carpeta && e.Nombre == nombre }); i >= 0 {
		return g.Reemplazar(ctx, hijos[i], datos)
	}
	var cuerpo bytes.Buffer
	mw := multipart.NewWriter(&cuerpo)
	meta, _ := json.Marshal(map[string]any{"name": nombre, "parents": []string{cmp.Or(carpeta, "root")}})
	h := textproto.MIMEHeader{"Content-Type": {"application/json; charset=UTF-8"}}
	w, _ := mw.CreatePart(h)
	_, _ = w.Write(meta)
	w, _ = mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"application/octet-stream"}})
	_, _ = w.Write(datos)
	_ = mw.Close()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cmp.Or(g.Subidas, subidasGoogle)+"/files?uploadType=multipart", &cuerpo)
	req.Header.Set("Content-Type", "multipart/related; boundary="+mw.Boundary())
	return g.respuestaFichero(ctx, req)
}

func (g *GoogleDrive) respuestaFichero(ctx context.Context, req *http.Request) (Elemento, error) {
	resp, err := g.do(ctx, req)
	if err != nil {
		return Elemento{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Elemento{}, errorDeEstado("Google Drive", resp)
	}
	var f struct{ ID, Name, MimeType, Version string }
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return Elemento{}, err
	}
	return Elemento{ID: f.ID, Nombre: f.Name, Tipo: f.MimeType, Version: f.Version, Carpeta: f.MimeType == carpetaGoogle, Modificado: time.Now(), Estable: f.ID}, nil
}

// Reemplazar sube datos encima del fichero e por su identificador (PATCH del contenido).
func (g *GoogleDrive) Reemplazar(ctx context.Context, e Elemento, datos []byte) (Elemento, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPatch, cmp.Or(g.Subidas, subidasGoogle)+"/files/"+url.PathEscape(e.ID)+"?uploadType=media&fields=id,name,mimeType,version", bytes.NewReader(datos))
	req.Header.Set("Content-Type", "application/octet-stream")
	return g.respuestaFichero(ctx, req)
}

const carpetaGoogle = "application/vnd.google-apps.folder"

var (
	_ Gestor       = (*GoogleDrive)(nil)
	_ Reemplazador = (*GoogleDrive)(nil)
)

// metadatos cambia los metadatos de un fichero (PATCH sin contenido).
func (g *GoogleDrive) metadatos(ctx context.Context, metodo, destino string, cuerpo map[string]any) (Elemento, error) {
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return Elemento{}, err
	}
	req, _ := http.NewRequestWithContext(ctx, metodo, destino, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	return g.respuestaFichero(ctx, req)
}

func (g *GoogleDrive) fichero(id string, extra url.Values) string {
	q := url.Values{"fields": {"id,name,mimeType,version"}}
	for k, v := range extra {
		q[k] = v
	}
	return cmp.Or(g.API, apiGoogle) + "/files/" + url.PathEscape(id) + "?" + q.Encode()
}

// CrearCarpeta crea una carpeta de Drive.
func (g *GoogleDrive) CrearCarpeta(ctx context.Context, padre, nombre string) (Elemento, error) {
	return g.metadatos(ctx, http.MethodPost, cmp.Or(g.API, apiGoogle)+"/files?fields=id,name,mimeType,version",
		map[string]any{"name": nombre, "mimeType": carpetaGoogle, "parents": []string{cmp.Or(padre, "root")}})
}

// Renombrar cambia el nombre.
func (g *GoogleDrive) Renombrar(ctx context.Context, e Elemento, nombre string) (Elemento, error) {
	return g.metadatos(ctx, http.MethodPatch, g.fichero(e.ID, nil), map[string]any{"name": nombre})
}

// Mover cambia de carpeta: se añade el destino y se quitan los padres que tenga.
func (g *GoogleDrive) Mover(ctx context.Context, e Elemento, destino string) (Elemento, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, g.fichero(e.ID, url.Values{"fields": {"parents"}}), http.NoBody)
	resp, err := g.do(ctx, req)
	if err != nil {
		return Elemento{}, err
	}
	var p struct {
		Parents []string `json:"parents"`
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return Elemento{}, errorDeEstado("Google Drive", resp)
	}
	err = json.NewDecoder(resp.Body).Decode(&p)
	resp.Body.Close()
	if err != nil {
		return Elemento{}, err
	}
	return g.metadatos(ctx, http.MethodPatch, g.fichero(e.ID, url.Values{"addParents": {cmp.Or(destino, "root")}, "removeParents": {strings.Join(p.Parents, ",")}}), map[string]any{})
}

// Borrar lo manda a la papelera de Drive (se puede recuperar desde la web de Google).
func (g *GoogleDrive) Borrar(ctx context.Context, e Elemento) error {
	_, err := g.metadatos(ctx, http.MethodPatch, g.fichero(e.ID, nil), map[string]any{"trashed": true})
	return err
}
