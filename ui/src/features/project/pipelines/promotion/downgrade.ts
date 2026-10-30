import semver from 'semver';

import type { Freight, FreightReference } from '@ui/gen/api/v2/models';

// parseVersion mirrors the leniency of the server's check (Masterminds semver):
// a leading "v" and a missing minor/patch are accepted, anything else that is
// not a semantic version is not comparable.
const parseVersion = (version?: string): semver.SemVer | null => {
  if (!version) {
    return null;
  }
  const parsed = semver.parse(version, { loose: true });
  if (parsed) {
    return parsed;
  }
  return /^v?\d+(\.\d+)?$/.test(version) ? semver.coerce(version) : null;
};

const isDowngrade = (from?: string, to?: string): boolean => {
  if (!from || !to || from === to) {
    return false;
  }
  const fromVer = parseVersion(from);
  const toVer = parseVersion(to);
  return Boolean(fromVer && toVer && semver.lt(toVer, fromVer));
};

// Artifacts are paired within the same subscription as well as the same
// repository, like the server does: a Warehouse may subscribe to one
// repository more than once.
//
// findDowngrades lists every artifact of the incoming Freight whose version
// would go down relative to the Freight from the same origin currently on the
// Stage. It mirrors the promotion webhook, which denies such a promotion unless
// it is marked as a rollback; the webhook stays the authority, this only lets
// the drawer ask for the rollback flag up front.
export const findDowngrades = (current?: FreightReference, incoming?: Freight): string[] => {
  if (!current || !incoming) {
    return [];
  }
  const downgrades: string[] = [];
  const check = (what: string, from?: string, to?: string) => {
    if (isDowngrade(from, to)) {
      downgrades.push(`${what} ${from} → ${to}`);
    }
  };

  for (const image of incoming.images || []) {
    for (const cur of current.images || []) {
      if (cur.repoURL === image.repoURL && cur.subscriptionName === image.subscriptionName) {
        check(`image ${image.repoURL}`, cur.tag, image.tag);
      }
    }
  }
  for (const chart of incoming.charts || []) {
    for (const cur of current.charts || []) {
      if (
        cur.repoURL === chart.repoURL &&
        cur.name === chart.name &&
        cur.subscriptionName === chart.subscriptionName
      ) {
        const id = chart.name
          ? `${chart.repoURL?.replace(/\/$/, '')}/${chart.name}`
          : chart.repoURL;
        check(`chart ${id}`, cur.version, chart.version);
      }
    }
  }
  for (const commit of incoming.commits || []) {
    for (const cur of current.commits || []) {
      if (cur.repoURL === commit.repoURL && cur.subscriptionName === commit.subscriptionName) {
        check(`git ${commit.repoURL}`, cur.tag, commit.tag);
      }
    }
  }
  for (const artifact of incoming.artifacts || []) {
    for (const cur of current.artifacts || []) {
      if (cur.subscriptionName === artifact.subscriptionName) {
        check(`artifact ${artifact.subscriptionName}`, cur.version, artifact.version);
      }
    }
  }
  return downgrades;
};
