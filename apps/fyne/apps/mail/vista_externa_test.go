// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package mail

import (
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// «Todos los buzones de entrada» mezcla como la web: los vectores de
// `testdata/mezcla-de-entradas.json` los recorre también `vistaExterna.test.ts`.
//
// VISTO EN ROJO (2026-11-06): sin la parada de la fuente vacía con páginas, la Entrada
// de Sytue salía entera antes de pedir la segunda página de Gmail; eligiendo con
// `!Before` en vez de `After`, el empate lo ganaba la cuenta y no Sytue. Deshecho.
func TestLaMezclaDeEntradasEsLaDeLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/mezcla-de-entradas.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Casos []struct {
			Nombre  string `json:"nombre"`
			Fuentes []struct {
				Clave      string `json:"clave"`
				Cursor     string `json:"cursor"`
				Pendientes []struct {
					ID    string `json:"id"`
					Fecha string `json:"fecha"`
				} `json:"pendientes"`
			} `json:"fuentes"`
			Salida   []string `json:"salida"`
			PorPedir []string `json:"porPedir"` //nolint:tagliatelle // el JSON es el de la web (camelCase de TS)
			Quedan   bool     `json:"quedan"`
		} `json:"casos"`
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Casos) < 6 {
		t.Fatalf("solo %d casos", len(v.Casos))
	}
	for _, c := range v.Casos {
		t.Run(c.Nombre, func(t *testing.T) {
			var fuentes []fuenteMezcla
			for _, f := range c.Fuentes {
				fm := fuenteMezcla{clave: f.Clave, cursor: f.Cursor}
				for _, p := range f.Pendientes {
					fm.pendientes = append(fm.pendientes, api.MailMessage{ID: p.ID, ReceivedAt: fechaExterna(p.Fecha)})
				}
				fuentes = append(fuentes, fm)
			}
			antes := make([]int, len(fuentes))
			for i, f := range fuentes {
				antes[i] = len(f.pendientes)
			}
			salida, quedan := mezclarPorFecha(fuentes)
			ids := make([]string, 0, len(salida))
			for _, m := range salida {
				ids = append(ids, m.ID)
			}
			if !slices.Equal(ids, c.Salida) {
				t.Errorf("salida %v, quería %v", ids, c.Salida)
			}
			if got := porPedir(quedan); !slices.Equal(got, c.PorPedir) && (len(got) > 0 || len(c.PorPedir) > 0) {
				t.Errorf("por pedir %v, quería %v", got, c.PorPedir)
			}
			if got := quedanEnMezcla(quedan); got != c.Quedan {
				t.Errorf("quedan %v, quería %v", got, c.Quedan)
			}
			for i, f := range fuentes {
				if len(f.pendientes) != antes[i] {
					t.Errorf("tocó la fuente %q que recibió", f.clave)
				}
			}
		})
	}
}

// VISTO EN ROJO (2026-11-06) sin `QueryEscape`: un id de IMAP con «:» se partía en otra
// carpeta. Deshecho.
func TestElIdExternoVuelveASuOrigen(t *testing.T) {
	o := OrigenExterno{Cuenta: "inst-1", Carpeta: "INBOX/Facturas", ID: "42:7"}
	id := idExterno(o)
	if got, ok := origenExterno(id); !ok || got != o {
		t.Fatalf("%q volvió como %+v, %v", id, got, ok)
	}
	for _, id := range []string{"0b8a3b1e-6f0e-4d33-9a4e-5b0f0b7c1d2e", "ext:solo:dos", "ext:%zz:x:y"} {
		if _, ok := origenExterno(id); ok {
			t.Errorf("%q no es externo", id)
		}
	}
}

func TestElSobreExternoEsUnaConversacionDeUno(t *testing.T) {
	m := sobreExterno("inst-1", "INBOX", api.ExternalMailMessage{
		ID: "7", Subject: "Factura", Date: "2026-11-06T10:00:00+02:00", Snippet: "Adjunto",
		From:    []api.ExternalMailAddress{{Name: "Ana", Email: "ana@example.com"}},
		To:      []api.ExternalMailAddress{{Email: "yo@example.com"}},
		Flagged: true, Attachments: []api.ExternalMailAttachment{{Name: "f.pdf"}},
	})
	want := api.MailMessage{
		ID: idExterno(OrigenExterno{Cuenta: "inst-1", Carpeta: "INBOX", ID: "7"}), MailboxID: "INBOX",
		Subject: "Factura", Preview: "Adjunto", From: "ana@example.com", FromName: "Ana",
		To: []string{"yo@example.com"}, Keywords: []string{api.MailFlagFlagged}, HasAttachment: true,
		ReceivedAt: time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC),
	}
	if !m.ReceivedAt.Equal(want.ReceivedAt) {
		t.Fatalf("fecha %v", m.ReceivedAt)
	}
	m.ReceivedAt = want.ReceivedAt
	if !reflect.DeepEqual(m, want) {
		t.Fatalf("%+v\nquería %+v", m, want)
	}
	if c := AgruparPorHilo([]Mensaje{MensajeDe(m)}); len(c) != 1 || c[0].HiloID != "msg:"+m.ID || c[0].SinLeer != true {
		t.Fatalf("conversación %+v", c)
	}
}

// Qué buzones unificados se abren mezclados, qué carpetas pide cada uno y qué abre una
// fila, como la web: los vectores de `testdata/buzones-mezclados.json` los recorre
// también `vistaExterna.test.ts`.
//
// VISTO EN ROJO (2026-11-07): con fuentesDeUnificado buscando siempre «inbox», los
// enviados pedían las Entradas; con abreRedactor mirando solo el rol de Sytue, el
// borrador de Sytue en «Todos los borradores» se abría en el lector; y sin mirar la
// búsqueda, un recibido buscado desde «Todos los borradores» abría el redactor. Deshecho.
func TestLosBuzonesMezcladosSonLosDeLaWeb(t *testing.T) {
	crudo, err := os.ReadFile("testdata/buzones-mezclados.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Mezclados   []string `json:"mezclados"`
		NoMezclados []string `json:"noMezclados"` //nolint:tagliatelle // el JSON es el de la web (camelCase de TS)
		Unificados  []struct {
			Rol     string `json:"rol"`
			Fuentes []struct {
				Cuenta  string `json:"cuenta"`
				Carpeta string `json:"carpeta"`
			} `json:"fuentes"`
		} `json:"unificados"`
		Fuentes []struct {
			Rol      string   `json:"rol"`
			Carpetas []string `json:"carpetas"`
		} `json:"fuentes"`
		Redactor []struct {
			Nombre string `json:"nombre"`
			Vista  *struct {
				Tipo    string `json:"tipo"`
				Rol     string `json:"rol"`
				Cuenta  string `json:"cuenta"`
				Carpeta string `json:"carpeta"`
			} `json:"vista"`
			RolSytue   string `json:"rolSytue"` //nolint:tagliatelle // el JSON es el de la web (camelCase de TS)
			Externo    bool   `json:"externo"`
			EnBusqueda bool   `json:"enBusqueda"` //nolint:tagliatelle // el JSON es el de la web (camelCase de TS)
			Abre       bool   `json:"abre"`
		} `json:"redactor"`
	}
	if err := json.Unmarshal(crudo, &v); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(rolesMezclados, v.Mezclados) {
		t.Fatalf("rolesMezclados %q; la web mezcla %q", rolesMezclados, v.Mezclados)
	}
	for _, rol := range v.Mezclados {
		if !esMezclado(rol) {
			t.Errorf("%q no se mezcla", rol)
		}
	}
	for _, rol := range v.NoMezclados {
		if esMezclado(rol) {
			t.Errorf("%q se mezcla", rol)
		}
	}
	unificados := make([]BuzonUnificado, 0, len(v.Unificados))
	for _, u := range v.Unificados {
		bu := BuzonUnificado{Rol: u.Rol}
		for _, f := range u.Fuentes {
			bu.Fuentes = append(bu.Fuentes, FuenteDeBuzon{Cuenta: f.Cuenta, Carpeta: f.Carpeta})
		}
		unificados = append(unificados, bu)
	}
	for _, c := range v.Fuentes {
		t.Run("fuentes de "+c.Rol, func(t *testing.T) {
			fuentes := fuentesDeUnificado(unificados, c.Rol)
			got := make([]string, 0, len(fuentes))
			for _, f := range fuentes {
				got = append(got, f.Cuenta+"/"+f.Carpeta)
			}
			if !slices.Equal(got, c.Carpetas) {
				t.Fatalf("pide %q; quiero %q", got, c.Carpetas)
			}
			// Una copia: el refresco no puede tocar el árbol del lateral.
			fuentes[0].Carpeta = "tocada"
			if fuentesDeUnificado(unificados, c.Rol)[0].Carpeta == "tocada" {
				t.Fatal("fuentesDeUnificado devuelve el árbol, no una copia")
			}
		})
	}
	if got := fuentesDeUnificado(nil, "sent"); len(got) != 0 {
		t.Fatalf("sin carpetas de ese rol se pide %v", got)
	}
	for _, c := range v.Redactor {
		t.Run(c.Nombre, func(t *testing.T) {
			var vista *VistaExterna
			if c.Vista != nil {
				vista = &VistaExterna{Unificado: c.Vista.Rol, Cuenta: c.Vista.Cuenta, Carpeta: c.Vista.Carpeta}
			}
			id := "0b8a3b1e-6f0e-4d33-9a4e-5b0f0b7c1d2e"
			if c.Externo {
				id = idExterno(OrigenExterno{Cuenta: "gmail", Carpeta: "DRAFT", ID: "r1"})
			}
			if got := abreRedactor(vista, c.RolSytue, id, c.EnBusqueda); got != c.Abre {
				t.Fatalf("abreRedactor = %v; quiero %v", got, c.Abre)
			}
		})
	}
}
