// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package smime da a Sytue Mail firma y cifrado S/MIME (RFC 8551) SIN coste y
// SIN que nadie tenga que ir a comprar un certificado.
//
// # Por qué hay una autoridad propia
//
// S/MIME lleva treinta años existiendo y casi nadie lo usa por una razón
// económica, no técnica: un certificado de correo lo emite una CA pública, se
// paga por buzón y por año, y hay que renovarlo a mano. A día de hoy NO existe
// ninguna CA pública que emita certificados S/MIME de dominio gratis y de forma
// automatizada —el estándar para hacerlo (ACME para S/MIME, RFC 8823) está
// publicado, pero apenas hay emisores—.
//
// Así que el valor por defecto es el que sí se puede dar gratis y sin fricción:
// una autoridad POR ORGANIZACIÓN dentro de su propio despliegue, que emite un
// certificado a cada buzón en cuanto envía su primer correo. Con eso:
//
//	– Todo el correo sale FIRMADO, y la firma es verificable por cualquiera con
//	  el certificado (que viaja en el propio mensaje).
//	– Dentro de la organización la confianza es REAL: su ancla se distribuye una
//	  vez a los dispositivos (MDM, o el fichero que publica este paquete) y los
//	  clientes muestran el correo como firmado y de confianza.
//	– Fuera, un cliente que no tenga el ancla verá «firma de un emisor
//	  desconocido». Eso es lo que hay, y se dice: no se disfraza de confianza
//	  pública lo que no lo es.
//
// # Cómo se consigue que valide FUERA, sin instalar nada
//
// Instalar el ancla en el dispositivo de quien recibe el correo no es una
// opción: quien lo recibe es justo el que no va a instalar nada. Lo único que
// hace que la firma se valide sola es un certificado emitido por una CA que ya
// está de fábrica en el sistema de destino. No hay atajo: ni publicar el ancla,
// ni una CRL, ni SMIMEA en el DNS cambian eso.
//
// Por eso [ImportIdentity] existe: se trae el certificado de una CA pública con
// su cadena y su clave, y a partir de ahí se firma con él. Todo lo demás —el
// almacén, la KEK, el camino de firma, la fusión de organizaciones— es idéntico,
// y [CA.Emitio] evita que la renovación automática de aquí lo pise.
//
// Cuando haya un emisor que automatice la emisión, lo que enchufa es ESE punto:
// emitir y llamar a Import.
//
// ACME PARA S/MIME (RFC 8823) NO ES ESE EMISOR TODAVÍA, y esto está MEDIDO y no
// supuesto. El 2026-09-11 se probó contra el endpoint real de Actalis
// (`acme-api.actalis.com`) con credenciales EAB válidas:
//
//	identificador `email`  → 400 rejectedIdentifier: invalid domain names
//	identificador `dns`    → aceptado
//
// El mismo endpoint acepta el dominio y rechaza el buzón, así que lo que falta
// es el identificador de correo y no el permiso: ese ACME es el de TLS. El
// control con `dns` es lo que lo hace concluyente —sin él, «invalid domain
// names» también podría ser una cuenta mal configurada—.
//
// El estándar está en los requisitos del CA/Browser Forum desde la papeleta
// SMC012 (mayo de 2025) y hay clientes libres que hablan `email-reply-00`, pero
// ninguna CA pública lo sirve. Mientras tanto, la vía son las APIs de emisión
// propietarias de cada CA.
package smime

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

const (
	// caValidity es lo que vive la autoridad de una organización. Diez años es lo
	// habitual en una CA interna: renovarla obliga a redistribuir el ancla.
	caValidity = 10 * 365 * 24 * time.Hour
	// certValidity es lo que vive el certificado de un buzón. Un año, como el de
	// cualquier CA pública, y la renovación es automática (no la hace nadie a mano).
	certValidity = 365 * 24 * time.Hour
	// renewBefore es cuánto antes del vencimiento se renueva sola.
	renewBefore = 30 * 24 * time.Hour
)

// CA es la autoridad de certificación de UNA organización.
type CA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	der  []byte
}

// NewCA crea la autoridad de una organización. El nombre va en el emisor, que es
// lo que ve quien recibe un correo firmado.
//
// P-256 y no RSA: las claves son 8 veces más pequeñas, la firma es más rápida y
// todos los clientes de correo con soporte S/MIME moderno la entienden. Un
// mensaje firmado lleva el certificado dentro, así que el tamaño importa.
func NewCA(org string) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("smime: generar clave de la autoridad: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Sytue Mail S/MIME — " + org,
			Organization: []string{org},
		},
		NotBefore:             now.Add(-time.Hour), // margen de reloj entre máquinas
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("smime: crear la autoridad: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("smime: releer la autoridad: %w", err)
	}
	return &CA{cert: cert, key: key, der: der}, nil
}

// LoadCA reconstruye la autoridad desde su material PEM (como se guarda en el
// vault de la organización).
func LoadCA(certPEM, keyPEM string) (*CA, error) {
	certBlock, _ := pem.Decode([]byte(certPEM))
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if certBlock == nil || keyBlock == nil {
		return nil, fmt.Errorf("smime: el material de la autoridad no es PEM válido")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("smime: certificado de la autoridad ilegible: %w", err)
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("smime: clave de la autoridad ilegible: %w", err)
	}
	return &CA{cert: cert, key: key, der: certBlock.Bytes}, nil
}

// PEM devuelve el material de la autoridad para guardarlo (la clave privada va
// al vault sellada con la KEK; el certificado es público).
func (ca *CA) PEM() (certPEM, keyPEM string, err error) {
	keyDER, err := x509.MarshalECPrivateKey(ca.key)
	if err != nil {
		return "", "", fmt.Errorf("smime: serializar la clave de la autoridad: %w", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})), nil
}

// Certificate es el certificado de la autoridad (el ancla que se distribuye).
func (ca *CA) Certificate() *x509.Certificate { return ca.cert }

// Identity es el material S/MIME de UN buzón.
//
// Key es un crypto.Signer y no una clave concreta porque el certificado no
// siempre lo emite la autoridad de aquí: el de una CA pública casi siempre es
// RSA, y con el tipo cerrado no había forma de usarlo.
//
// Chain son los certificados INTERMEDIOS entre Certificate y la raíz. Con la
// autoridad propia está vacía —ella misma es el emisor—, y con una CA pública
// lleva su intermedia. Sin incrustarla, quien recibe el correo no puede
// construir la cadena y ve «no fiable» aunque tenga la raíz de fábrica.
type Identity struct {
	Address     string
	Certificate *x509.Certificate
	CertDER     []byte
	Chain       []*x509.Certificate
	Key         crypto.Signer
	NotAfter    time.Time
}

// Issue emite el certificado de un buzón. El correo va en el SAN rfc822Name,
// que es lo que un cliente compara con el remitente: sin eso, la firma es
// válida pero «no coincide con el remitente», que es peor que no firmar.
func (ca *CA) Issue(address, displayName string) (*Identity, error) {
	addr := strings.TrimSpace(strings.ToLower(address))
	if addr == "" || !strings.Contains(addr, "@") {
		return nil, fmt.Errorf("smime: dirección de correo inválida para emitir: %q", address)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("smime: generar clave del buzón: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	cn := displayName
	if strings.TrimSpace(cn) == "" {
		cn = addr
	}
	now := time.Now().UTC()
	tmpl := &x509.Certificate{
		SerialNumber:   serial,
		Subject:        pkix.Name{CommonName: cn},
		EmailAddresses: []string{addr},
		NotBefore:      now.Add(-time.Hour),
		NotAfter:       now.Add(certValidity),
		// Firmar y cifrar: las dos cosas que hace S/MIME.
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageKeyAgreement,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, fmt.Errorf("smime: emitir el certificado del buzón: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("smime: releer el certificado del buzón: %w", err)
	}
	return &Identity{Address: addr, Certificate: cert, CertDER: der, Key: key, NotAfter: cert.NotAfter}, nil
}

// NeedsRenewal dice si al certificado le queda poco. La renovación es automática
// justamente porque «renovar a mano cada año» es lo que hizo que S/MIME no se
// usara nunca.
func (id *Identity) NeedsRenewal(now time.Time) bool {
	return id == nil || now.Add(renewBefore).After(id.NotAfter)
}

// PEM serializa la identidad de un buzón para guardarla.
//
// El certificado sale con su cadena detrás, en un solo PEM y en orden (hoja
// primero), que es el formato en el que la entregan las CAs y el que vuelve a
// leer LoadIdentity. Así la cadena se guarda sin columna nueva.
//
// La clave va en PKCS#8, que vale para cualquier algoritmo. Lo anterior era
// SEC1 («EC PRIVATE KEY»), que solo vale para curvas elípticas; lo ya guardado
// se sigue leyendo.
func (id *Identity) PEM() (certPEM, keyPEM string, err error) {
	keyDER, err := x509.MarshalPKCS8PrivateKey(id.Key)
	if err != nil {
		return "", "", fmt.Errorf("smime: serializar la clave del buzón: %w", err)
	}
	var certs strings.Builder
	certs.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: id.CertDER}))
	for _, c := range id.Chain {
		certs.Write(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}))
	}
	return certs.String(),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})), nil
}

// LoadIdentity reconstruye la identidad de un buzón desde su PEM. El primer
// bloque CERTIFICATE es el del buzón y los siguientes, si los hay, su cadena.
func LoadIdentity(address, certPEM, keyPEM string) (*Identity, error) {
	certs, ders, err := parseCertificates(certPEM)
	if err != nil {
		return nil, err
	}
	keyBlock, _ := pem.Decode([]byte(keyPEM))
	if keyBlock == nil {
		return nil, invalido("el material del buzón no trae la clave privada")
	}
	if keyBlock.Headers["DEK-Info"] != "" {
		return nil, invalido("la clave privada está protegida con contraseña; " +
			"conviértela antes con `openssl pkcs12 -in fichero.p12 -nodes -out material.pem`")
	}
	key, err := parsePrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &Identity{
		Address: address, Certificate: certs[0], CertDER: ders[0], Chain: certs[1:],
		Key: key, NotAfter: certs[0].NotAfter,
	}, nil
}

// ImportIdentity construye la identidad de un buzón con material emitido FUERA:
// el certificado de una CA pública, su cadena y la clave privada.
//
// Es lo único que hace que un correo se valide SOLO en el móvil de quien lo
// recibe: la autoridad de aquí no está en el almacén de confianza de nadie y
// una pública sí. Todo lo demás —firmar, guardar, usar— es idéntico.
//
// Se comprueba aquí lo que si no falla en casa del destinatario, donde nadie
// puede depurarlo: que la clave sea la del certificado, que el buzón esté en el
// SAN, que el certificado sirva para correo y que esté vigente.
func ImportIdentity(address, certPEM, keyPEM string) (*Identity, error) {
	id, err := LoadIdentity(address, certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	pub, ok := id.Certificate.PublicKey.(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !pub.Equal(id.Key.Public()) {
		return nil, invalido("la clave privada no es la de este certificado")
	}
	addr := strings.TrimSpace(strings.ToLower(address))
	// Algunos PKCS#12 sacan la intermedia delante de la hoja. Se busca cuál es la
	// del buzón en vez de fallar con «el certificado no es de esta dirección», que
	// manda a depurar algo que no está roto.
	if !esDelBuzon(id.Certificate, addr) {
		for i, c := range id.Chain {
			if esDelBuzon(c, addr) {
				id.Chain[i], id.Certificate, id.CertDER, id.NotAfter = id.Certificate, c, c.Raw, c.NotAfter
				break
			}
		}
	}
	if !esDelBuzon(id.Certificate, addr) {
		return nil, invalido("el certificado no es de %s (lleva %v)", addr, id.Certificate.EmailAddresses)
	}
	if !slices.Contains(id.Certificate.ExtKeyUsage, x509.ExtKeyUsageEmailProtection) &&
		!slices.Contains(id.Certificate.ExtKeyUsage, x509.ExtKeyUsageAny) {
		return nil, invalido("el certificado no sirve para correo: le falta el uso emailProtection")
	}
	now := time.Now()
	if now.Before(id.Certificate.NotBefore) || now.After(id.Certificate.NotAfter) {
		return nil, invalido("el certificado no está vigente (del %s al %s)",
			id.Certificate.NotBefore.Format(time.DateOnly), id.Certificate.NotAfter.Format(time.DateOnly))
	}
	id.Address = addr
	return id, nil
}

// ErrMaterialInvalido marca el material que no sirve: mal formado, protegido con
// contraseña, caducado o de otro buzón. Es culpa de quien lo manda y no una
// avería del servicio, así que quien lo expone por HTTP contesta 400 y no 500.
//
// El texto es «smime» a secas porque siempre viaja delante de la explicación:
// lo que se lee es «smime: la clave privada no es la de este certificado».
var ErrMaterialInvalido = errors.New("smime")

// invalido compone un error de material con el centinela delante.
func invalido(formato string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMaterialInvalido, fmt.Sprintf(formato, args...))
}

// esDelBuzon dice si el certificado lleva esa dirección en el SAN rfc822Name,
// que es lo que un cliente compara con el remitente.
func esDelBuzon(cert *x509.Certificate, addr string) bool {
	return slices.ContainsFunc(cert.EmailAddresses, func(e string) bool {
		return strings.EqualFold(strings.TrimSpace(e), addr)
	})
}

// SplitBundle separa un PEM que trae de todo —certificado, cadena y clave
// privada, en cualquier orden— en las dos partes que pide ImportIdentity.
//
// Es el formato que sale de `openssl pkcs12 -in fichero.p12 -nodes`, que es como
// se convierte lo que entregan casi todas las CAs. Pedir dos ficheros separados
// sería pedir un paso más a quien ya ha dado tres.
func SplitBundle(bundlePEM string) (certPEM, keyPEM string) {
	var certs, keys strings.Builder
	rest := []byte(bundlePEM)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		switch {
		case block.Type == "CERTIFICATE":
			certs.Write(pem.EncodeToMemory(block))
		case strings.HasSuffix(block.Type, "PRIVATE KEY") && keys.Len() == 0:
			// La PRIMERA clave: un material con dos es un error de quien lo armó, y
			// quedarse con la última en silencio sería peor que firmar con la que
			// corresponde al certificado o fallar diciéndolo.
			keys.Write(pem.EncodeToMemory(block))
		}
	}
	return certs.String(), keys.String()
}

// parseCertificates lee todos los bloques CERTIFICATE de un PEM, en orden.
func parseCertificates(certPEM string) ([]*x509.Certificate, [][]byte, error) {
	var certs []*x509.Certificate
	var ders [][]byte
	rest := []byte(certPEM)
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, invalido("certificado ilegible: %v", err)
		}
		certs = append(certs, cert)
		ders = append(ders, block.Bytes)
	}
	if len(certs) == 0 {
		return nil, nil, invalido("el material del buzón no trae ningún certificado")
	}
	return certs, ders, nil
}

// parsePrivateKey lee una clave privada en los tres formatos que uno se
// encuentra: PKCS#8 (lo que escribe PEM), SEC1 (lo que escribía antes) y PKCS#1
// (lo que entrega media internet para RSA).
func parsePrivateKey(der []byte) (crypto.Signer, error) {
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		signer, ok := k.(crypto.Signer)
		if !ok {
			return nil, invalido("esa clave privada no sirve para firmar (%T)", k)
		}
		return signer, nil
	}
	if k, err := x509.ParseECPrivateKey(der); err == nil {
		return k, nil
	}
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	return nil, invalido("clave del buzón ilegible")
}

// Emitio dice si un certificado lo firmó ESTA autoridad. Sirve para no pisar con
// una renovación propia un certificado que vino de fuera.
func (ca *CA) Emitio(cert *x509.Certificate) bool {
	return cert != nil && cert.CheckSignatureFrom(ca.cert) == nil
}

// randomSerial genera el número de serie (positivo, 128 bits) que exige X.509.
func randomSerial() (*big.Int, error) {
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("smime: generar número de serie: %w", err)
	}
	return serial.Add(serial, big.NewInt(1)), nil
}
