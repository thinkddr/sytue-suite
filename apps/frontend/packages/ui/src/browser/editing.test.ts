import { describe, expect, test } from 'bun:test';
import {
  editingCommandEnabled,
  editingCommandState,
  editingCommandValue,
  executeEditingCommand,
} from './editing.js';

function fakeDocument(
  overrides: Partial<Record<string, (...args: unknown[]) => unknown>>,
): Document {
  return overrides as unknown as Document;
}

describe('frontera de edición nativa', () => {
  test('delega comandos y consultas en el documento', () => {
    const calls: unknown[][] = [];
    const documentRef = fakeDocument({
      execCommand: (...args) => {
        calls.push(args);
        return true;
      },
      queryCommandState: () => true,
      queryCommandValue: () => 'h2',
      queryCommandEnabled: () => true,
    });

    expect(executeEditingCommand('formatBlock', 'h2', documentRef)).toBe(true);
    expect(calls).toEqual([['formatBlock', false, 'h2']]);
    expect(editingCommandState('bold', documentRef)).toBe(true);
    expect(editingCommandValue('formatBlock', documentRef)).toBe('h2');
    expect(editingCommandEnabled('undo', documentRef)).toBe(true);
  });

  test('falla de forma segura si el motor retira la API', () => {
    const unavailable = () => {
      throw new Error('unsupported');
    };
    const documentRef = fakeDocument({
      execCommand: unavailable,
      queryCommandState: unavailable,
      queryCommandValue: unavailable,
      queryCommandEnabled: unavailable,
    });

    expect(executeEditingCommand('copy', undefined, documentRef)).toBe(false);
    expect(editingCommandState('bold', documentRef)).toBe(false);
    expect(editingCommandValue('formatBlock', documentRef)).toBe('');
    expect(editingCommandEnabled('undo', documentRef)).toBe(false);
  });
});
