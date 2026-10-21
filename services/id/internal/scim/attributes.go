// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package scim

import (
	"encoding/json"
	"strings"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// Mapeo de los atributos de PERFIL que llegan por SCIM a las columnas que evalúan las
// REGLAS de los grupos dinámicos (mig 0021: users.department/title/location). Sin esto,
// una organización sincronizada desde su directorio corporativo (Entra ID, Okta, Google)
// tenía esos datos SOLO en el payload y sus grupos dinámicos quedaban vacíos salvo que un
// admin rellenara los atributos a mano por la API de perfil.
//
// QUÉ SE MAPEA — y de dónde sale en el esquema SCIM 2.0:
//
//	users.title      ← title, del NÚCLEO (RFC 7643 §4.1.2). OJO: title NO está en la
//	                   extensión enterprise, es un atributo del User de toda la vida.
//	users.department ← department, de la EXTENSIÓN ENTERPRISE (§4.3,
//	                   urn:ietf:params:scim:schemas:extension:enterprise:2.0:User) — que
//	                   es donde Entra/Okta/Google lo mandan.
//	users.location   ← addresses[type eq "work"].locality, del NÚCLEO (§4.1.2). SCIM no
//	                   tiene un atributo «location»; la ciudad de la dirección de trabajo
//	                   es lo que Entra y Okta mapean desde `city`, y es el equivalente
//	                   razonable. Si ninguna dirección declara type=work se usa la
//	                   primaria, y si tampoco, la primera con locality.
//	users.name       ← name.formatted, o displayName si falta. `name` TAMBIÉN es atributo
//	                   de regla (domain.AttrName): sin esto una regla sobre el nombre no
//	                   veía los cambios del directorio, y un PUT de Entra (que manda
//	                   displayName + givenName/familyName, sin formatted) lo VACIABA.
//
// QUÉ NO SE MAPEA: el resto de la extensión enterprise (employeeNumber, costCenter,
// organization, division, manager) no tiene columna en users ni atributo de regla en
// domain.RuleAttributes — persistirlo sería estado muerto. Se ignora en silencio, que es
// lo que la especificación pide para atributos no soportados. Añadir uno exige migración
// + entrada en RuleAttributes + soporte en el constructor de reglas de la Console.

const schemaEnterpriseUser = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"

// scimEnterprise es la extensión enterprise (RFC 7643 §4.3). Solo declara lo que
// PERSISTIMOS; el resto del objeto se descarta al decodificar.
type scimEnterprise struct {
	Department string `json:"department,omitempty"`
}

// scimAddress es una dirección del núcleo (§4.1.2). De ella solo persistimos la localidad
// de la dirección de trabajo (→ users.location); los demás subatributos ni se guardan ni
// se devuelven.
type scimAddress struct {
	Type     string `json:"type,omitempty"`
	Locality string `json:"locality,omitempty"`
	Primary  bool   `json:"primary,omitempty"`
}

// workLocality devuelve la ciudad de la dirección de TRABAJO: la de type "work"; si
// ninguna lo declara, la primaria; si tampoco, la primera con localidad.
func workLocality(addrs []scimAddress) string {
	fallback := ""
	for _, a := range addrs {
		loc := strings.TrimSpace(a.Locality)
		if loc == "" {
			continue
		}
		if strings.EqualFold(a.Type, "work") {
			return loc
		}
		if fallback == "" || a.Primary {
			fallback = loc
		}
	}
	return fallback
}

// applySCIMAttrs vuelca sobre u los atributos de perfil de un payload COMPLETO de usuario
// (POST /Users y PUT /Users). Semántica de la especificación para PUT (§3.5.1): el
// recurso se REEMPLAZA, de modo que un department que el directorio ya no manda se BORRA
// — es justo lo que saca al usuario del grupo dinámico que ya no le toca. El directorio
// es la fuente de verdad de estos tres atributos cuando hay SCIM.
func applySCIMAttrs(u *domain.User, in scimUser) {
	if in.Name != nil {
		u.GivenName = in.Name.GivenName
		u.FamilyName = in.Name.FamilyName
	}
	if in.Name != nil && strings.TrimSpace(in.Name.Formatted) != "" {
		u.Name = in.Name.Formatted
	} else if strings.TrimSpace(in.DisplayName) != "" {
		u.Name = in.DisplayName
	}
	u.Title = strings.TrimSpace(in.Title)
	u.Department = ""
	if in.Enterprise != nil {
		u.Department = strings.TrimSpace(in.Enterprise.Department)
	}
	u.Location = workLocality(in.Addresses)
}

// applySCIMPatchOp aplica UNA operación PATCH (RFC 7644 §3.5.2) sobre u. Acepta las DOS
// formas que usan los proveedores reales:
//
//   - con path: {"op":"replace","path":"urn:…:enterprise:2.0:User:department","value":"Ventas"}
//     — incluido el filtro de atributo multivaluado addresses[type eq "work"].locality.
//   - sin path y con un objeto de atributos: {"op":"replace","value":{"active":false}} —
//     la forma que usa Okta, con la extensión enterprise anidada bajo su URN.
//
// `remove` llega con value nil ⇒ el atributo queda vacío (y el usuario sale del grupo que
// dependía de él). Un atributo no soportado se ignora en silencio (§3.5.2).
func applySCIMPatchOp(u *domain.User, path string, value any) {
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			applySCIMPatchOp(u, joinSCIMPath(path, k), v)
		}
		return
	}
	switch normalizeSCIMPath(path) {
	case "active":
		if b, ok := value.(bool); ok {
			u.Status = activeToStatus(b)
		}
	case "username":
		if s := patchString(value); s != "" {
			u.Email = s
		}
	case "displayname", "name.formatted":
		u.Name = patchString(value)
	case "name.givenname":
		u.GivenName = patchString(value)
	case "name.familyname":
		u.FamilyName = patchString(value)
	case "title":
		u.Title = patchString(value)
	case "department":
		u.Department = patchString(value)
	case "addresses.locality":
		u.Location = patchString(value)
	case "addresses":
		// La lista entera (forma «value objeto» de Okta, o un add de addresses).
		u.Location = workLocality(decodeAddresses(value))
	}
}

// normalizeSCIMPath reduce un path SCIM a la clave que entiende applySCIMPatchOp:
// minúsculas, sin el filtro del atributo multivaluado y sin el prefijo URN del esquema
// (que la extensión enterprise SIEMPRE lleva, con ':' o '.' de separador según cliente).
func normalizeSCIMPath(p string) string {
	s := strings.ToLower(strings.TrimSpace(p))
	if i := strings.IndexByte(s, '['); i >= 0 {
		if j := strings.IndexByte(s[i:], ']'); j >= 0 {
			s = s[:i] + s[i+j+1:]
		}
	}
	const urnMark = ":2.0:user"
	if i := strings.LastIndex(s, urnMark); i >= 0 {
		s = strings.TrimLeft(s[i+len(urnMark):], ":.")
	}
	return strings.Trim(s, ".")
}

func joinSCIMPath(prefix, attr string) string {
	if prefix == "" {
		return attr
	}
	return prefix + "." + attr
}

// patchString normaliza el valor escalar de una operación PATCH ("" si no es texto o si
// la operación es un remove, que viene sin value).
func patchString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// decodeAddresses reinterpreta el valor genérico de una operación PATCH como la lista de
// direcciones del núcleo (round-trip JSON: el cuerpo ya se validó al decodificar).
func decodeAddresses(v any) []scimAddress {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out []scimAddress
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil
	}
	return out
}
