// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Arrays;

/** Presupuestos de navegación comprobados en la JVM, antes de producir una APK. */
public final class GestosRegionPrueba {
    private static int comprobaciones;

    private static void comprobar(boolean ok, String mensaje) {
        comprobaciones++;
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    private static void region(int[] esperada, int ancho, int alto, int arriba, int abajo, float densidad) {
        comprobar(Arrays.equals(esperada, GestosRegion.calcular(ancho, alto, 0, arriba, 0, abajo, densidad)),
                "región fuera del presupuesto: " + ancho + "×" + alto + " @" + densidad);
    }

    private static final class Ventana {
        final GestosRegion.Aplicada aplicada = new GestosRegion.Aplicada();
        int llamadasSO;

        void poner(int[] region) {
            if (aplicada.distinta(region)) {
                llamadasSO++;
                aplicada.recordar(region);
            }
        }
    }

    public static void main(String[] args) {
        // Teléfono con notch e indicador Home; sólo se reserva cerca del menú.
        region(new int[] {0, 90, 72, 690}, 1080, 2400, 90, 96, 3);
        // Paisaje y pantalla partida: no se supera el alto visible disponible.
        region(new int[] {0, 24, 48, 176}, 1600, 200, 24, 24, 2);
        region(new int[] {0, 0, 12, 80}, 12, 80, 0, 0, 1);
        region(new int[] {0, 30, 31, 296}, 1080, 2400, 30, 40, 1.333f);
        // La barra/cutout lateral no es un borde útil del lienzo de Fyne.
        comprobar(GestosRegion.calcular(1600, 900, 90, 0, 0, 0, 3) == null,
                "sin exclusión sobre cutout o navegación a la izquierda");
        comprobar(GestosRegion.calcular(1600, 900, 1, 0, 0, 0, 3) == null,
                "cualquier inset izquierdo conserva Back sin trasladar la región");
        comprobar(Arrays.equals(new int[] {0, 0, 72, 600},
                GestosRegion.calcular(1600, 900, 0, 0, 90, 0, 3)),
                "navegación a la derecha no desplaza la franja izquierda");
        comprobar(Arrays.equals(new int[] {0, 0, 10, 80},
                GestosRegion.calcular(100, 80, 0, 0, 90, 0, 3)), "banda acotada al ancho útil");
        comprobar(GestosRegion.calcular(100, 80, 0, 0, 100, 0, 3) == null, "sin área horizontal útil");
        comprobar(GestosRegion.calcular(100, 80, -1, 0, 0, 0, 3) == null, "inset izquierdo inválido");
        comprobar(GestosRegion.calcular(100, 80, 0, 0, -1, 0, 3) == null, "inset derecho inválido");
        // Una pantalla sin área segura no debe tener exclusión alguna.
        region(null, 1080, 200, 201, 0, 3);
        region(null, 1080, 200, 100, 100, 3);
        region(null, 1080, 200, 0, Integer.MAX_VALUE, 3);
        region(null, 0, 800, 0, 0, 1);
        region(null, 800, 0, 0, 0, 1);
        region(null, 800, 800, -1, 0, 1);
        region(null, 800, 800, 0, -1, 1);
        for (float densidad : new float[] {0, -1, Float.NaN, Float.POSITIVE_INFINITY, Float.NEGATIVE_INFINITY, 0.001f}) {
            region(null, 800, 800, 0, 0, densidad);
        }
        for (float densidad : new float[] {0.75f, 1, 1.25f, 1.333f, 2, 2.625f, 3.5f, 4}) {
            for (int alto : new int[] {250, 800, 2400}) {
                int[] r = GestosRegion.calcular(1080, alto, 0, 24, 0, 32, densidad);
                comprobar(r != null && r[0] == 0 && r[1] == 24, "anclaje junto al acceso al menú");
                comprobar(r[2] > 0 && r[2] <= 24 * densidad, "borde de 24 dp como máximo");
                comprobar(r[3] > r[1] && r[3] - r[1] <= 200 * densidad, "banda de 200 dp como máximo");
                comprobar(r[3] <= alto - 32 && r[2] < 1080, "Back fuera de la banda y Home preservados");
            }
        }
        // Evita overflow al acotar geometrías nativas dañadas o extremas.
        region(new int[] {0, 1, Integer.MAX_VALUE, Integer.MAX_VALUE - 1}, Integer.MAX_VALUE,
                Integer.MAX_VALUE, 1, 1, Float.MAX_VALUE);
        Ventana ventana = new Ventana();
        ventana.poner(null);
        ventana.poner(null);
        comprobar(ventana.llamadasSO == 1, "región vacía inicial aplicada una sola vez");
        int[] menu = new int[] {0, 30, 72, 630};
        ventana.poner(menu);
        ventana.poner(new int[] {0, 30, 72, 630});
        comprobar(ventana.llamadasSO == 2, "layouts idénticos no vuelven a llamar al SO");
        menu[1] = 40;
        comprobar(!ventana.aplicada.distinta(new int[] {0, 30, 72, 630}), "la caché no conserva arrays mutables ajenos");
        ventana.poner(menu);
        comprobar(ventana.llamadasSO == 3, "giro o cambio de insets actualiza la región");
        ventana.poner(null);
        ventana.poner(null);
        comprobar(ventana.llamadasSO == 4, "pausa o modal retiran la región una sola vez");
        ventana.aplicada.reiniciar();
        ventana.poner(null);
        comprobar(ventana.llamadasSO == 5, "actividad nueva recibe su región vacía aunque coincida");
        ventana.poner(menu);
        ventana.aplicada.reiniciar();
        ventana.poner(menu);
        comprobar(ventana.llamadasSO == 7, "una actividad recreada recibe su propio rectángulo");
        GestosRegion.Aplicada pendiente = new GestosRegion.Aplicada();
        comprobar(pendiente.distinta(menu) && pendiente.distinta(menu), "un cambio fallido del SO no se memoriza");
        System.out.println("Gestos: " + comprobaciones + " comprobaciones de navegación superadas");
    }
}
