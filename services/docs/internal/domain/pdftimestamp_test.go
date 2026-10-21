// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del sello de tiempo. La autoridad es falsa pero el PROTOCOLO es de
// verdad: se decodifica la petición con `encoding/asn1` y se responde una
// `TimeStampResp` bien formada, así que lo que se prueba es la codificación real
// y no un acuerdo entre dos mentiras.
package domain

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// autoridadFalsa levanta un servidor que habla RFC 3161. `estado` es el
// PKIStatus que devuelve y `token` los bytes del token; con token nil se emite
// uno reconocible.
func autoridadFalsa(t *testing.T, estado int, token []byte, verPeticion func(peticionDeSello)) *httptest.Server {
	t.Helper()
	// El token se genera a partir de la huella que llegue en la petición: la
	// verificación de `comprobarToken` exige que CUBRA lo que se pidió, así que un
	// token de adorno no cuela — y eso es justo lo que se quiere.
	generar := token == nil
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/timestamp-query" {
			t.Errorf("Content-Type = %q, quería application/timestamp-query", ct)
		}
		cuerpo := make([]byte, 4096)
		n, _ := r.Body.Read(cuerpo)
		var pet peticionDeSello
		if _, err := asn1.Unmarshal(cuerpo[:n], &pet); err != nil {
			t.Errorf("la petición no es un TimeStampReq válido: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if verPeticion != nil {
			verPeticion(pet)
		}
		if generar {
			token = tokenConHuella(t, pet.HuellaMensaje.Huella, pet.Nonce)
		}
		resp := respuestaDeSello{
			Estado: estadoPKI{Estado: estado},
			Token:  asn1.RawValue{FullBytes: token},
		}
		datos, err := asn1.Marshal(resp)
		if err != nil {
			t.Errorf("no se pudo codificar la respuesta: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/timestamp-reply")
		_, _ = w.Write(datos)
	}))
}

func TestPedirSelloIdaYVuelta(t *testing.T) {
	resumen := sha256.Sum256([]byte("una firma cualquiera"))
	var vista peticionDeSello
	srv := autoridadFalsa(t, 0, nil, func(p peticionDeSello) { vista = p })
	defer srv.Close()

	token, err := PedirSello(context.Background(), resumen[:], OpcionesSello{URL: srv.URL})
	if err != nil {
		t.Fatalf("PedirSello: %v", err)
	}
	if len(token) == 0 {
		t.Fatal("el token vino vacío")
	}

	if vista.Version != 1 {
		t.Errorf("versión = %d, quería 1", vista.Version)
	}
	if !bytes.Equal(vista.HuellaMensaje.Huella, resumen[:]) {
		t.Error("la huella enviada no es la que se pidió sellar")
	}
	if !vista.HuellaMensaje.Algoritmo.Algorithm.Equal(oidSHA256) {
		t.Errorf("algoritmo = %v, quería SHA-256", vista.HuellaMensaje.Algoritmo.Algorithm)
	}
	// El certificado se pide para que el token se sostenga solo dentro del PDF.
	if !vista.PedirCertificado {
		t.Error("no se pidió el certificado de la autoridad")
	}
	// El nonce ata la respuesta a ESTA petición.
	if vista.Nonce == nil || vista.Nonce.Sign() == 0 {
		t.Error("falta el nonce: sin él vale un token viejo de otra firma")
	}
}

// TestPedirSelloRechazadoSeDistingue: un estado que no es 0 ni 1 es un NO, y hay
// que distinguirlo de un fallo de red. Aceptar un token pegado detrás de un
// rechazo sería un sello que no vale nada.
func TestPedirSelloRechazadoSeDistingue(t *testing.T) {
	resumen := sha256.Sum256([]byte("x"))
	for _, estado := range []int{2, 3, 5} {
		srv := autoridadFalsa(t, estado, nil, nil)
		_, err := PedirSello(context.Background(), resumen[:], OpcionesSello{URL: srv.URL})
		srv.Close()
		if !errors.Is(err, ErrSelloRechazado) {
			t.Errorf("estado %d: err = %v, quería ErrSelloRechazado", estado, err)
		}
	}
}

func TestPedirSelloSinAutoridad(t *testing.T) {
	resumen := sha256.Sum256([]byte("x"))
	_, err := PedirSello(context.Background(), resumen[:], OpcionesSello{})
	if !errors.Is(err, ErrSelloNoConfigurado) {
		t.Errorf("err = %v, quería ErrSelloNoConfigurado", err)
	}
}

// TestPedirSelloConservaLosBytesDelToken es el check que evita el fallo más
// silencioso de todos: el token va FIRMADO por la autoridad, así que
// recodificarlo cambia sus bytes y su propia firma deja de verificar. El PDF
// abriría bien y el sello sería basura.
func TestPedirSelloConservaLosBytesDelToken(t *testing.T) {
	// Un token VÁLIDO —tiene que pasar la verificación— para poder compararlo
	// byte a byte a la salida.
	resumen := sha256.Sum256([]byte("x"))
	original := tokenConHuella(t, resumen[:], nil)
	srv := autoridadFalsa(t, 0, original, nil)
	defer srv.Close()
	token, err := PedirSello(context.Background(), resumen[:], OpcionesSello{URL: srv.URL})
	if err != nil {
		t.Fatalf("PedirSello: %v", err)
	}
	if !bytes.Equal(token, original) {
		t.Error("los bytes del token cambiaron: su firma dejaría de verificar")
	}
}

// TestPedirSelloRespetaElPlazo: una autoridad colgada no puede colgar una firma.
func TestPedirSelloRespetaElPlazo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	resumen := sha256.Sum256([]byte("x"))
	inicio := time.Now()
	_, err := PedirSello(context.Background(), resumen[:], OpcionesSello{
		URL: srv.URL, Timeout: 200 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("debía agotarse el plazo")
	}
	if pasado := time.Since(inicio); pasado > time.Second {
		t.Errorf("tardó %v: el plazo no se respetó", pasado)
	}
}

// TestFirmaConSelloLoIncrusta comprueba el cableado completo: el token tiene que
// acabar dentro del PKCS#7 de la firma, como atributo NO firmado.
func TestFirmaConSelloLoIncrusta(t *testing.T) {
	cert, clave := identidadDePrueba(t)
	srv := autoridadFalsa(t, 0, nil, nil)
	defer srv.Close()

	w := pdfsimple.Nuevo()
	w.Texto("documento a firmar", "F1", 12)
	doc, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}

	var pedido int
	firmado, err := FirmarConCertificadoPDF(doc, OpcionesFirma{
		CertificadoPEM: cert,
		ClavePEM:       clave,
		Nombre:         "Quien firma",
		Fecha:          time.Now(),
		Sello: func(resumen []byte) ([]byte, error) {
			pedido++
			if len(resumen) != sha256.Size {
				t.Errorf("el resumen a sellar mide %d, debía ser SHA-256", len(resumen))
			}
			return PedirSello(context.Background(), resumen, OpcionesSello{URL: srv.URL})
		},
	})
	if err != nil {
		t.Fatalf("FirmarConCertificadoPDF: %v", err)
	}
	if pedido != 1 {
		t.Errorf("se pidió el sello %d veces, quería 1", pedido)
	}
	// El OID del atributo de sello tiene que aparecer en el fichero: es lo que
	// busca un validador.
	oidDER, err := asn1.Marshal(oidAtributoSelloFirma)
	if err != nil {
		t.Fatalf("marshal del OID: %v", err)
	}
	if !bytes.Contains(hexADatos(t, firmado), oidDER) {
		t.Error("el token de sello no llegó al PKCS#7 del documento")
	}
}

// TestFirmaSinSelloSigueFirmando es la otra mitad, y la que de verdad importa en
// producción: una autoridad caída NO puede impedir firmar un contrato. La firma
// sale sin sello y la interfaz lo dice.
func TestFirmaSinSelloSigueFirmando(t *testing.T) {
	cert, clave := identidadDePrueba(t)
	w := pdfsimple.Nuevo()
	w.Texto("documento a firmar", "F1", 12)
	doc, err := LeerPDF(w.Bytes())
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}

	firmado, err := FirmarConCertificadoPDF(doc, OpcionesFirma{
		CertificadoPEM: cert,
		ClavePEM:       clave,
		Fecha:          time.Now(),
		Sello: func([]byte) ([]byte, error) {
			return nil, errors.New("la autoridad está caída")
		},
	})
	if err != nil {
		t.Fatalf("una autoridad caída no debe impedir firmar: %v", err)
	}
	if len(firmado) == 0 {
		t.Fatal("no salió documento")
	}
	oidDER, _ := asn1.Marshal(oidAtributoSelloFirma)
	if bytes.Contains(hexADatos(t, firmado), oidDER) {
		t.Error("no debería haber atributo de sello si la autoridad falló")
	}
}

func TestOidDesdeTexto(t *testing.T) {
	casos := map[string]bool{
		"1.2.840.113549.1.9.16.2.14": true,
		"1.2":                        true,
		"1":                          false, // un OID necesita al menos dos arcos
		"":                           false,
		"1..2":                       false,
		"1.a.2":                      false,
		"1.2.":                       false,
	}
	for texto, vale := range casos {
		_, err := oidDesdeTexto(texto)
		if (err == nil) != vale {
			t.Errorf("oidDesdeTexto(%q): err = %v, esperaba vale=%v", texto, err, vale)
		}
	}
}

// hexADatos saca del PDF firmado los bytes del PKCS#7, que van en hexadecimal
// dentro de /Contents.
func hexADatos(t *testing.T, pdf []byte) []byte {
	t.Helper()
	i := bytes.Index(pdf, []byte("/Contents <"))
	if i < 0 {
		t.Fatal("no se encontró /Contents en el documento firmado")
	}
	j := bytes.IndexByte(pdf[i:], '>')
	if j < 0 {
		t.Fatal("el /Contents no está cerrado")
	}
	return decodificarHexASCII(pdf[i+len("/Contents <") : i+j])
}

// --- Verificación del token --------------------------------------------------

// tokenConHuella arma un token de sello bien formado que sella la huella y el
// nonce dados. Es la única forma de probar la verificación sin depender de una
// autoridad de verdad.
func tokenConHuella(t *testing.T, huella []byte, nonce *big.Int) []byte {
	t.Helper()
	return tokenConAlgoritmo(t, oidSHA256, huella, nonce)
}

func tokenConAlgoritmo(t *testing.T, alg asn1.ObjectIdentifier, huella []byte, nonce *big.Int) []byte {
	t.Helper()
	info := infoDelSello{
		Version:  1,
		Politica: asn1.ObjectIdentifier{1, 2, 3, 4},
		HuellaMensaje: huellaDeMensaje{
			Algoritmo: pkix.AlgorithmIdentifier{Algorithm: alg, Parameters: asn1.NullRawValue},
			Huella:    huella,
		},
		Serie:    big.NewInt(1),
		Generado: time.Now().UTC().Truncate(time.Second),
		Nonce:    nonce,
	}
	infoDER, err := asn1.Marshal(info)
	if err != nil {
		t.Fatalf("marshal TSTInfo: %v", err)
	}
	// El TSTInfo va DENTRO de un OCTET STRING dentro del eContent.
	sobre, err := asn1.Marshal(infoDER)
	if err != nil {
		t.Fatalf("marshal del sobre: %v", err)
	}
	firmado := tokenFirmado{
		Version:           3,
		AlgoritmosResumen: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true},
		Contenido: contenidoConDatos{
			Tipo:  oidTokenSello,
			Datos: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sobre},
		},
		Firmantes: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSet, IsCompound: true},
	}
	cuerpo, err := asn1.Marshal(firmado)
	if err != nil {
		t.Fatalf("marshal SignedData: %v", err)
	}
	token, err := asn1.Marshal(contenidoInfo{
		Tipo: oidFirmadoPKCS7,
		Contenido: asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: cuerpo,
		},
	})
	if err != nil {
		t.Fatalf("marshal ContentInfo: %v", err)
	}
	return token
}

// TestTokenDeOtraHuellaSeRechaza es EL check de esta pieza. Las autoridades de
// sellado hablan HTTP plano, así que un intermediario puede devolver un token
// auténtico y válido... de otra huella. Sin rechazarlo, el PDF se guardaría con un
// sello que no cubre su firma y nadie se enteraría hasta el día en que hiciera
// falta la prueba.
func TestTokenDeOtraHuellaSeRechaza(t *testing.T) {
	mio := sha256.Sum256([]byte("mi firma"))
	ajeno := sha256.Sum256([]byte("la firma de otro"))
	nonce := big.NewInt(42)

	token := tokenConHuella(t, ajeno[:], nonce)
	srv := autoridadFalsa(t, 0, token, nil)
	defer srv.Close()

	_, err := PedirSello(context.Background(), mio[:], OpcionesSello{URL: srv.URL})
	if !errors.Is(err, ErrSelloRechazado) {
		t.Fatalf("err = %v, quería ErrSelloRechazado: un token de otra huella no vale", err)
	}
}

// TestTokenDeLaHuellaCorrectaSeAcepta es la otra mitad: la comprobación no puede
// rechazar lo bueno.
func TestTokenDeLaHuellaCorrectaSeAcepta(t *testing.T) {
	mio := sha256.Sum256([]byte("mi firma"))
	// El nonce lo genera `PedirSello` y no se conoce aquí, así que el token se hace
	// SIN nonce: la comprobación solo lo compara cuando el token lo trae, que es
	// lo que dice RFC 3161 (es opcional en la respuesta).
	token := tokenConHuella(t, mio[:], nil)
	srv := autoridadFalsa(t, 0, token, nil)
	defer srv.Close()

	got, err := PedirSello(context.Background(), mio[:], OpcionesSello{URL: srv.URL})
	if err != nil {
		t.Fatalf("PedirSello: %v", err)
	}
	if !bytes.Equal(got, token) {
		t.Error("el token devuelto no es el mismo que emitió la autoridad")
	}
}

// TestNonceDistintoSeRechaza: un token de otra petición puede ser auténtico y
// VIEJO, que es justo el ataque que el nonce cierra.
func TestNonceDistintoSeRechaza(t *testing.T) {
	mio := sha256.Sum256([]byte("mi firma"))
	token := tokenConHuella(t, mio[:], big.NewInt(999999))
	if err := comprobarToken(token, mio[:], big.NewInt(1)); !errors.Is(err, ErrSelloRechazado) {
		t.Errorf("err = %v, quería ErrSelloRechazado", err)
	}
	// Con el nonce correcto, pasa.
	if err := comprobarToken(token, mio[:], big.NewInt(999999)); err != nil {
		t.Errorf("con el nonce correcto no debía fallar: %v", err)
	}
}

func TestFechaDelSello(t *testing.T) {
	mio := sha256.Sum256([]byte("x"))
	token := tokenConHuella(t, mio[:], nil)
	fecha, err := FechaDelSello(token)
	if err != nil {
		t.Fatalf("FechaDelSello: %v", err)
	}
	if fecha.IsZero() || time.Since(fecha) > time.Minute {
		t.Errorf("fecha = %v, esperaba algo de ahora mismo", fecha)
	}
}

// TestTokenIlegibleSeRechaza: el token llega de fuera y por HTTP plano.
func TestTokenIlegibleSeRechaza(t *testing.T) {
	mio := sha256.Sum256([]byte("x"))
	for _, basura := range [][]byte{
		{},
		{0x30, 0x00},
		[]byte("no soy ASN.1"),
		bytes.Repeat([]byte{0xFF}, 64),
	} {
		if err := comprobarToken(basura, mio[:], nil); err == nil {
			t.Errorf("se aceptó un token ilegible de %d bytes", len(basura))
		}
	}
}

// TestTokenDeOtroAlgoritmoSeRechaza: la huella buena declarada con otro algoritmo es
// un sello que un validador recalcula distinto y da por no correspondiente.
func TestTokenDeOtroAlgoritmoSeRechaza(t *testing.T) {
	huella := bytes.Repeat([]byte{0x42}, 32)
	sha512 := asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	if err := comprobarToken(tokenConAlgoritmo(t, sha512, huella, nil), huella, nil); !errors.Is(err, ErrSelloRechazado) {
		t.Fatalf("err = %v, quería ErrSelloRechazado", err)
	}
}
