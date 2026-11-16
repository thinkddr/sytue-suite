// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package verifactu

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"git.sytue.eu/sytue/one/platform/pkg/pkcs12"
)

var (
	// ErrFicheroCertificado: lo subido no es ni un PKCS#12 ni un PEM con un
	// certificado y su clave privada.
	ErrFicheroCertificado = errors.New("verifactu: el fichero no contiene un certificado con su clave privada")
	// ErrContrasenaCertificado: el `.p12` es legible pero la contraseña no lo abre.
	ErrContrasenaCertificado = errors.New("verifactu: la contraseña del certificado no es correcta")
)

// PEMDeFichero convierte el certificado tal y como lo tiene quien lo sube al par
// PEM (certificado y cadena, clave privada) que guarda el servicio y que usa
// NuevoCliente para autenticarse ante la Agencia Tributaria.
//
// La FNMT, las cámaras y los colegios lo entregan como `.p12`/`.pfx`; quien lo
// gestiona con OpenSSL lo tiene en PEM, a veces con la clave en un fichero
// aparte (`clave`). Se aceptan las tres formas para que nadie tenga que
// convertir nada con la consola antes de activar VERI*FACTU.
//
// La clave sale siempre como PKCS#8 sin cifrar porque es lo que entiende
// tls.X509KeyPair; quien la guarda la sella con la clave del despliegue antes
// de tocar la base de datos. Los errores nunca llevan la contraseña.
func PEMDeFichero(fichero, clave []byte, contrasena string) (certPEM, keyPEM []byte, err error) {
	if esPEM(fichero) {
		return pemDeBloques(fichero, clave)
	}
	cadena, privada, err := pkcs12.Leer(fichero, contrasena)
	switch {
	case errors.Is(err, pkcs12.ErrContrasena):
		return nil, nil, ErrContrasenaCertificado
	case err != nil:
		return nil, nil, fmt.Errorf("%w: %w", ErrFicheroCertificado, err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(privada)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: clave privada no exportable", ErrFicheroCertificado)
	}
	// Leer deja el certificado del titular el primero: tls.X509KeyPair toma ese
	// como hoja y comprueba que case con la clave.
	var certs bytes.Buffer
	for _, c := range cadena {
		_ = pem.Encode(&certs, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
	}
	return certs.Bytes(), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), nil
}

func esPEM(datos []byte) bool {
	return bytes.Contains(datos, []byte("-----BEGIN "))
}

// pemDeBloques separa certificados y clave privada de uno o dos ficheros PEM.
// Una clave cifrada («ENCRYPTED PRIVATE KEY») no se descifra aquí: quien la
// tiene así puede exportar el `.p12`, que es lo que entrega su emisor.
func pemDeBloques(ficheros ...[]byte) (certPEM, keyPEM []byte, err error) {
	var certs, claves bytes.Buffer
	for _, resto := range ficheros {
		for {
			var bloque *pem.Block
			bloque, resto = pem.Decode(resto)
			if bloque == nil {
				break
			}
			switch {
			case bloque.Type == "CERTIFICATE":
				_ = pem.Encode(&certs, bloque)
			case bloque.Type == "ENCRYPTED PRIVATE KEY", bloque.Headers["Proc-Type"] != "":
				return nil, nil, fmt.Errorf("%w: la clave privada PEM está cifrada", ErrFicheroCertificado)
			case strings.HasSuffix(bloque.Type, "PRIVATE KEY") && claves.Len() == 0:
				_ = pem.Encode(&claves, bloque)
			}
		}
	}
	if certs.Len() == 0 || claves.Len() == 0 {
		return nil, nil, ErrFicheroCertificado
	}
	return certs.Bytes(), claves.Bytes(), nil
}
