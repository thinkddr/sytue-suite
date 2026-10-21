// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Firma digital de PDF con certificado (ola P3 de Sytue PDF). Es lo que separa
// la edición Pro de la básica: una rúbrica dibujada dice quién dice ser el
// firmante; una firma con certificado lo DEMUESTRA y además detecta si alguien
// cambió un solo byte del documento después.
//
// CÓMO FUNCIONA, porque el mecanismo es peculiar. La firma se guarda DENTRO del
// mismo fichero que firma, así que no puede cubrirse a sí misma: se deja un
// hueco (/Contents), se declara con /ByteRange qué dos trozos del fichero sí
// entran en el resumen —todo menos el hueco— y en el hueco se mete la firma
// PKCS#7. Cambiar cualquier cosa fuera del hueco invalida el resumen; cambiar
// algo dentro del hueco destruye la propia firma.
//
// SIN DEPENDENCIAS NUEVAS. La estructura CMS (RFC 5652) se construye a mano con
// `encoding/asn1`, que es lo que la biblioteca estándar da: hay `crypto/x509`
// para el certificado y `crypto/rsa`/`crypto/ecdsa` para firmar, pero no un
// PKCS#7. Son unas doscientas líneas de ASN.1 declarativo y evitan meter una
// dependencia en el camino de una firma, que es el último sitio donde uno quiere
// código de fuera.
//
// EL PERFIL ES PAdES (ETSI EN 319 142-1), no el PKCS#7 de Adobe de antes. Se
// declara /SubFilter /ETSI.CAdES.detached y los atributos firmados son los que
// pide la línea base B-B: content-type, message-digest y signing-certificate-v2
// (RFC 5035), que ata la firma al certificado CONCRETO y no solo a su clave. Lo
// que NO va es signing-time: PAdES lo prohíbe, porque la hora que declara el
// firmante vive en la /M del diccionario y dos horas distintas en el mismo
// fichero son una ambigüedad que un validador resuelve mal. Con sello de tiempo
// de un tercero (RFC 3161, `pdftimestamp.go`) la firma sube a B-T, y entonces sí
// hay una hora que prueba algo.
//
// LO QUE ESTA FIRMA ES Y NO ES. Es una firma electrónica avanzada si el
// certificado lo es (uno cualificado de la FNMT, por ejemplo), y puede llegar a
// cualificada si además la clave vive en un dispositivo cualificado —un DNI
// electrónico, una tarjeta—, que es lo que permite la firma en dos pasos
// (`pdfsigndospasos.go`, ADR 0051): aquí nunca entra esa clave.
//
// Lo que sigue faltando para el nivel LTV completo es la información de
// REVOCACIÓN incrustada (OCSP o CRL en un diccionario /DSS). Con sello y sin
// revocación un validador ya puede fechar la firma, que es la mayor parte del
// valor, pero no comprobar sin red que el certificado no estaba revocado. La
// interfaz tiene que decir qué tiene cada firma, porque es exactamente la letra
// pequeña que nadie lee.
package domain

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"
)

// tamanoHuecoFirma es el hueco que se reserva para el CMS. Medido el 2026-09-24:
// un sello real de Izenpe pesa 4,9 KB (lleva su certificado y el de su CA) y una
// cadena de DNI electrónico de tres certificados, unos 4,5 KB; con claves de
// 4096 bits y tres certificados más en el sello se pasa de 16 KiB. Se reservan
// 24 KiB porque, en la firma en dos pasos, quedarse corto se descubre DESPUÉS de
// que la persona haya puesto su PIN, y eso no se puede deshacer. Si aun así no
// cupiera, se prefiere fallar a escribir un fichero con la firma cortada, que
// parece válido hasta que se valida.
const tamanoHuecoFirma = 24576

// OpcionesFirma describe quién firma y con qué.
type OpcionesFirma struct {
	// CertificadoPEM es el certificado del firmante y, opcionalmente detrás, la
	// cadena hasta la raíz. Todos los bloques PEM del mismo texto.
	CertificadoPEM []byte
	// ClavePEM es la clave privada (PKCS#8, PKCS#1 o SEC1).
	ClavePEM []byte
	// PKCS12 es el certificado y la clave juntos en un `.p12`/`.pfx`, que es como
	// los entregan la FNMT y las autoridades de certificación. Si viene, manda
	// sobre los dos campos de arriba.
	PKCS12 []byte
	// ClaveDelPKCS12 es la contraseña con la que se protegió ese fichero.
	ClaveDelPKCS12 string

	Nombre   string
	Motivo   string
	Lugar    string
	Contacto string
	Fecha    time.Time

	// Pagina y Rect colocan el recuadro visible. Con Rect a cero la firma es
	// INVISIBLE, que es lo correcto cuando lo que importa es la validez y no
	// enseñar un sello.
	Pagina int
	Rect   [4]float64
	// Trazos dibuja la rúbrica dentro del recuadro, si se quiere las dos cosas.
	Trazos [][][2]float64

	// Bloquear CERTIFICA el documento: después de esta firma, cualquier cambio
	// lo invalida y el lector lo dice. Es el /DocMDP del PDF con /P 1, la firma
	// de certificación de toda la vida —lo que Acrobat llama «bloquear tras
	// firmar»— y solo tiene sentido en la PRIMERA firma: certifica el documento
	// entero, así que si ya hay otra firma no se puede certificar sin invalidarla.
	//
	// Sin esto, un PDF firmado sigue admitiendo que le añadan páginas o cambien
	// un formulario por actualización incremental, y la firma anterior se sigue
	// dando por buena sobre el trozo que cubría: el lector enseña «firmado» y el
	// documento ya no es el que se firmó.
	Bloquear bool

	// Sello pide un sello de tiempo a un tercero (RFC 3161). Sin él la firma
	// prueba quién firmó pero no cuándo, así que deja de verificarse cuando el
	// certificado caduca. Ver pdftimestamp.go.
	//
	// Es una función y no una URL para que este paquete no tenga que saber de HTTP:
	// quien firma decide de dónde sale el sello y con qué plazo.
	Sello pedirSelloFn
}

var (
	// ErrCertificadoNoUsable cubre todo lo que impide firmar antes de empezar.
	ErrCertificadoNoUsable = errors.New("pdf: el certificado o la clave no sirven para firmar")
	// ErrFirmaNoCabe salta si el PKCS#7 no entra en el hueco reservado.
	ErrFirmaNoCabe = errors.New("pdf: la firma no cabe en el hueco reservado")
)

// FirmarConCertificadoPDF devuelve el documento firmado REESCRIBIÉNDOLO entero.
//
// Es el camino de reserva cuando no se puede anexar (FirmarIncrementalPDF), y
// reescribir invalida cualquier firma anterior. Por eso, si el documento ya
// lleva firmas, se niega con ErrInvalidaFirmas salvo que quien llama lo haya
// autorizado (PermitirInvalidarFirmas) tras preguntarlo.
func FirmarConCertificadoPDF(doc *PDFDoc, opts OpcionesFirma) ([]byte, error) {
	if doc.conservaFirmas() {
		return nil, fmt.Errorf("%w: no se puede firmar anexando y reescribirlo borraría las firmas que ya tiene",
			ErrInvalidaFirmas)
	}
	cadena, clave, err := cargarIdentidadDe(opts)
	if err != nil {
		return nil, err
	}
	opts = conValoresPorDefecto(opts, cadena[0])
	h, err := huecoReescrito(doc, opts)
	if err != nil {
		return nil, err
	}
	cms, err := construirCMS(h.resumen(), cadena, clave, opts.Sello)
	if err != nil {
		return nil, err
	}
	return h.incrustar(cms)
}

// conValoresPorDefecto rellena la fecha y el nombre que no vengan. El nombre sale
// del CN del certificado: es lo que dice quién firma, y la apariencia visible lo
// enseña.
func conValoresPorDefecto(opts OpcionesFirma, firmante *x509.Certificate) OpcionesFirma {
	if opts.Fecha.IsZero() {
		opts.Fecha = time.Now().UTC()
	}
	if opts.Nombre == "" {
		opts.Nombre = firmante.Subject.CommonName
	}
	return opts
}

// conHueco es un PDF con el hueco de la firma reservado y su /ByteRange ya
// escrito: todo lo que hace falta para resumirlo y, después, meter el CMS dentro.
// Es la pieza que comparten los dos caminos de una firma —reescribir o anexar— y
// los dos pasos de la firma con el certificado del sistema.
type conHueco struct {
	datos       []byte
	inicio, fin int // el hueco, con sus dos '<' '>'
}

// huecoReescrito reescribe el documento entero con el hueco. Se lleva por
// delante las firmas anteriores; ver FirmarIncrementalPDF.
func huecoReescrito(doc *PDFDoc, opts OpcionesFirma) (conHueco, error) {
	sinFirmar, err := ensamblarConHuecoDeFirma(doc, opts)
	if err != nil {
		return conHueco{}, err
	}
	inicio, fin, err := localizarHueco(sinFirmar)
	if err != nil {
		return conHueco{}, err
	}
	// /ByteRange dice qué se resume: desde el principio hasta el hueco, y desde
	// el final del hueco hasta el final del fichero. El hueco queda fuera porque
	// es donde va a caer la propia firma.
	//
	// Se escribe en el de ESTA firma —el último antes del hueco—, no en el primero
	// del fichero: si el documento ya traía firmas, sus diccionarios viajan con
	// el suyo delante y se pisaba aquel («el /ByteRange real no cabe»).
	desde := max(bytes.LastIndex(sinFirmar[:inicio], []byte("/ByteRange [")), 0)
	if err := escribirByteRange(sinFirmar[desde:], [4]int{0, inicio, fin, len(sinFirmar) - fin}); err != nil {
		return conHueco{}, err
	}
	return conHueco{datos: sinFirmar, inicio: inicio, fin: fin}, nil
}

// resumen es el SHA-256 de lo que cubre el /ByteRange: todo menos el hueco.
func (h conHueco) resumen() []byte {
	s := sha256.New()
	s.Write(h.datos[:h.inicio])
	s.Write(h.datos[h.fin:])
	return s.Sum(nil)
}

// incrustar escribe el CMS en el hueco, en hexadecimal.
func (h conHueco) incrustar(cms []byte) ([]byte, error) {
	if len(cms)*2 > h.fin-h.inicio-2 {
		return nil, fmt.Errorf("%w: hacen falta %d bytes y hay %d",
			ErrFirmaNoCabe, len(cms), (h.fin-h.inicio-2)/2)
	}
	escribirHex(h.datos[h.inicio+1:h.fin-1], cms)
	return h.datos, nil
}

// cargarIdentidad lee el certificado y la clave, y comprueba que se
// corresponden. Descubrirlo aquí y no al validar la firma ahorra un documento
// firmado con una clave que no es la del certificado, que es indetectable a
// simple vista.
// cargarIdentidadDe elige de dónde sale el certificado: del `.p12` si lo hay,
// y si no de los dos PEM.
func cargarIdentidadDe(opts OpcionesFirma) ([]*x509.Certificate, crypto.Signer, error) {
	if len(opts.PKCS12) > 0 {
		cadena, clave, err := LeerPKCS12(opts.PKCS12, opts.ClaveDelPKCS12)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrCertificadoNoUsable, err)
		}
		return cadena, clave, nil
	}
	return cargarIdentidad(opts.CertificadoPEM, opts.ClavePEM)
}

func cargarIdentidad(certPEM, clavePEM []byte) ([]*x509.Certificate, crypto.Signer, error) {
	var cadena []*x509.Certificate
	resto := certPEM
	for {
		bloque, siguiente := pem.Decode(resto)
		if bloque == nil {
			break
		}
		resto = siguiente
		if bloque.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(bloque.Bytes)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrCertificadoNoUsable, err)
		}
		cadena = append(cadena, c)
	}
	if len(cadena) == 0 {
		return nil, nil, fmt.Errorf("%w: no hay ningún bloque CERTIFICATE", ErrCertificadoNoUsable)
	}

	bloque, _ := pem.Decode(clavePEM)
	if bloque == nil {
		return nil, nil, fmt.Errorf("%w: la clave no está en formato PEM", ErrCertificadoNoUsable)
	}
	clave, err := parsearClave(bloque.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrCertificadoNoUsable, err)
	}
	// La aserción se comprueba: una clave de un tipo que no sepa compararse no es
	// motivo para tumbar el proceso, es motivo para no aceptar el certificado.
	publica, ok := clave.Public().(interface{ Equal(crypto.PublicKey) bool })
	if !ok || !publica.Equal(cadena[0].PublicKey) {
		return nil, nil, fmt.Errorf("%w: la clave no es la del certificado", ErrCertificadoNoUsable)
	}
	return cadena, clave, nil
}

// parsearClave prueba los tres formatos en que llega una clave privada. Se
// intentan todos porque el usuario no sabe cuál tiene: lo que exportó su gestor
// de certificados puede ser cualquiera de ellos.
func parsearClave(der []byte) (crypto.Signer, error) {
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		if s, ok := k.(crypto.Signer); ok {
			return s, nil
		}
		return nil, errors.New("la clave PKCS#8 no sirve para firmar")
	}
	if k, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return k, nil
	}
	if k, err := x509.ParseECPrivateKey(der); err == nil {
		return k, nil
	}
	return nil, errors.New("formato de clave privada no reconocido")
}

// --- El documento con el hueco ----------------------------------------------

// marcaByteRange son los valores de relleno de /ByteRange: diez dígitos cada uno
// para poder sustituirlos después SIN mover un solo byte, que es lo que mantiene
// válidos los desplazamientos de la tabla de referencias cruzadas.
const marcaByteRange = 9999999999

func ensamblarConHuecoDeFirma(doc *PDFDoc, opts OpcionesFirma) ([]byte, error) {
	if opts.Pagina < 0 || opts.Pagina >= doc.NumPaginas() {
		return nil, fmt.Errorf("pdf: la página %d no existe", opts.Pagina+1)
	}
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}

	numSig := e.reservar()
	e.cuerpos[numSig-1] = objetoSalida{valor: dictDeFirma(opts)}

	numCampo := e.reservar()
	campo := pdfDict{
		"Type":    pdfName("Annot"),
		"Subtype": pdfName("Widget"),
		"FT":      pdfName("Sig"),
		"T":       cadenaUTF16("Sytue.Firma1"),
		"V":       pdfRef{Num: numSig},
		"F":       int64(4),
		"Rect":    rectPDF(opts.Rect),
	}
	visible := opts.Rect[2]-opts.Rect[0] > 1 && opts.Rect[3]-opts.Rect[1] > 1
	if visible {
		campo["AP"] = pdfDict{"N": pdfRef{Num: e.aparienciaDeFirma(opts)}}
	} else {
		// Rect a cero = firma INVISIBLE. Es lo correcto cuando lo que importa es
		// la validez, no enseñar un sello: el documento se ve exactamente igual.
		campo["Rect"] = pdfArray{0.0, 0.0, 0.0, 0.0}
		numAp := e.reservar()
		e.cuerpos[numAp-1] = aparienciaVacia()
		campo["AP"] = pdfDict{"N": pdfRef{Num: numAp}}
	}
	e.cuerpos[numCampo-1] = objetoSalida{valor: campo}

	// El widget va en la página que toca, y el campo en el /AcroForm del catálogo.
	pp := &e.pendientes[opts.Pagina]
	switch ya := pp.dict["Annots"].(type) {
	case pdfArray:
		pp.dict["Annots"] = append(append(pdfArray{}, ya...), pdfRef{Num: numCampo})
	default:
		pp.dict["Annots"] = pdfArray{pdfRef{Num: numCampo}}
	}
	e.reserializarPendiente(pp)

	if e.extrasCatalogo == nil {
		e.extrasCatalogo = pdfDict{}
	}
	e.extrasCatalogo["AcroForm"] = pdfDict{
		"Fields": pdfArray{pdfRef{Num: numCampo}},
		// /SigFlags 3 = el documento tiene firmas y NO se puede guardar de forma
		// que las invalide. Sin este 3, un lector ofrece «guardar» sin avisar.
		"SigFlags": int64(3),
	}
	if opts.Bloquear {
		// /Perms es lo que convierte la firma en CERTIFICADORA: sin esta entrada
		// en el catálogo, el /DocMDP del diccionario de firma no lo mira nadie y
		// el documento se queda simplemente firmado.
		e.extrasCatalogo["Perms"] = pdfDict{"DocMDP": pdfRef{Num: numSig}}
	}
	return e.Bytes()
}

// aparienciaDeFirma reserva un número y guarda ahí la apariencia dibujada.
func (e *PDFEnsamblador) aparienciaDeFirma(opts OpcionesFirma) int {
	num := e.reservar()
	e.cuerpos[num-1] = objetoAparienciaFirma(opts)
	return num
}

// dictDeFirma es el diccionario /Sig con el hueco y el /ByteRange de relleno.
// El hueco se rellena DESPUÉS, cuando ya se sabe dónde cayó en el fichero.
func dictDeFirma(opts OpcionesFirma) pdfDict {
	d := dictDeFirmaBase(opts)
	if opts.Bloquear {
		// /P 1: no se admite NINGÚN cambio posterior. Los otros valores del
		// estándar —2 rellenar formularios, 3 además comentar— dejan puertas que
		// aquí no se quieren: quien pide bloquear pide que no se toque.
		// /V 1.2 es la versión del método de transformación, no la del PDF.
		d["Reference"] = pdfArray{pdfDict{
			"Type":            pdfName("SigRef"),
			"TransformMethod": pdfName("DocMDP"),
			"TransformParams": pdfDict{
				"Type": pdfName("TransformParams"),
				"P":    int64(1),
				"V":    pdfName("1.2"),
			},
		}}
	}
	return d
}

// El /SubFilter es PAdES y no `adbe.pkcs7.detached`: es lo que dice a un validador
// europeo que la firma sigue EN 319 142 y que tiene que exigirle sus atributos.
func dictDeFirmaBase(opts OpcionesFirma) pdfDict {
	return pdfDict{
		"Type":        pdfName("Sig"),
		"Filter":      pdfName("Adobe.PPKLite"),
		"SubFilter":   pdfName(subFiltroPAdES),
		"ByteRange":   pdfArray{int64(0), int64(marcaByteRange), int64(marcaByteRange), int64(marcaByteRange)},
		"Contents":    pdfString(make([]byte, tamanoHuecoFirma)),
		"M":           pdfString(fechaPDF(opts.Fecha)),
		"Name":        cadenaUTF16(opts.Nombre),
		"Reason":      cadenaUTF16(opts.Motivo),
		"Location":    cadenaUTF16(opts.Lugar),
		"ContactInfo": cadenaUTF16(opts.Contacto),
	}
}

// aparienciaVacia es la apariencia de la firma INVISIBLE. No dibuja nada, pero
// tiene que estar: PDF/A exige una apariencia en cada campo (ISO 19005-1 6.9,
// 19005-2 6.4.2) y sin ella firmar un BOE —PDF/A-1a— lo dejaba fuera de norma.
func aparienciaVacia() objetoSalida {
	return objetoSalida{
		valor: pdfDict{
			"Type":    pdfName("XObject"),
			"Subtype": pdfName("Form"),
			"BBox":    pdfArray{0.0, 0.0, 0.0, 0.0},
		},
		flujo: []byte{}, esFlujo: true,
	}
}

// objetoAparienciaFirma dibuja el recuadro visible: la rúbrica si la hay, el
// nombre y la fecha.
func objetoAparienciaFirma(opts OpcionesFirma) objetoSalida {
	ancho := opts.Rect[2] - opts.Rect[0]
	alto := opts.Rect[3] - opts.Rect[1]

	var b strings.Builder
	b.WriteString("q 0.98 0.98 0.97 rg 0 0 " + numPDF(ancho) + " " + numPDF(alto) + " re f\n")
	b.WriteString("0.72 0.56 0.24 RG 1 w 0.5 0.5 " + numPDF(ancho-1) + " " + numPDF(alto-1) + " re S\n")
	b.WriteString("0.06 0.09 0.16 RG 1.6 w 1 J 1 j\n")
	for _, tr := range opts.Trazos {
		// Los trazos llegan en coordenadas de la página y la apariencia dibuja en
		// su propia caja: hay que restar la esquina o la rúbrica sale fuera.
		local := make([][2]float64, 0, len(tr))
		for _, p := range tr {
			local = append(local, [2]float64{p[0] - opts.Rect[0], p[1] - opts.Rect[1]})
		}
		dibujarPolilinea(&b, local, false)
		b.WriteString("S\n")
	}
	tam := 7.0
	b.WriteString("BT /SytueFirmaF " + numPDF(tam) + " Tf 0.2 0.22 0.25 rg\n")
	fmt.Fprintf(&b, "4 %s Td (%s) Tj\n", numPDF(tam*1.4), escaparCadenaContenido("Firmado por "+opts.Nombre))
	fmt.Fprintf(&b, "0 %s Td (%s) Tj\n", numPDF(-tam*1.15),
		escaparCadenaContenido(opts.Fecha.UTC().Format("2006-01-02 15:04 UTC")))
	b.WriteString("ET Q\n")

	return objetoSalida{
		valor: pdfDict{
			"Type":      pdfName("XObject"),
			"Subtype":   pdfName("Form"),
			"FormType":  int64(1),
			"BBox":      pdfArray{0.0, 0.0, ancho, alto},
			"Resources": pdfDict{"Font": pdfDict{"SytueFirmaF": fuenteDeCampo()}},
		},
		flujo:   []byte(b.String()),
		esFlujo: true,
	}
}

// marcaHueco identifica el /Contents reservado por su relleno. NO se busca por
// cercanía al /SubFilter porque las claves de un diccionario se escriben en
// orden alfabético y /SubFilter cae DETRÁS de /Contents; y no se busca solo
// «/Contents <» porque cualquier flujo puede tener uno. Sesenta y cuatro ceros
// seguidos tras la marca son inequívocos.
var marcaHueco = append([]byte("/Contents <"), bytes.Repeat([]byte("0"), 64)...)

// subFiltroPAdES es el /SubFilter de una firma PAdES (ETSI EN 319 142-1).
const subFiltroPAdES = "ETSI.CAdES.detached"

// localizarHueco encuentra la cadena hexadecimal reservada para la firma.
func localizarHueco(datos []byte) (int, int, error) {
	if !bytes.Contains(datos, []byte("/SubFilter /"+subFiltroPAdES)) {
		return 0, 0, fmt.Errorf("pdf: no se encontró el diccionario de firma")
	}
	i := bytes.Index(datos, marcaHueco)
	if i < 0 {
		return 0, 0, fmt.Errorf("pdf: el diccionario de firma no tiene hueco /Contents")
	}
	inicio := i + len("/Contents ")
	relFin := bytes.IndexByte(datos[inicio:], '>')
	if relFin < 0 {
		return 0, 0, fmt.Errorf("pdf: el hueco /Contents no está cerrado")
	}
	return inicio, inicio + relFin + 1, nil
}

// escribirByteRange sustituye los valores de relleno por los reales SIN cambiar
// la longitud: se rellena con espacios, que dentro de un array de PDF son
// separador válido. Un solo byte de más movería todos los desplazamientos de la
// tabla xref y el fichero dejaría de abrir.
func escribirByteRange(datos []byte, rango [4]int) error {
	marca := bytes.Index(datos, []byte("/ByteRange ["))
	if marca < 0 {
		return fmt.Errorf("pdf: no se encontró /ByteRange")
	}
	inicio := marca + len("/ByteRange [")
	relFin := bytes.IndexByte(datos[inicio:], ']')
	if relFin < 0 {
		return fmt.Errorf("pdf: /ByteRange sin cerrar")
	}
	hueco := datos[inicio : inicio+relFin]
	texto := fmt.Sprintf("%d %d %d %d", rango[0], rango[1], rango[2], rango[3])
	if len(texto) > len(hueco) {
		return fmt.Errorf("pdf: el /ByteRange real no cabe en el reservado")
	}
	copy(hueco, texto)
	for i := len(texto); i < len(hueco); i++ {
		hueco[i] = ' '
	}
	return nil
}

// escribirHex vuelca los bytes como hexadecimal en el hueco y rellena el resto
// con ceros, que es lo que espera un validador.
func escribirHex(destino, datos []byte) {
	const digitos = "0123456789ABCDEF"
	for i, b := range datos {
		if 2*i+1 >= len(destino) {
			return
		}
		destino[2*i] = digitos[b>>4]
		destino[2*i+1] = digitos[b&0x0F]
	}
	for i := 2 * len(datos); i < len(destino); i++ {
		destino[i] = '0'
	}
}

// --- CMS / PKCS#7 ------------------------------------------------------------

// Identificadores de objeto de RFC 5652, RFC 5035 y RFC 8017, escritos aquí
// porque `crypto/x509` no los exporta.
var (
	oidDatosPKCS7    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidFirmadoPKCS7  = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidTipoContenido = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidResumen       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	// oidCertificadoFirmante es signing-certificate-v2 (RFC 5035 §3): la huella
	// del certificado que firma, DENTRO de lo firmado. Sin él, otro certificado
	// con la misma clave pública pasaría por el firmante, y PAdES lo exige.
	oidCertificadoFirmante = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	oidSHA256              = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSA                 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
	oidECDSAConSHA256      = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
)

// contenidoInfo es el envoltorio de RFC 5652 §3. El [0] EXPLICIT se construye a
// mano y NO con la opción `explicit` del paquete: cuando un asn1.RawValue trae
// `FullBytes`, el codificador lo escribe TAL CUAL y se salta el envoltorio, así
// que el documento salía sin la etiqueta [0] y ningún validador lo reconocía.
type contenidoInfo struct {
	Tipo      asn1.ObjectIdentifier
	Contenido asn1.RawValue `asn1:"optional,tag:0"`
}

type datosFirmados struct {
	Version           int
	AlgoritmosResumen []pkix.AlgorithmIdentifier `asn1:"set"`
	Contenido         contenidoInfoVacio
	Certificados      asn1.RawValue  `asn1:"optional,tag:0"`
	Firmantes         []infoFirmante `asn1:"set"`
}

// contenidoInfoVacio es el contenido «desprendido»: se declara el tipo y NO se
// incluye el dato, porque el dato es el PDF y ya está en el propio fichero.
type contenidoInfoVacio struct {
	Tipo asn1.ObjectIdentifier
}

type emisorYSerie struct {
	// El nombre del emisor va COMO VIENE en el certificado, sin volver a
	// codificarlo: reconstruir un DN puede cambiar sus bytes y entonces el
	// validador no encuentra el certificado que corresponde a la firma.
	Emisor asn1.RawValue
	Serie  *big.Int
}

type atributo struct {
	Tipo    asn1.ObjectIdentifier
	Valores []asn1.RawValue `asn1:"set"`
}

type infoFirmante struct {
	Version           int
	Emisor            emisorYSerie
	AlgoritmoResumen  pkix.AlgorithmIdentifier
	AtributosFirmados []atributo `asn1:"tag:0,implicit,set"`
	AlgoritmoFirma    pkix.AlgorithmIdentifier
	Firma             []byte
	// AtributosNoFirmados es donde vive el sello de tiempo (RFC 3161). Tiene que
	// ir aquí y no arriba por una razón de orden: el sello se calcula SOBRE la
	// firma, así que no existe hasta que la firma está hecha y es imposible
	// firmarlo. Un validador lo sabe y lo comprueba aparte.
	AtributosNoFirmados []atributo `asn1:"optional,tag:1,implicit,set"`
}

// idCertificadoV2 es ESSCertIDv2 (RFC 5035). El algoritmo de la huella NO se
// escribe: su valor por defecto es SHA-256, y en DER un valor por defecto se omite;
// escribirlo daría una codificación que un validador estricto rechaza.
type idCertificadoV2 struct {
	Huella []byte
	Emisor emisorYSerieESS
}

// emisorYSerieESS es IssuerSerial: el emisor como GeneralNames (un solo
// directoryName) y la serie. Es la segunda mitad de la atadura: la huella dice
// QUÉ certificado, esto dice DE QUIÉN, y los dos van firmados.
type emisorYSerieESS struct {
	Nombres asn1.RawValue
	Serie   *big.Int
}

type certificadoFirmanteV2 struct {
	Certs []idCertificadoV2
}

// pedirSelloFn, si no es nil, se llama con el resumen de la FIRMA para obtener
// el token RFC 3161. Se pasa como función y no como URL para que el dominio no
// tenga que saber de HTTP ni de contextos: quien firma decide de dónde sale el
// sello.
type pedirSelloFn func(resumenDeFirma []byte) ([]byte, error)

// construirCMS arma el CMS desprendido que va en el hueco, firmando con una clave
// que está AQUÍ (un .p12 o un PEM). La firma en dos pasos usa las mismas dos
// mitades —prepararAtributos y montarCMS— con la firma hecha fuera.
func construirCMS(resumen []byte, cadena []*x509.Certificate, clave crypto.Signer, sello pedirSelloFn) ([]byte, error) {
	atributos, porFirmar, err := prepararAtributos(resumen, cadena[0])
	if err != nil {
		return nil, err
	}
	huella := sha256.Sum256(porFirmar)
	firma, err := clave.Sign(rand.Reader, huella[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("pdf: no se pudo firmar: %w", err)
	}
	// Un fallo del sello aquí NO tumba la firma: se emite sin sello. Es
	// deliberado — una autoridad caída no debe impedir firmar un contrato, y la
	// interfaz ya dice cuándo una firma tiene sello y cuándo no (ADR-0030). La
	// firma en dos pasos decide distinto, y lo decide quien firma: ver
	// CompletarFirmaPDF.
	var token []byte
	if sello != nil {
		if t, err := sello(huellaDeFirma(firma)); err == nil && len(t) > 0 {
			token = t
		}
	}
	return montarCMS(atributos, firma, cadena, token)
}

// prepararAtributos devuelve los atributos firmados y su DER, que es lo que de
// verdad se firma: NO el resumen del documento, sino el de los ATRIBUTOS —que lo
// contienen—. Es lo que ata el tipo de contenido y el certificado a la firma en
// vez de dejarlos como adorno.
func prepararAtributos(resumen []byte, firmante *x509.Certificate) ([]atributo, []byte, error) {
	atributos, err := atributosFirmados(resumen, firmante)
	if err != nil {
		return nil, nil, err
	}
	porFirmar, err := derDeAtributos(atributos)
	if err != nil {
		return nil, nil, err
	}
	return atributos, porFirmar, nil
}

// huellaDeFirma es lo que se manda a sellar: el resumen de la FIRMA, no del
// documento. Es lo que ata «esta firma concreta existía a esta hora».
func huellaDeFirma(firma []byte) []byte {
	h := sha256.Sum256(firma)
	return h[:]
}

// algoritmoDeFirma dice cómo está hecha la firma a partir de la clave pública
// del certificado. Solo RSA (PKCS#1 v1.5) y ECDSA: son los que emiten el DNI
// electrónico, las tarjetas y los almacenes de los tres sistemas.
func algoritmoDeFirma(publica crypto.PublicKey) (pkix.AlgorithmIdentifier, error) {
	switch publica.(type) {
	case *rsa.PublicKey:
		return pkix.AlgorithmIdentifier{Algorithm: oidRSA, Parameters: asn1.NullRawValue}, nil
	case *ecdsa.PublicKey:
		return pkix.AlgorithmIdentifier{Algorithm: oidECDSAConSHA256}, nil
	}
	return pkix.AlgorithmIdentifier{}, fmt.Errorf("%w: solo se firma con RSA o ECDSA", ErrCertificadoNoUsable)
}

// montarCMS arma el SignedData con una firma YA hecha. `token` es el sello de
// tiempo, o nil.
func montarCMS(atributos []atributo, firma []byte, cadena []*x509.Certificate, token []byte) ([]byte, error) {
	algFirma, err := algoritmoDeFirma(cadena[0].PublicKey)
	if err != nil {
		return nil, err
	}
	var certsDER []byte
	for _, c := range cadena {
		certsDER = append(certsDER, c.Raw...)
	}
	var noFirmados []atributo
	if len(token) > 0 {
		noFirmados = append(noFirmados, atributoDeSello(token))
	}

	firmado := datosFirmados{
		Version:           1,
		AlgoritmosResumen: []pkix.AlgorithmIdentifier{{Algorithm: oidSHA256, Parameters: asn1.NullRawValue}},
		Contenido:         contenidoInfoVacio{Tipo: oidDatosPKCS7},
		Certificados:      asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: certsDER},
		Firmantes: []infoFirmante{{
			Version: 1,
			Emisor: emisorYSerie{
				Emisor: asn1.RawValue{FullBytes: cadena[0].RawIssuer},
				Serie:  cadena[0].SerialNumber,
			},
			AlgoritmoResumen:    pkix.AlgorithmIdentifier{Algorithm: oidSHA256, Parameters: asn1.NullRawValue},
			AtributosFirmados:   atributos,
			AlgoritmoFirma:      algFirma,
			Firma:               firma,
			AtributosNoFirmados: noFirmados,
		}},
	}

	cuerpo, err := asn1.Marshal(firmado)
	if err != nil {
		return nil, fmt.Errorf("pdf: no se pudo codificar el CMS: %w", err)
	}
	return asn1.Marshal(contenidoInfo{
		Tipo: oidFirmadoPKCS7,
		Contenido: asn1.RawValue{
			Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: cuerpo,
		},
	})
}

// atributosFirmados son los tres de PAdES B-B, ya en el orden de DER.
func atributosFirmados(resumen []byte, firmante *x509.Certificate) ([]atributo, error) {
	tipoDER, err := asn1.Marshal(oidDatosPKCS7)
	if err != nil {
		return nil, err
	}
	resumenDER, err := asn1.Marshal(resumen)
	if err != nil {
		return nil, err
	}
	certDER, err := certificadoFirmanteDER(firmante)
	if err != nil {
		return nil, err
	}
	atributos := []atributo{
		{Tipo: oidTipoContenido, Valores: []asn1.RawValue{{FullBytes: tipoDER}}},
		{Tipo: oidResumen, Valores: []asn1.RawValue{{FullBytes: resumenDER}}},
		{Tipo: oidCertificadoFirmante, Valores: []asn1.RawValue{{FullBytes: certDER}}},
	}
	return ordenarComoDER(atributos)
}

// certificadoFirmanteDER codifica signing-certificate-v2 con la huella SHA-256
// del certificado y su IssuerSerial.
func certificadoFirmanteDER(c *x509.Certificate) ([]byte, error) {
	// GeneralNames es SEQUENCE OF GeneralName, y el directoryName es [4] con el
	// Name dentro: un CHOICE etiquetado es siempre EXPLICIT, así que el Name va
	// entero, como viene en el certificado.
	directorio, err := asn1.Marshal(asn1.RawValue{
		Class: asn1.ClassContextSpecific, Tag: 4, IsCompound: true, Bytes: c.RawIssuer,
	})
	if err != nil {
		return nil, err
	}
	huella := sha256.Sum256(c.Raw)
	return asn1.Marshal(certificadoFirmanteV2{Certs: []idCertificadoV2{{
		Huella: huella[:],
		Emisor: emisorYSerieESS{
			Nombres: asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: directorio},
			Serie:   c.SerialNumber,
		},
	}}})
}

// ordenarComoDER pone los atributos en el orden que exige DER a un SET OF: por
// sus codificaciones, de menor a mayor. No es pedantería: un validador que
// vuelve a codificar el conjunto en DER antes de comprobar la firma —BouncyCastle,
// que es lo que hay debajo del validador de la Comisión Europea— lo reordena, y
// si nosotros no lo firmamos ya ordenado, su resumen sale distinto del nuestro.
func ordenarComoDER(atributos []atributo) ([]atributo, error) {
	cods := make([][]byte, len(atributos))
	for i, a := range atributos {
		der, err := asn1.Marshal(a)
		if err != nil {
			return nil, err
		}
		cods[i] = der
	}
	orden := make([]int, len(atributos))
	for i := range orden {
		orden[i] = i
	}
	sort.SliceStable(orden, func(i, j int) bool { return bytes.Compare(cods[orden[i]], cods[orden[j]]) < 0 })
	out := make([]atributo, len(atributos))
	for i, k := range orden {
		out[i] = atributos[k]
	}
	return out, nil
}

// derDeAtributos codifica los atributos como SET OF, que es lo que hay que
// firmar (RFC 5652 §5.4), no como el [0] IMPLICIT con el que viajan dentro del
// SignerInfo. Go serializa una lista como SEQUENCE OF (0x30); el SET OF (0x31)
// tiene EXACTAMENTE la misma codificación salvo esa etiqueta, así que se
// cambia el primer byte. Es el atajo de siempre y es correcto porque la longitud
// no cambia — pero conviene que esté dicho, porque leído a secas parece un
// parcheo a ciegas.
func derDeAtributos(atributos []atributo) ([]byte, error) {
	der, err := asn1.Marshal(atributos)
	if err != nil {
		return nil, err
	}
	if len(der) == 0 || der[0] != 0x30 {
		return nil, fmt.Errorf("pdf: codificación de atributos inesperada")
	}
	out := append([]byte(nil), der...)
	out[0] = 0x31
	return out, nil
}

// --- Lectura de firmas -------------------------------------------------------

// FirmaLeida describe una firma encontrada en el documento.
type FirmaLeida struct {
	Nombre string `json:"nombre,omitempty"`
	Motivo string `json:"motivo,omitempty"`
	Lugar  string `json:"lugar,omitempty"`
	Fecha  string `json:"fecha,omitempty"`
	Campo  string `json:"campo,omitempty"`
	// CubreTodo dice si el /ByteRange abarca el fichero entero. Si no, alguien
	// añadió contenido DESPUÉS de firmar y ese añadido no está firmado, que es
	// exactamente el truco con el que se cuela una cláusula en un contrato.
	CubreTodo bool `json:"cubre_todo"`
}

// FirmasDelDocumento lista lo que DECLARA cada firma del documento, y si cubre el
// fichero entero —que es lo que más engaña y sale de la estructura, sin
// criptografía—. La verificación de verdad (integridad, cadena, sello y cambios
// posteriores) es VerificarFirmas, en pdfverificar.go.
func (d *PDFDoc) FirmasDelDocumento() []FirmaLeida {
	campos := d.camposDeFirma()
	out := make([]FirmaLeida, 0, len(campos))
	for _, c := range campos {
		out = append(out, d.firmaLeida(c))
	}
	return out
}

// rangoCubreTodo comprueba que /ByteRange llega hasta el final del fichero.
func (d *PDFDoc) rangoCubreTodo(sig pdfDict) bool {
	arr, ok := d.resolver(sig["ByteRange"]).(pdfArray)
	if !ok || len(arr) < 4 {
		return false
	}
	inicio, _ := comoEntero(d.resolver(arr[2]))
	largo, _ := comoEntero(d.resolver(arr[3]))
	fin := inicio + largo
	// Se tolera el rabo de espacios y saltos que algunos escritores dejan tras
	// %%EOF; más de eso es contenido añadido después de firmar.
	return int(fin) >= len(d.buf)-8
}
