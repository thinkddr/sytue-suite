// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package certificados son los ALMACENES de certificados del sistema con los que las
// apps nativas firman en dos pasos (ADR 0051): tarjetas y DNIe por PKCS#11 en los tres
// escritorios, el Llavero de macOS e iOS, el almacén de Windows y el KeyChain de
// Android.
//
// # Lo único que hace un almacén
//
// Enseñar qué certificados de firma hay y pedirle al SISTEMA que firme un resumen con
// la clave de uno. No escribe ni lee ningún formato criptográfico —el CMS, el PDF y la
// comprobación los hace el servicio de Docs—: la clave no sale del equipo ni de la
// tarjeta, y lo que devuelve es la firma tal cual, con su formato dicho
// ([api.PDFFirmaDER] o [api.PDFFirmaP1363]) para que el servicio la normalice.
package certificados

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"sync"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

var (
	// ErrPIN: el PIN no es el de la tarjeta. Quedan intentos: se puede volver a pedir.
	ErrPIN = errors.New("certificados: el PIN no es correcto")
	// ErrPINBloqueado: la tarjeta ya no admite más intentos.
	ErrPINBloqueado = errors.New("certificados: el PIN está bloqueado")
	// ErrNoEsta: el certificado ya no está (se sacó la tarjeta, se borró del almacén).
	ErrNoEsta = errors.New("certificados: el certificado ya no está en el almacén")
)

// Certificado es uno con el que se puede firmar: su cadena y dónde está su clave.
type Certificado struct {
	// Cadena es el certificado (Cadena[0]) y, detrás, los emisores que el almacén
	// tenga, en orden: cada uno emite al anterior. Es lo que se manda al preparar.
	Cadena []*x509.Certificate
	// Almacen dice de dónde viene, para enseñarlo: «Tarjeta: DNIe», «Llavero».
	Almacen string
	// PidePIN es que firmar pide el PIN a la app (una tarjeta sin teclado propio).
	// Falso: lo pide el sistema, o no hace falta.
	PidePIN bool
	ref     any
}

// Nombre es el del titular, que es lo que se elige.
func (c Certificado) Nombre() string { return c.Cadena[0].Subject.CommonName }

// DER es la cadena como la quiere [api.PDF.PrepararFirma].
func (c Certificado) DER() [][]byte {
	out := make([][]byte, len(c.Cadena))
	for i, x := range c.Cadena {
		out[i] = x.Raw
	}
	return out
}

// Almacen es un origen de certificados del sistema.
type Almacen interface {
	// Certificados son los de firma que hay ahora mismo.
	Certificados(ctx context.Context) ([]Certificado, error)
	// Firmar pide al sistema la firma de lo preparado con la clave de `c`, y dice en
	// qué formato sale. `pin` solo cuenta si c.PidePIN.
	Firmar(ctx context.Context, c Certificado, prep api.PDFFirmaPreparada, pin string) (firma []byte, formato string, err error)
}

// sirveParaFirmar es el filtro de lo que se enseña: con uso de firma (o sin
// restricción), RSA o ECDSA. Lo demás lo rechazaría el servicio al preparar.
func sirveParaFirmar(c *x509.Certificate) bool {
	if c.KeyUsage != 0 && c.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0 {
		return false
	}
	if c.IsCA || (c.PublicKeyAlgorithm != x509.RSA && c.PublicKeyAlgorithm != x509.ECDSA) {
		return false
	}
	// Sin usos extendidos, vale para todo. Con ellos, tiene que haber uno de persona:
	// el Llavero de un Mac guarda también identidades de firma de CÓDIGO y de servidor,
	// que nadie quiere ver al firmar un contrato.
	if len(c.ExtKeyUsage) == 0 && len(c.UnknownExtKeyUsage) == 0 {
		return true
	}
	for _, u := range c.ExtKeyUsage {
		if u == x509.ExtKeyUsageAny || u == x509.ExtKeyUsageEmailProtection || u == x509.ExtKeyUsageClientAuth {
			return true
		}
	}
	for _, u := range c.UnknownExtKeyUsage {
		if u.Equal(oidFirmaDeDocumentos) || u.Equal(oidFirmaPDFAdobe) {
			return true
		}
	}
	return false
}

// Usos extendidos de firma de documentos: el de Microsoft (1.3.6.1.4.1.311.10.3.12) y
// el de Adobe para PDF (1.2.840.113583.1.1.5).
var (
	oidFirmaDeDocumentos = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 311, 10, 3, 12}
	oidFirmaPDFAdobe     = asn1.ObjectIdentifier{1, 2, 840, 113583, 1, 1, 5}
)

// cadenaDe ordena la cadena de `hoja` con los certificados que tenga el almacén: cada
// uno, el que lo emite, hasta uno autofirmado o hasta que no haya emisor. El servicio
// exige ese orden y descubrirlo después del PIN es tirar la firma.
func cadenaDe(hoja *x509.Certificate, otros []*x509.Certificate) []*x509.Certificate {
	cadena := []*x509.Certificate{hoja}
	for len(cadena) < 6 {
		ultimo := cadena[len(cadena)-1]
		if ultimo.CheckSignatureFrom(ultimo) == nil {
			break
		}
		var emisor *x509.Certificate
		for _, o := range otros {
			if !o.Equal(ultimo) && ultimo.CheckSignatureFrom(o) == nil {
				emisor = o
				break
			}
		}
		if emisor == nil {
			break
		}
		cadena = append(cadena, emisor)
	}
	return cadena
}

// prefijoDigestInfoSHA256 es la cabecera DER del DigestInfo de SHA-256 (RFC 8017 §9.2).
var prefijoDigestInfoSHA256 = []byte{
	0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20,
}

// ErrPreparadaIncoherente: lo que llega para firmar no cuadra consigo mismo.
var ErrPreparadaIncoherente = errors.New("certificados: la firma preparada no es coherente")

// comprobarPreparada exige, ANTES de pedir nada a la tarjeta, que el resumen sea de
// SHA-256 y que el DigestInfo sea exactamente ese resumen envuelto: con RSA se firma el
// DigestInfo y con ECDSA el resumen, y si no fueran lo mismo la firma de una clave RSA
// sería de otra cosa que lo que dice el resumen.
func comprobarPreparada(prep api.PDFFirmaPreparada) error {
	if len(prep.Resumen) != sha256.Size || !bytes.Equal(prep.DigestInfo, append(bytes.Clone(prefijoDigestInfoSHA256), prep.Resumen...)) {
		return ErrPreparadaIncoherente
	}
	return nil
}

// esECDSA dice si la clave del certificado es de curva elíptica: entonces se firma el
// resumen tal cual; con RSA, el DigestInfo.
func esECDSA(c *x509.Certificate) bool {
	_, ok := c.PublicKey.(*ecdsa.PublicKey)
	return ok
}

// fuentes son los almacenes que cada plataforma aporta (PKCS#11, Llavero, CNG,
// KeyChain), cada uno desde su fichero con build tag.
var fuentes []func() []Almacen

var (
	unaVez   sync.Once
	abiertos []Almacen
)

// DelSistema son los almacenes de esta máquina, abiertos una vez por proceso: iniciar
// un módulo PKCS#11 dos veces en el mismo proceso es un error de Cryptoki. Vacío donde
// no hay ninguno (la app lo dice en vez de enseñar una lista vacía).
func DelSistema() []Almacen {
	unaVez.Do(func() {
		for _, f := range fuentes {
			abiertos = append(abiertos, f()...)
		}
	})
	return abiertos
}
