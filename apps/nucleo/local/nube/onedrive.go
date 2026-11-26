// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OneDriveGraph habla con OneDrive (personal y de Microsoft 365) por Microsoft Graph.
type OneDriveGraph struct {
	Token func(context.Context) (string, error)
	HTTP  *http.Client
	// API se cambia en pruebas.
	API string
}

const apiGraph = "https://graph.microsoft.com/v1.0"

// Subidas sencillas de Graph: hasta 250 MB (más necesitaría una sesión de subida).
const maxSubidaGraph = 250 << 20

func (o *OneDriveGraph) do(ctx context.Context, metodo, ruta string, cuerpo []byte) (*http.Response, error) {
	t, err := o.Token(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, metodo, cmp.Or(o.API, apiGraph)+ruta, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t)
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/octet-stream")
	}
	return o.HTTP.Do(req)
}

// itemGraph es un driveItem de Microsoft Graph: los nombres JSON son los de su API, no
// los nuestros.
type itemGraph struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	ETag                 string    `json:"eTag"` //nolint:tagliatelle // contrato de Microsoft Graph
	Size                 int64     `json:"size"`
	LastModifiedDateTime time.Time `json:"lastModifiedDateTime"` //nolint:tagliatelle // contrato de Microsoft Graph
	CreatedDateTime      time.Time `json:"createdDateTime"`      //nolint:tagliatelle // contrato de Microsoft Graph
	Folder               *struct{} `json:"folder"`
	File                 *struct {
		MimeType string `json:"mimeType"` //nolint:tagliatelle // contrato de Microsoft Graph
		// Hashes: OneDrive personal da sha1Hash; el de empresa, solo quickXorHash, que
		// aquí no se calcula (la copia se comprueba volviéndola a leer).
		Hashes struct {
			Sha1Hash string `json:"sha1Hash"` //nolint:tagliatelle // contrato de Microsoft Graph
		} `json:"hashes"`
	} `json:"file"`
}

func (i itemGraph) elemento() Elemento {
	e := Elemento{
		ID: i.ID, Nombre: i.Name, Tamano: i.Size, Modificado: i.LastModifiedDateTime, Creado: i.CreatedDateTime, Carpeta: i.Folder != nil,
		Version: i.ETag, Estable: i.ID,
	}
	if i.File != nil {
		e.Tipo = i.File.MimeType
		if h := i.File.Hashes.Sha1Hash; h != "" {
			e.Huella = "sha1:" + strings.ToLower(h)
		}
	}
	return e
}

// Listar pide los hijos de la carpeta ("" es la raíz de «Mis archivos»).
func (o *OneDriveGraph) Listar(ctx context.Context, carpeta string) ([]Elemento, error) {
	ruta := "/me/drive/root/children?$top=200"
	if carpeta != "" {
		ruta = "/me/drive/items/" + url.PathEscape(carpeta) + "/children?$top=200"
	}
	var out []Elemento
	for ruta != "" {
		resp, err := o.do(ctx, http.MethodGet, ruta, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			err := errorDeEstado("OneDrive", resp)
			_ = resp.Body.Close()
			return nil, err
		}
		var r struct {
			Value []itemGraph `json:"value"`
			Next  string      `json:"@odata.nextLink"` //nolint:tagliatelle // contrato de Microsoft Graph (OData)
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, i := range r.Value {
			out = append(out, i.elemento())
		}
		ruta = ""
		if r.Next != "" {
			// El enlace siguiente es absoluto; se queda solo si es del mismo API.
			if u, err := url.Parse(r.Next); err == nil {
				base, _ := url.Parse(cmp.Or(o.API, apiGraph))
				if u.Host == base.Host {
					ruta = u.RequestURI()[len(base.Path):]
				}
			}
		}
	}
	ordenar(out)
	return out, nil
}

// Descargar baja el contenido (Graph redirige a la descarga, que el cliente sigue).
func (o *OneDriveGraph) Descargar(ctx context.Context, e Elemento) ([]byte, string, error) {
	resp, err := o.do(ctx, http.MethodGet, "/me/drive/items/"+url.PathEscape(e.ID)+"/content", nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errorDeEstado("OneDrive", resp)
	}
	datos, err := leerAcotado(resp.Body)
	return datos, e.Nombre, err
}

// Subir crea o reemplaza carpeta/nombre. En OneDrive el nombre es único en su carpeta
// (sin distinguir mayúsculas): no hay dos que se llamen igual.
func (o *OneDriveGraph) Subir(ctx context.Context, carpeta, nombre string, datos []byte) (Elemento, error) {
	ruta := "/me/drive/root:/" + url.PathEscape(nombre) + ":/content"
	if carpeta != "" {
		ruta = "/me/drive/items/" + url.PathEscape(carpeta) + ":/" + url.PathEscape(nombre) + ":/content"
	}
	return o.subirA(ctx, ruta, datos)
}

// Reemplazar sube datos encima del fichero e por su identificador.
func (o *OneDriveGraph) Reemplazar(ctx context.Context, e Elemento, datos []byte) (Elemento, error) {
	return o.subirA(ctx, rutaItem(e.ID)+"/content", datos)
}

// subirA hace la subida sencilla (PUT del contenido) a ruta.
func (o *OneDriveGraph) subirA(ctx context.Context, ruta string, datos []byte) (Elemento, error) {
	if len(datos) > maxSubidaGraph {
		return Elemento{}, errorTamano
	}
	resp, err := o.do(ctx, http.MethodPut, ruta, datos)
	if err != nil {
		return Elemento{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Elemento{}, errorDeEstado("OneDrive", resp)
	}
	var i itemGraph
	if err := json.NewDecoder(resp.Body).Decode(&i); err != nil {
		return Elemento{}, err
	}
	return i.elemento(), nil
}

var (
	_ Gestor       = (*OneDriveGraph)(nil)
	_ Reemplazador = (*OneDriveGraph)(nil)
)

// json manda un cuerpo JSON y decodifica el elemento que devuelve Graph.
func (o *OneDriveGraph) json(ctx context.Context, metodo, ruta string, cuerpo any) (Elemento, error) {
	raw, err := json.Marshal(cuerpo)
	if err != nil {
		return Elemento{}, err
	}
	t, err := o.Token(ctx)
	if err != nil {
		return Elemento{}, err
	}
	req, err := http.NewRequestWithContext(ctx, metodo, cmp.Or(o.API, apiGraph)+ruta, bytes.NewReader(raw))
	if err != nil {
		return Elemento{}, err
	}
	req.Header.Set("Authorization", "Bearer "+t)
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return Elemento{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return Elemento{}, errorDeEstado("OneDrive", resp)
	}
	var i itemGraph
	if err := json.NewDecoder(resp.Body).Decode(&i); err != nil {
		return Elemento{}, err
	}
	return i.elemento(), nil
}

func rutaItem(id string) string {
	if id == "" {
		return "/me/drive/root"
	}
	return "/me/drive/items/" + url.PathEscape(id)
}

// CrearCarpeta crea una carpeta; si el nombre ya existe, falla (no renombra solo).
func (o *OneDriveGraph) CrearCarpeta(ctx context.Context, padre, nombre string) (Elemento, error) {
	return o.json(ctx, http.MethodPost, rutaItem(padre)+"/children",
		map[string]any{"name": nombre, "folder": map[string]any{}, "@microsoft.graph.conflictBehavior": "fail"})
}

// Renombrar cambia el nombre.
func (o *OneDriveGraph) Renombrar(ctx context.Context, e Elemento, nombre string) (Elemento, error) {
	return o.json(ctx, http.MethodPatch, rutaItem(e.ID), map[string]any{"name": nombre})
}

// Mover cambia la carpeta padre.
func (o *OneDriveGraph) Mover(ctx context.Context, e Elemento, destino string) (Elemento, error) {
	padre := map[string]any{"id": destino}
	if destino == "" {
		padre = map[string]any{"path": "/drive/root"}
	}
	return o.json(ctx, http.MethodPatch, rutaItem(e.ID), map[string]any{"parentReference": padre})
}

// Borrar lo manda a la papelera de reciclaje de OneDrive.
func (o *OneDriveGraph) Borrar(ctx context.Context, e Elemento) error {
	resp, err := o.do(ctx, http.MethodDelete, rutaItem(e.ID), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return errorDeEstado("OneDrive", resp)
	}
	return nil
}
