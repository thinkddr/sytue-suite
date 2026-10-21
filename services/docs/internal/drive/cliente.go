// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package drive habla con Sytue Drive para que un documento de Sytue Docs tenga
// un FICHERO REAL donde vive todo lo demás de la persona.
//
// POR QUÉ EXISTE. Hasta ahora un documento nacido en Docs no existía en ninguna
// parte fuera de Docs: ni se veía en la unidad, ni se descargaba desde donde la
// gente busca sus cosas, ni contaba en el espacio que la unidad dice que queda.
// Dos almacenes para una sola idea —«mis archivos»— y la persona en medio
// decidiendo en cuál mirar.
//
// SE PROPAGA EL JWT DE QUIEN PIDE, no un secreto de servicio. Es la misma
// decisión que tomó Sytue Rooms para resolver adjuntos: así Drive aplica SU
// autorización y SU RLS, y Docs no puede escribir en la unidad de nadie más allá
// de lo que esa persona ya podía hacer por su cuenta. Un secreto interno habría
// convertido a Docs en un escritor omnipotente sobre todas las unidades del
// tenant, que es exactamente la clase de puerta que nadie recuerda haber abierto.
//
// CLEAN-ROOM Y SIN DEPENDENCIAS NUEVAS: la API pública de Drive por HTTP, con la
// stdlib. Importar sus paquetes `internal` no está permitido y tampoco sería
// deseable: lo que une a los dos servicios es un contrato REST, no un tipo de Go.
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// CarpetaDocumentos es la carpeta donde aterrizan los documentos nacidos en Docs.
//
// UNA carpeta y no una por persona: en Drive «Mi unidad» es hoy un espacio de
// toda la organización —lo dice el propio Drive en channelfiles.go— así que una
// subcarpeta por sub daría la ilusión de privacidad sin darla. El fichero SÍ
// queda con su `owner_sub`, que es lo que de verdad se puede prometer hoy; el día
// que Drive tenga raíces personales, esto pasa a colgar de la raíz de cada uno y
// no cambia nada más de aquí.
const CarpetaDocumentos = "Sytue Docs"

var (
	// ErrCuota es la unidad llena. Se distingue del resto a propósito: es lo único
	// que quien crea el documento puede arreglar, y merece decírselo.
	ErrCuota = errors.New("drive: no queda espacio en la unidad")
	// ErrNoAutorizado es que el token de la persona no vale para Drive.
	ErrNoAutorizado = errors.New("drive: sin autorización")
	// ErrProhibido es un 403: la sesión vale, pero Drive no deja hacer ESO (un permiso
	// que no alcanza, o una etiqueta que no deja compartir fuera). Es un ErrNoAutorizado.
	ErrProhibido = fmt.Errorf("%w: prohibido", ErrNoAutorizado)
)

// Fichero es la vista mínima que Docs necesita de un fichero de Drive.
type Fichero struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	// Path es la ruta absoluta en la unidad; solo la trae [Cliente.Metadatos].
	Path string `json:"path,omitempty"`
}

// carpeta es la vista mínima de una carpeta de Drive.
type carpeta struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Cliente llama a la API REST de Sytue Drive en nombre de quien pide.
type Cliente struct {
	baseURL string
	http    *http.Client
	// secreto es el de la suite, para la única puerta interna que usa Docs
	// (papelera.go). Vacío, esa puerta rechaza y la papelera no llega a Drive.
	secreto string
}

// ConSecretoInterno devuelve el cliente con el secreto de la suite.
func (c *Cliente) ConSecretoInterno(secreto string) *Cliente {
	if c != nil {
		c.secreto = secreto
	}
	return c
}

// Nuevo construye el cliente. Con `baseURL` vacía devuelve NIL, y quien lo use
// tiene que comprobarlo: es lo que permite que una instalación sin Drive siga
// creando documentos como siempre en vez de no poder crear ninguno. Mismo patrón
// que el cliente de Sytue OCR.
func Nuevo(baseURL string, timeout time.Duration) *Cliente {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Cliente{baseURL: baseURL, http: &http.Client{Timeout: timeout}}
}

// maxRespuesta acota lo que se lee de vuelta. El listado de una carpeta son unos
// cientos de kilobytes en el peor caso; diez megas es un servidor descontrolado.
const maxRespuesta = 10 << 20

// pedir hace una llamada JSON a Drive con la identidad de quien pide y
// deserializa la respuesta en `dest` (nil para descartarla).
func (c *Cliente) pedir(ctx context.Context, metodo, ruta, token string, cuerpo, dest any) error {
	var body io.Reader = http.NoBody
	if cuerpo != nil {
		raw, err := json.Marshal(cuerpo)
		if err != nil {
			return fmt.Errorf("drive: serializar petición: %w", err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, c.baseURL+ruta, body)
	if err != nil {
		return fmt.Errorf("drive: construir petición: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if cuerpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.ejecutar(req, dest)
}

// ejecutar despacha la petición y traduce el estado a los errores de este paquete.
func (c *Cliente) ejecutar(req *http.Request, dest any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("drive: solicitud: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
	case http.StatusInsufficientStorage:
		// 507 es lo que responde Drive cuando la cuota del tenant no da más de sí.
		return ErrCuota
	case http.StatusUnauthorized:
		return ErrNoAutorizado
	case http.StatusForbidden:
		return ErrProhibido
	default:
		return fmt.Errorf("drive: respuesta %d", resp.StatusCode)
	}
	if dest == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRespuesta)).Decode(dest); err != nil {
		return fmt.Errorf("drive: decodificar: %w", err)
	}
	return nil
}

// hijos son las subcarpetas y ficheros de una carpeta, como los devuelve Drive.
type hijos struct {
	Folders []carpeta `json:"folders"`
	Files   []Fichero `json:"files"`
}

// asegurarCarpeta devuelve el id de «Sytue Docs», creándola la primera vez.
//
// NO se cachea el id: la carpeta se puede borrar, renombrar o mover desde Drive
// —es una carpeta normal de la persona, no un recurso oculto del sistema— y una
// caché convertiría eso en documentos que se crean contra una carpeta que ya no
// existe. Son dos llamadas por documento CREADO, no por pulsación de tecla.
func (c *Cliente) asegurarCarpeta(ctx context.Context, token string) (string, error) {
	var raiz carpeta
	if err := c.pedir(ctx, http.MethodGet, "/api/v1/folders/root", token, nil, &raiz); err != nil {
		return "", err
	}
	var h hijos
	if err := c.pedir(ctx, http.MethodGet, "/api/v1/folders/"+raiz.ID+"/children", token, nil, &h); err != nil {
		return "", err
	}
	for _, f := range h.Folders {
		// Drive compara nombres sin distinguir mayúsculas (lower(name) en sus
		// consultas), así que aquí se compara igual: si no, pedir «Sytue Docs»
		// junto a una «sytue docs» ya existente sería un conflicto en cada creación.
		if strings.EqualFold(f.Name, CarpetaDocumentos) {
			return f.ID, nil
		}
	}
	var nueva carpeta
	err := c.pedir(ctx, http.MethodPost, "/api/v1/folders", token,
		map[string]any{"parent_id": raiz.ID, "name": CarpetaDocumentos}, &nueva)
	if err != nil {
		return "", err
	}
	return nueva.ID, nil
}

// CrearDocumento deja el documento como fichero en la carpeta de Sytue Docs y
// devuelve el fichero creado.
//
// EL NOMBRE SE DESAMBIGUA, y no es un detalle cosmético: Drive trata subir con un
// nombre que ya existe como SUSTITUCIÓN del fichero (nueva versión del mismo id).
// Crear dos documentos «Sin título» seguidos —lo más normal del mundo— habría
// devuelto el MISMO id las dos veces, los dos documentos habrían compartido
// `source_ref` y el índice único de la migración 0006 habría hecho que el segundo
// «Nuevo documento» abriera el primero. Un documento que se come a otro es mucho
// peor que un nombre con un «(2)» detrás.
func (c *Cliente) CrearDocumento(ctx context.Context, token, titulo, ext, contentType string, datos []byte) (Fichero, error) {
	carpetaID, err := c.asegurarCarpeta(ctx, token)
	if err != nil {
		return Fichero{}, err
	}
	var h hijos
	if err := c.pedir(ctx, http.MethodGet, "/api/v1/folders/"+carpetaID+"/children", token, nil, &h); err != nil {
		return Fichero{}, err
	}
	return c.subir(ctx, token, carpetaID, NombreLibre(h.Files, titulo, ext), contentType, datos)
}

// subir manda los bytes a Drive. El nombre viaja en cabecera y no en el cuerpo
// porque el cuerpo SON los bytes del fichero: es el contrato de Drive.
func (c *Cliente) subir(ctx context.Context, token, carpetaID, nombre, contentType string, datos []byte) (Fichero, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/folders/"+carpetaID+"/files", bytes.NewReader(datos))
	if err != nil {
		return Fichero{}, fmt.Errorf("drive: construir subida: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-File-Name", nombre)
	req.ContentLength = int64(len(datos))

	var f Fichero
	if err := c.ejecutar(req, &f); err != nil {
		return Fichero{}, err
	}
	return f, nil
}

// Descargar trae el contenido de un fichero de Drive con la identidad de quien pide,
// hasta `tope` bytes: más que eso es un error, no un fichero cortado.
func (c *Cliente) Descargar(ctx context.Context, token, id string, tope int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/files/"+url.PathEscape(id)+"/content", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("drive: construir descarga: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("drive: descarga: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
		return nil, ErrNoAutorizado
	default:
		return nil, fmt.Errorf("drive: descarga: respuesta %d", resp.StatusCode)
	}
	datos, err := io.ReadAll(io.LimitReader(resp.Body, tope+1))
	if err == nil && int64(len(datos)) > tope {
		err = fmt.Errorf("drive: el fichero pasa de %d bytes", tope)
	}
	return datos, err
}

// Metadatos es el fichero `id` tal como lo ve quien pide: si no lo puede ver, error.
func (c *Cliente) Metadatos(ctx context.Context, token, id string) (Fichero, error) {
	var f Fichero
	err := c.pedir(ctx, http.MethodGet, "/api/v1/files/"+url.PathEscape(id), token, nil, &f)
	return f, err
}

// # Sacar un PDF a otra organización por un enlace (ADR 0057)
//
// Es lo que Drive ya tiene para que algo salga fuera: un enlace de solo lectura. Lo
// crea la persona dueña de la sesión, con SU permiso sobre el fichero y con la verja de
// etiquetas de Drive (una etiqueta que bloquea compartir fuera lo bloquea también aquí:
// [ErrProhibido]).

// CompartirEnlace crea un enlace de solo lectura al fichero `id` que caduca en `caduca`
// y devuelve el id del enlace y su token, que Drive enseña una sola vez.
func (c *Cliente) CompartirEnlace(ctx context.Context, token, id string, caduca time.Time) (enlaceID, tokenEnlace string, err error) {
	var out struct {
		Share struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"share"`
	}
	err = c.pedir(ctx, http.MethodPost, "/api/v1/shares", token, map[string]string{
		"file_id": id, "kind": "link", "permission": "read", "expires_at": caduca.UTC().Format(time.RFC3339),
	}, &out)
	if err != nil {
		return "", "", err
	}
	if out.Share.ID == "" || out.Share.Token == "" {
		return "", "", errors.New("drive: el enlace llegó sin id o sin token")
	}
	return out.Share.ID, out.Share.Token, nil
}

// BorrarFichero manda a la papelera el fichero `id` con la sesión de quien pide.
func (c *Cliente) BorrarFichero(ctx context.Context, token, id string) error {
	return c.pedir(ctx, http.MethodDelete, "/api/v1/files/"+url.PathEscape(id), token, nil, nil)
}

// QuitarEnlace revoca el enlace `enlaceID` con la sesión de quien lo creó.
func (c *Cliente) QuitarEnlace(ctx context.Context, token, enlaceID string) error {
	return c.pedir(ctx, http.MethodDelete, "/api/v1/shares/"+url.PathEscape(enlaceID), token, nil, nil)
}

// DescargarEnlace trae, SIN sesión, el fichero de un enlace, hasta `tope` bytes, y su
// nombre. Un enlace revocado, caducado o con contraseña es [ErrNoAutorizado].
func (c *Cliente) DescargarEnlace(ctx context.Context, tokenEnlace string, tope int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/public/"+url.PathEscape(tokenEnlace), http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("drive: construir descarga del enlace: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("drive: descarga del enlace: %w", err)
	}
	defer resp.Body.Close()
	// Un enlace de carpeta o con contraseña contesta con una página HTML y 200: solo
	// vale un fichero que se descarga.
	_, params, _ := mime.ParseMediaType(resp.Header.Get("Content-Disposition"))
	if resp.StatusCode != http.StatusOK || params["filename"] == "" {
		return nil, "", ErrNoAutorizado
	}
	datos, err := io.ReadAll(io.LimitReader(resp.Body, tope+1))
	if err == nil && int64(len(datos)) > tope {
		err = fmt.Errorf("drive: el fichero pasa de %d bytes", tope)
	}
	return datos, params["filename"], err
}

// GuardarPDF deja un PDF en la carpeta de Sytue Docs de quien pide, con un nombre
// libre, y devuelve el fichero con su ruta.
func (c *Cliente) GuardarPDF(ctx context.Context, token, nombre string, datos []byte) (Fichero, error) {
	titulo := strings.TrimSuffix(nombre, path.Ext(nombre))
	f, err := c.CrearDocumento(ctx, token, titulo, "pdf", "application/pdf", datos)
	if err != nil {
		return Fichero{}, err
	}
	return c.Metadatos(ctx, token, f.ID)
}
