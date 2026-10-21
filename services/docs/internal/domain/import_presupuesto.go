// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"bytes"
	"cmp"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp" // las imágenes que el navegador sabe leer de un data: URL

	"git.sytue.eu/sytue/one/platform/pkg/miniatura"
)

// # Lo convertido tiene que CABER donde se siembra
//
// Lo que devuelve Import se siembra con UNA operación `set` del registro del documento
// (`open-from-drive.ts`, la importación de la lista y, al guardar, cada autoguardado de
// la web), y el cuerpo de una operación no pasa de 1 MiB (`httpx.DecodeJSON`,
// `maxBodyBytes`). El importador metía las imágenes del fichero tal cual, en base64, y
// una presentación con un par de fotos pasa de largo: la conversión salía bien, sembrarla
// devolvía 413 y quien abría un .odp desde Drive volvía a la lista con «no se pudo
// abrir» —y un documento vacío anclado al fichero, que es lo que encontraba la vez
// siguiente—. Pasaba igual con un .pptx, un .docx o un .odt con fotos: es de la pieza
// común, no del formato.
//
// La regla es la que ya aplica la web al insertar una imagen en una diapositiva
// (`encodeSlideImage` en Insertar.tsx): lado mayor de 1600 px, JPEG de calidad 82 —PNG
// solo si la imagen tiene transparencia— y, si aún no cabe, tres cuartos del lado y siete
// puntos menos de calidad por paso. Aquí se aplica a TODAS las imágenes a la vez hasta
// que el contenido entero cabe; lo que ya cabía no se toca. El fichero de Drive no
// cambia: esto es la copia que se edita.

// MaxContenidoSembrado es lo que puede ocupar lo convertido, medido como irá en el cuerpo
// de la operación que lo siembra (una cadena JSON, con su escapado). Es el
// `MODEL_MAX_CHARS` de SlidesCanvas.tsx: holgura bajo el MiB para el sobre de la operación.
const MaxContenidoSembrado = 900_000

// ErrDemasiadoGrande es lo convertido que no cabe ni con las imágenes al mínimo: se
// dice que no cabe en vez de crear un documento que no se puede sembrar.
var ErrDemasiadoGrande = errors.New("docs: lo convertido no cabe en un documento")

// # Y se tiene que cortar MIENTRAS se convierte
//
// Mirar el tamaño al final no basta: hasta ahí el importador ya ha levantado el modelo
// entero en memoria. Un .xlsx de 14 MB (80 MB de XML) llegaba a 2 GB antes de fallar, y
// con un .docx, un .odt, un .pptx, un .odp o un .ods de unos cientos de KB comprimidos
// pasaba igual (TestImportarGrandeCortaAlConvertir): dos importaciones así a la vez tumban
// el pod. Por eso cada importación lleva un presupuesto, el mismo para todos los formatos:
// el decodificador de la parte con el contenido le carga el texto que lee
// (decodificadorConPresupuesto) y las hojas, además, cada celda que colocan (una cadena
// compartida o una celda repetida se leen una vez y se escriben muchas). En cuanto se
// agota, el decodificador da fin de fichero, todos los bucles de lectura terminan solos y
// `importar` devuelve ErrDemasiadoGrande sin terminar el modelo.
//
// Lo que se carga es una estimación, y por arriba: hay texto que se lee y no llega (un
// `mc:Fallback` que repite el de su `mc:Choice`, un campo que se sustituye por su valor).
// Por eso el corte está en el doble de lo que cabe: nunca rechaza lo que habría cabido y
// la memoria queda acotada igual. Las imágenes no cuentan: se reducen después.

// presupuesto es lo que lleva convertido una importación. nil no corta nunca (las pruebas
// que llaman a un importador suelto).
//
// Al recortar un libro que no cabe (recortarLibro) lleva además el tope de filas: de cada
// hoja se leen las filas 1..tope y nada más. Entonces solo se cargan las celdas que se
// colocan, no el texto que se lee: la tabla de cadenas compartidas se lee entera (cada hoja
// coge de ella lo suyo) y las filas de más se saltan sin gastar.
type presupuesto struct {
	usado      int
	recortando bool
	tope       int
	techo      int // lo que se puede cargar antes de agotarse; 0 = el doble de lo que cabe
}

// recorta dice si la fila `n` (desde 1) queda fuera del tope de un libro recortado.
func (p *presupuesto) recorta(n int) bool { return p != nil && p.recortando && n > p.tope }

// gastar carga n bytes de lo convertido.
func (p *presupuesto) gastar(n int) {
	if p != nil {
		p.usado += n
	}
}

// agotado dice si lo convertido ya no puede caber en un documento.
func (p *presupuesto) agotado() bool {
	return p != nil && p.usado > cmp.Or(p.techo, 2*MaxContenidoSembrado)
}

// decodificador lee una parte cargando al presupuesto su texto y dando fin de fichero en
// cuanto se agota. Es un *xml.Decoder corriente: el código que lo usa no cambia.
func (p *presupuesto) decodificador(raw []byte) *xml.Decoder {
	d := xml.NewDecoder(bytes.NewReader(raw))
	if p == nil {
		return d
	}
	return xml.NewTokenDecoder(lectorConPresupuesto{d, p})
}

type lectorConPresupuesto struct {
	d *xml.Decoder
	p *presupuesto
}

func (l lectorConPresupuesto) Token() (xml.Token, error) {
	if l.p.agotado() {
		return nil, io.EOF
	}
	t, err := l.d.RawToken()
	if c, ok := t.(xml.CharData); ok && !l.p.recortando && len(bytes.TrimSpace(c)) > 0 {
		l.p.gastar(len(c))
	}
	return t, err
}

// Los pasos de `encodeSlideImage`, con un suelo de lado: por debajo de 64 px una imagen
// ya no dice nada, y seguir encogiendo solo retrasa el «no cabe».
const (
	ladoImagenMax   = 1600
	calidadInicial  = 82
	calidadMinima   = 50
	pasoCalidad     = 7
	ladoImagenMin   = 64
	maxPasosImagen  = 12
	factorPasoLado  = 0.75
	prefijoDataURLs = "data:image/"
)

// reDataURL es una imagen en un data: URL, dentro del HTML de un documento o del JSON
// de una hoja o una presentación: el base64 no lleva comillas ni nada que se escape.
var reDataURL = regexp.MustCompile(`data:(image/[a-zA-Z0-9.+-]+);base64,([A-Za-z0-9+/=]+)`)

// tamanoSembrado es lo que ocupa `s` como valor de la operación: una cadena JSON.
func tamanoSembrado(s string) int {
	b, _ := json.Marshal(s)
	return len(b)
}

// imagenIncrustada es una imagen de lo convertido: dónde está y, si se puede leer, su
// imagen y si es PNG con transparencia.
type imagenIncrustada struct {
	desde, hasta int
	img          image.Image
	conAlfa      bool
}

// caberEnElDocumento devuelve `content` tal cual si cabe y, si no, con sus imágenes
// reducidas lo justo para que quepa. ErrDemasiadoGrande si ni así.
func caberEnElDocumento(content string) (string, error) {
	if tamanoSembrado(content) <= MaxContenidoSembrado {
		return content, nil
	}
	if !strings.Contains(content, prefijoDataURLs) {
		return "", ErrDemasiadoGrande
	}
	var imgs []imagenIncrustada
	for _, m := range reDataURL.FindAllStringSubmatchIndex(content, -1) {
		mime := content[m[2]:m[3]]
		datos, err := base64.StdEncoding.DecodeString(content[m[4]:m[5]])
		if err != nil {
			continue
		}
		img, ok := decodificarAcotada(datos)
		if !ok {
			continue // un SVG, un EMF: se queda como está y cuenta lo que ocupa
		}
		imgs = append(imgs, imagenIncrustada{
			desde: m[0], hasta: m[1], img: img,
			conAlfa: mime != "image/jpeg" && tieneAlfa(img),
		})
	}
	if len(imgs) == 0 {
		return "", ErrDemasiadoGrande
	}
	calidad := calidadInicial
	escala := 1.0
	for paso := 0; paso < maxPasosImagen; paso++ {
		candidato, ok := reescribirImagenes(content, imgs, escala, calidad)
		if ok && tamanoSembrado(candidato) <= MaxContenidoSembrado {
			return candidato, nil
		}
		escala *= factorPasoLado
		calidad = max(calidadMinima, calidad-pasoCalidad)
	}
	return "", ErrDemasiadoGrande
}

// reescribirImagenes vuelve a escribir `content` con cada imagen reducida a `escala` de
// su lado de partida (el menor de 1600 y el suyo) y a esa calidad. Siempre desde la
// imagen ORIGINAL: recomprimir lo ya recomprimido acumula artefactos en cada paso.
func reescribirImagenes(content string, imgs []imagenIncrustada, escala float64, calidad int) (string, bool) {
	var b strings.Builder
	b.Grow(len(content))
	ultimo := 0
	for _, im := range imgs {
		lado := max(im.img.Bounds().Dx(), im.img.Bounds().Dy())
		objetivo := max(int(float64(min(ladoImagenMax, lado))*escala+0.5), min(ladoImagenMin, lado))
		reducida := miniatura.Reducir(im.img, objetivo)
		var cod bytes.Buffer
		mime := "image/jpeg"
		var err error
		if im.conAlfa {
			mime = "image/png"
			err = png.Encode(&cod, reducida)
		} else {
			err = jpeg.Encode(&cod, reducida, &jpeg.Options{Quality: calidad})
		}
		if err != nil {
			return "", false
		}
		b.WriteString(content[ultimo:im.desde])
		b.WriteString("data:" + mime + ";base64,")
		b.WriteString(base64.StdEncoding.EncodeToString(cod.Bytes()))
		ultimo = im.hasta
	}
	b.WriteString(content[ultimo:])
	return b.String(), true
}

// decodificarAcotada lee una imagen sin dejar que su cabecera decida cuánta memoria se
// gasta: primero las dimensiones, y solo si son razonables, los píxeles.
func decodificarAcotada(datos []byte) (image.Image, bool) {
	img, err := miniatura.Decodificar(datos)
	return img, err == nil
}

// tieneAlfa dice si algún píxel es translúcido: decide PNG frente a JPEG, como
// `canvasHasAlpha` en la web.
func tieneAlfa(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	return true
}

// # Un libro que no cabe se abre recortado, como copia aparte
//
// Decisión del dueño (2026-10-16). Un .xlsx cuyo contenido no cabe entero en un
// documento —el documento es un solo valor que se siembra y se guarda en una operación de
// menos de 1 MiB, y una hoja de 100 000 filas no cabe ni con un modelo más compacto— se abre
// con las primeras N filas de cada hoja, la N más alta con la que cabe, y una hoja «Aviso»
// delante que dice qué falta y que el original no se ha tocado. El resultado lleva
// `Truncated`: quien lo abre desde Drive (open-from-drive.ts) crea un documento aparte, sin
// anclarlo al fichero, de modo que «Guardar en Drive» no existe para él y guardar no puede
// sobrescribir el original con la copia recortada.

// recortarLibro abre recortado un libro que no cabe entero: lo convertido y sus claves, y si
// de verdad se ha recortado alguna hoja (un libro que solo no cabía por el presupuesto, que
// carga por arriba, puede caber entero al contar solo sus celdas). ErrDemasiadoGrande si no
// cabe ni sin filas (lo que ocupan sus objetos, por ejemplo).
func recortarLibro(f srcFormat, data []byte, idioma string) (string, map[string]ClaveWire, bool, error) {
	files, err := readZip(data)
	if err != nil {
		return "", nil, false, err
	}
	// ponytail: solo .xlsx. En un .ods todas las hojas van en content.xml, y saltar las filas
	// de más obliga a leerlas en cada intento (en un .xlsx se salta al cierre de sheetData);
	// un .ods que no cabe sigue sin abrirse hasta que se parta content.xml por hojas.
	if f != fmtXlsx {
		return "", nil, false, ErrDemasiadoGrande
	}
	l, err := abrirLibroXlsx(files, &presupuesto{recortando: true})
	if err != nil {
		return "", nil, false, err
	}
	// leer convierte el libro con las n primeras filas de cada hoja. El techo es holgado: el
	// modelo de la primera n que no cabe se reutiliza para buscar, recortándolo, la que sí.
	leer := func(n int) *sheetModel {
		return l.modelo(&presupuesto{recortando: true, tope: n, techo: 4 * MaxContenidoSembrado})
	}
	type intento struct {
		contenido string
		claves    map[string]ClaveWire
		recortado bool
		cabe      bool
	}
	// medir mira si el libro leído cabe con las n primeras filas de cada hoja y su aviso.
	medir := func(m *sheetModel, n int) intento {
		if m == nil {
			return intento{} // agotado: ya pasaba de lo que cabe
		}
		r := *m
		r.Sheets = make([]sheetData, 0, len(m.Sheets)+1)
		usados := map[string]bool{}
		var cortadas []string
		for _, h := range m.Sheets {
			h = h.hasta(n)
			r.Sheets = append(r.Sheets, h)
			usados[h.Name] = true
			if h.recortada {
				cortadas = append(cortadas, h.Name)
			}
		}
		if len(cortadas) > 0 {
			lineas := append([]string{strings.ReplaceAll(avisoImportar(idioma, "texto.recortado"), "{n}", strconv.Itoa(n))}, cortadas...)
			r.Sheets = append([]sheetData{hojaConAviso(idioma, f, lineas, usados)}, r.Sheets...)
		}
		contenido, claves := extraerClavesImportadas(marshalSheetModel(&r))
		contenido, err := caberEnElDocumento(contenido)
		return intento{contenido, claves, len(cortadas) > 0, err == nil}
	}
	mejor := medir(leer(0), 0)
	if !mejor.cabe {
		return "", nil, false, ErrDemasiadoGrande // ni sin filas: sus objetos no caben
	}
	// La N más alta que cabe: se dobla hasta la primera que no y se busca entre las dos,
	// recortando el modelo de esa (sin volver a leer el XML), o leyendo si se agotó.
	// lo es la N más alta que cabe y hi, la primera que no.
	lo := 0
	var hi int
	var mHi *sheetModel
	for n := 256; ; n *= 2 {
		m := leer(n)
		r := medir(m, n)
		if !r.cabe {
			hi, mHi = n, m
			break
		}
		lo, mejor = n, r
		if !r.recortado || n >= maxFilasHoja {
			return r.contenido, r.claves, r.recortado, nil // cabe entero
		}
	}
	for hi-lo > 1 {
		mitad := lo + (hi-lo)/2
		m := mHi
		if m == nil {
			m = leer(mitad)
		}
		if r := medir(m, mitad); r.cabe {
			lo, mejor = mitad, r
		} else {
			hi = mitad
		}
	}
	return mejor.contenido, mejor.claves, mejor.recortado, nil
}

// hasta es la hoja con solo sus filas 1..n: celdas, estilos, formatos y altos, y recortada si
// tenía algo más abajo. Lo demás (objetos, reglas, combinadas…) queda como al leerla.
func (sd sheetData) hasta(n int) sheetData {
	out := sd
	out.Cells = filasHasta(sd.Cells, n, &out.recortada)
	out.Styles = filasHasta(sd.Styles, n, &out.recortada)
	out.Formats = filasHasta(sd.Formats, n, nil)
	if sd.RowHeights != nil {
		out.RowHeights = map[string]float64{}
		for k, v := range sd.RowHeights {
			if i, err := strconv.Atoi(k); err == nil && i < n {
				out.RowHeights[k] = v
			}
		}
	}
	return out
}

// filasHasta es el mapa por celda (A1) sin las de las filas de debajo de n; apunta en
// `recortada` si quitó alguna.
func filasHasta[V any](m map[string]V, n int, recortada *bool) map[string]V {
	if m == nil {
		return nil
	}
	out := make(map[string]V, len(m))
	for ref, v := range m {
		if r, _, ok := ParseA1(ref); ok && r > n {
			if recortada != nil {
				*recortada = true
			}
			continue
		}
		out[ref] = v
	}
	return out
}
