// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// vectoresCarpetasExternas es `testdata/carpetas-externas.json`, que recorre también
// `carpetas-externas.test.ts` de la web.
type vectoresCarpetasExternas struct {
	Casos []struct {
		Nombre   string                   `json:"nombre"`
		Locale   string                   `json:"locale"`
		Carpetas []api.ExternalMailFolder `json:"carpetas"`
		Esperado []struct {
			ID     string `json:"id"`
			Rotulo string `json:"rotulo"`
			Glifo  string `json:"glifo"`
		} `json:"esperado"`
	} `json:"casos"`
}

// B5: las carpetas de una cuenta externa salían con el nombre que da el proveedor
// («INBOX», «SENT», «DRAFT») y en su orden (Gmail, la Papelera antes que Recibidos).
// Ahora se ordenan, se nombran y se dibujan como las de Sytue, con los mismos casos
// que la web.
//
// VISTO EN ROJO (2026-11-06): sin ordenar en `ordenarPorRol`, falla el caso de Gmail
// por los puestos; con `RotuloCarpeta` devolviendo el nombre crudo, sale «INBOX» donde
// la web dice «Recibidos». Deshecho.
func TestLasCarpetasExternasSonLasDeLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/carpetas-externas.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresCarpetasExternas
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 6 {
		t.Fatalf("solo hay %d casos en los vectores", len(v.Casos))
	}
	for _, c := range v.Casos {
		t.Run(c.Nombre, func(t *testing.T) {
			l := i18n.Locale(c.Locale)
			tr, err := Traductor(l)
			if err != nil {
				t.Fatal(err)
			}
			got := ordenarCarpetasExternas(l, c.Carpetas)
			if len(got) != len(c.Esperado) {
				t.Fatalf("salen %d carpetas y se esperaban %d", len(got), len(c.Esperado))
			}
			for i, want := range c.Esperado {
				f := got[i]
				glifo, hay := glifosPropios[want.Glifo]
				if !hay {
					t.Fatalf("el glifo %q de los vectores no está en glifosPropios", want.Glifo)
				}
				rotulo := RotuloCarpeta(tr, f.Name, f.Role)
				if f.ID != want.ID || rotulo != want.Rotulo || GlifoCarpeta(f.Role) != glifo {
					t.Errorf("puesto %d: %q «%s» y la web pinta %q «%s» con el glifo %q", i, f.ID, rotulo, want.ID, want.Rotulo, want.Glifo)
				}
			}
		})
	}
}

// carpetasGmail es lo que da Gmail: en su orden y con el nombre de la etiqueta.
type carpetasGmail struct{ *correoFalso }

func (carpetasGmail) MailFolders(context.Context, string, string) ([]api.ExternalMailFolder, error) {
	return []api.ExternalMailFolder{
		{ID: "Label_1", Name: "Facturas", Unread: 2},
		{ID: "TRASH", Name: "TRASH", Role: "trash"},
		{ID: "SENT", Name: "SENT", Role: "sent"},
		{ID: "INBOX", Name: "INBOX", Role: "inbox", Unread: 3},
	}, nil
}

// Una cuenta abierta sin carpeta («Abrir buzón», o la recién conectada) empieza en su
// Recibidos, aunque Gmail la dé la última, y la lista se titula con la carpeta y la
// cuenta. VISTO EN ROJO (2026-11-06) con la primera carpeta del proveedor en vez de la
// de rol «inbox»: se abría «Facturas».
func TestUnaCuentaAbiertaSinCarpetaEmpiezaEnSuRecibidos(t *testing.T) {
	falso := &correoFalso{cuentas: api.ExternalMailAccounts{Items: []api.ExternalMailAccount{
		{InstanceID: "fixture", Provider: "gmail", Address: "ana@gmail.com", Connected: true, Services: []string{"mail"}},
	}}, carpetas: map[string][]api.ExternalMailFolder{"fixture": {
		{ID: "Label_1", Name: "Facturas", Unread: 2}, {ID: "INBOX", Name: "INBOX", Role: "inbox", Unread: 3},
	}}}
	c := correoDePrueba(t, falso)
	c.b.CorreoExterno = carpetasGmail{falso}
	c.b.cargarExternas()
	c.b.ElegirExterna("fixture", "")
	if v := c.b.externa; v == nil || v.Carpeta != "INBOX" {
		t.Fatalf("se abre %+v y tenía que ser Recibidos", v)
	}
	if want := c.b.P.T.T("folder.inbox", nil) + " · ana@gmail.com"; c.b.P.Bandeja.Titulo != want {
		t.Fatalf("la lista se titula %q; quiero %q", c.b.P.Bandeja.Titulo, want)
	}
}
