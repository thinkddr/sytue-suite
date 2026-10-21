// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package domain

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"maps"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Medida de la importación contra ficheros REALES de terceros: .xlsx y .pptx de Excel,
// PowerPoint, Numbers, Keynote, Google, LibreOffice y de las bibliotecas que los escriben,
// sacados de proyectos que publican sus ficheros de prueba con licencia Apache-2.0, MIT o
// BSD (nunca GPL ni AGPL). El corpus pesa cientos de MB y no está en el repositorio: lo trae
// `testdata/corpus-terceros/traer.sh`. La tabla de antes y después está en
// docs/architecture.md#corpus-terceros.
//
//	SYTUE_CORPUS_TERCEROS=/ruta/del/corpus SYTUE_CORPUS_SALIDA=/ruta/salida \
//	  go test ./services/docs/internal/domain -run TestCorpusTerceros -v
//
// Con SYTUE_CORPUS_MODELOS=/ruta deja además el modelo de cada fichero que se abre, para
// pintarlo en el editor (test/e2e/importados.mjs).
//
// Sin SYTUE_CORPUS_TERCEROS se salta. Lo que cuenta de cada fichero sale del PAQUETE, leído
// aquí con un recorrido propio, y se compara con el modelo que devuelve Import (la misma
// puerta que usan la web y las apps nativas):
//
//   - si se abre, falla o hace entrar en pánico al importador (o tarda más de 30 s);
//   - hojas, celdas con valor y fórmulas; diapositivas y caracteres de texto de las formas;
//   - color: celdas con relleno o con color de letra, pestañas con color, reglas de formato
//     condicional con relleno; diapositivas con fondo propio o de su diseño, formas con
//     relleno (también el de su estilo, `p:style/a:fillRef`), celdas de tabla con relleno y
//     texto resaltado;
//   - qué elementos del paquete no nombra el importador en ningún sitio, por frecuencia (una
//     cota: un nombre que el importador cita puede seguir perdiéndose en un caso concreto).

// corpusTiempoMax es lo que puede tardar un fichero. Holgado: el contenedor de los agentes es
// compartido, y con carga un fichero normal tardaba diez veces más y salía como «tiempo».
const corpusTiempoMax = 120 * time.Second

type cuenta struct{ De, Ok int }

func (c *cuenta) sumar(o cuenta) { c.De += o.De; c.Ok += o.Ok }

func (c cuenta) pct() string {
	if c.De == 0 {
		return "—"
	}
	return strconv.FormatFloat(100*float64(c.Ok)/float64(c.De), 'f', 1, 64) + " %"
}

// medidaCorpus es lo medido en un fichero; los nombres de las cuentas son las columnas.
type medidaCorpus struct {
	Fichero, Formato, Programa string
	Estado                     string // ok, error, panico, tiempo, memoria, caida
	Motivo                     string
	MemoriaMB                  int // lo más que ocupó el proceso que lo importó
	Cuentas                    map[string]cuenta
}

var columnasCorpus = map[string][]string{
	"xlsx": {
		"hojas", "celdas", "formulas", "relleno", "color de letra", "pestaña", "relleno condicional",
		"borde con color", "borde con trazo", "letra condicional",
	},
	"pptx": {
		"diapositivas", "texto", "texto de más", "fondo", "fondo de imagen", "relleno de forma", "relleno de tabla", "resaltado",
		"marcador: tamaño", "marcador: alineación", "marcador: color", "formas del diseño",
	},
}

func TestCorpusTerceros(t *testing.T) {
	dir := os.Getenv("SYTUE_CORPUS_TERCEROS")
	if dir == "" {
		t.Skip("SYTUE_CORPUS_TERCEROS no está: el corpus de terceros no viaja con el repositorio")
	}
	var ficheros []string
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && formatoCorpus(p) != "" && !strings.Contains(p, "/src/") {
			ficheros = append(ficheros, p)
		}
		return nil
	})
	slices.Sort(ficheros)
	vocab := vocabularioImportador(t)
	censo := map[string]map[string]int{"xlsx": {}, "pptx": {}}
	medidas := make([]medidaCorpus, len(ficheros))
	// Cada fichero en un proceso aparte, con su reloj y su tope de memoria: uno que se cuelga o
	// que agota la memoria es una medida, no el final de la medida.
	turnos := make(chan struct{}, 2)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, f := range ficheros {
		wg.Add(1)
		turnos <- struct{}{}
		go func() {
			defer func() { <-turnos; wg.Done() }()
			m, c := medirEnProceso(t, f, false)
			if m.Estado == "tiempo" || m.Estado == "memoria" || m.Estado == "caida" {
				m2, c2 := medirEnProceso(t, f, true) // solo contar lo que había
				m2.Estado, m2.Motivo, m2.MemoriaMB = m.Estado, m.Motivo, m.MemoriaMB
				m, c = m2, c2
			}
			mu.Lock()
			defer mu.Unlock()
			medidas[i] = m
			for k, v := range c {
				censo[m.Formato][k] += v
			}
		}()
	}
	wg.Wait()
	resumen := resumenCorpus(medidas, censo, vocab)
	t.Log("\n" + resumen)
	if out := os.Getenv("SYTUE_CORPUS_SALIDA"); out != "" {
		if err := os.MkdirAll(out, 0o750); err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		for _, m := range medidas {
			fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%dMB\t%s", filepath.Base(m.Fichero), m.Formato, m.Programa, m.Estado, m.MemoriaMB, m.Motivo)
			for _, k := range columnasCorpus[m.Formato] {
				fmt.Fprintf(&b, "\t%s=%d/%d", k, m.Cuentas[k].Ok, m.Cuentas[k].De)
			}
			b.WriteString("\n")
		}
		if err := os.WriteFile(filepath.Join(out, "ficheros.tsv"), []byte(b.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, "resumen.md"), []byte(resumen), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// Tope de memoria de cada proceso: el doble de lo que se le pide de verdad al servicio, para
// que un fichero que se pasa se vea como «memoria» y no tumbe el contenedor.
const corpusMemoriaMax = 4 << 30

// TestCorpusTercerosFichero es el proceso hijo de TestCorpusTerceros: mide un fichero y escribe
// la medida en una línea «CORPUS:{json}».
func TestCorpusTercerosFichero(t *testing.T) {
	f := os.Getenv("SYTUE_CORPUS_FICHERO")
	if f == "" {
		t.Skip("solo como proceso hijo de TestCorpusTerceros")
	}
	limitarMemoria(corpusMemoriaMax)
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	censo := map[string]int{}
	m := medirFicheroCorpus(f, data, censo, os.Getenv("SYTUE_CORPUS_SOLO_CONTAR") != "")
	b, _ := json.Marshal(struct {
		M medidaCorpus
		C map[string]int
	}{m, censo})
	fmt.Println("CORPUS:" + string(b))
}

func medirEnProceso(t *testing.T, f string, soloContar bool) (medidaCorpus, map[string]int) {
	tope := corpusTiempoMax
	if soloContar {
		tope *= 5 // contar lo que hay no puede quedarse sin hacer: es la mitad de la medida
	}
	ctx, cancel := context.WithTimeout(context.Background(), tope)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCorpusTercerosFichero$", "-test.v")
	cmd.Env = append(os.Environ(), "SYTUE_CORPUS_FICHERO="+f, "SYTUE_CORPUS_TERCEROS=")
	if soloContar {
		cmd.Env = append(cmd.Env, "SYTUE_CORPUS_SOLO_CONTAR=1")
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	mb := 0
	if cmd.ProcessState != nil {
		mb = memoriaMaxMB(cmd.ProcessState)
	}
	for _, l := range strings.Split(out.String(), "\n") {
		if j, ok := strings.CutPrefix(l, "CORPUS:"); ok {
			var r struct {
				M medidaCorpus
				C map[string]int
			}
			if json.Unmarshal([]byte(j), &r) == nil {
				r.M.MemoriaMB = mb
				return r.M, r.C
			}
		}
	}
	m := medidaCorpus{Fichero: f, Formato: formatoCorpus(f), Cuentas: map[string]cuenta{}, MemoriaMB: mb}
	todo := out.String() + errb.String()
	switch {
	case ctx.Err() != nil:
		m.Estado, m.Motivo = "tiempo", "más de "+corpusTiempoMax.String()
	case strings.Contains(todo, "out of memory"):
		m.Estado, m.Motivo = "memoria", "más de "+strconv.Itoa(corpusMemoriaMax>>30)+" GiB"
	default:
		m.Estado, m.Motivo = "caida", recortar(fmt.Sprint(err)+" "+lineaDeError(todo), 160)
	}
	if strings.Contains(todo, "panic:") {
		m.Estado = "panico"
	}
	return m, nil
}

// lineaDeError es la primera línea de un pánico o un error fatal de la salida de un proceso.
func lineaDeError(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(l, "panic:") || strings.HasPrefix(l, "fatal error:") {
			return l
		}
	}
	return ""
}

func formatoCorpus(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".xlsx", ".xlsm":
		return "xlsx"
	case ".pptx", ".pptm":
		return "pptx"
	}
	return ""
}

// medirFicheroCorpus importa un fichero (con un reloj y recuperando un pánico) y lo compara
// con lo que dice su paquete.
func medirFicheroCorpus(nombre string, data []byte, censo map[string]int, soloContar bool) medidaCorpus {
	m := medidaCorpus{Fichero: nombre, Formato: formatoCorpus(nombre), Cuentas: map[string]cuenta{}}
	files, zerr := corpusZip(data)
	m.Programa = programaCorpus(files)
	type salida struct {
		res *ImportResult
		err error
		pan any
	}
	var s salida
	if !soloContar {
		func() {
			defer func() {
				if r := recover(); r != nil {
					s = salida{pan: r}
				}
			}()
			res, err := Import(filepath.Base(nombre), data)
			s = salida{res: res, err: err}
		}()
	}
	switch {
	case soloContar:
		m.Estado = "contado"
	case s.pan != nil:
		m.Estado, m.Motivo = "panico", recortar(fmt.Sprint(s.pan), 120)
	case s.err != nil:
		m.Estado, m.Motivo = "error", recortar(s.err.Error(), 120)
		if bytes.HasPrefix(data, []byte("\xD0\xCF\x11\xE0")) {
			m.Motivo = "cifrado o binario (CFB): " + m.Motivo
		}
	default:
		m.Estado = "ok"
	}
	if zerr != nil {
		return m
	}
	for parte, raw := range files {
		if tipo := tipoDeParte(m.Formato, parte); tipo != "" && strings.HasSuffix(parte, ".xml") {
			censarParte(censo, tipo, raw)
		}
	}
	if m.Estado != "ok" {
		// Lo que el paquete tenía se cuenta igual: un fichero que no se abre lo pierde todo.
		s.res = &ImportResult{}
	} else if dir := os.Getenv("SYTUE_CORPUS_MODELOS"); dir != "" {
		// El modelo tal cual llega al editor, para pintarlo (test/e2e/importados.mjs).
		_ = os.WriteFile(filepath.Join(dir, filepath.Base(nombre)+".json"), []byte(s.res.Content), 0o600)
	}
	if m.Formato == "xlsx" {
		var sm sheetModel
		_ = json.Unmarshal([]byte(s.res.Content), &sm)
		medirHojasCorpus(files, &sm, m.Cuentas)
	} else {
		var pm slidesModel
		_ = json.Unmarshal([]byte(s.res.Content), &pm)
		medirDiaposCorpus(files, &pm, m.Cuentas)
	}
	return m
}

func recortar(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

// corpusZip abre el paquete sin los topes del importador (es para contar, no para importar).
func corpusZip(data []byte) (map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		var b bytes.Buffer
		_, _ = b.ReadFrom(rc)
		rc.Close()
		out[strings.TrimPrefix(path.Clean(strings.ReplaceAll(f.Name, "\\", "/")), "/")] = b.Bytes()
	}
	return out, nil
}

// programaCorpus es el programa que escribió el paquete: el `Application` de docProps/app.xml
// y, si no lo dice, sus firmas. Numbers y Keynote no se presentan; Numbers deja app.xml sin
// `Application` y declara su paleta indexada en minúsculas («ff000000»); Keynote y Google
// Slides se reconocen por el nombre de sus diseños, y Google Sheets por sus tamaños de letra
// con decimales («10.0») y la falta de docProps. Son heurísticas: se cuentan aparte.
func programaCorpus(files map[string][]byte) string {
	var app struct {
		Application string `xml:"Application"`
	}
	_ = xml.Unmarshal(files["docProps/app.xml"], &app)
	if a := strings.Join(strings.Fields(app.Application), " "); a != "" {
		if i := strings.IndexAny(a, "/$"); i > 0 {
			a = a[:i] // «LibreOffice/7.4.7.2$Linux_X86_64 …»
		}
		return a
	}
	estilos := string(files["xl/styles.xml"])
	disenos := ""
	for p, raw := range files {
		if strings.HasPrefix(p, "ppt/slideLayouts/slideLayout") {
			disenos += string(firstN(raw, 4096))
		}
	}
	switch {
	case strings.Contains(estilos, `rgb="ff000000"`) && strings.Contains(estilos, `formatCode="General"`):
		return "Numbers (firma)"
	case strings.Contains(estilos, `<sz val="10.0"/>`) || strings.Contains(estilos, `<sz val="11.0"/>`):
		return "Google Sheets (firma)"
	case strings.Contains(disenos, `name="Title &amp; Subtitle"`) || strings.Contains(disenos, `name="Title &amp; Bullets"`):
		return "Keynote (firma)"
	case strings.Contains(disenos, `name="TITLE_AND_BODY"`) || strings.Contains(disenos, `name="SECTION_HEADER"`):
		return "Google Slides (firma)"
	case len(files["docProps/app.xml"]) > 0:
		return "(app.xml sin Application)"
	}
	return "(sin docProps/app.xml)"
}

// --- Recorrido propio del paquete ---

// destinoCorpus resuelve el destino de una relación (relativo a la parte, o absoluto).
func destinoCorpus(parte, destino string) string {
	if strings.HasPrefix(destino, "/") {
		return path.Clean(destino[1:])
	}
	return path.Clean(path.Join(path.Dir(parte), destino))
}

type relCorpus struct{ ID, Type, Target, Mode string }

func relsCorpus(files map[string][]byte, parte string) []relCorpus {
	var r struct {
		Rel []struct {
			ID     string `xml:"Id,attr"`
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			Mode   string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	_ = xml.Unmarshal(files[path.Join(path.Dir(parte), "_rels", path.Base(parte)+".rels")], &r)
	out := make([]relCorpus, 0, len(r.Rel))
	for _, x := range r.Rel {
		out = append(out, relCorpus{x.ID, x.Type, destinoCorpus(parte, x.Target), x.Mode})
	}
	return out
}

func relCorpusPorID(rels []relCorpus, id string) string {
	for _, r := range rels {
		if r.ID == id {
			return r.Target
		}
	}
	return ""
}

func relCorpusPorTipo(rels []relCorpus, sufijo string) []string {
	var out []string
	for _, r := range rels {
		if strings.HasSuffix(r.Type, "/"+sufijo) {
			out = append(out, r.Target)
		}
	}
	return out
}

// parteRaiz es la parte principal del paquete (`officeDocument` de _rels/.rels).
func parteRaiz(files map[string][]byte, porDefecto string) string {
	for _, r := range relsCorpus(files, "") {
		if strings.HasSuffix(r.Type, "/officeDocument") {
			return r.Target
		}
	}
	return porDefecto
}

// --- Hojas ---

type xfCorpus struct{ relleno, colorLetra, bordeColor, bordeTrazo bool }

// colorCorpus es un `<color>` de styles.xml.
type colorCorpus struct {
	Auto    string `xml:"auto,attr"`
	RGB     string `xml:"rgb,attr"`
	Theme   string `xml:"theme,attr"`
	Indexed string `xml:"indexed,attr"`
}

// visible dice si el color es uno que se ve distinto del de serie: ni «auto» ni negro (el
// rgb, el tx1 del tema o los negros de la paleta indexada).
func (c *colorCorpus) visible() bool {
	if c == nil || c.Auto == "1" || c.Auto == "true" {
		return false
	}
	negro := strings.HasSuffix(strings.ToUpper(c.RGB), "000000") || c.Theme == "1" ||
		c.Indexed == "8" || c.Indexed == "64" || c.Indexed == "0"
	return (c.RGB != "" || c.Theme != "" || c.Indexed != "") && !negro
}

// estilosCorpus dice, por índice de cellXfs, si la celda lleva relleno, color de letra y un
// borde con color o con un trazo que no es el fino; y por dxf, si lleva relleno y si lleva
// color de letra.
func estilosCorpus(raw []byte) (xfs []xfCorpus, dxfConRelleno, dxfConLetra []bool) {
	var st struct {
		Fonts []struct {
			Color *struct {
				Auto    string `xml:"auto,attr"`
				RGB     string `xml:"rgb,attr"`
				Theme   string `xml:"theme,attr"`
				Indexed string `xml:"indexed,attr"`
			} `xml:"color"`
		} `xml:"fonts>font"`
		Fills []struct {
			P *struct {
				Tipo string `xml:"patternType,attr"`
			} `xml:"patternFill"`
			G *struct{} `xml:"gradientFill"`
		} `xml:"fills>fill"`
		Bordes []struct {
			Lados []struct {
				XMLName xml.Name
				Style   string       `xml:"style,attr"`
				Color   *colorCorpus `xml:"color"`
			} `xml:",any"`
		} `xml:"borders>border"`
		Xfs []struct {
			FontID   string `xml:"fontId,attr"`
			FillID   string `xml:"fillId,attr"`
			BorderID string `xml:"borderId,attr"`
		} `xml:"cellXfs>xf"`
		Dxfs []struct {
			Fill *struct {
				P *struct {
					Tipo string    `xml:"patternType,attr"`
					Fg   *struct{} `xml:"fgColor"`
					Bg   *struct{} `xml:"bgColor"`
				} `xml:"patternFill"`
				G *struct{} `xml:"gradientFill"`
			} `xml:"fill"`
			Font *struct {
				Color *colorCorpus `xml:"color"`
			} `xml:"font"`
		} `xml:"dxfs>dxf"`
	}
	_ = xml.Unmarshal(raw, &st)
	for _, x := range st.Xfs {
		var c xfCorpus
		if i, err := strconv.Atoi(x.FillID); err == nil && i >= 0 && i < len(st.Fills) {
			f := st.Fills[i]
			c.relleno = f.G != nil || f.P != nil && f.P.Tipo != "" && f.P.Tipo != "none"
		}
		if i, err := strconv.Atoi(x.FontID); err == nil && i >= 0 && i < len(st.Fonts) {
			if col := st.Fonts[i].Color; col != nil && col.Auto != "1" && col.Auto != "true" {
				negro := strings.HasSuffix(strings.ToUpper(col.RGB), "000000") || col.Theme == "1" ||
					col.Indexed == "8" || col.Indexed == "64" || col.Indexed == "0"
				c.colorLetra = (col.RGB != "" || col.Theme != "" || col.Indexed != "") && !negro
			}
		}
		if i, err := strconv.Atoi(x.BorderID); err == nil && i > 0 && i < len(st.Bordes) {
			for _, l := range st.Bordes[i].Lados {
				if lado := l.XMLName.Local; lado == "diagonal" || l.Style == "" || l.Style == "none" {
					continue
				}
				c.bordeColor = c.bordeColor || l.Color.visible()
				c.bordeTrazo = c.bordeTrazo || l.Style != "thin"
			}
		}
		xfs = append(xfs, c)
	}
	for _, d := range st.Dxfs {
		dxfConRelleno = append(dxfConRelleno, d.Fill != nil && (d.Fill.G != nil || d.Fill.P != nil && (d.Fill.P.Fg != nil || d.Fill.P.Bg != nil) && d.Fill.P.Tipo != "none"))
		dxfConLetra = append(dxfConLetra, d.Font != nil && d.Font.Color != nil && (d.Font.Color.RGB != "" || d.Font.Color.Theme != "" || d.Font.Color.Indexed != ""))
	}
	return xfs, dxfConRelleno, dxfConLetra
}

// hojaCorpus es lo que cuenta de una hoja: celdas con valor, fórmulas y estilos por referencia.
type hojaCorpus struct {
	valor, formula, relleno, colorLetra []string
	bordeColor, bordeTrazo              []string
	pestana                             bool
	condRelleno, condLetra              int
}

func leerHojaCorpus(raw []byte, xfs []xfCorpus, dxf, dxfLetra []bool) hojaCorpus {
	var h hojaCorpus
	dec := xml.NewDecoder(bytes.NewReader(raw))
	fila, col := 0, 0
	var ref, estilo string
	var conValor, conFormula, enV, enIs bool
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			switch e.Name.Local {
			case "tabColor":
				h.pestana = true
			case "row":
				if r, err := strconv.Atoi(attrLocal(e, "r")); err == nil {
					fila = r
				} else {
					fila++
				}
				col = 0
			case "c":
				ref, estilo, conValor, conFormula = attrLocal(e, "r"), attrLocal(e, "s"), false, false
				if r, c, ok := ParseA1(ref); ok {
					fila, col = r, c
				} else {
					col++
					ref = A1Ref(fila, col)
				}
			case "f":
				conFormula = true
			case "v":
				enV = true
			case "is":
				enIs = true
			case "cfRule":
				if i, err := strconv.Atoi(attrLocal(e, "dxfId")); err == nil && i >= 0 && i < len(dxf) && dxf[i] {
					h.condRelleno++
				}
				if i, err := strconv.Atoi(attrLocal(e, "dxfId")); err == nil && i >= 0 && i < len(dxfLetra) && dxfLetra[i] {
					h.condLetra++
				}
			}
		case xml.CharData:
			if (enV || enIs) && len(bytes.TrimSpace(e)) > 0 {
				conValor = true
			}
		case xml.EndElement:
			switch e.Name.Local {
			case "v":
				enV = false
			case "is":
				enIs = false
			case "c":
				if conValor || conFormula {
					h.valor = append(h.valor, ref)
				}
				if conFormula {
					h.formula = append(h.formula, ref)
				}
				if i, err := strconv.Atoi(estilo); err == nil && i >= 0 && i < len(xfs) {
					if xfs[i].relleno {
						h.relleno = append(h.relleno, ref)
					}
					if xfs[i].colorLetra && conValor {
						h.colorLetra = append(h.colorLetra, ref)
					}
					if xfs[i].bordeColor {
						h.bordeColor = append(h.bordeColor, ref)
					}
					if xfs[i].bordeTrazo {
						h.bordeTrazo = append(h.bordeTrazo, ref)
					}
				}
			}
		}
	}
	return h
}

func medirHojasCorpus(files map[string][]byte, sm *sheetModel, c map[string]cuenta) {
	libro := parteRaiz(files, "xl/workbook.xml")
	var wb struct {
		Sheets []struct {
			RID string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	_ = xml.Unmarshal(files[libro], &wb)
	rels := relsCorpus(files, libro)
	var estilos []byte
	for _, p := range relCorpusPorTipo(rels, "styles") {
		estilos = files[p]
	}
	xfs, dxf, dxfLetra := estilosCorpus(estilos)
	sumar := func(k string, de, ok int) {
		x := c[k]
		x.De += de
		x.Ok += min(ok, de)
		c[k] = x
	}
	for i, sh := range wb.Sheets {
		var hoja *sheetData
		if i < len(sm.Sheets) {
			hoja = &sm.Sheets[i]
		}
		sumar("hojas", 1, map[bool]int{true: 1}[hoja != nil])
		h := leerHojaCorpus(files[relCorpusPorID(rels, sh.RID)], xfs, dxf, dxfLetra)
		hay := func(refs []string, f func(ref string) bool) int {
			n := 0
			for _, r := range refs {
				if hoja != nil && f(r) {
					n++
				}
			}
			return n
		}
		sumar("celdas", len(h.valor), hay(h.valor, func(r string) bool { return hoja.Cells[r] != "" }))
		sumar("formulas", len(h.formula), hay(h.formula, func(r string) bool { return strings.HasPrefix(hoja.Cells[r], "=") }))
		sumar("relleno", len(h.relleno), hay(h.relleno, func(r string) bool { return hoja.Styles[r].Fill != "" }))
		sumar("color de letra", len(h.colorLetra), hay(h.colorLetra, func(r string) bool { return hoja.Styles[r].Color != "" }))
		pest, cond := 0, 0
		if hoja != nil {
			if h.pestana && hojaConPestana(hoja) {
				pest = 1
			}
			for _, r := range hoja.CondFormats {
				if r.Fill != "" {
					cond++
				}
			}
		}
		sumar("pestaña", map[bool]int{true: 1}[h.pestana], pest)
		sumar("relleno condicional", h.condRelleno, cond)
		// Del JSON y no de los campos: así la medida vale también antes de que el modelo los tenga.
		enJSON := func(v any, claves ...string) bool {
			b, _ := json.Marshal(v)
			for _, k := range claves {
				if bytes.Contains(b, []byte(`"`+k+`":"`)) {
					return true
				}
			}
			return false
		}
		sumar("borde con color", len(h.bordeColor), hay(h.bordeColor, func(r string) bool { return enJSON(hoja.Styles[r], "tc", "rc", "bc", "lc") }))
		sumar("borde con trazo", len(h.bordeTrazo), hay(h.bordeTrazo, func(r string) bool { return enJSON(hoja.Styles[r], "ts", "rs", "bs", "ls") }))
		letra := 0
		if hoja != nil {
			for _, r := range hoja.CondFormats {
				if enJSON(r, "textColor") {
					letra++
				}
			}
		}
		sumar("letra condicional", h.condLetra, letra)
	}
}

// --- Diapositivas ---

// palabrasDeA son las palabras de los `a:t` de las partes, párrafo a párrafo (un tramo puede
// partir una palabra en dos `a:t`).
func palabrasDeA(raws ...[]byte) []string {
	var out []string
	for _, raw := range raws {
		dec := xml.NewDecoder(bytes.NewReader(raw))
		en, oculta := false, false // oculta: la forma de ahora no se ve (`hidden`), y su texto tampoco
		var par strings.Builder
		for {
			tok, err := dec.Token()
			if err != nil {
				break
			}
			switch e := tok.(type) {
			case xml.StartElement:
				en = e.Name.Local == "t" && !oculta
				switch e.Name.Local {
				case "br", "tab":
					par.WriteByte(' ')
				case "sp":
					oculta = false
				case "cNvPr":
					oculta = verdadero(attrLocal(e, "hidden"))
				}
			case xml.CharData:
				if en {
					par.Write(e)
				}
			case xml.EndElement:
				en = false
				if e.Name.Local == "p" {
					out = append(out, strings.Fields(strings.ToLower(par.String()))...)
					par.Reset()
				}
			}
		}
	}
	return out
}

// reBloqueHTML son las etiquetas que separan palabras (párrafos, celdas, saltos).
var reBloqueHTML = regexp.MustCompile(`(?i)</?(p|h[1-6]|li|ul|ol|br|td|th|tr|table|tbody|thead|div|blockquote|section)\b[^>]*>`)

// palabrasDeHTML son las palabras del HTML de una caja.
func palabrasDeHTML(s string) []string {
	return strings.Fields(strings.ToLower(stripHTML(reBloqueHTML.ReplaceAllString(s, " "))))
}

// fondoDe dice qué fondo declara una parte: "", "color" o "imagen"; y el XML del `p:bg`.
func fondoDe(raw []byte) (string, string) {
	i := bytes.Index(raw, []byte(":bg>"))
	if i < 0 {
		return "", ""
	}
	j := bytes.Index(raw[i:], []byte(":bg>"))
	k := bytes.Index(raw[i+4:], []byte(":bg>"))
	if j < 0 || k < 0 {
		return "", ""
	}
	bg := string(raw[i : i+4+k])
	if strings.Contains(bg, "blipFill") {
		return "imagen", bg
	}
	return "color", bg
}

func medirDiaposCorpus(files map[string][]byte, pm *slidesModel, c map[string]cuenta) {
	pres := parteRaiz(files, "ppt/presentation.xml")
	var p struct {
		Sld []struct {
			RID string `xml:"id,attr"`
		} `xml:"sldIdLst>sldId"`
		Patrones []struct {
			RID string `xml:"id,attr"`
		} `xml:"sldMasterIdLst>sldMasterId"`
	}
	_ = xml.Unmarshal(files[pres], &p)
	rels := relsCorpus(files, pres)
	primerPatron := ""
	if len(p.Patrones) > 0 {
		primerPatron = relCorpusPorID(rels, p.Patrones[0].RID)
	}
	sumar := func(k string, de, ok int) {
		x := c[k]
		x.De += de
		x.Ok += min(ok, de)
		c[k] = x
	}
	n := 0
	disenos := map[string]bool{} // las formas de un diseño se cuentan una vez, no por diapositiva
	for _, s := range p.Sld {
		parte := relCorpusPorID(rels, s.RID)
		raw := files[parte]
		if len(raw) == 0 {
			continue
		}
		var sl *slideT
		if n < len(pm.Slides) {
			sl = &pm.Slides[n]
		}
		n++
		sumar("diapositivas", 1, map[bool]int{true: 1}[sl != nil])
		srels := relsCorpus(files, parte)
		partes := [][]byte{raw}
		for _, d := range relCorpusPorTipo(srels, "diagramDrawing") {
			partes = append(partes, files[d])
		}
		quiero := palabrasDeA(partes...)
		var tengo []string
		if sl != nil {
			for _, b := range sl.Boxes {
				tengo = append(tengo, palabrasDeHTML(b.HTML)...)
			}
		}
		bolsa := map[string]int{}
		for _, w := range tengo {
			bolsa[w]++
		}
		de, ok := 0, 0
		for _, w := range quiero {
			de += len(w)
			if bolsa[w] > 0 {
				bolsa[w]--
				ok += len(w)
			}
		}
		sumar("texto", de, ok)
		sobra := 0
		for w, k := range bolsa {
			sobra += len(w) * k
		}
		x := c["texto de más"]
		x.De += sobra
		c["texto de más"] = x
		// Fondo propio o del diseño (el del patrón va al tema de la presentación).
		tipo, _ := fondoDe(raw)
		if tipo == "" {
			for _, d := range relCorpusPorTipo(srels, "slideLayout") {
				var bg string
				tipo, bg = fondoDe(files[d])
				var patron string
				for _, m := range relCorpusPorTipo(relsCorpus(files, d), "slideMaster") {
					_, patron = fondoDe(files[m])
				}
				if bg == patron {
					tipo = ""
				}
			}
		}
		conFondo := sl != nil && (sl.Bg != "" || len(sl.Boxes) > 0 && sl.Boxes[0].Kind == "image" && sl.Boxes[0].W >= 99 && sl.Boxes[0].H >= 99)
		switch tipo {
		case "color":
			sumar("fondo", 1, map[bool]int{true: 1}[conFondo])
		case "imagen":
			sumar("fondo de imagen", 1, map[bool]int{true: 1}[conFondo])
		}
		formas, resaltado := rellenosDeDiapositiva(raw)
		tablas := celdasConRellenoCorpus(raw, estilosDeTablaCorpus(files["ppt/tableStyles.xml"]))
		var cf, ct, cr int
		if sl != nil {
			for _, b := range sl.Boxes {
				if b.Kind == "shape" && b.Fill != "" && b.Fill != "transparent" {
					cf++
				}
				ct += strings.Count(b.HTML, "background:#")       // relleno de celda
				cr += strings.Count(b.HTML, "background-color:#") // resaltado de un tramo
			}
		}
		sumar("relleno de forma", formas, cf)
		sumar("relleno de tabla", tablas, ct)
		sumar("resaltado", resaltado, cr)
		medirHerenciaCorpus(files, pres, parte, srels, sl, sumar)
		for _, d := range relCorpusPorTipo(srels, "slideLayout") {
			if !disenos[d] {
				disenos[d] = true
				de := formasDeDisenoCorpus(files[d])
				// El de otro patrón lleva además las de su patrón, si no lo tapa: el modelo tiene uno.
				for _, mp := range relCorpusPorTipo(relsCorpus(files, d), "slideMaster") {
					if mp != primerPatron && !bytes.Contains(files[d], []byte(`showMasterSp="0"`)) {
						de += formasDeDisenoCorpus(files[mp])
					}
				}
				sumar("formas del diseño", de, formasDelDisenoEnModelo(pm, sl))
			}
		}
	}
}

// formasDeDisenoCorpus cuenta las formas de un diseño que no son marcadores ni están ocultas:
// formas, imágenes y líneas, también dentro de un grupo.
func formasDeDisenoCorpus(raw []byte) int {
	var n nodoCorpus
	if xml.Unmarshal(raw, &n) != nil {
		return 0
	}
	arbol := n.first("spTree")
	if arbol == nil {
		return 0
	}
	cuantas := 0
	var contar func(g *nodoCorpus)
	contar = func(g *nodoCorpus) {
		for i := range g.Kids {
			k := &g.Kids[i]
			switch k.XMLName.Local {
			case "grpSp":
				contar(k)
			case "sp", "pic", "cxnSp":
				if c := k.first("cNvPr"); k.first("ph") == nil && (c == nil || !activaAttr(c, "hidden")) {
					cuantas++
				}
			}
		}
	}
	contar(arbol)
	return cuantas
}

// formasDelDisenoEnModelo son las formas y las líneas del diseño que lleva la diapositiva.
func formasDelDisenoEnModelo(pm *slidesModel, sl *slideT) int {
	if sl == nil || pm.Master == nil {
		return 0
	}
	for _, l := range pm.Master.Layouts {
		if l.ID == sl.Layout {
			return len(l.Decor) + len(l.Connectors)
		}
	}
	return 0
}

// --- Lo que hereda el texto de los marcadores ---

// nodoCorpus es un elemento XML con su texto, para recorrer una parte entera.
type nodoCorpus struct {
	XMLName xml.Name
	Attrs   []xml.Attr   `xml:",any,attr"`
	Texto   string       `xml:",chardata"`
	Kids    []nodoCorpus `xml:",any"`
}

func (n *nodoCorpus) attr(local string) string {
	for _, a := range n.Attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func (n *nodoCorpus) kid(local string) *nodoCorpus {
	for i := range n.Kids {
		if n.Kids[i].XMLName.Local == local {
			return &n.Kids[i]
		}
	}
	return nil
}

// each recorre los descendientes `local` sin entrar en ellos.
func (n *nodoCorpus) each(local string, fn func(*nodoCorpus)) {
	for i := range n.Kids {
		if n.Kids[i].XMLName.Local == local {
			fn(&n.Kids[i])
			continue
		}
		n.Kids[i].each(local, fn)
	}
}

func (n *nodoCorpus) first(local string) *nodoCorpus {
	var f *nodoCorpus
	n.each(local, func(x *nodoCorpus) {
		if f == nil {
			f = x
		}
	})
	return f
}

// listaCorpus son los niveles (`a:lvl1pPr`…) de una lista de estilos de texto, por nivel.
type listaCorpus map[int]*nodoCorpus

func listaDeCorpus(n *nodoCorpus) listaCorpus {
	out := listaCorpus{}
	if n == nil {
		return out
	}
	for i := range n.Kids {
		var l int
		if _, err := fmt.Sscanf(n.Kids[i].XMLName.Local, "lvl%dpPr", &l); err == nil {
			out[l-1] = &n.Kids[i]
		}
	}
	return out
}

// marcadorCorpus normaliza el tipo de un marcador como lo busca PowerPoint en el diseño y en el
// patrón: sin tipo, obj y subTitle son el cuerpo; ctrTitle, el título.
func marcadorCorpus(t string) string {
	switch t {
	case "", "obj", "subTitle":
		return "body"
	case "ctrTitle":
		return "title"
	}
	return t
}

// listasDeMarcadores son las listas de estilos de los marcadores de una parte, por idx y por tipo.
func listasDeMarcadores(raw []byte) (porIdx, porTipo map[string]listaCorpus) {
	porIdx, porTipo = map[string]listaCorpus{}, map[string]listaCorpus{}
	var root nodoCorpus
	if xml.Unmarshal(raw, &root) != nil {
		return porIdx, porTipo
	}
	root.each("sp", func(sp *nodoCorpus) {
		ph := sp.first("ph")
		tx := sp.kid("txBody")
		if ph == nil || tx == nil {
			return
		}
		l := listaDeCorpus(tx.kid("lstStyle"))
		if i := ph.attr("idx"); i != "" {
			porIdx[i] = l
		}
		if t := marcadorCorpus(ph.attr("type")); len(porTipo[t]) == 0 {
			porTipo[t] = l
		}
	})
	return porIdx, porTipo
}

var (
	reParrafoCorpus = regexp.MustCompile(`<(?:p|h[1-6]|li)\b[^>]*>.*?</(?:p|h[1-6]|li)>`)
	reTamPxCorpus   = regexp.MustCompile(`font-size:([0-9.]+)px`)
)

// medirHerenciaCorpus mide si el texto de los marcadores de una diapositiva llega con el tamaño,
// la alineación y el color que hereda (de la lista de su forma, del marcador del diseño, del
// del patrón, de `p:txStyles` y de `p:defaultTextStyle`) cuando no dice el suyo. Solo cuenta lo
// que se ve distinto de lo que pinta el editor sin más: un tamaño que no es el de un párrafo
// (20 px del escenario de 960), una alineación que no es la izquierda, un color que no es tx1.
//
//nolint:gocognit,gocyclo // el recorrido de la herencia, en un sitio: trocearlo la reparte sin aclararla
func medirHerenciaCorpus(files map[string][]byte, pres, parte string, srels []relCorpus, sl *slideT, sumar func(string, int, int)) {
	var slide, presentacion nodoCorpus
	if xml.Unmarshal(files[parte], &slide) != nil || xml.Unmarshal(files[pres], &presentacion) != nil {
		return
	}
	ancho := 9144000.0
	if sz := presentacion.kid("sldSz"); sz != nil {
		if v, err := strconv.ParseFloat(sz.attr("cx"), 64); err == nil && v > 0 {
			ancho = v
		}
	}
	escala := 960 / (ancho / 12700)
	defecto := listaDeCorpus(presentacion.kid("defaultTextStyle"))
	var dIdx, dTipo, mIdx, mTipo map[string]listaCorpus
	clases := map[string]listaCorpus{}
	colores := map[string]string{} // tx1, accent1, dk2… → «RRGGBB», con el clrMap del patrón
	for _, d := range relCorpusPorTipo(srels, "slideLayout") {
		dIdx, dTipo = listasDeMarcadores(files[d])
		for _, m := range relCorpusPorTipo(relsCorpus(files, d), "slideMaster") {
			mIdx, mTipo = listasDeMarcadores(files[m])
			var master nodoCorpus
			if xml.Unmarshal(files[m], &master) != nil {
				continue
			}
			if tx := master.kid("txStyles"); tx != nil {
				for i := range tx.Kids {
					clases[tx.Kids[i].XMLName.Local] = listaDeCorpus(&tx.Kids[i])
				}
			}
			for _, tema := range relCorpusPorTipo(relsCorpus(files, m), "theme") {
				var th nodoCorpus
				if xml.Unmarshal(files[tema], &th) == nil {
					if cs := th.first("clrScheme"); cs != nil {
						for i := range cs.Kids {
							k := &cs.Kids[i]
							for j := range k.Kids {
								colores[k.XMLName.Local] = strings.ToUpper(cmp.Or(k.Kids[j].attr("lastClr"), k.Kids[j].attr("val")))
							}
						}
					}
				}
			}
			if cm := master.kid("clrMap"); cm != nil {
				for _, a := range cm.Attrs {
					colores[a.Name.Local] = colores[a.Value]
				}
			}
		}
	}
	// tinta dice si un relleno de letra es la tinta del tema, que el editor pinta sin decirla:
	// un color liso, sin variaciones, que da lo mismo que tx1.
	tinta := func(n *nodoCorpus) bool {
		if n.XMLName.Local != "solidFill" || len(n.Kids) != 1 || len(n.Kids[0].Kids) != 0 {
			return false
		}
		c := &n.Kids[0]
		hex := ""
		switch c.XMLName.Local {
		case "srgbClr":
			hex = strings.ToUpper(c.attr("val"))
		case "schemeClr":
			hex = colores[c.attr("val")]
		case "sysClr":
			hex = strings.ToUpper(c.attr("lastClr"))
		}
		return hex != "" && hex == colores["tx1"]
	}
	type parrafoHTML struct{ texto, html string }
	var importados []parrafoHTML
	if sl != nil {
		for _, b := range sl.Boxes {
			for _, h := range reParrafoCorpus.FindAllString(b.HTML, -1) {
				importados = append(importados, parrafoHTML{strings.Join(strings.Fields(stripHTML(h)), " "), h})
			}
		}
	}
	buscar := func(texto string) string {
		for i, p := range importados {
			if p.texto == texto {
				importados = append(importados[:i], importados[i+1:]...)
				return p.html
			}
		}
		return ""
	}
	slide.each("sp", func(sp *nodoCorpus) {
		ph, tx := sp.first("ph"), sp.kid("txBody")
		if ph == nil || tx == nil {
			return
		}
		typ, idx := ph.attr("type"), ph.attr("idx")
		cadena := []listaCorpus{listaDeCorpus(tx.kid("lstStyle"))}
		for _, par := range [][2]map[string]listaCorpus{{dIdx, dTipo}, {mIdx, mTipo}} {
			if l, ok := par[0][idx]; ok && idx != "" {
				cadena = append(cadena, l)
			} else if l, ok := par[1][marcadorCorpus(typ)]; ok {
				cadena = append(cadena, l)
			}
		}
		clase := "bodyStyle"
		switch typ {
		case "title", "ctrTitle":
			clase = "titleStyle"
		case "dt", "ftr", "sldNum", "hdr":
			clase = "otherStyle"
		}
		cadena = append(cadena, clases[clase], defecto)
		encoge := 1.0 // el `fontScale` del ajuste automático: lo que PowerPoint encogió el cuerpo
		if na := tx.first("normAutofit"); na != nil {
			if f, err := strconv.ParseFloat(na.attr("fontScale"), 64); err == nil && f > 0 && f < 100000 {
				encoge = f / 100000
			}
		}
		for i := range tx.Kids {
			p := &tx.Kids[i]
			if p.XMLName.Local != "p" {
				continue
			}
			var texto strings.Builder
			p.each("t", func(t *nodoCorpus) { texto.WriteString(t.Texto) })
			t := strings.Join(strings.Fields(texto.String()), " ")
			if t == "" {
				continue
			}
			rPr := p.first("rPr")
			nivel := 0
			pPr := p.kid("pPr")
			if pPr != nil {
				nivel = atoiDefault(pPr.attr("lvl"), 0)
			}
			var algn, sz string
			var color *nodoCorpus
			for _, l := range cadena {
				n := l[nivel]
				if n == nil {
					continue
				}
				if algn == "" {
					algn = n.attr("algn")
				}
				d := n.kid("defRPr")
				if d == nil {
					continue
				}
				if sz == "" {
					sz = d.attr("sz")
				}
				if color == nil {
					color = rellenoCorpus(d)
				}
			}
			h := buscar(t)
			if (pPr == nil || pPr.attr("algn") == "") && (algn == "ctr" || algn == "r" || algn == "just") {
				quiero := map[string]string{"ctr": "center", "r": "right", "just": "justify"}[algn]
				sumar("marcador: alineación", 1, map[bool]int{true: 1}[strings.Contains(h, "text-align:"+quiero)])
			}
			if v, err := strconv.ParseFloat(sz, 64); err == nil && (rPr == nil || rPr.attr("sz") == "") {
				if px := v / 100 * escala * encoge; math.Abs(px-20) >= 0.5 {
					ok := false
					for _, m := range reTamPxCorpus.FindAllStringSubmatch(h, -1) {
						if x, err := strconv.ParseFloat(m[1], 64); err == nil && math.Abs(x-px) < 0.6 {
							ok = true
						}
					}
					for tag, tam := range map[string]float64{"<h1": 44, "<h2": 30, "<h3": 24} {
						if strings.HasPrefix(h, tag) && math.Abs(tam-px) < 0.6 {
							ok = true
						}
					}
					sumar("marcador: tamaño", 1, map[bool]int{true: 1}[ok])
				}
			}
			if color != nil && (rPr == nil || rellenoCorpus(rPr) == nil) && !tinta(color) {
				sumar("marcador: color", 1, map[bool]int{true: 1}[strings.Contains(h, "color:#")])
			}
		}
	})
}

// rellenoCorpus es el relleno (el color de la letra) de un rPr o un defRPr; nil si no lleva.
func rellenoCorpus(n *nodoCorpus) *nodoCorpus {
	for i := range n.Kids {
		if l := n.Kids[i].XMLName.Local; strings.HasSuffix(l, "Fill") && l != "noFill" {
			return &n.Kids[i]
		}
	}
	return nil
}

// rellenosDeDiapositiva cuenta las formas con relleno (el suyo o el de su estilo) y los tramos
// resaltados; las celdas de tabla las cuenta celdasConRellenoCorpus.
func rellenosDeDiapositiva(raw []byte) (formas, resaltado int) {
	dec := xml.NewDecoder(bytes.NewReader(raw))
	var pila []string
	var spRelleno, spSinRelleno, spRefRelleno bool
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			padre := ""
			if len(pila) > 0 {
				padre = pila[len(pila)-1]
			}
			pila = append(pila, e.Name.Local)
			switch e.Name.Local {
			case "sp":
				spRelleno, spSinRelleno, spRefRelleno = false, false, false
			case "solidFill", "gradFill", "pattFill": // un blipFill de forma llega como imagen
				if padre == "spPr" {
					spRelleno = true
				}
			case "noFill":
				if padre == "spPr" {
					spSinRelleno = true
				}
			case "fillRef":
				spRefRelleno = attrLocal(e, "idx") != "0" && attrLocal(e, "idx") != ""
			case "highlight":
				resaltado++
			}
		case xml.EndElement:
			pila = pila[:max(0, len(pila)-1)]
			if e.Name.Local == "sp" && !spSinRelleno && (spRelleno || spRefRelleno) {
				formas++
			}
		}
	}
	return formas, resaltado
}

// estilosDeTablaCorpus son los estilos de tabla (ppt/tableStyles.xml) por su id: de cada parte
// (wholeTbl, band1H, firstRow…), si rellena su celda. Una parte sin `fill` no dice nada; con
// `a:fill/a:noFill` dice que no («Sin estilo, cuadrícula» solo pone bordes).
func estilosDeTablaCorpus(raw []byte) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	var lista nodoCorpus
	if xml.Unmarshal(raw, &lista) != nil {
		return out
	}
	for i := range lista.Kids {
		e := &lista.Kids[i]
		partes := map[string]bool{}
		for j := range e.Kids {
			tc := e.Kids[j].kid("tcStyle")
			if tc == nil {
				continue
			}
			if f := tc.kid("fill"); f != nil && len(f.Kids) > 0 {
				partes[e.Kids[j].XMLName.Local] = f.Kids[0].XMLName.Local != "noFill"
			} else if tc.kid("fillRef") != nil {
				partes[e.Kids[j].XMLName.Local] = true
			}
		}
		out[e.attr("styleId")] = partes
	}
	return out
}

// celdasConRellenoCorpus cuenta las celdas de las tablas de una diapositiva que se ven rellenas:
// las que dicen su relleno en `a:tcPr` y las que no dicen nada pero las rellena la parte de su
// estilo de tabla que les toca (ECMA-376 §20.1.4.2: de menos a más precedencia, la tabla entera,
// las bandas, las columnas y filas primera y última y las esquinas). Una celda cubierta por una
// combinación (`hMerge`, `vMerge`) no se cuenta: se ve la de su origen, que ocupa su sitio.
func celdasConRellenoCorpus(raw []byte, estilos map[string]map[string]bool) int {
	var sl nodoCorpus
	if xml.Unmarshal(raw, &sl) != nil {
		return 0
	}
	n := 0
	sl.each("tbl", func(tbl *nodoCorpus) {
		pr := tbl.kid("tblPr")
		activa := func(a string) bool { return pr != nil && (pr.attr(a) == "1" || pr.attr(a) == "true") }
		estilo := map[string]bool{}
		if pr != nil && pr.kid("tableStyleId") != nil {
			estilo = estilos[strings.TrimSpace(pr.kid("tableStyleId").Texto)]
		}
		var filas []*nodoCorpus
		for i := range tbl.Kids {
			if tbl.Kids[i].XMLName.Local == "tr" {
				filas = append(filas, &tbl.Kids[i])
			}
		}
		for r, tr := range filas {
			var celdas []*nodoCorpus
			for i := range tr.Kids {
				if tr.Kids[i].XMLName.Local == "tc" {
					celdas = append(celdas, &tr.Kids[i])
				}
			}
			for k, tc := range celdas {
				if activaAttr(tc, "hMerge") || activaAttr(tc, "vMerge") {
					continue
				}
				relleno, dicho := false, false
				if tcPr := tc.kid("tcPr"); tcPr != nil {
					for i := range tcPr.Kids {
						switch tcPr.Kids[i].XMLName.Local {
						case "solidFill", "gradFill", "pattFill", "blipFill", "grpFill":
							relleno, dicho = true, true
						case "noFill":
							relleno, dicho = false, true
						}
					}
				}
				if !dicho {
					banda := func(i int, sinPrimera bool, impar, par string) string {
						if sinPrimera {
							i--
						}
						if i%2 == 0 {
							return impar
						}
						return par
					}
					partes := []string{"wholeTbl"}
					if activa("bandRow") {
						partes = append(partes, banda(r, activa("firstRow"), "band1H", "band2H"))
					}
					if activa("bandCol") {
						partes = append(partes, banda(k, activa("firstCol"), "band1V", "band2V"))
					}
					ultF, ultC := activa("lastRow") && r == len(filas)-1, activa("lastCol") && k == len(celdas)-1
					priF, priC := activa("firstRow") && r == 0, activa("firstCol") && k == 0
					for _, x := range []struct {
						si    bool
						parte string
					}{{ultC, "lastCol"}, {priC, "firstCol"}, {ultF, "lastRow"}, {priF, "firstRow"}, {ultF && ultC, "seCell"}, {ultF && priC, "swCell"}, {priF && ultC, "neCell"}, {priF && priC, "nwCell"}} {
						if x.si {
							partes = append(partes, x.parte)
						}
					}
					for _, p := range partes {
						if v, ok := estilo[p]; ok {
							relleno = v
						}
					}
				}
				if relleno {
					n++
				}
			}
		}
	})
	return n
}

func activaAttr(n *nodoCorpus, a string) bool { return n.attr(a) == "1" || n.attr(a) == "true" }

// --- Censo de elementos ---

var prefijosCorpus = map[string]string{
	"http://schemas.openxmlformats.org/spreadsheetml/2006/main":                 "x",
	"http://purl.oclc.org/ooxml/spreadsheetml/main":                             "x",
	"http://schemas.openxmlformats.org/drawingml/2006/main":                     "a",
	"http://purl.oclc.org/ooxml/drawingml/main":                                 "a",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                "p",
	"http://purl.oclc.org/ooxml/presentationml/main":                            "p",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                    "c",
	"http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing":       "xdr",
	"http://schemas.openxmlformats.org/drawingml/2006/diagram":                  "dgm",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":               "mc",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":       "r",
	"http://schemas.microsoft.com/office/spreadsheetml/2009/9/main":             "x14",
	"http://schemas.microsoft.com/office/excel/2006/main":                       "xm",
	"http://schemas.microsoft.com/office/powerpoint/2010/main":                  "p14",
	"http://schemas.microsoft.com/office/drawing/2010/main":                     "a14",
	"http://schemas.microsoft.com/office/spreadsheetml/2010/11/main":            "x15",
	"http://schemas.microsoft.com/office/powerpoint/2012/main":                  "p15",
	"http://schemas.microsoft.com/office/spreadsheetml/2014/revision":           "xr",
	"http://schemas.microsoft.com/office/spreadsheetml/2017/richdata":           "xlrd",
	"http://schemas.openxmlformats.org/officeDocument/2006/sharedTypes":         "s",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                  "pic",
	"http://schemas.openxmlformats.org/drawingml/2006/chartDrawing":             "cdr",
	"http://schemas.microsoft.com/office/drawing/2014/main":                     "a16",
	"http://schemas.microsoft.com/office/powerpoint/2015/main":                  "p16",
	"http://schemas.microsoft.com/office/drawing/2012/chart":                    "c15",
	"http://schemas.microsoft.com/office/drawing/2007/8/2/chart":                "c14",
	"http://schemas.microsoft.com/office/spreadsheetml/2009/9/ac":               "x14ac",
	"http://schemas.microsoft.com/office/thememl/2012/main":                     "thm15",
	"http://schemas.microsoft.com/office/drawing/2008/diagram":                  "dsp",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties": "ep",
}

// censoSinInteres son los elementos que no se ven en el editor ni guardan contenido: la
// estructura del paquete, la impresión, las vistas, las cachés y los identificadores.
var censoSinInteres = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range strings.Fields(`worksheet sheetViews sheetView sheetFormatPr pageMargins pageSetup
		printOptions headerFooter selection phoneticPr workbook styleSheet cellStyleXfs cellStyles cellStyle
		workbookPr calcPr bookViews workbookView fileVersion tableStyles diagonal scheme extLst ext absPath
		wsDr clientData cNvGraphicFramePr spTree nvGrpSpPr cNvGrpSpPr sldLayout sldMaster sld spLocks
		creationId masterClrMapping avLst sldLayoutId printSettings ptCount numCache strCache crossAx
		crosses crossBetween tickLblPos lblOffset lblAlgn axId slicerStyles timelineStyles revisionPtr
		dimension family charset docPr cNvPr nvPr cNvSpPr cNvPicPr picLocks graphicFrameLocks
		evenFooter oddFooter oddHeader evenHeader rowBreaks colBreaks brk legacyDrawing webPublishing
		fileRecoveryPr alternateContent AlternateContent Choice Fallback bookView autoTitleDeleted
		roundedCorners date1904 lang txPr formatCode`) {
		m[k] = true
	}
	return m
}()

// tipoDeParte clasifica una parte del paquete para el censo; "" si no se censa.
func tipoDeParte(formato, parte string) string {
	tipos := map[string][][2]string{
		"xlsx": {
			{"xl/worksheets/", "hoja"},
			{"xl/styles.xml", "estilos"},
			{"xl/workbook.xml", "libro"},
			{"xl/drawings/", "dibujo"},
			{"xl/charts/", "grafico"},
			{"xl/tables/", "tabla"},
			{"xl/pivot", "dinamica"},
			{"xl/comments", "comentarios"},
			{"xl/threadedComments", "comentarios"},
		},
		"pptx": {
			{"ppt/slides/", "diapositiva"},
			{"ppt/slideLayouts/", "diseño"},
			{"ppt/slideMasters/", "patrón"},
			{"ppt/charts/", "grafico"},
			{"ppt/diagrams/", "smartart"},
			{"ppt/presentation.xml", "presentación"},
			{"ppt/tableStyles.xml", "estilos de tabla"},
		},
	}
	if strings.Contains(parte, "/_rels/") {
		return ""
	}
	for _, t := range tipos[formato] {
		if strings.HasPrefix(parte, t[0]) {
			return t[1]
		}
	}
	return ""
}

// censarParte apunta, por elemento, en cuántas partes de ese tipo aparece.
func censarParte(censo map[string]int, tipo string, raw []byte) {
	visto := map[string]bool{}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if e, ok := tok.(xml.StartElement); ok {
			pre := prefijosCorpus[e.Name.Space]
			if pre == "" {
				pre = path.Base(e.Name.Space)
			}
			visto[tipo+" "+pre+":"+e.Name.Local] = true
		}
	}
	for k := range visto {
		censo[k]++
	}
}

// vocabularioImportador son los nombres que el importador cita (literales y etiquetas xml).
func vocabularioImportador(t *testing.T) map[string]bool {
	fuentes, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`"([^"\n]*)"`)
	v := map[string]bool{}
	for _, f := range fuentes {
		b := filepath.Base(f)
		del := slices.ContainsFunc([]string{"import", "pptx", "sheet", "slides", "proteccion", "comments"}, func(p string) bool {
			return strings.HasPrefix(b, p)
		})
		if !del || strings.HasSuffix(b, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			for _, w := range strings.FieldsFunc(m[1], func(r rune) bool { return r == '>' || r == ',' || r == ' ' || r == ':' }) {
				v[w] = true
			}
		}
	}
	return v
}

// --- Resumen ---

func resumenCorpus(ms []medidaCorpus, censo map[string]map[string]int, vocab map[string]bool) string {
	var b strings.Builder
	for _, f := range []string{"xlsx", "pptx"} {
		estados := map[string]int{}
		tot := map[string]*cuenta{}
		porPrograma := map[string]map[string]int{}
		cuentasPrograma := map[string]map[string]cuenta{}
		n := 0
		for _, m := range ms {
			if m.Formato != f {
				continue
			}
			n++
			estados[m.Estado]++
			if porPrograma[m.Programa] == nil {
				porPrograma[m.Programa] = map[string]int{}
			}
			porPrograma[m.Programa][m.Estado]++
			if cuentasPrograma[m.Programa] == nil {
				cuentasPrograma[m.Programa] = map[string]cuenta{}
			}
			for k, c := range m.Cuentas {
				x := cuentasPrograma[m.Programa][k]
				x.sumar(c)
				cuentasPrograma[m.Programa][k] = x
				if tot[k] == nil {
					tot[k] = &cuenta{}
				}
				tot[k].sumar(c)
			}
		}
		fmt.Fprintf(&b, "### %s: %d ficheros\n\n", f, n)
		fmt.Fprintf(&b, "| Se abren | Error | Pánico | Más de %s | Más de %d GiB | Caída |\n|---|---|---|---|---|---|\n| %d | %d | %d | %d | %d | %d |\n\n",
			corpusTiempoMax, corpusMemoriaMax>>30, estados["ok"], estados["error"], estados["panico"], estados["tiempo"], estados["memoria"], estados["caida"])
		b.WriteString("| Qué | En el paquete | Llega al editor | % |\n|---|---|---|---|\n")
		for _, k := range columnasCorpus[f] {
			c := cuenta{}
			if tot[k] != nil {
				c = *tot[k]
			}
			if k == "texto de más" {
				fmt.Fprintf(&b, "| %s (caracteres que no estaban) | — | %d | — |\n", k, c.De)
				continue
			}
			fmt.Fprintf(&b, "| %s | %d | %d | %s |\n", k, c.De, c.Ok, c.pct())
		}
		// Por programa: lo que llega al editor de cada cosa, «ok/de».
		cols := columnasCorpus[f]
		b.WriteString("\n| Programa | Ficheros | No se abren | " + strings.Join(cols, " | ") + " |\n|---|---|---|" + strings.Repeat("---|", len(cols)) + "\n")
		progs := slices.Collect(maps.Keys(porPrograma))
		slices.SortFunc(progs, func(a, c string) int { return sumaEstados(porPrograma[c]) - sumaEstados(porPrograma[a]) })
		for _, p := range progs {
			e := porPrograma[p]
			fmt.Fprintf(&b, "| %s | %d | %d |", p, sumaEstados(e), sumaEstados(e)-e["ok"])
			for _, k := range cols {
				c := cuentasPrograma[p][k]
				if k == "texto de más" {
					fmt.Fprintf(&b, " %d |", c.De)
				} else {
					fmt.Fprintf(&b, " %d/%d |", c.Ok, c.De)
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\nNo se abren:\n\n")
		motivos := map[string]int{}
		for _, m := range ms {
			if m.Formato == f && m.Estado != "ok" {
				motivos[m.Estado+": "+recortar(m.Motivo, 90)]++
			}
		}
		for _, k := range ordenarPorCuenta(motivos, 25) {
			fmt.Fprintf(&b, "- %d × %s\n", motivos[k], k)
		}
		b.WriteString("\nElementos que el importador no nombra (en cuántas partes aparecen):\n\n")
		sin := map[string]int{}
		for k, v := range censo[f] {
			local := k[strings.LastIndexByte(k, ':')+1:]
			if !vocab[local] && !censoSinInteres[local] {
				sin[k] = v
			}
		}
		for _, k := range ordenarPorCuenta(sin, 50) {
			fmt.Fprintf(&b, "- %d × %s\n", sin[k], k)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func sumaEstados(m map[string]int) int {
	n := 0
	for _, v := range m {
		n += v
	}
	return n
}

func ordenarPorCuenta(m map[string]int, n int) []string {
	ks := slices.Collect(maps.Keys(m))
	slices.SortFunc(ks, func(a, b string) int {
		if m[a] != m[b] {
			return m[b] - m[a]
		}
		return strings.Compare(a, b)
	})
	return ks[:min(n, len(ks))]
}

// hojaConPestana dice si la hoja importada lleva color de pestaña (por el JSON: así la medida
// vale antes y después de que el modelo lo tenga).
func hojaConPestana(h *sheetData) bool {
	b, _ := json.Marshal(h)
	return bytes.Contains(b, []byte(`"tabColor":"#`))
}

// limitarMemoria pone un tope al espacio de direcciones del proceso: pasado, el runtime de Go
// muere con «out of memory» en vez de que el núcleo mate el contenedor entero.
func limitarMemoria(tope uint64) {
	_ = syscall.Setrlimit(syscall.RLIMIT_AS, &syscall.Rlimit{Cur: tope, Max: tope})
}

// memoriaMaxMB es lo más que ocupó en memoria un proceso hijo (Maxrss viene en KiB en Linux).
func memoriaMaxMB(ps *os.ProcessState) int {
	if ru, ok := ps.SysUsage().(*syscall.Rusage); ok {
		return int(ru.Maxrss / 1024)
	}
	return 0
}
