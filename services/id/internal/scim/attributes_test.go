// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Tests unitarios (sin BD) del mapeo de atributos de perfil de SCIM 2.0 a las columnas
// que evalúan las reglas de grupos dinámicos: extensión enterprise (department), núcleo
// (title, addresses[work].locality, displayName), las dos formas de PATCH que usan los
// proveedores reales, y el round-trip de la representación de salida.
package scim

import (
	"encoding/json"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
)

// decodeUser parsea un payload SCIM tal cual lo mandaría el proveedor.
func decodeUser(t *testing.T, body string) scimUser {
	t.Helper()
	var in scimUser
	if err := json.Unmarshal([]byte(body), &in); err != nil {
		t.Fatalf("payload SCIM inválido: %v", err)
	}
	return in
}

// TestApplySCIMAttrs_EnterpriseExtension cubre el payload REAL de Entra/Okta: department
// viaja en la extensión enterprise y title en el núcleo; la ciudad de trabajo alimenta
// location.
func TestApplySCIMAttrs_EnterpriseExtension(t *testing.T) {
	in := decodeUser(t, `{
		"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User",
		            "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"],
		"userName": "ada@acme.eu",
		"displayName": "Ada Lovelace",
		"name": {"givenName": "Ada", "familyName": "Lovelace"},
		"title": "Head of Sales",
		"addresses": [
			{"type": "home", "locality": "Lisboa"},
			{"type": "work", "locality": "Madrid", "primary": true}
		],
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": {
			"employeeNumber": "42", "costCenter": "CC-1", "department": "Sales",
			"manager": {"value": "mgr-id"}
		}
	}`)
	var u domain.User
	applySCIMAttrs(&u, in)
	if u.Department != "Sales" {
		t.Errorf("department = %q; quería Sales (extensión enterprise)", u.Department)
	}
	if u.Title != "Head of Sales" {
		t.Errorf("title = %q; quería 'Head of Sales' (núcleo)", u.Title)
	}
	if u.Location != "Madrid" {
		t.Errorf("location = %q; quería Madrid (addresses[type eq \"work\"].locality)", u.Location)
	}
	// name.formatted ausente ⇒ displayName (lo que manda Entra por defecto).
	if u.Name != "Ada Lovelace" || u.GivenName != "Ada" || u.FamilyName != "Lovelace" {
		t.Errorf("nombre = (%q,%q,%q)", u.Name, u.GivenName, u.FamilyName)
	}
}

// TestApplySCIMAttrs_Reemplaza comprueba la semántica de PUT (§3.5.1): lo que el
// directorio ya NO manda se BORRA — es lo que saca al usuario del grupo que ya no le toca.
func TestApplySCIMAttrs_Reemplaza(t *testing.T) {
	u := domain.User{Department: "Sales", Title: "SDR", Location: "Madrid", Name: "Ada"}
	applySCIMAttrs(&u, decodeUser(t, `{"userName":"ada@acme.eu","displayName":"Ada"}`))
	if u.Department != "" || u.Title != "" || u.Location != "" {
		t.Errorf("PUT sin atributos debía vaciarlos: dept=%q title=%q loc=%q", u.Department, u.Title, u.Location)
	}
}

// TestWorkLocality cubre el orden de preferencia sin dirección de trabajo declarada.
func TestWorkLocality(t *testing.T) {
	cases := map[string]struct {
		addrs []scimAddress
		want  string
	}{
		"work gana": {[]scimAddress{{Type: "home", Locality: "Lisboa"}, {Type: "work", Locality: "Madrid"}}, "Madrid"},
		"primaria":  {[]scimAddress{{Locality: "Lisboa"}, {Locality: "Madrid", Primary: true}}, "Madrid"},
		"primera":   {[]scimAddress{{Locality: "Lisboa"}, {Locality: "Madrid"}}, "Lisboa"},
		"vacías":    {[]scimAddress{{Type: "work"}}, ""},
		"ninguna":   {nil, ""},
	}
	for name, c := range cases {
		if got := workLocality(c.addrs); got != c.want {
			t.Errorf("%s: workLocality = %q; quería %q", name, got, c.want)
		}
	}
}

func TestNormalizeSCIMPath(t *testing.T) {
	cases := map[string]string{
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department": "department",
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User.department": "department",
		"urn:ietf:params:scim:schemas:core:2.0:User:userName":                   "username",
		`addresses[type eq "work"].locality`:                                    "addresses.locality",
		"name.formatted":                                                        "name.formatted",
		"ACTIVE":                                                                "active",
		"":                                                                      "",
	}
	for in, want := range cases {
		if got := normalizeSCIMPath(in); got != want {
			t.Errorf("normalizeSCIMPath(%q) = %q; quería %q", in, got, want)
		}
	}
}

// TestApplySCIMPatchOp cubre las dos formas de PATCH que llegan de proveedores reales:
// con path URN-cualificado (Entra) y con objeto de atributos sin path (Okta), más el
// remove que vacía el atributo.
func TestApplySCIMPatchOp(t *testing.T) {
	// Entra: una operación por atributo, con el path completo.
	u := domain.User{Department: "Sales", Title: "SDR", Location: "Madrid"}
	applySCIMPatchOp(&u, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department", "Engineering")
	applySCIMPatchOp(&u, "title", "Staff Engineer")
	applySCIMPatchOp(&u, `addresses[type eq "work"].locality`, "Lisboa")
	if u.Department != "Engineering" || u.Title != "Staff Engineer" || u.Location != "Lisboa" {
		t.Fatalf("PATCH con path: dept=%q title=%q loc=%q", u.Department, u.Title, u.Location)
	}

	// Okta: un único objeto de atributos sin path, con la extensión anidada bajo su URN.
	u = domain.User{}
	applySCIMPatchOp(&u, "", map[string]any{
		"active":      false,
		"displayName": "Ada L.",
		"title":       "CTO",
		"addresses":   []any{map[string]any{"type": "work", "locality": "Madrid"}},
		"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User": map[string]any{
			"department": "Engineering",
		},
	})
	if u.Department != "Engineering" || u.Title != "CTO" || u.Location != "Madrid" {
		t.Fatalf("PATCH sin path: dept=%q title=%q loc=%q", u.Department, u.Title, u.Location)
	}
	if u.Name != "Ada L." || u.Status != domain.StatusSuspended {
		t.Fatalf("PATCH sin path: name=%q status=%q", u.Name, u.Status)
	}

	// remove: llega sin value ⇒ el atributo queda vacío (y el usuario sale del grupo).
	applySCIMPatchOp(&u, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department", nil)
	if u.Department != "" {
		t.Errorf("remove de department dejó %q", u.Department)
	}

	// Un atributo no soportado se ignora en silencio, sin tocar nada más.
	before := u
	applySCIMPatchOp(&u, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:costCenter", "CC-9")
	if u != before {
		t.Errorf("un atributo no soportado no debe cambiar el usuario: %+v", u)
	}
	// userName vacío no debe borrar el email (dejaría el usuario sin identificador).
	u.Email = "ada@acme.eu"
	applySCIMPatchOp(&u, "userName", "")
	if u.Email != "ada@acme.eu" {
		t.Errorf("email = %q; un value vacío no debe borrarlo", u.Email)
	}
}

// TestToSCIMRoundTrip comprueba que lo que devolvemos vuelve a entrar igual: un cliente
// que lee-modifica-escribe (GET → PUT) no pierde department/title/location.
func TestToSCIMRoundTrip(t *testing.T) {
	h := &Handler{issuer: "https://id.test"}
	out := h.toSCIM(domain.User{
		ID: "u1", Email: "ada@acme.eu", Name: "Ada", Status: domain.StatusActive,
		Department: "Sales", Title: "SDR", Location: "Madrid",
	})
	raw, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	ext, _ := wire[schemaEnterpriseUser].(map[string]any)
	if ext == nil || ext["department"] != "Sales" {
		t.Fatalf("la extensión enterprise no viaja en la respuesta: %s", raw)
	}
	schemas, _ := wire["schemas"].([]any)
	if len(schemas) != 2 || schemas[1] != schemaEnterpriseUser {
		t.Errorf("schemas debe declarar la extensión (§3.3): %v", schemas)
	}
	var back domain.User
	applySCIMAttrs(&back, decodeUser(t, string(raw)))
	if back.Department != "Sales" || back.Title != "SDR" || back.Location != "Madrid" {
		t.Errorf("round-trip perdió atributos: %+v", back)
	}
}
