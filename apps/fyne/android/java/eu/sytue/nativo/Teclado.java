// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.Context;
import android.os.Build;
import android.view.View;
import android.view.WindowInsets;

/**
 * Cuánto tapa el teclado en pantalla (`sesion.AltoTeclado`). Fyne no lo sabe: ni su
 * lienzo ni su zona interactiva encogen al salir el teclado (medido en el emulador,
 * 2026-09-28), y el campo en el que se escribe se quedaba debajo.
 */
public final class Teclado {
    private Teclado() {}

    /** Píxeles que tapa el teclado desde abajo de la pantalla; 0 sin teclado o antes de la API 30. */
    public static int alto(Context ctx) {
        if (Build.VERSION.SDK_INT < 30 || !(ctx instanceof Activity)) {
            return 0;
        }
        View decor = ((Activity) ctx).getWindow().getDecorView();
        WindowInsets insets = decor.getRootWindowInsets();
        if (insets == null || !insets.isVisible(WindowInsets.Type.ime())) {
            return 0;
        }
        return insets.getInsets(WindowInsets.Type.ime()).bottom;
    }
}
