// SPDX-License-Identifier: Apache-2.0
package mail

import (
	"encoding/json"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/fyne/ui"
	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

func pulsarPieCorreo(t *testing.T, c *correoExterno, key string) {
	t.Helper()
	for _, item := range c.pie.Objects {
		if b, ok := item.(*ui.Boton); ok && b.Texto == c.t(key) {
			b.AlPulsar()
			return
		}
	}
	t.Fatalf("no se encontró el botón %s", key)
}

func TestCuentaNuevaNoHeredaOpcionesOcultasDeOtraCuenta(t *testing.T) {
	c := correoDePrueba(t, &correoFalso{})
	a := api.ExternalMailAccount{InstanceID: "account-a", Provider: "imap", Name: "Account A", Address: "a@example.eu", Services: []string{"mail", "calendar"}}
	if err := json.Unmarshal([]byte(`{"configuration":{
		"incoming":{"host":"imap-a.example.eu","port":1993,"username":"user-a","security":"tls"},
		"outgoing":{"host":"smtp-a.example.eu","port":2525,"username":"sender-a","security":"starttls"}
	}}`), &a); err != nil {
		t.Fatal(err)
	}
	c.reconectar(a)
	var plegar *ui.Boton
	for _, item := range c.cuerpo.Objects {
		if b, ok := item.(*ui.Boton); ok && b.Texto == c.t("externalMail.advanced") {
			plegar = b
		}
	}
	if plegar == nil {
		t.Fatal("no se encuentran las opciones avanzadas")
	}
	plegar.AlPulsar()
	plegar.AlPulsar()
	if c.campos["username"].Texto() != "user-a" || c.campos["smtpPort"].Texto() != "2525" {
		t.Fatal("reconectar perdió la configuración deliberada de A")
	}
	pulsarPieCorreo(t, c, "common.cancel")
	pulsarPieCorreo(t, c, "externalMail.add")
	if c.proveedor != "gmail" || c.instancia != "" || c.paso != 0 || c.avanzado || !slices.Equal(c.servicios, []string{"mail"}) {
		t.Error("Añadir no restauró el estado inicial del asistente")
	}
	for _, key := range []string{"name", "address", "host", "username", "password", "smtpHost", "smtpUser", "smtpPassword"} {
		if c.campos[key].Texto() != "" {
			t.Errorf("Añadir conservó el campo %s de A", key)
		}
	}
	c.proveedor = "imap"
	for key, value := range map[string]string{"address": "b@example.eu", "host": "imap-b.example.eu", "password": "synthetic-password-b", "smtpHost": "smtp-b.example.eu"} {
		c.campos[key].PonerTexto(value)
	}
	cfg, err := c.configuracion()
	if err != nil || cfg.Incoming.Username != "b@example.eu" || cfg.Incoming.Port != 993 || cfg.Outgoing.Username != "b@example.eu" || cfg.Outgoing.Port != 465 || cfg.Outgoing.Security != "tls" || cfg.InstanceID != "" || cfg.SMTPPassword != "" {
		t.Fatal("la nueva configuración de B heredó usuarios, transporte o credenciales de A")
	}
}
