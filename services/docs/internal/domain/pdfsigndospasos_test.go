// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Checks de la firma en dos pasos. «El sistema» es una clave en memoria que firma
// EXACTAMENTE como lo hacen las funciones de firma de cada almacén —el resumen ya
// calculado (Llavero, CNG), el DigestInfo en crudo (CKM_RSA_PKCS, NONEwithRSA) y
// ECDSA en DER o en r‖s—, así que lo que se prueba es que el servicio acepta las
// cuatro formas y que el resultado lo verifica un validador de CMS de verdad
// (`platform/pkg/cms`, que además exige signing-certificate-v2).
//
// Los certificados se generan en el test: no hay claves en el repo.
package domain

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cms"
)

// identidadSistema es una CA de prueba y un certificado de firma emitido por ella,
// con su clave: lo que en la vida real vive en una tarjeta.
type identidadSistema struct {
	ca    *x509.Certificate
	cert  *x509.Certificate
	clave crypto.Signer
}

func (i identidadSistema) cadena() []*x509.Certificate { return []*x509.Certificate{i.cert, i.ca} }

func (i identidadSistema) anclas() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(i.ca)
	return p
}

// nuevaIdentidadSistema emite el certificado; `qc` son las declaraciones QC que
// lleva (ninguna = un certificado corriente).
func nuevaIdentidadSistema(t *testing.T, eliptica bool, qc ...asn1.ObjectIdentifier) identidadSistema {
	t.Helper()
	claveCA, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plantillaCA := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "CA de prueba de Sytue"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(48 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	derCA, err := x509.CreateCertificate(rand.Reader, plantillaCA, plantillaCA, &claveCA.PublicKey, claveCA)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(derCA)

	var clave crypto.Signer
	if eliptica {
		clave, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	} else {
		clave, err = rsa.GenerateKey(rand.Reader, 2048)
	}
	if err != nil {
		t.Fatal(err)
	}
	plantilla := &x509.Certificate{
		SerialNumber: big.NewInt(20260924),
		Subject:      pkix.Name{CommonName: "María López (FIRMA)", Country: []string{"ES"}},
		NotBefore:    time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		// El de FIRMA del DNI electrónico lleva nonRepudiation, no digitalSignature.
		KeyUsage: x509.KeyUsageContentCommitment,
	}
	if len(qc) > 0 {
		declaraciones := make([]struct{ ID asn1.ObjectIdentifier }, len(qc))
		for i, o := range qc {
			declaraciones[i].ID = o
		}
		valor, err := asn1.Marshal(declaraciones)
		if err != nil {
			t.Fatal(err)
		}
		plantilla.ExtraExtensions = []pkix.Extension{{Id: oidDeclaracionesQC, Value: valor}}
	}
	der, err := x509.CreateCertificate(rand.Reader, plantilla, ca, clave.Public(), claveCA)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return identidadSistema{ca: ca, cert: cert, clave: clave}
}

// Las cuatro formas en que firma «el sistema».
func firmaDelResumen(t *testing.T, k crypto.Signer, p *FirmaPreparada) []byte {
	t.Helper()
	f, err := k.Sign(rand.Reader, p.Resumen, crypto.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func firmaRSACruda(t *testing.T, k crypto.Signer, p *FirmaPreparada) []byte {
	t.Helper()
	// crypto.Hash(0) = «firma estos bytes tal cual»: es CKM_RSA_PKCS y NONEwithRSA,
	// que reciben el DigestInfo entero.
	f, err := rsa.SignPKCS1v15(rand.Reader, k.(*rsa.PrivateKey), 0, p.DigestInfo)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func firmaECDSAP1363(t *testing.T, k crypto.Signer, p *FirmaPreparada) []byte {
	t.Helper()
	r, s, err := ecdsa.Sign(rand.Reader, k.(*ecdsa.PrivateKey), p.Resumen)
	if err != nil {
		t.Fatal(err)
	}
	return append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
}

// cmsDelPDF saca el CMS (sin el relleno de ceros) y lo que cubre su /ByteRange,
// con la misma lectura del rango que usa el verificador.
func cmsDelPDF(t *testing.T, pdf []byte, n int) (der, contenido []byte) {
	t.Helper()
	d, err := LeerPDF(pdf)
	if err != nil {
		t.Fatal(err)
	}
	campos := d.camposDeFirma()
	if n >= len(campos) {
		t.Fatalf("no hay firma número %d", n)
	}
	r, err := d.rangoDeFirma(campos[n])
	if err != nil {
		t.Fatal(err)
	}
	return r.der, append(append([]byte(nil), pdf[:r.hueco0]...), pdf[r.hueco1:r.fin]...)
}

func TestFirmaEnDosPasosConCadaFormaDeFirmarDelSistema(t *testing.T) {
	casos := []struct {
		nombre   string
		eliptica bool
		formato  string
		firmar   func(*testing.T, crypto.Signer, *FirmaPreparada) []byte
	}{
		{"RSA con el resumen (Llavero, CNG)", false, "", firmaDelResumen},
		{"RSA en crudo con el DigestInfo (PKCS#11, Android)", false, "", firmaRSACruda},
		{"ECDSA en DER (Llavero, Android)", true, "der", firmaDelResumen},
		{"ECDSA en r‖s (CNG, PKCS#11)", true, "p1363", firmaECDSAP1363},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			id := nuevaIdentidadSistema(t, c.eliptica)
			p, err := PrepararFirmaPDF(docDePrueba(t, "contrato de arrendamiento"), OpcionesFirma{
				Motivo: "Conforme", Lugar: "Madrid", Rect: [4]float64{80, 80, 300, 150},
				// Lo que diga la app del nombre y la fecha NO cuenta.
				Nombre: "Otra Persona", Fecha: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC),
			}, id.cadena())
			if err != nil {
				t.Fatalf("PrepararFirmaPDF: %v", err)
			}
			firma, err := FirmaEnDER(id.cert.PublicKey, c.firmar(t, id.clave, p), c.formato)
			if err != nil {
				t.Fatalf("FirmaEnDER: %v", err)
			}
			srv := autoridadFalsa(t, 0, nil, nil)
			defer srv.Close()
			firmado, sellado, err := CompletarFirmaPDF(*p, id.cadena(), firma, func(h []byte) ([]byte, error) {
				return PedirSello(t.Context(), h, OpcionesSello{URL: srv.URL})
			})
			if err != nil {
				t.Fatalf("CompletarFirmaPDF: %v", err)
			}
			if !sellado {
				t.Error("con autoridad, la firma debía salir sellada")
			}

			// El validador de CMS de la suite: resumen, firma, signing-certificate-v2 y
			// cadena hasta la CA de prueba.
			der, contenido := cmsDelPDF(t, firmado, 0)
			sig, err := cms.Verify(der, contenido, x509.VerifyOptions{Roots: id.anclas()})
			if err != nil {
				t.Fatalf("cms.Verify no acepta la firma: %v", err)
			}
			if !sig.Signer.Equal(id.cert) {
				t.Error("el firmante verificado no es el certificado elegido")
			}
			if !sig.SigningTime.IsZero() {
				t.Error("PAdES prohíbe signing-time entre los atributos firmados")
			}
			if !bytes.Contains(firmado, []byte("/SubFilter /ETSI.CAdES.detached")) {
				t.Error("el /SubFilter no es el de PAdES")
			}
			oidSelloDER, _ := asn1.Marshal(oidAtributoSelloFirma)
			oidCertDER, _ := asn1.Marshal(oidCertificadoFirmante)
			if !bytes.Contains(der, oidSelloDER) || !bytes.Contains(der, oidCertDER) {
				t.Error("faltan el sello o signing-certificate-v2 en el CMS")
			}

			d, err := LeerPDF(firmado)
			if err != nil {
				t.Fatalf("el PDF firmado no abre: %v", err)
			}
			f := d.FirmasDelDocumento()
			if len(f) != 1 || f[0].Nombre != "María López (FIRMA)" || !f[0].CubreTodo {
				t.Errorf("firmas leídas = %+v (el nombre sale del certificado, no de la app)", f)
			}
			if p.Fecha.Year() == 1999 {
				t.Error("la fecha la pone el servicio, no la app")
			}
		})
	}
}

// TestUnClienteQueMienteNoConsigueUnaFirma es EL check de seguridad del segundo
// paso: nada de lo que manda la app se cree sin comprobarlo.
func TestUnClienteQueMienteNoConsigueUnaFirma(t *testing.T) {
	id := nuevaIdentidadSistema(t, false)
	otra := nuevaIdentidadSistema(t, false)
	p, err := PrepararFirmaPDF(docDePrueba(t, "importe: 1000 euros"), OpcionesFirma{}, id.cadena())
	if err != nil {
		t.Fatalf("PrepararFirmaPDF: %v", err)
	}
	buena := firmaDelResumen(t, id.clave, p)

	otroResumen := *p
	otroResumen.Resumen = bytes.Repeat([]byte{7}, 32)
	manipulado := *p
	manipulado.PDF = bytes.Clone(p.PDF)
	manipulado.PDF[10] ^= 1
	rellenado := *p
	rellenado.PDF = bytes.Clone(p.PDF)
	rellenado.PDF[p.Inicio+1] = 'A'

	casos := []struct {
		nombre string
		p      FirmaPreparada
		cadena []*x509.Certificate
		firma  []byte
		quiere error
	}{
		{"firma de otro resumen", *p, id.cadena(), firmaDelResumen(t, id.clave, &otroResumen), ErrFirmaNoCorresponde},
		{"firma con otra clave", *p, id.cadena(), firmaDelResumen(t, otra.clave, p), ErrFirmaNoCorresponde},
		{"otro certificado para la misma firma", *p, otra.cadena(), buena, ErrPreparadoAlterado},
		{"basura", *p, id.cadena(), []byte("no es una firma"), ErrFirmaNoCorresponde},
		{"documento tocado fuera del hueco", manipulado, id.cadena(), buena, ErrPreparadoAlterado},
		{"hueco ya relleno", rellenado, id.cadena(), buena, ErrPreparadoAlterado},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			_, _, err := CompletarFirmaPDF(c.p, c.cadena, c.firma, nil)
			if !errors.Is(err, c.quiere) {
				t.Errorf("err = %v, quería %v", err, c.quiere)
			}
		})
	}
	// Y la buena, sí.
	if _, _, err := CompletarFirmaPDF(*p, id.cadena(), buena, nil); err != nil {
		t.Fatalf("la firma buena no se acepta: %v", err)
	}
}

// TestSinSelloNoSeFirmaEnSilencio: en dos pasos, si la autoridad falla NO se
// firma sin sello a escondidas; y si quien firma pide seguir sin sello, sí.
func TestSinSelloNoSeFirmaEnSilencio(t *testing.T) {
	id := nuevaIdentidadSistema(t, true)
	p, err := PrepararFirmaPDF(docDePrueba(t, "acta"), OpcionesFirma{}, id.cadena())
	if err != nil {
		t.Fatal(err)
	}
	firma := firmaDelResumen(t, id.clave, p)
	caida := func([]byte) ([]byte, error) { return nil, errors.New("la autoridad está caída") }
	if _, _, err := CompletarFirmaPDF(*p, id.cadena(), firma, caida); !errors.Is(err, ErrSelloNoDisponible) {
		t.Fatalf("err = %v, quería ErrSelloNoDisponible", err)
	}
	// La MISMA firma sirve para reintentar: el sello no entra en lo firmado, así
	// que no hace falta volver a pedir el PIN.
	firmado, sellado, err := CompletarFirmaPDF(*p, id.cadena(), firma, nil)
	if err != nil || sellado {
		t.Fatalf("sin sello pedido: sellado=%v err=%v", sellado, err)
	}
	der, contenido := cmsDelPDF(t, firmado, 0)
	if _, err := cms.Verify(der, contenido, x509.VerifyOptions{Roots: id.anclas()}); err != nil {
		t.Fatalf("la firma sin sello no verifica: %v", err)
	}
}

// TestLaSegundaFirmaNoRompeLaPrimera: la firma del sistema sobre un documento ya
// firmado se anexa, y la primera sigue verificando sobre SU rango.
func TestLaSegundaFirmaNoRompeLaPrimera(t *testing.T) {
	certPEM, clavePEM := identidadDePrueba(t)
	primero, err := FirmarIncrementalPDF(docDePrueba(t, "contrato"), OpcionesFirma{CertificadoPEM: certPEM, ClavePEM: clavePEM})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := LeerPDF(primero)
	if err != nil {
		t.Fatal(err)
	}
	id := nuevaIdentidadSistema(t, false)
	p, err := PrepararFirmaPDF(doc, OpcionesFirma{}, id.cadena())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(p.PDF, primero) {
		t.Fatal("preparar la segunda firma ha tocado los bytes de la primera")
	}
	firmado, _, err := CompletarFirmaPDF(*p, id.cadena(), firmaDelResumen(t, id.clave, p), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := validarFirmaN(firmado, 0); err != nil {
		t.Errorf("la primera firma ya no valida: %v", err)
	}
	der, contenido := cmsDelPDF(t, firmado, 1)
	if _, err := cms.Verify(der, contenido, x509.VerifyOptions{Roots: id.anclas()}); err != nil {
		t.Errorf("la segunda firma no valida: %v", err)
	}
}

func TestCertificadoParaFirmar(t *testing.T) {
	id := nuevaIdentidadSistema(t, false)
	ahora := time.Now()
	if err := CertificadoParaFirmar(id.cert, ahora); err != nil {
		t.Fatalf("uno vigente y de firma: %v", err)
	}
	if err := CertificadoParaFirmar(id.cert, ahora.Add(72*time.Hour)); !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("caducado: err = %v", err)
	}
	if err := CertificadoParaFirmar(id.ca, ahora); !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("uno que solo firma certificados: err = %v", err)
	}
	if _, err := PrepararFirmaPDF(docDePrueba(t, "x"), OpcionesFirma{}, []*x509.Certificate{id.ca}); !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("preparar con un certificado que no firma: err = %v", err)
	}
}

func TestNivelDelCertificado(t *testing.T) {
	casos := []struct {
		nombre     string
		qc         []asn1.ObjectIdentifier
		cual, disp bool
	}{
		{"sin declaraciones (software corriente)", nil, false, false},
		{"cualificado en software (FNMT en fichero)", []asn1.ObjectIdentifier{oidQCCumple}, true, false},
		{"cualificado en dispositivo (DNI electrónico)", []asn1.ObjectIdentifier{oidQCCumple, oidQCDispositivoSeg}, true, true},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			cual, disp := NivelDelCertificado(nuevaIdentidadSistema(t, false, c.qc...).cert)
			if cual != c.cual || disp != c.disp {
				t.Errorf("cualificado=%v dispositivo=%v, quería %v %v", cual, disp, c.cual, c.disp)
			}
		})
	}
}

// TestLosAtributosVanEnOrdenDER: un validador que recodifica en DER —BouncyCastle,
// debajo del de la Comisión— reordena el SET; si no estaba ya ordenado, su resumen
// sale distinto y la firma «no verifica» sin que aquí se note nada.
func TestLosAtributosVanEnOrdenDER(t *testing.T) {
	id := nuevaIdentidadSistema(t, false)
	atributos, err := atributosFirmados(bytes.Repeat([]byte{1}, 32), id.cert)
	if err != nil {
		t.Fatal(err)
	}
	var anterior []byte
	for _, a := range atributos {
		der, _ := asn1.Marshal(a)
		if anterior != nil && bytes.Compare(anterior, der) > 0 {
			t.Fatalf("el SET OF no va en orden DER: %x antes que %x", anterior[:8], der[:8])
		}
		anterior = der
	}
}

// pemDe es el certificado en PEM, para las herramientas de fuera.
func pemDe(c *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})
}

// TestLaCadenaSeCompruebaAntesDelPIN: una cadena desordenada o que no cabe con el
// sello se rechaza al preparar, no después de que la persona haya firmado.
func TestLaCadenaSeCompruebaAntesDelPIN(t *testing.T) {
	id := nuevaIdentidadSistema(t, false)
	doc := docDePrueba(t, "contrato")
	if _, err := PrepararFirmaPDF(doc, OpcionesFirma{}, []*x509.Certificate{id.cert, nuevaIdentidadSistema(t, false).ca}); !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("una CA que no emite el certificado: err = %v", err)
	}
	relleno := make([]*x509.Certificate, 0, 42)
	relleno = append(relleno, id.cert, id.ca)
	for range 40 {
		relleno = append(relleno, id.ca)
	}
	if _, err := PrepararFirmaPDF(doc, OpcionesFirma{}, relleno); !errors.Is(err, ErrCertificadoNoUsable) {
		t.Errorf("una cadena que no cabe: err = %v", err)
	}
	if _, err := PrepararFirmaPDF(doc, OpcionesFirma{}, id.cadena()); err != nil {
		t.Errorf("la cadena buena: %v", err)
	}
}
