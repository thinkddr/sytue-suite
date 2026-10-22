/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Historial — deshacer y rehacer a nivel de CUADERNO: crear, editar, mover y
 * eliminar notas. Es el «Deshacer» de la barra del BasKet clásico, que no era
 * el del texto sino el de las cestas: borras una nota por error y la recuperas
 * con un gesto, sin ir a la papelera.
 *
 * Cada acción sabe deshacerse y rehacerse a sí misma; aquí solo viven las dos
 * pilas y su disciplina: una acción nueva vacía el futuro, y una que falla al
 * deshacerse vuelve a su sitio en vez de perderse.
 */

export interface Accion {
  /** Lo que se deshace, para decirlo: «eliminar nota». */
  etiqueta: string;
  deshacer: () => Promise<void> | void;
  rehacer: () => Promise<void> | void;
}

/** Una acción ya en la pila, con la sesión que la agrupa. */
type Registrada = Accion & { fusion?: string };

export class Historial {
  private pasado: Registrada[] = [];
  private futuro: Registrada[] = [];

  constructor(private readonly limite = 50) {}

  /**
   * `fusion` agrupa en UNA entrada los guardados de una misma sesión de edición.
   * El autoguardado de una nota registra una acción por cada pausa al escribir;
   * sin esto, deshacer la eliminación de un cuaderno obligaría a retroceder
   * antes por cada pausa de la última frase. Se conserva el `deshacer` del
   * primero —el estado previo a empezar a escribir— y se adopta el `rehacer`
   * del último, que es el texto tal como quedó.
   */
  registrar(a: Accion, fusion?: string): void {
    const cima = this.pasado[this.pasado.length - 1];
    if (fusion !== undefined && cima?.fusion === fusion) {
      cima.rehacer = a.rehacer;
      this.futuro = [];
      return;
    }
    // El `fusion` solo se escribe si lo hay: con `exactOptionalPropertyTypes`,
    // una propiedad opcional no admite un `undefined` explícito.
    this.pasado.push({ ...a, ...(fusion === undefined ? {} : { fusion }) });
    if (this.pasado.length > this.limite) this.pasado.shift();
    // Lo que se podía rehacer deja de tener sentido: la rama cambió.
    this.futuro = [];
  }

  get siguienteDeshacer(): Accion | undefined {
    return this.pasado[this.pasado.length - 1];
  }

  get siguienteRehacer(): Accion | undefined {
    return this.futuro[this.futuro.length - 1];
  }

  async deshacer(): Promise<Accion | undefined> {
    const a = this.pasado.pop();
    if (!a) return undefined;
    try {
      await a.deshacer();
    } catch (err) {
      // Si no se pudo, sigue siendo lo siguiente que deshacer.
      this.pasado.push(a);
      throw err;
    }
    this.futuro.push(a);
    return a;
  }

  async rehacer(): Promise<Accion | undefined> {
    const a = this.futuro.pop();
    if (!a) return undefined;
    try {
      await a.rehacer();
    } catch (err) {
      this.futuro.push(a);
      throw err;
    }
    this.pasado.push(a);
    return a;
  }
}
