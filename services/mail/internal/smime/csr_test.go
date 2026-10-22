// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
)

// LO QUE SE COMPRUEBA AQUÍ SE PAGA EN CASA DEL DESTINATARIO. Una petición mal
// armada no da error: la CA emite, el certificado se instala, y el correo llega
// con la firma marcada como no válida en un cliente que no podemos depurar.
func TestNuevaPeticion(t *testing.T) {
	const buzon = "ana@corporate.sytue.com"
	der, keyPEM, err := NuevaPeticion(buzon)
	if err != nil {
		t.Fatalf("NuevaPeticion: %v", err)
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		t.Fatalf("la petición no se puede leer: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("la petición no está bien firmada: %v", err)
	}

	// LA DIRECCIÓN VA EN EL SAN. Los clientes comparan el `rfc822Name` con el
	// `From`; sin él se ve «no coincide el remitente» aunque la firma sea buena.
	if len(csr.EmailAddresses) != 1 || csr.EmailAddresses[0] != buzon {
		t.Errorf("SAN = %v, quiero [%s]", csr.EmailAddresses, buzon)
	}
	// RSA: los S/MIME de las CAs públicas lo son casi sin excepción, y Outlook
	// con ECDSA sigue siendo terreno resbaladizo.
	pub, ok := csr.PublicKey.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("la clave pública es %T, quiero RSA", csr.PublicKey)
	}
	if bits := pub.N.BitLen(); bits < 2048 {
		t.Errorf("clave de %d bits; el mínimo del CA/Browser Forum son 2048", bits)
	}

	// La clave sale en PKCS#8, que es lo que lee LoadIdentity.
	bloque, _ := pem.Decode([]byte(keyPEM))
	if bloque == nil || bloque.Type != "PRIVATE KEY" {
		t.Fatalf("la clave no salió como PKCS#8: %v", bloque)
	}
	priv, err := x509.ParsePKCS8PrivateKey(bloque.Bytes)
	if err != nil {
		t.Fatalf("la clave no se puede leer: %v", err)
	}
	// Y ES LA DE ESTA PETICIÓN. Devolver una clave que no corresponde produce un
	// certificado inservible que nadie detecta hasta que falla al firmar.
	rsaPriv, ok := priv.(*rsa.PrivateKey)
	if !ok || !rsaPriv.PublicKey.Equal(pub) {
		t.Error("la clave devuelta no es la de la petición")
	}
}

func TestNuevaPeticionRechazaLoQueNoEsUnBuzon(t *testing.T) {
	// Un dominio NO es una dirección. Es el error que cuesta un certificado
	// emitido y tirado: lo aprendimos con el ACME de Actalis, que acepta dominios
	// y rechaza buzones.
	for _, malo := range []string{"", "corporate.sytue.com", "ana@", "@sytue.com", "ana sytue.com"} {
		if _, _, err := NuevaPeticion(malo); err == nil {
			t.Errorf("%q debería rechazarse", malo)
		}
	}
}

func TestPeticionPEM(t *testing.T) {
	der, _, err := NuevaPeticion("ana@corporate.sytue.com")
	if err != nil {
		t.Fatalf("NuevaPeticion: %v", err)
	}
	p := PeticionPEM(der)
	if !strings.HasPrefix(p, "-----BEGIN CERTIFICATE REQUEST-----") {
		t.Errorf("cabecera inesperada: %.40q", p)
	}
	bloque, _ := pem.Decode([]byte(p))
	if bloque == nil || !bytes.Equal(bloque.Bytes, der) {
		t.Error("el PEM no envuelve la misma petición")
	}
}
