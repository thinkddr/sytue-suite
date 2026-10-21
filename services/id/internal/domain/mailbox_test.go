// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import "testing"

// Lo que se prueba aquí es lo que rompe en silencio: un nombre con tilde o con
// apellido compuesto produce una dirección distinta de la que la gente teclearía, y
// nadie se entera hasta que alguien no recibe un correo. El camino feliz
// (ana.perez) casi no necesita test; los bordes sí.
func TestMailboxLocalPart(t *testing.T) {
	casos := []struct {
		nombre, given, family, full, email, quiere string
	}{
		{"caso corriente", "Ana", "Pérez", "", "", "ana.perez"},
		{"tildes y eñe plegadas", "Íñigo", "Muñoz", "", "", "inigo.munoz"},
		{"apellido doble español", "Laura", "Perez Ruiz", "", "", "laura.perez.ruiz"},
		{"nombre compuesto", "María del Carmen", "Pérez", "", "", "maria.del.carmen.perez"},
		{"guion del apellido se conserva", "Jean", "Sáenz-López", "", "", "jean.saenz-lopez"},
		{"apóstrofo se cae", "Sean", "O'Donnell", "", "", "sean.odonnell"},
		{"eszett alemana da dos eses", "Hans", "Straßer", "", "", "hans.strasser"},
		{"inicial con punto no duplica separador", "J.", "Pérez", "", "", "j.perez"},
		{"sin apellido no deja punto colgando", "Cher", "", "", "", "cher"},
		{"cae al nombre completo", "", "", "Ada Lovelace", "", "ada.lovelace"},
		{"último recurso: el local del correo", "", "", "", "ada.l@externo.com", "ada.l"},
		{"alfabeto no cubierto cae al correo", "Ω", "Ψ", "", "griego@externo.com", "griego"},
		{"sin nada utilizable devuelve vacío", "", "", "", "", ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			if got := MailboxLocalPart(c.given, c.family, c.full, c.email); got != c.quiere {
				t.Errorf("MailboxLocalPart(%q,%q,%q,%q) = %q, quiere %q",
					c.given, c.family, c.full, c.email, got, c.quiere)
			}
		})
	}
}

// El local-part no puede pasar de 64 octetos ni acabar en punto: un servidor ajeno
// rechazaría la dirección entera y el rebote llegaría mucho después del alta.
func TestMailboxLocalPartRespetaElLimite(t *testing.T) {
	largo := "Wolfeschlegelsteinhausenbergerdorff Von Und Zu Hohenzollern Sigmaringen"
	got := MailboxLocalPart(largo, "", "", "")
	if len(got) > maxLocalPart {
		t.Fatalf("local-part de %d octetos, máximo %d: %q", len(got), maxLocalPart, got)
	}
	if got[len(got)-1] == '.' || got[len(got)-1] == '-' {
		t.Fatalf("el recorte dejó un separador al final: %q", got)
	}
}

// El desempate tiene que ser adivinable y seguir cabiendo en 64 octetos.
func TestMailboxCandidate(t *testing.T) {
	if got := MailboxCandidate("ana.perez", "acme.eu", 0); got != "ana.perez@acme.eu" {
		t.Errorf("el primer intento no lleva sufijo: %q", got)
	}
	// n=1 es el SEGUNDO intento y por eso el sufijo es 2: nadie escribe «ana.perez1».
	if got := MailboxCandidate("ana.perez", "acme.eu", 1); got != "ana.perez2@acme.eu" {
		t.Errorf("el segundo intento debe ser ana.perez2@: %q", got)
	}
	if got := MailboxCandidate("ana.perez", "acme.eu", 9); got != "ana.perez10@acme.eu" {
		t.Errorf("el décimo intento debe ser ana.perez10@: %q", got)
	}
	raiz := ""
	for len(raiz) < maxLocalPart {
		raiz += "a"
	}
	got := MailboxCandidate(raiz, "acme.eu", 1)
	if local := got[:len(got)-len("@acme.eu")]; len(local) > maxLocalPart {
		t.Fatalf("el sufijo desbordó el límite: %d octetos", len(local))
	}
}

// Una dirección tecleada a mano entra por la misma normalización que el patrón:
// admitirla cruda metería tildes y espacios en la base, y eso solo falla el día que
// alguien intenta escribir a esa dirección.
func TestNormalizeMailboxAddress(t *testing.T) {
	casos := []struct{ nombre, entrada, quiere string }{
		{"solo el local toma el dominio de la org", "soporte", "soporte@acme.eu"},
		{"dirección completa se respeta", "Soporte@Acme.EU", "soporte@acme.eu"},
		{"otro dominio se acepta y se valida aparte", "ana@otra.com", "ana@otra.com"},
		{"tildes plegadas también aquí", "José.Pérez", "jose.perez"},
		{"espacios convertidos en punto", "ana perez", "ana.perez"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			quiere := c.quiere
			if !contieneArroba(quiere) {
				quiere += "@acme.eu"
			}
			got, err := NormalizeMailboxAddress(c.entrada, "acme.eu")
			if err != nil {
				t.Fatalf("NormalizeMailboxAddress(%q): %v", c.entrada, err)
			}
			if got != quiere {
				t.Errorf("NormalizeMailboxAddress(%q) = %q, quiere %q", c.entrada, got, quiere)
			}
		})
	}
	for _, malo := range []string{"", "   ", "@acme.eu", "ana@", "ana@sindominio", "!!!"} {
		if got, err := NormalizeMailboxAddress(malo, "acme.eu"); err == nil {
			t.Errorf("NormalizeMailboxAddress(%q) = %q, se esperaba error", malo, got)
		}
	}
}

func contieneArroba(s string) bool {
	for i := range s {
		if s[i] == '@' {
			return true
		}
	}
	return false
}

// Un dominio propio SIN verificar no puede repartir buzones: sería crear
// direcciones que el MX no sirve y que pisan las del dueño real del dominio.
func TestOrgMailDomain(t *testing.T) {
	if got := OrgMailDomain("acme", "acme.eu", true, "sytue.eu"); got != "acme.eu" {
		t.Errorf("dominio propio verificado = %q", got)
	}
	if got := OrgMailDomain("acme", "acme.eu", false, "sytue.eu"); got != "acme.sytue.eu" {
		t.Errorf("dominio propio sin verificar debe caer al subdominio: %q", got)
	}
	if got := OrgMailDomain("acme", "", false, "sytue.eu"); got != "acme.sytue.eu" {
		t.Errorf("organización sin dominio propio = %q", got)
	}
}

// LA AGENDA NO PUBLICA EL CORREO PERSONAL DE NADIE.
//
// Es la regla que pidió el propietario: en la libreta de la organización van las
// direcciones en las que se recibe DENTRO de Sytue, y el correo de invitación
// solo si su dominio está verificado por la organización.
func TestDireccionesDeAgenda(t *testing.T) {
	bz := func(dir string, p bool) OrgMailbox { return OrgMailbox{Address: dir, Primary: p} }

	casos := []struct {
		nombre      string
		buzones     []OrgMailbox
		login       string
		verificados []string
		quiero      []string
	}{
		{
			nombre:  "el correo de invitación NO sale",
			buzones: []OrgMailbox{bz("juan@acme.sytue.eu", true)},
			login:   "juanp.arce15@gmail.com",
			quiero:  []string{"juan@acme.sytue.eu"},
		},
		{
			nombre:      "…salvo que su dominio esté verificado por la organización",
			buzones:     []OrgMailbox{bz("juan@acme.sytue.eu", true)},
			login:       "juan@acme.com",
			verificados: []string{"acme.com"},
			quiero:      []string{"juan@acme.sytue.eu", "juan@acme.com"},
		},
		{
			// LA PRINCIPAL DELANTE AUNQUE LLEGUE LA ÚLTIMA. El almacén ya la pone
			// primera (`ORDER BY primary_address DESC, address`), pero la agenda no
			// depende de ese orden: si mañana cambia la consulta, esto sigue bien.
			// Entre los alias se respeta el orden en que llegan, que es cosa del
			// almacén y no de esta función.
			nombre:  "la principal va delante aunque llegue la última",
			buzones: []OrgMailbox{bz("ventas@acme.com", false), bz("j.arce@acme.com", false), bz("juan@acme.com", true)},
			login:   "x@gmail.com",
			quiero:  []string{"juan@acme.com", "ventas@acme.com", "j.arce@acme.com"},
		},
		{
			nombre:      "no se repite la que ya estaba como buzón",
			buzones:     []OrgMailbox{bz("juan@acme.com", true)},
			login:       "JUAN@ACME.COM",
			verificados: []string{"acme.com"},
			quiero:      []string{"juan@acme.com"},
		},
		{
			nombre:      "un dominio que solo SE PARECE al verificado no entra",
			buzones:     []OrgMailbox{bz("juan@acme.sytue.eu", true)},
			login:       "juan@noacme.com",
			verificados: []string{"acme.com"},
			quiero:      []string{"juan@acme.sytue.eu"},
		},
		{
			nombre:      "ni uno que lo lleva de prefijo",
			buzones:     []OrgMailbox{bz("juan@acme.sytue.eu", true)},
			login:       "juan@acme.com.de-otro.net",
			verificados: []string{"acme.com"},
			quiero:      []string{"juan@acme.sytue.eu"},
		},
		{
			nombre:  "sin buzón y con invitación de fuera, NADA: no hay dónde escribirle",
			buzones: nil,
			login:   "juan@gmail.com",
			quiero:  []string{},
		},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := DireccionesDeAgenda(c.buzones, c.login, c.verificados)
			if len(got) != len(c.quiero) {
				t.Fatalf("got %v, quiero %v", got, c.quiero)
			}
			for i := range got {
				if got[i] != c.quiero[i] {
					t.Fatalf("got %v, quiero %v", got, c.quiero)
				}
			}
		})
	}
}
