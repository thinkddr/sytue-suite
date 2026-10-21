// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build darwin && cgo && !ios

package certificados

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// llaveroTemporal crea un llavero en un directorio temporal con la identidad de `clave`
// y la CA que la emite, y lo borra al acabar. `security create-keychain` lo añade a la
// lista de búsqueda de la persona: se deja la lista como estaba justo después, para que
// ni un fallo a medias le cambie a nadie sus llaveros.
func llaveroTemporal(t *testing.T, clave crypto.Signer) (*Llavero, *x509.Certificate, *x509.Certificate) {
	t.Helper()
	security, err := exec.LookPath("security")
	if err != nil {
		t.Skip("sin la orden security no hay llavero de prueba")
	}
	antes, err := exec.Command(security, "list-keychains", "-d", "user").Output()
	if err != nil {
		t.Fatal(err)
	}
	var lista []string
	for _, l := range strings.Split(string(antes), "\n") {
		if l = strings.Trim(strings.TrimSpace(l), `"`); l != "" {
			lista = append(lista, l)
		}
	}
	dir := t.TempDir()
	ruta := filepath.Join(dir, "sytue-prueba.keychain-db")
	correr(t, security, "create-keychain", "-p", "prueba", ruta)
	correr(t, security, append([]string{"list-keychains", "-d", "user", "-s"}, lista...)...)
	t.Cleanup(func() { _ = exec.Command(security, "delete-keychain", ruta).Run() })
	correr(t, security, "unlock-keychain", "-p", "prueba", ruta)
	correr(t, security, "set-keychain-settings", ruta)

	ca, claveCA := emitir(t, nil, nil, "CA de pruebas del Llavero", true, nil)
	cert, _ := emitir(t, ca, claveCA, "Irati Etxeberria (FIRMA)", false, clave)
	claveDER, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	pemClave := escribir(t, filepath.Join(dir, "clave.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: claveDER}))
	pemCert := escribir(t, filepath.Join(dir, "cert.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
	// La identidad entra como la trae cualquier CA: un .p12 con la clave y el
	// certificado. Lo escribe el openssl del sistema (Go no sabe escribir PKCS#12).
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl no hay .p12 que importar")
	}
	p12 := filepath.Join(dir, "firma.p12")
	correr(t, openssl, "pkcs12", "-export", "-inkey", pemClave, "-in", pemCert, "-out", p12, "-passout", "pass:prueba")
	// -A: cualquier app usa la clave sin preguntar. En la prueba no hay nadie que pulse
	// «Permitir»; en el llavero de verdad, el sistema sí lo pregunta.
	correr(t, security, "import", p12, "-k", ruta, "-f", "pkcs12", "-P", "prueba", "-A")
	correr(t, security, "import", escribir(t, filepath.Join(dir, "ca.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: ca.Raw})), "-k", ruta)
	return &Llavero{ruta: ruta}, cert, ca
}

func TestFirmarConElLlavero(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		clave  func() (crypto.Signer, error)
	}{
		{"RSA", func() (crypto.Signer, error) { return rsa.GenerateKey(rand.Reader, 2048) }},
		{"ECDSA P-256", func() (crypto.Signer, error) { return ecdsa.GenerateKey(elliptic.P256(), rand.Reader) }},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			clave, err := caso.clave()
			if err != nil {
				t.Fatal(err)
			}
			l, cert, ca := llaveroTemporal(t, clave)
			ctx := context.Background()
			cs, err := l.Certificados(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(cs) != 1 || !cs[0].Cadena[0].Equal(cert) || len(cs[0].Cadena) != 2 || !cs[0].Cadena[1].Equal(ca) || cs[0].PidePIN {
				t.Fatalf("certificados = %+v", cs)
			}
			resumen := sha256.Sum256([]byte("atributos firmados de un contrato"))
			prep := api.PDFFirmaPreparada{Resumen: resumen[:], DigestInfo: append(append([]byte(nil), prefijoDigestInfoSHA256...), resumen[:]...)}
			firma, formato, err := l.Firmar(ctx, cs[0], prep, "")
			if err != nil {
				t.Fatal(err)
			}
			if formato != api.PDFFirmaDER {
				t.Errorf("formato = %q", formato)
			}
			switch k := cert.PublicKey.(type) {
			case *rsa.PublicKey:
				if rsa.VerifyPKCS1v15(k, crypto.SHA256, resumen[:], firma) != nil {
					t.Error("la firma RSA del Llavero no verifica")
				}
			case *ecdsa.PublicKey:
				if !ecdsa.VerifyASN1(k, resumen[:], firma) {
					t.Error("la firma ECDSA del Llavero no verifica")
				}
			}
		})
	}
}
