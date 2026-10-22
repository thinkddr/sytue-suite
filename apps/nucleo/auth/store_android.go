// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"git.wow.st/gmp/jni"

	"git.sytue.eu/sytue/one/apps/nucleo/ajustes"
	"git.sytue.eu/sytue/one/apps/nucleo/sistema"
)

// La sesión en ANDROID: cifrada con una clave AES-256-GCM del Android Keystore y
// guardada en el directorio privado de la app.
//
// Por qué las dos cosas y no solo el directorio privado. Ese directorio ya lo protegen
// el UID de la app y el cifrado del dispositivo, pero `android:allowBackup` vale true
// si el manifiesto no dice otra cosa, así que una copia de seguridad se llevaría el
// fichero fuera del teléfono. Una clave del
// Keystore NO se copia y no sale del dispositivo (en la mayoría, del TEE), de modo que
// el fichero copiado no vale en ningún otro sitio. Es lo que hace
// EncryptedSharedPreferences por debajo, sin arrastrar AndroidX ni un `.jar` propio que
// compilar y mantener.
//
// Todo el diálogo con el Keystore es JNI contra clases del PROPIO Android
// (java.security.KeyStore, javax.crypto.Cipher…): no hay código Java nuestro.
const (
	aliasClave     = "eu.sytue.one.sesion"
	ficheroSesion  = "sesion.bin"
	etiquetaGCMbit = 128 // la etiqueta de autenticación de GCM, en bits
	cifrar         = 1   // Cipher.ENCRYPT_MODE
	descifrar      = 2   // Cipher.DECRYPT_MODE
)

func rutaSesion(cuenta ...string) (string, error) {
	dir, err := dirPrivado()
	if err != nil {
		return "", err
	}
	nombre := ficheroSesion
	if len(cuenta) > 0 && cuenta[0] != "sesion" {
		nombre = cuenta[0] + ".bin"
	}
	return filepath.Join(dir, nombre), nil
}

// dirPrivado es `context.getFilesDir().getAbsolutePath()`: el directorio de la app al
// que no llega ninguna otra.
func dirPrivado() (string, error) {
	var dir string
	err := sistema.Java(func(env jni.Env, ctx jni.Object) error {
		f, err := jni.CallObjectMethod(env, ctx,
			jni.GetMethodID(env, jni.GetObjectClass(env, ctx), "getFilesDir", "()Ljava/io/File;"))
		if err != nil {
			return err
		}
		ruta, err := jni.CallObjectMethod(env, f,
			jni.GetMethodID(env, jni.GetObjectClass(env, f), "getAbsolutePath", "()Ljava/lang/String;"))
		if err != nil {
			return err
		}
		dir = jni.GoString(env, jni.String(ruta))
		return nil
	})
	if err == nil && dir == "" {
		err = errors.New("auth: Android no ha dado el directorio privado de la app")
	}
	return dir, err
}

// cargar recupera la sesión guardada. Sin ninguna, devuelve Tokens vacíos y nil.
func cargar() (Tokens, error) { return cargarCuenta("sesion") }

func cargarCuenta(cuenta string) (Tokens, error) {
	ruta, err := rutaSesion(cuenta)
	if err != nil {
		return Tokens{}, err
	}
	sobre, err := os.ReadFile(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return Tokens{}, nil
	}
	if err != nil {
		return Tokens{}, err
	}
	iv, cifrado, err := desempaquetar(sobre)
	if err != nil {
		return Tokens{}, err
	}
	claro, err := abrirSobre(iv, cifrado)
	if err != nil {
		return Tokens{}, err
	}
	return recuperar(claro)
}

// guardar persiste el refresh token, o borra el fichero cuando ya no hay sesión.
func guardar(t Tokens) error { return guardarCuenta("sesion", t) }

func guardarCuenta(cuenta string, t Tokens) error {
	ruta, err := rutaSesion(cuenta)
	if err != nil {
		return err
	}
	if t.Refresh == "" {
		if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	crudo, err := serializar(t)
	if err != nil {
		return err
	}
	iv, cifrado, err := cerrarSobre(crudo)
	if err != nil {
		return err
	}
	sobre, err := empaquetar(iv, cifrado)
	if err != nil {
		return err
	}
	return ajustes.EscribirEntero(ruta, sobre)
}

// Los demás secretos, cada uno en su fichero del directorio privado y cifrado con la
// MISMA clave del Keystore que la sesión: `<cuenta>.bin`, que ya viene validada.
func rutaSecreto(cuenta string) (string, error) {
	dir, err := dirPrivado()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "secreto-"+cuenta+".bin"), nil
}

func leerSecreto(cuenta string) ([]byte, error) {
	if cuenta == CuentaBloqueo {
		if habilitada, err := sistema.SesionAndroidHabilitada(); err != nil {
			return nil, err
		} else if habilitada {
			texto, err := sistema.EstadoSesionAndroid()
			if err != nil {
				return nil, err
			}
			var estado struct {
				Bloqueo bool `json:"bloqueo"`
			}
			if err := json.Unmarshal([]byte(texto), &estado); err != nil {
				return nil, err
			}
			if estado.Bloqueo {
				return []byte("1"), nil
			}
			return nil, nil
		}
	}
	ruta, err := rutaSecreto(cuenta)
	if err != nil {
		return nil, err
	}
	sobre, err := os.ReadFile(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	iv, cifrado, err := desempaquetar(sobre)
	if err != nil {
		return nil, err
	}
	return abrirSobre(iv, cifrado)
}

func guardarSecreto(cuenta string, datos []byte) error {
	if cuenta == CuentaBloqueo {
		if habilitada, err := sistema.SesionAndroidHabilitada(); err != nil {
			return err
		} else if habilitada {
			return sistema.BloquearSesionAndroid(true)
		}
	}
	ruta, err := rutaSecreto(cuenta)
	if err != nil {
		return err
	}
	iv, cifrado, err := cerrarSobre(datos)
	if err != nil {
		return err
	}
	sobre, err := empaquetar(iv, cifrado)
	if err != nil {
		return err
	}
	return ajustes.EscribirEntero(ruta, sobre)
}

func borrarSecreto(cuenta string) error {
	if cuenta == CuentaBloqueo {
		if habilitada, err := sistema.SesionAndroidHabilitada(); err != nil {
			return err
		} else if habilitada {
			return sistema.BloquearSesionAndroid(false)
		}
	}
	ruta, err := rutaSecreto(cuenta)
	if err != nil {
		return err
	}
	if err := os.Remove(ruta); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// --- JNI --------------------------------------------------------------------------

// enAndroid ejecuta f con el entorno JNI y un cargador de clases del sistema.
func enAndroid(f func(env jni.Env, clase func(string) (jni.Class, error)) error) error {
	return sistema.Java(func(env jni.Env, ctx jni.Object) error {
		return f(env, func(nombre string) (jni.Class, error) {
			return sistema.CargarClase(env, ctx, nombre)
		})
	})
}

// clave devuelve la clave de la app en el Android Keystore, creándola la primera vez.
// Nunca sale del almacén: se usa por referencia.
//
//	KeyStore ks = KeyStore.getInstance("AndroidKeyStore"); ks.load(null);
//	Key k = ks.getKey(alias, null);
func clave(env jni.Env, clase func(string) (jni.Class, error)) (jni.Object, error) {
	claseKS, err := clase("java/security/KeyStore")
	if err != nil {
		return 0, err
	}
	ks, err := jni.CallStaticObjectMethod(env, claseKS,
		jni.GetStaticMethodID(env, claseKS, "getInstance", "(Ljava/lang/String;)Ljava/security/KeyStore;"),
		jni.Value(jni.JavaString(env, "AndroidKeyStore")))
	if err != nil {
		return 0, fmt.Errorf("auth: no hay Android Keystore: %w", err)
	}
	if err := jni.CallVoidMethod(env, ks,
		jni.GetMethodID(env, claseKS, "load", "(Ljava/security/KeyStore$LoadStoreParameter;)V"), 0); err != nil {
		return 0, err
	}
	k, err := jni.CallObjectMethod(env, ks,
		jni.GetMethodID(env, claseKS, "getKey", "(Ljava/lang/String;[C)Ljava/security/Key;"),
		jni.Value(jni.JavaString(env, aliasClave)), 0)
	if err != nil {
		return 0, err
	}
	if k != 0 {
		return k, nil
	}
	return generarClave(env, clase)
}

// generarClave crea la clave en el Keystore, una vez en la vida de la instalación:
//
//	KeyGenerator g = KeyGenerator.getInstance("AES", "AndroidKeyStore");
//	g.init(new KeyGenParameterSpec.Builder(alias, PURPOSE_ENCRYPT | PURPOSE_DECRYPT)
//	    .setBlockModes("GCM").setEncryptionPaddings("NoPadding").setKeySize(256).build());
//	return g.generateKey();
func generarClave(env jni.Env, clase func(string) (jni.Class, error)) (jni.Object, error) {
	const builder = "android/security/keystore/KeyGenParameterSpec$Builder"
	claseBuilder, err := clase(builder)
	if err != nil {
		return 0, err
	}
	claseTexto, err := clase("java/lang/String")
	if err != nil {
		return 0, err
	}
	// 1|2 son PURPOSE_ENCRYPT y PURPOSE_DECRYPT de android.security.keystore.KeyProperties.
	// Por reflexión y no con jni.NewObject, que pasa mal los argumentos (sistema.Construir).
	propositos, err := sistema.Entero(env, 1|2)
	if err != nil {
		return 0, err
	}
	b, err := sistema.Construir(env, claseBuilder,
		[]jni.Object{sistema.Clase(env, "java/lang/String"), sistema.TipoInt(env)},
		jni.Object(jni.JavaString(env, aliasClave)), propositos)
	if err != nil {
		return 0, err
	}
	textos := func(v string) jni.Value {
		return jni.Value(jni.NewObjectArray(env, 1, claseTexto, jni.Object(jni.JavaString(env, v))))
	}
	for _, paso := range []struct {
		nombre, firma string
		arg           jni.Value
	}{
		{"setBlockModes", "([Ljava/lang/String;)L" + builder + ";", textos("GCM")},
		{"setEncryptionPaddings", "([Ljava/lang/String;)L" + builder + ";", textos("NoPadding")},
		{"setKeySize", "(I)L" + builder + ";", jni.Value(256)},
	} {
		if b, err = jni.CallObjectMethod(env, b,
			jni.GetMethodID(env, claseBuilder, paso.nombre, paso.firma), paso.arg); err != nil {
			return 0, fmt.Errorf("auth: KeyGenParameterSpec.%s: %w", paso.nombre, err)
		}
	}
	spec, err := jni.CallObjectMethod(env, b,
		jni.GetMethodID(env, claseBuilder, "build", "()Landroid/security/keystore/KeyGenParameterSpec;"))
	if err != nil {
		return 0, err
	}
	claseGen, err := clase("javax/crypto/KeyGenerator")
	if err != nil {
		return 0, err
	}
	g, err := jni.CallStaticObjectMethod(env, claseGen,
		jni.GetStaticMethodID(env, claseGen, "getInstance",
			"(Ljava/lang/String;Ljava/lang/String;)Ljavax/crypto/KeyGenerator;"),
		jni.Value(jni.JavaString(env, "AES")), jni.Value(jni.JavaString(env, "AndroidKeyStore")))
	if err != nil {
		return 0, err
	}
	if err := jni.CallVoidMethod(env, g,
		jni.GetMethodID(env, claseGen, "init", "(Ljava/security/spec/AlgorithmParameterSpec;)V"),
		jni.Value(spec)); err != nil {
		return 0, fmt.Errorf("auth: el Keystore no acepta la clave: %w", err)
	}
	return jni.CallObjectMethod(env, g,
		jni.GetMethodID(env, claseGen, "generateKey", "()Ljavax/crypto/SecretKey;"))
}

func nuevoCipher(env jni.Env, clase func(string) (jni.Class, error)) (jni.Class, jni.Object, error) {
	claseCipher, err := clase("javax/crypto/Cipher")
	if err != nil {
		return 0, 0, err
	}
	cipher, err := jni.CallStaticObjectMethod(env, claseCipher,
		jni.GetStaticMethodID(env, claseCipher, "getInstance", "(Ljava/lang/String;)Ljavax/crypto/Cipher;"),
		jni.Value(jni.JavaString(env, "AES/GCM/NoPadding")))
	return claseCipher, cipher, err
}

// cerrarSobre cifra y devuelve el IV que ha elegido GCM y el criptograma. El IV lo
// elige el proveedor a propósito: repetir uno con la misma clave rompe GCM.
func cerrarSobre(claro []byte) (iv, cifrado []byte, err error) {
	err = enAndroid(func(env jni.Env, clase func(string) (jni.Class, error)) error {
		k, err := clave(env, clase)
		if err != nil {
			return err
		}
		claseCipher, cipher, err := nuevoCipher(env, clase)
		if err != nil {
			return err
		}
		if err := jni.CallVoidMethod(env, cipher,
			jni.GetMethodID(env, claseCipher, "init", "(ILjava/security/Key;)V"),
			jni.Value(cifrar), jni.Value(k)); err != nil {
			return err
		}
		vector, err := jni.CallObjectMethod(env, cipher,
			jni.GetMethodID(env, claseCipher, "getIV", "()[B"))
		if err != nil {
			return err
		}
		salida, err := jni.CallObjectMethod(env, cipher,
			jni.GetMethodID(env, claseCipher, "doFinal", "([B)[B"),
			jni.Value(jni.NewByteArray(env, claro)))
		if err != nil {
			return err
		}
		iv = jni.GetByteArrayElements(env, jni.ByteArray(vector))
		cifrado = jni.GetByteArrayElements(env, jni.ByteArray(salida))
		return nil
	})
	return iv, cifrado, err
}

func abrirSobre(iv, cifrado []byte) ([]byte, error) {
	var claro []byte
	err := enAndroid(func(env jni.Env, clase func(string) (jni.Class, error)) error {
		k, err := clave(env, clase)
		if err != nil {
			return err
		}
		claseCipher, cipher, err := nuevoCipher(env, clase)
		if err != nil {
			return err
		}
		claseSpec, err := clase("javax/crypto/spec/GCMParameterSpec")
		if err != nil {
			return err
		}
		etiqueta, err := sistema.Entero(env, etiquetaGCMbit)
		if err != nil {
			return err
		}
		spec, err := sistema.Construir(env, claseSpec,
			[]jni.Object{sistema.TipoInt(env), sistema.Clase(env, "[B")},
			etiqueta, jni.Object(jni.NewByteArray(env, iv)))
		if err != nil {
			return err
		}
		if err := jni.CallVoidMethod(env, cipher,
			jni.GetMethodID(env, claseCipher, "init",
				"(ILjava/security/Key;Ljava/security/spec/AlgorithmParameterSpec;)V"),
			jni.Value(descifrar), jni.Value(k), jni.Value(spec)); err != nil {
			return err
		}
		// doFinal falla si alguien ha tocado el fichero: GCM autentica, no solo cifra.
		salida, err := jni.CallObjectMethod(env, cipher,
			jni.GetMethodID(env, claseCipher, "doFinal", "([B)[B"),
			jni.Value(jni.NewByteArray(env, cifrado)))
		if err != nil {
			return fmt.Errorf("auth: la sesión guardada no se deja descifrar: %w", err)
		}
		claro = jni.GetByteArrayElements(env, jni.ByteArray(salida))
		return nil
	})
	return claro, err
}
