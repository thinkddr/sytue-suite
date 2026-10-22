// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package escritorio

import "strings"

// # Las hermanas en Windows
//
// Cada BUNDLE de la suite («Sytue One», «Sytue Drive») se instala en su carpeta con su
// instalador de NSIS, que pone todas sus apps y deja su entrada en
// `HKCU\…\Uninstall\eu.sytue.bundle.<clave>` con la versión y la carpeta
// (`empaquetar.sh`). Las hermanas son las apps de las OTRAS carpetas: las de la propia
// van en el mismo bundle y las pone su mismo instalador, así que contarlas lanzaría ese
// instalador una vez por cada una. Se ponen al día con el instalador de su bundle en
// silencio (`/S`), que recuerda la carpeta (`InstallDirRegKey`) y aparta el ejecutable si
// está en uso. Se escribe aquí, sin etiqueta de sistema, para que su guardián corra en
// Linux: lo único de Windows es leer el registro y lanzar el instalador.

// entradaNSIS es lo que deja el instalador de una app en su clave de desinstalación.
type entradaNSIS struct {
	Clave, Carpeta, Version string
}

// hermanasNSIS son las apps de la suite de esas entradas, sin contar las de la carpeta
// de `exe` (esta y las de su bundle). `ejecutables` dice qué `sytue-*.exe` hay en una
// carpeta.
func hermanasNSIS(exe string, entradas []entradaNSIS, ejecutables func(carpeta string) []string) []Hermana {
	// Con las dos barras y sin `filepath`: el guardián corre en Linux con rutas de Windows.
	propia := exe[:max(strings.LastIndexAny(exe, `\/`), 0)]
	var out []Hermana
	for _, e := range entradas {
		// En Windows las rutas no distinguen mayúsculas.
		if !strings.HasPrefix(e.Clave, "eu.sytue.") || e.Carpeta == "" ||
			strings.EqualFold(strings.TrimRight(e.Carpeta, `\/`), propia) {
			continue
		}
		for _, x := range ejecutables(e.Carpeta) {
			if strings.EqualFold(x, exe) {
				continue
			}
			if app := AppDeEjecutable(x); app != "" {
				out = append(out, Hermana{App: app, Destino: x, Version: e.Version})
			}
		}
	}
	return out
}

// argsSilencio son los argumentos del instalador de NSIS para ponerse al día sin
// asistente. La carpeta no hace falta: la recuerda el propio instalador.
func argsSilencio() []string { return []string{"/S"} }
