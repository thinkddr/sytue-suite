// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos

package eu.sytue.nativo;

import java.util.Arrays;
import java.util.Collections;
import java.util.Map;
import java.util.LinkedHashMap;

public final class SesionTrabajoPrueba {
    private static final String A = "a".repeat(64);
    private static final String B = "b".repeat(64);
    private static final String CUENTA = "sesion-" + "1".repeat(32);
    private static final String MAIL = "eu.sytue.mail";
    private static final String DRIVE = "eu.sytue.drive";
    private static final String AMBITO_A = "v1.eyJ0ZW5hbnQiOiJhY21lIiwic3ViamVjdCI6ImFuYSIsInByb2R1Y3QiOiJtYWlsIn0";
    private static final String AMBITO_B = "v1.eyJ0ZW5hbnQiOiJhbm90aGVyIiwic3ViamVjdCI6ImFuYSIsInByb2R1Y3QiOiJtYWlsIn0";

    private static String n(int valor) { return String.format("%032x", valor); }
    private static void exigir(boolean ok, String mensaje) {
        if (!ok) { throw new AssertionError(mensaje); }
    }
    private static void rechaza(Runnable accion, String mensaje) {
        try { accion.run(); } catch (SecurityException esperado) { return; }
        throw new AssertionError(mensaje);
    }
    private static SesionTrabajo.Marca activa() {
        return SesionTrabajo.publicar(SesionTrabajo.vacia(n(1)), A, true, true, n(2));
    }
    private static SesionTrabajo.Marca pausada(SesionTrabajo.Marca actual) {
        return SesionTrabajo.pausar(actual, actual, true, n(3));
    }

    private static void marcador() {
        SesionTrabajo.Marca inicial = SesionTrabajo.vacia(n(1));
        rechaza(() -> SesionTrabajo.publicar(inicial, A, true, false, n(2)), "unverified token context accepted");
        SesionTrabajo.Marca a = activa();
        SesionTrabajo.Marca p = pausada(a);
        exigir(SesionTrabajo.publicar(p, A, true, true, n(4)) == p, "refresh rotated the nonce or resumed paused work");
        rechaza(() -> SesionTrabajo.proteger(p, a, false), "paused login admitted new SH work");
        SesionTrabajo.Marca cerrado = SesionTrabajo.publicar(a, "", false, true, n(4));
        SesionTrabajo.Marca vuelto = SesionTrabajo.publicar(cerrado, A, true, true, n(5));
        rechaza(() -> SesionTrabajo.vigente(vuelto, a, true), "interactive A to A reused the old witness");
        SesionTrabajo.Marca b = SesionTrabajo.publicar(a, B, true, true, n(6));
        SesionTrabajo.Marca aba = SesionTrabajo.publicar(b, A, true, true, n(7));
        rechaza(() -> SesionTrabajo.vigente(aba, a, true), "A to B to A accepted old work");
        Object proceso = new Object();
        SesionTrabajo.Exclusiva ex = SesionTrabajo.confirmarExclusiva(CUENTA, p, a, proceso, MAIL, n(8));
        SesionTrabajo.Marca otraPausa = SesionTrabajo.pausar(SesionTrabajo.pausar(p, a, false, ""), a, true, n(9));
        rechaza(() -> SesionTrabajo.exigirExclusiva(ex, CUENTA, otraPausa, proceso), "ticket survived another close cycle");
    }

    private static void clientesYRenovacion() {
        Object proceso = new Object();
        SesionTrabajo.Marca a = activa();
        SesionTrabajo.Marca protegida = SesionTrabajo.proteger(a, a, false);
        SesionTrabajo.Resumen cero = SesionTrabajo.resumir(Collections.emptyList(), A, "");
        SesionTrabajo.Trabajo noProtegido = new SesionTrabajo.Trabajo(A, "mail", AMBITO_A, MAIL, 1, Collections.emptySet(), "", false);
        SesionTrabajo.Resumen pendienteSinBit = SesionTrabajo.resumir(Arrays.asList(noProtegido), A, "");
        rechaza(() -> SesionTrabajo.permitirGuardado(CUENTA, a, 0, "", false, proceso, null, pendienteSinBit, false), "missing protection flag hid pending work from an old client");
        rechaza(() -> SesionTrabajo.empezar(SesionTrabajo.vacio(A, "mail", AMBITO_A, MAIL), a, a, proceso, MAIL, n(23)), "WAL admitted before broker protection was durable");
        rechaza(() -> SesionTrabajo.permitirGuardado(CUENTA, protegida, 0, "", true, proceso, null, cero, false), "old client closed protected login");
        rechaza(() -> SesionTrabajo.permitirGuardado(CUENTA, protegida, 0, "", false, proceso, null, cero, false), "old client replaced protected login");
        SesionTrabajo.Marca p = pausada(protegida);
        // OIDC refresh needs no SH lease: logout EX must not wait for a reporter
        // that tries to acquire SH again while the same account is paused.
        SesionTrabajo.permitirGuardado(CUENTA, p, 1, p.revision, false, proceso, null, cero, false);
        SesionTrabajo.Exclusiva ex = SesionTrabajo.confirmarExclusiva(CUENTA, p, a, proceso, MAIL, n(10));
        SesionTrabajo.permitirGuardado(CUENTA, p, 1, p.revision, true, proceso, ex, cero, false);
        rechaza(() -> SesionTrabajo.permitirGuardado(CUENTA, p, 1, p.revision, true, new Object(), ex, cero, false), "another process used the close ticket");
    }

    private static void diarioDurable() {
        Object proceso = new Object();
        SesionTrabajo.Marca original = activa();
        SesionTrabajo.Marca a = SesionTrabajo.proteger(original, original, false);
        SesionTrabajo.Trabajo vacio = SesionTrabajo.vacio(A, "mail", AMBITO_A, MAIL);
        SesionTrabajo.Inicio uno = SesionTrabajo.empezar(vacio, a, a, proceso, MAIL, n(11));
        exigir(uno.trabajo.incierto(), "WAL IO could start without durable dirty state");
        SesionTrabajo.Trabajo otroTenant = new SesionTrabajo.Trabajo(A, "mail", AMBITO_B, MAIL, 0, uno.trabajo.operaciones, "", false);
        rechaza(() -> SesionTrabajo.terminar(otroTenant, a, uno.permiso, proceso, 0, false), "another tenant of the same session completed this work");
        SesionTrabajo.Inicio dos = SesionTrabajo.empezar(uno.trabajo, a, a, proceso, MAIL, n(12));
        SesionTrabajo.Trabajo fallo = SesionTrabajo.terminar(dos.trabajo, a, uno.permiso, proceso, 0, true);
        exigir(fallo == dos.trabajo && fallo.incierto(), "failed report became zero pending");
        SesionTrabajo.Trabajo parcial = SesionTrabajo.terminar(dos.trabajo, a, uno.permiso, proceso, 0, false);
        exigir(parcial.incierto(), "one completed lease hid another active operation");
        rechaza(() -> SesionTrabajo.terminar(parcial, a, dos.permiso, new Object(), 0, false), "foreign process reported pending work");
        SesionTrabajo.Marca b = SesionTrabajo.publicar(a, B, true, true, n(13));
        rechaza(() -> SesionTrabajo.terminar(parcial, b, dos.permiso, proceso, 0, false), "old nonce reported on another login");
        SesionTrabajo.Trabajo completo = SesionTrabajo.terminar(parcial, pausada(a), dos.permiso, proceso, 4, false);
        exigir(completo.pendientes == 4 && !completo.incierto(), "report under paused logout did not finish without a new SH lease");
        SesionTrabajo.Inicio lento = SesionTrabajo.empezar(completo, a, a, proceso, MAIL, n(26));
        SesionTrabajo.Inicio rapido = SesionTrabajo.empezar(lento.trabajo, a, a, proceso, MAIL, n(27));
        SesionTrabajo.Trabajo actualizado = SesionTrabajo.terminar(rapido.trabajo, a, rapido.permiso, proceso, 7, false);
        SesionTrabajo.Trabajo atrasado = SesionTrabajo.terminar(actualizado, a, lento.permiso, proceso, 0, false);
        exigir(atrasado.pendientes == 7 && !atrasado.incierto(), "late shared report erased another job's pending WAL");
        SesionTrabajo.Trabajo verificado = SesionTrabajo.reconciliar(atrasado, CUENTA, a, proceso,
                SesionTrabajo.confirmarExclusiva(CUENTA, a, a, proceso, MAIL, n(28)), 0);
        exigir(verificado.pendientes == 0 && !verificado.incierto(), "exclusive stable metadata could not clear completed work");
        // Recreate only durable state, as after broker/app process death.
        SesionTrabajo.Trabajo reinicio = new SesionTrabajo.Trabajo(A, "mail", AMBITO_A, MAIL, 0, uno.trabajo.operaciones, "", false);
        exigir(SesionTrabajo.resumir(Arrays.asList(reinicio), A, "").incierto, "broker restart erased dirty work");
        SesionTrabajo.Exclusiva ex = SesionTrabajo.confirmarExclusiva(CUENTA, a, a, proceso, MAIL, n(14));
        SesionTrabajo.Trabajo recuperado = SesionTrabajo.reconciliar(reinicio, CUENTA, a, proceso, ex, 3);
        exigir(recuperado.pendientes == 3 && !recuperado.incierto(), "exclusive metadata reconciliation did not preserve pending WAL");
        SesionTrabajo.Exclusiva ajena = SesionTrabajo.confirmarExclusiva(CUENTA, a, a, proceso, DRIVE, n(15));
        rechaza(() -> SesionTrabajo.reconciliar(reinicio, CUENTA, a, proceso, ajena, 0), "another UID's app reconciled this WAL to zero");
        rechaza(() -> SesionTrabajo.empezar(vacio, a, a, proceso, DRIVE, n(16)), "another app opened this scope's WAL");
    }

    private static void desechoEntreApps() {
        Object proceso = new Object();
        SesionTrabajo.Marca a = activa();
        SesionTrabajo.Marca p = pausada(SesionTrabajo.proteger(a, a, false));
        SesionTrabajo.Exclusiva cierre = SesionTrabajo.confirmarExclusiva(CUENTA, p, a, proceso, DRIVE, n(17));
        SesionTrabajo.Trabajo mail = new SesionTrabajo.Trabajo(A, "mail", AMBITO_A, MAIL, 4, Collections.singleton(n(18)), "", false);
        SesionTrabajo.Resumen pendientes = SesionTrabajo.resumir(Arrays.asList(mail), A, p.cicloCierre);
        rechaza(() -> SesionTrabajo.permitirGuardado(CUENTA, p, 1, p.revision, true, proceso, cierre, pendientes, false), "logout ignored another UID's WAL");
        SesionTrabajo.Trabajo orden = SesionTrabajo.pedirDesecho(mail, CUENTA, p, proceso, cierre, p.cicloCierre);
        exigir(orden.pendientes == 4 && orden.incierto() && !orden.desechoAplicado, "discard request pretended another UID had erased its WAL");
        SesionTrabajo.Resumen ordenado = SesionTrabajo.resumir(Arrays.asList(orden), A, p.cicloCierre);
        exigir(ordenado.pendientes == 4 && ordenado.incierto && ordenado.todoConDesecho, "discard acknowledgement state was lost");
        SesionTrabajo.permitirGuardado(CUENTA, p, 1, p.revision, true, proceso, cierre, ordenado, true);
        rechaza(() -> SesionTrabajo.permitirGuardado(CUENTA, p, 1, p.revision, true, proceso, cierre, ordenado, false), "implicit discard accepted pending work");
        rechaza(() -> SesionTrabajo.confirmarDesecho(orden, CUENTA, p, proceso, cierre, p.cicloCierre, 0, false), "Drive acknowledged Mail's private UID cleanup");
        SesionTrabajo.Exclusiva propio = SesionTrabajo.confirmarExclusiva(CUENTA, p, a, proceso, MAIL, n(19));
        rechaza(() -> SesionTrabajo.reconciliar(orden, CUENTA, p, proceso, propio, 0), "WAL opened before tombstone cleanup and acknowledgement");
        rechaza(() -> SesionTrabajo.confirmarDesecho(orden, CUENTA, p, proceso, propio, n(20), 0, false), "wrong discard generation acknowledged");
        rechaza(() -> SesionTrabajo.confirmarDesecho(orden, CUENTA, p, proceso, propio, p.cicloCierre, 1, false), "remaining staged data acknowledged as discarded");
        exigir(SesionTrabajo.confirmarDesecho(orden, CUENTA, p, proceso, propio, p.cicloCierre, 0, true) == orden, "failed local delete became acknowledged");
        SesionTrabajo.Trabajo confirmado = SesionTrabajo.confirmarDesecho(orden, CUENTA, p, proceso, propio, p.cicloCierre, 0, false);
        exigir(confirmado.desechoAplicado && !confirmado.incierto() && confirmado.pendientes == 0, "verified cleanup did not acknowledge the tombstone");
        SesionTrabajo.Marca reabierta = SesionTrabajo.pausar(p, a, false, "");
        SesionTrabajo.Inicio nuevo = SesionTrabajo.empezar(confirmado, reabierta, a, proceso, MAIL, n(21));
        SesionTrabajo.Marca nuevaPausa = SesionTrabajo.pausar(reabierta, a, true, n(22));
        exigir(!SesionTrabajo.resumir(Arrays.asList(nuevo.trabajo), A, nuevaPausa.cicloCierre).todoConDesecho,
                "an earlier discard authorized erasing newly created work");
        SesionTrabajo.Trabajo otraCuenta = new SesionTrabajo.Trabajo(B, "mail", AMBITO_B, MAIL, 9, Collections.emptySet(), "", false);
        exigir(SesionTrabajo.resumir(Arrays.asList(confirmado, otraCuenta), A, "").limpio(), "another account's work leaked into this close");
    }

    private static void formatoDurable() {
        SesionTrabajo.Marca protegida = pausada(SesionTrabajo.proteger(activa(), activa(), false));
        Map<String, Object> mapa = SesionTrabajo.marcaEnMapa(protegida);
        SesionTrabajo.Marca vuelto = SesionTrabajo.marcaDesdeMapa(mapa);
        exigir(vuelto.protegida && vuelto.pausada && vuelto.identidad.equals(protegida.identidad)
                && vuelto.revision.equals(protegida.revision) && vuelto.cicloCierre.equals(protegida.cicloCierre),
                "persisted marker lost its protection or close cycle");
        for (String campo : Arrays.asList("version", "identity_hash", "revision", "active", "paused", "protected", "close_cycle")) {
            Map<String, Object> malo = new LinkedHashMap<>(mapa);
            malo.remove(campo);
            rechaza(() -> SesionTrabajo.marcaDesdeMapa(malo), "missing persisted marker field was accepted: " + campo);
        }
        SesionTrabajo.Trabajo dirty = new SesionTrabajo.Trabajo(A, "mail", AMBITO_A, MAIL, 4, Collections.singleton(n(24)), n(25), false);
        Map<String, Object> trabajo = SesionTrabajo.trabajoEnMapa(dirty);
        SesionTrabajo.Trabajo recuperado = SesionTrabajo.trabajoDesdeMapa(trabajo);
        exigir(recuperado.incierto() && recuperado.pendientes == 4 && recuperado.operaciones.equals(dirty.operaciones)
                && recuperado.desecho.equals(dirty.desecho) && !recuperado.desechoAplicado,
                "persisted dirty work or unapplied tombstone became clean");
        for (String campo : Arrays.asList("v", "identity_hash", "product", "scope_header", "package", "pending", "operations", "discard", "discard_applied")) {
            Map<String, Object> malo = new LinkedHashMap<>(trabajo);
            malo.remove(campo);
            rechaza(() -> SesionTrabajo.trabajoDesdeMapa(malo), "missing dirty record field became zero: " + campo);
        }
        Map<String, Object> duplicado = new LinkedHashMap<>(trabajo);
        duplicado.put("operations", Arrays.asList(n(24), n(24)));
        rechaza(() -> SesionTrabajo.trabajoDesdeMapa(duplicado), "duplicate durable operation silently collapsed");
        Map<String, Object> fraccion = new LinkedHashMap<>(trabajo);
        fraccion.put("pending", 4.5);
        rechaza(() -> SesionTrabajo.trabajoDesdeMapa(fraccion), "fractional pending count was rounded");
        Map<String, Object> texto = new LinkedHashMap<>(mapa);
        texto.put("protected", "true");
        rechaza(() -> SesionTrabajo.marcaDesdeMapa(texto), "text protection flag was coerced");
    }

    public static void main(String[] args) {
        formatoDurable();
        marcador();
        clientesYRenovacion();
        diarioDurable();
        desechoEntreApps();
        System.out.println("SESSION_WORK_RULES_PASS");
    }
    private SesionTrabajoPrueba() {}
}
