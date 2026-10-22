// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build integration

// Tests de integración de los buzones de organización contra PostgreSQL real
// (pgtest). Lo que se fija aquí es lo que rompe SIN HACER RUIDO: dos personas que
// se llaman igual, un alias que choca con el buzón de otro, una organización que
// intenta reclamar una dirección de otra, y el alta que pasa dos veces. Ninguno de
// esos casos falla en el camino feliz — fallan meses después, cuando alguien deja
// de recibir correo y nadie sabe por qué.
package store_test

import (
	"context"
	"errors"
	"testing"

	"git.sytue.eu/sytue/one/services/id/internal/domain"
	"git.sytue.eu/sytue/one/services/id/internal/store"
)

// orgConBuzones crea una organización lista para repartir direcciones.
func orgConBuzones(t *testing.T, ctx context.Context, st *store.Store, slug string) domain.Organization {
	t.Helper()
	org, err := st.CreateOrg(ctx, nil, domain.Organization{Slug: slug, Name: slug})
	if err != nil {
		t.Fatalf("CreateOrg(%q): %v", slug, err)
	}
	return org
}

// usuarioConNombre da de alta a alguien con nombre y apellidos separados, que es de
// donde sale el patrón nombre.apellidos.
func usuarioConNombre(t *testing.T, ctx context.Context, st *store.Store, email, nombre, apellidos string) domain.User {
	t.Helper()
	u, err := st.CreateUser(ctx, nil, domain.User{
		Email: email, EmailVerified: true,
		Name: nombre + " " + apellidos, GivenName: nombre, FamilyName: apellidos,
	})
	if err != nil {
		t.Fatalf("CreateUser(%q): %v", email, err)
	}
	return u
}

// TestBuzonPorDefectoYHomonimos: la dirección sale del patrón nombre.apellidos y
// dos personas que se llaman IGUAL no pueden acabar con la misma. Es el caso que
// más silenciosamente rompe: sin desempate, la segunda alta o falla o —peor— se
// queda con la dirección de la primera y le roba el correo.
func TestBuzonPorDefectoYHomonimos(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	primera := usuarioConNombre(t, ctx, st, "ana.perez@personal.example", "Ana", "Pérez")
	segunda := usuarioConNombre(t, ctx, st, "otra.ana@personal.example", "Ana", "Pérez")

	dir1, err := st.EnsureOrgMailbox(ctx, nil, org.ID, primera.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(primera): %v", err)
	}
	// La organización no tiene dominio propio verificado ⇒ subdominio de la suite.
	if dir1 != "ana.perez@acme.sytue.eu" {
		t.Fatalf("dirección por defecto = %q, quiero ana.perez@acme.sytue.eu", dir1)
	}

	dir2, err := st.EnsureOrgMailbox(ctx, nil, org.ID, segunda.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(segunda): %v", err)
	}
	if dir2 == dir1 {
		t.Fatalf("las dos homónimas comparten dirección %q: se robarían el correo", dir1)
	}
	if dir2 != "ana.perez2@acme.sytue.eu" {
		t.Fatalf("desempate = %q, quiero ana.perez2@acme.sytue.eu", dir2)
	}

	// IDEMPOTENCIA: repetir el alta —un reintento de SCIM, un cambio de rol— no
	// puede reasignar la dirección de nadie. Quien ya recibe correo en una
	// dirección tiene que seguir recibiéndolo en ESA.
	otra, err := st.EnsureOrgMailbox(ctx, nil, org.ID, primera.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(repetido): %v", err)
	}
	if otra != dir1 {
		t.Fatalf("el alta repetida cambió la dirección: %q -> %q", dir1, otra)
	}
}

// TestBuzonPersonalizado: la dirección elegida al dar de alta manda sobre el
// patrón; y si está cogida, el alta NO se cae — cae al patrón. Dejar a alguien sin
// cuenta porque su dirección preferida estaba pillada sería el peor de los males.
func TestBuzonPersonalizado(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	sofia := usuarioConNombre(t, ctx, st, "sofia@personal.example", "Sofía", "Ramos")
	dir, err := st.EnsureOrgMailbox(ctx, nil, org.ID, sofia.ID, "soporte")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(personalizada): %v", err)
	}
	if dir != "soporte@acme.sytue.eu" {
		t.Fatalf("dirección personalizada = %q", dir)
	}

	// Otra persona pide la MISMA: se le da la del patrón, sin romper su alta.
	luis := usuarioConNombre(t, ctx, st, "luis@personal.example", "Luis", "Gil")
	dir2, err := st.EnsureOrgMailbox(ctx, nil, org.ID, luis.ID, "soporte")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(personalizada cogida): %v", err)
	}
	if dir2 != "luis.gil@acme.sytue.eu" {
		t.Fatalf("al estar cogida la elegida debe caer al patrón, no a %q", dir2)
	}
}

// TestBuzonAislamientoEntreOrganizaciones: el espacio de direcciones es GLOBAL. Dos
// organizaciones distintas no pueden reclamar la misma dirección, porque el MX
// resuelve por dirección y no sabría a cuál de las dos entregar.
func TestBuzonAislamientoEntreOrganizaciones(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	acme := orgConBuzones(t, ctx, st, "acme")
	otra := orgConBuzones(t, ctx, st, "otra")

	ana := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")

	// La MISMA persona en DOS organizaciones: una dirección en cada dominio, porque
	// su correo de trabajo en una nada tiene que ver con el de la otra.
	enAcme, err := st.EnsureOrgMailbox(ctx, nil, acme.ID, ana.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(acme): %v", err)
	}
	enOtra, err := st.EnsureOrgMailbox(ctx, nil, otra.ID, ana.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(otra): %v", err)
	}
	if enAcme != "ana.perez@acme.sytue.eu" || enOtra != "ana.perez@otra.sytue.eu" {
		t.Fatalf("una persona en dos orgs debe tener una dirección por dominio: %q / %q", enAcme, enOtra)
	}

	// Y un administrador de `otra` no puede robar una dirección de `acme`.
	//
	// El error es de ENTRADA INVÁLIDA y no de conflicto, a propósito: un 409
	// significaría «esa dirección ya existe», y quien pregunta es de otra
	// organización. Distinguir «no es de tu dominio» de «es de tu dominio pero
	// está cogida» le dejaría enumerar las direcciones de un cliente ajeno
	// probando alias. Lo que se le dice es lo único que le incumbe: ese dominio
	// no es suyo.
	bob := usuarioConNombre(t, ctx, st, "bob@personal.example", "Bob", "Gil")
	if _, err := st.AddMailboxAlias(ctx, otra.ID, bob.ID, enAcme); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("alias sobre la dirección de otra org = %v, quiero ErrInvalidInput", err)
	}
}

// TestAliasChocaConBuzonExistente: un alias que coincide con la dirección
// principal de OTRA persona tiene que rebotar. Es el caso que la migración 0048
// resuelve con un solo índice: si buzones y alias vivieran en tablas distintas, el
// choque se colaría por la rendija entre las dos comprobaciones.
func TestAliasChocaConBuzonExistente(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	ana := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	bob := usuarioConNombre(t, ctx, st, "bob@personal.example", "Bob", "Gil")
	dirAna, err := st.EnsureOrgMailbox(ctx, nil, org.ID, ana.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox(ana): %v", err)
	}
	if _, err := st.EnsureOrgMailbox(ctx, nil, org.ID, bob.ID, ""); err != nil {
		t.Fatalf("EnsureOrgMailbox(bob): %v", err)
	}

	// Bob no puede quedarse con la dirección de Ana disfrazándola de alias.
	if _, err := st.AddMailboxAlias(ctx, org.ID, bob.ID, dirAna); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("alias sobre el buzón de otra persona = %v, quiero ErrConflict", err)
	}

	// Un alias libre sí entra, y dos veces el mismo no.
	if _, err := st.AddMailboxAlias(ctx, org.ID, bob.ID, "ventas"); err != nil {
		t.Fatalf("AddMailboxAlias(ventas): %v", err)
	}
	if _, err := st.AddMailboxAlias(ctx, org.ID, bob.ID, "ventas"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("alias repetido = %v, quiero ErrConflict", err)
	}
}

// TestCambiarDireccionConservaLaAnterior: cambiar la dirección principal degrada la
// vieja a alias en vez de borrarla. Si desapareciera, todo el que tuviera guardada
// la anterior —incluidos remitentes automáticos que nadie recuerda— empezaría a
// recibir rebotes sin que la organización se enterara.
func TestCambiarDireccionConservaLaAnterior(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	ana := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	vieja, err := st.EnsureOrgMailbox(ctx, nil, org.ID, ana.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox: %v", err)
	}

	// Se casa y cambia de apellido.
	nueva, err := st.SetPrimaryMailbox(ctx, org.ID, ana.ID, "ana.perez.ruiz")
	if err != nil {
		t.Fatalf("SetPrimaryMailbox: %v", err)
	}
	boxes, err := st.ListOrgMailboxes(ctx, org.ID, ana.ID)
	if err != nil {
		t.Fatalf("ListOrgMailboxes: %v", err)
	}
	var viejaSigue, nuevaEsPrincipal bool
	for _, b := range boxes {
		if b.Address == vieja && !b.Primary {
			viejaSigue = true
		}
		if b.Address == nueva && b.Primary {
			nuevaEsPrincipal = true
		}
	}
	if !viejaSigue {
		t.Fatalf("la dirección anterior (%q) desapareció: se perdería el correo que iba a ella", vieja)
	}
	if !nuevaEsPrincipal {
		t.Fatalf("la nueva dirección (%q) no quedó como principal", nueva)
	}

	// Y borrar la principal no puede pasar: dejaría a la persona sin recibir.
	for _, b := range boxes {
		if b.Primary {
			if err := st.DeleteMailboxAlias(ctx, org.ID, b.ID); !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("borrar la dirección principal = %v, quiero que se niegue", err)
			}
		}
	}
}

// TestQuitarAliasEsperaARetirarloDeMail: quitar un alias lo esconde al instante
// pero NO libera la dirección hasta que se ha retirado también de Sytue Mail. Si la
// liberase antes, se podría reasignar a otra persona y el reconciliador borraría
// después el alias del nuevo dueño, dejándolo sin recibir sin que nadie lo tocara.
func TestQuitarAliasEsperaARetirarloDeMail(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	ana := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	bob := usuarioConNombre(t, ctx, st, "bob@personal.example", "Bob", "Gil")
	if _, err := st.EnsureOrgMailbox(ctx, nil, org.ID, ana.ID, ""); err != nil {
		t.Fatalf("EnsureOrgMailbox(ana): %v", err)
	}
	if _, err := st.EnsureOrgMailbox(ctx, nil, org.ID, bob.ID, ""); err != nil {
		t.Fatalf("EnsureOrgMailbox(bob): %v", err)
	}
	alias, err := st.AddMailboxAlias(ctx, org.ID, ana.ID, "ventas")
	if err != nil {
		t.Fatalf("AddMailboxAlias: %v", err)
	}

	if err := st.DeleteMailboxAlias(ctx, org.ID, alias.ID); err != nil {
		t.Fatalf("DeleteMailboxAlias: %v", err)
	}
	// Deja de verse en la consola de inmediato.
	boxes, err := st.ListOrgMailboxes(ctx, org.ID, ana.ID)
	if err != nil {
		t.Fatalf("ListOrgMailboxes: %v", err)
	}
	for _, b := range boxes {
		if b.ID == alias.ID {
			t.Fatal("el alias retirado sigue apareciendo en el listado")
		}
	}
	// Pero la dirección sigue reservada mientras no se retire de Sytue Mail.
	if _, err := st.AddMailboxAlias(ctx, org.ID, bob.ID, "ventas"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("reasignar una dirección aún no retirada = %v, quiero ErrConflict", err)
	}
	// Está en la cola de retirada, y al purgarla la dirección queda libre.
	pend, err := st.PendingMailboxRemovals(ctx, 10)
	if err != nil {
		t.Fatalf("PendingMailboxRemovals: %v", err)
	}
	var encontrada bool
	for _, m := range pend {
		if m.ID == alias.ID {
			encontrada = true
			// El reconciliador se autentica como el DUEÑO para pedir la baja: sin su
			// dirección principal no sabría de qué cuenta colgaba el alias.
			if m.PrimaryAddress != "ana.perez@acme.sytue.eu" {
				t.Fatalf("la retirada no lleva la dirección del dueño: %q", m.PrimaryAddress)
			}
		}
	}
	if !encontrada {
		t.Fatal("el alias retirado no está en la cola de retirada de Sytue Mail")
	}
	if err := st.PurgeMailbox(ctx, alias.ID); err != nil {
		t.Fatalf("PurgeMailbox: %v", err)
	}
	if _, err := st.AddMailboxAlias(ctx, org.ID, bob.ID, "ventas"); err != nil {
		t.Fatalf("tras purgar, la dirección debe poder reasignarse: %v", err)
	}
}

// TestSinDominioDeSuiteNoSeAsignaBuzon: sin dominio configurado no se inventa una
// dirección. Un buzón en un dominio que no servimos es peor que no tener buzón:
// parece que funciona y no recibe nada.
func TestSinDominioDeSuiteNoSeAsignaBuzon(t *testing.T) {
	st, ctx := newStore(t)
	org := orgConBuzones(t, ctx, st, "acme") // sin SetSuiteDomain

	ana := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	dir, err := st.EnsureOrgMailbox(ctx, nil, org.ID, ana.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox sin dominio debe ser inocuo, no un error: %v", err)
	}
	if dir != "" {
		t.Fatalf("sin dominio configurado no debe asignarse dirección, salió %q", dir)
	}
}

// conDominioPropio reclama un dominio y lo da por verificado, que es el estado del
// que cuelgan las dos garantías de abajo.
func conDominioPropio(t *testing.T, ctx context.Context, st *store.Store, orgID, host string) {
	t.Helper()
	if _, err := st.SetOrgDomain(ctx, nil, orgID, host, "tok-"+host); err != nil {
		t.Fatalf("SetOrgDomain(%q): %v", host, err)
	}
	if err := st.MarkOrgDomainVerified(ctx, nil, orgID, host); err != nil {
		t.Fatalf("MarkOrgDomainVerified(%q): %v", host, err)
	}
}

// direcciones devuelve las direcciones de una persona, con cuál es la principal.
func direcciones(t *testing.T, ctx context.Context, st *store.Store, orgID, userID string) (principal string, todas []string) {
	t.Helper()
	boxes, err := st.ListOrgMailboxes(ctx, orgID, userID)
	if err != nil {
		t.Fatalf("ListOrgMailboxes: %v", err)
	}
	for _, b := range boxes {
		todas = append(todas, b.Address)
		if b.Primary {
			principal = b.Address
		}
	}
	return principal, todas
}

func contiene(lista []string, quiero string) bool {
	for _, v := range lista {
		if v == quiero {
			return true
		}
	}
	return false
}

// TestConDominioPropioSigueHabiendoDireccionDeLaSuite: conectar un dominio propio
// NO puede dejar a nadie sin dirección en un dominio nuestro.
//
// Es la garantía de la que cuelga el cableado interno de la suite —el correo a
// canal de Rooms, los avisos de Drive, el remitente de un documento compartido—,
// que resuelve siempre contra `<slug>.sytue.eu`. Si esa dirección no existiera,
// bastaría con que un cliente dejase caducar su dominio o moviera su MX para que
// media suite dejase de entregar sin que nadie hubiera tocado nada, y el fallo no
// aparecería hasta que alguien echase de menos un aviso que nunca llegó.
func TestConDominioPropioSigueHabiendoDireccionDeLaSuite(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")
	conDominioPropio(t, ctx, st, org.ID, "acme.com")

	u := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	dir, err := st.EnsureOrgMailbox(ctx, nil, org.ID, u.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox: %v", err)
	}
	if dir != "ana.perez@acme.com" {
		t.Fatalf("principal = %q, quiero ana.perez@acme.com", dir)
	}
	principal, todas := direcciones(t, ctx, st, org.ID, u.ID)
	if principal != "ana.perez@acme.com" {
		t.Errorf("principal = %q, quiero ana.perez@acme.com", principal)
	}
	if !contiene(todas, "ana.perez@acme.sytue.eu") {
		t.Errorf("falta la dirección del dominio base: %v", todas)
	}

	// Repetirlo no puede ir añadiendo ana.perez2, ana.perez3… en el dominio base:
	// el desempate es para homónimos, no para uno mismo.
	if _, err := st.EnsureOrgMailbox(ctx, nil, org.ID, u.ID, ""); err != nil {
		t.Fatalf("EnsureOrgMailbox(repetido): %v", err)
	}
	_, otraVez := direcciones(t, ctx, st, org.ID, u.ID)
	if len(otraVez) != len(todas) {
		t.Fatalf("repetir el alta añadió direcciones: %v -> %v", todas, otraVez)
	}
}

// TestVerificarElDominioMudaAQuienYaEstaba: conectar un dominio tiene que mudar a
// la plantilla que YA está dentro, conservando su dirección anterior como alias.
//
// Sin la mudanza, un cliente conecta su dominio, la pantalla dice «verificado» y
// toda su gente sigue escribiendo desde el subdominio de la suite: la compra no
// hace nada visible. Y sin conservar la anterior, todo lo que hubiera apuntado a
// esa dirección dejaría de entregar el mismo día.
func TestVerificarElDominioMudaAQuienYaEstaba(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	org := orgConBuzones(t, ctx, st, "acme")

	// Entra ANTES de que exista el dominio propio.
	u := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	antes, err := st.EnsureOrgMailbox(ctx, nil, org.ID, u.ID, "")
	if err != nil {
		t.Fatalf("EnsureOrgMailbox: %v", err)
	}
	if antes != "ana.perez@acme.sytue.eu" {
		t.Fatalf("dirección previa = %q, quiero ana.perez@acme.sytue.eu", antes)
	}

	conDominioPropio(t, ctx, st, org.ID, "acme.com")
	movidos, err := st.PromoteOrgMailDomain(ctx, org.ID)
	if err != nil {
		t.Fatalf("PromoteOrgMailDomain: %v", err)
	}
	if movidos != 1 {
		t.Fatalf("movidos = %d, quiero 1", movidos)
	}

	principal, todas := direcciones(t, ctx, st, org.ID, u.ID)
	if principal != "ana.perez@acme.com" {
		t.Errorf("principal tras mudar = %q, quiero ana.perez@acme.com", principal)
	}
	if !contiene(todas, antes) {
		t.Errorf("la dirección anterior %q se perdió: %v", antes, todas)
	}

	// IDEMPOTENTE: repetirlo tras un fallo a mitad tiene que terminar el trabajo,
	// no volver a mudar a quien ya está ni inventarle un ana.perez2@acme.com.
	otra, err := st.PromoteOrgMailDomain(ctx, org.ID)
	if err != nil {
		t.Fatalf("PromoteOrgMailDomain(repetido): %v", err)
	}
	if otra != 0 {
		t.Errorf("la segunda pasada movió %d, quiero 0", otra)
	}
	_, despues := direcciones(t, ctx, st, org.ID, u.ID)
	if len(despues) != len(todas) {
		t.Fatalf("la segunda pasada añadió direcciones: %v -> %v", todas, despues)
	}
}

// TestAliasReservadoSoloEnElDominioRaiz: en el dominio raíz —donde viven las
// cuentas personales y la propia Sytue— «admin@» sería phishing con nuestra
// firma y se rechaza; en el subdominio de una organización es asunto suyo.
func TestAliasReservadoSoloEnElDominioRaiz(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")

	personal := orgConBuzones(t, ctx, st, domain.PersonalSlug("ana@personal.example"))
	ana := usuarioConNombre(t, ctx, st, "ana@personal.example", "Ana", "Pérez")
	if _, err := st.EnsureOrgMailbox(ctx, nil, personal.ID, ana.ID, ""); err != nil {
		t.Fatalf("EnsureOrgMailbox(ana): %v", err)
	}
	for _, local := range []string{"admin", "no-reply", "sytue-id"} {
		if _, err := st.AddMailboxAlias(ctx, personal.ID, ana.ID, local); !errors.Is(err, domain.ErrAliasReservado) {
			t.Fatalf("%s@ en el raíz = %v, quiero ErrAliasReservado", local, err)
		}
	}
	if _, err := st.AddMailboxAlias(ctx, personal.ID, ana.ID, "ana.perez.fotos"); err != nil {
		t.Fatalf("un alias corriente en el raíz debe entrar: %v", err)
	}

	acme := orgConBuzones(t, ctx, st, "acme")
	bob := usuarioConNombre(t, ctx, st, "bob@personal.example", "Bob", "Gil")
	if _, err := st.EnsureOrgMailbox(ctx, nil, acme.ID, bob.ID, ""); err != nil {
		t.Fatalf("EnsureOrgMailbox(bob): %v", err)
	}
	if _, err := st.AddMailboxAlias(ctx, acme.ID, bob.ID, "admin"); err != nil {
		t.Fatalf("admin@acme.sytue.eu es de acme, debe entrar: %v", err)
	}
}

// El claim `mailboxes` (ADR 0053): las direcciones principales de la persona en TODAS
// sus organizaciones activas, y ninguna de una organización de la que ya no es miembro.
func TestBuzonesDeLaPersonaSoloDeSusOrganizaciones(t *testing.T) {
	st, ctx := newStore(t)
	st.SetSuiteDomain("sytue.eu")
	acme, globex := orgConBuzones(t, ctx, st, "acme"), orgConBuzones(t, ctx, st, "globex")
	eva := usuarioConNombre(t, ctx, st, "eva@personal.example", "Eva", "Gil")
	for _, o := range []domain.Organization{acme, globex} {
		if err := st.UpsertMembership(ctx, nil, domain.Membership{OrgID: o.ID, UserID: eva.ID, Role: domain.RoleMember}); err != nil {
			t.Fatal(err)
		}
		if _, err := st.EnsureOrgMailbox(ctx, nil, o.ID, eva.ID, ""); err != nil {
			t.Fatal(err)
		}
	}
	if b, err := st.BuzonesDeLaPersona(ctx, eva.ID); err != nil || len(b) != 2 {
		t.Fatalf("en las dos: %v %v", b, err)
	}
	if err := st.RemoveMembership(ctx, nil, acme.ID, eva.ID); err != nil {
		t.Fatal(err)
	}
	b, _ := st.BuzonesDeLaPersona(ctx, eva.ID)
	if len(b) != 1 || b[0] != "eva.gil@globex.sytue.eu" {
		t.Fatalf("fuera de acme, sus buzones son %v", b)
	}
}
