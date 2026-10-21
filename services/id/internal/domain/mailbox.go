// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"strconv"
	"strings"
)

// Dirección de correo por defecto de quien entra en una organización. El patrón es
// nombre.apellidos@dominio-de-la-organización, y todo lo interesante está en qué
// hacer cuando el nombre real no se deja meter en ese molde.
//
// La regla es que la dirección sea PREVISIBLE: quien conoce a la persona debe poder
// escribirle sin consultar el directorio. Por eso las tildes se pliegan a su letra
// base (nadie teclea Pérez con acento al escribir un correo), los espacios se hacen
// punto en vez de desaparecer —maria.del.carmen.perez se lee, mariadelcarmenperez
// no— y los apellidos compuestos con guion lo conservan, porque el guion es parte
// del apellido y sí se teclea. Todo lo demás (apóstrofos, puntos ya presentes,
// signos) se cae: son ruido que nadie reproduce igual dos veces.

// pliegue traduce las letras acentuadas y ligaduras de los idiomas de la suite a
// ASCII. Se hace a mano en vez de arrastrar golang.org/x/text por el mismo motivo
// que los nombres de mes en internal/i18n: es media docena de líneas contra una
// dependencia entera. La ß alemana da dos letras y las ligaduras también, así que
// no vale un mapa de rune a rune.
var pliegue = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a", "ą", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e", "ę", "e", "ě", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o", "ø", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u", "ů", "u",
	"ý", "y", "ÿ", "y",
	"ñ", "n", "ń", "n", "ç", "c", "ć", "c", "č", "c",
	"š", "s", "ś", "s", "ž", "z", "ź", "z", "ż", "z", "ř", "r", "ł", "l",
	"ď", "d", "ť", "t", "ğ", "g", "ı", "i",
	"ß", "ss", "æ", "ae", "œ", "oe", "ð", "d", "đ", "d", "þ", "th",
)

// maxLocalPart es el límite del local-part en RFC 5321 §4.5.3.1.1 (64 octetos).
// Se recorta ahí y no en un número redondo nuestro porque un servidor ajeno puede
// rechazar la dirección entera si se pasa, y el rebote llegaría meses después.
const maxLocalPart = 64

// MailboxLocalPart deriva la parte local del patrón nombre.apellidos a partir del
// perfil. Cae en cascada hacia lo que haya: nombre y apellidos por separado si el
// alta los trajo (SCIM y el registro los rellenan), el nombre completo si solo hay
// eso, y como último recurso la parte local del correo con el que se registró.
//
// Devuelve cadena vacía si no queda nada utilizable —un nombre escrito íntegro en
// un alfabeto que el pliegue no cubre se queda en nada—; decidir qué hacer con eso
// es del llamante, que es quien sabe si puede inventarse una dirección o debe
// dejar a la persona sin buzón.
func MailboxLocalPart(givenName, familyName, fullName, email string) string {
	if local := normalizarLocal(givenName + " " + familyName); local != "" {
		return local
	}
	if local := normalizarLocal(fullName); local != "" {
		return local
	}
	if i := strings.IndexByte(email, '@'); i > 0 {
		return normalizarLocal(email[:i])
	}
	return ""
}

// normalizarLocal pliega, baja a minúsculas y deja solo lo que un local-part puede
// llevar sin sorpresas: letras, dígitos, punto y guion. Los espacios se convierten
// en punto ANTES de filtrar para que la separación entre nombre y apellidos
// sobreviva; los separadores repetidos se colapsan y no pueden quedar en los
// extremos, porque un local-part que empieza o acaba en punto es inválido
// (RFC 5322 §3.4.1) y algunos servidores lo rechazan de plano.
func normalizarLocal(s string) string {
	s = pliegue.Replace(strings.ToLower(strings.TrimSpace(s)))

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '\t' || r == '.' || r == '_':
			// El punto que ya venía en el nombre se trata como separador, no se
			// duplica: «J. Pérez» y «J Pérez» tienen que dar la misma dirección.
			b.WriteByte('.')
		}
	}

	// Colapsar separadores consecutivos: «Ana  María» o «Ana . María» no pueden
	// producir puntos dobles, que además son inválidos sin comillas.
	out := b.String()
	for strings.Contains(out, "..") {
		out = strings.ReplaceAll(out, "..", ".")
	}
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, ".-")
	if len(out) > maxLocalPart {
		out = strings.Trim(out[:maxLocalPart], ".-")
	}
	return out
}

// MailboxCandidate compone el intento n-ésimo de dirección para un local-part y un
// dominio. n=0 es la dirección limpia; a partir de ahí se sufija con el número
// (ana.perez2@, ana.perez3@…) porque es lo que la gente espera y lo que puede
// adivinar: pegar el número al final deja claro que es un desempate, mientras que
// ana.perez.2 se leería como un tercer apellido. El sufijo se cuenta dentro del
// límite de 64 octetos recortando la raíz, nunca dejándola pasarse.
func MailboxCandidate(localPart, mailDomain string, n int) string {
	return candidatoLocal(localPart, n) + "@" + mailDomain
}

// candidatoLocal es el desempate de MailboxCandidate sin el dominio: lo comparten
// las direcciones y el @usuario, que viven en el mismo espacio de nombres.
func candidatoLocal(localPart string, n int) string {
	if n > 0 {
		sufijo := strconv.Itoa(n + 1)
		if len(localPart)+len(sufijo) > maxLocalPart {
			localPart = strings.Trim(localPart[:maxLocalPart-len(sufijo)], ".-")
		}
		localPart += sufijo
	}
	return localPart
}

// NormalizeMailboxAddress valida y normaliza una dirección escrita a mano (la que
// personaliza quien invita, o un alias). Devuelve la dirección en minúsculas o
// ErrInvalidInput. Acepta la dirección completa o solo el local-part, en cuyo caso
// le pone el dominio de la organización: en la consola se teclea la parte de la
// izquierda y el dominio se enseña al lado, así que exigir el arroba sería pedirle
// al administrador que repita lo que ya está en pantalla.
func NormalizeMailboxAddress(entrada, mailDomain string) (string, error) {
	entrada = strings.ToLower(strings.TrimSpace(entrada))
	if entrada == "" {
		return "", ErrInvalidInput
	}
	local, dom := entrada, mailDomain
	if i := strings.LastIndexByte(entrada, '@'); i >= 0 {
		local, dom = entrada[:i], entrada[i+1:]
	}
	// El local-part se pasa por la misma normalización que el patrón por defecto en
	// vez de aceptarse tal cual: si se admitiera crudo, una dirección tecleada con
	// una tilde o un espacio entraría en la base y solo fallaría al recibir el primer
	// correo, cuando ya nadie relaciona el rebote con el alta.
	local = normalizarLocal(local)
	if local == "" || dom == "" || !strings.Contains(dom, ".") {
		return "", ErrInvalidInput
	}
	if len(local) > maxLocalPart {
		return "", ErrInvalidInput
	}
	return local + "@" + dom, nil
}

// OrgMailbox es una dirección que entrega en el buzón de una persona dentro de una
// organización. Primary distingue la dirección principal de sus alias: en la base
// son la misma fila porque comparten la regla de que una dirección entrega en un
// solo sitio (ver la migración 0048).
type OrgMailbox struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id"`
	Address string `json:"address"`
	Primary bool   `json:"primary"`
}

// OrgMailDomain decide en qué dominio vive el correo de una organización: el suyo
// propio si lo tiene VERIFICADO, y si no el subdominio que la suite le da por su
// slug. Solo cuenta el dominio verificado porque reclamarlo no prueba nada —
// cualquiera escribe «acme.com» en el asistente de alta— y repartir buzones en un
// dominio ajeno crearía direcciones que nuestro MX no sirve y que además pisarían
// las de su dueño real. Mientras no se verifique, la organización tiene correo que
// funciona en su subdominio; al verificar, los buzones nuevos ya nacen en el suyo.
func OrgMailDomain(orgSlug, ownDomain string, verified bool, suiteDomain string) string {
	if verified && ownDomain != "" {
		return strings.ToLower(ownDomain)
	}
	return OrgBaseMailDomain(orgSlug, suiteDomain)
}

// OrgBaseMailDomain es el subdominio que la suite da a una organización por su
// slug. No es «el dominio de las que no tienen el suyo»: es el CIMIENTO, y existe
// siempre, también cuando hay dominio propio verificado.
//
// Que exista siempre es lo que hace que el cableado interno no dependa de una
// decisión comercial del cliente. Todo lo que en la suite necesita una dirección
// —el correo a canal de Rooms, las notificaciones de Drive, el remitente de un
// documento compartido— resuelve contra este dominio, que es nuestro y que
// nuestro MX sirve. Si esas piezas colgaran del dominio propio, el día que un
// cliente deja caducar su dominio, o mueve el MX a otro proveedor, la mitad de la
// suite se queda sin poder entregar nada y el fallo no aparece hasta que alguien
// echa de menos un aviso que nunca llegó.
//
// Por eso la dirección en este dominio se conserva como alias cuando la principal
// se muda al dominio propio, en vez de sustituirse.
func OrgBaseMailDomain(orgSlug, suiteDomain string) string {
	if suiteDomain == "" || orgSlug == "" {
		return ""
	}
	// Una cuenta personal no tiene subdominio: su organización es el DOMINIO
	// PADRE de la suite (nombre@sytue.eu), que se administra en platform.<dominio>
	// y es el único que presta correo a quien no tiene organización (ADR 0038).
	// Un subdominio `p-3f9a…` sería una dirección que nadie sabría teclear.
	if strings.HasPrefix(orgSlug, PersonalSlugPrefix) {
		return strings.ToLower(suiteDomain)
	}
	return strings.ToLower(orgSlug + "." + suiteDomain)
}

// DireccionesDeAgenda decide qué correos de una persona salen en la agenda de su
// organización: aquellos en los que SE RECIBE DENTRO DE SYTUE, y ninguno más.
//
// NO ES EL CORREO CON EL QUE ENTRA. A quien se invita desde fuera, su `Email` es
// la dirección a la que se le mandó la invitación —su Gmail, el correo de su
// empresa anterior—. Publicarla en la agenda de la organización es dos cosas
// malas a la vez: enseña el correo personal de la gente a toda la empresa, y no
// sirve para lo que la agenda existe, porque escribir ahí sale de Sytue y entra
// en el buzón de otro proveedor.
//
// Lo que sí vale son sus BUZONES: la principal y sus alias, que por construcción
// viven en el subdominio de la organización o en su dominio propio verificado
// (ver `OrgMailDomain`). Son exactamente las direcciones que Sytue Mail entrega.
//
// LA EXCEPCIÓN, y es la que pidió el propietario: si el correo de invitación está
// en un dominio que la organización ha VERIFICADO en la consola, deja de ser «de
// fuera» —lo sirve su MX, que es el nuestro desde que se verificó— y entra. Es el
// caso de quien ya tenía cuenta en el dominio de su empresa antes de entrar aquí;
// quitarla escondería la dirección por la que todo el mundo le escribe.
//
// La principal va primero, y no se repite ninguna aunque venga por los dos lados.
func DireccionesDeAgenda(buzones []OrgMailbox, correoDeLogin string, dominiosVerificados []string) []string {
	out := make([]string, 0, len(buzones)+1)
	vistos := make(map[string]bool, len(buzones)+1)
	// Los buzones llegan con la principal delante (el ORDER BY del almacén), pero
	// no se confía en eso: si alguna vez cambia, la agenda seguiría bien.
	for _, m := range buzones {
		if !m.Primary {
			continue
		}
		if l := strings.ToLower(m.Address); m.Address != "" && !vistos[l] {
			vistos[l] = true
			out = append(out, m.Address)
		}
	}
	for _, m := range buzones {
		if l := strings.ToLower(m.Address); m.Address != "" && !vistos[l] {
			vistos[l] = true
			out = append(out, m.Address)
		}
	}
	if l := strings.ToLower(correoDeLogin); l != "" && !vistos[l] && enDominio(l, dominiosVerificados) {
		out = append(out, correoDeLogin)
	}
	return out
}

// enDominio: ¿está esta dirección en alguno de estos dominios? Compara el host
// entero, no un sufijo: con `strings.HasSuffix`, «acme.com» dejaría entrar
// «noacme.com» y «acme.com.attacker.net».
func enDominio(correo string, dominios []string) bool {
	i := strings.LastIndex(correo, "@")
	if i < 0 {
		return false
	}
	host := strings.ToLower(correo[i+1:])
	for _, d := range dominios {
		if strings.EqualFold(strings.TrimSpace(d), host) {
			return true
		}
	}
	return false
}
