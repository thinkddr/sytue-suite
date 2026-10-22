// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// API de ADMINISTRACIÓN DE PLATAFORMA. Es la única superficie del producto que
// atraviesa organizaciones, y existe porque soporte y dirección necesitan operar
// la plataforma: dar de alta clientes, suspenderlos, fusionarlos, medir uso y
// regalar plazas.
//
// AUTORIZACIÓN, en tres candados independientes (`requirePlatformAdmin`):
//
//  1. Token válido de Sytue ID (requireAccessToken, ya aplicado por el grupo).
//  2. El usuario está en `platform_admins`. Se consulta la BASE en cada
//     petición, no un claim: retirar a alguien tiene efecto inmediato y no
//     depende de que caduque su token. Un error de consulta DENIEGA (fail-closed).
//  3. La sesión se autenticó con segundo factor (`amr`). Aquí no se hereda la
//     regla de los roles elevados: esta API se exige SIEMPRE con MFA, tenga o no
//     rol de organización quien llama.
//
// Todo lo que escribe queda AUDITADO con el actor, la organización afectada y el
// motivo. Un descuento o una suspensión sin autor ni motivo es indistinguible de
// un error seis meses después.
package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// errPlatformMFA distingue «no eres admin de plataforma» de «lo eres pero sin
// segundo factor», que se arregla volviendo a entrar con MFA.
var errPlatformMFA = httpx.ErrForbidden.
	WithCode("mfa_required").
	WithMessage("Platform administration requires signing in with two-step verification.")

// errPlatformNoActivo distingue «puedes, pero no estás elevado» de los otros dos
// candados: este lo arregla quien llama, pidiendo la elevación.
var errPlatformNoActivo = httpx.ErrForbidden.
	WithCode("activation_required").
	WithMessage("Your platform scope is not active. Activate it stating what you need it for; it expires on its own.")

// requirePlatformAdmin es el gate de toda la API de plataforma.
func (s *Server) requirePlatformAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, ok := claimsFrom(r.Context())
		if !ok || c.Subject == "" {
			httpx.Fail(w, r, httpx.ErrUnauthorized)
			return
		}
		esAdmin, err := s.store.IsPlatformAdmin(r.Context(), c.Subject)
		if err != nil {
			// Fail-closed explícito: un fallo de la base NO abre la puerta.
			s.log.Error("platform: no se pudo comprobar el alcance de plataforma",
				"user_id", c.Subject, "error", err)
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		// Los dos candados devuelven 403 y la respuesta es deliberadamente parca
		// —no se le cuenta a quien llama por qué no pasa—, pero el operador SÍ
		// necesita distinguirlos: «no está en la allowlist» y «entró sin segundo
		// factor» se arreglan en sitios distintos, y sin esta línea un despliegue
		// en rojo obliga a adivinar cuál de los dos es.
		if !esAdmin {
			s.log.WarnContext(r.Context(), "platform: denegado, el usuario no está en platform_admins",
				"user_id", c.Subject, "path", r.URL.Path)
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		if !hasMFA(c.AMR) {
			s.log.WarnContext(r.Context(), "platform: denegado, la sesión no acredita segundo factor",
				"user_id", c.Subject, "amr", c.AMR, "path", r.URL.Path)
			httpx.Fail(w, r, errPlatformMFA)
			return
		}
		// Y una ACTIVACIÓN viva (mig 0043). Estar en la allowlist dice que este
		// usuario PUEDE elevarse, no que lo esté: la elevación se pide aparte, con
		// motivo y ventana, y caduca sola. Es el mismo modelo que el PIM de
		// organización, aplicado a la superficie que manda sobre todas ellas.
		//
		// Fail-closed como los otros dos candados. Y con código propio: a diferencia
		// de «no estás en la allowlist», esto lo arregla quien llama —elevándose— y
		// decírselo le ahorra el viaje a soporte.
		vivo, err := s.store.HasLivePlatformActivation(r.Context(), c.Subject, time.Now())
		if err != nil {
			s.log.Error("platform: no se pudo comprobar la activación",
				"user_id", c.Subject, "error", err)
			httpx.Fail(w, r, httpx.ErrForbidden)
			return
		}
		if !vivo {
			s.log.WarnContext(r.Context(), "platform: denegado, sin activación viva",
				"user_id", c.Subject, "path", r.URL.Path)
			httpx.Fail(w, r, errPlatformNoActivo)
			return
		}
		next.ServeHTTP(w, r.WithContext(r.Context()))
	})
}

// hasMFA indica si la autenticación incluyó un segundo factor.
func hasMFA(amr []string) bool {
	for _, m := range amr {
		if m == "mfa" || m == "otp" || m == "webauthn" {
			return true
		}
	}
	return false
}

// platformActor devuelve el id del admin que llama (ya validado por el gate).
func platformActor(r *http.Request) string {
	c, _ := claimsFrom(r.Context())
	return c.Subject
}

// auditPlatform deja constancia de una escritura de plataforma.
func (s *Server) auditPlatform(r *http.Request, action, orgID, detail string) {
	entry := auditEntry(platformActor(r), orgID, action, detail, r)
	if err := s.store.Audit(r.Context(), nil, entry); err != nil {
		s.log.Error("platform: no se pudo auditar", "action", action, "error", err)
	}
}

// platformOrgWire es la forma JSON de una organización para la consola global.
func platformOrgWire(o store.PlatformOrg, platformSlug string) map[string]any {
	out := map[string]any{
		"id":              o.ID,
		"slug":            o.Slug,
		"name":            o.Name,
		"status":          string(o.Status),
		"namespace":       tenancy.Tenant{Slug: o.Slug}.Namespace(),
		"active_users":    o.ActiveUsers,
		"seats_purchased": o.SeatsPurchased,
		"seats_granted":   o.SeatsGranted,
		"seats_effective": o.SeatsEffective(),
		// over_capacity es la pregunta que de verdad se hace soporte: ¿esta
		// organización tiene más gente dentro que plazas pagadas + regaladas?
		"over_capacity":       o.ActiveUsers > o.SeatsEffective(),
		"subscription_status": o.SubStatus,
		"owner_emails":        o.OwnerEmails,
		"education":           o.Education,
		"created_at":          o.CreatedAt.UTC().Format(time.RFC3339),
	}
	// La que OPERA la plataforma va marcada para que la consola no ofrezca
	// suspenderla ni eliminarla. El servidor lo deniega igualmente
	// (`aplicarEstadoOrg`): esto es para no enseñar un botón que solo sabe
	// fallar, no la verja.
	if o.Slug == platformSlug {
		out["is_platform"] = true
	}
	if o.MergedInto != "" {
		out["merged_into_org_id"] = o.MergedInto
		if o.MergedAt != nil {
			out["merged_at"] = o.MergedAt.UTC().Format(time.RFC3339)
		}
	}
	return out
}

// handlePlatformListOrgs — GET|QUERY /api/v1/platform/orgs?q=&limit=.
//
// QUERY porque `q` es el nombre de una organización cliente tecleado por quien
// administra la plataforma: en la URL acaba en el log de accesos de cada salto.
// GET se queda como respaldo para los clientes que no entiendan el método.
func (s *Server) handlePlatformListOrgs(w http.ResponseWriter, r *http.Request) {
	params, err := httpx.Consulta(w, r)
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.AnunciarQuery(w)
	limit, _ := strconv.Atoi(params.Get("limit"))
	orgs, err := s.store.ListAllOrgs(r.Context(), strings.TrimSpace(params.Get("q")), limit)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(orgs))
	for _, o := range orgs {
		items = append(items, platformOrgWire(o, s.cfg.PlatformOrgSlug))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handlePlatformGetOrg — GET /api/v1/platform/orgs/{id}.
func (s *Server) handlePlatformGetOrg(w http.ResponseWriter, r *http.Request) {
	org, err := s.platformOrgByID(w, r, chi.URLParam(r, "id"))
	if err != nil {
		return
	}
	grants, err := s.store.ListSeatGrants(r.Context(), org.ID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	out := platformOrgWire(org, s.cfg.PlatformOrgSlug)
	out["seat_grants"] = grants
	httpx.JSON(w, r, http.StatusOK, out)
}

// platformOrgByID resuelve y valida el id de la ruta, respondiendo ya el error.
func (s *Server) platformOrgByID(w http.ResponseWriter, r *http.Request, id string) (store.PlatformOrg, error) {
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return store.PlatformOrg{}, err
	}
	org, err := s.store.GetPlatformOrg(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
		} else {
			httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		}
		return store.PlatformOrg{}, err
	}
	return org, nil
}

// handlePlatformCreateOrg — POST /api/v1/platform/orgs.
//
// Alta manual de un cliente: la vía para dar de alta a alguien que compra por
// contrato y no por el asistente de onboarding. Se crea con una suscripción
// pendiente para que el aforo y la facturación existan desde el minuto cero.
func (s *Server) handlePlatformCreateOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slug       string `json:"slug"`
		Name       string `json:"name"`
		OwnerEmail string `json:"owner_email"`
		Seats      int    `json:"seats"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	in.Slug = strings.ToLower(strings.TrimSpace(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = in.Slug
	}
	if !tenancy.ValidSlug(in.Slug) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The identifier is not valid."))
		return
	}
	if domain.SlugReserved(in.Slug) {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("That identifier is reserved."))
		return
	}
	if in.Seats <= 0 {
		in.Seats = 1
	}

	var org domain.Organization
	ctx := r.Context()
	err := s.store.InTx(ctx, func(q pg.Querier) error {
		var err error
		org, err = s.store.CreateOrg(ctx, q, domain.Organization{
			Slug: in.Slug, Name: in.Name, Status: domain.StatusActive,
		})
		if err != nil {
			return err
		}
		if err := s.store.EnsurePendingSubscription(ctx, q, org.ID, in.Seats); err != nil {
			return err
		}
		// El propietario es opcional: una organización puede crearse antes de
		// saber quién la administrará. Si se indica, debe existir ya como usuario
		// (crear cuentas es competencia de la invitación, con su correo y su
		// verificación, no de un alta manual silenciosa).
		if in.OwnerEmail == "" {
			return nil
		}
		u, err := s.store.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(in.OwnerEmail)))
		if err != nil {
			return err
		}
		return s.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: u.ID, Role: domain.RoleOwner,
		})
	})
	switch {
	case errors.Is(err, domain.ErrConflict):
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("An organisation with that identifier already exists."))
		return
	case errors.Is(err, domain.ErrNotFound):
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("There is no user with that email address."))
		return
	case err != nil:
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	s.auditPlatform(r, "platform.org.created", org.ID, in.Slug)
	httpx.JSON(w, r, http.StatusCreated, map[string]any{
		"id": org.ID, "slug": org.Slug, "name": org.Name, "status": string(org.Status),
	})
}

// handlePlatformSetOrgStatus — POST /api/v1/platform/orgs/{id}/status.
//
// Suspender corta el acceso de toda la organización sin borrar nada; reactivar
// lo devuelve. Una organización fusionada NO se reactiva: su estado es terminal.
//
// SUSPENDER exige una segunda persona (mig 0046): deja fuera a una empresa
// entera, y hasta ahora bastaba con que una cuenta elevada lo pidiera.
// Reactivar no la exige — devolver el servicio no es el peligro, y ponerle
// trabas convertiría el arreglo de un error en otra espera.
func (s *Server) handlePlatformSetOrgStatus(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	var st domain.Status
	switch in.Status {
	case "suspended":
		st = domain.StatusSuspended
	case "active":
		st = domain.StatusActive
	case "deleted":
		// ELIMINAR ES UN ESTADO, NO UN `DELETE FROM`.
		//
		// `organizations` solo tiene DOS claves foráneas con `ON DELETE CASCADE`
		// apuntándole; el aislamiento del resto del esquema es por RLS y columna
		// `org_id`, sin FK. Un borrado físico dejaría huérfanas las membresías, los
		// buzones, los grupos, los roles y —lo peor— el registro de auditoría, que
		// es justo lo que hay que conservar cuando alguien se va.
		//
		// El estado ya existía en el dominio (`StatusDeleted`) y la base ya lo
		// admite en su CHECK desde la primera migración: lo único que faltaba era
		// poder pedirlo. Eso deja la fila para responder «qué fue de aquella
		// empresa» y permite deshacerlo reactivándola.
		st = domain.StatusDeleted
	default:
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"The status must be active, suspended or deleted.",
		))
		return
	}
	// El motivo es obligatorio. Lo era desde el principio como INVARIANTE —«toda
	// escritura exige motivo», y este handler es el ejemplo literal del documento
	// (security.md §4.9)— pero lo exigía únicamente el navegador, con el
	// window.prompt() del panel. Cualquier llamada directa suspendía a una empresa
	// entera dejando en el registro «suspended: », que no responde nada seis meses
	// después; y la solicitud de segunda firma se creaba igual de muda, que es lo
	// que el otro admin tiene que leer para decidir. Aquí porque es el único punto
	// por el que pasan las dos vías, la directa y la aprobada.
	in.Reason = trim(in.Reason)
	if in.Reason == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A reason is required."))
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	// Las dos direcciones peligrosas exigen segunda firma; reactivar no, porque
	// devolver el servicio no es el riesgo (ver la cabecera de este handler).
	switch st {
	case domain.StatusSuspended:
		if !s.pedirSegundaPersona(w, r, "org.suspend",
			payloadSuspend{OrgID: id, Reason: in.Reason}, in.Reason) {
			return
		}
	case domain.StatusDeleted:
		if !s.pedirSegundaPersona(w, r, "org.delete",
			payloadSuspend{OrgID: id, Reason: in.Reason}, in.Reason) {
			return
		}
	case domain.StatusActive:
		// Reactivar NO pide segunda firma, y va escrito como caso en vez de caer
		// por omisión: así el día que aparezca un estado nuevo el compilador
		// obliga a decidir si es peligroso, en vez de dejarlo pasar sin firma.
	}
	org, err := s.aplicarEstadoOrg(r, id, st, in.Reason)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"id": org.ID, "slug": org.Slug, "status": string(org.Status),
	})
}

// aplicarEstadoOrg cambia el estado y lo audita. Lo comparten la vía directa y
// la aprobación de una segunda persona: si cada una lo hiciera por su cuenta,
// una acabaría auditando distinto que la otra.
func (s *Server) aplicarEstadoOrg(r *http.Request, id string, st domain.Status, reason string) (domain.Organization, error) {
	// La organización que OPERA la plataforma no se suspende ni se elimina.
	//
	// No es una preferencia: sus miembros son los administradores globales, y
	// `createSession` deniega el login a quien pertenece a una organización que
	// no está activa. Suspenderla —o eliminarla— deja a TODO el mundo fuera,
	// incluidos los que tendrían que reactivarla, y no hay otra palanca: no
	// existe una consola por encima de esta. Un clic, y la plataforma se queda
	// sin nadie que pueda entrar a arreglarlo.
	//
	// Se comprueba aquí, en el punto por el que pasan la vía directa y la
	// aprobada por una segunda persona; ponerlo solo en el handler dejaría la
	// otra abierta. Cualquier organización CLIENTE se sigue pudiendo eliminar.
	// Reactivar una organización FUSIONADA no: sus datos ya viven en la de
	// destino, así que devolverla al servicio pondría dos sirviendo lo mismo.
	// Eliminarla sí se permite —es retirar de la lista algo que ya no opera—.
	if st == domain.StatusActive {
		if org, err := s.store.GetOrgByID(r.Context(), id); err == nil &&
			org.Status == domain.Status("merged") {
			return domain.Organization{}, httpx.ErrConflict.WithMessage(
				"That organisation was merged into another one and cannot be reactivated.",
			)
		}
	}
	if st != domain.StatusActive {
		if org, err := s.store.GetOrgByID(r.Context(), id); err == nil &&
			org.Slug == s.cfg.PlatformOrgSlug {
			return domain.Organization{}, httpx.ErrForbidden.WithMessage(
				"The organisation that operates the platform cannot be suspended or deleted: " +
					"its members are the global administrators, and they would all lose access.",
			)
		}
	}
	org, err := s.store.SetOrgStatus(r.Context(), nil, id, st)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Organization{}, httpx.ErrNotFound.WithMessage(
				"That organisation does not exist, or it has been merged and its status no longer changes.",
			)
		}
		return domain.Organization{}, err
	}
	s.auditPlatform(r, "platform.org.status", org.ID, string(st)+": "+reason)
	return org, nil
}

// --- Plazas gratuitas -------------------------------------------------------

// handlePlatformGrantSeats — POST /api/v1/platform/orgs/{id}/seat-grants.
//
// Amplía el aforo sin tocar la suscripción: gestos comerciales y plazas de
// empleados de Sytue. Se cuenta aparte de `seats` para que la factura siga
// siendo lo vendido y para poder responder qué se regaló, a quién y por qué.
func (s *Server) handlePlatformGrantSeats(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Seats     int    `json:"seats"`
		Reason    string `json:"reason"`
		Kind      string `json:"kind"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	org, err := s.platformOrgByID(w, r, chi.URLParam(r, "id"))
	if err != nil {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Seats <= 0 || in.Seats > 10000 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Seats must be between 1 and 10000."))
		return
	}
	if in.Reason == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A reason is required."))
		return
	}
	if in.Kind == "" {
		in.Kind = "goodwill"
	}
	kindOK := false
	for _, k := range store.SeatGrantKinds {
		if k == in.Kind {
			kindOK = true
			break
		}
	}
	if !kindOK {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"The type must be one of: "+strings.Join(store.SeatGrantKinds, ", ")+".",
		))
		return
	}
	g := store.SeatGrant{
		OrgID: org.ID, Seats: in.Seats, Reason: in.Reason, Kind: in.Kind,
		GrantedBy: platformActor(r),
	}
	if in.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, in.ExpiresAt)
		if err != nil {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The expiry must be in RFC3339 format."))
			return
		}
		g.ExpiresAt = &t
	}
	out, err := s.store.CreateSeatGrant(r.Context(), nil, g)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Los asientos concedidos son uno de los tres factores del derecho, así que
	// conceder o quitar uno invalida lo cacheado (ver entitledTTL).
	s.invalidarEntitled(r.Context(), org.ID)
	s.auditPlatform(r, "platform.seats.granted", org.ID,
		strconv.Itoa(in.Seats)+" ("+in.Kind+"): "+in.Reason)
	httpx.JSON(w, r, http.StatusCreated, out)
}

// handlePlatformRevokeSeatGrant — DELETE /api/v1/platform/seat-grants/{id}.
func (s *Server) handlePlatformRevokeSeatGrant(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	g, err := s.store.RevokeSeatGrant(r.Context(), nil, id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Retirar asientos puede quitarle el derecho a la organización, así que lo
	// cacheado deja de valer aquí mismo (ver entitledTTL).
	s.invalidarEntitled(r.Context(), g.OrgID)
	// Con la organización y el motivo por el que se habían dado: el registro
	// decía «se retiró la concesión <uuid>», que no responde a quién ni cuántas
	// —y ese uuid ya no lo va a resolver nadie seis meses después—.
	s.auditPlatform(r, "platform.seats.revoked", g.OrgID,
		strconv.Itoa(g.Seats)+" ("+g.Kind+"): "+g.Reason)
	w.WriteHeader(http.StatusNoContent)
}

// --- Uso de la plataforma ---------------------------------------------------

// handlePlatformUsage — GET /api/v1/platform/usage.
func (s *Server) handlePlatformUsage(w http.ResponseWriter, r *http.Request) {
	u, err := s.store.PlatformUsageNow(r.Context(), time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, u)
}

// --- Admins de plataforma ---------------------------------------------------

// avisarCambioDeAlcance manda un correo a quien gana o pierde el alcance global
// y AL RESTO de administradores de plataforma.
//
// Que alguien reciba el permiso más amplio del sistema sin que nadie más se
// entere es el agujero que esto cierra: la auditoría ya lo escribía, pero un
// registro solo lo lee quien va a mirarlo, y a estas cosas se llega tarde.
//
// Va en segundo plano y ha de sobrevivir a la respuesta: el correo NO puede hacer
// fallar la concesión —el permiso ya está dado cuando esto corre— ni morir porque
// la petición haya respondido. Un envío fallido se registra; no se reintenta,
// porque el hecho ya está en la auditoría y esto solo lo acerca.
//
// El contexto se deriva del de la petición con context.WithoutCancel en vez de
// partir de context.Background(): así se descarta SOLO la cancelación y se
// conservan los valores que puso el middleware (traza, tenant, id de petición),
// que son justo los que permiten atar este envío al cambio que lo provocó.
func (s *Server) avisarCambioDeAlcance(ctx context.Context, sujeto, actorEmail, motivo string, concedido bool) {
	base := context.WithoutCancel(ctx)
	go func() {
		ctx, cancel := context.WithTimeout(base, 30*time.Second)
		defer cancel()
		admins, err := s.store.ListPlatformAdmins(ctx)
		if err != nil {
			s.log.Error("aviso de alcance: no se pudo listar a quién avisar", "error", err)
			return
		}
		// El sujeto primero, aunque acabe de perder el alcance y ya no esté en la
		// lista: es a quien más le importa.
		destinos := map[string]bool{sujeto: true}
		for _, a := range admins {
			destinos[a.Email] = true
		}
		for to := range destinos {
			if to == "" {
				continue
			}
			if err := s.mailer.SendPlatformScopeChange(ctx, to, sujeto, actorEmail, motivo, concedido); err != nil {
				s.log.Error("aviso de alcance: no se pudo enviar", "to", to, "error", err)
			}
		}
	}()
}

// actorEmail resuelve el correo de quien llama, para poder nombrarlo en el
// aviso. Si no se puede, se dice así: mejor «un administrador» que un uuid.
func (s *Server) actorEmail(r *http.Request) string {
	u, err := s.store.GetUserByID(r.Context(), platformActor(r))
	if err != nil {
		return "un administrador de plataforma"
	}
	return u.Email
}

// platformReviewDays es cada cuánto hay que volver a mirar quién tiene alcance
// global. Tres meses: lo bastante para no ser una molestia mensual, lo bastante
// poco para que un acceso dado «para una migración» no sobreviva un año entero.
// Va en la respuesta para que el panel no tenga que repetir el número.
const platformReviewDays = 90

// handlePlatformListAdmins — GET /api/v1/platform/admins.
//
// Cada fila dice cuándo se revisó por última vez y si toca revisarla (mig 0047).
// La caducidad de la revisión se calcula al leer, como todo lo demás de esta
// API: nada la persiste ni hay barrido que la ponga.
func (s *Server) handlePlatformListAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := s.store.ListPlatformAdmins(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	limite := time.Now().AddDate(0, 0, -platformReviewDays)
	items := make([]map[string]any, 0, len(admins))
	for _, a := range admins {
		items = append(items, map[string]any{
			"user_id":     a.UserID,
			"email":       a.Email,
			"name":        a.Name,
			"granted_by":  a.GrantedBy,
			"reason":      a.Reason,
			"created_at":  a.CreatedAt.UTC().Format(time.RFC3339),
			"reviewed_at": a.ReviewedAt.UTC().Format(time.RFC3339),
			"reviewed_by": a.ReviewedBy,
			"review_due":  a.ReviewedAt.Before(limite),
		})
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"items": items, "review_days": platformReviewDays,
	})
}

// handlePlatformReviewAdmin — POST /api/v1/platform/admins/{id}/review — «he
// mirado este acceso y sigue haciendo falta».
//
// NO se puede revisar el propio: una revisión en la que cada uno se confirma a
// sí mismo no es una revisión, es un botón. Confirmar el de otro sí, que es el
// caso normal cuando alguien repasa la lista entera.
func (s *Server) handlePlatformReviewAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if id == platformActor(r) {
		httpx.Fail(w, r, httpx.ErrForbidden.WithCode("same_person").WithMessage(
			"Tu propio alcance lo revisa otro administrador global.",
		))
		return
	}
	existia, err := s.store.ReviewPlatformAdmin(r.Context(), nil, id, platformActor(r), time.Now())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if !existia {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	s.auditPlatform(r, "platform.admin.reviewed", "", id)
	httpx.JSON(w, r, http.StatusOK, map[string]any{"reviewed": true})
}

// handlePlatformGrantAdmin — POST /api/v1/platform/admins.
//
// Exige una SEGUNDA PERSONA (mig 0046), como suspender y fusionar. Es el permiso
// más amplio del sistema —quien lo recibe puede suspender a cualquier cliente,
// fusionar organizaciones y repartir este mismo alcance—, y hasta ahora se
// concedía con una sola firma mientras suspender a UN cliente, que es reversible,
// exigía dos. El riesgo estaba justo al revés: una única cuenta elevada y
// comprometida podía fabricarse compañía permanente en un solo POST.
//
// El aviso por correo a todos los admins sigue estando y no sustituye a esto:
// avisa DESPUÉS, y a estas cosas se llega tarde.
func (s *Server) handlePlatformGrantAdmin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email  string `json:"email"`
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Reason == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A reason is required."))
		return
	}
	u, err := s.store.GetUserByEmail(r.Context(), strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("There is no user with that email address."))
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	// Después de validar, para no dejar registrada una solicitud sobre un correo
	// que no existe.
	if !s.pedirSegundaPersona(w, r, "platform.admin.grant",
		payloadGrantAdmin{UserID: u.ID, Reason: in.Reason}, u.Email+": "+in.Reason) {
		return
	}
	if err := s.concederAlcance(r, u.ID, u.Email, in.Reason); err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusCreated, map[string]any{"user_id": u.ID, "email": u.Email})
}

// concederAlcance inscribe en `platform_admins`, lo audita y avisa. Lo comparten
// la vía directa (cuando solo hay un admin elegible) y la segunda firma: si cada
// una lo hiciera por su cuenta, una acabaría auditando o avisando distinto que la
// otra, y el aviso es medio mecanismo de control.
func (s *Server) concederAlcance(r *http.Request, userID, email, motivo string) error {
	if err := s.store.UpsertPlatformAdmin(r.Context(), nil, userID, platformActor(r), motivo); err != nil {
		return err
	}
	s.auditPlatform(r, "platform.admin.granted", "", email+": "+motivo)
	s.avisarCambioDeAlcance(r.Context(), email, s.actorEmail(r), motivo, true)
	return nil
}

// handlePlatformRevokeAdmin — DELETE /api/v1/platform/admins/{id}.
//
// No se admite quedarse sin ningún admin de plataforma: recuperarlo exigiría
// volver a ejecutar el bootstrap del despliegue. Tampoco revocarse a uno mismo,
// que es la forma habitual de llegar a cero por accidente.
func (s *Server) handlePlatformRevokeAdmin(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	if id == platformActor(r) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"You cannot remove your own platform scope.",
		))
		return
	}
	n, err := s.store.CountPlatformAdmins(r.Context())
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if n <= 1 {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
			"This is the last platform admin: appoint another one before removing them.",
		))
		return
	}
	// El correo se resuelve ANTES de retirar: después sigue existiendo el
	// usuario, pero el aviso quiere decir a quién se le quitó, no un uuid, y
	// leerlo aquí evita depender del orden de dos consultas.
	quien, err := s.store.GetUserByID(r.Context(), id)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if err := s.store.RevokePlatformAdmin(r.Context(), nil, id); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	s.auditPlatform(r, "platform.admin.revoked", "", quien.Email)
	s.avisarCambioDeAlcance(r.Context(), quien.Email, s.actorEmail(r), "", false)
	w.WriteHeader(http.StatusNoContent)
}

// --- Fusión de organizaciones ----------------------------------------------

// mergeWire es la forma JSON de una fusión.
func mergeWire(m store.OrgMerge) map[string]any {
	return map[string]any{
		"id":            m.ID,
		"source_org_id": m.SourceOrgID,
		"target_org_id": m.TargetOrgID,
		"status":        m.Status,
		"services":      m.Services,
		"error":         m.Error,
		"requested_by":  m.RequestedBy,
		"created_at":    m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// handlePlatformListMerges — GET /api/v1/platform/merges.
func (s *Server) handlePlatformListMerges(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	merges, err := s.store.ListOrgMerges(r.Context(), limit)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	items := make([]map[string]any, 0, len(merges))
	for _, m := range merges {
		items = append(items, mergeWire(m))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"items": items})
}

// handlePlatformGetMerge — GET /api/v1/platform/merges/{id}.
func (s *Server) handlePlatformGetMerge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	m, err := s.store.GetOrgMerge(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	httpx.JSON(w, r, http.StatusOK, mergeWire(m))
}

// handlePlatformCreateMerge — POST /api/v1/platform/merges.
//
// Fusiona el ORIGEN dentro del DESTINO: los miembros pasan al destino y los
// datos de los diez servicios se reetiquetan con el tenant del destino. El
// origen queda en estado `merged`, que es terminal.
//
// La petición exige repetir el slug del origen en `confirm_source_slug`. No es
// ceremonia: es la única operación de esta API que no se puede deshacer, y un
// id equivocado en una llamada fusionaría al cliente que no era.
func (s *Server) handlePlatformCreateMerge(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SourceOrgID string `json:"source_org_id"`
		TargetOrgID string `json:"target_org_id"`
		ConfirmSlug string `json:"confirm_source_slug"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	if in.SourceOrgID == in.TargetOrgID {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Source and target are the same organisation."))
		return
	}
	source, err := s.platformOrgByID(w, r, in.SourceOrgID)
	if err != nil {
		return
	}
	target, err := s.platformOrgByID(w, r, in.TargetOrgID)
	if err != nil {
		return
	}
	if in.ConfirmSlug != source.Slug {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Type the source organisation identifier again to confirm the merge.",
		))
		return
	}
	if source.Status == domain.Status("merged") || target.Status == domain.Status("merged") {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("An organisation that has already been merged cannot be merged again."))
		return
	}
	// Segunda persona (mig 0046). Repetir el slug protege del despiste; esto, de
	// que una sola cuenta pueda fusionar al cliente equivocado sin que nadie más
	// lo vea. Va después de las validaciones para no registrar una solicitud
	// sobre algo que ya se sabe que no se puede hacer.
	if !s.pedirSegundaPersona(w, r, "org.merge",
		payloadMerge{SourceOrgID: source.ID, TargetOrgID: target.ID},
		source.Slug+" → "+target.Slug) {
		return
	}
	m, err := s.arrancarFusion(r, source.ID, target.ID)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	httpx.JSON(w, r, http.StatusAccepted, mergeWire(m))
}

// arrancarFusion valida el estado ACTUAL de las dos organizaciones, crea la
// fusión y la lanza. Lo comparten la vía directa y la aprobación de una segunda
// persona: entre pedir y firmar pasa hasta un día, así que las comprobaciones
// tienen que rehacerse en el momento de ejecutar y no valer las de entonces.
func (s *Server) arrancarFusion(r *http.Request, sourceID, targetID string) (store.OrgMerge, error) {
	source, err := s.store.GetPlatformOrg(r.Context(), sourceID)
	if err != nil {
		return store.OrgMerge{}, err
	}
	target, err := s.store.GetPlatformOrg(r.Context(), targetID)
	if err != nil {
		return store.OrgMerge{}, err
	}
	if source.Status == domain.Status("merged") || target.Status == domain.Status("merged") {
		return store.OrgMerge{}, httpx.ErrConflict.WithMessage(
			"An organisation that has already been merged cannot be merged again.",
		)
	}
	m, err := s.store.CreateOrgMerge(r.Context(), nil, source.ID, target.ID, platformActor(r))
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return store.OrgMerge{}, httpx.ErrConflict.WithMessage(
				"There is already an open merge involving one of the two organisations.",
			)
		}
		return store.OrgMerge{}, err
	}
	s.auditPlatform(r, "platform.merge.requested", source.ID, source.Slug+" → "+target.Slug)
	//nolint:contextcheck // runMerge corre en segundo plano con contexto propio a propósito.
	s.runMerge(m.ID, source.Slug, target.Slug, source.ID, target.ID)
	return m, nil
}

// handlePlatformResumeMerge — POST /api/v1/platform/merges/{id}/resume.
//
// Reanuda una fusión fallida. Los servicios ya movidos no se repiten porque el
// criterio de cada UPDATE es `tenant = origen`: tras un éxito, esa consulta no
// encuentra nada que mover.
func (s *Server) handlePlatformResumeMerge(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	m, err := s.store.GetOrgMerge(r.Context(), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			httpx.Fail(w, r, httpx.ErrNotFound)
			return
		}
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	if m.Status != "failed" && m.Status != "pending" {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("Only a pending or failed merge can be resumed."))
		return
	}
	source, err := s.store.GetOrgByID(r.Context(), m.SourceOrgID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	target, err := s.store.GetOrgByID(r.Context(), m.TargetOrgID)
	if err != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(err))
		return
	}
	s.auditPlatform(r, "platform.merge.resumed", m.SourceOrgID, m.ID)
	//nolint:contextcheck // runMerge corre en segundo plano con contexto propio a propósito.
	s.runMerge(m.ID, source.Slug, target.Slug, m.SourceOrgID, m.TargetOrgID)
	httpx.JSON(w, r, http.StatusAccepted, mergeWire(m))
}

// handlePlatformSetOrgEducation — POST /api/v1/platform/orgs/{id}/education —
// marca (o desmarca) a una organización como institución educativa: su plan
// Pro cuesta commerce.EducationDiscountPercent menos. Con motivo y auditado.
func (s *Server) handlePlatformSetOrgEducation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Education bool   `json:"education"`
		Reason    string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &in); err != nil {
		return
	}
	if in.Reason = trim(in.Reason); in.Reason == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A reason is required."))
		return
	}
	id := chi.URLParam(r, "id")
	if _, err := uuid.Parse(id); err != nil {
		httpx.Fail(w, r, httpx.ErrNotFound)
		return
	}
	org, err := s.store.SetOrgEducation(r.Context(), id, in.Education)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	c, _ := claimsFrom(r.Context())
	_ = s.store.Audit(r.Context(), nil, auditEntry(c.Subject, org.ID, "platform.org.education", in.Reason, r))
	httpx.JSON(w, r, http.StatusOK, map[string]any{"id": org.ID, "slug": org.Slug, "education": org.Education})
}
