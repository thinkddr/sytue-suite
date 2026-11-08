// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/i18n"
)

// vectoresArbol es `testdata/arbol-de-buzones.json`, que recorre también
// `buzones.test.ts` de la web.
type vectoresArbol struct {
	Casos []struct {
		Nombre string `json:"nombre"`
		Locale string `json:"locale"`
		Sytue  []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Role   string `json:"role"`
			Unread int    `json:"unread"`
		} `json:"sytue"`
		Externas []api.ExternalMailAccountSummary `json:"externas"`
		Esperado struct {
			Unificados []struct {
				Rol     string `json:"rol"`
				Fuentes []struct {
					Cuenta  string `json:"cuenta"`
					Carpeta string `json:"carpeta"`
				} `json:"fuentes"`
				NoLeidos int64 `json:"noLeidos"` //nolint:tagliatelle // el JSON es el de la web (camelCase de TS)
			} `json:"unificados"`
			Cuentas []struct {
				ID       string   `json:"id"`
				Titulo   string   `json:"titulo"`
				ICloud   bool     `json:"icloud"`
				Estado   string   `json:"estado"`
				NoLeidos int64    `json:"noLeidos"` //nolint:tagliatelle // el JSON es el de la web (camelCase de TS)
				Carpetas []string `json:"carpetas"`
			} `json:"cuentas"`
		} `json:"esperado"`
	} `json:"casos"`
}

// El árbol de buzones del lateral (fase B de las cuentas externas) es el de la web:
// los mismos unificados, con las mismas fuentes y no leídos, y las mismas cuentas.
//
// VISTO EN ROJO (2026-11-06): sin mirar `Connected`, la cuenta desconectada que traía
// carpetas entraba en la Entrada unificada; sin `ServesMail`, salía la de Calendario;
// sin sumar las carpetas de las cuentas, la Entrada de Gmail e iCloud daba 2 y no 6.
// Deshecho.
func TestElArbolDeBuzonesEsElDeLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/arbol-de-buzones.json")
	if err != nil {
		t.Fatal(err)
	}
	var v vectoresArbol
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 5 {
		t.Fatalf("solo hay %d casos en los vectores", len(v.Casos))
	}
	for _, c := range v.Casos {
		t.Run(c.Nombre, func(t *testing.T) {
			sytue := make([]api.Mailbox, 0, len(c.Sytue))
			for _, s := range c.Sytue {
				sytue = append(sytue, api.Mailbox{ID: s.ID, Name: s.Name, Role: s.Role, UnreadEmails: s.Unread})
			}
			arbol := arbolDeBuzones(i18n.Locale(c.Locale), sytue, c.Externas)
			unificados := make([]BuzonUnificado, 0, len(c.Esperado.Unificados))
			for _, u := range c.Esperado.Unificados {
				fuentes := make([]FuenteDeBuzon, 0, len(u.Fuentes))
				for _, f := range u.Fuentes {
					fuentes = append(fuentes, FuenteDeBuzon(f))
				}
				unificados = append(unificados, BuzonUnificado{Rol: u.Rol, Fuentes: fuentes, NoLeidos: u.NoLeidos})
			}
			if !reflect.DeepEqual(arbol.Unificados, unificados) {
				t.Errorf("unificados:\n  %+v\nla web:\n  %+v", arbol.Unificados, unificados)
			}
			if len(arbol.Cuentas) != len(c.Esperado.Cuentas) {
				t.Fatalf("salen %d cuentas y la web pinta %d", len(arbol.Cuentas), len(c.Esperado.Cuentas))
			}
			for i, want := range c.Esperado.Cuentas {
				got := arbol.Cuentas[i]
				ids := make([]string, 0, len(got.Carpetas))
				for _, f := range got.Carpetas {
					ids = append(ids, f.ID)
				}
				if got.ID != want.ID || got.Titulo != want.Titulo || got.ICloud != want.ICloud || got.Estado != want.Estado || got.NoLeidos != want.NoLeidos || !reflect.DeepEqual(ids, want.Carpetas) {
					t.Errorf("cuenta %d: %+v con carpetas %q; la web pinta %+v", i, got, ids, want)
				}
			}
		})
	}
}
