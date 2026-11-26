// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package ficheros

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"
)

// # Una carpeta elegida con el selector del sistema (DR-12)
//
// En Android, Drive de Sytue Office puede trabajar sobre una carpeta del almacenamiento
// compartido (Descargas, una tarjeta…) que la persona elige con el selector de carpetas del
// sistema (`ACTION_OPEN_DOCUMENT_TREE`, Storage Access Framework). No es una ruta: es un
// ÁRBOL de documentos (`content://…/tree/…`) con permiso de lectura y escritura que la app
// guarda y que sigue valiendo tras reiniciar, hasta que la persona lo retire.
//
// [ElegirArbol] abre el selector; [ArbolVigente] dice si el permiso sigue; [Arbol] es lo
// que se hace con sus documentos (lo de `DocumentsContract`), por su identificador de
// proveedor. Lo usa `archivos.Arbol`, que lo presenta como rutas a Drive. Fuera de Android
// todo devuelve ErrNoDisponible.
//
// Lo hace la clase propia de la APK `eu.sytue.nativo.ArbolDocumentos`
// (apps/fyne/android/java), como «Guardar como…» (movil_guardar_android.go): no hace falta
// tocar el fork de Fyne, cuyo selector de carpetas solo se alcanza con `dialog`.

// ErrNoSoportado: el proveedor de la carpeta no sabe hacer eso (mover, en uno de nube).
var ErrNoSoportado = errors.New("ficheros: el proveedor de la carpeta no lo admite")

// ErrSoloLectura: la carpeta elegida no deja escribir. [ElegirArbol] la devuelve con lo
// elegido ([ArbolElegido.SoloLectura]), que aún se puede leer.
var ErrSoloLectura = errors.New("ficheros: la carpeta elegida no deja escribir")

// ArbolElegido es lo que dio el selector de carpetas.
type ArbolElegido struct {
	// URI del árbol (`content://…/tree/…`).
	URI string
	// Nombre de la carpeta, el que enseña el proveedor.
	Nombre string
	// Persistente: el permiso queda guardado y vale tras cerrar la app. Si no, vale hasta
	// cerrarla (un proveedor que no los da).
	Persistente bool
	// SoloLectura: el proveedor solo deja leer (va con ErrSoloLectura). Vale para leer
	// mientras la app siga abierta: sirve de ORIGEN («Subir carpeta»), no de destino.
	SoloLectura bool
}

// DocumentoArbol es un documento del árbol como lo describe su proveedor.
type DocumentoArbol struct {
	ID         string
	Nombre     string
	Carpeta    bool
	Tamano     int64 // -1 si el proveedor no lo dice
	Modificado time.Time
}

// Arbol es la carpeta elegida, por su URI.
type Arbol struct{ URI string }

// leerElegido interpreta lo que devuelve ArbolDocumentos.elegir: `<uri>\n<nombre>\n<permiso>`,
// "" al cancelar o "!" sin ventana.
func leerElegido(crudo string) (ArbolElegido, error) {
	switch crudo {
	case "":
		return ArbolElegido{}, nil
	case "!":
		return ArbolElegido{}, fmt.Errorf("%w: sin ventana a la que pegar el selector", ErrNoDisponible)
	}
	partes := strings.SplitN(crudo, "\n", 3)
	if len(partes) != 3 || !strings.HasPrefix(partes[0], "content://") {
		return ArbolElegido{}, fmt.Errorf("ficheros: respuesta del selector ilegible")
	}
	e := ArbolElegido{URI: partes[0], Nombre: partes[1]}
	switch partes[2] {
	case "rw":
		e.Persistente = true
	case "t":
	case "r":
		e.SoloLectura = true
		return e, ErrSoloLectura
	default:
		return ArbolElegido{}, ErrSoloLectura
	}
	return e, nil
}

// leerDocumento interpreta una línea de ArbolDocumentos.registro: `id`, `1`/`0`, tamaño,
// fecha en milisegundos y nombre, separados por tabuladores y con `\`, el tabulador y el
// salto de línea escapados.
func leerDocumento(linea string) (DocumentoArbol, error) {
	f := strings.Split(linea, "\t")
	if len(f) != 5 {
		return DocumentoArbol{}, fmt.Errorf("ficheros: documento ilegible: %q", linea)
	}
	tamano, err1 := strconv.ParseInt(f[2], 10, 64)
	ms, err2 := strconv.ParseInt(f[3], 10, 64)
	if err := errors.Join(err1, err2); err != nil {
		return DocumentoArbol{}, fmt.Errorf("ficheros: documento ilegible: %w", err)
	}
	d := DocumentoArbol{ID: desescapar(f[0]), Carpeta: f[1] == "1", Tamano: tamano, Nombre: desescapar(f[4])}
	if ms > 0 {
		d.Modificado = time.UnixMilli(ms)
	}
	return d, nil
}

// leerDocumentos es una línea por documento; "" es ninguno.
func leerDocumentos(crudo string) ([]DocumentoArbol, error) {
	if crudo == "" {
		return nil, nil
	}
	var out []DocumentoArbol
	for linea := range strings.SplitSeq(crudo, "\n") {
		d, err := leerDocumento(linea)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func desescapar(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 't':
				b.WriteByte('\t')
			case 'n':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// leerEspacio interpreta `<total>\t<libre>`; "" es que el proveedor no lo dice.
func leerEspacio(crudo string) (total, libre int64, err error) {
	t, l, ok := strings.Cut(crudo, "\t")
	if !ok {
		return 0, 0, ErrNoDisponible
	}
	total, err1 := strconv.ParseInt(t, 10, 64)
	libre, err2 := strconv.ParseInt(l, 10, 64)
	if err := errors.Join(err1, err2); err != nil {
		return 0, 0, ErrNoDisponible
	}
	return total, libre, nil
}

// errorJava traduce la excepción de ArbolDocumentos (el `toString` que da jni): lo que
// no está, lo que el proveedor no sabe hacer y lo que no deja.
func errorJava(err error) error {
	if err == nil {
		return nil
	}
	m := err.Error()
	switch {
	case strings.HasPrefix(m, "java.io.FileNotFoundException"):
		return fmt.Errorf("%w: %s", fs.ErrNotExist, m)
	case strings.HasPrefix(m, "java.lang.UnsupportedOperationException"):
		return fmt.Errorf("%w: %s", ErrNoSoportado, m)
	case strings.HasPrefix(m, "java.lang.SecurityException"):
		return fmt.Errorf("%w: %s", fs.ErrPermission, m)
	}
	return err
}
