// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package tema

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// raizVieja reconoce un comentario que hace las cuentas con el rem de catorce píxeles que
// tuvo la suite: es el mismo patrón que el `grep` de la pieza DS-20. Se arma por partes
// para que no se reconozca a sí mismo y ese `grep` salga vacío también aquí.
const catorce = "14"

var raizVieja = regexp.MustCompile("rem de " + catorce + "|raíz.*" + catorce)

// TestNingunComentarioCalculaConLaRaizDe14 es el guardián de los comentarios de medidas.
// La raíz es `Rem` (16) desde que el tema compartido la subió, pero decenas de
// comentarios seguían diciendo «49» o «31,5» con aquel rem al lado de una constante que
// ya medía 56 o 36: quien comparaba con la web echaba las cuentas con el número viejo y
// veía diferencias que no existían. Una medida de la HOJA se escribe con `Rem`; una
// medida histórica se cuenta como pasada («cuando 1 rem eran 14 px»).
func TestNingunComentarioCalculaConLaRaizDe14(t *testing.T) {
	var malos []string
	err := filepath.WalkDir("..", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Los catálogos se generan de la web y no llevan medidas.
			if d.Name() == "catalogos" || strings.HasPrefix(d.Name(), ".") && ruta != ".." {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(ruta); ext != ".go" && ext != ".md" {
			return nil
		}
		datos, err := os.ReadFile(ruta)
		if err != nil {
			return err
		}
		for i, linea := range strings.Split(string(datos), "\n") {
			if raizVieja.MatchString(linea) {
				malos = append(malos, ruta+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(linea))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range malos {
		t.Errorf("calcula con la raíz vieja; la de la suite es tema.Rem (%v): %s", Rem, m)
	}
}
