// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package verifactu

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// Lo que entrega la FNMT es un `.p12`, y hasta ahora el servicio solo aceptaba
// dos PEM: quien no sabía usar OpenSSL no podía activar VERI*FACTU. Los
// `testdata/` son certificados sintéticos (autofirmados, NIF VATES-B89890001,
// contraseña «secreta») en los dos envoltorios que circulan.
func TestPEMDeFicheroAceptaLosP12QueEntreganLosEmisores(t *testing.T) {
	for _, nombre := range []string{"moderno.p12", "clasico.p12"} {
		t.Run(nombre, func(t *testing.T) {
			datos, err := os.ReadFile("testdata/" + nombre)
			if err != nil {
				t.Fatal(err)
			}
			certPEM, keyPEM, err := PEMDeFichero(datos, nil, "secreta")
			if err != nil {
				t.Fatalf("PEMDeFichero: %v", err)
			}
			// Lo que sale tiene que ser lo que guarda el store y usa el cliente.
			nif, err := ValidarCertificado(certPEM, keyPEM, time.Now())
			if err != nil || nif != "B89890001" {
				t.Fatalf("ValidarCertificado: nif=%q err=%v", nif, err)
			}
			if _, err := NuevoCliente(certPEM, keyPEM, false, false); err != nil {
				t.Fatalf("NuevoCliente: %v", err)
			}
		})
	}
}

func TestPEMDeFicheroDistingueLaContrasena(t *testing.T) {
	datos, err := os.ReadFile("testdata/moderno.p12")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = PEMDeFichero(datos, nil, "no-es-esta")
	if !errors.Is(err, ErrContrasenaCertificado) {
		t.Fatalf("err=%v, se esperaba ErrContrasenaCertificado", err)
	}
	if strings.Contains(err.Error(), "no-es-esta") {
		t.Fatal("el error lleva la contraseña")
	}
	if _, _, err := PEMDeFichero([]byte("ni p12 ni pem"), nil, ""); !errors.Is(err, ErrFicheroCertificado) {
		t.Fatalf("basura: err=%v", err)
	}
}

func TestPEMDeFicheroAceptaPEMJuntoOSeparado(t *testing.T) {
	ahora := time.Now().UTC()
	certPEM, keyPEM := emitirCertPrueba(t, "VATES-B12345678", "", false, ahora.Add(-time.Hour), ahora.Add(time.Hour))

	// Un solo fichero con las dos cosas, en cualquier orden.
	junto := append(append([]byte{}, keyPEM...), certPEM...)
	c, k, err := PEMDeFichero(junto, nil, "")
	if err != nil {
		t.Fatalf("junto: %v", err)
	}
	if nif, err := ValidarCertificado(c, k, ahora); err != nil || nif != "B12345678" {
		t.Fatalf("junto: nif=%q err=%v", nif, err)
	}

	// Certificado y clave en dos ficheros.
	if c, k, err = PEMDeFichero(certPEM, keyPEM, ""); err != nil || !bytes.Equal(c, certPEM) || !bytes.Equal(k, keyPEM) {
		t.Fatalf("separados: err=%v", err)
	}

	// El certificado sin su clave no sirve para presentar.
	if _, _, err := PEMDeFichero(certPEM, nil, ""); !errors.Is(err, ErrFicheroCertificado) {
		t.Fatalf("sin clave: err=%v", err)
	}
	cifrada := []byte("-----BEGIN ENCRYPTED PRIVATE KEY-----\nAAAA\n-----END ENCRYPTED PRIVATE KEY-----\n")
	if _, _, err := PEMDeFichero(certPEM, cifrada, ""); !errors.Is(err, ErrFicheroCertificado) {
		t.Fatalf("clave cifrada: err=%v", err)
	}
}
