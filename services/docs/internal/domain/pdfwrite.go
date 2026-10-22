// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Ensamblador de PDF: toma páginas de uno o varios documentos ya abiertos por
// `pdfread.go` y escribe un PDF nuevo con ellas. Es la base de la ola P4 del
// roadmap —combinar, dividir, reordenar, rotar, borrar y extraer páginas— y de
// todo lo que después necesite reescribir el fichero.
//
// Lo caro aquí no es cortar páginas: es COPIAR EL GRAFO que cada página arrastra.
// Una página no se basta a sí misma —sus fuentes, imágenes, XObjects y espacios
// de color son referencias indirectas a otros objetos, que a su vez referencian
// más— así que copiar una página es recorrer su grafo entero, traer cada objeto
// alcanzable y renumerarlo en el documento destino. Copiar solo el diccionario
// de la página produce un PDF que abre y sale en blanco.
//
// Dos cortes deliberados en ese recorrido:
//
//   - **/Parent**, que apunta hacia ARRIBA. Seguirlo arrastraría el árbol de
//     páginas del origen entero —incluidas las páginas que no se querían— y
//     además cerraría un ciclo. Se sustituye por el /Pages del destino.
//   - **/Annots** se conserva, porque ahí viven las anotaciones y los campos de
//     formulario: perderlos al reordenar páginas sería destruir trabajo.
//
// Sin dependencias nuevas, como el resto del motor: los flujos se copian tal
// cual, sin descomprimir ni recomprimir, que además de barato evita degradar
// imágenes en cada guardado.
package domain

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"strconv"
)

// objetoSalida es un objeto del documento en construcción SIN serializar. Se
// guarda el valor y no sus bytes porque la serialización es lo último que puede
// pasar: al cifrar, cada cadena y cada flujo se transforman con una clave que
// depende del número de objeto, y sobre bytes ya escritos no hay forma de
// distinguir una cadena de un nombre.
type objetoSalida struct {
	valor pdfObj
	// flujo son los datos del stream ya en CLARO. nil = el objeto no es un stream.
	flujo []byte
	// esFlujo distingue un stream de cuerpo vacío de un objeto que no lo es.
	esFlujo bool
}

// PDFEnsamblador construye un PDF nuevo a partir de páginas de otros.
type PDFEnsamblador struct {
	// cuerpos[i] es el objeto número i+1.
	cuerpos []objetoSalida
	// cifrado, si no es nil, protege el documento resultante. Ver pdfprotect.go.
	cifrado *cifradorPDF
	// paginas son los números de objeto de las páginas añadidas, en orden.
	paginas []int
	// copiados evita duplicar un objeto compartido entre páginas (la fuente que
	// usan las veinte páginas de un documento se copia UNA vez).
	copiados map[origenObj]int
	// pendientes conserva el diccionario de cada página para poder modificarlo
	// después de copiarla (superponer una capa).
	pendientes []paginaPendiente
	// extrasCatalogo son entradas que se añaden al /Catalog al cerrar: /AcroForm
	// de los formularios, /OCProperties de las capas, /MarkInfo del etiquetado.
	extrasCatalogo pdfDict
	// parches modifican objetos concretos al copiarlos. Ver Parchear.
	parches map[origenObj]parcheObjeto
	// comprimir, si no es nil, pasa cada flujo copiado por el optimizador.
	comprimir *ajustesCompresion
	// reemplazos sustituye el CUERPO de un flujo del origen por otros bytes
	// (número de objeto en el origen → datos nuevos). Lo usa la sustitución de
	// imágenes: cambiar el diccionario con un parche y dejar el cuerpo viejo
	// produciría un objeto que se contradice a sí mismo.
	reemplazos map[int][]byte
	// paginaReservada son los números apartados para una página del origen que
	// alguien referencia (un enlace, un marcador, el /P de un widget) antes de
	// que se añada, o aunque no se añada nunca. Ver copiarObjeto.
	paginaReservada map[int]bool
	// sinCatalogo impide conservar el catálogo de origen. Lo piden censurar y
	// sanear, que existen para que NO viaje nada que no se haya pedido.
	sinCatalogo bool
	// excluidos son objetos del origen que NO viajan aunque algo los enlace: un
	// campo que censurar quita, y que su hermano conservado nombraría por /Kids.
	// La referencia queda a null, y en un array desaparece.
	excluidos map[origenObj]bool
	// soloCatalogo, si no es nil, deja conservar del catálogo SOLO lo que
	// acepta. Lo pide censurar (catalogoSinTexto).
	soloCatalogo func(doc *PDFDoc, k pdfName, v pdfObj) bool
	// fallo es el primer error al copiar algo que no se debe escribir mal: Bytes
	// lo devuelve en vez de un fichero.
	fallo error
	// origenAnexo es el documento sobre el que se anexa (pdfanexo.go): lo que no
	// está entre los objetos nuevos se lee de él.
	origenAnexo *PDFDoc
}

// parcheObjeto modifica un objeto ya copiado, justo antes de escribirlo. Recibe
// el ensamblador para poder crear objetos nuevos (una apariencia, por ejemplo).
type parcheObjeto func(e *PDFEnsamblador, dict pdfDict)

// origenObj identifica un objeto por documento de origen y número, para que
// combinar dos PDF distintos no confunda su objeto 5 con el objeto 5 del otro.
type origenObj struct {
	doc *PDFDoc
	num int
}

// NuevoEnsambladorPDF crea un ensamblador vacío.
func NuevoEnsambladorPDF() *PDFEnsamblador {
	return &PDFEnsamblador{copiados: map[origenObj]int{}}
}

// NumPaginas indica cuántas páginas se han añadido.
func (e *PDFEnsamblador) NumPaginas() int { return len(e.paginas) }

// reservar aparta un número de objeto y devuelve su índice (1-based). El cuerpo
// se rellena después: hace falta reservar ANTES de recorrer el grafo para que un
// objeto que se referencia a sí mismo por un ciclo no se copie sin fin.
func (e *PDFEnsamblador) reservar() int {
	e.cuerpos = append(e.cuerpos, objetoSalida{valor: pdfNull{}})
	return len(e.cuerpos)
}

// opcionesPagina ajusta cómo se copia una página. El valor cero es «cópiala tal
// cual», que es lo que quiere todo el mundo menos la censura.
type opcionesPagina struct {
	// contenido, si no es nil, SUSTITUYE el flujo de contenido de la página y el
	// original no se copia. Ese «no se copia» es la mitad de la garantía de
	// RedactarPDF: un objeto que se copia y luego se deja sin apuntar se escribe
	// igual en el fichero, así que el texto censurado seguiría dentro y a un
	// editor hexadecimal de distancia.
	contenido []byte
	// annots, si no es nil, sustituye la lista de anotaciones (en objetos del
	// documento ORIGEN, todavía sin copiar). Vacía deja la página sin ninguna.
	annots pdfArray
	// recursos, si no es nil, SUSTITUYE los /Resources de la página y los de origen
	// no se copian (en el destino ya, con sus referencias resueltas). Los originales
	// pueden ser compartidos entre páginas y llevar una imagen que se ha rehecho.
	recursos pdfDict
}

// AnadirPagina copia la página `indice` de `doc` al documento en construcción.
// La rotación se aplica sobre la que ya tuviera la página (0, 90, 180 o 270).
func (e *PDFEnsamblador) AnadirPagina(doc *PDFDoc, indice, rotacionExtra int) error {
	return e.anadirPagina(doc, indice, rotacionExtra, opcionesPagina{})
}

func (e *PDFEnsamblador) anadirPagina(doc *PDFDoc, indice, rotacionExtra int, opts opcionesPagina) error {
	p, err := doc.Pagina(indice)
	if err != nil {
		return err
	}

	num := e.numeroDePagina(doc, p)
	e.paginas = append(e.paginas, num)

	// El diccionario de la página se copia entrada por entrada, saltando lo que
	// no debe viajar y resolviendo el resto del grafo.
	dst := pdfDict{}
	for _, k := range clavesOrdenadas(p.dict) {
		switch {
		case claveQueNoViaja(k):
			continue
		case k == "Rotate":
			continue // se recalcula abajo
		case k == "Contents" && opts.contenido != nil:
			continue // lo sustituye el de opts, y el original NO se copia
		case k == "Thumb" && opts.contenido != nil:
			continue // la miniatura es una foto de la página ANTERIOR al cambio
		case k == "Resources" && opts.recursos != nil:
			dst[k] = opts.recursos
			continue
		case k == "Annots" && opts.annots != nil:
			if len(opts.annots) > 0 {
				dst[k] = e.copiarValor(doc, opts.annots)
			}
			continue
		}
		dst[k] = e.copiarValor(doc, p.dict[k])
	}
	dst["Type"] = pdfName("Page")
	if opts.contenido != nil {
		numFlujo := e.reservar()
		e.cuerpos[numFlujo-1] = objetoSalida{valor: pdfDict{}, flujo: opts.contenido, esFlujo: true}
		dst["Contents"] = pdfRef{Num: numFlujo}
	}

	// MediaBox explícito: en el origen podía venir heredado del nodo /Pages, y
	// ese nodo no viaja. Sin esto la página saldría con el tamaño por defecto.
	if _, hay := dst["MediaBox"]; !hay {
		ancho, alto := p.Ancho, p.Alto
		if p.Rotacion == 90 || p.Rotacion == 270 {
			ancho, alto = alto, ancho // MediaBox va SIN rotar
		}
		dst["MediaBox"] = pdfArray{int64(0), int64(0), ancho, alto}
	}
	if r := ((p.Rotacion+rotacionExtra)%360 + 360) % 360; r != 0 {
		dst["Rotate"] = int64(r)
	}

	e.pendientes = append(e.pendientes, paginaPendiente{num: num, dict: dst, origen: doc, p: p})
	e.cuerpos[num-1] = objetoSalida{valor: dst}
	return nil
}

// numeroDePagina da el número de objeto de la página en el documento nuevo.
//
// La página se registra como cualquier otro objeto copiado: así un enlace, un
// marcador o el /P de un widget que la nombran apuntan a ESTA página y no a una
// copia suelta fuera del árbol, que es lo que pasaba —el enlace del índice
// llevaba a una página que ningún lector enseña—. Si alguien la nombró antes de
// añadirla, se usa el número que copiarObjeto le reservó.
func (e *PDFEnsamblador) numeroDePagina(doc *PDFDoc, p PDFPagina) int {
	llave := origenObj{doc: doc, num: p.num}
	num, yaReservada := e.copiados[llave]
	if p.num == 0 || !yaReservada || !e.paginaReservada[num] {
		// Sin número de origen, la primera vez, o la misma página añadida dos
		// veces (la segunda es una copia más y no se registra).
		num = e.reservar()
		if p.num != 0 && !yaReservada {
			e.copiados[llave] = num
		}
	}
	delete(e.paginaReservada, num)
	return num
}

// paginaPendiente guarda la página copiada SIN serializar del todo, para que una
// capa posterior pueda tocar su diccionario en vez de parchear bytes. Editar el
// texto ya serializado parecía más simple y no lo era: cuando /Resources es una
// referencia indirecta, inyectar otra entrada produce la clave DUPLICADA y el
// recurso nuevo se pierde sin dar ningún error.
type paginaPendiente struct {
	num    int
	dict   pdfDict
	origen *PDFDoc
	p      PDFPagina
}

// ultimaPendiente devuelve la última página añadida, para modificarla.
func (e *PDFEnsamblador) ultimaPendiente() *paginaPendiente {
	if len(e.pendientes) == 0 {
		return nil
	}
	return &e.pendientes[len(e.pendientes)-1]
}

// reserializarPendiente vuelve a escribir el cuerpo de una página tras tocarla.
func (e *PDFEnsamblador) reserializarPendiente(pp *paginaPendiente) {
	e.cuerpos[pp.num-1] = objetoSalida{valor: pp.dict}
}

// copiarValor copia un objeto del origen al destino, siguiendo las referencias.
func (e *PDFEnsamblador) copiarValor(doc *PDFDoc, v pdfObj) pdfObj {
	switch t := v.(type) {
	case pdfRef:
		if e.excluidos[origenObj{doc: doc, num: t.Num}] {
			return pdfNull{}
		}
		return pdfRef{Num: e.copiarObjeto(doc, t.Num), Gen: 0}
	case pdfArray:
		out := make(pdfArray, 0, len(t))
		for _, x := range t {
			if r, ok := x.(pdfRef); ok && e.excluidos[origenObj{doc: doc, num: r.Num}] {
				continue
			}
			out = append(out, e.copiarValor(doc, x))
		}
		return out
	case pdfDict:
		out := pdfDict{}
		// Por clave ordenada, no por el orden del mapa: este recorrido decide en
		// qué ORDEN se reservan los números de objeto, así que iterar al azar da
		// un fichero distinto en cada ejecución con la misma entrada.
		for _, k := range clavesOrdenadas(t) {
			if claveQueNoViajaEn(k, t) {
				continue
			}
			out[k] = e.copiarValor(doc, t[k])
		}
		return out
	case *pdfStream:
		// Un stream inline dentro de otro objeto no es legal en PDF (siempre es
		// indirecto), pero si apareciera se copia su diccionario y se pierde el
		// cuerpo, que es preferible a escribir un fichero inválido.
		return e.copiarValor(doc, t.Dict)
	}
	return v
}

// copiarObjeto trae el objeto `num` de `doc` y devuelve su número en el destino.
func (e *PDFEnsamblador) copiarObjeto(doc *PDFDoc, num int) int {
	llave := origenObj{doc: doc, num: num}
	if ya, existe := e.copiados[llave]; existe {
		return ya
	}
	destino := e.reservar()
	// Se registra ANTES de recorrer: un objeto que vuelve a sí mismo por un
	// ciclo encuentra su número ya asignado y no recursa para siempre.
	e.copiados[llave] = destino

	orig, err := doc.objeto(num)
	if err != nil {
		e.cuerpos[destino-1] = objetoSalida{valor: pdfNull{}}
		return destino
	}
	// Una PÁGINA no se copia por el grafo: o la añade AnadirPagina —que reutiliza
	// este número— o no está en el documento nuevo y la referencia queda a null.
	if dict, ok := orig.(pdfDict); ok {
		if tipo, _ := dict["Type"].(pdfName); tipo == "Page" {
			if e.paginaReservada == nil {
				e.paginaReservada = map[int]bool{}
			}
			e.paginaReservada[destino] = true
			return destino
		}
	}

	if st, ok := orig.(*pdfStream); ok {
		dict := pdfDict{}
		for _, k := range clavesOrdenadas(st.Dict) {
			if claveQueNoViajaEn(k, st.Dict) || k == "Length" {
				continue // /Length lo recalcula la serialización, que es quien cifra
			}
			dict[k] = e.copiarValor(doc, st.Dict[k])
		}
		// El cuerpo viaja SIN descomprimir: recomprimir aquí no aporta nada y
		// degradaría las imágenes con pérdida en cada guardado. Lo que sí hay que
		// deshacer es el CIFRADO del origen: sus bytes están cifrados con la clave
		// de aquel documento, y copiarlos tal cual a un fichero sin proteger
		// produce un PDF que abre y no se ve, sin ningún error.
		datos := st.Raw
		if doc.cripto != nil && !st.sinCifrar {
			var err error
			if datos, err = doc.cripto.descifrarFlujo(doc, st); err != nil && e.fallo == nil {
				// Un adjunto que no se puede descifrar saldría como basura en un
				// fichero sin cifrar: se dice, y la interfaz pide la contraseña.
				e.fallo = err
			}
		}
		// El parche se aplica TAMBIÉN a los flujos. Saltárselo aquí dejaba el
		// diccionario viejo con el cuerpo nuevo: un objeto que se contradice a sí
		// mismo, que abre sin error y se dibuja como un hueco gris.
		if parche, hay := e.parches[llave]; hay {
			parche(e, dict)
		}
		if nuevos, hay := e.reemplazos[num]; hay {
			datos = nuevos
		} else if e.comprimir != nil {
			datos = e.comprimir.tratarFlujo(doc, st, dict, datos)
		}
		e.cuerpos[destino-1] = objetoSalida{valor: dict, flujo: datos, esFlujo: true}
		return destino
	}

	copiado := e.copiarValor(doc, orig)
	if parche, hay := e.parches[llave]; hay {
		if dict, ok := copiado.(pdfDict); ok {
			parche(e, dict)
		}
	}
	e.cuerpos[destino-1] = objetoSalida{valor: copiado}
	return destino
}

// Parchear registra una modificación sobre el objeto `num` de `doc`, que se
// aplica cuando el recorrido lo copie. Es la vía para tocar algo que NO está en
// el diccionario de la página —un campo de formulario cuelga de su widget, que
// cuelga de /Annots— sin volver a recorrer el grafo por fuera.
func (e *PDFEnsamblador) Parchear(doc *PDFDoc, num int, parche parcheObjeto) {
	if e.parches == nil {
		e.parches = map[origenObj]parcheObjeto{}
	}
	e.parches[origenObj{doc: doc, num: num}] = parche
}

// ConservarFormulario copia el /AcroForm del catálogo de origen. Sin esto,
// combinar o reordenar un PDF con formulario deja los campos dibujados pero el
// documento deja de SER un formulario: no se puede rellenar ni enviar, y no hay
// ningún error que lo diga.
func (e *PDFEnsamblador) ConservarFormulario(doc *PDFDoc) {
	raiz, _ := doc.resolver(doc.trailer["Root"]).(pdfDict)
	if raiz == nil {
		return
	}
	forma, _ := doc.resolver(raiz["AcroForm"]).(pdfDict)
	if forma == nil {
		return
	}
	copia := pdfDict{}
	for _, k := range clavesOrdenadas(forma) {
		copia[k] = e.copiarValor(doc, forma[k])
	}
	if e.extrasCatalogo == nil {
		e.extrasCatalogo = pdfDict{}
	}
	e.extrasCatalogo["AcroForm"] = copia
}

// Bytes cierra el documento: añade el catálogo y el nodo de páginas, escribe la
// tabla de referencias cruzadas y devuelve el PDF completo.
func (e *PDFEnsamblador) Bytes() ([]byte, error) {
	if len(e.paginas) == 0 {
		return nil, fmt.Errorf("pdf: no se puede escribir un documento sin páginas")
	}
	numPages := e.reservar()
	numCatalogo := e.reservar()

	kids := make(pdfArray, 0, len(e.paginas))
	for _, n := range e.paginas {
		kids = append(kids, pdfRef{Num: n})
		// Cada página apunta a su padre ya con el número definitivo. Se pone en el
		// diccionario, no parcheando los bytes: con /Resources indirecto, inyectar
		// una entrada en el texto ya escrito produce la clave DUPLICADA.
		if dict, ok := e.cuerpos[n-1].valor.(pdfDict); ok {
			dict["Parent"] = pdfRef{Num: numPages}
		}
	}
	e.cuerpos[numPages-1] = objetoSalida{valor: pdfDict{
		"Type":  pdfName("Pages"),
		"Kids":  kids,
		"Count": int64(len(kids)),
	}}
	catalogo := pdfDict{"Type": pdfName("Catalog"), "Pages": pdfRef{Num: numPages}}
	version20 := false // PDF/A-4 exige la cabecera de PDF 2.0
	if origen := e.documentoEntero(); origen != nil && !e.sinCatalogo {
		e.conservarCatalogo(origen, catalogo)
		// El XMP no viaja (es historial), pero la declaración PDF/A del original
		// sí: sin ella un documento de archivo deja de serlo al girar una página.
		// Se escribe solo el identificador, sin fechas ni autores.
		if dec, ok := origen.declaraPDFA(); ok {
			if dec, motivo := e.incumplePDFA(origen, dec, catalogo); motivo == "" {
				catalogo["Metadata"] = e.flujoXMP(xmpPDFA(dec, OpcionesArchivo{}))
				version20 = dec.parte == "4"
			}
		}
	}
	for k, v := range e.extrasCatalogo {
		catalogo[k] = v
		if _, quitar := v.(pdfNull); quitar {
			// Un extra a null QUITA la entrada del catálogo conservado: así dice
			// aplanar que el documento deja de ser un formulario.
			delete(catalogo, k)
		}
	}
	e.cuerpos[numCatalogo-1] = objetoSalida{valor: catalogo}
	if e.fallo != nil {
		return nil, e.fallo
	}

	trailer := pdfDict{
		"Size": int64(len(e.cuerpos) + 1),
		"Root": pdfRef{Num: numCatalogo},
	}
	if e.cifrado != nil {
		numEnc := e.reservar()
		e.cuerpos[numEnc-1] = objetoSalida{valor: e.cifrado.diccionario()}
		// El propio /Encrypt va en claro: sus cadenas (/U, /O, /UE, /OE) son lo que
		// se necesita para DERIVAR la clave, así que cifrarlas con esa clave dejaría
		// el documento imposible de abrir.
		e.cifrado.sinCifrar[numEnc] = true
		trailer["Encrypt"] = pdfRef{Num: numEnc}
		// El /ID entra en la derivación de la clave, así que va SIEMPRE con el
		// /Encrypt y sin cifrar: es la semilla, no un secreto.
		trailer["ID"] = pdfArray{pdfString(e.cifrado.id), pdfString(e.cifrado.id)}
	}

	var buf bytes.Buffer
	if version20 {
		buf.WriteString("%PDF-2.0\n%\xe2\xe3\xcf\xd3\n")
	} else {
		buf.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	}
	offsets := make([]int, len(e.cuerpos)+1)
	for i, obj := range e.cuerpos {
		offsets[i+1] = buf.Len()
		buf.WriteString(strconv.Itoa(i+1) + " 0 obj\n")
		buf.Write(e.serializar(i+1, obj))
		buf.WriteString("\nendobj\n")
	}
	if e.cifrado == nil {
		// /ID SIEMPRE (§14.4): PDF 2.0 y PDF/A lo exigen, y sin él un lector no
		// distingue dos versiones del mismo fichero. Sale del contenido y no del
		// azar para que la misma entrada dé siempre los mismos bytes.
		huella := sha256.Sum256(buf.Bytes())
		trailer["ID"] = pdfArray{pdfString(huella[:16]), pdfString(huella[:16])}
	}
	inicioXref := buf.Len()
	buf.WriteString("xref\n0 " + strconv.Itoa(len(e.cuerpos)+1) + "\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(e.cuerpos); i++ {
		buf.WriteString(pad10(offsets[i]) + " 00000 n \n")
	}
	// El tráiler NUNCA se cifra: hay que poder leerlo para llegar al /Encrypt.
	buf.WriteString("trailer\n" + string(serializarObjetoPDF(trailer)) + "\n")
	buf.WriteString("startxref\n" + strconv.Itoa(inicioXref) + "\n%%EOF")
	return buf.Bytes(), nil
}

// documentoEntero devuelve el documento de origen si el nuevo tiene TODAS sus
// páginas y ninguna más —rellenar, anotar, girar, proteger…—, o nil si es otro
// documento —combinar, extraer, borrar páginas—.
func (e *PDFEnsamblador) documentoEntero() *PDFDoc {
	if len(e.pendientes) == 0 {
		return nil
	}
	doc := e.pendientes[0].origen
	if doc == nil || len(e.pendientes) != doc.NumPaginas() {
		return nil
	}
	vistas := make(map[int]bool, len(e.pendientes))
	for _, pp := range e.pendientes {
		if pp.origen != doc || pp.p.num == 0 || vistas[pp.p.num] {
			return nil
		}
		vistas[pp.p.num] = true
	}
	return doc
}

// conservarCatalogo copia al catálogo nuevo lo que el documento ES y no está en
// sus páginas: el índice (/Outlines), el árbol de estructura del etiquetado, el
// idioma, las etiquetas de página, los destinos con nombre y los adjuntos
// (/Names), el formulario, las capas, la intención de salida de PDF/A y las
// preferencias del visor. Sin esto, anotar un documento le quitaba el índice, la
// accesibilidad y el formulario, sin decir nada (medido con
// scripts/certipdf-compat). Lo que es HISTORIAL sigue sin viajar.
func (e *PDFEnsamblador) conservarCatalogo(doc *PDFDoc, catalogo pdfDict) {
	raiz, _ := doc.resolver(doc.trailer["Root"]).(pdfDict)
	for _, k := range clavesOrdenadas(raiz) {
		if _, propia := e.extrasCatalogo[k]; propia || catalogoQueNoViaja(k) {
			continue
		}
		if e.soloCatalogo != nil && !e.soloCatalogo(doc, k, raiz[k]) {
			continue
		}
		catalogo[k] = e.copiarValor(doc, raiz[k])
	}
}

// catalogoQueNoViaja son las entradas del catálogo que no se conservan:
//   - /Type y /Pages las escribe Bytes;
//   - /Metadata, /PieceInfo y /LastModified son historial (claveQueNoViaja);
//   - /Perms (DocMDP), /DSS y /Legal hablan de FIRMAS, y reescribir el fichero
//     invalida las que hubiera: conservar su certificación o sus datos de
//     validación diría algo falso del documento nuevo;
//   - /Version la fija la cabecera que escribe Bytes.
func catalogoQueNoViaja(k pdfName) bool {
	switch k {
	case "Type", "Pages", "Perms", "DSS", "Legal", "Version":
		return true
	}
	return claveQueNoViaja(k)
}

// serializar escribe un objeto, cifrando sus cadenas y su flujo si toca. El
// número de objeto llega aquí porque la clave de cifrado depende de él.
func (e *PDFEnsamblador) serializar(num int, obj objetoSalida) []byte {
	var cif cifradorObjeto
	if e.cifrado != nil && !e.cifrado.sinCifrar[num] {
		cif = e.cifrado.paraObjeto(num)
	}
	if !obj.esFlujo {
		var buf bytes.Buffer
		escribirObjetoPDF(&buf, obj.valor, cif)
		return buf.Bytes()
	}

	datos := obj.flujo
	if cif != nil {
		datos = cif(datos, false)
	}
	dict, ok := obj.valor.(pdfDict)
	if !ok {
		dict = pdfDict{}
	}
	// /Length se escribe con la longitud FINAL: AES añade el vector de
	// inicialización y el relleno, así que la del texto en claro no vale.
	dict["Length"] = int64(len(datos))

	var buf bytes.Buffer
	escribirObjetoPDF(&buf, dict, cif)
	buf.WriteString("\nstream\n")
	buf.Write(datos)
	buf.WriteString("\nendstream")
	return buf.Bytes()
}

// claveQueNoViaja son las entradas que NUNCA se copian al documento nuevo.
//
//   - **/Parent** apunta hacia ARRIBA: seguirlo arrastraría el árbol de páginas
//     del origen entero —incluidas las páginas que no se querían— y cerraría un
//     ciclo. Se sustituye por el /Pages del destino.
//
// OJO con /Parent: solo sobra en el árbol de PÁGINAS. En un campo de formulario
// apunta al campo padre y es lo que sostiene la jerarquía de nombres, así que
// cortarlo ahí convierte un formulario en un montón de casillas sueltas. Por eso
// existe `claveQueNoViajaEn`, que mira el diccionario y no solo la clave.
//   - **/Metadata** (el XMP), **/PieceInfo** (datos privados de la aplicación que
//     generó el fichero: ahí es donde algunos programas guardan el documento
//     ORIGINAL entero) y **/LastModified** son HISTORIAL. No hacen falta para
//     dibujar nada y arrastran autor, fechas, aplicación de origen e
//     identificadores que siguen a una copia de mano en mano.
//
// El historial se va SIEMPRE, no solo al «sanear»: en una suite que promete
// privacidad, lo que hay que justificar es conservarlo, no quitarlo. El /Info del
// tráiler, el XMP del catálogo y los /ID desaparecen por su cuenta, porque
// `Bytes` escribe un catálogo y un tráiler nuevos en vez de copiar los de origen.
func claveQueNoViaja(k pdfName) bool {
	switch k {
	case "Parent", "Metadata", "PieceInfo", "LastModified":
		return true
	}
	return false
}

// claveQueNoViajaEn decide con el diccionario delante. Es la que hay que usar
// dentro del grafo; `claveQueNoViaja` queda para el diccionario de la página,
// donde /Parent siempre sobra.
func claveQueNoViajaEn(k pdfName, dict pdfDict) bool {
	if k != "Parent" {
		return claveQueNoViaja(k)
	}
	tipo, _ := dict["Type"].(pdfName)
	// El /Parent de una página o de un nodo del árbol de páginas sobra; el de
	// cualquier otra cosa —un campo de formulario, un elemento del árbol de
	// estructura— es parte del dato.
	return tipo == "Page" || tipo == "Pages"
}

// clavesOrdenadas devuelve las claves de un diccionario en orden estable. Se usa
// en TODO recorrido que tenga efectos —reservar números de objeto, escribir
// bytes—: el orden de un mapa de Go es aleatorio por diseño, y sin fijarlo dos
// ejecuciones con la misma entrada producen ficheros distintos.
func clavesOrdenadas(d pdfDict) []pdfName {
	out := make([]pdfName, 0, len(d))
	for k := range d {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// --- Serialización -----------------------------------------------------------

// cifradorObjeto transforma los datos de UN objeto al escribirlo. nil = sin
// cifrar, que es el caso de todo documento no protegido.
type cifradorObjeto func(datos []byte, esCadena bool) []byte

// serializarObjetoPDF vuelca un objeto a su sintaxis de fichero, sin cifrar.
func serializarObjetoPDF(o pdfObj) []byte {
	var buf bytes.Buffer
	escribirObjetoPDF(&buf, o, nil)
	return buf.Bytes()
}

func escribirObjetoPDF(buf *bytes.Buffer, o pdfObj, cif cifradorObjeto) {
	switch v := o.(type) {
	case nil, pdfNull:
		buf.WriteString("null")
	case bool:
		if v {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case int64:
		buf.WriteString(strconv.FormatInt(v, 10))
	case int:
		buf.WriteString(strconv.Itoa(v))
	case float64:
		// 'f' sin exponente: la notación científica no es válida en PDF, y un
		// número de página escrito como «1e+06» rompe el fichero entero.
		buf.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	case pdfName:
		buf.WriteByte('/')
		buf.WriteString(escaparNombrePDF(string(v)))
	case pdfString:
		if cif != nil {
			v = pdfString(cif([]byte(v), true))
		}
		escribirCadenaPDF(buf, v)
	case pdfRef:
		buf.WriteString(strconv.Itoa(v.Num) + " 0 R")
	case pdfArray:
		buf.WriteByte('[')
		for i, x := range v {
			if i > 0 {
				buf.WriteByte(' ')
			}
			escribirObjetoPDF(buf, x, cif)
		}
		buf.WriteByte(']')
	case pdfDict:
		buf.WriteString("<<")
		// Orden estable de claves: sin él, dos ejecuciones con la misma entrada
		// producen ficheros distintos byte a byte y no hay forma de comparar ni
		// de reproducir un build.
		claves := make([]string, 0, len(v))
		for k := range v {
			claves = append(claves, string(k))
		}
		sort.Strings(claves)
		for _, k := range claves {
			buf.WriteString(" /" + escaparNombrePDF(k) + " ")
			escribirObjetoPDF(buf, v[pdfName(k)], cif)
		}
		buf.WriteString(" >>")
	default:
		buf.WriteString("null")
	}
}

// escaparNombrePDF codifica como «#xx» lo que no puede ir literal en un nombre.
func escaparNombrePDF(s string) string {
	var buf bytes.Buffer
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '!' || c > '~' || c == '#' || esDelimitadorPDF(c) {
			fmt.Fprintf(&buf, "#%02X", c)
			continue
		}
		buf.WriteByte(c)
	}
	return buf.String()
}

// escribirCadenaPDF vuelca una cadena. Se elige hexadecimal cuando hay bytes no
// imprimibles: es más largo pero no depende de escapes, y una cadena binaria
// (una fecha cifrada, un identificador) escrita como literal se corrompe.
func escribirCadenaPDF(buf *bytes.Buffer, s pdfString) {
	imprimible := true
	for _, b := range s {
		if b < 32 || b > 126 {
			imprimible = false
			break
		}
	}
	if !imprimible {
		buf.WriteByte('<')
		for _, b := range s {
			fmt.Fprintf(buf, "%02X", b)
		}
		buf.WriteByte('>')
		return
	}
	buf.WriteByte('(')
	for _, b := range s {
		switch b {
		case '(', ')', '\\':
			buf.WriteByte('\\')
		}
		buf.WriteByte(b)
	}
	buf.WriteByte(')')
}

// --- Operaciones de alto nivel (ola P4) --------------------------------------

// CombinarPDF une varios documentos en uno, en el orden dado.
func CombinarPDF(docs ...*PDFDoc) ([]byte, error) {
	if len(docs) == 0 {
		return nil, fmt.Errorf("pdf: combinar necesita al menos un documento")
	}
	e := NuevoEnsambladorPDF()
	for _, d := range docs {
		for i := 0; i < d.NumPaginas(); i++ {
			if err := e.AnadirPagina(d, i, 0); err != nil {
				return nil, err
			}
		}
	}
	return e.Bytes()
}

// ExtraerPaginasPDF construye un PDF con las páginas indicadas (0-based) y en el
// orden en que se den, que es lo que convierte esta única función en extraer,
// reordenar, dividir y borrar: borrar es extraer las demás.
func ExtraerPaginasPDF(doc *PDFDoc, indices []int) ([]byte, error) {
	if len(indices) == 0 {
		return nil, fmt.Errorf("pdf: no se ha indicado ninguna página")
	}
	e := NuevoEnsambladorPDF()
	for _, i := range indices {
		if err := e.AnadirPagina(doc, i, 0); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}

// RotarPaginasPDF gira las páginas de `giros` (índice 0-based → grados, que se
// SUMAN a la rotación que ya tuviera) y deja el resto como estaban.
func RotarPaginasPDF(doc *PDFDoc, giros map[int]int) ([]byte, error) {
	e := NuevoEnsambladorPDF()
	for i := 0; i < doc.NumPaginas(); i++ {
		if err := e.AnadirPagina(doc, i, giros[i]); err != nil {
			return nil, err
		}
	}
	return e.Bytes()
}

// BorrarPaginasPDF devuelve el documento sin las páginas indicadas.
func BorrarPaginasPDF(doc *PDFDoc, borrar []int) ([]byte, error) {
	fuera := make(map[int]bool, len(borrar))
	for _, i := range borrar {
		fuera[i] = true
	}
	var quedan []int
	for i := 0; i < doc.NumPaginas(); i++ {
		if !fuera[i] {
			quedan = append(quedan, i)
		}
	}
	if len(quedan) == 0 {
		return nil, fmt.Errorf("pdf: no se pueden borrar todas las páginas del documento")
	}
	return ExtraerPaginasPDF(doc, quedan)
}
