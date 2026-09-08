import * as React from 'react';
import { Label, Tooltip } from '@patternfly/react-core';
import { DatabaseConfig, RedisConfig } from '../frappe/types';

const PROVIDER_LABELS: Record<string, string> = {
  mariadb: 'MariaDB',
  postgres: 'PostgreSQL',
  sqlite: 'SQLite',
  external: 'External',
};

export interface DatabaseLabelProps {
  db?: DatabaseConfig;
  /**
   * Bench this configuration came from, when the resource does not declare its
   * own. Set it only when the value really is inherited — it drives the tooltip
   * that tells the two cases apart in a list.
   */
  inheritedFrom?: string;
}

/**
 * Compact database summary for list columns and detail rows. An external
 * database gets its own colour: it is the one configuration where the operator
 * manages nothing behind the endpoint, which is worth spotting at a glance.
 */
export const DatabaseLabel: React.FC<DatabaseLabelProps> = ({ db, inheritedFrom }) => {
  if (!db?.provider) {
    return <span>—</span>;
  }

  const provider = PROVIDER_LABELS[db.provider] ?? db.provider;
  let content: React.ReactElement;
  let detail: string;

  if (db.provider === 'external') {
    detail = `External database at ${
      db.host ? `${db.host}${db.port ? `:${db.port}` : ''}` : 'an endpoint that is not set'
    }`;
    content = (
      <Label isCompact color="orange">
        External
      </Label>
    );
  } else if (db.provider === 'sqlite') {
    detail = 'SQLite on the bench volume';
    content = (
      <Label isCompact color="grey">
        SQLite
      </Label>
    );
  } else {
    const mode = db.mode ?? 'shared';
    const engine =
      db.provider === 'postgres' && mode === 'dedicated' && db.postgresEngine
        ? ` · ${db.postgresEngine}`
        : '';
    detail = `${provider}, ${mode} mode`;
    content = (
      <Label isCompact color={db.provider === 'postgres' ? 'purple' : 'teal'}>
        {provider} · {mode}
        {engine}
      </Label>
    );
  }

  if (inheritedFrom) {
    return <Tooltip content={`${detail}, inherited from bench ${inheritedFrom}`}>{content}</Tooltip>;
  }
  return <Tooltip content={detail}>{content}</Tooltip>;
};

/** Compact Redis summary. Benches own this configuration; sites inherit it. */
export const RedisLabel: React.FC<{ redis?: RedisConfig }> = ({ redis }) => {
  if (!redis?.type && !redis?.external) {
    return (
      <Label isCompact color="grey">
        Operator default
      </Label>
    );
  }

  const engine = redis.type === 'dragonfly' ? 'Dragonfly' : 'Redis';

  if (redis.external) {
    return (
      <Tooltip
        content={`External ${engine} at ${
          redis.host ? `${redis.host}:${redis.port ?? 6379}` : 'an endpoint that is not set'
        }`}
      >
        <Label isCompact color="orange">
          {engine} · external
        </Label>
      </Tooltip>
    );
  }

  return (
    <Tooltip content={`${engine} provisioned by the operator for this bench`}>
      <Label isCompact color="blue">
        {engine} · in-cluster
      </Label>
    </Tooltip>
  );
};
