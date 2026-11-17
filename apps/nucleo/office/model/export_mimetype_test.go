// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package model

import (
	"encoding/binary"
	"testing"
)

// LibreOffice reconoce un paquete ODF por el `mimetype` leído en un offset fijo de la
// cabecera local: sin compresión, con el CRC y los tamaños ahí mismo y sin descriptor de
// datos detrás (bit 3 de las banderas). Con el descriptor responde «source file could not be
// loaded» aunque el ZIP sea válido.
func comprobarMimetypeODF(t *testing.T, data []byte, tipo string) {
	t.Helper()
	if len(data) < 30+len("mimetype") || binary.LittleEndian.Uint32(data) != 0x04034b50 {
		t.Fatal("el paquete no empieza por una cabecera local")
	}
	flags := binary.LittleEndian.Uint16(data[6:])
	metodo := binary.LittleEndian.Uint16(data[8:])
	tam := binary.LittleEndian.Uint32(data[18:])
	nombre := int(binary.LittleEndian.Uint16(data[26:]))
	extra := int(binary.LittleEndian.Uint16(data[28:]))
	if got := string(data[30 : 30+nombre]); got != "mimetype" {
		t.Fatalf("primer miembro %q; quiero mimetype", got)
	}
	if flags&0x8 != 0 || metodo != 0 || int(tam) != len(tipo) {
		t.Fatalf("mimetype con banderas %#x, método %d y tamaño %d; quiero sin descriptor, STORE y %d", flags, metodo, tam, len(tipo))
	}
	if got := string(data[30+nombre+extra : 30+nombre+extra+len(tipo)]); got != tipo {
		t.Fatalf("mimetype dice %q; quiero %q", got, tipo)
	}
}

func TestElMimetypeODFVaSinDescriptor(t *testing.T) {
	const tipo = "application/vnd.oasis.opendocument.spreadsheet"
	data, err := buildODFPackage(tipo, "<c/>", "<s/>", nil)
	if err != nil {
		t.Fatal(err)
	}
	comprobarMimetypeODF(t, data, tipo)

	// Y la plantilla, que reescribe el `mimetype` del paquete.
	plantilla, _, _, err := ComoPlantilla(KindSpreadsheet, FormatODF, data)
	if err != nil {
		t.Fatal(err)
	}
	comprobarMimetypeODF(t, plantilla, tipo+"-template")
}
