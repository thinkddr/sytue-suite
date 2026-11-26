// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import android.app.Activity;
import android.app.Fragment;
import android.content.ActivityNotFoundException;
import android.content.ContentResolver;
import android.content.Context;
import android.content.Intent;
import android.content.UriPermission;
import android.database.Cursor;
import android.net.Uri;
import android.os.Bundle;
import android.os.Environment;
import android.os.Handler;
import android.os.Looper;
import android.os.ParcelFileDescriptor;
import android.os.StatFs;
import android.provider.DocumentsContract;
import android.provider.DocumentsContract.Document;
import android.webkit.MimeTypeMap;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.TimeUnit;

/**
 * La carpeta de Drive de Sytue Office elegida con el selector del sistema (DR-12):
 * `ACTION_OPEN_DOCUMENT_TREE` del Storage Access Framework y, sobre el árbol que devuelve,
 * lo que Drive hace con sus ficheros por `DocumentsContract` (`ficheros.Arbol`,
 * arbol_android.go; `archivos.Arbol` lo presenta como rutas).
 *
 * El permiso que se pide y se GUARDA es de lectura y ESCRITURA, persistente: así Drive
 * vuelve a abrir esa carpeta tras cerrar la app o reiniciar el teléfono, hasta que la
 * persona lo retire (en Ajustes de Drive o en los del sistema).
 *
 * Elegir es un fragmento sin vista, como {@link GuardarDocumento}: la GoNativeActivity no ve
 * `onActivityResult` y un fragmento suyo sí. {@link #elegir} ESPERA la respuesta: Go la
 * llama desde un hilo suyo.
 *
 * Lo demás son métodos estáticos que Go llama con el `content://…/tree/…` y el
 * identificador de documento del proveedor. Un documento se describe en una línea
 * ({@link #registro}): `id`, `1`/`0` (carpeta), tamaño, fecha en milisegundos y nombre,
 * separados por tabuladores, con `\`, el tabulador y el salto de línea escapados. Lo que no
 * está lanza FileNotFoundException, lo que el proveedor no sabe hacer
 * UnsupportedOperationException, y un fallo sin excepción (el proveedor devuelve null) una
 * IOException: Go los distingue por la clase.
 */
@SuppressWarnings("deprecation")
public final class ArbolDocumentos extends Fragment {
    private static final int PEDIR = 7418;
    private static final int LEER_Y_ESCRIBIR = Intent.FLAG_GRANT_READ_URI_PERMISSION | Intent.FLAG_GRANT_WRITE_URI_PERMISSION;
    private static final String[] COLUMNAS = {Document.COLUMN_DOCUMENT_ID, Document.COLUMN_DISPLAY_NAME,
        Document.COLUMN_MIME_TYPE, Document.COLUMN_SIZE, Document.COLUMN_LAST_MODIFIED};
    private static final String ALMACENAMIENTO = "com.android.externalstorage.documents";
    /** Lo elegido (ver {@link #elegir}), o "" si se canceló. */
    private static final LinkedBlockingQueue<String> ELEGIDOS = new LinkedBlockingQueue<>();

    /** Android recrea los fragmentos por este constructor: tiene que existir y ser público. */
    public ArbolDocumentos() {}

    // --- Elegir la carpeta ------------------------------------------------------------

    /**
     * Abre el selector de carpetas del sistema y espera. Devuelve
     * `<content://…/tree/…>\n<nombre>\n<permiso>`, donde el permiso es `rw` (de lectura y
     * escritura y guardado), `r` (el proveedor solo deja leer) o `t` (de escritura, pero el
     * proveedor no deja guardarlo: vale hasta cerrar la app); "" si la persona canceló, o
     * "!" si no hay actividad a la que pegar el selector.
     */
    public static String elegir(Context ctx) throws InterruptedException {
        if (!(ctx instanceof Activity)) {
            return "!";
        }
        final Activity a = (Activity) ctx;
        ELEGIDOS.clear();
        new Handler(Looper.getMainLooper()).post(() ->
                a.getFragmentManager().beginTransaction().add(new ArbolDocumentos(), "sytue-arbol").commitAllowingStateLoss());
        // Sin plazo corto: la persona puede tardar lo que quiera en elegir. Una hora.
        String r = ELEGIDOS.poll(60, TimeUnit.MINUTES);
        return r == null ? "" : r;
    }

    @Override
    public void onCreate(Bundle guardado) {
        super.onCreate(guardado);
        if (guardado != null) {
            return; // recreado tras girar: el selector ya está abierto y contestará a este
        }
        Intent i = new Intent(Intent.ACTION_OPEN_DOCUMENT_TREE)
                .addFlags(LEER_Y_ESCRIBIR | Intent.FLAG_GRANT_PERSISTABLE_URI_PERMISSION | Intent.FLAG_GRANT_PREFIX_URI_PERMISSION);
        try {
            startActivityForResult(i, PEDIR);
        } catch (Exception e) {
            ELEGIDOS.offer("!");
            quitar();
        }
    }

    @Override
    public void onActivityResult(int pedido, int resultado, Intent datos) {
        super.onActivityResult(pedido, resultado, datos);
        if (pedido != PEDIR) {
            return;
        }
        Activity a = getActivity();
        String r = "";
        if (resultado == Activity.RESULT_OK && datos != null && datos.getData() != null && a != null) {
            r = elegido(a.getContentResolver(), datos.getData(), datos.getFlags());
        }
        ELEGIDOS.offer(r);
        quitar();
    }

    /** Lo que devuelve {@link #elegir} para el árbol `u` que dio el selector con `banderas`. */
    static String elegido(ContentResolver cr, Uri u, int banderas) {
        String permiso;
        if ((banderas & Intent.FLAG_GRANT_WRITE_URI_PERMISSION) == 0) {
            permiso = "r";
        } else {
            permiso = "t";
            try {
                cr.takePersistableUriPermission(u, LEER_Y_ESCRIBIR);
                permiso = "rw";
            } catch (SecurityException ignorada) {
                // Un proveedor que no da permisos persistentes: vale mientras la app siga abierta.
            }
        }
        String nombre = u.getLastPathSegment();
        try {
            String[] d = campos(datos(cr, u.toString(), DocumentsContract.getTreeDocumentId(u)));
            nombre = d[4];
        } catch (Exception ignorada) {
            // el último trozo del URI
        }
        return u + "\n" + nombre + "\n" + permiso;
    }

    private void quitar() {
        Activity a = getActivity();
        if (a != null) {
            a.getFragmentManager().beginTransaction().remove(this).commitAllowingStateLoss();
        }
    }

    // --- El permiso ------------------------------------------------------------------

    /** Si la app sigue teniendo permiso guardado de leer y escribir en el árbol. */
    public static boolean vigente(Context ctx, String arbol) {
        Uri u = Uri.parse(arbol);
        for (UriPermission p : ctx.getContentResolver().getPersistedUriPermissions()) {
            if (u.equals(p.getUri()) && p.isReadPermission() && p.isWritePermission()) {
                return true;
            }
        }
        return false;
    }

    /** Devuelve el permiso guardado sobre el árbol: Drive deja de usar esa carpeta. */
    public static void soltar(Context ctx, String arbol) {
        try {
            ctx.getContentResolver().releasePersistableUriPermission(Uri.parse(arbol), LEER_Y_ESCRIBIR);
        } catch (SecurityException yaNoEstaba) {
            // nada que soltar
        }
    }

    // --- Los documentos ----------------------------------------------------------------

    private static Uri documento(String arbol, String id) {
        return DocumentsContract.buildDocumentUriUsingTree(Uri.parse(arbol), id);
    }

    /** La carpeta elegida, en una línea. */
    public static String raiz(Context ctx, String arbol) throws IOException {
        return datos(ctx.getContentResolver(), arbol, DocumentsContract.getTreeDocumentId(Uri.parse(arbol)));
    }

    /** Un documento, en una línea. */
    public static String datos(Context ctx, String arbol, String id) throws IOException {
        return datos(ctx.getContentResolver(), arbol, id);
    }

    static String datos(ContentResolver cr, String arbol, String id) throws IOException {
        try (Cursor c = cr.query(documento(arbol, id), COLUMNAS, null, null, null)) {
            // Un proveedor contesta null (no una excepción) por lo que ya no está.
            if (c == null || !c.moveToFirst()) {
                throw new FileNotFoundException("no está");
            }
            return registro(c);
        }
    }

    /** Los hijos de una carpeta, una línea cada uno. */
    public static String hijos(Context ctx, String arbol, String id) throws IOException {
        Uri u = DocumentsContract.buildChildDocumentsUriUsingTree(Uri.parse(arbol), id);
        try (Cursor c = ctx.getContentResolver().query(u, COLUMNAS, null, null, null)) {
            if (c == null) {
                throw new FileNotFoundException("no está");
            }
            StringBuilder sb = new StringBuilder();
            while (c.moveToNext()) {
                if (sb.length() > 0) {
                    sb.append('\n');
                }
                sb.append(registro(c));
            }
            return sb.toString();
        }
    }

    /**
     * Crea un fichero vacío o una carpeta en `padre`. El tipo del fichero sale de su
     * extensión, como lo calcularía el proveedor: con otro, el almacenamiento del teléfono
     * le AÑADE la extensión de ese tipo al nombre.
     */
    public static String crear(Context ctx, String arbol, String padre, String nombre, boolean carpeta) throws IOException {
        ContentResolver cr = ctx.getContentResolver();
        Uri u = DocumentsContract.createDocument(cr, documento(arbol, padre), carpeta ? Document.MIME_TYPE_DIR : tipoDe(nombre), nombre);
        if (u == null) {
            throw new IOException("el proveedor no lo creó");
        }
        return datos(cr, arbol, DocumentsContract.getDocumentId(u));
    }

    /** Cambia el nombre; el identificador puede cambiar con él. */
    public static String renombrar(Context ctx, String arbol, String id, String nombre) throws IOException {
        ContentResolver cr = ctx.getContentResolver();
        Uri u = DocumentsContract.renameDocument(cr, documento(arbol, id), nombre);
        if (u == null) {
            throw new IOException("el proveedor no lo renombró");
        }
        return datos(cr, arbol, DocumentsContract.getDocumentId(u));
    }

    /** Lleva un documento de la carpeta `origen` a `destino` (UnsupportedOperationException si no sabe). */
    public static String mover(Context ctx, String arbol, String id, String origen, String destino) throws IOException {
        ContentResolver cr = ctx.getContentResolver();
        Uri u = DocumentsContract.moveDocument(cr, documento(arbol, id), documento(arbol, origen), documento(arbol, destino));
        if (u == null) {
            throw new IOException("el proveedor no lo movió");
        }
        return datos(cr, arbol, DocumentsContract.getDocumentId(u));
    }

    /** Borra un documento; una carpeta, con todo lo que tenga. */
    public static void borrar(Context ctx, String arbol, String id) throws IOException {
        if (!DocumentsContract.deleteDocument(ctx.getContentResolver(), documento(arbol, id))) {
            throw new IOException("el proveedor no lo borró");
        }
    }

    /**
     * Abre el contenido en modo `r` o `wt` y devuelve el descriptor, que pasa a ser de Go
     * (`os.NewFile`): él lo cierra.
     */
    public static int abrir(Context ctx, String arbol, String id, String modo) throws IOException {
        ParcelFileDescriptor pfd = ctx.getContentResolver().openFileDescriptor(documento(arbol, id), modo);
        if (pfd == null) {
            throw new FileNotFoundException("no está");
        }
        return pfd.detachFd();
    }

    /**
     * `<total>\t<libre>` del almacenamiento donde está el árbol, o "" si no se sabe. Solo lo
     * sabe del almacenamiento del teléfono y de las tarjetas (`primary:…`, `XXXX-XXXX:…`):
     * las raíces de un proveedor no se pueden preguntar sin MANAGE_DOCUMENTS.
     */
    public static String espacio(Context ctx, String arbol) {
        Uri u = Uri.parse(arbol);
        if (!ALMACENAMIENTO.equals(u.getAuthority())) {
            return "";
        }
        String doc = DocumentsContract.getTreeDocumentId(u);
        int dos = doc.indexOf(':');
        if (dos <= 0) {
            return "";
        }
        String volumen = doc.substring(0, dos);
        File dir = "primary".equals(volumen) ? Environment.getExternalStorageDirectory() : new File("/storage", volumen);
        try {
            StatFs s = new StatFs(dir.getPath());
            return s.getTotalBytes() + "\t" + s.getAvailableBytes();
        } catch (IllegalArgumentException noSeSabe) {
            return "";
        }
    }

    /**
     * Abre el documento con la app que el sistema tenga para su tipo (`abrir`), o la hoja de
     * compartir. La otra app recibe permiso de leerlo y, al abrirlo, de escribirlo: lo
     * guarda en su sitio.
     */
    public static void ver(Context ctx, String arbol, String id, boolean abrir) throws IOException {
        Uri u = documento(arbol, id);
        String tipo = null;
        try (Cursor c = ctx.getContentResolver().query(u, new String[] {Document.COLUMN_MIME_TYPE}, null, null, null)) {
            if (c == null || !c.moveToFirst()) {
                throw new FileNotFoundException("no está");
            }
            tipo = c.getString(0);
        }
        if (tipo == null || tipo.isEmpty()) {
            tipo = "application/octet-stream";
        }
        Intent i = abrir
                ? new Intent(Intent.ACTION_VIEW).setDataAndType(u, tipo).addFlags(LEER_Y_ESCRIBIR)
                : new Intent(Intent.ACTION_SEND).setType(tipo).putExtra(Intent.EXTRA_STREAM, u)
                        .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        Intent elegir = Intent.createChooser(i, null);
        if (!(ctx instanceof Activity)) {
            elegir.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        }
        ctx.startActivity(elegir);
    }

    /**
     * Abre el documento en una app concreta de Sytue Office (`eu.sytue.office.docus`…): un
     * `ACTION_VIEW` explícito a su paquete, con el tipo que su manifiesto declara para
     * «Abrir con» (lo pone Go, por la extensión) y permiso de leerlo y escribirlo. Devuelve
     * false si esa app no está instalada o no acepta el tipo, y entonces Go cae a
     * {@link #ver}, la app del sistema. Sin consultar antes el gestor de paquetes: con la
     * visibilidad de paquetes de Android 11 haría falta declararlos en `<queries>`, y
     * {@code startActivity} no lo necesita.
     */
    public static boolean verEn(Context ctx, String arbol, String id, String paquete, String tipo) {
        Intent i = new Intent(Intent.ACTION_VIEW).setDataAndType(documento(arbol, id), tipo)
                .setPackage(paquete).addFlags(LEER_Y_ESCRIBIR);
        if (!(ctx instanceof Activity)) {
            i.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        }
        try {
            ctx.startActivity(i);
            return true;
        } catch (ActivityNotFoundException noEsta) {
            return false;
        }
    }

    // --- Una línea por documento ---------------------------------------------------------

    /** El tipo MIME por la extensión del nombre, o el genérico. */
    static String tipoDe(String nombre) {
        int punto = nombre.lastIndexOf('.');
        if (punto >= 0 && punto < nombre.length() - 1) {
            String t = MimeTypeMap.getSingleton().getMimeTypeFromExtension(nombre.substring(punto + 1).toLowerCase(java.util.Locale.ROOT));
            if (t != null) {
                return t;
            }
        }
        return "application/octet-stream";
    }

    /** La fila del cursor (en el orden de {@link #COLUMNAS}) como una línea. */
    static String registro(Cursor c) {
        boolean carpeta = Document.MIME_TYPE_DIR.equals(c.getString(2));
        long tamano = c.isNull(3) ? -1 : c.getLong(3);
        long fecha = c.isNull(4) ? 0 : c.getLong(4);
        return escapar(c.getString(0)) + "\t" + (carpeta ? "1" : "0") + "\t" + tamano + "\t" + fecha + "\t" + escapar(c.getString(1));
    }

    static String escapar(String s) {
        if (s == null) {
            return "";
        }
        StringBuilder sb = new StringBuilder(s.length());
        for (int i = 0; i < s.length(); i++) {
            char ch = s.charAt(i);
            switch (ch) {
                case '\\': sb.append("\\\\"); break;
                case '\t': sb.append("\\t"); break;
                case '\n': sb.append("\\n"); break;
                default: sb.append(ch);
            }
        }
        return sb.toString();
    }

    /** Lo contrario de {@link #registro} (para las pruebas). */
    static String[] campos(String linea) {
        String[] f = linea.split("\t", -1);
        for (int i = 0; i < f.length; i++) {
            StringBuilder sb = new StringBuilder();
            for (int j = 0; j < f[i].length(); j++) {
                char ch = f[i].charAt(j);
                if (ch == '\\' && j + 1 < f[i].length()) {
                    char sig = f[i].charAt(++j);
                    sb.append(sig == 't' ? '\t' : sig == 'n' ? '\n' : sig);
                } else {
                    sb.append(ch);
                }
            }
            f[i] = sb.toString();
        }
        return f;
    }
}
