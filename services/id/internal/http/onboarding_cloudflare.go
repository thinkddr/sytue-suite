// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// «Mi dominio está en Cloudflare — configúralo por mí»: la alternativa al paso de
// dominio para quien no quiere ir a buscar la pantalla de DNS de su proveedor.
//
// El cliente pega un token de SU cuenta de Cloudflare con permiso «Zone · DNS ·
// Edit» y Sytue le crea los registros. Todo lo que ocurre aquí es REAL: se busca
// su zona de verdad, se lee lo que ya tiene puesto, se crea lo que falta y se
// vuelve a LEER la zona para contar el resultado. Nada se da por hecho porque
// Cloudflare haya respondido 200.
//
// TRES REGLAS QUE NO SE NEGOCIAN
//
//  1. El token es un secreto DE UN TERCERO. Va en un cliente aparte (ForCustomer),
//     nunca se mezcla con el de plataforma, nunca vuelve al navegador, nunca entra
//     en un log ni en un mensaje de error, y por DEFECTO NO SE GUARDA: se usa y se
//     descarta en la misma petición. Solo si el cliente lo pide expresamente
//     (remember_token) se guarda SELLADO con la KEK (mig 0028), y desmarcarlo lo
//     borra de verdad.
//
//  2. No se pisa nada ajeno. Antes de tocar la zona se PREVISUALIZA: qué ya está
//     bien, qué falta y qué choca con algo que el cliente puso. Un choque solo se
//     sustituye si él manda su clave en `replace`, viendo antes el valor actual.
//     Es la diferencia entre configurar un dominio y romper el correo de una
//     empresa.
//
//  3. No se inventa ningún registro. Solo entra en el plan lo que la suite sabe
//     de verdad. Lo que hoy no podemos generar (DKIM/DMARC: la clave pública vive
//     en services/mail y no la publica por API) se enseña como PENDIENTE y se
//     explica, en vez de escribir un valor de mentira en la zona del cliente.
//
// Lo que NO está aquí, y por qué: los CNAME de autodescubrimiento del correo
// (autoconfig./autodiscover.<dominio del cliente>). El registro se podría crear en
// un segundo, pero no funcionaría: nuestro ingress solo enruta autoconfig. y
// autodiscover. del dominio de la suite (deploy/helm/charts/mail: extraSubdomains
// + el certificado de mail.<suite>), así que el cliente que los siguiera se
// llevaría un 404 con un certificado que no es el suyo. Peor que no tenerlos:
// Thunderbird y Outlook, sin esos nombres, caen a su búsqueda normal y funcionan.
// Cuando el ingress sirva hosts de cliente, este es el sitio donde se añaden.
package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/httpx"
	"git.sytue.eu/sytue/one/platform/pkg/logsafe"
	"git.sytue.eu/sytue/one/services/id/internal/dns"
	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// cloudflareTimeout acota TODA la operación contra la cuenta del cliente (buscar
// la zona, leerla, crear y volver a leer). El asistente espera por ella.
const cloudflareTimeout = 25 * time.Second

// Estado de cada registro del plan. Los tres primeros salen de LEER la zona; los
// tres siguientes, de haberla tocado; el último es lo que Sytue todavía no puede
// generar y por eso no escribe.
const (
	stateOK       = "ok"       // ya está puesto y es exactamente el nuestro
	stateMissing  = "missing"  // no está: hay que crearlo
	stateConflict = "conflict" // hay otra cosa ocupando ese hueco (no se toca sin permiso)
	stateCreated  = "created"  // lo hemos creado en esta pasada
	stateReplaced = "replaced" // hemos sustituido lo que había, con permiso explícito
	stateFailed   = "failed"   // se intentó y no se pudo (el motivo va en `detail`)
	statePending  = "pending"  // Sytue aún no puede generarlo; se explica por qué
)

// plannedRecord es UN registro que Sytue necesita en el dominio del cliente.
type plannedRecord struct {
	// Type/Name/Content/Priority son el registro tal cual iría a la zona.
	Type     string
	Name     string
	Content  string
	Priority int
	// Match acota QUÉ registro existente ocupa este hueco, por prefijo del valor
	// (en minúsculas). Vacío = cualquiera del mismo tipo y nombre. Sirve para no
	// confundir un TXT ajeno del vértice (una verificación de Google, pongamos)
	// con el SPF que sí competiría con el nuestro.
	Match string
	// Purpose es la explicación en llano que lee el cliente.
	Purpose string
	// Detail acompaña a los pendientes: por qué todavía no se puede.
	Detail string
	// Pending marca lo que no se escribe (DKIM/DMARC): entra en la lista para que
	// se vea que falta, pero jamás se crea con un valor inventado.
	Pending bool
}

// key identifica el registro de forma estable para que el cliente pueda confirmar
// su sustitución (`replace`). Tipo+nombre: es lo que define el hueco en la zona.
func (p plannedRecord) key() string {
	return strings.ToLower(p.Type + ":" + p.Name)
}

// cloudflarePlan enumera lo que hay que poner en el dominio del cliente. Sale
// entero de lo que la suite YA sabe: el registro de verificación lo define
// domain/onboarding.go (el mismo del camino manual) y el MX, la configuración del
// servicio (MailMXHost, mx.<suite> por convención de services/mail).
func (s *Server) cloudflarePlan(prof store.OrgProfile, wantMail bool) []plannedRecord {
	plan := make([]plannedRecord, 0, 3) // TXT de verificación + (MX y SPF si quiere correo).
	plan = append(plan, plannedRecord{
		Type:    "TXT",
		Name:    domain.RecordName(prof.Domain),
		Content: domain.RecordValue(prof.DomainToken),
		Purpose: "Demuestra que el dominio es tuyo. Es el imprescindible.",
	})
	if !wantMail || s.cfg.MailMXHost == "" {
		return plan
	}
	return append(plan,
		plannedRecord{
			Type: "MX", Name: prof.Domain, Content: s.cfg.MailMXHost, Priority: 10,
			Purpose: "Hace que el correo de @" + prof.Domain + " llegue a Sytue Mail.",
		},
		plannedRecord{
			// ponytail: el SPF se sustituye entero, no se fusiona con el que haya
			// (fusionar SPF es un parser y un modo nuevo de romperle el correo a
			// alguien). Por eso un SPF existente sale como conflicto CON su valor
			// a la vista: quien decide qué combinar es la persona, no nosotros.
			// `~all` y no `-all`: durante una migración el dominio casi siempre
			// sigue enviando desde otro sitio, y un hardfail lo tumbaría.
			Type: "TXT", Name: prof.Domain, Content: "v=spf1 mx ~all", Match: "v=spf1",
			Purpose: "Autoriza a Sytue Mail a enviar en nombre de tu dominio.",
		},
		plannedRecord{
			Type: "TXT", Name: "sytue._domainkey." + prof.Domain, Pending: true,
			Purpose: "Firma DKIM y política DMARC de tu correo.",
			Detail: "La clave pública la genera Sytue Mail y todavía no la publica " +
				"por API, así que no podemos escribir este registro por ti sin " +
				"inventárnoslo. Tu correo funciona sin él; te avisaremos para " +
				"añadirlo en cuanto esté.",
		},
	)
}

// --- Comparación con lo que ya hay en la zona -------------------------------

// dnsValue normaliza un valor de registro para compararlo: Cloudflare devuelve
// los TXT entrecomillados y los destinos con punto final según el caso.
func dnsValue(v string) string {
	return strings.TrimSuffix(strings.Trim(strings.TrimSpace(v), `"`), ".")
}

// sameRecord indica si el registro que ya está en la zona ES el que queremos
// (entonces no hay nada que hacer: la operación es idempotente).
func sameRecord(p plannedRecord, e dns.Record) bool {
	if !strings.EqualFold(dnsValue(e.Content), dnsValue(p.Content)) {
		return false
	}
	return p.Type != "MX" || p.Priority == e.Priority
}

// occupies indica si un registro existente compite por el mismo hueco (y por
// tanto es un conflicto, no un vecino).
func occupies(p plannedRecord, e dns.Record) bool {
	return p.Match == "" || strings.HasPrefix(strings.ToLower(dnsValue(e.Content)), p.Match)
}

// classify LEE la zona y dice en qué estado está ese registro. No escribe nada.
func classify(ctx context.Context, cf *dns.Client, p plannedRecord) (state, current, id string, err error) {
	existing, err := cf.ListRecords(ctx, p.Name, p.Type)
	if err != nil {
		return "", "", "", err
	}
	for _, e := range existing {
		if sameRecord(p, e) {
			return stateOK, "", e.ID, nil
		}
	}
	for _, e := range existing {
		if occupies(p, e) {
			return stateConflict, dnsValue(e.Content), e.ID, nil
		}
	}
	return stateMissing, "", "", nil
}

// --- Endpoint ---------------------------------------------------------------

type cloudflareRequest struct {
	Domain string `json:"domain"`
	// Token es el de la cuenta del CLIENTE. Vacío ⇒ se usa el que él mismo pidió
	// recordar, si lo hay.
	Token string `json:"token"`
	// Mail añade al plan los registros de Sytue Mail (MX + SPF). Va aparte porque
	// redirigir el correo de un dominio es una decisión con consecuencias: no se
	// hace por venir en el mismo botón.
	Mail bool `json:"mail"`
	// Apply false ⇒ solo previsualizar (únicamente lecturas contra Cloudflare).
	Apply bool `json:"apply"`
	// Replace son las claves (tipo:nombre) que el cliente autoriza a SUSTITUIR.
	Replace []string `json:"replace"`
	// RememberToken guarda el token sellado para futuros cambios. Por defecto
	// false: se usa y se tira. Volver a enviarlo en false lo BORRA.
	RememberToken bool `json:"remember_token"`
}

type recordWire struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content,omitempty"`
	Priority int    `json:"priority,omitempty"`
	Purpose  string `json:"purpose"`
	State    string `json:"state"`
	// Current es lo que hay AHORA en ese hueco cuando estorba: el cliente decide
	// viendo el valor real, no una etiqueta.
	Current string `json:"current,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// handleOnboardingDomainCloudflare — POST /api/v1/onboarding/domain/cloudflare.
//
// Con `apply:false` solo LEE: localiza la zona del cliente y devuelve el plan con
// el estado de cada registro (ya está / falta / choca con esto). Con `apply:true`
// crea lo que falta, sustituye solo lo que él haya autorizado en `replace`, y
// vuelve a LEER la zona para contar lo que quedó de verdad.
//
// Si al releer el TXT de verificación está y coincide, el dominio queda verificado
// en el mismo paso: hemos visto el registro en la zona autoritativa, autenticados
// con las credenciales de su dueño. Es una prueba más fuerte que la consulta
// pública del camino manual, no un atajo.
//
// Un fallo parcial NO da la operación por buena: cada registro lleva su propio
// estado y el dominio solo se marca verificado si SU registro está.
func (s *Server) handleOnboardingDomainCloudflare(w http.ResponseWriter, r *http.Request) {
	c, _ := claimsFrom(r.Context())
	org, oerr := s.onboardingOrg(r.Context(), c.Subject)
	if oerr != nil {
		httpx.Fail(w, r, oerr)
		return
	}
	s.reclamarDominioCloudflare(w, r, org)
}

// reclamarDominioCloudflare es el cuerpo compartido por el asistente y por Sytue
// Admin, por lo mismo que reclamarDominio: las dos puertas tienen que crear los
// MISMOS registros y sellar la verificación igual. Quién decide sobre qué
// organización se opera lo resuelve el llamante.
func (s *Server) reclamarDominioCloudflare(w http.ResponseWriter, r *http.Request, org domain.Organization) {
	c, _ := claimsFrom(r.Context())
	var req cloudflareRequest
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
	// El dominio se reclama por la MISMA vía que el camino manual: misma fila,
	// mismo token TXT (que se conserva si el dominio no ha cambiado).
	newTok, terr := domain.NewDomainToken()
	if terr != nil {
		httpx.Fail(w, r, httpx.ErrInternal.Wrap(terr))
		return
	}
	prof, serr := s.store.SetOrgDomain(ctx, nil, org.ID, host, newTok)
	if serr != nil {
		httpx.Fail(w, r, mapDomainErr(serr))
		return
	}

	token := strings.TrimSpace(req.Token)
	if token == "" {
		// Sin token en la petición: el que el cliente pidió recordar, si lo hay.
		_, stored, gerr := s.store.GetOrgCloudflare(ctx, org.ID)
		if gerr != nil {
			s.log.ErrorContext(ctx, "leer las credenciales de Cloudflare del cliente falló", "error", gerr)
			httpx.Fail(w, r, mapDomainErr(gerr))
			return
		}
		token = stored
	}
	if token == "" {
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"We need a token from your Cloudflare account with permission to edit the DNS of "+host+".",
		))
		return
	}

	cfCtx, cancel := context.WithTimeout(ctx, cloudflareTimeout)
	defer cancel()

	// La zona se localiza SIEMPRE con el token, aunque la tuviéramos guardada:
	// es la comprobación de que ese token manda de verdad en ESTE dominio.
	cf := s.cf.ForCustomer(token, "")
	zone, found, zerr := cf.FindZone(cfCtx, host)
	switch {
	case dns.Denied(zerr):
		// El token no vale o le falta el permiso. Ni el token ni su longitud
		// entran en el log: solo lo que dijo Cloudflare.
		s.log.WarnContext(ctx, "el token de Cloudflare del cliente no autoriza", "domain", logsafe.Text(host))
		httpx.Fail(w, r, httpx.ErrForbidden.WithMessage(
			"That token does not let us see your DNS. When creating it in Cloudflare you have to tick "+
				"el permiso «Zone · DNS · Edit» e incluir la zona de "+host+".",
		))
		return
	case zerr != nil:
		s.log.ErrorContext(ctx, "buscar la zona del cliente en Cloudflare falló", "error", zerr, "domain", logsafe.Text(host))
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage(
			"We could not reach Cloudflare just now. Try again in a moment.",
		))
		return
	case !found:
		// El token SÍ vale: el problema es otro y se dice cuál.
		httpx.Fail(w, r, httpx.ErrBadRequest.WithMessage(
			"The token works, but that Cloudflare account does not list "+host+
				". Comprueba que el dominio esté en la misma cuenta y que el token incluya su zona.",
		))
		return
	}
	cf = cf.ForCustomer(token, zone.ID)

	plan := s.cloudflarePlan(prof, req.Mail)
	out, applied, cerr := s.applyCloudflarePlan(cfCtx, cf, plan, req, host)
	if cerr != nil {
		s.log.ErrorContext(ctx, "leer la zona del cliente en Cloudflare falló", "error", cerr, "domain", logsafe.Text(host))
		httpx.Fail(w, r, httpx.ErrUnavailable.WithMessage(
			"Hemos localizado tu zona pero no hemos podido leer sus registros. Vuelve a intentarlo.",
		))
		return
	}

	// Recordar (o borrar) las credenciales. Se hace SIEMPRE, también en la
	// previsualización: desmarcar la casilla tiene que olvidar de verdad.
	keep := ""
	if req.RememberToken {
		keep = token
	}
	if uerr := s.store.SetOrgCloudflare(ctx, nil, org.ID, zone.ID, keep); uerr != nil {
		s.log.ErrorContext(ctx, "guardar las credenciales de Cloudflare del cliente falló", "error", uerr)
	}

	// Verificación: el TXT está en la zona autoritativa y es el nuestro.
	chk := domain.DNSCheck{Status: domain.DomainPending}
	if verifiedByZone(out) {
		chk.Status = domain.DomainVerified
		if prof.DomainVerifiedAt == nil {
			if merr := s.store.MarkOrgDomainVerified(ctx, nil, org.ID, prof.Domain); merr != nil {
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
	}
	if applied > 0 {
		// Tocar la zona de un cliente tiene efecto REAL fuera de Sytue: se audita.
		_ = s.store.Audit(ctx, nil, auditEntry(c.Subject, org.ID, "org.domain.cloudflare_applied", prof.Domain, r))
	}

	httpx.JSON(w, r, http.StatusOK, map[string]any{
		// La zona: su identificador es público (no es el token) y sirve para que el
		// cliente confirme que hemos mirado la zona correcta.
		"zone":    map[string]any{"id": zone.ID, "name": zone.Name, "status": zone.Status},
		"records": out,
		"applied": applied,
		// Se dice explícitamente qué hemos hecho con su token.
		"token_stored": req.RememberToken,
		"domain":       domainWire(prof, chk),
	})
}

// applyCloudflarePlan lee la zona, actúa si toca y VUELVE A LEERLA para contar el
// resultado. Devuelve cuántos registros se tocaron de verdad.
//
// El orden importa: la verdad final sale siempre de la relectura, no de que
// Cloudflare respondiera 200 a la creación.
func (s *Server) applyCloudflarePlan(
	ctx context.Context, cf *dns.Client, plan []plannedRecord, req cloudflareRequest, host string,
) (out []recordWire, applied int, err error) {
	allowed := make(map[string]bool, len(req.Replace))
	for _, k := range req.Replace {
		allowed[strings.ToLower(strings.TrimSpace(k))] = true
	}
	out = make([]recordWire, 0, len(plan))
	for _, p := range plan {
		w := recordWire{
			Key: p.key(), Type: p.Type, Name: p.Name, Content: p.Content,
			Priority: p.Priority, Purpose: p.Purpose,
		}
		if p.Pending {
			// No se lee ni se escribe: no hay nada que Sytue pueda poner ahí.
			w.State, w.Detail, w.Content = statePending, p.Detail, ""
			out = append(out, w)
			continue
		}
		state, current, id, cerr := classify(ctx, cf, p)
		if cerr != nil {
			return nil, applied, cerr
		}
		w.State, w.Current = state, current

		if req.Apply && (state == stateMissing || (state == stateConflict && allowed[p.key()])) {
			rec := dns.Record{
				Type: p.Type, Name: p.Name, Content: p.Content, Priority: p.Priority,
				TTL: 1, Comment: "Sytue — " + host,
			}
			var aerr error
			if state == stateMissing {
				_, aerr = cf.CreateRecord(ctx, rec)
			} else {
				_, aerr = cf.UpdateRecord(ctx, id, rec)
			}
			switch {
			case aerr == nil, dns.Exists(aerr):
				applied++
			default:
				// Un fallo aquí NO tumba el resto: se cuenta en SU registro y los
				// demás siguen. La operación entera no se da por buena.
				s.log.ErrorContext(ctx, "escribir en la zona del cliente falló",
					"error", aerr, "domain", logsafe.Text(host), "record", p.key())
				w.State, w.Detail = stateFailed, cloudflareReason(aerr)
				out = append(out, w)
				continue
			}
			// Relectura: lo que cuenta es lo que hay en la zona, no la respuesta.
			after, _, _, rerr := classify(ctx, cf, p)
			switch {
			case rerr != nil:
				w.State, w.Detail = stateFailed, "Lo hemos enviado, pero no hemos podido releer la zona para confirmarlo."
			case after != stateOK:
				w.State, w.Detail = stateFailed, "Cloudflare aceptó el cambio pero el registro no aparece al releer la zona."
			case state == stateMissing:
				w.State, w.Current = stateCreated, ""
			default:
				w.State, w.Current = stateReplaced, ""
			}
		}
		out = append(out, w)
	}
	return out, applied, nil
}

// verifiedByZone indica si el TXT de verificación está PUESTO en la zona (leído,
// no supuesto). Es el único registro que decide si el dominio queda verificado.
func verifiedByZone(out []recordWire) bool {
	for _, w := range out {
		if w.Type != "TXT" || !strings.HasPrefix(w.Name, "_sytue.") {
			continue
		}
		return w.State == stateOK || w.State == stateCreated || w.State == stateReplaced
	}
	return false
}

// cloudflareReason traduce a llano por qué falló un registro concreto. Lleva el
// mensaje de Cloudflare porque suele decir exactamente qué pasa («record already
// exists», «content is invalid»); jamás lleva el token.
func cloudflareReason(err error) string {
	switch {
	case dns.Denied(err):
		return "El token no tiene permiso para escribir en esta zona."
	case dns.Missing(err):
		return "Cloudflare ya no encuentra ese registro (¿lo has cambiado a la vez desde su panel?)."
	default:
		return "Cloudflare lo ha rechazado: " + dns.Detail(err)
	}
}
