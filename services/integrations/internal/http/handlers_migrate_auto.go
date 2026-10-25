// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/authn"
	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/log"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
	"git.sytue.eu/sytue/one/services/integrations/internal/migrate"
	"git.sytue.eu/sytue/one/services/integrations/internal/store"
)

// El asistente AUTOMÁTICO de migración: una conexión, y todo lo que esa conexión
// alcanza se trae solo.
//
// La diferencia con POST /migrations (que sigue existiendo para el control fino)
// es de quién es el trabajo de averiguar las cosas. Allí lo hace un administrador:
// teclea de qué cuenta a qué cuenta y marca qué contenido. Aquí se le pregunta AL
// ORIGEN con la credencial que se acaba de autorizar —quién eres, qué tienes,
// hasta dónde llegas— y se le pregunta al directorio de Sytue en qué cuenta
// aterriza cada cual. Lo único que pone la persona es el login.
//
// Dos endpoints y no uno porque nada se copia sin verlo antes: el plan se mira
// primero (GET) y se lanza después (POST) con exactamente lo que se enseñó.

// migrationPlanResp es el plan: qué se traería, para quién, y qué queda fuera.
type migrationPlanResp struct {
	Source      domain.MigrationSource `json:"source"`
	InstanceID  string                 `json:"instance_id"`
	Connected   bool                   `json:"connected"`
	ConnectedAs string                 `json:"connected_as,omitempty"`
	// Scope: "user" (la cuenta que se conectó) u "organization" (todo el personal).
	Scope string `json:"scope"`
	// CanExpandToOrg + DelegationSetupURL son el camino para pasar de una cuenta a
	// toda la organización SIN buscar nada: el enlace lleva a la pantalla exacta del
	// proveedor, con cliente y permisos ya rellenos.
	CanExpandToOrg     bool                     `json:"can_expand_to_org"`
	DelegationSetupURL string                   `json:"delegation_setup_url,omitempty"`
	Items              []migrate.PlanItem       `json:"items"`
	People             []planPerson             `json:"people"`
	Unmatched          []string                 `json:"unmatched,omitempty"`
	Domains            []domain.MigrationDomain `json:"domains"`
	// Jobs es cuántos trabajos se crearían (personas × tipos de contenido).
	Jobs int `json:"jobs"`
}

// planPerson es una pareja «cuenta del origen → cuenta de Sytue» ya resuelta contra
// el directorio. Nada se migra a una cuenta que no se ha podido emparejar: copiar
// los datos de alguien en el buzón equivocado no tiene deshacer.
type planPerson struct {
	SourceUser string `json:"source_user"`
	TargetUser string `json:"target_user"`
	Name       string `json:"name,omitempty"`
}

// MigrationPlan — GET /api/v1/instances/{id}/migration-plan — descubre qué se
// puede migrar con la conexión ya autorizada. No escribe nada.
func (h *Handlers) MigrationPlan(w http.ResponseWriter, r *http.Request) {
	if !exigirAdminMigracion(w, r) {
		return
	}
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	plan, err := h.buildPlan(r.Context(), id)
	if err != nil {
		httpx.Fail(w, r, planError(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"plan": plan})
}

type autoMigrationReq struct {
	InstanceID string `json:"instance_id"`
}

// AutoMigrate — POST /api/v1/migrations/auto — lanza el plan entero: un trabajo
// por persona y tipo de contenido, encolados en el outbox.
//
// Se vuelve a construir el plan aquí en vez de fiarse de lo que mande el cliente:
// lo que se migra lo decide el origen y el directorio, no el navegador.
func (h *Handlers) AutoMigrate(w http.ResponseWriter, r *http.Request) {
	if !exigirAdminMigracion(w, r) {
		return
	}
	var req autoMigrationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	if !validUUID(req.InstanceID) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid instance_id."))
		return
	}
	ctx := r.Context()
	plan, err := h.buildPlan(ctx, req.InstanceID)
	if err != nil {
		httpx.Fail(w, r, planError(err))
		return
	}
	if len(plan.People) == 0 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"There is no one to migrate: none of the accounts in the source has an equivalent account in Sytue. Create those people in the directory first.",
		))
		return
	}

	// Un trabajo por persona y tipo de contenido PERSONAL; los tipos compartidos
	// (los equipos de Teams) van UNA sola vez, a nombre de quien lanza: crearlos por
	// cada persona copiaría los mismos canales tantas veces como plantilla haya.
	jobs := make([]domain.MigrationJob, 0, len(plan.People)*len(plan.Domains))
	create := func(targetUser, sourceUser string, dom domain.MigrationDomain) error {
		job, err := h.store.CreateMigrationJob(ctx, store.NewMigrationJob{
			InstanceID: plan.InstanceID,
			TargetUser: targetUser,
			SourceUser: sourceUser,
			Source:     plan.Source,
			Domain:     dom,
		})
		if errors.Is(err, domain.ErrConflict) {
			return nil // ya está en cola o copiándose: volver a lanzar trae lo nuevo, no lo duplica
		}
		if err != nil {
			return err
		}
		jobs = append(jobs, job)
		return nil
	}
	for _, p := range plan.People {
		for _, dom := range plan.Domains {
			if !dom.PerPerson() {
				continue
			}
			if err := create(p.TargetUser, p.SourceUser, dom); err != nil {
				// Los ya creados son válidos y están encolados: se devuelven con el error
				// en vez de cancelarlos porque uno posterior no cupiera.
				httpx.Fail(w, r, toAPIError(err))
				return
			}
		}
	}
	// Lo compartido lo trae quien lanza la migración, con SU cuenta del origen.
	owner := plan.People[0]
	for _, dom := range plan.Domains {
		if dom.PerPerson() {
			continue
		}
		if err := create(owner.TargetUser, owner.SourceUser, dom); err != nil {
			httpx.Fail(w, r, toAPIError(err))
			return
		}
	}
	httpx.JSON(w, r, http.StatusAccepted, map[string]any{"migrations": jobs})
}

// --- Construcción del plan ---------------------------------------------------

// buildPlan interroga al origen y empareja con el directorio de Sytue.
func (h *Handlers) buildPlan(ctx context.Context, instanceID string) (migrationPlanResp, error) {
	inst, err := h.store.LoadInstanceWithSecrets(ctx, instanceID)
	if err != nil {
		return migrationPlanResp{}, err
	}
	src, ok := sourceForTemplate(inst.TemplateKey)
	if !ok {
		return migrationPlanResp{}, httpx.ErrBadRequest.WithMessage(
			"This connection is not a migration source. The automatic assistant brings content from Google and from GitHub.",
		)
	}

	rt := h.egressRuntime(inst, log.From(ctx))
	disc, err := migrate.Discover(ctx, rt, inst, src)
	if err != nil {
		return migrationPlanResp{}, err
	}

	plan := migrationPlanResp{
		Source:         src,
		InstanceID:     instanceID,
		Connected:      true,
		ConnectedAs:    disc.ConnectedAs,
		Scope:          disc.Scope,
		CanExpandToOrg: disc.CanExpandToOrg,
		Items:          disc.Items,
		Domains:        src.Domains(),
	}
	if disc.CanExpandToOrg {
		switch src {
		case domain.SourceGoogle:
			// Con la cuenta de servicio ya subida, el enlace lleva el cliente relleno; sin
			// ella, lleva al menos los permisos exactos que hay que autorizar.
			plan.DelegationSetupURL = migrate.DelegationSetupURL(
				gws.ServiceAccountClientID(inst),
				strings.TrimSpace(inst.Config[migrate.ConfigGCPProject]) != "",
			)
		case domain.SourceM365:
			// Microsoft no pide cuenta de servicio: un administrador aprueba UNA
			// pantalla y la misma aplicación pasa a poder leer toda la organización.
			plan.DelegationSetupURL = migrate.AdminConsentURL(
				inst.Config[graph.ConfigAuthorityBase], disc.TenantID, h.oauthClientIDFor(ctx, inst),
			)
		case domain.SourceGitHub:
			// GitHub no tiene ampliación: el token ya alcanza lo que su cuenta alcanza.
		}
	}
	h.rememberDiscovery(ctx, inst, disc)

	people, unmatched, err := h.matchPeople(ctx, disc)
	if err != nil {
		return migrationPlanResp{}, err
	}
	plan.People, plan.Unmatched = people, unmatched
	plan.Jobs = len(people) * len(plan.Domains)
	return plan, nil
}

// oauthClientIDFor es el client con el que Sytue se presenta ante el proveedor: el
// de la suite si el operador lo configuró, o el que registró la organización.
func (h *Handlers) oauthClientIDFor(ctx context.Context, inst connector.Instance) string {
	if id := h.oauthClients.get(ctx, inst.TemplateKey).ClientID; id != "" {
		return id
	}
	return inst.Config[configKeyClientID]
}

// rememberDiscovery anota en la conexión lo que el origen acaba de decir de sí
// mismo: quién autorizó y en qué inquilino. No son secretos y evitan pedírselos a
// un administrador —el inquilino, además, es imprescindible para pedir después los
// tokens de aplicación con los que se migra a toda la organización—.
//
// Es best-effort: si la escritura falla, el plan que ya se ha calculado sigue
// siendo válido y se devuelve igual; solo se pierde el atajo.
func (h *Handlers) rememberDiscovery(ctx context.Context, inst connector.Instance, disc migrate.Discovery) {
	cfg := map[string]string{}
	for k, v := range inst.Config {
		cfg[k] = v
	}
	changed := false
	if disc.ConnectedAs != "" && cfg[migrate.ConfigConnectedAs] != disc.ConnectedAs {
		cfg[migrate.ConfigConnectedAs] = disc.ConnectedAs
		changed = true
	}
	if disc.TenantID != "" && cfg[graph.ConfigTenant] == "" {
		cfg[graph.ConfigTenant] = disc.TenantID
		changed = true
	}
	if !changed {
		return
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	current, err := h.store.GetInstance(ctx, inst.ID)
	if err != nil {
		return
	}
	if _, err := h.store.UpdateInstance(ctx, inst.ID, current.Name, raw, current.Status, current.LevelEnabled); err != nil {
		log.From(ctx).Warn("integrations: no se pudo anotar lo descubierto del origen",
			"instance", logsafe.Text(inst.ID), "error", err)
	}
}

// matchPeople resuelve en qué cuenta de Sytue aterriza cada cuenta del origen.
//
// El emparejamiento es por CORREO, y solo exacto o por la parte local dentro del
// directorio de la organización. No hay heurística de nombres: parecerse no es ser,
// y el precio de equivocarse es el contenido de una persona en el buzón de otra.
// Lo que no case sale en `unmatched` para que se vea, no se migra a ciegas.
func (h *Handlers) matchPeople(ctx context.Context, disc migrate.Discovery) ([]planPerson, []string, error) {
	sources := disc.SourceUsers
	if disc.Scope != migrate.ScopeOrganization || len(sources) == 0 {
		// Alcance de una sola cuenta: la que autorizó. SourceUser va VACÍO a propósito
		// —el trabajo usa el consentimiento, no la delegación— y el destino es la
		// persona de Sytue con ese correo.
		who, err := h.matchOne(ctx, disc.ConnectedAs)
		if err != nil {
			return nil, nil, err
		}
		// La cuenta que autorizó puede ser CUALQUIERA (un gmail personal). Por la
		// parte local solo se acepta si la persona emparejada es quien lanza: es
		// su cuenta antigua. Si no, admin@cualquiera.com metía su contenido en el
		// buzón de admin@<la organización> (NEG-19).
		who = cuentaConectada(who, disc.ConnectedAs, quienLlama(ctx))
		if who == nil {
			return nil, []string{disc.ConnectedAs}, nil
		}
		return []planPerson{{SourceUser: "", TargetUser: who.Email, Name: who.Name}}, nil, nil
	}

	users, err := h.directoryUsers(ctx)
	if err != nil {
		return nil, nil, err
	}
	people, unmatched := matchSources(users, sources)
	return people, unmatched, nil
}

// matchSources empareja las cuentas del origen con el directorio. Una parte local
// que se repite en el ORIGEN (ana@acme.com y ana@filial.com) no identifica a
// nadie: las dos acababan en la misma cuenta de Sytue (NEG-19). Esas solo casan
// por correo exacto.
func matchSources(users []directory.User, sources []string) ([]planPerson, []string) {
	locales := map[string]int{}
	for _, srcUser := range sources {
		l, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(srcUser)), "@")
		locales[l]++
	}
	people := make([]planPerson, 0, len(sources))
	unmatched := make([]string, 0)
	for _, srcUser := range sources {
		u := matchByEmail(users, srcUser)
		if l, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(srcUser)), "@"); u != nil && locales[l] > 1 && !strings.EqualFold(u.Email, strings.TrimSpace(srcUser)) {
			u = nil
		}
		if u == nil {
			unmatched = append(unmatched, srcUser)
			continue
		}
		people = append(people, planPerson{SourceUser: srcUser, TargetUser: u.Email, Name: u.Name})
	}
	return people, unmatched
}

// cuentaConectada decide si la cuenta que autorizó la conexión es de `who`. Esa
// cuenta puede ser CUALQUIERA (un gmail personal): por la parte local solo se
// acepta si `who` es quien lanza, que trae su cuenta antigua. Si no,
// admin@cualquiera.com metía su contenido en el buzón de admin@<la organización>
// (NEG-19).
func cuentaConectada(who *directory.User, connectedAs, caller string) *directory.User {
	if who == nil || strings.EqualFold(who.Email, strings.TrimSpace(connectedAs)) {
		return who
	}
	if caller != "" && strings.EqualFold(who.Email, caller) {
		return who
	}
	return nil
}

// quienLlama es el correo de quien hace la petición.
func quienLlama(ctx context.Context) string {
	c, _ := authn.FromContext(ctx)
	return strings.TrimSpace(c.Email)
}

// matchOne busca en el directorio a la persona con ese correo (nil si no está).
func (h *Handlers) matchOne(ctx context.Context, email string) (*directory.User, error) {
	if strings.TrimSpace(email) == "" {
		return nil, httpx.ErrBadRequest.WithMessage(
			"The provider did not say which account authorised the connection, so there is no way to tell whose the content is.",
		)
	}
	users, err := h.directoryUsers(ctx)
	if err != nil {
		return nil, err
	}
	return matchByEmail(users, email), nil
}

// directoryUsers lee el directorio de la organización activa (Sytue ID).
func (h *Handlers) directoryUsers(ctx context.Context) ([]directory.User, error) {
	if h.dir == nil {
		return nil, httpx.ErrInternal.WithMessage(
			"This deployment cannot read the organisation directory, so it cannot decide which account the content lands in.",
		)
	}
	t, err := tenancy.From(ctx)
	if err != nil {
		return nil, err
	}
	users, err := h.dir.ListUsers(ctx, t.Slug)
	if err != nil {
		return nil, httpx.NewError(http.StatusBadGateway, "directory_unavailable",
			"No se ha podido leer el directorio de la organización.").Wrap(err)
	}
	return users, nil
}

// matchByEmail empareja por correo exacto y, si no, por la parte local (alguien que
// en el origen es ana@acme.com y en Sytue ana@acme.eu). Solo se acepta si la parte
// local identifica a UNA persona: con dos candidatas no se elige, se informa.
func matchByEmail(users []directory.User, email string) *directory.User {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil
	}
	for i := range users {
		if strings.EqualFold(users[i].Email, email) {
			return &users[i]
		}
	}
	local, _, ok := strings.Cut(email, "@")
	if !ok || local == "" {
		return nil
	}
	var found *directory.User
	for i := range users {
		l, _, _ := strings.Cut(strings.ToLower(users[i].Email), "@")
		if l != local {
			continue
		}
		if found != nil {
			return nil // ambiguo: dos personas con la misma parte local.
		}
		found = &users[i]
	}
	return found
}

// planError traduce un fallo del descubrimiento. Lo que el ADMINISTRADOR puede
// arreglar (una conexión sin autorizar, una configuración a medias) es un 400 con
// el motivo dentro; lo que falla del lado del proveedor, un 502. Un 500 opaco aquí
// dejaría al asistente sin nada útil que decir.
func planError(err error) error {
	var apiErr *httpx.Error
	switch {
	case err == nil:
		return nil
	case errors.As(err, &apiErr):
		return err
	case errors.Is(err, connector.ErrBadQuery):
		msg := err.Error()
		if _, detail, found := strings.Cut(msg, connector.ErrBadQuery.Error()+": "); found {
			msg = detail
		}
		return httpx.ErrBadRequest.WithMessage(strings.ToUpper(msg[:1]) + msg[1:])
	case errors.Is(err, domain.ErrNotFound):
		return httpx.ErrNotFound
	default:
		return httpx.NewError(http.StatusBadGateway, "source_unreachable",
			"No se ha podido consultar el origen. Puede que la autorización haya caducado: vuelve a conectar.").Wrap(err)
	}
}

// sourceForTemplate traduce la plantilla de la conexión al origen de migración.
// Es la ÚNICA correspondencia entre el catálogo de apps y los orígenes: una
// plantilla que no esté aquí no es un origen y el asistente lo dice.
func sourceForTemplate(templateKey string) (domain.MigrationSource, bool) {
	switch templateKey {
	case "google-workspace":
		return domain.SourceGoogle, true
	case "github":
		return domain.SourceGitHub, true
	case "microsoft365":
		return domain.SourceM365, true
	default:
		return "", false
	}
}
