// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package contactos

import (
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

const dosTarjetas = "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n" +
	"item1.EMAIL;TYPE=INTERNET,WORK,pref:ada@acme.test\r\nEMAIL;TYPE=home:ada@casa.test\r\n" +
	"TEL;TYPE=CELL:+34 600 11 22 33\r\nORG:Analytical Engines;I+D\r\nBDAY:18151210\r\n" +
	"NOTE:Primera\\nprogramadora\\, dicen.\r\nADR;TYPE=work:;;12 St James's\r\n  Square;London;;SW1Y;UK\r\n" +
	"X-SOCIALPROFILE:https://ejemplo.es/ada\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\r\nVERSION:4.0\r\nEMAIL:sin-nombre@acme.test\r\nEND:VCARD\r\n" +
	"begin:vcard\r\nfn:Bruno\r\nend:vcard\r\n"

// TestTarjetasComoLaWeb: una tarjeta por BEGIN/END (sin mirar mayúsculas), fuera la
// que no trae nombre, y cada una TAL CUAL, con sus X-* y sus líneas plegadas.
func TestTarjetasComoLaWeb(t *testing.T) {
	ts := Tarjetas(dosTarjetas + "BEGIN:VCARD\r\nFN:\r\n Plegada\r\nEND:VCARD\r\n")
	if len(ts) != 3 || !strings.HasPrefix(ts[0], "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada") ||
		!strings.Contains(ts[0], "X-SOCIALPROFILE:https://ejemplo.es/ada\r\n") ||
		!strings.Contains(ts[0], "12 St James's\r\n  Square") || ts[1] != "begin:vcard\r\nfn:Bruno\r\nend:vcard" {
		t.Fatalf("tarjetas %q", ts)
	}
}

// TestImportarVCardCreaCadaUnaYLoDice: una petición por tarjeta, con la tarjeta ENTERA
// —así el servicio conserva la foto y los X-*—, el avance en el botón y el aviso de
// cuántas entraron.
func TestImportarVCardCreaCadaUnaYLoDice(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.ElegirFichero = func(_ []string, listo func([]byte)) { listo([]byte(dosTarjetas)) }
	a.Pantalla.AlImportarVcard()
	if len(s.escrituras) != 2 {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	if e := s.escrituras[0]; e.ruta != "POST /api/v1/address-books/l1/contacts" ||
		!strings.HasPrefix(e.cuerpo, "BEGIN:VCARD\r\nVERSION:3.0\r\nFN:Ada") || !strings.Contains(e.cuerpo, "X-SOCIALPROFILE") {
		t.Errorf("se manda %+v", e)
	}
	if av := a.avisos.Avisos(); len(av) != 1 || av[0].Titulo != "2 contactos importados" {
		t.Errorf("avisos %v", av)
	}
	if a.Pantalla.ImportOcupado || a.Pantalla.Importando != "" {
		t.Error("terminada la importación, el botón sigue ocupado")
	}
}

// TestImportarCSVEsUnaPeticion: el CSV va entero en UNA petición y el aviso cuenta lo
// que el servicio dice que entró y lo que se omitió.
func TestImportarCSVEsUnaPeticion(t *testing.T) {
	s := &servicioFalso{contactos: []api.Contact{persona("ana", "Ana")}}
	a := appDePrueba(t, s)
	a.Cargar()
	a.ElegirFichero = func(_ []string, listo func([]byte)) { listo([]byte("Name\r\nAda\r\nBruno\r\n")) }
	a.Pantalla.AlImportarCSV()
	if len(s.escrituras) != 1 || s.escrituras[0].ruta != "POST /api/v1/address-books/l1/import-csv" {
		t.Fatalf("escrituras %v", s.escrituras)
	}
	av := a.avisos.Avisos()
	if len(av) != 1 || av[0].Titulo != "2 contactos importados" || av[0].Descripcion != "1 se omitió (duplicado o inválido)." {
		t.Errorf("avisos %+v", av)
	}
}
