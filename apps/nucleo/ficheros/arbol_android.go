// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package ficheros

import (
	"fmt"
	"io"
	"os"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// La clase propia de la APK que habla con el Storage Access Framework (arbol.go).
const claseArbol = "eu.sytue.nativo.ArbolDocumentos"

// Las firmas de sus métodos: (Context, String…) → String salvo donde se dice.
const (
	firmaCadena1 = "(Landroid/content/Context;Ljava/lang/String;)Ljava/lang/String;"
	firmaCadena2 = "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;"
	firmaCadena3 = "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;"
	firmaCadena4 = "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;"
)

// llamarArbol llama a un método estático de ArbolDocumentos con el contexto y las cadenas
// dadas, y devuelve la cadena que conteste.
func llamarArbol(metodo, firma string, cadenas ...string) (string, error) {
	var out string
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		args := []jni.Value{jni.Value(ctx)}
		for _, c := range cadenas {
			args = append(args, jni.Value(jni.JavaString(env, c)))
		}
		o, err := jni.CallStaticObjectMethod(env, cls, jni.GetStaticMethodID(env, cls, metodo, firma), args...)
		if err == nil && o != 0 {
			out = jni.GoString(env, jni.String(o))
		}
		return err
	})
	return out, errorJava(err)
}

// ElegirArbol abre el selector de carpetas del sistema y espera a que la persona elija
// (o cancele: ArbolElegido vacío y sin error). Lo elegido queda con permiso guardado de
// leer y escribir; una carpeta que solo deja leer da ErrSoloLectura con lo elegido, que
// se puede leer mientras la app siga abierta. Se llama fuera de la rutina de la interfaz.
func ElegirArbol() (ArbolElegido, error) {
	var crudo string
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		o, err := jni.CallStaticObjectMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "elegir", "(Landroid/content/Context;)Ljava/lang/String;"), jni.Value(ctx))
		if err == nil && o != 0 {
			crudo = jni.GoString(env, jni.String(o))
		}
		return err
	})
	if err != nil {
		return ArbolElegido{}, err
	}
	return leerElegido(crudo)
}

// ArbolVigente dice si la app sigue teniendo permiso guardado de leer y escribir en el
// árbol (la persona puede retirarlo en los ajustes del sistema).
func ArbolVigente(uri string) bool {
	var si bool
	_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		si, err = jni.CallStaticBooleanMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "vigente", "(Landroid/content/Context;Ljava/lang/String;)Z"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, uri)))
		return err
	})
	return si
}

// SoltarArbol devuelve el permiso guardado sobre el árbol.
func SoltarArbol(uri string) {
	_ = sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls,
			jni.GetStaticMethodID(env, cls, "soltar", "(Landroid/content/Context;Ljava/lang/String;)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, uri)))
	})
}

func (a Arbol) uno(metodo, firma string, cadenas ...string) (DocumentoArbol, error) {
	crudo, err := llamarArbol(metodo, firma, append([]string{a.URI}, cadenas...)...)
	if err != nil {
		return DocumentoArbol{}, err
	}
	return leerDocumento(crudo)
}

// Raiz es la carpeta elegida.
func (a Arbol) Raiz() (DocumentoArbol, error) { return a.uno("raiz", firmaCadena1) }

// Datos de un documento.
func (a Arbol) Datos(id string) (DocumentoArbol, error) { return a.uno("datos", firmaCadena2, id) }

// Hijos de una carpeta.
func (a Arbol) Hijos(id string) ([]DocumentoArbol, error) {
	crudo, err := llamarArbol("hijos", firmaCadena2, a.URI, id)
	if err != nil {
		return nil, err
	}
	return leerDocumentos(crudo)
}

// Crear crea en padre un fichero vacío o una carpeta.
func (a Arbol) Crear(padre, nombre string, carpeta bool) (DocumentoArbol, error) {
	var crudo string
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		c := jni.Value(0)
		if carpeta {
			c = jni.Value(1)
		}
		o, err := jni.CallStaticObjectMethod(env, cls, jni.GetStaticMethodID(env, cls, "crear",
			"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Z)Ljava/lang/String;"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, a.URI)), jni.Value(jni.JavaString(env, padre)),
			jni.Value(jni.JavaString(env, nombre)), c)
		if err == nil && o != 0 {
			crudo = jni.GoString(env, jni.String(o))
		}
		return err
	})
	if err != nil {
		return DocumentoArbol{}, errorJava(err)
	}
	return leerDocumento(crudo)
}

// Renombrar cambia el nombre; el identificador puede cambiar.
func (a Arbol) Renombrar(id, nombre string) (DocumentoArbol, error) {
	return a.uno("renombrar", firmaCadena3, id, nombre)
}

// Mover lleva id de la carpeta origen a destino; ErrNoSoportado si el proveedor no sabe.
func (a Arbol) Mover(id, origen, destino string) (DocumentoArbol, error) {
	return a.uno("mover", firmaCadena4, id, origen, destino)
}

// Borrar borra un documento; una carpeta, con lo que tenga.
func (a Arbol) Borrar(id string) error {
	return errorJava(sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, cls, jni.GetStaticMethodID(env, cls, "borrar",
			"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, a.URI)), jni.Value(jni.JavaString(env, id)))
	}))
}

// abrir da el contenido como un fichero de Go sobre el descriptor que suelta el proveedor.
func (a Arbol) abrir(id, modo string) (*os.File, error) {
	fd := -1
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		fd, err = jni.CallStaticIntMethod(env, cls, jni.GetStaticMethodID(env, cls, "abrir",
			"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)I"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, a.URI)), jni.Value(jni.JavaString(env, id)),
			jni.Value(jni.JavaString(env, modo)))
		return err
	})
	if err != nil {
		return nil, errorJava(err)
	}
	if fd < 0 {
		return nil, fmt.Errorf("ficheros: el proveedor no abrió el documento")
	}
	return os.NewFile(uintptr(fd), id), nil
}

// Leer abre el contenido para leerlo.
func (a Arbol) Leer(id string) (io.ReadCloser, error) { return a.abrir(id, "r") }

// Escribir lo abre vaciándolo antes.
func (a Arbol) Escribir(id string) (io.WriteCloser, error) { return a.abrir(id, "wt") }

// Espacio del almacenamiento del árbol; ErrNoDisponible si el proveedor no lo dice.
func (a Arbol) Espacio() (total, libre int64, err error) {
	crudo, err := llamarArbol("espacio", firmaCadena1, a.URI)
	if err != nil {
		return 0, 0, err
	}
	return leerEspacio(crudo)
}

// Ver abre el documento con la app del sistema para su tipo (abrir) o la hoja de compartir.
func (a Arbol) Ver(id string, abrir bool) error {
	return errorJava(sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		v := jni.Value(0)
		if abrir {
			v = jni.Value(1)
		}
		return jni.CallStaticVoidMethod(env, cls, jni.GetStaticMethodID(env, cls, "ver",
			"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Z)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, a.URI)), jni.Value(jni.JavaString(env, id)), v)
	}))
}

// VerEn abre el documento con la app de ese paquete (`eu.sytue.office.docus`) como un
// `ACTION_VIEW` explícito del tipo dado, con permiso de leerlo y escribirlo. Devuelve
// false, sin error, si esa app no está instalada o no acepta el tipo: entonces quien
// llama lo enseña con [Arbol.Ver], con la app del sistema.
func (a Arbol) VerEn(id, paquete, tipo string) (bool, error) {
	var si bool
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		cls, err := sistema.CargarClase(env, ctx, claseArbol)
		if err != nil {
			return err
		}
		si, err = jni.CallStaticBooleanMethod(env, cls, jni.GetStaticMethodID(env, cls, "verEn",
			"(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Z"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, a.URI)), jni.Value(jni.JavaString(env, id)),
			jni.Value(jni.JavaString(env, paquete)), jni.Value(jni.JavaString(env, tipo)))
		return err
	})
	return si, errorJava(err)
}
