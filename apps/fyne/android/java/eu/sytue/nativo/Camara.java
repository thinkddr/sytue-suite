// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.Manifest;
import android.app.Activity;
import android.content.Context;
import android.content.pm.PackageManager;
import android.graphics.ImageFormat;
import android.hardware.camera2.CameraCaptureSession;
import android.hardware.camera2.CameraCharacteristics;
import android.hardware.camera2.CameraDevice;
import android.hardware.camera2.CameraManager;
import android.hardware.camera2.CaptureRequest;
import android.hardware.camera2.params.StreamConfigurationMap;
import android.media.Image;
import android.media.ImageReader;
import android.os.Build;
import android.view.Display;
import android.view.WindowManager;
import android.os.Handler;
import android.os.HandlerThread;
import android.util.Range;
import android.util.Size;
import java.nio.ByteBuffer;
import java.util.Collections;

/**
 * La cámara de la llamada nativa de Rooms (`apps/fyne/llamada/camara`): Camera2 con un
 * ImageReader en YUV_420_888 que deja el último fotograma en I420. Go lo LEE a su ritmo
 * con {@link #leer}; nada llama a Go desde aquí.
 */
public final class Camara {
    private static HandlerThread hilo;
    private static Handler manejador;
    private static CameraDevice dispositivo;
    private static CameraCaptureSession sesion;
    private static ImageReader lector;
    private static byte[] ultimo;
    private static int ancho, alto;
    private static long secuencia, leida;
    // Para poner derecho lo que da el sensor: su giro, si es la delantera y la actividad
    // de la que se lee cómo está girado el teléfono.
    private static int giroSensor;
    private static boolean delantera;
    private static Context actividad;
    private static int rotacion;

    private Camara() {}

    /**
     * 1 concedido; 0 pedido ahora (la respuesta llega sola, se reintenta al encender). Por
     * debajo de la API 23 los permisos se conceden al instalar: el mínimo de la APK es 21.
     */
    public static int permiso(Context ctx) {
        if (Build.VERSION.SDK_INT < 23
                || ctx.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
            return 1;
        }
        if (ctx instanceof Activity) {
            ((Activity) ctx).requestPermissions(new String[] {Manifest.permission.CAMERA}, 7415);
        }
        return 0;
    }

    /** Si ya se puede usar la cámara, SIN pedirlo: para esperar la respuesta del diálogo. */
    public static boolean concedido(Context ctx) {
        return Build.VERSION.SDK_INT < 23
                || ctx.checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED;
    }

    /** «id\tnombre\n» por cámara: delantera o trasera, que es lo que distingue en un teléfono. */
    public static String lista(Context ctx) {
        StringBuilder s = new StringBuilder();
        try {
            CameraManager m = (CameraManager) ctx.getSystemService(Context.CAMERA_SERVICE);
            for (String id : m.getCameraIdList()) {
                Integer lado = m.getCameraCharacteristics(id).get(CameraCharacteristics.LENS_FACING);
                String nombre = lado != null && lado == CameraCharacteristics.LENS_FACING_FRONT ? "front"
                        : lado != null && lado == CameraCharacteristics.LENS_FACING_BACK ? "back" : "external";
                s.append(id).append('\t').append(nombre).append('\n');
            }
        } catch (Exception e) {
            // Sin cámara o sin servicio: lista vacía.
        }
        return s.toString();
    }

    /** Abre la cámara; 0 si ha empezado, -1 si no hay, -2 sin permiso. La apertura es asíncrona. */
    public static synchronized int abrir(Context ctx, String id, int w, int h, final int fps) {
        if (permiso(ctx) != 1) {
            return -2;
        }
        cerrar();
        try {
            CameraManager m = (CameraManager) ctx.getSystemService(Context.CAMERA_SERVICE);
            if (id == null || id.isEmpty()) {
                id = null;
                for (String c : m.getCameraIdList()) {
                    Integer lado = m.getCameraCharacteristics(c).get(CameraCharacteristics.LENS_FACING);
                    if (id == null || (lado != null && lado == CameraCharacteristics.LENS_FACING_FRONT)) {
                        id = c; // la delantera, que es la de una videollamada
                    }
                }
                if (id == null) {
                    return -1;
                }
            }
            CameraCharacteristics car = m.getCameraCharacteristics(id);
            Integer giro = car.get(CameraCharacteristics.SENSOR_ORIENTATION);
            Integer lado = car.get(CameraCharacteristics.LENS_FACING);
            giroSensor = giro == null ? 0 : giro;
            delantera = lado != null && lado == CameraCharacteristics.LENS_FACING_FRONT;
            actividad = ctx;
            StreamConfigurationMap mapa = car.get(CameraCharacteristics.SCALER_STREAM_CONFIGURATION_MAP);
            Size elegida = new Size(640, 480);
            long mejor = Long.MAX_VALUE;
            if (mapa != null) {
                for (Size s : mapa.getOutputSizes(ImageFormat.YUV_420_888)) {
                    long d = Math.abs((long) s.getWidth() * s.getHeight() - (long) w * h);
                    if (d < mejor) {
                        mejor = d;
                        elegida = s;
                    }
                }
            }
            hilo = new HandlerThread("sytue-camara");
            hilo.start();
            manejador = new Handler(hilo.getLooper());
            lector = ImageReader.newInstance(elegida.getWidth(), elegida.getHeight(), ImageFormat.YUV_420_888, 3);
            // Bajo el cerrojo de la clase y solo si este lector sigue siendo EL lector:
            // cambiar de cámara (cerrar) cerraba el lector con un fotograma a medio copiar
            // en este hilo, y «Image is already closed» tumbaba la app entera (visto en el
            // emulador al pasar a la trasera en Cast, 2026-09-28).
            lector.setOnImageAvailableListener(r -> {
                synchronized (Camara.class) {
                    if (r != lector) {
                        return;
                    }
                    try (Image img = r.acquireLatestImage()) {
                        if (img != null) {
                            copiar(img);
                        }
                    } catch (IllegalStateException e) {
                        // El lector se cerró entre medias: ese fotograma se pierde y ya.
                    }
                }
            }, manejador);
            m.openCamera(id, new CameraDevice.StateCallback() {
                @Override
                public void onOpened(CameraDevice d) {
                    synchronized (Camara.class) {
                        dispositivo = d;
                        empezar(fps);
                    }
                }

                @Override
                public void onDisconnected(CameraDevice d) {
                    d.close();
                }

                @Override
                public void onError(CameraDevice d, int error) {
                    d.close();
                }
            }, manejador);
            return 0;
        } catch (SecurityException e) {
            return -2;
        } catch (Exception e) {
            return -1;
        }
    }

    @SuppressWarnings("deprecation") // createCaptureSession(List, …): la de API 24, que es el mínimo
    private static void empezar(final int fps) {
        try {
            final CaptureRequest.Builder b = dispositivo.createCaptureRequest(CameraDevice.TEMPLATE_RECORD);
            b.addTarget(lector.getSurface());
            if (fps > 0) {
                b.set(CaptureRequest.CONTROL_AE_TARGET_FPS_RANGE, new Range<>(fps, fps));
            }
            dispositivo.createCaptureSession(Collections.singletonList(lector.getSurface()),
                    new CameraCaptureSession.StateCallback() {
                        @Override
                        public void onConfigured(CameraCaptureSession s) {
                            synchronized (Camara.class) {
                                sesion = s;
                                try {
                                    s.setRepeatingRequest(b.build(), null, manejador);
                                } catch (Exception e) {
                                    // La cámara se cerró mientras se configuraba.
                                }
                            }
                        }

                        @Override
                        public void onConfigureFailed(CameraCaptureSession s) {}
                    }, manejador);
        } catch (Exception e) {
            // Sin sesión: Leer no dará fotogramas y la llamada sigue sin cámara.
        }
    }

    /** YUV_420_888 a I420 empaquetado, respetando los pasos de fila y de píxel de cada plano. */
    private static synchronized void copiar(Image img) {
        int w = img.getWidth() & ~1, h = img.getHeight() & ~1;
        int n = w * h * 3 / 2;
        if (ultimo == null || ultimo.length != n) {
            ultimo = new byte[n];
        }
        Image.Plane[] p = img.getPlanes();
        ByteBuffer y = p[0].getBuffer();
        int sy = p[0].getRowStride();
        for (int f = 0; f < h; f++) {
            y.position(f * sy);
            y.get(ultimo, f * w, w);
        }
        int o = w * h;
        for (int k = 1; k <= 2; k++) {
            ByteBuffer c = p[k].getBuffer();
            int sf = p[k].getRowStride(), sp = p[k].getPixelStride();
            for (int f = 0; f < h / 2; f++) {
                for (int x = 0; x < w / 2; x++) {
                    ultimo[o++] = c.get(f * sf + x * sp);
                }
            }
        }
        rotacion = rotacionPantalla();
        ancho = w;
        alto = h;
        secuencia++;
    }

    /** Cuántos grados está girado el teléfono respecto de su postura natural (0, 90, 180, 270). */
    private static int rotacionPantalla() {
        try {
            Display d = Build.VERSION.SDK_INT >= 30 ? actividad.getDisplay()
                    : ((WindowManager) actividad.getSystemService(Context.WINDOW_SERVICE)).getDefaultDisplay();
            if (d != null) {
                return d.getRotation() * 90;
            }
        } catch (Exception e) {
            // Sin pantalla que consultar (la app en el fondo): como si estuviera de pie.
        }
        return 0;
    }

    /**
     * El último fotograma si es nuevo: cinco enteros big-endian —ancho, alto, giro del
     * sensor, 1 si es la delantera y giro de la pantalla— y luego el I420 tal como lo da
     * el sensor. null si no hay uno nuevo. Ponerlo derecho lo hace Go (camara.derecho),
     * que se prueba sin teléfono.
     */
    public static synchronized byte[] leer() {
        if (ultimo == null || secuencia == leida) {
            return null;
        }
        leida = secuencia;
        byte[] out = new byte[20 + ultimo.length];
        ByteBuffer.wrap(out).putInt(ancho).putInt(alto).putInt(giroSensor).putInt(delantera ? 1 : 0).putInt(rotacion);
        System.arraycopy(ultimo, 0, out, 20, ultimo.length);
        return out;
    }

    public static synchronized void cerrar() {
        if (sesion != null) {
            sesion.close();
            sesion = null;
        }
        if (dispositivo != null) {
            dispositivo.close();
            dispositivo = null;
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
        actividad = null;
    }
}
