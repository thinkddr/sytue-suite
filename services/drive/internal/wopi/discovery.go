// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package wopi

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
)

// Descubrimiento del EDITOR.
//
// Un host WOPI no sabe por sí mismo con qué URL se abre un .docx: eso lo publica cada
// editor en su `discovery.xml`, y no coinciden entre ellos (Collabora sirve una acción
// por tipo, Office Online otra distinta con parámetros propios). Adivinar la plantilla
// daría una URL que carga en blanco, que es el peor fallo posible: parece que funciona.
//
// El editor lo pone quien despliega: la suite no incorpora motores AGPL. Sin URL de
// descubrimiento configurada, esto queda apagado y Drive simplemente no ofrece abrir en
// un editor externo.

// discoveryTimeout acota la descarga del catálogo. Es una llamada de arranque perezoso,
// no del camino caliente.
const discoveryTimeout = 10 * time.Second

// Discovery es el catálogo de acciones del editor, cacheado.
type Discovery struct {
	src    string
	client *http.Client
	ttl    time.Duration
	// confiable: la dirección la puso quien despliega (WOPI_DISCOVERY_URL), no una
	// organización. Solo entonces puede ser interna o ir sin TLS.
	confiable bool

	mu      sync.Mutex
	acts    map[string]string // "docx|edit" → urlsrc
	exts    []string          // extensiones abribles, ordenadas
	fetched time.Time
}

// NewDiscovery construye el catálogo. `src` es la URL del discovery.xml del editor;
// vacía ⇒ nil (la función que consulta lo trata como «no hay editor»).
//
// La dirección que llega aquí la ha escrito el administrador de una ORGANIZACIÓN,
// así que es entrada de fuera: tiene que ser https y pública, y se descarga con
// el cliente de `ssrfhttp`, que valida el destino al conectar y en cada
// redirección. La del despliegue va por NewDiscoveryDeConfianza.
func NewDiscovery(src string, ttl time.Duration) *Discovery {
	d := nuevoDiscovery(src, ttl)
	if d != nil {
		d.client = clienteDeFuera()
	}
	return d
}

// NewDiscoveryDeConfianza es el catálogo del editor que configura quien DESPLIEGA
// (`WOPI_DISCOVERY_URL`): puede ser un servicio del propio clúster, sin TLS.
func NewDiscoveryDeConfianza(src string, ttl time.Duration) *Discovery {
	d := nuevoDiscovery(src, ttl)
	if d != nil {
		d.confiable = true
	}
	return d
}

// ErrDireccionDelEditor: la dirección del editor de una organización no es https
// o no es pública.
var ErrDireccionDelEditor = errors.New("wopi: la dirección del editor tiene que ser https y pública")

// clienteDeFuera descarga un catálogo cuya dirección puso una organización.
func clienteDeFuera() *http.Client {
	c := ssrfhttp.NewClient(discoveryTimeout, 3)
	revalidar := c.CheckRedirect
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("%w: la redirección sale de https", ErrDireccionDelEditor)
		}
		return revalidar(req, via)
	}
	return c
}

func nuevoDiscovery(src string, ttl time.Duration) *Discovery {
	src = strings.TrimSpace(src)
	if src == "" {
		return nil
	}
	if ttl <= 0 {
		// Un editor solo cambia sus acciones al actualizarse. Una hora evita machacarlo
		// y sigue recogiendo el cambio sin reiniciar Drive.
		ttl = time.Hour
	}
	return &Discovery{
		src:    src,
		ttl:    ttl,
		client: &http.Client{Timeout: discoveryTimeout},
		acts:   map[string]string{},
	}
}

// wopiDiscoveryXML es la forma del documento. Solo se leen las acciones: el resto
// (favicons, requisitos de licencia, zonas de red) no cambia a qué URL se abre.
type wopiDiscoveryXML struct {
	XMLName xml.Name `xml:"wopi-discovery"`
	Zones   []struct {
		Apps []struct {
			Actions []struct {
				Name   string `xml:"name,attr"`
				Ext    string `xml:"ext,attr"`
				URLSrc string `xml:"urlsrc,attr"`
			} `xml:"action"`
		} `xml:"app"`
	} `xml:"net-zone"`
}

// placeholderRe casa los parámetros opcionales que Office Online deja marcados en su
// urlsrc: `<ui=UI_LLCC&>`, `<rs=DC_LLCC&>`… Son plantillas, no parámetros: pasarlos tal
// cual produce una URL que el editor no entiende.
var placeholderRe = regexp.MustCompile(`<[^>]*>`)

// Refresh descarga y reemplaza el catálogo.
//
// Si falla, NO borra lo que hubiera: un editor reiniciándose no debe dejar a nadie sin
// poder abrir sus documentos.
func (d *Discovery) Refresh(ctx context.Context) error {
	if !d.confiable {
		u, err := url.Parse(d.src)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return ErrDireccionDelEditor
		}
		if err := ssrfhttp.ValidateURL(ctx, d.src); err != nil {
			return fmt.Errorf("%w: %w", ErrDireccionDelEditor, err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.src, http.NoBody)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("wopi: descubrimiento: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("wopi: descubrimiento: estado %d", resp.StatusCode)
	}
	// Cota: un discovery.xml real ronda las decenas de KB; 4 MiB es holgado y evita que
	// una respuesta enorme (o un destino equivocado) se lea entera en memoria.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("wopi: descubrimiento: %w", err)
	}
	acts, exts, err := parseDiscovery(raw)
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.acts, d.exts, d.fetched = acts, exts, time.Now()
	d.mu.Unlock()
	return nil
}

// parseDiscovery extrae el mapa de acciones y la lista de extensiones abribles.
func parseDiscovery(raw []byte) (map[string]string, []string, error) {
	var doc wopiDiscoveryXML
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return nil, nil, fmt.Errorf("wopi: descubrimiento ilegible: %w", err)
	}
	acts := map[string]string{}
	seen := map[string]struct{}{}
	for _, z := range doc.Zones {
		for _, a := range z.Apps {
			for _, act := range a.Actions {
				ext := strings.ToLower(strings.TrimSpace(act.Ext))
				name := strings.ToLower(strings.TrimSpace(act.Name))
				if ext == "" || act.URLSrc == "" {
					continue
				}
				// Solo interesan las dos que usa Drive. `editnew`, `convert`,
				// `imagepreview` y demás no abren un fichero existente.
				if name != "edit" && name != "view" {
					continue
				}
				key := ext + "|" + name
				// La PRIMERA gana: el discovery lista las zonas de red en orden de
				// preferencia y quedarse con la última daría la menos adecuada.
				if _, dup := acts[key]; dup {
					continue
				}
				acts[key] = placeholderRe.ReplaceAllString(act.URLSrc, "")
				if _, dup := seen[ext]; !dup {
					seen[ext] = struct{}{}
				}
			}
		}
	}
	exts := make([]string, 0, len(seen))
	for e := range seen {
		exts = append(exts, e)
	}
	sort.Strings(exts) // orden estable: la UI lo enseña y los tests lo comparan
	return acts, exts, nil
}

// ensure recarga si el catálogo está vacío o caducado. Un fallo con catálogo cargado se
// ignora a propósito (ver Refresh).
func (d *Discovery) ensure(ctx context.Context) {
	d.mu.Lock()
	vacio := len(d.acts) == 0
	viejo := time.Since(d.fetched) > d.ttl
	d.mu.Unlock()
	if vacio || viejo {
		_ = d.Refresh(ctx)
	}
}

// Extensions devuelve las extensiones que el editor sabe abrir. La UI la usa para no
// ofrecer «abrir en el editor» sobre un fichero que acabaría en una pantalla en blanco.
func (d *Discovery) Extensions(ctx context.Context) []string {
	if d == nil {
		return nil
	}
	d.ensure(ctx)
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.exts...)
}

// URLFor compone la URL con la que el navegador abre `wopiSrc` en el editor.
//
// Con `write` se prefiere la acción de edición y se cae a la de vista si el editor no
// ofrece edición para ese tipo (un PDF, por ejemplo). Sin `write` solo se busca vista, y
// si no hay, se acepta la de edición: el permiso REAL lo impone el token, no esta URL.
func (d *Discovery) URLFor(ctx context.Context, filename, wopiSrc string, write bool) (string, bool) {
	if d == nil {
		return "", false
	}
	ext := strings.ToLower(strings.TrimPrefix(strings.ToLower(extOf(filename)), "."))
	if ext == "" {
		return "", false
	}
	d.ensure(ctx)

	orden := []string{"view", "edit"}
	if write {
		orden = []string{"edit", "view"}
	}
	d.mu.Lock()
	var src string
	for _, name := range orden {
		if u, ok := d.acts[ext+"|"+name]; ok {
			src = u
			break
		}
	}
	d.mu.Unlock()
	if src == "" {
		return "", false
	}

	// WOPISrc es la dirección del fichero EN EL HOST: es lo que hace que el editor
	// vuelva a llamarnos. Se añade respetando los parámetros que ya trajera el urlsrc.
	sep := "?"
	if strings.Contains(src, "?") {
		sep = "&"
	}
	if strings.HasSuffix(src, "?") || strings.HasSuffix(src, "&") {
		sep = ""
	}
	return src + sep + "WOPISrc=" + url.QueryEscape(wopiSrc), true
}

// extOf devuelve la extensión de un nombre de fichero (con el punto), o "".
func extOf(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 || i == len(name)-1 {
		return ""
	}
	return name[i:]
}
