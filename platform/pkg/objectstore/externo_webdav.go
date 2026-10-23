// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
)

// ErrPresignUnsupported lo devuelve un almacén que no sabe firmar URLs (WebDAV):
// el servicio debe transmitir los bytes con Put/Get cuando no hay firma.
var ErrPresignUnsupported = errors.New("service: este almacén no firma URLs")

// WebDAV es el almacén de terceros sobre WebDAV (mig 0023): un NAS
// (Synology, QNAP, TrueNAS), Nextcloud/ownCloud o la CLI de Internxt, que
// publica uno. Cada clave de blob es una ruta bajo la carpeta base; las
// colecciones intermedias se crean con MKCOL la primera vez y se recuerdan.
//
// ponytail: la subida se vuelca a un fichero temporal para mandar Content-Length
// —nginx y algunos NAS rechazan PUT en trozos—; si el disco efímero del pod se
// queda corto con subidas grandes, el siguiente paso es PUT por trozos donde el
// servidor lo admita.
type WebDAV struct {
	base     *url.URL
	user     string
	password string
	http     *http.Client

	mu     sync.Mutex
	creado map[string]bool
}

// NewWebDAV abre un destino HTTPS; cada conexión aplica la barrera SSRF.
func NewWebDAV(rawBase, basePath, user, password string) (*WebDAV, error) {
	u, err := url.Parse(strings.TrimSpace(rawBase))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Opaque != "" {
		return nil, errors.New("service: la URL del WebDAV tiene que ser https://host/ruta sin credenciales")
	}
	base, carpeta := strings.Trim(u.Path, "/"), strings.Trim(basePath, "/")
	if !rutaWebDAVValida(base) || !rutaWebDAVValida(carpeta) {
		return nil, errors.New("service: la carpeta base del WebDAV contiene segmentos inválidos")
	}
	u.Path = "/"
	for _, parte := range []string{base, carpeta} {
		if parte != "" {
			u.Path += parte + "/"
		}
	}
	u.RawPath, u.RawQuery, u.Fragment, u.RawFragment = "", "", "", ""
	u.ForceQuery = false
	cliente := clienteDeTerceros(30 * time.Second)
	// Un WebDAV configurado es un único destino: no reenviar sus credenciales
	// ni un cuerpo de subida a otro host, subdominio, puerto o ruta por redirect.
	cliente.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &WebDAV{
		base: u, user: user, password: password,
		http:   cliente,
		creado: map[string]bool{},
	}, nil
}

// rutaWebDAVValida rechaza recorridos antes de que el servidor o su proxy
// normalice la ruta. PathEscape no escapa los segmentos "." y "..".
func rutaWebDAVValida(ruta string) bool {
	if ruta == "" {
		return true
	}
	for _, parte := range strings.Split(ruta, "/") {
		if parte == "" || parte == "." || parte == ".." || strings.ContainsRune(parte, '\\') || strings.ContainsFunc(parte, unicode.IsControl) {
			return false
		}
	}
	return true
}

func (w *WebDAV) ruta(t tenancy.Tenant, key string) (string, error) {
	// Solo la ruta puede depender del blob; esquema y autoridad son la base
	// configurada. El tenant se valida incluso si se construyó sin tenancy.New.
	key = strings.TrimLeft(key, "/")
	if !tenancy.ValidSlug(t.Slug) || key == "" || !rutaWebDAVValida(key) {
		return "", errors.New("webdav: tenant o clave de blob inválidos")
	}
	return w.base.Path + t.StoragePrefix() + key, nil
}

func (w *WebDAV) hacer(ctx context.Context, metodo, ruta string, body io.Reader, size int64, cab map[string]string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, metodo, w.base.String(), body)
	if err != nil {
		return nil, err
	}
	// No parsear una URL procedente de la clave: Path trata ?, # y % como
	// datos y no puede sustituir el host ni las credenciales del destino.
	req.URL.Path, req.URL.RawPath = ruta, ""
	if size >= 0 {
		req.ContentLength = size
	}
	req.SetBasicAuth(w.user, w.password)
	for k, v := range cab {
		req.Header.Set(k, v)
	}
	return w.http.Do(req)
}

// asegurarCarpetas crea las colecciones padre de una ruta, de fuera hacia
// dentro. 405 significa «ya existe» en WebDAV.
func (w *WebDAV) asegurarCarpetas(ctx context.Context, ruta string) error {
	partes := strings.Split(strings.TrimPrefix(ruta, w.base.Path), "/")
	acum := w.base.Path
	for _, p := range partes[:len(partes)-1] {
		acum += p + "/"
		w.mu.Lock()
		hecho := w.creado[acum]
		w.mu.Unlock()
		if hecho {
			continue
		}
		res, err := w.hacer(ctx, "MKCOL", acum, http.NoBody, 0, nil)
		if err != nil {
			return fmt.Errorf("webdav: MKCOL %s: %w", acum, err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusCreated && res.StatusCode != http.StatusMethodNotAllowed && res.StatusCode != http.StatusOK {
			return fmt.Errorf("webdav: MKCOL %s: %s", acum, res.Status)
		}
		w.mu.Lock()
		w.creado[acum] = true
		w.mu.Unlock()
	}
	return nil
}

func (w *WebDAV) Put(ctx context.Context, t tenancy.Tenant, key string, r io.Reader, size int64, opts PutOptions) error {
	ruta, err := w.ruta(t, key)
	if err != nil {
		return err
	}
	if err := w.asegurarCarpetas(ctx, ruta); err != nil {
		return err
	}
	body := r
	if size < 0 {
		tmp, err := os.CreateTemp("", "sytue-webdav-*")
		if err != nil {
			return fmt.Errorf("webdav: temporal: %w", err)
		}
		defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
		n, err := io.Copy(tmp, r)
		if err != nil {
			return fmt.Errorf("webdav: volcar la subida: %w", err)
		}
		if _, err := tmp.Seek(0, io.SeekStart); err != nil {
			return err
		}
		body, size = tmp, n
	}
	cab := map[string]string{}
	if opts.ContentType != "" {
		cab["Content-Type"] = opts.ContentType
	}
	res, err := w.hacer(ctx, http.MethodPut, ruta, body, size, cab)
	if err != nil {
		return fmt.Errorf("webdav: put %q: %w", key, err)
	}
	_ = res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return fmt.Errorf("webdav: put %q: %s", key, res.Status)
	}
	return nil
}

func (w *WebDAV) Get(ctx context.Context, t tenancy.Tenant, key string) (io.ReadCloser, error) {
	ruta, err := w.ruta(t, key)
	if err != nil {
		return nil, err
	}
	res, err := w.hacer(ctx, http.MethodGet, ruta, http.NoBody, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("webdav: get %q: %w", key, err)
	}
	if res.StatusCode == http.StatusNotFound {
		_ = res.Body.Close()
		return nil, ErrNotFound
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return nil, fmt.Errorf("webdav: get %q: %s", key, res.Status)
	}
	return res.Body, nil
}

func (w *WebDAV) Delete(ctx context.Context, t tenancy.Tenant, key string) error {
	ruta, err := w.ruta(t, key)
	if err != nil {
		return err
	}
	res, err := w.hacer(ctx, http.MethodDelete, ruta, http.NoBody, 0, nil)
	if err != nil {
		return fmt.Errorf("webdav: delete %q: %w", key, err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
		return fmt.Errorf("webdav: delete %q: %s", key, res.Status)
	}
	return nil
}

func (w *WebDAV) Stat(ctx context.Context, t tenancy.Tenant, key string) (int64, error) {
	ruta, err := w.ruta(t, key)
	if err != nil {
		return 0, err
	}
	res, err := w.hacer(ctx, http.MethodHead, ruta, http.NoBody, 0, nil)
	if err != nil {
		return 0, fmt.Errorf("webdav: head %q: %w", key, err)
	}
	_ = res.Body.Close()
	switch {
	case res.StatusCode == http.StatusNotFound:
		return 0, ErrNotFound
	case res.StatusCode != http.StatusOK:
		return 0, fmt.Errorf("webdav: head %q: %s", key, res.Status)
	}
	n, _ := strconv.ParseInt(res.Header.Get("Content-Length"), 10, 64)
	return n, nil
}

func (w *WebDAV) Existe(ctx context.Context, t tenancy.Tenant, key string) (bool, error) {
	_, err := w.Stat(ctx, t, key)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (w *WebDAV) Copiar(ctx context.Context, t tenancy.Tenant, origen, destino string) error {
	rutaOrigen, err := w.ruta(t, origen)
	if err != nil {
		return err
	}
	rutaDestino, err := w.ruta(t, destino)
	if err != nil {
		return err
	}
	if err := w.asegurarCarpetas(ctx, rutaDestino); err != nil {
		return err
	}
	destinoURL := *w.base
	destinoURL.Path, destinoURL.RawPath = rutaDestino, ""
	res, err := w.hacer(ctx, "COPY", rutaOrigen, http.NoBody, 0, map[string]string{"Destination": destinoURL.String(), "Overwrite": "T"})
	if err != nil {
		return fmt.Errorf("webdav: copy %q: %w", origen, err)
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusNoContent {
		return nil
	}
	// Sin COPY (algunos servidores mínimos): leer y escribir.
	rc, err := w.Get(ctx, t, origen)
	if err != nil {
		return err
	}
	defer rc.Close()
	return w.Put(ctx, t, destino, rc, -1, PutOptions{})
}

func (w *WebDAV) PresignGet(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

func (w *WebDAV) PresignPut(context.Context, tenancy.Tenant, string, time.Duration) (*url.URL, error) {
	return nil, ErrPresignUnsupported
}

// Probar comprueba que la carpeta base responde con esas credenciales.
func (w *WebDAV) Probar(ctx context.Context) error {
	res, err := w.hacer(ctx, "PROPFIND", w.base.Path, http.NoBody, 0, map[string]string{"Depth": "0"})
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusMultiStatus || res.StatusCode == http.StatusOK {
		return nil
	}
	if res.StatusCode == http.StatusNotFound {
		// La carpeta base puede no existir aún: se crea al primer uso. Que el
		// servidor conteste con credenciales aceptadas es lo que se comprueba.
		return nil
	}
	return fmt.Errorf("webdav: %s", res.Status)
}

var _ BlobStore = (*WebDAV)(nil)
