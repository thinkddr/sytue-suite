// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

/**
 * Las reglas de la tarea en segundo plano (TareaFondo), sin Android: se prueban en la JVM
 * del equipo (TareaReglasPrueba), como RefrescoReglas.
 */
final class TareaReglas {
    /** El periódico, de respaldo: cada seis horas con una de holgura. */
    static final long CADA_MS = 6L * 60 * 60 * 1000;
    static final long HOLGURA_MS = 60L * 60 * 1000;
    /** Por fotos nuevas: se espera a que la ráfaga acabe (30 s de calma), y no más de 10 min. */
    static final long CALMA_MS = 30L * 1000;
    static final long COMO_MUCHO_MS = 10L * 60 * 1000;
    /** Lo aplazado o fallido se reintenta a los 5 min (lineal). */
    static final long REINTENTO_MS = 5L * 60 * 1000;

    private TareaReglas() {}

    /**
     * Con el dispositivo bloqueado no se corre: la sesión de la suite no se abre con la
     * pantalla bloqueada (SesionProveedor), igual que en el refresco del widget.
     */
    static boolean correr(boolean bloqueado) {
        return !bloqueado;
    }

    /** Si Android debe reintentar ESTE trabajo: el de fotos nuevas, aplazado o fallido. */
    static boolean reintentar(int resultado, boolean porContenido) {
        return porContenido && (resultado == RefrescoReglas.APLAZADO || resultado == RefrescoReglas.FALLO);
    }
}
