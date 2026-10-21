// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package hub

import (
	"maps"
	"regexp"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
	"git.sytue.eu/sytue/one/apps/nucleo/webref"
)

// TestLaFichaDelInstaladorEsLaDeLaWeb: el `ARQUITECTURA` de descargas.astro y el de
// Fyne son el mismo, y la ficha va en su orden: plataforma, procesador y tamaño. Hasta el
// 2026-09-28 Fyne no decía el procesador y «Linux» a secas no dice si arranca en un ARM.
// VISTO EN ROJO sin la arquitectura en `MetaInstalador` y con «x86-64» en el mapa.
func TestLaFichaDelInstaladorEsLaDeLaWeb(t *testing.T) {
	pagina := webref.Read(t, "apps/frontend/apps/portal/src/pages/descargas.astro")
	bloque := regexp.MustCompile(`(?s)const ARQUITECTURA[^{]*\{(.*?)\};`).FindStringSubmatch(pagina)
	if bloque == nil {
		t.Fatal("no encuentro `ARQUITECTURA` en descargas.astro")
	}
	web := map[string]string{}
	for _, m := range regexp.MustCompile(`(\w+): '([^']+)'`).FindAllStringSubmatch(bloque[1], -1) {
		web[m[1]] = m[2]
	}
	if !maps.Equal(web, NombreArquitectura) {
		t.Errorf("la web escribe %v y Fyne %v", web, NombreArquitectura)
	}
	i := api.Instalador{Plataforma: "macos", Arquitectura: "universal", Bytes: 84 << 20}
	if got := MetaInstalador("es", i); got != "macOS · Universal · 84 MB" {
		t.Errorf("la ficha dice %q", got)
	}
}
