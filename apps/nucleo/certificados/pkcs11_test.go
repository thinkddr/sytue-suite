// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build cgo && (linux || darwin || windows) && !android && !ios

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
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/apps/nucleo/api"
)

// Una TARJETA de verdad, pero de software: SoftHSM2 habla PKCS#11 igual que el módulo de
// OpenSC con un DNIe, con su PIN y sus objetos. Se prepara con sus propias herramientas
// (softhsm2-util y pkcs11-tool), así que lo que se prueba es este almacén contra un
// módulo que no sabe nada de él. Sin ellas, se salta y lo dice.

// moduloSoftHSM es la biblioteca de SoftHSM2 de esta máquina, o "".
func moduloSoftHSM() string {
	for _, r := range []string{
		os.Getenv("SYTUE_SOFTHSM"),
		"/usr/lib/softhsm/libsofthsm2.so",
		"/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so",
		"/usr/lib/aarch64-linux-gnu/softhsm/libsofthsm2.so",
		"/opt/homebrew/lib/softhsm/libsofthsm2.so",
	} {
		if r != "" {
			if _, err := os.Stat(r); err == nil {
				return r
			}
		}
	}
	return ""
}

// tarjetaDePrueba crea un token con `clave` y su certificado autofirmado emitido por una
// CA que también se guarda en la tarjeta, y devuelve el módulo abierto.
func tarjetaDePrueba(t *testing.T, clave crypto.Signer) (*PKCS11, *x509.Certificate, *x509.Certificate) {
	t.Helper()
	mod := moduloSoftHSM()
	util, err1 := exec.LookPath("softhsm2-util")
	tool, err2 := exec.LookPath("pkcs11-tool")
	if mod == "" || err1 != nil || err2 != nil {
		t.Skip("sin SoftHSM2 (softhsm2-util) y OpenSC (pkcs11-tool) no hay tarjeta de prueba")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "tokens"), 0o700); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(dir, "softhsm2.conf")
	escribir(t, conf, []byte("directories.tokendir = "+filepath.Join(dir, "tokens")+"\nobjectstore.backend = file\n"))
	t.Setenv("SOFTHSM2_CONF", conf)

	ca, claveCA := emitir(t, nil, nil, "CA de pruebas de tarjetas", true, nil)
	cert, _ := emitir(t, ca, claveCA, "Irati Etxeberria (FIRMA)", false, clave)

	claveDER, err := x509.MarshalPKCS8PrivateKey(clave)
	if err != nil {
		t.Fatal(err)
	}
	pemClave := escribir(t, filepath.Join(dir, "clave.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: claveDER}))
	correr(t, util, "--init-token", "--free", "--label", "DNIe de prueba", "--pin", "1234", "--so-pin", "56781234")
	correr(t, util, "--import", pemClave, "--token", "DNIe de prueba", "--label", "firma", "--id", "01", "--pin", "1234")
	correr(t, tool, "--module", mod, "--token-label", "DNIe de prueba", "--login", "--pin", "1234",
		"--write-object", escribir(t, filepath.Join(dir, "cert.der"), cert.Raw), "--type", "cert", "--id", "01", "--label", "firma")
	correr(t, tool, "--module", mod, "--token-label", "DNIe de prueba", "--login", "--pin", "1234",
		"--write-object", escribir(t, filepath.Join(dir, "ca.der"), ca.Raw), "--type", "cert", "--id", "0a", "--label", "ca")

	p, err := AbrirPKCS11(mod, "SoftHSM")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Cerrar() })
	return p, cert, ca
}

func TestFirmarConUnaTarjetaPKCS11(t *testing.T) {
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
			p, cert, ca := tarjetaDePrueba(t, clave)
			ctx := context.Background()

			cs, err := p.Certificados(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// La CA también está en la tarjeta, pero no firma: solo sale el de firma, con
			// la CA detrás como su cadena.
			if len(cs) != 1 || !cs[0].Cadena[0].Equal(cert) || len(cs[0].Cadena) != 2 || !cs[0].Cadena[1].Equal(ca) {
				t.Fatalf("certificados = %+v", cs)
			}
			if !cs[0].PidePIN || cs[0].Almacen != "SoftHSM · DNIe de prueba" || cs[0].Nombre() != "Irati Etxeberria (FIRMA)" {
				t.Errorf("lo que se enseña: pin=%v almacén=%q nombre=%q", cs[0].PidePIN, cs[0].Almacen, cs[0].Nombre())
			}

			resumen := sha256.Sum256([]byte("atributos firmados de un contrato"))
			prep := api.PDFFirmaPreparada{
				Resumen:    resumen[:],
				DigestInfo: append([]byte{0x30, 0x31, 0x30, 0x0d, 0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, 0x05, 0x00, 0x04, 0x20}, resumen[:]...),
			}
			if _, _, err := p.Firmar(ctx, cs[0], prep, "0000"); !errors.Is(err, ErrPIN) {
				t.Fatalf("con otro PIN: err = %v", err)
			}
			firma, formato, err := p.Firmar(ctx, cs[0], prep, "1234")
			if err != nil {
				t.Fatal(err)
			}
			// Lo que devuelve la tarjeta es una firma de ESE resumen con ESA clave, en el
			// formato que dice: la misma comprobación que hará el servicio.
			switch k := cert.PublicKey.(type) {
			case *rsa.PublicKey:
				if formato != api.PDFFirmaDER || rsa.VerifyPKCS1v15(k, crypto.SHA256, resumen[:], firma) != nil {
					t.Errorf("la firma RSA no verifica (formato %q)", formato)
				}
			case *ecdsa.PublicKey:
				n := len(firma) / 2
				r, s := new(big.Int).SetBytes(firma[:n]), new(big.Int).SetBytes(firma[n:])
				if formato != api.PDFFirmaP1363 || len(firma) != 64 || !ecdsa.Verify(k, resumen[:], r, s) {
					t.Errorf("la firma ECDSA no verifica (formato %q, %d bytes)", formato, len(firma))
				}
			}
		})
	}
}

// Una tarjeta que ya no está (o un certificado de otra) no firma: lo dice.
// Un resumen y un DigestInfo que no son lo mismo no llegan a la tarjeta.
func TestUnaPreparadaIncoherenteNoSeFirma(t *testing.T) {
	clave, _ := rsa.GenerateKey(rand.Reader, 2048)
	p, _, _ := tarjetaDePrueba(t, clave)
	cs, _ := p.Certificados(context.Background())
	resumen := sha256.Sum256([]byte("uno"))
	otro := sha256.Sum256([]byte("otro"))
	prep := api.PDFFirmaPreparada{Resumen: resumen[:], DigestInfo: append(append([]byte(nil), prefijoDigestInfoSHA256...), otro[:]...)}
	if _, _, err := p.Firmar(context.Background(), cs[0], prep, "1234"); !errors.Is(err, ErrPreparadaIncoherente) {
		t.Errorf("err = %v", err)
	}
}

func TestSinLaClaveNoSeFirma(t *testing.T) {
	clave, _ := rsa.GenerateKey(rand.Reader, 2048)
	p, cert, _ := tarjetaDePrueba(t, clave)
	falso := Certificado{Cadena: []*x509.Certificate{cert}, PidePIN: true, ref: refPKCS11{ranura: 0, id: []byte{0x99}}}
	cs, _ := p.Certificados(context.Background())
	if len(cs) == 1 {
		falso.ref = refPKCS11{ranura: cs[0].ref.(refPKCS11).ranura, id: []byte{0x99}}
	}
	resumen := sha256.Sum256([]byte("x"))
	prep := api.PDFFirmaPreparada{Resumen: resumen[:], DigestInfo: append(append([]byte(nil), prefijoDigestInfoSHA256...), resumen[:]...)}
	if _, _, err := p.Firmar(context.Background(), falso, prep, "1234"); !errors.Is(err, ErrNoEsta) {
		t.Errorf("err = %v", err)
	}
}

func emitir(t *testing.T, emisor *x509.Certificate, claveEmisor crypto.Signer, cn string, esCA bool, clave crypto.Signer) (*x509.Certificate, crypto.Signer) {
	t.Helper()
	if clave == nil {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		clave = k
	}
	p := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageContentCommitment,
	}
	if esCA {
		p.IsCA, p.BasicConstraintsValid, p.KeyUsage = true, true, x509.KeyUsageCertSign
	}
	if emisor == nil {
		emisor, claveEmisor = p, clave
	}
	der, err := x509.CreateCertificate(rand.Reader, p, emisor, clave.Public(), claveEmisor)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := x509.ParseCertificate(der)
	return c, clave
}

func escribir(t *testing.T, ruta string, datos []byte) string {
	t.Helper()
	if err := os.WriteFile(ruta, datos, 0o600); err != nil {
		t.Fatal(err)
	}
	return ruta
}

func correr(t *testing.T, orden string, args ...string) {
	t.Helper()
	if out, err := exec.Command(orden, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", orden, args, err, out)
	}
}
