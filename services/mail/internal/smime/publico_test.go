// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime_test

// Lo que se prueba aquí es lo ÚNICO que hace que un correo de Sytue se valide en
// el móvil de quien lo recibe sin instalar nada: firmar con el certificado de una
// CA pública, con su cadena dentro del sobre.
//
// El árbitro es `openssl smime -verify` y SOLO se le da la RAÍZ. Si acepta la
// firma es porque la intermedia viajaba incrustada en el CMS, que es justo lo que
// no hacía la implementación anterior y lo que rompe la validación fuera.

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/smime"
)

func pemDe(tipo string, der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: tipo, Bytes: der}))
}

// comoUnaCAPublica arma lo que entrega una CA pública de verdad: una raíz, una
// intermedia firmada por ella y el certificado del buzón firmado por la
// intermedia. Con claves RSA, que es lo que emiten casi todas —y lo que la firma
// de este paquete no sabía usar—.
func comoUnaCAPublica(t *testing.T, addr string) (raizPEM, bundlePEM string) {
	t.Helper()
	clave := func() *rsa.PrivateKey {
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generar clave RSA: %v", err)
		}
		return k
	}
	firmar := func(tmpl, padre *x509.Certificate, pub any, clavePadre *rsa.PrivateKey) (*x509.Certificate, []byte) {
		t.Helper()
		der, err := x509.CreateCertificate(rand.Reader, tmpl, padre, pub, clavePadre)
		if err != nil {
			t.Fatalf("emitir %s: %v", tmpl.Subject.CommonName, err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatalf("releer %s: %v", tmpl.Subject.CommonName, err)
		}
		return cert, der
	}

	raizKey, interKey, hojaKey := clave(), clave(), clave()
	ahora := time.Now()
	autoridad := func(serie int64, nombre string) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber:          big.NewInt(serie),
			Subject:               pkix.Name{CommonName: nombre},
			NotBefore:             ahora.Add(-time.Hour),
			NotAfter:              ahora.Add(24 * time.Hour),
			KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
			BasicConstraintsValid: true,
			IsCA:                  true,
		}
	}
	raizTmpl := autoridad(1, "Raíz de prueba")
	raiz, raizDER := firmar(raizTmpl, raizTmpl, &raizKey.PublicKey, raizKey)
	inter, interDER := firmar(autoridad(2, "Intermedia de prueba"), raiz, &interKey.PublicKey, raizKey)
	_, hojaDER := firmar(&x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "Ana Ruiz"},
		EmailAddresses:        []string{addr},
		NotBefore:             ahora.Add(-time.Hour),
		NotAfter:              ahora.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
		BasicConstraintsValid: true,
	}, inter, &hojaKey.PublicKey, interKey)

	keyDER, err := x509.MarshalPKCS8PrivateKey(hojaKey)
	if err != nil {
		t.Fatalf("serializar la clave del buzón: %v", err)
	}
	// En el orden que sale de `openssl pkcs12 -nodes`: primero la clave, luego el
	// certificado y luego la cadena.
	return pemDe("CERTIFICATE", raizDER),
		pemDe("PRIVATE KEY", keyDER) + pemDe("CERTIFICATE", hojaDER) + pemDe("CERTIFICATE", interDER)
}

func TestCertificadoDeCAPublicaSeValidaSoloConLaRaiz(t *testing.T) {
	t.Parallel()
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl en el entorno: este test necesita un verificador independiente")
	}
	raizPEM, bundlePEM := comoUnaCAPublica(t, "ana@acme.test")

	certPEM, keyPEM := smime.SplitBundle(bundlePEM)
	id, err := smime.ImportIdentity("ana@acme.test", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("ImportIdentity: %v", err)
	}
	if len(id.Chain) != 1 {
		t.Fatalf("la intermedia no se guardó: Chain tiene %d certificados", len(id.Chain))
	}

	signed, err := smime.Sign(id, []byte(mensaje))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	dir := t.TempDir()
	msgPath, raizPath := filepath.Join(dir, "mensaje.eml"), filepath.Join(dir, "raiz.pem")
	if err := os.WriteFile(msgPath, signed, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(raizPath, []byte(raizPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	// SIN la intermedia: es la prueba. Si openssl llega hasta la raíz es porque
	// venía dentro del mensaje, que es lo que tendrá el móvil de quien lo reciba.
	out, err := exec.Command(openssl, "smime", "-verify", "-in", msgPath, "-CAfile", raizPath).CombinedOutput()
	if err != nil {
		t.Fatalf("openssl rechazó la firma de un certificado público: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Verification successful") {
		t.Errorf("openssl no confirmó la verificación:\n%s", out)
	}
}

func TestImportIdentityRechazaLoQueNoSirve(t *testing.T) {
	t.Parallel()
	_, bundlePEM := comoUnaCAPublica(t, "ana@acme.test")
	certPEM, keyPEM := smime.SplitBundle(bundlePEM)

	casos := map[string]struct{ addr, cert, key string }{
		"de otro buzón":        {"luis@acme.test", certPEM, keyPEM},
		"sin clave":            {"ana@acme.test", certPEM, ""},
		"sin certificado":      {"ana@acme.test", "", keyPEM},
		"clave de otro cert":   {"ana@acme.test", certPEM, otraClave(t)},
		"certificado ilegible": {"ana@acme.test", pemDe("CERTIFICATE", []byte("esto no es DER")), keyPEM},
	}
	for nombre, c := range casos {
		t.Run(nombre, func(t *testing.T) {
			t.Parallel()
			_, err := smime.ImportIdentity(c.addr, c.cert, c.key)
			if err == nil {
				t.Fatal("se aceptó material que no sirve")
			}
			// Tiene que ser reconocible como culpa de quien lo manda: quien lo expone
			// por HTTP contesta 400 con el motivo en vez de un 500 mudo.
			if !errors.Is(err, smime.ErrMaterialInvalido) {
				t.Errorf("el error no es de material inválido: %v", err)
			}
		})
	}
}

// otraClave devuelve una clave RSA que no es la de ningún certificado del test.
func otraClave(t *testing.T) string {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pemDe("PRIVATE KEY", der)
}

func TestIdentidadImportadaSobreviveAlGuardado(t *testing.T) {
	t.Parallel()
	_, bundlePEM := comoUnaCAPublica(t, "ana@acme.test")
	certPEM, keyPEM := smime.SplitBundle(bundlePEM)
	id, err := smime.ImportIdentity("ana@acme.test", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("ImportIdentity: %v", err)
	}

	guardadoCert, guardadaKey, err := id.PEM()
	if err != nil {
		t.Fatalf("PEM: %v", err)
	}
	vuelta, err := smime.LoadIdentity("ana@acme.test", guardadoCert, guardadaKey)
	if err != nil {
		t.Fatalf("LoadIdentity de lo guardado: %v", err)
	}
	// La cadena es lo que se pierde si se guarda solo la hoja, y sin ella la firma
	// deja de validar fuera: se comprueba que da la vuelta entera.
	if len(vuelta.Chain) != 1 {
		t.Errorf("la cadena no sobrevivió al guardado: %d certificados", len(vuelta.Chain))
	}
	if !vuelta.Certificate.Equal(id.Certificate) {
		t.Error("el certificado que vuelve no es el mismo")
	}
	if _, err := smime.Sign(vuelta, []byte(mensaje)); err != nil {
		t.Errorf("no se puede firmar con la identidad releída: %v", err)
	}
}

func TestSeSigueLeyendoLaClaveGuardadaComoSEC1(t *testing.T) {
	t.Parallel()
	// Los buzones que ya tienen certificado guardaron su clave en SEC1 («EC
	// PRIVATE KEY»). Si dejara de leerse, su correo saldría sin firmar el día del
	// despliegue.
	ca, err := smime.NewCA("Acme S.L.")
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	id, err := ca.Issue("ana@acme.test", "Ana Ruiz")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	ec, ok := id.Key.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("la autoridad ya no emite con ECDSA, sino con %T", id.Key)
	}
	sec1, err := x509.MarshalECPrivateKey(ec)
	if err != nil {
		t.Fatalf("serializar en SEC1: %v", err)
	}
	certPEM := pemDe("CERTIFICATE", id.CertDER)
	vuelta, err := smime.LoadIdentity("ana@acme.test", certPEM, pemDe("EC PRIVATE KEY", sec1))
	if err != nil {
		t.Fatalf("no se pudo leer una clave guardada en SEC1: %v", err)
	}
	if _, err := smime.Sign(vuelta, []byte(mensaje)); err != nil {
		t.Errorf("no se puede firmar con la clave vieja: %v", err)
	}
}

func TestSplitBundleSeparaCertificadosYClave(t *testing.T) {
	t.Parallel()
	_, bundlePEM := comoUnaCAPublica(t, "ana@acme.test")
	certPEM, keyPEM := smime.SplitBundle(bundlePEM)
	if n := strings.Count(certPEM, "BEGIN CERTIFICATE"); n != 2 {
		t.Errorf("se esperaban el certificado y su intermedia, hay %d", n)
	}
	if strings.Contains(certPEM, "PRIVATE KEY") {
		t.Error("la clave privada se coló entre los certificados")
	}
	if n := strings.Count(keyPEM, "BEGIN"); n != 1 {
		t.Errorf("se esperaba una sola clave, hay %d bloques", n)
	}
}
