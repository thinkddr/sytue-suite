// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hubhttp

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"slices"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/portabilidad"
	"git.sytue.eu/sytue/one/services/hub/internal/domain"
)

// Exportar tus datos (RGPD art. 20): un único ZIP con lo que entrega cada
// producto. Hub no sabe leer un buzón ni una agenda: junta las entregas
// (platform/pkg/portabilidad) y las escribe en orden, cada una en su carpeta.

// exportable es un producto que entrega su parte.
type exportable struct {
	// App es la clave de despliegue (APPS) y el nombre de su carpeta en el ZIP.
	App string
	URL string
}

func (h *Handlers) exportables() []exportable {
	c := h.cfg
	todos := []exportable{
		{"account", c.IDBaseURL + "/api/v1/me/export"},
		{"mail", c.MailBaseURL + "/api/v1/export"},
		{"drive", c.DriveBaseURL + "/api/v1/export"},
		{"calendar", c.CalendarBaseURL + "/api/v1/export"},
		{"contacts", c.ContactsBaseURL + "/api/v1/export"},
		{"notes", c.NotesBaseURL + "/api/v1/export"},
		{"docs", c.DocsBaseURL + "/api/v1/export"},
		{"photos", c.PhotosBaseURL + "/api/v1/export"},
		{"rooms", c.RoomsBaseURL + "/api/v1/export"},
		{"cast", c.CastBaseURL + "/api/v1/export"},
		{"pages", c.PagesBaseURL + "/api/v1/export"},
	}
	// Lo que esta instalación no levanta ni se pide: un producto que no existe no
	// es un fallo que haya que contar en el resumen. La cuenta está siempre.
	if len(c.AppsDesplegadas) == 0 {
		return todos
	}
	return slices.DeleteFunc(todos, func(e exportable) bool {
		return e.App != "account" && !slices.Contains(c.AppsDesplegadas, e.App)
	})
}

// clienteExportacion no tiene Timeout: el corte lo pone el contexto de la
// descarga, que dura lo que tarde la persona en bajársela.
var clienteExportacion = &http.Client{}

// exportando evita dos exportaciones a la vez de la misma persona: cada una abre
// una entrega en cada producto, y repetir el clic no la acelera.
// ponytail: por réplica; con varias réplicas el tope real se multiplica.
var exportando sync.Map

// Export responde GET /api/v1/export?format=json|csv|xml.
func (h *Handlers) Export(w http.ResponseWriter, r *http.Request) {
	formato, err := portabilidad.FormatoDe(r.URL.Query().Get("format"))
	if err != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The format must be json, csv or xml."))
		return
	}
	sub := subFromCtx(r)
	if _, ya := exportando.LoadOrStore(sub, struct{}{}); ya {
		httpx.Fail(w, r, httpx.ErrTooManyRequests.WithMessage("An export is already being prepared."))
		return
	}
	defer exportando.Delete(sub)

	entregas := abrirEntregas(r.Context(), bearerToken(r), formato, h.exportables())

	// La organización va en el nombre: quien pertenece a varias se baja una copia de
	// cada una, y las dos no pueden llamarse igual en su carpeta de descargas.
	nombre := "sytue-one-" + time.Now().UTC().Format("2006-01-02")
	if c, ok := authn.FromContext(r.Context()); ok && c.Tenant != "" {
		nombre = "sytue-one-" + limpiarRuta(c.Tenant) + "-" + time.Now().UTC().Format("2006-01-02")
	}
	raiz := nombre + "/"
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nombre+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.WriteHeader(http.StatusOK)

	zw := zip.NewWriter(w)
	vistos := map[string]int{}
	resumen := make([]filaResumen, 0, len(entregas))
	for _, e := range entregas {
		resumen = append(resumen, volcar(zw, raiz+e.App+"/", e, vistos))
	}
	// Y los avisos de su campana, de TODAS sus organizaciones: son suyos (ADR 0053).
	if h.store != nil {
		resumen = append(resumen, h.volcarAvisos(r, zw, raiz+"hub/notifications."+string(formato), formato))
	}
	// El resumen va el último porque hasta el final no se sabe cómo fue cada producto.
	wr, err := zw.CreateHeader(&zip.FileHeader{
		Name: raiz + "summary." + string(formato), Method: zip.Deflate, Modified: time.Now(),
	})
	if err == nil {
		err = portabilidad.EscribirTabla(wr, formato, "summary", "product", portabilidad.Todas(resumen))
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		// Las cabeceras ya salieron: lo único que queda es que el ZIP llegue sin
		// índice, y eso el sistema de la persona lo dice al abrirlo.
		log.From(r.Context()).Warn("export: la descarga no terminó", "error", err)
	}
}

// entrega es la respuesta, ya abierta, de un producto.
type entrega struct {
	exportable
	resp *http.Response
	err  error
}

// abrirEntregas pide la suya a TODOS los productos a la vez y antes de escribir un
// byte. El token de la persona dura minutos y la descarga puede durar horas; cada
// producto lo comprueba al recibir la petición, y desde ahí da igual cuándo se lea
// su entrega. Mientras le toca, el producto se queda parado escribiendo en un
// socket lleno: no trabaja de más.
func abrirEntregas(ctx context.Context, token string, f portabilidad.Formato, lista []exportable) []entrega {
	out := make([]entrega, len(lista))
	var wg sync.WaitGroup
	for i, e := range lista {
		out[i].exportable = e
		wg.Go(func() {
			//nolint:bodyclose // se cierra en volcar, cuando le toca a su producto.
			out[i].resp, out[i].err = abrirEntrega(ctx, token, f, e.URL)
		})
	}
	wg.Wait()
	return out
}

func abrirEntrega(ctx context.Context, token string, f portabilidad.Formato, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"?format="+string(f), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := clienteExportacion.Do(req)
	if err != nil {
		// Sin la URL: es interna y no pinta nada en el resumen de nadie.
		return nil, errors.New("the service did not respond")
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, errors.New("this installation does not export this product yet")
		}
		return nil, fmt.Errorf("the service answered HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

// filaResumen es una fila de summary.<formato>.
type filaResumen struct {
	Product string `json:"product"`
	// Status: complete, incomplete (se cortó a mitad) o unavailable (no entregó nada).
	Status string `json:"status"`
	Files  int    `json:"files"`
	Error  string `json:"error"`
}

func volcar(zw *zip.Writer, prefijo string, e entrega, vistos map[string]int) filaResumen {
	fila := filaResumen{Product: e.App, Status: "complete"}
	if e.err != nil {
		fila.Status, fila.Error = "unavailable", e.err.Error()
		return fila
	}
	defer e.resp.Body.Close()
	for parte, err := range portabilidad.Leer(e.resp) {
		if err == nil {
			err = copiarParte(zw, prefijo, parte, vistos)
		}
		if err != nil {
			fila.Status, fila.Error = "incomplete", err.Error()
			break
		}
		fila.Files++
	}
	return fila
}

func copiarParte(zw *zip.Writer, prefijo string, p portabilidad.Parte, vistos map[string]int) error {
	nombre := unico(prefijo+limpiarRuta(p.Ruta), vistos)
	mod := p.Modificado
	if mod.IsZero() {
		mod = time.Now()
	}
	wr, err := zw.CreateHeader(&zip.FileHeader{Name: nombre, Method: metodo(nombre), Modified: mod})
	if err != nil {
		return err
	}
	if _, err := io.Copy(wr, p); err != nil {
		return fmt.Errorf("%s: %w", strings.TrimPrefix(nombre, prefijo), err)
	}
	return nil
}

// limpiarRuta deja una ruta que se puede descomprimir en cualquier sistema: sin
// «..» ni raíz (nada se sale de la carpeta), sin los caracteres que Windows no
// admite y sin nombres acabados en punto o espacio, que Windows tampoco crea.
func limpiarRuta(ruta string) string {
	var segs []string
	for s := range strings.SplitSeq(strings.ReplaceAll(ruta, `\`, "/"), "/") {
		s = strings.Map(func(r rune) rune {
			if r < 0x20 || strings.ContainsRune(`<>:"|?*`, r) {
				return '_'
			}
			return r
		}, s)
		s = strings.TrimRight(s, ". ")
		if s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) == 0 {
		return "_"
	}
	return strings.Join(segs, "/")
}

// unico evita dos entradas con el mismo nombre —dos «Factura.pdf» en la misma
// carpeta, o «a.txt» y «A.txt», que en Windows y macOS son el mismo fichero—: la
// segunda sale como «Factura (2).pdf».
func unico(nombre string, vistos map[string]int) string {
	clave := strings.ToLower(nombre)
	n := vistos[clave]
	vistos[clave] = n + 1
	if n == 0 {
		return nombre
	}
	ext := path.Ext(nombre)
	if strings.Contains(ext, "/") {
		ext = ""
	}
	otro := fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(nombre, ext), n+1, ext)
	return unico(otro, vistos)
}

// metodo comprime lo que es texto y guarda tal cual lo demás: una foto o un vídeo
// ya van comprimidos, y pasarlos por Deflate solo gasta CPU.
func metodo(nombre string) uint16 {
	switch strings.ToLower(path.Ext(nombre)) {
	case ".json", ".csv", ".xml", ".eml", ".ics", ".vcf", ".html", ".htm", ".txt", ".md", ".svg":
		return zip.Deflate
	}
	return zip.Store
}

// filaAviso es un aviso en «Tus datos».
type filaAviso struct {
	Organization string `json:"organization"`
	Source       string `json:"source"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	URL          string `json:"url"`
	Read         bool   `json:"read"`
	CreatedAt    string `json:"created_at"`
}

// avisosExportables es el tope de avisos que van a «Tus datos».
// ponytail: la retención ya los purga; si alguien llegara al tope, paginar el fichero.
const avisosExportables = 10000

// volcarAvisos escribe los avisos de quien exporta —de todas sus organizaciones— en el
// ZIP, y devuelve su fila del resumen.
func (h *Handlers) volcarAvisos(r *http.Request, zw *zip.Writer, nombre string, f portabilidad.Formato) filaResumen {
	fila := filaResumen{Product: "hub", Status: "complete", Files: 1}
	ctx := personales(r.Context())
	var filas []filaAviso
	var antes *domain.Antes
	for len(filas) < avisosExportables {
		pagina, err := h.store.ListNotifications(ctx, buzones(r), false, antes, 100)
		if err != nil {
			fila.Status, fila.Error = "incomplete", err.Error()
			break
		}
		for _, n := range pagina {
			filas = append(filas, filaAviso{
				Organization: n.Tenant, Source: string(n.Source), Type: n.Type, Title: n.Title,
				Body: n.Body, URL: n.URL, Read: n.Read, CreatedAt: n.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		if len(pagina) < 100 {
			break
		}
		ultimo := pagina[len(pagina)-1]
		antes = &domain.Antes{En: ultimo.CreatedAt, ID: ultimo.ID}
	}
	w, err := zw.CreateHeader(&zip.FileHeader{Name: nombre, Method: zip.Deflate, Modified: time.Now()})
	if err == nil {
		err = portabilidad.EscribirTabla(w, f, "notifications", "notification", portabilidad.Todas(filas))
	}
	if err != nil {
		fila.Status, fila.Error, fila.Files = "unavailable", err.Error(), 0
	}
	return fila
}
