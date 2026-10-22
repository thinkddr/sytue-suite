// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Firma en DOS PASOS con el certificado del SISTEMA (ADR 0051).
//
// El certificado de la mayoría de la gente en Europa no está en un fichero: está en
// el Llavero, en el almacén de Windows, en una tarjeta o en el DNI electrónico, y su
// clave privada NO SE PUEDE sacar de ahí —en una tarjeta, físicamente—. Así que la
// firma se parte en tres y cada trozo lo hace quien puede:
//
//  1. Aquí se prepara el PDF con el hueco y se calcula lo que hay que firmar
//     ([PrepararFirmaPDF]): el resumen de los atributos firmados, que contienen el
//     resumen del documento y la huella del certificado.
//  2. La app nativa le pide al SISTEMA que firme ese resumen con la clave elegida.
//     No escribe ningún formato criptográfico: es una llamada a la función de firma
//     del sistema, y la clave no sale del equipo ni de la tarjeta.
//  3. Aquí se comprueba que esa firma es de ESE resumen con ESA clave, se pide el
//     sello de tiempo, se monta el CMS y se incrusta ([CompletarFirmaPDF]).
//
// El motor sigue siendo UNO: los dos pasos usan las mismas piezas que la firma con
// un .p12 (el hueco, los atributos, el montaje del CMS), así que las dos producen
// el mismo perfil PAdES.
package domain

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var (
	// ErrFirmaNoCorresponde: la firma que devuelve la app no es de ese resumen
	// con la clave de ese certificado. Es lo que para a un cliente que miente.
	ErrFirmaNoCorresponde = errors.New("pdf: la firma recibida no corresponde al resumen ni al certificado")
	// ErrPreparadoAlterado: el documento preparado no es el que se entregó.
	ErrPreparadoAlterado = errors.New("pdf: el documento preparado no es el que se entregó")
	// ErrSelloNoDisponible: la autoridad de sellado no ha dado el sello. En dos
	// pasos NO se firma sin él en silencio: quien firma decide si reintenta o si
	// sigue sin sello (ADR 0051).
	ErrSelloNoDisponible = errors.New("pdf: la autoridad de sellado no ha dado el sello")
)

// FirmaPreparada es lo que deja el primer paso.
type FirmaPreparada struct {
	// PDF es el documento con el hueco reservado y el /ByteRange escrito. Lo que
	// se firme solo vale para ESTOS bytes.
	PDF []byte
	// Inicio y Fin delimitan el hueco, con sus dos '<' '>'.
	Inicio, Fin int
	// PorFirmar es el DER (SET OF) de los atributos firmados.
	PorFirmar []byte
	// Resumen es SHA-256(PorFirmar): lo que firma el sistema cuando su función
	// de firma recibe el resumen ya calculado (Llavero, CNG, ECDSA en PKCS#11).
	Resumen []byte
	// DigestInfo es el mismo resumen envuelto como pide PKCS#1 v1.5: lo que
	// firman las funciones «en crudo» (CKM_RSA_PKCS, NONEwithRSA). Se entrega
	// hecho para que la app no tenga que escribir ni un byte de ASN.1.
	DigestInfo []byte
	// Fecha es la hora que declara la firma (/M). La pone el servicio, no la app.
	Fecha time.Time
	// Nombre es el del certificado: el que dirá la apariencia visible.
	Nombre string
	// Anexado: PDF es el documento tal como llegó más una revisión anexada (§7.5.6),
	// así que EMPIEZA por sus bytes, sin tocar ninguno. false si hubo que reescribirlo
	// (cifrado, o con la tabla de referencias reconstruida).
	Anexado bool
}

// PrepararFirmaPDF es el primer paso: el documento con el hueco y lo que hay que
// firmar, para el certificado `cadena[0]`.
//
// Se anexa (§7.5.6) para conservar las firmas anteriores; solo si el documento no
// lo admite —cifrado, o con la tabla de referencias reconstruida— se reescribe,
// igual que la firma con un .p12.
func PrepararFirmaPDF(doc *PDFDoc, opts OpcionesFirma, cadena []*x509.Certificate) (*FirmaPreparada, error) {
	if len(cadena) == 0 || cadena[0] == nil {
		return nil, fmt.Errorf("%w: falta el certificado", ErrCertificadoNoUsable)
	}
	firmante := cadena[0]
	// La fecha y el nombre los decide el SERVICIO: una app que mandase otro
	// nombre haría que el recuadro visible dijera «Firmado por» alguien que no
	// es el titular del certificado.
	opts.Fecha = time.Now().UTC()
	opts.Nombre = firmante.Subject.CommonName
	opts.PKCS12, opts.CertificadoPEM, opts.ClavePEM, opts.Sello = nil, nil, nil, nil
	if err := CertificadoParaFirmar(firmante, opts.Fecha); err != nil {
		return nil, err
	}
	if err := cadenaQueCabe(cadena); err != nil {
		return nil, err
	}

	h, err := huecoIncremental(doc, opts)
	if errors.Is(err, ErrNoIncremental) {
		h, err = huecoReescrito(doc, opts)
	}
	if err != nil {
		return nil, err
	}
	_, porFirmar, err := prepararAtributos(h.resumen(), firmante)
	if err != nil {
		return nil, err
	}
	resumen := sha256.Sum256(porFirmar)
	return &FirmaPreparada{
		PDF: h.datos, Inicio: h.inicio, Fin: h.fin,
		PorFirmar: porFirmar, Resumen: resumen[:],
		DigestInfo: append(append([]byte(nil), prefijoDigestInfoSHA256...), resumen[:]...),
		Fecha:      opts.Fecha, Nombre: opts.Nombre,
		// Se mira el resultado y no el camino que se tomó: así dice la verdad aunque cambie
		// cuándo se puede anexar y cuándo hay que reescribir.
		Anexado: bytes.HasPrefix(h.datos, doc.buf),
	}, nil
}

// reservaSelloYFirma es lo que el hueco guarda para lo que NO se sabe al preparar:
// el token de la autoridad (el de Izenpe mide 4,9 KB con su cadena; se deja casi el
// doble), la firma (una RSA de 8192 bits son 1 KiB) y los atributos y el envoltorio
// del CMS.
const reservaSelloYFirma = 10 << 10

// cadenaQueCabe comprueba ANTES del PIN dos cosas que, si fallan después, tiran la
// firma: que la cadena que manda la app va en orden —cada certificado firmado por el
// siguiente, que es lo que un validador espera encontrar incrustado— y que cabe en el
// hueco con el sello. Una cadena desordenada o con certificados de relleno no la
// arregla nadie después: la firma ya estaría hecha.
func cadenaQueCabe(cadena []*x509.Certificate) error {
	total := 0
	for i, c := range cadena {
		total += len(c.Raw)
		if i+1 < len(cadena) {
			if err := c.CheckSignatureFrom(cadena[i+1]); err != nil {
				return fmt.Errorf("%w: el certificado %d de la cadena no lo emite el %d: %w",
					ErrCertificadoNoUsable, i+1, i+2, err)
			}
		}
	}
	if total+reservaSelloYFirma > tamanoHuecoFirma {
		return fmt.Errorf("%w: la cadena mide %d bytes y en la firma caben %d",
			ErrCertificadoNoUsable, total, tamanoHuecoFirma-reservaSelloYFirma)
	}
	return nil
}

// prefijoDigestInfoSHA256 es la cabecera DER de DigestInfo para SHA-256 (RFC 8017
// §9.2, nota 1). Es la misma que usa `crypto/rsa` por dentro.
var prefijoDigestInfoSHA256 = []byte{
	0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20,
}

// CompletarFirmaPDF es el tercer paso: con la firma que hizo el sistema, monta el
// CMS y lo incrusta en el documento preparado.
//
// Todo lo que llega de fuera se comprueba contra lo que se preparó, porque la app
// puede mentir: el hueco tiene que seguir vacío, los atributos tienen que resumir
// ESTE documento, y la firma tiene que ser de ESE resumen con la clave de ESE
// certificado. `sello` nil es firmar sin sello porque quien firma lo ha pedido;
// con sello, un fallo de la autoridad devuelve [ErrSelloNoDisponible] y NO firma.
func CompletarFirmaPDF(p FirmaPreparada, cadena []*x509.Certificate, firma []byte, sello pedirSelloFn) (firmado []byte, sellado bool, err error) {
	if len(cadena) == 0 || cadena[0] == nil {
		return nil, false, fmt.Errorf("%w: falta el certificado", ErrCertificadoNoUsable)
	}
	if p.Inicio <= 0 || p.Fin > len(p.PDF) || p.Fin-p.Inicio < 4 ||
		p.PDF[p.Inicio] != '<' || p.PDF[p.Fin-1] != '>' {
		return nil, false, fmt.Errorf("%w: el hueco no está donde se preparó", ErrPreparadoAlterado)
	}
	// Una copia: el preparado puede estar en la memoria de quien llama y un fallo
	// a medias no debe dejárselo escrito.
	h := conHueco{datos: bytes.Clone(p.PDF), inicio: p.Inicio, fin: p.Fin}
	if len(bytes.TrimLeft(h.datos[h.inicio+1:h.fin-1], "0")) != 0 {
		return nil, false, fmt.Errorf("%w: el hueco ya tiene una firma", ErrPreparadoAlterado)
	}

	atributos, err := atributosDe(p.PorFirmar)
	if err != nil {
		return nil, false, err
	}
	if !resumenFirmadoEs(atributos, h.resumen()) {
		return nil, false, fmt.Errorf("%w: los atributos no resumen este documento", ErrPreparadoAlterado)
	}
	if !certificadoFirmadoEs(atributos, cadena[0]) {
		return nil, false, fmt.Errorf("%w: los atributos nombran otro certificado", ErrPreparadoAlterado)
	}
	resumen := sha256.Sum256(p.PorFirmar)
	if err := VerificarFirmaDeResumen(cadena[0].PublicKey, resumen[:], firma); err != nil {
		return nil, false, err
	}

	var token []byte
	if sello != nil {
		token, err = sello(huellaDeFirma(firma))
		if err != nil || len(token) == 0 {
			if err == nil {
				err = errors.New("la autoridad no devolvió token")
			}
			return nil, false, fmt.Errorf("%w: %w", ErrSelloNoDisponible, err)
		}
	}
	cms, err := montarCMS(atributos, firma, cadena, token)
	if err != nil {
		return nil, false, err
	}
	out, err := h.incrustar(cms)
	return out, token != nil, err
}

// atributosDe lee el DER de los atributos firmados que se guardó al preparar, y
// comprueba que volver a codificarlos da EXACTAMENTE los mismos bytes: si no, lo
// que va en el CMS no sería lo que el sistema firmó.
func atributosDe(porFirmar []byte) ([]atributo, error) {
	var atributos []atributo
	resto, err := asn1.UnmarshalWithParams(porFirmar, &atributos, "set")
	if err != nil || len(resto) != 0 {
		return nil, fmt.Errorf("%w: los atributos firmados no se pueden leer", ErrPreparadoAlterado)
	}
	otra, err := derDeAtributos(atributos)
	if err != nil || !bytes.Equal(otra, porFirmar) {
		return nil, fmt.Errorf("%w: los atributos firmados no se vuelven a codificar igual", ErrPreparadoAlterado)
	}
	return atributos, nil
}

// resumenFirmadoEs dice si el message-digest de los atributos es `resumen`.
func resumenFirmadoEs(atributos []atributo, resumen []byte) bool {
	for _, a := range atributos {
		if !a.Tipo.Equal(oidResumen) || len(a.Valores) != 1 {
			continue
		}
		var declarado []byte
		if _, err := asn1.Unmarshal(a.Valores[0].FullBytes, &declarado); err != nil {
			return false
		}
		return bytes.Equal(declarado, resumen)
	}
	return false
}

// certificadoFirmadoEs dice si signing-certificate-v2 nombra a `c`.
func certificadoFirmadoEs(atributos []atributo, c *x509.Certificate) bool {
	for _, a := range atributos {
		if !a.Tipo.Equal(oidCertificadoFirmante) || len(a.Valores) != 1 {
			continue
		}
		var v certificadoFirmanteV2
		if _, err := asn1.Unmarshal(a.Valores[0].FullBytes, &v); err != nil || len(v.Certs) == 0 {
			return false
		}
		huella := sha256.Sum256(c.Raw)
		return bytes.Equal(v.Certs[0].Huella, huella[:])
	}
	return false
}

// VerificarFirmaDeResumen comprueba que `firma` es la de `resumen` (SHA-256) con la
// clave `publica`: RSA PKCS#1 v1.5, o ECDSA en DER.
func VerificarFirmaDeResumen(publica any, resumen, firma []byte) error {
	switch k := publica.(type) {
	case *rsa.PublicKey:
		if rsa.VerifyPKCS1v15(k, crypto.SHA256, resumen, firma) == nil {
			return nil
		}
	case *ecdsa.PublicKey:
		if ecdsa.VerifyASN1(k, resumen, firma) {
			return nil
		}
	default:
		return fmt.Errorf("%w: solo se firma con RSA o ECDSA", ErrCertificadoNoUsable)
	}
	return ErrFirmaNoCorresponde
}

// FirmaEnDER normaliza la firma tal como la devuelve el sistema. ECDSA sale en
// DOS formatos según quién firme: en DER (X9.62) del Llavero y de Android, y como
// r‖s de tamaño fijo (IEEE P1363) de CNG y de PKCS#11. El CMS la quiere en DER, y
// la conversión se hace AQUÍ para que la app no tenga que escribir ASN.1. RSA no
// tiene formatos: se devuelve tal cual.
func FirmaEnDER(publica any, firma []byte, formato string) ([]byte, error) {
	k, ok := publica.(*ecdsa.PublicKey)
	if !ok || formato != "p1363" {
		return firma, nil
	}
	n := (k.Curve.Params().BitSize + 7) / 8
	if len(firma) != 2*n {
		return nil, fmt.Errorf("%w: una firma ECDSA r‖s de esta curva mide %d bytes y llegan %d",
			ErrFirmaNoCorresponde, 2*n, len(firma))
	}
	return asn1.Marshal(struct{ R, S *big.Int }{
		new(big.Int).SetBytes(firma[:n]), new(big.Int).SetBytes(firma[n:]),
	})
}

// CertificadoParaFirmar comprueba ANTES de pedir el PIN que el certificado puede
// firmar hoy: una firma con un certificado caducado o sin uso de firma la rechaza
// cualquier validador, y descubrirlo después de que la persona haya puesto el PIN
// en su tarjeta es tirarle el trabajo.
func CertificadoParaFirmar(c *x509.Certificate, ahora time.Time) error {
	if ahora.Before(c.NotBefore) || ahora.After(c.NotAfter) {
		return fmt.Errorf("%w: el certificado no está vigente (%s – %s)", ErrCertificadoNoUsable,
			c.NotBefore.Format(time.DateOnly), c.NotAfter.Format(time.DateOnly))
	}
	// KeyUsage vacío es «sin restricción» (RFC 5280 §4.2.1.3); si viene, tiene
	// que permitir firmar. contentCommitment es el nonRepudiation de siempre: el
	// que lleva el certificado de FIRMA del DNI electrónico.
	if c.KeyUsage != 0 && c.KeyUsage&(x509.KeyUsageDigitalSignature|x509.KeyUsageContentCommitment) == 0 {
		return fmt.Errorf("%w: el certificado no es de firma", ErrCertificadoNoUsable)
	}
	switch k := c.PublicKey.(type) {
	case *rsa.PublicKey:
		if k.N.BitLen() < 2048 {
			return fmt.Errorf("%w: una clave RSA de %d bits ya no se admite", ErrCertificadoNoUsable, k.N.BitLen())
		}
	case *ecdsa.PublicKey:
		if k.Curve != elliptic.P256() && k.Curve != elliptic.P384() && k.Curve != elliptic.P521() {
			return fmt.Errorf("%w: curva no admitida", ErrCertificadoNoUsable)
		}
	default:
		return fmt.Errorf("%w: solo se firma con RSA o ECDSA", ErrCertificadoNoUsable)
	}
	return nil
}

// Declaraciones QC (ETSI EN 319 412-5) que miran [NivelDelCertificado].
var (
	oidDeclaracionesQC  = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3}
	oidQCCumple         = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}
	oidQCDispositivoSeg = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}
)

// NivelDelCertificado dice lo que el certificado DECLARA de sí mismo: si es
// cualificado (QcCompliance) y si su clave vive en un dispositivo cualificado de
// creación de firma (QcSSCD). Con los dos, la firma PUEDE ser cualificada; sin los
// dos, es avanzada, y la pantalla no puede decir otra cosa.
//
// Es una DECLARACIÓN del emisor, no una comprobación: saber si ese emisor es de
// verdad un prestador cualificado exige la lista de confianza de la UE, que aquí
// no se consulta (ADR-0030 §lo que decide el propietario). Por eso la pantalla dice
// «puede ser cualificada» y no «es cualificada».
func NivelDelCertificado(c *x509.Certificate) (cualificado, dispositivoSeguro bool) {
	for _, e := range c.Extensions {
		if !e.Id.Equal(oidDeclaracionesQC) {
			continue
		}
		var declaraciones []struct {
			ID   asn1.ObjectIdentifier
			Info asn1.RawValue `asn1:"optional"`
		}
		// Ilegibles = no declara nada: prometer de menos es el error barato.
		if _, err := asn1.Unmarshal(e.Value, &declaraciones); err != nil {
			return false, false
		}
		for _, d := range declaraciones {
			cualificado = cualificado || d.ID.Equal(oidQCCumple)
			dispositivoSeguro = dispositivoSeguro || d.ID.Equal(oidQCDispositivoSeg)
		}
	}
	return cualificado, dispositivoSeguro
}
