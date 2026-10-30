import { describe, expect, test } from 'vitest';

import type { Freight, FreightReference } from '@ui/gen/api/v2/models';

import { findDowngrades } from './downgrade';

const repo = 'example.com/svc-manifests';

const current = (tag: string): FreightReference =>
  ({ images: [{ repoURL: repo, tag }] }) as unknown as FreightReference;

const incoming = (tag: string): Freight =>
  ({ images: [{ repoURL: repo, tag }] }) as unknown as Freight;

describe('findDowngrades', () => {
  test('nothing to compare against', () => {
    expect(findDowngrades(undefined, incoming('1.0.0'))).toEqual([]);
  });

  test('upgrade', () => {
    expect(findDowngrades(current('1.1.33'), incoming('1.1.34'))).toEqual([]);
  });

  test('same version', () => {
    expect(findDowngrades(current('1.1.34'), incoming('1.1.34'))).toEqual([]);
  });

  test('downgrade', () => {
    expect(findDowngrades(current('1.1.34'), incoming('1.1.33'))).toEqual([
      `image ${repo} 1.1.34 → 1.1.33`
    ]);
  });

  test('compares numerically, not lexically', () => {
    expect(findDowngrades(current('1.0.10'), incoming('1.0.9'))).toHaveLength(1);
  });

  test('v-prefixed and short versions', () => {
    expect(findDowngrades(current('v2.0'), incoming('v1.9.0'))).toHaveLength(1);
  });

  test('non-semver tags are not judged', () => {
    expect(findDowngrades(current('latest'), incoming('1.0.0'))).toEqual([]);
  });

  test('different subscriptions of the same repository are not compared', () => {
    const cur = {
      images: [{ repoURL: repo, tag: '2.0.0', subscriptionName: 'stable' }]
    } as unknown as FreightReference;
    const inc = {
      images: [{ repoURL: repo, tag: '1.5.0', subscriptionName: 'lts' }]
    } as unknown as Freight;
    expect(findDowngrades(cur, inc)).toEqual([]);
  });

  test('chart downgrade', () => {
    const cur = {
      charts: [{ repoURL: 'oci://example.com/charts', name: 'cdp-release', version: '4.9220.0' }]
    } as unknown as FreightReference;
    const inc = {
      charts: [{ repoURL: 'oci://example.com/charts', name: 'cdp-release', version: '4.9219.0' }]
    } as unknown as Freight;
    expect(findDowngrades(cur, inc)).toEqual([
      'chart oci://example.com/charts/cdp-release 4.9220.0 → 4.9219.0'
    ]);
  });
});
