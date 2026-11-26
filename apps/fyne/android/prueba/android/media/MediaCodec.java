// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.media;

import android.view.Surface;
import java.io.IOException;
import java.nio.ByteBuffer;
import java.util.ArrayDeque;
import java.util.HashMap;
import java.util.Map;

/**
 * Un MediaCodec de MENTIRA para VideoSistemaPrueba, en modo síncrono como lo usa VideoSistema.
 * Qué sale de cada muestra lo decide la prueba con un {@link Falso} por tipo MIME. Se porta
 * como uno de verdad en lo que importa: avisa del formato de salida antes de la primera, tiene
 * cuatro entradas que no se liberan hasta que sale lo suyo, se guarda {@link #retraso} salidas
 * (como un descodificador que reordena) hasta el final de la entrada, y flush() lo vacía.
 */
public final class MediaCodec {
    public static final int BUFFER_FLAG_KEY_FRAME = 1;
    public static final int BUFFER_FLAG_END_OF_STREAM = 4;
    public static final int INFO_TRY_AGAIN_LATER = -1;
    public static final int INFO_OUTPUT_FORMAT_CHANGED = -2;
    public static final int INFO_OUTPUT_BUFFERS_CHANGED = -3;

    public static final class BufferInfo {
        public int offset;
        public int size;
        public long presentationTimeUs;
        public int flags;

        public void set(int offset, int size, long presentationTimeUs, int flags) {
            this.offset = offset;
            this.size = size;
            this.presentationTimeUs = presentationTimeUs;
            this.flags = flags;
        }
    }

    /** Lo que hace el descodificador de mentira de un tipo MIME. */
    public interface Falso {
        /** El formato de salida, a partir del de la pista. */
        MediaFormat salida(MediaFormat entrada);

        /** Lo que sale de una muestra; null, nada. */
        Salida descodificar(byte[] muestra, long pts);
    }

    /** Una salida: la imagen (vídeo) y el búfer que la lleva, o el PCM (sonido). */
    public static final class Salida {
        public final long pts;
        public final Image imagen;
        public final ByteBuffer datos;

        public Salida(long pts, Image imagen, ByteBuffer datos) {
            this.pts = pts;
            this.imagen = imagen;
            this.datos = datos;
        }
    }

    public static final Map<String, Falso> FALSOS = new HashMap<>();
    /** Los creados y no soltados. */
    public static int vivos;
    /** Salidas que se guarda antes de dar la primera, hasta que acaba la entrada. */
    public static int retraso = 2;
    /** Si es ≥ 0, tras dar tantas salidas dequeueOutputBuffer lanza (un códec que se rompe). */
    public static int fallarTras = -1;
    /** Sin getOutputImage (un descodificador viejo): da null y hay que leer el búfer. */
    public static boolean sinImagen;
    /** El fin de la entrada va en la última salida con datos, y no en una vacía aparte. */
    public static boolean finConDatos;

    private static final int ENTRADAS = 4;
    /** Donde empiezan los datos en cada búfer de salida: como en Android, no siempre en 0. */
    private static final int DESDE = 7;

    private final ByteBuffer[] entradas = new ByteBuffer[ENTRADAS];
    private final ArrayDeque<Integer> libres = new ArrayDeque<>();
    private final ArrayDeque<Integer> ocupadas = new ArrayDeque<>();
    private final ArrayDeque<Salida> pendientes = new ArrayDeque<>();
    private final Map<Integer, Salida> fuera = new HashMap<>();
    private Falso falso;
    private MediaFormat formato;
    private int numero, dadas;
    private boolean avisado, finEntrada, soltado;

    private MediaCodec() {
        vivos++;
        for (int i = 0; i < ENTRADAS; i++) {
            entradas[i] = ByteBuffer.allocate(1 << 16);
        }
    }

    public static MediaCodec createByCodecName(String nombre) throws IOException {
        if (!nombre.startsWith("falso.")) {
            throw new IOException("no hay códec " + nombre);
        }
        return new MediaCodec();
    }

    public void configure(MediaFormat f, Surface superficie, MediaCrypto cifrado, int flags) {
        String mime = f.getString(MediaFormat.KEY_MIME);
        falso = FALSOS.get(mime);
        if (falso == null) {
            throw new IllegalArgumentException("falso: no sé configurarme para " + mime);
        }
        formato = new MediaFormat(f);
    }

    public void start() {
        vaciar();
    }

    public void flush() {
        vaciar();
    }

    private void vaciar() {
        libres.clear();
        for (int i = 0; i < ENTRADAS; i++) {
            libres.add(i);
        }
        ocupadas.clear();
        pendientes.clear();
        fuera.clear();
        finEntrada = false;
    }

    public void stop() {}

    public void release() {
        if (!soltado) {
            soltado = true;
            vivos--;
        }
    }

    /** El formato con el que se configuró (para ver qué pidió VideoSistema). */
    public MediaFormat formatoConfigurado() {
        return formato;
    }

    public MediaFormat getOutputFormat() {
        return falso.salida(formato);
    }

    public int dequeueInputBuffer(long us) {
        Integer j = libres.poll();
        return j == null ? INFO_TRY_AGAIN_LATER : j;
    }

    public ByteBuffer getInputBuffer(int j) {
        return entradas[j];
    }

    public void queueInputBuffer(int j, int desde, int n, long pts, int flags) {
        if ((flags & BUFFER_FLAG_END_OF_STREAM) != 0) {
            finEntrada = true;
            libres.add(j);
            pendientes.add(new Salida(pts, null, null));
            return;
        }
        byte[] m = new byte[n];
        ByteBuffer b = entradas[j].duplicate();
        b.clear();
        b.position(desde);
        b.get(m);
        Salida s = falso.descodificar(m, pts);
        if (s == null) {
            libres.add(j);
            return;
        }
        pendientes.add(s);
        ocupadas.add(j);
    }

    public int dequeueOutputBuffer(BufferInfo info, long us) {
        if (fallarTras >= 0 && dadas >= fallarTras) {
            throw new IllegalStateException("falso: el códec se ha roto");
        }
        if (!avisado) {
            avisado = true;
            return INFO_OUTPUT_FORMAT_CHANGED;
        }
        if (pendientes.isEmpty() || (!finEntrada && pendientes.size() <= retraso)) {
            return INFO_TRY_AGAIN_LATER;
        }
        Salida s = pendientes.poll();
        int i = numero++;
        if (s.imagen == null && s.datos == null) {
            fuera.put(i, s);
            info.set(0, 0, s.pts, BUFFER_FLAG_END_OF_STREAM);
            return i;
        }
        Integer j = ocupadas.poll();
        if (j != null) {
            libres.add(j);
        }
        int flags = 0;
        if (finConDatos && pendientes.size() == 1 && pendientes.peek().imagen == null && pendientes.peek().datos == null) {
            // La salida vacía del final se quita y el aviso va en esta.
            pendientes.poll();
            flags = BUFFER_FLAG_END_OF_STREAM;
        }
        fuera.put(i, s);
        info.set(DESDE, s.datos != null ? s.datos.remaining() : 1, s.pts, flags);
        dadas++;
        return i;
    }

    public Image getOutputImage(int i) {
        Salida s = salida(i);
        return sinImagen ? null : s.imagen;
    }

    /**
     * Como en Android: de solo lectura, con los datos desde info.offset (aquí, tras DESDE bytes
     * de basura) y la posición y el límite en ellos.
     */
    public ByteBuffer getOutputBuffer(int i) {
        Salida s = salida(i);
        if (s.datos == null) {
            return null;
        }
        ByteBuffer d = s.datos.duplicate();
        ByteBuffer b = ByteBuffer.allocate(DESDE + d.remaining() + 3);
        while (b.position() < DESDE) {
            b.put((byte) 0x55);
        }
        b.put(d);
        b.limit(b.position());
        b.position(DESDE);
        return b.asReadOnlyBuffer();
    }

    public void releaseOutputBuffer(int i, boolean pintar) {
        if (fuera.remove(i) == null) {
            throw new IllegalStateException("falso: la salida " + i + " no está fuera");
        }
    }

    private Salida salida(int i) {
        Salida s = fuera.get(i);
        if (s == null) {
            throw new IllegalStateException("falso: la salida " + i + " no está fuera");
        }
        return s;
    }
}
