// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sistema

import (
	"errors"

	"git.wow.st/gmp/jni"
)

// # `new Clase(…)` desde Go
//
// `jni.NewObject` de git.wow.st/gmp/jni NO sirve: su C llama a `(*env)->NewObject(env,
// cls, m, args)` —la variante de argumentos variables— pasándole el `jvalue*` que debería
// ir a `NewObjectA`. El constructor recibe como primer argumento la dirección del vector
// y lo demás es basura: guardar la sesión en el Keystore tumbaba la app con SIGSEGV en
// `KeyGenParameterSpec$Builder.<init>` justo después de iniciar sesión (visto en el
// emulador de la API 36 con Sytue Drive, 2026-09-28). Las llamadas a métodos sí usan las
// variantes `A`, así que se construye por reflexión: `getConstructor(tipos)` y
// `newInstance(args)`. Nada de C nuevo; lo vigila TestNadieUsaJniNewObject.

// Construir es `new cls(args…)` con el constructor de esos tipos. `tipos` son objetos
// `Class` (TipoInt, Clase) y `args`, objetos (los enteros, con Entero).
func Construir(env jni.Env, cls jni.Class, tipos []jni.Object, args ...jni.Object) (jni.Object, error) {
	if len(tipos) != len(args) {
		return 0, errors.New("sistema: Construir con distinto número de tipos y de argumentos")
	}
	claseClase := jni.GetObjectClass(env, jni.Object(cls))
	arrTipos := jni.NewObjectArray(env, jni.Size(len(tipos)), claseClase, 0)
	for i, t := range tipos {
		if err := jni.SetObjectArrayElement(env, arrTipos, jni.Size(i), t); err != nil {
			return 0, err
		}
	}
	ctor, err := jni.CallObjectMethod(env, jni.Object(cls),
		jni.GetMethodID(env, claseClase, "getConstructor", "([Ljava/lang/Class;)Ljava/lang/reflect/Constructor;"),
		jni.Value(arrTipos))
	if err != nil {
		return 0, err
	}
	claseObjeto := jni.FindClass(env, "java/lang/Object")
	arrArgs := jni.NewObjectArray(env, jni.Size(len(args)), claseObjeto, 0)
	for i, a := range args {
		if err := jni.SetObjectArrayElement(env, arrArgs, jni.Size(i), a); err != nil {
			return 0, err
		}
	}
	return jni.CallObjectMethod(env, ctor,
		jni.GetMethodID(env, jni.GetObjectClass(env, ctor), "newInstance", "([Ljava/lang/Object;)Ljava/lang/Object;"),
		jni.Value(arrArgs))
}

// Clase es el objeto `Class` de una clase del sistema, como tipo de un parámetro
// (`java/lang/String`, `[B`).
func Clase(env jni.Env, nombre string) jni.Object { return jni.Object(jni.FindClass(env, nombre)) }

// TipoInt es `int.class` (`Integer.TYPE`), el tipo de un parámetro `int`.
func TipoInt(env jni.Env) jni.Object {
	c := jni.FindClass(env, "java/lang/Integer")
	return jni.GetStaticObjectField(env, c, jni.GetStaticFieldID(env, c, "TYPE", "Ljava/lang/Class;"))
}

// Entero es `Integer.valueOf(n)`: un `int` como argumento de Construir.
func Entero(env jni.Env, n int) (jni.Object, error) {
	c := jni.FindClass(env, "java/lang/Integer")
	return jni.CallStaticObjectMethod(env, c, jni.GetStaticMethodID(env, c, "valueOf", "(I)Ljava/lang/Integer;"), jni.Value(n))
}
