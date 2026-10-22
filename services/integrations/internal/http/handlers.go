// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package integrationshttp implementa la superficie HTTP del Sytue Integration
// Gateway: catálogo de plantillas, CRUD de instancias por organización, escritura
// write-only de secretos y despacho de Driver.Test. Auth por JWT de Sytue ID;
// escrituras restringidas a admin/owner de la org; aislamiento por RLS.
package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/keycrypto"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/manifest"
	"git.sytue.eu/sytue/one/services/integrations/internal/policy"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

const maxNameLen = 200

// Handlers agrupa los manejadores del gateway.
type Handlers struct {
	store             *store.Store
	registry          *connector.Registry
	limiter           httpx.RateLimiter
	audit             policy.AuditSink
	engine            *policy.Engine
	idApps            manifest.IDApps
	egressTimeout     time.Duration
	egressMaxRedirect int
	// URLs internas de Sytue ID y Sytue Drive: las inyecta el servicio en las
	// instancias de la propia suite (ver loadInstance en dispatch.go). Que salgan de
	// AQUÍ —configuración del servicio— y no de un formulario de admin es lo que
	// permite tratar ese destino como interno.
	idBaseURL    string
	driveBaseURL string
	// Entregas de «Tus datos» que lee la salida propia (handlers_salida.go).
	contactsBaseURL, mailBaseURL, calendarBaseURL string
	// fondo vive lo que el proceso: una salida propia sigue al cerrar la petición que
	// la lanzó y se corta al apagar el servicio. migrationTimeout acota cada petición
	// de contenido (un fichero grande no sube en el tope de las llamadas a API).
	fondo            context.Context
	migrationTimeout time.Duration
	// dir lee el directorio de la organización en Sytue ID. Lo usa el asistente
	// automático de migración para saber en qué cuenta de Sytue aterriza cada cuenta
	// del origen sin que nadie teclee la correspondencia. Nil ⇒ el asistente
	// automático lo dice en vez de adivinar.
	dir *directory.Client

	// Consentimiento OAuth (handlers_oauth.go). publicURL da el redirect_uri que el
	// proveedor tiene registrado; consoleURL y drivePublicURL eligen el BFF de
	// vuelta. Salen de la CONFIGURACIÓN, nunca de la petición. Derivarlos de Host o
	// X-Forwarded-Host dejaría que una cabecera falsificada desviara el código de
	// autorización. oauthCipher sella el `state`; sin él no se ofrece el flujo.
	// Resolutor y no un mapa fijo: las credenciales de la suite se editan desde la
	// superficie de plataforma y tienen que surtir efecto sin reiniciar el pod.
	// Conserva SYTUE_INTEGRATIONS_OAUTH_CLIENTS como respaldo.
	oauthClients               *oauthClientResolver
	oauthCipher                *keycrypto.Cipher
	publicURL                  string
	consoleURL                 string
	drivePublicURL             string
	roomsPublicURL             string
	mailPublicURL              string
	calendarPublicURL          string
	contactsPublicURL          string
	notesPublicURL             string
	whatsAppConfigID           string
	whatsAppGraphVersion       string
	whatsAppWebhookVerifyToken string
}

// --- Plantillas --------------------------------------------------------------

// ListTemplates devuelve el catálogo de plantillas habilitadas.
func (h *Handlers) ListTemplates(w http.ResponseWriter, r *http.Request) {
	tpls, err := h.store.ListTemplates(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	off, err := h.store.DisabledIntegrations(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]oauthTemplateStatus, 0, len(tpls))
	for _, tpl := range tpls {
		items = append(items, oauthTemplateStatus{AppTemplate: tpl, OAuthAvailable: !off[proveedorDePlantilla(tpl.Key)] && h.oauthAvailable(r.Context(), tpl.Key, tpl.Manifest, nil, false)})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"templates": items})
}

// --- Instancias --------------------------------------------------------------

// ListInstances lista las instancias de la organización activa.
func (h *Handlers) ListInstances(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListInstances(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	off, err := h.store.DisabledIntegrations(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	items := make([]oauthInstanceStatus, 0, len(list))
	for _, inst := range list {
		items = append(items, oauthInstanceStatus{AppInstance: inst, OAuthAvailable: !off[proveedorDePlantilla(inst.TemplateKey)] && h.instanceOAuthAvailable(r.Context(), inst)})
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"instances": items})
}

// GetInstance devuelve una instancia por id.
func (h *Handlers) GetInstance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	inst, err := h.store.GetInstance(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if personalConsentTemplate(inst.TemplateKey) {
		actor, _ := authn.FromContext(r.Context())
		if err := h.store.RequireExternalChatActor(r.Context(), id, actor.Subject); err != nil {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
	}

	httpx.JSON(w, r, http.StatusOK, inst)
}

type createInstanceReq struct {
	TemplateKey  string          `json:"template_key"`
	Name         string          `json:"name"`
	Config       json.RawMessage `json:"config"`
	LevelEnabled []string        `json:"level_enabled"`
}

// CreateInstance crea una instancia (estado inicial 'draft').
func (h *Handlers) CreateInstance(w http.ResponseWriter, r *http.Request) {
	var req createInstanceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name, err := validName(req.Name)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if strings.TrimSpace(req.TemplateKey) == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("template_key is required."))
		return
	}
	cfg, err := validConfig(req.Config)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	// Una plantilla que no existe la sigue diciendo el store, con su mensaje de siempre.
	if m, merr := h.manifestDe(r.Context(), strings.TrimSpace(req.TemplateKey)); merr == nil {
		if err := soloDestinosDeclarados(m, cfg); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	inst, err := h.store.CreateInstance(r.Context(), strings.TrimSpace(req.TemplateKey), name, cfg, req.LevelEnabled)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, inst)
}

type patchInstanceReq struct {
	Name         *string         `json:"name"`
	Config       json.RawMessage `json:"config"`
	Status       *string         `json:"status"`
	LevelEnabled *[]string       `json:"level_enabled"`
}

// UpdateInstance aplica un PATCH parcial (nombre/config/estado/niveles).
func (h *Handlers) UpdateInstance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req patchInstanceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	cur, err := h.store.GetInstance(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	if externalMailTemplate(cur.TemplateKey) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Configure personal mail accounts with the Mail connection wizard."))
		return
	}

	name := cur.Name
	if req.Name != nil {
		if name, err = validName(*req.Name); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	cfg := cur.Config
	if len(req.Config) > 0 {
		if cfg, err = validConfig(req.Config); err != nil {
			httpx.Fail(w, r, err)
			return
		}
		if err := h.alCambiarDeServidor(r.Context(), cur, cfg); err != nil {
			httpx.Fail(w, r, err)
			return
		}
	}
	status := cur.Status
	if req.Status != nil {
		status = domain.Status(*req.Status)
		if !status.Valid() {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid status (draft|active|error|disabled)."))
			return
		}
	}
	levels := cur.LevelEnabled
	if req.LevelEnabled != nil {
		levels = *req.LevelEnabled
	}

	inst, err := h.store.UpdateInstance(r.Context(), id, name, cfg, status, levels)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, inst)
}

// DeleteInstance borra una instancia (y en cascada sus secretos), revocando
// antes el token ante el proveedor si está disponible.
func (h *Handlers) DeleteInstance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	current, err := h.store.GetInstance(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if err := h.mailInstanceAccess(r.Context(), current); err != nil {
		httpx.Fail(w, r, err)
		return
	}

	ctx := r.Context()
	if inst, m, cfg, err := h.loadInstanceManifest(ctx, id); err == nil {
		_ = manifest.Deactivate(ctx, h.idApps, inst.OrgID, cfg)
		h.revokeAtProvider(ctx, id, inst.TemplateKey, m, cfg)
	}
	if err := h.store.DeleteInstance(ctx, id); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

type putSecretsReq struct {
	Secrets map[string]string `json:"secrets"`
}

// PutSecrets reemplaza el conjunto de secretos de una instancia (write-only: no
// se devuelven ni se pueden leer).
func (h *Handlers) PutSecrets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	inst, err := h.store.GetInstance(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if personalConsentTemplate(inst.TemplateKey) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Connect personal accounts by consent."))
		return
	}

	var req putSecretsReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.validarHostsEnSecretos(r.Context(), inst, req.Secrets); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.PutSecrets(r.Context(), id, req.Secrets); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// PatchSecrets añade o reemplaza SOLO los secretos indicados y deja los demás como
// estaban. Es lo que usa el asistente de migración al aportar la cuenta de servicio
// de Google: la conexión ya tiene el token del consentimiento y perderlo dejaría la
// migración a medias sin decir por qué.
func (h *Handlers) PatchSecrets(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	inst, err := h.store.GetInstance(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	if personalConsentTemplate(inst.TemplateKey) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage("Connect personal accounts by consent."))
		return
	}

	var req putSecretsReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.validarHostsEnSecretos(r.Context(), inst, req.Secrets); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if err := h.store.MergeSecrets(r.Context(), id, req.Secrets); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.NoContent(w)
}

// validarHostsEnSecretos comprueba que un secreto estructurado (p. ej. la clave JSON de
// cuenta de servicio de Google con `token_uri`) solo apunte a los hosts oficiales
// declarados en el manifiesto del proveedor.
func (h *Handlers) validarHostsEnSecretos(ctx context.Context, inst *domain.AppInstance, secrets map[string]string) error {
	rawSA := strings.TrimSpace(secrets["service_account_key"])
	if rawSA == "" || h.store == nil || inst == nil {
		return nil
	}
	var sa struct {
		TokenURI string `json:"token_uri"`
	}
	if err := json.Unmarshal([]byte(rawSA), &sa); err != nil {
		return httpx.ErrBadRequest.WithMessage("service_account_key must be valid JSON.")
	}
	if strings.TrimSpace(sa.TokenURI) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(sa.TokenURI))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return httpx.ErrBadRequest.WithMessage("token_uri in service_account_key must be a valid https URL.")
	}
	if m, merr := h.manifestDe(ctx, inst.TemplateKey); merr == nil {
		var cfg map[string]string
		_ = json.Unmarshal(inst.Config, &cfg)
		if oficiales := m.HostsOficiales(cfg); len(oficiales) > 0 && !manifest.HostPermitido(u.Hostname(), oficiales) {
			return httpx.ErrBadRequest.WithMessage("token_uri in service_account_key does not point to an official provider host.")
		}
	}
	return nil
}

// TestInstance prueba la conexión con el tercero. Una instancia puede tener VARIAS
// capacidades servidas por drivers distintos —Jira aprovisiona por SCIM y trae datos por
// JQL, con credenciales diferentes—, así que se prueban TODAS y se devuelve un resultado
// por capacidad. Antes se probaba un único driver resuelto por la clave de la plantilla,
// que en el mejor caso era el de datos: el aprovisionamiento no se verificaba nunca.
//
// Una plantilla sin capacidades salientes (solo SSO, como GitHub Enterprise) responde
// 200 con la lista vacía y NO toca el estado: no hay credencial que verificar, y el
// inicio de sesión lo prueba el propio SP/client creado en Sytue ID.
func (h *Handlers) TestInstance(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()

	inst, err := h.loadInstance(ctx, id)
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}

	results := make([]capabilityResult, 0, 2)
	var failed bool
	for _, capab := range []manifest.Capability{manifest.CapProvision, manifest.CapData} {
		drv, cfg, resErr := h.resolveCapability(ctx, inst.TemplateKey, inst.Config, capab)
		if resErr != nil {
			continue // la plantilla no declara esta capacidad: no hay nada que probar.
		}

		// Runtime: único cliente de egress. El Bearer sale del vault; el driver no lo ve.
		rt := h.egressRuntime(inst, log.From(ctx).With(
			"instance", inst.ID, "org", inst.OrgID, "capability", string(capab),
		))

		// El driver recibe la instancia SIN secretos (seguridad por construcción), con la
		// config que su capacidad necesita ya resuelta.
		driverInst := inst
		driverInst.Config = cfg
		driverInst.Secrets = nil

		if testErr := drv.Test(ctx, driverInst, rt); testErr != nil {
			if blocked := blockedErr(testErr); blocked != nil {
				httpx.Fail(w, r, blocked)
				return
			}
			log.From(ctx).Warn("integrations: Test de conector falló",
				"instance", id, "capability", capab, "error", testErr)
			results = append(results, capabilityResult{Capability: string(capab), Error: testErr.Error()})
			failed = true
			continue
		}
		results = append(results, capabilityResult{Capability: string(capab), OK: true})
	}

	if failed {
		if err := h.store.SetStatus(ctx, id, domain.StatusError); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
		httpx.JSON(w, r, http.StatusBadGateway, map[string]any{
			"status":  domain.StatusError,
			"results": results,
		})
		return
	}
	if len(results) == 0 {
		// Solo SSO: nada que probar contra el tercero, y el estado se queda como está.
		httpx.JSON(w, r, http.StatusOK, map[string]any{"results": results})
		return
	}
	if err := h.store.SetStatus(ctx, id, domain.StatusActive); err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"status": domain.StatusActive, "results": results})
}

// capabilityResult es el resultado de probar UNA capacidad de la conexión. El asistente
// de la consola lo usa para decir cuál funciona y cuál no, en vez de un «falló» opaco.
type capabilityResult struct {
	Capability string `json:"capability"`
	OK         bool   `json:"ok"`
	Error      string `json:"error,omitempty"`
}

// --- helpers -----------------------------------------------------------------

// blockedErr traduce un bloqueo de política (policy.ErrBlockedByPolicy, propagado
// desde el RoundTripper a través del driver) a un 403 explícito: lo prohíbe la
// organización, no es un fallo del tercero. Devuelve nil si el error no es un bloqueo.
func blockedErr(err error) error {
	if errors.Is(err, policy.ErrBlockedByPolicy) {
		return httpx.NewError(http.StatusForbidden, "policy_blocked",
			"La política de gobernanza de la organización bloqueó esta llamada al tercero.")
	}
	return nil
}

// toAPIError traduce errores de dominio/store a *httpx.Error canónicos.
func toAPIError(err error) error {
	switch {
	case err == nil:
		return nil
	// Un error que ya trae su estado y su mensaje para el administrador se respeta: sin
	// esto acababa envuelto en un 500 opaco.
	case errors.As(err, new(*httpx.Error)):
		return err
	case errors.Is(err, domain.ErrForbidden):
		return httpx.ErrForbidden
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	case errors.Is(err, domain.ErrConflict):
		return httpx.ErrConflict.WithMessage(
			"It is already being copied. If it looks stuck, cancel it and start it again: it continues where it left off.",
		)
	case errors.Is(err, domain.ErrInvalidInput):
		return httpx.ErrBadRequest.WithMessage("Unknown or disabled template.")
	case errors.Is(err, store.ErrDemasiadasMigraciones):
		return httpx.NewError(http.StatusTooManyRequests, "too_many_migrations",
			"Your organisation already has too many copies queued. Wait for some to finish and try again.")
	case errors.Is(err, store.ErrNoCipher):
		return httpx.NewError(http.StatusServiceUnavailable, "no_cipher",
			"El cifrado de secretos no está configurado en el servidor.")
	default:
		return httpx.ErrInternal.Wrap(err)
	}
}

// pathID extrae y valida el {id} de la ruta (uuid canónico). Responde 400 y
// devuelve ok=false si es inválido.
func pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid identifier."))
		return "", false
	}
	return id, true
}

// validName valida/normaliza el nombre de una instancia.
func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", httpx.ErrBadRequest.WithMessage("A name is required.")
	}
	if len(name) > maxNameLen {
		return "", httpx.ErrBadRequest.WithMessage("The name is too long.")
	}
	return name, nil
}

// validConfig exige que la config sea un objeto JSON PLANO de strings (contrato
// Config = map[string]string) y la devuelve en forma canónica. Vacía ⇒ "{}".
func validConfig(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage("{}"), nil
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("config must be a JSON object of strings.")
	}
	if m == nil {
		return json.RawMessage("{}"), nil
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, httpx.ErrBadRequest.WithMessage("Invalid config.")
	}
	return out, nil
}

// manifestDe lee y valida el manifiesto de una plantilla del catálogo.
func (h *Handlers) manifestDe(ctx context.Context, templateKey string) (*manifest.Manifest, error) {
	tpl, err := h.store.GetTemplate(ctx, templateKey)
	if err != nil {
		return nil, err
	}
	return manifest.Parse(tpl.Manifest)
}

// soloDestinosDeclarados rechaza una config que lleve una URL en una clave que la
// plantilla no declara como campo.
//
// Los drivers aceptan raíces de API alternativas (`graph_base`, `authority_base`,
// `google_drive_base`, `github_api_base`…) para nubes soberanas y para los servidores
// simulados de los tests, y las leen de la config. Por la API eso dejaba a quien
// administra la organización apuntar la conexión a un servidor suyo SIN tocar las
// credenciales guardadas —el token que consintió otra persona, el secreto de la app—,
// que la pasarela mandaba allí en la siguiente llamada. Los campos que la plantilla
// declara son los que se ven en el asistente; lo demás no puede decidir adónde se sale.
// Las claves internas de la pasarela (`_…`) no son destinos de ninguna llamada.
func soloDestinosDeclarados(m *manifest.Manifest, cfgJSON json.RawMessage) error {
	var cfg map[string]string
	if err := json.Unmarshal(cfgJSON, &cfg); err != nil {
		return httpx.ErrBadRequest.WithMessage("config must be a JSON object of strings.")
	}
	for clave, valor := range cfg {
		if strings.HasPrefix(clave, "_") || m.Declara(clave) {
			continue
		}
		// Por el valor (una URL) o por el nombre: hay drivers que aceptan el servidor
		// sin esquema («instance_url» de Salesforce), y ahí el valor no lo delata.
		if strings.Contains(valor, "://") || reClaveDeServidor.MatchString(clave) {
			return httpx.ErrBadRequest.WithMessage("This app does not take a server address in \"" + clave + "\".")
		}
	}
	return nil
}

// reClaveDeServidor casa las claves de config que nombran un servidor.
var reClaveDeServidor = regexp.MustCompile(`(^|_)(url|uri|base|host|endpoint)$`)

// alCambiarDeServidor aplica las dos reglas a una config nueva: solo los campos
// declarados dicen adónde se sale, y si el servidor cambia, las credenciales guardadas
// —que eran para el anterior— se borran. La conexión queda por reconectar, que es lo
// que es: otra conexión.
func (h *Handlers) alCambiarDeServidor(ctx context.Context, cur *domain.AppInstance, nueva json.RawMessage) error {
	m, err := h.manifestDe(ctx, cur.TemplateKey)
	if err != nil {
		return capabilityError(err, "changing its details")
	}
	if err := soloDestinosDeclarados(m, nueva); err != nil {
		return err
	}
	if !cambiaDeServidor(m, cur.Config, nueva) {
		return nil
	}
	if err := h.store.PutSecrets(ctx, cur.ID, nil); err != nil && !errors.Is(err, store.ErrNoCipher) {
		return toAPIError(err)
	}
	return nil
}

// cambiaDeServidor dice si dos configs de la misma plantilla apuntan a servidores
// distintos (manifest.Destinos).
func cambiaDeServidor(m *manifest.Manifest, antes, despues json.RawMessage) bool {
	var a, d map[string]string
	_ = json.Unmarshal(antes, &a)
	_ = json.Unmarshal(despues, &d)
	return !slices.Equal(m.Destinos(a), m.Destinos(d))
}

// validUUID valida el formato canónico 8-4-4-4-12 hexadecimal (sin dependencias).
func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	// Recorrido por BYTES (no runas): un UTF-8 multibyte de 36 bytes no debe colarse
	// truncado a byte en isHex.
	for i := 0; i < len(s); i++ {
		switch i {
		case 8, 13, 18, 23:
			if s[i] != '-' {
				return false
			}
		default:
			if !isHex(s[i]) {
				return false
			}
		}
	}
	return true
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
