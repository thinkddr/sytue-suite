// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// La firma en dos pasos validada por herramientas que NO son nuestras: OpenSSL
// (el CMS y el sello), pdfsig de poppler (el PDF entero) y pyHanko (el perfil
// PAdES). Un validador propio que da por buena una firma propia prueba poco; estos
// tres no saben nada de cómo se escribió.
//
// Cada herramienta que falte se salta y lo dice. El sello lo emite OpenSSL haciendo
// de autoridad (`openssl ts -reply`), con una TSA de prueba generada aquí: es un
// token RFC 3161 de verdad, firmado, que los tres validadores comprueban.
package domain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tsaDePrueba es una autoridad de sellado con OpenSSL: su CA, su certificado con
// el uso timeStamping CRÍTICO (sin eso OpenSSL no firma sellos) y su config.
type tsaDePrueba struct {
	openssl, dir string
}

func nuevaTSADePrueba(t *testing.T, openssl, dir string) tsaDePrueba {
	t.Helper()
	claveCA, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	pCA := &x509.Certificate{
		SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "CA de sellos de prueba"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	derCA, err := x509.CreateCertificate(rand.Reader, pCA, pCA, &claveCA.PublicKey, claveCA)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(derCA)
	clave, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	eku, _ := asn1.Marshal([]asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 8}})
	p := &x509.Certificate{
		SerialNumber: big.NewInt(8), Subject: pkix.Name{CommonName: "TSA de prueba"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage:        x509.KeyUsageDigitalSignature,
		ExtraExtensions: []pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Critical: true, Value: eku}},
	}
	der, err := x509.CreateCertificate(rand.Reader, p, ca, &clave.PublicKey, claveCA)
	if err != nil {
		t.Fatal(err)
	}
	claveDER, _ := x509.MarshalPKCS8PrivateKey(clave)
	escribir(t, dir, "tsa.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	escribir(t, dir, "tsakey.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: claveDER}))
	escribir(t, dir, "tsaca.pem", pemDe(ca))
	escribir(t, dir, "serie", []byte("01\n"))
	escribir(t, dir, "tsa.cnf", []byte(`[ tsa ]
default_tsa = tsa_sytue
[ tsa_sytue ]
serial = `+filepath.Join(dir, "serie")+`
crypto_device = builtin
signer_cert = `+filepath.Join(dir, "tsa.pem")+`
certs = `+filepath.Join(dir, "tsaca.pem")+`
signer_key = `+filepath.Join(dir, "tsakey.pem")+`
signer_digest = sha256
default_policy = 1.2.3.4.1
digests = sha256
accuracy = secs:1
ordering = yes
tsa_name = no
ess_cert_id_chain = no
ess_cert_id_alg = sha256
`))
	return tsaDePrueba{openssl: openssl, dir: dir}
}

// sellar hace de autoridad: petición y respuesta con OpenSSL, y devuelve el token.
func (s tsaDePrueba) sellar(huella []byte) ([]byte, error) {
	q, tok := filepath.Join(s.dir, "q.tsq"), filepath.Join(s.dir, "tok.der")
	if out, err := exec.Command(s.openssl, "ts", "-query", "-digest", hex.EncodeToString(huella),
		"-sha256", "-cert", "-out", q).CombinedOutput(); err != nil {
		return nil, &errHerramienta{"openssl ts -query", out, err}
	}
	if out, err := exec.Command(s.openssl, "ts", "-reply", "-config", filepath.Join(s.dir, "tsa.cnf"),
		"-queryfile", q, "-token_out", "-out", tok).CombinedOutput(); err != nil {
		return nil, &errHerramienta{"openssl ts -reply", out, err}
	}
	return os.ReadFile(tok)
}

type errHerramienta struct {
	orden string
	out   []byte
	err   error
}

func (e *errHerramienta) Error() string { return e.orden + ": " + e.err.Error() + "\n" + string(e.out) }

func escribir(t *testing.T, dir, nombre string, datos []byte) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func TestLaFirmaEnDosPasosLaValidanHerramientasDeFuera(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl no hay autoridad de sellado de prueba")
	}
	dir := t.TempDir()
	tsa := nuevaTSADePrueba(t, openssl, dir)
	id := nuevaIdentidadSistema(t, false)
	p, err := PrepararFirmaPDF(docDePrueba(t, "contrato que valida otro"), OpcionesFirma{
		Motivo: "Conforme", Lugar: "Bilbao", Rect: [4]float64{80, 80, 300, 150},
	}, id.cadena())
	if err != nil {
		t.Fatal(err)
	}
	firmado, sellado, err := CompletarFirmaPDF(*p, id.cadena(), firmaDelResumen(t, id.clave, p), tsa.sellar)
	if err != nil || !sellado {
		t.Fatalf("CompletarFirmaPDF: sellado=%v err=%v", sellado, err)
	}
	pdf := escribir(t, dir, "firmado.pdf", firmado)
	ca := escribir(t, dir, "ca.pem", pemDe(id.ca))
	der, contenido := cmsDelPDF(t, firmado, 0)

	t.Run("openssl cms", func(t *testing.T) {
		out, err := exec.Command(openssl, "cms", "-verify", "-binary", "-inform", "DER",
			"-in", escribir(t, dir, "firma.der", der), "-content", escribir(t, dir, "contenido.bin", contenido),
			"-CAfile", ca, "-purpose", "any", "-out", os.DevNull).CombinedOutput()
		if err != nil || !bytes.Contains(out, []byte("Verification successful")) {
			t.Fatalf("openssl no verifica el CMS: %v\n%s", err, out)
		}
	})

	t.Run("pdfsig", func(t *testing.T) {
		pdfsig, err := exec.LookPath("pdfsig")
		if err != nil {
			t.Skip("pdfsig (poppler-utils) no está instalado")
		}
		out, err := exec.Command(pdfsig, pdf).CombinedOutput()
		t.Logf("pdfsig:\n%s", out)
		if err != nil && !bytes.Contains(out, []byte("Signature is Valid")) {
			t.Fatalf("pdfsig falla: %v", err)
		}
		for _, quiere := range []string{"Signature Type: ETSI.CAdES.detached", "Signature Validation: Signature is Valid", "Total document signed"} {
			if !bytes.Contains(out, []byte(quiere)) {
				t.Errorf("pdfsig no dice %q", quiere)
			}
		}
	})

	t.Run("pyHanko", func(t *testing.T) {
		pyhanko := os.Getenv("SYTUE_PYHANKO")
		if pyhanko == "" {
			pyhanko, _ = exec.LookPath("pyhanko")
		}
		if pyhanko == "" {
			t.Skip("pyHanko no está (SYTUE_PYHANKO apunta al de un entorno aislado)")
		}
		out, err := exec.Command(pyhanko, "sign", "validate", "--pretty-print", "--no-revocation-check",
			"--trust-replace", "--trust", ca, "--trust", filepath.Join(dir, "tsaca.pem"), pdf).CombinedOutput()
		t.Logf("pyHanko:\n%s", out)
		if err != nil {
			t.Fatalf("pyHanko no la da por buena: %v", err)
		}
		// La CLI no escribe el nivel PAdES; lo que sí dice, frase a frase, es lo que
		// lo define: firma sana, cadena de confianza, sello SOBRE la firma y válido,
		// y el fichero entero cubierto.
		for _, quiere := range []string{
			"The signature is cryptographically sound.",
			"The signer's certificate is trusted.",
			"The token is guaranteed to be newer than the signature.",
			"The timestamp token is cryptographically sound.",
			"The TSA certificate is trusted.",
			"The signature covers the entire file.",
			"The signature is judged VALID.",
		} {
			if !strings.Contains(string(out), quiere) {
				t.Errorf("pyHanko no dice %q", quiere)
			}
		}
	})
}

// responder hace de autoridad HTTP de verdad: la petición entra por el cuerpo y
// sale la TimeStampResp entera que escribe OpenSSL.
func (s tsaDePrueba) responder(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ := io.ReadAll(r.Body)
		q, resp := filepath.Join(s.dir, "http.tsq"), filepath.Join(s.dir, "http.tsr")
		escribir(t, s.dir, "http.tsq", cuerpo)
		if out, err := exec.Command(s.openssl, "ts", "-reply", "-config", filepath.Join(s.dir, "tsa.cnf"),
			"-queryfile", q, "-out", resp).CombinedOutput(); err != nil {
			t.Errorf("openssl ts -reply: %v\n%s", err, out)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		datos, _ := os.ReadFile(resp)
		_, _ = w.Write(datos)
	}))
}

// TestConAnclasSoloEntraElSelloDeLaAutoridad es el check contra el intermediario
// que FABRICA un token: sobre HTTP plano puede devolver uno con la huella y el
// nonce buenos, firmado por cualquiera —o sin firmar, como el de autoridadFalsa—.
// Con las anclas de la autoridad, ese no entra y el de la autoridad sí.
func TestConAnclasSoloEntraElSelloDeLaAutoridad(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl no hay autoridad de sellado de prueba")
	}
	dir := t.TempDir()
	tsa := nuevaTSADePrueba(t, openssl, dir)
	raiz, err := os.ReadFile(filepath.Join(dir, "tsaca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	anclas := x509.NewCertPool()
	anclas.AppendCertsFromPEM(raiz)
	huella := bytes.Repeat([]byte{0x5a}, 32)

	buena := tsa.responder(t)
	defer buena.Close()
	if _, err := PedirSello(t.Context(), huella, OpcionesSello{URL: buena.URL, Anclas: anclas}); err != nil {
		t.Fatalf("el sello de la autoridad no entra: %v", err)
	}

	fabricada := autoridadFalsa(t, 0, nil, nil)
	defer fabricada.Close()
	if _, err := PedirSello(t.Context(), huella, OpcionesSello{URL: fabricada.URL, Anclas: anclas}); !errors.Is(err, ErrSelloRechazado) {
		t.Fatalf("un token fabricado con la huella y el nonce buenos ha entrado: err = %v", err)
	}
	otraCA := x509.NewCertPool()
	otraCA.AddCert(nuevaIdentidadSistema(t, true).ca)
	if _, err := PedirSello(t.Context(), huella, OpcionesSello{URL: buena.URL, Anclas: otraCA}); !errors.Is(err, ErrSelloRechazado) {
		t.Fatalf("un sello de otra autoridad ha entrado: err = %v", err)
	}
}

// TestSelloRealCualificado es la prueba de extremo a extremo con la autoridad DE
// VERDAD: dos pasos, sello pedido a `SYTUE_SELLO_REAL` (p. ej. http://tsa.izenpe.com)
// y autenticado con las anclas de `SYTUE_SELLO_ANCLAS` (un PEM), y pyHanko diciendo
// que la TSA está en la lista de confianza de la UE para España. No corre por
// defecto: sale a Internet y gasta una petición del cupo gratuito de la autoridad.
func TestSelloRealCualificado(t *testing.T) {
	url, anclasPEM, pyhanko := os.Getenv("SYTUE_SELLO_REAL"), os.Getenv("SYTUE_SELLO_ANCLAS"), os.Getenv("SYTUE_PYHANKO")
	if url == "" || anclasPEM == "" || pyhanko == "" {
		t.Skip("SYTUE_SELLO_REAL, SYTUE_SELLO_ANCLAS y SYTUE_PYHANKO: sin los tres no se sale a la autoridad")
	}
	raiz, err := os.ReadFile(anclasPEM)
	if err != nil {
		t.Fatal(err)
	}
	anclas := x509.NewCertPool()
	if !anclas.AppendCertsFromPEM(raiz) {
		t.Fatal("SYTUE_SELLO_ANCLAS no trae ningún certificado")
	}
	id := nuevaIdentidadSistema(t, true)
	p, err := PrepararFirmaPDF(docDePrueba(t, "contrato sellado de verdad"), OpcionesFirma{Motivo: "Conforme"}, id.cadena())
	if err != nil {
		t.Fatal(err)
	}
	sello := func(huella []byte) ([]byte, error) {
		return PedirSello(t.Context(), huella, OpcionesSello{URL: url, Anclas: anclas, Timeout: 30 * time.Second})
	}
	firmado, sellado, err := CompletarFirmaPDF(*p, id.cadena(), firmaDelResumen(t, id.clave, p), sello)
	if err != nil || !sellado {
		t.Fatalf("CompletarFirmaPDF: sellado=%v err=%v", sellado, err)
	}
	dir := t.TempDir()
	pdf := escribir(t, dir, "firmado.pdf", firmado)
	if destino := os.Getenv("SYTUE_SELLO_REAL_PDF"); destino != "" {
		_ = os.WriteFile(destino, firmado, 0o600)
	}
	// La firmante es de prueba (su CA se da por buena a mano); la TSA, NO: su
	// confianza sale de la lista de confianza de la UE, territorio ES.
	out, err := exec.Command(pyhanko, "sign", "validate", "--pretty-print", "--no-revocation-check",
		"--trust", escribir(t, dir, "ca.pem", pemDe(id.ca)), "--eutl-territories", "ES", pdf).CombinedOutput()
	t.Logf("pyHanko:\n%s", out)
	if err != nil {
		t.Fatalf("pyHanko no la da por buena: %v", err)
	}
	for _, quiere := range []string{
		"The timestamp token is cryptographically sound.",
		"The TSA certificate is trusted.",
		"The signature covers the entire file.",
		"The signature is judged VALID.",
	} {
		if !strings.Contains(string(out), quiere) {
			t.Errorf("pyHanko no dice %q", quiere)
		}
	}
}
