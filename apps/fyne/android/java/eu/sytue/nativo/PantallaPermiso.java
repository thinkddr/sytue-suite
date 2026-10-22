// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.app.Fragment;
import android.content.Intent;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.Bundle;

/**
 * El fragmento sin vista que abre el diálogo de Android para compartir la pantalla:
 * `createScreenCaptureIntent` solo devuelve su respuesta a una actividad o a un fragmento
 * suyo. Actividad propia no puede ser: la CLI de Fyne solo empaqueta una, la
 * GoNativeActivity («can only build an .apk for GoNativeActivity», Codemagic v3.7.10), y
 * esa no ve `onActivityResult`. Un fragmento pegado a ella sí: Android le entrega el
 * resultado a él. Con el sí, pasa el permiso al servicio en primer plano
 * ({@link PantallaServicio}) y se quita.
 *
 * `android.app.Fragment` está en desuso desde la API 28 en favor del de AndroidX, que la
 * APK no lleva; sigue en la API 36 y es la única forma sin AndroidX.
 */
@SuppressWarnings("deprecation")
public final class PantallaPermiso extends Fragment {
    private static final int PEDIR = 7416;
    private static final String AVISO = "aviso";

    /** Android recrea los fragmentos por este constructor: tiene que existir y ser público. */
    public PantallaPermiso() {}

    /** Pega el fragmento a la actividad y abre el diálogo. Desde el hilo de la interfaz. */
    static void pedir(Activity a, String aviso) {
        PantallaPermiso f = new PantallaPermiso();
        Bundle b = new Bundle();
        b.putString(AVISO, aviso);
        f.setArguments(b);
        a.getFragmentManager().beginTransaction().add(f, "sytue-pantalla").commitAllowingStateLoss();
    }

    @Override
    public void onCreate(Bundle guardado) {
        super.onCreate(guardado);
        if (guardado != null) {
            return; // recreado tras girar: el diálogo ya está abierto y contestará a este
        }
        MediaProjectionManager m = (MediaProjectionManager) getActivity().getSystemService(Activity.MEDIA_PROJECTION_SERVICE);
        startActivityForResult(m.createScreenCaptureIntent(), PEDIR);
    }

    @Override
    public void onActivityResult(int pedido, int resultado, Intent datos) {
        super.onActivityResult(pedido, resultado, datos);
        if (pedido != PEDIR) {
            return;
        }
        Activity a = getActivity();
        if (resultado == Activity.RESULT_OK && datos != null && a != null) {
            Intent i = new Intent(a, PantallaServicio.class)
                    .putExtra(PantallaServicio.RESULTADO, resultado)
                    .putExtra(PantallaServicio.DATOS, datos)
                    .putExtra(PantallaServicio.AVISO_TEXTO, getArguments().getString(AVISO));
            if (Build.VERSION.SDK_INT >= 26) {
                a.startForegroundService(i);
            } else {
                a.startService(i);
            }
        } else {
            Pantalla.negado();
        }
        if (a != null) {
            a.getFragmentManager().beginTransaction().remove(this).commitAllowingStateLoss();
        }
    }
}
