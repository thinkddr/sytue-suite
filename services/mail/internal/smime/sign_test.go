// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package smime_test

// La prueba de que una firma S/MIME sirve no es que la verifique quien la hizo:
// es que la acepte OTRO. Estos tests firman con nuestro código y verifican con
// `openssl smime -verify`, que es una implementación independiente y la misma
// familia de código que usan por debajo muchos clientes de correo.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/services/mail/internal/smime"
)

const mensaje = "From: Ana <ana@acme.test>\r\n" +
	"To: luis@otra.example\r\n" +
	"Subject: Presupuesto\r\n" +
	"Date: Mon, 03 Aug 2026 09:12:00 +0000\r\n" +
	"MIME-Version: 1.0\r\n" +
	"Content-Type: text/plain; charset=utf-8\r\n" +
	"\r\n" +
	"Te paso el presupuesto firmado.\r\n"

func TestFirmaLaAceptaOpenSSL(t *testing.T) {
	t.Parallel()
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl en el entorno: este test necesita un verificador independiente")
	}

	ca, err := smime.NewCA("Acme S.L.")
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	id, err := ca.Issue("ana@acme.test", "Ana Ruiz")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	signed, err := smime.Sign(id, []byte(mensaje))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	dir := t.TempDir()
	msgPath := filepath.Join(dir, "mensaje.eml")
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(msgPath, signed, 0o600); err != nil {
		t.Fatal(err)
	}
	caPEM, _, err := ca.PEM()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caPath, []byte(caPEM), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(openssl, "smime", "-verify",
		"-in", msgPath, "-CAfile", caPath, "-purpose", "any").CombinedOutput()
	if err != nil {
		t.Fatalf("openssl rechazó la firma: %v\n%s", err, out)
	}
	// openssl escribe el contenido verificado: tiene que ser el original.
	if !strings.Contains(string(out), "Te paso el presupuesto firmado.") {
		t.Errorf("el contenido verificado no es el del mensaje:\n%s", out)
	}
	if !strings.Contains(string(out), "Verification successful") {
		t.Errorf("openssl no confirmó la verificación:\n%s", out)
	}
}

func TestFirmaSeRompeSiCambiaElCuerpo(t *testing.T) {
	t.Parallel()
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl en el entorno")
	}
	ca, _ := smime.NewCA("Acme S.L.")
	id, _ := ca.Issue("ana@acme.test", "Ana Ruiz")
	signed, err := smime.Sign(id, []byte(mensaje))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	// Alguien manipula el importe por el camino.
	manipulado := strings.Replace(string(signed), "presupuesto firmado", "presupuesto MODIFICADO", 1)

	dir := t.TempDir()
	msgPath := filepath.Join(dir, "manipulado.eml")
	caPath := filepath.Join(dir, "ca.pem")
	_ = os.WriteFile(msgPath, []byte(manipulado), 0o600)
	caPEM, _, _ := ca.PEM()
	_ = os.WriteFile(caPath, []byte(caPEM), 0o600)

	if out, err := exec.Command(openssl, "smime", "-verify",
		"-in", msgPath, "-CAfile", caPath, "-purpose", "any").CombinedOutput(); err == nil {
		t.Fatalf("un mensaje manipulado pasó la verificación:\n%s", out)
	}
}

// TestCertificadoDeBuzonEsUsableParaCorreo: el certificado tiene que llevar la
// dirección en el SAN y el uso de protección de correo. Sin eso, la firma es
// válida pero el cliente dice «no coincide con el remitente», que es peor que
// no firmar.
func TestCertificadoDeBuzonEsUsableParaCorreo(t *testing.T) {
	t.Parallel()
	ca, err := smime.NewCA("Acme S.L.")
	if err != nil {
		t.Fatalf("NewCA: %v", err)
	}
	id, err := ca.Issue("Ana@Acme.test", "Ana Ruiz")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(id.Certificate.EmailAddresses) != 1 || id.Certificate.EmailAddresses[0] != "ana@acme.test" {
		t.Errorf("el certificado no lleva la dirección normalizada: %v", id.Certificate.EmailAddresses)
	}
	var emailProtection bool
	for _, u := range id.Certificate.ExtKeyUsage {
		if u.String() == "1" || u == 4 { // x509.ExtKeyUsageEmailProtection
			emailProtection = true
		}
	}
	if !emailProtection {
		t.Errorf("el certificado no declara protección de correo: %v", id.Certificate.ExtKeyUsage)
	}
	if id.NeedsRenewal(time.Now()) {
		t.Error("un certificado recién emitido no puede necesitar renovación")
	}
	if !id.NeedsRenewal(id.NotAfter.Add(-24 * time.Hour)) {
		t.Error("un certificado a un día de caducar debe renovarse solo")
	}
}

// TestIdentidadSobreviveAlVault: emitir, guardar y recuperar tiene que dar la
// misma identidad; si no, cada reinicio del servicio dejaría de firmar.
func TestIdentidadSobreviveAlVault(t *testing.T) {
	t.Parallel()
	ca, _ := smime.NewCA("Acme S.L.")
	id, _ := ca.Issue("ana@acme.test", "Ana")
	certPEM, keyPEM, err := id.PEM()
	if err != nil {
		t.Fatalf("PEM: %v", err)
	}
	back, err := smime.LoadIdentity("ana@acme.test", certPEM, keyPEM)
	if err != nil {
		t.Fatalf("LoadIdentity: %v", err)
	}
	if !back.Certificate.Equal(id.Certificate) {
		t.Error("el certificado recuperado no es el mismo")
	}
	if _, err := smime.Sign(back, []byte(mensaje)); err != nil {
		t.Errorf("la identidad recuperada ya no firma: %v", err)
	}

	caCert, caKey, err := ca.PEM()
	if err != nil {
		t.Fatal(err)
	}
	backCA, err := smime.LoadCA(caCert, caKey)
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	if _, err := backCA.Issue("otra@acme.test", "Otra"); err != nil {
		t.Errorf("la autoridad recuperada ya no emite: %v", err)
	}
}
