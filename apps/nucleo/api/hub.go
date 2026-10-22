// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Hub es el cliente de las notificaciones unificadas de Sytue Hub (services/hub, `/api/v1`).
type Hub struct{ c *Client }

// NewHub crea el cliente sobre la raíz del servicio (ServiceURL(apex, "hub")).
func NewHub(servicio string, s Session, h *http.Client) *Hub {
	return &Hub{c: New(servicio+"/api/v1", s, h)}
}

// HubNotification es un aviso: de qué app viene, qué dice y a dónde lleva.
type HubNotification struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	// Type y Params componen el título en el idioma de quien lo lee
	// (`notifications.kind.<type>`); Title es el respaldo, en el del emisor.
	Type      string            `json:"type"`
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	URL       string            `json:"url"`
	Params    map[string]string `json:"params"`
	Read      bool              `json:"read"`
	CreatedAt time.Time         `json:"created_at"`
	// Organization es la que lo mandó, solo si no es la de la sesión (ADR 0053).
	Organization string `json:"organization"`
	// OrganizationName es su nombre visible, si Sytue ID lo da; si no, se enseña
	// Organization.
	OrganizationName string `json:"organization_name"`
	// Historic: reconstruido de lo que ya había, anterior al registro de avisos. La
	// campana lo marca (ui.ProcedenciaAviso).
	Historic bool `json:"historic"`
}

// NotificationsAntes devuelve los avisos de DETRÁS de `antes` (el último de la página
// anterior), por (fecha, id): con la fecha sola se perdían los del mismo instante.
func (h *Hub) NotificationsAntes(ctx context.Context, limite int, antes HubNotification) ([]HubNotification, error) {
	out, err := JSON[struct {
		Notifications []HubNotification `json:"notifications"`
	}](ctx, h.c, Request{Path: "/notifications", Query: url.Values{
		"limit":     {strconv.Itoa(limite)},
		"before":    {antes.CreatedAt.UTC().Format(time.RFC3339Nano)},
		"before_id": {antes.ID},
	}})
	return out.Notifications, err
}

// Notifications devuelve los últimos avisos, el más reciente primero.
func (h *Hub) Notifications(ctx context.Context, limite int) ([]HubNotification, error) {
	out, err := JSON[struct {
		Notifications []HubNotification `json:"notifications"`
	}](ctx, h.c, Request{Path: "/notifications", Query: url.Values{"limit": {strconv.Itoa(limite)}}})
	return out.Notifications, err
}

// HubAvisoPropio es un aviso que la persona se deja a sí misma: el de la app de
// escritorio que ha visto una versión nueva. Mismos campos que uno de un servicio.
type HubAvisoPropio struct {
	Source string            `json:"source"`
	Type   string            `json:"type"`
	Title  string            `json:"title"`
	URL    string            `json:"url,omitempty"`
	Params map[string]string `json:"params,omitempty"`
}

// AvisarAMi deja el aviso en la campana de quien tiene la sesión
// (`POST /api/v1/notifications` sin sujeto: a uno mismo siempre vale).
func (h *Hub) AvisarAMi(ctx context.Context, n HubAvisoPropio) error {
	cuerpo, err := json.Marshal(n)
	if err != nil {
		return err
	}
	resp, err := h.c.Do(ctx, Request{
		Method: http.MethodPost, Path: "/notifications", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// HubContadores es lo que cuenta la campana: los avisos sin leer y, si Sytue Mail
// contesta, los correos sin leer (nil si no se sabe). El correo es ESTADO y no aviso:
// el que ya esperaba antes de que Mail avisara no dejó ninguno.
type HubContadores struct {
	Unread     int  `json:"unread"`
	MailUnread *int `json:"mail_unread,omitempty"`
}

// Contadores cuenta los avisos y el correo sin leer.
func (h *Hub) Contadores(ctx context.Context) (HubContadores, error) {
	return JSON[HubContadores](ctx, h.c, Request{Path: "/notifications/unread_count"})
}

// StreamNotifications escucha el flujo SSE de avisos del Hub
// (`GET /api/v1/notifications/stream`) y llama a `cada` con lo que llega. No vuelve
// hasta que el flujo se corta o `ctx` muere: la reconexión la decide quien llama, que
// es quien sabe cuánto esperar (la bandeja del Hub, `apps/fyne/apps/hub/bandeja.go`).
//
// Es el MISMO flujo que consume `NotificationCenter.tsx` con `EventSource`, y en Go
// tiene una ventaja que en el navegador no: el token va en la cabecera `Authorization`
// y no en la query, así que no hay que rehacer la URL cuando caduca ni acaba en el log
// de acceso de ningún proxy.
//
// Solo se miran los eventos `notification`. El `ready` del saludo trae el contador
// inicial, que quien llama ya pide por `Unread`, y los `: keep-alive` son comentarios.
func (h *Hub) StreamNotifications(ctx context.Context, cada func(HubNotification)) error {
	return h.Escuchar(ctx, cada, nil)
}

// EventoAvatar es el evento de estado con el que Sytue ID avisa de que la persona
// cambió o quitó su foto (`hubnotify.EventoAvatar` en el servidor). No es un aviso:
// no se guarda ni sale en la campana.
const EventoAvatar = "avatar"

// Escuchar es StreamNotifications con los eventos de ESTADO además de los avisos:
// `alEstado` recibe el nombre (`avatar`) y el `data` en crudo. nil, no se miran.
func (h *Hub) Escuchar(ctx context.Context, cada func(HubNotification), alEstado func(evento, datos string)) error {
	resp, err := h.c.Do(ctx, Request{
		Path:   "/notifications/stream",
		Header: http.Header{"Accept": {"text/event-stream"}},
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	return leerSSE(resp.Body, func(evento, datos string) {
		var n HubNotification
		switch {
		case evento == "notification":
			if datos != "" && json.Unmarshal([]byte(datos), &n) == nil {
				cada(n)
			}
		case evento == EventoAvatar && alEstado != nil:
			alEstado(evento, datos)
		}
	})
}

// MarkRead da un aviso por leído.
func (h *Hub) MarkRead(ctx context.Context, id string) error {
	resp, err := h.c.Do(ctx, Request{Method: http.MethodPost, Path: "/notifications/" + url.PathEscape(id) + "/read"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// MarkAllRead da por leídos TODOS los avisos de una vez (`POST /notifications/read_all`).
func (h *Hub) MarkAllRead(ctx context.Context) error {
	resp, err := h.c.Do(ctx, Request{Method: http.MethodPost, Path: "/notifications/read_all"})
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// HubNotificationPref es una regla de silencio (`domain.NotificationPref`). Con `Kind`
// vacío silencia el ORIGEN entero; con él, solo ese tipo. La regla más específica gana.
type HubNotificationPref struct {
	Source string `json:"source"`
	Kind   string `json:"kind,omitempty"`
	Muted  bool   `json:"muted"`
}

// NotificationPrefs son las reglas de silencio del PROPIO usuario: el Hub no deja leer
// las de otro, ni con permiso, y por eso no hay parámetro de sujeto.
func (h *Hub) NotificationPrefs(ctx context.Context) ([]HubNotificationPref, error) {
	out, err := JSON[struct {
		Prefs []HubNotificationPref `json:"prefs"`
	}](ctx, h.c, Request{Path: "/notifications/prefs"})
	return out.Prefs, err
}

// SetNotificationPref silencia —o vuelve a permitir— un origen, o un tipo dentro de él.
// El `kind` viaja siempre, vacío si no lo hay: es lo que manda la web.
func (h *Hub) SetNotificationPref(ctx context.Context, p HubNotificationPref) (HubNotificationPref, error) {
	cuerpo, err := json.Marshal(map[string]any{"source": p.Source, "kind": p.Kind, "muted": p.Muted})
	if err != nil {
		return HubNotificationPref{}, err
	}
	return JSON[HubNotificationPref](ctx, h.c, Request{
		Method: http.MethodPut, Path: "/notifications/prefs", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Apps son las claves de producto que ESTA instalación levanta (`GET /api/v1/apps`),
// con lo que el lanzador decide qué teselas enseña. Vacía es «no se sabe».
func (h *Hub) Apps(ctx context.Context) ([]string, error) {
	out, err := JSON[struct {
		Apps []string `json:"apps"`
	}](ctx, h.c, Request{Path: "/apps"})
	return out.Apps, err
}

// HubHit es un resultado de la búsqueda federada, ya normalizado por el Hub
// (`domain.SearchHit`): venga de Mail, de Drive o de Rooms, tiene esta forma.
type HubHit struct {
	Source  string `json:"source"`
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	// SizeBytes es el tamaño de un fichero: se enseña formateado en el idioma de la app
	// y no el Snippet, que el Hub escribe siempre igual («11.7 KiB»).
	SizeBytes *int64     `json:"size_bytes,omitempty"`
	URL       string     `json:"url"`
	Timestamp *time.Time `json:"timestamp"`
}

// HubOrigen es lo que ha contestado UN origen. `Error` no vacío es que ese producto no
// ha contestado: la búsqueda es federada y sigue con los demás, pero hay que DECIRLO —si
// Mail se cae y nadie lo dice, quien busca da por perdido un correo que sí existe—.
type HubOrigen struct {
	Source string   `json:"source"`
	Hits   []HubHit `json:"hits"`
	Error  string   `json:"error"`
}

// HubBusqueda es la respuesta de la búsqueda federada (`domain.SearchResponse`).
type HubBusqueda struct {
	Query   string      `json:"query"`
	Hits    []HubHit    `json:"hits"`
	Sources []HubOrigen `json:"sources"`
	Total   int         `json:"total"`
	HasMore bool        `json:"has_more"`
}

// Fallidos son los orígenes que no han contestado, en el orden en que los cuenta el Hub.
func (b HubBusqueda) Fallidos() []string {
	var out []string
	for _, s := range b.Sources {
		if s.Error != "" {
			out = append(out, s.Source)
		}
	}
	return out
}

// Search busca a la vez en todos los productos autorizados.
//
// Va por GET y no por el método QUERY que el Hub también monta: por GET el término viaja
// en la URL, que es lo que el handler lee cuando no hay cuerpo, y así no depende de que
// un proxy por medio sepa reenviar un método que no conoce.
func (h *Hub) Search(ctx context.Context, texto string, limite int) (HubBusqueda, error) {
	q := url.Values{"q": {texto}}
	if limite > 0 {
		q.Set("limit", strconv.Itoa(limite))
	}
	return JSON[HubBusqueda](ctx, h.c, Request{Path: "/search", Query: q})
}

// Recent es la ACTIVIDAD RECIENTE cross-app: el mismo abanico federado que Search pero
// SIN término, con lo último de cada producto ordenado por recencia
// (`GET /api/v1/recent`). Es lo que enseña la tarjeta de abajo de Sytue Hub.
//
// Degrada igual que la búsqueda: un origen caído no tumba a los demás y sale en
// `Sources` con su error, para que se pueda DECIR que faltan cosas.
func (h *Hub) Recent(ctx context.Context, limite int) (HubBusqueda, error) {
	return h.RecentDe(ctx, nil, limite)
}

// RecentDe es Recent acotado a unos productos (`sources=mail,drive`), que es como filtra
// la actividad de la web: así los veinte que se piden son veinte de ESOS productos, y
// no veinte de todos de los que luego se tira la mayoría.
func (h *Hub) RecentDe(ctx context.Context, origenes []string, limite int) (HubBusqueda, error) {
	q := url.Values{}
	if len(origenes) > 0 {
		q.Set("sources", strings.Join(origenes, ","))
	}
	if limite > 0 {
		q.Set("limit", strconv.Itoa(limite))
	}
	return JSON[HubBusqueda](ctx, h.c, Request{Path: "/recent", Query: q})
}

// HubFormato es en qué formato viajan las LISTAS de la copia de «Tus datos». Lo que ya
// tiene un formato estándar va en él pase lo que pase (los correos en .eml, las agendas
// en .ics, los contactos en .vcf, los archivos tal cual): esto solo decide los índices.
// Son los tres que acepta el servicio (platform/pkg/portabilidad.FormatoDe).
type HubFormato string

// Los tres formatos, en el orden en que los ofrece la web: CSV primero, que es el que
// abre cualquiera sin instalar nada.
const (
	HubCSV  HubFormato = "csv"
	HubJSON HubFormato = "json"
	HubXML  HubFormato = "xml"
)

// HubFormatos son los formatos que se pueden pedir, en el orden en que se enseñan.
var HubFormatos = []HubFormato{HubCSV, HubJSON, HubXML}

// Export abre la copia de TODO lo que la persona tiene en la suite: el derecho de
// portabilidad del RGPD (art. 20), no una función más. El ZIP lo arma Sytue Hub pidiendo
// a la vez su entrega a cada producto —correo, archivos, agenda, contactos, notas,
// documentos, fotos y mensajes— y lo sirve en flujo
// (services/hub/internal/http/export.go).
//
// Devuelve el cuerpo SIN LEER: la copia puede pesar gigas y no cabe en memoria. Quien
// llama lo cierra, y lo escribe conforme llega.
func (h *Hub) Export(ctx context.Context, f HubFormato) (io.ReadCloser, string, error) {
	resp, err := h.c.Do(ctx, Request{Path: "/export", Query: url.Values{"format": {string(f)}}})
	if err != nil {
		return nil, "", err
	}
	return resp.Body, nombreAdjunto(resp.Header.Get("Content-Disposition"), "sytue-one.zip"), nil
}

// nombreAdjunto saca el nombre que propone el servidor en Content-Disposition. El Hub
// mete ahí la organización y la fecha («sytue-one-corporate-2026-09-21.zip»), que es lo
// que distingue dos copias en la carpeta de descargas; sin cabecera válida, `respaldo`.
func nombreAdjunto(cabecera, respaldo string) string {
	_, params, err := mime.ParseMediaType(cabecera)
	if err != nil || params["filename"] == "" {
		return respaldo
	}
	return params["filename"]
}

// HubIntent es el resultado de un intent entre apps (`domain.IntentResult`): el enlace
// que lo materializa y la app que lo atiende. Con `Target` se abre en SU app
// (`AbrirURL`), y `URL` es el respaldo en la web.
type HubIntent struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Target string `json:"target"`
	Label  string `json:"label"`
}

// ResolveIntent le pide al Hub el enlace de un intent (`POST /api/v1/intents`): «crear
// un evento desde este correo» y compañía. El Hub revalida los parámetros y NO ejecuta
// nada: solo compone el enlace a la app destino, que es quien abre su formulario ya
// relleno y deja a la persona confirmar.
func (h *Hub) ResolveIntent(ctx context.Context, tipo string, params map[string]string) (HubIntent, error) {
	cuerpo, err := json.Marshal(map[string]any{"type": tipo, "params": params})
	if err != nil {
		return HubIntent{}, err
	}
	return JSON[HubIntent](ctx, h.c, Request{
		Method: http.MethodPost, Path: "/intents", Body: cuerpo,
		Header: http.Header{"Content-Type": {"application/json"}},
	})
}

// Instalador es un instalador nativo publicado (`GET /api/v1/instaladores`,
// services/hub/internal/instaladores): lo que el actualizador de escritorio necesita
// para saber si hay versión nueva y para comprobar lo que baja. En escritorio `Fichero`
// es el instalador del BUNDLE que lleva la app (`sytue-one-…`): el catálogo da una
// entrada por cada app de un bundle, todas con el mismo fichero.
type Instalador struct {
	App        string `json:"app"`
	Plataforma string `json:"plataforma"`
	// Arquitectura es el procesador (`amd64`, `arm64`, `universal`), o vacía si el
	// nombre del fichero no lo dice.
	Arquitectura string `json:"arquitectura,omitempty"`
	Fichero      string `json:"fichero"`
	Version      string `json:"version"`
	Bytes        int64  `json:"bytes"`
	SHA256       string `json:"sha256"`
	// URL firmada de corta vida; vacía cuando se baja por el propio Hub.
	URL string `json:"url,omitempty"`
}

// Instaladores lista los instaladores publicados para `plataforma` (macos, windows,
// linux, android), o los de todas si va vacía. Un 503 es que este despliegue no tiene
// bucket de instaladores.
func (h *Hub) Instaladores(ctx context.Context, plataforma string) ([]Instalador, error) {
	var q url.Values
	if plataforma != "" {
		q = url.Values{"plataforma": {plataforma}}
	}
	out, err := JSON[struct {
		Instaladores []Instalador `json:"instaladores"`
	}](ctx, h.c, Request{Path: "/instaladores", Query: q})
	return out.Instaladores, err
}

// SumasInstaladores es el SHA256SUMS de los instaladores publicados y su firma en
// base64 (vacía si no se firmó), tal como los sirve el Hub. Sin SHA256SUMS, un *Error
// con Status 404. Quien los usa los verifica con la clave embebida (apps/nucleo/firma).
func (h *Hub) SumasInstaladores(ctx context.Context) (sumas []byte, firma string, err error) {
	out, err := JSON[struct {
		Sumas string `json:"sumas"`
		Firma string `json:"firma"`
	}](ctx, h.c, Request{Path: "/instaladores/sumas"})
	return []byte(out.Sumas), out.Firma, err
}

// Los motivos por los que `/descargas` no enseña ningún instalador.
const (
	DescargasSinConfigurar   = "unconfigured"
	DescargasSinInstaladores = "no_installers"
)

// HubDescargas es lo que enseña la página `/descargas`: cada instalador UNA vez —el
// catálogo repite el de un bundle por cada app que lleva—, la versión publicada y, si no
// hay nada, por qué.
type HubDescargas struct {
	Version      string
	Instaladores []Instalador
	Motivo       string
}

// codigoSinInstaladores es el del 503 de un Hub sin bucket de instaladores
// (services/hub/internal/http/instaladores.go).
const codigoSinInstaladores = "installers_unconfigured"

// Descargas es la lista de la página `/descargas` del portal, que una app nativa no
// puede pedir al almacenamiento porque no lleva su credencial: la pide al mismo catálogo
// que el actualizador, con el token. Un despliegue sin bucket es «sin configurar», como
// en la web, y no un fallo; un bucket que no contesta sí lo es: «inténtalo más tarde».
func (h *Hub) Descargas(ctx context.Context) (HubDescargas, error) {
	todos, err := h.Instaladores(ctx, "")
	if e := (*Error)(nil); errors.As(err, &e) && e.Code() == codigoSinInstaladores {
		return HubDescargas{Instaladores: []Instalador{}, Motivo: DescargasSinConfigurar}, nil
	}
	if err != nil {
		return HubDescargas{}, err
	}
	d := HubDescargas{Instaladores: []Instalador{}}
	for _, i := range todos {
		if slices.ContainsFunc(d.Instaladores, func(o Instalador) bool { return o.Fichero == i.Fichero }) {
			continue
		}
		d.Instaladores = append(d.Instaladores, i)
		// ponytail: la del primero que la diga; traer-instaladores.sh los sube todos de
		// la misma release.
		if d.Version == "" && i.Version != "" {
			d.Version = "v" + i.Version
		}
	}
	if len(d.Instaladores) == 0 {
		d.Motivo = DescargasSinInstaladores
	}
	return d, nil
}

// Descargar abre UN instalador de `Descargas` para guardarlo, con su nombre. Antes vuelve
// a pedir los de su plataforma: la URL firmada vive cinco minutos y la página puede
// llevar abierta más (la web enlaza a su BFF por lo mismo).
func (h *Hub) Descargar(ctx context.Context, i Instalador) (io.ReadCloser, string, error) {
	frescos, err := h.Instaladores(ctx, i.Plataforma)
	if err != nil {
		return nil, "", err
	}
	for _, f := range frescos {
		if f.Fichero == i.Fichero {
			resp, err := h.abrirInstalador(ctx, f)
			if err != nil {
				return nil, "", err
			}
			return resp.Body, f.Fichero, nil
		}
	}
	return nil, "", &Error{Status: http.StatusNotFound, URL: i.Fichero}
}

// BajarInstalador escribe el instalador en `w` conforme llega. Por su URL firmada si la
// trae, SIN token: la firma ya autoriza, y el almacén rechaza una petición con dos
// mecanismos de autenticación. Si no, por el Hub, con el token. Comprobar la suma es
// cosa de quien llama, que es quien sabe dónde lo ha dejado.
func (h *Hub) BajarInstalador(ctx context.Context, i Instalador, w io.Writer) error {
	resp, err := h.abrirInstalador(ctx, i)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, err = io.Copy(w, resp.Body)
	return err
}

// abrirInstalador es la respuesta de un 200 con el instalador, sin leer.
func (h *Hub) abrirInstalador(ctx context.Context, i Instalador) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	if i.URL != "" {
		req, errReq := http.NewRequestWithContext(ctx, http.MethodGet, i.URL, http.NoBody)
		if errReq != nil {
			return nil, errReq
		}
		resp, err = h.c.http.Do(req)
	} else {
		resp, err = h.c.Do(ctx, Request{Path: "/instaladores/" + url.PathEscape(i.Fichero)})
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, &Error{Status: resp.StatusCode, URL: i.Fichero}
	}
	return resp, nil
}

// OrgVisible es cómo se enseña la organización de fuera: su nombre visible, o su
// identificador si Sytue ID no lo da. Lo comparten la campana y las bandejas de
// invitaciones de Rooms y Calendar (nombreDeOrganizacion en la web).
func OrgVisible(slug, nombre string) string {
	if nombre != "" {
		return nombre
	}
	return slug
}
