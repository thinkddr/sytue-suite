// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Las etiquetas JSON de este fichero NO siguen la convención de la casa, y es a
// propósito: son las que el motor manda y espera. `Permisos` y `Huella` van en
// PascalCase y el resto en snake_case porque así las escribe el servicio.
// Renombrarlas para contentar al linter rompería el trato con el BFF en silencio —el
// campo llegaría vacío, no habría error—, así que se silencia el linter y no el
// protocolo.
//
//nolint:tagliatelle // las etiquetas son las del motor, no nuestras: ver arriba
package certipdf

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

/*
Los tipos que cruzan la frontera con el BFF (`/api/pdf` y `/api/pdf-aplicar`), portados
de `apps/frontend/packages/office/src/pdf/api.ts`.

Las etiquetas JSON son las de la web PORQUE ES EL MISMO SERVICIO: el motor contesta
`solo_lectura`, `cubre_todo` y `tamano_bytes`, y renombrarlas aquí sería hablar otro
idioma con el mismo interlocutor.
*/

// Rect es un rectángulo del PDF, `[x0 y0 x1 y1]`. Se escribe como vector y no como
// estructura porque es lo que el motor manda y recibe, y traducir en cada frontera es
// donde se cuelan los ejes cambiados.
type Rect = [4]float64

// Pagina es una página tal y como la describe el motor.
type Pagina struct {
	Numero   int     `json:"numero"`
	Ancho    float64 `json:"ancho"`
	Alto     float64 `json:"alto"`
	Rotacion int     `json:"rotacion"`
	// Caja es el MediaBox ordenado [x0 y0 x1 y1].
	Caja Rect `json:"caja"`
	// Extracto son las primeras palabras. Vacío: la página no tiene texto. Nil: el motor
	// no llegó a leerla —abrir no espera a leerlas todas—, que NO es lo mismo.
	Extracto *string `json:"extracto,omitempty"`
}

// AnotacionLeida es una anotación que ya está en el documento.
type AnotacionLeida struct {
	Pagina int         `json:"pagina"`
	Tipo   string      `json:"tipo"`
	Rect   Rect        `json:"rect"`
	Texto  string      `json:"texto,omitempty"`
	Autor  string      `json:"autor,omitempty"`
	Fecha  string      `json:"fecha,omitempty"`
	Color  *[3]float64 `json:"color,omitempty"`
	// RespuestaA es el índice, en esta misma lista, del comentario al que contesta
	// (/IRT). Nil si no contesta a ninguno.
	RespuestaA *int `json:"respuesta_a,omitempty"`
	// Estado es el de revisión (/State), en INGLÉS porque es lo que lee Acrobat.
	Estado string `json:"estado,omitempty"`
}

// Campo es un campo de formulario del documento.
type Campo struct {
	Nombre       string   `json:"nombre"`
	Etiqueta     string   `json:"etiqueta,omitempty"`
	Tipo         string   `json:"tipo"`
	Valor        string   `json:"valor,omitempty"`
	Opciones     []string `json:"opciones,omitempty"`
	Pagina       int      `json:"pagina"`
	Rect         Rect     `json:"rect"`
	Obligatorio  bool     `json:"obligatorio"`
	SoloLectura  bool     `json:"solo_lectura"`
	VariasLineas bool     `json:"varias_lineas,omitempty"`
	Contrasena   bool     `json:"contrasena,omitempty"`
}

// FirmaLeida es lo que una firma DECLARA. Nada de esto está verificado: ver el
// comentario de paquete, punto 1.
type FirmaLeida struct {
	Nombre string `json:"nombre,omitempty"`
	Motivo string `json:"motivo,omitempty"`
	Lugar  string `json:"lugar,omitempty"`
	Fecha  string `json:"fecha,omitempty"`
	Campo  string `json:"campo,omitempty"`
	// CubreTodo dice si la firma abarca el fichero entero. Es el único dato de una
	// firma que esta vista puede afirmar sin verificar nada, porque sale de la
	// estructura del fichero y no de la criptografía.
	CubreTodo bool `json:"cubre_todo"`
}

// Permisos son los del documento, tal y como los declara su diccionario de cifrado.
type Permisos struct {
	Imprimir            bool `json:"Imprimir"`
	Modificar           bool `json:"Modificar"`
	Copiar              bool `json:"Copiar"`
	Anotar              bool `json:"Anotar"`
	RellenarCampos      bool `json:"RellenarCampos"`
	Accesibilidad       bool `json:"Accesibilidad"`
	Ensamblar           bool `json:"Ensamblar"`
	ImprimirEnAlta      bool `json:"ImprimirEnAlta"`
	SoloConClaveDeDueno bool `json:"SoloConClaveDeDueno"`
}

// Huella son los metadatos del documento.
type Huella struct {
	Autor       string `json:"Autor"`
	Creador     string `json:"Creador"`
	Productor   string `json:"Productor"`
	Titulo      string `json:"Titulo"`
	Asunto      string `json:"Asunto"`
	Palabras    string `json:"Palabras"`
	Creado      string `json:"Creado"`
	Modificado  string `json:"Modificado"`
	ConXMP      bool   `json:"ConXMP"`
	ConAdjuntos bool   `json:"ConAdjuntos"`
	ConJS       bool   `json:"ConJS"`
}

// Marcador es una entrada del índice del documento (/Outlines).
type Marcador struct {
	Titulo string `json:"titulo"`
	// Pagina es 0-based; -1 si el destino no se pudo resolver.
	Pagina  int  `json:"pagina"`
	Nivel   int  `json:"nivel"`
	Abierto bool `json:"abierto,omitempty"`
}

// Enlace es una zona pulsable de una página.
type Enlace struct {
	Pagina int  `json:"pagina"`
	Rect   Rect `json:"rect"`
	// URL es externa y YA viene filtrada por el servidor (solo http/https/mailto).
	URL string `json:"url,omitempty"`
	// Destino es la página 0-based de destino; -1 si el enlace es externo.
	Destino int `json:"destino"`
}

// Adjunto es un fichero incrustado en el PDF (una factura electrónica lleva su XML así).
type Adjunto struct {
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion,omitempty"`
	TamanoBytes int64  `json:"tamano_bytes"`
	// Tipo es el MIME que DECLARA el documento: informativo, no decide cómo se sirve.
	Tipo string `json:"tipo,omitempty"`
	// Pagina es donde está pegado, o -1 si cuelga del documento entero.
	Pagina int `json:"pagina"`
}

// Contenido es lo que devuelve `op: contenido`: todo lo que la pantalla necesita saber
// del documento sin haber dibujado ni una página.
type Contenido struct {
	Paginas     []Pagina         `json:"paginas"`
	Total       int              `json:"total"`
	Indice      []Marcador       `json:"indice"`
	Enlaces     []Enlace         `json:"enlaces"`
	Adjuntos    []Adjunto        `json:"adjuntos"`
	Anotaciones []AnotacionLeida `json:"anotaciones"`
	Campos      []Campo          `json:"campos"`
	Firmas      []FirmaLeida     `json:"firmas"`
	Cifrado     bool             `json:"cifrado"`
	Permisos    Permisos         `json:"permisos"`
	Huella      Huella           `json:"huella"`
}

// AnotacionNueva es una anotación que se pide poner (`op: anotar`): el `AnotacionNueva`
// de `api.ts`, con los mismos campos opcionales.
type AnotacionNueva struct {
	Pagina   int            `json:"pagina"`
	Tipo     string         `json:"tipo"`
	Rect     *Rect          `json:"rect,omitempty"`
	Cuadros  []Rect         `json:"cuadros,omitempty"`
	Trazos   [][][2]float64 `json:"trazos,omitempty"`
	Puntos   [][2]float64   `json:"puntos,omitempty"`
	Color    [3]float64     `json:"color"`
	Opacidad float64        `json:"opacidad"`
	Grosor   float64        `json:"grosor"`
	Texto    string         `json:"texto,omitempty"`
	Autor    string         `json:"autor,omitempty"`
	Tamano   float64        `json:"tamano,omitempty"`
	Etiqueta string         `json:"etiqueta,omitempty"`
}

// ColorARGB es `colorARGB`: `#rrggbb` → componentes de 0 a 1, que es como el PDF
// guarda el color.
func ColorARGB(hex string) [3]float64 {
	n, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return [3]float64{}
	}
	return [3]float64{float64(n>>16&255) / 255, float64(n>>8&255) / 255, float64(n&255) / 255}
}

// Fragmento es un trozo de texto con su posición, en coordenadas del PDF.
type Fragmento struct {
	Texto  string  `json:"texto"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Tamano float64 `json:"tamano"`
}

/* --------------------------------------------------------------------------
 * Coordenadas
 *
 * El render entrega la página en coordenadas de SVG —origen arriba-izquierda, Y hacia
 * abajo, y ya con /Rotate aplicado—; el motor de anotaciones espera las del PDF —origen
 * abajo-izquierda, sin rotar—. Convertir mal es el fallo que deja el resaltado media
 * página más arriba, así que la vuelta se escribe UNA vez y se usa en todos los sitios.
 *
 * Es traducción literal de `aPDF`/`aSVG` en `pdf/api.ts`: el mismo documento se anota
 * desde el navegador y desde aquí, y dos conversiones que no coincidan ponen la misma
 * marca en dos sitios distintos.
 * ------------------------------------------------------------------------ */

// APDF pasa de coordenadas del SVG dibujado a coordenadas del PDF.
func APDF(p Pagina, sx, sy float64) (float64, float64) {
	x0, y0, x1, y1 := p.Caja[0], p.Caja[1], p.Caja[2], p.Caja[3]
	switch p.Rotacion {
	case 90:
		return sy + x0, sx + y0
	case 180:
		return x1 - sx, sy + y0
	case 270:
		return x1 - sy, y1 - sx
	}
	return sx + x0, y1 - sy
}

// ASVG pasa de coordenadas del PDF a las del SVG dibujado. Es la inversa exacta de
// [APDF], y `TestLasCoordenadasVanYVuelven` lo comprueba en las cuatro rotaciones.
func ASVG(p Pagina, px, py float64) (float64, float64) {
	x0, y0, x1, y1 := p.Caja[0], p.Caja[1], p.Caja[2], p.Caja[3]
	switch p.Rotacion {
	case 90:
		return py - y0, px - x0
	case 180:
		return x1 - px, py - y0
	case 270:
		return y1 - py, x1 - px
	}
	return px - x0, y1 - py
}

// CajaSVG es un rectángulo ya en coordenadas del SVG dibujado.
type CajaSVG struct{ X, Y, Ancho, Alto float64 }

// RectEnSVG pasa un rectángulo del PDF a caja del SVG.
func RectEnSVG(p Pagina, r Rect) CajaSVG {
	ax, ay := ASVG(p, r[0], r[1])
	bx, by := ASVG(p, r[2], r[3])
	return CajaSVG{
		X:     math.Min(ax, bx),
		Y:     math.Min(ay, by),
		Ancho: math.Abs(bx - ax),
		Alto:  math.Abs(by - ay),
	}
}

/* -------------------------------------------------------------------------- */

// Tramo es un trozo del documento con su título, listo para `op: extraer`.
type Tramo struct {
	Titulo string
	Rango  string
}

// TramosDelIndice son los tramos en que el índice parte el documento.
//
// Cada marcador de primer nivel abre un tramo y el siguiente lo cierra; el último llega
// al final. Es «dividir por marcadores» de Foxit, y sale entero de dos cosas que ya
// existen —el índice leído y extraer un rango—, así que no necesita endpoint propio.
//
// Portado de `tramosDelIndice` con su test (`tramos.test.ts`) traído entero: es
// aritmética de bordes, que es donde falla sola.
func TramosDelIndice(indice []Marcador, total int) []Tramo {
	// Los que no dicen a qué página van no pueden abrir un tramo, y un subcapítulo
	// partiría el capítulo por la mitad.
	capitulos := make([]Marcador, 0, len(indice))
	for _, m := range indice {
		if m.Nivel == 0 && m.Pagina >= 0 {
			capitulos = append(capitulos, m)
		}
	}
	out := make([]Tramo, 0, len(capitulos))
	for i, m := range capitulos {
		desde := m.Pagina + 1
		hasta := total
		if i+1 < len(capitulos) {
			hasta = max(desde, capitulos[i+1].Pagina)
		}
		rango := strconv.Itoa(desde)
		if desde != hasta {
			rango = strconv.Itoa(desde) + "-" + strconv.Itoa(hasta)
		}
		out = append(out, Tramo{Titulo: m.Titulo, Rango: rango})
	}
	return out
}

// TextoParaLeer junta los fragmentos de una página en texto seguido, para leerlo en voz
// alta o mandarlo al asistente.
//
// Un PDF no guarda párrafos: guarda trozos colocados. «Estimado» y «cliente» pueden ser
// dos fragmentos de la misma línea, y leerlos por separado suena entrecortado —la voz
// hace una pausa donde no la hay—. Se agrupan por altura y se ordenan por posición, que
// es como se leería en papel.
func TextoParaLeer(frags []Fragmento) string {
	if len(frags) == 0 {
		return ""
	}
	// Dos fragmentos son de la misma línea si sus bases caen dentro de media altura de
	// letra. Con un umbral fijo, un pie de página de 6 pt y un título de 30 pt no
	// pueden compartir criterio.
	orden := make([]Fragmento, len(frags))
	copy(orden, frags)
	sort.SliceStable(orden, func(i, j int) bool {
		if orden[i].Y != orden[j].Y {
			return orden[i].Y > orden[j].Y
		}
		return orden[i].X < orden[j].X
	})

	var lineas [][]Fragmento
	for _, f := range orden {
		if n := len(lineas); n > 0 {
			base := lineas[n-1][0]
			if math.Abs(base.Y-f.Y) <= math.Max(base.Tamano, f.Tamano)*0.5 {
				lineas[n-1] = append(lineas[n-1], f)
				continue
			}
		}
		lineas = append(lineas, []Fragmento{f})
	}

	salida := make([]string, 0, len(lineas))
	for _, l := range lineas {
		sort.SliceStable(l, func(i, j int) bool { return l[i].X < l[j].X })
		trozos := make([]string, 0, len(l))
		for _, f := range l {
			if s := strings.TrimSpace(f.Texto); s != "" {
				trozos = append(trozos, s)
			}
		}
		// Sin espacio entre trozos, «Estimado»+«cliente» se leería como una sola
		// palabra inventada.
		if linea := strings.Join(trozos, " "); linea != "" {
			salida = append(salida, linea)
		}
	}
	return strings.Join(salida, ". ")
}
