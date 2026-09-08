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
import { FrappeSiteGVK } from '../frappe/models';
import { FrappeBench, FrappeSite } from '../frappe/types';
import { benchRefOf } from '../frappe/utils';
import { DatabaseLabel, RedisLabel } from './DataServiceLabels';
import PluginRoot from './PluginRoot';

export interface BenchDataServicesTabProps {
  obj?: FrappeBench;
}

const Muted: React.FC<{ children: React.ReactNode }> = ({ children }) => (
  <span style={{ color: 'var(--pf-t--global--text--color--subtle)' }}>{children}</span>
);

/**
 * The data services a bench provides to its sites: the database defaults every
 * site inherits unless it overrides them, and the Redis that all of them share.
 */
export const BenchDataServicesTab: React.FC<BenchDataServicesTabProps> = ({ obj }) => {
  const db = obj?.spec?.dbConfig;
  const redis = obj?.spec?.redisConfig;
  const namespace = obj?.metadata?.namespace;
  const benchName = obj?.metadata?.name;

  const [sites] = useK8sWatchResource<FrappeSite[]>(
    namespace ? { groupVersionKind: FrappeSiteGVK, isList: true, namespace } : null,
  );

  // Sites that pin their own provider rather than taking the bench default.
  const overridingSites = React.useMemo(
    () =>
      (sites ?? []).filter(
        (s) => benchRefOf(s)?.name === benchName && !!s.spec?.dbConfig?.provider,
      ),
    [sites, benchName],
  );

  const instanceRef =
    db?.provider === 'postgres' ? db?.postgresRef?.name : db?.mariadbRef?.name;

  return (
    <div className="co-m-pane__body">
      <Title headingLevel="h2" size="xl" style={{ marginBottom: '16px' }}>
        Data Services
      </Title>

      <Grid hasGutter>
        {/* ── Database ──────────────────────────────────────────── */}
        <GridItem md={6}>
          <Card isFullHeight>
            <CardTitle>Database defaults</CardTitle>
            <CardBody>
              <DescriptionList isCompact columnModifier={{ default: '1Col' }}>
                <DescriptionListGroup>
                  <DescriptionListTerm>Configuration</DescriptionListTerm>
                  <DescriptionListDescription>
                    <DatabaseLabel db={db} />
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
                      <DescriptionListTerm>Credentials</DescriptionListTerm>
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
                    <DescriptionListGroup>
                      <DescriptionListTerm>Instance</DescriptionListTerm>
                      <DescriptionListDescription>
                        {instanceRef ? (
                          <code>{instanceRef}</code>
                        ) : (
                          <Muted>Operator-managed default</Muted>
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                    {db?.provider === 'postgres' && db?.mode === 'dedicated' && (
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
                  <DescriptionListTerm>Sites overriding this</DescriptionListTerm>
                  <DescriptionListDescription>
                    {overridingSites.length === 0 ? (
                      <Muted>None — every site uses the bench default</Muted>
                    ) : (
                      <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                        {overridingSites.map((s) => (
                          <ResourceLink
                            key={s.metadata?.name}
                            groupVersionKind={FrappeSiteGVK}
                            name={s.metadata?.name}
                            namespace={s.metadata?.namespace}
                            inline
                          />
                        ))}
                      </div>
                    )}
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
                    <RedisLabel redis={redis} />
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
                      <DescriptionListTerm>Credentials</DescriptionListTerm>
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
                  <>
                    <DescriptionListGroup>
                      <DescriptionListTerm>Image</DescriptionListTerm>
                      <DescriptionListDescription>
                        {redis?.image ? <code>{redis.image}</code> : <Muted>Operator default</Muted>}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
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
                    <DescriptionListGroup>
                      <DescriptionListTerm>Volume size</DescriptionListTerm>
                      <DescriptionListDescription>
                        {redis?.storageSize ? (
                          <code>{redis.storageSize}</code>
                        ) : (
                          <Muted>Operator default</Muted>
                        )}
                      </DescriptionListDescription>
                    </DescriptionListGroup>
                  </>
                )}

                <DescriptionListGroup>
                  <DescriptionListTerm>Scope</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Muted>
                      Shared by every site on this bench — cache, background queues and socketio.
                    </Muted>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              </DescriptionList>
            </CardBody>
          </Card>
        </GridItem>

        {(db?.provider === 'external' || redis?.external) && (
          <GridItem md={12}>
            <Alert variant="info" isInline title="Externally managed services in use">
              The operator connects to{' '}
              {[
                db?.provider === 'external' ? 'the database' : null,
                redis?.external ? 'Redis' : null,
              ]
                .filter(Boolean)
                .join(' and ')}{' '}
              but does not provision, back up or upgrade{' '}
              {db?.provider === 'external' && redis?.external ? 'them' : 'it'}. Site backups still
              cover the Frappe data itself.
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
const BenchDataServicesTabPage: React.FC<React.ComponentProps<typeof BenchDataServicesTab>> = (props) => (
  <PluginRoot>
    <BenchDataServicesTab {...props} />
  </PluginRoot>
);

export default BenchDataServicesTabPage;
