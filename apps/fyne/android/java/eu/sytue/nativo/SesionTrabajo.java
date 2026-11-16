// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Collections;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.LinkedHashSet;
import java.util.Set;

/** Pure rules for the encrypted broker record. No token, file or Binder access. */
final class SesionTrabajo {
    static final int VERSION = 1;

    static final class Marca {
        final String identidad;
        final String revision;
        final boolean activa;
        final boolean pausada;
        final boolean protegida;
        final String cicloCierre;

        Marca(String identidad, String revision, boolean activa, boolean pausada,
                boolean protegida, String cicloCierre) {
            exigir(nonce(revision) && (!activa || hash(identidad))
                    && (activa || identidad.isEmpty()) && (!pausada || activa)
                    && (pausada ? nonce(cicloCierre) : cicloCierre.isEmpty()), "Invalid work witness");
            this.identidad = identidad;
            this.revision = revision;
            this.activa = activa;
            this.pausada = pausada;
            this.protegida = protegida;
            this.cicloCierre = cicloCierre;
        }
    }

    static Marca vacia(String revision) {
        return new Marca("", revision, false, false, false, "");
    }

    /** verificada means the provider compared the complete context with vault tokens. */
    static Marca publicar(Marca actual, String identidad, boolean activa,
            boolean verificada, String siguienteRevision) {
        exigir(!activa || (verificada && hash(identidad)), "Work identity was not verified against the vault");
        exigir(activa || identidad.isEmpty(), "Inactive work must not name an identity");
        if (actual.activa == activa && actual.identidad.equals(identidad)) {
            return actual; // Refresh must not resume paused work or rotate its nonce.
        }
        exigir(!actual.revision.equals(siguienteRevision), "Work incarnation was reused");
        return new Marca(identidad, siguienteRevision, activa, false, actual.protegida, "");
    }

    static Marca pausar(Marca actual, Marca esperado, boolean pausada, String ciclo) {
        vigente(actual, esperado, true);
        if (actual.pausada == pausada) { return actual; }
        return new Marca(actual.identidad, actual.revision, true, pausada,
                actual.protegida, pausada ? ciclo : "");
    }

    static Marca proteger(Marca actual, Marca esperado, boolean exclusiva) {
        vigente(actual, esperado, exclusiva);
        return new Marca(actual.identidad, actual.revision, true, actual.pausada, true, actual.cicloCierre);
    }

    static void vigente(Marca actual, Marca esperado, boolean permitePausa) {
        exigir(actual.activa && esperado.activa && actual.identidad.equals(esperado.identidad)
                && actual.revision.equals(esperado.revision) && (permitePausa || !actual.pausada),
                "Work belongs to a previous or paused login");
    }

    /** Issued only by the provider after the new client confirms its kernel EX lease. */
    static final class Exclusiva {
        final String cuenta;
        final Marca marca;
        final Object proceso;
        final String paquete;
        final String permiso;
        Exclusiva(String cuenta, Marca marca, Object proceso, String paquete, String permiso) {
            exigir(cuenta != null && cuenta.matches("sesion-[0-9a-f]{32}")
                    && proceso != null && paquete != null && paquete.matches("eu\\.sytue\\.[a-z]+") && nonce(permiso), "Invalid exclusive work proof");
            this.cuenta = cuenta;
            this.marca = marca;
            this.proceso = proceso;
            this.paquete = paquete;
            this.permiso = permiso;
        }
    }

    static Exclusiva confirmarExclusiva(String cuenta, Marca actual, Marca esperado,
            Object proceso, String paquete, String permiso) {
        vigente(actual, esperado, true);
        return new Exclusiva(cuenta, actual, proceso, paquete, permiso);
    }

    static void exigirExclusiva(Exclusiva permiso, String cuenta, Marca actual, Object proceso) {
        exigir(permiso != null && permiso.proceso == proceso && permiso.cuenta.equals(cuenta),
                "Exclusive work proof belongs to another caller");
        vigente(actual, permiso.marca, true);
        exigir(actual.cicloCierre.equals(permiso.marca.cicloCierre), "Exclusive work proof is from an earlier close");
    }

    /** Old clients may never overwrite a protected login, even when work is idle. */
    static void permitirGuardado(String cuenta, Marca actual, int protocolo, String revision,
            boolean cierre, Object proceso, Exclusiva permiso, Resumen trabajo, boolean desechoAutorizado) {
        if (!actual.protegida && (trabajo == null || trabajo.limpio())) { return; }
        exigir(protocolo == VERSION && actual.revision.equals(revision), "Update this app before changing a protected session");
        if (!cierre) { return; } // Same-nonce OIDC refresh never waits for an application work lease.
        exigirExclusiva(permiso, cuenta, actual, proceso);
        exigir(actual.pausada, "Pause and drain work before closing the login");
        exigir(trabajo != null && (trabajo.limpio()
                || (desechoAutorizado && trabajo.todoConDesecho)), "Pending work has not finished or been explicitly discarded");
    }

    static final class Trabajo {
        final String identidad;
        final String producto;
        final String ambito;
        final String paquete;
        final int pendientes;
        final Set<String> operaciones;
        final String desecho;
        final boolean desechoAplicado;
        Trabajo(String identidad, String producto, String ambito, String paquete, int pendientes,
                Set<String> operaciones, String desecho, boolean desechoAplicado) {
            exigir(hash(identidad) && producto != null && producto.matches("[a-z][a-z0-9-]{0,40}")
                    && ambito != null && ambito.matches("v1\\.[A-Za-z0-9_-]{1,4093}")
                    && paquete != null && paquete.matches("eu\\.sytue\\.[a-z]+")
                    && pendientes >= 0 && pendientes <= 1000000 && operaciones != null
                    && operaciones.size() <= 128 && (desecho.isEmpty() || nonce(desecho))
                    && (!desecho.isEmpty() || !desechoAplicado), "Invalid durable pending-work record");
            LinkedHashSet<String> copia = new LinkedHashSet<>(operaciones);
            for (String operacion : copia) { exigir(nonce(operacion), "Invalid pending-work operation"); }
            this.identidad = identidad;
            this.producto = producto;
            this.ambito = ambito;
            this.paquete = paquete;
            this.pendientes = pendientes;
            this.operaciones = Collections.unmodifiableSet(copia);
            this.desecho = desecho;
            this.desechoAplicado = desechoAplicado;
        }
        boolean incierto() { return !operaciones.isEmpty() || (!desecho.isEmpty() && !desechoAplicado); }
        Trabajo con(int numero, Set<String> operaciones, String desecho, boolean aplicado) {
            return new Trabajo(identidad, producto, ambito, paquete, numero, operaciones, desecho, aplicado);
        }
    }

    static Trabajo vacio(String identidad, String producto, String ambito, String paquete) {
        return new Trabajo(identidad, producto, ambito, paquete, 0, Collections.emptySet(), "", false);
    }

    static final class Operacion {
        final Trabajo ambito;
        final Marca marca;
        final Object proceso;
        final String id;
        Operacion(Trabajo ambito, Marca marca, Object proceso, String id) {
            exigir(proceso != null && nonce(id), "Invalid work process or operation");
            this.ambito = ambito;
            this.marca = marca;
            this.proceso = proceso;
            this.id = id;
        }
    }

    static final class Inicio {
        final Trabajo trabajo;
        final Operacion permiso;
        Inicio(Trabajo trabajo, Operacion permiso) { this.trabajo = trabajo; this.permiso = permiso; }
    }

    /** The caller must persist trabajo before returning permiso and before any WAL IO. */
    static Inicio empezar(Trabajo actual, Marca marca, Marca esperado, Object proceso, String paquete, String id) {
        vigente(marca, esperado, false);
        exigir(marca.protegida, "Protect the broker record before opening WAL");
        exigir(actual.identidad.equals(marca.identidad) && actual.paquete.equals(paquete), "Work scope does not belong to this login and app");
        exigir(actual.desecho.isEmpty() || actual.desechoAplicado, "Apply and acknowledge the durable discard before opening WAL");
        LinkedHashSet<String> operaciones = new LinkedHashSet<>(actual.operaciones);
        exigir(operaciones.add(id), "Pending-work operation was reused");
        Trabajo siguiente = actual.con(actual.pendientes, operaciones, actual.desecho, actual.desechoAplicado);
        return new Inicio(siguiente, new Operacion(siguiente, marca, proceso, id));
    }

    /** A failed report retains the durable dirty operation; absence is never zero. */
    static Trabajo terminar(Trabajo actual, Marca marca, Operacion permiso, Object proceso, int pendientes, boolean fallo) {
        exigir(permiso != null, "Missing work report proof");
        vigente(marca, permiso.marca, true);
        exigir(permiso != null && permiso.proceso == proceso && mismoAmbito(actual, permiso.ambito)
                && actual.operaciones.contains(permiso.id) && actual.desecho.equals(permiso.ambito.desecho),
                "Pending-work report is not from its original process and scope");
        if (fallo) { return actual; }
        LinkedHashSet<String> operaciones = new LinkedHashSet<>(actual.operaciones);
        operaciones.remove(permiso.id);
        // SH operations may report out of order. A later stale scan cannot lower
        // a count observed by another operation; only EX reconciliation can.
        return actual.con(Math.max(actual.pendientes, pendientes), operaciones, actual.desecho, actual.desechoAplicado);
    }

    static Trabajo reconciliar(Trabajo actual, String cuenta, Marca marca, Object proceso,
            Exclusiva permiso, int pendientes) {
        exigirExclusiva(permiso, cuenta, marca, proceso);
        exigir(actual.identidad.equals(marca.identidad)
                && actual.paquete.equals(permiso.paquete)
                && (actual.desecho.isEmpty() || actual.desechoAplicado), "Unapplied discard or another app blocks WAL reconciliation");
        return actual.con(pendientes, Collections.emptySet(), actual.desecho, actual.desechoAplicado);
    }

    static Trabajo pedirDesecho(Trabajo actual, String cuenta, Marca marca, Object proceso,
            Exclusiva permiso, String desecho) {
        exigirExclusiva(permiso, cuenta, marca, proceso);
        exigir(marca.pausada && actual.identidad.equals(marca.identidad), "Discard does not belong to this closing login");
        exigir(nonce(desecho) && desecho.equals(marca.cicloCierre) && !desecho.equals(actual.desecho), "Discard is not from this close cycle");
        return actual.con(actual.pendientes, actual.operaciones, desecho, false);
    }

    static Trabajo confirmarDesecho(Trabajo actual, String cuenta, Marca marca, Object proceso,
            Exclusiva permiso, String desecho, int restantes, boolean fallo) {
        exigirExclusiva(permiso, cuenta, marca, proceso);
        exigir(actual.identidad.equals(marca.identidad) && actual.paquete.equals(permiso.paquete)
                && nonce(desecho) && desecho.equals(actual.desecho),
                "Discard acknowledgement belongs to another scope or generation");
        if (fallo) { return actual; }
        exigir(restantes == 0, "Discard must remove and verify all local work before acknowledgement");
        return actual.con(0, Collections.emptySet(), desecho, true);
    }

    static final class Resumen {
        final int pendientes;
        final boolean incierto;
        final boolean todoConDesecho;
        Resumen(int pendientes, boolean incierto, boolean todoConDesecho) {
            this.pendientes = pendientes; this.incierto = incierto; this.todoConDesecho = todoConDesecho;
        }
        boolean limpio() { return pendientes == 0 && !incierto; }
    }

    static Resumen resumir(Iterable<Trabajo> registros, String identidad, String cicloCierre) {
        exigir(hash(identidad), "Invalid pending-work identity");
        int total = 0;
        boolean incierto = false, todoConDesecho = true;
        for (Trabajo trabajo : registros) {
            if (!identidad.equals(trabajo.identidad)) { continue; }
            total = Math.addExact(total, trabajo.pendientes);
            incierto |= trabajo.incierto();
            if (trabajo.pendientes > 0 || trabajo.incierto()) { todoConDesecho &= nonce(cicloCierre) && trabajo.desecho.equals(cicloCierre) && !trabajo.desechoAplicado; }
        }
        return new Resumen(total, incierto, todoConDesecho);
    }

    static Map<String, Object> marcaEnMapa(Marca marca) {
        Map<String, Object> valor = new LinkedHashMap<>();
        valor.put("version", VERSION);
        valor.put("identity_hash", marca.identidad);
        valor.put("revision", marca.revision);
        valor.put("active", marca.activa);
        valor.put("paused", marca.pausada);
        valor.put("protected", marca.protegida);
        valor.put("close_cycle", marca.cicloCierre);
        return valor;
    }

    static Marca marcaDesdeMapa(Map<String, Object> valor) {
        exigir(numero(valor, "version") == VERSION, "Unknown work witness version");
        return new Marca(texto(valor, "identity_hash"), texto(valor, "revision"),
                bandera(valor, "active"), bandera(valor, "paused"), bandera(valor, "protected"), texto(valor, "close_cycle"));
    }

    static Map<String, Object> trabajoEnMapa(Trabajo trabajo) {
        Map<String, Object> valor = new LinkedHashMap<>();
        valor.put("v", VERSION);
        valor.put("identity_hash", trabajo.identidad);
        valor.put("product", trabajo.producto);
        valor.put("scope_header", trabajo.ambito);
        valor.put("package", trabajo.paquete);
        valor.put("pending", trabajo.pendientes);
        valor.put("operations", new ArrayList<>(trabajo.operaciones));
        valor.put("discard", trabajo.desecho);
        valor.put("discard_applied", trabajo.desechoAplicado);
        return valor;
    }

    static Trabajo trabajoDesdeMapa(Map<String, Object> valor) {
        exigir(numero(valor, "v") == VERSION, "Unknown pending-work version");
        Object lista = valor.get("operations");
        exigir(lista instanceof List<?> && ((List<?>) lista).size() <= 128, "Missing or oversized durable dirty operations");
        Set<String> operaciones = new LinkedHashSet<>();
        for (Object operacion : (List<?>) lista) {
            exigir(operacion instanceof String && operaciones.add((String) operacion), "Invalid or duplicate dirty operation");
        }
        return new Trabajo(texto(valor, "identity_hash"), texto(valor, "product"), texto(valor, "scope_header"), texto(valor, "package"),
                numero(valor, "pending"), operaciones, texto(valor, "discard"), bandera(valor, "discard_applied"));
    }

    static String texto(Map<String, Object> valor, String clave) {
        Object campo = valor.get(clave);
        exigir(campo instanceof String, "Missing or mistyped work field");
        return (String) campo;
    }

    static boolean bandera(Map<String, Object> valor, String clave) {
        Object campo = valor.get(clave);
        exigir(campo instanceof Boolean, "Missing or mistyped work flag");
        return (Boolean) campo;
    }

    static int numero(Map<String, Object> valor, String clave) {
        Object campo = valor.get(clave);
        exigir(campo instanceof Integer || campo instanceof Long, "Missing or mistyped work count");
        long numero = ((Number) campo).longValue();
        exigir(numero >= 0 && numero <= 1000000, "Work count is outside its limit");
        return (int) numero;
    }

    static boolean mismoAmbito(Trabajo a, Trabajo b) {
        return a.identidad.equals(b.identidad) && a.producto.equals(b.producto) && a.ambito.equals(b.ambito) && a.paquete.equals(b.paquete);
    }
    private static boolean hash(String valor) { return valor != null && valor.matches("[0-9a-f]{64}"); }
    private static boolean nonce(String valor) { return valor != null && valor.matches("[0-9a-f]{32}"); }
    private static void exigir(boolean ok, String mensaje) {
        if (!ok) { throw new SecurityException(mensaje); }
    }
    private SesionTrabajo() {}
}
