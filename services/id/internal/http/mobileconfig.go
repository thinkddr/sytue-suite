// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Perfil de configuración de Apple (.mobileconfig): el «conector de sistema
// operativo» de iOS/macOS. Un solo toque instala las cuentas de Sytue Mail
// (IMAP+SMTP), Calendar (CalDAV) y Contacts (CardDAV) ya rellenas con la
// dirección del usuario.
//
// El perfil NO lleva contraseñas: el sistema las pide al instalarlo. El endpoint
// exige sesión (es el único dato personal que se emite: la propia cuenta de quien
// lo descarga). Los hosts/puertos salen de la configuración del servicio.

// handleAppleProfile sirve el perfil del usuario de la sesión actual.
func (s *Server) handleAppleProfile(w http.ResponseWriter, r *http.Request) {
	sess, ok := s.currentSession(r)
	if !ok {
		http.Redirect(w, r, "/login/?return_to=/account", http.StatusFound)
		return
	}
	user, err := s.store.GetUserByID(r.Context(), sess.UserID)
	if err != nil {
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo cargar tu cuenta.")
		return
	}
	// La autoridad S/MIME de SU organización. Si no hay —o Mail no contesta— el
	// perfil sale igual con sus tres cuentas: el correo tiene que poder
	// configurarse aunque la firma no esté montada.
	profile, err := buildAppleProfile(s.cfg, user, s.anclaSMIMEDeLaOrg(r, sess.OrgID), "")
	if err != nil {
		s.log.Error("perfil .mobileconfig", "error", err)
		httpFailHTML(w, r, http.StatusInternalServerError, "No se pudo generar el perfil de configuración.")
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	// Nombre fijo: nada del usuario entra en una cabecera (inyección).
	w.Header().Set("Content-Disposition", `attachment; filename="sytue.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(profile)
}

// anclaSMIMEDeLaOrg trae el certificado PÚBLICO de la autoridad S/MIME de una
// organización, del endpoint que Sytue Mail publica sin sesión.
//
// POR HTTP Y NO POR LA BASE: la autoridad es de Mail, y su material de firma no
// sale de ahí. Lo que se pide es el ancla —el certificado público, el mismo que
// viaja dentro de cada correo firmado—, así que pedirla no da acceso a nada.
//
// NUNCA ROMPE EL PERFIL. Si Mail no contesta, tarda o no tiene autoridad, se
// devuelve vacío y el perfil sale con sus tres cuentas: configurar el correo no
// puede depender de que la firma esté montada. El plazo es corto por lo mismo —
// esto corre dentro de una descarga que alguien está esperando.
func (s *Server) anclaSMIMEDeLaOrg(r *http.Request, orgID string) string {
	if s.cfg.MailAPIURL == "" || orgID == "" {
		return ""
	}
	org, err := s.store.GetOrgByID(r.Context(), orgID)
	if err != nil || org.Slug == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	destino := strings.TrimSuffix(s.cfg.MailAPIURL, "/") + "/.well-known/smime/" + url.PathEscape(org.Slug) + ".pem"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, destino, http.NoBody)
	if err != nil {
		return ""
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return ""
	}
	// Tope de lectura: un certificado son unos pocos kilobytes y al otro lado hay
	// un servicio que podría estar devolviendo cualquier cosa.
	cuerpo, err := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	if err != nil {
		return ""
	}
	return string(cuerpo)
}

// buildAppleProfile construye el plist XML del perfil con las tres cuentas y,
// cuando la organización tiene autoridad S/MIME, su certificado raíz. Con `clave`
// (una contraseña de aplicación recién creada) las tres cuentas la llevan dentro y el
// sistema no pide nada al instalar; vacía, la pide él.
func buildAppleProfile(cfg Config, user domain.User, caPEM, clave string) ([]byte, error) {
	imapHost, imapPort, err := splitAddr(cfg.MailIMAPAddr, 993)
	if err != nil {
		return nil, fmt.Errorf("MAIL_IMAP_ADDR: %w", err)
	}
	smtpHost, smtpPort, err := splitAddr(cfg.MailSMTPAddr, 465)
	if err != nil {
		return nil, fmt.Errorf("MAIL_SMTP_ADDR: %w", err)
	}
	calHost, calPort, calPath, calSSL, err := splitDAVURL(cfg.CalDAVURL)
	if err != nil {
		return nil, fmt.Errorf("CALDAV_URL: %w", err)
	}
	cardHost, cardPort, cardPath, cardSSL, err := splitDAVURL(cfg.CardDAVURL)
	if err != nil {
		return nil, fmt.Errorf("CARDDAV_URL: %w", err)
	}

	base := reverseDomain(suiteDomain(cfg.Issuer)) // eu.sytue
	// UUID estable por usuario+servicio: reinstalar reemplaza el perfil anterior
	// en el dispositivo en vez de duplicar cuentas.
	puid := func(kind string) string {
		return strings.ToUpper(uuid.NewSHA1(uuid.NameSpaceURL,
			[]byte(cfg.Issuer+"/mobileconfig/"+kind+"/"+user.ID)).String())
	}
	name := user.Name
	if name == "" {
		name = user.Email
	}

	payload := func(kind, ptype, display string, fields plistDict) plistDict {
		return append(plistDict{
			{"PayloadType", ptype},
			{"PayloadVersion", 1},
			{"PayloadIdentifier", base + ".sytue-one." + kind},
			{"PayloadUUID", puid(kind)},
			{"PayloadDisplayName", display},
			{"PayloadOrganization", "Sytue"},
		}, fields...)
	}

	mail := payload("mail", "com.apple.mail.managed", "Sytue Mail", plistDict{
		{"EmailAccountDescription", "Sytue Mail"},
		{"EmailAccountName", name},
		{"EmailAccountType", "EmailTypeIMAP"},
		{"EmailAddress", user.Email},
		{"IncomingMailServerHostName", imapHost},
		{"IncomingMailServerPortNumber", imapPort},
		{"IncomingMailServerUseSSL", true},
		{"IncomingMailServerAuthentication", "EmailAuthPassword"},
		{"IncomingMailServerUsername", user.Email},
		{"OutgoingMailServerHostName", smtpHost},
		{"OutgoingMailServerPortNumber", smtpPort},
		{"OutgoingMailServerUseSSL", true},
		{"OutgoingMailServerAuthentication", "EmailAuthPassword"},
		{"OutgoingMailServerUsername", user.Email},
		// El sistema pide UNA contraseña al instalar y la reutiliza en el envío.
		{"OutgoingPasswordSameAsIncomingPassword", true},
	})
	if clave != "" {
		mail = append(mail, plistDict{{"IncomingPassword", clave}, {"OutgoingPassword", clave}}...)
	}

	caldav := payload("caldav", "com.apple.caldav.account", "Sytue Calendar", plistDict{
		{"CalDAVAccountDescription", "Sytue Calendar"},
		{"CalDAVHostName", calHost},
		{"CalDAVPort", calPort},
		{"CalDAVUseSSL", calSSL},
		{"CalDAVPrincipalURL", calPath},
		{"CalDAVUsername", user.Email},
	})
	if clave != "" {
		caldav = append(caldav, plistDict{{"CalDAVPassword", clave}}...)
	}

	carddav := payload("carddav", "com.apple.carddav.account", "Sytue Contactos", plistDict{
		{"CardDAVAccountDescription", "Sytue Contactos"},
		{"CardDAVHostName", cardHost},
		{"CardDAVPort", cardPort},
		{"CardDAVUseSSL", cardSSL},
		{"CardDAVPrincipalURL", cardPath},
		{"CardDAVUsername", user.Email},
	})
	if clave != "" {
		carddav = append(carddav, plistDict{{"CardDAVPassword", clave}}...)
	}

	// LA AUTORIDAD S/MIME, DENTRO DEL PERFIL.
	//
	// Publicar el `.pem` en una URL —que es lo que había— no instala nada: en un
	// teléfono, abrir un certificado suelto desde el navegador acaba, con suerte,
	// en un fichero en Descargas. Dentro del perfil va al almacén correcto de una
	// vez, junto a las cuentas que ya configura, y se retira con él al quitarlo.
	//
	// VA EL DER Y NO EL PEM: `PayloadContent` de `com.apple.security.root` es
	// binario. Metiendo el PEM —que es texto— el perfil se instala igual y el
	// certificado NO queda utilizable, que es el fallo más difícil de ver de los
	// dos.
	cuentas := []plistDict{mail, caldav, carddav}
	if der := derDelPEM(caPEM); len(der) > 0 {
		cuentas = append(cuentas, payload("smime-ca", "com.apple.security.root",
			"Autoridad S/MIME de la organización", plistDict{
				{"PayloadCertificateFileName", "sytue-smime-ca.cer"},
				{"PayloadContent", plistData(der)},
			}))
	}

	root := plistDict{
		{"PayloadType", "Configuration"},
		{"PayloadVersion", 1},
		{"PayloadIdentifier", base + ".sytue-one"},
		{"PayloadUUID", puid("configuration")},
		{"PayloadDisplayName", "Sytue One — " + user.Email},
		{"PayloadDescription", descripcionPerfil(clave != "")},
		{"PayloadOrganization", "Sytue"},
		{"PayloadRemovalDisallowed", false},
		{"PayloadContent", cuentas},
	}
	return encodePlist(root)
}

// descripcionPerfil es lo que el sistema enseña antes de instalar. Las cuentas entran
// con una contraseña de APLICACIÓN —el IMAP rechaza la de la cuenta—: con ella dentro no
// se pide nada; sin ella, la que pide el sistema es la que se crea en Mi cuenta.
func descripcionPerfil(conClave bool) string {
	if conClave {
		return "Configura Correo, Calendario y Contactos de Sytue en este dispositivo. Lleva su propia contraseña de aplicación, que puedes revocar en Mi cuenta."
	}
	return "Configura Correo, Calendario y Contactos de Sytue en este dispositivo. Se te pedirá una contraseña de aplicación: créala en Mi cuenta."
}

// derDelPEM saca el primer certificado de un PEM. Devuelve nil si no hay ninguno
// —sin autoridad configurada, o con un fichero que no es un certificado—, y
// entonces el perfil sale con sus tres cuentas y sin carga de certificado, que es
// exactamente como salía antes.
//
// Se recorre el PEM en vez de decodificar solo el primer bloque: un fichero de
// cadena puede empezar por parámetros o por una clave, y quedarse con el primer
// bloque metería eso en el perfil.
func derDelPEM(texto string) []byte {
	resto := []byte(texto)
	for {
		bloque, siguiente := pem.Decode(resto)
		if bloque == nil {
			return nil
		}
		if bloque.Type == "CERTIFICATE" {
			return bloque.Bytes
		}
		resto = siguiente
	}
}

// --- plist XML mínimo sobre encoding/xml ---
//
// ponytail: un plist de perfil solo necesita dict/array/string/integer/bool y el
// ORDEN de las claves; no hace falta una librería (ni existe en stdlib). Si algún
// día hiciera falta <data>/<date>, se añaden aquí dos casos más.

type plistKV struct {
	Key   string
	Value any // string | int | bool | plistData | plistDict | []plistDict
}

// plistData es un valor binario del plist (`<data>`, en base64). Se distingue de
// `string` por el tipo y no por el contenido: un certificado en base64 ES una
// cadena, y sin un tipo propio saldría como `<string>` y el perfil no instalaría
// nada.
type plistData []byte

type plistDict []plistKV

const plistDoctype = `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">`

func encodePlist(root plistDict) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(plistDoctype + "\n")

	enc := xml.NewEncoder(&buf)
	enc.Indent("", "\t")
	start := xml.StartElement{
		Name: xml.Name{Local: "plist"},
		Attr: []xml.Attr{{Name: xml.Name{Local: "version"}, Value: "1.0"}},
	}
	if err := enc.EncodeToken(start); err != nil {
		return nil, err
	}
	if err := encodePlistValue(enc, root); err != nil {
		return nil, err
	}
	if err := enc.EncodeToken(start.End()); err != nil {
		return nil, err
	}
	if err := enc.Flush(); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	// encoding/xml no emite elementos vacíos autocerrados; los booleanos de un
	// plist se escriben siempre <true/>/<false/> (forma canónica de Apple).
	return []byte(boolTagsCanonical.Replace(buf.String())), nil
}

var boolTagsCanonical = strings.NewReplacer("<true></true>", "<true/>", "<false></false>", "<false/>")

func encodePlistValue(enc *xml.Encoder, v any) error {
	el := func(local string) xml.StartElement {
		return xml.StartElement{Name: xml.Name{Local: local}}
	}
	switch t := v.(type) {
	case string:
		return enc.EncodeElement(t, el("string"))
	case plistData:
		// `<data>` con el contenido en base64. Es lo que pide un certificado, y
		// el único tipo que faltaba: el perfil llevaba tres cuentas y ningún
		// binario. Apple acepta el base64 en una sola línea.
		return enc.EncodeElement(base64.StdEncoding.EncodeToString(t), el("data"))
	case int:
		return enc.EncodeElement(t, el("integer"))
	case bool:
		tag := el("false")
		if t {
			tag = el("true")
		}
		if err := enc.EncodeToken(tag); err != nil {
			return err
		}
		return enc.EncodeToken(tag.End())
	case plistDict:
		start := el("dict")
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		for _, kv := range t {
			if err := enc.EncodeElement(kv.Key, el("key")); err != nil {
				return err
			}
			if err := encodePlistValue(enc, kv.Value); err != nil {
				return err
			}
		}
		return enc.EncodeToken(start.End())
	case []plistDict:
		start := el("array")
		if err := enc.EncodeToken(start); err != nil {
			return err
		}
		for _, d := range t {
			if err := encodePlistValue(enc, d); err != nil {
				return err
			}
		}
		return enc.EncodeToken(start.End())
	default:
		return fmt.Errorf("plist: tipo no soportado %T", v)
	}
}

// --- helpers de coordenadas ---

// splitAddr parte "host:puerto" (el puerto es opcional: cae en def).
func splitAddr(addr string, def int) (string, int, error) {
	if addr == "" {
		return "", 0, fmt.Errorf("vacío")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// Sin puerto explícito ("imap.sytue.eu"): NO es un error de configuración,
		// es la forma corta. Se toma el puerto por defecto del servicio.
		//nolint:nilerr // el fallo de SplitHostPort ES el caso «solo host», no un error a propagar.
		return addr, def, nil
	}
	n, err := strconv.Atoi(port)
	if err != nil || n <= 0 || n > 65535 {
		return "", 0, fmt.Errorf("puerto no válido: %q", port)
	}
	if host == "" {
		return "", 0, fmt.Errorf("falta el host en %q", addr)
	}
	return host, n, nil
}

// splitDAVURL descompone la URL del principal DAV en host, puerto, ruta y TLS.
func splitDAVURL(raw string) (host string, port int, path string, ssl bool, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", 0, "", false, err
	}
	if u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", 0, "", false, fmt.Errorf("URL DAV no válida: %q", raw)
	}
	ssl = u.Scheme == "https"
	port = 443
	if !ssl {
		port = 80
	}
	if p := u.Port(); p != "" {
		n, cerr := strconv.Atoi(p)
		if cerr != nil || n <= 0 || n > 65535 {
			return "", 0, "", false, fmt.Errorf("puerto no válido en %q", raw)
		}
		port = n
	}
	path = u.Path
	if path == "" {
		path = "/"
	}
	return u.Hostname(), port, path, ssl, nil
}

// reverseDomain convierte sytue.eu en eu.sytue (identificadores DNS invertidos
// de Apple).
func reverseDomain(d string) string {
	parts := strings.Split(d, ".")
	slices.Reverse(parts)
	return strings.Join(parts, ".")
}
