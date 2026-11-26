// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package android.os;

/** De MENTIRA para ArbolDocumentosPrueba: un descriptor con el número que le dé el proveedor falso. */
public final class ParcelFileDescriptor {
    private final int fd;

    public ParcelFileDescriptor(int fd) { this.fd = fd; }

    public int detachFd() { return fd; }
}
