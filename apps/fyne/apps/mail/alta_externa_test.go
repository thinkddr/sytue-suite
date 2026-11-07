// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"context"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// avisosDeAlta son los avisos «conectada» que hay en la esquina, y los descarta al
// acabar la prueba.
func avisosDeAlta(t *testing.T, b *Buzon) []*ui.Aviso {
	t.Helper()
	t.Cleanup(func() {
		for _, a := range b.P.Avisos.Avisos() {
			b.P.Avisos.Descartar(a)
		}
	})
	var out []*ui.Aviso
	for _, a := range b.P.Avisos.Avisos() {
		if a.Titulo == b.P.T.T("externalMail.connected", i18n.Vars{"account": "ana@example.eu"}) {
			out = append(out, a)
		}
	}
	return out
}

// Fase B, punto 7: al terminar el alta el diálogo se va, la Entrada de la cuenta
// nueva queda elegida —con su rama desplegada, aunque se hubiera plegado— y se dice
// qué cuenta se conectó (`onOpenAccount(account, true)` y su toast en MailApp.tsx).
// «Abrir buzón» de una cuenta que ya estaba no avisa de nada. VISTO EN ROJO
// (2026-11-07) sin el Avisar de cargarCuentas y sin el delete de la rama.
func TestAltaTerminadaCierraEligeLaEntradaYAvisa(t *testing.T) {
	cuentas := cuentaGmailNueva()
	s := &correoFalso{cuentas: cuentas, carpetas: map[string][]api.ExternalMailFolder{
		"nueva": {{ID: "Label_1", Name: "Facturas"}, {ID: "INBOX", Name: "INBOX", Role: "inbox"}},
	}}
	c := correoDePrueba(t, s)
	c.proveedor, c.asistente, c.paso, c.instancia = "gmail", true, 1, ""
	c.cuentas = cuentas
	c.b.P.Menu.Ramas = map[string]bool{claveCuenta("nueva"): true} // plegada antes
	c.b.AutorizarCorreo = func(context.Context, string, string, ...string) (string, error) {
		return "nueva", nil
	}
	c.autorizar()
	if !c.cerrado || c.b.correoAbierto != nil && !c.b.correoAbierto.cerrado {
		t.Fatal("el diálogo sigue abierto al terminar el alta")
	}
	if v := c.b.externa; v == nil || v.Cuenta != "nueva" || v.Carpeta != "INBOX" {
		t.Fatalf("se ve %+v y tenía que ser la Entrada de la cuenta nueva", v)
	}
	if !c.b.P.Menu.abierta(claveCuenta("nueva"), true) {
		t.Fatal("la rama de la cuenta nueva sigue plegada: su Entrada no se ve elegida")
	}
	if av := avisosDeAlta(t, c.b); len(av) != 1 || av[0].Tono != ui.AvisoExito {
		t.Fatalf("avisos de alta: %+v; quiero uno, de éxito, con la dirección", av)
	}

	// A mano (IMAP) igual: lo que guarda el servidor se abre y se avisa.
	s = &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{{
		InstanceID: "fixture", Provider: "imap", Address: "ana@example.eu", Connected: true, Services: []string{"mail"},
	}}}}
	c = correoDePrueba(t, s)
	c.asistente, c.instancia = true, ""
	for key, value := range map[string]string{"address": "ana@example.eu", "host": "imap.example.eu", "smtpHost": "smtp.example.eu", "password": "secreta", "port": "993", "smtpPort": "465"} {
		c.campos[key].PonerTexto(value)
	}
	c.conectar()
	if v := c.b.externa; !c.cerrado || v == nil || v.Cuenta != "fixture" || v.Carpeta != "INBOX" {
		t.Fatalf("alta a mano: cerrado %v, vista %+v", c.cerrado, v)
	}
	if av := avisosDeAlta(t, c.b); len(av) != 1 {
		t.Fatalf("alta a mano: %d avisos de alta; quiero uno", len(av))
	}

	// «Abrir buzón» de la lista: se abre, sin aviso.
	c = correoDePrueba(t, &correoFalso{cuentas: cuentas})
	c.abrirCuenta(cuentas.Items[0])
	if c.b.externa == nil || c.b.externa.Cuenta != "nueva" {
		t.Fatalf("abrir buzón: vista %+v", c.b.externa)
	}
	if av := avisosDeAlta(t, c.b); len(av) != 0 {
		t.Fatalf("abrir una cuenta que ya estaba avisó %d veces de un alta", len(av))
	}
}
