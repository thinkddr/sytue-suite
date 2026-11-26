// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.Manifest;
import android.app.Activity;
import android.content.ContentResolver;
import android.content.ContentUris;
import android.content.ContentValues;
import android.content.Context;
import android.content.pm.PackageManager;
import android.database.Cursor;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.media.ExifInterface;
import android.net.ConnectivityManager;
import android.net.Uri;
import android.os.Build;
import android.os.Environment;
import android.provider.MediaStore;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;

/**
 * La galería del teléfono (MediaStore) para Sytue Photos (`apps/nucleo/carrete`): leer
 * las fotos y los vídeos para la copia de seguridad, pasar a JPEG lo que la galería de
 * Sytue no sabe enseñar (HEIC, AVIF) y guardar en «Pictures/Sytue Photos» lo que se baja
 * de la nube. Go la llama desde un hilo suyo y espera; nada llama a Go desde aquí.
 *
 * <p>Los permisos: {@code READ_MEDIA_IMAGES} y {@code READ_MEDIA_VIDEO} desde la API 33
 * ({@code READ_EXTERNAL_STORAGE} antes), {@code READ_MEDIA_VISUAL_USER_SELECTED} para el
 * acceso parcial de la 34, {@code ACCESS_NETWORK_STATE} para saber si la red es de pago, y
 * {@code ACCESS_MEDIA_LOCATION}, sin el que Android borra el GPS del EXIF al leer y la
 * vista «Cerca» de Fotos se quedaría vacía para todo lo que sube la copia. GUARDAR no pide
 * nada desde la API 29: una app siempre puede añadir a MediaStore.
 */
public final class Carrete {
    /** El código de la petición de permisos: el de la cámara es 7415. */
    private static final int PETICION = 7416;
    private static final String CARPETA = Environment.DIRECTORY_PICTURES + "/Sytue Photos";

    private Carrete() {}

    private static String[] permisosDeLectura() {
        if (Build.VERSION.SDK_INT >= 34) {
            return new String[] {
                Manifest.permission.READ_MEDIA_IMAGES,
                Manifest.permission.READ_MEDIA_VIDEO,
                Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED,
                Manifest.permission.ACCESS_MEDIA_LOCATION,
            };
        }
        if (Build.VERSION.SDK_INT >= 33) {
            return new String[] {
                Manifest.permission.READ_MEDIA_IMAGES,
                Manifest.permission.READ_MEDIA_VIDEO,
                Manifest.permission.ACCESS_MEDIA_LOCATION,
            };
        }
        if (Build.VERSION.SDK_INT >= 29) {
            return new String[] {Manifest.permission.READ_EXTERNAL_STORAGE, Manifest.permission.ACCESS_MEDIA_LOCATION};
        }
        return new String[] {Manifest.permission.READ_EXTERNAL_STORAGE};
    }

    private static boolean tiene(Context ctx, String p) {
        return ctx.checkSelfPermission(p) == PackageManager.PERMISSION_GRANTED;
    }

    /**
     * 1 todas las fotos, 2 solo las elegidas (API 34), 0 ninguna. Sin preguntar. Por debajo
     * de la API 23 los permisos se conceden al instalar.
     */
    public static int permiso(Context ctx) {
        if (Build.VERSION.SDK_INT < 23) {
            return 1;
        }
        String total = Build.VERSION.SDK_INT >= 33
                ? Manifest.permission.READ_MEDIA_IMAGES : Manifest.permission.READ_EXTERNAL_STORAGE;
        if (tiene(ctx, total)) {
            return 1;
        }
        if (Build.VERSION.SDK_INT >= 34 && tiene(ctx, Manifest.permission.READ_MEDIA_VISUAL_USER_SELECTED)) {
            return 2;
        }
        return 0;
    }

    /**
     * Enseña el diálogo del sistema. La respuesta no llega aquí (una NativeActivity no ve
     * {@code onRequestPermissionsResult}): Go vuelve a mirar {@link #permiso} hasta que
     * cambie o la app recupere el foco. Devuelve false si no hay actividad a la que pedírselo.
     */
    public static boolean pedir(Context ctx) {
        if (Build.VERSION.SDK_INT < 23 || !(ctx instanceof Activity)) {
            return false;
        }
        ((Activity) ctx).requestPermissions(permisosDeLectura(), PETICION);
        return true;
    }

    /** Si la actividad tiene el foco: con el diálogo de permisos delante no lo tiene. */
    public static boolean conFoco(Context ctx) {
        return !(ctx instanceof Activity) || ((Activity) ctx).hasWindowFocus();
    }

    private static String limpio(String s) {
        return s == null ? "" : s.replace('\t', ' ').replace('\n', ' ').replace('\r', ' ');
    }

    /**
     * Las fotos y los vídeos de la galería, uno por línea:
     * «id\tnombre\ttipo\tmilisegundos\tbytes» en las fotos y
     * «vid\tnombre\ttipo\tmilisegundos\tbytes\tv\tduración\tancho\talto» en los vídeos:
     * el id con «v» delante (los ids de MediaStore son de cada tabla), la «v» que los marca y
     * su duración en milisegundos y sus lados, 0 si MediaStore no los sabe. Sin lo que guardó
     * Sytue («Pictures/Sytue Photos»). La fecha es la de la toma ({@code DATE_TAKEN}) y, si
     * no la hay, la de llegada ({@code DATE_ADDED}, en segundos).
     */
    public static String listar(Context ctx) {
        StringBuilder s = new StringBuilder();
        boolean nueva = Build.VERSION.SDK_INT >= 29;
        listar(ctx, s, nueva ? MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL)
                : MediaStore.Images.Media.EXTERNAL_CONTENT_URI, "");
        listar(ctx, s, nueva ? MediaStore.Video.Media.getContentUri(MediaStore.VOLUME_EXTERNAL)
                : MediaStore.Video.Media.EXTERNAL_CONTENT_URI, "v");
        return s.toString();
    }

    /**
     * Una tabla de MediaStore. Las columnas se llaman igual en la de fotos y la de vídeos; la
     * de vídeos trae además la duración y los lados.
     */
    private static void listar(Context ctx, StringBuilder s, Uri origen, String marca) {
        boolean video = !marca.isEmpty();
        String[] columnas = video ? new String[] {
            MediaStore.MediaColumns._ID,
            MediaStore.MediaColumns.DISPLAY_NAME,
            MediaStore.MediaColumns.MIME_TYPE,
            MediaStore.Video.Media.DATE_TAKEN,
            MediaStore.MediaColumns.DATE_ADDED,
            MediaStore.MediaColumns.SIZE,
            MediaStore.Video.Media.DURATION,
            MediaStore.MediaColumns.WIDTH,
            MediaStore.MediaColumns.HEIGHT,
        } : new String[] {
            MediaStore.MediaColumns._ID,
            MediaStore.MediaColumns.DISPLAY_NAME,
            MediaStore.MediaColumns.MIME_TYPE,
            MediaStore.Images.Media.DATE_TAKEN,
            MediaStore.MediaColumns.DATE_ADDED,
            MediaStore.MediaColumns.SIZE,
        };
        String donde = null;
        String[] args = null;
        if (Build.VERSION.SDK_INT >= 29) {
            donde = "(" + MediaStore.MediaColumns.RELATIVE_PATH + " IS NULL OR "
                    + MediaStore.MediaColumns.RELATIVE_PATH + " NOT LIKE ?)";
            args = new String[] {CARPETA + "/%"};
        }
        try (Cursor c = ctx.getContentResolver().query(origen, columnas, donde, args, MediaStore.MediaColumns._ID + " ASC")) {
            if (c == null) {
                return;
            }
            while (c.moveToNext()) {
                long tomada = c.isNull(3) ? 0 : c.getLong(3);
                if (tomada <= 0 && !c.isNull(4)) {
                    tomada = c.getLong(4) * 1000L;
                }
                s.append(marca).append(c.getLong(0)).append('\t')
                        .append(limpio(c.getString(1))).append('\t')
                        .append(limpio(c.getString(2))).append('\t')
                        .append(tomada).append('\t')
                        .append(c.isNull(5) ? -1 : c.getLong(5));
                if (video) {
                    s.append("\tv\t").append(positivo(c, 6))
                            .append('\t').append(positivo(c, 7))
                            .append('\t').append(positivo(c, 8));
                }
                s.append('\n');
            }
        } catch (SecurityException e) {
            // Sin permiso para esa tabla (los vídeos, si solo se dieron las fotos): sin ella.
        }
    }

    /** La columna i como número, o 0 si está vacía o no es positiva. */
    private static long positivo(Cursor c, int i) {
        if (c.isNull(i)) {
            return 0;
        }
        long n = c.getLong(i);
        return n > 0 ? n : 0;
    }

    /**
     * Copia el original de la foto (o del vídeo, con «v» delante) `id` al fichero `destino` (en la caché de la app). Con
     * {@code ACCESS_MEDIA_LOCATION} pide el original de verdad, con su GPS. Devuelve "" si
     * ha ido bien, o el motivo.
     */
    public static String copiar(Context ctx, String id, String destino) {
        Uri uri;
        try {
            uri = id.startsWith("v")
                    ? ContentUris.withAppendedId(MediaStore.Video.Media.EXTERNAL_CONTENT_URI, Long.parseLong(id.substring(1)))
                    : ContentUris.withAppendedId(MediaStore.Images.Media.EXTERNAL_CONTENT_URI, Long.parseLong(id));
        } catch (NumberFormatException e) {
            return "id no válido";
        }
        if (Build.VERSION.SDK_INT >= 29 && tiene(ctx, Manifest.permission.ACCESS_MEDIA_LOCATION)) {
            try {
                uri = MediaStore.setRequireOriginal(uri);
            } catch (Exception ignorado) {
                // Sin el original, la foto sin GPS: mejor eso que no subirla.
            }
        }
        try (InputStream in = ctx.getContentResolver().openInputStream(uri);
                OutputStream out = new FileOutputStream(destino)) {
            if (in == null) {
                return "sin contenido";
            }
            byte[] b = new byte[64 * 1024];
            for (int n; (n = in.read(b)) > 0; ) {
                out.write(b, 0, n);
            }
            return "";
        } catch (IOException | SecurityException e) {
            return String.valueOf(e.getMessage());
        }
    }

    /** Las etiquetas que se llevan de una foto a su JPEG: cuándo, dónde, cómo y con qué. */
    private static final String[] ETIQUETAS = {
        ExifInterface.TAG_DATETIME, ExifInterface.TAG_DATETIME_ORIGINAL, ExifInterface.TAG_DATETIME_DIGITIZED,
        ExifInterface.TAG_OFFSET_TIME, ExifInterface.TAG_OFFSET_TIME_ORIGINAL, ExifInterface.TAG_OFFSET_TIME_DIGITIZED,
        ExifInterface.TAG_SUBSEC_TIME, ExifInterface.TAG_SUBSEC_TIME_ORIGINAL, ExifInterface.TAG_SUBSEC_TIME_DIGITIZED,
        ExifInterface.TAG_GPS_LATITUDE, ExifInterface.TAG_GPS_LATITUDE_REF,
        ExifInterface.TAG_GPS_LONGITUDE, ExifInterface.TAG_GPS_LONGITUDE_REF,
        ExifInterface.TAG_GPS_ALTITUDE, ExifInterface.TAG_GPS_ALTITUDE_REF,
        ExifInterface.TAG_GPS_TIMESTAMP, ExifInterface.TAG_GPS_DATESTAMP,
        // Sin el objetivo: el ExifInterface del sistema (no el de AndroidX) no lo nombra.
        ExifInterface.TAG_ORIENTATION, ExifInterface.TAG_MAKE, ExifInterface.TAG_MODEL,
        ExifInterface.TAG_F_NUMBER, ExifInterface.TAG_EXPOSURE_TIME, ExifInterface.TAG_ISO_SPEED_RATINGS,
        ExifInterface.TAG_FOCAL_LENGTH, ExifInterface.TAG_FOCAL_LENGTH_IN_35MM_FILM,
        ExifInterface.TAG_FLASH, ExifInterface.TAG_WHITE_BALANCE,
        ExifInterface.TAG_IMAGE_DESCRIPTION, ExifInterface.TAG_ARTIST, ExifInterface.TAG_COPYRIGHT,
    };

    /**
     * Pasa la foto `origen` (HEIC desde la API 28, AVIF desde la 31, WebP) a JPEG en
     * `destino`, con calidad 92 y sus etiquetas EXIF: la galería de Sytue mide y coloca las
     * fotos por su fecha y su lugar. Los píxeles NO se giran: va la etiqueta de orientación,
     * como en la cámara. Devuelve "" si ha ido bien, o el motivo.
     */
    public static String convertir(String origen, String destino) {
        Bitmap b;
        try {
            b = BitmapFactory.decodeFile(origen);
        } catch (OutOfMemoryError e) {
            return "sin memoria para abrir la foto";
        }
        if (b == null) {
            return "este Android no sabe abrir la foto";
        }
        try (OutputStream out = new FileOutputStream(destino)) {
            if (!b.compress(Bitmap.CompressFormat.JPEG, 92, out)) {
                return "no se ha podido escribir el JPEG";
            }
        } catch (IOException e) {
            return String.valueOf(e.getMessage());
        } finally {
            b.recycle();
        }
        try {
            ExifInterface de = new ExifInterface(origen);
            ExifInterface a = new ExifInterface(destino);
            for (String etiqueta : ETIQUETAS) {
                String valor = de.getAttribute(etiqueta);
                if (valor != null) {
                    a.setAttribute(etiqueta, valor);
                }
            }
            a.saveAttributes();
        } catch (IOException | RuntimeException e) {
            // Sin metadatos la foto sigue siendo la foto: la galería la pone por su subida.
        }
        return "";
    }

    /** Si la red de ahora es de pago (datos móviles, un punto de acceso). Sin saberlo, sí. */
    public static boolean medida(Context ctx) {
        ConnectivityManager cm = ctx.getSystemService(ConnectivityManager.class);
        try {
            return cm == null || cm.isActiveNetworkMetered();
        } catch (SecurityException sinPermiso) {
            return true;
        }
    }

    /**
     * Añade el fichero `origen` a la galería, en «Pictures/Sytue Photos», como `nombre`.
     * Devuelve el id nuevo, o "" si no se pudo. Solo desde la API 29: antes haría falta
     * {@code WRITE_EXTERNAL_STORAGE}, y quien llama lo deja en descargas.
     */
    public static String guardar(Context ctx, String origen, String nombre, String tipo) {
        if (Build.VERSION.SDK_INT < 29) {
            return "";
        }
        ContentResolver cr = ctx.getContentResolver();
        ContentValues v = new ContentValues();
        v.put(MediaStore.Images.Media.DISPLAY_NAME, nombre);
        if (tipo != null && !tipo.isEmpty()) {
            v.put(MediaStore.Images.Media.MIME_TYPE, tipo);
        }
        v.put(MediaStore.Images.Media.RELATIVE_PATH, CARPETA);
        v.put(MediaStore.Images.Media.IS_PENDING, 1);
        Uri uri;
        try {
            uri = cr.insert(MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY), v);
        } catch (Exception e) {
            return "";
        }
        if (uri == null) {
            return "";
        }
        try (InputStream in = new FileInputStream(origen); OutputStream out = cr.openOutputStream(uri)) {
            if (out == null) {
                throw new IOException("sin destino");
            }
            byte[] b = new byte[64 * 1024];
            for (int n; (n = in.read(b)) > 0; ) {
                out.write(b, 0, n);
            }
        } catch (IOException | SecurityException e) {
            cr.delete(uri, null, null);
            return "";
        }
        ContentValues listo = new ContentValues();
        listo.put(MediaStore.Images.Media.IS_PENDING, 0);
        cr.update(uri, listo, null, null);
        return String.valueOf(ContentUris.parseId(uri));
    }
}
