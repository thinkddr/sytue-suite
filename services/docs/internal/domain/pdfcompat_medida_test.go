// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Sonda de COMPATIBILIDAD de CertiPDF contra un corpus de PDF ajenos. No es una
// comprobación: es la mitad de la herramienta con la que se mide qué abre, qué
// pinta y qué estropea CertiPDF con documentos reales (la otra mitad, que
// compara con lectores de referencia usados como caja negra, es
// `scripts/certipdf-compat/medir.py`). Sin `SYTUE_COMPAT_PDF` no hace nada.
//
// Trata UN fichero por proceso, a propósito: un pánico en una gorrutina o un
// bucle sin fin se llevan el proceso entero, y el guion que la llama lo cuenta
// como lo que es en vez de perder la medida del resto del corpus. Por la misma
// razón escribe `resultado.json` después de CADA operación, con la que está en
// curso: si el proceso muere, se sabe en cuál.
//
//	SYTUE_COMPAT_PDF=a.pdf SYTUE_COMPAT_OUT=/dir [SYTUE_COMPAT_CLAVE=…]
//	SYTUE_COMPAT_CERT=cadena.pem SYTUE_COMPAT_KEY=clave.pem [SYTUE_COMPAT_PAGINAS=3]
//	  ./domain.test -test.run '^TestMedirCompatibilidad$'
package domain

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// opCompat es lo medido de una operación.
type opCompat struct {
	OK      bool    `json:"ok"`
	Error   string  `json:"error,omitempty"`
	Panico  string  `json:"panico,omitempty"`
	Ms      float64 `json:"ms"`
	AsigMiB float64 `json:"asig_mib"`
	// RSSMiB es el pico de memoria residente del proceso al acabar la operación.
	RSSMiB float64        `json:"rss_mib"`
	Datos  map[string]any `json:"datos,omitempty"`
}

type resultadoCompat struct {
	Fichero   string               `json:"fichero"`
	EnCurso   string               `json:"en_curso,omitempty"`
	Ops       map[string]*opCompat `json:"ops"`
	Paginas   int                  `json:"paginas"`
	Salidas   map[string]string    `json:"salidas,omitempty"`
	Terminado bool                 `json:"terminado"`
}

func TestMedirCompatibilidad(t *testing.T) {
	ruta := os.Getenv("SYTUE_COMPAT_PDF")
	if ruta == "" {
		t.Skip("sin SYTUE_COMPAT_PDF: no se mide nada")
	}
	out := os.Getenv("SYTUE_COMPAT_OUT")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	maxPag, _ := strconv.Atoi(os.Getenv("SYTUE_COMPAT_PAGINAS"))
	if maxPag == 0 {
		maxPag = 3
	}
	escala, _ := strconv.ParseFloat(os.Getenv("SYTUE_COMPAT_ESCALA"), 64)
	if escala == 0 {
		escala = 1.5
	}
	res := &resultadoCompat{Fichero: ruta, Ops: map[string]*opCompat{}, Salidas: map[string]string{}}
	guardar := func() {
		b, _ := json.MarshalIndent(res, "", " ")
		tmp := filepath.Join(out, "resultado.json.tmp")
		_ = os.WriteFile(tmp, b, 0o644)
		_ = os.Rename(tmp, filepath.Join(out, "resultado.json"))
	}
	tope, _ := strconv.ParseUint(os.Getenv("SYTUE_COMPAT_MAX_MIB"), 10, 64)
	if tope == 0 {
		tope = 4096
	}
	vigilarMemoria(tope<<20, func() {
		res.Ops[res.EnCurso] = &opCompat{Error: fmt.Sprintf("memoria: más de %d MiB", tope)}
		guardar()
	})

	// medir ejecuta f como la operación `nombre`, con su pánico recogido.
	medir := func(nombre string, f func(o *opCompat) error) *opCompat {
		res.EnCurso = nombre
		guardar()
		o := &opCompat{Datos: map[string]any{}}
		runtime.GC()
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		t0 := time.Now()
		func() {
			defer func() {
				if p := recover(); p != nil {
					o.Panico = fmt.Sprintf("%v | %s", p, pilaCorta(debug.Stack()))
				}
			}()
			if err := f(o); err != nil {
				o.Error = err.Error()
			}
		}()
		o.Ms = float64(time.Since(t0).Microseconds()) / 1000
		runtime.ReadMemStats(&b)
		o.AsigMiB = mib(b.TotalAlloc - a.TotalAlloc)
		o.RSSMiB = rssPico()
		o.OK = o.Error == "" && o.Panico == ""
		res.Ops[nombre] = o
		res.EnCurso = ""
		guardar()
		return o
	}
	escribir := func(nombre string, datos []byte) {
		p := filepath.Join(out, nombre)
		if os.WriteFile(p, datos, 0o644) == nil {
			res.Salidas[strings.TrimSuffix(strings.TrimPrefix(nombre, "salida-"), ".pdf")] = p
		}
	}

	datos, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	var doc *PDFDoc
	medir("abrir", func(o *opCompat) error {
		d, err := LeerPDFConClave(datos, os.Getenv("SYTUE_COMPAT_CLAVE"))
		if err != nil {
			return err
		}
		doc = d
		o.Datos["paginas"] = d.NumPaginas()
		o.Datos["cifrado"] = d.EstaCifrado()
		o.Datos["reparado"] = d.EstabaDanado()
		return nil
	})
	if doc == nil {
		res.Terminado = true
		guardar()
		return
	}
	res.Paginas = doc.NumPaginas()
	// conPermiso repite una operación que se negó por invalidar firmas, como hace
	// la interfaz cuando el usuario lo confirma, para medir también esa salida.
	conPermiso := func(o *opCompat, f func(d *PDFDoc) ([]byte, error)) ([]byte, error) {
		salida, err := f(doc)
		if !errors.Is(err, ErrInvalidaFirmas) {
			return salida, err
		}
		o.Datos["pide_confirmacion"] = err.Error()
		d2, err := LeerPDFConClave(datos, os.Getenv("SYTUE_COMPAT_CLAVE"))
		if err != nil {
			return nil, err
		}
		d2.PermitirInvalidarFirmas()
		return f(d2)
	}
	n := min(maxPag, doc.NumPaginas())

	// Lo que el documento DICE que tiene, para compararlo con un lector ajeno.
	medir("inventario", func(o *opCompat) error {
		o.Datos["firmas"] = len(doc.FirmasDelDocumento())
		o.Datos["campos"] = len(doc.CamposDelFormulario())
		o.Datos["anotaciones"] = len(doc.AnotacionesDelDocumento())
		o.Datos["marcadores"] = len(doc.IndiceDelDocumento())
		o.Datos["capas"] = len(doc.CapasDelDocumento())
		o.Datos["adjuntos"] = len(doc.AdjuntosDelDocumento())
		o.Datos["enlaces"] = len(doc.EnlacesDelDocumento())
		return nil
	})

	for i := 0; i < n; i++ {
		medir(fmt.Sprintf("pintar-%d", i+1), func(o *opCompat) error {
			p, err := doc.Pagina(i)
			if err != nil {
				return err
			}
			o.Datos["tamano"] = [2]float64{p.Ancho, p.Alto}
			o.Datos["rotacion"] = p.Rotacion
			png, err := doc.RenderizarPaginaPNG(i, OpcionesRender{Escala: escala})
			if err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(out, fmt.Sprintf("nuestro-%d.png", i+1)), png, 0o644)
		})
		medir(fmt.Sprintf("svg-%d", i+1), func(o *opCompat) error {
			svg, err := doc.RenderizarPaginaSVG(i, OpcionesRender{Escala: escala})
			o.Datos["kib"] = len(svg) / 1024
			if err == nil && os.Getenv("SYTUE_COMPAT_SVG") != "" {
				err = os.WriteFile(filepath.Join(out, fmt.Sprintf("nuestro-%d.svg", i+1)), []byte(svg), 0o644)
			}
			return err
		})
	}

	// Texto: los fragmentos tal cual los recibe el buscador de la web, que busca
	// DENTRO de cada fragmento (Paneles.tsx), y el texto plano por página.
	medir("texto", func(o *opCompat) error {
		type pag struct {
			Plano      string   `json:"plano"`
			Fragmentos []string `json:"fragmentos"`
			Error      string   `json:"error,omitempty"`
		}
		var paginas []pag
		for i := 0; i < min(20, doc.NumPaginas()); i++ {
			var p pag
			frags, err := doc.TextoDePagina(i)
			if err != nil {
				p.Error = err.Error()
			}
			for _, f := range frags {
				if strings.TrimSpace(f.Texto) != "" {
					p.Fragmentos = append(p.Fragmentos, f.Texto)
				}
			}
			p.Plano, _ = doc.TextoPlano(i)
			paginas = append(paginas, p)
		}
		b, _ := json.Marshal(paginas)
		return os.WriteFile(filepath.Join(out, "texto.json"), b, 0o644)
	})

	medir("formulario", func(o *opCompat) error {
		campos := doc.CamposDelFormulario()
		o.Datos["campos"] = len(campos)
		valores := valoresDePrueba(campos)
		o.Datos["rellenados"] = len(valores)
		if len(valores) == 0 {
			return nil
		}
		b, _ := json.Marshal(valores)
		_ = os.WriteFile(filepath.Join(out, "valores.json"), b, 0o644)
		salida, err := conPermiso(o, func(d *PDFDoc) ([]byte, error) { return RellenarFormularioPDF(d, valores) })
		if err != nil {
			return err
		}
		escribir("salida-formulario.pdf", salida)
		// Ida y vuelta con el propio lector: lo escrito se tiene que volver a leer.
		d2, err := LeerPDF(salida)
		if err != nil {
			return fmt.Errorf("la salida no se vuelve a abrir: %w", err)
		}
		iguales := 0
		for _, c := range d2.CamposDelFormulario() {
			if v, ok := valores[c.Nombre]; ok && (c.Valor == v || (c.Tipo == CampoCasilla && c.Valor != "" && c.Valor != "Off")) {
				iguales++
			}
		}
		o.Datos["releidos"] = iguales
		return nil
	})

	p0, _ := doc.Pagina(0)
	w, h := p0.Ancho, p0.Alto
	medir("anotar", func(o *opCompat) error {
		anots := []Anotacion{
			{Pagina: 0, Tipo: AnotResaltado, Cuadros: [][4]float64{{w * .1, h * .8, w * .5, h * .83}}, Color: [3]float64{1, 1, 0}, Opacidad: .5},
			{Pagina: 0, Tipo: AnotNota, Rect: [4]float64{w * .8, h * .9, w*.8 + 20, h*.9 + 20}, Texto: "Nota de prueba ñ", Autor: "Sytue", Color: [3]float64{1, .8, 0}, Opacidad: 1},
			{Pagina: 0, Tipo: AnotRectangulo, Rect: [4]float64{w * .1, h * .1, w * .3, h * .2}, Color: [3]float64{1, 0, 0}, Opacidad: 1, Grosor: 2},
			{Pagina: 0, Tipo: AnotTinta, Trazos: [][][2]float64{{{w * .6, h * .1}, {w * .7, h * .15}, {w * .8, h * .1}}}, Color: [3]float64{0, 0, 1}, Opacidad: 1, Grosor: 2},
			{Pagina: 0, Tipo: AnotTextoLibre, Rect: [4]float64{w * .4, h * .3, w * .7, h * .35}, Texto: "Texto libre áé", Color: [3]float64{0, 0, 0}, Opacidad: 1, Tamano: 12},
		}
		salida, err := conPermiso(o, func(d *PDFDoc) ([]byte, error) { return AnadirAnotacionesPDF(d, anots) })
		if err != nil {
			return err
		}
		escribir("salida-anotado.pdf", salida)
		d2, err := LeerPDF(salida)
		if err != nil {
			return fmt.Errorf("la salida no se vuelve a abrir: %w", err)
		}
		o.Datos["antes"] = len(doc.AnotacionesDelDocumento())
		o.Datos["despues"] = len(d2.AnotacionesDelDocumento())
		return nil
	})

	// Censurar la franja central de la primera página, de lado a lado.
	medir("censurar", func(o *opCompat) error {
		o.Datos["franja"] = [2]float64{h * .4, h * .6}
		salida, err := RedactarPDF(doc, []AreaRedaccion{{Pagina: 0, X1: 0, Y1: h * .4, X2: w, Y2: h * .6}})
		if errors.Is(err, ErrRedaccionNoGarantizada) {
			o.Datos["no_garantizada"] = true
		}
		if err != nil {
			return err
		}
		escribir("salida-censurado.pdf", salida)
		return nil
	})

	medir("editar", func(o *opCompat) error {
		an, err := doc.AnalizarEdicion(0)
		if err != nil {
			return err
		}
		o.Datos["fragmentos"] = len(an.Fragmentos)
		for _, f := range an.Fragmentos {
			r := []rune(strings.TrimSpace(f.Texto))
			// Un trozo con letras sin identificar (la fuente no dice qué carácter es
			// un código y sale U+200B o U+FFFD) no se puede reescribir con sentido.
			if f.LetrasDesconocidas || len(r) < 4 || strings.ContainsAny(string(r), "\u200b\ufffd") {
				continue
			}
			// El mismo texto al revés: mismas letras, así que el subconjunto de la
			// fuente incrustada las tiene todas.
			for a, b := 0, len(r)-1; a < b; a, b = a+1, b-1 {
				r[a], r[b] = r[b], r[a]
			}
			o.Datos["original"] = f.Texto
			o.Datos["nuevo"] = string(r)
			salida, err := EditarTextoPDF(doc, []CambioDeTexto{{Pagina: 0, ID: f.ID, Nuevo: string(r)}})
			if err != nil {
				return err
			}
			escribir("salida-editado.pdf", salida)
			return nil
		}
		return nil
	})

	medir("paginas", func(o *opCompat) error {
		salida, err := RotarPaginasPDF(doc, map[int]int{0: 90})
		if err != nil {
			return err
		}
		escribir("salida-rotado.pdf", salida)
		return nil
	})

	if cert := os.Getenv("SYTUE_COMPAT_CERT"); cert != "" {
		medir("firmar", func(o *opCompat) error {
			c, err := os.ReadFile(cert)
			if err != nil {
				return err
			}
			k, err := os.ReadFile(os.Getenv("SYTUE_COMPAT_KEY"))
			if err != nil {
				return err
			}
			opts := OpcionesFirma{CertificadoPEM: c, ClavePEM: k, Nombre: "Prueba Sytue", Motivo: "Compatibilidad", Fecha: time.Now()}
			// Lo mismo que hace el servicio (pdfstudio.go): anexar y, si el
			// documento no lo admite, reescribir.
			salida, err := FirmarIncrementalPDF(doc, opts)
			o.Datos["incremental"] = err == nil
			if errors.Is(err, ErrNoIncremental) {
				o.Datos["motivo_reescrito"] = err.Error()
				salida, err = conPermiso(o, func(d *PDFDoc) ([]byte, error) { return FirmarConCertificadoPDF(d, opts) })
			}
			if err != nil {
				return err
			}
			escribir("salida-firmado.pdf", salida)
			d2, err := LeerPDF(salida)
			if err != nil {
				return fmt.Errorf("la salida no se vuelve a abrir: %w", err)
			}
			vs := d2.VerificarFirmas(OpcionesVerificacion{Anclas: raizDeCadena(c)})
			if len(vs) > 0 {
				ult := vs[len(vs)-1]
				o.Datos["nuestra_veredicto"] = ult.Veredicto
				o.Datos["nuestra_integra"] = ult.Integra
				o.Datos["nuestra_motivos"] = ult.Motivos
			}
			o.Datos["firmas_salida"] = len(vs)
			return nil
		})
	}

	if len(doc.FirmasDelDocumento()) > 0 {
		medir("verificar", func(o *opCompat) error {
			verificadas := doc.VerificarFirmas(OpcionesVerificacion{})
			vs := make([]map[string]any, 0, len(verificadas))
			for _, v := range verificadas {
				vs = append(vs, map[string]any{
					"campo": v.Campo, "tipo": v.Tipo, "veredicto": v.Veredicto, "integra": v.Integra,
					"motivos": v.Motivos, "cubre_todo": v.CubreTodo, "confianza": v.Confianza,
				})
			}
			o.Datos["firmas"] = vs
			return nil
		})
	}
	res.Terminado = true
	guardar()
}

// valoresDePrueba elige un valor para cada campo que se puede rellenar.
func valoresDePrueba(campos []CampoFormulario) map[string]string {
	v := map[string]string{}
	for _, c := range campos {
		if c.SoloLectura || c.Nombre == "" {
			continue
		}
		switch c.Tipo {
		case CampoTexto:
			s := "Sytue Ñandú 123"
			if c.MaxLongitud > 0 && c.MaxLongitud < len([]rune(s)) {
				s = string([]rune(s)[:c.MaxLongitud])
			}
			v[c.Nombre] = s
		case CampoCasilla:
			v[c.Nombre] = "sí"
		case CampoOpcion, CampoLista, CampoDesplegable:
			if len(c.Opciones) > 0 {
				v[c.Nombre] = c.Opciones[len(c.Opciones)-1]
			} else if c.EstadoActivado != "" {
				v[c.Nombre] = c.EstadoActivado
			}
		case CampoBoton, CampoFirma:
			// Ni un botón ni una firma se rellenan con un valor.
		}
	}
	return v
}

// raizDeCadena confía en el ÚLTIMO certificado de la cadena, que es la raíz.
func raizDeCadena(cadena []byte) *x509.CertPool {
	var ultimo *pem.Block
	for b, resto := pem.Decode(cadena); b != nil; b, resto = pem.Decode(resto) {
		ultimo = b
	}
	pool := x509.NewCertPool()
	if ultimo != nil {
		if c, err := x509.ParseCertificate(ultimo.Bytes); err == nil {
			pool.AddCert(c)
		}
	}
	return pool
}

// pilaCorta se queda con las primeras líneas de la pila que son del paquete.
func pilaCorta(pila []byte) string {
	var out []string
	for _, l := range strings.Split(string(pila), "\n") {
		if strings.Contains(l, "/domain/") && strings.Contains(l, ".go:") && !strings.Contains(l, "_test.go") {
			out = append(out, strings.TrimSpace(l[strings.LastIndex(l, "/")+1:]))
			if len(out) == 4 {
				break
			}
		}
	}
	return strings.Join(out, " < ")
}

func rssPico() float64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return float64(ru.Maxrss) / 1024 // Linux: KiB
}

// vigilarMemoria corta el proceso si el montón pasa del tope: el frontal es
// compartido y un fichero no puede llevarse la memoria de los demás.
func vigilarMemoria(tope uint64, alPasarse func()) {
	go func() {
		m := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		for range time.Tick(100 * time.Millisecond) {
			metrics.Read(m)
			if m[0].Value.Uint64() > tope {
				alPasarse()
				os.Exit(3) //nolint:revive // morir es el objetivo: el guion lee el código 3 como «se pasó de memoria»
			}
		}
	}()
}
