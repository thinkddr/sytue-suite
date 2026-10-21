// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.Context;
import android.os.Build;
import android.window.OnBackInvokedCallback;
import android.window.OnBackInvokedDispatcher;

/**
 * El botón «atrás» con la navegación predictiva de Android 16 (`sesion/atras_android.go`).
 *
 * Una app con target 36 en Android 16 ya no recibe KEYCODE_BACK: el sistema lo gestiona
 * él y la saca de la pantalla. Fyne (NativeActivity) solo sabía de la tecla, así que
 * «atrás» cerraba la app desde cualquier página —desde un vídeo, desde un diálogo
 * abierto— en vez de volver (visto en el emulador de la API 36, 2026-09-28). Aquí se
 * registra el receptor de la API 33 y se le pasa a Go, que lo reparte como la tecla.
 * Por debajo de la API 33 sigue llegando la tecla y esto no hace nada.
 */
public final class Atras {
    private static OnBackInvokedCallback receptor;

    private Atras() {}

    /** Lo implementa Go: «atrás», en el hilo de la interfaz de Android. */
    static native void pulsado();

    public static void registrar(Context ctx) {
        if (Build.VERSION.SDK_INT < 33 || !(ctx instanceof Activity)) {
            return;
        }
        final Activity a = (Activity) ctx;
        a.runOnUiThread(() -> {
            if (receptor != null) {
                a.getOnBackInvokedDispatcher().unregisterOnBackInvokedCallback(receptor);
            }
            receptor = Atras::pulsado;
            a.getOnBackInvokedDispatcher().registerOnBackInvokedCallback(
                    OnBackInvokedDispatcher.PRIORITY_DEFAULT, receptor);
        });
    }
}
