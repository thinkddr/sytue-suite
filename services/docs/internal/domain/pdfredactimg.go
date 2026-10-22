// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Censura de IMÁGENES y de OBJETOS INCRUSTADOS.
//
// Quitar texto no toca una imagen: un escaneo lleva el texto DENTRO de los
// píxeles. Aquí se hace lo único que garantiza que ese contenido no queda: se
// DECODIFICA la imagen, se ponen a negro los píxeles que caen en la zona (y su
// máscara suave, que también puede llevar una silueta) y se emite una imagen
// NUEVA, con un nombre nuevo, solo para la página que se censura. El `Do` del
// flujo apunta a ella y el objeto original deja de estar en los recursos de la
// página, así que no viaja al fichero de salida. Si otra página sigue dibujando
// la original, la conserva esa página, que no se ha censurado.
//
// Un formulario (membrete, logotipo, sello) que toca la zona se trata igual: su
// contenido se censura en una COPIA propia, con los mismos pasos, y la original
// deja de apuntarse.
//
// Todo esto es entrada no confiable: una imagen declara su tamaño en pocos bytes
// y un formulario puede llamarse a sí mismo. Hay topes —píxeles por imagen y por
// documento, número de formularios, profundidad— y al pasarlos se falla con
// ErrRedaccionNoGarantizada en vez de gastar memoria o tiempo sin fin.
package domain

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"sort"
	"strconv"
)

const (
	// maxPixelesRedaccion acota una sola imagen: 24 Mpx son 96 MB en memoria como
	// RGBA, y cubre una foto de 6000×4000.
	maxPixelesRedaccion = 24 << 20
	// maxPixelesPresupuesto acota todo el documento: un PDF hostil puede dibujar
	// mil veces la misma imagen enorme.
	maxPixelesPresupuesto = 96 << 20
	// maxFormulariosCensurados acota los objetos de formulario que se reescriben,
	// que con llamadas recursivas crecen exponencialmente aunque la profundidad
	// esté limitada.
	maxFormulariosCensurados = 2048
	// maxBytesFormularios acota el contenido de formularios que se recorre: uno de
	// 100 MiB llamado doscientas veces sería 20 GiB de trabajo.
	maxBytesFormularios = 128 << 20
	// maxObjetosCensura acota las imágenes y formularios nuevos de un documento.
	maxObjetosCensura = 5000
	// maxVisitasPresupuesto acota los píxeles que se recorren para marcar zonas en
	// todo el documento (cada zona recorre lo que cubre: mil zonas sobre una imagen
	// de 24 Mpx serían 24 000 Mpx).
	maxVisitasPresupuesto = 400 << 20
	// maxProfundidadCensura es la profundidad de formularios dentro de formularios.
	maxProfundidadCensura = 8
	// calidadJPEGCensura es la de las fotos que se vuelven a codificar con
	// pérdida, que es como llegaron.
	calidadJPEGCensura = 92
)

// presupuestoCensura reparte lo que un documento puede gastar al censurarse.
type presupuestoCensura struct {
	pixeles     int
	formularios int
	visitas     int // píxeles recorridos al marcar zonas: zonas × imagen puede multiplicar
	bytes       int // contenido de formularios que se vuelve a recorrer
	objetos     int // objetos nuevos que se pueden crear
	nombres     int // contador de nombres nuevos de XObject
}

func nuevoPresupuestoCensura() *presupuestoCensura {
	return &presupuestoCensura{
		pixeles: maxPixelesPresupuesto, formularios: maxFormulariosCensurados,
		visitas: maxVisitasPresupuesto, bytes: maxBytesFormularios, objetos: maxObjetosCensura,
	}
}

// nivelCensura son los recursos de UN flujo de contenido (la página o un
// formulario con recursos propios) y lo que la censura les cambia.
type nivelCensura struct {
	recursos pdfDict
	// nuevos son los XObject que se añaden, por nombre.
	nuevos map[pdfName]*xobjNuevo
	// usos cuenta los `Do` de cada nombre original; sustituidos, cuántos de ellos
	// se han cambiado por un objeto nuevo. Un original solo sale de los recursos
	// si TODOS sus usos se sustituyeron.
	usos, sustituidos map[pdfName]int
	// noQuitar se marca cuando hay un formulario sin recursos propios, que usa
	// los de este nivel sin que se cuente: quitar un original podría romperlo.
	noQuitar bool
}

func nuevoNivelCensura(recursos pdfDict) *nivelCensura {
	return &nivelCensura{
		recursos: recursos, nuevos: map[pdfName]*xobjNuevo{},
		usos: map[pdfName]int{}, sustituidos: map[pdfName]int{},
	}
}

// xobjNuevo es un objeto que la censura crea. Su diccionario puede llevar
// referencias al documento de ORIGEN (un formulario conserva su /Group, por
// ejemplo): se copian al materializarlo.
type xobjNuevo struct {
	dict  pdfDict
	datos []byte // el cuerpo ya codificado según dict["Filter"]
	smask *xobjNuevo
	nivel *nivelCensura // formularios con recursos propios
}

// originalesQueSobran son los nombres cuyos usos se han sustituido todos.
func (n *nivelCensura) originalesQueSobran() map[pdfName]bool {
	out := map[pdfName]bool{}
	for nombre, hechos := range n.sustituidos {
		if hechos > 0 && hechos == n.usos[nombre] {
			out[nombre] = true
		}
	}
	return out
}

func (c *censor) nombreNuevo() pdfName {
	xobjs, _ := c.doc.resolver(c.recursos["XObject"]).(pdfDict)
	for {
		c.pres.nombres++
		nombre := pdfName("SyRed" + strconv.Itoa(c.pres.nombres))
		if _, hay := xobjs[nombre]; hay {
			continue
		}
		if _, hay := c.nivel.nuevos[nombre]; hay {
			continue
		}
		return nombre
	}
}

// registrar anota un objeto nuevo y devuelve el `Do` que lo dibuja. `original`
// es el nombre al que sustituye, o vacío en una imagen en línea (que no tenía).
func (c *censor) registrar(original pdfName, nuevo *xobjNuevo) string {
	c.pres.objetos--
	if original != "" {
		c.nivel.sustituidos[original]++
	}
	nombre := c.nombreNuevo()
	c.nivel.nuevos[nombre] = nuevo
	c.cambios++
	return "/" + string(nombre) + " Do\n"
}

// hayObjetos dice si aún se pueden crear objetos nuevos.
func (c *censor) hayObjetos() error {
	if c.pres.objetos <= 0 {
		return fmt.Errorf("%w: la zona toca demasiados objetos (imágenes o formularios)",
			ErrRedaccionNoGarantizada)
	}
	return nil
}

// tratarXObject decide qué hacer con un `Do`.
//
// Si el objeto no toca la zona, ni se mira. Si la toca: una imagen se rehace sin
// los píxeles de la zona, y un formulario se reescribe en una copia. Lo que no se
// puede leer, se dice: nada de dar por buena una censura que no se ha hecho.
func (c *censor) tratarXObject(nombre pdfName) (string, bool, error) {
	c.nivel.usos[nombre]++
	if !c.ctm.finita() {
		// Con una matriz que no es un número no se dibuja nada, y los datos
		// seguirían en el fichero sin que ninguna zona los alcance: se quita.
		return c.quitarDo(nombre)
	}
	xobjs, _ := c.doc.resolver(c.recursos["XObject"]).(pdfDict)
	st, _ := c.doc.resolver(xobjs[nombre]).(*pdfStream)
	unidad := cajaDeRectangulo(c.ctm, 0, 0, 1, 1)
	if st == nil {
		// No se puede resolver: se supone que ocupa el cuadrado unidad, que es
		// donde lo pondría el CTM, y se falla si eso toca la zona.
		if unidad.tocaAlguna(c.zonas) {
			return "", false, fmt.Errorf("%w: en la zona se dibuja /%s y no se puede leer qué es",
				ErrRedaccionNoGarantizada, nombre)
		}
		return "", false, nil
	}
	if sub, _ := c.doc.resolver(st.Dict["Subtype"]).(pdfName); sub == "Form" {
		return c.formularioXObject(nombre, st)
	}
	if !unidad.tocaAlguna(c.zonas) {
		return "", false, nil
	}
	if _, ok := c.ctm.inversa(); !ok {
		return c.quitarDo(nombre) // matriz degenerada: no se dibuja nada
	}
	if err := c.hayObjetos(); err != nil {
		return "", false, err
	}
	nuevo, err := c.imagenCensurada(st, c.ctm, nombre)
	if errors.Is(err, errNadaQueCensurar) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return c.registrar(nombre, nuevo), true, nil
}

// quitarDo elimina un `Do` que no dibuja nada (matriz degenerada).
func (c *censor) quitarDo(nombre pdfName) (string, bool, error) {
	c.nivel.sustituidos[nombre]++
	c.cambios++
	return "", true, nil
}

// formularioXObject trata un `Do` de formulario: si toca la zona, su contenido se
// reescribe en una copia.
func (c *censor) formularioXObject(nombre pdfName, st *pdfStream) (string, bool, error) {
	compuesta, bbox := c.cajaDeFormulario(st)
	_, propiosRecursos := c.doc.resolver(st.Dict["Resources"]).(pdfDict)
	if !propiosRecursos {
		// Usa los recursos de quien lo dibuja: aunque no toque la zona, sus `Do`
		// no se cuentan y quitar un original de aquí podría romperlo.
		c.nivel.noQuitar = true
	}
	if !cajaDeRectangulo(compuesta, bbox[0], bbox[1], bbox[2], bbox[3]).tocaAlguna(c.zonas) {
		return "", false, nil
	}
	if _, ok := compuesta.inversa(); !ok {
		return c.quitarDo(nombre)
	}
	datos, err := c.reservarFormulario(nombre, st)
	if err != nil {
		return "", false, err
	}
	nivel := c.nivel
	if propios, ok := c.doc.resolver(st.Dict["Resources"]).(pdfDict); ok {
		nivel = nuevoNivelCensura(propios)
	}
	dentro := nuevoCensor(c.doc, nivel, c.pres, c.zonas, c.diagonal)
	dentro.ctm = compuesta
	dentro.prof = c.prof + 1
	limpio, err := dentro.reescribir(datos)
	if err != nil {
		return "", false, err
	}
	if dentro.cambios == 0 {
		return "", false, nil // nada que quitar: se queda la original
	}
	dict := pdfDict{}
	for k, v := range st.Dict {
		switch k {
		case "Length", "Filter", "DecodeParms", "DP", "F", "Resources":
			continue // el cuerpo nuevo va comprimido por nosotros y los recursos se reconstruyen
		}
		dict[k] = v
	}
	dict["Filter"] = pdfName("FlateDecode")
	hijo := &xobjNuevo{dict: dict, datos: comprimirFlate(limpio)}
	if nivel != c.nivel {
		hijo.nivel = nivel
	}
	return c.registrar(nombre, hijo), true, nil
}

// cajaDeFormulario devuelve la matriz del formulario compuesta con la actual y su /BBox.
func (c *censor) cajaDeFormulario(st *pdfStream) (matrizPDF, [4]float64) {
	propia := matrizIdentidad
	if mm, ok := c.doc.resolver(st.Dict["Matrix"]).(pdfArray); ok && len(mm) >= 6 {
		for i := 0; i < 6; i++ {
			propia[i] = comoReal(c.doc.resolver(mm[i]))
		}
	}
	bbox := [4]float64{0, 0, 1, 1}
	if bb, ok := c.doc.resolver(st.Dict["BBox"]).(pdfArray); ok && len(bb) >= 4 {
		for i := 0; i < 4; i++ {
			bbox[i] = comoReal(c.doc.resolver(bb[i]))
		}
	}
	return propia.por(c.ctm), bbox
}

// reservarFormulario gasta del presupuesto lo que cuesta recorrer un formulario y
// devuelve su contenido.
func (c *censor) reservarFormulario(nombre pdfName, st *pdfStream) ([]byte, error) {
	if c.prof >= maxProfundidadCensura {
		return nil, fmt.Errorf("%w: los objetos incrustados de la zona se anidan demasiado",
			ErrRedaccionNoGarantizada)
	}
	if err := c.hayObjetos(); err != nil {
		return nil, err
	}
	c.pres.formularios--
	if c.pres.formularios < 0 {
		return nil, fmt.Errorf("%w: la zona toca demasiados objetos incrustados",
			ErrRedaccionNoGarantizada)
	}
	datos, err := c.doc.datosDeFlujo(st)
	if err != nil {
		return nil, fmt.Errorf("%w: no se puede leer el objeto /%s que hay en la zona: %w",
			ErrRedaccionNoGarantizada, nombre, err)
	}
	if c.pres.bytes -= len(datos); c.pres.bytes < 0 {
		return nil, fmt.Errorf("%w: los objetos incrustados de la zona son demasiado grandes",
			ErrRedaccionNoGarantizada)
	}
	return datos, nil
}

// imagenEnLinea trata `BI … ID … EI`. Devuelve el texto que sustituye a la imagen
// si se ha cambiado, y dónde acaba (justo detrás de EI).
func (c *censor) imagenEnLinea(contenido []byte, pos int) (string, bool, int, error) {
	fin, err := finDeImagenIncrustada(contenido, pos)
	if err != nil {
		return "", false, 0, err
	}
	if !c.ctm.finita() {
		c.cambios++
		return "", true, fin, nil
	}
	if !cajaDeRectangulo(c.ctm, 0, 0, 1, 1).tocaAlguna(c.zonas) {
		return "", false, fin, nil
	}
	if _, ok := c.ctm.inversa(); !ok {
		c.cambios++
		return "", true, fin, nil // degenerada: no se dibuja, se quita
	}
	if err := c.hayObjetos(); err != nil {
		return "", false, 0, err
	}
	st, err := imagenEnLineaComoFlujo(c.doc, contenido[pos:fin])
	if err != nil {
		return "", false, 0, err
	}
	nuevo, err := c.imagenCensurada(st, c.ctm, "en línea")
	if errors.Is(err, errNadaQueCensurar) {
		return "", false, fin, nil
	}
	if err != nil {
		return "", false, 0, err
	}
	return c.registrar("", nuevo), true, fin, nil
}

// imagenEnLineaComoFlujo arma un flujo de imagen con el diccionario y los datos
// crudos de una imagen en línea. `trozo` va de justo después de BI hasta detrás de EI.
func imagenEnLineaComoFlujo(doc *PDFDoc, trozo []byte) (*pdfStream, error) {
	lx := &pdfLexer{buf: trozo}
	dict := pdfDict{}
	for {
		lx.saltarEspacios()
		if lx.pos >= len(lx.buf) {
			return nil, fmt.Errorf("%w: una imagen en línea no tiene datos", ErrRedaccionNoGarantizada)
		}
		if lx.aceptaPalabra("ID") {
			break
		}
		if lx.byteActual() != '/' {
			lx.pos++
			continue
		}
		clave := lx.nombre()
		antes := lx.pos
		valor, err := doc.parsearObjeto(lx)
		if err != nil || lx.pos == antes {
			lx.pos = antes + 1
			continue
		}
		dict[clave] = valor
	}
	if lx.pos < len(lx.buf) && esEspacioPDF(lx.buf[lx.pos]) {
		lx.pos++
	}
	if lx.pos > len(trozo)-2 {
		return nil, fmt.Errorf("%w: una imagen en línea no tiene datos", ErrRedaccionNoGarantizada)
	}
	datos := trozo[lx.pos : len(trozo)-2] // sin el EI
	if n := len(datos); n > 0 && esEspacioPDF(datos[n-1]) {
		datos = datos[:n-1]
	}
	if _, ok := dict["Filter"]; !ok {
		if f, ok := dict["F"]; ok {
			dict["Filter"] = f
		}
	}
	if _, ok := dict["DecodeParms"]; !ok {
		if p, ok := dict["DP"]; ok {
			dict["DecodeParms"] = p
		}
	}
	// Los datos ya salieron en claro del flujo de contenido: descifrarlos otra vez
	// los destrozaría.
	return &pdfStream{Dict: dict, Raw: datos, sinCifrar: true}, nil
}

// --- Píxeles de la zona ---------------------------------------------------------

// rangoPixeles devuelve los índices de pixel [lo, hi] de un eje cuyo borde i está
// en esc*i+off y que se solapan con el intervalo (a, b), recortados a [0, n).
// Con un hueco de 1e-6 puntos en cada extremo: una zona que coincide con el borde
// de un pixel no ennegrece también el siguiente.
func rangoPixeles(a, b, esc, off float64, n int) (int, int) {
	const eps = 1e-6
	a, b = a+eps, b-eps
	if esc == 0 || a >= b {
		return 0, -1
	}
	var lo, hi float64
	if esc > 0 {
		lo, hi = math.Floor((a-off)/esc), math.Ceil((b-off)/esc)-1
	} else {
		lo, hi = math.Floor((b-off)/esc), math.Ceil((a-off)/esc)-1
	}
	// Se acota ANTES de pasar a entero: convertir un float fuera de rango (una zona
	// de 1e308 puntos) es un valor sin definir.
	ilo := int(math.Min(math.Max(lo, 0), float64(n)))
	ihi := int(math.Max(math.Min(hi, float64(n-1)), -1))
	return ilo, ihi
}

// mascaraDeZona marca, en una imagen de w×h colocada con `ctm`, los píxeles que se
// solapan con alguna zona. Devuelve la máscara (1 = dentro), cuántos son y false
// si se ha pasado del presupuesto de píxeles recorridos.
func mascaraDeZona(ctm matrizPDF, w, h int, zonas []AreaRedaccion, visitas *int) ([]byte, int, bool) {
	// Reservar todo el trabajo antes de crear la máscara o recorrer píxeles.
	// Rechazar después de 400 Mpx ya consumidos no protege frente a una entrada hostil.
	restantes := *visitas
	if _, ok := recorrerZonas(nil, ctm, w, h, zonas, &restantes); !ok {
		return nil, 0, false
	}
	mask := make([]byte, w*h)
	n, ok := recorrerZonas(mask, ctm, w, h, zonas, visitas)
	return mask, n, ok
}

// Con mask nil, recorrerZonas solo calcula el presupuesto usando los mismos rangos
// que la censura. Así no divergen la comprobación y los píxeles que se recorren.
func recorrerZonas(mask []byte, ctm matrizPDF, w, h int, zonas []AreaRedaccion, visitas *int) (int, bool) {
	// Del pixel (i, j) —con j hacia abajo— a la página.
	mp := matrizPDF{1 / float64(w), 0, 0, -1 / float64(h), 0, 1}.por(ctm)
	cuenta := 0
	tol := 1e-9 * (math.Abs(mp[0]) + math.Abs(mp[1]) + math.Abs(mp[2]) + math.Abs(mp[3]))
	ejes := math.Abs(mp[1]) <= tol && math.Abs(mp[2]) <= tol
	inv, hayInv := mp.inversa()
	for _, z := range zonas {
		var n int
		var ok bool
		switch {
		case ejes:
			n, ok = marcarAlineada(mask, mp, w, h, z, visitas)
		case hayInv:
			n, ok = marcarGirada(mask, mp, inv, w, h, z, visitas)
		default:
			continue
		}
		if !ok {
			return 0, false
		}
		cuenta += n
	}
	return cuenta, true
}

// marcarAlineada marca los píxeles de una zona cuando la imagen conserva los ejes:
// el rango sale de despejar, sin probar píxel a píxel.
func marcarAlineada(mask []byte, mp matrizPDF, w, h int, z AreaRedaccion, visitas *int) (int, bool) {
	ilo, ihi := rangoPixeles(z.X1, z.X2, mp[0], mp[4], w)
	jlo, jhi := rangoPixeles(z.Y1, z.Y2, mp[3], mp[5], h)
	if ihi < ilo || jhi < jlo {
		return 0, true
	}
	if *visitas -= (ihi - ilo + 1) * (jhi - jlo + 1); *visitas < 0 {
		return 0, false
	}
	if mask == nil {
		return 0, true
	}
	cuenta := 0
	for j := jlo; j <= jhi; j++ {
		fila := mask[j*w : (j+1)*w]
		for i := ilo; i <= ihi; i++ {
			if fila[i] == 0 {
				fila[i] = 1
				cuenta++
			}
		}
	}
	return cuenta, true
}

// marcarGirada marca los píxeles de una zona sobre una imagen girada o sesgada: se
// recorre la caja de la zona en espacio de pixel y se prueba cada uno. Conservador:
// usa la caja del pixel girado.
func marcarGirada(mask []byte, mp, inv matrizPDF, w, h int, z AreaRedaccion, visitas *int) (int, bool) {
	const eps = 1e-6
	cp := cajaDeRectangulo(inv, z.X1, z.Y1, z.X2, z.Y2)
	acota := func(v float64, n int) int { return int(math.Min(math.Max(v, 0), float64(n-1))) }
	ilo, ihi := acota(math.Floor(cp.x1)-1, w), acota(math.Ceil(cp.x2)+1, w)
	jlo, jhi := acota(math.Floor(cp.y1)-1, h), acota(math.Ceil(cp.y2)+1, h)
	if ihi < ilo || jhi < jlo {
		return 0, true
	}
	if *visitas -= (ihi - ilo + 1) * (jhi - jlo + 1); *visitas < 0 {
		return 0, false
	}
	if mask == nil {
		return 0, true
	}
	cuenta := 0
	for j := jlo; j <= jhi; j++ {
		for i := ilo; i <= ihi; i++ {
			if mask[j*w+i] != 0 {
				continue
			}
			px := cajaDeRectangulo(mp, float64(i), float64(j), float64(i+1), float64(j+1))
			if px.x2 > z.X1+eps && px.x1 < z.X2-eps && px.y2 > z.Y1+eps && px.y1 < z.Y2-eps {
				mask[j*w+i] = 1
				cuenta++
			}
		}
	}
	return cuenta, true
}

// --- Rehacer una imagen -------------------------------------------------------

// errNadaQueCensurar avisa, sin ser un fallo, de que ningún píxel de la imagen
// cae en la zona: se queda la original.
var errNadaQueCensurar = errors.New("pdf: ningún píxel de la imagen cae en la zona")

func (c *censor) imagenDemasiado(nombre pdfName, w, h int) error {
	return fmt.Errorf("%w: la imagen /%s de la zona es demasiado grande para censurarla (%d×%d)",
		ErrRedaccionNoGarantizada, nombre, w, h)
}

func (c *censor) imagenIlegible(nombre pdfName, motivo string) error {
	return fmt.Errorf("%w: la imagen /%s de la zona %s", ErrRedaccionNoGarantizada, nombre, motivo)
}

// imagenCensurada devuelve la imagen sin los píxeles de la zona, o
// errNadaQueCensurar si ninguno cae dentro. `ctm` es donde se dibuja: el cuadrado
// unidad por esa matriz.
func (c *censor) imagenCensurada(st *pdfStream, ctm matrizPDF, nombre pdfName) (*xobjNuevo, error) {
	l := &lienzo{d: c.doc, soloRaster: true}
	w, h := l.tamanoImagen(st.Dict)
	if w <= 0 || h <= 0 {
		return nil, c.imagenIlegible(nombre, "no declara su tamaño")
	}
	if w > 1<<16 || h > 1<<16 || w*h > maxPixelesRedaccion || w*h > c.pres.pixeles {
		return nil, c.imagenDemasiado(nombre, w, h)
	}
	// El tamaño declarado también permite rechazar el exceso de zonas antes de
	// decodificar una imagen grande. La máscara vuelve a comprobar el tamaño real,
	// que puede ser distinto en un JPEG.
	visitas := c.pres.visitas
	if _, ok := recorrerZonas(nil, ctm, w, h, c.zonas, &visitas); !ok {
		return nil, c.imagenDemasiado(nombre, w, h)
	}
	c.pres.pixeles -= w * h

	// Una máscara de color (/Mask como array) compara valores ANTES de pasar a
	// RGB, y aquí se vuelve a codificar: no se puede conservar con garantías.
	conAlfa := false
	switch c.doc.resolver(st.Dict["Mask"]).(type) {
	case pdfArray:
		return nil, c.imagenIlegible(nombre, "usa una máscara por color")
	case *pdfStream:
		conAlfa = true
	}
	if _, ok := c.doc.resolver(st.Dict["SMask"]).(*pdfStream); ok {
		conAlfa = true
	}

	base := pdfDict{
		"Type": pdfName("XObject"), "Subtype": pdfName("Image"),
		"Width": int64(w), "Height": int64(h),
	}
	for _, k := range []pdfName{"Interpolate", "OC"} {
		if v, ok := st.Dict[k]; ok {
			base[k] = v
		}
	}
	if esMascara(l, st.Dict) {
		return c.mascaraCensurada(l, st, ctm, nombre, base, w, h)
	}

	rgba, gris, esJPEG, err := c.decodificarImagen(l, st, nombre)
	if err != nil {
		return nil, err
	}
	w2, h2 := rgba.Bounds().Dx(), rgba.Bounds().Dy()
	mask, n, ok := mascaraDeZona(ctm, w2, h2, c.zonas, &c.pres.visitas)
	if !ok {
		return nil, c.imagenDemasiado(nombre, w2, h2)
	}
	if n == 0 {
		return nil, errNadaQueCensurar
	}
	alfa := ennegrecerZona(rgba, mask, conAlfa)

	nuevo, err := codificarImagenCensurada(rgba, base, gris, esJPEG, c.espacioCalibrado(st, gris))
	if err != nil {
		return nil, c.imagenIlegible(nombre, "no se pudo volver a codificar")
	}
	if alfa != nil {
		nuevo.smask = &xobjNuevo{
			dict: pdfDict{
				"Type": pdfName("XObject"), "Subtype": pdfName("Image"),
				"Width": int64(w2), "Height": int64(h2), "ColorSpace": pdfName("DeviceGray"),
				"BitsPerComponent": int64(8), "Filter": pdfName("FlateDecode"),
			},
			datos: comprimirFlate(alfa),
		}
	}
	return nuevo, nil
}

// espacioCalibrado devuelve el espacio de color del original si es calibrado y de
// los mismos canales que la imagen nueva (ICCBased de 3 o 1 componentes, CalRGB,
// CalGray): sus muestras ya son las que se decodificaron, así que se conservan
// el color y, en un PDF/A, la conformidad. Pasarla a DeviceRGB sin una
// intención de salida RGB la incumplía (6.2.3.3 de PDF/A-1). nil = de dispositivo.
func (c *censor) espacioCalibrado(st *pdfStream, gris bool) pdfObj {
	cs := st.Dict["ColorSpace"]
	arr, ok := c.doc.resolver(cs).(pdfArray)
	if !ok || len(arr) < 2 {
		return nil
	}
	familia, _ := c.doc.resolver(arr[0]).(pdfName)
	switch familia {
	case "CalRGB":
		if !gris {
			return cs
		}
	case "CalGray":
		if gris {
			return cs
		}
	case "ICCBased":
		perfil, _ := c.doc.resolver(arr[1]).(*pdfStream)
		if perfil == nil {
			return nil
		}
		n, _ := comoEntero(c.doc.resolver(perfil.Dict["N"]))
		if (n == 3 && !gris) || (n == 1 && gris) {
			return cs
		}
	}
	return nil
}

// codificarImagenCensurada escribe el mapa de bits como imagen nueva: JPEG si
// llegó como JPEG, y Flate sin pérdida en el resto.
func codificarImagenCensurada(rgba *image.NRGBA, base pdfDict, gris, esJPEG bool, calibrado pdfObj) (*xobjNuevo, error) {
	dict := pdfDict{}
	for k, v := range base {
		dict[k] = v
	}
	dict["Width"], dict["Height"] = int64(rgba.Bounds().Dx()), int64(rgba.Bounds().Dy())
	dict["BitsPerComponent"] = int64(8)
	dict["ColorSpace"] = pdfName("DeviceRGB")
	if calibrado != nil && !gris {
		dict["ColorSpace"] = calibrado
	}
	if esJPEG {
		// Una foto llegó con pérdida y se devuelve con pérdida: pasarla a Flate la
		// multiplicaría por diez. El alfa va aparte, así que se opaca el RGB.
		opacar(rgba)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, rgba, &jpeg.Options{Quality: calidadJPEGCensura}); err != nil {
			return nil, err
		}
		dict["Filter"] = pdfName("DCTDecode")
		return &xobjNuevo{dict: dict, datos: buf.Bytes()}, nil
	}
	if gris {
		dict["ColorSpace"] = pdfName("DeviceGray")
	}
	if calibrado != nil {
		dict["ColorSpace"] = calibrado
	}
	dict["Filter"] = pdfName("FlateDecode")
	return &xobjNuevo{dict: dict, datos: muestrasFlate(rgba, gris)}, nil
}

// decodificarImagen lleva la imagen a un mapa de bits RGBA no premultiplicado y
// dice si es gris (se vuelve a escribir en un canal) y si venía como JPEG.
func (c *censor) decodificarImagen(l *lienzo, st *pdfStream, nombre pdfName) (*image.NRGBA, bool, bool, error) {
	datos, terminal, err := c.doc.datosDeFlujoHasta(st)
	if err != nil {
		return nil, false, false, fmt.Errorf("%w: no se puede leer la imagen /%s de la zona: %w",
			ErrRedaccionNoGarantizada, nombre, err)
	}
	esJPEG := terminal == "DCTDecode" || terminal == "DCT"
	if esJPEG {
		// El tamaño REAL de un JPEG va dentro y puede no coincidir con el declarado.
		cfg, _, err := image.DecodeConfig(bytes.NewReader(datos))
		if err != nil {
			return nil, false, false, c.imagenIlegible(nombre, "no se puede decodificar")
		}
		if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPixelesRedaccion {
			return nil, false, false, c.imagenDemasiado(nombre, cfg.Width, cfg.Height)
		}
	}
	href, ok := l.datosImagen(st, c.recursos)
	if !ok {
		return nil, false, false, c.imagenIlegible(nombre, "no se puede decodificar (formato no "+
			"soportado) y el texto podría estar en sus píxeles")
	}
	img := l.imagenParaRaster(href)
	if img == nil {
		return nil, false, false, c.imagenIlegible(nombre, "no se puede decodificar")
	}
	gris := false
	if !esJPEG {
		ec := l.espacioDeImagen(st.Dict, c.recursos)
		gris = ec.clase != "Indexed" && ec.componentes() == 1
	}
	rgba, ok := img.(*image.NRGBA)
	if !ok {
		b := img.Bounds()
		rgba = image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
	}
	if w, h := rgba.Bounds().Dx(), rgba.Bounds().Dy(); w <= 0 || h <= 0 || w*h > maxPixelesRedaccion {
		return nil, false, false, c.imagenDemasiado(nombre, w, h)
	}
	return rgba, gris, esJPEG, nil
}

// ennegrecerZona pone a negro opaco los píxeles marcados. Si la imagen lleva
// máscara suave, devuelve su canal alfa con la zona también opaca: una silueta
// recortada en la máscara es información, igual que el color.
func ennegrecerZona(img *image.NRGBA, mask []byte, conAlfa bool) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var alfa []byte
	if conAlfa {
		alfa = make([]byte, w*h)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*img.Stride + x*4
			if alfa != nil {
				alfa[y*w+x] = img.Pix[o+3]
			}
			if mask[y*w+x] == 0 {
				continue
			}
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = 0, 0, 0, 255
			if alfa != nil {
				alfa[y*w+x] = 255
			}
		}
	}
	return alfa
}

// opacar deja el alfa a 255 en todo el mapa de bits.
func opacar(img *image.NRGBA) {
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.Pix[y*img.Stride+x*4+3] = 255
		}
	}
}

// mascaraCensurada rehace una imagen-máscara (estarcido): la zona queda sin pintar.
func (c *censor) mascaraCensurada(l *lienzo, st *pdfStream, ctm matrizPDF, nombre pdfName,
	base pdfDict, w, h int,
) (*xobjNuevo, error) {
	bits, ok := l.muestrasDe(st)
	if !ok {
		return nil, fmt.Errorf("%w: la imagen-máscara /%s de la zona no se puede decodificar",
			ErrRedaccionNoGarantizada, nombre)
	}
	mask, n, ok := mascaraDeZona(ctm, w, h, c.zonas, &c.pres.visitas)
	if !ok {
		return nil, c.imagenDemasiado(nombre, w, h)
	}
	if n == 0 {
		return nil, errNadaQueCensurar
	}
	porFila := (w + 7) / 8
	datos := make([]byte, porFila*h)
	copy(datos, bits)
	// En una máscara la muestra 0 PINTA salvo que /Decode diga lo contrario.
	sinPintar := byte(1)
	decode := l.reales(primeroDe(st.Dict, "Decode", "D"))
	if len(decode) >= 1 && decode[0] == 1 {
		sinPintar = 0
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if mask[y*w+x] == 0 {
				continue
			}
			i, bit := y*porFila+x/8, uint(7-x%8)
			datos[i] = datos[i]&^(1<<bit) | sinPintar<<bit
		}
	}
	dict := pdfDict{}
	for k, v := range base {
		dict[k] = v
	}
	dict["ImageMask"] = true
	dict["BitsPerComponent"] = int64(1)
	if len(decode) >= 2 {
		dict["Decode"] = pdfArray{int64(decode[0]), int64(decode[1])}
	}
	dict["Filter"] = pdfName("FlateDecode")
	return &xobjNuevo{dict: dict, datos: comprimirFlate(datos)}, nil
}

// muestrasFlate empaqueta el mapa de bits como RGB (o gris) de 8 bits y lo comprime.
func muestrasFlate(img *image.NRGBA, gris bool) []byte {
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	n := 3
	if gris {
		n = 1
	}
	fila := make([]byte, w*n)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*img.Stride + x*4
			if gris {
				fila[x] = img.Pix[o]
			} else {
				fila[x*3], fila[x*3+1], fila[x*3+2] = img.Pix[o], img.Pix[o+1], img.Pix[o+2]
			}
		}
		_, _ = zw.Write(fila)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func comprimirFlate(datos []byte) []byte {
	var buf bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	_, _ = zw.Write(datos)
	_ = zw.Close()
	return buf.Bytes()
}

// --- Escribir los objetos nuevos ---------------------------------------------

// recursosCensurados construye los /Resources de la página censurada: los de
// origen, sin los XObject que se han sustituido del todo y con los nuevos. Si la
// censura no ha cambiado ningún recurso, `cambian` es false y la página copia los suyos.
func (e *PDFEnsamblador) recursosCensurados(doc *PDFDoc, n *nivelCensura) (recursos pdfDict, cambian bool, err error) {
	quitar := n.originalesQueSobran()
	if len(n.nuevos) == 0 && len(quitar) == 0 {
		return nil, false, nil
	}
	if n.noQuitar && len(quitar) > 0 {
		return nil, false, fmt.Errorf("%w: un objeto incrustado sin recursos propios comparte los de la "+
			"página y no se puede quitar el original sin romperlo", ErrRedaccionNoGarantizada)
	}
	out := pdfDict{}
	for _, k := range clavesOrdenadas(n.recursos) {
		if k != "XObject" {
			out[k] = e.copiarValor(doc, n.recursos[k])
		}
	}
	xo := pdfDict{}
	if orig, ok := doc.resolver(n.recursos["XObject"]).(pdfDict); ok {
		for _, nombre := range clavesOrdenadas(orig) {
			if !quitar[nombre] {
				xo[nombre] = e.copiarValor(doc, orig[nombre])
			}
		}
	}
	nombres := make([]string, 0, len(n.nuevos))
	for nombre := range n.nuevos {
		nombres = append(nombres, string(nombre))
	}
	sort.Strings(nombres)
	for _, nombre := range nombres {
		num, err := e.materializarCensura(doc, n.nuevos[pdfName(nombre)])
		if err != nil {
			return nil, false, err
		}
		xo[pdfName(nombre)] = pdfRef{Num: num}
	}
	out["XObject"] = xo
	return out, true, nil
}

// materializarCensura escribe un objeto nuevo en el documento de salida.
func (e *PDFEnsamblador) materializarCensura(doc *PDFDoc, x *xobjNuevo) (int, error) {
	num := e.reservar()
	dict := pdfDict{}
	for _, k := range clavesOrdenadas(x.dict) {
		if claveQueNoViaja(k) {
			continue // procedencia: /Metadata, /PieceInfo… como en cualquier otra copia
		}
		dict[k] = e.copiarValor(doc, x.dict[k])
	}
	if x.smask != nil {
		sn, err := e.materializarCensura(doc, x.smask)
		if err != nil {
			return 0, err
		}
		dict["SMask"] = pdfRef{Num: sn}
	}
	if x.nivel != nil {
		r, cambian, err := e.recursosCensurados(doc, x.nivel)
		if err != nil {
			return 0, err
		}
		if !cambian {
			// Sin cambios de recursos, pero un formulario con recursos propios los
			// sigue necesitando.
			r, _ = e.copiarValor(doc, x.nivel.recursos).(pdfDict)
		}
		dict["Resources"] = r
	}
	e.cuerpos[num-1] = objetoSalida{valor: dict, flujo: x.datos, esFlujo: true}
	return num, nil
}
