// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"encoding/asn1"
	"errors"
	"testing"
)

// EL LECTOR DE `.p12` NO TENÍA NI UN TEST, y es de lo que más lo necesita en
// todo Sytue PDF: parsea ASN.1 anidado —«una matrioska», dice su propio
// comentario— sobre un fichero que sube QUIEN QUIERE FIRMAR, con una contraseña
// que también llega de fuera. Está marcado `nolint:gocognit,gocyclo` porque es
// grande, y grande + binario + entrada ajena es donde vive el pánico que tumba
// el servicio entero.
//
// Lo que se fija aquí NO es que sepa leer un certificado —para eso hace falta un
// `.p12` real, que no se mete en el repositorio— sino la propiedad que protege:
// ante cualquier entrada, ERROR y no pánico. Un `panic` en este camino no es un
// fallo de una petición: se lleva por delante el proceso que atiende a todos.

// asn1Valido envuelve unos bytes en una SECUENCIA ASN.1 bien formada, para que
// el parser pase del primer `Unmarshal` y llegue a las ramas de dentro. Sin
// esto, todos los casos morirían en la misma línea y no se probaría nada.
func asn1Valido(t *testing.T, v any) []byte {
	t.Helper()
	b, err := asn1.Marshal(v)
	if err != nil {
		t.Fatalf("preparar el caso: %v", err)
	}
	return b
}

func TestLeerPKCS12RechazaSinReventar(t *testing.T) {
	type pfxMinimo struct {
		Version  int
		AuthSafe struct {
			Tipo asn1.ObjectIdentifier
		}
	}
	var conOIDAjeno pfxMinimo
	conOIDAjeno.Version = 3
	conOIDAjeno.AuthSafe.Tipo = asn1.ObjectIdentifier{1, 2, 3, 4}

	casos := []struct {
		nombre string
		datos  []byte
	}{
		{"vacío", nil},
		{"un byte", []byte{0x30}},
		{"cabecera de secuencia sin cuerpo", []byte{0x30, 0x82, 0xff, 0xff}},
		{"texto plano", []byte("esto no es un p12, es una frase")},
		// Longitud declarada ENORME con cuerpo corto: el caso clásico de reserva
		// desmedida o de índice fuera de rango en un parser binario.
		{"longitud mentirosa", []byte{0x30, 0x84, 0x7f, 0xff, 0xff, 0xff, 0x02, 0x01, 0x03}},
		{"solo ceros", make([]byte, 64)},
		{"secuencia vacía", []byte{0x30, 0x00}},
		{"ASN.1 válido pero no es un PKCS#12", asn1Valido(t, []int{1, 2, 3})},
		{"envoltorio con un OID que no es CMS Data", asn1Valido(t, conOIDAjeno)},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			// Sin `recover`: si esto entra en pánico, el test FALLA con su traza, que
			// es exactamente lo que se quiere ver. Envolverlo lo escondería.
			cadena, clave, err := LeerPKCS12(c.datos, "loquesea")
			if err == nil {
				t.Fatal("aceptó una entrada que no es un .p12 válido")
			}
			if !errors.Is(err, ErrPKCS12) {
				t.Errorf("error = %v; quiero que envuelva ErrPKCS12 para que quien llama lo distinga", err)
			}
			if cadena != nil || clave != nil {
				t.Error("devolvió certificado o clave junto a un error")
			}
		})
	}
}

// La contraseña vacía es un caso aparte porque es lo que llega cuando el
// formulario no la pide o alguien la deja en blanco: no puede tomar un camino
// distinto al de una contraseña incorrecta.
func TestLeerPKCS12ConContrasenaVacia(t *testing.T) {
	if _, _, err := LeerPKCS12([]byte{0x30, 0x03, 0x02, 0x01, 0x03}, ""); err == nil {
		t.Fatal("aceptó basura con la contraseña vacía")
	}
}

// FuzzLeerPKCS12 recorre el parser con entradas arbitrarias. En CI corre con el
// corpus semilla como un test normal; con `-fuzz` explora por su cuenta.
//
// La única propiedad que afirma es la que importa: NO ENTRA EN PÁNICO. Cualquier
// combinación de longitudes, anidamientos y algoritmos tiene que salir por un
// error, porque el fichero lo elige quien firma y no nosotros.
func FuzzLeerPKCS12(f *testing.F) {
	f.Add([]byte{}, "")
	f.Add([]byte{0x30, 0x00}, "x")
	f.Add([]byte{0x30, 0x82, 0xff, 0xff}, "contraseña")
	f.Add([]byte("no soy un p12"), "")
	f.Add([]byte{0x30, 0x84, 0x7f, 0xff, 0xff, 0xff, 0x02, 0x01, 0x03}, "1234")

	f.Fuzz(func(t *testing.T, datos []byte, contrasena string) {
		// El resultado da igual; lo que se prueba es que se llegue hasta aquí.
		_, _, _ = LeerPKCS12(datos, contrasena)
	})
}
