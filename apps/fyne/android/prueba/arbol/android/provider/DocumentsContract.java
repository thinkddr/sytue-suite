// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.provider;

import android.content.ContentResolver;
import android.net.Uri;
import java.io.FileNotFoundException;

/**
 * Un DocumentsContract de MENTIRA para ArbolDocumentosPrueba: construye los URI como el de
 * Android (`…/tree/<id>/document/<id>[/children]`) y lo demás se lo pide a
 * {@link ProveedorFalso}.
 */
public final class DocumentsContract {
    private DocumentsContract() {}

    /** Las columnas y el tipo de carpeta, con los valores de Android. */
    public static final class Document {
        public static final String COLUMN_DOCUMENT_ID = "document_id";
        public static final String COLUMN_DISPLAY_NAME = "_display_name";
        public static final String COLUMN_MIME_TYPE = "mime_type";
        public static final String COLUMN_SIZE = "_size";
        public static final String COLUMN_LAST_MODIFIED = "last_modified";
        public static final String MIME_TYPE_DIR = "vnd.android.document/directory";

        private Document() {}
    }

    static String arbol(Uri u) {
        String s = u.toString();
        int doc = s.indexOf("/document/");
        return doc < 0 ? s : s.substring(0, doc);
    }

    static String trozoTras(Uri u, String marca) {
        String s = u.toString();
        int i = s.indexOf(marca);
        if (i < 0) {
            throw new IllegalArgumentException("sin " + marca + ": " + s);
        }
        String r = s.substring(i + marca.length());
        int barra = r.indexOf('/');
        return Uri.decode(barra < 0 ? r : r.substring(0, barra));
    }

    public static Uri buildDocumentUriUsingTree(Uri arbol, String id) {
        return Uri.parse(arbol(arbol) + "/document/" + Uri.encode(id));
    }

    public static Uri buildChildDocumentsUriUsingTree(Uri arbol, String id) {
        return Uri.parse(buildDocumentUriUsingTree(arbol, id) + "/children");
    }

    public static String getTreeDocumentId(Uri u) { return trozoTras(u, "/tree/"); }

    public static String getDocumentId(Uri u) { return trozoTras(u, "/document/"); }

    public static Uri createDocument(ContentResolver cr, Uri padre, String tipo, String nombre) throws FileNotFoundException {
        return ProveedorFalso.actual.crear(padre, tipo, nombre);
    }

    public static Uri renameDocument(ContentResolver cr, Uri doc, String nombre) throws FileNotFoundException {
        return ProveedorFalso.actual.renombrar(doc, nombre);
    }

    public static Uri moveDocument(ContentResolver cr, Uri doc, Uri origen, Uri destino) throws FileNotFoundException {
        return ProveedorFalso.actual.mover(doc, origen, destino);
    }

    public static boolean deleteDocument(ContentResolver cr, Uri doc) throws FileNotFoundException {
        return ProveedorFalso.actual.borrar(doc);
    }
}
