import * as React from 'react';
import {
  K8sResourceCommon,
  ResourceLink,
  VirtualizedTable,
  TableData,
  RowProps,
  TableColumn,
} from '@openshift-console/dynamic-plugin-sdk';
import {
  Card,
  CardBody,
  CardTitle,
  Label,
  Title,
  EmptyState,
  EmptyStateBody,
} from '@patternfly/react-core';
import PluginRoot from './PluginRoot';

// ── Types ────────────────────────────────────────────────────────────────────

export interface BenchFPMTabProps {
  obj?: K8sResourceCommon & {
    spec?: {
      apps?: (string | { name: string; source: string })[];
      fpmConfig?: {
        repositories?: {
          name: string;
          url?: string;
          priority?: number;
          authSecretRef?: { name: string };
        }[];
      };
    };
  };
}

interface FPMRepoRowItem {
  name: string;
  url?: string;
  priority?: number;
  authSecretName?: string;
  namespace?: string;
}

const repoColumns: TableColumn<FPMRepoRowItem>[] = [
  { title: 'Repository Name', id: 'name' },
  { title: 'Endpoint URL',    id: 'url' },
  { title: 'Priority',        id: 'priority' },
  { title: 'Auth Secret',     id: 'authSecret' },
];

const FPMRepoRow: React.FC<RowProps<FPMRepoRowItem>> = ({ obj, activeColumnIDs }) => (
  <>
    <TableData id="name" activeColumnIDs={activeColumnIDs}>
      <strong>{obj.name}</strong>
    </TableData>
    <TableData id="url" activeColumnIDs={activeColumnIDs}>
      <code>{obj.url ?? '—'}</code>
    </TableData>
    <TableData id="priority" activeColumnIDs={activeColumnIDs}>
      <Label isCompact color="grey">{obj.priority ?? 50}</Label>
    </TableData>
    <TableData id="authSecret" activeColumnIDs={activeColumnIDs}>
      {obj.authSecretName ? (
        <ResourceLink
          kind="Secret"
          name={obj.authSecretName}
          namespace={obj.namespace}
        />
      ) : (
        '—'
      )}
    </TableData>
  </>
);

// ── Component ────────────────────────────────────────────────────────────────

export const BenchFPMTab: React.FC<BenchFPMTabProps> = ({ obj }) => {
  const apps = obj?.spec?.apps ?? [];
  const repos = obj?.spec?.fpmConfig?.repositories ?? [];
  const namespace = obj?.metadata?.namespace;

  const repoRows: FPMRepoRowItem[] = React.useMemo(() => {
    return repos.map((r) => ({
      name: r.name,
      url: r.url,
      priority: r.priority,
      authSecretName: r.authSecretRef?.name,
      namespace,
    }));
  }, [repos, namespace]);

  return (
    <div className="co-m-pane__body">
      {/* Installed Apps */}
      <Card style={{ marginBottom: '16px' }}>
        <CardTitle>Installed Applications</CardTitle>
        <CardBody>
          {apps.length > 0 ? (
            <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
              {apps.map((app) => {
                const name = typeof app === 'string' ? app : app.name;
                const source = typeof app === 'object' ? app.source : undefined;
                return (
                  <Label key={name} color="blue" isCompact>
                    {name}{source ? ` (${source})` : ''}
                  </Label>
                );
              })}
            </div>
          ) : (
            <EmptyState variant="xs">
              <EmptyStateBody>No apps configured on this bench spec.</EmptyStateBody>
            </EmptyState>
          )}
        </CardBody>
      </Card>

      {/* FPM Repositories */}
      <Title headingLevel="h3" size="lg" style={{ marginBottom: '8px' }}>
        Configured FPM Repositories
      </Title>

      {repoRows.length > 0 ? (
        <VirtualizedTable<FPMRepoRowItem>
          data={repoRows}
          unfilteredData={repoRows}
          loaded={true}
          loadError={undefined}
          columns={repoColumns}
          Row={FPMRepoRow}
        />
      ) : (
        <EmptyState variant="xs">
          <EmptyStateBody>
            No FPM repositories configured on this bench. Bench uses operator-level defaults or local images.
          </EmptyStateBody>
        </EmptyState>
      )}
    </div>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const BenchFPMTabPage: React.FC<React.ComponentProps<typeof BenchFPMTab>> = (props) => (
  <PluginRoot>
    <BenchFPMTab {...props} />
  </PluginRoot>
);

export default BenchFPMTabPage;
