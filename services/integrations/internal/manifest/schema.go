// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package manifest es el motor declarativo del catálogo del Sytue Integration
// Gateway (OLA 3): parsea y VALIDA el manifiesto de una plantilla —qué identidad
// (SAML/OIDC), aprovisionamiento (SCIM), driver de datos, campos de admin y modo de
// lanzamiento la definen— y lo MATERIALIZA al activar una instancia (crea el SP SAML
// o el client OIDC en Sytue ID, resuelve la URL de lanzamiento, agenda el sync SCIM).
//
// El manifiesto es JSON opaco para el núcleo del gateway; su forma y sus invariantes
// viven AQUÍ (un JSON Schema propio, sin dependencias externas). La presentación del
// tile «como producto Sytue» (nombre/descripcion/color/icono) también sale del
// manifiesto, de modo que una plantilla nueva es solo un fichero de semilla.
package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"git.sytue.eu/sytue/one/services/integrations/internal/consent"
)

// ErrInvalid indica un manifiesto que no cumple el esquema (la capa HTTP → 400/500
// según el origen: entrada de admin vs. semilla del repo).
var ErrInvalid = errors.New("manifest: manifiesto inválido")

// maxFields acota los campos de admin de una plantilla: un alta 1-click no debe pedir
// un formulario largo. Tres bastan para subdominio/host/instancia.
const maxFields = 3

// Identity/Provisioning/Launch kinds. Cadenas estables (se serializan en semillas).
const (
	IdentitySAML = "saml"
	IdentityOIDC = "oidc"
	IdentityNone = "none"

	ProvisioningSCIM = "scim"
	ProvisioningNone = "none"

	LaunchInitiate = "initiate" // SAML IdP-initiated: la URL la da el SP creado en id.
	LaunchURL      = "url"      // URL directa (plantilla {campo}); típico OIDC.
	// LaunchNone: la conexión NO se abre desde el portal. Es el caso honesto de un
	// conector de solo datos cuyo destino no tiene una URL de producto que ofrecer
	// (la propia suite: sus apps ya están en el lanzador). Sin URL resuelta, MyApps
	// no pinta tile — que es justo lo que debe pasar, en vez de inventar un href.
	LaunchNone = "none"
)

// fieldKeyRe acota la clave de un campo de admin: minúscula inicial, luego minúsculas/
// dígitos/guion bajo. Garantiza que NUNCA colisione con las claves internas del
// gateway en Config (prefijadas con '_', ver materialize.go).
var fieldKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,30}$`)

// placeholderRe extrae los {campo} de una plantilla de URL.
var placeholderRe = regexp.MustCompile(`\{([a-z0-9_]+)\}`)

// Manifest es la forma tipada y validada de un manifiesto de plantilla.
type Manifest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category,omitempty"`
	Color       string   `json:"color"`
	Icon        string   `json:"icon"` // SVG inline curado (nunca hotlink a terceros).
	Identity    Identity `json:"identity"`
	// Provisioning: "scim" agenda un sync SCIM saliente al activar; "" ⇒ "none".
	// El valor ES el Kind del driver que sirve la capacidad (ver DriverFor).
	Provisioning string `json:"provisioning,omitempty"`
	// SCIMBaseURL es la raíz SCIM 2.0 del SaaS, con plantillas {campo} como el resto de
	// URLs del manifiesto. OBLIGATORIA si Provisioning es "scim": sin ella el driver no
	// sabe contra qué endpoint reconciliar y el aprovisionamiento moriría en el outbox
	// tras reintentar — que es exactamente lo que pasaba cuando esto no se validaba.
	SCIMBaseURL string `json:"scim_base_url,omitempty"`
	// Data es el Kind del driver tabular (odata/odoo…) si la app expone datos; "" si no.
	Data string `json:"data,omitempty"`
	// Embed queda reservado para el EmbedBroker (aún sin driver): se valida si viene,
	// no se actúa. ponytail: sin capacidad Embed en el registro todavía.
	Embed  bool    `json:"embed,omitempty"`
	Launch Launch  `json:"launch"`
	Fields []Field `json:"fields,omitempty"`
	// Secret describe (si la app lo necesita) el único secreto write-only que el admin
	// aporta al activar —un token/API key— que la pasarela inyecta en el egress SCIM/
	// datos (clave convencional del vault: "bearer"). El client_secret OIDC NO es esto:
	// ese lo genera Sytue ID. nil ⇒ la app no pide secreto.
	Secret *SecretField `json:"secret,omitempty"`
	// OAuth declara que la app se conecta por CONSENTIMIENTO (authorization code +
	// PKCE): el admin pulsa «Conectar con X», aprueba en el proveedor y vuelve
	// conectado. Es el camino POR DEFECTO del producto; `Secret` es el último recurso,
	// solo para proveedores que no ofrezcan flujo de consentimiento. nil ⇒ no lo ofrece.
	//
	// CONDICIÓN PARA AÑADIRLO A UNA PLANTILLA: su driver tiene que consumir el bearer
	// que INYECTA la pasarela (policy.BuildRuntime lo saca de la clave `bearer` del
	// vault). El canje deja ahí el access token, así que un driver que lea esa clave
	// —powerbi, databricks— o que confíe en la cabecera inyectada —salesforce, odata—
	// funciona sin tocarlo. Los que se fabrican su propia credencial NO: declarar `oauth`
	// en su plantilla haría que el asistente prometiera un consentimiento que el driver
	// luego ignora, y la conexión fallaría DESPUÉS de que el admin haya autorizado — el
	// peor momento para fallar. Convertirlas exige tocar el driver primero:
	//
	//   - `jira` y `servicenow` ya están convertidos: arman Basic con claves propias, pero
	//     se APARTAN cuando la instancia tiene bearer (connector.Instance.HasBearer), y
	//     Jira además habla por api.atlassian.com/ex/jira/{cloudid}, que es donde Atlassian
	//     atiende los tokens de consentimiento.
	//   - `graph` y `gws` mintean su propio token (client_credentials / JWT bearer): siguen
	//     sin consentimiento porque su credencial no sale de un canje con un humano.
	//   - `odoo` NO se puede convertir, y no es cuestión de esfuerzo: su API externa solo
	//     acepta login + contraseña/API key DENTRO del cuerpo del JSON-RPC (verificado en
	//     la documentación de Odoo), y Odoo no publica servidor de autorización OAuth 2.0.
	//     No hay Bearer que inyectar ni endpoint al que mandar al admin.
	//
	// Declararlo hace que `Secret` deje de ser obligatorio para `provisioning: scim`:
	// el token con el que se aprovisiona sale del canje del código, no del portapapeles
	// del admin. Un token así caduca, se acota por scope y se revoca desde el proveedor.
	OAuth *OAuthFlow `json:"oauth,omitempty"`
	// Substitutes declara qué productos de la suite puede REEMPLAZAR esta app. Es la
	// pieza de la libertad de elección: conectar Google Workspace y designarlo
	// sustituto de Sytue Sheets, Teams en vez de Rooms, Azure en vez de Cloud.
	//
	// Es un hecho del PROVEEDOR y por eso vive en la semilla, no en una preferencia de
	// organización: solo se puede ofrecer como sustituto de hojas de cálculo algo que
	// de verdad las edite. Sin esta lista, el catálogo dejaría elegir a Slack como
	// sustituto de Sytue Mail.
	Substitutes []Substitution `json:"substitutes,omitempty"`
	// Hosts declara los hosts oficiales de API del proveedor (p. ej. *.googleapis.com,
	// graph.microsoft.com). Cuando una plantilla los declara, el runtime de egreso y la
	// salida delegada de migraciones solo permiten hablar con ellos y con las URLs del
	// propio manifiesto.
	Hosts []string `json:"hosts,omitempty"`
}

// Substitution es la capacidad de reemplazar UN producto de la suite.
//
// El alcance es deliberadamente explícito para no prometer más de lo que hay: hoy la
// sustitución significa que el lanzador y el conmutador de apps abren el sustituto en
// lugar del producto Sytue. El HANDOFF por documento —«abre ESTE fichero allí»— solo
// existe si el proveedor publica una URL para ello, y la federación del dato (búsqueda
// del Hub, adjuntos, notificaciones) es otra capa que aún no cubre esto.
type Substitution struct {
	// Product es la clave canónica del producto Sytue (mail, drive, docs, sheets,
	// slides, calendar, contacts, rooms, meet, cloud, id). Coincide con ProductKey del
	// front (@sytue/ui apps.ts): es el mismo catálogo visto desde el otro lado.
	Product string `json:"product"`
	// OpenURL abre el sustituto (plantilla {campo}, como el resto de URLs). Vacío ⇒ se
	// usa la URL de lanzamiento normal de la conexión.
	OpenURL string `json:"open_url,omitempty"`
	// DocumentURL abre UN documento concreto en el sustituto; admite {campo} y además
	// {document_id}, que lo aporta quien hace el handoff. Vacío ⇒ no hay handoff por
	// documento y la UI NO debe ofrecerlo: mandar al usuario a la portada del tercero
	// cuando pidió abrir un fichero concreto es peor que no ofrecer el botón.
	DocumentURL string `json:"document_url,omitempty"`
}

// substitutableProducts son las claves de producto que se pueden sustituir. Es una lista
// CERRADA a propósito: una semilla con una errata («sheet» en vez de «sheets») fallaría
// en silencio y la sustitución no se ofrecería nunca sin que nadie supiera por qué.
var substitutableProducts = map[string]bool{
	"mail": true, "drive": true, "docs": true, "sheets": true, "slides": true,
	"calendar": true, "contacts": true, "rooms": true, "meet": true, "chat": true,
	"cloud": true, "id": true, "hub": true,
}

// OAuthFlow son los datos del proveedor —no del tenant— que hacen falta para el flujo
// de consentimiento. Las URLs admiten plantillas {campo} como el resto del manifiesto,
// porque hay proveedores con endpoint por instancia (ServiceNow, Odoo autoalojado).
//
// Lo que NO vive aquí es el client_id/client_secret con el que Sytue se presenta ante
// el proveedor: eso es una credencial, no una descripción, y su sitio es la config del
// servicio (app registrada por Sytue ⇒ 1 clic de verdad) o la instancia (el tenant trae
// la suya). El manifiesto es una semilla del repo y jamás lleva secretos.
//
// PKCE va SIEMPRE con S256, sin interruptor: es obligatorio en OAuth 2.1 y los
// proveedores que aún no lo exigen ignoran los parámetros extra (RFC 7636 §4.4). Un
// flag solo serviría para poder desactivar la protección.
type OAuthFlow struct {
	// ScopeParameter selects Slack user consent; it cannot override another OAuth parameter.
	ScopeParameter string `json:"scope_parameter,omitempty"`
	AuthorizeURL   string `json:"authorize_url"`
	TokenURL       string `json:"token_url"`
	// Scopes que se piden en el consentimiento. Se muestran al admin ANTES de
	// redirigir: el paso «qué va a pasar» explica en llano qué se está autorizando.
	Scopes []string `json:"scopes,omitempty"`
	// WriteScopes se piden ADEMÁS de Scopes solo cuando el consentimiento es para
	// LLEVAR contenido de Sytue a este proveedor (purpose "out" en oauth/start). Van
	// aparte para no conceder de más: quien conecta para traer sus datos no tiene por
	// qué dar permiso para escribir en su cuenta, y quien se los lleva sí lo necesita.
	WriteScopes []string `json:"write_scopes,omitempty"`
	// RevokeURL (RFC 7009) permite que DESCONECTAR revoque de verdad contra el
	// proveedor en vez de solo borrar la fila local. Vacío ⇒ el proveedor no publica
	// endpoint de revocación y la desconexión solo puede ser local; la UI debe decirlo.
	RevokeURL string `json:"revoke_url,omitempty"`
	// AuthorizeParams son parámetros FIJOS que el proveedor exige en la URL de
	// autorización además de los del estándar: Google no entrega refresh token
	// sin `access_type=offline` y `prompt=consent`. Nunca sustituyen a los del
	// estándar (client_id, state, PKCE…), que pone consent.Begin.
	AuthorizeParams map[string]string `json:"authorize_params,omitempty"`
}

// SecretField describe el campo de secreto write-only del alta (solo presentación: el
// valor viaja aparte, a PUT /secrets, y jamás se devuelve).
type SecretField struct {
	Label string `json:"label"`
	Help  string `json:"help,omitempty"`
}

// Identity describe cómo Sytue ID actúa de IdP para la app: SAML (SP registrado) u
// OIDC (client registrado por DCR), o none (la app no delega login en Sytue).
type Identity struct {
	Kind string `json:"kind"`
	// SAML (plantillas {campo} resueltas con los valores de admin al activar):
	EntityID     string `json:"entity_id,omitempty"`
	ACSURL       string `json:"acs_url,omitempty"`
	NameIDFormat string `json:"name_id_format,omitempty"`
	// OIDC: los redirect_uris que la app registra (≥1, plantillas {campo}).
	RedirectURIs []string `json:"redirect_uris,omitempty"`
}

// Launch resuelve el href del tile «como producto Sytue»: initiate (SAML IdP-initiated,
// URL emitida por id) o url (plantilla {campo} directa al SaaS).
type Launch struct {
	Kind string `json:"kind"`
	URL  string `json:"url,omitempty"`
}

// Field es un campo de admin del alta 1-click (subdominio, host…). Su valor rellena
// las plantillas {campo} de las URLs del manifiesto.
type Field struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Parse decodifica y VALIDA un manifiesto. Devuelve ErrInvalid (envuelto con el motivo)
// si no cumple el esquema.
func Parse(raw []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: JSON: %w", ErrInvalid, err)
	}
	if err := m.validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// validate aplica el esquema. Cada fallo nombra el campo (mensajes sobrios para el UX
// de admin y para diagnosticar una semilla mal escrita).
func (m *Manifest) validate() error {
	if err := m.validatePresentation(); err != nil {
		return err
	}
	keys, err := m.validateFields()
	if err != nil {
		return err
	}
	if err := m.validateTemplates(keys); err != nil {
		return err
	}
	if m.Secret != nil && strings.TrimSpace(m.Secret.Label) == "" {
		return fmt.Errorf("%w: secret.label es obligatorio si se declara secret", ErrInvalid)
	}

	if err := m.validateOAuth(); err != nil {
		return err
	}
	if err := m.validateSubstitutes(keys); err != nil {
		return err
	}
	if err := m.validateHosts(); err != nil {
		return err
	}
	if err := m.validateIdentity(); err != nil {
		return err
	}
	if err := m.validateProvisioning(); err != nil {
		return err
	}
	return m.validateLaunch()
}

// validatePresentation exige lo que la tesela del catálogo necesita para pintarse.
func (m *Manifest) validatePresentation() error {
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("%w: falta name", ErrInvalid)
	}
	if strings.TrimSpace(m.Description) == "" {
		return fmt.Errorf("%w: falta description", ErrInvalid)
	}
	// color alimenta el acento del tile; se exige un color CSS reconocible (# o var()).
	if c := strings.TrimSpace(m.Color); c == "" || (!strings.HasPrefix(c, "#") && !strings.HasPrefix(c, "var(")) {
		return fmt.Errorf("%w: color debe ser un color CSS (#hex o var(--token))", ErrInvalid)
	}
	// icono curado en el repo: SVG inline, jamás una URL a un tercero.
	if !strings.HasPrefix(strings.TrimSpace(m.Icon), "<svg") {
		return fmt.Errorf("%w: icon debe ser un SVG inline (curado en el repo)", ErrInvalid)
	}
	return nil
}

// validateFields comprueba los campos de admin (≤3, claves válidas y únicas) y
// devuelve el conjunto de claves declaradas, que es lo que resuelve las plantillas.
func (m *Manifest) validateFields() (map[string]struct{}, error) {
	if len(m.Fields) > maxFields {
		return nil, fmt.Errorf("%w: como máximo %d campos de admin", ErrInvalid, maxFields)
	}
	keys := make(map[string]struct{}, len(m.Fields))
	for _, f := range m.Fields {
		if !fieldKeyRe.MatchString(f.Key) {
			return nil, fmt.Errorf("%w: clave de campo inválida %q", ErrInvalid, f.Key)
		}
		if _, dup := keys[f.Key]; dup {
			return nil, fmt.Errorf("%w: clave de campo duplicada %q", ErrInvalid, f.Key)
		}
		if strings.TrimSpace(f.Label) == "" {
			return nil, fmt.Errorf("%w: el campo %q necesita label", ErrInvalid, f.Key)
		}
		keys[f.Key] = struct{}{}
	}
	return keys, nil
}

// validateTemplates exige que toda URL con plantilla {x} referencie un campo declarado:
// un placeholder colgante se filtraría SIN RESOLVER a la petición saliente.
func (m *Manifest) validateTemplates(keys map[string]struct{}) error {
	for _, t := range m.plantillasDeURL() {
		for _, ph := range placeholders(t) {
			if _, ok := keys[ph]; !ok {
				return fmt.Errorf("%w: la plantilla usa {%s} pero no hay un campo con esa clave", ErrInvalid, ph)
			}
		}
	}
	return nil
}

// validateOAuth exige que declarar consentimiento implique poder ejecutarlo. Las dos
// URLs van por HTTPS sin excepción: por ahí viaja el código de autorización y el canje
// que devuelve el access token, así que en claro sería entregar la cuenta del cliente.
func (m *Manifest) validateOAuth() error {
	if m.OAuth == nil {
		return nil
	}
	for _, u := range []struct{ name, val string }{
		{"authorize_url", m.OAuth.AuthorizeURL},
		{"token_url", m.OAuth.TokenURL},
	} {
		if strings.TrimSpace(u.val) == "" {
			return fmt.Errorf("%w: oauth exige %s", ErrInvalid, u.name)
		}
		if !httpsOrTemplated(u.val) {
			return fmt.Errorf("%w: oauth.%s debe ser https", ErrInvalid, u.name)
		}
	}
	if r := strings.TrimSpace(m.OAuth.RevokeURL); r != "" && !httpsOrTemplated(r) {
		return fmt.Errorf("%w: oauth.revoke_url debe ser https", ErrInvalid)
	}
	if m.OAuth.ScopeParameter != "" && m.OAuth.ScopeParameter != "scope" && m.OAuth.ScopeParameter != "user_scope" {
		return fmt.Errorf("%w: oauth.scope_parameter no permitido", ErrInvalid)
	}
	// Sin scopes el consentimiento pediría el permiso por defecto del proveedor, que
	// suele ser el más amplio. Declararlos es lo que permite enseñárselos al admin.
	if len(nonEmpty(m.OAuth.Scopes)) == 0 {
		return fmt.Errorf("%w: oauth exige al menos un scope (se le muestran al admin antes de redirigir)", ErrInvalid)
	}
	if len(nonEmpty(m.OAuth.WriteScopes)) != len(m.OAuth.WriteScopes) {
		return fmt.Errorf("%w: oauth.write_scopes no admite scopes vacíos", ErrInvalid)
	}
	return nil
}

// validateSubstitutes comprueba que lo que la plantilla dice poder reemplazar existe y
// que sus URLs se pueden resolver. Un producto desconocido se RECHAZA en vez de
// ignorarse: una errata en una semilla («sheet») haría que la sustitución no se ofreciera
// jamás y nadie sabría por qué.
func (m *Manifest) validateSubstitutes(fieldKeys map[string]struct{}) error {
	seen := make(map[string]struct{}, len(m.Substitutes))
	for _, sub := range m.Substitutes {
		p := strings.TrimSpace(sub.Product)
		if !substitutableProducts[p] {
			return fmt.Errorf("%w: %q no es un producto de la suite que se pueda sustituir", ErrInvalid, sub.Product)
		}
		if _, dup := seen[p]; dup {
			return fmt.Errorf("%w: la plantilla declara dos veces que sustituye a %q", ErrInvalid, p)
		}
		seen[p] = struct{}{}
		for _, u := range []string{sub.OpenURL, sub.DocumentURL} {
			for _, ph := range placeholders(u) {
				// {document_id} lo aporta quien hace el handoff, no un campo de admin.
				if ph == "document_id" {
					continue
				}
				if _, ok := fieldKeys[ph]; !ok {
					return fmt.Errorf("%w: la URL de sustitución usa {%s} pero no hay un campo con esa clave", ErrInvalid, ph)
				}
			}
		}
		if strings.TrimSpace(sub.DocumentURL) != "" && !strings.Contains(sub.DocumentURL, "{document_id}") {
			return fmt.Errorf("%w: document_url de %q debe llevar {document_id}: sin él abriría la portada del tercero en vez del documento", ErrInvalid, p)
		}
	}
	return nil
}

// SubstitutionFor devuelve lo que la plantilla declara sobre reemplazar un producto, o
// nil si no lo reemplaza.
func (m *Manifest) SubstitutionFor(product string) *Substitution {
	for i := range m.Substitutes {
		if m.Substitutes[i].Product == product {
			return &m.Substitutes[i]
		}
	}
	return nil
}

func (m *Manifest) validateIdentity() error {
	switch m.Identity.Kind {
	case IdentitySAML:
		if strings.TrimSpace(m.Identity.EntityID) == "" || strings.TrimSpace(m.Identity.ACSURL) == "" {
			return fmt.Errorf("%w: identity SAML exige entity_id y acs_url", ErrInvalid)
		}
	case IdentityOIDC:
		if len(nonEmpty(m.Identity.RedirectURIs)) == 0 {
			return fmt.Errorf("%w: identity OIDC exige al menos un redirect_uri", ErrInvalid)
		}
	case IdentityNone, "":
		m.Identity.Kind = IdentityNone
	default:
		return fmt.Errorf("%w: identity.kind debe ser 'saml', 'oidc' o 'none'", ErrInvalid)
	}
	return nil
}

// validateProvisioning exige que declarar el aprovisionamiento implique poder servirlo:
// sin raíz SCIM el reconcile no tiene destino y sin secreto no tiene con qué
// autenticarse. Antes se podía declarar `provisioning: scim` a secas, y el catálogo
// prometía altas y bajas automáticas que morían en el outbox sin que nadie lo viera.
func (m *Manifest) validateProvisioning() error {
	switch m.provisioningKind() {
	case ProvisioningSCIM:
		if strings.TrimSpace(m.SCIMBaseURL) == "" {
			return fmt.Errorf("%w: provisioning 'scim' exige scim_base_url (la raíz SCIM 2.0 del SaaS)", ErrInvalid)
		}
		// Hace falta CON QUÉ autenticarse, pero da igual de dónde salga: el canje del
		// consentimiento (oauth) o un token pegado (secret). Antes solo valía lo
		// segundo, y eso obligaba a pedir un token a mano incluso a los proveedores
		// que ofrecen consentimiento — justo lo que el producto quiere evitar.
		if m.OAuth == nil && m.Secret == nil {
			return fmt.Errorf("%w: provisioning 'scim' exige oauth (preferente) o secret: sin credencial no hay con qué aprovisionar", ErrInvalid)
		}
		return nil
	case ProvisioningNone:
		if strings.TrimSpace(m.SCIMBaseURL) != "" {
			return fmt.Errorf("%w: scim_base_url sin provisioning 'scim'", ErrInvalid)
		}
		return nil
	default:
		return fmt.Errorf("%w: provisioning debe ser 'scim' o 'none'", ErrInvalid)
	}
}

func (m *Manifest) validateLaunch() error {
	switch m.Launch.Kind {
	case LaunchInitiate:
		if m.Identity.Kind != IdentitySAML {
			return fmt.Errorf("%w: launch 'initiate' solo aplica con identity SAML", ErrInvalid)
		}
	case LaunchURL:
		if strings.TrimSpace(m.Launch.URL) == "" {
			return fmt.Errorf("%w: launch 'url' exige una url", ErrInvalid)
		}
	case LaunchNone:
		if strings.TrimSpace(m.Launch.URL) != "" {
			return fmt.Errorf("%w: launch 'none' no lleva url", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: launch.kind debe ser 'initiate', 'url' o 'none'", ErrInvalid)
	}
	return nil
}

// provisioningKind normaliza "" a "none".
func (m *Manifest) provisioningKind() string {
	if strings.TrimSpace(m.Provisioning) == "" {
		return ProvisioningNone
	}
	return m.Provisioning
}

// RequiredFieldKeys devuelve las claves de los campos obligatorios (el activar valida
// que estén presentes en la config de la instancia).
func (m *Manifest) RequiredFieldKeys() []string {
	out := make([]string, 0, len(m.Fields))
	for _, f := range m.Fields {
		if f.Required {
			out = append(out, f.Key)
		}
	}
	return out
}

// reEnHost es lo que admite un {campo} que cae en el HOST de una URL: letras, dígitos,
// guiones y puntos. Sin «/», «?», «#», «@» ni «:», el valor no puede sacar la URL del
// dominio que escribe el manifiesto: «https://{subdominio}.proveedor.com» acaba siempre
// en «.proveedor.com».
var reEnHost = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$`)

// reConPuerto admite además «:puerto», y solo vale para el campo que CIERRA el host
// («https://{host}/…», un GitHub Enterprise en otro puerto).
var reConPuerto = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?(:\d{1,5})?$`)

// hostDe devuelve dónde empieza y acaba el host de una plantilla con esquema literal.
// ok=false si el esquema (y con él el host entero) lo aporta un {campo}.
func hostDe(tmpl string) (ini, fin int, ok bool) {
	switch {
	case strings.HasPrefix(tmpl, "https://"):
		ini = len("https://")
	case strings.HasPrefix(tmpl, "http://"):
		ini = len("http://")
	default:
		return 0, 0, false
	}
	fin = len(tmpl)
	if i := strings.IndexAny(tmpl[ini:], "/?#"); i >= 0 {
		fin = ini + i
	}
	return ini, fin, true
}

// HostFijo dice si el host de una URL del manifiesto lo escribe el MANIFIESTO y no un
// campo de la instancia: sin plantillas en el host, o con ellas solo como subdominio de
// un sufijo literal («{subdominio}.my.salesforce.com»). Es la condición para mandar a
// esa URL una credencial que no es de la organización: la de la app que Sytue registra
// ante el proveedor.
func HostFijo(tmpl string) bool {
	ini, fin, ok := hostDe(tmpl)
	if !ok || !strings.HasPrefix(tmpl, "https://") {
		return false
	}
	host := tmpl[ini:fin]
	ultimo := strings.LastIndex(host, "}")
	if ultimo < 0 {
		return host != ""
	}
	// Tras el último campo tiene que quedar un dominio entero: «.proveedor.com».
	sufijo := host[ultimo+1:]
	return strings.HasPrefix(sufijo, ".") && strings.Contains(sufijo[1:], ".")
}

// render resuelve las plantillas {campo} de s con los valores dados. Error si falta un
// valor (no se deja un {campo} sin resolver en una URL saliente) o si el valor de un
// campo que cae en el host lleva algo que no es de un nombre de host.
func render(s string, values map[string]string) (string, error) {
	ini, fin, conHost := hostDe(s)
	var b strings.Builder
	pos := 0
	for _, m := range placeholderRe.FindAllStringSubmatchIndex(s, -1) {
		key := s[m[2]:m[3]]
		v, ok := values[key]
		if !ok || strings.TrimSpace(v) == "" {
			return "", fmt.Errorf("%w: falta el valor del campo {%s}", ErrInvalid, key)
		}
		if conHost && m[0] >= ini && m[0] < fin {
			valido := reEnHost
			if m[1] == fin {
				valido = reConPuerto
			}
			if !valido.MatchString(v) {
				return "", fmt.Errorf("%w: el campo {%s} va en el nombre del servidor y solo admite letras, dígitos, guiones y puntos", ErrInvalid, key)
			}
		}
		b.WriteString(s[pos:m[0]])
		b.WriteString(v)
		pos = m[1]
	}
	b.WriteString(s[pos:])
	return b.String(), nil
}

// Destinos son los servidores a los que apunta una instancia con esos valores: el host
// de cada URL del manifiesto ya resuelta y el de cada valor que es una URL. Los compara
// quien guarda una configuración nueva: si cambian, las credenciales guardadas eran
// para OTRO servidor.
func (m *Manifest) Destinos(values map[string]string) []string {
	vistos := map[string]struct{}{}
	apuntar := func(raw string) {
		if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" {
			vistos[strings.ToLower(u.Host)] = struct{}{}
		}
	}
	for _, t := range m.plantillasDeURL() {
		if out, err := render(t, values); err == nil {
			apuntar(out)
		}
	}
	for k, v := range values {
		if !strings.HasPrefix(k, "_") && strings.Contains(v, "://") {
			apuntar(v)
		}
	}
	out := make([]string, 0, len(vistos))
	for h := range vistos {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// Declara dice si `clave` es un campo de admin de la plantilla.
func (m *Manifest) Declara(clave string) bool {
	for _, f := range m.Fields {
		if f.Key == clave {
			return true
		}
	}
	return false
}

// reHostOficial valida un host declarado en el manifiesto (exacto o comodín *.dominio.tld).
var reHostOficial = regexp.MustCompile(`^(\*\.)?[a-z0-9][a-z0-9.-]*\.[a-z0-9-]+$`)

func (m *Manifest) validateHosts() error {
	for i, raw := range m.Hosts {
		h := strings.ToLower(strings.TrimSpace(raw))
		if !reHostOficial.MatchString(h) {
			return fmt.Errorf("%w: hosts[%d] (%q) debe ser un nombre de servidor oficial (o *.dominio)", ErrInvalid, i, raw)
		}
	}
	return nil
}

// HostsOficiales devuelve los hosts oficiales autorizados por el manifiesto para una
// instancia: los declarados en `hosts` más los de las URLs del propio manifiesto y de
// sus campos declarados. Devuelve nil si la plantilla no declara `hosts`.
func (m *Manifest) HostsOficiales(values map[string]string) []string {
	if len(m.Hosts) == 0 {
		return nil
	}
	vistos := map[string]struct{}{}
	apuntarHost := func(h string) {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			vistos[h] = struct{}{}
		}
	}
	apuntarURL := func(raw string) {
		if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Hostname() != "" {
			apuntarHost(u.Hostname())
		}
	}
	for _, h := range m.Hosts {
		apuntarHost(h)
	}
	for _, t := range m.plantillasDeURL() {
		if out, err := render(t, values); err == nil {
			apuntarURL(out)
		}
	}
	for _, sub := range m.Substitutes {
		if out, err := render(sub.OpenURL, values); err == nil {
			apuntarURL(out)
		}
		if out, err := RenderURLKeeping(sub.DocumentURL, values, "document_id"); err == nil {
			apuntarURL(out)
		}
	}
	for k, v := range values {
		if m.Declara(k) && strings.Contains(v, "://") {
			apuntarURL(v)
		}
	}
	out := make([]string, 0, len(vistos))
	for h := range vistos {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// HostPermitido comprueba si `host` (sin puerto) pertenece a la lista de hosts
// oficiales del manifiesto (coincidencia exacta o comodín `*.sufijo`).
func HostPermitido(host string, oficiales []string) bool {
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

// plantillasDeURL son todas las URL con {campo} del manifiesto.
func (m *Manifest) plantillasDeURL() []string {
	tmpls := append([]string{m.Identity.EntityID, m.Identity.ACSURL, m.Launch.URL, m.SCIMBaseURL}, m.Identity.RedirectURIs...)
	if m.OAuth != nil {
		tmpls = append(tmpls, m.OAuth.AuthorizeURL, m.OAuth.TokenURL, m.OAuth.RevokeURL)
	}
	return tmpls
}

// placeholders lista las claves {x} presentes en s (sin duplicar).
func placeholders(s string) []string {
	ms := placeholderRe.FindAllStringSubmatch(s, -1)
	if len(ms) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(ms))
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		if _, ok := seen[m[1]]; ok {
			continue
		}
		seen[m[1]] = struct{}{}
		out = append(out, m[1])
	}
	return out
}

func nonEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// RenderOAuth materializa el flujo de consentimiento con los valores de admin de la
// instancia: hay proveedores con endpoint por instancia (ServiceNow, Odoo autoalojado,
// el {tenant} de Microsoft), así que las URLs llevan plantillas {campo} como el resto
// del manifiesto. Devuelve ErrInvalid si falta el valor de algún campo, en vez de
// mandar al admin a una URL con un {campo} sin resolver.
//
// No rellena ClientID: esa es una credencial y la aporta quien llama (config del
// servicio o de la instancia), nunca el manifiesto.
func (m *Manifest) RenderOAuth(values map[string]string) (consent.Flow, error) {
	if m.OAuth == nil {
		return consent.Flow{}, fmt.Errorf("%w: la plantilla no declara oauth", ErrInvalid)
	}
	authorize, err := renderHTTPS(m.OAuth.AuthorizeURL, values, "authorize_url")
	if err != nil {
		return consent.Flow{}, err
	}
	token, err := renderHTTPS(m.OAuth.TokenURL, values, "token_url")
	if err != nil {
		return consent.Flow{}, err
	}
	return consent.Flow{AuthorizeURL: authorize, TokenURL: token, Scopes: m.OAuth.Scopes, ScopeParameter: m.OAuth.ScopeParameter, Extra: m.OAuth.AuthorizeParams}, nil
}

// RenderURL resuelve las plantillas {campo} de una URL del manifiesto con los valores de
// admin. Exportada para que la capa HTTP pueda materializar la revoke_url sin duplicar
// el renderizador ni exponer el resto del motor.
func RenderURL(tmpl string, values map[string]string) (string, error) {
	return render(tmpl, values)
}

// SubstitutableProducts devuelve las claves de producto sustituibles, en orden estable
// (el mapa de Go no lo tiene y la lista se pinta en una pantalla de administración).
func SubstitutableProducts() []string {
	out := make([]string, 0, len(substitutableProducts))
	for p := range substitutableProducts {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// IsSubstitutableProduct dice si una clave nombra un producto de la suite sustituible.
func IsSubstitutableProduct(product string) bool { return substitutableProducts[product] }

// RenderURLKeeping resuelve las plantillas {campo} PERO deja intactos los marcadores
// indicados. Lo usa el handoff por documento: {document_id} lo pone el front al abrir un
// fichero concreto, así que la URL se sirve a medio resolver a propósito.
func RenderURLKeeping(tmpl string, values map[string]string, keep ...string) (string, error) {
	vals := make(map[string]string, len(values)+len(keep))
	for k, v := range values {
		vals[k] = v
	}
	for _, k := range keep {
		vals[k] = "{" + k + "}"
	}
	return render(tmpl, vals)
}

// httpsOrTemplated acepta una URL de OAuth en el manifiesto: o empieza por https, o
// empieza por un {campo} que aporta el esquema.
//
// Hay proveedores cuyo endpoint es la propia raíz del workspace —Databricks pide
// «https://dbc-….cloud.databricks.com» como dato de admin— así que exigir el literal
// «https://» al principio del manifiesto obligaría a partir ese campo en dos o a
// duplicarlo. La garantía no se pierde: se traslada a renderHTTPS, que la comprueba
// sobre la URL YA RESUELTA, que es el único momento en que se puede comprobar de verdad.
func httpsOrTemplated(u string) bool {
	return strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "{")
}

// renderHTTPS resuelve las plantillas {campo} y EXIGE que el resultado sea https. Por
// ahí viajan el código de autorización y el canje que devuelve el access token: en claro
// sería entregar la cuenta del cliente a quien escuche la red.
func renderHTTPS(tmpl string, values map[string]string, name string) (string, error) {
	out, err := render(tmpl, values)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(out, "https://") {
		return "", fmt.Errorf("%w: oauth.%s resuelve a una URL que no es https (%q)", ErrInvalid, name, out)
	}
	return out, nil
}
