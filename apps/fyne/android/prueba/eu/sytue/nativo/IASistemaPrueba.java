// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import com.google.mlkit.genai.prompt.java.GenerativeModelFutures;

/**
 * El puente con Gemini Nano (IASistema) contra un ML Kit de mentira con la forma de la
 * Prompt API: que la reflexión encuentra el `object` de Kotlin, el `from` del Companion,
 * espera los futuros y saca el texto del primer candidato; y que un fallo de ML Kit llega
 * como su excepción, no como una de reflexión. En la JVM, antes de armar una APK.
 */
public final class IASistemaPrueba {
    private static int comprobaciones;

    private static void comprobar(boolean ok, String mensaje) {
        comprobaciones++;
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    public static void main(String[] args) throws Exception {
        String r = IASistema.responder(null, "SISTEMA", "hola");
        comprobar(("SISTEMA\n\nhola").equals(GenerativeModelFutures.ultimo), "el prompt no llega entero: " + GenerativeModelFutures.ultimo);
        comprobar(("eco: " + "SISTEMA\n\nhola".length()).equals(r), "respuesta: " + r);

        // Sin instrucciones, solo el texto.
        IASistema.responder(null, "", "solo");
        comprobar("solo".equals(GenerativeModelFutures.ultimo), "sin instrucciones: " + GenerativeModelFutures.ultimo);

        try {
            IASistema.responder(null, "", "FALLA");
            comprobar(false, "un fallo de ML Kit no puede ser una respuesta");
        } catch (IllegalStateException e) {
            comprobar(e.getMessage().contains("AICore"), "el error de ML Kit, tal cual: " + e);
        }

        // La descarga, con el aviso de ML Kit hecho Proxy: se pide una vez y no lanza nada.
        IASistema.descargar(null);
        comprobar(GenerativeModelFutures.descargas == 1, "descargar() no llama a download(DownloadCallback)");

        comprobar("".equals(IASistema.textoDe(null)), "sin respuesta, texto vacío");
        comprobar(IASistema.metodo(Object.class, "equals", (Object) null) != null, "un nulo casa con un objeto");
        comprobar(IASistema.metodo(String.class, "charAt", 1) != null, "un int casa con un Integer");
        System.out.println("IASistemaPrueba: " + comprobaciones + " comprobaciones");
    }
}
