// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.content.Context;
import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import android.os.Bundle;
import android.os.CancellationSignal;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelFileDescriptor;
import android.provider.DocumentsContract;
import android.provider.DocumentsContract.Document;
import android.provider.DocumentsContract.Root;
import android.provider.DocumentsProvider;
import android.util.Base64;
import android.util.Log;
import android.webkit.MimeTypeMap;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import org.json.JSONObject;

/**
 * Sytue Drive en «Archivos» y en el selector de cualquier app (Storage Access Framework),
 * también con la app CERRADA.
 *
 * Habla él solo con el WebDAV de Drive ({@link WebDAV}), con Basic y la contraseña de
 * aplicación de este equipo, que la app crea al entrar y revoca al salir
 * (`apps/nucleo/documentos/credencial.go`). La credencial la deja Go en el almacén de la
 * sesión: `secreto-drive-sistema.bin` del directorio privado, cifrado con AES-GCM y la
 * clave `eu.sytue.one.sesion` del Android Keystore (`apps/nucleo/auth/store_android.go`).
 * Aquí solo se lee. Sin ella, la raíz dice qué hacer («abre Sytue Drive»).
 *
 * Los identificadores son las rutas de Drive: `/` es Mi unidad y `/Facturas/enero.pdf` un
 * fichero. Renombrar cambia el identificador, y así se le dice a «Archivos».
 */
public final class Documentos extends DocumentsProvider {
    private static final String RAIZ = "/";
    private static final String PREFS = "sytue-documentos";
    private static final String ETIQUETA = "SytueDocumentos";
    private static final String[] COLUMNAS_RAIZ = {Root.COLUMN_ROOT_ID, Root.COLUMN_TITLE, Root.COLUMN_SUMMARY,
        Root.COLUMN_DOCUMENT_ID, Root.COLUMN_FLAGS, Root.COLUMN_ICON, Root.COLUMN_MIME_TYPES};
    private static final String[] COLUMNAS = {Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME,
        Document.COLUMN_MIME_TYPE, Document.COLUMN_SIZE, Document.COLUMN_LAST_MODIFIED, Document.COLUMN_FLAGS};
    /** Los intentos de subir lo escrito al cerrarlo, con una pausa creciente entre ellos. */
    private static final int INTENTOS = 4;

    // --- Lo que llama Go -----------------------------------------------------------

    /** Guarda los textos ya traducidos, para cuando la app no esté. Go lo llama al arrancar. */
    public static void configurar(Context ctx, String titulo, String sinApp) {
        ctx.getSharedPreferences(PREFS, Context.MODE_PRIVATE).edit()
                .putString("titulo", titulo).putString("sinApp", sinApp).apply();
        cambio(ctx);
    }

    /** La credencial ha cambiado: «Archivos» vuelve a pedir la raíz y lo que hay en ella. */
    public static void cambio(Context ctx) {
        ctx.getContentResolver().notifyChange(DocumentsContract.buildRootsUri(autoridad(ctx)), null);
        ctx.getContentResolver().notifyChange(DocumentsContract.buildChildDocumentsUri(autoridad(ctx), RAIZ), null);
    }

    // --- La credencial ---------------------------------------------------------------

    /** Lee la credencial del almacén; null si no hay, o si es una que queda por revocar. */
    private static WebDAV dav(Context ctx) {
        File f = new File(ctx.getFilesDir(), "secreto-drive-sistema.bin");
        if (!f.isFile()) {
            return null;
        }
        try {
            JSONObject j = new JSONObject(new String(SesionPrivada.leer(ctx, "secreto-drive-sistema.bin"),
                    StandardCharsets.UTF_8));
            String url = j.optString("url"), usuario = j.optString("usuario"), clave = j.optString("clave");
            if (url.isEmpty() || clave.isEmpty()) {
                return null;
            }
            String basic = Base64.encodeToString((usuario + ":" + clave).getBytes(StandardCharsets.UTF_8),
                    Base64.NO_WRAP);
            return new WebDAV(url, "Basic " + basic);
        } catch (Exception e) {
            Log.w(ETIQUETA, "la credencial de Drive no se deja leer", e);
            return null;
        }
    }

    private WebDAV dav() throws FileNotFoundException {
        WebDAV d = dav(getContext());
        if (d == null) {
            throw new FileNotFoundException(texto("sinApp", "Sytue Drive"));
        }
        return d;
    }

    // --- El proveedor ----------------------------------------------------------------

    private static String autoridad(Context ctx) {
        return ctx.getPackageName() + ".sytue.documentos";
    }

    private String texto(String clave, String siNo) {
        return getContext().getSharedPreferences(PREFS, Context.MODE_PRIVATE).getString(clave, siNo);
    }

    private String titulo() {
        return texto("titulo", getContext().getApplicationInfo().loadLabel(getContext().getPackageManager()).toString());
    }

    /** El tipo por la extensión del nombre, o null. */
    private static String tipoPorNombre(String nombre) {
        String ext = MimeTypeMap.getFileExtensionFromUrl(Uri.encode(nombre));
        return ext == null || ext.isEmpty() ? null : MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext.toLowerCase());
    }

    /** El tipo de un elemento: el que diga Drive o, si no, el de su extensión. */
    private static String tipoDe(WebDAV.Entrada e) {
        if (e.carpeta) {
            return Document.MIME_TYPE_DIR;
        }
        if (e.tipo != null) {
            return e.tipo;
        }
        String t = tipoPorNombre(e.nombre());
        return t != null ? t : "application/octet-stream";
    }

    private void fila(MatrixCursor c, WebDAV.Entrada e) {
        MatrixCursor.RowBuilder f = c.newRow();
        boolean raiz = RAIZ.equals(e.ruta);
        int banderas = e.carpeta
                ? Document.FLAG_DIR_SUPPORTS_CREATE | (raiz ? 0 : Document.FLAG_SUPPORTS_DELETE | Document.FLAG_SUPPORTS_RENAME)
                : Document.FLAG_SUPPORTS_WRITE | Document.FLAG_SUPPORTS_DELETE | Document.FLAG_SUPPORTS_RENAME;
        f.add(Document.COLUMN_DOCUMENT_ID, e.ruta);
        f.add(Document.COLUMN_DISPLAY_NAME, raiz ? titulo() : e.nombre());
        f.add(Document.COLUMN_MIME_TYPE, tipoDe(e));
        f.add(Document.COLUMN_SIZE, e.carpeta || e.tamano < 0 ? null : e.tamano);
        f.add(Document.COLUMN_LAST_MODIFIED, e.modificado > 0 ? e.modificado : null);
        f.add(Document.COLUMN_FLAGS, banderas);
    }

    private static FileNotFoundException fallo(IOException e) {
        if (e instanceof FileNotFoundException) {
            return (FileNotFoundException) e;
        }
        FileNotFoundException f = new FileNotFoundException(e.getMessage());
        f.initCause(e);
        return f;
    }

    private void cambioEn(String carpeta) {
        getContext().getContentResolver()
                .notifyChange(DocumentsContract.buildChildDocumentsUri(autoridad(getContext()), carpeta), null);
    }

    @Override
    public boolean onCreate() {
        return true;
    }

    @Override
    public Cursor queryRoots(String[] proyeccion) {
        boolean hay = dav(getContext()) != null;
        MatrixCursor c = new MatrixCursor(proyeccion != null ? proyeccion : COLUMNAS_RAIZ);
        MatrixCursor.RowBuilder f = c.newRow();
        f.add(Root.COLUMN_ROOT_ID, "drive");
        f.add(Root.COLUMN_TITLE, titulo());
        f.add(Root.COLUMN_SUMMARY, hay ? null : texto("sinApp", null));
        f.add(Root.COLUMN_DOCUMENT_ID, RAIZ);
        f.add(Root.COLUMN_FLAGS, hay ? Root.FLAG_SUPPORTS_CREATE | Root.FLAG_SUPPORTS_IS_CHILD : 0);
        f.add(Root.COLUMN_ICON, getContext().getApplicationInfo().icon);
        f.add(Root.COLUMN_MIME_TYPES, "*/*");
        c.setNotificationUri(getContext().getContentResolver(), DocumentsContract.buildRootsUri(autoridad(getContext())));
        return c;
    }

    @Override
    public Cursor queryDocument(String id, String[] proyeccion) throws FileNotFoundException {
        MatrixCursor c = new MatrixCursor(proyeccion != null ? proyeccion : COLUMNAS);
        if (RAIZ.equals(id)) {
            // La raíz se enseña siempre, para que «Archivos» la abra y diga qué hacer.
            fila(c, new WebDAV.Entrada(RAIZ, true, -1, 0, null));
            return c;
        }
        try {
            fila(c, dav().uno(id));
        } catch (IOException e) {
            throw fallo(e);
        }
        return c;
    }

    @Override
    public Cursor queryChildDocuments(String padre, String[] proyeccion, String orden) throws FileNotFoundException {
        MatrixCursor c = new MatrixCursor(proyeccion != null ? proyeccion : COLUMNAS);
        c.setNotificationUri(getContext().getContentResolver(),
                DocumentsContract.buildChildDocumentsUri(autoridad(getContext()), padre));
        WebDAV d = dav(getContext());
        if (d == null) {
            Bundle extra = new Bundle();
            extra.putString(DocumentsContract.EXTRA_INFO, texto("sinApp", ""));
            c.setExtras(extra);
            return c;
        }
        try {
            for (WebDAV.Entrada e : d.hijos(padre)) {
                fila(c, e);
            }
        } catch (IOException e) {
            throw fallo(e);
        }
        return c;
    }

    @Override
    public boolean isChildDocument(String padre, String id) {
        return RAIZ.equals(padre) ? id.startsWith("/") : id.startsWith(padre + "/");
    }

    @Override
    public ParcelFileDescriptor openDocument(final String id, String modo, CancellationSignal cancelar)
            throws FileNotFoundException {
        final WebDAV d = dav();
        File dir = new File(getContext().getCacheDir(), "sytue-documentos");
        if (!dir.isDirectory() && !dir.mkdirs()) {
            throw new FileNotFoundException("sin caché");
        }
        // Un nombre por ruta, y la extensión al final para saber el tipo al subir.
        final File f = new File(dir, Integer.toHexString(id.hashCode()) + "-" + new File(id).getName());
        final int m = ParcelFileDescriptor.parseMode(modo);
        boolean escribe = (m & (ParcelFileDescriptor.MODE_WRITE_ONLY | ParcelFileDescriptor.MODE_READ_WRITE)) != 0;
        try {
            // Escribir sobre lo que hay (`rw`) necesita lo de ahora; un `wt` lo trunca al abrir.
            if (!escribe || (m & ParcelFileDescriptor.MODE_TRUNCATE) == 0) {
                d.bajar(id, f);
            }
            if (!escribe) {
                return ParcelFileDescriptor.open(f, m);
            }
            final Context ctx = getContext().getApplicationContext();
            // Al cerrarlo quien escribe, se sube como versión nueva del mismo fichero.
            return ParcelFileDescriptor.open(f, m, new Handler(Looper.getMainLooper()),
                    new ParcelFileDescriptor.OnCloseListener() {
                        @Override
                        public void onClose(IOException e) {
                            if (e != null) {
                                return;
                            }
                            new Thread(new Runnable() {
                                @Override
                                public void run() {
                                    subirAlCerrar(ctx, d, id, f);
                                }
                            }).start();
                        }
                    });
        } catch (IOException e) {
            throw fallo(e);
        }
    }

    /**
     * Sube lo escrito, con unos pocos reintentos. ponytail: sin red durante todos ellos el
     * cambio se queda en la caché y no se sube; una cola persistente lo arreglaría.
     */
    private static void subirAlCerrar(Context ctx, WebDAV d, String id, File f) {
        for (int i = 0; i < INTENTOS; i++) {
            try {
                d.subir(id, f, tipoPorNombre(f.getName()));
                ctx.getContentResolver().notifyChange(DocumentsContract.buildDocumentUri(autoridad(ctx), id), null);
                ctx.getContentResolver().notifyChange(
                        DocumentsContract.buildChildDocumentsUri(autoridad(ctx), WebDAV.padre(id)), null);
                return;
            } catch (IOException e) {
                Log.w(ETIQUETA, "no se ha podido subir lo escrito (intento " + (i + 1) + ")", e);
                try {
                    Thread.sleep(2000L << i);
                } catch (InterruptedException parar) {
                    Thread.currentThread().interrupt();
                    return;
                }
            }
        }
    }

    /** Un nombre libre en `carpeta`: el mismo, o con « (2)», « (3)»… delante de la extensión. */
    private static String libre(WebDAV d, String carpeta, String nombre) throws IOException {
        int punto = nombre.lastIndexOf('.');
        String base = punto > 0 ? nombre.substring(0, punto) : nombre;
        String ext = punto > 0 ? nombre.substring(punto) : "";
        for (int n = 1; n < 100; n++) {
            String ruta = WebDAV.dentro(carpeta, n == 1 ? nombre : base + " (" + n + ")" + ext);
            if (!d.existe(ruta)) {
                return ruta;
            }
        }
        throw new IOException("no hay nombre libre para " + nombre);
    }

    @Override
    public String createDocument(String padre, String tipo, String nombre) throws FileNotFoundException {
        WebDAV d = dav();
        try {
            String ruta = libre(d, padre, nombre);
            if (Document.MIME_TYPE_DIR.equals(tipo)) {
                d.crearCarpeta(ruta);
            } else {
                // Vacío: el contenido llega al cerrarlo quien escribe, como versión nueva.
                File vacio = File.createTempFile("vacio", null, getContext().getCacheDir());
                try {
                    d.subir(ruta, vacio, tipo);
                } finally {
                    vacio.delete();
                }
            }
            cambioEn(padre);
            return ruta;
        } catch (IOException e) {
            throw fallo(e);
        }
    }

    @Override
    public String renameDocument(String id, String nombre) throws FileNotFoundException {
        String padre = WebDAV.padre(id);
        String nueva = WebDAV.dentro(padre, nombre);
        if (nueva.equals(id)) {
            return null;
        }
        try {
            dav().mover(id, nueva);
        } catch (IOException e) {
            throw fallo(e);
        }
        cambioEn(padre);
        return nueva;
    }

    @Override
    public void deleteDocument(String id) throws FileNotFoundException {
        try {
            dav().borrar(id);
        } catch (IOException e) {
            throw fallo(e);
        }
        cambioEn(WebDAV.padre(id));
    }
}
