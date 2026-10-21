// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Cifrado y descifrado de PDF (§7.6), con el gestor de seguridad estándar.
//
// POR QUÉ ESTO NO ES OPCIONAL EN UN VISOR. La mayoría de los PDF «protegidos»
// que circulan por una empresa —una nómina, una factura, un extracto del banco—
// NO tienen contraseña de apertura: llevan contraseña de USUARIO VACÍA y solo
// declaran permisos («no imprimir», «no copiar»). Están cifrados igual. Un lector
// que rechace todo /Encrypt no puede abrirlos, y quien lo sufre no entiende por
// qué su visor de siempre sí puede. Por eso se prueba la contraseña vacía antes
// de pedir nada, y solo se pregunta cuando de verdad hace falta.
//
// QUÉ SE IMPLEMENTA. Las cuatro revisiones que existen en la práctica: RC4 de 40
// y 128 bits (R2/R3), AES-128 (R4/AESV2) y AES-256 (R5 y R6/AESV3). Todo con la
// biblioteca estándar: `crypto/rc4`, `crypto/aes`, `crypto/md5` y `crypto/sha256`
// —MD5 y RC4 están aquí porque el FORMATO los exige, no por elección: son el
// único modo de abrir un documento de 2005—.
//
// Y LOS PERMISOS. Se leen y se exponen, pero quede claro lo que son: /P es un
// campo del documento, no una barrera criptográfica. Cualquiera con la clave
// puede ignorarlo, y de hecho la clave va DENTRO del fichero. Sytue PDF los
// respeta en su interfaz porque es lo correcto con quien publicó el documento,
// no porque impidan nada.
package domain

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5" //nolint:gosec // G501: lo exige el formato PDF (§7.6.3), no es una elección
	"crypto/rc4" //nolint:gosec // G503: ídem, es el cifrado que define la especificación
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
)

// ErrPDFClaveIncorrecta distingue «está cifrado y no sé la contraseña» de «está
// roto». Son dos mensajes de interfaz muy distintos: uno pide una contraseña y
// el otro dice que el fichero no sirve.
var ErrPDFClaveIncorrecta = errors.New("pdf: contraseña incorrecta")

// Permisos son los bits de /P (§7.6.4.2, tabla 22). Los que no se usan en la
// interfaz no se exponen.
type Permisos struct {
	Imprimir       bool
	Modificar      bool
	Copiar         bool
	Anotar         bool
	RellenarCampos bool
	Accesibilidad  bool
	Ensamblar      bool
	ImprimirEnAlta bool
	// SoloConClaveDeDueno indica que se abrió con la contraseña de usuario y por
	// tanto los permisos aplican. Con la del dueño no hay restricción ninguna.
	SoloConClaveDeDueno bool
}

// relleno es la cadena de 32 bytes con la que se completa toda contraseña
// (§7.6.4.3). Está escrita en la especificación tal cual.
var relleno = []byte{
	0x28, 0xBF, 0x4E, 0x5E, 0x4E, 0x75, 0x8A, 0x41, 0x64, 0x00, 0x4E, 0x56,
	0xFF, 0xFA, 0x01, 0x08, 0x2E, 0x2E, 0x00, 0xB6, 0xD0, 0x68, 0x3E, 0x80,
	0x2F, 0x0C, 0xA9, 0xFE, 0x64, 0x53, 0x69, 0x7A,
}

type modoCifrado int

const (
	modoNinguno modoCifrado = iota
	modoRC4
	modoAES
)

// criptoPDF guarda la clave del fichero y cómo aplicarla.
type criptoPDF struct {
	clave     []byte
	revision  int
	version   int
	flujos    modoCifrado
	cadenas   modoCifrado
	permisos  Permisos
	conDueno  bool
	metadatos bool
	// adjuntos es el filtro de los ficheros adjuntos (/EFF), y porNombre el de
	// cada filtro de /CF, para el /Crypt que un flujo pida para sí mismo.
	adjuntos  modoCifrado
	porNombre map[pdfName]modoCifrado
	// sinClave: solo los adjuntos van cifrados y no se dio su contraseña. El
	// documento se lee; el adjunto, no (descifrarFlujo lo dice).
	sinClave bool
}

// prepararCripto monta el descifrado desde el /Encrypt del trailer. Devuelve nil
// sin error si el documento no está cifrado.
func (d *PDFDoc) prepararCripto(contrasena string) error {
	ref, cifrado := d.trailer["Encrypt"]
	if !cifrado {
		return nil
	}
	enc, ok := d.resolver(ref).(pdfDict)
	if !ok {
		return ErrPDFCifrado
	}
	if filtro, _ := d.resolver(enc["Filter"]).(pdfName); filtro != "" && filtro != "Standard" {
		// Un gestor de seguridad de terceros (certificados, DRM de un fabricante)
		// no se puede abrir sin su complemento, y decirlo claro vale más que
		// devolver páginas de basura.
		return ErrPDFCifrado
	}

	c := &criptoPDF{metadatos: true, version: 1, revision: 2}
	if v, ok := comoEntero(d.resolver(enc["V"])); ok {
		c.version = int(v)
	}
	if v, ok := comoEntero(d.resolver(enc["R"])); ok {
		c.revision = int(v)
	}
	if v, ok := d.resolver(enc["EncryptMetadata"]).(bool); ok {
		c.metadatos = v
	}
	longitud := 40
	if v, ok := comoEntero(d.resolver(enc["Length"])); ok && v >= 40 && v <= 256 {
		longitud = int(v)
	}
	permisos := int32(-1)
	if v, ok := comoEntero(d.resolver(enc["P"])); ok {
		// /P es un entero de 32 bits CON SIGNO (§7.6.4.2): los permisos son los bits
		// bajos y el alto está siempre a 1, así que el valor normal es negativo. La
		// conversión truncando a 32 bits es exactamente lo que pide el formato.
		permisos = int32(v & 0xFFFFFFFF) //nolint:gosec // G115: ver arriba
	}
	c.permisos = permisosDeP(permisos)

	o := []byte(cadenaDeDict(d, enc, "O"))
	u := []byte(cadenaDeDict(d, enc, "U"))
	id0 := d.primerID()

	c.flujos, c.cadenas = modoRC4, modoRC4
	if c.version >= 4 {
		c.flujos = d.modoDeFiltro(enc, "StmF")
		c.cadenas = d.modoDeFiltro(enc, "StrF")
		if n, ok := longitudDeFiltro(d, enc); ok {
			longitud = n
		}
	}
	d.filtrosConNombre(c, enc)

	var err error
	if c.version >= 5 {
		c.clave, c.conDueno, err = claveV5(d, enc, []byte(contrasena), o, u)
	} else {
		c.clave, c.conDueno, err = claveClasica(
			[]byte(contrasena), o, u, id0, permisos, c.revision, longitud, c.metadatos,
		)
	}
	if err != nil {
		// Solo van cifrados los adjuntos (/StmF y /StrF /Identity, /AuthEvent
		// /EFOpen): el documento se lee sin contraseña, y la pide quien abra uno.
		if errors.Is(err, ErrPDFClaveIncorrecta) && c.flujos == modoNinguno && c.cadenas == modoNinguno {
			c.sinClave = true
			c.permisos.SoloConClaveDeDueno = true
			d.cripto = c
			return nil
		}
		return err
	}
	c.permisos.SoloConClaveDeDueno = !c.conDueno
	d.cripto = c
	return nil
}

func cadenaDeDict(d *PDFDoc, dict pdfDict, clave pdfName) pdfString {
	s, _ := d.resolver(dict[clave]).(pdfString)
	return s
}

// primerID devuelve el primer elemento de /ID, que entra en la derivación de la
// clave. Un PDF sin /ID es raro pero legal, y entonces cuenta como vacío.
func (d *PDFDoc) primerID() []byte {
	arr, ok := d.resolver(d.trailer["ID"]).(pdfArray)
	if !ok || len(arr) == 0 {
		return nil
	}
	s, _ := d.resolver(arr[0]).(pdfString)
	return []byte(s)
}

// filtrosConNombre fija el filtro de los adjuntos (/EFF, o el de los flujos) y
// el de cada nombre de /CF, que puede pedir un flujo con su propio /Crypt.
func (d *PDFDoc) filtrosConNombre(c *criptoPDF, enc pdfDict) {
	c.adjuntos = c.flujos
	if _, hay := enc["EFF"]; hay && c.version >= 4 {
		c.adjuntos = d.modoDeFiltro(enc, "EFF")
	}
	c.porNombre = map[pdfName]modoCifrado{"Identity": modoNinguno}
	if cf, _ := d.resolver(enc["CF"]).(pdfDict); cf != nil {
		for k := range cf {
			c.porNombre[k] = d.modoDeNombre(enc, k)
		}
	}
}

// modoDeFiltro resuelve /StmF, /StrF o /EFF contra el diccionario /CF.
func (d *PDFDoc) modoDeFiltro(enc pdfDict, cual pdfName) modoCifrado {
	nombre, _ := d.resolver(enc[cual]).(pdfName)
	return d.modoDeNombre(enc, nombre)
}

func (d *PDFDoc) modoDeNombre(enc pdfDict, nombre pdfName) modoCifrado {
	if nombre == "" || nombre == "Identity" {
		// Sin filtro nombrado, /Identity: los datos NO están cifrados. Es como se
		// firma un documento dejando legible lo que la firma cubre.
		return modoNinguno
	}
	cf, _ := d.resolver(enc["CF"]).(pdfDict)
	if cf == nil {
		return modoRC4
	}
	filtro, _ := d.resolver(cf[nombre]).(pdfDict)
	if filtro == nil {
		return modoRC4
	}
	switch cfm, _ := d.resolver(filtro["CFM"]).(pdfName); cfm {
	case "AESV2", "AESV3":
		return modoAES
	case "None":
		return modoNinguno
	default:
		return modoRC4
	}
}

// longitudDeFiltro saca la longitud de clave del filtro, que en V4 manda sobre
// el /Length del diccionario exterior.
func longitudDeFiltro(d *PDFDoc, enc pdfDict) (int, bool) {
	cf, _ := d.resolver(enc["CF"]).(pdfDict)
	nombre, _ := d.resolver(enc["StmF"]).(pdfName)
	if cf == nil || nombre == "" {
		return 0, false
	}
	filtro, _ := d.resolver(cf[nombre]).(pdfDict)
	if filtro == nil {
		return 0, false
	}
	v, ok := comoEntero(d.resolver(filtro["Length"]))
	if !ok {
		return 0, false
	}
	// Unos generadores lo escriben en bytes y otros en bits. 5-32 solo tiene
	// sentido como bytes, así que se normaliza en vez de derivar una clave del
	// tamaño equivocado, que falla sin decir por qué.
	if v <= 32 {
		v *= 8
	}
	return int(v), true
}

func permisosDeP(p int32) Permisos {
	bit := func(n uint) bool { return p&(1<<(n-1)) != 0 }
	return Permisos{
		Imprimir:       bit(3),
		Modificar:      bit(4),
		Copiar:         bit(5),
		Anotar:         bit(6),
		RellenarCampos: bit(9),
		Accesibilidad:  bit(10),
		Ensamblar:      bit(11),
		ImprimirEnAlta: bit(12),
	}
}

// --- Revisiones 2 a 4: RC4 y AES-128 ----------------------------------------

// claveClasica deriva la clave del fichero probando la contraseña como de
// usuario y, si no cuadra, como de dueño.
func claveClasica(pwd, o, u, id0 []byte, permisos int32, r, longitud int, cifrarMetadatos bool) ([]byte, bool, error) {
	clave := derivarClave(pwd, o, id0, permisos, r, longitud, cifrarMetadatos)
	if verificarUsuario(clave, u, id0, r) {
		return clave, false, nil
	}
	// Contraseña de dueño: se descifra /O con ella para recuperar la de usuario,
	// y esa es la que se verifica. Es el rodeo que define la especificación.
	if usuario, ok := usuarioDesdeDueno(pwd, o, r, longitud); ok {
		clave2 := derivarClave(usuario, o, id0, permisos, r, longitud, cifrarMetadatos)
		if verificarUsuario(clave2, u, id0, r) {
			return clave2, true, nil
		}
	}
	return nil, false, ErrPDFClaveIncorrecta
}

func rellenarClave(pwd []byte) []byte {
	out := make([]byte, 32)
	n := copy(out, pwd)
	copy(out[n:], relleno)
	return out
}

// derivarClave es el algoritmo 2 de §7.6.4.3.
func derivarClave(pwd, o, id0 []byte, permisos int32, r, longitud int, cifrarMetadatos bool) []byte {
	h := md5.New() //nolint:gosec // G401: lo fija el formato
	h.Write(rellenarClave(pwd))
	if len(o) >= 32 {
		h.Write(o[:32])
	} else {
		h.Write(o)
	}
	var p [4]byte
	// Reinterpretación en complemento a dos, no un cambio de valor: la derivación
	// de la clave usa los cuatro bytes de /P tal cual están en el fichero.
	binary.LittleEndian.PutUint32(p[:], uint32(permisos)) //nolint:gosec // G115: ver arriba
	h.Write(p[:])
	h.Write(id0)
	if r >= 4 && !cifrarMetadatos {
		h.Write([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	}
	clave := h.Sum(nil)

	n := longitud / 8
	if r == 2 {
		n = 5
	}
	if n < 5 {
		n = 5
	}
	if n > 16 {
		n = 16
	}
	if r >= 3 {
		// Cincuenta vueltas de MD5 sobre los primeros n bytes: es lo que encarece
		// la fuerza bruta en las revisiones 3 y 4.
		for i := 0; i < 50; i++ {
			s := md5.Sum(clave[:n]) //nolint:gosec // G401: ídem
			clave = s[:]
		}
	}
	return clave[:n]
}

// verificarUsuario es el algoritmo 6: se recalcula /U y se compara.
func verificarUsuario(clave, u, id0 []byte, r int) bool {
	if r == 2 {
		esperado := rc4Aplicar(clave, relleno)
		return len(u) >= 32 && bytes.Equal(esperado, u[:32])
	}
	h := md5.New() //nolint:gosec // G401: ídem
	h.Write(relleno)
	h.Write(id0)
	tmp := rc4Aplicar(clave, h.Sum(nil))
	for i := 1; i <= 19; i++ {
		tmp = rc4Aplicar(clavePorXor(clave, byte(i)), tmp)
	}
	// Solo los 16 primeros bytes son significativos: los otros 16 son relleno
	// arbitrario y hay generadores que ponen cualquier cosa.
	return len(u) >= 16 && bytes.Equal(tmp[:16], u[:16])
}

// usuarioDesdeDueno es el algoritmo 7 al revés: descifra /O con la clave que
// sale de la contraseña de dueño y devuelve la contraseña de usuario.
func usuarioDesdeDueno(pwd, o []byte, r, longitud int) ([]byte, bool) {
	if len(o) < 32 {
		return nil, false
	}
	h := md5.New() //nolint:gosec // G401: ídem
	h.Write(rellenarClave(pwd))
	clave := h.Sum(nil)
	n := longitud / 8
	if r == 2 {
		n = 5
	}
	if n < 5 {
		n = 5
	}
	if n > 16 {
		n = 16
	}
	if r >= 3 {
		for i := 0; i < 50; i++ {
			s := md5.Sum(clave) //nolint:gosec // G401: ídem
			clave = s[:]
		}
	}
	clave = clave[:n]

	if r == 2 {
		return rc4Aplicar(clave, o[:32]), true
	}
	tmp := append([]byte(nil), o[:32]...)
	// Las 20 vueltas van al revés que al cifrar: de la 19 a la 0.
	for i := 19; i >= 0; i-- {
		tmp = rc4Aplicar(clavePorXor(clave, byte(i)), tmp)
	}
	return tmp, true
}

func clavePorXor(clave []byte, v byte) []byte {
	out := make([]byte, len(clave))
	for i, b := range clave {
		out[i] = b ^ v
	}
	return out
}

func rc4Aplicar(clave, datos []byte) []byte {
	c, err := rc4.NewCipher(clave) //nolint:gosec // G401: lo fija el formato
	if err != nil {
		return append([]byte(nil), datos...)
	}
	out := make([]byte, len(datos))
	c.XORKeyStream(out, datos)
	return out
}

// --- Revisiones 5 y 6: AES-256 ----------------------------------------------

// claveV5 valida la contraseña contra /U o /O y descifra la clave real del
// fichero, que en AES-256 va guardada aparte en /UE o /OE.
func claveV5(d *PDFDoc, enc pdfDict, pwd, o, u []byte) ([]byte, bool, error) {
	r := 6
	if v, ok := comoEntero(d.resolver(enc["R"])); ok {
		r = int(v)
	}
	if len(u) < 48 {
		return nil, false, ErrPDFClaveIncorrecta
	}
	// La contraseña va en UTF-8 saneada por SASLprep; se usa tal cual, que es lo
	// que hace todo el mundo y lo que acierta con cualquier contraseña ASCII.
	if len(pwd) > 127 {
		pwd = pwd[:127]
	}

	if bytes.Equal(hashV5(pwd, u[32:40], nil, r), u[:32]) {
		clave := descifrarClaveEnvuelta(hashV5(pwd, u[40:48], nil, r), []byte(cadenaDeDict(d, enc, "UE")))
		if clave == nil {
			return nil, false, ErrPDFClaveIncorrecta
		}
		return clave, false, nil
	}
	if len(o) >= 48 && bytes.Equal(hashV5(pwd, o[32:40], u[:48], r), o[:32]) {
		clave := descifrarClaveEnvuelta(hashV5(pwd, o[40:48], u[:48], r), []byte(cadenaDeDict(d, enc, "OE")))
		if clave == nil {
			return nil, false, ErrPDFClaveIncorrecta
		}
		return clave, true, nil
	}
	return nil, false, ErrPDFClaveIncorrecta
}

// hashV5 es SHA-256 en la revisión 5 y el algoritmo 2.B en la 6, que encadena
// SHA-256/384/512 en un número de vueltas que depende de los propios datos.
func hashV5(pwd, sal, datosU []byte, r int) []byte {
	h := sha256.New()
	h.Write(pwd)
	h.Write(sal)
	h.Write(datosU)
	k := h.Sum(nil)
	if r < 6 {
		return k
	}

	for vuelta := 0; ; vuelta++ {
		k1 := make([]byte, 0, 64*(len(pwd)+len(k)+len(datosU)))
		for i := 0; i < 64; i++ {
			k1 = append(k1, pwd...)
			k1 = append(k1, k...)
			k1 = append(k1, datosU...)
		}
		bloque, err := aes.NewCipher(k[:16])
		if err != nil {
			return k
		}
		e := make([]byte, len(k1)-len(k1)%aes.BlockSize)
		cipher.NewCBCEncrypter(bloque, k[16:32]).CryptBlocks(e, k1[:len(e)])
		if len(e) == 0 {
			return k[:32]
		}

		suma := 0
		for _, b := range e[:16] {
			suma += int(b)
		}
		var hh hash.Hash
		switch suma % 3 {
		case 0:
			hh = sha256.New()
		case 1:
			hh = sha512.New384()
		default:
			hh = sha512.New()
		}
		hh.Write(e)
		k = hh.Sum(nil)

		// El corte lo decide el ÚLTIMO byte del cifrado, no un contador fijo: a
		// partir de la vuelta 64 se para cuando ese byte cabe en las vueltas ya
		// dadas menos 32.
		if vuelta >= 63 && int(e[len(e)-1]) <= vuelta+1-32 {
			return k[:32]
		}
		if vuelta > 512 {
			return k[:32] // salvaguarda: datos manipulados no deben girar sin fin
		}
	}
}

// descifrarClaveEnvuelta abre /UE o /OE: AES-256 en CBC con vector de ceros y
// SIN relleno, que es como la especificación guarda la clave del fichero.
func descifrarClaveEnvuelta(intermedia, envuelta []byte) []byte {
	if len(envuelta) != 32 || len(intermedia) < 32 {
		return nil
	}
	bloque, err := aes.NewCipher(intermedia[:32])
	if err != nil {
		return nil
	}
	out := make([]byte, 32)
	cipher.NewCBCDecrypter(bloque, make([]byte, aes.BlockSize)).CryptBlocks(out, envuelta)
	return out
}

// --- Aplicación --------------------------------------------------------------

// descifrar devuelve los datos en claro de un objeto. `esCadena` distingue las
// cadenas de los flujos, que pueden llevar filtros distintos.
func (c *criptoPDF) descifrar(datos []byte, num, gen int, esCadena bool) []byte {
	modo := c.flujos
	if esCadena {
		modo = c.cadenas
	}
	return c.aplicar(modo, datos, num, gen)
}

// descifrarFlujo es descifrar para un flujo entero: un adjunto va con /EFF y un
// flujo con su propio /Crypt, con el filtro que nombre (por defecto, ninguno).
func (c *criptoPDF) descifrarFlujo(d *PDFDoc, st *pdfStream) ([]byte, error) {
	modo := c.flujos
	if t, _ := d.resolver(st.Dict["Type"]).(pdfName); t == "EmbeddedFile" {
		modo = c.adjuntos
	}
	filtros := d.listaDeNombres(st.Dict["Filter"])
	if len(filtros) > 0 && filtros[0] == "Crypt" {
		nombre := pdfName("Identity")
		parms := d.resolver(st.Dict["DecodeParms"])
		if a, ok := parms.(pdfArray); ok && len(a) > 0 {
			parms = d.resolver(a[0])
		}
		if p, ok := parms.(pdfDict); ok {
			if n, ok := d.resolver(p["Name"]).(pdfName); ok {
				nombre = n
			}
		}
		if m, ok := c.porNombre[nombre]; ok {
			modo = m
		}
	}
	if modo != modoNinguno && c.sinClave && len(st.Raw) > 0 {
		return nil, fmt.Errorf("%w: el adjunto va protegido con contraseña", ErrPDFClaveIncorrecta)
	}
	return c.aplicar(modo, st.Raw, st.Num, st.Gen), nil
}

func (c *criptoPDF) aplicar(modo modoCifrado, datos []byte, num, gen int) []byte {
	if modo == modoNinguno || len(datos) == 0 {
		return datos
	}
	clave := c.claveDeObjeto(num, gen, modo)
	if modo == modoAES {
		return descifrarAES(clave, datos)
	}
	return rc4Aplicar(clave, datos)
}

// claveDeObjeto es el algoritmo 1: cada objeto se cifra con una clave propia
// derivada de la del fichero y de su número. En AES-256 no hay tal derivación y
// se usa la clave del fichero directamente.
func (c *criptoPDF) claveDeObjeto(num, gen int, modo modoCifrado) []byte {
	if c.version >= 5 {
		return c.clave
	}
	h := md5.New() //nolint:gosec // G401: lo fija el formato
	h.Write(c.clave)
	h.Write([]byte{
		byte(num & 0xFF), byte(num >> 8 & 0xFF), byte(num >> 16 & 0xFF),
		byte(gen & 0xFF), byte(gen >> 8 & 0xFF),
	})
	if modo == modoAES {
		// La «sal» literal que pide §7.6.2 para AES-128.
		h.Write([]byte{0x73, 0x41, 0x6C, 0x54})
	}
	k := h.Sum(nil)
	n := len(c.clave) + 5
	if n > 16 {
		n = 16
	}
	return k[:n]
}

// descifrarAES abre un flujo AES-CBC: los 16 primeros bytes son el vector de
// inicialización y el resto lleva relleno PKCS#7.
func descifrarAES(clave, datos []byte) []byte {
	if len(datos) <= aes.BlockSize {
		return nil
	}
	bloque, err := aes.NewCipher(clave)
	if err != nil {
		return nil
	}
	cuerpo := datos[aes.BlockSize:]
	cuerpo = cuerpo[:len(cuerpo)-len(cuerpo)%aes.BlockSize]
	if len(cuerpo) == 0 {
		return nil
	}
	out := make([]byte, len(cuerpo))
	cipher.NewCBCDecrypter(bloque, datos[:aes.BlockSize]).CryptBlocks(out, cuerpo)

	// Relleno PKCS#7. Se valida antes de recortar: con datos manipulados, un
	// último byte de 200 dejaría un corte fuera del array.
	n := int(out[len(out)-1])
	if n < 1 || n > aes.BlockSize || n > len(out) {
		return out
	}
	return out[:len(out)-n]
}

// descifrarObjeto recorre un objeto ya parseado y descifra las cadenas que
// contenga. Los flujos no se tocan aquí: se descifran al leerlos, porque un
// documento de mil páginas no debe descomprimirse entero para contarlas.
func (d *PDFDoc) descifrarObjeto(o pdfObj, num, gen, prof int) pdfObj {
	if d.cripto == nil || prof > 32 {
		return o
	}
	switch v := o.(type) {
	case pdfString:
		return pdfString(d.cripto.descifrar([]byte(v), num, gen, true))
	case pdfArray:
		for i := range v {
			v[i] = d.descifrarObjeto(v[i], num, gen, prof+1)
		}
	case pdfDict:
		for k := range v {
			v[k] = d.descifrarObjeto(v[k], num, gen, prof+1)
		}
	case *pdfStream:
		for k := range v.Dict {
			v.Dict[k] = d.descifrarObjeto(v.Dict[k], num, gen, prof+1)
		}
	}
	return o
}

// EstaCifrado dice si el documento venía protegido. La interfaz lo usa para
// avisar de que al guardar hay que decidir qué hacer con la protección.
func (d *PDFDoc) EstaCifrado() bool { return d.cripto != nil }

// PermisosDelDocumento devuelve lo que el autor permitía. Sin cifrado, todo.
func (d *PDFDoc) PermisosDelDocumento() Permisos {
	if d.cripto == nil {
		return Permisos{
			Imprimir: true, Modificar: true, Copiar: true, Anotar: true,
			RellenarCampos: true, Accesibilidad: true, Ensamblar: true, ImprimirEnAlta: true,
		}
	}
	return d.cripto.permisos
}
