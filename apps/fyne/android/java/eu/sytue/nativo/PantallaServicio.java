// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.content.pm.ServiceInfo;
import android.media.projection.MediaProjection;
import android.media.projection.MediaProjectionManager;
import android.os.Build;
import android.os.IBinder;

/**
 * El servicio en primer plano mientras se comparte la pantalla: Android 14 no deja
 * proyectar sin él, y su aviso fijo es lo que le dice a la persona que se está
 * compartiendo aunque salga de la app.
 */
public final class PantallaServicio extends Service {
    static final String RESULTADO = "resultado", DATOS = "datos", AVISO_TEXTO = "aviso";
    private static final String CANAL = "sytue-pantalla";
    private static final int AVISO = 7417;

    @Override
    @SuppressWarnings("deprecation") // getParcelableExtra(String) y Notification.Builder(Context)
    public int onStartCommand(Intent i, int flags, int arranque) {
        if (i == null) {
            stopSelf();
            return START_NOT_STICKY;
        }
        Notification.Builder b;
        if (Build.VERSION.SDK_INT >= 26) {
            NotificationManager m = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
            m.createNotificationChannel(new NotificationChannel(CANAL, "Sytue Rooms", NotificationManager.IMPORTANCE_LOW));
            b = new Notification.Builder(this, CANAL);
        } else {
            b = new Notification.Builder(this);
        }
        Notification aviso = b.setContentTitle(getApplicationInfo().loadLabel(getPackageManager()))
                .setContentText(i.getStringExtra(AVISO_TEXTO))
                .setSmallIcon(getApplicationInfo().icon).setOngoing(true).build();
        if (Build.VERSION.SDK_INT >= 29) {
            startForeground(AVISO, aviso, ServiceInfo.FOREGROUND_SERVICE_TYPE_MEDIA_PROJECTION);
        } else {
            startForeground(AVISO, aviso);
        }
        MediaProjectionManager m = (MediaProjectionManager) getSystemService(MEDIA_PROJECTION_SERVICE);
        Intent datos = i.getParcelableExtra(DATOS);
        MediaProjection p = datos == null ? null : m.getMediaProjection(i.getIntExtra(RESULTADO, 0), datos);
        if (p == null) {
            Pantalla.negado();
            stopSelf();
            return START_NOT_STICKY;
        }
        Pantalla.empezar(this, p);
        return START_NOT_STICKY;
    }

    @Override
    public IBinder onBind(Intent i) {
        return null;
    }
}
