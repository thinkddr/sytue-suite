// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cicontratos

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/tools/guiontest"
)

// Las excepciones de govulncheck perdonan solo lo que está en la lista y en fecha.

// govulncheckExcepcionesAlcanza es una salida como la de govulncheck para esos
// avisos alcanzados, con un aviso de módulo aparte (GO-2026-9999).
func govulncheckExcepcionesAlcanza(ids ...string) string {
	var b strings.Builder
	b.WriteString("=== Symbol Results ===\n\n")
	for _, id := range ids {
		b.WriteString("Vulnerability #1: " + id + "\n    algo\n\n")
	}
	b.WriteString("=== Module Results ===\n\nVulnerability #1: GO-2026-9999")
	return b.String()
}

// govulncheckExcepcionesBanco deja un govulncheck de mentira, que imprime lo que
// se le diga y sale con el código que se le diga, y una lista de una excepción.
// Devuelve una función que corre la acción real en esa fecha.
func govulncheckExcepcionesBanco(t *testing.T) func(t *testing.T, hoy, salida string, codigo int) guiontest.Resultado {
	t.Helper()
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	guiontest.Doble(t, bin, "govulncheck", `printf '%s\n' "$SALIDA_FALSA"
exit "${CODIGO_FALSO:-3}"`)
	lista := filepath.Join(tmp, "lista.tsv")
	guiontest.Escribir(t, lista, "# id\tcaduca\tmotivo\nGO-2026-0001\t2026-12-14\tmitigado\n")
	return func(t *testing.T, hoy, salida string, codigo int) guiontest.Resultado {
		t.Helper()
		return guiontest.Guion(t, guiontest.Opciones{Env: []string{
			guiontest.ConPATH(bin), "SALIDA_FALSA=" + salida, "CODIGO_FALSO=" + strconv.Itoa(codigo),
			"SYTUE_HOY=" + hoy, "SYTUE_GOVULNCHECK_EXCEPCIONES=" + lista,
		}}, ".github/actions/govulncheck/accion.sh", "./...")
	}
}

func TestGovulncheckExcepcionesPerdonanSoloLoListadoYEnFecha(t *testing.T) {
	correr := govulncheckExcepcionesBanco(t)

	t.Run("una excepción en fecha pasa y dice hasta cuándo", func(t *testing.T) {
		r := correr(t, "2026-11-06", govulncheckExcepcionesAlcanza("GO-2026-0001"), 3)
		if r.Codigo != 0 {
			t.Errorf("una excepción en fecha debería pasar: %s", r.Todo())
		}
		if !strings.Contains(r.Todo(), "aceptado hasta 2026-12-14") {
			t.Errorf("no dice hasta cuándo se acepta: %s", r.Todo())
		}
	})
	t.Run("el último día todavía vale", func(t *testing.T) {
		if r := correr(t, "2026-12-14", govulncheckExcepcionesAlcanza("GO-2026-0001"), 3); r.Codigo != 0 {
			t.Errorf("el último día todavía vale: %s", r.Todo())
		}
	})
	t.Run("una excepción caducada no pasa y lo explica", func(t *testing.T) {
		r := correr(t, "2026-12-15", govulncheckExcepcionesAlcanza("GO-2026-0001"), 3)
		if r.Codigo == 0 {
			t.Error("una excepción caducada no debe pasar")
		}
		if !strings.Contains(r.Todo(), "caducó") {
			t.Errorf("no explica que caducó: %s", r.Todo())
		}
	})
	t.Run("un aviso sin excepción junto a uno con ella no pasa", func(t *testing.T) {
		r := correr(t, "2026-11-06", govulncheckExcepcionesAlcanza("GO-2026-0001", "GO-2026-0002"), 3)
		if r.Codigo == 0 {
			t.Error("un aviso sin excepción junto a uno con ella no debe pasar")
		}
		if !strings.Contains(r.Todo(), "GO-2026-0002: alcanzado y sin excepción") {
			t.Errorf("no nombra el aviso sin excepción: %s", r.Todo())
		}
		if strings.Contains(r.Todo(), "GO-2026-9999") && strings.Contains(r.Todo(), "::error::GO-2026-9999") {
			t.Errorf("un aviso solo de módulo no se juzga: %s", r.Todo())
		}
	})
	t.Run("un 3 sin avisos legibles no pasa", func(t *testing.T) {
		if r := correr(t, "2026-11-06", "algo raro", 3); r.Codigo == 0 {
			t.Error("un 3 sin avisos legibles no debe pasar")
		}
	})
	t.Run("un verde sigue verde", func(t *testing.T) {
		if r := correr(t, "2026-11-06", "No vulnerabilities found.", 0); r.Codigo != 0 {
			t.Errorf("un verde sigue verde: %s", r.Todo())
		}
	})
	t.Run("otro código de salida pasa tal cual, con la salida intacta", func(t *testing.T) {
		r := correr(t, "2026-11-06", "source-processing packages", 1)
		if r.Codigo != 1 {
			t.Errorf("otro código de salida debe pasar tal cual: salió con %d", r.Codigo)
		}
		if !strings.Contains(r.Todo(), "source-processing packages") {
			t.Errorf("la salida debe llegar intacta a quien llama: %s", r.Todo())
		}
	})
	t.Run("sin cabeceras se juzgan todos los avisos", func(t *testing.T) {
		if r := correr(t, "2026-11-06", "Vulnerability #1: GO-2026-0003", 3); r.Codigo == 0 {
			t.Error("sin cabeceras se juzgan todos los avisos")
		}
	})
}

// La lista real: cada fila con id, fecha y motivo, y en osv-scanner.toml también.
func TestGovulncheckExcepcionesLaListaReal(t *testing.T) {
	esID := regexp.MustCompile(`^GO-\d{4}-\d+$`)
	esFecha := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	osv := strings.Split(guiontest.Leer(t, "osv-scanner.toml"), "\n")
	for _, fila := range strings.Split(guiontest.Leer(t, "compliance/supply-chain/govulncheck-excepciones.tsv"), "\n") {
		campos := append(strings.SplitN(fila, "\t", 3), "", "")
		id, caduca, motivo := campos[0], campos[1], campos[2]
		if id == "" || strings.HasPrefix(id, "#") {
			continue
		}
		if !esID.MatchString(id) {
			t.Errorf("id raro en la lista: %s", id)
		}
		if !esFecha.MatchString(caduca) {
			t.Errorf("%s: fecha rara: %s", id, caduca)
		}
		if motivo == "" {
			t.Errorf("%s: sin motivo", id)
		}
		enOSV := false
		for _, linea := range osv {
			enOSV = enOSV || strings.HasPrefix(linea, `id = "`+id+`"`)
		}
		if !enOSV {
			t.Errorf("%s: falta en osv-scanner.toml", id)
		}
	}
}
