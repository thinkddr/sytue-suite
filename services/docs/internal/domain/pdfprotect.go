// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Protección de un PDF al escribirlo: contraseña de apertura, contraseña de
// permisos y qué se puede hacer con el documento. Es la otra mitad de
// `pdfcrypt.go`, que solo abre.
//
// SE ESCRIBE AES-256 Y NADA MÁS. Leer RC4 es obligatorio —hay documentos de 2005
// que abrir— pero escribirlo sería producir hoy ficheros con un cifrado roto
// desde 2001. La revisión 6 (AES-256 con SHA-2) es la del PDF 2.0 y la abre
// cualquier lector desde hace más de una década, así que no hay que elegir entre
// compatibilidad y hacerlo bien.
//
// LAS DOS CONTRASEÑAS, que se confunden constantemente:
//
//   - la de USUARIO abre el documento. Sin ella no se ve nada.
//   - la de DUEÑO solo levanta los permisos. Quien tenga la de usuario abre el
//     fichero igual, y su lector decidirá si respeta o no lo que /P declara.
//
// Por eso la interfaz no puede prometer que «no se puede copiar»: puede decir
// que el documento pide que no se copie. Es la verdad, y es lo que hace Foxit
// cuando se lee la letra pequeña.
package domain

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"strings"
)

// OpcionesProteccion describe la protección que se quiere aplicar.
type OpcionesProteccion struct {
	// ContrasenaUsuario abre el documento. Vacía = se abre sin pedir nada y solo
	// quedan los permisos, que es el caso más común en una empresa.
	ContrasenaUsuario string
	// ContrasenaDueno levanta los permisos. Vacía = se usa la de usuario, y
	// entonces quien abre el documento puede además quitarle la protección.
	ContrasenaDueno string
	// Permitir dice qué se puede hacer. El valor cero lo prohíbe todo, que es un
	// documento inútil: usa PermisosTodos() como punto de partida.
	Permitir Permisos
}

// PermisosTodos es el punto de partida razonable: se puede todo, y quien
// protege va quitando.
func PermisosTodos() Permisos {
	return Permisos{
		Imprimir: true, Modificar: true, Copiar: true, Anotar: true,
		RellenarCampos: true, Accesibilidad: true, Ensamblar: true, ImprimirEnAlta: true,
	}
}

// ProtegerPDF escribe una copia protegida del documento.
func ProtegerPDF(doc *PDFDoc, opts OpcionesProteccion) ([]byte, error) {
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	cif, err := nuevoCifrador(opts)
	if err != nil {
		return nil, err
	}
	e.cifrado = cif
	return e.Bytes()
}

// DesprotegerPDF escribe una copia SIN protección. Solo funciona sobre un
// documento ya abierto, o sea sobre uno cuya contraseña se conocía: no es un
// atajo para saltarse nada, es «guardar como» sin la contraseña.
func DesprotegerPDF(doc *PDFDoc) ([]byte, error) {
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}

// --- El cifrador -------------------------------------------------------------

// cifradorPDF guarda lo necesario para escribir un documento protegido con
// AES-256 (V5 / R6).
type cifradorPDF struct {
	clave []byte // la clave del fichero, 32 bytes
	id    []byte // /ID, 16 bytes
	u, ue []byte
	o, oe []byte
	p     int32
	// sinCifrar marca los objetos que se escriben en claro. Hoy solo el propio
	// /Encrypt, que no puede cifrarse a sí mismo.
	sinCifrar map[int]bool
}

func nuevoCifrador(opts OpcionesProteccion) (*cifradorPDF, error) {
	usuario := []byte(opts.ContrasenaUsuario)
	dueno := []byte(opts.ContrasenaDueno)
	if len(dueno) == 0 {
		dueno = usuario
	}
	if len(usuario) > 127 || len(dueno) > 127 {
		return nil, fmt.Errorf("pdf: la contraseña no puede pasar de 127 bytes")
	}

	c := &cifradorPDF{p: pDePermisos(opts.Permitir), sinCifrar: map[int]bool{}}
	// La clave del fichero es aleatoria y NO se deriva de la contraseña: la
	// contraseña solo sirve para envolverla. Así se puede cambiar sin volver a
	// cifrar el documento entero, que es para lo que se diseñó la revisión 6.
	c.clave = make([]byte, 32)
	c.id = make([]byte, 16)
	sales := make([]byte, 32) // cuatro sales de 8 bytes
	for _, b := range [][]byte{c.clave, c.id, sales} {
		if _, err := rand.Read(b); err != nil {
			return nil, fmt.Errorf("pdf: no hay entropía para cifrar: %w", err)
		}
	}
	salValU, salClaveU := sales[0:8], sales[8:16]
	salValO, salClaveO := sales[16:24], sales[24:32]

	// /U = hash(contraseña, salDeValidación) ‖ salDeValidación ‖ salDeClave.
	c.u = append(append(append([]byte{}, hashV5(usuario, salValU, nil, 6)...), salValU...), salClaveU...)
	ue, err := envolverClave(hashV5(usuario, salClaveU, nil, 6), c.clave)
	if err != nil {
		return nil, err
	}
	c.ue = ue

	// El hash del dueño incluye /U entero: así la contraseña de dueño queda atada
	// a ESTE documento y no se puede trasplantar a otro.
	c.o = append(append(append([]byte{}, hashV5(dueno, salValO, c.u, 6)...), salValO...), salClaveO...)
	oe, err := envolverClave(hashV5(dueno, salClaveO, c.u, 6), c.clave)
	if err != nil {
		return nil, err
	}
	c.oe = oe
	return c, nil
}

// envolverClave cifra la clave del fichero con la intermedia: AES-256 en CBC,
// vector de ceros y sin relleno (§7.6.4.3.3).
func envolverClave(intermedia, clave []byte) ([]byte, error) {
	bloque, err := aes.NewCipher(intermedia[:32])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 32)
	cipher.NewCBCEncrypter(bloque, make([]byte, aes.BlockSize)).CryptBlocks(out, clave) //nolint:gosec // G407: el IV a cero lo fija la norma (ISO 32000-2 §7.6.4.3.3) y la intermedia sale de una sal nueva por documento
	return out, nil
}

// diccionario devuelve el /Encrypt que va en el tráiler.
func (c *cifradorPDF) diccionario() pdfDict {
	return pdfDict{
		"Filter": pdfName("Standard"),
		"V":      int64(5),
		"R":      int64(6),
		"Length": int64(256),
		"CF": pdfDict{"StdCF": pdfDict{
			"CFM":       pdfName("AESV3"),
			"AuthEvent": pdfName("DocOpen"),
			"Length":    int64(32),
		}},
		"StmF": pdfName("StdCF"),
		"StrF": pdfName("StdCF"),
		"U":    pdfString(c.u),
		"UE":   pdfString(c.ue),
		"O":    pdfString(c.o),
		"OE":   pdfString(c.oe),
		"P":    int64(c.p),
		// /Perms es una copia de los permisos cifrada con la clave del fichero:
		// permite a un lector detectar que alguien editó /P a mano.
		"Perms":           pdfString(c.perms()),
		"EncryptMetadata": true,
	}
}

// perms construye el bloque de 16 bytes de /Perms (§7.6.4.5): los permisos, el
// marcador «adb» y relleno aleatorio, cifrado con AES-256 en ECB de un bloque.
func (c *cifradorPDF) perms() []byte {
	claro := make([]byte, 16)
	claro[0] = byte(c.p & 0xFF)
	claro[1] = byte(c.p >> 8 & 0xFF)
	claro[2] = byte(c.p >> 16 & 0xFF)
	claro[3] = byte(c.p >> 24 & 0xFF)
	claro[4], claro[5], claro[6], claro[7] = 0xFF, 0xFF, 0xFF, 0xFF
	claro[8] = 'T' // los metadatos van cifrados
	claro[9], claro[10], claro[11] = 'a', 'd', 'b'
	if _, err := rand.Read(claro[12:16]); err != nil {
		return nil
	}
	bloque, err := aes.NewCipher(c.clave)
	if err != nil {
		return nil
	}
	out := make([]byte, 16)
	bloque.Encrypt(out, claro)
	return out
}

// paraObjeto devuelve el cifrador de un objeto. En AES-256 no hay clave por
// objeto —eso era de RC4—, así que solo cambia el vector de inicialización, que
// es aleatorio en cada llamada.
func (c *cifradorPDF) paraObjeto(_ int) cifradorObjeto {
	return func(datos []byte, _ bool) []byte {
		return cifrarAES(c.clave, datos)
	}
}

// cifrarAES escribe el vector de inicialización delante y aplica relleno PKCS#7,
// que es la forma en que un PDF guarda un flujo AES.
func cifrarAES(clave, datos []byte) []byte {
	bloque, err := aes.NewCipher(clave)
	if err != nil {
		return datos
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return datos
	}
	// Relleno SIEMPRE, incluso con longitud múltiplo del bloque: sin un bloque
	// entero de relleno, al descifrar se comería 16 bytes buenos.
	falta := aes.BlockSize - len(datos)%aes.BlockSize
	relleno := make([]byte, falta)
	for i := range relleno {
		relleno[i] = byte(falta)
	}
	cuerpo := append(append([]byte{}, datos...), relleno...)

	out := make([]byte, aes.BlockSize+len(cuerpo))
	copy(out, iv)
	cipher.NewCBCEncrypter(bloque, iv).CryptBlocks(out[aes.BlockSize:], cuerpo)
	return out
}

// pDePermisos construye el entero /P. Los bits 1-2 y 7-8 están reservados y van
// a 1, y los bits altos también: /P es un entero de 32 bits CON SIGNO, y dejar
// los bits altos a cero produce un número que algunos lectores interpretan como
// «sin ningún permiso».
func pDePermisos(p Permisos) int32 {
	v := int32(-1)    // todos los bits a 1
	v &^= 1<<0 | 1<<1 // bits 1 y 2: reservados, a 0
	quitar := func(n uint, permitido bool) {
		if !permitido {
			v &^= 1 << (n - 1)
		}
	}
	quitar(3, p.Imprimir)
	quitar(4, p.Modificar)
	quitar(5, p.Copiar)
	quitar(6, p.Anotar)
	quitar(9, p.RellenarCampos)
	quitar(10, p.Accesibilidad)
	quitar(11, p.Ensamblar)
	quitar(12, p.ImprimirEnAlta)
	return v
}

// --- Saneado de metadatos ----------------------------------------------------

// huellaDelDocumento resume qué rastros lleva encima un PDF, para poder
// enseñarlo antes de compartirlo. Lo que no se puede quitar no se lista.
type huellaDelDocumento struct {
	Autor       string
	Creador     string
	Productor   string
	Titulo      string
	Asunto      string
	Palabras    string
	Creado      string
	Modificado  string
	ConXMP      bool
	ConAdjuntos bool
	ConJS       bool
}

// Huella lee los metadatos del documento. Se usa para avisar antes de compartir:
// un PDF exportado de Word lleva el nombre de quien lo escribió y la ruta de su
// carpeta, y casi nadie lo sabe.
func (d *PDFDoc) Huella() huellaDelDocumento {
	var h huellaDelDocumento
	if info, ok := d.resolver(d.trailer["Info"]).(pdfDict); ok {
		txt := func(k pdfName) string {
			s, _ := d.resolver(info[k]).(pdfString)
			return decodificarTextoPDF(s)
		}
		h.Autor, h.Creador, h.Productor = txt("Author"), txt("Creator"), txt("Producer")
		h.Titulo, h.Asunto, h.Palabras = txt("Title"), txt("Subject"), txt("Keywords")
		h.Creado, h.Modificado = txt("CreationDate"), txt("ModDate")
	}
	raiz, _ := d.resolver(d.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return h
	}
	_, h.ConXMP = raiz["Metadata"]
	if nombres, ok := d.resolver(raiz["Names"]).(pdfDict); ok {
		_, h.ConAdjuntos = nombres["EmbeddedFiles"]
		_, h.ConJS = nombres["JavaScript"]
	}
	if _, hay := raiz["OpenAction"]; hay {
		// Una acción de apertura puede ser JavaScript que se ejecuta solo con abrir.
		h.ConJS = true
	}
	return h
}

// --- Comparación de documentos (ola P8) --------------------------------------

// DiferenciaPagina es el resultado de comparar una página con su equivalente.
type DiferenciaPagina struct {
	Pagina    int      `json:"pagina"`
	Anadido   []string `json:"anadido"`
	Quitado   []string `json:"quitado"`
	SoloEnUno bool     `json:"solo_en_uno"`
}

// CompararPDF compara dos documentos línea a línea de texto y devuelve qué
// cambia en cada página.
//
// Se compara el TEXTO y no los píxeles a propósito: quien compara dos versiones
// de un contrato quiere saber qué cláusula cambió, no que el logotipo se movió
// dos puntos. Un diff visual encuentra ruido de maquetación que nadie ha pedido. Para
// la otra pregunta —¿se ve igual?— está [CompararVisual], con sus defensas contra ese
// ruido; son dos modos y no uno que sustituya al otro.
func CompararPDF(a, b *PDFDoc) ([]DiferenciaPagina, error) {
	n := a.NumPaginas()
	if b.NumPaginas() > n {
		n = b.NumPaginas()
	}
	out := make([]DiferenciaPagina, 0, n)
	for i := 0; i < n; i++ {
		dif := DiferenciaPagina{Pagina: i + 1}
		var lineasA, lineasB []string
		if i < a.NumPaginas() {
			t, err := a.TextoPlano(i)
			if err != nil {
				return nil, err
			}
			lineasA = lineasNoVacias(t)
		} else {
			dif.SoloEnUno = true
		}
		if i < b.NumPaginas() {
			t, err := b.TextoPlano(i)
			if err != nil {
				return nil, err
			}
			lineasB = lineasNoVacias(t)
		} else {
			dif.SoloEnUno = true
		}
		dif.Quitado, dif.Anadido = diferenciaDeLineas(lineasA, lineasB)
		if len(dif.Quitado) > 0 || len(dif.Anadido) > 0 || dif.SoloEnUno {
			out = append(out, dif)
		}
	}
	return out, nil
}

func lineasNoVacias(t string) []string {
	var out []string
	for _, l := range strings.Split(t, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// diferenciaDeLineas devuelve qué líneas de `a` no están en `b` y al revés.
//
// Es una diferencia por CONJUNTO con recuento, no la subsecuencia común más
// larga: mover un párrafo dentro de la misma página no debe salir como «se
// borró y se añadió lo mismo», que es el ruido que hace inútil un comparador.
func diferenciaDeLineas(a, b []string) (quitado, anadido []string) {
	cuenta := make(map[string]int, len(b))
	for _, l := range b {
		cuenta[l]++
	}
	for _, l := range a {
		if cuenta[l] > 0 {
			cuenta[l]--
			continue
		}
		quitado = append(quitado, l)
	}
	sobran := make(map[string]int, len(a))
	for _, l := range a {
		sobran[l]++
	}
	for _, l := range b {
		if sobran[l] > 0 {
			sobran[l]--
			continue
		}
		anadido = append(anadido, l)
	}
	return quitado, anadido
}

// hashDeContenido resume el texto de un documento. Sirve para decir «estos dos
// ficheros son el mismo documento» sin comparar byte a byte, que fallaría solo
// porque uno se guardó con otra herramienta.
func hashDeContenido(d *PDFDoc) string {
	h := sha256.New()
	for i := 0; i < d.NumPaginas(); i++ {
		t, err := d.TextoPlano(i)
		if err != nil {
			continue
		}
		h.Write([]byte(t))
		h.Write([]byte{0})
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:16])
}
