// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import android.os.Build;
import java.lang.reflect.Field;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;
import java.lang.reflect.Modifier;
import java.lang.reflect.Proxy;
import java.util.List;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

/**
 * La IA del sistema de Android para Sytue Office (`apps/nucleo/local/asistente/sistema_android.go`):
 * Gemini Nano en el dispositivo por la Prompt API de ML Kit GenAI
 * (`com.google.mlkit:genai-prompt`), que habla con AICore. Nada sale del teléfono.
 *
 * POR REFLEXIÓN, Y NO CON LAS CLASES DE ML KIT: la APK la arma `empaquetar.sh` con javac
 * contra el `android.jar` y nada más (sin Gradle ni AAR), así que estas clases tienen que
 * compilar sin la biblioteca. La APK de Sytue Office la lleva dexada detrás de estas clases
 * (`android-mlkit.py`, salvo con SYTUE_MLKIT=no); si no la lleva, o el teléfono no tiene
 * AICore, {@link #estado} lo dice y Go devuelve `ErrSinIANativa`. Que cada método que se
 * busca aquí exista en la versión fijada lo comprueba `IASistemaContrato` al empaquetar.
 *
 * Go llama desde un hilo suyo, enganchado a la máquina, y aquí se ESPERA a los futuros
 * de ML Kit (como {@link Captura#fotografiar}): nada llama a Go desde aquí.
 */
public final class IASistema {
    private IASistema() {}

    // Los estados que entiende Go (sistema_android.go). Los cuatro primeros son los de
    // `com.google.mlkit.genai.common.FeatureStatus` traducidos; el resto, de aquí.
    static final int DISPONIBLE = 0;
    static final int DESCARGABLE = 1;
    static final int DESCARGANDO = 2;
    static final int NO_DISPONIBLE = 3;
    static final int SIN_BIBLIOTECA = 4;
    static final int ANDROID_ANTIGUO = 5;
    static final int FALLO = 6;

    private static final String GENERACION = "com.google.mlkit.genai.prompt.Generation";
    private static final String FUTUROS = "com.google.mlkit.genai.prompt.java.GenerativeModelFutures";
    private static final String AVISO_DESCARGA = "com.google.mlkit.genai.common.DownloadCallback";
    /** Lo que se espera a que AICore diga si hay modelo: Ajustes no puede quedarse colgado. */
    private static final long PLAZO_ESTADO_S = 5;
    /** Lo que se espera a una respuesta; Go además deja de esperar si se cancela. */
    private static final long PLAZO_RESPUESTA_S = 120;

    private static Object futuros;

    /** Si hay modelo en el dispositivo, con los valores de arriba. */
    public static int estado(Context ctx) {
        if (Build.VERSION.SDK_INT < 26) {
            return ANDROID_ANTIGUO; // lo mínimo de ML Kit GenAI
        }
        Object f;
        try {
            f = cliente();
        } catch (ClassNotFoundException e) {
            return SIN_BIBLIOTECA;
        } catch (Throwable e) {
            return FALLO;
        }
        try {
            Object st = esperar(llamar(f, "checkStatus"), PLAZO_ESTADO_S);
            if (!(st instanceof Integer)) {
                return FALLO;
            }
            switch ((Integer) st) {
                case 3: // FeatureStatus.AVAILABLE
                    return DISPONIBLE;
                case 1: // DOWNLOADABLE
                    return DESCARGABLE;
                case 2: // DOWNLOADING
                    return DESCARGANDO;
                default: // UNAVAILABLE
                    return NO_DISPONIBLE;
            }
        } catch (Throwable e) {
            return FALLO;
        }
    }

    /**
     * Pide al modelo que responda. `instrucciones` es el prompt de sistema de la suite (el
     * de `ia/proveedor`, con el contexto como datos delimitados) y `texto`, el mensaje.
     * La Prompt API de Nano no separa el prompt de sistema: van juntos, el de sistema
     * primero. Lanza la excepción de ML Kit tal cual, y Go la enseña.
     */
    public static String responder(Context ctx, String instrucciones, String texto) throws Exception {
        Object f = cliente();
        String prompt = instrucciones == null || instrucciones.isEmpty() ? texto : instrucciones + "\n\n" + texto;
        Object respuesta = esperar(llamar(f, "generateContent", prompt), PLAZO_RESPUESTA_S);
        return textoDe(respuesta);
    }

    /**
     * Pide a AICore que descargue el modelo si es descargable; no espera a que acabe.
     * `download(DownloadCallback)`: la interfaz trae todos sus métodos por defecto, así que
     * basta un Proxy que no haga nada (estado() dirá «descargando» y luego «disponible»).
     */
    public static void descargar(Context ctx) {
        try {
            Object f = cliente();
            Class<?> aviso = Class.forName(AVISO_DESCARGA);
            Object nada = Proxy.newProxyInstance(aviso.getClassLoader(), new Class<?>[] {aviso}, (p, m, a) -> {
                switch (m.getName()) {
                    case "hashCode":
                        return System.identityHashCode(p);
                    case "equals":
                        return a != null && a.length == 1 && p == a[0];
                    case "toString":
                        return "IASistema.descargar";
                    default:
                        return null;
                }
            });
            llamar(f, "download", nada);
        } catch (Throwable ignorada) {
            // Sin biblioteca o sin AICore: estado() ya lo dice.
        }
    }

    // --- Reflexión ------------------------------------------------------------------

    /** El GenerativeModelFutures de Gemini Nano, una vez por proceso. */
    private static synchronized Object cliente() throws Exception {
        if (futuros != null) {
            return futuros;
        }
        Class<?> gen = Class.forName(GENERACION);
        // `Generation` es un `object` de Kotlin: getClient() estático con @JvmStatic, o
        // por su INSTANCE.
        Object modelo = estatico(gen, "getClient");
        Class<?> fut = Class.forName(FUTUROS);
        // `GenerativeModelFutures.from(modelo)`: estático, o por su Companion.
        futuros = estatico(fut, "from", modelo);
        return futuros;
    }

    private static Object estatico(Class<?> cls, String nombre, Object... args) throws Exception {
        Method m = metodo(cls, nombre, args);
        if (m != null && Modifier.isStatic(m.getModifiers())) {
            return invocar(m, null, args);
        }
        for (String campo : new String[] {"INSTANCE", "Companion"}) {
            try {
                Field fl = cls.getField(campo);
                Object o = fl.get(null);
                if (o != null) {
                    Method mm = metodo(o.getClass(), nombre, args);
                    if (mm != null) {
                        return invocar(mm, o, args);
                    }
                }
            } catch (NoSuchFieldException ignorada) {
                // el siguiente
            }
        }
        throw new NoSuchMethodException(cls.getName() + "." + nombre);
    }

    private static Object llamar(Object o, String nombre, Object... args) throws Exception {
        Method m = metodo(o.getClass(), nombre, args);
        if (m == null) {
            throw new NoSuchMethodException(o.getClass().getName() + "." + nombre);
        }
        return invocar(m, o, args);
    }

    /** El método público `nombre` con tantos parámetros como args y de tipos que casan. */
    static Method metodo(Class<?> cls, String nombre, Object... args) {
        for (Method m : cls.getMethods()) {
            Class<?>[] ps = m.getParameterTypes();
            if (!m.getName().equals(nombre) || ps.length != args.length) {
                continue;
            }
            boolean casa = true;
            for (int i = 0; i < ps.length && casa; i++) {
                casa = args[i] == null ? !ps[i].isPrimitive() : envoltorio(ps[i]).isInstance(args[i]);
            }
            if (casa) {
                return m;
            }
        }
        return null;
    }

    private static Class<?> envoltorio(Class<?> c) {
        if (!c.isPrimitive()) {
            return c;
        }
        if (c == int.class) {
            return Integer.class;
        }
        if (c == long.class) {
            return Long.class;
        }
        if (c == boolean.class) {
            return Boolean.class;
        }
        return c;
    }

    private static Object invocar(Method m, Object o, Object... args) throws Exception {
        try {
            return m.invoke(o, args);
        } catch (InvocationTargetException e) {
            Throwable c = e.getCause();
            if (c instanceof Exception) {
                throw (Exception) c;
            }
            throw e;
        }
    }

    /** Espera un ListenableFuture (que es un Future) o devuelve el valor si no lo es. */
    private static Object esperar(Object o, long segundos) throws Exception {
        if (o instanceof Future) {
            try {
                return ((Future<?>) o).get(segundos, TimeUnit.SECONDS);
            } catch (java.util.concurrent.ExecutionException e) {
                Throwable c = e.getCause();
                if (c instanceof Exception) {
                    throw (Exception) c;
                }
                throw e;
            }
        }
        return o;
    }

    /** El texto del primer candidato de un GenerateContentResponse. */
    static String textoDe(Object respuesta) throws Exception {
        if (respuesta == null) {
            return "";
        }
        if (respuesta instanceof CharSequence) {
            return respuesta.toString();
        }
        Object cs = llamar(respuesta, "getCandidates");
        if (cs instanceof List && !((List<?>) cs).isEmpty()) {
            Object t = llamar(((List<?>) cs).get(0), "getText");
            return t == null ? "" : t.toString();
        }
        return "";
    }
}
