// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package integrationshttp

// Tests OFFLINE del emparejamiento del asistente automático: en qué cuenta de
// Sytue aterriza cada cuenta del origen. Es la decisión con menos margen de error
// de todo el asistente —equivocarse deja el contenido de alguien en el buzón de
// otra persona—, así que se prueba aparte del plumbing HTTP.

import (
	"testing"

	"git.sytue.eu/sytue/one/services/integrations/internal/directory"
)

func TestMatchByEmail(t *testing.T) {
	t.Parallel()
	users := []directory.User{
		{ID: "1", Email: "ana@acme.eu", Name: "Ana"},
		{ID: "2", Email: "luis@acme.eu", Name: "Luis"},
		{ID: "3", Email: "ana.b@acme.eu", Name: "Ana B"},
	}

	cases := []struct {
		name  string
		email string
		want  string // id esperado; "" = ninguno
	}{
		{"correo idéntico", "luis@acme.eu", "2"},
		{"mayúsculas y espacios", "  Luis@ACME.eu ", "2"},
		{"mismo usuario, otro dominio", "ana@acme.com", "1"},
		{"nadie con ese nombre", "pepe@acme.com", ""},
		{"sin arroba", "ana", ""},
		{"vacío", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := matchByEmail(users, tc.email)
			switch {
			case tc.want == "" && got != nil:
				t.Fatalf("emparejó con %q y no debía", got.Email)
			case tc.want != "" && (got == nil || got.ID != tc.want):
				t.Fatalf("emparejó = %v, quiero el usuario %s", got, tc.want)
			}
		})
	}
}

// TestMatchByEmailAmbiguoNoElige: si dos personas comparten la parte local en
// dominios distintos, NO se elige ninguna. Preferimos que el asistente diga «esta
// cuenta no la sé emparejar» a copiar el Drive de alguien en el sitio equivocado.
func TestMatchByEmailAmbiguoNoElige(t *testing.T) {
	t.Parallel()
	users := []directory.User{
		{ID: "1", Email: "ana@acme.eu"},
		{ID: "2", Email: "ana@filial.eu"},
	}
	if got := matchByEmail(users, "ana@otra.com"); got != nil {
		t.Fatalf("eligió %q con dos candidatas igual de válidas", got.Email)
	}
}

// TestSourceForTemplate: solo las plantillas que de verdad son un origen de
// migración lo son. Una plantilla cualquiera del catálogo no se cuela.
func TestSourceForTemplate(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"google-workspace", "github", "microsoft365"} {
		if _, ok := sourceForTemplate(key); !ok {
			t.Errorf("%q debería ser un origen de migración", key)
		}
	}
	for _, key := range []string{"slack", "github-enterprise", "", "notion"} {
		if src, ok := sourceForTemplate(key); ok {
			t.Errorf("%q no es un origen de migración, pero devolvió %q", key, src)
		}
	}
}

// TestSectionOr: la vuelta del consentimiento acaba en una cabecera Location, así
// que solo se aceptan secciones conocidas. Un destino inventado (o un intento de
// redirección abierta) cae en el catálogo de apps, nunca fuera de la consola.
func TestSectionOr(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"migrations":                        "migrations",
		"storage":                           "storage",
		"rooms":                             "rooms",
		"rooms?next=https://evil.example":   "apps",
		"storage?next=https://evil.example": "apps",
		"apps":                              "apps",
		"":                                  "apps",
		"../../etc":                         "apps",
		"https://evil.example":              "apps",
		"//evil.example/phish":              "apps",
		"migrations?x=1":                    "apps",
		"MIGRATIONS":                        "apps",
	}
	for in, want := range cases {
		if got := sectionOr(in); got != want {
			t.Errorf("sectionOr(%q) = %q, quiero %q", in, got, want)
		}
	}
}

// TestLaParteLocalNoCruzaCuentasAjenas (NEG-19): dos cuentas del origen con la
// misma parte local no acaban en la misma persona, y la cuenta que autorizó una
// migración de una sola cuenta solo casa por la parte local si es de quien lanza.
func TestLaParteLocalNoCruzaCuentasAjenas(t *testing.T) {
	t.Parallel()
	users := []directory.User{{ID: "1", Email: "ana@acme.eu"}, {ID: "2", Email: "luis@acme.eu"}}

	people, unmatched := matchSources(users, []string{"ana@acme.com", "ana@filial.com", "luis@acme.com"})
	if len(people) != 1 || people[0].SourceUser != "luis@acme.com" || len(unmatched) != 2 {
		t.Fatalf("emparejadas %+v, sin emparejar %v", people, unmatched)
	}
	if people, _ := matchSources(users, []string{"ana@acme.eu", "ana@filial.com"}); len(people) != 1 || people[0].SourceUser != "ana@acme.eu" {
		t.Fatalf("el correo exacto sigue casando: %+v", people)
	}

	ana := &users[0]
	if cuentaConectada(ana, "ana@gmail.com", "luis@acme.eu") != nil {
		t.Fatal("la cuenta de otra persona, por la parte local, no")
	}
	if cuentaConectada(ana, "ana@gmail.com", "ana@acme.eu") != ana {
		t.Fatal("la cuenta antigua de quien lanza, sí")
	}
	if cuentaConectada(ana, "ANA@acme.eu", "luis@acme.eu") != ana {
		t.Fatal("el correo exacto, siempre")
	}
}
