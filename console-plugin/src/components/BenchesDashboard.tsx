import React, { useState } from 'react';
import {
  PageSection,
  Title,
  Card,
  CardBody,
  CardTitle,
  Label,
  Button,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  SearchInput,
  EmptyState,
  EmptyStateIcon,
  EmptyStateBody,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Server, Database, Layers, Plus, ExternalLink, ShieldCheck } from 'lucide-react';

export interface FrappeBenchResource {
  name: string;
  namespace: string;
  frappeVersion: string;
  apps: string[];
  storageSize: string;
  ready: boolean;
  activeSites: number;
  fpmReposCount: number;
}

export const BenchesDashboard: React.FC = () => {
  const [filter, setFilter] = useState('');

  // Sample data reflecting active FrappeBench custom resources
  const benches: FrappeBenchResource[] = [
    {
      name: 'production-bench',
      namespace: 'frappe-system',
      frappeVersion: 'version-15',
      apps: ['frappe', 'erpnext', 'hrms'],
      storageSize: '50Gi',
      ready: true,
      activeSites: 12,
      fpmReposCount: 2,
    },
    {
      name: 'staging-bench',
      namespace: 'frappe-staging',
      frappeVersion: 'version-15',
      apps: ['frappe', 'erpnext'],
      storageSize: '20Gi',
      ready: true,
      activeSites: 3,
      fpmReposCount: 1,
    },
  ];

  const filtered = benches.filter(
    (b) =>
      b.name.toLowerCase().includes(filter.toLowerCase()) ||
      b.namespace.toLowerCase().includes(filter.toLowerCase())
  );

  return (
    <PageSection>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <Title headingLevel="h1" size="2xl" style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <Server color="#00BC86" size={28} />
            Frappe Benches
          </Title>
          <p style={{ color: '#6A6E73', marginTop: 4 }}>
            Shared multi-tenant platform infrastructure, runtime worker pools, and storage management.
          </p>
        </div>
        <Button
          variant="primary"
          icon={<Plus size={16} />}
          style={{ backgroundColor: '#0050A4', borderColor: '#0050A4' }}
        >
          Create Bench
        </Button>
      </div>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 16, marginBottom: 24 }}>
        <Card>
          <CardTitle style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Layers size={18} color="#0050A4" />
            Total Benches
          </CardTitle>
          <CardBody>
            <div style={{ fontSize: 28, fontWeight: 700, color: '#141917' }}>{benches.length}</div>
          </CardBody>
        </Card>

        <Card>
          <CardTitle style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Server size={18} color="#00BC86" />
            Active Tenant Sites
          </CardTitle>
          <CardBody>
            <div style={{ fontSize: 28, fontWeight: 700, color: '#00BC86' }}>
              {benches.reduce((acc, b) => acc + b.activeSites, 0)}
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardTitle style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <ShieldCheck size={18} color="#0050A4" />
            FPM Air-Gapped Repos
          </CardTitle>
          <CardBody>
            <div style={{ fontSize: 28, fontWeight: 700, color: '#141917' }}>
              {benches.reduce((acc, b) => acc + b.fpmReposCount, 0)}
            </div>
          </CardBody>
        </Card>
      </div>

      <Card>
        <CardBody>
          <Toolbar>
            <ToolbarContent>
              <ToolbarItem>
                <SearchInput
                  placeholder="Filter benches by name or namespace..."
                  value={filter}
                  onChange={(_e, val) => setFilter(val)}
                  onClear={() => setFilter('')}
                />
              </ToolbarItem>
            </ToolbarContent>
          </Toolbar>

          <Table aria-label="Frappe Benches Table" variant="compact">
            <Thead>
              <Tr>
                <Th>Name</Th>
                <Th>Namespace</Th>
                <Th>Version</Th>
                <Th>Status</Th>
                <Th>Apps</Th>
                <Th>Tenant Sites</Th>
                <Th>FPM Repos</Th>
                <Th>Storage</Th>
              </Tr>
            </Thead>
            <Tbody>
              {filtered.map((bench) => (
                <Tr key={bench.name}>
                  <Td dataLabel="Name" style={{ fontWeight: 600 }}>
                    <a href={`/k8s/ns/${bench.namespace}/vyogo.tech~v1~FrappeBench/${bench.name}`}>
                      {bench.name}
                    </a>
                  </Td>
                  <Td dataLabel="Namespace">
                    <Label color="blue">{bench.namespace}</Label>
                  </Td>
                  <Td dataLabel="Version">
                    <code>{bench.frappeVersion}</code>
                  </Td>
                  <Td dataLabel="Status">
                    <Label color={bench.ready ? 'green' : 'orange'}>
                      {bench.ready ? 'Ready' : 'Configuring'}
                    </Label>
                  </Td>
                  <Td dataLabel="Apps">
                    <div style={{ display: 'flex', gap: 4, flexWrap: 'wrap' }}>
                      {bench.apps.map((app) => (
                        <Label key={app} color="grey" isCompact>
                          {app}
                        </Label>
                      ))}
                    </div>
                  </Td>
                  <Td dataLabel="Tenant Sites">{bench.activeSites} sites</Td>
                  <Td dataLabel="FPM Repos">
                    <Label color="purple" isCompact>
                      {bench.fpmReposCount} Repos
                    </Label>
                  </Td>
                  <Td dataLabel="Storage">{bench.storageSize}</Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </CardBody>
      </Card>
    </PageSection>
  );
};

export default BenchesDashboard;
