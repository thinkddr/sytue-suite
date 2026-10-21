// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Command docslinks comprueba que los enlaces INTERNOS de la documentación llevan a algún
// sitio: que el fichero exista y que el ancla exista dentro de él.
//
// Por qué esto y no un formateador. La documentación de este repositorio es la «fuente de
// verdad» declarada —CONVENTIONS fija nombres, puertos, versiones y licencias, y el resto
// se apoya en ella—, así que el defecto que de verdad hace daño es un enlace que miente:
// manda a un ancla que se renombró o a un fichero que se movió, y quien lo sigue se queda
// sin el dato en el momento en que lo necesita. Eso no lo ve ningún compilador.
//
// El formato de estos `.md` lo vigila Prettier aparte (`format:check` en apps/frontend
// incluye `docs/*.md` desde el 2026-09-28): los ficheros ya estaban alineados por Prettier y
// cada edición a mano los desalineaba, así que el diff de la tabla la pagaba el siguiente.
//
// Ejecutar: `make docs-links` (entra en `make gates`).
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	// Enlaces markdown: se captura solo el destino. Se ignoran las imágenes (`!`) porque
	// un `alt` con corchetes rompería la captura y no aportan navegación.
	reEnlace = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	// Anclas explícitas, que son las que sobreviven a un cambio de título.
	reAncla = regexp.MustCompile(`<a\s+id="([^"]+)"`)
	// Encabezados ATX.
	reTitulo = regexp.MustCompile(`(?m)^#{1,6}\s+(.+?)\s*$`)
	// Bloques cercados y `código` en línea: dentro va SINTAXIS de ejemplo, no enlaces.
	reCercado = regexp.MustCompile("(?s)```.*?```|~~~.*?~~~")
	reEnLinea = regexp.MustCompile("`[^`\n]*`")
)

// slug replica el identificador que GitHub y Gitea generan para un encabezado: minúsculas,
// fuera todo lo que no sea letra, dígito, espacio, guion o guion bajo (las tildes y la ñ SÍ
// se conservan) y los espacios a guiones.
func slug(titulo string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(titulo) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// sinCodigo sustituye el código por espacios en blanco del mismo tamaño: así los ejemplos
// de sintaxis no cuentan como enlaces y los números de línea siguen cuadrando.
func sinCodigo(s string) string {
	blanquear := func(m string) string {
		var b strings.Builder
		for _, r := range m {
			if r == '\n' {
				b.WriteRune('\n')
			} else {
				b.WriteRune(' ')
			}
		}
		return b.String()
	}
	return reEnLinea.ReplaceAllStringFunc(reCercado.ReplaceAllStringFunc(s, blanquear), blanquear)
}

func main() {
	raiz, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "docslinks:", err)
		os.Exit(2)
	}
	ficheros, err := markdownDelRepo(raiz)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docslinks:", err)
		os.Exit(2)
	}
	sort.Strings(ficheros)

	anclas, cuerpos, err := indexar(ficheros)
	if err != nil {
		fmt.Fprintln(os.Stderr, "docslinks:", err)
		os.Exit(2)
	}

	fallos := revisar(raiz, ficheros, anclas, cuerpos)
	if len(fallos) > 0 {
		fmt.Fprintf(os.Stderr, "docslinks: %d enlace(s) interno(s) roto(s):\n", len(fallos))
		for _, x := range fallos {
			fmt.Fprintln(os.Stderr, "  "+x)
		}
		os.Exit(1)
	}
	fmt.Printf("docslinks: %d documentos, todos los enlaces internos resuelven.\n", len(ficheros))
}

// indexar lee cada documento una vez y saca sus anclas: las explícitas (`<a id>`) y el slug
// de cada encabezado. Se hace ANTES de comprobar nada porque un enlace puede apuntar a
// cualquier otro documento del repositorio.
func indexar(ficheros []string) (map[string]map[string]bool, map[string]string, error) {
	anclas := make(map[string]map[string]bool, len(ficheros))
	cuerpos := make(map[string]string, len(ficheros))
	for _, f := range ficheros {
		raw, err := os.ReadFile(f)
		if err != nil {
			return nil, nil, err
		}
		texto := string(raw)
		cuerpos[f] = texto
		conjunto := map[string]bool{}
		for _, m := range reAncla.FindAllStringSubmatch(texto, -1) {
			conjunto[m[1]] = true
		}
		for _, m := range reTitulo.FindAllStringSubmatch(sinCercados(texto), -1) {
			conjunto[slug(m[1])] = true
		}
		anclas[f] = conjunto
	}
	return anclas, cuerpos, nil
}

// revisar devuelve un fallo por cada enlace interno que no resuelve, con fichero y línea.
func revisar(raiz string, ficheros []string, anclas map[string]map[string]bool, cuerpos map[string]string) []string {
	var fallos []string
	for _, f := range ficheros {
		texto := sinCodigo(cuerpos[f])
		for _, m := range reEnlace.FindAllStringSubmatchIndex(texto, -1) {
			destino := texto[m[2]:m[3]]
			if esExterno(destino) {
				continue
			}
			if fallo := comprobar(raiz, f, destino, 1+strings.Count(texto[:m[0]], "\n"), anclas); fallo != "" {
				fallos = append(fallos, fallo)
			}
		}
	}
	return fallos
}

// comprobar resuelve UN enlace; devuelve "" si está bien.
func comprobar(raiz, origen, destino string, linea int, anclas map[string]map[string]bool) string {
	ruta, frag := partir(destino)
	objetivo := origen
	if ruta != "" {
		objetivo = filepath.Join(filepath.Dir(origen), ruta)
		if _, err := os.Stat(objetivo); err != nil {
			return fmt.Sprintf("%s:%d: no existe %q", rel(raiz, origen), linea, ruta)
		}
	}
	// Solo se puede comprobar el ancla de un markdown; en otro fichero el fragmento no
	// significa nada que este programa pueda verificar.
	if frag == "" || !strings.EqualFold(filepath.Ext(objetivo), ".md") {
		return ""
	}
	if !anclas[objetivo][frag] {
		return fmt.Sprintf("%s:%d: %q no tiene el ancla #%s", rel(raiz, origen), linea, ruta, frag)
	}
	return ""
}

// markdownDelRepo lista los `.md` que git tiene en seguimiento. Se pregunta a git y no se
// recorre el disco porque en el árbol hay documentación VENDORIZADA —los proveedores que
// OpenTofu descarga en `.terraform/`, por ejemplo— cuyos enlaces roto no son nuestros ni
// podemos arreglar. Lo que git ignora, no es documentación de este repositorio.
func markdownDelRepo(raiz string) ([]string, error) {
	cmd := exec.CommandContext(context.Background(), "git", "ls-files", "-z", "*.md")
	cmd.Dir = raiz
	salida, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var out []string
	for _, b := range bytes.Split(salida, []byte{0}) {
		if len(b) == 0 {
			continue
		}
		// Lo VENDORIZADO tampoco es documentación de este repositorio, aunque sí
		// esté versionado: el README y el Changelog del chart de Traefik enlazan
		// a ficheros de su propio repositorio que aquí no existen. Eran cinco
		// enlaces rotos ajenos que dejaban este gate en rojo permanente, y un
		// gate que siempre está rojo no lo mira nadie: el que sí era nuestro
		// —un enlace a `.sygit/`, la ruta de workflows de antes de git.gay—
		// llevaba tapado debajo.
		if strings.Contains(string(b), "/.vendor/") {
			continue
		}
		out = append(out, filepath.Join(raiz, string(b)))
	}
	return out, nil
}

// sinCercados quita solo los bloques cercados: un `# comentario` de un ejemplo en shell no
// es un encabezado del documento.
func sinCercados(s string) string {
	return reCercado.ReplaceAllString(s, "")
}

func esExterno(destino string) bool {
	return strings.HasPrefix(destino, "http://") || strings.HasPrefix(destino, "https://") ||
		strings.HasPrefix(destino, "mailto:") || strings.HasPrefix(destino, "tel:")
}

func partir(destino string) (ruta, frag string) {
	if i := strings.IndexByte(destino, '#'); i >= 0 {
		return destino[:i], destino[i+1:]
	}
	return destino, ""
}

func rel(raiz, f string) string {
	if r, err := filepath.Rel(raiz, f); err == nil {
		return r
	}
	return f
}
