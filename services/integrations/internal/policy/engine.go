// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package policy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// ErrBlockedByPolicy es el error que devuelve el hook cuando una regla `block`
// casa: el request NO llega al transporte subyacente. El despacho HTTP lo mapea a
// 403 (lo prohíbe la organización), no a un fallo de red.
var ErrBlockedByPolicy = errors.New("policy: egress bloqueado por política de la organización")

// bodyPeekCap acota cuánto cuerpo inspecciona una regla body_contains: un tope
// duro para que la inspección sea BARATA. Solo se lee cuando alguna regla lo pide
// Y el cuerpo es re-leíble (GetBody != nil); el req.Body real nunca se consume.
//
// ponytail: substring acotado sobre el prefijo del cuerpo — NO es inspección
// profunda (nada de descompresión, parseo ni regex/ReDoS). Es el alcance honesto
// de "patrones simples si es barato y seguro" del Día 1; la clasificación de
// contenido real es trabajo futuro. Las reglas por METADATOS son la garantía dura.
const bodyPeekCap = 64 << 10

// Loader carga las reglas HABILITADAS de una org (orden estable). Lo
// satisface la capa store; se declara como interfaz para no acoplar el motor a la
// persistencia y poder inyectar un doble en los tests.
type Loader interface {
	LoadPolicies(ctx context.Context, orgID string) ([]domain.Policy, error)
}

// DecisionSink persiste una decisión de política (block/warn). Lo satisface la
// capa store; interfaz por la misma razón que PolicyLoader.
type DecisionSink interface {
	RecordDecision(ctx context.Context, d domain.PolicyDecision) error
}

// rule es la forma COMPILADA de una domain.Policy: la condición ya parseada y
// normalizada, lista para evaluar sin tocar JSON en el camino caliente.
type rule struct {
	id     string
	name   string
	action domain.PolicyAction
	// Condición (campos vacíos = comodín, AND-combinados).
	apiFamily    string // == template_key de la instancia
	hostSuffix   string // exacto, o sufijo si empieza por '.' (".example.com")
	method       string // método HTTP en mayúsculas
	pathPrefix   string // prefijo de la ruta de la URL
	bodyContains string // substring sobre el prefijo del cuerpo (best-effort)
}

// condition es la forma en JSON de rule (columna policies.condition).
type condition struct {
	APIFamily    string `json:"api_family"`
	Host         string `json:"host"`
	Method       string `json:"method"`
	PathPrefix   string `json:"path_prefix"`
	BodyContains string `json:"body_contains"`
}

// compile traduce las políticas persistidas a reglas evaluables. Una condición
// ilegible degrada a comodín total con su acción: preferimos aplicar la acción
// declarada (típicamente block) de más que dejar de aplicarla por un JSON roto.
func compile(pols []domain.Policy) []rule {
	rules := make([]rule, 0, len(pols))
	for _, p := range pols {
		var c condition
		_ = json.Unmarshal(p.Condition, &c) // condición vacía/rota ⇒ comodín.
		rules = append(rules, rule{
			id:           p.ID,
			name:         p.Name,
			action:       p.Action,
			apiFamily:    c.APIFamily,
			hostSuffix:   strings.ToLower(c.Host),
			method:       strings.ToUpper(c.Method),
			pathPrefix:   c.PathPrefix,
			bodyContains: c.BodyContains,
		})
	}
	return rules
}

// reqMeta son los METADATOS del egress contra los que se evalúan las reglas. body
// puede ser nil (no re-leíble o ninguna regla lo pedía): en ese caso una condición
// body_contains no casa (best-effort, fail-open SOLO en la dimensión del cuerpo).
type reqMeta struct {
	apiFamily string
	host      string
	method    string
	path      string
	body      []byte
}

// match indica si la regla casa los metadatos (campos vacíos = comodín).
func (r rule) match(m reqMeta) bool {
	if r.apiFamily != "" && r.apiFamily != m.apiFamily {
		return false
	}
	if r.method != "" && r.method != m.method {
		return false
	}
	if r.hostSuffix != "" && !hostMatches(r.hostSuffix, m.host) {
		return false
	}
	if r.pathPrefix != "" && !strings.HasPrefix(m.path, r.pathPrefix) {
		return false
	}
	if r.bodyContains != "" {
		// Sin cuerpo re-leíble no podemos afirmar la coincidencia: no casa.
		if m.body == nil || !bytes.Contains(m.body, []byte(r.bodyContains)) {
			return false
		}
	}
	return true
}

// hostMatches compara el host destino con el patrón: sufijo si el patrón empieza
// por '.' (".acme.com" casa "api.acme.com" y "acme.com"), exacto en otro caso.
func hostMatches(pattern, host string) bool {
	if strings.HasPrefix(pattern, ".") {
		return host == pattern[1:] || strings.HasSuffix(host, pattern)
	}
	return host == pattern
}

// evaluate aplica las reglas EN ORDEN (el de carga, por created_at): la primera
// regla DECISIVA (allow o block) que casa determina el veredicto —así un `allow`
// colocado antes actúa de excepción sobre un `block` posterior—. Los `warn` que
// casan se recuerdan (el primero) pero no cortan. Sin regla decisiva ⇒ el primer
// warn, si lo hubo; si no, allow.
func evaluate(m reqMeta, rules []rule) (domain.PolicyAction, rule) {
	var warned rule
	haveWarn := false
	for _, r := range rules {
		if !r.match(m) {
			continue
		}
		switch r.action {
		case domain.PolicyAllow:
			return domain.PolicyAllow, r
		case domain.PolicyBlock:
			return domain.PolicyBlock, r
		case domain.PolicyWarn:
			if !haveWarn {
				warned, haveWarn = r, true
			}
		}
	}
	if haveWarn {
		return domain.PolicyWarn, warned
	}
	return domain.PolicyAllow, rule{}
}

// needBody indica si algún patrón del conjunto exige leer el cuerpo. Se calcula
// una vez por evaluación para no tocar el cuerpo cuando ninguna regla lo pide.
func needBody(rules []rule) bool {
	for _, r := range rules {
		if r.bodyContains != "" {
			return true
		}
	}
	return false
}

// cacheEntry es el conjunto de reglas compiladas de una org con su marca de carga.
type cacheEntry struct {
	rules    []rule
	loadedAt time.Time
}

// Engine es el motor de política de egress: mantiene las reglas de cada org en una
// caché con TTL (una carga por org cada TTL, no una query por request) y produce el
// Hook que el RoundTripper ejecuta en CADA salida. Seguro para uso concurrente.
type Engine struct {
	loader DecisionLoader
	ttl    time.Duration
	log    *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry
}

// DecisionLoader agrupa las dos capacidades de persistencia que el motor necesita
// (carga de reglas + registro de decisiones): el *store.Store las satisface ambas.
type DecisionLoader interface {
	Loader
	DecisionSink
}

// NewEngine construye el motor. ttl<=0 ⇒ 30s. store provee carga+registro.
func NewEngine(store DecisionLoader, ttl time.Duration, log *slog.Logger) *Engine {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Engine{loader: store, ttl: ttl, log: log, cache: make(map[string]cacheEntry)}
}

// Invalidate descarta la caché de reglas de una org (la fuerza a recargar en el
// próximo egress). La invocan los handlers admin tras un alta/edición/borrado de
// regla, para que el cambio surta efecto de inmediato en ESTA réplica.
//
// ponytail: caché con TTL por org; entre réplicas el cambio se propaga al vencer
// el TTL (no hay pub/sub de invalidación). Basta para política —consistencia
// eventual acotada por el TTL—; si hiciera falta invalidación cross-réplica
// instantánea, un canal de events/DragonflyDB.
func (e *Engine) Invalidate(orgID string) {
	e.mu.Lock()
	delete(e.cache, orgID)
	e.mu.Unlock()
}

// Hook devuelve el policy.Hook ligado a una instancia concreta. Es lo que
// BuildRuntime enchufa en el RoundTripper; se ejecuta ANTES de conectar, de modo
// que un `block` impide que el request llegue al transporte subyacente.
func (e *Engine) Hook(orgID, instanceID, apiFamily string) Hook {
	return func(req *http.Request) error {
		rules, err := e.rulesFor(req.Context(), orgID)
		if err != nil {
			// Fallo de carga SIN caché previa (p. ej. BD caída en el primer egress de
			// la org): fail-open y log. Una vez cargada, servimos last-known-good aunque
			// la BD falle, así que este camino es raro. ponytail: fail-open prioriza
			// disponibilidad; un modo fail-closed por config si algún cliente lo exige.
			e.log.WarnContext(req.Context(), "policy: no se pudieron cargar reglas; egress permitido sin evaluar", "error", err, "org", orgID)
			return nil
		}
		if len(rules) == 0 {
			return nil
		}
		meta := reqMeta{
			apiFamily: apiFamily,
			host:      strings.ToLower(req.URL.Hostname()),
			method:    strings.ToUpper(req.Method),
			path:      req.URL.Path,
		}
		if needBody(rules) {
			meta.body = peekBody(req)
		}
		action, matched := evaluate(meta, rules)
		if action == domain.PolicyAllow {
			return nil // camino caliente: sin regla decisiva, sin escritura.
		}
		e.record(req, orgID, instanceID, matched, action)
		if action == domain.PolicyBlock {
			return ErrBlockedByPolicy
		}
		return nil // warn: sale, ya quedó marcado.
	}
}

// rulesFor devuelve las reglas compiladas de una org desde la caché, recargando si
// venció el TTL. En un fallo de recarga con entrada previa, sirve la última buena
// (no vacía la política por un parpadeo de la BD). El map se bloquea solo para
// leer/escribir la entrada, no durante la carga (orgs distintas no se serializan).
//
// ponytail: sin singleflight; al vencer el TTL puede haber una ráfaga breve de
// recargas concurrentes de la MISMA org. A este ritmo (una carga/TTL/org) es
// asumible; añadir singleflight si el coste de carga apretara.
func (e *Engine) rulesFor(ctx context.Context, orgID string) ([]rule, error) {
	e.mu.Lock()
	ent, ok := e.cache[orgID]
	e.mu.Unlock()
	if ok && time.Since(ent.loadedAt) < e.ttl {
		return ent.rules, nil
	}

	// Carga con contexto propio y acotado: la cancelación de ESTA petición no debe
	// envenenar una carga cuyo resultado comparten las demás. LoadPolicies fija el
	// tenant desde orgID (no del contexto).
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	pols, err := e.loader.LoadPolicies(loadCtx, orgID)
	if err != nil {
		if ok {
			return ent.rules, nil // last-known-good.
		}
		return nil, err
	}
	rules := compile(pols)
	e.mu.Lock()
	e.cache[orgID] = cacheEntry{rules: rules, loadedAt: time.Now()}
	e.mu.Unlock()
	return rules, nil
}

// record persiste la decisión (best-effort, contexto propio). El enforcement NO
// depende de esta escritura: para un block, se registra ANTES de devolver el error,
// pero un fallo de registro se loguea y AUN ASÍ se bloquea (fail-closed).
func (e *Engine) record(req *http.Request, orgID, instanceID string, r rule, action domain.PolicyAction) {
	d := domain.PolicyDecision{
		OrgID:      orgID,
		InstanceID: instanceID,
		PolicyID:   r.id,
		PolicyName: r.name,
		Action:     action,
		RequestID:  httpx.RequestIDFromContext(req.Context()),
		Host:       req.URL.Hostname(),
		Method:     req.Method,
		OccurredAt: time.Now().UTC(),
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(req.Context()), 5*time.Second)
	defer cancel()
	if err := e.loader.RecordDecision(ctx, d); err != nil {
		e.log.WarnContext(req.Context(), "policy: no se pudo registrar la decisión", "error", err, "action", action, "org", orgID)
	}
}

// peekBody lee hasta bodyPeekCap bytes de una COPIA del cuerpo (req.GetBody), sin
// tocar el req.Body que saldrá a la red. Devuelve nil si el cuerpo no es re-leíble
// (streaming: GetBody nil) o está vacío: en tal caso las reglas body_contains no
// casan (limitación honesta del Día 1).
func peekBody(req *http.Request) []byte {
	if req.Body == nil || req.Body == http.NoBody || req.GetBody == nil {
		return nil
	}
	rc, err := req.GetBody()
	if err != nil {
		return nil
	}
	defer rc.Close()
	buf, err := io.ReadAll(io.LimitReader(rc, bodyPeekCap))
	if err != nil || len(buf) == 0 {
		return nil
	}
	return buf
}
