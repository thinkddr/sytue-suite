// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"cmp"
	"net/http"
	"slices"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
)

// DE DÓNDE SE PUEDE TRAER UNO SUS DATOS — una vez por cuenta, no una vez por conexión.
//
// «Trae tus datos» enseñaba un botón por cada conexión de la organización cuya plantilla
// EMPEZABA por «google» o «microsoft». Eso juntaba dos fallos que se veían igual —dos
// «Traerlo de Google» idénticos—:
//
//   - Conexiones que no son orígenes de migración: la misma cuenta de Google conectada
//     para Drive («Añadir almacenamiento»), Meet y Chat, o la de Microsoft para Teams y
//     para el correo de Outlook de una persona. Cada una salía como un botón más, y todos
//     menos uno fallaban al pulsarlos porque la pasarela solo migra desde
//     `google-workspace` y `microsoft365` (sourceForTemplate).
//   - La misma cuenta conectada varias veces con la plantilla buena: dos «Google
//     Workspace» de acme.com. Cada botón lanzaba su propia copia, con su propio mapa de
//     lo ya copiado, así que traerse el correo desde los dos lo duplicaba entero.
//
// Aquí se decide UNA vez, en el servidor, para la web, las apps nativas y quien venga
// después: solo cuentan las plantillas de sourceForTemplate que traen contenido personal,
// y las conexiones de la misma cuenta se unifican en un origen que apunta a la mejor de
// ellas. Los clientes deduplican además por su cuenta, pero solo como red.

// Claves de la config PÚBLICA con las que una conexión recuerda de qué cuenta es. No son
// secretos: el dominio de una organización o la dirección de quien la conectó, lo mismo
// que ya enseña la consola. Las rellena el consentimiento (importAccountFromProvider).
const (
	// configKeyImportAccount es la identidad ESTABLE de la cuenta: «google:acme.com»,
	// «m365:contoso.onmicrosoft.com». Es lo que decide si dos conexiones son la misma.
	configKeyImportAccount = "_import_account"
	// configKeyImportAccountLabel es cómo se le enseña a una persona: «acme.com».
	configKeyImportAccountLabel = "_import_account_label"
	// configKeyAuthorizedAt es cuándo se autorizó por última vez: entre dos conexiones
	// de la misma cuenta, la autorizada más tarde es la que tiene el token vivo.
	configKeyAuthorizedAt = "_oauth_authorized_at"
)

// Estados de un origen tal y como los ve quien se trae sus datos.
const (
	// importOriginReady: hay una conexión activa con credencial; se puede lanzar.
	importOriginReady = "ready"
	// importOriginUnavailable: la conexión existe pero no puede leer nada —caducó, se
	// revocó, falló su última prueba o nunca se autorizó para datos—. Se enseña igual,
	// apagada y con el motivo, porque un botón que siempre falla no explica nada y uno
	// que desaparece hace pensar que nunca hubo conexión.
	importOriginUnavailable = "unavailable"
)

// importOrigin es un origen del que una persona puede traerse sus datos: UNA cuenta del
// proveedor, aunque la organización la haya conectado varias veces.
type importOrigin struct {
	// InstanceID es la conexión con la que se lanza: la mejor de las de esa cuenta.
	InstanceID string `json:"instance_id"`
	// InstanceIDs son TODAS las conexiones de la cuenta, la elegida primero. El cliente
	// las usa para atribuir a esta cuenta los trabajos lanzados con una conexión vieja.
	InstanceIDs []string               `json:"instance_ids"`
	Name        string                 `json:"name"`
	Source      domain.MigrationSource `json:"source"`
	// AccountKey vacío: no se sabe de qué cuenta es (conexión anterior a recordarlo).
	AccountKey   string `json:"account_key,omitempty"`
	AccountLabel string `json:"account_label,omitempty"`
	State        string `json:"state"`
}

// selfImportSource dice si una plantilla es un origen del que una persona puede traerse
// contenido PERSONAL. Sale de sourceForTemplate, la única correspondencia entre catálogo
// y orígenes, y no de un prefijo del nombre: «google-drive» o «microsoft-teams» empiezan
// igual y no son orígenes. GitHub sí es un origen, pero de repositorios, que son de la
// organización (selfMigrationDomains).
func selfImportSource(templateKey string) (domain.MigrationSource, bool) {
	src, ok := sourceForTemplate(templateKey)
	if !ok {
		return "", false
	}
	for _, d := range src.Domains() {
		if _, personal := selfMigrationDomains[d]; personal {
			return src, true
		}
	}
	return "", false
}

// importAccount dice de qué cuenta del proveedor es una conexión: la clave estable y el
// rótulo. Primero lo que recordó el consentimiento; si no hay —conexiones de antes, o
// las que se configuraron a mano—, el campo del formulario que la identifica: el dominio
// de Google Workspace o el inquilino de Entra. Vacío si no se sabe.
func importAccount(inst domain.AppInstance) (key, label string) {
	src, ok := selfImportSource(inst.TemplateKey)
	if !ok {
		return "", ""
	}
	cfg := inst.ConfigMap()
	if k := strings.ToLower(strings.TrimSpace(cfg[configKeyImportAccount])); k != "" {
		label = strings.TrimSpace(cfg[configKeyImportAccountLabel])
		if label == "" {
			_, label, _ = strings.Cut(k, ":")
		}
		return k, label
	}
	// El campo del formulario que identifica la cuenta: el dominio de Workspace o el
	// inquilino de Entra (selfImportSource solo deja pasar esos dos orígenes).
	declared := cfg["tenant"]
	if src == domain.SourceGoogle {
		declared = cfg["domain"]
	}
	if v := strings.ToLower(strings.Trim(strings.TrimSpace(declared), "@.")); v != "" {
		return importAccountKey(src, v), v
	}
	return "", ""
}

// importAccountKey compone la clave de cuenta. Lleva el origen delante para que el
// dominio de Google y el de Microsoft de la misma empresa no se confundan.
func importAccountKey(src domain.MigrationSource, account string) string {
	return string(src) + ":" + strings.ToLower(strings.TrimSpace(account))
}

// importReady dice si una conexión puede leer: activa y con credencial guardada.
func importReady(inst domain.AppInstance) bool {
	return inst.Status == domain.StatusActive && inst.HasSecrets
}

// authorizedAt es cuándo se autorizó la conexión por última vez. Las anteriores a
// recordarlo usan su última modificación, que es lo más parecido que hay.
func authorizedAt(inst domain.AppInstance) time.Time {
	if raw := inst.ConfigMap()[configKeyAuthorizedAt]; raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t
		}
	}
	return inst.UpdatedAt
}

// preferImport ordena las conexiones de UNA cuenta: primero la que puede leer, luego la
// autorizada más tarde (su token es el vivo), luego la más nueva. El id desempata para
// que dos lecturas seguidas elijan la misma.
func preferImport(a, b domain.AppInstance) int {
	if ra, rb := importReady(a), importReady(b); ra != rb {
		if ra {
			return -1
		}
		return 1
	}
	if c := authorizedAt(b).Compare(authorizedAt(a)); c != 0 {
		return c
	}
	if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.ID, b.ID)
}

// importOrigins unifica las conexiones de la organización en orígenes de importación:
// uno por (proveedor, cuenta).
//
// Una conexión de la que no se sabe la cuenta se suma a la única cuenta conocida de su
// proveedor, si solo hay una: casi siempre es la misma conectada antes de que esto se
// recordara, y enseñarla aparte es justo el duplicado que había. Con varias cuentas
// conocidas no se adivina: las desconocidas van juntas en su propio origen.
//
// Los borradores (consentimientos sin terminar) y las desactivadas por la organización
// no son orígenes; tampoco las de una plantilla que la plataforma retiró del catálogo.
func importOrigins(instances []domain.AppInstance) []importOrigin {
	type candidate struct {
		inst       domain.AppInstance
		src        domain.MigrationSource
		key, label string
	}
	var cands []candidate
	known := map[domain.MigrationSource]map[string]bool{}
	for _, inst := range instances {
		src, ok := selfImportSource(inst.TemplateKey)
		if !ok || !inst.ConsentTemplateEnabled {
			continue
		}
		if inst.Status != domain.StatusActive && inst.Status != domain.StatusError {
			continue
		}
		key, label := importAccount(inst)
		if key != "" {
			if known[src] == nil {
				known[src] = map[string]bool{}
			}
			known[src][key] = true
		}
		cands = append(cands, candidate{inst: inst, src: src, key: key, label: label})
	}

	type groupKey struct {
		src domain.MigrationSource
		key string
	}
	groups := map[groupKey][]candidate{}
	var order []groupKey
	for _, c := range cands {
		key := c.key
		if key == "" && len(known[c.src]) == 1 {
			for only := range known[c.src] {
				key = only
			}
		}
		g := groupKey{c.src, key}
		if _, seen := groups[g]; !seen {
			order = append(order, g)
		}
		groups[g] = append(groups[g], c)
	}

	out := make([]importOrigin, 0, len(order))
	for _, g := range order {
		members := groups[g]
		slices.SortStableFunc(members, func(a, b candidate) int { return preferImport(a.inst, b.inst) })
		best := members[0]
		o := importOrigin{
			InstanceID:  best.inst.ID,
			InstanceIDs: make([]string, 0, len(members)),
			Name:        best.inst.Name,
			Source:      g.src,
			AccountKey:  g.key,
			State:       importOriginUnavailable,
		}
		if importReady(best.inst) {
			o.State = importOriginReady
		}
		for _, m := range members {
			o.InstanceIDs = append(o.InstanceIDs, m.inst.ID)
			// El rótulo de la elegida si lo tiene; si no, el de cualquiera de la cuenta.
			if o.AccountLabel == "" && m.key == g.key {
				o.AccountLabel = m.label
			}
		}
		out = append(out, o)
	}
	// Orden estable para la interfaz: por proveedor y, dentro, por cuenta.
	slices.SortStableFunc(out, func(a, b importOrigin) int {
		return cmp.Or(cmp.Compare(a.Source, b.Source), cmp.Compare(a.AccountLabel, b.AccountLabel))
	})
	return out
}

// importOriginOf devuelve el origen al que pertenece una conexión. Se busca en TODAS las
// de la cuenta: un cliente que aún enseña una conexión vieja de la misma cuenta lanza
// con la buena, en vez de con la que tenía a la vista.
func importOriginOf(origins []importOrigin, instanceID string) (importOrigin, bool) {
	for _, o := range origins {
		if slices.Contains(o.InstanceIDs, instanceID) {
			return o, true
		}
	}
	return importOrigin{}, false
}

// ListImportOrigins — GET /api/v1/migrations/self/origins: de dónde puede traerse sus
// datos quien llama, una vez por cuenta. Lectura de cualquier miembro, como /instances,
// de la que sale: no enseña nada que /instances no enseñara ya.
func (h *Handlers) ListImportOrigins(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListInstances(r.Context())
	if err != nil {
		httpx.Fail(w, r, toAPIError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.JSON(w, r, http.StatusOK, map[string]any{"origins": importOrigins(list)})
}

// selfJob es un trabajo tal y como lo ve quien se trae sus datos: con la cuenta de la
// que viene, para que dos trabajos de la misma cuenta se lean como uno, y con el motivo
// de un fallo dicho como código que la interfaz traduce.
type selfJob struct {
	domain.MigrationJob
	AccountKey   string `json:"account_key,omitempty"`
	AccountLabel string `json:"account_label,omitempty"`
	// ErrorCode clasifica LastError: «connection_expired» o «needs_admin». Vacío ⇒ el
	// fallo no tiene arreglo en manos de nadie más que reintentar.
	ErrorCode string `json:"error_code,omitempty"`
}

// selfJobs adjunta a cada trabajo la cuenta de su conexión. Un trabajo de una conexión
// que ya no es origen (borrada o retirada) se queda sin cuenta, y el cliente lo agrupa
// por su conexión.
func selfJobs(jobs []domain.MigrationJob, origins []importOrigin) []selfJob {
	out := make([]selfJob, 0, len(jobs))
	for _, j := range jobs {
		v := selfJob{MigrationJob: j}
		if o, ok := importOriginOf(origins, j.InstanceID); ok {
			v.AccountKey, v.AccountLabel = o.AccountKey, o.AccountLabel
		}
		if j.Status == domain.MigrationError {
			v.ErrorCode = importErrorCode(j.LastError)
		}
		out = append(out, v)
	}
	return out
}

// importErrorCode traduce el último fallo de un trabajo a algo que una persona puede
// entender y a quién acudir. El texto es el de NUESTROS propios errores —el canje de
// token, el origen respondiendo 401, la conexión sin delegación—, así que se reconoce
// por sus marcas; lo que no case se queda en el «algo no se pudo copiar» de siempre.
func importErrorCode(lastError string) string {
	e := strings.ToLower(lastError)
	switch {
	case containsAny(e, "invalid_grant", "token_expired", "token_revoked", "origen respondió http 401"):
		return "connection_expired"
	case containsAny(e, `falta el secreto "service_account_key"`, "faltan credenciales de la instancia",
		"unauthorized_client", "todavía no está autorizada"):
		return "needs_admin"
	default:
		return ""
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
