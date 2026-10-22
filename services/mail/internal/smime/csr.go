// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"net/mail"
	"strings"
)

// LA PETICIÓN QUE SE LE MANDA A UNA CA PÚBLICA.
//
// Es lo primero que pide cualquier emisor y lo que no teníamos: la autoridad de
// aquí se firma sus certificados y nunca necesitó pedirle nada a nadie.
//
// Tres decisiones, y las tres se pagan en casa del destinatario si se fallan:
//
//   - RSA Y NO ECDSA. El sobre CMS ya sabe firmar con las dos, pero lo que
//     manda aquí es qué acepta la CA y qué entiende el cliente que recibe: los
//     certificados S/MIME de las CAs públicas son RSA casi sin excepción, y
//     Outlook con ECDSA sigue siendo terreno resbaladizo. 2048 bits, que es el
//     mínimo de los requisitos del CA/Browser Forum.
//   - LA DIRECCIÓN VA EN EL SAN, no en el CN. Desde los requisitos de S/MIME de
//     2023 el Common Name no identifica al buzón; los clientes comparan el
//     `rfc822Name` del SAN con el `From`, y un certificado sin él se ve como «no
//     coincide el remitente» aunque la firma sea válida.
//   - CON `emailProtection`. Un certificado sin ese uso extendido no firma
//     correo: el cliente lo trata como si no estuviera.
//
// La clave privada NO SALE DE AQUÍ: se genera en este proceso, se devuelve para
// sellarla con la KEK y a la CA solo viaja la petición. Una CA que ofrezca
// generarla por ti te está ofreciendo tener una copia.

// NuevaPeticion genera una clave RSA y su PKCS#10 para un buzón.
//
// Devuelve la petición en DER —que es lo que piden las APIs de emisión— y la
// clave en PEM, lista para el vault.
func NuevaPeticion(address string) (csrDER []byte, keyPEM string, err error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(address))
	if err != nil {
		return nil, "", fmt.Errorf("smime: dirección inválida para la petición: %q", address)
	}
	clave, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", fmt.Errorf("smime: generar la clave del buzón: %w", err)
	}
	plantilla := &x509.CertificateRequest{
		// El CN con la dirección es lo que esperan casi todas las interfaces de
		// emisión, aunque quien identifica el buzón sea el SAN. Ponerlo no cuesta
		// y omitirlo hace que algunas rechacen la petición.
		Subject:            pkix.Name{CommonName: addr.Address},
		EmailAddresses:     []string{addr.Address},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	csrDER, err = x509.CreateCertificateRequest(rand.Reader, plantilla, clave)
	if err != nil {
		return nil, "", fmt.Errorf("smime: construir la petición: %w", err)
	}
	// PKCS#8 y no PKCS#1: es lo que lee `LoadIdentity`.
	claveDER, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		return nil, "", fmt.Errorf("smime: serializar la clave del buzón: %w", err)
	}
	keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: claveDER}))
	return csrDER, keyPEM, nil
}

// PeticionPEM envuelve la petición para las APIs que la quieren en texto.
func PeticionPEM(csrDER []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}))
}
