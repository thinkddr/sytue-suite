// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.provider;

import android.database.Cursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.DocumentsContract.Document;
import android.webkit.MimeTypeMap;
import java.io.File;
import java.io.FileNotFoundException;
import java.io.IOException;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.TreeMap;

/**
 * No es una clase de Android: es el proveedor de documentos de MENTIRA de
 * ArbolDocumentosPrueba, que se porta como el del almacenamiento del teléfono
 * (ExternalStorageProvider) sobre una carpeta temporal: el identificador es
 * `primary:<ruta>` (y cambia con ella), un nombre repetido se vuelve «nombre (1)», al
 * fichero cuyo tipo no casa con su extensión se le AÑADE la del tipo, y lo que no está
 * contesta null en query (no una excepción).
 */
public final class ProveedorFalso {
    public static ProveedorFalso actual;

    private final File base;
    /** Si es true, mover lanza UnsupportedOperationException, como un proveedor sin FLAG_SUPPORTS_MOVE. */
    public boolean sinMover;
    /** Lo abierto: descriptor → `<id> <modo>`. */
    public final Map<Integer, String> abiertos = new TreeMap<>();
    private int siguiente = 100;

    public ProveedorFalso(File base) { this.base = base; }

    public File fichero(String id) {
        if (!id.startsWith("primary:")) {
            throw new IllegalArgumentException("id ajeno: " + id);
        }
        String rel = id.substring("primary:".length());
        return rel.isEmpty() ? base : new File(base, rel);
    }

    public String id(File f) {
        String rel = base.toPath().relativize(f.toPath()).toString();
        return "primary:" + rel;
    }

    private Uri uri(Uri parecido, File f) { return DocumentsContract.buildDocumentUriUsingTree(parecido, id(f)); }

    private File de(Uri u) { return fichero(DocumentsContract.getDocumentId(u)); }

    private static String tipo(File f) {
        if (f.isDirectory()) {
            return Document.MIME_TYPE_DIR;
        }
        String n = f.getName();
        int p = n.lastIndexOf('.');
        String t = p < 0 ? null : MimeTypeMap.getSingleton().getMimeTypeFromExtension(n.substring(p + 1));
        return t == null ? "application/octet-stream" : t;
    }

    private Object[] fila(File f, String[] columnas) {
        Object[] o = new Object[columnas.length];
        for (int i = 0; i < columnas.length; i++) {
            switch (columnas[i]) {
                case Document.COLUMN_DOCUMENT_ID: o[i] = id(f); break;
                case Document.COLUMN_DISPLAY_NAME: o[i] = f.getName(); break;
                case Document.COLUMN_MIME_TYPE: o[i] = tipo(f); break;
                case Document.COLUMN_SIZE: o[i] = f.isDirectory() ? null : f.length(); break;
                case Document.COLUMN_LAST_MODIFIED: o[i] = f.lastModified(); break;
                default: throw new IllegalArgumentException("columna " + columnas[i]);
            }
        }
        return o;
    }

    public Cursor query(Uri u, String[] columnas) {
        File f = de(u);
        if (!f.exists()) {
            return null;
        }
        List<Object[]> filas = new ArrayList<>();
        if (u.toString().endsWith("/children")) {
            File[] hs = f.listFiles();
            if (hs == null) {
                return null;
            }
            for (File h : hs) {
                filas.add(fila(h, columnas));
            }
        } else {
            filas.add(fila(f, columnas));
        }
        return new Cursor(filas);
    }

    /** buildUniqueFile de Android. */
    private static File unico(File dir, String nombre) {
        File f = new File(dir, nombre);
        if (!f.exists()) {
            return f;
        }
        int p = nombre.lastIndexOf('.');
        String b = p < 0 ? nombre : nombre.substring(0, p);
        String e = p < 0 ? "" : nombre.substring(p);
        for (int n = 1; ; n++) {
            f = new File(dir, b + " (" + n + ")" + e);
            if (!f.exists()) {
                return f;
            }
        }
    }

    public Uri crear(Uri padre, String tipo, String nombre) throws FileNotFoundException {
        File dir = de(padre);
        if (!dir.isDirectory()) {
            throw new FileNotFoundException("no está " + padre);
        }
        if (!Document.MIME_TYPE_DIR.equals(tipo) && !"application/octet-stream".equals(tipo)) {
            int p = nombre.lastIndexOf('.');
            String porExt = p < 0 ? null : MimeTypeMap.getSingleton().getMimeTypeFromExtension(nombre.substring(p + 1));
            if (!tipo.equals(porExt)) {
                nombre = nombre + "." + MimeTypeMap.getSingleton().getExtensionFromMimeType(tipo);
            }
        }
        File f = unico(dir, nombre);
        try {
            boolean ok = Document.MIME_TYPE_DIR.equals(tipo) ? f.mkdir() : f.createNewFile();
            if (!ok) {
                return null;
            }
        } catch (IOException e) {
            return null;
        }
        return uri(padre, f);
    }

    public Uri renombrar(Uri doc, String nombre) throws FileNotFoundException {
        File f = de(doc);
        if (!f.exists()) {
            throw new FileNotFoundException("no está " + doc);
        }
        File nuevo = unico(f.getParentFile(), nombre);
        return f.renameTo(nuevo) ? uri(doc, nuevo) : null;
    }

    public Uri mover(Uri doc, Uri origen, Uri destino) throws FileNotFoundException {
        if (sinMover) {
            throw new UnsupportedOperationException("Move not supported");
        }
        File f = de(doc);
        if (!f.exists()) {
            throw new FileNotFoundException("no está " + doc);
        }
        if (!f.getParentFile().equals(de(origen))) {
            throw new IllegalArgumentException("no cuelga de " + origen);
        }
        File nuevo = new File(de(destino), f.getName());
        if (nuevo.exists()) {
            throw new IllegalStateException("Already exists " + nuevo);
        }
        return f.renameTo(nuevo) ? uri(doc, nuevo) : null;
    }

    public boolean borrar(Uri doc) {
        File f = de(doc);
        return f.exists() && borrarTodo(f);
    }

    private static boolean borrarTodo(File f) {
        File[] hs = f.listFiles();
        if (hs != null) {
            for (File h : hs) {
                borrarTodo(h);
            }
        }
        return f.delete();
    }

    public ParcelFileDescriptor abrir(Uri doc, String modo) throws FileNotFoundException {
        File f = de(doc);
        if (!f.isFile()) {
            throw new FileNotFoundException("no está " + doc);
        }
        int fd = siguiente++;
        abiertos.put(fd, DocumentsContract.getDocumentId(doc) + " " + modo);
        return new ParcelFileDescriptor(fd);
    }
}
