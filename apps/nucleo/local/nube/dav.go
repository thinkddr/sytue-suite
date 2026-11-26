// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package nube

import (
	"bytes"
	"cmp"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// DAV es una cuenta WebDAV (RFC 4918): Sytue Drive con contraseña de aplicación,
// Nextcloud, ownCloud… Los ID son rutas relativas a Base.
type DAV struct {
	Base, Usuario, Clave string
	HTTP                 *http.Client
}

const maxDescarga = 100 << 20

func (d *DAV) url(ruta string) string {
	base := strings.TrimRight(d.Base, "/")
	partes := strings.Split(strings.Trim(ruta, "/"), "/")
	for i, p := range partes {
		partes[i] = url.PathEscape(p)
	}
	if ruta == "" || ruta == "/" {
		return base + "/"
	}
	return base + "/" + strings.Join(partes, "/")
}

func (d *DAV) peticion(ctx context.Context, metodo, destino string, cuerpo []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, metodo, destino, bytes.NewReader(cuerpo))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(d.Usuario, d.Clave)
	return d.HTTP.Do(req)
}

type multistatus struct {
	Respuestas []struct {
		Href  string `xml:"href"`
		Props []struct {
			Prop   propDAV `xml:"prop"`
			Estado string  `xml:"status"`
		} `xml:"propstat"`
	} `xml:"response"`
}

// propDAV son las propiedades de un recurso en la respuesta de PROPFIND.
type propDAV struct {
	Nombre     string `xml:"displayname"`
	Tamano     string `xml:"getcontentlength"`
	Modificado string `xml:"getlastmodified"`
	Tipo       string `xml:"getcontenttype"`
	Etiqueta   string `xml:"getetag"`
	Creado     string `xml:"creationdate"`
	// Fichero es el `oc:fileid` de Nextcloud y ownCloud: no cambia al renombrar.
	Fichero string `xml:"http://owncloud.org/ns fileid"`
	Recurso struct {
		Coleccion *struct{} `xml:"collection"`
	} `xml:"resourcetype"`
}

// aplicarProp pone en e lo que dice p; lo que p no trae, no lo toca.
func aplicarProp(e *Elemento, p propDAV) {
	if p.Nombre != "" {
		e.Nombre = p.Nombre
	}
	e.Carpeta = e.Carpeta || p.Recurso.Coleccion != nil
	if n, err := strconv.ParseInt(p.Tamano, 10, 64); err == nil {
		e.Tamano = n
	}
	if t, err := http.ParseTime(p.Modificado); err == nil {
		e.Modificado = t
	}
	if p.Tipo != "" {
		e.Tipo = p.Tipo
	}
	if p.Etiqueta != "" {
		e.Version = p.Etiqueta
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(p.Creado)); err == nil {
		e.Creado = t
	}
	if f := strings.TrimSpace(p.Fichero); f != "" {
		e.Estable = "oc:" + f
	}
}

// propfind pide también `oc:fileid`: un servidor que no lo conoce lo contesta aparte con 404,
// y el resto llega igual.
const propfind = `<?xml version="1.0" encoding="utf-8"?><d:propfind xmlns:d="DAV:" xmlns:oc="http://owncloud.org/ns"><d:prop><d:displayname/><d:getcontentlength/><d:getlastmodified/><d:getcontenttype/><d:getetag/><d:creationdate/><d:resourcetype/><oc:fileid/></d:prop></d:propfind>`

// Listar hace PROPFIND con Depth 1.
func (d *DAV) Listar(ctx context.Context, carpeta string) ([]Elemento, error) {
	destino := d.url(carpeta)
	if !strings.HasSuffix(destino, "/") {
		destino += "/"
	}
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", destino, strings.NewReader(propfind))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(d.Usuario, d.Clave)
	req.Header.Set("Depth", "1")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus {
		return nil, errorDeEstado("WebDAV", resp)
	}
	var ms multistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&ms); err != nil {
		return nil, err
	}
	base, _ := url.Parse(strings.TrimRight(d.Base, "/") + "/")
	propia, _ := url.Parse(destino)
	var out []Elemento
	for _, r := range ms.Respuestas {
		h, err := url.Parse(r.Href)
		if err != nil {
			continue
		}
		abs := propia.ResolveReference(h)
		if strings.TrimRight(abs.Path, "/") == strings.TrimRight(propia.Path, "/") {
			continue // la propia carpeta
		}
		rel := strings.TrimPrefix(abs.Path, base.Path)
		e := Elemento{ID: "/" + strings.Trim(rel, "/"), Nombre: path.Base(strings.TrimRight(abs.Path, "/"))}
		if n, err := url.PathUnescape(e.Nombre); err == nil {
			e.Nombre = n
		}
		if id, err := url.PathUnescape(e.ID); err == nil {
			e.ID = id
		}
		for _, ps := range r.Props {
			if ps.Estado != "" && !strings.Contains(ps.Estado, " 200 ") {
				continue
			}
			aplicarProp(&e, ps.Prop)
		}
		out = append(out, e)
	}
	ordenar(out)
	return out, nil
}

// Descargar hace GET.
func (d *DAV) Descargar(ctx context.Context, e Elemento) ([]byte, string, error) {
	resp, err := d.peticion(ctx, http.MethodGet, d.url(e.ID), nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errorDeEstado("WebDAV", resp)
	}
	datos, err := leerAcotado(resp.Body)
	return datos, e.Nombre, err
}

// Subir hace PUT en carpeta/nombre.
func (d *DAV) Subir(ctx context.Context, carpeta, nombre string, datos []byte) (Elemento, error) {
	return d.poner(ctx, strings.TrimRight(carpeta, "/")+"/"+nombre, nombre, datos)
}

// Reemplazar hace PUT en la ruta de e (su ID). Su nombre en la lista puede ser el
// `displayname` que da el servidor, que no es por fuerza el de la ruta, y dos recursos
// pueden dar el mismo: subir por el nombre crearía otro o pisaría el que no es.
func (d *DAV) Reemplazar(ctx context.Context, e Elemento, datos []byte) (Elemento, error) {
	return d.poner(ctx, e.ID, cmp.Or(e.Nombre, path.Base(e.ID)), datos)
}

func (d *DAV) poner(ctx context.Context, ruta, nombre string, datos []byte) (Elemento, error) {
	resp, err := d.peticion(ctx, http.MethodPut, d.url(ruta), datos)
	if err != nil {
		return Elemento{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return Elemento{}, errorDeEstado("WebDAV", resp)
	}
	return Elemento{ID: ruta, Nombre: nombre, Tamano: int64(len(datos)), Modificado: time.Now(), Version: resp.Header.Get("ETag")}, nil
}

var (
	_ Gestor       = (*DAV)(nil)
	_ Reemplazador = (*DAV)(nil)
)

// CrearCarpeta hace MKCOL.
func (d *DAV) CrearCarpeta(ctx context.Context, padre, nombre string) (Elemento, error) {
	ruta := strings.TrimRight(padre, "/") + "/" + nombre
	resp, err := d.peticion(ctx, "MKCOL", d.url(ruta)+"/", nil)
	if err != nil {
		return Elemento{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return Elemento{}, errorDeEstado("WebDAV", resp)
	}
	return Elemento{ID: ruta, Nombre: nombre, Carpeta: true, Modificado: time.Now()}, nil
}

// mover hace MOVE de e a destino sin pisar lo que haya (`Overwrite: F`).
func (d *DAV) mover(ctx context.Context, e Elemento, destino string) (Elemento, error) {
	origen, dst := d.url(e.ID), d.url(destino)
	if e.Carpeta {
		origen, dst = origen+"/", dst+"/"
	}
	req, err := http.NewRequestWithContext(ctx, "MOVE", origen, http.NoBody)
	if err != nil {
		return Elemento{}, err
	}
	req.SetBasicAuth(d.Usuario, d.Clave)
	req.Header.Set("Destination", dst)
	req.Header.Set("Overwrite", "F")
	resp, err := d.HTTP.Do(req)
	if err != nil {
		return Elemento{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		if resp.StatusCode == http.StatusPreconditionFailed {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
			return Elemento{}, fmt.Errorf("nube: ya hay algo con ese nombre")
		}
		return Elemento{}, errorDeEstado("WebDAV", resp)
	}
	e.ID, e.Nombre = destino, path.Base(destino)
	return e, nil
}

// Renombrar mueve e dentro de su carpeta.
func (d *DAV) Renombrar(ctx context.Context, e Elemento, nombre string) (Elemento, error) {
	return d.mover(ctx, e, path.Join(path.Dir("/"+strings.Trim(e.ID, "/")), nombre))
}

// Mover lleva e a la carpeta destino.
func (d *DAV) Mover(ctx context.Context, e Elemento, destino string) (Elemento, error) {
	return d.mover(ctx, e, strings.TrimRight(destino, "/")+"/"+path.Base("/"+strings.Trim(e.ID, "/")))
}

// Borrar hace DELETE (WebDAV no tiene papelera estándar).
func (d *DAV) Borrar(ctx context.Context, e Elemento) error {
	destino := d.url(e.ID)
	if e.Carpeta {
		destino += "/"
	}
	resp, err := d.peticion(ctx, http.MethodDelete, destino, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return errorDeEstado("WebDAV", resp)
	}
	return nil
}

func leerAcotado(r io.Reader) ([]byte, error) {
	datos, err := io.ReadAll(io.LimitReader(r, maxDescarga+1))
	if err == nil && len(datos) > maxDescarga {
		return nil, fmt.Errorf("nube: el fichero supera los %d MiB", maxDescarga>>20)
	}
	return datos, err
}

// errorDeEstado no copia el cuerpo: puede llevar datos de la cuenta.
func errorDeEstado(quien string, resp *http.Response) error {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("nube: %s rechazó las credenciales (%d); vuelve a conectar la cuenta", quien, resp.StatusCode)
	case http.StatusNotFound:
		return fmt.Errorf("nube: %s no encuentra ese elemento", quien)
	}
	return fmt.Errorf("nube: %s respondió %d", quien, resp.StatusCode)
}
