// SPDX-License-Identifier: Apache-2.0
//go:build android

package sistema

import "git.wow.st/gmp/jni"

// CapacidadTrabajoSesionAndroid distinguishes a known old broker from an IPC failure.
func CapacidadTrabajoSesionAndroid() (int, error) {
	version := 0
	err := Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		version, err = jni.CallStaticIntMethod(env, clase, jni.GetStaticMethodID(env, clase, "capacidadTrabajo", "(Landroid/content/Context;)I"), jni.Value(ctx))
		return err
	})
	return version, err
}

// TrabajoSesionAndroid carries the one typed native work protocol, without tokens.
func TrabajoSesionAndroid(accion, datos string) (string, error) {
	return sesionAndroidTexto("trabajo", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;", accion, datos)
}

// CerrojoTrabajoSesionAndroid returns a newly opened description, never a cached duplicate.
func CerrojoTrabajoSesionAndroid(datos string) (int, error) {
	fd := -1
	err := Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		fd, err = jni.CallStaticIntMethod(env, clase, jni.GetStaticMethodID(env, clase, "cerrojoTrabajo", "(Landroid/content/Context;Ljava/lang/String;)I"), jni.Value(ctx), jni.Value(jni.JavaString(env, datos)))
		return err
	})
	return fd, err
}
