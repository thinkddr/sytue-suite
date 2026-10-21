// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Context;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.os.Build;
import android.os.IBinder;

/**
 * El servicio en primer plano del Hub: mantiene vivo el proceso —y con él el flujo del
 * Hub que escucha Go— cuando la actividad pasa a segundo plano o se cierra, para que los
 * avisos de la suite lleguen con la app cerrada (ADR 0039). No hace nada más: la conexión
 * y los avisos siguen en Go (`escritorio.Avisar`). Lo declara solo el manifiesto del Hub
 * (`empaquetar.sh`) y avisa por las trece apps.
 *
 * Android exige que un servicio así se vea: una notificación fija, de importancia
 * mínima, que abre el Hub al tocarla.
 */
public final class Avisos extends Service {
    private static final String CANAL = "sytue-segundo-plano";
    private static final int ID = 0x5347;

    /**
     * Lo arranca Go al entrar en la sesión, con los textos ya traducidos. Desde Android 12
     * solo se puede con la app delante; si no, el sistema lo rechaza y se devuelve false.
     */
    public static boolean empezar(Context ctx, String titulo, String texto) {
        Intent i = new Intent(ctx, Avisos.class).putExtra("titulo", titulo).putExtra("texto", texto);
        try {
            if (Build.VERSION.SDK_INT >= 26) {
                ctx.startForegroundService(i);
            } else {
                ctx.startService(i);
            }
            return true;
        } catch (RuntimeException e) { // ForegroundServiceStartNotAllowedException, SecurityException
            return false;
        }
    }

    /** Lo para Go al salir de la sesión: sin nadie dentro no hay de qué avisar. */
    public static void parar(Context ctx) {
        ctx.stopService(new Intent(ctx, Avisos.class));
    }

    @Override
    @SuppressWarnings("deprecation") // Notification.Builder(Context), el único antes de Android 8
    public int onStartCommand(Intent i, int flags, int arranque) {
        String titulo = i != null ? i.getStringExtra("titulo") : null;
        String texto = i != null ? i.getStringExtra("texto") : null;
        Notification.Builder b;
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationManager m = getSystemService(NotificationManager.class);
            m.createNotificationChannel(new NotificationChannel(CANAL, titulo != null ? titulo : "Sytue",
                    NotificationManager.IMPORTANCE_MIN));
            b = new Notification.Builder(this, CANAL);
        } else {
            b = new Notification.Builder(this);
        }
        b.setContentTitle(titulo).setContentText(texto).setSmallIcon(getApplicationInfo().icon).setOngoing(true);
        Intent abrir = getPackageManager().getLaunchIntentForPackage(getPackageName());
        if (abrir != null) {
            b.setContentIntent(PendingIntent.getActivity(this, 0, abrir, PendingIntent.FLAG_IMMUTABLE));
        }
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(ID, b.build(), ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE);
        } else {
            startForeground(ID, b.build());
        }
        // NOT_STICKY: si Android mata el proceso, rearrancar el servicio sin la actividad
        // no rearranca Go, y quedaría la notificación fija sin nadie escuchando.
        return START_NOT_STICKY;
    }

    @Override
    public IBinder onBind(Intent i) {
        return null;
    }
}
