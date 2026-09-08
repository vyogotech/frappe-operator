import * as React from 'react';
import { ResourceLink, useK8sWatchResource } from '@openshift-console/dynamic-plugin-sdk';
import {
  Alert,
  Card,
  CardBody,
  CardTitle,
  DescriptionList,
  DescriptionListDescription,
  DescriptionListGroup,
  DescriptionListTerm,
  Grid,
  GridItem,
  Label,
  Title,
} from '@patternfly/react-core';
import { FrappeBenchGVK } from '../frappe/models';
import { FrappeBench, FrappeSite } from '../frappe/types';
import { benchRefOf } from '../frappe/utils';
import { DatabaseLabel, RedisLabel } from './DataServiceLabels';
import PluginRoot from './PluginRoot';

export interface SiteDatabaseTabProps {
  obj?: FrappeSite;
}

const Muted: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <span style={{ color: 'var(--pf-t--global--text--color--subtle)' }}>{children}</span>
);

/**
 * Database, Redis and routing for one site. A site may declare its own
 * `dbConfig` or inherit the bench's, and Redis always comes from the bench, so
 * this resolves both and says which is which rather than showing the site spec
 * alone.
 */
export const SiteDatabaseTab: React.FC<SiteDatabaseTabProps> = ({ obj }) => {
  const namespace = obj?.metadata?.namespace;
  const benchRef = benchRefOf(obj);

  const [bench] = useK8sWatchResource<FrappeBench>(
    benchRef
      ? {
          groupVersionKind: FrappeBenchGVK,
          name: benchRef.name,
          namespace: benchRef.namespace ?? namespace,
        }
      : null,
  );

  const ownDb = obj?.spec?.dbConfig?.provider ? obj.spec.dbConfig : undefined;
  const db = ownDb ?? bench?.spec?.dbConfig;
  const isInherited = !ownDb;
  const redis = bench?.spec?.redisConfig;

  const route = obj?.spec?.routeConfig ?? {};
  const status = obj?.status ?? {};
  const instanceRef = db?.provider === 'postgres' ? db?.postgresRef?.name : db?.mariadbRef?.name;

  return (
    <div className="co-m-pane__body">
      <Title headingLevel="h2" size="xl" style={{ marginBottom: '16px' }}>
        Database, Redis &amp; Route
      </Title>

      <Grid hasGutter>
        {/* ── Database ──────────────────────────────────────────── */}
        <GridItem md={6}>
          <Card isFullHeight>
            <CardTitle>Database</CardTitle>
            <CardBody>
              <DescriptionList isCompact columnModifier={{ default: '1Col' }}>
                <DescriptionListGroup>
                  <DescriptionListTerm>Configuration</DescriptionListTerm>
                  <DescriptionListDescription>
                    <DatabaseLabel db={db} />{' '}
                    {isInherited && benchRef && (
                      <Label isCompact color="grey">
                        inherited from {benchRef.name}
                      </Label>
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {db?.provider === 'external' ? (
                  <>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Endpoint</DescriptionListTerm>
                      <DescriptionListDescription>
                        {db.host ? (
                          <code>
                            {db.host}
                            {db.port ? `:${db.port}` : ''}
                          </code>
                        ) : (
                          <Muted>Not set</Muted>
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Connection secret</DescriptionListTerm>
                      <DescriptionListDescription>
                        {db.connectionSecretRef?.name ? (
                          <ResourceLink
                            kind="Secret"
                            name={db.connectionSecretRef.name}
                            namespace={db.connectionSecretRef.namespace ?? namespace}
                          />
                        ) : (
                          <Muted>Not set</Muted>
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  </>
                ) : (
                  <>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Mode</DescriptionListTerm>
                      <DescriptionListDescription>
                        <Label isCompact color="blue">{db?.mode ?? 'shared'}</Label>
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    {instanceRef && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>Instance</DescriptionListTerm>
                        <DescriptionListDescription>
                          <code>{instanceRef}</code>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}
                    {db?.provider === 'postgres' && (
                      <DescriptionListGroup>
                        <DescriptionListTerm>PostgreSQL operator</DescriptionListTerm>
                        <DescriptionListDescription>
                          <Label isCompact color="purple">
                            {db.postgresEngine ?? 'resolved at reconcile time'}
                          </Label>
                        </DescriptionListDescription>
                      </DescriptionListGroup>
                    )}
                  </>
                )}

                <DescriptionListGroup>
                  <DescriptionListTerm>Database name</DescriptionListTerm>
                  <DescriptionListDescription>
                    {status.databaseName ? (
                      <code>{status.databaseName}</code>
                    ) : (
                      <Muted>Assigned during initialization</Muted>
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Site credentials</DescriptionListTerm>
                  <DescriptionListDescription>
                    {status.databaseCredentialsSecret ? (
                      <ResourceLink
                        kind="Secret"
                        name={status.databaseCredentialsSecret}
                        namespace={namespace}
                      />
                    ) : (
                      <Muted>Provisioning…</Muted>
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>On delete</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label
                      isCompact
                      color={obj?.spec?.deletionPolicy === 'Delete' ? 'red' : 'grey'}
                    >
                      {obj?.spec?.deletionPolicy ?? 'Retain'}
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {/* ── Redis ─────────────────────────────────────────────── */}
        <GridItem md={6}>
          <Card isFullHeight>
            <CardTitle>Redis</CardTitle>
            <CardBody>
              <DescriptionList isCompact columnModifier={{ default: '1Col' }}>
                <DescriptionListGroup>
                  <DescriptionListTerm>Configuration</DescriptionListTerm>
                  <DescriptionListDescription>
                    <RedisLabel redis={redis} />{' '}
                    {benchRef && (
                      <Label isCompact color="grey">
                        from {benchRef.name}
                      </Label>
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {redis?.external ? (
                  <>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Endpoint</DescriptionListTerm>
                      <DescriptionListDescription>
                        {redis.host ? (
                          <code>
                            {redis.host}:{redis.port ?? 6379}
                          </code>
                        ) : (
                          <Muted>Not set</Muted>
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Connection secret</DescriptionListTerm>
                      <DescriptionListDescription>
                        {redis.connectionSecretRef?.name ? (
                          <ResourceLink
                            kind="Secret"
                            name={redis.connectionSecretRef.name}
                            namespace={redis.connectionSecretRef.namespace ?? namespace}
                          />
                        ) : (
                          <Muted>Unauthenticated</Muted>
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  </>
                ) : (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Max memory</DescriptionListTerm>
                    <DescriptionListDescription>
                      {redis?.maxMemory ? (
                        <code>{redis.maxMemory}</code>
                      ) : (
                        <Muted>Operator default</Muted>
                      )}
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}

                <DescriptionListGroup>
                  <DescriptionListTerm>Scope</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Muted>
                      Configured on the bench and shared with every site on it. Cache, background
                      queues and socketio all use it.
                    </Muted>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {/* ── Route ─────────────────────────────────────────────── */}
        <GridItem md={6}>
          <Card isFullHeight>
            <CardTitle>Route &amp; TLS</CardTitle>
            <CardBody>
              <DescriptionList isCompact columnModifier={{ default: '1Col' }}>
                <DescriptionListGroup>
                  <DescriptionListTerm>Route</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label isCompact color={route.enabled !== false ? 'green' : 'grey'}>
                      {route.enabled !== false ? 'Enabled' : 'Disabled'}
                    </Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>TLS termination</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label isCompact color="blue">{route.tlsTermination ?? 'edge'}</Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Resolved domain</DescriptionListTerm>
                  <DescriptionListDescription>
                    {status.resolvedDomain ? (
                      <code>{status.resolvedDomain}</code>
                    ) : (
                      <Muted>Not resolved yet</Muted>
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                <DescriptionListGroup>
                  <DescriptionListTerm>Live URL</DescriptionListTerm>
                  <DescriptionListDescription>
                    {status.siteURL ? (
                      <a href={status.siteURL} target="_blank" rel="noopener noreferrer">
                        {status.siteURL}
                      </a>
                    ) : (
                      <Muted>Not yet provisioned</Muted>
                    )}
                  </DescriptionListDescription>
                </DescriptionListGroup>

                {benchRef && (
                  <DescriptionListGroup>
                    <DescriptionListTerm>Bench</DescriptionListTerm>
                    <DescriptionListDescription>
                      <ResourceLink
                        groupVersionKind={FrappeBenchGVK}
                        name={benchRef.name}
                        namespace={benchRef.namespace ?? namespace}
                      />
                    </DescriptionListDescription>
                  </DescriptionListGroup>
                )}
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {(db?.provider === 'external' || redis?.external) && (
          <GridItem md={6}>
            <Alert
              variant="info"
              isInline
              isPlain
              title="Externally managed services"
              style={{ height: '100%' }}
            >
              The operator connects to{' '}
              {[
                db?.provider === 'external' ? 'this database' : null,
                redis?.external ? 'this Redis' : null,
              ]
                .filter(Boolean)
                .join(' and ')}{' '}
              but does not provision or upgrade it. Site backups still capture the Frappe data.
            </Alert>
          </GridItem>
        )}
      </Grid>
    </div>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const SiteDatabaseTabPage: React.FC<React.ComponentProps<typeof SiteDatabaseTab>> = (props) => (
  <PluginRoot>
    <SiteDatabaseTab {...props} />
  </PluginRoot>
);

export default SiteDatabaseTabPage;
