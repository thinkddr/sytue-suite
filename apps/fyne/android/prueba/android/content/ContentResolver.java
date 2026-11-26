// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import android.database.Cursor;
import android.database.MatrixCursor;
import android.net.Uri;
import java.io.ByteArrayInputStream;
import java.io.ByteArrayOutputStream;
import java.io.FileNotFoundException;
import java.io.InputStream;
import java.io.OutputStream;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HashMap;
import java.util.HashSet;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Un ContentResolver de MENTIRA para CarretePrueba, con MediaStore dentro: las tablas
 * ({@link #tablas}: la uri de la tabla → sus filas, columna → valor) y los bytes de cada
 * elemento ({@link #contenido}: la uri del elemento → sus bytes) los pone la prueba. De la
 * selección solo entiende la del carrete («RELATIVE_PATH … NOT LIKE ?»). Una tabla de
 * {@link #prohibidas} lanza SecurityException, como sin el permiso de esa tabla. Apunta las
 * consultas en {@link #consultas} y las uris abiertas en {@link #abiertas}.
 */
public class ContentResolver {
    public final Map<String, List<Map<String, Object>>> tablas = new HashMap<>();
    public final Map<String, byte[]> contenido = new HashMap<>();
    public final Set<String> prohibidas = new HashSet<>();
    public final List<String> consultas = new ArrayList<>();
    public final List<String> abiertas = new ArrayList<>();
    private long siguiente = 1000;

    /** Una fila nueva en la tabla de uri. No es de Android. */
    public Map<String, Object> fila(String tabla, Object... columnasYValores) {
        Map<String, Object> f = new LinkedHashMap<>();
        for (int i = 0; i + 1 < columnasYValores.length; i += 2) {
            f.put((String) columnasYValores[i], columnasYValores[i + 1]);
        }
        tablas.computeIfAbsent(tabla, k -> new ArrayList<>()).add(f);
        return f;
    }

    public final Cursor query(Uri uri, String[] columnas, String seleccion, String[] args, String orden) {
        String tabla = uri.sinConsulta();
        consultas.add(tabla + " " + String.join(",", columnas) + (seleccion == null ? "" : " WHERE " + seleccion));
        if (prohibidas.contains(tabla)) {
            throw new SecurityException("sin permiso para " + tabla);
        }
        List<Map<String, Object>> filas = new ArrayList<>(tablas.getOrDefault(tabla, List.of()));
        if (seleccion != null && seleccion.contains("NOT LIKE ?") && args != null && args.length == 1) {
            String prefijo = args[0].endsWith("%") ? args[0].substring(0, args[0].length() - 1) : args[0];
            filas.removeIf(f -> f.get("relative_path") != null && f.get("relative_path").toString().startsWith(prefijo));
        }
        filas.sort(Comparator.comparingLong(f -> ((Number) f.get("_id")).longValue()));
        MatrixCursor c = new MatrixCursor(columnas);
        for (Map<String, Object> f : filas) {
            Object[] v = new Object[columnas.length];
            for (int i = 0; i < columnas.length; i++) {
                v[i] = f.get(columnas[i]);
            }
            c.addRow(v);
        }
        return c;
    }

    public final InputStream openInputStream(Uri uri) throws FileNotFoundException {
        abiertas.add(uri.toString());
        byte[] b = contenido.get(uri.sinConsulta());
        if (b == null) {
            throw new FileNotFoundException("no existe: " + uri);
        }
        return new ByteArrayInputStream(b);
    }

    public final OutputStream openOutputStream(Uri uri) {
        String clave = uri.sinConsulta();
        return new ByteArrayOutputStream() {
            @Override
            public void close() {
                contenido.put(clave, toByteArray());
            }
        };
    }

    public final Uri insert(Uri tabla, ContentValues valores) {
        long id = siguiente++;
        Map<String, Object> f = fila(tabla.sinConsulta(), "_id", id);
        f.putAll(valores.valores);
        return ContentUris.withAppendedId(tabla, id);
    }

    private Map<String, Object> buscar(Uri uri) {
        String s = uri.sinConsulta();
        String tabla = s.substring(0, s.lastIndexOf('/'));
        long id = ContentUris.parseId(uri);
        for (Map<String, Object> f : tablas.getOrDefault(tabla, List.of())) {
            if (((Number) f.get("_id")).longValue() == id) {
                return f;
            }
        }
        return null;
    }

    public final int update(Uri uri, ContentValues valores, String seleccion, String[] args) {
        Map<String, Object> f = buscar(uri);
        if (f == null) {
            return 0;
        }
        f.putAll(valores.valores);
        return 1;
    }

    public final int delete(Uri uri, String seleccion, String[] args) {
        Map<String, Object> f = buscar(uri);
        if (f == null) {
            return 0;
        }
        String s = uri.sinConsulta();
        tablas.get(s.substring(0, s.lastIndexOf('/'))).remove(f);
        contenido.remove(s);
        return 1;
    }
}
