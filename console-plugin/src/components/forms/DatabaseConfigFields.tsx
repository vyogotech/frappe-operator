import * as React from 'react';
import {
  Alert,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  HelperText,
  HelperTextItem,
  TextInput,
} from '@patternfly/react-core';
import { DatabaseConfig, DatabaseMode, DatabaseProvider } from '../../frappe/types';

/** Flat form state for a `dbConfig` block. */
export interface DatabaseConfigValue {
  provider: DatabaseProvider;
  mode: DatabaseMode;
  postgresEngine: 'stackgres' | 'percona';
  /** Existing MariaDB CR / PostgreSQL cluster to attach to in shared mode. */
  instanceRef: string;
  /** Out-of-cluster connection details, used by the external provider. */
  host: string;
  port: string;
  connectionSecret: string;
  /** Volume size for an operator-provisioned dedicated instance. */
  storageSize: string;
}

export const defaultDatabaseConfig = (): DatabaseConfigValue => ({
  provider: 'mariadb',
  mode: 'shared',
  postgresEngine: 'stackgres',
  instanceRef: '',
  host: '',
  port: '',
  connectionSecret: '',
  storageSize: '',
});

/** Default port shown as a placeholder for the selected provider. */
const defaultPort = (provider: DatabaseProvider) => (provider === 'postgres' ? '5432' : '3306');

/**
 * Fields the operator requires but that the form leaves empty. An external
 * database has no operator-managed instance behind it, so host and credentials
 * are the two things it cannot infer.
 */
export const databaseConfigErrors = (value: DatabaseConfigValue): string[] => {
  if (value.provider !== 'external') {
    return [];
  }
  const errors: string[] = [];
  if (!value.host) {
    errors.push('An external database needs a host.');
  }
  if (!value.connectionSecret) {
    errors.push('An external database needs a connection secret.');
  }
  return errors;
};

/** Projects the form state onto the CRD's `dbConfig`, omitting empty fields. */
export const buildDatabaseConfig = (value: DatabaseConfigValue): DatabaseConfig => {
  if (value.provider === 'sqlite') {
    return { provider: 'sqlite' };
  }

  if (value.provider === 'external') {
    return {
      provider: 'external',
      host: value.host,
      ...(value.port ? { port: value.port } : {}),
      ...(value.connectionSecret ? { connectionSecretRef: { name: value.connectionSecret } } : {}),
    };
  }

  const refKey = value.provider === 'postgres' ? 'postgresRef' : 'mariadbRef';

  return {
    provider: value.provider,
    mode: value.mode,
    ...(value.provider === 'postgres' && value.mode === 'dedicated'
      ? { postgresEngine: value.postgresEngine }
      : {}),
    ...(value.mode === 'shared' && value.instanceRef
      ? { [refKey]: { name: value.instanceRef } }
      : {}),
    ...(value.mode === 'dedicated' && value.storageSize ? { storageSize: value.storageSize } : {}),
  };
};

export interface DatabaseConfigFieldsProps {
  value: DatabaseConfigValue;
  onChange: (value: DatabaseConfigValue) => void;
  /** Namespaces the ids so two of these can coexist on one page. */
  idPrefix: string;
  namespace?: string;
}

/**
 * Database configuration shared by the bench and site forms. The CRD models
 * four providers with quite different needs — a shared or dedicated
 * operator-managed MariaDB or PostgreSQL, a file-backed SQLite, or an external
 * server the operator only connects to — so the visible fields follow the
 * provider rather than showing every field at once.
 */
export const DatabaseConfigFields: React.FC<DatabaseConfigFieldsProps> = ({
  value,
  onChange,
  idPrefix,
  namespace,
}) => {
  const set = (patch: Partial<DatabaseConfigValue>) => onChange({ ...value, ...patch });
  const id = (suffix: string) => `${idPrefix}-${suffix}`;

  return (
    <Grid hasGutter>
      <GridItem md={4}>
        <FormGroup label="Provider" fieldId={id('provider')}>
          <FormSelect
            id={id('provider')}
            value={value.provider}
            onChange={(_e, v) => set({ provider: v as DatabaseProvider })}
            aria-label="Database provider"
          >
            <FormSelectOption value="mariadb" label="MariaDB" />
            <FormSelectOption value="postgres" label="PostgreSQL" />
            <FormSelectOption value="external" label="External server" />
            <FormSelectOption value="sqlite" label="SQLite" />
          </FormSelect>
        </FormGroup>
      </GridItem>

      {(value.provider === 'mariadb' || value.provider === 'postgres') && (
        <>
          <GridItem md={4}>
            <FormGroup label="Mode" fieldId={id('mode')}>
              <FormSelect
                id={id('mode')}
                value={value.mode}
                onChange={(_e, v) => set({ mode: v as DatabaseMode })}
                aria-label="Database mode"
              >
                <FormSelectOption value="shared" label="Shared instance" />
                <FormSelectOption value="dedicated" label="Dedicated instance" />
              </FormSelect>
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    {value.mode === 'shared'
                      ? 'One database server holds a separate database per site.'
                      : 'The operator provisions a database server per site.'}
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          </GridItem>

          {value.mode === 'shared' && (
            <GridItem md={4}>
              <FormGroup
                label={value.provider === 'postgres' ? 'PostgreSQL cluster' : 'MariaDB instance'}
                fieldId={id('instance-ref')}
              >
                <TextInput
                  id={id('instance-ref')}
                  value={value.instanceRef}
                  onChange={(_e, v) => set({ instanceRef: v })}
                  placeholder="operator default"
                />
                <FormHelperText>
                  <HelperText>
                    <HelperTextItem>
                      Name of an existing instance in {namespace || 'this namespace'}. Leave empty to
                      let the operator use or create its default.
                    </HelperTextItem>
                  </HelperText>
                </FormHelperText>
              </FormGroup>
            </GridItem>
          )}

          {value.mode === 'dedicated' && (
            <>
              <GridItem md={4}>
                <FormGroup label="Volume size" fieldId={id('storage')}>
                  <TextInput
                    id={id('storage')}
                    value={value.storageSize}
                    onChange={(_e, v) => set({ storageSize: v })}
                    placeholder="operator default"
                  />
                </FormGroup>
              </GridItem>
              {value.provider === 'postgres' && (
                <GridItem md={4}>
                  <FormGroup label="PostgreSQL operator" fieldId={id('pg-engine')}>
                    <FormSelect
                      id={id('pg-engine')}
                      value={value.postgresEngine}
                      onChange={(_e, v) => set({ postgresEngine: v as 'stackgres' | 'percona' })}
                      aria-label="PostgreSQL engine"
                    >
                      <FormSelectOption value="stackgres" label="StackGres" />
                      <FormSelectOption value="percona" label="Percona" />
                    </FormSelect>
                    <FormHelperText>
                      <HelperText>
                        <HelperTextItem>
                          Which operator provisions the dedicated cluster. Shared mode only needs a
                          reachable host, so this does not apply there.
                        </HelperTextItem>
                      </HelperText>
                    </FormHelperText>
                  </FormGroup>
                </GridItem>
              )}
            </>
          )}
        </>
      )}

      {value.provider === 'external' && (
        <>
          <GridItem md={4}>
            <FormGroup label="Host" isRequired fieldId={id('host')}>
              <TextInput
                id={id('host')}
                value={value.host}
                onChange={(_e, v) => set({ host: v })}
                validated={value.host ? 'default' : 'error'}
                placeholder="db.example.com"
              />
            </FormGroup>
          </GridItem>
          <GridItem md={4}>
            <FormGroup label="Port" fieldId={id('port')}>
              <TextInput
                id={id('port')}
                value={value.port}
                onChange={(_e, v) => set({ port: v })}
                placeholder={defaultPort(value.provider)}
              />
            </FormGroup>
          </GridItem>
          <GridItem md={6}>
            <FormGroup label="Connection secret" isRequired fieldId={id('secret')}>
              <TextInput
                id={id('secret')}
                value={value.connectionSecret}
                onChange={(_e, v) => set({ connectionSecret: v })}
                validated={value.connectionSecret ? 'default' : 'error'}
                placeholder="external-db-credentials"
              />
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    Secret in {namespace || 'the site namespace'} with the keys{' '}
                    <code>username</code> and <code>password</code>, plus an optional{' '}
                    <code>database</code> key. Without <code>database</code> the operator uses the
                    site name.
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          </GridItem>
          <GridItem md={12}>
            <Alert variant="info" isInline title="The operator does not manage this server">
              Provisioning, backups of the server itself, upgrades and network reachability stay
              yours. The operator only creates the site database and connects to it with the
              credentials above.
            </Alert>
          </GridItem>
        </>
      )}

      {value.provider === 'sqlite' && (
        <GridItem md={8}>
          <Alert variant="warning" isInline title="SQLite is single-node">
            The database lives on the bench volume, so it cannot be shared across replicas. Use it
            for development and evaluation, not production tenants.
          </Alert>
        </GridItem>
      )}
    </Grid>
  );
};

export default DatabaseConfigFields;
