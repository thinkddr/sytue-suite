// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import android.net.Uri;

/** Un Intent de MENTIRA para ArbolDocumentosPrueba, con los valores de Android. */
public class Intent {
    public static final String ACTION_OPEN_DOCUMENT_TREE = "android.intent.action.OPEN_DOCUMENT_TREE";
    public static final String ACTION_VIEW = "android.intent.action.VIEW";
    public static final String ACTION_SEND = "android.intent.action.SEND";
    public static final String ACTION_CHOOSER = "android.intent.action.CHOOSER";
    public static final String EXTRA_STREAM = "android.intent.extra.STREAM";
    public static final int FLAG_GRANT_READ_URI_PERMISSION = 0x00000001;
    public static final int FLAG_GRANT_WRITE_URI_PERMISSION = 0x00000002;
    public static final int FLAG_GRANT_PERSISTABLE_URI_PERMISSION = 0x00000040;
    public static final int FLAG_GRANT_PREFIX_URI_PERMISSION = 0x00000080;
    public static final int FLAG_ACTIVITY_NEW_TASK = 0x10000000;

    public final String accion;
    public Uri datos;
    public String tipo;
    public Uri flujo;
    public Intent dentro;
    public String paquete;
    private int banderas;

    public Intent(String accion) { this.accion = accion; }

    public Intent addFlags(int b) {
        banderas |= b;
        return this;
    }

    public int getFlags() { return banderas; }

    public Uri getData() { return datos; }

    public Intent setData(Uri u) {
        datos = u;
        return this;
    }

    public Intent setDataAndType(Uri u, String t) {
        datos = u;
        tipo = t;
        return this;
    }

    public Intent setType(String t) {
        tipo = t;
        return this;
    }

    public Intent setPackage(String p) {
        paquete = p;
        return this;
    }

    public Intent putExtra(String clave, Uri u) {
        flujo = u;
        return this;
    }

    public static Intent createChooser(Intent i, CharSequence titulo) {
        Intent c = new Intent(ACTION_CHOOSER);
        c.dentro = i;
        return c;
    }
}
