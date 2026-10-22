// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package cms firma y verifica CMS SignedData DESPRENDIDO (RFC 5652) con solo la
// biblioteca estándar.
//
// Es el envoltorio común de toda firma digital de la suite: una firma S/MIME
// (RFC 8551), una firma incrustada en un PDF (PAdES, ISO 32000-2 §12.8) y un sello
// de tiempo (RFC 3161) son las tres un CMS SignedData. Lo que cambia entre ellas es
// dónde se pega el resultado, no qué se firma.
//
// # La clave privada no entra aquí
//
// [Sign] recibe un [crypto.Signer], no una clave. La frontera es deliberada
// (ADR-0030): quien firma puede ser una clave efímera en memoria, un HSM por
// PKCS#11 o un prestador cualificado remoto, y este paquete no cambia. Mientras el
// ORIGEN de la clave siga sin decidirse, el empaquetado ya está resuelto y no
// bloquea al resto.
//
// # Perfil de lo que se emite
//
// Atributos firmados: los tres de CMS (content-type, message-digest, signing-time)
// más signing-certificate-v2 (RFC 5035), que ata la firma al certificado CONCRETO
// y no solo a su clave pública. Con esos cuatro y SHA-256, la firma es una
// CAdES-B-B básica; lo que le falta para ser una firma avanzada con validez a
// largo plazo no es código, es el certificado y el sello de tiempo (ADR-0030).
//
// # Lo que NO hace, dicho aquí para que nadie lo descubra tarde
//
//   - Se FIRMA solo desprendido: el contenido no viaja dentro. Es lo que necesitan
//     PAdES (el PDF va aparte, delimitado por /ByteRange) y multipart/signed. Lo
//     que se verifica con el contenido dentro es lo que emite otro: un sello de
//     tiempo ([VerifyEncapsulated]).
//   - Un firmante por documento; sin cofirmas.
//   - No pone sello de tiempo ni datos de revocación: sin ellos la firma deja de
//     poder verificarse cuando caduca el certificado (ADR-0030).
//   - SHA-1 y MD5 se rechazan al verificar, aunque openssl los siga aceptando.
package cms

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// OID de CMS, de los atributos y de los algoritmos (RFC 5652, RFC 5035, RFC 5758).
var (
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningTime   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 5}
	oidSigningCertV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}

	oidSHA256 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA384 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}

	oidRSAEncryption   = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidRSAPSS          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 10}
	oidMGF1            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 8}
	oidECDSAWithSHA256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
)

// --- Estructuras CMS (RFC 5652) ---------------------------------------------

type contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type signedData struct {
	Version          int
	DigestAlgorithms []algorithmIdentifier `asn1:"set"`
	EncapContentInfo encapContentInfo
	Certificates     asn1.RawValue `asn1:"optional,tag:0"`
	// CRLs no se usa, pero el campo tiene que existir: una firma ajena que las
	// traiga fallaría al parsear si el struct no las contempla.
	CRLs        asn1.RawValue `asn1:"optional,tag:1"`
	SignerInfos []signerInfo  `asn1:"set"`
}

type encapContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

type algorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type issuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type signerInfo struct {
	Version int
	// SID es un CHOICE (issuerAndSerialNumber | [0] subjectKeyIdentifier). Se
	// guarda crudo para poder dar un error legible en vez de uno de ASN.1 cuando
	// llega la variante que no soportamos.
	SID                asn1.RawValue
	DigestAlgorithm    algorithmIdentifier
	SignedAttrs        asn1.RawValue `asn1:"optional,tag:0"`
	SignatureAlgorithm algorithmIdentifier
	Signature          []byte
	// UnsignedAttrs es donde vive el sello de tiempo RFC 3161 de una firma ajena:
	// se saca tal cual (Signature.TimestampToken) y lo verifica quien llama.
	UnsignedAttrs asn1.RawValue `asn1:"optional,tag:1"`
}

type attribute struct {
	Type   asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

// essCertIDv2 (RFC 5035). hashAlgorithm es DEFAULT sha256, así que al emitir se
// omite; al leer una firma ajena puede venir, y también el issuerSerial.
type essCertIDv2 struct {
	HashAlgorithm algorithmIdentifier `asn1:"optional"`
	CertHash      []byte
	IssuerSerial  asn1.RawValue `asn1:"optional"`
}

type signingCertificateV2 struct {
	Certs    []essCertIDv2
	Policies asn1.RawValue `asn1:"optional"`
}

// --- Firmar ------------------------------------------------------------------

// Sign devuelve el CMS SignedData (DER) que firma content SIN incluirlo.
//
// certs[0] es el certificado del firmante; el resto son los intermedios, que se
// incrustan para que quien verifique pueda construir la cadena sin ir a buscarlos.
// El digest es SHA-256 y no se configura: no hay ningún caso que pida otro y una
// opción de más aquí es una manera de acabar firmando con SHA-1.
func Sign(signer crypto.Signer, certs []*x509.Certificate, content []byte) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("cms: no hay firmante")
	}
	if len(certs) == 0 || certs[0] == nil {
		return nil, errors.New("cms: hace falta el certificado del firmante en certs[0]")
	}
	if len(content) == 0 {
		return nil, errors.New("cms: no hay contenido que firmar")
	}
	cert := certs[0]
	// Firmar con una clave que no es la del certificado produce una firma que solo
	// falla en casa del destinatario. Se detecta aquí.
	pub, ok := cert.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !pub.Equal(signer.Public()) {
		return nil, errors.New("cms: la clave del firmante no es la del certificado")
	}
	sigOID, err := signatureOID(cert.PublicKey)
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256(content)
	attrs, err := signedAttributes(cert, digest[:], time.Now().UTC())
	if err != nil {
		return nil, err
	}
	// Lo que se firma es el conjunto de atributos con etiqueta SET OF (RFC 5652
	// §5.4), no el contenido. Se serializa el SET DESNUDO: envolverlo en un struct
	// añadiría un SEQUENCE que hace que openssl rechace la firma con «wrong tag».
	setDER, err := asn1.MarshalWithParams(attrs, "set")
	if err != nil {
		return nil, fmt.Errorf("cms: serializar los atributos firmados: %w", err)
	}
	sum := sha256.Sum256(setDER)
	signature, err := signer.Sign(rand.Reader, sum[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("cms: firmar: %w", err)
	}
	// En el SignerInfo ese MISMO conjunto va con etiqueta [0] IMPLICIT: la etiqueta
	// SUSTITUYE a la del SET, no lo envuelve.
	signedAttrsDER := retag(setDER, 0xa0)

	sidDER, err := asn1.Marshal(issuerAndSerial{
		Issuer: asn1.RawValue{FullBytes: cert.RawIssuer},
		Serial: cert.SerialNumber,
	})
	if err != nil {
		return nil, fmt.Errorf("cms: serializar el identificador del firmante: %w", err)
	}

	var certsDER []byte
	for _, c := range certs {
		certsDER = append(certsDER, c.Raw...)
	}

	sd := signedData{
		Version:          1,
		DigestAlgorithms: []algorithmIdentifier{{Algorithm: oidSHA256, Parameters: asn1.NullRawValue}},
		EncapContentInfo: encapContentInfo{ContentType: oidData}, // sin Content: desprendido
		Certificates:     asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: certsDER},
		SignerInfos: []signerInfo{{
			Version:            1,
			SID:                asn1.RawValue{FullBytes: sidDER},
			DigestAlgorithm:    algorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.NullRawValue},
			SignedAttrs:        asn1.RawValue{FullBytes: signedAttrsDER},
			SignatureAlgorithm: algorithmIdentifier{Algorithm: sigOID},
			Signature:          signature,
		}},
	}
	sdDER, err := asn1.Marshal(sd)
	if err != nil {
		return nil, fmt.Errorf("cms: serializar SignedData: %w", err)
	}
	out, err := asn1.Marshal(contentInfo{
		ContentType: oidSignedData,
		Content:     asn1.RawValue{Class: 2, Tag: 0, IsCompound: true, Bytes: sdDER},
	})
	if err != nil {
		return nil, fmt.Errorf("cms: serializar ContentInfo: %w", err)
	}
	return out, nil
}

// signedAttributes arma los cuatro atributos firmados del perfil.
func signedAttributes(cert *x509.Certificate, digest []byte, now time.Time) ([]attribute, error) {
	certHash := sha256.Sum256(cert.Raw)
	scv2, err := asn1.Marshal(signingCertificateV2{Certs: []essCertIDv2{{CertHash: certHash[:]}}})
	if err != nil {
		return nil, fmt.Errorf("cms: serializar signing-certificate-v2: %w", err)
	}
	ct, err := asn1.Marshal(oidData)
	if err != nil {
		return nil, err
	}
	md, err := asn1.Marshal(digest)
	if err != nil {
		return nil, err
	}
	// signing-time es lo que DICE el firmante, no una prueba: la hora de verdad
	// solo la da un sello RFC 3161 de un tercero (ADR-0030).
	st, err := asn1.Marshal(now)
	if err != nil {
		return nil, err
	}
	return []attribute{
		{Type: oidContentType, Values: set(ct)},
		{Type: oidMessageDigest, Values: set(md)},
		{Type: oidSigningTime, Values: set(st)},
		{Type: oidSigningCertV2, Values: set(scv2)},
	}, nil
}

func set(der []byte) asn1.RawValue {
	return asn1.RawValue{Class: 0, Tag: asn1.TagSet, IsCompound: true, Bytes: der}
}

// retag cambia la etiqueta exterior de un DER ya serializado (SET → [0] IMPLICIT).
// La etiqueta de contexto 0 es de forma corta, así que es un único byte.
func retag(der []byte, tag byte) []byte {
	out := make([]byte, len(der))
	copy(out, der)
	out[0] = tag
	return out
}

// signatureOID elige el algoritmo de firma a partir del tipo de clave.
//
// Ed25519 queda fuera a propósito: ni CAdES ni PAdES lo admiten, y su crypto.Signer
// exige el mensaje entero en vez del digest, que es justo lo contrario de lo que
// permite firmar contra un HSM remoto.
func signatureOID(pub crypto.PublicKey) (asn1.ObjectIdentifier, error) {
	switch pub.(type) {
	case *rsa.PublicKey:
		return oidRSAEncryption, nil
	case *ecdsa.PublicKey:
		return oidECDSAWithSHA256, nil
	default:
		return nil, fmt.Errorf("cms: tipo de clave no admitido para firmar: %T", pub)
	}
}

// --- Verificar ---------------------------------------------------------------

// ErrSinAnclas indica que [Verify] no recibió contra qué confiar. No hay valor por
// defecto y no lo habrá: el almacén de raíces del sistema NO es la lista de
// confianza de la UE, y dejar que Verify caiga en él sería tomar la decisión de
// confianza en silencio, con lo que trajera la imagen del contenedor.
var ErrSinAnclas = errors.New("cms: hay que decir contra qué anclas de confianza se verifica")

// ErrNoCorresponde es la firma LEGIBLE que no corresponde a lo firmado: otro
// contenido, otro certificado o un valor de firma que no cuadra. Se distingue del
// resto de errores —una firma ilegible, un algoritmo que no se admite— porque son
// dos mensajes distintos para quien la mira: «alguien cambió esto» frente a «esto
// no se sabe comprobar».
var ErrNoCorresponde = errors.New("cms: la firma no corresponde a lo firmado")

// ErrSerieNegativa es un certificado con número de serie negativo, que RFC 5280
// prohíbe y la biblioteca estándar de Go rechaza. No se relaja (GODEBUG
// x509negativeserial): se informa como certificado no válido.
var ErrSerieNegativa = errors.New("cms: certificado no válido: número de serie negativo")

// Signature es lo que una firma verificada dice de sí misma.
type Signature struct {
	// Signer es el certificado que firmó.
	Signer *x509.Certificate
	// Chains son las cadenas válidas hasta un ancla de opts.Roots. Vacío cuando
	// sale de [Inspect], que no decide la confianza.
	Chains [][]*x509.Certificate
	// SigningTime es la hora DECLARADA por el firmante. No es prueba de nada
	// mientras no haya un sello de tiempo de un tercero.
	SigningTime time.Time
	// Certificates son todos los que la firma trae dentro, el firmante incluido:
	// con ellos se construye la cadena sin ir a buscar nada fuera.
	Certificates []*x509.Certificate
	// Value es el valor de la firma. Un sello RFC 3161 sobre la firma sella el
	// resumen de ESTOS bytes, así que quien lo compruebe los necesita.
	Value []byte
	// SinAtributosFirmados: la firma va directamente sobre el contenido, sin
	// atributos firmados (CMS lo permite con contenido de tipo data). Se verifica
	// igual —firma, certificado y cadena—, pero no hay hora ni tipo de contenido
	// firmados, y quien la enseñe debe decirlo.
	SinAtributosFirmados bool
	// TimestampToken es el sello RFC 3161 colgado como atributo NO firmado
	// (id-aa-signatureTimeStampToken), con sus bytes tal cual; nil si no lo hay.
	// No se verifica aquí: es otro SignedData, de otra autoridad, y su confianza
	// se decide con otras anclas.
	TimestampToken []byte
}

// Verify comprueba una firma CMS desprendida sobre content y devuelve quién la
// hizo. Verifica, en este orden: que el resumen firmado es el de content, que la
// firma es válida sobre los atributos firmados, que el certificado declarado en
// signing-certificate-v2 es el que firma, y que la cadena llega a opts.Roots.
//
// opts.Roots es OBLIGATORIO: contra qué anclas se confía es una decisión del
// producto (el almacén del sistema NO es la lista de confianza de la UE), y
// dejarla implícita sería confiar en lo que hubiera en la imagen del contenedor.
func Verify(der, content []byte, opts x509.VerifyOptions) (*Signature, error) {
	if opts.Roots == nil {
		return nil, ErrSinAnclas
	}
	sig, err := Inspect(der, content)
	if err != nil {
		return nil, err
	}
	return confiar(sig, opts)
}

// Inspect comprueba la INTEGRIDAD de una firma desprendida —el resumen, la firma
// sobre los atributos firmados y signing-certificate-v2— y NO decide la
// confianza: Chains sale vacío.
//
// Existe para quien tiene que contar las dos cosas por separado. Un validador de
// PDF dice «el documento no ha cambiado desde que se firmó» aunque no conozca a
// la autoridad que emitió el certificado, y decide la confianza después con SUS
// anclas y a SU hora —la de un sello de tiempo, que no tiene por qué ser ahora—.
// Quien solo quiere un sí o un no usa [Verify].
func Inspect(der, content []byte) (*Signature, error) {
	if len(content) == 0 {
		return nil, errors.New("cms: no hay contenido contra el que verificar")
	}
	sd, err := signedDataDe(der)
	if err != nil {
		return nil, err
	}
	if len(sd.EncapContentInfo.Content.FullBytes) != 0 {
		return nil, errors.New("cms: la firma lleva el contenido dentro y aquí solo se admiten firmas desprendidas")
	}
	return integridad(sd, content, oidData)
}

// VerifyEncapsulated verifica un SignedData que lleva el contenido DENTRO y
// devuelve ese contenido, ya verificado. Un sello de tiempo RFC 3161 es así: el
// TSTInfo viaja en el eContent, firmado por la autoridad.
//
// `tipo` es el content-type que se espera (id-ct-TSTInfo para un sello), y se exige
// en los DOS sitios donde va —el eContentType y el atributo firmado—: una firma
// buena sobre otra clase de contenido no es un sello. Para exigir además el uso
// timeStamping, quien llama lo pone en opts.KeyUsages.
func VerifyEncapsulated(der []byte, tipo asn1.ObjectIdentifier, opts x509.VerifyOptions) (*Signature, []byte, error) {
	if opts.Roots == nil {
		return nil, nil, ErrSinAnclas
	}
	sig, content, err := InspectEncapsulated(der, tipo)
	if err != nil {
		return nil, nil, err
	}
	if sig, err = confiar(sig, opts); err != nil {
		return nil, nil, err
	}
	return sig, content, nil
}

// InspectEncapsulated es [Inspect] para un SignedData con el contenido dentro:
// comprueba la integridad, devuelve el contenido y deja la confianza a quien
// llama.
func InspectEncapsulated(der []byte, tipo asn1.ObjectIdentifier) (*Signature, []byte, error) {
	sd, err := signedDataDe(der)
	if err != nil {
		return nil, nil, err
	}
	if !sd.EncapContentInfo.ContentType.Equal(tipo) {
		return nil, nil, fmt.Errorf("cms: el contenido es %v y se esperaba %v", sd.EncapContentInfo.ContentType, tipo)
	}
	// Con `explicit`, FullBytes incluye la envoltura [0]; Bytes contiene el OCTET
	// STRING de dentro, cuyos bytes son el contenido que se resume.
	var content []byte
	if _, err := asn1.Unmarshal(sd.EncapContentInfo.Content.Bytes, &content); err != nil || len(content) == 0 {
		return nil, nil, errors.New("cms: el contenido de dentro no es un OCTET STRING")
	}
	sig, err := integridad(sd, content, tipo)
	if err != nil {
		return nil, nil, err
	}
	return sig, content, nil
}

// integridad es lo común a las dos formas: resumen, firma sobre los atributos y
// signing-certificate-v2. La cadena va aparte, en confiar.
func integridad(sd signedData, content []byte, tipo asn1.ObjectIdentifier) (*Signature, error) {
	si, signer, certs, err := firmanteDe(sd)
	if err != nil {
		return nil, err
	}

	hash, err := hashFromOID(si.DigestAlgorithm.Algorithm)
	if err != nil {
		return nil, err
	}
	if len(si.SignedAttrs.FullBytes) == 0 {
		// Sin atributos firmados la firma cubre el contenido mismo (RFC 5652
		// §5.4), y solo se admite con contenido de tipo data: un sello de tiempo
		// sin ellos no dice qué sella.
		if !tipo.Equal(oidData) {
			return nil, errors.New("cms: la firma no lleva atributos firmados")
		}
		algo, err := verifyAlgorithm(si.SignatureAlgorithm.Algorithm, hash, signer.PublicKey)
		if err != nil {
			return nil, err
		}
		if err := comprobarFirma(signer, algo, si.SignatureAlgorithm, hash, content, si.Signature); err != nil {
			return nil, err
		}
		return &Signature{
			Signer: signer, Certificates: certs, Value: si.Signature,
			TimestampToken: selloDe(si.UnsignedAttrs), SinAtributosFirmados: true,
		}, nil
	}
	attrs, err := parseAttributes(si.SignedAttrs)
	if err != nil {
		return nil, err
	}
	if err := checkContentType(attrs, tipo); err != nil {
		return nil, err
	}
	if err := checkMessageDigest(attrs, hash, content); err != nil {
		return nil, err
	}
	if err := checkSigningCertificate(attrs, signer); err != nil {
		return nil, err
	}

	algo, err := verifyAlgorithm(si.SignatureAlgorithm.Algorithm, hash, signer.PublicKey)
	if err != nil {
		return nil, err
	}
	// Lo firmado es el SET OF de atributos, no el [0] con el que viaja.
	if err := comprobarFirma(signer, algo, si.SignatureAlgorithm, hash, retag(si.SignedAttrs.FullBytes, 0x31), si.Signature); err != nil {
		return nil, err
	}
	return &Signature{
		Signer: signer, SigningTime: signingTime(attrs), Certificates: certs,
		Value: si.Signature, TimestampToken: selloDe(si.UnsignedAttrs),
	}, nil
}

// confiar construye la cadena del firmante hasta opts.Roots con los intermedios
// que la firma trae dentro.
func confiar(sig *Signature, opts x509.VerifyOptions) (*Signature, error) {
	// Sin esto x509 exige ExtKeyUsageServerAuth por defecto y rechaza cualquier
	// certificado de firma de documento. Quien necesite acotar el uso lo pasa.
	if len(opts.KeyUsages) == 0 {
		opts.KeyUsages = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
	}
	if opts.Intermediates == nil {
		pool := x509.NewCertPool()
		for _, c := range sig.Certificates {
			if !c.Equal(sig.Signer) {
				pool.AddCert(c)
			}
		}
		opts.Intermediates = pool
	}
	chains, err := sig.Signer.Verify(opts)
	if err != nil {
		return nil, fmt.Errorf("cms: el certificado del firmante no es de confianza: %w", err)
	}
	sig.Chains = chains
	return sig, nil
}

// oidSelloDeFirma es id-aa-signatureTimeStampToken (RFC 3161 apéndice A).
var oidSelloDeFirma = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}

// selloDe saca el token RFC 3161 de los atributos no firmados, o nil. Un
// atributo ilegible es lo mismo que no tenerlo: no invalida la firma, que no lo
// cubre, y quien enseñe el resultado dirá «sin sello».
func selloDe(raw asn1.RawValue) []byte {
	if len(raw.FullBytes) == 0 {
		return nil
	}
	var attrs []attribute
	if _, err := asn1.UnmarshalWithParams(retag(raw.FullBytes, 0x31), &attrs, "set"); err != nil {
		return nil
	}
	v, ok := attrValue(attrs, oidSelloDeFirma)
	if !ok {
		return nil
	}
	var token asn1.RawValue
	if _, err := asn1.Unmarshal(v, &token); err != nil {
		return nil
	}
	return token.FullBytes
}

// signedDataDe desmonta el ContentInfo hasta el SignedData.
func signedDataDe(der []byte) (signedData, error) {
	var ci contentInfo
	rest, err := asn1.Unmarshal(der, &ci)
	if err != nil {
		// Hay firmantes que emiten BER (longitudes indefinidas): se pasa a DER y se
		// vuelve a intentar.
		convertido, errBer := berADer(der)
		if errBer != nil {
			return signedData{}, fmt.Errorf("cms: no es un ContentInfo válido: %w", err)
		}
		if rest, err = asn1.Unmarshal(convertido, &ci); err != nil {
			return signedData{}, fmt.Errorf("cms: no es un ContentInfo válido: %w", err)
		}
	}
	if len(rest) != 0 {
		return signedData{}, errors.New("cms: sobran bytes después de la firma")
	}
	if !ci.ContentType.Equal(oidSignedData) {
		return signedData{}, fmt.Errorf("cms: el contenido no es SignedData sino %v", ci.ContentType)
	}
	var sd signedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return signedData{}, fmt.Errorf("cms: no es un SignedData válido: %w", err)
	}
	return sd, nil
}

// firmanteDe localiza el único SignerInfo y a su firmante entre los certificados
// incrustados.
// maxBytesCertificados acota los certificados que una firma trae dentro.
const maxBytesCertificados = 1 << 20

func firmanteDe(sd signedData) (signerInfo, *x509.Certificate, []*x509.Certificate, error) {
	var vacío signerInfo
	if len(sd.SignerInfos) != 1 {
		return vacío, nil, nil, fmt.Errorf("cms: se esperaba un firmante y hay %d", len(sd.SignerInfos))
	}
	// Entrada no confiable: un PDF recibido puede traer millones de certificados
	// minúsculos en el hueco de la firma, y cada uno se parsea y entra en el pool de
	// intermedios. Una firma real lleva una cadena de unos pocos KiB.
	if len(sd.Certificates.Bytes) > maxBytesCertificados {
		return vacío, nil, nil, fmt.Errorf("cms: los certificados incrustados pasan de %d bytes", maxBytesCertificados)
	}
	certs, err := x509.ParseCertificates(sd.Certificates.Bytes)
	if err != nil {
		if strings.Contains(err.Error(), "negative serial number") {
			return vacío, nil, nil, ErrSerieNegativa
		}
		return vacío, nil, nil, fmt.Errorf("cms: los certificados incrustados no se pueden leer: %w", err)
	}
	si := sd.SignerInfos[0]
	signer, err := findSigner(certs, si.SID)
	if err != nil {
		return vacío, nil, nil, err
	}
	return si, signer, certs, nil
}

// findSigner localiza el certificado del firmante entre los incrustados.
func findSigner(certs []*x509.Certificate, sid asn1.RawValue) (*x509.Certificate, error) {
	if sid.Class == asn1.ClassContextSpecific {
		return nil, errors.New("cms: el firmante se identifica por subjectKeyIdentifier y aquí solo se admite issuerAndSerialNumber")
	}
	var ias issuerAndSerial
	if _, err := asn1.Unmarshal(sid.FullBytes, &ias); err != nil {
		return nil, fmt.Errorf("cms: no se puede leer el identificador del firmante: %w", err)
	}
	for _, c := range certs {
		if bytes.Equal(c.RawIssuer, ias.Issuer.FullBytes) && c.SerialNumber.Cmp(ias.Serial) == 0 {
			return c, nil
		}
	}
	return nil, errors.New("cms: el certificado del firmante no viene en la firma")
}

// parseAttributes devuelve los atributos firmados indexados por OID.
func parseAttributes(raw asn1.RawValue) ([]attribute, error) {
	var attrs []attribute
	if _, err := asn1.UnmarshalWithParams(retag(raw.FullBytes, 0x31), &attrs, "set"); err != nil {
		return nil, fmt.Errorf("cms: los atributos firmados no se pueden leer: %w", err)
	}
	if len(attrs) == 0 {
		return nil, errors.New("cms: el conjunto de atributos firmados está vacío")
	}
	return attrs, nil
}

func attrValue(attrs []attribute, oid asn1.ObjectIdentifier) ([]byte, bool) {
	for _, a := range attrs {
		if a.Type.Equal(oid) {
			return a.Values.Bytes, true
		}
	}
	return nil, false
}

func checkContentType(attrs []attribute, tipo asn1.ObjectIdentifier) error {
	v, ok := attrValue(attrs, oidContentType)
	if !ok {
		return errors.New("cms: falta el atributo content-type")
	}
	var got asn1.ObjectIdentifier
	if _, err := asn1.Unmarshal(v, &got); err != nil {
		return fmt.Errorf("cms: content-type ilegible: %w", err)
	}
	if !got.Equal(tipo) {
		return fmt.Errorf("cms: el content-type firmado es %v y se esperaba %v", got, tipo)
	}
	return nil
}

func checkMessageDigest(attrs []attribute, hash crypto.Hash, content []byte) error {
	v, ok := attrValue(attrs, oidMessageDigest)
	if !ok {
		return errors.New("cms: falta el atributo message-digest")
	}
	var declared []byte
	if _, err := asn1.Unmarshal(v, &declared); err != nil {
		return fmt.Errorf("cms: message-digest ilegible: %w", err)
	}
	h := hash.New()
	h.Write(content)
	if subtle.ConstantTimeCompare(declared, h.Sum(nil)) != 1 {
		return fmt.Errorf("%w: el contenido no es el que se firmó", ErrNoCorresponde)
	}
	return nil
}

// checkSigningCertificate comprueba que el certificado que verifica la firma es el
// MISMO que el firmante declaró dentro de lo firmado. Sin esta atadura, otro
// certificado con la misma clave pública pasaría por bueno.
func checkSigningCertificate(attrs []attribute, signer *x509.Certificate) error {
	v, ok := attrValue(attrs, oidSigningCertV2)
	if !ok {
		// Ausente = la firma no es CAdES. Se acepta: CMS puro es válido y hay
		// firmadores que no lo emiten. Presente y falso, en cambio, se rechaza.
		return nil
	}
	var scv2 signingCertificateV2
	if _, err := asn1.Unmarshal(v, &scv2); err != nil {
		return fmt.Errorf("cms: signing-certificate-v2 ilegible: %w", err)
	}
	if len(scv2.Certs) == 0 {
		return errors.New("cms: signing-certificate-v2 no nombra ningún certificado")
	}
	first := scv2.Certs[0]
	hash := crypto.SHA256
	if len(first.HashAlgorithm.Algorithm) > 0 {
		var err error
		if hash, err = hashFromOID(first.HashAlgorithm.Algorithm); err != nil {
			return err
		}
	}
	h := hash.New()
	h.Write(signer.Raw)
	if subtle.ConstantTimeCompare(first.CertHash, h.Sum(nil)) != 1 {
		return fmt.Errorf("%w: la firma nombra un certificado distinto del que la verifica", ErrNoCorresponde)
	}
	return nil
}

func signingTime(attrs []attribute) time.Time {
	v, ok := attrValue(attrs, oidSigningTime)
	if !ok {
		return time.Time{}
	}
	var t time.Time
	if _, err := asn1.Unmarshal(v, &t); err != nil {
		// Informativo: una hora ilegible no invalida la firma, solo se ignora.
		return time.Time{}
	}
	return t.UTC()
}

// hashFromOID mapea el OID del resumen. SHA-1 y MD5 NO están y es deliberado:
// x509.CheckSignature todavía acepta SHA-1 y aquí no se acepta.
func hashFromOID(oid asn1.ObjectIdentifier) (crypto.Hash, error) {
	switch {
	case oid.Equal(oidSHA256):
		return crypto.SHA256, nil
	case oid.Equal(oidSHA384):
		return crypto.SHA384, nil
	case oid.Equal(oidSHA512):
		return crypto.SHA512, nil
	default:
		return 0, fmt.Errorf("cms: algoritmo de resumen no admitido: %v", oid)
	}
}

// verifyAlgorithm decide con qué esquema se comprueba la firma.
//
// La FAMILIA sale del tipo de clave del certificado y el resumen del campo
// digestAlgorithm, no del OID de firma: en el mundo real ese OID aparece como
// rsaEncryption o como sha256WithRSAEncryption indistintamente, y fiarse de él
// hace fallar firmas válidas. Solo se le pregunta si es PSS, que sí cambia el
// esquema.
func verifyAlgorithm(sigOID asn1.ObjectIdentifier, hash crypto.Hash, pub crypto.PublicKey) (x509.SignatureAlgorithm, error) {
	// Exhaustivo a propósito: un `default` que eligiera un esquema en vez de fallar
	// haría que un resumen no contemplado se verificara con el algoritmo de otro.
	var conRSA, conPSS, conECDSA x509.SignatureAlgorithm
	switch hash {
	case crypto.SHA256:
		conRSA, conPSS, conECDSA = x509.SHA256WithRSA, x509.SHA256WithRSAPSS, x509.ECDSAWithSHA256
	case crypto.SHA384:
		conRSA, conPSS, conECDSA = x509.SHA384WithRSA, x509.SHA384WithRSAPSS, x509.ECDSAWithSHA384
	case crypto.SHA512:
		conRSA, conPSS, conECDSA = x509.SHA512WithRSA, x509.SHA512WithRSAPSS, x509.ECDSAWithSHA512
	default:
		return 0, fmt.Errorf("cms: resumen no admitido al verificar: %v", hash)
	}
	switch pub.(type) {
	case *rsa.PublicKey:
		if sigOID.Equal(oidRSAPSS) {
			return conPSS, nil
		}
		return conRSA, nil
	case *ecdsa.PublicKey:
		return conECDSA, nil
	default:
		return 0, fmt.Errorf("cms: tipo de clave no admitido al verificar: %T", pub)
	}
}

// comprobarFirma verifica la firma sobre `datos`. Todo esquema sale por
// x509.CheckSignature salvo RSASSA-PSS, que lleva en sus parámetros la longitud de
// la sal y x509 solo comprueba la que es igual al resumen.
func comprobarFirma(signer *x509.Certificate, algo x509.SignatureAlgorithm, sigAlg algorithmIdentifier,
	hash crypto.Hash, datos, firma []byte,
) error {
	if pub, ok := signer.PublicKey.(*rsa.PublicKey); ok && sigAlg.Algorithm.Equal(oidRSAPSS) {
		return comprobarPSS(pub, sigAlg.Parameters, hash, datos, firma)
	}
	if err := signer.CheckSignature(algo, datos, firma); err != nil {
		return fmt.Errorf("%w: la firma no cuadra: %w", ErrNoCorresponde, err)
	}
	return nil
}

// parametrosPSS es RSASSA-PSS-params (RFC 4055 §3.1). Los valores por defecto de la
// norma son SHA-1 —que aquí no se acepta— y sal de 20 bytes.
type parametrosPSS struct {
	Hash    pkix.AlgorithmIdentifier `asn1:"explicit,tag:0,optional"`
	MGF     pkix.AlgorithmIdentifier `asn1:"explicit,tag:1,optional"`
	Sal     int                      `asn1:"explicit,tag:2,optional,default:20"`
	Trailer int                      `asn1:"explicit,tag:3,optional,default:1"`
}

// comprobarPSS verifica RSASSA-PSS con la longitud de sal que DECLARA la firma. No
// hay una sola: x509 solo admite la igual al resumen, y pyHanko, por ejemplo,
// firma con la máxima que cabe en la clave (222 bytes con RSA-2048 y SHA-256), que
// es una firma legítima. El resumen y el de la función de máscara (MGF1) tienen que
// ser el mismo que el de la firma: es lo único que admite la biblioteca estándar y
// lo único que se emite en la práctica.
func comprobarPSS(pub *rsa.PublicKey, params asn1.RawValue, hash crypto.Hash, datos, firma []byte) error {
	var p parametrosPSS
	if len(params.FullBytes) == 0 {
		return errors.New("cms: RSASSA-PSS sin parámetros (el resumen por omisión es SHA-1)")
	}
	if rest, err := asn1.Unmarshal(params.FullBytes, &p); err != nil || len(rest) != 0 {
		return errors.New("cms: parámetros RSASSA-PSS ilegibles")
	}
	if h, err := hashFromOID(p.Hash.Algorithm); err != nil || h != hash {
		return errors.New("cms: RSASSA-PSS con un resumen distinto del de la firma")
	}
	var mgfHash pkix.AlgorithmIdentifier
	if !p.MGF.Algorithm.Equal(oidMGF1) {
		return errors.New("cms: RSASSA-PSS con una función de máscara que no es MGF1")
	}
	if rest, err := asn1.Unmarshal(p.MGF.Parameters.FullBytes, &mgfHash); err != nil || len(rest) != 0 {
		return errors.New("cms: parámetros de MGF1 ilegibles")
	}
	if h, err := hashFromOID(mgfHash.Algorithm); err != nil || h != hash {
		return errors.New("cms: MGF1 con un resumen distinto del de la firma")
	}
	if p.Trailer != 1 {
		return errors.New("cms: RSASSA-PSS con un trailerField que no es 1")
	}
	if p.Sal < 0 || p.Sal > pub.Size() {
		return errors.New("cms: RSASSA-PSS con una longitud de sal imposible")
	}
	resumen := hash.New()
	resumen.Write(datos)
	// PSSSaltLengthAuto es 0 y PSSSaltLengthEqualsHash -1: la sal declarada es la
	// que se exige, salvo la de 0 bytes, que la biblioteca no distingue de «la que
	// venga» y que no emite nadie.
	sal := p.Sal
	if sal == 0 {
		sal = rsa.PSSSaltLengthAuto
	}
	if err := rsa.VerifyPSS(pub, hash, resumen.Sum(nil), firma, &rsa.PSSOptions{SaltLength: sal, Hash: hash}); err != nil {
		return fmt.Errorf("%w: la firma no cuadra: %w", ErrNoCorresponde, err)
	}
	return nil
}
