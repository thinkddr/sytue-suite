// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package migrate

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/connector"
	"git.sytue.eu/sytue/one/services/integrations/internal/domain"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/graph"
	"git.sytue.eu/sytue/one/services/integrations/internal/driver/gws"
)

// El asistente automático se apoya en esto: en vez de pedirle a un administrador
// que rellene de qué cuenta a qué cuenta y qué contenido, se le PREGUNTA AL ORIGEN
// con la credencial que acaba de autorizar. Lo que el token alcanza es lo que se
// migra; lo que no, se dice.

const (
	// maxDiscoveredUsers acota el descubrimiento del directorio de origen. Por
	// encima, el asistente lanza la migración en tandas (cada persona es un trabajo
	// independiente, así que partirlo no pierde nada).
	maxDiscoveredUsers = 2000
	// directoryPageSize es el tamaño de página del Directory API de Google.
	directoryPageSize = 200
	//nolint:gosec // G101: identificador de scope OAuth público, no una credencial.
	scopeDirectoryRead = "https://www.googleapis.com/auth/admin.directory.user.readonly"
	// scopeDirectoryGroupRead no lo usa el descubrimiento (que solo lista personas),
	// pero el driver del Directory pide AMBOS scopes en el mismo JWT: si este no
	// está delegado, el canje entero falla y con él las tablas de Sheets y Hub.
	//nolint:gosec // G101: identificador de scope OAuth público, no una credencial.
	scopeDirectoryGroupRead = "https://www.googleapis.com/auth/admin.directory.group.readonly"
	defaultDirectoryBase    = "https://admin.googleapis.com"
	// Raíz del servicio de autorización de Entra y scope de los permisos de
	// APLICACIÓN ya consentidos (los que se piden en el consentimiento de admin).
	defaultAuthorityBase = "https://login.microsoftonline.com"
	//nolint:gosec // G101: identificador de scope OAuth público, no una credencial.
	graphDefaultScope = "https://graph.microsoft.com/.default"
)

// ConfigConnectedAs es la clave de la configuración PÚBLICA de la instancia donde
// el asistente anota qué cuenta autorizó la conexión. No es un secreto (es un
// correo) y ahorra volver a preguntárselo al proveedor en cada plan.
const ConfigConnectedAs = "_oauth_connected_as"

// ScopeUser y ScopeOrganization son los dos alcances del asistente: la cuenta de
// quien se conecta, o todas las del origen.
const (
	ScopeUser         = "user"
	ScopeOrganization = "organization"
)

// PlanItem es un tipo de contenido encontrado en el origen y adónde va a parar en
// Sytue. Count es lo que el origen dice tener (-1 = no lo publica): sirve para que
// el asistente enseñe «42 repositorios» en vez de una promesa vaga.
type PlanItem struct {
	Domain      domain.MigrationDomain `json:"domain"`
	Destination string                 `json:"destination"` // drive | contacts | git
	Count       int                    `json:"count"`
}

// Discovery es lo que se sabe del origen DESPUÉS del login y ANTES de tocar nada.
type Discovery struct {
	// ConnectedAs es la cuenta que autorizó (correo o nombre de usuario).
	ConnectedAs   string `json:"connected_as"`
	ConnectedName string `json:"connected_name,omitempty"`
	// Scope es el alcance REAL de la credencial: solo esa cuenta, o la organización.
	Scope string `json:"scope"`
	// CanExpandToOrg dice que el origen SÍ permite migrar a toda la organización
	// pero esta conexión aún no tiene con qué (en Google, la cuenta de servicio con
	// delegación). El asistente lo usa para ofrecer el paso extra sin mentir sobre
	// lo que hoy se puede traer.
	CanExpandToOrg bool `json:"can_expand_to_org"`
	// SourceUsers son las cuentas del origen a migrar cuando el alcance es la
	// organización. Vacío ⇒ solo la conectada.
	SourceUsers []string   `json:"source_users,omitempty"`
	Items       []PlanItem `json:"items"`
	// TenantID es el inquilino del origen cuando este lo publica (Microsoft 365). El
	// asistente lo guarda en la conexión: sin él no se pueden pedir después los
	// tokens de aplicación con los que se migra a toda la organización, y pedírselo
	// a mano a un administrador es justo el paso que este asistente evita.
	TenantID string `json:"tenant_id,omitempty"`
}

// Discover interroga al origen con la credencial ya autorizada. No escribe nada.
func Discover(ctx context.Context, rt connector.Runtime, inst connector.Instance, src domain.MigrationSource) (Discovery, error) {
	switch src {
	case domain.SourceGitHub:
		return discoverGitHub(ctx, rt, inst)
	case domain.SourceGoogle:
		return discoverGoogle(ctx, rt, inst)
	case domain.SourceM365:
		return discoverM365(ctx, rt, inst)
	default:
		return Discovery{}, fmt.Errorf("migrate: origen %q no soportado", src)
	}
}

// --- GitHub ------------------------------------------------------------------

// discoverGitHub lee la cuenta autorizada y cuánto tiene. GitHub publica los
// contadores en el propio perfil, así que el plan enseña números reales sin
// recorrer nada.
func discoverGitHub(ctx context.Context, rt connector.Runtime, inst connector.Instance) (Discovery, error) {
	g, err := newGitHub(rt, inst)
	if err != nil {
		return Discovery{}, err
	}
	//nolint:tagliatelle // contrato de la API REST de GitHub.
	var me struct {
		Login             string `json:"login"`
		Name              string `json:"name"`
		Email             string `json:"email"`
		PublicRepos       int    `json:"public_repos"`
		TotalPrivateRepos int    `json:"total_private_repos"`
		PublicGists       int    `json:"public_gists"`
		PrivateGists      int    `json:"private_gists"`
	}
	if err := g.get(ctx, "/user", &me); err != nil {
		return Discovery{}, err
	}
	return Discovery{
		ConnectedAs:   firstNonEmpty(me.Login, me.Email),
		ConnectedName: firstNonEmpty(me.Name, me.Login),
		Scope:         ScopeUser,
		Items: []PlanItem{
			// Los contadores del perfil no incluyen los repositorios de otras personas
			// en los que se colabora, así que el recorrido puede traer MÁS de lo que
			// dice el plan. Quedarse corto es el error correcto: el informe final
			// cuenta lo que de verdad se migró.
			{Domain: domain.DomainRepos, Destination: "git", Count: me.PublicRepos + me.TotalPrivateRepos},
			{Domain: domain.DomainGists, Destination: "drive", Count: me.PublicGists + me.PrivateGists},
		},
	}, nil
}

// --- Google ------------------------------------------------------------------

// discoverGoogle mira quién autorizó y hasta dónde llega esa autorización.
//
// La distinción que importa: un token de USUARIO de Google solo abre el contenido
// de esa persona —no hay forma de leer el Drive de la plantilla con él, y fingir lo
// contrario acabaría en un 403 a mitad de migración—. Para toda la organización
// hace falta una cuenta de servicio con delegación a nivel de dominio; si la
// conexión la tiene, el directorio se lee y el plan sale con todo el personal.
func discoverGoogle(ctx context.Context, rt connector.Runtime, inst connector.Instance) (Discovery, error) {
	d := Discovery{
		Scope:          ScopeUser,
		CanExpandToOrg: true, // Google siempre permite el camino de la delegación.
		Items: []PlanItem{
			{Domain: domain.DomainDrive, Destination: "drive", Count: -1},
			{Domain: domain.DomainContacts, Destination: "contacts", Count: -1},
			{Domain: domain.DomainMail, Destination: "mail", Count: -1},
			{Domain: domain.DomainCalendar, Destination: "calendar", Count: -1},
		},
	}

	// Las aplicaciones solo se ofrecen si la conexión dice DE QUÉ proyecto de
	// Google Cloud migrar. Sin ese dato no hay nada que listar, y ofrecerlo igual
	// pondría en el plan una casilla que solo sabe fallar.
	if strings.TrimSpace(inst.Config[ConfigGCPProject]) != "" {
		d.Items = append(d.Items, PlanItem{Domain: domain.DomainCloud, Destination: "cloud", Count: -1})
	}

	// Camino de la organización: si la cuenta de servicio con delegación ya está
	// puesta, el directorio del Workspace es legible y se migra a todo el mundo.
	if users, err := googleDirectory(ctx, rt, inst); err == nil && len(users) > 0 {
		d.Scope = ScopeOrganization
		d.CanExpandToOrg = false // ya está: no hay nada más que pedir.
		d.SourceUsers = users
		d.ConnectedAs = strings.TrimSpace(inst.Config[gws.ConfigAdminEmail])
		return d, nil
	}

	// Camino del login: quién autorizó. userinfo es el endpoint estándar de OIDC y
	// el scope que lo abre (email) va en el consentimiento.
	base, err := baseURLOr(inst.Config[ConfigGoogleDriveBase], defaultGoogleDriveBase)
	if err != nil {
		return Discovery{}, err
	}
	var me struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := getJSON(ctx, rt, base+"/oauth2/v3/userinfo", "", &me); err != nil {
		return Discovery{}, err
	}
	d.ConnectedAs = me.Email
	d.ConnectedName = firstNonEmpty(me.Name, me.Email)
	return d, nil
}

// googleDirectory lista los correos del Workspace impersonando al administrador
// delegado. Un error aquí NO es un fallo del asistente: significa que esta conexión
// no tiene delegación, y el plan se queda en la cuenta de quien se conectó.
func googleDirectory(ctx context.Context, rt connector.Runtime, inst connector.Instance) ([]string, error) {
	token, err := gws.UserToken(ctx, rt, inst, "", scopeDirectoryRead)
	if err != nil {
		return nil, err
	}
	base, err := baseURLOr(inst.Config[gws.ConfigDirectoryBase], defaultDirectoryBase)
	if err != nil {
		return nil, err
	}
	customer := strings.TrimSpace(inst.Config[gws.ConfigCustomer])
	if customer == "" {
		customer = "my_customer"
	}

	out := make([]string, 0, 128)
	pageToken := ""
	for page := 0; page < maxPages && len(out) < maxDiscoveredUsers; page++ {
		q := url.Values{}
		q.Set("customer", customer)
		q.Set("maxResults", strconv.Itoa(directoryPageSize))
		q.Set("projection", "basic")
		q.Set("orderBy", "email") // orden estable: dos planes seguidos coinciden.
		if dom := strings.TrimSpace(inst.Config[gws.ConfigDomain]); dom != "" {
			q.Set("domain", dom)
		}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		//nolint:tagliatelle // contrato del Directory API de Google (camelCase).
		var resp struct {
			Users []struct {
				PrimaryEmail string `json:"primaryEmail"`
				Suspended    bool   `json:"suspended"`
			} `json:"users"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := getJSON(ctx, rt, base+"/admin/directory/v1/users?"+q.Encode(), token, &resp); err != nil {
			return nil, err
		}
		for _, u := range resp.Users {
			// Las cuentas suspendidas se dejan fuera: su contenido no se puede leer y
			// aparecerían en el informe como una fila de fallos por persona.
			if !u.Suspended && strings.TrimSpace(u.PrimaryEmail) != "" {
				out = append(out, u.PrimaryEmail)
			}
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}
	return out, nil
}

// --- Microsoft 365 ------------------------------------------------------------

// discoverM365 mira quién autorizó y hasta dónde llega esa autorización.
//
// Igual que en Google, un token de USUARIO solo abre el contenido de esa persona.
// La diferencia es cómo se amplía: en Microsoft basta con que un administrador del
// inquilino apruebe UNA pantalla (consentimiento de administrador) para que la
// misma aplicación pase a tener permisos de aplicación sobre toda la organización.
// No hay cuentas de servicio ni claves que pegar.
func discoverM365(ctx context.Context, rt connector.Runtime, inst connector.Instance) (Discovery, error) {
	base, err := graph.APIBase(inst)
	if err != nil {
		return Discovery{}, err
	}
	d := Discovery{
		Scope:          ScopeUser,
		CanExpandToOrg: true,
		Items: []PlanItem{
			{Domain: domain.DomainDrive, Destination: "drive", Count: -1},
			{Domain: domain.DomainContacts, Destination: "contacts", Count: -1},
			{Domain: domain.DomainTeams, Destination: "rooms", Count: -1},
			{Domain: domain.DomainTeamFiles, Destination: "drive", Count: -1},
		},
		TenantID: strings.TrimSpace(inst.Config[graph.ConfigTenant]),
	}

	// Camino de la organización: si la aplicación ya tiene permisos de aplicación en
	// el inquilino, el directorio es legible y se migra a todo el mundo.
	if users, err := m365Directory(ctx, rt, inst, base); err == nil && len(users) > 0 {
		d.Scope = ScopeOrganization
		d.CanExpandToOrg = false
		d.SourceUsers = users
		// Quién conectó se anotó en la conexión al autorizar; sin eso, el plan diría
		// «—» justo cuando ya sabemos de sobra de quién es la sesión.
		d.ConnectedAs = strings.TrimSpace(inst.Config[ConfigConnectedAs])
		return d, nil
	}

	// Camino del login: quién autorizó y en qué inquilino.
	//nolint:tagliatelle // tags = contrato de Microsoft Graph.
	var me struct {
		UserPrincipalName string `json:"userPrincipalName"`
		DisplayName       string `json:"displayName"`
		Mail              string `json:"mail"`
	}
	if err := getJSON(ctx, rt, base+"/v1.0/me", "", &me); err != nil {
		return Discovery{}, err
	}
	d.ConnectedAs = firstNonEmpty(me.Mail, me.UserPrincipalName)
	d.ConnectedName = firstNonEmpty(me.DisplayName, d.ConnectedAs)
	if d.TenantID == "" {
		d.TenantID = m365Tenant(ctx, rt, base, me.UserPrincipalName)
	}

	// Cuántos equipos hay: es el número que hace creíble el plan («12 equipos»), y
	// es barato. Si no se puede leer, se deja como desconocido en vez de inventarlo.
	if teams, err := (&m365Teams{m365: &m365{rt: rt, base: base}}).ListTeams(ctx); err == nil {
		for i := range d.Items {
			if d.Items[i].Domain == domain.DomainTeams {
				d.Items[i].Count = len(teams)
			}
		}
	}
	return d, nil
}

// m365Directory lista los UPN del inquilino con permisos de APLICACIÓN. Un error
// aquí no es un fallo del asistente: significa que todavía no hay consentimiento de
// administrador, y el plan se queda en la cuenta de quien se conectó.
func m365Directory(ctx context.Context, rt connector.Runtime, inst connector.Instance, base string) ([]string, error) {
	token, err := graph.AppToken(ctx, rt, inst)
	if err != nil {
		return nil, err
	}
	next := base + "/v1.0/users?$top=200&$select=userPrincipalName,accountEnabled,userType"
	out := make([]string, 0, 128)
	for page := 0; next != "" && page < maxPages && len(out) < maxDiscoveredUsers; page++ {
		//nolint:tagliatelle // @odata.nextLink es el nombre literal que emite Graph.
		var resp struct {
			Value []struct {
				UserPrincipalName string `json:"userPrincipalName"`
				AccountEnabled    bool   `json:"accountEnabled"`
				UserType          string `json:"userType"`
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := getJSON(ctx, rt, next, token, &resp); err != nil {
			return nil, err
		}
		for _, u := range resp.Value {
			// Cuentas deshabilitadas e invitados fuera: de las primeras no se puede
			// leer nada, y los segundos son de otra organización.
			if !u.AccountEnabled || strings.EqualFold(u.UserType, "Guest") {
				continue
			}
			if upn := strings.TrimSpace(u.UserPrincipalName); upn != "" {
				out = append(out, upn)
			}
		}
		if strings.TrimSpace(resp.NextLink) == "" {
			break
		}
		if next, err = sameHostNext(next, resp.NextLink); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// m365Tenant averigua el inquilino de quien acaba de conectar.
//
// Primero se pregunta a Graph; si el consentimiento no incluye leer la
// organización —y no lo incluye a propósito: pedir Organization.Read.All para esto
// llenaría de permisos una pantalla que debe caber en un vistazo—, se usa el
// DOMINIO del UPN. Entra acepta un dominio verificado como identificador de
// inquilino en su token endpoint, así que sirve igual y no cuesta ni un permiso.
func m365Tenant(ctx context.Context, rt connector.Runtime, base, upn string) string {
	var resp struct {
		Value []struct {
			ID string `json:"id"`
		} `json:"value"`
	}
	if err := getJSON(ctx, rt, base+"/v1.0/organization?$select=id", "", &resp); err == nil && len(resp.Value) > 0 {
		return resp.Value[0].ID
	}
	if _, domainPart, ok := strings.Cut(strings.TrimSpace(upn), "@"); ok {
		return domainPart
	}
	return ""
}

// AdminConsentURL es el atajo de Microsoft: UNA pantalla en la que un administrador
// del inquilino aprueba que la aplicación lea el contenido de la organización. A
// diferencia de Google, aquí no hay cuenta de servicio ni clave que pegar — por eso
// el asistente lleva directamente aquí en vez de explicar una receta de seis pasos.
func AdminConsentURL(authorityBase, tenantID, clientID string) string {
	authority := strings.TrimRight(strings.TrimSpace(authorityBase), "/")
	if authority == "" {
		authority = defaultAuthorityBase
	}
	tenant := strings.TrimSpace(tenantID)
	if tenant == "" {
		tenant = "common" // sin inquilino conocido, Microsoft lo resuelve al entrar.
	}
	q := url.Values{}
	q.Set("client_id", strings.TrimSpace(clientID))
	q.Set("scope", graphDefaultScope)
	return authority + "/" + url.PathEscape(tenant) + "/v2.0/adminconsent?" + q.Encode()
}

// DelegationSetupURL es el atajo que le ahorra a un administrador buscar la
// pantalla: lleva a la delegación a nivel de dominio de la consola de Google con el
// cliente y los scopes YA rellenos, listos para autorizar de una vez.
//
// clientID es el «Client ID» numérico de la cuenta de servicio (el campo
// client_id del JSON de la clave), no el client_id OAuth de la app de Sytue.
//
// La lista tiene que ser la MISMA que la migración pide después al impersonar
// (migrate.go: un scope por dominio) más la del driver del Directory. Un scope que
// falte aquí no se ve al autorizar: se ve horas más tarde, cuando el trabajo del
// correo muere con `unauthorized_client` a mitad de buzón y hay que volver a pedirle
// al administrador que abra la consola de Google.
//
// withCloud añade el scope de lectura de Google Cloud. Va aparte porque solo hace
// falta si la conexión dice de qué proyecto migrar, y pedir permiso de lectura sobre
// TODA la plataforma de Google a quien solo trae correo y ficheros es pedir de más
// —justo lo que hace que un administrador cierre la pantalla sin autorizar—.
func DelegationSetupURL(clientID string, withCloud bool) string {
	scopes := []string{
		scopeDriveRead,
		scopeContactsRead,
		scopeGmailRead,
		scopeCalendarRead,
		scopeDirectoryRead,
		scopeDirectoryGroupRead,
	}
	if withCloud {
		scopes = append(scopes, scopeCloudRead)
	}
	q := url.Values{}
	q.Set("clientScopes", strings.Join(scopes, ","))
	q.Set("overwriteClientId", "true")
	if strings.TrimSpace(clientID) != "" {
		q.Set("clientIdToAdd", strings.TrimSpace(clientID))
	}
	return "https://admin.google.com/ac/owl/domainwidedelegation?" + q.Encode()
}
