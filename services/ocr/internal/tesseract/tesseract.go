// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Envoltorio de Tesseract 5 (Apache-2.0), el motor de OCR autoalojado que usa
// Sytue PDF. Ver ADR-0034.
//
// SE INVOCA EL BINARIO, NO SE ENLAZA LA BIBLIOTECA. Enlazar `libtesseract` con
// cgo metería C en un binario que hoy es estático, obligaría a compilar con la
// cadena de C en cada plataforma y ataría el ciclo de vida del proceso al de una
// biblioteca que puede caerse con un fichero corrupto. Un proceso hijo se puede
// matar por tiempo, se lleva su propia memoria y no puede tumbar el servidor.
//
// EL FORMATO DE SALIDA ES TSV Y NO hOCR. Los dos traen las cajas; el TSV se lee
// con `strings.Split` y el hOCR obligaría a parsear HTML con expresiones
// regulares sobre un atributo `title`. Cuando dos formatos dan lo mismo, gana el
// que no necesita un analizador.
package tesseract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Palabra es un trozo de texto reconocido, en PÍXELES de la imagen y con el
// origen arriba-izquierda, que es como los da Tesseract.
type Palabra struct {
	Texto     string  `json:"texto"`
	X         int     `json:"x"`
	Y         int     `json:"y"`
	Ancho     int     `json:"ancho"`
	Alto      int     `json:"alto"`
	Confianza float64 `json:"confianza"`
	// Linea agrupa las palabras de un mismo renglón. Lo usa quien quiera
	// reconstruir párrafos; para la capa de texto de un PDF basta la palabra.
	Linea int `json:"linea"`
}

// Resultado es lo que devuelve un reconocimiento.
type Resultado struct {
	Palabras []Palabra `json:"palabras"`
	// Idiomas son los que se usaron de verdad, que pueden no ser los pedidos si
	// alguno no estaba: se devuelve para que la interfaz lo pueda decir.
	Idiomas string `json:"idiomas"`
}

var (
	// ErrNoDisponible indica que el binario de Tesseract no está en la imagen.
	// Es un error de DESPLIEGUE, no del usuario, y merece decirse distinto.
	ErrNoDisponible = errors.New("ocr: el motor de reconocimiento no está instalado")
	// ErrIdiomaDesconocido salta cuando se pide un idioma que no está instalado.
	ErrIdiomaDesconocido = errors.New("ocr: idioma no disponible")
	// ErrTiempoAgotado salta cuando una página tarda más de lo permitido.
	ErrTiempoAgotado = errors.New("ocr: el reconocimiento tardó demasiado")
)

// Motor invoca Tesseract.
type Motor struct {
	binario string
	// idiomas instalados, en minúsculas.
	instalados map[string]bool
	// TiempoMaximo por página. Una imagen absurda puede tener a Tesseract
	// pensando durante minutos, y esto es un servicio compartido.
	TiempoMaximo time.Duration
}

// TiempoPorDefecto es lo que se le da a una página. Una A4 escaneada a 300 ppp
// tarda entre uno y cuatro segundos; sesenta es margen de sobra y a la vez un
// techo que impide que una petición ate un núcleo indefinidamente.
const TiempoPorDefecto = 60 * time.Second

// Nuevo localiza el binario y averigua qué idiomas hay instalados.
//
// Se pregunta AL ARRANCAR y no en cada petición: la lista no cambia mientras el
// pod vive, y consultarla por petición sería un proceso más por página.
func Nuevo(ctx context.Context) (*Motor, error) {
	binario, err := exec.LookPath("tesseract")
	if err != nil {
		return nil, ErrNoDisponible
	}
	m := &Motor{binario: binario, instalados: map[string]bool{}, TiempoMaximo: TiempoPorDefecto}

	ctx, cancelar := context.WithTimeout(ctx, 15*time.Second)
	defer cancelar()
	var salida bytes.Buffer
	cmd := exec.CommandContext(ctx, binario, "--list-langs")
	cmd.Stdout = &salida
	cmd.Stderr = &salida
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoDisponible, err)
	}
	for _, linea := range strings.Split(salida.String(), "\n") {
		linea = strings.TrimSpace(linea)
		// La primera línea es «List of available languages…»; los idiomas no
		// llevan espacios, así que se distinguen sin depender del texto.
		if linea == "" || strings.Contains(linea, " ") {
			continue
		}
		m.instalados[strings.ToLower(linea)] = true
	}
	if len(m.instalados) == 0 {
		return nil, fmt.Errorf("%w: no hay ningún idioma instalado", ErrNoDisponible)
	}
	return m, nil
}

// Idiomas devuelve los instalados, ordenados.
func (m *Motor) Idiomas() []string {
	out := make([]string, 0, len(m.instalados))
	for l := range m.instalados {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// maxImagenBytes acota lo que se acepta reconocer.
const maxImagenBytes = 40 << 20

// Reconocer pasa una imagen por Tesseract y devuelve sus palabras.
func (m *Motor) Reconocer(ctx context.Context, imagen []byte, idiomas string) (Resultado, error) {
	var out Resultado
	if len(imagen) == 0 {
		return out, errors.New("ocr: la imagen está vacía")
	}
	if len(imagen) > maxImagenBytes {
		return out, errors.New("ocr: la imagen es demasiado grande")
	}
	lista, err := m.normalizarIdiomas(idiomas)
	if err != nil {
		return out, err
	}
	out.Idiomas = lista

	// El fichero temporal es obligatorio: Tesseract lee de stdin con «-», pero
	// entonces no puede rebobinar para detectar el formato y falla con varios
	// PNG perfectamente válidos. Un fichero en /tmp cuesta menos que ese fallo.
	dir, err := os.MkdirTemp("", "sytue-ocr-")
	if err != nil {
		return out, fmt.Errorf("ocr: no se pudo preparar el trabajo: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	entrada := filepath.Join(dir, "pagina")
	if err := os.WriteFile(entrada, imagen, 0o600); err != nil {
		return out, fmt.Errorf("ocr: no se pudo escribir la imagen: %w", err)
	}

	ctx, cancelar := context.WithTimeout(ctx, m.TiempoMaximo)
	defer cancelar()

	var stdout, stderr bytes.Buffer
	// G204: los dos únicos valores que vienen de fuera son la RUTA del temporal
	// —que la genera este proceso— y `lista`, que `normalizarIdiomas` ya ha
	// validado contra los modelos instalados y contra un alfabeto de solo letras
	// minúsculas. Además no hay shell de por medio: `exec.Command` pasa el vector
	// de argumentos tal cual, así que no hay nada que interpretar.
	cmd := exec.CommandContext(ctx, m.binario, //nolint:gosec // G204: ver arriba
		entrada, "stdout",
		"-l", lista,
		// PSM 3: página completa con segmentación automática. Es el modo correcto
		// para un documento escaneado; los modos de «una sola línea» dan mejor
		// resultado en un recorte y peor en una página, que es lo que llega aquí.
		"--psm", "3",
		"tsv",
	)
	// El directorio de trabajo es el temporal: Tesseract escribe ficheros
	// auxiliares junto al de entrada, y en un sistema de solo lectura fallaría.
	cmd.Dir = dir
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return out, ErrTiempoAgotado
		}
		return out, fmt.Errorf("ocr: el reconocimiento falló: %s", primeraLinea(stderr.String()))
	}

	out.Palabras = palabrasDeTSV(stdout.String())
	return out, nil
}

// normalizarIdiomas valida la lista pedida contra la instalada.
//
// Se RECHAZA lo que no está en vez de caer al inglés: reconocer un documento en
// español con el modelo inglés no da un error, da un texto plausible y mal, que
// es mucho peor —nadie lo revisa porque parece que funcionó—.
func (m *Motor) normalizarIdiomas(pedidos string) (string, error) {
	pedidos = strings.TrimSpace(pedidos)
	if pedidos == "" {
		if m.instalados["spa"] && m.instalados["eng"] {
			return "spa+eng", nil
		}
		return m.Idiomas()[0], nil
	}
	var out []string
	for _, l := range strings.Split(pedidos, "+") {
		l = strings.ToLower(strings.TrimSpace(l))
		if l == "" {
			continue
		}
		// Solo letras: el idioma acaba en una línea de comandos, y aunque
		// `exec.Command` no usa shell, un valor con rutas o guiones podría
		// interpretarse como una opción más de Tesseract.
		if !soloLetras(l) {
			return "", fmt.Errorf("%w: %q", ErrIdiomaDesconocido, l)
		}
		if !m.instalados[l] {
			return "", fmt.Errorf("%w: %q", ErrIdiomaDesconocido, l)
		}
		out = append(out, l)
	}
	if len(out) == 0 {
		return "", fmt.Errorf("%w: la lista está vacía", ErrIdiomaDesconocido)
	}
	if len(out) > 4 {
		// Cada idioma extra multiplica el trabajo; con más de cuatro, el
		// reconocimiento empeora además de tardar.
		return "", errors.New("ocr: demasiados idiomas a la vez (máximo 4)")
	}
	return strings.Join(out, "+"), nil
}

func soloLetras(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return s != ""
}

// confianzaMinima descarta lo que Tesseract mismo da por dudoso. Por debajo de
// 30 son casi siempre manchas del escaneo interpretadas como letras, y meterlas
// en la capa de texto ensucia la búsqueda del documento entero.
const confianzaMinima = 30.0

// palabrasDeTSV interpreta la salida tabulada de Tesseract.
//
// Columnas (§ la cabecera que emite): level, page_num, block_num, par_num,
// line_num, word_num, left, top, width, height, conf, text.
func palabrasDeTSV(tsv string) []Palabra {
	var out []Palabra
	lineaActual := 0
	ultimoBloque, ultimaLinea := -1, -1

	for i, fila := range strings.Split(tsv, "\n") {
		if i == 0 || strings.TrimSpace(fila) == "" {
			continue // cabecera y líneas vacías
		}
		campos := strings.Split(fila, "\t")
		if len(campos) < 12 {
			continue
		}
		// Nivel 5 es «palabra»; los otros son página, bloque, párrafo y línea, y
		// traen la caja del grupo entero, no la del texto.
		if campos[0] != "5" {
			continue
		}
		texto := strings.TrimSpace(campos[11])
		if texto == "" {
			continue
		}
		conf, err := strconv.ParseFloat(campos[10], 64)
		if err != nil || conf < confianzaMinima {
			continue
		}
		x, err1 := strconv.Atoi(campos[6])
		y, err2 := strconv.Atoi(campos[7])
		w, err3 := strconv.Atoi(campos[8])
		h, err4 := strconv.Atoi(campos[9])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil || w <= 0 || h <= 0 {
			continue
		}

		bloque, _ := strconv.Atoi(campos[2])
		linea, _ := strconv.Atoi(campos[4])
		if bloque != ultimoBloque || linea != ultimaLinea {
			lineaActual++
			ultimoBloque, ultimaLinea = bloque, linea
		}

		out = append(out, Palabra{
			Texto: texto, X: x, Y: y, Ancho: w, Alto: h,
			Confianza: conf, Linea: lineaActual,
		})
	}
	return out
}

func primeraLinea(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	if s == "" {
		return "sin detalle"
	}
	return s
}
