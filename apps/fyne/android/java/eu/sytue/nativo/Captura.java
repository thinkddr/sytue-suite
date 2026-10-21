// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.content.ActivityNotFoundException;
import android.content.ClipData;
import android.content.ContentProvider;
import android.content.ContentValues;
import android.content.Context;
import android.content.Intent;
import android.database.Cursor;
import android.net.Uri;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelFileDescriptor;
import android.provider.MediaStore;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

/**
 * La cámara directa de «Escanear» (el `capture="environment"` de la web): la app de
 * cámara del sistema guarda la foto en la caché PRIVADA de esta app a través de este
 * proveedor, y no en la galería del teléfono, que en muchos se copia sola a la nube de
 * otro. {@link #fotografiar} ESPERA a que la cámara cierre el fichero, como Certificados:
 * Go la llama desde un hilo suyo y nada llama a Go desde aquí.
 */
public final class Captura extends ContentProvider {
    private static final String PREFIJO = "sytue-captura-";
    private static final LinkedBlockingQueue<String> ESCRITAS = new LinkedBlockingQueue<>();

    /**
     * Abre la cámara para que deje la foto en `nombre` (que empieza por el prefijo) y
     * espera: devuelve la ruta de la foto, o "" si no hay cámara o no se hizo. Que no se
     * hizo se sabe porque la app vuelve a tener el foco tres segundos sin foto: Android
     * no avisa de otra forma a una NativeActivity, que no ve `onActivityResult`.
     */
    public static String fotografiar(Context ctx, String nombre) throws InterruptedException {
        ESCRITAS.clear();
        Uri uri = Uri.parse("content://" + ctx.getPackageName() + ".sytue.captura/" + nombre);
        Intent i = new Intent(MediaStore.ACTION_IMAGE_CAPTURE)
                .putExtra(MediaStore.EXTRA_OUTPUT, uri)
                .addFlags(Intent.FLAG_GRANT_WRITE_URI_PERMISSION | Intent.FLAG_GRANT_READ_URI_PERMISSION);
        i.setClipData(ClipData.newRawUri("", uri));
        if (!(ctx instanceof Activity)) {
            i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        }
        try {
            ctx.startActivity(i);
        } catch (ActivityNotFoundException | SecurityException e) {
            // Sin app de cámara, o una app que declara CAMERA sin tenerlo concedido (Rooms):
            // ahí Android no deja pedir la foto a otra.
            return "";
        }
        Activity a = ctx instanceof Activity ? (Activity) ctx : null;
        boolean fuera = false;
        int dentro = 0;
        for (int n = 0; n < 1200; n++) { // medio segundo cada vuelta: diez minutos
            String ruta = ESCRITAS.poll(500, TimeUnit.MILLISECONDS);
            if (ruta != null) {
                return ruta;
            }
            if (a == null) {
                continue;
            }
            if (!a.hasWindowFocus()) {
                fuera = true;
                dentro = 0;
            } else if (fuera && ++dentro >= 6) {
                return "";
            }
        }
        return "";
    }

    private static final java.util.concurrent.ConcurrentHashMap<String, File> COMPARTIDOS = new java.util.concurrent.ConcurrentHashMap<>();

    /**
     * Comparte un fichero local con otras aplicaciones (Intent.ACTION_VIEW o ACTION_SEND).
     * Si soloVer es true, abre el visor asociado. Si es false, abre la hoja de compartir.
     */
    public static void compartir(Context ctx, String ruta, boolean soloVer) {
        File f = new File(ruta);
        if (!f.exists()) {
            return;
        }
        String id = "share-" + System.currentTimeMillis() + "-" + f.getName();
        COMPARTIDOS.put(id, f);
        Uri uri = Uri.parse("content://" + ctx.getPackageName() + ".sytue.captura/" + id);
        String extension = android.webkit.MimeTypeMap.getFileExtensionFromUrl(f.getName());
        String mime = null;
        if (extension != null && !extension.isEmpty()) {
            mime = android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(extension.toLowerCase());
        }
        if (mime == null || mime.isEmpty()) {
            mime = "application/octet-stream";
        }
        Intent intent;
        if (soloVer) {
            intent = new Intent(Intent.ACTION_VIEW)
                    .setDataAndType(uri, mime)
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        } else {
            intent = new Intent(Intent.ACTION_SEND)
                    .setType(mime)
                    .putExtra(Intent.EXTRA_STREAM, uri)
                    .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        }
        if (!(ctx instanceof Activity)) {
            intent.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        }
        Intent chooser = Intent.createChooser(intent, null);
        if (!(ctx instanceof Activity)) {
            chooser.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        }
        try {
            ctx.startActivity(chooser);
        } catch (Exception ignored) {
        }
    }

    /**
     * Copia una imagen local al portapapeles del sistema como URI con permisos de lectura.
     */
    public static void copiarImagen(Context ctx, String ruta) {
        File f = new File(ruta);
        if (!f.exists()) {
            return;
        }
        String id = "clip-" + System.currentTimeMillis() + "-" + f.getName();
        COMPARTIDOS.put(id, f);
        Uri uri = Uri.parse("content://" + ctx.getPackageName() + ".sytue.captura/" + id);
        android.content.ClipboardManager cm = (android.content.ClipboardManager) ctx.getSystemService(Context.CLIPBOARD_SERVICE);
        if (cm != null) {
            ClipData cd = ClipData.newUri(ctx.getContentResolver(), "Imagen", uri);
            cm.setPrimaryClip(cd);
        }
    }

    @Override
    public ParcelFileDescriptor openFile(Uri uri, String modo) throws FileNotFoundException {
        String nombre = uri.getLastPathSegment();
        if (nombre == null || nombre.contains("/") || nombre.contains("..")) {
            throw new FileNotFoundException(String.valueOf(uri));
        }
        File compartido = COMPARTIDOS.get(nombre);
        if (compartido != null && compartido.exists()) {
            return ParcelFileDescriptor.open(compartido, ParcelFileDescriptor.parseMode(modo));
        }
        // Solo los ficheros que ha pedido `fotografiar`: el proveedor no sirve nada más.
        if (!nombre.startsWith(PREFIJO)) {
            throw new FileNotFoundException(String.valueOf(uri));
        }
        final File f = new File(getContext().getCacheDir(), nombre);
        try {
            return ParcelFileDescriptor.open(f, ParcelFileDescriptor.parseMode(modo),
                    new Handler(Looper.getMainLooper()), new ParcelFileDescriptor.OnCloseListener() {
                        @Override
                        public void onClose(IOException e) {
                            if (e == null && f.length() > 0) {
                                ESCRITAS.offer(f.getAbsolutePath());
                            }
                        }
                    });
        } catch (IOException e) {
            throw new FileNotFoundException(e.getMessage());
        }
    }

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public String getType(Uri uri) {
        String nombre = uri.getLastPathSegment();
        if (nombre != null) {
            File f = COMPARTIDOS.get(nombre);
            String nom = f != null ? f.getName() : nombre;
            int idx = nom.lastIndexOf('.');
            if (idx >= 0) {
                String ext = nom.substring(idx + 1).toLowerCase();
                String t = android.webkit.MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext);
                if (t != null) {
                    return t;
                }
            }
        }
        return "image/jpeg";
    }

    @Override
    public Cursor query(Uri uri, String[] p, String s, String[] a, String o) {
        return null;
    }

    @Override
    public Uri insert(Uri uri, ContentValues v) {
        return null;
    }

    @Override
    public int delete(Uri uri, String s, String[] a) {
        return 0;
    }

    @Override
    public int update(Uri uri, ContentValues v, String s, String[] a) {
        return 0;
    }
}
