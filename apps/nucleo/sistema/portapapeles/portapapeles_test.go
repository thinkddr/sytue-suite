// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package portapapeles

import (
	"fmt"
	"testing"
	"unicode/utf16"
)

// cfhtml arma un CF_HTML como el que escriben Chromium y Office, con sus desplazamientos
// en bytes.
// Sin documento, StartHTML y EndHTML valen -1, que la especificación permite.
func cfhtml(antes, fragmento, despues string, sinDocumento bool) []byte {
	const cab = "Version:0.9\r\nStartHTML:%010d\r\nEndHTML:%010d\r\nStartFragment:%010d\r\nEndFragment:%010d\r\n"
	largo := len(fmt.Sprintf(cab, 0, 0, 0, 0))
	ini := largo
	iniF := ini + len(antes)
	finF := iniF + len(fragmento)
	fin := finF + len(despues)
	if sinDocumento {
		ini, fin = -1, -1
	}
	return []byte(fmt.Sprintf(cab, ini, fin, iniF, finF) + antes + fragmento + despues + "\x00\x00")
}

// TestElCFHTMLDaElDocumentoEntero: de CF_HTML sale el documento entero —Excel marca el
// fragmento DENTRO de su tabla, y sin ella las filas se perderían—; sin desplazamientos
// del documento, el fragmento; y sin cabecera válida, desde el primer `<`.
func TestElCFHTMLDaElDocumentoEntero(t *testing.T) {
	antes, frag, despues := "<html><body><table><!--StartFragment-->", "<tr><td>1</td><td>año</td></tr>", "<!--EndFragment--></table></body></html>"
	if got := DeCFHTML(cfhtml(antes, frag, despues, false)); got != antes+frag+despues {
		t.Errorf("documento: %q", got)
	}
	if got := DeCFHTML(cfhtml(antes, frag, despues, true)); got != frag {
		t.Errorf("solo el fragmento: %q", got)
	}
	if got := DeCFHTML([]byte("Version:0.9\r\nbasura\r\n<p>hola</p>\x00")); got != "<p>hola</p>" {
		t.Errorf("sin desplazamientos: %q", got)
	}
}

// TestLoQueLlegaSePasaAUTF8: UTF-8 con y sin BOM, UTF-16 LE y BE con BOM (Firefox en X11)
// y LE sin él; los NUL del final se quitan después, sin partir la última letra.
func TestLoQueLlegaSePasaAUTF8(t *testing.T) {
	const quiero = "<p>Grace — ñ</p>"
	u := utf16.Encode([]rune(quiero))
	le, be := make([]byte, 0, 2*len(u)), make([]byte, 0, 2*len(u))
	for _, c := range u {
		le = append(le, byte(c), byte(c>>8))
		be = append(be, byte(c>>8), byte(c))
	}
	for nombre, datos := range map[string][]byte{
		"utf-8":             []byte(quiero + "\x00"),
		"utf-8 con BOM":     append([]byte{0xEF, 0xBB, 0xBF}, quiero...),
		"utf-16 LE":         append(append([]byte{0xFF, 0xFE}, le...), 0, 0),
		"utf-16 BE":         append([]byte{0xFE, 0xFF}, be...),
		"utf-16 LE sin BOM": le,
	} {
		if got := aUTF8(datos); got != quiero {
			t.Errorf("%s: %q", nombre, got)
		}
	}
}
