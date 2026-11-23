// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Regresiones de endurecimiento ante entradas hostiles: tamaños que desbordan,
// enteros fuera del rango de la norma y reservas que manda el propio fichero.
package pdf

import (
	"errors"
	"math"
	"strings"
	"testing"
)

// La reserva de ASCII85Decode multiplicaba la longitud por 4 antes de dividir:
// con una longitud enorme el producto da la vuelta y make recibe un negativo.
func TestCapacidadASCII85NoDesborda(t *testing.T) {
	for _, n := range []int{math.MaxInt, math.MaxInt - 1, math.MaxInt / 4 * 3, maxFlujoInflado * 2, -1, 0} {
		c := capacidadASCII85(n)
		if c < 0 || c > maxFlujoInflado {
			t.Errorf("capacidadASCII85(%d) = %d; fuera de [0, %d]", n, c, maxFlujoInflado)
		}
	}
	if c := capacidadASCII85(10); c != 8 {
		t.Errorf("capacidadASCII85(10) = %d; quería 8", c)
	}
	if got := string(decodificarASCII85([]byte(`87cURD]i,"Ebo80~>`))); got != "Hello World!" {
		t.Errorf("ASCII85 = %q", got)
	}
}

// hashV5 multiplica por 64 la suma de sus entradas para el búfer de cada vuelta.
// Fuera de los tamaños de la norma no debe calcular nada (ni reservar).
func TestHashV5RechazaEntradasFueraDeNorma(t *testing.T) {
	sal := make([]byte, 8)
	hostiles := []struct {
		nombre         string
		pwd, sal, datU []byte
	}{
		{"contraseña de 1 MiB", make([]byte, 1<<20), sal, nil},
		{"contraseña de 128 bytes", make([]byte, 128), sal, nil},
		{"/U de 49 bytes", []byte("x"), sal, make([]byte, 49)},
		{"sal de 9 bytes", []byte("x"), make([]byte, 9), nil},
	}
	for _, h := range hostiles {
		if k := hashV5(h.pwd, h.sal, h.datU, 6); k != nil {
			t.Errorf("%s: hashV5 devolvió %d bytes; debía rechazarse", h.nombre, len(k))
		}
	}
	if k := hashV5(make([]byte, 127), sal, make([]byte, 48), 6); len(k) != 32 {
		t.Fatalf("las entradas máximas de la norma deben dar 32 bytes, dio %d", len(k))
	}
}

// RC4 solo se usa para LEER documentos antiguos: lo que escribe ProtegerPDF es
// siempre AES-256 (V5/R6, AESV3). Si alguien cambiara el escritor a RC4, la
// alerta de CodeQL sobre rc4Aplicar dejaría de ser un falso positivo.
func TestProtegerPDFSoloEscribeAES256(t *testing.T) {
	protegido, err := ProtegerPDF(contratoDePrueba(t), OpcionesProteccion{ContrasenaUsuario: "secreta", Permitir: PermisosTodos()})
	if err != nil {
		t.Fatalf("ProtegerPDF: %v", err)
	}
	d, err := LeerPDFConClave(protegido, "secreta")
	if err != nil {
		t.Fatalf("LeerPDFConClave: %v", err)
	}
	enc, ok := d.resolver(d.trailer["Encrypt"]).(pdfDict)
	if !ok {
		t.Fatal("falta /Encrypt")
	}
	v, _ := comoEntero(d.resolver(enc["V"]))
	r, _ := comoEntero(d.resolver(enc["R"]))
	cf, _ := d.resolver(enc["CF"]).(pdfDict)
	std, _ := d.resolver(cf["StdCF"]).(pdfDict)
	if v != 5 || r != 6 || std["CFM"] != pdfName("AESV3") {
		t.Fatalf("el escritor no usa AES-256: V=%d R=%d CFM=%v", v, r, std["CFM"])
	}
}

// Un /Size hostil mandaba la reserva de la tabla del anexo sin tope propio.
func TestNuevoAnexoRechazaSizeHostil(t *testing.T) {
	for _, size := range []pdfObj{int64(math.MaxInt32), float64(3e9), int64(maxObjetosAnexo + 1)} {
		d := docDePrueba(t, "Hola")
		d.trailer["Size"] = size
		if _, err := nuevoAnexo(d); !errors.Is(err, ErrInvalidaFirmas) {
			t.Errorf("/Size %v: nuevoAnexo = %v; quería ErrInvalidaFirmas", size, err)
		}
	}
	if _, err := nuevoAnexo(docDePrueba(t, "Hola")); err != nil {
		t.Fatalf("un documento normal debe admitir anexo: %v", err)
	}
}

// UTF-16BE de un título: pares y sustitutos se decodifican, un byte suelto final
// se descarta, y ningún código queda fuera del rango de uint16.
func TestDecodificarTextoPDFUTF16Hostil(t *testing.T) {
	s := pdfString("\xFE\xFF\x00A\xD8\x3D\xDE\x00\xFF\xFF\x00")
	got := decodificarTextoPDF(s)
	if !strings.HasPrefix(got, "A\U0001F600") || !strings.HasSuffix(got, "￿") {
		t.Fatalf("decodificarTextoPDF = %q", got)
	}
}
