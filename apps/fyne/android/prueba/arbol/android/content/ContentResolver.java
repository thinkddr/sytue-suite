// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.content;

import android.database.Cursor;
import android.net.Uri;
import android.os.ParcelFileDescriptor;
import android.provider.ProveedorFalso;
import java.io.FileNotFoundException;
import java.util.ArrayList;
import java.util.List;

/** Un ContentResolver de MENTIRA para ArbolDocumentosPrueba: habla con {@link ProveedorFalso}. */
public class ContentResolver {
    public final List<UriPermission> guardados = new ArrayList<>();
    /** Si es true, el proveedor no deja guardar el permiso. */
    public boolean sinPersistir;

    public Cursor query(Uri u, String[] columnas, String sel, String[] args, String orden) {
        return ProveedorFalso.actual.query(u, columnas);
    }

    public void takePersistableUriPermission(Uri u, int banderas) {
        if (sinPersistir) {
            throw new SecurityException("no persistable");
        }
        guardados.add(new UriPermission(u, banderas));
    }

    public void releasePersistableUriPermission(Uri u, int banderas) {
        if (!guardados.removeIf(p -> p.getUri().equals(u))) {
            throw new SecurityException("no estaba");
        }
    }

    public List<UriPermission> getPersistedUriPermissions() { return guardados; }

    public ParcelFileDescriptor openFileDescriptor(Uri u, String modo) throws FileNotFoundException {
        return ProveedorFalso.actual.abrir(u, modo);
    }
}
