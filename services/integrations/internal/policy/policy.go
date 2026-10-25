// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package policy es el corazón del Sytue Integration Gateway: la ÚNICA fábrica del
// *http.Client con el que un driver sale a la red. No hay otra forma de construir
// un cliente de egress —esa es la garantía de seguridad por construcción—.
//
// BuildRuntime encadena, sobre el transporte SSRF-safe de platform/pkg/ssrfhttp,
// un http.RoundTripper que en CADA petición saliente:
//
//	(1) re-valida el destino con ssrfhttp (barrera 1: nunca un host interno);
//	(2) aplica rate-limit compartido entre réplicas con clave "org:instance";
//	(3) audita el egress (host/método/status/bytes/latencia; Authorization
//	    REDACTADO; jamás payloads);
//	(4) ejecuta el hook de política/DLP (no-op ahora; W8 lo rellena);
//	(5) inyecta la auth (Bearer del vault) clonando la petición, de modo que el
//	    driver nunca ve —ni fija— el token.
//
// El cliente resultante NO sigue redirects que cambien de host: como la auth se
// inyecta en cada salto, un redirect cross-host filtraría el token; se corta.
package policy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/ssrfhttp"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// ErrRateLimited indica que la instancia agotó su presupuesto de egress.
var ErrRateLimited = errors.New("policy: rate limit de egress excedido")

// ErrHostNotAllowed indica que el destino no pertenece a los hosts oficiales declarados
// en el manifiesto del proveedor.
var ErrHostNotAllowed = errors.New("policy: destino fuera de los hosts oficiales del manifiesto")

// AuditSink persiste una fila de egress_audit. Lo satisface la capa store; se
// declara aquí como interfaz para no acoplar policy a la persistencia (y poder
// inyectar un doble en los tests).
type AuditSink interface {
	RecordEgress(ctx context.Context, rec domain.EgressAudit) error
}

// sinCredencial marca una petición a la que la cadena NO debe añadir el Bearer.
type sinCredencial struct{}

// SinCredencial pide que el egress de esta petición no lleve el token de la
// instancia. Es para las URL PRE-AUTENTICADAS de los proveedores (la sesión de
// subida de OneDrive, una descarga firmada): viven en otro host, la autorización va
// dentro de la URL y un Authorization de más hace que Graph conteste 401.
func SinCredencial(ctx context.Context) context.Context {
	return context.WithValue(ctx, sinCredencial{}, true)
}

// Hook es el punto de extensión de política/DLP sobre una petición saliente. Un
// error aborta el egress ANTES de conectar. Nil = sin hook.
type Hook func(req *http.Request) error

// RuntimeParams parametriza la construcción de un Runtime para una instancia.
type RuntimeParams struct {
	OrgID      string
	InstanceID string
	// Bearer es el token a inyectar como "Authorization: Bearer …". Vacío = sin
	// inyección. Lo extrae el llamante del vault (Secrets); el driver nunca lo ve.
	Bearer string
	// Limiter es el limitador de egress (clave "org:instance"). Nil = sin límite.
	Limiter httpx.RateLimiter
	// Audit recibe cada fila de egress. Nil = sin auditoría (solo tests).
	Audit AuditSink
	// Policy es el motor de gobernanza (DLP L5). Cuando no es nil, deriva el hook de
	// política de la org y lo enchufa en el RoundTripper: la evaluación allow|warn|
	// block ocurre DENTRO de la única cadena de egress. Tiene prioridad sobre DLP.
	Policy *Engine
	// APIFamily identifica la familia de API del destino (== template_key de la
	// instancia); es uno de los metadatos que evalúan las reglas de Policy.
	APIFamily string
	// DLP es el hook de política de bajo nivel (punto de extensión / tests). Nil =
	// no-op. Si Policy != nil, se usa el hook del motor en su lugar.
	DLP Hook
	// Internal marca que el destino es un SERVICIO DE LA PROPIA SUITE (Sytue Drive,
	// Sytue ID), no un tercero: se omiten las DOS barreras SSRF —la validación previa
	// del destino y el dial a IP validada— y se conserva TODO lo demás (rate-limit,
	// auditoría, política/DLP, redirects same-host acotados).
	//
	// Por qué: la barrera SSRF bloquea —con razón— los hosts internos, y los servicios
	// de la suite viven exactamente ahí (http://drive:8082). Es el mismo criterio con el
	// que internal/directory e internal/migrate hablan con Sytue ID y Sytue Drive por un
	// http.Client normal. Lo que lo hace seguro NO es la confianza en el driver, sino
	// que el llamante solo puede activarlo con destinos que salen de la CONFIGURACIÓN
	// DEL SERVICIO (ID_URL/DRIVE_URL); un valor tecleado por un administrador jamás
	// llega aquí, así que no hay forma de convertir esto en un SSRF.
	Internal bool
	// AllowedHosts restringe el egreso a los hosts oficiales declarados en el manifiesto
	// del proveedor (exactos o comodín *.dominio). Vacío = sin lista (plantillas que no
	// declaran hosts propios).
	AllowedHosts []string
	// Log es el logger de la instancia (etiquetado por el llamante).
	Log *slog.Logger
	// Timeout total por petición (<=0 ⇒ 15s). MaxRedirects (<=0 ⇒ 5).
	Timeout      time.Duration
	MaxRedirects int
}

// BuildRuntime fabrica el connector.Runtime de una instancia: el único cliente de
// egress, con toda la cadena de políticas aplicada.
func BuildRuntime(p RuntimeParams) connector.Runtime {
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	// El motor de gobernanza, si está cableado, produce el hook de política de la
	// org; es el enforcement L5 imposible de esquivar (vive en el único RoundTripper).
	dlp := p.DLP
	if p.Policy != nil {
		dlp = p.Policy.Hook(p.OrgID, p.InstanceID, p.APIFamily)
	}
	// Barrera 2 (SafeDialContext: dial a IP validada) para TODO destino de terceros.
	// Un servicio de la propia suite vive en una IP privada, así que la barrera lo
	// bloquearía; ahí se usa el transporte normal y se conserva el resto de la cadena.
	// Lo que lo hace seguro no es confiar en el driver, sino que Internal solo se activa
	// con destinos que salen de la CONFIGURACIÓN DEL SERVICIO, nunca de un valor tecleado.
	base := http.RoundTripper(ssrfhttp.Transport())
	if p.Internal {
		base = http.DefaultTransport
	}
	tr := &transport{
		base:         base,
		internal:     p.Internal,
		orgID:        p.OrgID,
		instanceID:   p.InstanceID,
		bearer:       p.Bearer,
		allowedHosts: p.AllowedHosts,
		limiter:      p.Limiter,
		audit:        p.Audit,
		dlp:          dlp,
		log:          log,
	}
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	maxRedirects := p.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 5
	}
	client := &http.Client{
		Timeout:       timeout,
		Transport:     tr,
		CheckRedirect: redirectPolicy(maxRedirects, p.Internal, p.AllowedHosts),
	}
	return connector.Runtime{HTTP: client, Log: log}
}

// redirectPolicy acota el número de saltos, re-valida cada destino por la barrera
// SSRF, prohíbe cambiar de host y prohíbe BAJAR de https a http (las dos formas de
// filtrar el Bearer inyectado). Con internal, la re-validación se omite —el destino es
// un servicio de la suite, en IP privada— pero el tope de saltos, el same-host y el
// no-downgrade se mantienen: son los que impiden que un redirect saque el token fuera.
func redirectPolicy(maxRedirects int, internal bool, allowedHosts []string) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return errors.New("policy: demasiados redirects")
		}
		if len(via) > 0 && !equalHost(req, via[0]) {
			return fmt.Errorf("policy: redirect cross-host bloqueado (%s → %s)",
				via[0].URL.Hostname(), req.URL.Hostname())
		}
		// La auth se inyecta en CADA salto (ver transport.RoundTrip), así que un 302 del
		// tercero hacia http:// en su propio host pondría el token del vault en claro en
		// la red. ssrfhttp.ValidateURL no lo para: http es un esquema válido para él, y
		// la regla de same-host tampoco, porque el host no cambia. El sentido contrario
		// (http → https) es la redirección más común que existe y sigue permitida.
		if len(via) > 0 && via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return fmt.Errorf("policy: redirect que baja de https a %q bloqueado (filtraría el token en claro)",
				req.URL.Scheme)
		}
		if internal {
			return nil
		}
		if len(allowedHosts) > 0 && !hostPermitido(req.URL.Hostname(), allowedHosts) {
			return fmt.Errorf("%w: %s", ErrHostNotAllowed, req.URL.Hostname())
		}
		return ssrfhttp.ValidateURL(req.Context(), req.URL.String())
	}
}

func equalHost(a, b *http.Request) bool { return a.URL.Hostname() == b.URL.Hostname() }

func hostPermitido(host string, oficiales []string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	for _, patron := range oficiales {
		p := strings.ToLower(strings.TrimSpace(patron))
		if p == h {
			return true
		}
		if sufijo, ok := strings.CutPrefix(p, "*."); ok && sufijo != "" && strings.HasSuffix(h, "."+sufijo) {
			return true
		}
	}
	return false
}

// transport es el http.RoundTripper encadenado. Envuelve el transporte SSRF-safe
// (base) y aplica la cadena de políticas en cada RoundTrip.
type transport struct {
	base         http.RoundTripper
	internal     bool
	orgID        string
	instanceID   string
	bearer       string
	allowedHosts []string
	limiter      httpx.RateLimiter
	audit        AuditSink
	dlp          Hook
	log          *slog.Logger
}

// RoundTrip aplica la cadena y delega en el transporte base. No muta la petición
// original (clona antes de inyectar auth), conforme al contrato de RoundTripper.
func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// (1) Barrera SSRF 1: re-validar el destino antes de conectar. Se omite —como la
	// barrera 2— cuando el destino es un servicio de la PROPIA suite: vive en una IP
	// privada y la barrera lo rechazaría aquí aunque el transporte base ya lo permita.
	// Sin esto, Internal no servía de nada: el egress moría antes de dialar.
	if !t.internal {
		if err := ssrfhttp.ValidateURL(req.Context(), req.URL.String()); err != nil {
			return nil, err
		}
		if len(t.allowedHosts) > 0 && !hostPermitido(req.URL.Hostname(), t.allowedHosts) {
			return nil, fmt.Errorf("%w: %s", ErrHostNotAllowed, req.URL.Hostname())
		}
	}
	// (2) Rate-limit por org:instance.
	if t.limiter != nil {
		res, err := t.limiter.Allow(req.Context(), t.key())
		if err != nil {
			return nil, fmt.Errorf("policy: rate limit: %w", err)
		}
		if !res.Allowed {
			return nil, ErrRateLimited
		}
	}
	// (4) Hook de política/DLP (no-op por defecto).
	if t.dlp != nil {
		if err := t.dlp(req); err != nil {
			return nil, err
		}
	}
	// (5) Inyección de auth: clonar y añadir el Bearer (el driver nunca lo ve).
	//
	// La cabecera que el driver YA fijó gana. Los protocolos que no van con Bearer
	// —Jira con correo+API token, ServiceNow con usuario+contraseña— componen su propia
	// cabecera Basic, y pisarla los rompía en cuanto la instancia guardaba un `bearer`
	// para otra de sus capacidades (p. ej. el token de aprovisionamiento). El secreto
	// convencional `bearer` es el que inyecta la pasarela; cualquier otra clave del vault
	// va al driver, que decide cómo autenticarse.
	out := req
	authPresent := req.Header.Get("Authorization") != ""
	if t.bearer != "" && !authPresent && req.Context().Value(sinCredencial{}) == nil {
		out = req.Clone(req.Context())
		out.Header.Set("Authorization", "Bearer "+t.bearer)
		authPresent = true
	}

	start := time.Now()
	resp, err := t.base.RoundTrip(out)
	// (3) Auditar el egress (best-effort; el egress ya ocurrió).
	t.record(req, resp, err, authPresent, time.Since(start))
	return resp, err
}

// key es la clave del bucket de rate-limit: una por instancia dentro de la org.
func (t *transport) key() string { return t.orgID + ":" + t.instanceID }

// record escribe la fila de auditoría. Nunca persiste el valor de Authorization
// (solo la marca "REDACTED") ni el cuerpo. Un fallo se registra y se ignora: la
// petición ya salió, romperla por un fallo de auditoría no aporta.
func (t *transport) record(req *http.Request, resp *http.Response, callErr error, authPresent bool, lat time.Duration) {
	if t.audit == nil {
		return
	}
	rec := domain.EgressAudit{
		OrgID:      t.orgID,
		InstanceID: t.instanceID,
		Host:       req.URL.Hostname(),
		Method:     req.Method,
		ReqBytes:   nonNeg(req.ContentLength),
		LatencyMS:  lat.Milliseconds(),
		OccurredAt: time.Now().UTC(),
	}
	if authPresent {
		rec.Auth = "REDACTED"
	}
	if resp != nil {
		rec.Status = resp.StatusCode
		rec.RespBytes = nonNeg(resp.ContentLength)
	}
	if callErr != nil {
		rec.Err = callErr.Error()
	}
	// Contexto propio: la auditoría debe persistir aunque el ctx de la petición se
	// cancele tras responder. RecordEgress fija el tenant desde rec.OrgID.
	// ponytail: escritura síncrona por egress; si el volumen aprieta, batch/async.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
	defer cancel()
	if err := t.audit.RecordEgress(ctx, rec); err != nil {
		t.log.Warn("policy: no se pudo auditar el egress", "error", err, "host", logsafe.Text(rec.Host))
	}
}

// nonNeg convierte un Content-Length desconocido (-1) en 0.
func nonNeg(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}
