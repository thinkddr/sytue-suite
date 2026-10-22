// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// El verificador de firmas contra una BATERÍA DE PDF FIRMADOS POR HERRAMIENTAS AJENAS
// (OpenSSL, LibreOffice y pyHanko; ver testdata/pdf/generar.sh).
//
// Los tests de pdfverificar_test.go firman con el mismo código de la casa: si
// firmar y verificar compartieran un malentendido de la norma, pasarían los dos.
// Aquí el PDF y la firma vienen de fuera, así que un acierto es una prueba de
// interoperabilidad y un fallo es un hallazgo, no un test mal escrito.
//
// Lo que debe salir de cada fichero está en testdata/pdf/manifiesto.json, escrito a
// mano. La hora de la verificación está fijada en el manifiesto (las vigencias del
// corpus son absolutas), así que el resultado no depende del reloj.
package domain

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const dirCorpusPDF = "testdata/pdf"

type manifiestoCorpus struct {
	Ahora string       `json:"ahora"`
	Casos []casoCorpus `json:"casos"`
}

type casoCorpus struct {
	ID          string `json:"id"`
	Fichero     string `json:"fichero"`
	Descripcion string `json:"descripcion"`
	// Anclas y AnclasSello son nombres de anclas/<nombre>.crt.
	Anclas      []string        `json:"anclas"`
	AnclasSello []string        `json:"anclas_sello"`
	Firmas      []firmaEsperada `json:"firmas"`
}

type firmaEsperada struct {
	Campo     string   `json:"campo"`
	Tipo      string   `json:"tipo"`
	Veredicto string   `json:"veredicto"`
	Motivos   []string `json:"motivos"`
	Integra   bool     `json:"integra"`
	Confianza string   `json:"confianza"`
	// Firmante es el CN; vacío si no se llegó a leer el certificado.
	Firmante string `json:"firmante"`
	// Alcance de lo cambiado después; vacío si la firma ni se llegó a juzgar.
	Alcance       string         `json:"alcance"`
	Paginas       []int          `json:"paginas"`
	Certificacion int            `json:"certificacion"`
	Sello         *selloEsperado `json:"sello"`
	// HoraReferencia es opcional: solo se compara si se declara.
	HoraReferencia string `json:"hora_referencia"`
}

type selloEsperado struct {
	Integro   bool   `json:"integro"`
	Confianza string `json:"confianza"`
	Autoridad string `json:"autoridad"`
}

func leerManifiestoCorpus(t *testing.T) manifiestoCorpus {
	t.Helper()
	crudo, err := os.ReadFile(filepath.Join(dirCorpusPDF, "manifiesto.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m manifiestoCorpus
	dec := json.NewDecoder(strings.NewReader(string(crudo)))
	dec.DisallowUnknownFields() // una errata en una clave no puede dejar un campo sin comprobar
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("manifiesto.json: %v", err)
	}
	return m
}

func anclasDelCorpus(t *testing.T, nombres []string) *x509.CertPool {
	t.Helper()
	if len(nombres) == 0 {
		return nil
	}
	p := x509.NewCertPool()
	for _, n := range nombres {
		crudo, err := os.ReadFile(filepath.Join(dirCorpusPDF, "anclas", n+".crt"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := pem.Decode(crudo)
		if b == nil {
			t.Fatalf("anclas/%s.crt no es PEM", n)
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			t.Fatalf("anclas/%s.crt: %v", n, err)
		}
		p.AddCert(c)
	}
	return p
}

func TestVerificarCorpusDePDFFirmados(t *testing.T) {
	m := leerManifiestoCorpus(t)
	ahora, err := time.Parse(time.RFC3339, m.Ahora)
	if err != nil {
		t.Fatalf("ahora: %v", err)
	}
	if len(m.Casos) == 0 {
		t.Fatal("el manifiesto no declara ningún caso")
	}
	vistos := map[string]bool{}
	for _, c := range m.Casos {
		if vistos[c.ID] {
			t.Fatalf("caso repetido: %s", c.ID)
		}
		vistos[c.ID] = true
		t.Run(c.ID, func(t *testing.T) {
			pdf, err := os.ReadFile(filepath.Join(dirCorpusPDF, c.Fichero))
			if err != nil {
				t.Fatal(err)
			}
			d, err := LeerPDF(pdf)
			if err != nil {
				t.Fatalf("LeerPDF(%s): %v", c.Fichero, err)
			}
			got := d.VerificarFirmas(OpcionesVerificacion{
				Anclas: anclasDelCorpus(t, c.Anclas), AnclasSello: anclasDelCorpus(t, c.AnclasSello), Ahora: ahora,
			})
			if len(got) != len(c.Firmas) {
				t.Fatalf("%s: %d firmas, el manifiesto espera %d: %+v", c.Fichero, len(got), len(c.Firmas), got)
			}
			for i, want := range c.Firmas {
				comprobarFirmaDelCorpus(t, c.Fichero, i, got[i], want)
			}
		})
	}
}

func comprobarFirmaDelCorpus(t *testing.T, fichero string, i int, got FirmaVerificada, want firmaEsperada) {
	t.Helper()
	donde := func(campo string) string { return fichero + " firma " + want.Campo + " (" + campo + ")" }
	if want.Campo == "" || want.Tipo == "" || want.Veredicto == "" || want.Confianza == "" {
		t.Fatalf("%s #%d: el manifiesto no declara campo, tipo, veredicto y confianza", fichero, i)
	}
	// Un campo que acaba en «*» se compara por prefijo: pyHanko pone a los sellos de
	// documento un nombre con un UUID (Timestamp-<uuid>) distinto en cada generación.
	if prefijo, esPrefijo := strings.CutSuffix(want.Campo, "*"); (esPrefijo && !strings.HasPrefix(got.Campo, prefijo)) ||
		(!esPrefijo && got.Campo != want.Campo) {
		t.Errorf("%s: campo = %q, quería %q", donde("campo"), got.Campo, want.Campo)
	}
	if got.Tipo != want.Tipo {
		t.Errorf("%s: tipo = %q, quería %q", donde("tipo"), got.Tipo, want.Tipo)
	}
	if got.Veredicto != want.Veredicto {
		t.Errorf("%s: veredicto = %q, quería %q (motivos %v)", donde("veredicto"), got.Veredicto, want.Veredicto, got.Motivos)
	}
	gm, wm := slices.Clone(got.Motivos), slices.Clone(want.Motivos)
	slices.Sort(gm)
	slices.Sort(wm)
	if !slices.Equal(gm, wm) {
		t.Errorf("%s: motivos = %v, quería %v", donde("motivos"), got.Motivos, want.Motivos)
	}
	if got.Integra != want.Integra {
		t.Errorf("%s: íntegra = %v, quería %v", donde("integra"), got.Integra, want.Integra)
	}
	if got.Confianza != want.Confianza {
		t.Errorf("%s: confianza = %q, quería %q", donde("confianza"), got.Confianza, want.Confianza)
	}
	firmante := ""
	if got.Firmante != nil {
		firmante = got.Firmante.Sujeto
	}
	if firmante != want.Firmante {
		t.Errorf("%s: firmante = %q, quería %q", donde("firmante"), firmante, want.Firmante)
	}
	if got.Cambios.Alcance != want.Alcance {
		t.Errorf("%s: alcance de los cambios = %q, quería %q", donde("alcance"), got.Cambios.Alcance, want.Alcance)
	}
	if !slices.Equal(got.Cambios.Paginas, want.Paginas) {
		t.Errorf("%s: páginas cambiadas = %v, quería %v", donde("paginas"), got.Cambios.Paginas, want.Paginas)
	}
	if got.Certificacion != want.Certificacion {
		t.Errorf("%s: certificación /P = %d, quería %d", donde("certificacion"), got.Certificacion, want.Certificacion)
	}
	switch {
	case want.Sello == nil && got.Sello != nil:
		t.Errorf("%s: trae sello %+v y el manifiesto no lo espera", donde("sello"), *got.Sello)
	case want.Sello != nil && got.Sello == nil:
		t.Errorf("%s: no se encontró el sello que el manifiesto espera", donde("sello"))
	case want.Sello != nil:
		if got.Sello.Integro != want.Sello.Integro || got.Sello.Confianza != want.Sello.Confianza ||
			got.Sello.Autoridad != want.Sello.Autoridad {
			t.Errorf("%s: sello = %+v, quería %+v", donde("sello"), *got.Sello, *want.Sello)
		}
	}
	if want.HoraReferencia != "" && got.HoraReferencia != want.HoraReferencia {
		t.Errorf("%s: hora de referencia = %q, quería %q", donde("hora"), got.HoraReferencia, want.HoraReferencia)
	}
}

// El corpus y el manifiesto no pueden separarse: un PDF sin caso no prueba nada (y
// quien lo añadió creerá que sí), y un caso sin PDF es un test que no se ejecuta.
func TestCorpusDePDFYManifiestoVanJuntos(t *testing.T) {
	m := leerManifiestoCorpus(t)
	declarados := map[string]bool{}
	for _, c := range m.Casos {
		declarados[c.Fichero] = true
		if _, err := os.Stat(filepath.Join(dirCorpusPDF, c.Fichero)); err != nil {
			t.Errorf("caso %s: %v", c.ID, err)
		}
	}
	ficheros, err := filepath.Glob(filepath.Join(dirCorpusPDF, "*.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range ficheros {
		if !declarados[filepath.Base(f)] {
			t.Errorf("%s no está en el manifiesto", filepath.Base(f))
		}
	}
}

// Una batería cuyo manifiesto se fuera pareciendo a «todo válido» no vigilaría
// nada. Esto fija QUÉ situaciones tiene que cubrir: si se quita el caso del
// certificado caducado, el test lo dice en vez de quedarse en verde.
func TestCorpusDePDFCubreLosVeredictosYLosMotivos(t *testing.T) {
	m := leerManifiestoCorpus(t)
	veredictos, motivos, tipos, alcances := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	certificaciones := map[int]bool{}
	var conSello, sinSello, selloDeDocumento int
	for _, c := range m.Casos {
		for _, f := range c.Firmas {
			veredictos[f.Veredicto] = true
			tipos[f.Tipo] = true
			alcances[f.Alcance] = true
			if f.Certificacion != 0 {
				certificaciones[f.Certificacion] = true
			}
			for _, mo := range f.Motivos {
				motivos[mo] = true
			}
			switch {
			case f.Tipo == "sello_documento":
				selloDeDocumento++
			case f.Sello != nil:
				conSello++
			default:
				sinSello++
			}
		}
	}
	exigir := func(que string, hay map[string]bool, quiero ...string) {
		for _, q := range quiero {
			if !hay[q] {
				t.Errorf("la batería no cubre %s %q", que, q)
			}
		}
	}
	exigir("el veredicto", veredictos, VeredictoValida, VeredictoIndeterminada, VeredictoModificada, VeredictoInvalida)
	exigir("el motivo", motivos, MotivoRango, MotivoIntegridad, MotivoCertificacion, MotivoCambios,
		MotivoSinAnclas, MotivoNoConfiable, MotivoCaducado, MotivoSelloNoConfiable, MotivoSelloInvalido, MotivoFormato)
	exigir("el tipo", tipos, "pades", "pkcs7", "sello_documento")
	exigir("el alcance", alcances, AlcanceNinguno, AlcancePermitidos, AlcanceOtros)
	for _, p := range []int{1, 2, 3} {
		if !certificaciones[p] {
			t.Errorf("la batería no cubre la certificación /P %d", p)
		}
	}
	if conSello == 0 || sinSello == 0 || selloDeDocumento == 0 {
		t.Errorf("faltan firmas con sello (%d), sin sello (%d) o sellos de documento (%d)", conSello, sinSello, selloDeDocumento)
	}
	if len(m.Casos) < 20 {
		t.Errorf("la batería tiene %d casos; se esperaban al menos 20", len(m.Casos))
	}
}

// Las dos reglas nuevas del catálogo y la de las anotaciones, por separado: el
// corpus prueba que lo legítimo pasa; esto, que lo que no lo es sigue sin pasar.
func TestComparadorReglasDeVersionYAnotaciones(t *testing.T) {
	k := comparador{}
	for _, c := range []struct {
		viejo, nuevo pdfObj
		quiero       bool
	}{
		{pdfName("1.7"), pdfName("2.0"), true},
		{pdfName("1.7"), pdfName("1.7"), true},
		{nil, pdfName("1.7"), true},
		{pdfName("2.0"), pdfName("1.7"), false}, // bajar la versión no es firmar otra vez
		{pdfName("1.7"), pdfName("10.0"), false},
		{pdfName("1.7"), pdfString("2.0"), false},
		{pdfName("1.7"), int64(2), false},
	} {
		if got := k.versionNoBaja(c.viejo, c.nuevo); got != c.quiero {
			t.Errorf("versionNoBaja(%v, %v) = %v, quería %v", c.viejo, c.nuevo, got, c.quiero)
		}
	}
	for _, c := range []struct {
		d      pdfDict
		quiero bool
	}{
		{pdfDict{"Subtype": pdfName("Text")}, true},
		{pdfDict{"Subtype": pdfName("Highlight")}, true},
		{pdfDict{"Subtype": pdfName("Widget")}, false}, // un campo de formulario o una firma
		{pdfDict{"Subtype": pdfName("Text"), "FT": pdfName("Sig")}, false},
		{pdfDict{"Type": pdfName("Page")}, false},
		{nil, false},
	} {
		if got := esAnotacionLibre(c.d); got != c.quiero {
			t.Errorf("esAnotacionLibre(%v) = %v, quería %v", c.d, got, c.quiero)
		}
	}
}
