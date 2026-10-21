// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Verificar las firmas de un PDF RECIBIDO. Firmar ya se hacía (pdfsign.go); esto es
// la otra mitad, la que importa a quien recibe un contrato: ¿lo firmó quien dice,
// y es lo que firmó?
//
// QUÉ SE COMPRUEBA, firma a firma y por separado, porque cada cosa falla por su
// lado y la pantalla tiene que decir cuál:
//
//  1. ESTRUCTURA. El /ByteRange empieza en 0, deja fuera EXACTAMENTE la cadena
//     hexadecimal de la firma y nada más, y esa cadena es el /Contents del MISMO
//     diccionario, escrito en la parte firmada. Es lo que cierra los ataques
//     conocidos contra validadores de PDF (USF, SWA): una firma buena colocada
//     donde no toca.
//  2. INTEGRIDAD. El resumen de los dos tramos es el firmado y la firma cuadra con
//     el certificado (`platform/pkg/cms`, el mismo validador que la firma en dos
//     pasos y los sellos).
//  3. CAMBIOS POSTERIORES. Si detrás del tramo firmado hay revisiones
//     incrementales, se comparan objeto a objeto con la versión firmada: añadir
//     otra firma o datos de validación es lo normal en un contrato que firman
//     dos; cambiar una página, no.
//  4. SELLO DE TIEMPO. Que sella ESTA firma (su resumen) y de qué autoridad es.
//  5. CONFIANZA. La cadena del firmante hasta un ancla, a la hora del sello si lo
//     hay y es de confianza, y si no a la de ahora.
//
// CONTRA QUÉ ANCLAS. Solo las que el despliegue configura (SYTUE_DOCS_FIRMA_ANCLAS,
// y las del sello para las autoridades de sellado). NUNCA el almacén del sistema:
// no es la lista de confianza de la UE y decidiría la confianza en silencio, con lo
// que trajera la imagen del contenedor (`cms.ErrSinAnclas`). Sin anclas, la
// confianza sale «sin_anclas» y la firma, como mucho, indeterminada: es verdad que
// el documento no ha cambiado y es verdad que no sabemos quién es el firmante.
//
// LO QUE NO SE HACE, para que nadie lo descubra tarde: no se consulta la REVOCACIÓN
// (OCSP o CRL). Exigiría salir a la red de cada autoridad —que no siempre es de la
// UE— en cada apertura. Lo dice la pantalla, siempre.
//
// ENTRADA NO CONFIABLE. El fichero viene de fuera, así que todo va acotado: cuántas
// firmas se verifican, cuántos campos se recorren y a qué profundidad, cuántos
// objetos se comparan. El tamaño del fichero lo acota ya el handler
// (`MaxImportBytes`), y la criptografía es la de la biblioteca estándar.
package domain

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"errors"
	"math"
	"sort"
	"time"

	"git.sytue.eu/sytue/one/platform/pkg/cms"
)

const (
	// maxFirmasVerificadas acota el trabajo de un fichero hostil: cada firma
	// resume hasta el fichero entero. Un contrato real lleva dos o tres.
	maxFirmasVerificadas = 32
	// maxBytesVerificados acota el trabajo TOTAL: cada firma vuelve a resumir y a
	// releer el fichero (hasta 100 MiB), así que 32 firmas sobre un fichero máximo
	// son 3 GiB de resumen y de lectura, no 32 firmas baratas. Se cobra el tamaño del
	// fichero por firma, que es el techo de lo que cuesta.
	// maxCamposRecorridos y profCampos acotan el árbol del formulario.
	maxCamposRecorridos = 10000
	profCampos          = 16
	// maxObjetosComparados acota la comparación con la versión firmada. Pasado el
	// tope, lo que quede se cuenta como cambio: prometer de menos es lo barato.
	maxObjetosComparados = 2000
)

// maxBytesVerificados es una variable solo para que la prueba pueda bajarla; en
// producción es una constante de hecho.
var maxBytesVerificados int64 = 1 << 30

// Veredictos de una firma, de peor a mejor.
const (
	VeredictoInvalida      = "invalida"      // estructura o criptografía rotas
	VeredictoModificada    = "modificada"    // íntegra, pero el documento cambió después
	VeredictoIndeterminada = "indeterminada" // íntegra; falta confianza, vigencia o sello
	VeredictoValida        = "valida"
)

// Motivos de un veredicto: códigos estables que la interfaz traduce.
const (
	MotivoRango            = "rango"         // /ByteRange o hueco que no son los de esta firma
	MotivoFormato          = "formato"       // firma ilegible o de un tipo que no se sabe comprobar
	MotivoIntegridad       = "integridad"    // el contenido no es el que se firmó
	MotivoCertificacion    = "certificacion" // cambios que la firma de certificación prohíbe
	MotivoCambios          = "cambios"       // cambios posteriores que no son firmas
	MotivoSinAnclas        = "sin_anclas"
	MotivoNoConfiable      = "no_confiable"
	MotivoCaducado         = "caducado" // certificado fuera de vigencia a la hora comprobada
	MotivoSelloInvalido    = "sello_invalido"
	MotivoSelloNoConfiable = "sello_no_confiable"
	MotivoLimite           = "limite" // más firmas de las que se verifican
	// MotivoSerieNegativa: el certificado lleva un número de serie negativo, que
	// RFC 5280 prohíbe. Decisión del 2026-10-16: se rechaza (firma inválida).
	MotivoSerieNegativa = "serie_negativa"
	// MotivoSinAtributos es informativo: la firma no lleva atributos firmados
	// (ni hora ni tipo de contenido firmados). Se verifica igual y no cambia el
	// veredicto (decisión del 2026-10-16).
	MotivoSinAtributos = "sin_atributos"
)

// Estados de confianza de un certificado.
const (
	ConfianzaConfiable   = "confiable"
	ConfianzaNoConfiable = "no_confiable"
	ConfianzaSinAnclas   = "sin_anclas"
)

// Alcance de lo que cambió después de firmar.
const (
	AlcanceNinguno    = "ninguno"    // la firma cubre el fichero entero
	AlcancePermitidos = "permitidos" // solo otras firmas, sellos o datos de validación
	AlcanceOtros      = "otros"
)

// OpcionesVerificacion dice contra qué se confía. Todo es opcional.
type OpcionesVerificacion struct {
	// Anclas son las raíces en las que se confía para los firmantes.
	Anclas *x509.CertPool
	// AnclasSello son las de las autoridades de sellado de tiempo.
	AnclasSello *x509.CertPool
	// Ahora es la hora de la verificación; cero = la del reloj.
	Ahora time.Time
}

// CertificadoLeido es lo que la pantalla enseña de un certificado.
type CertificadoLeido struct {
	Sujeto       string `json:"sujeto"`
	Organizacion string `json:"organizacion,omitempty"`
	Emisor       string `json:"emisor"`
	Serie        string `json:"serie"`
	Desde        string `json:"desde"`
	Hasta        string `json:"hasta"`
	// Cualificado y DispositivoSeguro son lo que el certificado DECLARA de sí
	// mismo (NivelDelCertificado), no una consulta a la lista de confianza.
	Cualificado       bool `json:"cualificado,omitempty"`
	DispositivoSeguro bool `json:"dispositivo_seguro,omitempty"`
}

// SelloLeido es el sello de tiempo de una firma.
type SelloLeido struct {
	Fecha     string `json:"fecha,omitempty"`
	Autoridad string `json:"autoridad,omitempty"`
	// Integro: el token está bien firmado y sella el resumen de ESTA firma.
	Integro   bool   `json:"integro"`
	Confianza string `json:"confianza"`
}

// CambiosPosteriores es lo que pasó con el fichero después de esta firma.
type CambiosPosteriores struct {
	BytesFirmados int    `json:"bytes_firmados"`
	BytesTotales  int    `json:"bytes_totales"`
	Revisiones    int    `json:"revisiones"`
	Alcance       string `json:"alcance"`
	// Paginas son las que cambiaron (desde 1); Objetos, el resto de lo cambiado.
	Paginas []int `json:"paginas,omitempty"`
	Objetos int   `json:"objetos,omitempty"`
}

// FirmaVerificada es el informe de una firma.
type FirmaVerificada struct {
	FirmaLeida
	// Tipo: pades | pkcs7 | sello_documento, o el /SubFilter tal cual si no se
	// sabe comprobar.
	Tipo      string   `json:"tipo"`
	Veredicto string   `json:"veredicto"`
	Motivos   []string `json:"motivos,omitempty"`
	Integra   bool     `json:"integra"`
	// Firmante es quien firmó; en un sello de documento, la autoridad.
	Firmante  *CertificadoLeido  `json:"firmante,omitempty"`
	Cadena    []CertificadoLeido `json:"cadena,omitempty"`
	Confianza string             `json:"confianza"`
	// HoraReferencia es la hora a la que se juzgó la vigencia: la del sello si es
	// de confianza, la de la verificación si no.
	HoraReferencia string             `json:"hora_referencia,omitempty"`
	Sello          *SelloLeido        `json:"sello,omitempty"`
	Cambios        CambiosPosteriores `json:"cambios"`
	// Certificacion es el /P de una firma de certificación (DocMDP); 0 si no lo es.
	Certificacion int `json:"certificacion,omitempty"`
}

func (f *FirmaVerificada) motivo(m string) { f.Motivos = append(f.Motivos, m) }

// cerrar decide el veredicto a partir de los motivos: manda el peor.
func (f *FirmaVerificada) cerrar() FirmaVerificada {
	f.Veredicto = VeredictoValida
	for _, m := range f.Motivos {
		switch m {
		case MotivoRango, MotivoFormato, MotivoIntegridad, MotivoCertificacion, MotivoSerieNegativa:
			f.Veredicto = VeredictoInvalida
			return *f
		case MotivoSinAtributos:
		case MotivoCambios:
			f.Veredicto = VeredictoModificada
		default:
			if f.Veredicto == VeredictoValida {
				f.Veredicto = VeredictoIndeterminada
			}
		}
	}
	return *f
}

// VerificarFirmas devuelve el informe de cada firma del documento.
func (d *PDFDoc) VerificarFirmas(opts OpcionesVerificacion) []FirmaVerificada {
	if opts.Ahora.IsZero() {
		opts.Ahora = time.Now()
	}
	campos := d.camposDeFirma()
	out := make([]FirmaVerificada, 0, len(campos))
	presupuesto := maxBytesVerificados
	for i, c := range campos {
		presupuesto -= int64(len(d.buf))
		if i >= maxFirmasVerificadas || presupuesto < 0 {
			f := FirmaVerificada{FirmaLeida: d.firmaLeida(c), Tipo: tipoDeFirma(c.sig), Confianza: ConfianzaSinAnclas}
			f.motivo(MotivoLimite)
			out = append(out, f.cerrar())
			continue
		}
		out = append(out, d.verificarFirma(c, opts))
	}
	return out
}

// --- Los campos de firma ------------------------------------------------------

// campoDeFirma es un campo /FT /Sig con su /V ya resuelto.
type campoDeFirma struct {
	nombre string
	sig    pdfDict
	// num es el objeto donde está escrito el /V: el suyo si es indirecto, el del
	// campo si va dentro. Es lo que ata el hueco a ESTE diccionario.
	num int
}

// camposDeFirma recorre el árbol del formulario, /Kids incluidos: un campo de
// firma puede colgar de otro, y el /FT se hereda.
func (d *PDFDoc) camposDeFirma() []campoDeFirma {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	forma, _ := d.resolver(raiz["AcroForm"]).(pdfDict)
	campos, _ := d.resolver(forma["Fields"]).(pdfArray)

	r := recorridoCampos{d: d, vistos: map[int]bool{}}
	r.recorrer(campos, "", "", 0)
	return r.out
}

// recorridoCampos lleva el estado de un recorrido acotado del formulario.
type recorridoCampos struct {
	d          *PDFDoc
	out        []campoDeFirma
	vistos     map[int]bool
	recorridos int
}

func (r *recorridoCampos) recorrer(arr pdfArray, padre string, ftHeredado pdfName, prof int) {
	for _, c := range arr {
		if prof > profCampos || r.recorridos >= maxCamposRecorridos {
			return
		}
		r.recorridos++
		num := 0
		if ref, ok := c.(pdfRef); ok {
			if r.vistos[ref.Num] {
				continue // un /Kids que vuelve atrás no puede colgar el proceso
			}
			r.vistos[ref.Num], num = true, ref.Num
		}
		cd, ok := r.d.resolver(c).(pdfDict)
		if !ok {
			continue
		}
		nombre := padre
		if t, ok := r.d.resolver(cd["T"]).(pdfString); ok {
			if nombre != "" {
				nombre += "."
			}
			nombre += decodificarTextoPDF(t)
		}
		ft := ftHeredado
		if f, ok := r.d.resolver(cd["FT"]).(pdfName); ok {
			ft = f
		}
		if ft == "Sig" {
			if sig, ok := r.d.resolver(cd["V"]).(pdfDict); ok {
				if ref, ok := cd["V"].(pdfRef); ok {
					num = ref.Num
				}
				r.out = append(r.out, campoDeFirma{nombre: nombre, sig: sig, num: num})
				continue
			}
		}
		if kids, ok := r.d.resolver(cd["Kids"]).(pdfArray); ok {
			r.recorrer(kids, nombre, ft, prof+1)
		}
	}
}

// firmaLeida es lo que la firma DECLARA, que es lo que ya enseñaba la barra.
func (d *PDFDoc) firmaLeida(c campoDeFirma) FirmaLeida {
	txt := func(k pdfName) string {
		s, _ := d.resolver(c.sig[k]).(pdfString)
		return decodificarTextoPDF(s)
	}
	return FirmaLeida{
		Nombre: txt("Name"), Motivo: txt("Reason"), Lugar: txt("Location"), Fecha: txt("M"),
		Campo: c.nombre, CubreTodo: d.rangoCubreTodo(c.sig),
	}
}

func tipoDeFirma(sig pdfDict) string {
	sub, _ := sig["SubFilter"].(pdfName)
	switch sub {
	case subFiltroPAdES:
		return "pades"
	case "adbe.pkcs7.detached":
		return "pkcs7"
	case "ETSI.RFC3161":
		return "sello_documento"
	}
	return string(sub)
}

// --- Una firma ----------------------------------------------------------------

func (d *PDFDoc) verificarFirma(c campoDeFirma, opts OpcionesVerificacion) FirmaVerificada {
	f := FirmaVerificada{FirmaLeida: d.firmaLeida(c), Tipo: tipoDeFirma(c.sig), Confianza: ConfianzaSinAnclas}
	f.Certificacion = d.permisoDocMDP(c)

	r, err := d.rangoDeFirma(c)
	if err != nil {
		f.motivo(MotivoRango)
		return f.cerrar()
	}
	f.Cambios = d.cambiosTras(r.fin, f.Certificacion == 3)
	if f.Cambios.Alcance == AlcanceOtros {
		f.motivo(MotivoCambios)
	}
	// /P 1 no admite NADA después, ni siquiera otra firma.
	if (f.Certificacion == 1 && f.Cambios.Alcance != AlcanceNinguno) ||
		(f.Certificacion > 1 && f.Cambios.Alcance == AlcanceOtros) {
		f.motivo(MotivoCertificacion)
	}

	contenido := make([]byte, 0, r.hueco0+r.fin-r.hueco1)
	contenido = append(append(contenido, d.buf[:r.hueco0]...), d.buf[r.hueco1:r.fin]...)

	switch f.Tipo {
	case "pades", "pkcs7":
		d.verificarCMS(&f, r.der, contenido, opts)
	case "sello_documento":
		d.verificarSelloDeDocumento(&f, r.der, contenido, opts)
	default:
		// adbe.pkcs7.sha1, adbe.x509.rsa_sha1…: SHA-1 o formatos que hace años
		// que no emite nadie. Mejor decir «no se sabe comprobar» que adivinar.
		f.motivo(MotivoFormato)
	}
	return f.cerrar()
}

// verificarCMS es la firma corriente: CMS desprendido sobre los dos tramos.
func (d *PDFDoc) verificarCMS(f *FirmaVerificada, der, contenido []byte, opts OpcionesVerificacion) {
	sig, err := cms.Inspect(der, contenido)
	if err != nil {
		f.motivo(motivoDeCMS(err))
		return
	}
	f.Integra = true
	if sig.SinAtributosFirmados {
		f.motivo(MotivoSinAtributos)
	}

	hora := opts.Ahora
	if sig.TimestampToken != nil {
		var fecha time.Time
		f.Sello, fecha = selloDeFirma(sig.TimestampToken, sig.Value, opts)
		switch {
		case !f.Sello.Integro:
			f.motivo(MotivoSelloInvalido)
		case f.Sello.Confianza != ConfianzaConfiable:
			f.motivo(MotivoSelloNoConfiable)
		default:
			// Solo un sello de confianza prueba la hora; con él, un certificado
			// caducado HOY sigue valiendo para lo que firmó cuando estaba vigente.
			hora = fecha
		}
	}
	d.juzgarFirmante(f, sig, hora, anclasDe(opts.Anclas), []x509.ExtKeyUsage{x509.ExtKeyUsageAny})
}

// motivoDeCMS traduce el error de una firma que no se pudo comprobar.
func motivoDeCMS(err error) string {
	switch {
	case errors.Is(err, cms.ErrNoCorresponde):
		return MotivoIntegridad
	case errors.Is(err, cms.ErrSerieNegativa):
		return MotivoSerieNegativa
	}
	return MotivoFormato
}

// verificarSelloDeDocumento es /SubFilter /ETSI.RFC3161: el /Contents es un sello
// de tiempo cuyo TSTInfo sella el resumen de los dos tramos (PAdES B-LTA). No hay
// firmante humano: el «firmante» es la autoridad, y la hora es la del sello.
func (d *PDFDoc) verificarSelloDeDocumento(f *FirmaVerificada, der, contenido []byte, opts OpcionesVerificacion) {
	sig, tst, err := cms.InspectEncapsulated(der, oidTokenSello)
	if err != nil {
		f.motivo(motivoDeCMS(err))
		return
	}
	var info infoDelSello
	if _, err := asn1.Unmarshal(tst, &info); err != nil {
		f.motivo(MotivoFormato)
		return
	}
	if !sellaEsto(info.HuellaMensaje, contenido) {
		f.motivo(MotivoIntegridad)
		return
	}
	f.Integra = true
	f.Sello = &SelloLeido{Fecha: info.Generado.UTC().Format(time.RFC3339), Autoridad: sig.Signer.Subject.CommonName, Integro: true}
	d.juzgarFirmante(f, sig, info.Generado, anclasDe(opts.AnclasSello, opts.Anclas),
		[]x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping})
	f.Sello.Confianza = f.Confianza
}

// juzgarFirmante rellena el firmante, su cadena, la confianza y la vigencia a la
// hora de referencia.
func (d *PDFDoc) juzgarFirmante(f *FirmaVerificada, sig *cms.Signature, hora time.Time,
	anclas []*x509.CertPool, usos []x509.ExtKeyUsage,
) {
	firmante := certificadoLeido(sig.Signer)
	f.Firmante = &firmante
	f.HoraReferencia = hora.UTC().Format(time.RFC3339)

	var cadena []*x509.Certificate
	f.Confianza, cadena = confianzaDe(sig.Signer, sig.Certificates, anclas, hora, usos)
	for _, c := range cadena {
		f.Cadena = append(f.Cadena, certificadoLeido(c))
	}
	switch f.Confianza {
	case ConfianzaSinAnclas:
		f.motivo(MotivoSinAnclas)
	case ConfianzaNoConfiable:
		f.motivo(MotivoNoConfiable)
	}
	if hora.Before(sig.Signer.NotBefore) || hora.After(sig.Signer.NotAfter) {
		f.motivo(MotivoCaducado)
	}
}

// confianzaDe busca una cadena hasta alguna de las anclas. La VIGENCIA se juzga
// aparte (MotivoCaducado), así que la cadena se construye a una hora dentro de la
// vida del certificado: «es de una autoridad de confianza» y «estaba vigente» son
// dos preguntas, y fundirlas en un solo error escondería cuál falló.
//
// ponytail: la vigencia de los intermedios no se juzga por separado; un
// intermedio caducado a la hora de referencia sale como «no confiable». Si un caso
// real lo pide, se reporta cada eslabón.
func confianzaDe(c *x509.Certificate, embebidos []*x509.Certificate, anclas []*x509.CertPool,
	hora time.Time, usos []x509.ExtKeyUsage,
) (string, []*x509.Certificate) {
	if len(anclas) == 0 {
		return ConfianzaSinAnclas, encadenar(c, embebidos)
	}
	intermedios := x509.NewCertPool()
	for _, e := range embebidos {
		if !e.Equal(c) {
			intermedios.AddCert(e)
		}
	}
	switch {
	case hora.Before(c.NotBefore):
		hora = c.NotBefore
	case hora.After(c.NotAfter):
		hora = c.NotAfter
	}
	for _, raices := range anclas {
		cadenas, err := c.Verify(x509.VerifyOptions{
			Roots: raices, Intermediates: intermedios, CurrentTime: hora, KeyUsages: usos,
		})
		if err == nil && len(cadenas) > 0 {
			return ConfianzaConfiable, cadenas[0]
		}
	}
	return ConfianzaNoConfiable, encadenar(c, embebidos)
}

// encadenar ordena los certificados que trae la firma desde el firmante hacia
// arriba, para enseñarlos aunque no lleguen a ningún ancla.
func encadenar(c *x509.Certificate, embebidos []*x509.Certificate) []*x509.Certificate {
	cadena := []*x509.Certificate{c}
	for len(cadena) < 10 {
		ultimo := cadena[len(cadena)-1]
		if bytes.Equal(ultimo.RawIssuer, ultimo.RawSubject) {
			break // raíz autofirmada
		}
		var padre *x509.Certificate
		for _, e := range embebidos {
			if bytes.Equal(e.RawSubject, ultimo.RawIssuer) && !contieneCert(cadena, e) {
				padre = e
				break
			}
		}
		if padre == nil {
			break
		}
		cadena = append(cadena, padre)
	}
	return cadena
}

func contieneCert(cs []*x509.Certificate, c *x509.Certificate) bool {
	for _, x := range cs {
		if x.Equal(c) {
			return true
		}
	}
	return false
}

func anclasDe(pools ...*x509.CertPool) []*x509.CertPool {
	var out []*x509.CertPool
	for _, p := range pools {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}

func certificadoLeido(c *x509.Certificate) CertificadoLeido {
	cualificado, seguro := NivelDelCertificado(c)
	l := CertificadoLeido{
		Sujeto: c.Subject.CommonName, Emisor: c.Issuer.CommonName, Serie: c.SerialNumber.Text(16),
		Desde: c.NotBefore.UTC().Format(time.RFC3339), Hasta: c.NotAfter.UTC().Format(time.RFC3339),
		Cualificado: cualificado, DispositivoSeguro: seguro,
	}
	// Sin CN, el DN entero: un certificado de sello de empresa a veces solo trae
	// la organización, y una ficha en blanco no dice nada.
	if l.Sujeto == "" {
		l.Sujeto = c.Subject.String()
	}
	if l.Emisor == "" {
		l.Emisor = c.Issuer.String()
	}
	if len(c.Subject.Organization) > 0 {
		l.Organizacion = c.Subject.Organization[0]
	}
	return l
}

// selloDeFirma comprueba el sello que cuelga de una firma: que está bien firmado,
// que sella el resumen de `valor` —el valor de ESTA firma— y de quién es. Devuelve
// también la hora que atestigua.
func selloDeFirma(token, valor []byte, opts OpcionesVerificacion) (*SelloLeido, time.Time) {
	s := &SelloLeido{Confianza: ConfianzaSinAnclas}
	sig, tst, err := cms.InspectEncapsulated(token, oidTokenSello)
	if err != nil {
		return s, time.Time{}
	}
	var info infoDelSello
	if _, err := asn1.Unmarshal(tst, &info); err != nil || !sellaEsto(info.HuellaMensaje, valor) {
		return s, time.Time{}
	}
	s.Integro = true
	s.Fecha = info.Generado.UTC().Format(time.RFC3339)
	s.Autoridad = sig.Signer.Subject.CommonName
	// La autoridad tiene que estar vigente CUANDO selló: aquí no se recorta la
	// hora, a diferencia del firmante, porque esa hora es la que el sello prueba.
	s.Confianza = ConfianzaSinAnclas
	if anclas := anclasDe(opts.AnclasSello, opts.Anclas); len(anclas) > 0 {
		s.Confianza = ConfianzaNoConfiable
		if !info.Generado.Before(sig.Signer.NotBefore) && !info.Generado.After(sig.Signer.NotAfter) {
			s.Confianza, _ = confianzaDe(sig.Signer, sig.Certificates, anclas, info.Generado,
				[]x509.ExtKeyUsage{x509.ExtKeyUsageTimeStamping})
		}
	}
	return s, info.Generado
}

var (
	oidSHA384 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512 = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
)

// sellaEsto dice si la huella de un TSTInfo es la de `datos`. SHA-1 no se admite,
// igual que en el resto de la suite.
func sellaEsto(h huellaDeMensaje, datos []byte) bool {
	var alg crypto.Hash
	switch a := h.Algoritmo.Algorithm; {
	case a.Equal(oidSHA256):
		alg = crypto.SHA256
	case a.Equal(oidSHA384):
		alg = crypto.SHA384
	case a.Equal(oidSHA512):
		alg = crypto.SHA512
	default:
		return false
	}
	s := alg.New()
	s.Write(datos)
	return bytes.Equal(s.Sum(nil), h.Huella)
}

// permisoDocMDP es el /P de una firma de certificación, o 0 si no lo es. Cuenta
// solo si el catálogo la nombra en /Perms /DocMDP: sin eso ningún lector la trata
// como certificadora, y decir lo contrario sería prometer un bloqueo que no hay.
func (d *PDFDoc) permisoDocMDP(c campoDeFirma) int {
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	perms, _ := d.resolver(raiz["Perms"]).(pdfDict)
	if r, ok := perms["DocMDP"].(pdfRef); !ok || c.num == 0 || r.Num != c.num {
		return 0
	}
	sig := c.sig
	refs, _ := d.resolver(sig["Reference"]).(pdfArray)
	for _, r := range refs {
		rd, _ := d.resolver(r).(pdfDict)
		if m, _ := d.resolver(rd["TransformMethod"]).(pdfName); m != "DocMDP" {
			continue
		}
		params, _ := d.resolver(rd["TransformParams"]).(pdfDict)
		if p, ok := comoEntero(d.resolver(params["P"])); ok && p >= 1 && p <= 3 {
			return int(p)
		}
		return 2 // el valor por defecto de la norma (§12.8.2.2)
	}
	return 0
}

// --- El rango firmado ---------------------------------------------------------

// rangoFirma es lo firmado: [0, hueco0) y [hueco1, fin). En medio, la firma.
type rangoFirma struct {
	hueco0, hueco1, fin int
	der                 []byte // el CMS, sin el relleno de ceros
}

var errRango = errors.New("pdf: el /ByteRange no es el de esta firma")

// rangoDeFirma valida el /ByteRange contra el fichero y saca el CMS del hueco.
//
// Las tres comprobaciones de en medio son las que importan: que el hueco sea SOLO
// la cadena hexadecimal (si cupiera algo más, ese algo no estaría firmado), que el
// diccionario de la firma empiece en la parte firmada y contenga el hueco (si no,
// el /ByteRange que se lee no es el que se firmó), y que lo que hay en el hueco
// sea el /Contents de ese diccionario.
func (d *PDFDoc) rangoDeFirma(c campoDeFirma) (rangoFirma, error) {
	arr, ok := d.resolver(c.sig["ByteRange"]).(pdfArray)
	if !ok || len(arr) != 4 {
		return rangoFirma{}, errRango
	}
	var v [4]int64
	for i, o := range arr {
		n, ok := comoEntero(d.resolver(o))
		if !ok || n < 0 || n > int64(len(d.buf)) {
			return rangoFirma{}, errRango
		}
		v[i] = n
	}
	h0, h1, fin := int(v[1]), int(v[2]), int(v[2]+v[3])
	if v[0] != 0 || h0 < 1 || h1 < h0+2 || fin > len(d.buf) {
		return rangoFirma{}, errRango
	}
	crudo, ok := huecoHexadecimal(d.buf[h0:h1])
	if !ok || !d.diccionarioContieneElHueco(c, h0, crudo) {
		return rangoFirma{}, errRango
	}

	// El hueco va relleno de ceros hasta su tamaño: el CMS es el primer TLV. Si no
	// se lee, `der` queda vacío y lo dirá cms: ilegible.
	var tlv asn1.RawValue
	der := []byte(nil)
	if _, err := asn1.Unmarshal(crudo, &tlv); err == nil {
		der = tlv.FullBytes
	} else if len(crudo) > 1 && crudo[1] == 0x80 {
		// BER con longitud indefinida: no se sabe dónde acaba sin leerlo entero;
		// cms lo convierte a DER y descarta el relleno.
		der = crudo
	}
	return rangoFirma{h0, h1, fin, der}, nil
}

// huecoHexadecimal decodifica el hueco de la firma, que tiene que ser SOLO la
// cadena hexadecimal «<…>»: si cupiera algo más, ese algo no estaría firmado.
func huecoHexadecimal(hueco []byte) ([]byte, bool) {
	if hueco[0] != '<' || hueco[len(hueco)-1] != '>' || len(hueco)%2 != 0 {
		return nil, false
	}
	crudo := make([]byte, (len(hueco)-2)/2)
	if _, err := hex.Decode(crudo, hueco[1:len(hueco)-1]); err != nil {
		return nil, false
	}
	return crudo, true
}

// diccionarioContieneElHueco ata el hueco a ESTE diccionario de firma: que el
// objeto empiece en la parte firmada y acabe después del hueco, y que lo que hay
// en el hueco sea su /Contents.
func (d *PDFDoc) diccionarioContieneElHueco(c campoDeFirma, h0 int, crudo []byte) bool {
	e, ok := d.xref[c.num]
	if c.num == 0 || !ok || e.enFlujo || e.offset < 0 || int(e.offset) >= h0 ||
		bytes.Contains(d.buf[e.offset:h0], []byte("endobj")) {
		return false
	}
	// En un documento cifrado el lector descifra las cadenas, y el /Contents de
	// una firma NO va cifrado (§7.6.2): compararlas daría un falso negativo. La
	// atadura la siguen dando las comprobaciones de arriba.
	if d.cripto == nil {
		if contenido, _ := d.resolver(c.sig["Contents"]).(pdfString); !bytes.Equal(contenido, crudo) {
			return false
		}
	}
	return true
}

// --- Lo que cambió después ----------------------------------------------------

// cambiosTras compara el fichero con la versión que acaba en `fin`, que es la
// que se firmó. `anotaciones` es el /P 3 de una firma de certificación (DocMDP),
// el único nivel que admite crear, modificar y borrar anotaciones (comentarios):
// ahí no son un cambio del documento; en ninguna otra firma lo son.
//
// Se miran SOLO los objetos que ya estaban y se redefinieron después: uno nuevo
// no cambia nada mientras nadie lo enlace, y para enlazarlo hay que redefinir uno
// que ya estaba —la página, el catálogo—, que es el que se juzga.
func (d *PDFDoc) cambiosTras(fin int, anotaciones bool) CambiosPosteriores {
	c := CambiosPosteriores{BytesFirmados: fin, BytesTotales: len(d.buf), Alcance: AlcanceNinguno}
	// Se tolera el rabo de saltos que algunos escritores dejan tras %%EOF, como
	// en rangoCubreTodo; ahí no cabe un objeto.
	if fin >= len(d.buf)-8 {
		return c
	}
	// ponytail: cuenta los %%EOF del rabo; uno dentro de un flujo sin comprimir
	// sumaría de más. Es informativo: el alcance lo decide la comparación.
	c.Revisiones = bytes.Count(d.buf[fin:], []byte("%%EOF"))
	c.Alcance = AlcancePermitidos

	antes, err := LeerPDF(d.buf[:fin])
	raizNueva, _ := d.trailer["Root"].(pdfRef)
	if err != nil || c.Revisiones == 0 {
		// Bytes detrás de la firma que no forman una revisión que se pueda leer:
		// un lector que repare el fichero los vería, así que cuentan como cambio.
		c.Alcance, c.Objetos = AlcanceOtros, 1
		return c
	}
	if raizVieja, _ := antes.trailer["Root"].(pdfRef); raizVieja.Num != raizNueva.Num {
		c.Alcance, c.Objetos = AlcanceOtros, 1 // el catálogo entero, sustituido
		return c
	}

	cmp := comparador{antes: antes, despues: d, raiz: raizNueva.Num, anotaciones: anotaciones}
	paginaDe := d.paginasPorObjeto()
	paginas := map[int]bool{}
	comparados := 0
	for _, num := range d.objetosRedefinidosTras(fin) {
		if _, estaba := antes.xref[num]; !estaba {
			continue
		}
		comparados++
		if comparados > maxObjetosComparados {
			c.Objetos++
			continue
		}
		viejo, nuevo := antes.resolver(pdfRef{Num: num}), d.resolver(pdfRef{Num: num})
		if igualesPDF(viejo, nuevo, 0) || cmp.permitido(num, viejo, nuevo) {
			continue
		}
		if p, ok := paginaDe[num]; ok {
			paginas[p+1] = true
		} else {
			c.Objetos++
		}
	}
	for p := range paginas {
		c.Paginas = append(c.Paginas, p)
	}
	sort.Ints(c.Paginas)
	if len(c.Paginas) > 0 || c.Objetos > 0 {
		c.Alcance = AlcanceOtros
	}
	return c
}

// objetosRedefinidosTras son los objetos cuya definición vigente está detrás de
// `fin`, en orden.
func (d *PDFDoc) objetosRedefinidosTras(fin int) []int {
	var out []int
	for num, e := range d.xref {
		off := e.offset
		if e.enFlujo {
			off = d.xref[e.flujoNum].offset
		}
		if int(off) >= fin {
			out = append(out, num)
		}
	}
	sort.Ints(out)
	return out
}

// paginasPorObjeto: qué página es cada objeto de página y cada flujo de
// contenido, para decir «cambió la página 3» y no «cambió el objeto 41».
func (d *PDFDoc) paginasPorObjeto() map[int]int {
	out := map[int]int{}
	for i, p := range d.paginas {
		if p.num != 0 {
			out[p.num] = i
		}
		switch c := p.dict["Contents"].(type) {
		case pdfRef:
			out[c.Num] = i
		case pdfArray:
			for _, x := range c {
				if r, ok := x.(pdfRef); ok {
					out[r.Num] = i
				}
			}
		}
	}
	return out
}

// comparador juzga si un objeto redefinido después de firmar es de los cambios
// que trae firmar otra vez: el catálogo y el formulario con un campo más, la
// página con un widget de firma más, los metadatos de fecha y los datos de
// validación (/DSS). Es la lista que admite cualquier validador de PAdES; lo
// demás es un cambio del documento.
type comparador struct {
	antes, despues *PDFDoc
	raiz           int
	anotaciones    bool // DocMDP /P 3: las anotaciones se pueden crear, cambiar y borrar
}

type reglaCambio func(viejo, nuevo pdfObj) bool

func cualquiera(_, _ pdfObj) bool { return true }

func (k comparador) permitido(num int, viejo, nuevo pdfObj) bool {
	if num == k.raiz {
		return k.soloCambia(viejo, nuevo, map[pdfName]reglaCambio{
			"AcroForm":   k.formularioConFirmas,
			"DSS":        cualquiera,
			"Extensions": cualquiera,
			// Firmar otra vez con una herramienta que sube la versión del fichero
			// (pyHanko pasa de 1.7 a 2.0) y le pone o actualiza el XMP: no cambia lo
			// que se ve. Son las dos claves del catálogo, además de las de arriba,
			// que admite la política por defecto de pyHanko.
			"Version":  k.versionNoBaja,
			"Metadata": k.metadatosXMP,
		})
	}
	var dict pdfDict
	switch v := nuevo.(type) {
	case pdfDict:
		dict = v
	case *pdfStream:
		dict = v.Dict
	case pdfArray:
		return k.soloAnadeFirmas(viejo, nuevo)
	}
	// Un comentario que ya estaba y se edita: solo con /P 3, y solo si antes ya era
	// una anotación que no es un campo de formulario.
	if vd, ok := viejo.(pdfDict); ok && k.anotaciones && esAnotacionLibre(vd) && esAnotacionLibre(dict) {
		return true
	}
	switch tipo, _ := dict["Type"].(pdfName); tipo {
	case "Page":
		return k.soloCambia(viejo, nuevo, map[pdfName]reglaCambio{"Annots": k.soloAnadeFirmas})
	case "Metadata", "DSS", "VRI":
		return true
	}
	if info, ok := k.despues.trailer["Info"].(pdfRef); ok && info.Num == num {
		return k.soloCambia(viejo, nuevo, map[pdfName]reglaCambio{"ModDate": cualquiera, "Producer": cualquiera})
	}
	catalogo := k.catalogo()
	if forma, ok := catalogo["AcroForm"].(pdfRef); ok && forma.Num == num {
		return k.formularioConFirmas(viejo, nuevo)
	}
	// El /DSS no está obligado a llevar /Type: se reconoce por quién lo enlaza.
	if dss, ok := catalogo["DSS"].(pdfRef); ok && dss.Num == num {
		return true
	}
	// Un campo de firma que estaba vacío y ahora está firmado: se le añade /V.
	if vd, ok := viejo.(pdfDict); ok && vd["V"] == nil {
		if ft, _ := k.antes.resolver(vd["FT"]).(pdfName); ft == "Sig" {
			return k.soloCambia(viejo, nuevo, map[pdfName]reglaCambio{"V": k.esFirma, "AP": cualquiera})
		}
	}
	return false
}

func (k comparador) catalogo() pdfDict {
	c, _ := k.despues.resolver(pdfRef{Num: k.raiz}).(pdfDict)
	return c
}

// soloCambia: los dos diccionarios son iguales salvo en las claves con regla, y
// esas cumplen la suya.
func (k comparador) soloCambia(viejo, nuevo pdfObj, reglas map[pdfName]reglaCambio) bool {
	v, ok1 := k.antes.resolver(viejo).(pdfDict)
	n, ok2 := k.despues.resolver(nuevo).(pdfDict)
	if !ok1 || !ok2 {
		return false
	}
	for _, dic := range []pdfDict{v, n} {
		for clave := range dic {
			if igualesPDF(v[clave], n[clave], 0) {
				continue
			}
			if regla, ok := reglas[clave]; !ok || !regla(v[clave], n[clave]) {
				return false
			}
		}
	}
	return true
}

// formularioConFirmas: el /AcroForm solo gana campos de firma (y sus recursos).
func (k comparador) formularioConFirmas(viejo, nuevo pdfObj) bool {
	if viejo == nil {
		viejo = pdfDict{} // el primer campo del documento crea el formulario
	}
	return k.soloCambia(viejo, nuevo, map[pdfName]reglaCambio{
		"Fields": k.soloAnadeFirmas, "SigFlags": cualquiera, "DR": cualquiera, "DA": cualquiera,
	})
}

// soloAnadeFirmas: el array nuevo conserva todo lo que había, y lo que añade son
// campos de firma YA firmados. Uno vacío no vale: su apariencia podría tapar la
// página con lo que quisiera sin firmar nada. Con /P 3 también valen las
// anotaciones que no son campos de formulario, y se pueden quitar.
func (k comparador) soloAnadeFirmas(viejo, nuevo pdfObj) bool {
	v, _ := k.antes.resolver(viejo).(pdfArray)
	n, ok := k.despues.resolver(nuevo).(pdfArray)
	if !ok || len(n)-len(v) > maxFirmasVerificadas || (!k.anotaciones && len(n) < len(v)) {
		return false
	}
	quedan := map[int]bool{}
	for _, x := range n {
		if r, ok := x.(pdfRef); ok {
			quedan[r.Num] = true
		}
	}
	antes := map[int]bool{}
	for _, x := range v {
		r, ok := x.(pdfRef)
		if !ok {
			return false // algo directo que no se sabe seguir
		}
		if !quedan[r.Num] {
			// Se quitó algo: solo un comentario, y solo con /P 3.
			if d, _ := k.antes.resolver(x).(pdfDict); !k.anotaciones || !esAnotacionLibre(d) {
				return false
			}
			continue
		}
		antes[r.Num] = true
	}
	for _, x := range n {
		if r, ok := x.(pdfRef); ok && antes[r.Num] {
			continue
		}
		if k.esCampoDeFirma(x) {
			continue
		}
		if d, _ := k.despues.resolver(x).(pdfDict); k.anotaciones && esAnotacionLibre(d) {
			continue
		}
		return false
	}
	return true
}

// esAnotacionLibre: una anotación (tiene /Subtype) que no es un widget. Los
// widgets son los campos de formulario y las firmas; esos no entran en el permiso
// de comentar.
func esAnotacionLibre(d pdfDict) bool {
	sub, ok := d["Subtype"].(pdfName)
	return ok && sub != "Widget" && d["FT"] == nil
}

// versionNoBaja: la /Version del catálogo solo sube (o aparece).
func (k comparador) versionNoBaja(viejo, nuevo pdfObj) bool {
	n, ok := nuevo.(pdfName)
	if !ok || len(n) != 3 || n[1] != '.' {
		return false
	}
	if viejo == nil {
		return true
	}
	v, ok := viejo.(pdfName)
	return ok && len(v) == 3 && n >= v
}

// metadatosXMP: el /Metadata nuevo es una referencia a un flujo XML, que es lo
// que es un XMP. No se mira lo que dice: no se pinta.
func (k comparador) metadatosXMP(viejo, nuevo pdfObj) bool {
	if viejo != nil {
		if _, ok := viejo.(pdfRef); !ok {
			return false
		}
	}
	ref, ok := nuevo.(pdfRef)
	if !ok {
		return false
	}
	flujo, ok := k.despues.resolver(ref).(*pdfStream)
	if !ok {
		return false
	}
	tipo, _ := flujo.Dict["Type"].(pdfName)
	sub, _ := flujo.Dict["Subtype"].(pdfName)
	return tipo == "Metadata" || sub == "XML"
}

// esCampoDeFirma: un widget o campo /FT /Sig (heredado del padre) con su firma.
func (k comparador) esCampoDeFirma(o pdfObj) bool {
	cd, _ := k.despues.resolver(o).(pdfDict)
	for i := 0; cd != nil && i < 8; i++ {
		if ft, _ := k.despues.resolver(cd["FT"]).(pdfName); ft == "Sig" {
			return k.esFirma(nil, cd["V"])
		}
		cd, _ = k.despues.resolver(cd["Parent"]).(pdfDict)
	}
	return false
}

func (k comparador) esFirma(_, nuevo pdfObj) bool {
	sig, ok := k.despues.resolver(nuevo).(pdfDict)
	return ok && sig["ByteRange"] != nil && sig["Contents"] != nil
}

// igualesPDF compara dos objetos como los lee el lector. Los números se comparan
// por valor —«612» y «612.0» son el mismo, y reescribir una página los cambia de
// uno a otro— y las referencias por número, sin seguirlas: si lo apuntado cambió,
// es otro objeto redefinido y se juzga por su lado.
func igualesPDF(a, b pdfObj, prof int) bool {
	if prof > 32 {
		return false
	}
	if na, ok := numeroPDF(a); ok {
		nb, ok := numeroPDF(b)
		return ok && na == nb
	}
	switch x := a.(type) {
	case pdfArray:
		y, ok := b.(pdfArray)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !igualesPDF(x[i], y[i], prof+1) {
				return false
			}
		}
		return true
	case pdfDict:
		y, ok := b.(pdfDict)
		return ok && diccionariosIguales(x, y, prof)
	case *pdfStream:
		y, ok := b.(*pdfStream)
		return ok && bytes.Equal(x.Raw, y.Raw) && diccionariosIguales(x.Dict, y.Dict, prof)
	}
	return igualesSimples(a, b)
}

// igualesSimples compara lo que no lleva dentro otros objetos.
func igualesSimples(a, b pdfObj) bool {
	switch x := a.(type) {
	case nil, pdfNull:
		switch b.(type) {
		case nil, pdfNull:
			return true
		}
		return false
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case pdfName:
		y, ok := b.(pdfName)
		return ok && x == y
	case pdfString:
		y, ok := b.(pdfString)
		return ok && bytes.Equal(x, y)
	case pdfRef:
		y, ok := b.(pdfRef)
		return ok && x.Num == y.Num
	}
	return false
}

func diccionariosIguales(x, y pdfDict, prof int) bool {
	if len(x) != len(y) {
		return false
	}
	for k, v := range x {
		if !igualesPDF(v, y[k], prof+1) {
			return false
		}
	}
	return true
}

func numeroPDF(o pdfObj) (float64, bool) {
	switch v := o.(type) {
	case int64:
		return float64(v), true
	case float64:
		if math.IsNaN(v) {
			return 0, false
		}
		return v, true
	}
	return 0, false
}
