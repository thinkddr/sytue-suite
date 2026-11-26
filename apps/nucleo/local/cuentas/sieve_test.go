// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cuentas

import (
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/local/red"
)

// El servidor de la respuesta automática (ManageSieve, M-10) entra como un servidor más
// de una cuenta de correo, sin cambiar los demás: el que escribió la persona o, en un IMAP
// cualquiera, el host de IMAP en el 4190. Gmail, Outlook y compañía no tienen: no abren
// nada más.
func TestElServidorSieveEsUnoMasDeLaCuenta(t *testing.T) {
	ajustes, datos := t.TempDir(), t.TempDir()
	escribir(t, filepath.Join(datos, "Correo", "cuentas.json"), `[
{"id":"g","proveedor":"gmail","direccion":"ana@gmail.com","imap":{"host":"imap.gmail.com","puerto":993,"seguridad":"tls"},"smtp":{"host":"smtp.gmail.com","puerto":465,"seguridad":"tls"}},
{"id":"m","proveedor":"microsoft","direccion":"ana@outlook.com","imap":{"host":"outlook.office365.com","puerto":993,"seguridad":"tls"},"smtp":{"host":"smtp.office365.com","puerto":587,"seguridad":"starttls"},"acceso":"oauth-microsoft"},
{"id":"i","proveedor":"imap","direccion":"ana@casa.eu","imap":{"host":"mail.casa.eu","puerto":993,"seguridad":"tls"},"smtp":{"host":"mail.casa.eu","puerto":587,"seguridad":"starttls"}},
{"id":"s","proveedor":"icloud","direccion":"ana@icloud.com","imap":{"host":"imap.mail.me.com","puerto":993,"seguridad":"tls"},"smtp":{"host":"smtp.mail.me.com","puerto":587,"seguridad":"starttls"},"sieve":{"host":"sieve.casa.eu","puerto":2000}},
{"id":"l","proveedor":"imap","direccion":"yo@localhost","imap":{"host":"127.0.0.1","puerto":1143,"seguridad":"ninguna"},"smtp":{"host":"127.0.0.1","puerto":1025,"seguridad":"ninguna"}}]`)
	r := Abrir(ajustes, datos)
	cs := r.Cuentas()
	if len(cs) != 5 {
		t.Fatalf("%+v", cs)
	}
	for i, n := range []int{2, 2, 3, 3, 3} {
		if len(cs[i].Servidores) != n {
			t.Errorf("%s: %+v", cs[i].ID, cs[i].Servidores)
		}
	}
	if s := cs[2].Servidores[2]; s.Protocolo != "managesieve" || s.Host != "mail.casa.eu" || s.Puerto != PuertoSieve || s.Seguridad != "starttls" {
		t.Errorf("IMAP cualquiera: %+v", s)
	}
	if s := cs[3].Servidores[2]; s.Host != "sieve.casa.eu" || s.Puerto != 2000 {
		t.Errorf("el escrito por la persona: %+v", s)
	}
	if s := cs[4].Servidores[2]; s.Seguridad != "ninguna" {
		t.Errorf("sin cifrar solo en este equipo: %+v", s)
	}
	p := red.Abrir("")
	p.Fuente(r.Destinos)
	for _, caso := range []struct {
		host   string
		puerto int
		vale   bool
	}{
		{"mail.casa.eu", 4190, true},
		{"mail.casa.eu", 993, true},
		{"mail.casa.eu", 4191, false},
		{"sieve.casa.eu", 2000, true},
		{"sieve.casa.eu", 4190, false},
		{"imap.gmail.com", 4190, false},
		{"outlook.office365.com", 4190, false},
		{"imap.mail.me.com", 4190, false},
		{"127.0.0.1", 4190, true},
	} {
		if err := p.Permitido(red.Correo, caso.host, caso.puerto, true); (err == nil) != caso.vale {
			t.Errorf("%s:%d: %v", caso.host, caso.puerto, err)
		}
	}
}
