// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Alta inicial guiada (mig 0027): el primer contacto de un cliente con Sytue.
// Un asistente de 4-6 pasos (según sea para una persona o para una organización)
// cuyo estado se PERSISTE en cada paso, de modo que si cierra el navegador vuelve
// donde lo dejó con lo ya introducido.
//
// Principio de estos endpoints: NADA es decorativo. La disponibilidad del
// identificador se consulta contra la BD; la dirección <slug>.sytue.eu se PUBLICA
// de verdad en nuestra zona de Cloudflare (registro CNAME real, estado real); el
// dominio propio se verifica con una consulta DNS REAL (TXT); la disponibilidad y
// el PRECIO de un dominio que aún no se tiene salen del registrador de Cloudflare;
// las invitaciones son las MISMAS que las de la Console (mismo token firmado,
// mismo correo) y fallan visiblemente si el correo no está configurado; los roles
// recomendados se crean como roles personalizados reales (mig 0019) con permisos
// del catálogo fijo; y el resumen se construye leyendo lo que hay en la BD, no lo
// que el navegador dice que escribió.
//
// Sytue no vende dominios: el registrador de Cloudflare tiene API para comprarlos,
// pero cobraría a la cuenta de plataforma de Sytue. Hasta que haya forma de
// cobrárselo al cliente no hay botón de compra ni se insinúa (ver internal/dns).
//
// AUTORIZACIÓN. Estos endpoints los usa alguien que puede NO tener todavía
// organización (ni rol, ni claim `tenant`), así que no cuelgan de requireRole. El
// anclaje de aislamiento es su PROPIA fila de onboarding: la org sale de ahí y se
// re-verifica contra la BD que el solicitante es OWNER de ella (fail-closed). Un
// org_id ajeno no autoriza nada, y cada usuario solo ve/escribe su propio progreso.
package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/pg"
	"git.sytue.eu/sytue/one/platform/pkg/tenancy"
	"git.sytue.eu/sytue/one/services/id/internal/dns"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// maxBatchInvites acota una tanda de invitaciones del alta. Es una cortesía del
// primer día (invitar al equipo de golpe), no una importación masiva: para eso
// están SCIM y el directorio de la Console.
const maxBatchInvites = 25

// --- Lectura del estado + resumen REAL -------------------------------------

// handleOnboardingGet — GET /api/v1/onboarding — devuelve dónde se quedó el
// usuario y, si ya creó su organización, lo que REALMENTE existe en el backend:
// la org con su subdominio, el dominio propio y su verificación, los datos
// fiscales, las invitaciones vigentes y los roles creados. El asistente pinta
// esto tal cual: el resumen final no se construye con el estado del formulario.
func (s *Server) handleOnboardingGet(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	ctx := r.Context()

	ob, err := s.store.GetOnboarding(ctx, c.Subject)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}

	// Pertenecer YA a una organización es estar dentro: no hay nada que
	// configurar. A quien entra por invitación nadie le va a preguntar «¿para
	// quién es Sytue?» —su organización existe, alguien la administra y él acaba
	// de ser añadido—, y sin esto lo veía igual, porque `completed` solo se
	// marcaba al terminar el asistente y un invitado no pasa por él nunca.
	//
	// PERO ese atajo solo vale para el INVITADO PURO, que es quien no tiene alta
	// propia. Con la condición a secas —«tiene alguna membresía»— también caía
	// quien ACABA de crear una organización teniendo ya otra: su alta se declaraba
	// terminada al instante y el asistente no volvía a guiarle.
	//
	// Medido contra producción el 2026-08-13 con una organización recién creada:
	// `completed: true` conviviendo con `step: "identidad"` —el primero—, sin
	// dominio (`dns.status: unconfigured`), sin suscripción (`pending`,
	// `entitled: false`), sin invitaciones y sin roles. Dos hechos contradictorios
	// en la misma respuesta.
	//
	// Y no es un detalle cosmético: conectar un dominio propio SOLO se podía hacer
	// dentro del asistente, así que declararlo terminado antes de tiempo dejaba a
	// la organización sin ninguna puerta para hacerlo (es el origen del «el
	// asistente se pasa una vez» que anota admin_domain.go).
	//
	// `ob.OrgID` distingue los dos casos sin ambigüedad: `handleOnboardingCreateOrg`
	// lo graba junto a `complete=false` al crear la organización, y un invitado que
	// nunca pasó por aquí no tiene fila —GetOnboarding devuelve el valor cero—.
	completado := ob.CompletedAt != nil
	if !completado && ob.OrgID == "" {
		if mems, merr := s.store.ListMembershipsForUser(ctx, c.Subject); merr == nil && len(mems) > 0 {
			completado = true
		}
	}

	out := map[string]any{
		"mode":              ob.Mode,
		"step":              ob.Step,
		"completed":         completado,
		"steps":             domain.OnboardingSteps(domain.OnboardingMode(ob.Mode)),
		"suite_domain":      suiteDomain(s.cfg.Issuer),
		"recommended_roles": recommendedRolesWire(),
		"organization":      nil,
	}
	if u, uerr := s.store.GetUserByID(ctx, c.Subject); uerr == nil {
		out["account"] = map[string]any{"email": u.Email, "name": u.Name}
	}
	if ob.OrgID == "" {
		httpx.JSON(w, r, http.StatusOK, out)
		return
	}
	org, oerr := s.store.GetOrgByID(ctx, ob.OrgID)
	if oerr != nil {
		httpx.Fail(w, r, mapDomainErr(oerr))
		return
	}
	out["organization"] = s.orgWire(ctx, org, r)
	prof, perr := s.store.GetOrgProfile(ctx, org.ID)
	if perr != nil {
		httpx.Fail(w, r, mapDomainErr(perr))
		return
	}
	if prof.Domain != "" {
		out["domain"] = domainWire(prof, domain.DNSCheck{})
	}
	out["billing"] = map[string]any{
		"legal_name":    prof.LegalName,
		"tax_id":        prof.TaxID,
		"address":       prof.Address,
		"billing_email": prof.BillingEmail,
	}
	invs, _ := s.store.ListPendingInvitations(ctx, org.ID)
	items := make([]map[string]any, 0, len(invs))
	for _, i := range invs {
		items = append(items, map[string]any{
			"email": i.Email, "role": i.Role,
			"expires_at": i.ExpiresAt.UTC().Format(time.RFC3339),
		})
	}
	out["invitations"] = items
	roles, _ := s.store.ListCustomRoles(ctx, org.ID)
	rw := make([]map[string]any, 0, len(roles))
	for _, role := range roles {
		rw = append(rw, map[string]any{
			"id": role.ID, "name": role.Name, "permissions": role.Permissions,
		})
	}
	out["roles"] = rw
	httpx.JSON(w, r, http.StatusOK, out)
}

// recommendedRolesWire expone el catálogo de roles recomendados CON sus permisos
// reales. El frontend no guarda una copia: la explicación que lee el usuario y
// los permisos que se crearán salen de la misma fuente (domain.RecommendedRoles).
func recommendedRolesWire() []map[string]any {
	out := make([]map[string]any, 0, len(domain.RecommendedRoles))
	for _, r := range domain.RecommendedRoles {
		out = append(out, map[string]any{
			"key": r.Key, "name": r.Name, "summary": r.Summary,
			"permissions": r.PermissionStrings(),
		})
	}
	return out
}

// domainWire serializa el estado del dominio propio. El registro que hay que
// crear se muestra SIEMPRE (para poder copiarlo), verificado o no.
func domainWire(p store.OrgProfile, chk domain.DNSCheck) map[string]any {
	m := map[string]any{
		"host":         p.Domain,
		"record_name":  domain.RecordName(p.Domain),
		"record_value": domain.RecordValue(p.DomainToken),
		"verified":     p.DomainVerifiedAt != nil,
	}
	if p.DomainVerifiedAt != nil {
		m["verified_at"] = p.DomainVerifiedAt.UTC().Format(time.RFC3339)
	}
	if chk.Status != "" {
		m["status"] = string(chk.Status)
		if len(chk.Found) > 0 {
			m["found"] = chk.Found
		}
	}
	return m
}

// --- Guardado del paso ------------------------------------------------------

type onboardingPatchRequest struct {
	Mode      string `json:"mode"`
	Step      string `json:"step"`
	Completed bool   `json:"completed"`
	// Billing es opcional: si no viene, no se toca. Todos sus campos son
	// opcionales — omitir la facturación NO bloquea el alta.
	Billing *billingRequest `json:"billing"`
}

type billingRequest struct {
	LegalName    string `json:"legal_name"`
	TaxID        string `json:"tax_id"`
	Address      string `json:"address"`
	BillingEmail string `json:"billing_email"`
}

// handleOnboardingPatch — PATCH /api/v1/onboarding — guarda el paso en curso.
// Lo llama el asistente en CADA avance: es lo que hace real la reanudación.
// Campos vacíos = no tocar (un PATCH parcial no borra lo ya guardado).
func (s *Server) handleOnboardingPatch(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req onboardingPatchRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	cur, err := s.store.GetOnboarding(ctx, c.Subject)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	mode := domain.OnboardingMode(strings.TrimSpace(req.Mode))
	if mode != "" && !domain.ValidOnboardingMode(mode) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid mode (individual|organization)."))
		return
	}
	effective := mode
	if effective == "" {
		effective = domain.OnboardingMode(cur.Mode)
	}
	step := strings.TrimSpace(req.Step)
	if step != "" && !domain.ValidOnboardingStep(effective, step) {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid step for this journey."))
		return
	}

	// La facturación es de la ORGANIZACIÓN: exige que ya exista (paso posterior al
	// de identidad) y que el solicitante sea su owner.
	if req.Billing != nil {
		org, oerr := s.onboardingOrg(ctx, c.Subject)
		if oerr != nil {
			httpx.Fail(w, r, oerr)
			return
		}
		b := req.Billing
		if e := strings.TrimSpace(b.BillingEmail); e != "" {
			addr, perr := mail.ParseAddress(e)
			if perr != nil {
				httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("The billing email address is not valid."))
				return
			}
			b.BillingEmail = strings.ToLower(addr.Address)
		}
		if _, uerr := s.store.UpsertOrgBilling(ctx, nil, org.ID,
			domain.SanitizeDisplayName(b.LegalName), domain.SanitizeDisplayName(b.TaxID),
			domain.SanitizeDisplayName(b.Address), b.BillingEmail); uerr != nil {
			httpx.Fail(w, r, mapDomainErr(uerr))
			return
		}
	}

	ob, err := s.store.UpsertOnboarding(ctx, nil, c.Subject, string(mode), step, "", req.Completed)
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	if req.Completed && ob.OrgID != "" {
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, ob.OrgID, "org.onboarding.completed", ob.Mode, r))
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{
		"mode": ob.Mode, "step": ob.Step, "completed": ob.CompletedAt != nil,
		"steps": domain.OnboardingSteps(domain.OnboardingMode(ob.Mode)),
	})
}

// --- Identificador y organización -------------------------------------------

// handleOnboardingSlug — GET /api/v1/onboarding/slug?slug=acme — disponibilidad
// REAL del identificador: formato, lista de reservados de la plataforma y
// consulta a la BD. Responde SIEMPRE 200 con el motivo, porque es una ayuda de
// escritura en vivo, no una operación que falle.
func (s *Server) handleOnboardingSlug(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("slug")))
	res := map[string]any{"slug": slug, "available": false}
	switch {
	case slug == "":
		res["reason"] = "Escribe un identificador para tu dirección."
	case len(slug) < 3:
		res["reason"] = "Necesita al menos 3 caracteres."
	case !tenancy.ValidSlug(slug):
		res["reason"] = "Solo minúsculas, números y guiones (sin espacios ni acentos)."
	case !domain.SlugAllowedForOrg(slug):
		res["reason"] = "Ese nombre lo usa Sytue para sus propios servicios. Elige otro."
	default:
		taken, err := s.store.SlugTaken(r.Context(), slug)
		if err != nil {
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
		if taken {
			res["reason"] = "Ya está en uso. Prueba con otro."
		} else {
			res["available"] = true
			res["subdomain"] = slug + "." + suiteDomain(s.cfg.Issuer)
		}
	}
	httpx.JSON(w, r, http.StatusOK, res)
}

type createOnboardingOrgRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// handleOnboardingCreateOrg — POST /api/v1/onboarding/organization — crea la
// organización del alta (también en el recorrido individual: por debajo el modelo
// es el mismo, aunque al usuario solo se le hable de «tu cuenta») y deja al
// solicitante como OWNER. Es la ÚNICA vía por la que un usuario sin rol puede
// crear una org, y solo una: si su fila de onboarding ya tiene organización, este
// endpoint la RENOMBRA en vez de crear otra (el slug es inmutable en toda la suite).
func (s *Server) handleOnboardingCreateOrg(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req createOnboardingOrgRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	name := domain.SanitizeDisplayName(req.Name)
	if name == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("A name is required."))
		return
	}
	ctx := r.Context()
	cur, err := s.store.GetOnboarding(ctx, c.Subject)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	// Ya existe: renombrar (y confirmar la propiedad). Slug inmutable.
	if cur.OrgID != "" {
		org, oerr := s.onboardingOrg(ctx, c.Subject)
		if oerr != nil {
			httpx.Fail(w, r, oerr)
			return
		}
		updated, uerr := s.store.UpdateOrgSettings(ctx, nil, org.ID, name, org.Status)
		if uerr != nil {
			httpx.Fail(w, r, mapDomainErr(uerr))
			return
		}
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.settings_updated", org.Slug, r))
		httpx.JSON(w, r, http.StatusOK, s.orgWire(ctx, updated, r))
		return
	}

	slug := strings.ToLower(strings.TrimSpace(req.Slug))
	if !tenancy.ValidSlug(slug) || len(slug) < 3 {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid identifier (lowercase letters, digits and hyphens)."))
		return
	}
	if !domain.SlugAllowedForOrg(slug) {
		httpx.Fail(w, r, httpx.ErrConflict.WithMessage("That identifier is reserved. Choose another one."))
		return
	}
	var org domain.Organization
	err = s.store.InTx(ctx, func(q pg.Querier) error {
		created, e := s.store.CreateOrg(ctx, q, domain.Organization{Slug: slug, Name: name})
		if e != nil {
			return e
		}
		org = created
		if e := s.store.UpsertMembership(ctx, q, domain.Membership{
			OrgID: org.ID, UserID: c.Subject, Role: domain.RoleOwner,
		}); e != nil {
			return e
		}
		_, e = s.store.UpsertOnboarding(ctx, q, c.Subject, "", "", org.ID, false)
		return e
	})
	if err != nil {
		httpx.Fail(w, r, mapDomainErr(err))
		return
	}
	_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.created", org.Slug, r))
	httpx.JSON(w, r, http.StatusCreated, s.orgWire(ctx, org, r))
}

// orgWire serializa la organización del alta CON el estado real de su dirección
// en Internet (ver ensureTenantDNS): el asistente no dice «tu dirección funciona»
// porque el identificador esté guardado, sino porque el registro DNS está.
func (s *Server) orgWire(ctx context.Context, o domain.Organization, r *http.Request) map[string]any {
	dnsWire, published := s.ensureTenantDNS(ctx, o.Slug)
	subdomain := o.Slug + "." + suiteDomain(s.cfg.Issuer) // el mismo host que publica ensureTenantDNS
	if published {
		// Crear un registro en la zona de la suite tiene efecto REAL fuera de
		// Sytue: se audita con el nombre exacto que se publicó.
		_ = s.store.Audit(ctx, nil, auditEntry(
			actorFrom(ctx), o.ID, "org.subdomain.published", subdomain, r,
		))
	}
	return map[string]any{
		"id": o.ID, "slug": o.Slug, "name": o.Name,
		"subdomain":  subdomain,
		"created_at": o.CreatedAt.UTC().Format(time.RFC3339),
		"dns":        dnsWire,
	}
}

// --- Dirección en Sytue: publicación REAL del subdominio --------------------

// tenantDNSTimeout acota el ida y vuelta a Cloudflare. El asistente espera por
// él: más allá de esto se reporta el estado que haya y se sigue.
const tenantDNSTimeout = 8 * time.Second

// ensureTenantDNS deja publicado <slug>.<dominio de la suite> en NUESTRA zona de
// Cloudflare y devuelve el estado REAL de ese registro:
//
//	live         → el registro está en la zona: la dirección resuelve de verdad
//	unconfigured → este despliegue no tiene Cloudflare configurado (ni token ni
//	               zona ni destino): la dirección queda reservada en Sytue pero
//	               NO se afirma que resuelva
//	failed       → Cloudflare respondió un error; `detail` dice cuál, en llano
//
// Es idempotente y se auto-repara: primero LEE si el registro está (barato) y
// solo lo crea si falta, así que sirve igual para publicarlo al crear la
// organización que para reflejar su estado al retomar el alta. `published` es
// true SOLO cuando esta llamada creó el registro (lo que hay que auditar).
//
// No devuelve error a propósito: publicar el subdominio nunca puede tumbar el
// alta. Un fallo se cuenta tal cual —y se registra para el operador—, jamás se
// disfraza de éxito.
//
// ponytail: el estado NO se persiste (una fila más que se queda vieja en cuanto
// alguien toca la zona a mano); se relee de Cloudflare, que es la única fuente
// que no miente. Techo: una llamada a Cloudflare por cada lectura del alta —un
// puñado por organización nueva, despreciable frente al límite de 1200/5 min de
// la API—. Si algún día ese GET se vuelve caliente, cachear el «live» por org
// (la publicación no se deshace sola) antes que persistirlo.
func (s *Server) ensureTenantDNS(ctx context.Context, slug string) (wire map[string]any, published bool) {
	host := slug + "." + suiteDomain(s.cfg.Issuer)
	out := map[string]any{"host": host, "status": "unconfigured"}
	// El nil explícito es por los tests que construyen el Server a mano: sin
	// publicador no se publica, que es el mismo camino que sin zona configurada.
	if s.zoneDNS == nil || !s.zoneDNS.HasZone() || s.cfg.TenantDNSTarget == "" {
		return out, false
	}
	cfCtx, cancel := context.WithTimeout(ctx, tenantDNSTimeout)
	defer cancel()

	_, found, err := s.zoneDNS.FindRecord(cfCtx, host)
	if err != nil {
		return s.tenantDNSFailed(ctx, out, host, "consultar", err), false
	}
	if found {
		out["status"] = "live"
		return out, false
	}
	// Falta: se crea. Pero NO siempre con la misma forma. Un CNAME no admite
	// ningún otro registro en el mismo nombre (RFC 1034 §3.6.2, RFC 2181 §10.1),
	// así que en una organización con buzones en `@<slug>.sytue.eu` —que tiene ahí
	// su MX y su SPF— el CNAME lo rechaza el proveedor y la dirección se queda sin
	// publicar. Esa se publica con A/AAAA, que sí conviven con el correo.
	conCorreo, err := s.zoneDNS.HasMail(cfCtx, host)
	if err != nil {
		return s.tenantDNSFailed(ctx, out, host, "consultar", err), false
	}
	// EL COMODÍN CUBRE EL NOMBRE ⇒ NO SE CREA NADA. Una zona con `*.<suite>` ya
	// sirve `<slug>.<suite>`, igual que sirve `mail.` o `drive.`. Crear un
	// registro propio ahí no añade nada —apunta al mismo sitio, TenantDNSTarget
	// es el host de la suite— y sí quita: en cuanto el nombre EXISTE, el comodín
	// deja de aplicarse a él.
	//
	// PERO SOLO SI EL NOMBRE NO TIENE YA REGISTROS, y eso es toda la sutileza.
	// La regla del comodín no es «si no hay dirección», es «si no hay NADA»
	// (RFC 4592 §2.2.1): un solo TXT en el nombre lo hace existir y desactiva el
	// comodín para él, sin publicar ninguna dirección. La respuesta pasa a ser
	// NOERROR con cero respuestas —NODATA—, que no es «no existe» sino «existe y
	// no tiene esto», así que no se parece a un nombre sin publicar.
	//
	// Es literalmente lo de `corporate.sytue.eu`: UN registro, su SPF
	// —`v=spf1 mx a:mx.sytue.eu include:spf.brevo.com -all`, que autoriza al
	// relé y no se puede quitar— y el nombre entero sin resolver mientras
	// cualquier subdominio inventado sí lo hacía.
	//
	// `conCorreo` es exactamente esa pregunta: MX o TXT en el nombre.
	if !conCorreo {
		if _, hayComodin, werr := s.zoneDNS.FindRecord(cfCtx, "*."+suiteDomain(s.cfg.Issuer)); werr == nil && hayComodin {
			out["status"] = "live"
			out["via"] = "wildcard"
			return out, false
		}
	}
	recs := s.tenantDNSRecords(host, slug, conCorreo)
	if len(recs) == 0 {
		// Hay correo y el despliegue no dice con qué IP publicar: no existe forma
		// legal de hacerlo. Se dice, no se intenta un CNAME condenado.
		s.log.ErrorContext(ctx, "el nombre tiene correo y no hay TENANT_DNS_ADDRESS para publicar su dirección", "host", host)
		out["status"] = "failed"
		out["detail"] = "No hemos podido publicar tu dirección en este momento. Ya lo estamos mirando."
		return out, false
	}

	// Varias direcciones (A y AAAA) son un solo resultado: basta con que una
	// exista para que el nombre resuelva, y un duplicado no es un fallo.
	creado := false
	for _, rec := range recs {
		switch _, cerr := s.zoneDNS.CreateRecord(cfCtx, rec); {
		case cerr == nil:
			creado = true
		case dns.Exists(cerr):
			// Otra réplica (o un intento anterior) lo creó entre la lectura y ahora.
		default:
			return s.tenantDNSFailed(ctx, out, host, "publicar", cerr), false
		}
	}
	out["status"] = "live"
	return out, creado
}

// tenantDNSRecords decide CON QUÉ registros se publica la dirección de una
// organización. Sin correo en el nombre, el CNAME de siempre. Con correo, las
// A/AAAA de TenantDNSAddress, porque el CNAME sería ilegal.
//
// Devuelve vacío SOLO en el caso sin salida: hay correo y no hay direcciones
// configuradas. El llamante lo reporta como fallo con motivo.
//
// TTL 1 = «automático» en Cloudflare (el publicador de Scaleway lo traduce).
func (s *Server) tenantDNSRecords(host, slug string, conCorreo bool) []dns.Record {
	comentario := "Sytue — organización " + slug
	// EL CORREO NO ESTORBA SI EL PROVEEDOR APLANA. Con el proxy encendido,
	// Cloudflare no publica un CNAME sino las direcciones de su edge, así que la
	// regla que lo prohíbe junto al MX no llega a aplicarse: en el cable no hay
	// ningún CNAME. Comprobado contra su API el 2026-09-07 con un nombre de usar
	// y tirar —MX creado y después CNAME proxied en el MISMO nombre, aceptado—.
	//
	// Importa más de lo que parece. La alternativa era publicar A/AAAA con la
	// dirección del origen, y en un despliegue doméstico esa IP es dinámica: el
	// DDNS solo persigue los nombres de DDNS_HOSTS, así que el subdominio de la
	// organización se quedaría apuntando a la IP anterior en el siguiente cambio
	// de ISP —sin un error, simplemente dejando de resolver un día cualquiera— y
	// además esquivaría el túnel, obligando a abrir 80/443 en el router.
	//
	// Las dos condiciones son necesarias: sin proxy un CNAME es un CNAME.
	aplana := s.cfg.TenantDNSProxied && s.zoneDNS != nil && s.zoneDNS.AplanaCNAMEProxied()
	if !conCorreo || aplana {
		return []dns.Record{{
			Type: "CNAME", Name: host, Content: s.cfg.TenantDNSTarget,
			TTL: 1, Proxied: s.cfg.TenantDNSProxied, Comment: comentario,
		}}
	}
	var recs []dns.Record
	for _, campo := range strings.Split(s.cfg.TenantDNSAddress, ",") {
		ip := net.ParseIP(strings.TrimSpace(campo))
		if ip == nil {
			continue
		}
		tipo := "AAAA"
		if ip.To4() != nil {
			tipo = "A"
		}
		recs = append(recs, dns.Record{
			Type: tipo, Name: host, Content: ip.String(),
			TTL: 1, Proxied: s.cfg.TenantDNSProxied, Comment: comentario,
		})
	}
	return recs
}

// tenantDNSFailed rellena el estado de fallo con un motivo en llano y lo registra
// para el operador. El token JAMÁS entra aquí: solo el error de Cloudflare.
func (s *Server) tenantDNSFailed(ctx context.Context, out map[string]any, host, what string, err error) map[string]any {
	s.log.ErrorContext(ctx, what+" el registro DNS del subdominio falló", "error", err, "host", host)
	out["status"] = "failed"
	switch {
	case dns.Denied(err):
		out["detail"] = "Sytue no tiene ahora mismo permiso para publicar esta dirección. Ya lo estamos mirando."
	case dns.Missing(err):
		out["detail"] = "La zona de Sytue donde va tu dirección no responde como esperábamos. Ya lo estamos mirando."
	default:
		out["detail"] = "No hemos podido publicar tu dirección en este momento. Vuelve a entrar en un rato."
	}
	return out
}

// actorFrom saca el usuario del contexto para auditar desde helpers que no
// reciben las claims. Vacío si no hay (no debería: estos endpoints van autenticados).
func actorFrom(ctx context.Context) string {
	c, _ := claimsFrom(ctx)
	return c.Subject
}

// --- Dominio propio: verificación DNS REAL ----------------------------------

type onboardingDomainRequest struct {
	Domain string `json:"domain"`
}

// handleOnboardingDomain — POST /api/v1/onboarding/domain — reclama un dominio
// propio y lo COMPRUEBA de verdad: guarda (o conserva) el token del registro TXT
// y consulta el DNS público con el resolver del sistema. La respuesta lleva el
// registro exacto a crear y el estado REAL:
//
//	verified    → el TXT existe y coincide (se sella en BD)
//	pending     → todavía no se ve el registro (DNS tarda en propagar)
//	mismatch    → hay TXT en ese nombre pero no es el nuestro (se devuelve el que hay)
//	unreachable → no hemos podido consultar el DNS (no es culpa del cliente)
//
// Pulsar el botón NO verifica nada por sí solo: si el registro no está, el estado
// no cambia. Mismo endpoint para «guardar dominio» y para «comprobar ahora».
func (s *Server) handleOnboardingDomain(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, oerr := s.onboardingOrg(r.Context(), c.Subject)
	if oerr != nil {
		httpx.Fail(w, r, oerr)
		return
	}
	s.reclamarDominio(w, r, org)
}

// reclamarDominio es el cuerpo compartido por las dos puertas desde las que se
// conecta un dominio: el asistente de alta y Sytue Admin. Está extraído —y no
// duplicado— porque las dos tienen que comportarse EXACTAMENTE igual: mismo token,
// misma comprobación DNS, mismo sellado y la misma mudanza de los buzones. Dos
// copias de esto habrían divergido en la primera corrección que se hiciera a una.
//
// Lo único que cambia entre las dos es quién decide sobre qué organización se
// opera, y eso lo resuelve cada llamante antes de entrar aquí.
func (s *Server) reclamarDominio(w http.ResponseWriter, r *http.Request, org domain.Organization) {
	c, _ := claimsFrom(r.Context())
	var req onboardingDomainRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	host, nerr := domain.NormalizeDomain(req.Domain)
	if nerr != nil {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"That does not look like a domain. Write it as acme.com, without 'https://' or '@'.",
		))
		return
	}
	token, terr := domain.NewDomainToken()
	if terr != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(terr))
		return
	}
	prof, serr := s.store.SetOrgDomain(ctx, nil, org.ID, host, token)
	if serr != nil {
		httpx.Fail(w, r, mapDomainErr(serr))
		return
	}
	// Consulta DNS real, acotada: el asistente espera por ella.
	dnsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	chk := domain.CheckDomainTXT(dnsCtx, s.resolver, prof.Domain, prof.DomainToken)
	if chk.Status == domain.DomainVerified && prof.DomainVerifiedAt == nil {
		if merr := s.store.MarkOrgDomainVerified(ctx, nil, org.ID, prof.Domain); merr != nil {
			// Índice único de dominios verificados: otra organización ya demostró
			// ser dueña de este dominio (reclamarlo sin verificar no bloquea a nadie).
			if errors.Is(merr, domain.ErrConflict) {
				httpx.Fail(w, r, httpx.ErrConflict.WithMessage(
					"Another Sytue organisation has already verified this domain. If it is yours, get in touch.",
				))
				return
			}
			httpx.Fail(w, r, mapDomainErr(merr))
			return
		}
		now := time.Now()
		prof.DomainVerifiedAt = &now
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.domain.verified", prof.Domain, r))
		s.mudarAlDominioPropio(r, org.ID, prof.Domain)
	}
	httpx.JSON(w, r, http.StatusOK, domainWire(prof, chk))
}

// mudarAlDominioPropio estrena el dominio recién verificado para toda la
// organización: los que ya estaban dentro pasan a tener su dirección en él y
// conservan la anterior como alias.
//
// Sin esto, el dominio verificado solo lo heredaban los buzones NUEVOS: un
// cliente conectaba el suyo, la pantalla decía «verificado» y su plantilla
// entera seguía escribiendo desde el subdominio de la suite. La compra no hacía
// nada visible y ninguna pantalla lo explicaba.
//
// UN FALLO AQUÍ NO INVALIDA LA VERIFICACIÓN. El dominio está verificado y sellado
// antes de llegar; devolver un error le diría al cliente que su dominio no ha
// quedado conectado cuando sí lo está, y le haría repetir una operación que ya
// salió bien. Se registra —para que quede la pista— y se sigue: la mudanza es
// idempotente, así que el siguiente intento la termina.
//
// Está extraído porque lo necesitan los dos caminos de verificación (el manual y
// el asistido por Cloudflare) y porque en línea disparaba el tope de complejidad
// del linter, que aquí es una señal correcta: son dos cosas distintas.
func (s *Server) mudarAlDominioPropio(r *http.Request, orgID, host string) {
	ctx := r.Context()
	movidos, err := s.store.PromoteOrgMailDomain(ctx, orgID)
	if err != nil {
		s.log.ErrorContext(ctx, "mudar los buzones al dominio propio falló",
			"error", err, "org_id", orgID, "domain", host)
		return
	}
	if movidos > 0 {
		c, _ := claimsFrom(ctx)
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, orgID, "org.domain.mailboxes_moved", host, r))
	}
}

// --- ¿Todavía sin dominio? Disponibilidad y precio REALES -------------------

// handleOnboardingDomainAvailability — GET /api/v1/onboarding/domain-availability?domain=acme.com
// pregunta al registrador de Cloudflare si ese dominio se puede registrar y a qué
// precio. Es SOLO una consulta: no reserva nada y no mueve un céntimo.
//
// Sytue NO vende dominios, y la respuesta lo dice explícitamente
// (`purchasable_here: false`) para que el frontend no tenga que adivinarlo. La
// API de Cloudflare sí permite registrarlos hoy (POST /accounts/{id}/registrar/
// registrations, beta), pero cobra al medio de pago de la CUENTA DE PLATAFORMA de
// Sytue y deja el dominio en ella; sin una vía para cobrárselo al cliente eso no
// es una función, es un agujero contable. Así que enseñamos el precio de verdad y
// decimos en llano dónde comprarlo y cómo volver a conectarlo aquí.
//
// Responde SIEMPRE 200 con su estado, como la disponibilidad del identificador:
// es una ayuda de escritura, no una operación que falle.
//
//	available    → se puede registrar; `price` trae el precio REAL de Cloudflare
//	taken        → ya está registrado por alguien
//	unsupported  → Cloudflare no cotiza esa extensión por API (no dice que esté libre)
//	unknown      → no hemos podido consultar (fallo o permiso); no se afirma nada
//	unconfigured → este despliegue no tiene consulta de dominios configurada
//	invalid      → lo escrito no es un dominio
func (s *Server) handleOnboardingDomainAvailability(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("domain"))
	res := map[string]any{"domain": raw, "purchasable_here": false}
	reply := func(status string) {
		res["status"] = status
		httpx.JSON(w, r, http.StatusOK, res)
	}
	host, nerr := domain.NormalizeDomain(raw)
	if nerr != nil {
		reply("invalid")
		return
	}
	res["domain"] = host
	if !s.cf.HasAccount() {
		reply("unconfigured")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), tenantDNSTimeout)
	defer cancel()
	av, err := s.cf.CheckDomain(ctx, host)
	if err != nil {
		s.log.ErrorContext(ctx, "consultar disponibilidad del dominio falló", "error", err, "domain", host)
		reply("unknown")
		return
	}
	if av.Registrable {
		if av.Pricing != nil {
			res["price"] = map[string]any{
				"currency":     av.Pricing.Currency,
				"registration": av.Pricing.RegistrationCost,
				"renewal":      av.Pricing.RenewalCost,
			}
		}
		reply("available")
		return
	}
	// `reason` es la clave cruda de Cloudflare: se devuelve tal cual para poder
	// diagnosticar, pero el estado que gobierna la pantalla es el nuestro.
	res["reason"] = av.Reason
	if av.Reason == "domain_unavailable" {
		reply("taken")
		return
	}
	reply("unsupported")
}

// --- Primeras personas ------------------------------------------------------

type onboardingInviteRequest struct {
	Emails string `json:"emails"`
	Role   string `json:"role"`
}

// handleOnboardingInvite — POST /api/v1/onboarding/invitations — invita a varias
// personas de golpe (lista separada por comas/espacios). Cada invitación es la
// MISMA que la de la Console: fila real con token firmado de un solo uso y correo
// enviado por el mismo camino. Se responde el resultado REAL de cada dirección
// (enviada / ya es miembro / no es un correo / no se pudo enviar): nunca se
// finge éxito, y si el correo no está configurado se ve aquí.
func (s *Server) handleOnboardingInvite(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req onboardingInviteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	org, oerr := s.onboardingOrg(ctx, c.Subject)
	if oerr != nil {
		httpx.Fail(w, r, oerr)
		return
	}
	role := domain.Role(strings.TrimSpace(req.Role))
	if role == "" {
		role = domain.RoleMember
	}
	// El alta no reparte owners: el owner es quien está dando de alta la org.
	if !role.Valid() || role == domain.RoleOwner {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Invalid role (admin|member|billing)."))
		return
	}
	emails, bad := domain.ParseInviteEmails(req.Emails)
	if len(emails) > maxBatchInvites {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"Too many addresses at once (25 maximum). Invite the rest afterwards from Sytue Admin.",
		))
		return
	}
	results := make([]map[string]any, 0, len(emails)+len(bad))
	for _, e := range bad {
		results = append(results, map[string]any{
			"email": e, "status": "invalid", "detail": "No parece una dirección de correo.",
		})
	}
	sent := 0
	for _, email := range emails {
		// El alta inicial invita en lote y no personaliza direcciones: con una lista
		// de correos pegada de golpe no hay dónde elegir una por persona, y todas
		// reciben el patrón por defecto.
		exp, err := s.inviteToOrg(ctx, org, email, role, c.Subject, "", "")
		switch {
		case err == nil:
			sent++
			results = append(results, map[string]any{
				"email": email, "status": "sent", "role": string(role),
				"expires_at": exp.UTC().Format(time.RFC3339),
			})
			_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.member.invited", email, r))
		case errors.Is(err, domain.ErrConflict):
			results = append(results, map[string]any{
				"email": email, "status": "already_member",
				"detail": "Ya forma parte de tu organización.",
			})
		case errors.Is(err, errMailNotSent):
			s.log.ErrorContext(ctx, "enviar invitación del alta falló", "error", err, "org", org.Slug)
			results = append(results, map[string]any{
				"email": email, "status": "not_sent",
				"detail": "No hemos podido enviar el correo. La invitación está creada: vuelve a intentarlo desde Sytue Admin.",
			})
		default:
			s.log.ErrorContext(ctx, "invitación del alta falló", "error", err, "org", org.Slug)
			results = append(results, map[string]any{
				"email": email, "status": "failed", "detail": "No se ha podido crear la invitación.",
			})
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"sent": sent, "results": results})
}

// --- Roles recomendados -----------------------------------------------------

type onboardingRolesRequest struct {
	Keys []string `json:"keys"`
}

// handleOnboardingRoles — POST /api/v1/onboarding/roles — crea los roles
// recomendados elegidos como roles personalizados REALES de la org (mig 0019),
// con los permisos del catálogo fijo. Aparecen luego en Sytue Admin → Roles como
// cualquier otro: se pueden editar, asignar y borrar. Un rol cuyo nombre ya
// existe se reporta como «exists» (no se pisa lo que el cliente ya tenga).
func (s *Server) handleOnboardingRoles(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	var req onboardingRolesRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.Fail(w, r, err)
		return
	}
	ctx := r.Context()
	org, oerr := s.onboardingOrg(ctx, c.Subject)
	if oerr != nil {
		httpx.Fail(w, r, oerr)
		return
	}
	results := make([]map[string]any, 0, len(req.Keys))
	for _, key := range req.Keys {
		rec, ok := domain.RecommendedRoleByKey(strings.TrimSpace(key))
		if !ok {
			httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage("Unknown recommended role: "+key))
			return
		}
		// Los permisos pasan por la MISMA validación que un rol creado a mano: si
		// alguno dejara de ser concedible, esto falla en vez de crear un rol falso.
		perms, valid := domain.NormalizePermissions(rec.PermissionStrings())
		if !valid {
			httpx.Fail(w, r, httpx.ErrInternal.WithMessage("Inconsistent permission catalogue."))
			return
		}
		role, err := s.store.CreateCustomRole(ctx, nil, org.ID, rec.Name, perms)
		switch {
		case err == nil:
			_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.role.created", role.ID, r))
			results = append(results, map[string]any{
				"key": rec.Key, "status": "created", "id": role.ID,
				"name": role.Name, "permissions": role.Permissions,
			})
		case errors.Is(err, domain.ErrConflict):
			results = append(results, map[string]any{
				"key": rec.Key, "status": "exists", "name": rec.Name,
			})
		default:
			httpx.Fail(w, r, mapDomainErr(err))
			return
		}
	}
	httpx.JSON(w, r, http.StatusOK, map[string]any{"results": results})
}

// --- Anclaje de autorización ------------------------------------------------

// onboardingOrg resuelve la organización del alta del solicitante y comprueba
// contra la BD que sigue siendo su OWNER. Fail-closed: sin fila, sin org o sin
// membresía de owner, no se autoriza (403). Es el sustituto de activeOrg para
// este flujo, donde el token puede no tener todavía claim `tenant`.
func (s *Server) onboardingOrg(ctx context.Context, userID string) (domain.Organization, error) {
	ob, err := s.store.GetOnboarding(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Organization{}, httpx.ErrBadRequest.WithMessage(
				"Create your organisation in the previous step first.",
			)
		}
		return domain.Organization{}, mapDomainErr(err)
	}
	if ob.OrgID == "" {
		return domain.Organization{}, httpx.ErrBadRequest.WithMessage(
			"Create your organisation in the previous step first.",
		)
	}
	org, err := s.store.GetOrgByID(ctx, ob.OrgID)
	if err != nil {
		return domain.Organization{}, mapDomainErr(err)
	}
	roles, rerr := s.store.RolesForUserInOrg(ctx, org.ID, userID)
	if rerr != nil || !hasAnyRole(roles, domain.RoleOwner) {
		return domain.Organization{}, httpx.ErrForbidden.WithMessage(
			"You are not an owner of this organisation.",
		)
	}
	return org, nil
}

// --- Invitación reutilizable ------------------------------------------------

// errMailNotSent distingue «la invitación existe pero el correo no salió» de un
// fallo de creación: el primero es recuperable (reenviar), el segundo no.
var errMailNotSent = errors.New("id: invitación creada pero correo no enviado")

// inviteToOrg crea una invitación REAL (token firmado de un solo uso, hash en BD)
// y envía el enlace por el mismo camino que las invitaciones de la Console. Es el
// núcleo compartido por handleAdminInviteUser, por el alta inicial y por «crear
// propietario» de la consola de plataforma: una sola implementación del alta por
// invitación.
//
// Devuelve domain.ErrConflict si el correo ya es miembro de la org, y
// errMailNotSent si la fila quedó creada pero el envío falló.
//
// `mailboxLocal` es la dirección de correo elegida para el invitado (vacía ⇒ el
// patrón nombre.apellidos). Se guarda con la invitación y se aplica al aceptarla,
// que es cuando existe el usuario al que asignársela. `name`, igual: es el nombre
// de la cuenta si se crea al aceptar (vacío ⇒ la parte local del correo).
func (s *Server) inviteToOrg(ctx context.Context, org domain.Organization, email string, role domain.Role, inviterID, mailboxLocal, name string) (time.Time, error) {
	// ¿Ya es miembro de ESTA org? Para cambiarle el rol está la acción de rol, no
	// una invitación nueva. Un usuario existente que NO es miembro sí se invita.
	if u, uerr := s.store.GetUserByEmail(ctx, email); uerr == nil {
		if _, merr := s.store.GetMembership(ctx, org.ID, u.ID); merr == nil {
			return time.Time{}, domain.ErrConflict
		}
	}
	token, hash, err := domain.NewResetToken(s.resetKey)
	if err != nil {
		return time.Time{}, err
	}
	exp := time.Now().Add(s.cfg.InviteTokenTTL)
	if err := s.store.InTx(ctx, func(q pg.Querier) error {
		return s.store.InsertInvitation(ctx, q, hash, org.ID, email, string(role), inviterID, mailboxLocal, name, exp)
	}); err != nil {
		return time.Time{}, err
	}
	link := strings.TrimRight(s.cfg.Issuer, "/") + "/accept-invitation?token=" + url.QueryEscape(token)
	if err := s.mailer.SendInvitation(ctx, email, org.Name, link); err != nil {
		// La fila queda (caducará; reinvitar la purga); el llamante ve el fallo real.
		return exp, errors.Join(errMailNotSent, err)
	}
	return exp, nil
}
