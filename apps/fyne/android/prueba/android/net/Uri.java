// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.net;

/**
 * Una Uri de MENTIRA para CarretePrueba: el texto y poco más. {@link #sinConsulta} es la
 * misma sin «?…», que es como la busca el ContentResolver de mentira.
 */
public final class Uri {
    private final String texto;

    private Uri(String texto) {
        this.texto = texto;
    }

    public static Uri parse(String texto) {
        return new Uri(texto);
    }

    public String getLastPathSegment() {
        String s = sinConsulta();
        return s.substring(s.lastIndexOf('/') + 1);
    }

    public String getQuery() {
        int i = texto.indexOf('?');
        return i < 0 ? null : texto.substring(i + 1);
    }

    /** La uri sin su consulta. No es de Android. */
    public String sinConsulta() {
        int i = texto.indexOf('?');
        return i < 0 ? texto : texto.substring(0, i);
    }

    @Override
    public String toString() {
        return texto;
    }

    @Override
    public boolean equals(Object o) {
        return o instanceof Uri && ((Uri) o).texto.equals(texto);
    }

    @Override
    public int hashCode() {
        return texto.hashCode();
    }
}
