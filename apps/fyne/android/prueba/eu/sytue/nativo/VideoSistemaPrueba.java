// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.graphics.Bitmap;
import android.graphics.Rect;
import android.media.AudioFormat;
import android.media.Image;
import android.media.MediaCodec;
import android.media.MediaCodecList;
import android.media.MediaExtractor;
import android.media.MediaFormat;
import android.media.MediaMetadataRetriever;
import android.os.Build;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/**
 * El vídeo del sistema de Android (VideoSistema) contra un MediaExtractor, un MediaCodec y un
 * MediaMetadataRetriever de mentira con la forma de los de Android (android/prueba/android):
 * que la imagen sale en I420 exacta desde planos con relleno, recorte y muestras intercaladas
 * (NV12), girada como dice la pista, también sin Image; que el sonido sale en mono al muestreo
 * pedido y sin cortes entre trozos; que las muestras salen en orden de tiempo; que buscar
 * descarta lo de antes; que lo que el teléfono no sabe da el código que hace que Go use lo de
 * hoy; que un códec roto llega a Go como excepción; que todo se suelta; y la miniatura. En la
 * JVM, antes de armar una APK.
 */
public final class VideoSistemaPrueba {
    private static int comprobaciones;

    private static void comprobar(boolean ok, String mensaje) {
        comprobaciones++;
        if (!ok) {
            throw new AssertionError(mensaje);
        }
    }

    // --- El vídeo de mentira -----------------------------------------------------------

    /** El fotograma n «de verdad», en coordenadas de lo visible y antes de girar. */
    static int luma(int n, int x, int y) {
        return (n * 7 + y * 16 + x) & 0xff;
    }

    static int azul(int n, int x, int y) {
        return (n + 64 + y * 4 + x) & 0xff;
    }

    static int rojo(int n, int x, int y) {
        return (n + 128 + y * 4 + x) & 0xff;
    }

    /** Cómo deja el descodificador de mentira cada fotograma en su búfer. */
    static final class Disposicion {
        final int ancho, alto, izquierda, arriba;
        final boolean semiplanar;
        final int codificadoAncho, codificadoAlto, paso, filas;

        Disposicion(int ancho, int alto, int izquierda, int arriba, boolean semiplanar) {
            this.ancho = ancho;
            this.alto = alto;
            this.izquierda = izquierda;
            this.arriba = arriba;
            this.semiplanar = semiplanar;
            codificadoAncho = izquierda + ancho + 2;
            codificadoAlto = arriba + alto + 2;
            paso = ((codificadoAncho + 3) / 4) * 4 + 4; // con relleno al final de cada fila
            filas = codificadoAlto + 2;
        }

        int croma() {
            return paso * filas;
        }

        /** El búfer del fotograma n, con basura (0xEE) en el relleno y fuera del recorte. */
        byte[] bufer(int n) {
            int tam = semiplanar ? croma() + paso * (filas / 2) : croma() + 2 * (paso / 2) * (filas / 2);
            byte[] b = new byte[tam];
            Arrays.fill(b, (byte) 0xEE);
            for (int y = 0; y < alto; y++) {
                for (int x = 0; x < ancho; x++) {
                    b[(arriba + y) * paso + izquierda + x] = (byte) luma(n, x, y);
                }
            }
            int cw = (ancho + 1) / 2, ch = (alto + 1) / 2;
            for (int y = 0; y < ch; y++) {
                for (int x = 0; x < cw; x++) {
                    int cy = arriba / 2 + y, cx = izquierda / 2 + x;
                    if (semiplanar) {
                        b[croma() + cy * paso + 2 * cx] = (byte) azul(n, x, y);
                        b[croma() + cy * paso + 2 * cx + 1] = (byte) rojo(n, x, y);
                    } else {
                        b[croma() + cy * (paso / 2) + cx] = (byte) azul(n, x, y);
                        b[croma() + (paso / 2) * (filas / 2) + cy * (paso / 2) + cx] = (byte) rojo(n, x, y);
                    }
                }
            }
            return b;
        }

        /** El Image de Android sobre ese búfer: los planos son vistas, y en NV12 se solapan. */
        Image imagen(byte[] b) {
            Image.Plane y = new Image.Plane(ByteBuffer.wrap(b), paso, 1);
            Image.Plane u, v;
            if (semiplanar) {
                u = new Image.Plane(ByteBuffer.wrap(b, croma(), b.length - croma()).slice(), paso, 2);
                v = new Image.Plane(ByteBuffer.wrap(b, croma() + 1, b.length - croma() - 1).slice(), paso, 2);
            } else {
                int q = (paso / 2) * (filas / 2);
                u = new Image.Plane(ByteBuffer.wrap(b, croma(), q).slice(), paso / 2, 1);
                v = new Image.Plane(ByteBuffer.wrap(b, croma() + q, q).slice(), paso / 2, 1);
            }
            return new Image(codificadoAncho, codificadoAlto,
                    new Rect(izquierda, arriba, izquierda + ancho, arriba + alto), new Image.Plane[] {y, u, v});
        }

        MediaFormat salida() {
            MediaFormat f = new MediaFormat();
            f.setInteger(MediaFormat.KEY_WIDTH, codificadoAncho);
            f.setInteger(MediaFormat.KEY_HEIGHT, codificadoAlto);
            f.setInteger(MediaFormat.KEY_COLOR_FORMAT, semiplanar ? 21 : 19);
            f.setInteger(MediaFormat.KEY_STRIDE, paso);
            f.setInteger(MediaFormat.KEY_SLICE_HEIGHT, filas);
            f.setInteger("crop-left", izquierda);
            f.setInteger("crop-top", arriba);
            f.setInteger("crop-right", izquierda + ancho - 1);
            f.setInteger("crop-bottom", arriba + alto - 1);
            return f;
        }
    }

    static byte[] entero(int n) {
        return ByteBuffer.allocate(4).putInt(n).array();
    }

    static int entero(byte[] b) {
        return ByteBuffer.wrap(b).getInt();
    }

    /** El descodificador de vídeo de mentira: la muestra lleva el número de fotograma. */
    static void descodificadorDeVideo(String mime, Disposicion d) {
        MediaCodecList.SABE.add(mime);
        MediaCodec.FALSOS.put(mime, new MediaCodec.Falso() {
            @Override
            public MediaFormat salida(MediaFormat entrada) {
                return d.salida();
            }

            @Override
            public MediaCodec.Salida descodificar(byte[] muestra, long pts) {
                byte[] b = d.bufer(entero(muestra));
                return new MediaCodec.Salida(pts, d.imagen(b), ByteBuffer.wrap(b));
            }
        });
    }

    /** Una muestra de sonido: el valor de cada muestra de cada canal. */
    static int sonido(long i, int canal) {
        return (int) (i % 1000) + 2 * canal;
    }

    /** El descodificador de sonido de mentira: la muestra lleva su índice y cuántas trae. */
    static void descodificadorDeSonido(String mime, int tasa, int canales, boolean flotante) {
        MediaCodecList.SABE.add(mime);
        MediaCodec.FALSOS.put(mime, new MediaCodec.Falso() {
            @Override
            public MediaFormat salida(MediaFormat entrada) {
                MediaFormat f = new MediaFormat();
                f.setInteger(MediaFormat.KEY_SAMPLE_RATE, tasa);
                f.setInteger(MediaFormat.KEY_CHANNEL_COUNT, canales);
                if (flotante) {
                    f.setInteger(MediaFormat.KEY_PCM_ENCODING, AudioFormat.ENCODING_PCM_FLOAT);
                }
                return f;
            }

            @Override
            public MediaCodec.Salida descodificar(byte[] muestra, long pts) {
                ByteBuffer m = ByteBuffer.wrap(muestra);
                long desde = m.getLong();
                int n = m.getInt();
                ByteBuffer b = ByteBuffer.allocate(n * canales * (flotante ? 4 : 2)).order(ByteOrder.nativeOrder());
                for (int i = 0; i < n; i++) {
                    for (int c = 0; c < canales; c++) {
                        int s = sonido(desde + i, c);
                        if (flotante) {
                            b.putFloat(s / 32768f);
                        } else {
                            b.putShort((short) s);
                        }
                    }
                }
                b.flip();
                return new MediaCodec.Salida(pts, null, b);
            }
        });
    }

    static MediaExtractor.Pista pistaDeVideo(String mime, int fotogramas, long cadaUs, int clave, int giro) {
        MediaFormat f = new MediaFormat();
        f.setString(MediaFormat.KEY_MIME, mime);
        f.setInteger(MediaFormat.KEY_WIDTH, 640);
        f.setInteger(MediaFormat.KEY_HEIGHT, 480);
        f.setLong(MediaFormat.KEY_DURATION, fotogramas * cadaUs);
        if (giro != 0) {
            f.setInteger(MediaFormat.KEY_ROTATION, giro);
        }
        MediaExtractor.Pista p = new MediaExtractor.Pista(f);
        for (int n = 0; n < fotogramas; n++) {
            p.muestras.add(new MediaExtractor.Muestra(n * cadaUs, entero(n), n % clave == 0));
        }
        return p;
    }

    static MediaExtractor.Pista pistaDeSonido(String mime, int tasa, int trozo, long duracionUs) {
        MediaFormat f = new MediaFormat();
        f.setString(MediaFormat.KEY_MIME, mime);
        f.setInteger(MediaFormat.KEY_SAMPLE_RATE, tasa);
        f.setLong(MediaFormat.KEY_DURATION, duracionUs);
        MediaExtractor.Pista p = new MediaExtractor.Pista(f);
        long total = duracionUs * tasa / 1_000_000L;
        for (long i = 0; i < total; i += trozo) {
            int n = (int) Math.min(trozo, total - i);
            byte[] m = ByteBuffer.allocate(12).putLong(i).putInt(n).array();
            p.muestras.add(new MediaExtractor.Muestra(i * 1_000_000L / tasa, m, true));
        }
        return p;
    }

    static void fichero(String ruta, MediaExtractor.Pista... pistas) {
        MediaExtractor.Fichero f = new MediaExtractor.Fichero();
        f.pistas.addAll(Arrays.asList(pistas));
        MediaExtractor.FICHEROS.put(ruta, f);
    }

    // --- Leer lo que sale --------------------------------------------------------------

    /** Una muestra de las que da siguiente(), leída como la lee Go. */
    static final class Muestra {
        final int tipo, a, b;
        final long us;
        final byte[] datos;

        Muestra(byte[] crudo) {
            ByteBuffer c = ByteBuffer.wrap(crudo);
            tipo = c.getInt();
            us = c.getLong();
            a = c.getInt();
            b = c.getInt();
            datos = Arrays.copyOfRange(crudo, VideoSistema.CABECERA, crudo.length);
        }

        short[] pcm() {
            short[] s = new short[a];
            ByteBuffer.wrap(datos).asShortBuffer().get(s);
            return s;
        }
    }

    static List<Muestra> todas(int id) throws Exception {
        List<Muestra> out = new ArrayList<>();
        for (byte[] b; (b = VideoSistema.siguiente(id)) != null; ) {
            out.add(new Muestra(b));
            comprobar(out.size() < 100_000, "siguiente() no acaba");
        }
        return out;
    }

    /** El I420 que tiene que salir del fotograma n, girado: se mira de dónde viene cada muestra. */
    static byte[] esperado(int n, int w, int h, int giro) {
        boolean tumbado = giro % 180 != 0;
        int wo = tumbado ? h : w, ho = tumbado ? w : h;
        int cw = (w + 1) / 2, ch = (h + 1) / 2, cwo = (wo + 1) / 2, cho = (ho + 1) / 2;
        byte[] out = new byte[wo * ho + 2 * cwo * cho];
        for (int plano = 0; plano < 3; plano++) {
            int pw = plano == 0 ? w : cw, ph = plano == 0 ? h : ch;
            int qw = plano == 0 ? wo : cwo, qh = plano == 0 ? ho : cho;
            int off = plano == 0 ? 0 : plano == 1 ? wo * ho : wo * ho + cwo * cho;
            for (int y = 0; y < qh; y++) {
                for (int x = 0; x < qw; x++) {
                    int sx, sy;
                    switch (giro) {
                        case 90:
                            sx = y;
                            sy = ph - 1 - x;
                            break;
                        case 180:
                            sx = pw - 1 - x;
                            sy = ph - 1 - y;
                            break;
                        case 270:
                            sx = pw - 1 - y;
                            sy = x;
                            break;
                        default:
                            sx = x;
                            sy = y;
                    }
                    int v = plano == 0 ? luma(n, sx, sy) : plano == 1 ? azul(n, sx, sy) : rojo(n, sx, sy);
                    out[off + y * qw + x] = (byte) v;
                }
            }
        }
        return out;
    }

    static void limpiar() {
        MediaExtractor.FICHEROS.clear();
        MediaCodec.FALSOS.clear();
        MediaCodecList.SABE.clear();
        MediaCodec.retraso = 2;
        MediaCodec.fallarTras = -1;
        MediaCodec.sinImagen = false;
        MediaCodec.finConDatos = false;
    }

    static void todoSuelto(String que) {
        comprobar(VideoSistema.abiertos() == 0, que + ": quedan vídeos abiertos");
        comprobar(MediaExtractor.vivos == 0, que + ": quedan " + MediaExtractor.vivos + " extractores sin soltar");
        comprobar(MediaCodec.vivos == 0, que + ": quedan " + MediaCodec.vivos + " códecs sin soltar");
    }

    // --- Las pruebas -------------------------------------------------------------------

    public static void main(String[] args) throws Exception {
        VideoSistema.maxEsperas = 20;
        peliculaEntera();
        giros();
        sinImagenComoConImagen();
        sonidoRemuestreado();
        loQueNoSabe();
        codecRoto();
        miniatura();
        System.out.println("VideoSistemaPrueba: " + comprobaciones + " comprobaciones");
    }

    /** MP4 de 2 s: 50 fotogramas de 6×4 con recorte y 100 trozos de sonido estéreo a 48 kHz. */
    static void peliculaEntera() throws Exception {
        limpiar();
        Disposicion d = new Disposicion(6, 4, 2, 2, false);
        descodificadorDeVideo("video/avc", d);
        descodificadorDeSonido("audio/mp4a-latm", 48000, 2, false);
        fichero("/v/peli.mp4", pistaDeVideo("video/avc", 50, 40_000, 10, 0),
                pistaDeSonido("audio/mp4a-latm", 48000, 960, 2_000_000));
        for (boolean conDatos : new boolean[] {false, true}) {
            MediaCodec.finConDatos = conDatos;
            int id = VideoSistema.abrir("/v/peli.mp4", 48000);
            comprobar(id > 0, "abrir: " + id);
            comprobar("2000000\t6\t4\t1\t1".equals(VideoSistema.info(id)), "info: " + VideoSistema.info(id));
            List<Muestra> ms = todas(id);
            int fotogramas = 0, sonidos = 0;
            long antes = Long.MIN_VALUE;
            for (Muestra m : ms) {
                comprobar(m.us >= antes, "fuera de orden: " + m.us + " tras " + antes);
                antes = m.us;
                if (m.tipo == VideoSistema.IMAGEN) {
                    comprobar(m.a == 6 && m.b == 4, "tamaño: " + m.a + "×" + m.b);
                    comprobar(m.us == fotogramas * 40_000L, "instante del fotograma " + fotogramas + ": " + m.us);
                    comprobar(Arrays.equals(esperado(fotogramas, 6, 4, 0), m.datos), "el I420 del fotograma " + fotogramas);
                    fotogramas++;
                } else {
                    comprobar(m.tipo == VideoSistema.SONIDO, "tipo " + m.tipo);
                    short[] pcm = m.pcm();
                    for (int i = 0; i < pcm.length; i++) {
                        // Mono: la media de los dos canales (s y s + 2).
                        comprobar(pcm[i] == sonido(sonidos + i, 0) + 1, "sonido " + (sonidos + i) + ": " + pcm[i]);
                    }
                    comprobar(m.us == sonidos * 1_000_000L / 48000, "instante del sonido: " + m.us);
                    sonidos += pcm.length;
                }
            }
            comprobar(fotogramas == 50, "fotogramas: " + fotogramas);
            comprobar(sonidos == 96000, "muestras de sonido: " + sonidos);
            comprobar(VideoSistema.siguiente(id) == null, "tras acabar, sigue acabado");

            // Buscar 1,01 s: el vídeo vuelve a la clave de 0,8 s y descarta hasta el de 1,04 s;
            // el sonido empieza justo en 1,01 s (el trozo de 1 s, recortado).
            comprobar(VideoSistema.buscar(id, 1010) == 1010, "buscar");
            ms = todas(id);
            List<Muestra> imagenes = new ArrayList<>(), sonido = new ArrayList<>();
            for (Muestra m : ms) {
                (m.tipo == VideoSistema.IMAGEN ? imagenes : sonido).add(m);
            }
            comprobar(imagenes.size() == 24, "fotogramas tras buscar: " + imagenes.size());
            comprobar(imagenes.get(0).us == 1_040_000, "primer fotograma tras buscar: " + imagenes.get(0).us);
            comprobar(Arrays.equals(esperado(26, 6, 4, 0), imagenes.get(0).datos), "el fotograma 26");
            comprobar(sonido.get(0).us == 1_010_000, "primer sonido tras buscar: " + sonido.get(0).us);
            comprobar(sonido.get(0).pcm()[0] == sonido(48480, 0) + 1, "la primera muestra tras buscar");
            int tras = 0;
            for (Muestra m : sonido) {
                tras += m.a;
            }
            comprobar(tras == 96000 - 48480, "muestras tras buscar: " + tras);

            // Buscar al principio otra vez, y cerrar a medias.
            comprobar(VideoSistema.buscar(id, 0) == 0, "buscar 0");
            Muestra primera = new Muestra(VideoSistema.siguiente(id));
            comprobar(primera.tipo == VideoSistema.IMAGEN && primera.us == 0, "tras volver al principio: " + primera.us);
            VideoSistema.cerrar(id);
            todoSuelto("película");
            comprobar("".equals(VideoSistema.info(id)), "info de uno cerrado");
            comprobar(VideoSistema.buscar(id, 0) == -1, "buscar en uno cerrado");
            try {
                VideoSistema.siguiente(id);
                comprobar(false, "siguiente() de uno cerrado");
            } catch (IllegalStateException e) {
                comprobar(e.getMessage().contains("no está abierto"), "el error de uno cerrado: " + e);
            }
            VideoSistema.cerrar(id); // dos veces no rompe nada
        }
        comprobar(Image.cerradas > 0, "los Image se cierran");
    }

    /** El giro de la pista: el fotograma sale derecho, con el NV12 y un tamaño impar. */
    static void giros() throws Exception {
        for (int giro : new int[] {90, 180, 270, -90}) {
            limpiar();
            Disposicion d = new Disposicion(5, 3, 2, 0, true);
            descodificadorDeVideo("video/hevc", d);
            fichero("/v/movil.mp4", pistaDeVideo("video/hevc", 3, 33_333, 1, giro));
            int id = VideoSistema.abrir("/v/movil.mp4", 48000);
            comprobar(id > 0, "abrir girado " + giro + ": " + id);
            int g = (giro + 360) % 360;
            String tam = g % 180 == 0 ? "5\t3" : "3\t5";
            comprobar(("99999\t" + tam + "\t1\t0").equals(VideoSistema.info(id)), "info girado " + giro + ": " + VideoSistema.info(id));
            List<Muestra> ms = todas(id);
            comprobar(ms.size() == 3, "fotogramas girados: " + ms.size());
            for (int n = 0; n < 3; n++) {
                comprobar(Arrays.equals(esperado(n, 5, 3, g), ms.get(n).datos), "fotograma " + n + " girado " + giro);
            }
            VideoSistema.cerrar(id);
            todoSuelto("girado " + giro);
        }
    }

    /** Sin getOutputImage (un descodificador viejo), del búfer con su formato de color. */
    static void sinImagenComoConImagen() throws Exception {
        for (boolean semiplanar : new boolean[] {false, true}) {
            limpiar();
            MediaCodec.sinImagen = true;
            Disposicion d = new Disposicion(6, 4, 2, 2, semiplanar);
            descodificadorDeVideo("video/avc", d);
            fichero("/v/viejo.mp4", pistaDeVideo("video/avc", 4, 40_000, 2, 90));
            int id = VideoSistema.abrir("/v/viejo.mp4", 48000);
            comprobar(id > 0, "abrir sin Image: " + id);
            List<Muestra> ms = todas(id);
            comprobar(ms.size() == 4, "fotogramas sin Image: " + ms.size());
            for (int n = 0; n < 4; n++) {
                comprobar(Arrays.equals(esperado(n, 6, 4, 90), ms.get(n).datos),
                        "fotograma " + n + " sin Image (" + (semiplanar ? "NV12" : "I420") + ")");
            }
            VideoSistema.cerrar(id);
            todoSuelto("sin Image");
        }
    }

    /** 44,1 kHz estéreo en coma flotante, sin vídeo, a 48 kHz mono. */
    static void sonidoRemuestreado() throws Exception {
        limpiar();
        descodificadorDeSonido("audio/opus", 44100, 2, true);
        fichero("/v/cancion.m4a", pistaDeSonido("audio/opus", 44100, 1024, 1_000_000));
        int id = VideoSistema.abrir("/v/cancion.m4a", 48000);
        comprobar(id > 0, "abrir solo sonido: " + id);
        comprobar("1000000\t0\t0\t0\t1".equals(VideoSistema.info(id)), "info solo sonido: " + VideoSistema.info(id));
        int total = 0;
        for (Muestra m : todas(id)) {
            comprobar(m.tipo == VideoSistema.SONIDO, "solo sonido");
            for (short s : m.pcm()) {
                comprobar(s >= 0 && s <= 1001, "muestra fuera de lo que entra: " + s);
            }
            total += m.a;
        }
        comprobar(Math.abs(total - 48000) <= 2, "muestras a 48 kHz: " + total);
        VideoSistema.cerrar(id);
        todoSuelto("solo sonido");

        // El remuestreo por trozos da lo mismo que de una vez (sin cortes entre trozos).
        short[] rampa = new short[10_000];
        for (int i = 0; i < rampa.length; i++) {
            rampa[i] = (short) ((i * 37) % 20000 - 10000);
        }
        short[] entera = new VideoSistema.Remuestreo().pasar(rampa, 44100, 48000);
        VideoSistema.Remuestreo r = new VideoSistema.Remuestreo();
        short[] trozos = new short[0];
        int[] cortes = {0, 1, 2, 100, 1023, 1024, 4000, 9999, 10_000};
        for (int k = 0; k + 1 < cortes.length; k++) {
            short[] t = r.pasar(Arrays.copyOfRange(rampa, cortes[k], cortes[k + 1]), 44100, 48000);
            short[] junta = Arrays.copyOf(trozos, trozos.length + t.length);
            System.arraycopy(t, 0, junta, trozos.length, t.length);
            trozos = junta;
        }
        comprobar(Arrays.equals(entera, trozos), "remuestrear por trozos (" + trozos.length + ") y de una vez (" + entera.length + ")");
        comprobar(Math.abs(entera.length - 10_000 * 48000 / 44100) <= 1, "cuántas a 48 kHz: " + entera.length);
        comprobar(new VideoSistema.Remuestreo().pasar(rampa, 48000, 48000) == rampa, "al mismo muestreo, tal cual");

        // Coma flotante: la media, recortada a 16 bits.
        ByteBuffer f = ByteBuffer.allocate(4 * 6).order(ByteOrder.nativeOrder());
        f.putFloat(0.5f).putFloat(-0.5f).putFloat(1f).putFloat(1f).putFloat(2f).putFloat(2f).flip();
        short[] m = VideoSistema.mezclar(f, 2, AudioFormat.ENCODING_PCM_FLOAT);
        comprobar(Arrays.equals(new short[] {0, 32767, 32767}, m), "flotante: " + Arrays.toString(m));
    }

    /** Lo que el teléfono no sabe da el código que hace que Go siga como hoy. */
    static void loQueNoSabe() throws Exception {
        limpiar();
        comprobar(VideoSistema.abrir("/no/existe.mp4", 48000) == VideoSistema.NO_LEE, "un fichero que no se lee");
        todoSuelto("no existe");

        MediaFormat subtitulos = new MediaFormat();
        subtitulos.setString(MediaFormat.KEY_MIME, "text/vtt");
        fichero("/v/subtitulos.mkv", new MediaExtractor.Pista(subtitulos));
        comprobar(VideoSistema.abrir("/v/subtitulos.mkv", 48000) == VideoSistema.SIN_PISTAS, "sin vídeo ni sonido");
        todoSuelto("sin pistas");

        // HEVC en un teléfono sin descodificador de HEVC.
        descodificadorDeSonido("audio/mp4a-latm", 48000, 1, false);
        fichero("/v/hevc.mp4", pistaDeVideo("video/hevc", 10, 40_000, 5, 0),
                pistaDeSonido("audio/mp4a-latm", 48000, 960, 400_000));
        comprobar(VideoSistema.abrir("/v/hevc.mp4", 48000) == VideoSistema.SIN_CODEC, "sin descodificador de vídeo");
        todoSuelto("sin HEVC");

        // Lo sabe según la lista, pero no se deja configurar.
        MediaCodecList.SABE.add("video/av01");
        fichero("/v/av1.mp4", pistaDeVideo("video/av01", 10, 40_000, 5, 0));
        comprobar(VideoSistema.abrir("/v/av1.mp4", 48000) == VideoSistema.SIN_CODEC, "un códec que no se configura");
        todoSuelto("AV1");

        // Un sonido que no sabe (AC-3): el vídeo se ve sin él.
        descodificadorDeVideo("video/avc", new Disposicion(4, 2, 0, 0, false));
        fichero("/v/ac3.mp4", pistaDeVideo("video/avc", 5, 40_000, 5, 0), pistaDeSonido("audio/ac3", 48000, 1536, 200_000));
        int id = VideoSistema.abrir("/v/ac3.mp4", 48000);
        comprobar(id > 0, "vídeo con un sonido que no sabe: " + id);
        comprobar("200000\t4\t2\t1\t0".equals(VideoSistema.info(id)), "sin el sonido: " + VideoSistema.info(id));
        comprobar(todas(id).size() == 5, "los fotogramas, sin el sonido");
        VideoSistema.cerrar(id);
        todoSuelto("AC-3");

        // Solo un sonido que no sabe: no se abre.
        fichero("/v/ac3.ac3", pistaDeSonido("audio/ac3", 48000, 1536, 200_000));
        comprobar(VideoSistema.abrir("/v/ac3.ac3", 48000) == VideoSistema.SIN_CODEC, "solo un sonido que no sabe");
        todoSuelto("solo AC-3");

        // Un descodificador que no da ningún fotograma.
        MediaCodecList.SABE.add("video/mp4v-es");
        MediaCodec.FALSOS.put("video/mp4v-es", new MediaCodec.Falso() {
            @Override
            public MediaFormat salida(MediaFormat entrada) {
                return new MediaFormat();
            }

            @Override
            public MediaCodec.Salida descodificar(byte[] muestra, long pts) {
                return null;
            }
        });
        fichero("/v/mudo.mp4", pistaDeVideo("video/mp4v-es", 10, 40_000, 5, 0));
        comprobar(VideoSistema.abrir("/v/mudo.mp4", 48000) == VideoSistema.FALLO, "un descodificador que no da nada");
        todoSuelto("mudo");

        // Uno atascado: ni fotogramas ni fin; se da por perdido tras sus esperas.
        MediaCodec.retraso = 1_000_000;
        fichero("/v/atascado.mp4", pistaDeVideo("video/avc", 10, 40_000, 5, 0));
        long t0 = System.nanoTime();
        comprobar(VideoSistema.abrir("/v/atascado.mp4", 48000) == VideoSistema.FALLO, "un descodificador atascado");
        comprobar(System.nanoTime() - t0 < 10_000_000_000L, "el atascado no espera para siempre");
        todoSuelto("atascado");
    }

    /** Un códec que se rompe a medias: Go recibe la excepción. */
    static void codecRoto() throws Exception {
        limpiar();
        descodificadorDeVideo("video/avc", new Disposicion(4, 2, 0, 0, false));
        fichero("/v/roto.mp4", pistaDeVideo("video/avc", 20, 40_000, 5, 0));
        MediaCodec.fallarTras = 3;
        int id = VideoSistema.abrir("/v/roto.mp4", 48000);
        comprobar(id > 0, "abrir el que se rompe: " + id);
        int vistas = 0;
        try {
            while (VideoSistema.siguiente(id) != null) {
                vistas++;
            }
            comprobar(false, "un códec roto no puede acabar bien");
        } catch (IllegalStateException e) {
            comprobar(e.getMessage().contains("roto"), "la excepción del códec, tal cual: " + e);
        }
        comprobar(vistas == 3, "lo que dio antes de romperse: " + vistas);
        VideoSistema.cerrar(id);
        todoSuelto("roto");
    }

    static String texto(byte[] b) {
        return b == null ? null : new String(b, StandardCharsets.UTF_8);
    }

    static void miniatura() {
        MediaMetadataRetriever.VIDEOS.put("/m/largo.mp4", new long[] {5000, 1920, 1080, 0});
        MediaMetadataRetriever.VIDEOS.put("/m/corto.mp4", new long[] {2000, 640, 360, 0});
        MediaMetadataRetriever.VIDEOS.put("/m/negro.mp4", new long[] {5000, 1280, 720, 2_000_000});
        MediaMetadataRetriever.VIDEOS.put("/m/raro.mp4", new long[] {-1, 100, 50, 0});

        Build.VERSION.SDK_INT = 36;
        MediaMetadataRetriever.llamadas.setLength(0);
        String s = texto(VideoSistema.miniatura("/m/largo.mp4", 512));
        comprobar("FALSO JPEG 512x288 q85".equals(s), "miniatura de uno largo: " + s);
        comprobar(MediaMetadataRetriever.llamadas.toString().equals("getScaledFrameAtTime(1000000,2,512,512) "),
                "a un segundo y escalada por el sistema: " + MediaMetadataRetriever.llamadas);
        MediaMetadataRetriever.llamadas.setLength(0);
        s = texto(VideoSistema.miniatura("/m/corto.mp4", 512));
        comprobar("FALSO JPEG 512x288 q85".equals(s), "miniatura de uno corto: " + s);
        comprobar(MediaMetadataRetriever.llamadas.toString().startsWith("getScaledFrameAtTime(0,"),
                "uno corto, del principio: " + MediaMetadataRetriever.llamadas);
        s = texto(VideoSistema.miniatura("/m/raro.mp4", 512));
        comprobar("FALSO JPEG 100x50 q85".equals(s), "sin duración y pequeño, tal cual: " + s);
        MediaMetadataRetriever.llamadas.setLength(0);
        comprobar(VideoSistema.miniatura("/m/negro.mp4", 512) == null, "sin fotograma, sin miniatura");
        comprobar(MediaMetadataRetriever.llamadas.toString().contains("(1000000,") &&
                MediaMetadataRetriever.llamadas.toString().contains("(0,"), "prueba a 1 s y luego al principio");
        comprobar(VideoSistema.miniatura("/no/existe.mp4", 512) == null, "un fichero que no está");

        // Antes de la API 27 no hay getScaledFrameAtTime: se escala aquí.
        Build.VERSION.SDK_INT = 26;
        MediaMetadataRetriever.llamadas.setLength(0);
        s = texto(VideoSistema.miniatura("/m/largo.mp4", 512));
        comprobar("FALSO JPEG 512x288 q85".equals(s), "miniatura en la API 26: " + s);
        comprobar(MediaMetadataRetriever.llamadas.toString().equals("getFrameAtTime(1000000,2) "),
                "en la API 26, getFrameAtTime: " + MediaMetadataRetriever.llamadas);
        Build.VERSION.SDK_INT = 36;

        comprobar(Bitmap.vivos == 0, "quedan " + Bitmap.vivos + " Bitmap sin reciclar");
        comprobar(MediaMetadataRetriever.vivos == 0, "quedan " + MediaMetadataRetriever.vivos + " MediaMetadataRetriever sin soltar");
    }
}
