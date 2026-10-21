// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package notes

import (
	"encoding/json"
	"regexp"
	"slices"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestElIconoSeGuardaSinPerderElRestoDeData: elegir un icono manda `data` entero con
// `icon` cambiado —el PUT sustituye— y «Sin icono» quita la clave, que es lo que deja
// el `icon: icono || undefined` de la web al pasar por JSON.stringify.
//
// VISTO EN ROJO (2026-09-23, en el frontal): armando `data` solo con `icon`, los
// adjuntos de la nota se perdían al ponerle un icono.
func TestElIconoSeGuardaSinPerderElRestoDeData(t *testing.T) {
	original := api.Note{
		ID: "n1", NotebookID: "c1", Kind: "text",
		Data: json.RawMessage(`{"icon":"pin","due":"2026-10-01"}`),
	}
	p, falso := pantallaConServicio(t, map[string]string{
		"PUT /notes/n1": `{"id":"n1","notebook_id":"c1","kind":"text","data":{"icon":"quote","due":"2026-10-01"}}`,
	}, original)
	if err := p.ponerIcono("n1", "quote"); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(falso.ultima(t, "PUT /notes/n1")["data"])
	if string(got) != `{"due":"2026-10-01","icon":"quote"}` {
		t.Errorf("poner el icono manda data %s", got)
	}
	if n, _ := p.nota("n1"); n.Icono != "quote" {
		t.Errorf("la nota se queda con el icono %q", n.Icono)
	}
	sin, err := conIcono(json.RawMessage(`{"icon":"pin","due":"x"}`), "")
	if err != nil || string(sin) != `{"due":"x"}` {
		t.Errorf("«Sin icono» deja data %s (%v)", sin, err)
	}
}

// TestElSelectorDeIconosEsElDeLaWeb: los iconos y su orden son los de
// ICONOS_PREDEFINIDOS en IconPickerModal.tsx, cada uno con su glifo portado y su rótulo
// en el catálogo (las claves se arman, así que el guardián central no las ve).
//
// VISTO EN ROJO (2026-09-23, en el frontal): quitando `"filter"` de IconosNota dice que
// el orden no es el de la web, y quitando `"send"` de Glifos, que ese icono no se pinta.
func TestElSelectorDeIconosEsElDeLaWeb(t *testing.T) {
	fuente := webref.Read(t, "apps/frontend/apps/notes/src/components/IconPickerModal.tsx")
	coincidencias := regexp.MustCompile(`\{ id: '([^']*)'`).FindAllStringSubmatch(fuente, -1)
	web := make([]string, 0, len(coincidencias))
	for _, m := range coincidencias {
		web = append(web, m[1])
	}
	if !slices.Equal(web, IconosNota) {
		t.Errorf("los iconos son\n%q\ny en la web\n%q", IconosNota, web)
	}
	tr := trInterno(t)
	for _, id := range IconosNota {
		if id != "" && GlifoPorNombre(id) == "" {
			t.Errorf("el icono %q no tiene glifo: la nota que lo lleve no lo pinta", id)
		}
		if k := RotuloIcono(id); tr.T(k, nil) == k {
			t.Errorf("la clave %q no está en el catálogo", k)
		}
	}
}
