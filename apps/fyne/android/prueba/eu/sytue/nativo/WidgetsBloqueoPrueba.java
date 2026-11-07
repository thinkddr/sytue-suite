// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

/**
 * Qué hace el receptor con el dispositivo bloqueado (WidgetSnapshot.pintado): en la
 * pantalla de inicio conserva la última imagen, en la de bloqueo la deja en blanco.
 */
public final class WidgetsBloqueoPrueba {
    private static int checks;

    private static void check(boolean ok, String message) {
        checks++;
        if (!ok) {
            throw new AssertionError(message);
        }
    }

    public static void main(String[] args) {
        final int inicio = 1; // AppWidgetProviderInfo.WIDGET_CATEGORY_HOME_SCREEN
        final int busqueda = 4; // WIDGET_CATEGORY_SEARCHBOX
        check(WidgetSnapshot.pintado(false, inicio) == WidgetSnapshot.Pintado.PINTAR, "unlocked home screen paints");
        check(WidgetSnapshot.pintado(false, WidgetSnapshot.CATEGORIA_KEYGUARD) == WidgetSnapshot.Pintado.PINTAR,
                "unlocked keyguard host paints");
        check(WidgetSnapshot.pintado(true, inicio) == WidgetSnapshot.Pintado.CONSERVAR,
                "locked home screen keeps the last image instead of blanking it");
        check(WidgetSnapshot.pintado(true, busqueda) == WidgetSnapshot.Pintado.CONSERVAR, "locked search box keeps it");
        check(WidgetSnapshot.pintado(true, WidgetSnapshot.CATEGORIA_KEYGUARD) == WidgetSnapshot.Pintado.EN_BLANCO,
                "a lock screen host is blanked while locked");
        check(WidgetSnapshot.pintado(true, inicio | WidgetSnapshot.CATEGORIA_KEYGUARD) == WidgetSnapshot.Pintado.EN_BLANCO,
                "any keyguard bit is enough to blank it");
        check(WidgetSnapshot.CATEGORIA_KEYGUARD == 2, "the Android KEYGUARD category value");
        System.out.println("Widget lock: " + checks + " checks passed");
    }
}
