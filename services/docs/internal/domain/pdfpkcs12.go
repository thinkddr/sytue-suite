// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Lectura de PKCS#12 (`.p12` / `.pfx`) para poder firmar con el certificado tal
// y como lo entrega quien lo emite.
//
// POR QUÉ HACE FALTA. En España un certificado de la FNMT, el de representante
// de una empresa o el de un colegio profesional se descargan SIEMPRE como `.p12`.
// Pedirle a alguien que lo convierta a PEM con `openssl` antes de firmar una
// factura es, en la práctica, decirle que no puede firmar. La biblioteca estándar
// de Go no trae PKCS#12 —y meter una dependencia en el camino de una clave
// privada es lo último que uno quiere—, así que se lee aquí.
//
// QUÉ SE SOPORTA: los dos envoltorios que produce todo el mundo —PBES2 con
// AES-CBC y HMAC-SHA-256 (lo moderno, y lo que emite OpenSSL 3) y el clásico
// PBE-SHA1-3DES / RC2-40 (lo que emiten los gestores antiguos y buena parte de
// las autoridades de certificación españolas)—. Todo con la biblioteca estándar.
package domain

import (
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des" //nolint:gosec // G502/G405: 3DES lo exige el formato PKCS#12 clásico
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // G505: la derivación de clave de PKCS#12 está definida sobre SHA-1
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
)

// ErrPKCS12 cubre todo lo que impide abrir el fichero.
var ErrPKCS12 = errors.New("pdf: el fichero .p12 no se pudo abrir")

// Identificadores del formato (RFC 7292 y PKCS#5).
var (
	oidDatosCMS         = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidCifradoCMS       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 6}
	oidBolsaClave       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 2} // pkcs8ShroudedKeyBag
	oidBolsaClaveSimple = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 1}
	oidBolsaCertificado = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 10, 1, 3}
	oidCertificadoX509  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 22, 1}
	oidPBES2            = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACSHA256       = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidAES128CBC        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES192CBC        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 22}
	oidAES256CBC        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
	oidPBEConSHA1y3DES  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 12, 1, 3}
)

type pfx struct {
	Version  int
	AuthSafe contenidoInfo
	MacData  asn1.RawValue `asn1:"optional"`
}

type contenidoCifrado struct {
	TipoContenido asn1.ObjectIdentifier
	Algoritmo     pkix.AlgorithmIdentifier
	Contenido     []byte `asn1:"tag:0,implicit,optional"`
}

type datosCifrados struct {
	Version   int
	Contenido contenidoCifrado
}

type bolsa struct {
	ID        asn1.ObjectIdentifier
	Valor     asn1.RawValue `asn1:"tag:0,explicit"`
	Atributos asn1.RawValue `asn1:"set,optional"`
}

type parametrosPBE struct {
	Sal         []byte
	Iteraciones int
}

type parametrosPBKDF2 struct {
	Sal         []byte
	Iteraciones int
	Longitud    int                      `asn1:"optional"`
	PRF         pkix.AlgorithmIdentifier `asn1:"optional"`
}

type parametrosPBES2 struct {
	KDF     pkix.AlgorithmIdentifier
	Cifrado pkix.AlgorithmIdentifier
}

// LeerPKCS12 extrae el certificado, su cadena y la clave privada de un `.p12`.
// cifrado y tres formatos de clave; el anidamiento es el del estándar.
//
//nolint:gocognit,gocyclo // PKCS#12 es una matrioska de ASN.1 con dos algoritmos de
func LeerPKCS12(datos []byte, contrasena string) (cadena []*x509.Certificate, clave crypto.Signer, err error) {
	var envoltorio pfx
	if _, err := asn1.Unmarshal(datos, &envoltorio); err != nil {
		return nil, nil, fmt.Errorf("%w: no es un PKCS#12", ErrPKCS12)
	}
	if !envoltorio.AuthSafe.Tipo.Equal(oidDatosCMS) {
		return nil, nil, fmt.Errorf("%w: envoltorio no soportado", ErrPKCS12)
	}
	var interior []byte
	if _, err := asn1.Unmarshal(envoltorio.AuthSafe.Contenido.Bytes, &interior); err != nil {
		return nil, nil, fmt.Errorf("%w: contenido ilegible", ErrPKCS12)
	}

	var trozos []contenidoInfo
	if _, err := asn1.Unmarshal(interior, &trozos); err != nil {
		return nil, nil, fmt.Errorf("%w: estructura inesperada", ErrPKCS12)
	}

	pass := []byte(contrasena)
	for _, t := range trozos {
		var bolsasDER []byte
		switch {
		case t.Tipo.Equal(oidDatosCMS):
			if _, err := asn1.Unmarshal(t.Contenido.Bytes, &bolsasDER); err != nil {
				continue
			}
		case t.Tipo.Equal(oidCifradoCMS):
			var cif datosCifrados
			if _, err := asn1.Unmarshal(t.Contenido.Bytes, &cif); err != nil {
				continue
			}
			claro, err := descifrarPKCS12(cif.Contenido.Algoritmo, cif.Contenido.Contenido, pass)
			if err != nil {
				return nil, nil, err
			}
			bolsasDER = claro
		default:
			continue
		}

		var bolsas []bolsa
		if _, err := asn1.Unmarshal(bolsasDER, &bolsas); err != nil {
			continue
		}
		for _, b := range bolsas {
			switch {
			case b.ID.Equal(oidBolsaCertificado):
				if c := certificadoDeBolsa(b); c != nil {
					cadena = append(cadena, c)
				}
			case b.ID.Equal(oidBolsaClave):
				claro, err := descifrarClaveEnvueltaPKCS8(b.Valor.Bytes, pass)
				if err != nil {
					return nil, nil, err
				}
				if s, err := parsearClave(claro); err == nil {
					clave = s
				}
			case b.ID.Equal(oidBolsaClaveSimple):
				if s, err := parsearClave(b.Valor.Bytes); err == nil {
					clave = s
				}
			}
		}
	}

	if len(cadena) == 0 || clave == nil {
		return nil, nil, fmt.Errorf("%w: no se encontró el certificado o la clave (¿contraseña incorrecta?)", ErrPKCS12)
	}
	// El certificado del FIRMANTE va primero: es el que tiene la clave. Sin este
	// orden se firmaría declarando el emisor de la raíz, y ningún validador
	// encontraría el certificado correcto.
	ordenarPorClave(cadena, clave)
	return cadena, clave, nil
}

func certificadoDeBolsa(b bolsa) *x509.Certificate {
	var interior contenidoInfo
	if _, err := asn1.Unmarshal(b.Valor.Bytes, &interior); err != nil {
		return nil
	}
	if !interior.Tipo.Equal(oidCertificadoX509) {
		return nil
	}
	var der []byte
	if _, err := asn1.Unmarshal(interior.Contenido.Bytes, &der); err != nil {
		return nil
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		return nil
	}
	return c
}

// ordenarPorClave pone delante el certificado cuya clave pública es la del
// firmante.
func ordenarPorClave(cadena []*x509.Certificate, clave crypto.Signer) {
	pub, ok := clave.Public().(interface{ Equal(crypto.PublicKey) bool })
	if !ok {
		return
	}
	for i, c := range cadena {
		if pub.Equal(c.PublicKey) {
			cadena[0], cadena[i] = cadena[i], cadena[0]
			return
		}
	}
}

// descifrarClaveEnvueltaPKCS8 abre el `EncryptedPrivateKeyInfo` de una bolsa.
func descifrarClaveEnvueltaPKCS8(der, pass []byte) ([]byte, error) {
	var env struct {
		Algoritmo pkix.AlgorithmIdentifier
		Datos     []byte
	}
	if _, err := asn1.Unmarshal(der, &env); err != nil {
		return nil, fmt.Errorf("%w: clave privada ilegible", ErrPKCS12)
	}
	return descifrarPKCS12(env.Algoritmo, env.Datos, pass)
}

// descifrarPKCS12 aplica el algoritmo que declare el fichero.
func descifrarPKCS12(alg pkix.AlgorithmIdentifier, datos, pass []byte) ([]byte, error) {
	switch {
	case alg.Algorithm.Equal(oidPBES2):
		return descifrarPBES2(alg.Parameters.FullBytes, datos, pass)
	case alg.Algorithm.Equal(oidPBEConSHA1y3DES):
		return descifrarPBE3DES(alg.Parameters.FullBytes, datos, pass)
	default:
		return nil, fmt.Errorf("%w: cifrado %v no soportado", ErrPKCS12, alg.Algorithm)
	}
}

// descifrarPBES2 es el camino moderno: PBKDF2 con HMAC-SHA-256 y AES-CBC.
func descifrarPBES2(paramsDER, datos, pass []byte) ([]byte, error) {
	var p parametrosPBES2
	if _, err := asn1.Unmarshal(paramsDER, &p); err != nil {
		return nil, fmt.Errorf("%w: parámetros PBES2 ilegibles", ErrPKCS12)
	}
	if !p.KDF.Algorithm.Equal(oidPBKDF2) {
		return nil, fmt.Errorf("%w: derivación de clave no soportada", ErrPKCS12)
	}
	var kdf parametrosPBKDF2
	if _, err := asn1.Unmarshal(p.KDF.Parameters.FullBytes, &kdf); err != nil {
		return nil, fmt.Errorf("%w: parámetros PBKDF2 ilegibles", ErrPKCS12)
	}

	var tam int
	switch {
	case p.Cifrado.Algorithm.Equal(oidAES128CBC):
		tam = 16
	case p.Cifrado.Algorithm.Equal(oidAES192CBC):
		tam = 24
	case p.Cifrado.Algorithm.Equal(oidAES256CBC):
		tam = 32
	default:
		return nil, fmt.Errorf("%w: cifrado simétrico no soportado", ErrPKCS12)
	}
	if kdf.Iteraciones <= 0 || kdf.Iteraciones > 10_000_000 {
		return nil, fmt.Errorf("%w: número de iteraciones absurdo", ErrPKCS12)
	}

	nueva := sha256.New
	if kdf.PRF.Algorithm != nil && !kdf.PRF.Algorithm.Equal(oidHMACSHA256) {
		// SHA-1 es el valor por DEFECTO de PBKDF2 cuando no se declara PRF, y hay
		// ficheros que lo declaran explícitamente.
		nueva = sha1.New //nolint:gosec // G401: lo fija el fichero
	}
	clave := pbkdf2(pass, kdf.Sal, kdf.Iteraciones, tam, nueva)

	var iv []byte
	if _, err := asn1.Unmarshal(p.Cifrado.Parameters.FullBytes, &iv); err != nil || len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("%w: vector de inicialización inválido", ErrPKCS12)
	}
	bloque, err := aes.NewCipher(clave)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPKCS12, err)
	}
	return descifrarCBCConRelleno(bloque, iv, datos)
}

// descifrarPBE3DES es el camino clásico de PKCS#12: derivación propia sobre
// SHA-1 y 3DES en CBC. Está aquí porque es lo que emiten las autoridades de
// certificación que llevan quince años emitiendo igual.
func descifrarPBE3DES(paramsDER, datos, pass []byte) ([]byte, error) {
	var p parametrosPBE
	if _, err := asn1.Unmarshal(paramsDER, &p); err != nil {
		return nil, fmt.Errorf("%w: parámetros PBE ilegibles", ErrPKCS12)
	}
	if p.Iteraciones <= 0 || p.Iteraciones > 10_000_000 {
		return nil, fmt.Errorf("%w: número de iteraciones absurdo", ErrPKCS12)
	}
	bmp := aBMPString(pass)
	clave := derivarPKCS12(bmp, p.Sal, p.Iteraciones, 1, 24)
	iv := derivarPKCS12(bmp, p.Sal, p.Iteraciones, 2, 8)

	bloque, err := des.NewTripleDESCipher(clave) //nolint:gosec // G405: lo fija el formato
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPKCS12, err)
	}
	return descifrarCBCConRelleno(bloque, iv, datos)
}

func descifrarCBCConRelleno(bloque cipher.Block, iv, datos []byte) ([]byte, error) {
	if len(datos) == 0 || len(datos)%bloque.BlockSize() != 0 {
		return nil, fmt.Errorf("%w: datos cifrados con longitud inválida", ErrPKCS12)
	}
	out := make([]byte, len(datos))
	cipher.NewCBCDecrypter(bloque, iv).CryptBlocks(out, datos)

	// El relleno se valida antes de recortar: con la contraseña equivocada sale
	// basura, y un último byte de 200 cortaría fuera del array.
	n := int(out[len(out)-1])
	if n < 1 || n > bloque.BlockSize() || n > len(out) {
		return nil, fmt.Errorf("%w: la contraseña no es correcta", ErrPKCS12)
	}
	for _, b := range out[len(out)-n:] {
		if int(b) != n {
			return nil, fmt.Errorf("%w: la contraseña no es correcta", ErrPKCS12)
		}
	}
	return out[:len(out)-n], nil
}

// aBMPString pasa la contraseña a UTF-16BE terminado en dos ceros, que es como
// PKCS#12 la mete en la derivación. Equivocarse aquí da «contraseña incorrecta»
// con la contraseña correcta, que es el fallo más desesperante posible.
func aBMPString(s []byte) []byte {
	out := make([]byte, 0, len(s)*2+2)
	for _, r := range string(s) {
		if r > 0xFFFF {
			r1, r2 := utf16Par(r)
			out = append(out, byte(r1>>8&0xFF), byte(r1&0xFF), byte(r2>>8&0xFF), byte(r2&0xFF))
			continue
		}
		out = append(out, byte(r>>8&0xFF), byte(r&0xFF))
	}
	return append(out, 0, 0)
}

// derivarPKCS12 es la derivación de clave del apéndice B de RFC 7292: no es
// PBKDF2, es una construcción propia sobre SHA-1 que solo existe para este
// formato. `id` vale 1 para la clave, 2 para el vector y 3 para el MAC.
func derivarPKCS12(bmpPass, sal []byte, iteraciones, id, tam int) []byte {
	const v = 64 // tamaño de bloque de SHA-1, en bytes
	const u = 20 // tamaño de su salida

	relleno := func(datos []byte) []byte {
		if len(datos) == 0 {
			return nil
		}
		n := v * ((len(datos) + v - 1) / v)
		out := make([]byte, n)
		for i := range out {
			out[i] = datos[i%len(datos)]
		}
		return out
	}

	d := make([]byte, v)
	for i := range d {
		d[i] = byte(id & 0xFF)
	}
	s := relleno(sal)
	p := relleno(bmpPass)
	i := append(append([]byte{}, s...), p...)

	out := make([]byte, 0, tam)
	for len(out) < tam {
		h := sha1.New() //nolint:gosec // G401: lo fija el formato
		h.Write(d)
		h.Write(i)
		a := h.Sum(nil)
		for j := 1; j < iteraciones; j++ {
			h.Reset()
			h.Write(a)
			a = h.Sum(nil)
		}
		out = append(out, a...)
		if len(out) >= tam {
			break
		}

		// Se incrementa el bloque I con B+1, que es lo que dice el apéndice B.
		b := make([]byte, v)
		for k := range b {
			b[k] = a[k%u]
		}
		for k := 0; k < len(i); k += v {
			acarreo := 1
			for j := v - 1; j >= 0; j-- {
				suma := int(i[k+j]) + int(b[j]) + acarreo
				i[k+j] = byte(suma & 0xFF)
				acarreo = suma >> 8
			}
		}
	}
	return out[:tam]
}

// pbkdf2 es la derivación de PKCS#5, que la biblioteca estándar no expone.
func pbkdf2(pass, sal []byte, iteraciones, tam int, nueva func() hash.Hash) []byte {
	prf := hmac.New(nueva, pass)
	tamHash := prf.Size()
	bloques := (tam + tamHash - 1) / tamHash
	out := make([]byte, 0, bloques*tamHash)

	buf := make([]byte, 4)
	for b := 1; b <= bloques; b++ {
		prf.Reset()
		prf.Write(sal)
		buf[0], buf[1], buf[2], buf[3] = byte(b>>24), byte(b>>16), byte(b>>8), byte(b)
		prf.Write(buf)
		u := prf.Sum(nil)
		t := append([]byte(nil), u...)
		for i := 1; i < iteraciones; i++ {
			prf.Reset()
			prf.Write(u)
			u = prf.Sum(nil)
			for j := range t {
				t[j] ^= u[j]
			}
		}
		out = append(out, t...)
	}
	return out[:tam]
}
