// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

//go:build android

package sistema

import "git.wow.st/gmp/jni"

const claseSesionCompartida = "eu.sytue.nativo.SesionCompartida"

// SesionAndroidHabilitada consulta el catálogo incorporado a la APK. No hay una
// preferencia local ni un flag del servidor que pueda ampliar la confianza.
func SesionAndroidHabilitada() (bool, error) {
	var habilitada bool
	err := Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		habilitada, err = jni.CallStaticBooleanMethod(env, clase,
			jni.GetStaticMethodID(env, clase, "habilitada", "()Z"))
		return err
	})
	return habilitada, err
}

func sesionAndroidTexto(metodo, firma string, textos ...string) (string, error) {
	var resultado string
	err := Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		argumentos := []jni.Value{jni.Value(ctx)}
		for _, texto := range textos {
			argumentos = append(argumentos, jni.Value(jni.JavaString(env, texto)))
		}
		valor, err := jni.CallStaticObjectMethod(env, clase,
			jni.GetStaticMethodID(env, clase, metodo, firma), argumentos...)
		if err == nil && valor != 0 {
			resultado = jni.GoString(env, jni.String(valor))
		}
		return err
	})
	return resultado, err
}

// EstadoSesionAndroid devuelve sólo generación y bloqueo, nunca una identidad o token.
func EstadoSesionAndroid() (string, error) {
	return sesionAndroidTexto("estado", "(Landroid/content/Context;)Ljava/lang/String;")
}

func CargarSesionAndroid(cuenta, emisor, cliente string) (string, error) {
	return sesionAndroidTexto("cargar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)Ljava/lang/String;",
		cuenta, emisor, cliente)
}

func GuardarSesionAndroid(cuenta, emisor, cliente, datos string) error {
	return Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		return jni.CallStaticVoidMethod(env, clase,
			jni.GetStaticMethodID(env, clase, "guardar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, cuenta)), jni.Value(jni.JavaString(env, emisor)),
			jni.Value(jni.JavaString(env, cliente)), jni.Value(jni.JavaString(env, datos)))
	})
}

// CerrojoSesionAndroid transfiere un descriptor del mismo inode privado del broker.
// El llamante lo cierra; la exclusión la sigue haciendo flock en ajustes.
func CerrojoSesionAndroid() (int, error) {
	fd := -1
	err := Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		fd, err = jni.CallStaticIntMethod(env, clase,
			jni.GetStaticMethodID(env, clase, "cerrojo", "(Landroid/content/Context;)I"), jni.Value(ctx))
		return err
	})
	return fd, err
}

func BloquearSesionAndroid(puesto bool) error {
	return Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		valor := jni.Value(0)
		if puesto {
			valor = 1
		}
		return jni.CallStaticVoidMethod(env, clase,
			jni.GetStaticMethodID(env, clase, "bloqueo", "(Landroid/content/Context;Z)V"), jni.Value(ctx), valor)
	})
}

func MigrarSesionAndroid(cuenta, emisor, cliente string, historica bool) error {
	return Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		valor := jni.Value(0)
		if historica {
			valor = 1
		}
		return jni.CallStaticVoidMethod(env, clase,
			jni.GetStaticMethodID(env, clase, "migrar", "(Landroid/content/Context;Ljava/lang/String;Ljava/lang/String;Ljava/lang/String;Z)V"),
			jni.Value(ctx), jni.Value(jni.JavaString(env, cuenta)), jni.Value(jni.JavaString(env, emisor)),
			jni.Value(jni.JavaString(env, cliente)), valor)
	})
}

// PonerDriveSistemaAndroid changes only the broker's typed Drive preference.
func PonerDriveSistemaAndroid(habilitado bool) error {
	activa, err := SesionAndroidHabilitada()
	if err != nil || !activa {
		return err
	}
	return Java(func(env jni.Env, ctx jni.Object) error {
		clase, err := CargarClase(env, ctx, claseSesionCompartida)
		if err != nil {
			return err
		}
		valor := jni.Value(0)
		if habilitado {
			valor = 1
		}
		return jni.CallStaticVoidMethod(env, clase,
			jni.GetStaticMethodID(env, clase, "drive", "(Landroid/content/Context;Z)V"), jni.Value(ctx), valor)
	})
}
