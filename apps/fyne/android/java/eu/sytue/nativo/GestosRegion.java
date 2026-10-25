// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

/** Geometría de la única zona que puede reservar el menú; sin dependencias Android. */
final class GestosRegion {
    private GestosRegion() {}

    /** Estado de una ventana; sólo se confirma después de que el SO acepte el cambio. */
    static final class Aplicada {
        private boolean conocida;
        private int[] region;

        boolean distinta(int[] nueva) {
            return !conocida || !java.util.Arrays.equals(region, nueva);
        }

        void recordar(int[] nueva) {
            region = nueva == null ? null : nueva.clone();
            conocida = true;
        }

        void reiniciar() {
            conocida = false;
            region = null;
        }
    }

    /** Coordenadas físicas de ventana: izquierda, arriba, derecha, abajo, o null. */
    static int[] calcular(int ancho, int alto, int izquierda, int arriba, int derecha, int abajo, float densidad) {
        if (ancho <= 0 || alto <= 0 || izquierda < 0 || arriba < 0 || derecha < 0 || abajo < 0
                || !Float.isFinite(densidad) || densidad <= 0) {
            return null;
        }
        // Con navegación/cutout a la izquierda, el borde útil ya queda fuera de
        // Back. No reservar una franja sobre el sistema ni trasladarla a ciegas.
        if (izquierda > 0 || derecha >= ancho) {
            return null;
        }
        // Redondear hacia abajo nunca supera los presupuestos de 24 y 200 dp.
        int borde = (int) Math.min(ancho - derecha, Math.floor(24.0 * densidad));
        int inicio = Math.min(alto, arriba);
        int disponible = Math.max(0, alto - inicio - Math.min(alto - inicio, abajo));
        int banda = (int) Math.min(disponible, Math.floor(200.0 * densidad));
        if (borde == 0 || banda == 0) {
            return null;
        }
        return new int[] {0, inicio, borde, inicio + banda};
    }
}
