// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package http

import "testing"

// TestHostPermitido: la lista de hosts es lo único que separa a Sytue Docs de
// ser el convertidor —y el proxy— gratuito de quien pase por ahí.
//
// El protocolo NO trae esta verja: un `WOPISrc` es una URL cualquiera. La
// barrera SSRF impide llegar a lo interno, pero no impide que alguien mande a
// Docs a buscar un fichero en su propio servidor y se lo devuelva convertido con
// la cara de la suite.
func TestHostPermitido(t *testing.T) {
	permitidos := []string{"drive.sytue.eu", "*.cliente.example"}

	sí := []string{
		"https://drive.sytue.eu/api/v1/wopi/files/abc",
		"https://DRIVE.SYTUE.EU/api/v1/wopi/files/abc", // el host no distingue mayúsculas
		"https://uno.cliente.example/wopi/files/1",
		"https://otro.cliente.example/wopi/files/1",
	}
	for _, src := range sí {
		if !hostPermitido(src, permitidos) {
			t.Errorf("%s debería estar permitido", src)
		}
	}

	no := []string{
		"https://drive.sytue.eu.malo.example/wopi/files/1", // sufijo pegado al dominio
		"https://malo.example/wopi/files/1",
		"https://cliente.example.malo.example/wopi/files/1",
		"https://sytue.eu/wopi/files/1", // el dominio raíz NO es el Drive
		"no-es-una-url",
		"",
	}
	for _, src := range no {
		if hostPermitido(src, permitidos) {
			t.Errorf("%s NO debería estar permitido", src)
		}
	}

	// Sin lista no se admite NADA. Es deliberado: un valor por defecto abierto
	// sería la forma cómoda de dejar esto mal configurado para siempre.
	if hostPermitido("https://drive.sytue.eu/wopi/files/1", nil) {
		t.Error("sin lista configurada no se admite ningún host")
	}
}

// TestHostsWopiPorDefecto: sin lista explícita se deriva el Drive de la propia
// suite a partir de la URL pública del editor.
func TestHostsWopiPorDefecto(t *testing.T) {
	if got := hostsWopi(Deps{PublicURL: "https://docs.sytue.eu"}); len(got) != 1 || got[0] != "drive.sytue.eu" {
		t.Fatalf("hostsWopi = %v, quería [drive.sytue.eu]", got)
	}
	// Lo explícito manda sobre lo derivado.
	got := hostsWopi(Deps{PublicURL: "https://docs.sytue.eu", WopiHosts: []string{"otro.example"}})
	if len(got) != 1 || got[0] != "otro.example" {
		t.Fatalf("la lista explícita debe mandar: %v", got)
	}
	// Sin URL pública no se inventa nada: mejor sin hosts que con uno equivocado.
	if got := hostsWopi(Deps{}); len(got) != 0 {
		t.Fatalf("sin URL pública no debe derivarse ningún host: %v", got)
	}
	// Y de un dominio a secas tampoco: cortar por el primer punto daba
	// `drive.eu` —un dominio de OTRO— y lo dejaba admitido.
	for _, u := range []string{"https://sytue.eu", "https://editor.sytue.eu"} {
		if got := hostsWopi(Deps{PublicURL: u}); len(got) != 0 {
			t.Fatalf("de %q no debe derivarse ningún host: %v", u, got)
		}
	}
}
