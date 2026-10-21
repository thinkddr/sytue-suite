// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.app.Application;
import android.app.PendingIntent;
import android.app.PictureInPictureParams;
import android.app.RemoteAction;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.content.IntentFilter;
import android.content.pm.PackageManager;
import android.graphics.drawable.Icon;
import android.os.Build;
import android.os.Bundle;
import android.util.Rational;
import java.util.Collections;

/**
 * La imagen dentro de imagen de Android para el vídeo de Cast (`sesion/imagenenimagen_android.go`).
 *
 * La del escritorio es una ventana aparte, que Fyne no tiene en el teléfono; en Android es la
 * propia actividad encogida en una esquina (`enterPictureInPictureMode`, API 26), con el
 * atributo `supportsPictureInPicture` en el manifiesto (lo pone empaquetar.sh a las apps con
 * `imagenEnImagen = "si"`). Aquí va lo que exige hablar con la actividad y con el sistema:
 * entrar, el botón de reproducir o pausar dentro de la ventana, la entrada sola al salir de
 * la app con el vídeo en marcha (API 31) y avisar a Go cuando la ventana se cierra.
 */
public final class ImagenEnImagen {
    /** Lo que llega a Go por {@link #suceso}. */
    static final int ALTERNAR = 1, CERRADA = 2;

    private static final String ACCION = "eu.sytue.nativo.IMAGEN_EN_IMAGEN_ALTERNAR";
    private static boolean preparada;
    private static boolean enEsquina;
    private static int ancho = 16, alto = 9;

    private ImagenEnImagen() {}

    /** Lo implementa Go (sesion): ALTERNAR o CERRADA, en el hilo de la interfaz de Android. */
    static native void suceso(int que);

    /** Si este teléfono sabe hacerlo: API 26 y la función del sistema (hay equipos que la quitan). */
    public static boolean disponible(Context ctx) {
        return Build.VERSION.SDK_INT >= 26
                && ctx.getPackageManager().hasSystemFeature(PackageManager.FEATURE_PICTURE_IN_PICTURE);
    }

    /** Si la actividad está ahora en la esquina. */
    public static boolean activa(Context ctx) {
        return Build.VERSION.SDK_INT >= 26 && ctx instanceof Activity
                && ((Activity) ctx).isInPictureInPictureMode();
    }

    /**
     * Deja preparada la ventana: la proporción del vídeo, el botón de reproducir o pausar
     * según `reproduciendo`, y si se entra sola al salir de la app (solo con el vídeo en
     * marcha, como hacen las apps de vídeo).
     */
    public static void preparar(Context ctx, int w, int h, boolean reproduciendo) {
        if (!disponible(ctx) || !(ctx instanceof Activity)) {
            return;
        }
        final Activity a = (Activity) ctx;
        if (w > 0 && h > 0) {
            ancho = w;
            alto = h;
        }
        final PictureInPictureParams p = parametros(a, reproduciendo);
        a.runOnUiThread(() -> {
            engancharUnaVez(a);
            a.setPictureInPictureParams(p);
        });
    }

    /** Entra en la esquina ahora. Devuelve false si el teléfono no sabe. */
    public static boolean entrar(Context ctx, boolean reproduciendo) {
        if (!disponible(ctx) || !(ctx instanceof Activity)) {
            return false;
        }
        final Activity a = (Activity) ctx;
        final PictureInPictureParams p = parametros(a, reproduciendo);
        a.runOnUiThread(() -> {
            engancharUnaVez(a);
            try {
                a.enterPictureInPictureMode(p);
            } catch (IllegalStateException e) {
                // La actividad no está en primer plano: no hay nada que encoger.
            }
        });
        return true;
    }

    private static PictureInPictureParams parametros(Activity a, boolean reproduciendo) {
        PictureInPictureParams.Builder b = new PictureInPictureParams.Builder();
        // Android rechaza proporciones fuera de 1:2,39 … 2,39:1 con una excepción.
        double r = (double) ancho / alto;
        if (r > 2.39) {
            b.setAspectRatio(new Rational(239, 100));
        } else if (r < 1 / 2.39) {
            b.setAspectRatio(new Rational(100, 239));
        } else {
            b.setAspectRatio(new Rational(ancho, alto));
        }
        Intent intent = new Intent(ACCION).setPackage(a.getPackageName());
        PendingIntent pi = PendingIntent.getBroadcast(a, 7416, intent,
                PendingIntent.FLAG_IMMUTABLE | PendingIntent.FLAG_UPDATE_CURRENT);
        int icono = reproduciendo ? android.R.drawable.ic_media_pause : android.R.drawable.ic_media_play;
        String rotulo = reproduciendo ? "Pause" : "Play";
        b.setActions(Collections.singletonList(
                new RemoteAction(Icon.createWithResource(a, icono), rotulo, rotulo, pi)));
        if (Build.VERSION.SDK_INT >= 31) {
            b.setAutoEnterEnabled(reproduciendo);
            b.setSeamlessResizeEnabled(true);
        }
        return b.build();
    }

    /** El botón de la ventana y el aviso de que se ha cerrado, una vez por proceso. */
    private static void engancharUnaVez(Activity a) {
        if (preparada) {
            return;
        }
        preparada = true;
        BroadcastReceiver r = new BroadcastReceiver() {
            @Override
            public void onReceive(Context c, Intent i) {
                suceso(ALTERNAR);
            }
        };
        IntentFilter f = new IntentFilter(ACCION);
        if (Build.VERSION.SDK_INT >= 33) {
            a.getApplicationContext().registerReceiver(r, f, Context.RECEIVER_NOT_EXPORTED);
        } else {
            a.getApplicationContext().registerReceiver(r, f);
        }
        // Cerrar la ventana de la esquina (la X, o arrastrarla fuera) para la actividad sin
        // volver a primer plano: si al pausarse estaba en la esquina y ahora se para, se cerró.
        a.getApplication().registerActivityLifecycleCallbacks(new Application.ActivityLifecycleCallbacks() {
            @Override public void onActivityPaused(Activity x) { enEsquina = activa(x); }
            @Override public void onActivityResumed(Activity x) { enEsquina = false; }
            @Override public void onActivityStopped(Activity x) {
                if (enEsquina) {
                    enEsquina = false;
                    suceso(CERRADA);
                }
            }
            @Override public void onActivityCreated(Activity x, Bundle b) {}
            @Override public void onActivityStarted(Activity x) {}
            @Override public void onActivitySaveInstanceState(Activity x, Bundle b) {}
            @Override public void onActivityDestroyed(Activity x) {}
        });
    }
}
