// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cuota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Errores de la lectura de un fichero de Drive.
var (
	// ErrFicheroNoExiste: Drive dice 404 (no existe, está en la papelera o la
	// persona no lo ve: desde fuera es lo mismo).
	ErrFicheroNoExiste = errors.New("cuota: el fichero de Drive no existe")
	// ErrFicheroProhibido: Drive dice 403 (lo ve, pero no puede leerlo).
	ErrFicheroProhibido = errors.New("cuota: sin permiso sobre el fichero de Drive")
)

// Fichero son los metadatos de un fichero de Drive (`GET /api/v1/files/{id}`,
// que responde en snake_case como toda la API de Drive).
type Fichero struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	SizeBytes   int64      `json:"size_bytes"`
	ContentType string     `json:"content_type"`
	DeletedAt   *time.Time `json:"deleted_at"`
}

// clienteDeDescargas es el cliente de los ficheros: sin plazo TOTAL (el cuerpo
// dura lo que dure el fichero) pero con plazo para conectar y para que llegue la
// cabecera, que es donde se nota un Drive o un almacén que no contesta. Las
// redirecciones no se siguen solas: ver Abrir.
func clienteDeDescargas() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConns:          16,
			IdleConnTimeout:       90 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// errorDeRespuesta traduce lo que Drive contestó cuando no es un 2xx.
func errorDeRespuesta(que string, codigo int) error {
	switch {
	case codigo == http.StatusNotFound:
		return ErrFicheroNoExiste
	case codigo == http.StatusForbidden:
		return ErrFicheroProhibido
	case codigo >= 500:
		return fmt.Errorf("%w: %s: respuesta %d", ErrNoDisponible, que, codigo)
	}
	return fmt.Errorf("%w: %s: respuesta %d", ErrRechazado, que, codigo)
}

// Fichero lee los metadatos de un fichero de Drive con el token de la persona:
// Drive aplica su ACL, así que quien no lo ve recibe ErrFicheroNoExiste.
func (d *Drive) Fichero(ctx context.Context, bearer, id string) (Fichero, error) {
	if bearer == "" {
		return Fichero{}, ErrSinSesion
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/api/v1/files/"+url.PathEscape(id), http.NoBody)
	if err != nil {
		return Fichero{}, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return Fichero{}, fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Fichero{}, errorDeRespuesta("metadatos", resp.StatusCode)
	}
	var f Fichero
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&f); err != nil {
		return Fichero{}, fmt.Errorf("%w: metadatos ilegibles: %w", ErrNoDisponible, err)
	}
	// Uno en la papelera no se sube: para quien lo mira ya no está.
	if f.DeletedAt != nil {
		return Fichero{}, ErrFicheroNoExiste
	}
	return f, nil
}

// Abrir abre el contenido de un fichero de Drive para leerlo en streaming y
// devuelve el cuerpo (quien llama lo cierra) y su tamaño exacto.
//
// EN DOS SALTOS Y A MANO, como hace Fotos al leer sus originales:
//
//  1. `GET /api/v1/files/{id}/content?presign=1` con el bearer de la persona.
//     Drive comprueba el permiso y contesta 307 a una URL prefirmada de su
//     almacén de objetos.
//  2. `GET <esa URL>` SIN cabecera de autorización: la URL lleva su propia
//     credencial y el token de la persona no tiene nada que hacer allí.
//
// El segundo salto es lo que hace posible un fichero de gigas: Drive monta su API
// con un plazo de 60 s por petición, y una descarga larga a través de Drive se
// cortaría a la mitad; el almacén de objetos no tiene ese plazo. Si Drive no sabe
// firmar URLs (una organización con almacén WebDAV) contesta 200 con el fichero
// y se lee de ahí. ponytail: por ese camino un fichero que tarde más de 60 s en
// bajar se corta y el vídeo queda `failed` con el motivo; el relevo es que Drive
// saque la descarga de su plazo global, como ya hace Cast con la subida.
//
// `ctx` gobierna TODA la lectura, no solo la petición: quien llama lo desliga de
// la petición HTTP si el cuerpo se va a leer después de contestar.
func (d *Drive) Abrir(ctx context.Context, bearer, id string) (io.ReadCloser, int64, error) {
	if bearer == "" {
		return nil, 0, ErrSinSesion
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		d.baseURL+"/api/v1/files/"+url.PathEscape(id)+"/content?presign=1", http.NoBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+bearer)
	resp, err := d.descargas.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return cuerpoConTamano(resp)
	case resp.StatusCode >= 300 && resp.StatusCode < 400:
		loc := resp.Header.Get("Location")
		_ = resp.Body.Close()
		return d.abrirPrefirmada(ctx, loc)
	default:
		_ = resp.Body.Close()
		return nil, 0, errorDeRespuesta("contenido", resp.StatusCode)
	}
}

// abrirPrefirmada da el segundo salto. La URL viene de Drive, que es de la casa,
// pero se valida igual: una Location relativa o con otro esquema convertiría el
// salto en algo que no es una descarga.
func (d *Drive) abrirPrefirmada(ctx context.Context, loc string) (io.ReadCloser, int64, error) {
	u, err := url.Parse(loc)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, 0, fmt.Errorf("%w: Location no utilizable: %q", ErrNoDisponible, loc)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, 0, err
	}
	resp, err := d.descargas.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: almacén de Drive: %w", ErrNoDisponible, err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		// Una URL prefirmada que no sirve (caducada, borrada entre los dos saltos)
		// es un fallo de Drive, no de quien pregunta.
		return nil, 0, fmt.Errorf("%w: almacén de Drive: respuesta %d", ErrNoDisponible, resp.StatusCode)
	}
	return cuerpoConTamano(resp)
}

// cuerpoConTamano exige el Content-Length: con él se comprueba el tope y la cuota
// ANTES de copiar un byte, y se sabe si la copia llegó entera.
func cuerpoConTamano(resp *http.Response) (io.ReadCloser, int64, error) {
	if resp.ContentLength <= 0 {
		_ = resp.Body.Close()
		return nil, 0, fmt.Errorf("%w: el contenido llegó sin Content-Length", ErrNoDisponible)
	}
	return resp.Body, resp.ContentLength, nil
}
