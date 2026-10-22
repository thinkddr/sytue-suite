// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build linux && !android

package soltar_test

import (
	"bufio"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// evento es una línea de testdata/ventana.
type evento struct {
	Tipo     string   `json:"tipo"`
	Texto    string   `json:"texto,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Ficheros []string `json:"ficheros,omitempty"`
	X        float32  `json:"x"`
	Y        float32  `json:"y"`
	Ventana  uintptr  `json:"ventana,omitempty"`
	Escala   float32  `json:"escala,omitempty"`
}

// TestX11AceptaTextoYEnlaces suelta, con un origen XDnD de verdad (testdata/origen.c),
// texto, un enlace y un fichero sobre una ventana de Fyne dentro de Xvfb, y mira lo que
// le llega, avisos de `AlArrastrar` incluidos. Es la prueba del parche de GLFW: sin él, el
// texto se rechaza en el XdndStatus, el enlace llega a `SetOnDropped` convertido en un
// fichero que no existe, y no hay ningún aviso de arrastre.
//
// Necesita Xvfb y las cabeceras de X11; sin ellos se salta.
func TestX11AceptaTextoYEnlaces(t *testing.T) {
	for _, h := range []string{"Xvfb", "go", cmp.Or(os.Getenv("CC"), "cc")} {
		if _, err := exec.LookPath(h); err != nil {
			t.Skipf("sin %s: esta prueba necesita un servidor X virtual y un compilador", h)
		}
	}
	dir := t.TempDir()
	construir(t, "go", "build", "-o", filepath.Join(dir, "ventana"), "./testdata/ventana")
	construir(t, cmp.Or(os.Getenv("CC"), "cc"), "-o", filepath.Join(dir, "origen"), "testdata/origen.c", "-lX11")

	pantalla := xvfb(t)
	env := append(os.Environ(), "DISPLAY="+pantalla, "FYNE_SCALE=1")

	ventana := exec.Command(filepath.Join(dir, "ventana"))
	ventana.Env = env
	ventana.Stderr = os.Stderr
	salida, err := ventana.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := ventana.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ventana.Process.Kill(); _ = ventana.Wait() })
	eventos := make(chan evento, 8)
	go func() {
		s := bufio.NewScanner(salida)
		for s.Scan() {
			var e evento
			if json.Unmarshal(s.Bytes(), &e) == nil {
				eventos <- e
			}
		}
		close(eventos)
	}()
	lista := esperar(t, eventos, 30*time.Second)
	if lista.Tipo != "lista" || lista.Ventana == 0 || lista.Escala == 0 {
		t.Fatalf("la ventana no dijo su identificador ni su escala: %+v", lista)
	}
	xid, escala := strconv.FormatUint(uint64(lista.Ventana), 10), lista.Escala

	const parrafo = "Hola, cuaderno\nsegunda línea con acentos: ñ, ü"
	entra, sale := evento{Tipo: "entra"}, evento{Tipo: "sale"}
	casos := []struct {
		nombre string
		salir  bool     // el origen se va sin soltar
		ofrece []string // tipo, datos, tipo, datos…
		quiere []evento // en orden
	}{
		{
			// Lo que ofrece un navegador al arrastrar una selección de texto: cuatro
			// tipos, así que van en la propiedad XdndTypeList y no en el XdndEnter. No es
			// un arrastre de ficheros: `AlArrastrar` no se entera.
			nombre: "texto",
			ofrece: []string{
				"text/html", "<b>Hola</b>", "text/plain;charset=utf-8", parrafo,
				"UTF8_STRING", parrafo, "text/plain", parrafo,
			},
			quiere: []evento{{Tipo: "texto", Texto: parrafo}},
		},
		{
			// Un terminal: solo el tipo del ICCCM.
			nombre: "UTF8_STRING",
			ofrece: []string{"UTF8_STRING", "solo UTF8_STRING"},
			quiere: []evento{{Tipo: "texto", Texto: "solo UTF8_STRING"}},
		},
		{
			// Un enlace arrastrado desde el navegador: la lista de URI manda sobre el
			// texto. En X11 una lista de URI se anuncia como arrastre de ficheros antes
			// de saber qué trae: es lo que hay en el protocolo.
			nombre: "enlace",
			ofrece: []string{
				"text/uri-list", "# comentario\r\nhttps://sytue.eu/notes?x=1\r\n",
				"text/plain", "Sytue Notepads",
			},
			quiere: []evento{entra, sale, {Tipo: "texto", URLs: []string{"https://sytue.eu/notes?x=1"}}},
		},
		{
			// Un fichero sigue llegando por donde llegaba, `SetOnDropped`, y el aviso de
			// arrastre se apaga ANTES de que llegue.
			nombre: "fichero",
			ofrece: []string{"text/uri-list", "file:///tmp/acta%20final.docx\r\n"},
			quiere: []evento{entra, sale, {Tipo: "ficheros", Ficheros: []string{"file:///tmp/acta%20final.docx"}}},
		},
		{
			// Y un fichero que pasa por encima y se va (XdndLeave) enciende y apaga el
			// aviso sin entregar nada.
			nombre: "fichero que se va",
			salir:  true,
			ofrece: []string{"text/uri-list", "file:///tmp/acta.docx\r\n"},
			quiere: []evento{entra, sale},
		},
	}
	for i, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			x, y := 40+30*i, 50+20*i
			args := append([]string{xid, strconv.Itoa(x), strconv.Itoa(y)}, c.ofrece...)
			if c.salir {
				args = append([]string{"--salir"}, args...)
			}
			origen := exec.Command(filepath.Join(dir, "origen"), args...)
			origen.Env = env
			if out, err := origen.CombinedOutput(); err != nil {
				t.Fatalf("el origen XDnD terminó con %v: %s", err, out)
			}
			for _, quiere := range c.quiere {
				visto := esperar(t, eventos, 5*time.Second)
				if visto.Tipo == "ficheros" {
					// La posición de un fichero la pone Fyne con el último movimiento del
					// puntero que ya procesó su bucle; no es cosa del parche.
					visto.X, visto.Y = 0, 0
				}
				if quiere.Tipo == "texto" {
					quiere.X, quiere.Y = float32(x)/escala, float32(y)/escala
				}
				if !reflect.DeepEqual(visto, quiere) {
					t.Fatalf("recibido %+v\n        quería %+v", visto, quiere)
				}
			}
		})
	}
	// Y nada más: un aviso de arrastre de sobra también es un fallo.
	select {
	case e := <-eventos:
		t.Errorf("ha llegado de más %+v", e)
	case <-time.After(500 * time.Millisecond):
	}
}

func construir(t *testing.T, orden string, args ...string) {
	t.Helper()
	if out, err := exec.Command(orden, args...).CombinedOutput(); err != nil {
		t.Fatalf("%s %s: %v\n%s", orden, strings.Join(args, " "), err, out)
	}
}

// xvfb arranca un servidor X virtual en una pantalla libre y devuelve su nombre.
func xvfb(t *testing.T) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	x := exec.Command("Xvfb", "-displayfd", "3", "-screen", "0", "800x600x24", "-nolisten", "tcp")
	x.ExtraFiles = []*os.File{w}
	if err := x.Start(); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	t.Cleanup(func() { _ = x.Process.Kill(); _ = x.Wait() })
	var n int
	if _, err := fmt.Fscan(r, &n); err != nil {
		t.Fatalf("Xvfb no dijo su pantalla: %v", err)
	}
	return ":" + strconv.Itoa(n)
}

func esperar(t *testing.T, eventos <-chan evento, plazo time.Duration) evento {
	t.Helper()
	select {
	case e, ok := <-eventos:
		if !ok {
			t.Fatal("la ventana se cerró")
		}
		return e
	case <-time.After(plazo):
		t.Fatalf("en %v no llegó nada a la ventana", plazo)
	}
	return evento{}
}
