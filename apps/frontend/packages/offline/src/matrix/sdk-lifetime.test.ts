/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test } from 'bun:test';
import { SDKLifetimePair, type SDKLifetimeMember } from './sdk-lifetime.js';
import { StorageJobs } from './storage-jobs.js';
for (const trigger of [0, 1])
  test('closing SDK ' + trigger + ' waits for both owners before erasing either key', async () => {
    const jobs = [new StorageJobs(), new StorageJobs()];
    const releases: Array<() => void> = [];
    const keys = [new Uint8Array([3]), new Uint8Array([7])];
    const retired = [false, false];
    const operations = jobs.map((job, index) =>
      job
        .run('actual-io', async () => {
          await new Promise<void>((resolve) => {
            releases[index] = resolve;
          });
          return index;
        })
        .catch(() => {}),
    );
    const members = jobs.map((job, index): SDKLifetimeMember => ({
      retire: () => {
        retired[index] = true;
        void job.close();
        void pair.retire();
      },
      drainIO: () => job.close(),
      erase: () => {
        keys[index]!.fill(0);
      },
    })) as [SDKLifetimeMember, SDKLifetimeMember];
    const pair = new SDKLifetimePair(members);
    members[trigger]!.retire();
    let settled = false;
    const closed = pair.retire().then(() => {
      settled = true;
    });
    expect(retired).toEqual([true, true]);
    expect(pair.retired).toBe(true);
    await Promise.resolve();
    expect(settled).toBe(false);
    expect(keys.map((x) => x[0])).toEqual([3, 7]);
    releases[0]!();
    await operations[0];
    expect(settled).toBe(false);
    expect(keys.map((x) => x[0])).toEqual([3, 7]);
    releases[1]!();
    await closed;
    expect(keys.map((x) => x[0])).toEqual([0, 0]);
    await Promise.all(operations);
  });
test('joined failure still attempts both erasures and rejects the common ACK', async () => {
  const erased: number[] = [];
  const pair = new SDKLifetimePair([
    {
      retire: () => {},
      drainIO: async () => {
        throw Error('joined failure');
      },
      erase: () => {
        erased.push(0);
        throw Error('erase failure');
      },
    },
    {
      retire: () => {},
      drainIO: async () => {},
      erase: () => {
        erased.push(1);
      },
    },
  ]);
  await expect(pair.retire()).rejects.toBeInstanceOf(AggregateError);
  expect(erased).toEqual([0, 1]);
});
