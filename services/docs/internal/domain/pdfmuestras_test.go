// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Genera las muestras de SVG que audita el navegador en el CI.
//
// POR QUÉ ESTO ES UN TEST Y NO UN COMANDO. El motor vive en `internal/`, así que
// nada fuera de `services/docs` puede importarlo — un `tools/` no compilaría—, y
// añadir un binario al servicio solo para esto engordaría su imagen con algo que
// producción no usa. Un test que escribe ficheros cuando se lo piden no cuesta
// nada y se queda donde está el motor.
//
// POR QUÉ HACE FALTA UN NAVEGADOR. Todo lo demás en este paquete comprueba la
// CADENA del SVG: que ponga `fill="#ff0000"`, que la matriz sea la esperada. Eso
// no dice que la página se PINTE. Un SVG puede ser correcto carácter a carácter
// y salir en blanco —un `clipPath` que recorta todo, una fuente que no carga, un
// `viewBox` que deja el contenido fuera—, y ninguno de esos fallos tiene forma de
// aparecer en un `strings.Contains`.
//
// Uso (lo hace el CI, ver .github/workflows/ci-frontend.yml):
//
//	SYTUE_PDF_MUESTRAS=/tmp/muestras go test ./services/docs/internal/domain -run TestMuestras
package domain

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"git.sytue.eu/sytue/one/platform/pkg/pdfsimple"
)

// muestraNavegador es un caso que se va a mirar en un navegador de verdad.
type muestraNavegador struct {
	// nombre da el fichero: `<nombre>.svg`.
	nombre string
	// contenido es el flujo de la página; vacío usa el escritor de la casa.
	contenido string
	extras    string
	objs      []string
	// texto, si no está vacío, es lo que el navegador debe poder seleccionar.
	texto string
	// construir, si está, produce el PDF entero. Lo necesitan las muestras que
	// no son un flujo de página suelto sino el resultado de una operación —un
	// formulario creado y relleno, por ejemplo—.
	construir func(t *testing.T) []byte
}

// TestMuestrasParaNavegador escribe los SVG en el directorio de
// `SYTUE_PDF_MUESTRAS`. Sin esa variable no hace nada: es una herramienta, no una
// comprobación, y no debe ensuciar el disco de quien corre los tests normales.
func TestMuestrasParaNavegador(t *testing.T) {
	destino := os.Getenv("SYTUE_PDF_MUESTRAS")
	if destino == "" {
		t.Skip("sin SYTUE_PDF_MUESTRAS: no se generan muestras")
	}
	if err := os.MkdirAll(destino, 0o750); err != nil {
		t.Fatalf("no se pudo crear %s: %v", destino, err)
	}

	for _, m := range muestrasDelNavegador() {
		var datos []byte
		switch {
		case m.construir != nil:
			datos = m.construir(t)
		case m.contenido == "" && m.texto != "" && len(m.objs) == 0 && m.extras == "":
			w := pdfsimple.Nuevo()
			w.Texto(m.texto, "F1", 24)
			datos = w.Bytes()
		default:
			datos = pdfConContenido(t, m.contenido, m.extras, m.objs...)
		}
		d, err := LeerPDF(datos)
		if err != nil {
			t.Fatalf("%s: LeerPDF: %v", m.nombre, err)
		}
		svg, err := d.RenderizarPaginaSVG(0, OpcionesRender{})
		if err != nil {
			t.Fatalf("%s: render: %v", m.nombre, err)
		}
		ruta := filepath.Join(destino, m.nombre+".svg")
		if err := os.WriteFile(ruta, []byte(svg), 0o600); err != nil {
			t.Fatalf("%s: escribir: %v", m.nombre, err)
		}
	}
	t.Logf("muestras escritas en %s", destino)
}

// muestrasDelNavegador son los casos elegidos por lo que ROMPEN si se rompen, no
// por cubrir operadores: cada uno falla de una forma que la cadena del SVG no
// delata.
func muestrasDelNavegador() []muestraNavegador {
	const fuente = `/Resources << /Font << /F1 << /Type /Font /Subtype /Type1 ` +
		`/BaseFont /Helvetica /FirstChar 32 /LastChar 126 >> >> >>`

	return []muestraNavegador{
		// El texto del escritor de la casa: si el volteo o la fuente fallan, sale
		// en blanco o cabeza abajo, y las dos cosas se ven pero no se leen.
		{nombre: "texto", texto: "Sytue PDF"},

		// Un cuadrado ROJO pegado al borde INFERIOR del papel. Es el caso que caza
		// el eje volteado del revés: la cadena del SVG es idéntica en los dos
		// casos —lo único que cambia es un signo dentro de la matriz— y solo
		// mirando dónde acaba el píxel se sabe cuál era.
		{
			nombre:    "abajo-izquierda",
			contenido: "1 0 0 rg 0 0 30 30 re f",
		},

		// Recorte: la mitad de un rectángulo grande. Un `clipPath` mal cerrado no
		// da error, deja la página EN BLANCO.
		{
			nombre:    "recorte",
			contenido: "q 0 50 100 50 re W n 0 0 1 rg 0 0 100 100 re f Q",
		},

		// Degradado. Traducir mal las paradas produce un bloque de color plano, que
		// en el texto del SVG sigue pareciendo un `<linearGradient>` correcto.
		{
			nombre:    "degradado",
			contenido: "q 0 0 100 100 re W n /Sh0 sh Q",
			extras: `/Resources << /Shading << /Sh0 << /ShadingType 2 /ColorSpace /DeviceRGB ` +
				`/Coords [0 0 100 0] /Function << /FunctionType 2 /Domain [0 1] ` +
				`/C0 [1 0 0] /C1 [0 0 1] /N 1 >> >> >> >>`,
		},

		// Texto invisible (modo 3), que es la capa de OCR de un escaneo. Tiene que
		// poder SELECCIONARSE sin verse: si se emite con `display:none` o `fill:none`
		// deja de estar en la selección, y eso no lo distingue ningún assert de cadena.
		{
			nombre:    "ocr-invisible",
			contenido: "BT 3 Tr /F1 20 Tf 10 50 Td (texto oculto) Tj ET",
			extras:    fuente,
		},

		// Un campo de formulario CREADO sobre un impreso, y relleno. Se mira en el
		// navegador porque los dos fallos de esta pieza dan un SVG impecable: si
		// el widget no cuelga del /Annots de la página, o si su apariencia no se
		// referencia, el campo simplemente NO ESTÁ y el texto del SVG no lo dice.
		{
			nombre: "campo-relleno",
			texto:  "Ana",
			construir: func(t *testing.T) []byte {
				// El impreso de partida es de 100×100 puntos, como el resto de las
				// muestras, y no el A4 del escritor de la casa: la auditoría mide qué
				// FRACCIÓN de la página lleva tinta, y un campo de 80×20 sobre un A4
				// es el 0,2 % del papel —indistinguible de la página en blanco que
				// esta puerta existe para cazar—.
				base, err := LeerPDF(pdfConContenido(t,
					"BT /F1 12 Tf 8 80 Td (impreso) Tj ET", fuente))
				if err != nil {
					t.Fatalf("LeerPDF del impreso: %v", err)
				}
				conCampo, err := AnadirCamposPDF(base, []CampoNuevo{{
					Tipo: CampoTexto, Nombre: "nombre", Pagina: 0,
					Rect: [4]float64{10, 40, 90, 60},
				}})
				if err != nil {
					t.Fatalf("AnadirCamposPDF: %v", err)
				}
				d, err := LeerPDF(conCampo)
				if err != nil {
					t.Fatalf("LeerPDF: %v", err)
				}
				relleno, err := RellenarFormularioPDF(d, map[string]string{"nombre": "Ana"})
				if err != nil {
					t.Fatalf("RellenarFormularioPDF: %v", err)
				}
				return relleno
			},
		},

		// Una imagen en escala de grises. Si el `data:` sale mal formado el
		// navegador no pinta nada y no se queja.
		{
			nombre:    "imagen",
			contenido: "q 80 0 0 80 10 10 cm /Im0 Do Q",
			extras:    `/Resources << /XObject << /Im0 5 0 R >> >>`,
			objs: []string{fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 "+
				"/ColorSpace /DeviceGray /BitsPerComponent 8 /Length %d >>\nstream\n%s\nendstream",
				4, "\x00\x40\x80\xff")},
		},
	}
}
