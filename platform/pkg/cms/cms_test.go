// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package cms_test

// La prueba de que una firma sirve no es que la verifique quien la hizo: es que la
// acepte OTRO, y que nosotros aceptemos la suya. Estos tests cruzan las dos
// direcciones contra `openssl cms`, que es una implementación independiente y la
// que hay debajo de casi todo lo que valida firmas en el mundo real.

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cms"
)

const contenido = "Presupuesto 2026: 14.500 € IVA incluido.\n"

// --- Andamiaje ---------------------------------------------------------------

func nuevaCA(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("clave de la CA: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Autoridad de pruebas"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	return firmaCert(t, tmpl, tmpl, key.Public(), key), key
}

func emite(t *testing.T, caCert *x509.Certificate, caKey crypto.Signer, pub crypto.PublicKey, cn string) *x509.Certificate {
	t.Helper()
	serie, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		t.Fatalf("número de serie: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: serie,
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(12 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
	}
	return firmaCert(t, tmpl, caCert, pub, caKey)
}

func firmaCert(t *testing.T, tmpl, padre *x509.Certificate, pub crypto.PublicKey, firmante crypto.Signer) *x509.Certificate {
	t.Helper()
	der, err := x509.CreateCertificate(rand.Reader, tmpl, padre, pub, firmante)
	if err != nil {
		t.Fatalf("crear certificado: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("releer certificado: %v", err)
	}
	return cert
}

// identidad devuelve una CA, un certificado de firmante y su clave.
func identidad(t *testing.T, rsaKey bool) (ca, cert *x509.Certificate, key crypto.Signer) {
	t.Helper()
	caCert, caKey := nuevaCA(t)
	if rsaKey {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("clave RSA: %v", err)
		}
		key = k
	} else {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("clave ECDSA: %v", err)
		}
		key = k
	}
	return caCert, emite(t, caCert, caKey, key.Public(), "Ana Ruiz"), key
}

func anclas(certs ...*x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	for _, c := range certs {
		pool.AddCert(c)
	}
	return pool
}

func escribe(t *testing.T, ruta string, datos []byte) string {
	t.Helper()
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatalf("escribir %s: %v", ruta, err)
	}
	return ruta
}

func certPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
}

func keyPEM(t *testing.T, key crypto.Signer) []byte {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("serializar clave: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

func openssl(t *testing.T) string {
	t.Helper()
	ruta, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl en el entorno: este test necesita un verificador independiente")
	}
	return ruta
}

// --- Ida: lo que firmamos lo acepta openssl ----------------------------------

func TestOpenSSLAceptaNuestraFirma(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		nombre string
		rsa    bool
	}{{"ECDSA P-256", false}, {"RSA 2048", true}} {
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			bin := openssl(t)
			ca, cert, key := identidad(t, tc.rsa)

			firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}

			dir := t.TempDir()
			sigPath := escribe(t, filepath.Join(dir, "firma.der"), firma)
			docPath := escribe(t, filepath.Join(dir, "doc.txt"), []byte(contenido))
			caPath := escribe(t, filepath.Join(dir, "ca.pem"), certPEM(ca))

			out, err := exec.Command(bin, "cms", "-verify", "-inform", "DER", "-in", sigPath,
				"-content", docPath, "-binary", "-CAfile", caPath, "-purpose", "any",
				"-out", os.DevNull).CombinedOutput()
			if err != nil {
				t.Fatalf("openssl rechazó nuestra firma: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "Verification successful") {
				t.Errorf("openssl no confirmó la verificación:\n%s", out)
			}
		})
	}
}

// La firma es DESPRENDIDA: si arrastrase el documento, publicar la firma de un
// contrato publicaría el contrato.
func TestLaFirmaNoLlevaElDocumentoDentro(t *testing.T) {
	t.Parallel()
	_, cert, key := identidad(t, false)
	firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if strings.Contains(string(firma), "Presupuesto 2026") {
		t.Error("la firma desprendida lleva el contenido dentro")
	}
}

// --- Vuelta: lo que firma openssl lo verificamos nosotros --------------------

// firmaConOpenSSL devuelve un CMS DER desprendido hecho por openssl.
func firmaConOpenSSL(t *testing.T, bin string, cert *x509.Certificate, key crypto.Signer, doc []byte, md string) []byte {
	t.Helper()
	dir := t.TempDir()
	docPath := escribe(t, filepath.Join(dir, "doc.txt"), doc)
	certPath := escribe(t, filepath.Join(dir, "cert.pem"), certPEM(cert))
	keyPath := escribe(t, filepath.Join(dir, "key.pem"), keyPEM(t, key))
	sigPath := filepath.Join(dir, "firma.der")

	out, err := exec.Command(bin, "cms", "-sign", "-binary", "-in", docPath,
		"-signer", certPath, "-inkey", keyPath, "-md", md, "-nosmimecap",
		"-outform", "DER", "-out", sigPath).CombinedOutput()
	if err != nil {
		t.Fatalf("openssl no pudo firmar: %v\n%s", err, out)
	}
	der, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("leer la firma de openssl: %v", err)
	}
	return der
}

func TestVerificamosLaFirmaDeOpenSSL(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		nombre string
		rsa    bool
	}{{"ECDSA P-256", false}, {"RSA 2048", true}} {
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			bin := openssl(t)
			ca, cert, key := identidad(t, tc.rsa)
			der := firmaConOpenSSL(t, bin, cert, key, []byte(contenido), "sha256")

			sig, err := cms.Verify(der, []byte(contenido), x509.VerifyOptions{Roots: anclas(ca)})
			if err != nil {
				t.Fatalf("no verificamos una firma válida de openssl: %v", err)
			}
			if sig.Signer.Subject.CommonName != "Ana Ruiz" {
				t.Errorf("firmante inesperado: %q", sig.Signer.Subject.CommonName)
			}
			if len(sig.Chains) == 0 {
				t.Error("no se construyó ninguna cadena hasta el ancla")
			}
			if sig.SigningTime.IsZero() {
				t.Error("la firma de openssl trae signing-time y no se leyó")
			}
		})
	}
}

// SHA-1 sigue siendo aceptado por x509.CheckSignature y por openssl. Aquí no.
func TestSHA1SeRechaza(t *testing.T) {
	t.Parallel()
	bin := openssl(t)
	ca, cert, key := identidad(t, true)
	der := firmaConOpenSSL(t, bin, cert, key, []byte(contenido), "sha1")

	if _, err := cms.Verify(der, []byte(contenido), x509.VerifyOptions{Roots: anclas(ca)}); err == nil {
		t.Fatal("se aceptó una firma con SHA-1")
	}
}

// --- Lo que tiene que fallar -------------------------------------------------

func TestVerificacionRechaza(t *testing.T) {
	t.Parallel()
	ca, cert, key := identidad(t, false)
	otraCA, _ := nuevaCA(t)
	firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	manipulada := append([]byte(nil), firma...)
	manipulada[len(manipulada)-1] ^= 0x01 // el último byte cae dentro de la firma

	for _, tc := range []struct {
		nombre    string
		der       []byte
		contenido string
		opts      x509.VerifyOptions
	}{
		{"contenido cambiado", firma, "Presupuesto 2026: 1.500 € IVA incluido.\n", x509.VerifyOptions{Roots: anclas(ca)}},
		{"firma manipulada", manipulada, contenido, x509.VerifyOptions{Roots: anclas(ca)}},
		{"ancla que no emitió el certificado", firma, contenido, x509.VerifyOptions{Roots: anclas(otraCA)}},
		{"sin contenido", firma, "", x509.VerifyOptions{Roots: anclas(ca)}},
		{"no es un CMS", []byte("esto no es DER"), contenido, x509.VerifyOptions{Roots: anclas(ca)}},
	} {
		t.Run(tc.nombre, func(t *testing.T) {
			t.Parallel()
			if _, err := cms.Verify(tc.der, []byte(tc.contenido), tc.opts); err == nil {
				t.Fatalf("pasó la verificación algo que no debía: %s", tc.nombre)
			}
		})
	}
}

// Sin anclas, Verify tiene que NEGARSE, no caer en el almacén del sistema. Se
// comprueba el motivo y no solo que falle: si cayera al almacén del sistema
// también fallaría, y con un error que no dice nada de la decisión que se tomó
// sola.
func TestSinAnclasSeNiegaYLoDice(t *testing.T) {
	t.Parallel()
	ca, cert, key := identidad(t, false)
	firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := cms.Verify(firma, []byte(contenido), x509.VerifyOptions{}); !errors.Is(err, cms.ErrSinAnclas) {
		t.Fatalf("sin anclas se esperaba ErrSinAnclas y se obtuvo: %v", err)
	}
	if _, err := cms.Verify(firma, []byte(contenido), x509.VerifyOptions{Roots: anclas(ca)}); err != nil {
		t.Fatalf("con anclas debería verificar: %v", err)
	}
}

// Una firma hecha con una clave que no es la del certificado solo falla en casa
// del destinatario. Tiene que fallar aquí.
func TestFirmarConClaveAjena(t *testing.T) {
	t.Parallel()
	_, cert, _ := identidad(t, false)
	otra, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("clave: %v", err)
	}
	if _, err := cms.Sign(otra, []*x509.Certificate{cert}, []byte(contenido)); err == nil {
		t.Fatal("se firmó con una clave que no es la del certificado")
	}
}

func TestFirmarExigeSusPiezas(t *testing.T) {
	t.Parallel()
	_, cert, key := identidad(t, false)
	if _, err := cms.Sign(nil, []*x509.Certificate{cert}, []byte(contenido)); err == nil {
		t.Error("se firmó sin firmante")
	}
	if _, err := cms.Sign(key, nil, []byte(contenido)); err == nil {
		t.Error("se firmó sin certificado")
	}
	if _, err := cms.Sign(key, []*x509.Certificate{cert}, nil); err == nil {
		t.Error("se firmó un contenido vacío")
	}
}

// La cadena tiene que poder construirse con los intermedios que viajan DENTRO de
// la firma: quien verifica no tiene por qué tenerlos a mano.
func TestCadenaConIntermedioIncrustado(t *testing.T) {
	t.Parallel()
	raiz, raizKey := nuevaCA(t)

	interKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("clave intermedia: %v", err)
	}
	inter := firmaCert(t, &x509.Certificate{
		SerialNumber:          big.NewInt(7),
		Subject:               pkix.Name{CommonName: "Autoridad intermedia"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(20 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}, raiz, interKey.Public(), raizKey)

	hojaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("clave de la hoja: %v", err)
	}
	hoja := emite(t, inter, interKey, hojaKey.Public(), "Ana Ruiz")

	firma, err := cms.Sign(hojaKey, []*x509.Certificate{hoja, inter}, []byte(contenido))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sig, err := cms.Verify(firma, []byte(contenido), x509.VerifyOptions{Roots: anclas(raiz)})
	if err != nil {
		t.Fatalf("no se pudo verificar con el intermedio incrustado: %v", err)
	}
	if len(sig.Chains) == 0 || len(sig.Chains[0]) != 3 {
		t.Errorf("cadena inesperada: %v", sig.Chains)
	}
}

// --- Sellos de tiempo: el contenido va DENTRO --------------------------------

// selloDeOpenSSL emite un sello RFC 3161 de verdad con `openssl ts -reply`, con una
// TSA de prueba de esta CA. Devuelve el token (el SignedData).
func selloDeOpenSSL(t *testing.T, bin string, ca *x509.Certificate, caKey crypto.Signer) []byte {
	t.Helper()
	dir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// El uso timeStamping tiene que ser CRÍTICO y el único: sin eso OpenSSL se
	// niega a firmar sellos, y un validador serio a aceptarlos.
	eku, err := asn1.Marshal([]asn1.ObjectIdentifier{asn1ObjectIdentifier(1, 3, 6, 1, 5, 5, 7, 3, 8)})
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99), Subject: pkix.Name{CommonName: "TSA de pruebas"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(12 * time.Hour),
		KeyUsage:        x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{{Id: asn1ObjectIdentifier(2, 5, 29, 37), Critical: true, Value: eku}},
	}
	tsa := firmaCert(t, tmpl, ca, key.Public(), caKey)
	cnf := "[ tsa ]\ndefault_tsa = t\n[ t ]\nserial = " + escribe(t, filepath.Join(dir, "serie"), []byte("01\n")) +
		"\nsigner_cert = " + escribe(t, filepath.Join(dir, "tsa.pem"), certPEM(tsa)) +
		"\nsigner_key = " + escribe(t, filepath.Join(dir, "tsa.key"), keyPEM(t, key)) +
		"\nsigner_digest = sha256\ndefault_policy = 1.2.3.4.1\ndigests = sha256\ness_cert_id_alg = sha256\n"
	q := filepath.Join(dir, "q.tsq")
	if out, err := exec.Command(bin, "ts", "-query", "-data", escribe(t, filepath.Join(dir, "dato"), []byte(contenido)),
		"-sha256", "-cert", "-out", q).CombinedOutput(); err != nil {
		t.Fatalf("openssl ts -query: %v\n%s", err, out)
	}
	tok := filepath.Join(dir, "tok.der")
	if out, err := exec.Command(bin, "ts", "-reply", "-config", escribe(t, filepath.Join(dir, "tsa.cnf"), []byte(cnf)),
		"-queryfile", q, "-token_out", "-out", tok).CombinedOutput(); err != nil {
		t.Fatalf("openssl ts -reply: %v\n%s", err, out)
	}
	der, err := os.ReadFile(tok)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// TestVerifyEncapsulatedAceptaUnSelloYRechazaLoDemas es el check de lo que impide
// que un intermediario cuele un sello FALSO: la autoridad habla HTTP plano, así que
// sin comprobar la firma del token cualquiera podría fabricar uno con la huella y
// el nonce correctos.
func TestVerifyEncapsulatedAceptaUnSelloYRechazaLoDemas(t *testing.T) {
	t.Parallel()
	bin := openssl(t)
	ca, caKey := nuevaCA(t)
	token := selloDeOpenSSL(t, bin, ca, caKey)
	tstInfo := asn1ObjectIdentifier(1, 2, 840, 113549, 1, 9, 16, 1, 4)
	sellado := x509.VerifyOptions{Roots: anclas(ca), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping}}

	sig, dentro, err := cms.VerifyEncapsulated(token, tstInfo, sellado)
	if err != nil {
		t.Fatalf("un sello de OpenSSL no verifica: %v", err)
	}
	if sig.Signer.Subject.CommonName != "TSA de pruebas" || len(dentro) == 0 {
		t.Errorf("firmante %q, %d bytes de TSTInfo", sig.Signer.Subject.CommonName, len(dentro))
	}

	otraCA, _ := nuevaCA(t)
	if _, _, err := cms.VerifyEncapsulated(token, tstInfo, x509.VerifyOptions{Roots: anclas(otraCA)}); err == nil {
		t.Error("un sello de otra autoridad no puede pasar por el de la nuestra")
	}
	if _, _, err := cms.VerifyEncapsulated(token, asn1ObjectIdentifier(1, 2, 840, 113549, 1, 7, 1), sellado); err == nil {
		t.Error("un sello no es un id-data: el tipo esperado tiene que contar")
	}
	if _, _, err := cms.VerifyEncapsulated(token, tstInfo, x509.VerifyOptions{}); !errors.Is(err, cms.ErrSinAnclas) {
		t.Errorf("sin anclas: err = %v", err)
	}
	// Tocar un byte del TSTInfo —la hora, la huella— rompe la firma de la autoridad.
	i := bytes.Index(token, dentro)
	if i < 0 {
		t.Fatal("no se encuentra el TSTInfo dentro del token")
	}
	tocado := append([]byte(nil), token...)
	tocado[i+len(dentro)-1] ^= 1
	if _, _, err := cms.VerifyEncapsulated(tocado, tstInfo, sellado); err == nil {
		t.Error("un sello con el TSTInfo tocado sigue verificando")
	}
}

func asn1ObjectIdentifier(arcos ...int) asn1.ObjectIdentifier { return arcos }

// TestInspectSeparaLaIntegridadDeLaConfianza es el check de por qué existe Inspect:
// una firma de una autoridad que nadie conoce sigue diciendo si el contenido es el
// que se firmó, y eso tiene que poder contarse sin fingir que se confía en ella.
func TestInspectSeparaLaIntegridadDeLaConfianza(t *testing.T) {
	t.Parallel()
	_, cert, key := identidad(t, false)
	firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	sig, err := cms.Inspect(firma, []byte(contenido))
	if err != nil {
		t.Fatalf("Inspect rechaza una firma íntegra de una CA desconocida: %v", err)
	}
	if !sig.Signer.Equal(cert) || len(sig.Chains) != 0 || len(sig.Certificates) != 1 || len(sig.Value) == 0 {
		t.Errorf("firmante %v, %d cadenas, %d certificados, %d bytes de firma",
			sig.Signer.Subject, len(sig.Chains), len(sig.Certificates), len(sig.Value))
	}
	if sig.TimestampToken != nil {
		t.Error("una firma sin sello no puede traer token")
	}
	// La misma firma sobre otro contenido: la integridad sí se decide aquí.
	if _, err := cms.Inspect(firma, []byte(contenido+" ")); !errors.Is(err, cms.ErrNoCorresponde) {
		t.Errorf("otro contenido tiene que dar ErrNoCorresponde, dio: %v", err)
	}
	// Y una firma ilegible es OTRA cosa: no se sabe comprobar, no «alguien la cambió».
	if _, err := cms.Inspect(firma[:len(firma)/2], []byte(contenido)); err == nil || errors.Is(err, cms.ErrNoCorresponde) {
		t.Errorf("una firma cortada no es una firma que no corresponde: %v", err)
	}
	// Y Verify sigue exigiendo anclas: Inspect no abre una puerta trasera.
	if _, err := cms.Verify(firma, []byte(contenido), x509.VerifyOptions{}); !errors.Is(err, cms.ErrSinAnclas) {
		t.Errorf("Verify sin anclas: err = %v", err)
	}
}

// Entrada no confiable: una firma con megabytes de certificados incrustados es un
// ataque de coste (cada uno se parsea y entra al pool de intermedios), no una firma.
func TestInspectAcotaLosCertificadosIncrustados(t *testing.T) {
	t.Parallel()
	ca, cert, key := identidad(t, false)
	certs := []*x509.Certificate{cert}
	for len(certs) < 4000 { // ~0,4 KiB cada uno: pasa de 1 MiB
		certs = append(certs, ca)
	}
	firma, err := cms.Sign(key, certs, []byte(contenido))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := cms.Inspect(firma, []byte(contenido)); err == nil || !strings.Contains(err.Error(), "certificados incrustados") {
		t.Errorf("una firma con 4000 certificados tiene que rechazarse por tamaño: %v", err)
	}
	// Y la normal sigue pasando: el tope no es el de las firmas reales.
	normal, _ := cms.Sign(key, []*x509.Certificate{cert, ca}, []byte(contenido))
	if _, err := cms.Inspect(normal, []byte(contenido)); err != nil {
		t.Errorf("una firma con cadena de dos certificados: %v", err)
	}
}

// Decisión del 2026-10-16: una firma CMS sin atributos firmados (la firma va
// directamente sobre el contenido) se ACEPTA y se verifica como cualquier otra;
// quien la enseñe dice que no los lleva. Antes se rechazaba como ilegible.
func TestFirmaSinAtributosFirmados(t *testing.T) {
	t.Parallel()
	bin := openssl(t)
	ca, cert, key := identidad(t, true)
	dir := t.TempDir()
	doc := escribe(t, filepath.Join(dir, "doc.txt"), []byte(contenido))
	certPath := escribe(t, filepath.Join(dir, "cert.pem"), certPEM(cert))
	keyPath := escribe(t, filepath.Join(dir, "key.pem"), keyPEM(t, key))
	sigPath := filepath.Join(dir, "firma.der")
	if out, err := exec.Command(bin, "cms", "-sign", "-binary", "-noattr", "-in", doc, "-signer", certPath,
		"-inkey", keyPath, "-md", "sha256", "-outform", "DER", "-out", sigPath).CombinedOutput(); err != nil {
		t.Fatalf("openssl: %v\n%s", err, out)
	}
	der, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := cms.Verify(der, []byte(contenido), x509.VerifyOptions{Roots: anclas(ca)})
	if err != nil {
		t.Fatalf("una firma válida sin atributos firmados se rechaza: %v", err)
	}
	if !sig.SinAtributosFirmados || !sig.SigningTime.IsZero() {
		t.Errorf("SinAtributosFirmados = %v, SigningTime = %v", sig.SinAtributosFirmados, sig.SigningTime)
	}
	if _, err := cms.Inspect(der, []byte(contenido+" ")); !errors.Is(err, cms.ErrNoCorresponde) {
		t.Errorf("sobre otro contenido tiene que dar ErrNoCorresponde, dio: %v", err)
	}
}

// Decisión del 2026-10-16: un certificado con número de serie negativo se sigue
// rechazando, y con su motivo propio, no como «ilegible».
func TestSerieNegativaSeRechazaConSuMotivo(t *testing.T) {
	t.Parallel()
	ca, caKey := nuevaCA(t)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(0x55AA55AA), Subject: pkix.Name{CommonName: "Serie"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	cert := firmaCert(t, tmpl, ca, key.Public(), caKey)
	firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
	if err != nil {
		t.Fatal(err)
	}
	// El mismo INTEGER con el bit de signo puesto: 0xD5AA55AA es negativo.
	negativa := bytes.ReplaceAll(firma, []byte{0x02, 0x04, 0x55, 0xAA, 0x55, 0xAA}, []byte{0x02, 0x04, 0xD5, 0xAA, 0x55, 0xAA})
	if bytes.Equal(negativa, firma) {
		t.Fatal("no se encontró el número de serie en la firma")
	}
	if _, err := cms.Inspect(negativa, []byte(contenido)); !errors.Is(err, cms.ErrSerieNegativa) {
		t.Fatalf("err = %v, quería ErrSerieNegativa", err)
	}
}

// Una firma en BER —la SEQUENCE de fuera con longitud indefinida y relleno de
// ceros detrás, como en el hueco de un PDF— se lee igual. Antes: «no es un
// ContentInfo válido».
func TestFirmaEnBER(t *testing.T) {
	t.Parallel()
	_, cert, key := identidad(t, false)
	firma, err := cms.Sign(key, []*x509.Certificate{cert}, []byte(contenido))
	if err != nil {
		t.Fatal(err)
	}
	// 30 82 LL LL <hijos> → 30 80 <hijos> 00 00, y ceros de relleno.
	ini := 2 + int(firma[1]&0x7F)
	ber := append(append([]byte{0x30, 0x80}, firma[ini:]...), 0, 0, 0, 0, 0, 0)
	if _, err := cms.Inspect(ber, []byte(contenido)); err != nil {
		t.Fatalf("firma en BER: %v", err)
	}
	if _, err := cms.Inspect(ber, []byte(contenido+" ")); !errors.Is(err, cms.ErrNoCorresponde) {
		t.Errorf("sobre otro contenido: %v", err)
	}
}
