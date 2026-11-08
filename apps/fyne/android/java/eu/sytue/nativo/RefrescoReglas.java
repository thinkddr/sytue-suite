// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Set;
import java.util.regex.Pattern;

/**
 * Las reglas del refresco del widget en segundo plano (RefrescoWidgets) y del relevo de
 * One (RelevoWidgets), sin Android: se prueban en la JVM del equipo
 * (RefrescoReglasPrueba), como WidgetSnapshot.
 */
final class RefrescoReglas {
    /** Lo que devuelve la entrada JNI: el Resultado de apps/fyne/sesion (fondo.go). */
    static final int SIN_CAMBIOS = 0;
    static final int NUEVOS = 1;
    static final int FALLO = 2;
    static final int APLAZADO = 3;

    /** El periódico: cada hora, con 15 min de holgura para que Android lo agrupe. */
    static final long CADA_MS = 60L * 60 * 1000;
    static final long HOLGURA_MS = 15L * 60 * 1000;
    /** El relevo: como mucho uno por minuto y app, por mucho que cambie en el servidor. */
    static final long MIN_RELEVO_MS = 60L * 1000;
    /** Con el teléfono bloqueado, el relevo se vuelve a intentar a los 5 min (lineal). */
    static final long REINTENTO_MS = 5L * 60 * 1000;

    /** El paquete de One, la única app que releva (la que mantiene el SSE abierto). */
    static final String ONE = "eu.sytue.one";

    private static final Pattern PAQUETE = Pattern.compile("eu\\.sytue\\.[a-z][a-z0-9]*");

    enum Paso {
        /** Esta compilación no refresca en Go: solo se repinta (rótulo y caducidad). */
        REPINTAR,
        /** Con el dispositivo bloqueado no se lee nada (como el receptor del widget). */
        APLAZAR,
        /** Se carga la biblioteca y lee la app (RefrescarSinVentana). */
        REFRESCAR,
    }

    private RefrescoReglas() {}

    static Paso paso(boolean conGo, boolean bloqueado) {
        if (!conGo) {
            return Paso.REPINTAR;
        }
        return bloqueado ? Paso.APLAZAR : Paso.REFRESCAR;
    }

    /** Cuánto esperar al siguiente relevo: lo que falte para el minuto desde el último. */
    static long espera(long ultimo, long ahora) {
        long falta = ultimo + MIN_RELEVO_MS - ahora;
        if (falta < 0) {
            return 0;
        }
        // Un reloj que va hacia atrás no deja el relevo esperando horas.
        return Math.min(falta, MIN_RELEVO_MS);
    }

    /** Si Android debe volver a intentarlo: solo el relevo, y solo si se aplazó. */
    static boolean reintentar(int resultado, boolean relevo) {
        return relevo && resultado == APLAZADO;
    }

    /** One solo releva a otra app de la suite, del catálogo fijado, que no sea ella misma. */
    static boolean destinoValido(String paquete, String propio, Set<String> conocidos) {
        return paquete != null && PAQUETE.matcher(paquete).matches() && !paquete.equals(propio)
                && !paquete.equals(ONE) && conocidos != null && conocidos.contains(paquete);
    }

    /** El relevo solo se acepta de One (el llamante ya verificado contra su firma fijada). */
    static boolean llamanteValido(String llamante, String propio) {
        return ONE.equals(llamante) && !ONE.equals(propio);
    }
}
