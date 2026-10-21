// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

// Package ficheros es lo que la suite hace con los ficheros DEL EQUIPO, y lo hace UNA
// vez para las quince apps de escritorio: dejar una descarga donde la persona la
// encuentre, enseñarla en el gestor de ficheros del sistema, abrirla con la aplicación
// que el sistema tenga asociada e imprimirla.
//
// Lo que cambia entre macOS, Windows y Linux está en un fichero por sistema
// (sistema_*.go) y NO se elige con un `switch runtime.GOOS`: así el compilador se lleva
// lo que no va, y `GOOS=windows go vet` comprueba de verdad la rama de Windows en vez de
// dejarla sin mirar hasta que alguien la ejecute.
//
// Deliberadamente NO hay aquí: acceso al sistema de ficheros más allá de la carpeta de
// descargas y de las rutas que la persona haya elegido en un diálogo del sistema, ni
// observación de carpetas. El encargo v4 §5 prohíbe el acceso total al sistema de
// ficheros, y una suite que solo sube y baja documentos no lo necesita.
package ficheros

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"git.sytue.eu/sytue/one/apps/nucleo/nombres"
)

// ErrNoDisponible es que ESTE sistema no ofrece la operación. Quien llama lo trata como
// «esta plataforma no lo tiene» y degrada: no es un fallo que enseñar como error.
var ErrNoDisponible = errors.New("ficheros: el sistema no ofrece esa operación")

// Guardar escribe `contenido` en `carpeta` SIN pisar nada —si el nombre existe, «informe
// (1).pdf»— y devuelve dónde ha quedado. El nombre lo pone un servidor, o el remitente de
// un correo: se queda con su última parte, y la escritura va confinada a la carpeta.
func Guardar(carpeta, nombre string, contenido io.Reader) (string, error) {
	if err := os.MkdirAll(carpeta, 0o750); err != nil {
		return "", err
	}
	raiz, err := os.OpenRoot(carpeta)
	if err != nil {
		return "", err
	}
	defer func() { _ = raiz.Close() }()
	nombre = filepath.Base(strings.ReplaceAll(nombres.Limpio(nombre), "\\", "/"))
	if nombre == "." || nombre == ".." || nombre == string(filepath.Separator) {
		nombre = "descarga"
	}
	extension := filepath.Ext(nombre)
	base := strings.TrimSuffix(nombre, extension)
	for n := range 1000 {
		candidato := nombre
		if n > 0 {
			candidato = fmt.Sprintf("%s (%d)%s", base, n, extension)
		}
		f, err := raiz.OpenFile(candidato, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = io.Copy(f, contenido)
		if cerrar := f.Close(); err == nil {
			err = cerrar
		}
		return filepath.Join(carpeta, candidato), err
	}
	return "", errors.New("ficheros: demasiados ficheros con ese nombre")
}

// ConProgreso envuelve un lector y dice por dónde va, en fracción de `total`, cada vez
// que avanza un punto porcentual: lo justo para una barra o un «37 %», sin inundar la
// rutina de la interfaz con un aviso por cada bloque leído. Sin total (0 o menos), no
// avisa: no hay fracción que dar.
func ConProgreso(r io.Reader, total int64, avisar func(float64)) io.Reader {
	return &conProgreso{r: r, total: total, avisar: avisar, ultimo: -1}
}

type conProgreso struct {
	r            io.Reader
	leido, total int64
	ultimo       int64
	avisar       func(float64)
}

func (c *conProgreso) Read(b []byte) (int, error) {
	n, err := c.r.Read(b)
	c.leido += int64(n)
	if c.total > 0 && c.avisar != nil {
		if pct := min(c.leido, c.total) * 100 / c.total; pct != c.ultimo {
			c.ultimo = pct
			c.avisar(float64(min(c.leido, c.total)) / float64(c.total))
		}
	}
	return n, err
}

// ImprimirDatos imprime lo que la app tiene en MEMORIA —el PDF abierto en CertiPDF, el
// que un editor acaba de exportar—: lo deja con su nombre en una carpeta temporal propia
// y se lo pasa a [Imprimir]. Si el sistema no sabe imprimir ese tipo sin más (Windows
// sin verbo `print` registrado para él), lo abre en su visor, donde Ctrl+P es el
// diálogo. BLOQUEA lo que bloquee Imprimir: desde una rutina propia.
//
// ponytail: la copia se queda en la carpeta temporal hasta que el sistema la limpia (o la
// pisa la siguiente impresión con el mismo nombre): el visor la necesita mientras su
// diálogo está abierto, y nadie avisa de cuándo se cierra.
func ImprimirDatos(nombre string, datos []byte) error {
	ruta, err := copiaParaImprimir(os.TempDir(), nombre, datos)
	if err != nil {
		return err
	}
	if err = Imprimir(ruta); err != nil && !errors.Is(err, ErrNoDisponible) {
		err = Abrir(ruta)
	}
	return err
}

// copiaParaImprimir escribe `datos` como `<tmp>/sytue-imprimir-<azar>/<nombre>`,
// confinado a esa carpeta, y devuelve la ruta. La carpeta es NUEVA en cada impresión
// y solo la ve quien la crea: con un nombre fijo en el /tmp compartido de Linux, otra
// cuenta del equipo la creaba antes, abierta o como enlace, y leía o cambiaba lo que
// se mandaba a imprimir (NAT-13).
func copiaParaImprimir(tmp, nombre string, datos []byte) (string, error) {
	dir, err := os.MkdirTemp(tmp, "sytue-imprimir-")
	if err != nil {
		return "", err
	}
	raiz, err := os.OpenRoot(dir)
	if err != nil {
		return "", err
	}
	defer func() { _ = raiz.Close() }()
	nombre = filepath.Base(strings.ReplaceAll(nombre, "\\", "/"))
	if nombre == "." || nombre == ".." || nombre == string(filepath.Separator) {
		nombre = "documento.pdf"
	}
	if err := raiz.WriteFile(nombre, datos, 0o600); err != nil {
		return "", err
	}
	return filepath.Join(dir, nombre), nil
}

// LeerLocalSeguro lee los bytes de un fichero de la máquina o de una URI local (file://).
// En iOS accede al recurso con la extensión de sandbox adecuada (security-scoped resource).
func LeerLocalSeguro(ruta string) ([]byte, error) {
	return leerLocalSeguro(ruta)
}

// local comprueba que `ruta` es un fichero de verdad de este equipo y la devuelve
// absoluta. Revelar, Abrir e Imprimir acaban las tres en un programa del sistema, y a un
// programa no se le pasa lo que no se ha mirado: no hay intérprete de órdenes de por
// medio —exec.Command no usa ninguno—, pero una ruta relativa que empezara por «-» sí se
// leería como una opción, y filepath.Abs lo descarta.
//
// El error nombra el fichero por su BASE, nunca por la ruta: el v4 §4 prohíbe rutas
// locales en errores, y sin el nombre no se sabe de qué fichero habla.
func local(ruta string) (string, error) {
	if strings.TrimSpace(ruta) == "" {
		return "", errors.New("ficheros: no hay ruta")
	}
	abs, err := filepath.Abs(ruta)
	if err != nil {
		return "", errors.New("ficheros: la ruta no se deja resolver")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("ficheros: %s no está donde se dijo", filepath.Base(abs))
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("ficheros: %s no es un fichero", filepath.Base(abs))
	}
	return abs, nil
}

// rutaElegida dice si una línea de la salida de un selector es una ruta que la
// persona eligió: no vacía y ABSOLUTA. El selector da una ruta por línea, y un nombre
// con salto de línea («informe.pdf⏎.netrc») salía partido en dos; la segunda mitad,
// relativa al directorio de trabajo, era un fichero que nadie eligió (FYS-01).
func rutaElegida(linea string) bool {
	return strings.TrimSpace(linea) != "" && (filepath.IsAbs(linea) || strings.HasPrefix(linea, "/"))
}

// lineas parte la salida de un selector en rutas: una por línea, sin el retorno de carro
// de Windows y sin las vacías del final.
func lineas(salida []byte) []string {
	var rutas []string
	for _, l := range strings.Split(string(salida), "\n") {
		if l = strings.TrimRight(l, "\r"); rutaElegida(l) {
			rutas = append(rutas, l)
		}
	}
	return rutas
}
