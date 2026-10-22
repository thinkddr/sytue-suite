// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks del verificador de firmas con FIXTURES FIRMADOS DE VERDAD, generados en
// cada ejecución: una CA de prueba, firmantes con la vigencia que pide cada caso y
// una autoridad de sellado que firma sus tokens con su propia clave. No hay
// ficheros ni claves en el repo, así que la batería es reproducible en cualquier
// máquina y no caduca.
//
// Cada caso dice qué ataque o qué accidente representa. Los que importan:
// manipular un byte firmado, añadir una revisión que cambia la página, desplazar
// el /ByteRange, un certificado caducado con y sin sello, y un sello que no es de
// esta firma.
package domain

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cms"
)

// autoridadDePrueba es una CA con su clave, para emitir lo que cada caso pida.
type autoridadDePrueba struct {
	ca    *x509.Certificate
	clave crypto.Signer
}

func nuevaAutoridadDePrueba(t *testing.T, nombre string) autoridadDePrueba {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: nombre},
		NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(10 * 365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, p, p, clave.Public(), clave)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(der)
	return autoridadDePrueba{ca: ca, clave: clave}
}

func (a autoridadDePrueba) anclas() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(a.ca)
	return p
}

// emitir firma un certificado de esta CA con la vigencia pedida; `eku` vacío =
// un certificado de firma corriente.
func (a autoridadDePrueba) emitir(t *testing.T, cn string, desde, hasta time.Time, eku ...x509.ExtKeyUsage) identidadSistema {
	t.Helper()
	clave, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serie, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	p := &x509.Certificate{
		SerialNumber: serie, Subject: pkix.Name{CommonName: cn, Organization: []string{"Sytue pruebas"}},
		NotBefore: desde, NotAfter: hasta, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		ExtKeyUsage: eku,
	}
	der, err := x509.CreateCertificate(rand.Reader, p, a.ca, clave.Public(), a.clave)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return identidadSistema{ca: a.ca, cert: cert, clave: clave}
}

// opcionesPEM es la identidad lista para FirmarConCertificadoPDF, cadena incluida.
func opcionesPEM(t *testing.T, id identidadSistema) OpcionesFirma {
	t.Helper()
	clave, err := x509.MarshalPKCS8PrivateKey(id.clave)
	if err != nil {
		t.Fatal(err)
	}
	return OpcionesFirma{
		CertificadoPEM: append(pemDe(id.cert), pemDe(id.ca)...),
		ClavePEM:       pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: clave}),
	}
}

// selloDePrueba emite un token RFC 3161 FIRMADO por la TSA `tsa`, con la hora que
// se le diga: es lo que permite el caso del certificado que caduca DESPUÉS de un
// sello. La estructura es la de la norma, la misma que valida `cms`.
func selloDePrueba(t *testing.T, tsa identidadSistema, huella []byte, cuando time.Time) []byte {
	t.Helper()
	info, err := asn1.Marshal(infoDelSello{
		Version: 1, Politica: asn1.ObjectIdentifier{1, 2, 3, 4},
		HuellaMensaje: huellaDeMensaje{
			Algoritmo: pkix.AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.NullRawValue},
			Huella:    huella,
		},
		Serie: big.NewInt(1), Generado: cuando.UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	sobre, _ := asn1.Marshal(info)
	tipo, _ := asn1.Marshal(oidTokenSello)
	resumen := sha256.Sum256(info)
	resumenDER, _ := asn1.Marshal(resumen[:])
	certDER, err := certificadoFirmanteDER(tsa.cert)
	if err != nil {
		t.Fatal(err)
	}
	atributos, err := ordenarComoDER([]atributo{
		{Tipo: oidTipoContenido, Valores: []asn1.RawValue{{FullBytes: tipo}}},
		{Tipo: oidResumen, Valores: []asn1.RawValue{{FullBytes: resumenDER}}},
		{Tipo: oidCertificadoFirmante, Valores: []asn1.RawValue{{FullBytes: certDER}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	porFirmar, _ := derDeAtributos(atributos)
	h := sha256.Sum256(porFirmar)
	firma, err := tsa.clave.Sign(rand.Reader, h[:], crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	alg, _ := algoritmoDeFirma(tsa.cert.PublicKey)
	cuerpo, err := asn1.Marshal(struct {
		Version   int
		Algs      []pkix.AlgorithmIdentifier `asn1:"set"`
		Contenido contenidoConDatos
		Certs     asn1.RawValue  `asn1:"optional,tag:0"`
		Firmantes []infoFirmante `asn1:"set"`
	}{
		Version: 3,
		Algs:    []pkix.AlgorithmIdentifier{{Algorithm: oidSHA256, Parameters: asn1.NullRawValue}},
		Contenido: contenidoConDatos{Tipo: oidTokenSello, Datos: asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sobre,
		}},
		Certs: asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true,
			Bytes: append(append([]byte(nil), tsa.cert.Raw...), tsa.ca.Raw...),
		},
		Firmantes: []infoFirmante{{
			Version:           1,
			Emisor:            emisorYSerie{Emisor: asn1.RawValue{FullBytes: tsa.cert.RawIssuer}, Serie: tsa.cert.SerialNumber},
			AlgoritmoResumen:  pkix.AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.NullRawValue},
			AtributosFirmados: atributos,
			AlgoritmoFirma:    alg,
			Firma:             firma,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := asn1.Marshal(contenidoInfo{Tipo: oidFirmadoPKCS7, Contenido: asn1.RawValue{
		Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: cuerpo,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// escenario reúne la CA de los firmantes, la de la TSA y la TSA.
type escenario struct {
	firmantes, sellos autoridadDePrueba
	tsa               identidadSistema
}

func nuevoEscenario(t *testing.T) escenario {
	t.Helper()
	sellos := nuevaAutoridadDePrueba(t, "CA de sellos de prueba")
	return escenario{
		firmantes: nuevaAutoridadDePrueba(t, "CA de firmantes de prueba"),
		sellos:    sellos,
		tsa: sellos.emitir(t, "TSA de prueba", time.Now().Add(-time.Hour), time.Now().Add(5*365*24*time.Hour),
			x509.ExtKeyUsageTimeStamping),
	}
}

func (e escenario) opciones(ahora time.Time) OpcionesVerificacion {
	return OpcionesVerificacion{Anclas: e.firmantes.anclas(), AnclasSello: e.sellos.anclas(), Ahora: ahora}
}

// sello es la función que se pasa a la firma: sella con la TSA a la hora `cuando`.
func (e escenario) sello(t *testing.T, cuando time.Time) pedirSelloFn {
	return func(h []byte) ([]byte, error) { return selloDePrueba(t, e.tsa, h, cuando), nil }
}

// firmar firma `doc` con `id` anexando (que es lo que hace el handler).
func firmarPara(t *testing.T, doc *PDFDoc, id identidadSistema, sello pedirSelloFn, ajustar ...func(*OpcionesFirma)) []byte {
	t.Helper()
	o := opcionesPEM(t, id)
	o.Sello = sello
	o.Rect = [4]float64{80, 80, 300, 150}
	for _, f := range ajustar {
		f(&o)
	}
	out, err := FirmarIncrementalPDF(doc, o)
	if err != nil {
		t.Fatalf("firmar: %v", err)
	}
	return out
}

func verificar(t *testing.T, pdf []byte, opts OpcionesVerificacion) []FirmaVerificada {
	t.Helper()
	d, err := LeerPDF(pdf)
	if err != nil {
		t.Fatalf("LeerPDF: %v", err)
	}
	return d.VerificarFirmas(opts)
}

func unaFirma(t *testing.T, fs []FirmaVerificada) FirmaVerificada {
	t.Helper()
	if len(fs) != 1 {
		t.Fatalf("firmas = %d, quería 1: %+v", len(fs), fs)
	}
	return fs[0]
}

func reabrir(t *testing.T, pdf []byte) *PDFDoc {
	t.Helper()
	d, err := LeerPDF(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// --- La batería ----------------------------------------------------------------

func TestVerificarFirmaValidaConCadenaYSello(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(365*24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "contrato de arrendamiento"), ana, e.sello(t, time.Now()))

	f := unaFirma(t, verificar(t, pdf, e.opciones(time.Time{})))
	if f.Veredicto != VeredictoValida || len(f.Motivos) != 0 {
		t.Fatalf("veredicto %q, motivos %v", f.Veredicto, f.Motivos)
	}
	if !f.Integra || f.Confianza != ConfianzaConfiable || f.Tipo != "pades" {
		t.Errorf("íntegra %v, confianza %q, tipo %q", f.Integra, f.Confianza, f.Tipo)
	}
	if f.Firmante == nil || f.Firmante.Sujeto != "Ana Pérez" || f.Firmante.Emisor != "CA de firmantes de prueba" {
		t.Errorf("firmante = %+v", f.Firmante)
	}
	if len(f.Cadena) != 2 || f.Cadena[1].Sujeto != "CA de firmantes de prueba" {
		t.Errorf("cadena = %+v", f.Cadena)
	}
	if f.Sello == nil || !f.Sello.Integro || f.Sello.Confianza != ConfianzaConfiable || f.Sello.Autoridad != "TSA de prueba" {
		t.Errorf("sello = %+v", f.Sello)
	}
	if f.Cambios.Alcance != AlcanceNinguno || !f.CubreTodo || f.Cambios.BytesFirmados != len(pdf) {
		t.Errorf("cambios = %+v, cubre todo %v", f.Cambios, f.CubreTodo)
	}
}

// Sin anclas no se finge confianza: íntegra, pero indeterminada.
func TestVerificarSinAnclasEsIndeterminada(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, nil)

	f := unaFirma(t, verificar(t, pdf, OpcionesVerificacion{}))
	if !f.Integra || f.Veredicto != VeredictoIndeterminada || f.Confianza != ConfianzaSinAnclas {
		t.Fatalf("íntegra %v, veredicto %q, confianza %q", f.Integra, f.Veredicto, f.Confianza)
	}
	if !slices.Contains(f.Motivos, MotivoSinAnclas) || f.Sello != nil {
		t.Errorf("motivos %v, sello %+v", f.Motivos, f.Sello)
	}
	if len(f.Cadena) != 2 {
		t.Errorf("sin anclas la cadena se enseña igual, con lo que trae la firma: %+v", f.Cadena)
	}
	// Una CA que no es la del firmante tampoco vale.
	otra := nuevaAutoridadDePrueba(t, "Otra CA")
	f = unaFirma(t, verificar(t, pdf, OpcionesVerificacion{Anclas: otra.anclas()}))
	if f.Confianza != ConfianzaNoConfiable || !slices.Contains(f.Motivos, MotivoNoConfiable) {
		t.Errorf("con anclas ajenas: confianza %q, motivos %v", f.Confianza, f.Motivos)
	}
}

// EL check: un byte cambiado en la parte firmada.
func TestVerificarManipuladoEsInvalida(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "importe: 1000 euros"), ana, nil)
	i := bytes.Index(pdf, []byte("1000"))
	if i < 0 {
		t.Fatal("el texto no está en claro: el check necesita verlo")
	}
	tocado := append([]byte(nil), pdf...)
	copy(tocado[i:], "9000")

	f := unaFirma(t, verificar(t, tocado, e.opciones(time.Time{})))
	if f.Integra || f.Veredicto != VeredictoInvalida || !slices.Contains(f.Motivos, MotivoIntegridad) {
		t.Fatalf("íntegra %v, veredicto %q, motivos %v", f.Integra, f.Veredicto, f.Motivos)
	}
}

// El ataque de la revisión incremental: la firma sigue cuadrando sobre SU tramo,
// pero detrás alguien reescribió la página. El lector enseña lo nuevo.
func TestVerificarRevisionQueCambiaLaPaginaEsModificada(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "importe: 1000 euros"), ana, e.sello(t, time.Now()))

	d := reabrir(t, pdf)
	contenido, ok := d.paginas[0].dict["Contents"].(pdfRef)
	if !ok {
		t.Fatal("la página de prueba no tiene un flujo de contenido indirecto")
	}
	xref, err := offsetDelUltimoXref(d.buf)
	if err != nil {
		t.Fatal(err)
	}
	raiz, _ := d.trailer["Root"].(pdfRef)
	manipulado, err := escribirAnexo(d, map[int]objetoSalida{contenido.Num: {
		valor: pdfDict{}, flujo: []byte("BT /F1 12 Tf 72 720 Td (importe: 9000 euros) Tj ET"), esFlujo: true,
	}}, d.maxObjeto()+1, raiz, xref, nil)
	if err != nil {
		t.Fatal(err)
	}

	f := unaFirma(t, verificar(t, manipulado, e.opciones(time.Time{})))
	if !f.Integra {
		t.Fatalf("la firma tiene que seguir cuadrando sobre su tramo: %v", f.Motivos)
	}
	if f.Veredicto != VeredictoModificada || f.Cambios.Alcance != AlcanceOtros || !slices.Equal(f.Cambios.Paginas, []int{1}) {
		t.Fatalf("veredicto %q, cambios %+v", f.Veredicto, f.Cambios)
	}
	if f.CubreTodo || f.Cambios.Revisiones != 1 || f.Cambios.BytesFirmados != len(pdf) {
		t.Errorf("cubre todo %v, cambios %+v", f.CubreTodo, f.Cambios)
	}
}

// Un contrato que firman dos: la primera firma ve DESPUÉS otra firma, y eso no es
// manipularlo.
func TestVerificarSegundaFirmaNoEsUnCambio(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	luis := e.firmantes.emitir(t, "Luis Gil", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	primera := firmarPara(t, docDePrueba(t, "contrato"), ana, e.sello(t, time.Now()))
	segunda := firmarPara(t, reabrir(t, primera), luis, e.sello(t, time.Now()))

	fs := verificar(t, segunda, e.opciones(time.Time{}))
	if len(fs) != 2 {
		t.Fatalf("firmas = %d", len(fs))
	}
	if fs[0].Veredicto != VeredictoValida || fs[0].Cambios.Alcance != AlcancePermitidos || fs[0].Cambios.Revisiones != 1 {
		t.Errorf("primera: veredicto %q, motivos %v, cambios %+v", fs[0].Veredicto, fs[0].Motivos, fs[0].Cambios)
	}
	if fs[1].Veredicto != VeredictoValida || fs[1].Cambios.Alcance != AlcanceNinguno || fs[1].Firmante.Sujeto != "Luis Gil" {
		t.Errorf("segunda: veredicto %q, motivos %v, cambios %+v", fs[1].Veredicto, fs[1].Motivos, fs[1].Cambios)
	}
}

// Certificar (/P 1) es «después, nada»: ni siquiera otra firma.
func TestVerificarCertificacionRotaPorOtraFirma(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	certificado := firmarPara(t, docDePrueba(t, "acta"), ana, nil, func(o *OpcionesFirma) { o.Bloquear = true })
	f := unaFirma(t, verificar(t, certificado, e.opciones(time.Time{})))
	if f.Certificacion != 1 || f.Veredicto != VeredictoValida {
		t.Fatalf("certificación %d, veredicto %q, motivos %v", f.Certificacion, f.Veredicto, f.Motivos)
	}
	// Certificar un documento que ya está firmado no se puede, y se dice ANTES:
	// caer a reescribirlo borraría la firma que tiene.
	o := opcionesPEM(t, ana)
	o.Bloquear = true
	if _, err := FirmarIncrementalPDF(reabrir(t, certificado), o); !errors.Is(err, ErrCertificarFirmado) {
		t.Errorf("certificar sobre una firma: err = %v", err)
	}
	encima := firmarPara(t, reabrir(t, certificado), ana, nil)
	fs := verificar(t, encima, e.opciones(time.Time{}))
	if fs[0].Veredicto != VeredictoInvalida || !slices.Contains(fs[0].Motivos, MotivoCertificacion) {
		t.Errorf("la firma certificadora con otra encima: veredicto %q, motivos %v", fs[0].Veredicto, fs[0].Motivos)
	}
}

// Caducado sin sello: no hay forma de saber si estaba vigente al firmar.
func TestVerificarCertificadoCaducadoSinSello(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-48*time.Hour), time.Now().Add(-24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, nil)

	f := unaFirma(t, verificar(t, pdf, e.opciones(time.Time{})))
	if !f.Integra || f.Veredicto != VeredictoIndeterminada || !slices.Contains(f.Motivos, MotivoCaducado) {
		t.Fatalf("íntegra %v, veredicto %q, motivos %v", f.Integra, f.Veredicto, f.Motivos)
	}
	// La cadena sí llega a la CA: caducado y desconocido son dos cosas distintas.
	if f.Confianza != ConfianzaConfiable {
		t.Errorf("confianza = %q", f.Confianza)
	}
}

// Caducado HOY, sellado cuando estaba vigente: la firma vale. Es para lo que
// existe el sello.
func TestVerificarCaducadoDespuesDelSelloEsValida(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, e.sello(t, time.Now()))

	dentroDeUnAno := time.Now().Add(365 * 24 * time.Hour)
	f := unaFirma(t, verificar(t, pdf, e.opciones(dentroDeUnAno)))
	if f.Veredicto != VeredictoValida {
		t.Fatalf("veredicto %q, motivos %v, sello %+v", f.Veredicto, f.Motivos, f.Sello)
	}
	if f.HoraReferencia != f.Sello.Fecha {
		t.Errorf("la hora de referencia (%s) tiene que ser la del sello (%s)", f.HoraReferencia, f.Sello.Fecha)
	}
	// El mismo fichero, sin anclas para la TSA: el sello no prueba la hora y la
	// firma ya no se sostiene.
	f = unaFirma(t, verificar(t, pdf, OpcionesVerificacion{Anclas: e.firmantes.anclas(), Ahora: dentroDeUnAno}))
	if f.Veredicto != VeredictoIndeterminada || !slices.Contains(f.Motivos, MotivoCaducado) ||
		!slices.Contains(f.Motivos, MotivoSelloNoConfiable) {
		t.Errorf("sin anclas de sello: veredicto %q, motivos %v", f.Veredicto, f.Motivos)
	}
}

// Un sello auténtico... de otra firma. El atacante que lo cuela quiere la fecha.
func TestVerificarSelloDeOtraFirmaNoVale(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	ajena := sha256.Sum256([]byte("otra firma"))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, func([]byte) ([]byte, error) {
		return selloDePrueba(t, e.tsa, ajena[:], time.Now()), nil
	})

	f := unaFirma(t, verificar(t, pdf, e.opciones(time.Time{})))
	if f.Sello == nil || f.Sello.Integro || !slices.Contains(f.Motivos, MotivoSelloInvalido) {
		t.Fatalf("sello %+v, motivos %v", f.Sello, f.Motivos)
	}
	if !f.Integra || f.Veredicto != VeredictoIndeterminada {
		t.Errorf("la firma sigue íntegra y queda indeterminada: %v %q", f.Integra, f.Veredicto)
	}
}

// El /ByteRange movido un byte: el hueco ya no es solo la cadena de la firma, así
// que ese byte no estaría firmado. Se cambia SIN mover el resto del fichero.
func TestVerificarByteRangeDesplazadoEsInvalida(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, nil)
	d := reabrir(t, pdf)
	c := d.camposDeFirma()[0]
	r, err := d.rangoDeFirma(c)
	if err != nil {
		t.Fatal(err)
	}
	tocado := append([]byte(nil), pdf...)
	if err := escribirByteRange(tocado[bytes.LastIndex(tocado, []byte("/ByteRange [")):],
		[4]int{0, r.hueco0 - 1, r.hueco1, r.fin - r.hueco1}); err != nil {
		t.Fatal(err)
	}
	f := unaFirma(t, verificar(t, tocado, e.opciones(time.Time{})))
	if f.Veredicto != VeredictoInvalida || !slices.Contains(f.Motivos, MotivoRango) {
		t.Fatalf("veredicto %q, motivos %v", f.Veredicto, f.Motivos)
	}
}

// El relleno de ceros del hueco NO está firmado. Si el verificador aceptara en
// él algo que no sea hexadecimal, se podría cerrar la cadena antes de tiempo e
// inyectar claves en el diccionario de la firma —otro /Reason, otra /M— sin
// tocar un solo byte de lo firmado: el CMS sigue cuadrando.
func TestVerificarInyeccionEnElRellenoEsInvalida(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, nil)
	d := reabrir(t, pdf)
	r, err := d.rangoDeFirma(d.camposDeFirma()[0])
	if err != nil {
		t.Fatal(err)
	}
	// Justo detrás del CMS, dentro del relleno: «>» cierra la cadena y lo que
	// sigue son claves nuevas; «/Relleno <» vuelve a abrir una para tragarse el
	// resto de ceros hasta el «>» original.
	inicio := r.hueco0 + 1 + 2*len(r.der)
	tocado := append([]byte(nil), pdf...)
	copy(tocado[inicio:], "> /Reason (Conforme con TODO) /Relleno <")
	if f := unaFirma(t, verificar(t, tocado, e.opciones(time.Time{}))); f.Veredicto != VeredictoInvalida ||
		!slices.Contains(f.Motivos, MotivoRango) {
		t.Fatalf("veredicto %q, motivos %v, motivo leído %q", f.Veredicto, f.Motivos, f.Motivo)
	}
}

// Una revisión que no toca ningún objeto que ya existía, pero apunta el /Root a
// un catálogo NUEVO: todo lo que se ve sale de ahí, así que es un cambio aunque
// ningún objeto viejo se haya redefinido.
func TestVerificarCatalogoSustituidoEsModificada(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, nil)
	d := reabrir(t, pdf)
	xref, err := offsetDelUltimoXref(d.buf)
	if err != nil {
		t.Fatal(err)
	}
	raiz, _ := d.trailer["Root"].(pdfRef)
	viejo, _ := d.resolver(raiz).(pdfDict)
	nuevo := copiaLlana(viejo)
	nuevo["OpenAction"] = pdfArray{pdfRef{Num: d.paginas[0].num}, pdfName("Fit")}
	num := d.maxObjeto() + 1
	manipulado, err := escribirAnexo(d, map[int]objetoSalida{num: {valor: nuevo}}, num+1, pdfRef{Num: num}, xref, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := unaFirma(t, verificar(t, manipulado, e.opciones(time.Time{})))
	if f.Veredicto != VeredictoModificada || f.Cambios.Alcance != AlcanceOtros {
		t.Fatalf("veredicto %q, cambios %+v", f.Veredicto, f.Cambios)
	}
}

// El sello de DOCUMENTO (/SubFilter /ETSI.RFC3161) de PAdES B-LTA: no hay firmante
// humano, la autoridad sella el resumen del fichero.
func TestVerificarSelloDeDocumento(t *testing.T) {
	e := nuevoEscenario(t)
	h, err := huecoIncremental(docDePrueba(t, "acta"), OpcionesFirma{Nombre: "sello"})
	if err != nil {
		t.Fatal(err)
	}
	// El /SubFilter se cambia ANTES de resumir y sin mover un byte: el nombre
	// nuevo es más corto y lo que sobra son espacios, separador válido en un dict.
	i := bytes.LastIndex(h.datos, []byte("/"+subFiltroPAdES))
	copy(h.datos[i:], append([]byte("/ETSI.RFC3161"), bytes.Repeat([]byte(" "), len(subFiltroPAdES)-len("ETSI.RFC3161"))...))
	pdf, err := h.incrustar(selloDePrueba(t, e.tsa, h.resumen(), time.Now()))
	if err != nil {
		t.Fatal(err)
	}

	f := unaFirma(t, verificar(t, pdf, e.opciones(time.Time{})))
	if f.Tipo != "sello_documento" || f.Veredicto != VeredictoValida {
		t.Fatalf("tipo %q, veredicto %q, motivos %v", f.Tipo, f.Veredicto, f.Motivos)
	}
	if f.Firmante.Sujeto != "TSA de prueba" || f.Sello == nil || f.Sello.Confianza != ConfianzaConfiable {
		t.Errorf("firmante %+v, sello %+v", f.Firmante, f.Sello)
	}
	// La TSA no es un firmante: con solo las anclas de firmantes no se confía en ella.
	f = unaFirma(t, verificar(t, pdf, OpcionesVerificacion{Anclas: e.firmantes.anclas()}))
	if f.Confianza != ConfianzaNoConfiable {
		t.Errorf("confianza = %q", f.Confianza)
	}
}

// El token de OTRA implementación: OpenSSL haciendo de autoridad. Si solo
// verificáramos los sellos que fabricamos nosotros, no probaría nada.
func TestVerificarSelloDeOpenSSL(t *testing.T) {
	openssl, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("sin openssl no hay autoridad de sellado independiente")
	}
	dir := t.TempDir()
	tsa := nuevaTSADePrueba(t, openssl, dir)
	raiz, err := os.ReadFile(filepath.Join(dir, "tsaca.pem"))
	if err != nil {
		t.Fatal(err)
	}
	anclasTSA := x509.NewCertPool()
	anclasTSA.AppendCertsFromPEM(raiz)
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, tsa.sellar)

	f := unaFirma(t, verificar(t, pdf, OpcionesVerificacion{Anclas: e.firmantes.anclas(), AnclasSello: anclasTSA}))
	if f.Veredicto != VeredictoValida || f.Sello == nil || f.Sello.Autoridad != "TSA de prueba" {
		t.Fatalf("veredicto %q, motivos %v, sello %+v", f.Veredicto, f.Motivos, f.Sello)
	}
}

// Entrada hostil: mil ficheros firmados con un byte cambiado al azar. Ninguno
// puede tumbar el proceso, y ninguno puede salir «válido» si el byte tocado está
// en la parte firmada.
func TestVerificarNoSeRompeConBytesAlAzar(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, e.sello(t, time.Now()))
	r, err := reabrir(t, pdf).rangoDeFirma(reabrir(t, pdf).camposDeFirma()[0])
	if err != nil {
		t.Fatal(err)
	}
	opts := e.opciones(time.Time{})
	for i := 0; i < 1000; i++ {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(pdf))))
		pos := int(n.Int64())
		if pos >= r.hueco0 && pos < r.hueco1 {
			continue // dentro del hueco: lo que cambia es la firma, otro caso
		}
		tocado := append([]byte(nil), pdf...)
		tocado[pos] ^= 0x5a
		d, err := LeerPDF(tocado)
		if err != nil {
			continue
		}
		for _, f := range d.VerificarFirmas(opts) {
			if f.Veredicto == VeredictoValida {
				t.Fatalf("byte %d cambiado y la firma sale válida", pos)
			}
		}
	}
}

// El trabajo total está acotado: pasado el presupuesto de bytes, la firma no se
// verifica y lo dice, en vez de resumir 32 veces un fichero de 100 MiB.
func TestVerificarRespetaElPresupuestoDeBytes(t *testing.T) {
	e := nuevoEscenario(t)
	ana := e.firmantes.emitir(t, "Ana Pérez", time.Now().Add(-time.Hour), time.Now().Add(24*time.Hour))
	pdf := firmarPara(t, docDePrueba(t, "acta"), ana, e.sello(t, time.Now()))

	anterior := maxBytesVerificados
	t.Cleanup(func() { maxBytesVerificados = anterior })

	maxBytesVerificados = int64(len(pdf)) // cabe justo una
	if f := unaFirma(t, verificar(t, pdf, e.opciones(time.Time{}))); f.Veredicto != VeredictoValida {
		t.Fatalf("con presupuesto justo: %q %v", f.Veredicto, f.Motivos)
	}
	maxBytesVerificados = int64(len(pdf)) - 1
	f := unaFirma(t, verificar(t, pdf, e.opciones(time.Time{})))
	if f.Veredicto != VeredictoIndeterminada || !slices.Contains(f.Motivos, MotivoLimite) || f.Integra {
		t.Fatalf("sin presupuesto: %q %v, íntegra %v", f.Veredicto, f.Motivos, f.Integra)
	}
}

// Decisiones del 2026-10-16: sin atributos firmados la firma se verifica y el
// informe lo dice sin bajar el veredicto; la serie negativa la invalida con su
// propio motivo, no como «ilegible».
func TestVerificarSinAtributosYSerieNegativa(t *testing.T) {
	if m := motivoDeCMS(fmt.Errorf("envuelto: %w", cms.ErrSerieNegativa)); m != MotivoSerieNegativa {
		t.Errorf("serie negativa da el motivo %q", m)
	}
	f := FirmaVerificada{Motivos: []string{MotivoSinAtributos}}
	if v := f.cerrar().Veredicto; v != VeredictoValida {
		t.Errorf("sin atributos: veredicto %q, quería válida", v)
	}
	f = FirmaVerificada{Motivos: []string{MotivoSinAtributos, MotivoSerieNegativa}}
	if v := f.cerrar().Veredicto; v != VeredictoInvalida {
		t.Errorf("serie negativa: veredicto %q, quería inválida", v)
	}
}
