// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.Context;
import android.content.Intent;
import android.graphics.PixelFormat;
import android.hardware.display.DisplayManager;
import android.hardware.display.VirtualDisplay;
import android.media.Image;
import android.media.ImageReader;
import android.media.projection.MediaProjection;
import android.os.Handler;
import android.os.HandlerThread;
import android.util.DisplayMetrics;
import java.nio.ByteBuffer;

/**
 * Compartir la pantalla en la llamada nativa de Rooms (`apps/fyne/llamada/camara`), con
 * MediaProjection. Android pide el consentimiento en su propio diálogo, que solo se puede
 * abrir desde una actividad ({@link PantallaPermiso}); y desde Android 14 la proyección
 * tiene que ir dentro de un servicio en primer plano de tipo mediaProjection
 * ({@link PantallaServicio}), con su aviso fijo mientras se comparte. Go LEE el último
 * fotograma con {@link #leer}, como con la cámara: nada llama a Go desde aquí.
 */
public final class Pantalla {
    /** Estados: pedido y sin respuesta, compartiendo, o negado o terminado. */
    static final int PEDIDO = 0, COMPARTIENDO = 1, NEGADO = -1;

    /** Lo más largo que se manda: la pantalla se escala a caber en esto. */
    private static final int LADO_MAXIMO = 1280;

    private static int estado = NEGADO;
    private static MediaProjection proyeccion;
    private static VirtualDisplay pantalla;
    private static ImageReader lector;
    private static HandlerThread hilo;
    private static byte[] ultimo;
    private static int ancho, alto;
    private static long secuencia, leida;

    private Pantalla() {}

    /**
     * Abre el diálogo de Android; la respuesta llega a {@link #estado}. `aviso` es el texto,
     * ya traducido, del aviso fijo mientras se comparte.
     */
    public static synchronized void pedir(Context ctx, final String aviso) {
        cerrar(ctx);
        // El diálogo solo se abre desde una actividad: la de la app es la que llega aquí.
        if (!(ctx instanceof Activity)) {
            estado = NEGADO;
            return;
        }
        estado = PEDIDO;
        final Activity a = (Activity) ctx;
        a.runOnUiThread(new Runnable() {
            @Override
            public void run() {
                PantallaPermiso.pedir(a, aviso);
            }
        });
    }

    public static synchronized int estado() {
        return estado;
    }

    static synchronized void negado() {
        estado = NEGADO;
    }

    /** Lo llama el servicio ya en primer plano con la proyección concedida. */
    static synchronized void empezar(Context ctx, MediaProjection p) {
        proyeccion = p;
        DisplayMetrics dm = ctx.getResources().getDisplayMetrics();
        float escala = Math.min(1f, (float) LADO_MAXIMO / Math.max(dm.widthPixels, dm.heightPixels));
        int w = Math.max(2, Math.round(dm.widthPixels * escala)) & ~1;
        int h = Math.max(2, Math.round(dm.heightPixels * escala)) & ~1;
        hilo = new HandlerThread("sytue-pantalla");
        hilo.start();
        Handler manejador = new Handler(hilo.getLooper());
        // Android 14 exige el aviso de parada ANTES de crear la pantalla virtual.
        p.registerCallback(new MediaProjection.Callback() {
            @Override
            public void onStop() {
                synchronized (Pantalla.class) {
                    estado = NEGADO;
                }
            }
        }, manejador);
        lector = ImageReader.newInstance(w, h, PixelFormat.RGBA_8888, 2);
        lector.setOnImageAvailableListener(r -> {
            Image img = r.acquireLatestImage();
            if (img != null) {
                copiar(img);
                img.close();
            }
        }, manejador);
        pantalla = p.createVirtualDisplay("sytue-pantalla", w, h, dm.densityDpi,
                DisplayManager.VIRTUAL_DISPLAY_FLAG_AUTO_MIRROR, lector.getSurface(), null, manejador);
        estado = COMPARTIENDO;
    }

    /** RGBA con su paso de fila a RGBA empaquetado. */
    private static synchronized void copiar(Image img) {
        Image.Plane p = img.getPlanes()[0];
        int w = img.getWidth(), h = img.getHeight();
        int n = w * h * 4;
        if (ultimo == null || ultimo.length != n) {
            ultimo = new byte[n];
        }
        ByteBuffer b = p.getBuffer();
        int paso = p.getRowStride();
        for (int f = 0; f < h; f++) {
            b.position(f * paso);
            b.get(ultimo, f * w * 4, w * 4);
        }
        ancho = w;
        alto = h;
        secuencia++;
    }

    /**
     * El último fotograma si es nuevo: 8 bytes con el ancho y el alto (big-endian) y luego
     * el RGBA. null si no hay uno nuevo.
     */
    public static synchronized byte[] leer() {
        if (ultimo == null || secuencia == leida) {
            return null;
        }
        leida = secuencia;
        byte[] out = new byte[8 + ultimo.length];
        ByteBuffer.wrap(out).putInt(ancho).putInt(alto);
        System.arraycopy(ultimo, 0, out, 8, ultimo.length);
        return out;
    }

    /** Deja de compartir: la pantalla virtual, la proyección y el servicio con su aviso. */
    public static synchronized void cerrar(Context ctx) {
        if (pantalla != null) {
            pantalla.release();
            pantalla = null;
        }
        if (proyeccion != null) {
            proyeccion.stop();
            proyeccion = null;
        }
        if (lector != null) {
            lector.close();
            lector = null;
        }
        if (hilo != null) {
            hilo.quitSafely();
            hilo = null;
        }
        ultimo = null;
        estado = NEGADO;
        ctx.stopService(new Intent(ctx, PantallaServicio.class));
    }
}
