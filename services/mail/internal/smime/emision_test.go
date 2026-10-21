// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime_test

// LA TUBERÍA ENTERA, SIN LA CA DE VERDAD EN MEDIO.
//
// Emitir con una CA pública son tres pasos: pedimos (NuevaPeticion), la CA firma
// nuestra petición, y guardamos lo que devuelve (ImportIdentity). Aquí se hace el
// paso de en medio con una raíz de usar y tirar, porque lo que puede romperse no
// es la CA: es que la clave que nos quedamos no sea la del certificado que llega.
//
// Eso no da error al guardarlo. Da un correo firmado que el móvil de quien lo
// recibe marca como no válido, y ahí ya no se puede depurar. El árbitro vuelve a
// ser `openssl smime -verify` con SOLO la raíz.

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/smime"
)

func TestLoPedidoALaCAFirmaCorreo(t *testing.T) {
	t.Parallel()
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl en el entorno: este test necesita un verificador independiente")
	}
	const buzon = "ana@corporate.sytue.com"

	csrDER, keyPEM, err := smime.NuevaPeticion(buzon)
	if err != nil {
		t.Fatalf("NuevaPeticion: %v", err)
	}
	certPEM, raizPEM := emiteComoUnaCA(t, csrDER)

	// Si la clave que nos guardamos no fuera la del certificado emitido, es AQUÍ
	// donde se ve, y no en el teléfono del destinatario.
	id, err := smime.ImportIdentity(buzon, certPEM, keyPEM)
	if err != nil {
		t.Fatalf("el certificado emitido de nuestra petición no pasa el importador: %v", err)
	}

	firmado, err := smime.Sign(id, []byte(mensaje))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	dir := t.TempDir()
	msgPath, raizPath := filepath.Join(dir, "firmado.eml"), filepath.Join(dir, "raiz.pem")
	if err := os.WriteFile(msgPath, firmado, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(raizPath, []byte(raizPEM), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(openssl, "smime", "-verify", "-in", msgPath, "-CAfile", raizPath).CombinedOutput()
	if err != nil {
		t.Fatalf("openssl rechazó la firma: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Verification successful") {
		t.Errorf("openssl no confirmó la verificación:\n%s", out)
	}
}

// emiteComoUnaCA hace lo que hará CertiManager: leer la petición, firmar SU clave
// pública —no otra— y devolver la hoja con su cadena.
func emiteComoUnaCA(t *testing.T, csrDER []byte) (certPEM, raizPEM string) {
	t.Helper()
	csr, err := x509.ParseCertificateRequest(csrDER)
	if err != nil {
		t.Fatalf("la CA no sabría leer nuestra petición: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("la CA rechazaría la firma de nuestra petición: %v", err)
	}
	raizKey, err := rsaDePrueba()
	if err != nil {
		t.Fatal(err)
	}
	ahora := time.Now()
	raizTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Raíz de prueba"},
		NotBefore:             ahora.Add(-time.Hour),
		NotAfter:              ahora.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	raizDER, err := x509.CreateCertificate(rand.Reader, raizTmpl, raizTmpl, &raizKey.PublicKey, raizKey)
	if err != nil {
		t.Fatalf("emitir la raíz: %v", err)
	}
	raiz, err := x509.ParseCertificate(raizDER)
	if err != nil {
		t.Fatal(err)
	}
	hojaDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      csr.Subject,
		// Lo que la CA copia de la petición: el buzón. Si nuestro CSR no lo llevara,
		// el certificado saldría sin SAN y el cliente diría «no coincide el remitente».
		EmailAddresses: csr.EmailAddresses,
		NotBefore:      ahora.Add(-time.Hour),
		NotAfter:       ahora.Add(24 * time.Hour),
		KeyUsage:       x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:    []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
	}, raiz, csr.PublicKey, raizKey)
	if err != nil {
		t.Fatalf("emitir la hoja: %v", err)
	}
	return pemDe("CERTIFICATE", hojaDER) + pemDe("CERTIFICATE", raizDER), pemDe("CERTIFICATE", raizDER)
}

func rsaDePrueba() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, 2048) }
