// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.graphics.Bitmap;
import android.graphics.Rect;
import android.media.AudioFormat;
import android.media.Image;
import android.media.MediaCodec;
import android.media.MediaCodecInfo;
import android.media.MediaCodecList;
import android.media.MediaExtractor;
import android.media.MediaFormat;
import android.media.MediaMetadataRetriever;
import android.os.Build;
import java.io.ByteArrayOutputStream;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.util.Arrays;
import java.util.HashMap;
import java.util.Map;

/**
 * El vídeo del sistema de Android para Sytue Office (PLAN-EDICION-LOCAL §6.10):
 *
 * <ul>
 *   <li>el descodificador del reproductor (CA-08, `apps/fyne/video/sistema/mediacodec_android.go`):
 *       `MediaExtractor` + `MediaCodec` sin superficie, que dan a Go cada fotograma en I420, ya
 *       derecho (con el giro de la pista), y el sonido en PCM mono de 16 bits al muestreo que
 *       pida Go, cada cosa con su instante;</li>
 *   <li>la miniatura de un vídeo (CA-06, `apps/nucleo/local/videoteca/miniatura_android.go`):
 *       `MediaMetadataRetriever`, en JPEG.</li>
 * </ul>
 *
 * Go llama desde un hilo suyo, enganchado a la máquina, y aquí todo es síncrono: el códec va
 * en modo síncrono (sin Looper ni avisos) con esperas cortas, y nada llama a Go desde aquí.
 * Cada vídeo abierto tiene un número. Lleva DOS extractores, uno por pista: el MP4 entrelaza
 * vídeo y sonido a trozos, y con uno solo la pista que no se pide tendría que esperar a que
 * Go vacíe la otra.
 *
 * Si el teléfono no sabe (no hay descodificador para el formato, o no da ni un fotograma),
 * {@link #abrir} devuelve un número negativo y Go sigue como sin descodificador del sistema:
 * lo que no es WebM se abre con el reproductor del sistema.
 */
public final class VideoSistema {
    private VideoSistema() {}

    // Lo que devuelve abrir() cuando no puede; Go lo traduce (mediacodec_android.go).
    /** El sistema no lee el fichero. */
    static final int NO_LEE = -1;
    /** Ni vídeo ni sonido. */
    static final int SIN_PISTAS = -2;
    /** El teléfono no tiene descodificador para su formato. */
    static final int SIN_CODEC = -3;
    /** El descodificador no dio nada. */
    static final int FALLO = -4;

    // Lo que hay en cada muestra de siguiente(): una cabecera de 20 bytes en orden de red
    // (tipo, instante en µs, y ancho y alto, o cuántas muestras y 0) y detrás la imagen en
    // I420 (Y, U y V seguidas y sin relleno) o el PCM mono de 16 bits, también en orden de red.
    static final int IMAGEN = 1;
    static final int SONIDO = 2;
    static final int CABECERA = 20;

    /** Lo que se espera a cada salida del códec, en µs. */
    private static final long ESPERA_US = 10_000;
    /** Esperas seguidas sin que salga nada antes de darlo por atascado (unos 5 s). */
    static int maxEsperas = 500;

    private static final Map<Integer, Abierto> ABIERTOS = new HashMap<>();
    private static int ultimo;

    /**
     * Abre `ruta` para descodificar, con el sonido a `muestreo` Hz. Devuelve el número con el
     * que Go lo pide, o uno de los negativos de arriba. Ya trae descodificado el primer
     * fotograma (y el primer trozo de sonido): si el teléfono no sabe, se sabe aquí.
     */
    public static int abrir(String ruta, int muestreo) {
        Abierto a;
        try {
            a = Abierto.abrir(ruta, muestreo);
        } catch (NoPuede e) {
            return e.codigo;
        } catch (Throwable e) {
            return FALLO;
        }
        synchronized (ABIERTOS) {
            do {
                ultimo = ultimo == Integer.MAX_VALUE ? 1 : ultimo + 1;
            } while (ABIERTOS.containsKey(ultimo));
            ABIERTOS.put(ultimo, a);
            return ultimo;
        }
    }

    /** «duración en µs\tancho\talto\tvídeo\tsonido» (1 o 0 los dos últimos); "" si no está. */
    public static String info(int id) {
        Abierto a = abierto(id);
        if (a == null) {
            return "";
        }
        return a.duracionUs + "\t" + a.ancho + "\t" + a.alto + "\t" + (a.video != null ? 1 : 0) + "\t"
                + (a.sonido != null ? 1 : 0);
    }

    /**
     * La muestra siguiente en orden de tiempo, con su cabecera; null al acabar. Si el códec
     * falla, lanza su excepción y Go la enseña.
     */
    public static byte[] siguiente(int id) throws Exception {
        Abierto a = abierto(id);
        if (a == null) {
            throw new IllegalStateException("VideoSistema: el vídeo " + id + " no está abierto");
        }
        return a.siguiente();
    }

    /**
     * Salta a `ms`: cada pista al punto de acceso anterior, y lo de antes de `ms` se descarta
     * aquí sin pasarlo a Go. Devuelve dónde queda, en ms, o -1 si no se pudo.
     */
    public static int buscar(int id, int ms) {
        Abierto a = abierto(id);
        if (a == null) {
            return -1;
        }
        try {
            return (int) (a.buscar(Math.max(0, ms) * 1000L) / 1000L);
        } catch (Throwable e) {
            return -1;
        }
    }

    /** Suelta el códec y los extractores. */
    public static void cerrar(int id) {
        Abierto a;
        synchronized (ABIERTOS) {
            a = ABIERTOS.remove(id);
        }
        if (a != null) {
            a.cerrar();
        }
    }

    /** Cuántos hay abiertos (para las pruebas). */
    static int abiertos() {
        synchronized (ABIERTOS) {
            return ABIERTOS.size();
        }
    }

    private static Abierto abierto(int id) {
        synchronized (ABIERTOS) {
            return ABIERTOS.get(id);
        }
    }

    // --- La miniatura ----------------------------------------------------------------

    /**
     * La miniatura del vídeo de `ruta` en JPEG, que cabe en `lado`×`lado`, o null si el
     * sistema no la da. El fotograma, a un segundo si el vídeo dura más de tres (el primero
     * suele ser negro), y si no, del principio. `MediaMetadataRetriever` ya lo da derecho.
     */
    public static byte[] miniatura(String ruta, int lado) {
        MediaMetadataRetriever r = new MediaMetadataRetriever();
        Bitmap b = null;
        try {
            r.setDataSource(ruta);
            long us = 0;
            String d = r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION);
            if (d != null) {
                try {
                    if (Long.parseLong(d.trim()) > 3000) {
                        us = 1_000_000L;
                    }
                } catch (NumberFormatException ignorada) {
                    // Sin duración: del principio.
                }
            }
            b = fotograma(r, us, lado);
            if (b == null && us > 0) {
                b = fotograma(r, 0, lado);
            }
            if (b == null) {
                return null;
            }
            ByteArrayOutputStream o = new ByteArrayOutputStream();
            if (!b.compress(Bitmap.CompressFormat.JPEG, 85, o) || o.size() == 0) {
                return null;
            }
            return o.toByteArray();
        } catch (Throwable e) {
            return null;
        } finally {
            if (b != null) {
                b.recycle();
            }
            try {
                r.release();
            } catch (Throwable ignorada) {
                // Ya no se usa.
            }
        }
    }

    /** El fotograma de `us` que cabe en lado×lado: escalado por el sistema desde la API 27. */
    private static Bitmap fotograma(MediaMetadataRetriever r, long us, int lado) {
        if (Build.VERSION.SDK_INT >= 27) {
            return r.getScaledFrameAtTime(us, MediaMetadataRetriever.OPTION_CLOSEST_SYNC, lado, lado);
        }
        Bitmap b = r.getFrameAtTime(us, MediaMetadataRetriever.OPTION_CLOSEST_SYNC);
        if (b == null || (b.getWidth() <= lado && b.getHeight() <= lado)) {
            return b;
        }
        double k = Math.min((double) lado / b.getWidth(), (double) lado / b.getHeight());
        Bitmap c = Bitmap.createScaledBitmap(b, Math.max(1, (int) Math.round(b.getWidth() * k)),
                Math.max(1, (int) Math.round(b.getHeight() * k)), true);
        if (c != b) {
            b.recycle();
        }
        return c;
    }

    // --- Un vídeo abierto ------------------------------------------------------------

    /** Lo que abrir() no puede, con su código. */
    static final class NoPuede extends Exception {
        private static final long serialVersionUID = 1L;
        final int codigo;

        NoPuede(int codigo) {
            super("VideoSistema: " + codigo);
            this.codigo = codigo;
        }
    }

    static final class Abierto {
        Pista video, sonido;
        long duracionUs;
        int ancho, alto;

        static Abierto abrir(String ruta, int muestreo) throws Exception {
            MediaExtractor sondeo = new MediaExtractor();
            int pv = -1, ps = -1;
            MediaFormat fv = null, fs = null;
            long dur = 0;
            try {
                try {
                    sondeo.setDataSource(ruta);
                } catch (Exception e) {
                    throw new NoPuede(NO_LEE);
                }
                for (int i = 0; i < sondeo.getTrackCount(); i++) {
                    MediaFormat f = sondeo.getTrackFormat(i);
                    String mime = f.containsKey(MediaFormat.KEY_MIME) ? f.getString(MediaFormat.KEY_MIME) : null;
                    if (mime == null) {
                        continue;
                    }
                    if (pv < 0 && mime.startsWith("video/")) {
                        pv = i;
                        fv = f;
                    } else if (ps < 0 && mime.startsWith("audio/")) {
                        ps = i;
                        fs = f;
                    } else {
                        continue;
                    }
                    if (f.containsKey(MediaFormat.KEY_DURATION)) {
                        dur = Math.max(dur, f.getLong(MediaFormat.KEY_DURATION));
                    }
                }
            } finally {
                sondeo.release();
            }
            if (pv < 0 && ps < 0) {
                throw new NoPuede(SIN_PISTAS);
            }
            Abierto a = new Abierto();
            a.duracionUs = dur;
            try {
                // Un vídeo que el teléfono no sabe descodificar no se reproduce solo con su
                // sonido: falla, y Go lo deja al reproductor del sistema.
                if (pv >= 0) {
                    a.video = Pista.abrir(ruta, pv, fv, true, muestreo);
                    if (!a.video.llenar()) {
                        throw new NoPuede(FALLO);
                    }
                    a.ancho = a.video.anchoSalida;
                    a.alto = a.video.altoSalida;
                }
                // Un sonido que no sabe, en cambio, se quita y el vídeo se ve sin él.
                if (ps >= 0) {
                    try {
                        a.sonido = Pista.abrir(ruta, ps, fs, false, muestreo);
                        if (!a.sonido.llenar()) {
                            a.sonido.cerrar();
                            a.sonido = null;
                        }
                    } catch (Exception e) {
                        if (a.sonido != null) {
                            a.sonido.cerrar();
                        }
                        a.sonido = null;
                    }
                }
            } catch (Exception e) {
                a.cerrar();
                throw e;
            }
            if (a.video == null && a.sonido == null) {
                throw new NoPuede(SIN_CODEC);
            }
            return a;
        }

        /** La salida pendiente que va antes; null si las dos pistas han acabado. */
        synchronized byte[] siguiente() throws Exception {
            if (video != null && !video.hay && !video.fin) {
                video.llenar();
            }
            if (sonido != null && !sonido.hay && !sonido.fin) {
                sonido.llenar();
            }
            Pista p = null;
            if (video != null && video.hay) {
                p = video;
            }
            if (sonido != null && sonido.hay && (p == null || sonido.pts < p.pts)) {
                p = sonido;
            }
            if (p == null) {
                return null;
            }
            p.hay = false;
            return p.salida;
        }

        synchronized long buscar(long us) {
            if (video != null) {
                video.buscar(us);
            }
            if (sonido != null) {
                sonido.buscar(us);
            }
            return us;
        }

        synchronized void cerrar() {
            if (video != null) {
                video.cerrar();
            }
            if (sonido != null) {
                sonido.cerrar();
            }
        }
    }

    /** Una pista con su extractor y su códec, y la salida que aún no se ha dado a Go. */
    static final class Pista {
        final boolean esVideo;
        final MediaExtractor ext;
        final MediaCodec codec;
        final MediaCodec.BufferInfo info = new MediaCodec.BufferInfo();
        /** El giro de la pista, en grados en el sentido del reloj (0, 90, 180 o 270). */
        final int giro;
        /** El muestreo del PCM que quiere Go. */
        final int muestreo;
        final Remuestreo remuestreo = new Remuestreo();
        MediaFormat formato;
        boolean finEntrada, fin, hay;
        /** Lo de antes de aquí (µs) se descarta: el punto buscado. */
        long objetivo = Long.MIN_VALUE;
        long pts;
        byte[] salida;
        int anchoSalida, altoSalida;

        private Pista(boolean esVideo, MediaExtractor ext, MediaCodec codec, int giro, int muestreo) {
            this.esVideo = esVideo;
            this.ext = ext;
            this.codec = codec;
            this.giro = giro;
            this.muestreo = muestreo;
        }

        static Pista abrir(String ruta, int indice, MediaFormat f, boolean video, int muestreo) throws Exception {
            String nombre = new MediaCodecList(MediaCodecList.REGULAR_CODECS).findDecoderForFormat(f);
            if (nombre == null) {
                throw new NoPuede(SIN_CODEC);
            }
            int giro = 0;
            if (video && f.containsKey(MediaFormat.KEY_ROTATION)) {
                giro = ((f.getInteger(MediaFormat.KEY_ROTATION) % 360) + 360) % 360;
                if (giro % 90 != 0) {
                    giro = 0;
                }
            }
            MediaExtractor ext = new MediaExtractor();
            MediaCodec codec = null;
            try {
                ext.setDataSource(ruta);
                ext.selectTrack(indice);
                codec = MediaCodec.createByCodecName(nombre);
                if (video) {
                    // Así getOutputImage() da los planos YUV de cualquier descodificador.
                    f.setInteger(MediaFormat.KEY_COLOR_FORMAT,
                            MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Flexible);
                }
                codec.configure(f, null, null, 0);
                codec.start();
            } catch (Exception e) {
                if (codec != null) {
                    codec.release();
                }
                ext.release();
                throw new NoPuede(SIN_CODEC);
            }
            return new Pista(video, ext, codec, giro, muestreo);
        }

        /**
         * Descodifica hasta tener una salida (true, en `salida`) o acabar la pista (false).
         * Lo de antes del punto buscado se suelta sin convertir.
         */
        boolean llenar() throws Exception {
            int esperas = 0;
            while (!fin) {
                while (!finEntrada && alimentar()) {
                    // Todas las entradas libres: el códec reordena y pide varias.
                }
                int i = codec.dequeueOutputBuffer(info, ESPERA_US);
                if (i == MediaCodec.INFO_OUTPUT_FORMAT_CHANGED) {
                    formato = codec.getOutputFormat();
                    continue;
                }
                if (i < 0) {
                    // INFO_TRY_AGAIN_LATER (o, en un Android viejo, INFO_OUTPUT_BUFFERS_CHANGED).
                    if (++esperas > maxEsperas) {
                        throw new IllegalStateException("VideoSistema: el descodificador no da nada");
                    }
                    continue;
                }
                esperas = 0;
                boolean acaba = (info.flags & MediaCodec.BUFFER_FLAG_END_OF_STREAM) != 0;
                boolean vale = false;
                try {
                    if (info.size > 0) {
                        vale = esVideo ? sacarImagen(i) : sacarSonido(i);
                    }
                } finally {
                    codec.releaseOutputBuffer(i, false);
                }
                if (acaba) {
                    fin = true;
                }
                if (vale) {
                    hay = true;
                    return true;
                }
            }
            return false;
        }

        /** Da al códec la muestra siguiente del extractor; false si no tiene entrada libre. */
        private boolean alimentar() {
            int j = codec.dequeueInputBuffer(0);
            if (j < 0) {
                return false;
            }
            ByteBuffer b = codec.getInputBuffer(j);
            int n = b == null ? -1 : ext.readSampleData(b, 0);
            if (n < 0) {
                codec.queueInputBuffer(j, 0, 0, 0, MediaCodec.BUFFER_FLAG_END_OF_STREAM);
                finEntrada = true;
                return false;
            }
            codec.queueInputBuffer(j, 0, n, ext.getSampleTime(), 0);
            ext.advance();
            return true;
        }

        void buscar(long us) {
            ext.seekTo(us, MediaExtractor.SEEK_TO_PREVIOUS_SYNC);
            codec.flush();
            finEntrada = false;
            fin = false;
            hay = false;
            objetivo = us;
            remuestreo.reiniciar();
        }

        void cerrar() {
            try {
                codec.stop();
            } catch (Throwable ignorada) {
                // Ya parado o roto: se suelta igual.
            }
            try {
                codec.release();
            } catch (Throwable ignorada) {
                // Nada más que hacer.
            }
            try {
                ext.release();
            } catch (Throwable ignorada) {
                // Nada más que hacer.
            }
        }

        private MediaFormat formatoSalida() {
            if (formato == null) {
                formato = codec.getOutputFormat();
            }
            return formato;
        }

        // --- Imagen ---

        @SuppressWarnings("deprecation") // los formatos de color del búfer, solo sin Image
        private boolean sacarImagen(int i) {
            if (info.presentationTimeUs < objetivo) {
                return false;
            }
            Image img = null;
            try {
                img = codec.getOutputImage(i);
            } catch (IllegalStateException e) {
                img = null;
            }
            if (img != null) {
                try {
                    Rect r = img.getCropRect();
                    int x0 = 0, y0 = 0, w = img.getWidth(), h = img.getHeight();
                    if (r != null && !r.isEmpty()) {
                        x0 = r.left;
                        y0 = r.top;
                        w = r.width();
                        h = r.height();
                    }
                    Image.Plane[] p = img.getPlanes();
                    Plano y = Plano.de(p[0]), u = Plano.de(p[1]), v = Plano.de(p[2]);
                    ponerImagen(y.desde(x0, y0), u.desde(x0 / 2, y0 / 2), v.desde(x0 / 2, y0 / 2), w, h);
                } finally {
                    img.close();
                }
                return true;
            }
            // Sin Image (un descodificador viejo): el búfer con el formato de color que diga.
            MediaFormat f = formatoSalida();
            ByteBuffer b = codec.getOutputBuffer(i);
            int color = f.containsKey(MediaFormat.KEY_COLOR_FORMAT) ? f.getInteger(MediaFormat.KEY_COLOR_FORMAT) : 0;
            int ancho = f.getInteger(MediaFormat.KEY_WIDTH), alto = f.getInteger(MediaFormat.KEY_HEIGHT);
            int paso = f.containsKey(MediaFormat.KEY_STRIDE) ? f.getInteger(MediaFormat.KEY_STRIDE) : ancho;
            int filas = f.containsKey(MediaFormat.KEY_SLICE_HEIGHT) ? f.getInteger(MediaFormat.KEY_SLICE_HEIGHT) : alto;
            paso = Math.max(paso, ancho);
            filas = Math.max(filas, alto);
            int x0 = entero(f, "crop-left", 0), y0 = entero(f, "crop-top", 0);
            int w = entero(f, "crop-right", ancho - 1) - x0 + 1, h = entero(f, "crop-bottom", alto - 1) - y0 + 1;
            // El búfer empieza donde están los datos (su posición es info.offset).
            int base = b.position();
            Plano y = new Plano(b, base, paso, 1);
            Plano u, v;
            if (color == MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420Planar) {
                u = new Plano(b, base + paso * filas, paso / 2, 1);
                v = new Plano(b, base + paso * filas + (paso / 2) * (filas / 2), paso / 2, 1);
            } else if (color == MediaCodecInfo.CodecCapabilities.COLOR_FormatYUV420SemiPlanar) {
                u = new Plano(b, base + paso * filas, paso, 2);
                v = new Plano(b, base + paso * filas + 1, paso, 2);
            } else {
                throw new IllegalStateException("VideoSistema: formato de color " + color + " sin imagen");
            }
            ponerImagen(y.desde(x0, y0), u.desde(x0 / 2, y0 / 2), v.desde(x0 / 2, y0 / 2), w, h);
            return true;
        }

        /** Deja en `salida` la cabecera y el I420 de w×h, girado. */
        private void ponerImagen(Plano y, Plano u, Plano v, int w, int h) {
            if (w <= 0 || h <= 0) {
                throw new IllegalStateException("VideoSistema: fotograma vacío");
            }
            boolean tumbado = giro % 180 != 0;
            int wo = tumbado ? h : w, ho = tumbado ? w : h;
            int cw = (w + 1) / 2, ch = (h + 1) / 2;
            int cwo = (wo + 1) / 2, cho = (ho + 1) / 2;
            int n = CABECERA + wo * ho + 2 * cwo * cho;
            if (salida == null || salida.length != n) {
                salida = new byte[n];
            }
            cabecera(salida, IMAGEN, info.presentationTimeUs, wo, ho);
            copiarPlano(y, w, h, giro, salida, CABECERA);
            copiarPlano(u, cw, ch, giro, salida, CABECERA + wo * ho);
            copiarPlano(v, cw, ch, giro, salida, CABECERA + wo * ho + cwo * cho);
            pts = info.presentationTimeUs;
            anchoSalida = wo;
            altoSalida = ho;
        }

        // --- Sonido ---

        private boolean sacarSonido(int i) {
            MediaFormat f = formatoSalida();
            int canales = f.containsKey(MediaFormat.KEY_CHANNEL_COUNT) ? Math.max(1, f.getInteger(MediaFormat.KEY_CHANNEL_COUNT)) : 1;
            int tasa = f.containsKey(MediaFormat.KEY_SAMPLE_RATE) ? f.getInteger(MediaFormat.KEY_SAMPLE_RATE) : muestreo;
            int codificacion = f.containsKey(MediaFormat.KEY_PCM_ENCODING)
                    ? f.getInteger(MediaFormat.KEY_PCM_ENCODING) : AudioFormat.ENCODING_PCM_16BIT;
            ByteBuffer d = codec.getOutputBuffer(i).duplicate().order(ByteOrder.nativeOrder());
            d.limit(info.offset + info.size);
            d.position(info.offset);
            short[] mono = mezclar(d, canales, codificacion);
            long t = info.presentationTimeUs;
            if (tasa > 0 && t < objetivo) {
                // Lo de antes del punto buscado no suena: se recorta el trozo que lo cruza.
                long fuera = (objetivo - t) * tasa / 1_000_000L;
                if (fuera >= mono.length) {
                    return false;
                }
                mono = Arrays.copyOfRange(mono, (int) fuera, mono.length);
                t = objetivo;
            }
            short[] pcm = remuestreo.pasar(mono, tasa, muestreo);
            if (pcm.length == 0) {
                return false;
            }
            salida = new byte[CABECERA + 2 * pcm.length];
            cabecera(salida, SONIDO, t, pcm.length, 0);
            ByteBuffer.wrap(salida, CABECERA, 2 * pcm.length).asShortBuffer().put(pcm);
            pts = t;
            return true;
        }
    }

    // --- Utilidades, sin estado (las prueba VideoSistemaPrueba en la JVM) ---------------

    /**
     * Un plano de una imagen: su búfer, el índice (absoluto) de la muestra (0, 0) y sus pasos
     * de fila y de muestra.
     */
    static final class Plano {
        final ByteBuffer b;
        final int base, fila, pixel;

        Plano(ByteBuffer b, int base, int fila, int pixel) {
            this.b = b;
            this.base = base;
            this.fila = fila;
            this.pixel = Math.max(1, pixel);
        }

        static Plano de(Image.Plane p) {
            ByteBuffer b = p.getBuffer();
            return new Plano(b, b.position(), p.getRowStride(), p.getPixelStride());
        }

        /** El mismo plano empezando en la muestra (x, y): el recorte. */
        Plano desde(int x, int y) {
            return new Plano(b, base + y * fila + x * pixel, fila, pixel);
        }
    }

    static void cabecera(byte[] dst, int tipo, long us, int a, int b) {
        ByteBuffer.wrap(dst, 0, CABECERA).putInt(tipo).putLong(us).putInt(a).putInt(b);
    }

    /**
     * Copia las w×h muestras de `p` a `dst` desde `off`, sin relleno y giradas `giro` grados
     * en el sentido del reloj: con 90 o 270 el destino es de h×w.
     */
    static void copiarPlano(Plano p, int w, int h, int giro, byte[] dst, int off) {
        ByteBuffer b = p.b;
        int base = p.base;
        if (giro == 0 && p.pixel == 1) {
            ByteBuffer d = b.duplicate();
            for (int y = 0; y < h; y++) {
                d.limit(d.capacity());
                d.position(base + y * p.fila);
                d.get(dst, off + y * w, w);
            }
            return;
        }
        for (int y = 0; y < h; y++) {
            int fila = base + y * p.fila;
            for (int x = 0; x < w; x++) {
                byte s = b.get(fila + x * p.pixel);
                int i;
                switch (giro) {
                    case 90:
                        i = x * h + (h - 1 - y);
                        break;
                    case 180:
                        i = (h - 1 - y) * w + (w - 1 - x);
                        break;
                    case 270:
                        i = (w - 1 - x) * h + y;
                        break;
                    default:
                        i = y * w + x;
                        break;
                }
                dst[off + i] = s;
            }
        }
    }

    /** El PCM de `d` (16 bits o coma flotante, intercalado) en mono de 16 bits. */
    static short[] mezclar(ByteBuffer d, int canales, int codificacion) {
        if (codificacion == AudioFormat.ENCODING_PCM_FLOAT) {
            int n = d.remaining() / (4 * canales);
            short[] out = new short[n];
            for (int i = 0; i < n; i++) {
                float s = 0;
                for (int c = 0; c < canales; c++) {
                    s += d.getFloat();
                }
                s = Math.max(-1f, Math.min(1f, s / canales));
                out[i] = (short) Math.round(s * 32767f);
            }
            return out;
        }
        int n = d.remaining() / (2 * canales);
        short[] out = new short[n];
        for (int i = 0; i < n; i++) {
            int s = 0;
            for (int c = 0; c < canales; c++) {
                s += d.getShort();
            }
            out[i] = (short) (s / canales);
        }
        return out;
    }

    private static int entero(MediaFormat f, String clave, int porDefecto) {
        return f.containsKey(clave) ? f.getInteger(clave) : porDefecto;
    }

    /**
     * Pasa el PCM mono de un muestreo a otro, por interpolación lineal, sin cortes entre
     * trozos: recuerda la última muestra del trozo anterior y por dónde iba. La posición va
     * en enteros (en 1/`a` de muestra de entrada), así que por trozos sale lo mismo que de una
     * vez.
     */
    static final class Remuestreo {
        /** Dónde cae la muestra de salida siguiente en el trozo que llega, en 1/a de muestra (de -a a 0 si cae entre la previa y la primera). */
        private long pos;
        private int previa;
        private int tasa;

        void reiniciar() {
            pos = 0;
            previa = 0;
        }

        short[] pasar(short[] x, int de, int a) {
            if (de <= 0 || a <= 0 || de == a) {
                return x;
            }
            if (de != tasa) {
                reiniciar();
                tasa = de;
            }
            int n = x.length;
            if (n == 0) {
                return x;
            }
            long fin = (long) (n - 1) * a;
            short[] out = new short[pos > fin ? 0 : (int) ((fin - pos) / de) + 1];
            for (int k = 0; k < out.length; k++, pos += de) {
                long i = Math.floorDiv(pos, (long) a);
                long resto = pos - i * a;
                int s0 = i < 0 ? previa : x[(int) i];
                int s1 = i + 1 < n ? x[(int) i + 1] : s0;
                out[k] = (short) Math.round(s0 + (double) (s1 - s0) * resto / a);
            }
            pos -= (long) n * a;
            previa = x[n - 1];
            return out;
        }
    }
}
