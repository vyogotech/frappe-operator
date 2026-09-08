import * as React from 'react';
import {
  ListPageHeader,
  ListPageCreateDropdown,
  ListPageBody,
  ListPageFilter,
  VirtualizedTable,
  TableData,
  useListPageFilter,
  useK8sWatchResource,
  ResourceLink,
  Timestamp,
  RowProps,
  TableColumn,
  useActiveNamespace,
} from '@openshift-console/dynamic-plugin-sdk';
import { Label } from '@patternfly/react-core';
import { FrappeBenchGVK } from '../frappe/models';
import { FrappeBench } from '../frappe/types';
import { benchAppSources, createYAMLPath, navigateTo, phaseColor } from '../frappe/utils';
import { DatabaseLabel, RedisLabel } from './DataServiceLabels';
import PluginRoot from './PluginRoot';

// ── Columns ──────────────────────────────────────────────────────────────────

const columns: TableColumn<FrappeBench>[] = [
  { title: 'Name',         id: 'name' },
  { title: 'Namespace',    id: 'namespace' },
  { title: 'Version',      id: 'version' },
  { title: 'Status',       id: 'status' },
  { title: 'Apps',         id: 'apps' },
  { title: 'Database',     id: 'database' },
  { title: 'Redis',        id: 'redis' },
  { title: 'Tenant Sites', id: 'sites' },
  { title: 'Created',      id: 'created' },
];

// ── Row ───────────────────────────────────────────────────────────────────────

const BenchRow: React.FC<RowProps<FrappeBench>> = ({ obj, activeColumnIDs }) => {
  const phase = obj.status?.phase;
  const apps = benchAppSources(obj);

  return (
    <>
      <TableData id="name" activeColumnIDs={activeColumnIDs}>
        <ResourceLink
          groupVersionKind={FrappeBenchGVK}
          name={obj.metadata!.name}
          namespace={obj.metadata!.namespace}
        />
      </TableData>

      <TableData id="namespace" activeColumnIDs={activeColumnIDs}>
        <ResourceLink kind="Namespace" name={obj.metadata!.namespace} />
      </TableData>

      <TableData id="version" activeColumnIDs={activeColumnIDs}>
        <Label isCompact color="blue">{obj.spec?.frappeVersion ?? '—'}</Label>
      </TableData>

      <TableData id="status" activeColumnIDs={activeColumnIDs}>
        {phase ? (
          <Label isCompact color={phaseColor(phase)}>{phase}</Label>
        ) : '—'}
      </TableData>

      <TableData id="apps" activeColumnIDs={activeColumnIDs}>
        <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
          {apps.map((app) => (
            <Label key={app.name} isCompact>{app.name}</Label>
          ))}
          {apps.length === 0 && '—'}
        </div>
      </TableData>

      <TableData id="database" activeColumnIDs={activeColumnIDs}>
        <DatabaseLabel db={obj.spec?.dbConfig} />
      </TableData>

      <TableData id="redis" activeColumnIDs={activeColumnIDs}>
        <RedisLabel redis={obj.spec?.redisConfig} />
      </TableData>

      <TableData id="sites" activeColumnIDs={activeColumnIDs}>
        {obj.status?.activeSites ?? '—'}
      </TableData>

      <TableData id="created" activeColumnIDs={activeColumnIDs}>
        <Timestamp timestamp={obj.metadata!.creationTimestamp!} />
      </TableData>
    </>
  );
};

// ── Page ─────────────────────────────────────────────────────────────────────

export const BenchesDashboard: React.FC = () => {
  const [activeNamespace] = useActiveNamespace();
  const [benches, loaded, loadError] = useK8sWatchResource<FrappeBench[]>({
    groupVersionKind: FrappeBenchGVK,
    isList: true,
    namespaced: false,
  });

  const [staticData, filteredData, onFilterChange] = useListPageFilter(benches ?? []);

  const namespace = activeNamespace !== '#ALL_NS#' ? activeNamespace : undefined;

  const onCreate = React.useCallback(
    (item: string) => {
      navigateTo(
        item === 'form'
          ? `/frappe/benches/~new${namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''}`
          : createYAMLPath(FrappeBenchGVK, namespace),
      );
    },
    [namespace],
  );

  return (
    <>
      <ListPageHeader title="Frappe Benches">
        <ListPageCreateDropdown
          items={{ form: 'With Form', yaml: 'With YAML' }}
          onClick={onCreate}
          createAccessReview={{ groupVersionKind: FrappeBenchGVK, namespace }}
        >
          Create Bench
        </ListPageCreateDropdown>
      </ListPageHeader>

      <ListPageBody>
        <ListPageFilter
          data={staticData}
          loaded={loaded}
          onFilterChange={onFilterChange}
        />
        <VirtualizedTable<FrappeBench>
          data={filteredData}
          unfilteredData={benches ?? []}
          loaded={loaded}
          loadError={loadError}
          columns={columns}
          Row={BenchRow}
        />
      </ListPageBody>
    </>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const BenchesDashboardPage: React.FC<React.ComponentProps<typeof BenchesDashboard>> = (props) => (
  <PluginRoot>
    <BenchesDashboard {...props} />
  </PluginRoot>
);

export default BenchesDashboardPage;
