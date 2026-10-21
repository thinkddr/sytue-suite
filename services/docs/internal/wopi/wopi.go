// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package wopi es el lado CLIENTE del protocolo WOPI en Sytue Docs: lo que
// convierte al editor de la casa en la aplicación que abre los ficheros que
// custodia Sytue Drive.
//
// POR QUÉ EXISTE. Drive habla WOPI como HOST desde la migración 0016 —sirve
// CheckFileInfo, GetFile, PutFile y los bloqueos—, pero un host sin cliente no
// abre nada: hace falta una aplicación que anuncie en un `discovery.xml` con qué
// URL se abre cada extensión. Esa aplicación era hasta hoy un tercero, y en
// producción `WOPI_DISCOVERY_URL` está VACÍA, así que «abrir en el editor»
// sencillamente no funcionaba. Con esto la aplicación es Sytue Docs.
//
// SOBRE EL ORIGEN. WOPI es una especificación de interoperabilidad publicada por
// Microsoft; implementarla es lo mismo que implementar IMAP o CalDAV. La
// arquitectura —descubrimiento por catálogo, acciones `view`/`edit` por
// extensión, `WOPISrc` + token de acceso por formulario— se ha tomado de la
// ESPECIFICACIÓN y de cómo la usan las suites que ya la hablan. No hay una sola
// línea portada de ningún motor: la casa no incorpora código AGPL (ver
// docs/CONVENTIONS.md §0.1) y este paquete es obra original sobre net/http.
//
// LA VENTAJA SOBRE ABRIR-E-IMPORTAR. La otra vía para ver un .docx de Drive era
// convertirlo y CREAR un documento en Docs. Eso duplica: abrir diez ficheros
// para saber cuál era deja diez documentos que nadie quería, y el original y la
// copia empiezan a divergir desde el primer minuto. Por WOPI el fichero se lee
// donde vive, se edita donde vive y se guarda donde vive; Docs es solo la
// superficie.
package wopi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// MaxFileBytes acota lo que se trae de un host. Es el mismo techo que el import
// del editor: por encima de eso no hay documento ofimático real, hay un fallo.
const MaxFileBytes = 25 << 20

// FileInfo es el subconjunto de CheckFileInfo que el editor necesita.
//
// La especificación define decenas de campos opcionales; aquí están los que se
// USAN. Traerlos todos sería declarar un contrato que no se cumple: un host que
// devuelva la mitad haría creer que el resto está soportado.
//
// estilo: WOPI transmite `BaseFileName`, `UserCanWrite`, `OwnerId`… en
// PascalCase. Pasarlos a snake_case dejaría de entenderse con cualquier host del
// mundo, empezando por el nuestro, a cambio de que un linter esté contento.
//
//nolint:tagliatelle // Los nombres los dicta la ESPECIFICACIÓN, no nuestro
type FileInfo struct {
	// BaseFileName es el nombre con extensión. De él sale el tipo de documento.
	BaseFileName string `json:"BaseFileName"`
	Size         int64  `json:"Size"`
	// OwnerId y UserId son opacos: solo se comparan, nunca se interpretan.
	OwnerID string `json:"OwnerId"`
	UserID  string `json:"UserId"`
	// UserFriendlyName es lo que se pinta en «lo está editando …».
	UserFriendlyName string `json:"UserFriendlyName"`
	// UserCanWrite decide entre ver y editar. Lo dice el HOST, no el editor: el
	// permiso vive donde vive el fichero.
	UserCanWrite bool `json:"UserCanWrite"`
	// Version cambia con cada guardado; sirve para detectar que otro escribió.
	Version string `json:"Version"`
	// SupportsUpdate distingue un host de solo lectura de uno completo.
	SupportsUpdate bool `json:"SupportsUpdate"`
	// SupportsLocks: sin bloqueos, editar a la vez es pisarse.
	SupportsLocks bool `json:"SupportsLocks"`
}

// Extension devuelve la extensión en minúsculas y sin punto, vacía si no hay.
func (f FileInfo) Extension() string {
	i := strings.LastIndex(f.BaseFileName, ".")
	if i < 0 || i == len(f.BaseFileName)-1 {
		return ""
	}
	return strings.ToLower(f.BaseFileName[i+1:])
}

// Client habla con un host WOPI.
type Client struct {
	http *http.Client
}

// New construye el cliente.
//
// El transporte lleva la barrera SSRF de la plataforma, y no es una precaución
// de manual: `WOPISrc` llega en la URL con la que alguien abre el editor, así
// que es un destino que aporta QUIEN LLAMA. Sin la barrera, un enlace preparado
// haría que Docs pidiera por dentro `http://169.254.169.254/…` o cualquier
// servicio interno, y devolviera la respuesta en la pantalla del atacante.
func New(timeout time.Duration) *Client {
	return &Client{http: ssrfhttp.NewClient(timeout, 3)}
}

// CheckFileInfo pide los metadatos del fichero al host.
func (c *Client) CheckFileInfo(ctx context.Context, src, token string) (FileInfo, error) {
	u, err := build(ctx, src, "", token)
	if err != nil {
		return FileInfo{}, err
	}
	res, err := c.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return FileInfo{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return FileInfo{}, fmt.Errorf("wopi: CheckFileInfo devolvió %d", res.StatusCode)
	}
	var info FileInfo
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&info); err != nil {
		return FileInfo{}, fmt.Errorf("wopi: CheckFileInfo ilegible: %w", err)
	}
	if info.BaseFileName == "" {
		return FileInfo{}, fmt.Errorf("wopi: CheckFileInfo sin BaseFileName")
	}
	return info, nil
}

// GetFile trae el contenido del fichero.
func (c *Client) GetFile(ctx context.Context, src, token string) ([]byte, error) {
	u, err := build(ctx, src, "/contents", token)
	if err != nil {
		return nil, err
	}
	res, err := c.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wopi: GetFile devolvió %d", res.StatusCode)
	}
	// Se lee con tope AUNQUE el host haya declarado un tamaño menor: el Size de
	// CheckFileInfo lo da el otro extremo y creérselo es dejar que decida cuánta
	// memoria gasta este proceso.
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("wopi: GetFile ilegible: %w", err)
	}
	if len(data) > MaxFileBytes {
		return nil, fmt.Errorf("wopi: el fichero supera %d MiB", MaxFileBytes>>20)
	}
	return data, nil
}

// PutFile guarda el contenido en el host. `lock` es el identificador de bloqueo
// que se tenga; vacío solo vale si el host no exige bloqueo para escribir.
func (c *Client) PutFile(ctx context.Context, src, token string, data []byte, lock string) error {
	u, err := build(ctx, src, "/contents", token)
	if err != nil {
		return err
	}
	res, err := c.do(ctx, http.MethodPost, u, data, header{"X-WOPI-Lock", lock})
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// 409 con X-WOPI-Lock es «lo tiene bloqueado otro», no un fallo del envío: se
	// distingue para que la interfaz pueda decir quién y no «no se pudo guardar».
	if res.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: %s", ErrLocked, res.Header.Get("X-WOPI-Lock"))
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("wopi: PutFile devolvió %d", res.StatusCode)
	}
	return nil
}

// Bloqueo. Sin él NO se puede guardar de forma segura: dos personas con el mismo
// fichero abierto se pisarían y el último en escribir se llevaría el trabajo del
// otro sin que ninguno se enterara. El protocolo lo resuelve con un identificador
// de bloqueo que el host guarda y que hay que presentar al escribir.
//
// Las tres operaciones se multiplexan por `X-WOPI-Override` sobre el MISMO POST,
// que es como lo define la especificación — no son rutas distintas.

// Lock toma el bloqueo del fichero con el identificador `lock`.
//
// Si lo tiene otra sesión devuelve ErrLocked con su identificador, y eso NO es un
// fallo: es la respuesta correcta, y quien llama debe decir quién lo tiene en vez
// de un «no se pudo guardar» que no explica nada.
func (c *Client) Lock(ctx context.Context, src, token, lock string) error {
	return c.override(ctx, src, token, "LOCK", header{"X-WOPI-Lock", lock})
}

// RefreshLock renueva un bloqueo que ya se tiene.
//
// Hace falta porque el bloqueo CADUCA —el host lo suelta a los 30 minutos, que es
// lo que recomienda WOPI— y sin renovarlo, una sesión larga perdería el suyo a
// mitad de la edición y el guardado siguiente fallaría sin motivo aparente.
func (c *Client) RefreshLock(ctx context.Context, src, token, lock string) error {
	return c.override(ctx, src, token, "REFRESH_LOCK", header{"X-WOPI-Lock", lock})
}

// Unlock suelta el bloqueo. Se llama al cerrar el editor.
//
// Un bloqueo que no se suelta deja el fichero de otro inaccesible hasta que caduca,
// así que el llamante debe intentarlo incluso cuando el guardado falló.
func (c *Client) Unlock(ctx context.Context, src, token, lock string) error {
	return c.override(ctx, src, token, "UNLOCK", header{"X-WOPI-Lock", lock})
}

// override manda una operación de bloqueo y traduce su respuesta.
func (c *Client) override(ctx context.Context, src, token, op string, extra ...header) error {
	u, err := build(ctx, src, "", token)
	if err != nil {
		return err
	}
	res, err := c.do(ctx, http.MethodPost, u, nil, append([]header{{"X-WOPI-Override", op}}, extra...)...)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	// 409 con X-WOPI-Lock es «lo tiene otro», igual que en PutFile: se distingue
	// para poder decir QUIÉN y no un «no se pudo».
	if res.StatusCode == http.StatusConflict {
		return fmt.Errorf("%w: %s", ErrLocked, res.Header.Get("X-WOPI-Lock"))
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("wopi: %s devolvió %d", op, res.StatusCode)
	}
	return nil
}

// ErrLocked: el fichero lo tiene bloqueado otra sesión.
var ErrLocked = fmt.Errorf("wopi: fichero bloqueado")

type header struct{ k, v string }

// build compone la URL de la operación validando el destino ANTES de tocarlo.
//
// El token viaja en la query porque así lo define el protocolo; por eso el host
// lo acuña opaco y de vida corta, y por eso aquí no se registra nunca.
func build(ctx context.Context, src, suffix, token string) (string, error) {
	if token == "" {
		return "", fmt.Errorf("wopi: falta el token de acceso")
	}
	u, err := url.Parse(src)
	if err != nil {
		return "", fmt.Errorf("wopi: WOPISrc no es una URL: %w", err)
	}
	// Solo https. Un `file://` o un `gopher://` en WOPISrc no son casos raros:
	// son el primer intento de quien prueba a romper esto.
	if u.Scheme != "https" {
		return "", fmt.Errorf("wopi: WOPISrc debe ser https")
	}
	if err := ssrfhttp.ValidateURL(ctx, src); err != nil {
		return "", fmt.Errorf("wopi: destino rechazado: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + suffix
	q := u.Query()
	q.Set("access_token", token)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (c *Client) do(
	ctx context.Context, method, u string, body []byte, extra ...header,
) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		rdr = strings.NewReader(string(body))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return nil, fmt.Errorf("wopi: petición inválida: %w", err)
	}
	for _, h := range extra {
		if h.v != "" {
			req.Header.Set(h.k, h.v)
		}
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wopi: el host no respondió: %w", err)
	}
	return res, nil
}
