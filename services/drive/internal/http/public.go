// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Endpoint público por enlace (kind='link'). Lo sirve el servicio Go directamente
// (sin auth; el token es el secreto), no el BFF de Astro. Cubre tres casos:
//
//   - Enlace a FICHERO: descarga directa (attachment, nosniff), como antes.
//   - Enlace a CARPETA: página HTML navegable (subcarpetas + ficheros) con migas
//     de pan; cada subruta se resuelve de nuevo desde la raíz del share (no puede
//     escapar del subárbol compartido). Los ficheros se descargan como attachment.
//   - Enlace con CONTRASEÑA: prompt HTML; al acertar se fija una cookie de prueba
//     (SHA-256 del hash, derivada por el servicio) acotada a /public/{token}.
//
// HTML renderizado con html/template (autoescapado); sin librerías de terceros.
package drivehttp

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/drive/internal/domain"
	"git.sytue.eu/sytue/one/services/drive/internal/service"
)

// publicCookiePrefix nombra la cookie de prueba de contraseña. El valor se compara
// siempre contra la prueba real del share, así que el nombre solo evita colisiones
// entre pestañas; el aislamiento fuerte lo da Path=/public/{token}.
const publicCookiePrefix = "sd_pw_"

// publicCookieMaxAge es la vida de la cookie de prueba (re-pedir contraseña tras
// caducar). ponytail: fija; atarla a expires_at del share si se pide.
const publicCookieMaxAge = 6 * time.Hour

func publicCookieName(token string) string {
	n := token
	if len(n) > 16 {
		n = n[:16]
	}
	return publicCookiePrefix + n
}

// DoS del desbloqueo por contraseña (única superficie sin auth): PublicVerify corre
// Argon2id (m=64 MiB, t=3) por intento y el router de Drive no tiene rate-limit. Dos
// verjas cierran (a) el agotamiento de memoria por floods concurrentes y (b) la
// fuerza bruta online de contraseñas de enlace, sin depender de la IP (spoofeable
// tras proxy) — la clave natural es el propio token del enlace:
//
//   - publicUnlockLimiter: token-bucket por TOKEN (no por IP), así acota los intentos
//     sobre un enlace concreto aunque el atacante rote IPs/XFF. En memoria (por
//     réplica); suficiente porque el vector es martillear UN enlace vivo conocido.
//   - publicUnlockSem: semáforo global que acota los Argon2id SIMULTÁNEOS (techo de
//     memoria = 8 × 64 MiB). El flood encola en goroutines baratas; solo N pagan la
//     memoria. ponytail: 8 fijo (unlock es raro y hay varias réplicas); subir/atar al
//     presupuesto de memoria del pod si hiciera falta.
var (
	publicUnlockLimiter = httpx.NewTokenBucket(1, 10)
	publicUnlockSem     = make(chan struct{}, 8)
)

// claveDelLimitador es la clave del token en el limitador: su hash, de tamaño
// fijo. Con el token tal cual, cada intento con un token inventado y largo dejaba
// en memoria su cadena entera durante diez minutos (FILE-13).
func claveDelLimitador(token string) string {
	suma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(suma[:])
}

// PublicEntry sirve la raíz de un enlace público (GET /public/{token}).
func (h *Handlers) PublicEntry(w http.ResponseWriter, r *http.Request) {
	h.servePublic(w, r, strings.TrimSpace(chi.URLParam(r, "token")), "")
}

// PublicPath sirve una subruta de un enlace a carpeta (GET /public/{token}/*):
// navega a una subcarpeta (listado) o descarga un fichero del subárbol compartido.
func (h *Handlers) PublicPath(w http.ResponseWriter, r *http.Request) {
	h.servePublic(w, r, strings.TrimSpace(chi.URLParam(r, "token")), chi.URLParam(r, "*"))
}

// servePublic resuelve el enlace (con verja de contraseña) y despacha: prompt,
// descarga de fichero o listado de carpeta.
func (h *Handlers) servePublic(w http.ResponseWriter, r *http.Request, token, subpath string) {
	if token == "" {
		h.renderPublicError(w, http.StatusNotFound)
		return
	}
	proof := publicProofFromCookie(r, token)
	share, tenant, err := h.svc.PublicResolve(r.Context(), token, proof)
	if errors.Is(err, domain.ErrPasswordRequired) {
		h.renderPasswordForm(w, token, false)
		return
	}
	if err != nil {
		h.renderPublicError(w, publicStatus(err))
		return
	}
	node, err := h.svc.PublicBrowse(r.Context(), share, tenant, subpath)
	if err != nil {
		h.renderPublicError(w, publicStatus(err))
		return
	}
	if !node.IsDir {
		defer node.Body.Close()
		writePublicFile(w, node.File, node.Body)
		return
	}
	h.renderListing(w, token, subpath, node)
}

// PublicUnlock recibe la contraseña de un enlace protegido (POST /public/{token}),
// la verifica y, si acierta, fija la cookie de prueba y redirige a la raíz.
func (h *Handlers) PublicUnlock(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(chi.URLParam(r, "token"))
	if token == "" {
		h.renderPublicError(w, http.StatusNotFound)
		return
	}
	_ = r.ParseForm()
	// Anti-fuerza-bruta: rate-limit por token antes de tocar Argon2id.
	if res, _ := publicUnlockLimiter.Allow(r.Context(), claveDelLimitador(token)); !res.Allowed {
		if res.RetryAfter > 0 {
			secs := int(res.RetryAfter.Seconds())
			if secs < 1 {
				secs = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(secs))
		}
		h.renderPublicError(w, http.StatusTooManyRequests)
		return
	}
	// Anti-OOM: acota los Argon2id concurrentes. El flood espera en goroutines
	// baratas; solo N pagan los 64 MiB. Si sigue saturado a los 2s, 429 (no encolar
	// más memoria). El RequestTimeout del router (60s) cubre la espera.
	select {
	case publicUnlockSem <- struct{}{}:
		defer func() { <-publicUnlockSem }()
	case <-time.After(2 * time.Second):
		h.renderPublicError(w, http.StatusTooManyRequests)
		return
	}
	proof, err := h.svc.PublicVerify(r.Context(), token, r.PostFormValue("password"))
	if errors.Is(err, domain.ErrWrongPassword) {
		h.renderPasswordForm(w, token, true)
		return
	}
	if err != nil {
		h.renderPublicError(w, publicStatus(err))
		return
	}
	if proof != "" {
		secure := strings.HasPrefix(h.cfg.PublicURL, "https")
		httpx.SetSecureCookie(w, httpx.CookieOptions{
			Name:   publicCookieName(token),
			Value:  proof,
			Path:   "/public/" + token,
			MaxAge: publicCookieMaxAge,
			// Lax (no Strict): así el enlace abierto desde un email/chat externo no
			// re-pide contraseña al volver. Sigue bloqueando POST/subrecursos cross-site.
			SameSite: http.SameSiteLaxMode,
			Secure:   &secure,
		})
	}
	http.Redirect(w, r, "/public/"+url.PathEscape(token), http.StatusSeeOther)
}

func publicProofFromCookie(r *http.Request, token string) string {
	c, err := r.Cookie(publicCookieName(token))
	if err != nil {
		return ""
	}
	return c.Value
}

// publicStatus mapea un error de dominio al código de la página pública. No filtra
// detalle: cualquier "no está" es 404; el resto, 500.
func publicStatus(err error) int {
	switch {
	case errors.Is(err, domain.ErrShareExpired), errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// writePublicFile transmite un fichero como descarga (attachment, nosniff): nunca
// inline, para no renderizar HTML/SVG hostiles en el origen del Drive.
func writePublicFile(w http.ResponseWriter, f domain.File, body io.Reader) {
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
	w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+escapeFilename(f.Name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, body)
}

// ---------------------------------------------------------------------------
// Render HTML
// ---------------------------------------------------------------------------

type publicCrumb struct {
	Name string
	Href template.URL
}

type publicItem struct {
	Name string
	Href template.URL
	Size string // solo ficheros
}

type listingData struct {
	Title   string
	Crumbs  []publicCrumb
	Folders []publicItem
	Files   []publicItem
	Empty   bool
	More    bool
}

type passwordData struct {
	Action template.URL
	Wrong  bool
}

func (h *Handlers) renderListing(w http.ResponseWriter, token, subpath string, node service.PublicNode) {
	segs := publicSegments(subpath)
	data := listingData{
		Title:  publicShareTitle(node.Folder),
		Crumbs: buildCrumbs(token, node.Folder, segs),
		More:   node.More,
	}
	for _, sub := range node.Subfolders {
		data.Folders = append(data.Folders, publicItem{Name: sub.Name, Href: publicHref(token, appendSeg(segs, sub.Name))})
	}
	for _, f := range node.Files {
		data.Files = append(data.Files, publicItem{
			Name: f.Name,
			Href: publicHref(token, appendSeg(segs, f.Name)),
			Size: formatBytes(f.SizeBytes),
		})
	}
	data.Empty = len(data.Folders) == 0 && len(data.Files) == 0
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	_ = publicTmpl.ExecuteTemplate(w, "listing", data)
}

func (h *Handlers) renderPasswordForm(w http.ResponseWriter, token string, wrong bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusUnauthorized)
	_ = publicTmpl.ExecuteTemplate(w, "password", passwordData{
		Action: template.URL("/public/" + url.PathEscape(token)), //nolint:gosec // G203: token percent-encoded (url.PathEscape), no HTML.
		Wrong:  wrong,
	})
}

func (h *Handlers) renderPublicError(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = publicTmpl.ExecuteTemplate(w, "error", nil)
}

// publicSegments parte una subruta (ya URL-decodificada por el router) en nombres.
func publicSegments(subpath string) []string {
	out := []string{}
	for _, s := range strings.Split(subpath, "/") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// appendSeg devuelve una copia de segs con name al final (sin aliasar el slice base).
func appendSeg(segs []string, name string) []string {
	out := make([]string, 0, len(segs)+1)
	out = append(out, segs...)
	return append(out, name)
}

// publicHref construye /public/<token>/<seg>/… con cada segmento percent-encoded.
func publicHref(token string, segs []string) template.URL {
	var b strings.Builder
	b.WriteString("/public/")
	b.WriteString(url.PathEscape(token))
	for _, s := range segs {
		b.WriteByte('/')
		b.WriteString(url.PathEscape(s))
	}
	return template.URL(b.String()) //nolint:gosec // G203: segmentos percent-encoded, no HTML.
}

func buildCrumbs(token string, folder domain.Folder, segs []string) []publicCrumb {
	crumbs := make([]publicCrumb, 0, 1+len(segs))
	crumbs = append(crumbs, publicCrumb{Name: publicShareTitle(folder), Href: publicHref(token, nil)})
	acc := make([]string, 0, len(segs))
	for _, s := range segs {
		acc = append(acc, s)
		crumbs = append(crumbs, publicCrumb{Name: s, Href: publicHref(token, acc)})
	}
	return crumbs
}

func publicShareTitle(folder domain.Folder) string {
	if strings.TrimSpace(folder.Name) != "" {
		return folder.Name
	}
	return "Carpeta compartida"
}

// formatBytes formatea un tamaño legible (mismo criterio que el front).
func formatBytes(n int64) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n) / 1024
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if v < 10 {
		return strconv.FormatFloat(v, 'f', 1, 64) + " " + units[i]
	}
	return strconv.FormatFloat(v, 'f', 0, 64) + " " + units[i]
}

// publicTmpl agrupa las vistas HTML del enlace público. Sobrio, blanco, hairlines
// 1px; sin dependencias ni activos externos (todo inline). Autoescapado.
var publicTmpl = template.Must(template.New("public").Parse(`
{{define "style"}}<style>
:root{color-scheme:light dark}
*{box-sizing:border-box}
body{margin:0;font:15px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:#1a1d21;background:#f7f8fa}
.wrap{max-width:720px;margin:0 auto;padding:32px 20px}
.card{background:#fff;border:1px solid #e3e6ea;border-radius:10px;overflow:hidden}
.brand{font-weight:700;letter-spacing:-.01em;color:#1a1d21;margin:0 0 20px;font-size:15px}
.brand span{color:#3d5afe}
h1{font-size:18px;margin:0;font-weight:650;letter-spacing:-.01em}
nav{font-size:13px;margin:0 0 16px;color:#6b7280;display:flex;flex-wrap:wrap;gap:2px;align-items:center}
nav a{color:#3d5afe;text-decoration:none}
nav a:hover{text-decoration:underline}
nav .sep{color:#c3c8ce;padding:0 2px}
ul{list-style:none;margin:0;padding:0}
li a{display:flex;align-items:center;gap:12px;padding:12px 16px;border-top:1px solid #eef0f2;color:inherit;text-decoration:none}
li:first-child a{border-top:0}
li a:hover{background:#f4f6f8}
.ico{width:20px;height:20px;flex:0 0 auto;color:#8a9099}
.nm{flex:1;min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;font-weight:500}
.sz{color:#9aa0a8;font-size:13px;font-variant-numeric:tabular-nums}
.empty,.note{padding:16px;color:#6b7280;font-size:14px}
.note{border-top:1px solid #eef0f2}
form{padding:20px}
label{display:block;font-size:13px;font-weight:600;margin:0 0 6px}
input[type=password]{width:100%;padding:9px 11px;border:1px solid #cfd4da;border-radius:8px;font:inherit;background:#fff;color:inherit}
input[type=password]:focus{outline:2px solid #3d5afe;outline-offset:0;border-color:#3d5afe}
button{margin-top:14px;width:100%;padding:9px 14px;border:0;border-radius:8px;background:#3d5afe;color:#fff;font:inherit;font-weight:600;cursor:pointer}
button:hover{background:#3550e0}
.err{color:#c0362c;font-size:13px;margin:10px 0 0}
.msg{color:#6b7280;font-size:14px;margin:0 0 16px}
@media(prefers-color-scheme:dark){
body{color:#e6e8ea;background:#16181c}
.card{background:#1e2126;border-color:#2b2f36}
.brand,h1{color:#f2f3f5}
li a{border-color:#282c32}
li a:hover{background:#24272d}
.note{border-color:#282c32}
input[type=password]{background:#16181c;border-color:#3a3f47;color:#e6e8ea}
}
</style>{{end}}

{{define "listing"}}<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">
<title>{{.Title}} · Sytue Drive</title>{{template "style"}}</head><body><div class="wrap">
<p class="brand">Sytue <span>Drive</span></p>
<nav aria-label="Ruta">{{range $i,$c := .Crumbs}}{{if $i}}<span class="sep">/</span>{{end}}<a href="{{$c.Href}}">{{$c.Name}}</a>{{end}}</nav>
<div class="card">
{{if .Empty}}<p class="empty">Esta carpeta está vacía.</p>{{else}}<ul>
{{range .Folders}}<li><a href="{{.Href}}"><svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg><span class="nm">{{.Name}}</span><span class="sz">Carpeta</span></a></li>{{end}}
{{range .Files}}<li><a href="{{.Href}}"><svg class="ico" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M14 3v5h5"/><path d="M14 3H6a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/></svg><span class="nm">{{.Name}}</span><span class="sz">{{.Size}}</span></a></li>{{end}}
</ul>{{if .More}}<p class="note">Se muestran los primeros elementos de esta carpeta.</p>{{end}}{{end}}
</div></div></body></html>{{end}}

{{define "password"}}<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">
<title>Enlace protegido · Sytue Drive</title>{{template "style"}}</head><body><div class="wrap">
<p class="brand">Sytue <span>Drive</span></p>
<div class="card"><form method="post" action="{{.Action}}">
<h1>Enlace protegido</h1>
<p class="msg">Introduce la contraseña para acceder a este contenido compartido.</p>
<label for="pw">Contraseña</label>
<input id="pw" type="password" name="password" autocomplete="current-password" autofocus required>
{{if .Wrong}}<p class="err">Contraseña incorrecta. Inténtalo de nuevo.</p>{{end}}
<button type="submit">Acceder</button>
</form></div></div></body></html>{{end}}

{{define "error"}}<!doctype html><html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1"><meta name="robots" content="noindex">
<title>Enlace no disponible · Sytue Drive</title>{{template "style"}}</head><body><div class="wrap">
<p class="brand">Sytue <span>Drive</span></p>
<div class="card"><div style="padding:28px 20px">
<h1>Este enlace no está disponible</h1>
<p class="msg" style="margin-top:10px">El enlace no existe, ha caducado o se ha revocado.</p>
</div></div></div></body></html>{{end}}
`))
