// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import android.graphics.Bitmap;
import java.io.IOException;
import java.util.HashMap;
import java.util.Map;

/**
 * Un MediaMetadataRetriever de MENTIRA para VideoSistemaPrueba. Los vídeos los pone la
 * prueba en {@link #VIDEOS}: ruta → {duración en ms (o -1: sin ella), ancho, alto, desde (µs:
 * antes no hay fotograma)}. {@link #llamadas} apunta lo que se pidió y {@link #vivos} cuenta
 * los que no se han soltado.
 */
public final class MediaMetadataRetriever {
    public static final int METADATA_KEY_DURATION = 9;
    public static final int OPTION_PREVIOUS_SYNC = 0;
    public static final int OPTION_NEXT_SYNC = 1;
    public static final int OPTION_CLOSEST_SYNC = 2;
    public static final int OPTION_CLOSEST = 3;

    public static final Map<String, long[]> VIDEOS = new HashMap<>();
    public static final StringBuilder llamadas = new StringBuilder();
    public static int vivos;

    private long[] v;

    public MediaMetadataRetriever() {
        vivos++;
    }

    public void setDataSource(String ruta) {
        v = VIDEOS.get(ruta);
        if (v == null) {
            throw new IllegalArgumentException("no existe: " + ruta);
        }
    }

    public String extractMetadata(int clave) {
        return clave == METADATA_KEY_DURATION && v[0] >= 0 ? Long.toString(v[0]) : null;
    }

    public Bitmap getFrameAtTime(long us, int opcion) {
        llamadas.append("getFrameAtTime(").append(us).append(',').append(opcion).append(") ");
        return us < v[3] ? null : new Bitmap((int) v[1], (int) v[2]);
    }

    /** Como en Android: cabe en ancho×alto sin deformar. */
    public Bitmap getScaledFrameAtTime(long us, int opcion, int ancho, int alto) {
        llamadas.append("getScaledFrameAtTime(").append(us).append(',').append(opcion).append(',')
                .append(ancho).append(',').append(alto).append(") ");
        if (us < v[3]) {
            return null;
        }
        double k = Math.min(1.0, Math.min((double) ancho / v[1], (double) alto / v[2]));
        return new Bitmap((int) Math.round(v[1] * k), (int) Math.round(v[2] * k));
    }

    /** Desde la API 29 declara IOException. */
    public void release() throws IOException {
        vivos--;
    }
}
