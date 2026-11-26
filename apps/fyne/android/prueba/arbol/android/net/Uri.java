// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.net;

import java.net.URLDecoder;
import java.net.URLEncoder;
import java.nio.charset.StandardCharsets;

/**
 * Un android.net.Uri de MENTIRA para ArbolDocumentosPrueba (no va en ninguna APK: está en
 * android/prueba/arbol): el texto tal cual.
 */
public final class Uri {
    private final String s;

    private Uri(String s) { this.s = s; }

    public static Uri parse(String s) { return new Uri(s); }

    public static String encode(String s) { return URLEncoder.encode(s, StandardCharsets.UTF_8); }

    public static String decode(String s) { return URLDecoder.decode(s, StandardCharsets.UTF_8); }

    public String getAuthority() {
        String r = s.substring(s.indexOf("://") + 3);
        int barra = r.indexOf('/');
        return barra < 0 ? r : r.substring(0, barra);
    }

    public String getLastPathSegment() { return decode(s.substring(s.lastIndexOf('/') + 1)); }

    @Override public boolean equals(Object o) { return o instanceof Uri && ((Uri) o).s.equals(s); }

    @Override public int hashCode() { return s.hashCode(); }

    @Override public String toString() { return s; }
}
