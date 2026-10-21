// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package portapapeles lee lo que el portapapeles del sistema trae CON FORMATO, que es
// lo que el de Fyne no da: `fyne.Clipboard` solo sabe de texto, y pegar en el redactor de
// Sytue Mail tiene que conservar la negrita, las listas y los enlaces, como en la web.
//
// Cada escritorio lo guarda a su manera, y aquí se lee cada una:
//
//   - macOS: `public.html` en NSPasteboard; y si solo está el archivo web de Safari
//     (`com.apple.webarchive`), el HTML de su recurso principal.
//   - Windows: el formato registrado `HTML Format` (CF_HTML), con su cabecera de
//     desplazamientos (DeCFHTML).
//   - Linux: el `text/html` de la selección CLIPBOARD de X11 —también bajo Wayland, por
//     XWayland, que es donde corre una app de Fyne—; sin X11 (o si por ahí no llega), el
//     del compositor Wayland por su control de datos (wayland_linux.go), y `wl-paste` de
//     respaldo.
//
// Lo que sale NO está saneado: es el HTML tal como lo dejó otra aplicación, y quien lo
// pega tiene que pasarlo por su lista blanca antes de meterlo en ningún sitio.
package portapapeles

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"unicode/utf16"
)

var (
	// ErrSinHTML es que el portapapeles no trae HTML (o está vacío): se pega el texto.
	ErrSinHTML = errors.New("portapapeles: no hay HTML")
	// ErrNoDisponible es que en esta plataforma, o sin servidor gráfico, no se puede leer.
	ErrNoDisponible = errors.New("portapapeles: no se puede leer el HTML aquí")
)

// MaxHTML es lo más que se acepta. Un documento de Word copiado entero pasa de sobra los
// cientos de KB, pero lo que pase de aquí se pega como texto: sanearlo y maquetarlo en el
// hilo de la interfaz bloquearía la ventana.
const MaxHTML = 8 << 20

// HTML es el HTML que hay en el portapapeles, en UTF-8.
func HTML() (string, error) {
	datos, err := leer()
	if err != nil {
		return "", err
	}
	s := aUTF8(datos)
	switch {
	case strings.TrimSpace(s) == "":
		return "", ErrSinHTML
	case len(s) > MaxHTML:
		return "", ErrSinHTML
	}
	return s, nil
}

// aUTF8 pasa a UTF-8 lo que llega. Casi todo es UTF-8, pero Firefox en X11 deja el
// `text/html` en UTF-16 (con su BOM), y el archivo web de Safari puede venir igual. Sin
// BOM, un NUL en la segunda posición de cada par delata UTF-16 LE.
//
// Los NUL del final (Windows redondea el bloque; hay quien termina en `\0`) se quitan
// DESPUÉS de decodificar: en UTF-16 LE, el `>` del final es `3E 00`, y quitarlos antes
// partiría la última letra.
func aUTF8(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(bytes.TrimRight(b[3:], "\x00"))
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return deUTF16(b[2:], false)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return deUTF16(b[2:], true)
	case len(b) >= 4 && b[0] != 0 && b[1] == 0 && b[2] != 0 && b[3] == 0:
		// `<\x00h\x00`: dos letras ASCII seguidas en UTF-16 LE. En UTF-8 un NUL no
		// aparece en un HTML de verdad.
		return deUTF16(b, false)
	}
	return string(bytes.TrimRight(b, "\x00"))
}

func deUTF16(b []byte, grande bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if grande {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return strings.TrimRight(string(utf16.Decode(u)), "\x00")
}

// DeCFHTML saca el HTML de un bloque CF_HTML de Windows: una cabecera de líneas
// `Clave:valor` con los desplazamientos, en bytes, de lo que va detrás.
//
//	Version:0.9
//	StartHTML:0000000105
//	EndHTML:0000000199
//	StartFragment:0000000141
//	EndFragment:0000000163
//	<html><body><!--StartFragment-->…<!--EndFragment--></body></html>
//
// Se toma el documento ENTERO (StartHTML…EndHTML) y no solo el fragmento: Excel marca el
// fragmento DENTRO de la tabla, alrededor de las filas, y sin el `<table>` que las
// envuelve un analizador de HTML las tira y deja las palabras sueltas. Si faltan los
// desplazamientos del documento (StartHTML puede ser -1), el fragmento; si no hay
// ninguno válido, lo que venga a partir del primer `<`.
func DeCFHTML(b []byte) string {
	b = bytes.TrimRight(b, "\x00")
	desp := map[string]int{}
	for _, linea := range strings.SplitN(string(b[:min(len(b), 512)]), "\n", 12) {
		clave, valor, ok := strings.Cut(strings.TrimSpace(linea), ":")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(valor)); err == nil {
			desp[clave] = n
		}
	}
	for _, par := range [][2]string{{"StartHTML", "EndHTML"}, {"StartFragment", "EndFragment"}} {
		ini, ok1 := desp[par[0]]
		fin, ok2 := desp[par[1]]
		if ok1 && ok2 && ini >= 0 && ini < fin && fin <= len(b) {
			return string(b[ini:fin])
		}
	}
	if i := bytes.IndexByte(b, '<'); i >= 0 {
		return string(b[i:])
	}
	return ""
}
