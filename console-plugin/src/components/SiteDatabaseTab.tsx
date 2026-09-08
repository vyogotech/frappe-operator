import React from 'react';
import '../patternfly-theme.css';
import { Card, CardBody, CardTitle, Label, Title, DescriptionList, DescriptionListGroup, DescriptionListTerm, DescriptionListDescription } from '@patternfly/react-core';
import { Database, ShieldCheck, Globe, ExternalLink } from 'lucide-react';

export interface SiteDatabaseTabProps {
  obj?: any;
}

export const SiteDatabaseTab: React.FC<SiteDatabaseTabProps> = ({ obj }) => {
  const dbConfig = obj?.spec?.dbConfig || {};
  const routeConfig = obj?.spec?.routeConfig || {};
  const status = obj?.status || {};

  return (
    <div style={{ padding: '24px 0' }}>
      <Title headingLevel="h2" size="xl" style={{ marginBottom: 16, display: 'flex', alignItems: 'center', gap: 8 }}>
        <Database color="#0050A4" size={22} />
        Database & Edge Route Infrastructure
      </Title>

      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))', gap: 20 }}>
        <Card>
          <CardTitle style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Database size={18} color="#00BC86" />
            Polymorphic Database Status
          </CardTitle>
          <CardBody>
            <DescriptionList isCompact>
              <DescriptionListGroup>
                <DescriptionListTerm>Provider</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label color="teal">{dbConfig.provider || 'mariadb'}</Label>
                </DescriptionListDescription>
              </DescriptionListGroup>

              {dbConfig.provider === 'postgres' && (
                <DescriptionListGroup>
                  <DescriptionListTerm>PostgreSQL Engine</DescriptionListTerm>
                  <DescriptionListDescription>
                    <Label color="purple">{dbConfig.postgresEngine || 'stackgres'}</Label>
                  </DescriptionListDescription>
                </DescriptionListGroup>
              )}

              <DescriptionListGroup>
                <DescriptionListTerm>Deployment Mode</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label color="blue">{dbConfig.mode || 'shared'}</Label>
                </DescriptionListDescription>
              </DescriptionListGroup>

              <DescriptionListGroup>
                <DescriptionListTerm>Database Name</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{status.databaseName || 'Auto-generated on init'}</code>
                </DescriptionListDescription>
              </DescriptionListGroup>

              <DescriptionListGroup>
                <DescriptionListTerm>Credentials Secret</DescriptionListTerm>
                <DescriptionListDescription>
                  {status.databaseCredentialsSecret ? (
                    <code>{status.databaseCredentialsSecret}</code>
                  ) : (
                    <span style={{ color: '#6A6E73' }}>Provisioning in progress...</span>
                  )}
                </DescriptionListDescription>
              </DescriptionListGroup>

              <DescriptionListGroup>
                <DescriptionListTerm>Deletion Policy</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label color="grey">{obj?.spec?.deletionPolicy || 'Retain (Safe)'}</Label>
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </CardBody>
        </Card>

        <Card>
          <CardTitle style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Globe size={18} color="#0050A4" />
            OpenShift Route & Edge TLS
          </CardTitle>
          <CardBody>
            <DescriptionList isCompact>
              <DescriptionListGroup>
                <DescriptionListTerm>OpenShift Route Redirection</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label color={routeConfig.enabled !== false ? 'green' : 'orange'}>
                    {routeConfig.enabled !== false ? 'Enforced HTTPS' : 'Disabled'}
                  </Label>
                </DescriptionListDescription>
              </DescriptionListGroup>

              <DescriptionListGroup>
                <DescriptionListTerm>TLS Termination</DescriptionListTerm>
                <DescriptionListDescription>
                  <code>{routeConfig?.tls?.termination || 'edge'}</code>
                </DescriptionListDescription>
              </DescriptionListGroup>

              <DescriptionListGroup>
                <DescriptionListTerm>Insecure Edge Policy</DescriptionListTerm>
                <DescriptionListDescription>
                  <Label color="blue">
                    {routeConfig?.tls?.insecureEdgeTerminationPolicy || 'Redirect'}
                  </Label>
                </DescriptionListDescription>
              </DescriptionListGroup>

              <DescriptionListGroup>
                <DescriptionListTerm>Live Site URL</DescriptionListTerm>
                <DescriptionListDescription>
                  {status.siteURL ? (
                    <a
                      href={status.siteURL}
                      target="_blank"
                      rel="noopener noreferrer"
                      style={{ display: 'flex', alignItems: 'center', gap: 6, color: '#0050A4' }}
                    >
                      {status.siteURL}
                      <ExternalLink size={14} />
                    </a>
                  ) : (
                    <span style={{ color: '#6A6E73' }}>Not yet ready</span>
                  )}
                </DescriptionListDescription>
              </DescriptionListGroup>
            </DescriptionList>
          </CardBody>
        </Card>
      </div>
    </div>
  );
};

export default SiteDatabaseTab;
