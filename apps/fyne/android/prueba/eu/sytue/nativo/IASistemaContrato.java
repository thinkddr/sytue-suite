// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.lang.reflect.ParameterizedType;
import java.lang.reflect.Proxy;
import java.lang.reflect.Type;
import java.util.List;
import java.util.concurrent.Future;

/**
 * IASistema contra la Prompt API DE VERDAD (los JAR de android/mlkit/dependencias.tsv), en la
 * JVM y sin inicializar ninguna clase de ML Kit: que cada método que el puente busca por
 * reflexión existe con la forma que espera, y que {@link IASistema#metodo} lo encuentra con
 * argumentos del tipo real. IASistemaPrueba ejercita el puente contra un ML Kit de mentira;
 * esta ata la mentira a la versión fijada. La corre `empaquetar.sh android` con ML Kit.
 */
public final class IASistemaContrato {
    private static int comprobaciones;

    private static void comprobar(boolean ok, String mensaje) {
        comprobaciones++;
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    private static Class<?> clase(String nombre) throws ClassNotFoundException {
        return Class.forName(nombre, false, IASistemaContrato.class.getClassLoader());
    }

    /** Un objeto de la interfaz `c` que no hace nada (como el aviso de descarga del puente). */
    private static Object nada(Class<?> c) {
        return Proxy.newProxyInstance(c.getClassLoader(), new Class<?>[] {c}, (p, m, a) -> null);
    }

    /** El tipo T de un `Future<T>` o `List<T>` devuelto por `m`. */
    private static Class<?> argumento(Method m) {
        Type t = m.getGenericReturnType();
        comprobar(t instanceof ParameterizedType, m + " no dice qué devuelve");
        Type a = ((ParameterizedType) t).getActualTypeArguments()[0];
        comprobar(a instanceof Class, m + " devuelve un tipo que no es una clase: " + a);
        return (Class<?>) a;
    }

    public static void main(String[] args) throws Exception {
        // Generation es un `object` de Kotlin: getClient() de instancia, por su INSTANCE.
        Class<?> gen = clase("com.google.mlkit.genai.prompt.Generation");
        Method cliente = IASistema.metodo(gen, "getClient");
        comprobar(cliente != null, "Generation.getClient() sin argumentos no existe");
        if (!Modifier.isStatic(cliente.getModifiers())) {
            Field instancia = gen.getField("INSTANCE");
            comprobar(Modifier.isStatic(instancia.getModifiers()) && instancia.getType() == gen,
                    "getClient() es de instancia y Generation no tiene su INSTANCE");
        }
        Class<?> modelo = cliente.getReturnType();
        comprobar(modelo.isInterface(), "getClient() ya no devuelve la interfaz GenerativeModel: " + modelo);

        // GenerativeModelFutures.from(modelo), estático (o por su Companion).
        Class<?> fut = clase("com.google.mlkit.genai.prompt.java.GenerativeModelFutures");
        Method from = IASistema.metodo(fut, "from", nada(modelo));
        comprobar(from != null, "GenerativeModelFutures.from(GenerativeModel) no existe");
        comprobar(Modifier.isStatic(from.getModifiers()) || fut.getField("Companion") != null, "from no es estático ni tiene Companion");
        comprobar(fut.isAssignableFrom(from.getReturnType()), "from no devuelve un GenerativeModelFutures");

        // checkStatus() -> Future<Integer> con los valores de FeatureStatus que traduce estado().
        Method estado = IASistema.metodo(fut, "checkStatus");
        comprobar(estado != null && Future.class.isAssignableFrom(estado.getReturnType()), "checkStatus() no devuelve un Future");
        comprobar(argumento(estado) == Integer.class, "checkStatus() ya no da un Integer");
        Class<?> fs = clase("com.google.mlkit.genai.common.FeatureStatus");
        String[] nombres = {"UNAVAILABLE", "DOWNLOADABLE", "DOWNLOADING", "AVAILABLE"};
        for (int i = 0; i < nombres.length; i++) {
            comprobar(fs.getField(nombres[i]).getInt(null) == i, "FeatureStatus." + nombres[i] + " ya no vale " + i);
        }

        // generateContent(String) -> Future<GenerateContentResponse>, y su texto.
        Method generar = IASistema.metodo(fut, "generateContent", "hola");
        comprobar(generar != null && Future.class.isAssignableFrom(generar.getReturnType()), "generateContent(String) no devuelve un Future");
        Class<?> respuesta = argumento(generar);
        Method candidatos = IASistema.metodo(respuesta, "getCandidates");
        comprobar(candidatos != null && List.class.isAssignableFrom(candidatos.getReturnType()), "la respuesta ya no tiene getCandidates()");
        Method texto = IASistema.metodo(argumento(candidatos), "getText");
        comprobar(texto != null && texto.getReturnType() == String.class, "un candidato ya no tiene getText()");

        // download(DownloadCallback), con un Proxy de la interfaz como el de descargar().
        Class<?> aviso = clase("com.google.mlkit.genai.common.DownloadCallback");
        comprobar(aviso.isInterface(), "DownloadCallback ya no es una interfaz: el Proxy de descargar() no vale");
        comprobar(IASistema.metodo(fut, "download", nada(aviso)) != null, "download(DownloadCallback) no existe");

        System.out.println("IASistemaContrato: " + comprobaciones + " comprobaciones contra ML Kit de verdad");
    }
}
