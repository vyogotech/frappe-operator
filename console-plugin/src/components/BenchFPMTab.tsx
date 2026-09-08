import React from 'react';
import '../patternfly-theme.css';
import { Card, CardBody, CardTitle, Label, Title } from '@patternfly/react-core';
import { Table, Thead, Tr, Th, Tbody, Td } from '@patternfly/react-table';
import { Package, ShieldCheck } from 'lucide-react';

export interface BenchFPMTabProps {
  obj?: any;
}

export const BenchFPMTab: React.FC<BenchFPMTabProps> = ({ obj }) => {
  const fpmConfig = obj?.spec?.fpmConfig;
  const repos = fpmConfig?.repositories || [];
  const apps = obj?.spec?.apps || [];

  return (
    <div style={{ padding: '24px 0' }}>
      <Title headingLevel="h2" size="xl" style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
        <ShieldCheck color="#00BC86" size={22} />
        Air-Gapped Package Manager (FPM) Configuration
      </Title>

      <Card style={{ marginBottom: 20 }}>
        <CardTitle>Installed Applications on Bench</CardTitle>
        <CardBody>
          <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap' }}>
            {apps.length > 0 ? (
              apps.map((app: any) => {
                const name = typeof app === 'string' ? app : app.name;
                const source = typeof app === 'object' ? app.source : 'image/fpm';
                return (
                  <Label key={name} color="blue">
                    {name} ({source})
                  </Label>
                );
              })
            ) : (
              <p style={{ color: '#6A6E73' }}>No explicit apps list configured on bench spec.</p>
            )}
          </div>
        </CardBody>
      </Card>

      <Card>
        <CardTitle>Configured FPM Repositories</CardTitle>
        <CardBody>
          {repos.length > 0 ? (
            <Table aria-label="Bench FPM Repos" variant="compact">
              <Thead>
                <Tr>
                  <Th>Repository Name</Th>
                  <Th>Endpoint URL</Th>
                  <Th>Priority</Th>
                  <Th>Auth Secret</Th>
                </Tr>
              </Thead>
              <Tbody>
                {repos.map((repo: any) => (
                  <Tr key={repo.name}>
                    <Td dataLabel="Name" style={{ fontWeight: 600 }}>
                      {repo.name}
                    </Td>
                    <Td dataLabel="URL">
                      <code>{repo.url}</code>
                    </Td>
                    <Td dataLabel="Priority">{repo.priority || 50}</Td>
                    <Td dataLabel="Auth Secret">
                      {repo.authSecretRef?.name ? (
                        <Label color="blue" isCompact>
                          {repo.authSecretRef.name}
                        </Label>
                      ) : (
                        'None'
                      )}
                    </Td>
                  </Tr>
                ))}
              </Tbody>
            </Table>
          ) : (
            <p style={{ color: '#6A6E73' }}>
              No custom FPM repositories declared on this bench. Bench uses operator-level defaults or local images.
            </p>
          )}
        </CardBody>
      </Card>
    </div>
  );
};

export default BenchFPMTab;
