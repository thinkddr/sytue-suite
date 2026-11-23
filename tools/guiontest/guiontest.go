// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package guiontest es el arnés de las pruebas de Go que ejercitan guiones de
// shell del repositorio (despliegue, CI, empaquetado): CONVENTIONS §13.4 pide
// que la prueba sea de Go aunque lo probado sea un guion. Ejecuta bash con un
// entorno controlado, deja poner dobles de binarios delante en el PATH y
// devuelve salida, error y código sin interpretar nada: cada prueba decide.
package guiontest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Raiz es la raíz del repositorio: el primer directorio hacia arriba con `.git`.
func Raiz(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Fatal("guiontest: no se encuentra la raíz del repositorio")
		}
		dir = padre
	}
}

// Resultado es lo que dejó una ejecución.
type Resultado struct {
	Salida string // stdout
	Error  string // stderr
	Codigo int    // código de salida; -1 si no llegó a terminar
}

// Texto es stdout como lo vería `$(…)` en shell: sin los saltos de línea finales.
func (r Resultado) Texto() string { return strings.TrimRight(r.Salida, "\n") }

// Todo es stdout seguido de stderr, para buscar un texto sin importar por dónde salió.
func (r Resultado) Todo() string { return r.Salida + r.Error }

// Opciones de una ejecución. El valor cero ejecuta en la raíz del repositorio,
// con el entorno del proceso y sin entrada.
type Opciones struct {
	Dir     string        // directorio de trabajo; por defecto, la raíz
	Env     []string      // variables `CLAVE=valor` añadidas al entorno (ganan a las heredadas)
	Limpio  bool          // true: no hereda el entorno; solo PATH, HOME y Env
	Entrada string        // stdin
	Plazo   time.Duration // por defecto, dos minutos
}

// Bash ejecuta `guion` con `bash -c`; los argumentos quedan en `$1`, `$2`, etc.
func Bash(t testing.TB, o Opciones, guion string, args ...string) Resultado {
	t.Helper()
	return Ejecutar(t, o, "bash", append([]string{"-c", guion, "bash"}, args...)...)
}

// Guion ejecuta un fichero de shell con `bash <ruta> args…`. La ruta es relativa a la raíz.
func Guion(t testing.TB, o Opciones, ruta string, args ...string) Resultado {
	t.Helper()
	if !filepath.IsAbs(ruta) {
		ruta = filepath.Join(Raiz(t), ruta)
	}
	return Ejecutar(t, o, "bash", append([]string{ruta}, args...)...)
}

// Ejecutar lanza un programa cualquiera con las mismas opciones.
func Ejecutar(t testing.TB, o Opciones, programa string, args ...string) Resultado {
	t.Helper()
	plazo := o.Plazo
	if plazo == 0 {
		plazo = 2 * time.Minute
	}
	ctx, cancelar := context.WithTimeout(context.Background(), plazo)
	defer cancelar()
	cmd := exec.CommandContext(ctx, programa, args...)
	cmd.Dir = o.Dir
	if cmd.Dir == "" {
		cmd.Dir = Raiz(t)
	}
	if o.Limpio {
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME")}
	} else {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, o.Env...)
	if o.Entrada != "" {
		cmd.Stdin = strings.NewReader(o.Entrada)
	}
	var salida, fallo bytes.Buffer
	cmd.Stdout, cmd.Stderr = &salida, &fallo
	err := cmd.Run()
	r := Resultado{Salida: salida.String(), Error: fallo.String()}
	var sale *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &sale):
		r.Codigo = sale.ExitCode()
	default:
		t.Fatalf("guiontest: %s no se pudo ejecutar: %v", programa, err)
	}
	if ctx.Err() != nil {
		t.Fatalf("guiontest: %s superó el plazo de %s\n%s", programa, plazo, r.Todo())
	}
	return r
}

// Doble escribe un ejecutable de mentira (`#!/usr/bin/env bash` + cuerpo) en dir
// y devuelve su ruta. Con ConPATH(dir) delante, sustituye al binario de verdad.
func Doble(t testing.TB, dir, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	Escribir(t, ruta, "#!/usr/bin/env bash\n"+cuerpo+"\n")
	if err := os.Chmod(ruta, 0o755); err != nil { //nolint:gosec // G302: un doble de binario tiene que ser ejecutable.
		t.Fatal(err)
	}
	return ruta
}

// ConPATH es la variable PATH con dir por delante del PATH del proceso.
func ConPATH(dir string) string {
	return "PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// Escribir crea el fichero (y sus directorios) con ese contenido.
func Escribir(t testing.TB, ruta, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(contenido), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Leer devuelve el contenido de un fichero; la ruta relativa lo es a la raíz.
func Leer(t testing.TB, ruta string) string {
	t.Helper()
	if !filepath.IsAbs(ruta) {
		ruta = filepath.Join(Raiz(t), ruta)
	}
	raw, err := os.ReadFile(ruta) //nolint:gosec // G304: rutas de prueba dentro del repositorio.
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// SinSaltos es la variable que convierte en fallo lo que Necesita saltaría. La
// pone el CI donde las herramientas están instaladas a propósito: allí una
// prueba saltada sería un verde que no ha comprobado nada.
const SinSaltos = "SYTUE_PRUEBAS_SIN_SALTOS"

// Necesita salta la prueba si falta alguno de esos programas (o falla, con
// SYTUE_PRUEBAS_SIN_SALTOS=1). El motivo queda escrito: una prueba que no corre
// tiene que verse.
func Necesita(t testing.TB, programas ...string) {
	t.Helper()
	for _, p := range programas {
		if _, err := exec.LookPath(p); err != nil {
			if os.Getenv(SinSaltos) == "1" {
				t.Fatalf("falta %s y %s=1 no deja saltar la prueba", p, SinSaltos)
			}
			t.Skipf("falta %s en este entorno: la prueba no puede ejercitar el guion", p)
		}
	}
}
