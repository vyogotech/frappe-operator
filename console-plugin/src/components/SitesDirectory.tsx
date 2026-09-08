import React, { useState } from 'react';
import {
  PageSection,
  Title,
  Card,
  CardBody,
  Label,
  Button,
  Toolbar,
  ToolbarContent,
  ToolbarItem,
  SearchInput,
} from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Globe, Database, ExternalLink, Plus, CheckCircle, Clock, AlertTriangle } from 'lucide-react';

export interface FrappeSiteResource {
  name: string;
  namespace: string;
  benchName: string;
  domain: string;
  siteURL: string;
  phase: 'Ready' | 'Provisioning' | 'Failed';
  dbProvider: 'postgres' | 'mariadb' | 'external';
  dbEngine?: 'stackgres' | 'percona';
  dbMode: 'dedicated' | 'shared';
  tlsEnabled: boolean;
}

export const SitesDirectory: React.FC = () => {
  const [filter, setFilter] = useState('');

  const sites: FrappeSiteResource[] = [
    {
      name: 'prod-customer1',
      namespace: 'frappe-system',
      benchName: 'production-bench',
      domain: 'customer1.myplatform.com',
      siteURL: 'https://customer1.myplatform.com',
      phase: 'Ready',
      dbProvider: 'postgres',
      dbEngine: 'stackgres',
      dbMode: 'dedicated',
      tlsEnabled: true,
    },
    {
      name: 'prod-customer2',
      namespace: 'frappe-system',
      benchName: 'production-bench',
      domain: 'erp.customer2.corp',
      siteURL: 'https://erp.customer2.corp',
      phase: 'Ready',
      dbProvider: 'postgres',
      dbEngine: 'percona',
      dbMode: 'dedicated',
      tlsEnabled: true,
    },
    {
      name: 'staging-tenant',
      namespace: 'frappe-staging',
      benchName: 'staging-bench',
      domain: 'staging.testsite.local',
      siteURL: 'https://staging.testsite.local',
      phase: 'Provisioning',
      dbProvider: 'mariadb',
      dbMode: 'shared',
      tlsEnabled: true,
    },
  ];

  const filtered = sites.filter(
    (s) =>
      s.name.toLowerCase().includes(filter.toLowerCase()) ||
      s.domain.toLowerCase().includes(filter.toLowerCase()) ||
      s.namespace.toLowerCase().includes(filter.toLowerCase())
  );

  const renderStatus = (phase: string) => {
    switch (phase) {
      case 'Ready':
        return (
          <Label color="green" icon={<CheckCircle size={14} />}>
            Ready
          </Label>
        );
      case 'Provisioning':
        return (
          <Label color="blue" icon={<Clock size={14} />}>
            Provisioning
          </Label>
        );
      default:
        return (
          <Label color="red" icon={<AlertTriangle size={14} />}>
            Failed
          </Label>
        );
    }
  };

  const renderDbBadge = (site: FrappeSiteResource) => {
    if (site.dbProvider === 'postgres') {
      return (
        <Label color="teal" isCompact>
          PG ({site.dbEngine || 'stackgres'}) - {site.dbMode}
        </Label>
      );
    }
    if (site.dbProvider === 'mariadb') {
      return (
        <Label color="blue" isCompact>
          MariaDB - {site.dbMode}
        </Label>
      );
    }
    return (
      <Label color="orange" isCompact>
        External DB
      </Label>
    );
  };

  return (
    <PageSection>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 20 }}>
        <div>
          <Title headingLevel="h1" size="2xl" style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
            <Globe color="#0050A4" size={28} />
            Tenant Sites Directory
          </Title>
          <p style={{ color: '#6A6E73', marginTop: 4 }}>
            Multi-tenant FrappeSites provisioned with isolated databases, automatic Edge TLS Routes, and declarative life-cycles.
          </p>
        </div>
        <Button
          variant="primary"
          icon={<Plus size={16} />}
          style={{ backgroundColor: '#00BC86', borderColor: '#00BC86', color: '#0B1411', fontWeight: 600 }}
        >
          New Tenant Site
        </Button>
      </div>

      <Card>
        <CardBody>
          <Toolbar>
            <ToolbarContent>
              <ToolbarItem>
                <SearchInput
                  placeholder="Filter sites by name, domain, or namespace..."
                  value={filter}
                  onChange={(_e, val) => setFilter(val)}
                  onClear={() => setFilter('')}
                />
              </ToolbarItem>
            </ToolbarContent>
          </Toolbar>

          <Table aria-label="Tenant Sites Table" variant="compact">
            <Thead>
              <Tr>
                <Th>Site Name</Th>
                <Th>Domain / OpenShift Route</Th>
                <Th>Parent Bench</Th>
                <Th>Status</Th>
                <Th>Database Engine</Th>
                <Th>Namespace</Th>
                <Th>Actions</Th>
              </Tr>
            </Thead>
            <Tbody>
              {filtered.map((site) => (
                <Tr key={site.name}>
                  <Td dataLabel="Site Name" style={{ fontWeight: 600 }}>
                    <a href={`/k8s/ns/${site.namespace}/vyogo.tech~v1~FrappeSite/${site.name}`}>
                      {site.name}
                    </a>
                  </Td>
                  <Td dataLabel="Domain / OpenShift Route">
                    <a
                      href={site.siteURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#0050A4' }}
                    >
                      {site.domain}
                      <ExternalLink size={14} />
                    </a>
                  </Td>
                  <Td dataLabel="Parent Bench">
                    <code>{site.benchName}</code>
                  </Td>
                  <Td dataLabel="Status">{renderStatus(site.phase)}</Td>
                  <Td dataLabel="Database Engine">{renderDbBadge(site)}</Td>
                  <Td dataLabel="Namespace">
                    <Label color="grey">{site.namespace}</Label>
                  </Td>
                  <Td dataLabel="Actions">
                    <Button
                      variant="link"
                      isInline
                      onClick={() => window.open(site.siteURL, '_blank')}
                      style={{ color: '#00BC86' }}
                    >
                      Launch Desk
                    </Button>
                  </Td>
                </Tr>
              ))}
            </Tbody>
          </Table>
        </CardBody>
      </Card>
    </PageSection>
  );
};

export default SitesDirectory;
