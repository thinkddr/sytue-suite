// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package camara

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Una firma JNI que no coincide con el Java compila en los dos lados y revienta en el
// teléfono con NoSuchMethodError al pulsar el botón. Nadie compila Android en los
// tests: esto compara las firmas que Go pide con los métodos que el Java declara.
func TestLasFirmasJNICoincidenConElJava(t *testing.T) {
	tipos := map[string]string{
		"V": "void", "I": "int", "Z": "boolean", "[B": "byte[]",
		"Ljava/lang/String;": "String", "Landroid/content/Context;": "Context",
	}
	uno := regexp.MustCompile(`\[B|[VIZ]|L[^;]+;`)
	pedida := regexp.MustCompile(`GetStaticMethodID\(env, cls, "(\w+)", "\(([^)]*)\)([^"]+)"\)`)
	for goF, clase := range map[string]string{
		"camara_android.go": "Camara", "pantalla_android.go": "Pantalla",
		// La pantalla encendida de Cast: vive en `sesion`, pero es la misma trampa.
		"../../sesion/encendida_android.go":      "Encendida",
		"../../sesion/atras_android.go":          "Atras",
		"../../sesion/teclado_android.go":        "Teclado",
		"../../sesion/imagenenimagen_android.go": "ImagenEnImagen",
	} {
		g, err := os.ReadFile(goF)
		if err != nil {
			t.Fatal(err)
		}
		j, err := os.ReadFile("../../android/java/eu/sytue/nativo/" + clase + ".java")
		if err != nil {
			t.Fatal(err)
		}
		ms := pedida.FindAllStringSubmatch(string(g), -1)
		if len(ms) == 0 {
			t.Fatalf("%s no pide ningún método: ¿ha cambiado la forma de llamar?", goF)
		}
		for _, m := range ms {
			args := uno.FindAllString(m[2], -1)
			ps := make([]string, 0, len(args))
			for _, p := range args {
				ps = append(ps, tipos[p])
			}
			ret := tipos[m[3]]
			// Los parámetros por tipo; los nombres dan igual.
			java := `static[\w ]* ` + regexp.QuoteMeta(ret) + ` ` + m[1] + `\(`
			for i, p := range ps {
				if i > 0 {
					java += `, `
				}
				java += `(?:final )?` + regexp.QuoteMeta(p) + ` \w+`
			}
			if !regexp.MustCompile(java + `\)`).Match(j) {
				t.Errorf("%s pide %s.%s(%s) %s y el Java no lo declara", goF, clase, m[1], strings.Join(ps, ", "), ret)
			}
		}
	}
}
