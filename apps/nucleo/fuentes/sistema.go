// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package fuentes

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// DelSistema busca en el equipo la letra de una familia que la suite NO trae —Arial,
// Georgia, Times New Roman, Courier New, JetBrains Mono, Inter—, que es lo que hace el
// navegador con el `font-family` de la web: pinta con la que haya instalada. Devuelve
// los bytes del .ttf de esa cara, o false si no está (y entonces quien llama cae en la
// de reserva, como el `sans-serif`/`monospace` del final de la pila de la web).
//
// Arial, Georgia, Times y Courier NO se pueden llevar dentro: su licencia no lo permite.
// Por eso se leen del sistema, y solo de sus carpetas de letras.
func DelSistema(familia string, negrita, cursiva bool) ([]byte, bool) {
	// La familia la escribe el documento (`font-family`): con «../» la ruta salía de
	// las carpetas de letras (FYE-06). Un nombre de familia no lleva separadores.
	if !familiaValida(familia) {
		return nil, false
	}
	clave := familia + "|" + estilo(negrita, cursiva)
	cache.Lock()
	defer cache.Unlock()
	if d, ok := cache.m[clave]; ok {
		return d, d != nil
	}
	var datos []byte
	for _, ruta := range candidatos(runtime.GOOS, carpetas(), familia, negrita, cursiva) {
		if d, err := leerLetra(ruta); err == nil && esTTF(d) {
			datos = d
			break
		}
	}
	cache.m[clave] = datos
	return datos, datos != nil
}

// familiaValida acepta lo que puede ser un nombre de familia: corto, sin rutas ni
// caracteres de control.
func familiaValida(familia string) bool {
	if familia == "" || len(familia) > 100 || strings.Contains(familia, "..") {
		return false
	}
	return !strings.ContainsFunc(familia, func(r rune) bool {
		return r == '/' || r == '\\' || r == ':' || r < 0x20 || r == 0x7f
	})
}

// maxLetra acota lo que se lee de un fichero de letra: las de sistema rondan unos
// MiB, y las CJK más grandes, unas decenas.
const maxLetra = 64 << 20

// leerLetra lee un fichero de letra solo si es un fichero normal y de un tamaño
// razonable: antes se leía entero lo que hubiera (un FIFO colgaba el pintado).
func leerLetra(ruta string) ([]byte, error) {
	fi, err := os.Stat(ruta)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > maxLetra {
		return nil, os.ErrInvalid
	}
	return os.ReadFile(ruta)
}

var cache = struct {
	sync.Mutex
	m map[string][]byte
}{m: map[string][]byte{}}

// esTTF mira la firma: TrueType (0x00010000 o «true») u OpenType con contornos TrueType.
// Una colección (.ttc) o un WOFF no le sirven al pintor de texto.
func esTTF(d []byte) bool {
	if len(d) < 4 {
		return false
	}
	s := string(d[:4])
	return s == "\x00\x01\x00\x00" || s == "true" || s == "OTTO"
}

func estilo(negrita, cursiva bool) string {
	switch {
	case negrita && cursiva:
		return "bi"
	case negrita:
		return "b"
	case cursiva:
		return "i"
	}
	return ""
}

// carpetas son las de letras del sistema y del usuario de este equipo.
func carpetas() []string {
	casa, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/System/Library/Fonts/Supplemental", "/System/Library/Fonts", "/Library/Fonts",
			filepath.Join(casa, "Library", "Fonts"),
		}
	case "windows":
		win := os.Getenv("WINDIR")
		if win == "" {
			win = `C:\Windows`
		}
		return []string{
			filepath.Join(win, "Fonts"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"),
		}
	}
	return []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(casa, ".local", "share", "fonts")}
}

// ficherosWindows son los nombres cortos de las cuatro caras de las familias de
// Windows: normal, negrita, cursiva y las dos.
var ficherosWindows = map[string][4]string{
	"Arial":           {"arial", "arialbd", "ariali", "arialbi"},
	"Georgia":         {"georgia", "georgiab", "georgiai", "georgiaz"},
	"Times New Roman": {"times", "timesbd", "timesi", "timesbi"},
	"Courier New":     {"cour", "courbd", "couri", "courbi"},
}

// candidatos son las rutas donde puede estar esa cara, en orden. En Linux, además,
// la que diga fontconfig —la que usa el navegador, que puede ser una sustituta
// métrica como Liberation Sans para Arial—.
func candidatos(goos string, dirs []string, familia string, negrita, cursiva bool) []string {
	i := 0
	if negrita {
		i++
	}
	if cursiva {
		i += 2
	}
	var nombres []string
	// El nombre «de Mac»: «Georgia Bold Italic.ttf».
	mac := [4]string{"", " Bold", " Italic", " Bold Italic"}[i]
	nombres = append(nombres, familia+mac+".ttf")
	// El de las familias que se instalan a mano: «JetBrainsMono-BoldItalic.ttf».
	guion := [4]string{"-Regular", "-Bold", "-Italic", "-BoldItalic"}[i]
	nombres = append(nombres, strings.ReplaceAll(familia, " ", "")+guion+".ttf")
	if w, ok := ficherosWindows[familia]; ok {
		nombres = append(nombres, w[i]+".ttf")
	}
	var out []string
	for _, d := range dirs {
		for _, n := range nombres {
			out = append(out, filepath.Join(d, n))
		}
	}
	if goos == "linux" || goos == "freebsd" {
		if ruta := fcMatch(familia, negrita, cursiva); ruta != "" {
			out = append(out, ruta)
		}
	}
	return out
}

// fcMatch pregunta a fontconfig. Sin fontconfig, nada.
//
// ponytail: fontconfig siempre contesta algo —DejaVu si no hay nada mejor—, así que
// en Linux una familia que no está sale con su sustituta, igual que en el navegador.
func fcMatch(familia string, negrita, cursiva bool) string {
	patron := familia
	if negrita {
		patron += ":weight=bold"
	}
	if cursiva {
		patron += ":slant=italic"
	}
	c, parar := context.WithTimeout(context.Background(), 2*time.Second)
	defer parar()
	out, err := exec.CommandContext(c, "fc-match", "-f", "%{file}", patron).Output() //nolint:gosec // sin shell: la familia va como argumento
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
