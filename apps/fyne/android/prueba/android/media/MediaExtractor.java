// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;

/**
 * Un MediaExtractor de MENTIRA para VideoSistemaPrueba: los «ficheros» los pone la prueba en
 * {@link #FICHEROS} (pistas con su formato y sus muestras), y {@link #vivos} cuenta los que no
 * se han soltado. Una pista elegida cada vez, que es como lo usa VideoSistema.
 */
public final class MediaExtractor {
    public static final int SEEK_TO_PREVIOUS_SYNC = 0;
    public static final int SEEK_TO_NEXT_SYNC = 1;
    public static final int SEEK_TO_CLOSEST_SYNC = 2;

    public static final Map<String, Fichero> FICHEROS = new HashMap<>();
    public static int vivos;

    public static final class Muestra {
        public final long pts;
        public final byte[] datos;
        public final boolean clave;

        public Muestra(long pts, byte[] datos, boolean clave) {
            this.pts = pts;
            this.datos = datos;
            this.clave = clave;
        }
    }

    public static final class Pista {
        public final MediaFormat formato;
        public final List<Muestra> muestras = new ArrayList<>();

        public Pista(MediaFormat formato) {
            this.formato = formato;
        }
    }

    public static final class Fichero {
        public final List<Pista> pistas = new ArrayList<>();
    }

    private Fichero f;
    private int pista = -1;
    private int i;
    private boolean soltado;

    public MediaExtractor() {
        vivos++;
    }

    public void setDataSource(String ruta) throws IOException {
        f = FICHEROS.get(ruta);
        if (f == null) {
            throw new IOException("no existe: " + ruta);
        }
    }

    public int getTrackCount() {
        return f.pistas.size();
    }

    /** Como en Android, un formato nuevo cada vez. */
    public MediaFormat getTrackFormat(int n) {
        return new MediaFormat(f.pistas.get(n).formato);
    }

    public void selectTrack(int n) {
        pista = n;
        i = 0;
    }

    private List<Muestra> muestras() {
        return f.pistas.get(pista).muestras;
    }

    public int readSampleData(ByteBuffer b, int desde) {
        if (pista < 0 || i >= muestras().size()) {
            return -1;
        }
        byte[] d = muestras().get(i).datos;
        b.clear();
        b.position(desde);
        b.put(d);
        return d.length;
    }

    public long getSampleTime() {
        return pista < 0 || i >= muestras().size() ? -1 : muestras().get(i).pts;
    }

    public boolean advance() {
        if (i < muestras().size()) {
            i++;
        }
        return i < muestras().size();
    }

    /** Al último punto de acceso (clave) que empieza en o antes de us. */
    public void seekTo(long us, int modo) {
        int j = 0;
        for (int k = 0; k < muestras().size(); k++) {
            Muestra m = muestras().get(k);
            if (m.clave && m.pts <= us) {
                j = k;
            }
        }
        i = j;
    }

    public void release() {
        if (!soltado) {
            soltado = true;
            vivos--;
        }
    }
}
