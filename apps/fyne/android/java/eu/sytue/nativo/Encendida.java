// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.Context;
import android.view.WindowManager;

/**
 * Que la pantalla no se apague mientras se ve un vídeo o se emite
 * (`sesion.PantallaEncendida`). En el hilo de la interfaz: el `SetDisableScreenBlanking`
 * de Fyne llama a `Window.addFlags` desde el hilo de Go, Android lo rechaza con
 * CalledFromWrongThreadException y la excepción se queda colgada en JNI sin que nadie la
 * vea (medido en el emulador, 2026-09-28: el indicador nunca llegaba a la ventana).
 */
public final class Encendida {
    private Encendida() {}

    public static void poner(Context ctx, final boolean si) {
        if (!(ctx instanceof Activity)) {
            return;
        }
        final Activity a = (Activity) ctx;
        a.runOnUiThread(() -> {
            if (si) {
                a.getWindow().addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            } else {
                a.getWindow().clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON);
            }
        });
    }
}
