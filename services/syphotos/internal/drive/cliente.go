// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package drive habla con Sytue Drive para leer los originales de las fotos.
//
// Sytue Photos NO guarda los originales: viven en la unidad de cada persona, con su
// cuota, su papelera, su compartición y su copia de seguridad. Este paquete es
// todo lo que hace falta para leerlos.
//
// POR QUÉ NO SE REUTILIZA EL CLIENTE DE DOCS. `services/docs/internal/drive`
// existe, pero hace otra cosa: asegura la carpeta «Sytue Docs/» y sube
// documentos. No lee, y menos por rango. Compartirlo obligaría a una interfaz
// con dos implementaciones que no comparten ni una operación; son cien líneas
// de cliente HTTP, y copiar el ESTILO cuesta menos que mantener una abstracción
// que no abstrae nada.
package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNoEncontrado se devuelve cuando el fichero no existe o la persona no lo ve.
// Los dos casos son el mismo desde fuera y distinguirlos filtraría qué existe.
var ErrNoEncontrado = errors.New("drive: fichero no encontrado")

// rutaFicheros es el prefijo REAL de la API de Drive, que monta todo bajo
// `/api/v1`. Sin el `/v1` las tres llamadas caían en 404, que aquí se traduce a
// ErrNoEncontrado: subir una foto devolvía 500 en producción con «drive:
// fichero no encontrado», con el original ya guardado en el Drive de la persona.
const rutaFicheros = "/api/v1/files/"

// Fichero son los metadatos que Drive devuelve de un fichero.
//
// Las etiquetas son las de `domain.File` de Drive, en snake_case como toda su
// API. Estuvieron en camelCase: el JSON no casaba, Go lo ignoraba sin error y
// cada foto se indexaba con tipo vacío y tamaño 0.
type Fichero struct {
	ID          string `json:"id"`
	Nombre      string `json:"name"`
	ContentType string `json:"content_type"`
	Bytes       int64  `json:"size_bytes"`
	// Duenyo es de quién es el fichero en Drive (`owner_sub`).
	Duenyo string `json:"owner_sub"`
}

// Cliente habla con la API de Drive en nombre de una persona (bearer OIDC).
type Cliente struct {
	base string
	http *http.Client
	// sinSeguir no sigue redirecciones. Es lo que permite leer por rango: la
	// descarga de Drive contesta 307 a una URL prefirmada del almacén de
	// objetos, y ese segundo salto hay que darlo A MANO — ver Cabecera.
	sinSeguir *http.Client
}

// Nuevo construye el cliente.
func Nuevo(baseURL string, timeout time.Duration) *Cliente {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Cliente{
		base: strings.TrimSuffix(baseURL, "/"),
		http: &http.Client{Timeout: timeout},
		sinSeguir: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Metadatos devuelve nombre, tipo y tamaño de un fichero.
func (c *Cliente) Metadatos(ctx context.Context, token, id string) (Fichero, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+rutaFicheros+url.PathEscape(id), http.NoBody)
	if err != nil {
		return Fichero{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return Fichero{}, fmt.Errorf("drive: pedir metadatos: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusForbidden {
		return Fichero{}, ErrNoEncontrado
	}
	if res.StatusCode != http.StatusOK {
		return Fichero{}, fmt.Errorf("drive: metadatos: HTTP %d", res.StatusCode)
	}
	var f Fichero
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&f); err != nil {
		return Fichero{}, fmt.Errorf("drive: metadatos ilegibles: %w", err)
	}
	return f, nil
}

// Cabecera lee los PRIMEROS `n` bytes del original.
//
// ES LA OPERACIÓN QUE MÁS AHORRA DE TODO EL SERVICIO. El EXIF vive en los
// primeros KiB del fichero, así que indexar una foto de 25 MB solo necesita
// leer 128 KiB: dos órdenes de magnitud menos de datos por la red, y por foto.
//
// EN DOS SALTOS Y A MANO, que es la parte delicada:
//
//  1. `GET /api/files/<id>/content?presign=1` con el bearer de la persona.
//     Drive comprueba el permiso y contesta 307 a una URL prefirmada del
//     almacén de objetos.
//  2. `GET <esa URL>` con `Range`, SIN cabecera de autorización.
//
// El segundo salto se da a mano en vez de dejar que el cliente HTTP siga la
// redirección solo. Go retira la cabecera `Authorization` al redirigir a otro
// host, así que automáticamente también funcionaría — pero eso es una sutileza
// del cliente que nadie leyendo este fichero puede comprobar, y de la que
// depende no filtrar el token de una persona a un almacén de objetos. Aquí se
// ve que no se envía.
//
// Y el rango va en el SEGUNDO salto porque el primero no lo atiende:
// `DownloadFile` escribe el cuerpo con `io.Copy` y un `Content-Length`, sin
// `Accept-Ranges`. Es el almacén de objetos quien sabe servir rangos.
func (c *Cliente) Cabecera(ctx context.Context, token, id string, n int64) ([]byte, error) {
	if n <= 0 {
		return nil, fmt.Errorf("drive: cabecera de %d bytes", n)
	}
	destino, err := c.urlPrefirmada(ctx, token, id)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, http.NoBody)
	if err != nil {
		return nil, err
	}
	// Sin Authorization: la URL prefirmada lleva su propia credencial en la
	// query, y el token de la persona no tiene nada que hacer ahí.
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", n-1))

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive: leer cabecera: %w", err)
	}
	defer res.Body.Close()
	// 206 es lo esperado; 200 significa que el almacén ignoró el rango y manda
	// el fichero entero. No es un error —se corta al leer— pero conviene que el
	// corte lo haga el LimitReader y no la confianza en el servidor.
	if res.StatusCode != http.StatusPartialContent && res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("drive: leer cabecera: HTTP %d", res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, n))
}

// Abrir devuelve el original COMPLETO. Se usa para generar las derivadas, que es
// lo único que necesita todos los píxeles. Quien llame cierra.
func (c *Cliente) Abrir(ctx context.Context, token, id string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+rutaFicheros+url.PathEscape(id)+"/content", http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive: abrir: %w", err)
	}
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusForbidden {
		res.Body.Close()
		return nil, ErrNoEncontrado
	}
	if res.StatusCode != http.StatusOK {
		res.Body.Close()
		return nil, fmt.Errorf("drive: abrir: HTTP %d", res.StatusCode)
	}
	return res.Body, nil
}

// urlPrefirmada hace el primer salto y devuelve el destino de la redirección.
func (c *Cliente) urlPrefirmada(ctx context.Context, token, id string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+rutaFicheros+url.PathEscape(id)+"/content?presign=1", http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	res, err := c.sinSeguir.Do(req)
	if err != nil {
		return "", fmt.Errorf("drive: prefirmar: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusForbidden {
		return "", ErrNoEncontrado
	}
	if res.StatusCode < 300 || res.StatusCode > 399 {
		return "", fmt.Errorf("drive: se esperaba una redirección a la URL prefirmada, llegó HTTP %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if loc == "" {
		return "", errors.New("drive: redirección sin Location")
	}
	// La URL viene de Drive, que es de la casa, pero se valida igual: una
	// Location relativa o con otro esquema convertiría el segundo salto en algo
	// que no es una descarga.
	u, err := url.Parse(loc)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("drive: Location no utilizable: %q", loc)
	}
	return u.String(), nil
}

// ErrSinCuota es que la copia no cabe: el almacenamiento de la organización está lleno.
var ErrSinCuota = errors.New("drive: cuota de almacenamiento superada")

// rutaCopiaFotos es `RutaCopiaFotos` de Drive: de servicio a servicio, por dentro del
// clúster y con el secreto de la suite.
const rutaCopiaFotos = "/internal/photos/copies"

// CopiarParaDuenya deja en el Drive de `duenya` (organización `tenant`) una copia del
// fichero `id`: la foto que alguien con edición añade a un álbum de ella. Drive no la
// repite si ya tiene una igual (`reusada`) y la cobra de la cuota de la organización;
// si no cabe, ErrSinCuota.
//
// `editora` es quien la añade: Drive solo copia un fichero que sea SUYO.
func (c *Cliente) CopiarParaDuenya(ctx context.Context, secreto, tenant, id, duenya, editora string) (copia string, reusada bool, err error) {
	cuerpo, err := json.Marshal(map[string]string{"tenant": tenant, "file_id": id, "owner_sub": duenya, "editor_sub": editora})
	if err != nil {
		return "", false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+rutaCopiaFotos, strings.NewReader(string(cuerpo)))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+secreto)
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("drive: copiar para la dueña: %w", err)
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusInsufficientStorage:
		return "", false, ErrSinCuota
	case http.StatusNotFound:
		return "", false, ErrNoEncontrado
	default:
		return "", false, fmt.Errorf("drive: copiar para la dueña: HTTP %d", res.StatusCode)
	}
	var r struct {
		File struct {
			ID string `json:"id"`
		} `json:"file"`
		Reused bool `json:"reused"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&r); err != nil || r.File.ID == "" {
		return "", false, fmt.Errorf("drive: copia ilegible: %w", err)
	}
	return r.File.ID, r.Reused, nil
}
