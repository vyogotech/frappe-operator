import * as React from 'react';
import {
  k8sCreate,
  useActiveNamespace,
  useK8sWatchResource,
} from '@openshift-console/dynamic-plugin-sdk';
import {
  Alert,
  Checkbox,
  Form,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  Grid,
  GridItem,
  HelperText,
  HelperTextItem,
  Label,
  Radio,
  TextInput,
  Title,
} from '@patternfly/react-core';
import { apiVersion, FrappeBenchGVK, FrappeSiteGVK, FrappeSiteModel } from '../../frappe/models';
import { FrappeBench } from '../../frappe/types';
import {
  benchAppNames,
  createYAMLPath,
  navigateTo,
  phaseColor,
  queryParam,
  resourcePath,
  toDNS1123,
} from '../../frappe/utils';
import { CreateFormFooter, FormSection, ManifestPreview, NamespaceSelect } from './formControls';
import DatabaseConfigFields, {
  buildDatabaseConfig,
  databaseConfigErrors,
  defaultDatabaseConfig,
  DatabaseConfigValue,
} from './DatabaseConfigFields';
import PluginRoot from '../PluginRoot';

// Frappe routes on the HTTP Host header, so the site name has to be a valid
// hostname — the same pattern the CRD enforces.
const SITE_NAME_PATTERN = /^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$/;

/**
 * Form-driven FrappeSite creation. The bench picker drives the rest of the
 * form: the apps offered are the ones its bench actually ships, and the
 * database section defaults to inheriting the bench's configuration.
 */
export const SiteCreateForm: React.FC = () => {
  const [activeNamespace] = useActiveNamespace();

  const [namespace, setNamespace] = React.useState(
    () => queryParam('namespace') || (activeNamespace !== '#ALL_NS#' ? activeNamespace : ''),
  );
  const [benchName, setBenchName] = React.useState(() => queryParam('bench') ?? '');

  const [siteName, setSiteName] = React.useState('');
  const [crName, setCrName] = React.useState('');
  const [crNameTouched, setCrNameTouched] = React.useState(false);

  const [selectedApps, setSelectedApps] = React.useState<string[]>([]);

  const [dbSource, setDbSource] = React.useState<'inherit' | 'override'>('inherit');
  const [dbConfig, setDbConfig] = React.useState<DatabaseConfigValue>(defaultDatabaseConfig);

  const [routeEnabled, setRouteEnabled] = React.useState(true);
  const [routeHost, setRouteHost] = React.useState('');
  const [tlsTermination, setTlsTermination] = React.useState('edge');

  const [adminSecret, setAdminSecret] = React.useState('');
  const [deletionPolicy, setDeletionPolicy] = React.useState('Retain');
  const [skipInit, setSkipInit] = React.useState(false);

  const [inProgress, setInProgress] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>();

  // ── Bench ──────────────────────────────────────────────────────────────────

  const [benches, benchesLoaded] = useK8sWatchResource<FrappeBench[]>(
    namespace ? { groupVersionKind: FrappeBenchGVK, isList: true, namespace } : null,
  );

  const bench = React.useMemo(
    () => (benches ?? []).find((b) => b.metadata?.name === benchName),
    [benches, benchName],
  );

  const availableApps = React.useMemo(() => benchAppNames(bench), [bench]);

  // Drop selections that the newly chosen bench does not provide.
  React.useEffect(() => {
    setSelectedApps((prev) => prev.filter((a) => availableApps.includes(a)));
  }, [availableApps]);

  // The CR name tracks the site name until the user takes it over.
  React.useEffect(() => {
    if (!crNameTouched) {
      setCrName(toDNS1123(siteName));
    }
  }, [siteName, crNameTouched]);

  // ── Validation ─────────────────────────────────────────────────────────────

  const siteNameError =
    siteName && !SITE_NAME_PATTERN.test(siteName)
      ? 'Must be a hostname: lowercase letters, digits, dashes and dots.'
      : '';
  const dbErrors = dbSource === 'override' ? databaseConfigErrors(dbConfig) : [];
  const canSubmit =
    !!namespace && !!benchName && !!siteName && !siteNameError && !!crName && dbErrors.length === 0;

  // ── Manifest ───────────────────────────────────────────────────────────────

  const manifest = React.useMemo(
    () => ({
      apiVersion,
      kind: 'FrappeSite',
      metadata: { name: crName || '<name>', namespace: namespace || '<namespace>' },
      spec: {
        benchRef: { name: benchName || '<bench>', namespace: namespace || '<namespace>' },
        siteName: siteName || '<site.example.com>',
        ...(selectedApps.length > 0 ? { apps: selectedApps } : {}),
        ...(dbSource === 'override' ? { dbConfig: buildDatabaseConfig(dbConfig) } : {}),
        routeConfig: {
          enabled: routeEnabled,
          ...(routeEnabled && routeHost ? { host: routeHost } : {}),
          ...(routeEnabled ? { tlsTermination } : {}),
        },
        ...(adminSecret ? { adminPasswordSecretRef: { name: adminSecret, namespace } } : {}),
        deletionPolicy,
        ...(skipInit ? { skipInit: true } : {}),
      },
    }),
    [
      crName,
      namespace,
      benchName,
      siteName,
      selectedApps,
      dbSource,
      dbConfig,
      routeEnabled,
      routeHost,
      tlsTermination,
      adminSecret,
      deletionPolicy,
      skipInit,
    ],
  );

  const onSubmit = React.useCallback(async () => {
    setInProgress(true);
    setError(undefined);
    try {
      await k8sCreate({ model: FrappeSiteModel, data: manifest });
      navigateTo(resourcePath(FrappeSiteGVK, crName, namespace));
    } catch (e) {
      setError((e as { message?: string })?.message ?? 'The API server rejected the request.');
      setInProgress(false);
    }
  }, [manifest, crName, namespace]);

  const benchDb = bench?.spec?.dbConfig;
  const benchRedis = bench?.spec?.redisConfig;

  return (
    <>
      <div className="co-m-pane__body">
        <Title headingLevel="h1" size="2xl">
          Create Tenant Site
        </Title>
        <FormHelperText style={{ marginTop: '8px' }}>
          <HelperText>
            <HelperTextItem>
              A site is one Frappe tenant — its own database, domain and set of installed apps —
              running on a bench.
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      </div>

      <div className="co-m-pane__body">
        <Form onSubmit={(e) => e.preventDefault()}>
          <FormSection title="Placement">
            <Grid hasGutter>
              <GridItem md={6}>
                <NamespaceSelect value={namespace} onChange={setNamespace} id="site-namespace" />
              </GridItem>
              <GridItem md={6}>
                <FormGroup label="Bench" isRequired fieldId="site-bench">
                  <FormSelect
                    id="site-bench"
                    value={benchName}
                    onChange={(_e, v) => setBenchName(v)}
                    aria-label="Bench"
                    isDisabled={!namespace || !benchesLoaded}
                    validated={benchName ? 'default' : 'error'}
                  >
                    <FormSelectOption value="" label="Select a bench" isDisabled />
                    {(benches ?? []).map((b) => (
                      <FormSelectOption
                        key={b.metadata?.name}
                        value={b.metadata?.name}
                        label={`${b.metadata?.name} (${b.spec?.frappeVersion ?? 'unknown version'})`}
                      />
                    ))}
                  </FormSelect>
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem>
                        {!namespace
                          ? 'Choose a namespace first.'
                          : benchesLoaded && (benches ?? []).length === 0
                          ? 'No benches in this namespace yet — create one first.'
                          : 'The bench supplies the Frappe runtime, apps and workers for this site.'}
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                </FormGroup>
              </GridItem>

              {bench && bench.status?.phase !== 'Ready' && (
                <GridItem md={12}>
                  <Alert
                    variant="info"
                    isInline
                    title={
                      <>
                        Bench is{' '}
                        <Label isCompact color={phaseColor(bench.status?.phase)}>
                          {bench.status?.phase ?? 'not reporting a phase'}
                        </Label>
                      </>
                    }
                  >
                    The site will be created and stay Pending until the bench is Ready.
                  </Alert>
                </GridItem>
              )}
            </Grid>
          </FormSection>

          <FormSection title="Identity">
            <Grid hasGutter>
              <GridItem md={6}>
                <FormGroup label="Site name" isRequired fieldId="site-name">
                  <TextInput
                    id="site-name"
                    value={siteName}
                    onChange={(_e, v) => setSiteName(v)}
                    validated={siteNameError ? 'error' : 'default'}
                    placeholder="erp.customer.com"
                  />
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem variant={siteNameError ? 'error' : 'default'}>
                        {siteNameError ||
                          'Frappe routes on the HTTP Host header, so this must be the domain that will receive traffic.'}
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                </FormGroup>
              </GridItem>
              <GridItem md={6}>
                <FormGroup label="Resource name" isRequired fieldId="site-cr-name">
                  <TextInput
                    id="site-cr-name"
                    value={crName}
                    onChange={(_e, v) => {
                      setCrNameTouched(true);
                      setCrName(v);
                    }}
                  />
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem>
                        Name of the FrappeSite object in Kubernetes. Derived from the site name.
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                </FormGroup>
              </GridItem>
            </Grid>
          </FormSection>

          <FormSection
            title="Apps"
            description="Chosen from what the bench provides. Apps can only be selected at creation time — afterwards, install them from the site's Actions menu."
          >
            {!bench ? (
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>Select a bench to see the apps it provides.</HelperTextItem>
                </HelperText>
              </FormHelperText>
            ) : availableApps.length === 0 ? (
              <Alert variant="info" isInline title="This bench reports no apps yet">
                It may still be initializing. You can create the site now and install apps onto it
                later.
              </Alert>
            ) : (
              <Grid hasGutter>
                {availableApps.map((app) => (
                  <GridItem md={4} key={app}>
                    <Checkbox
                      id={`site-app-${app}`}
                      label={app}
                      isChecked={selectedApps.includes(app)}
                      onChange={(_e, checked) =>
                        setSelectedApps((prev) =>
                          checked ? [...prev, app] : prev.filter((a) => a !== app),
                        )
                      }
                    />
                  </GridItem>
                ))}
              </Grid>
            )}
          </FormSection>

          <FormSection title="Database">
            <FormGroup fieldId="site-db-source" role="radiogroup" isStack>
              <Radio
                id="site-db-inherit"
                name="site-db-source"
                label="Inherit from the bench"
                description={
                  benchDb
                    ? `The bench uses ${benchDb.provider ?? 'mariadb'}${
                        benchDb.provider === 'external'
                          ? ` at ${benchDb.host ?? 'a configured host'}`
                          : ` in ${benchDb.mode ?? 'shared'} mode`
                      }.`
                    : 'The site takes whatever database defaults the bench declares.'
                }
                isChecked={dbSource === 'inherit'}
                onChange={(_e, checked) => checked && setDbSource('inherit')}
              />
              <Radio
                id="site-db-override"
                name="site-db-source"
                label="Configure for this site"
                description="Pin this tenant to its own provider — including a database server outside the cluster."
                isChecked={dbSource === 'override'}
                onChange={(_e, checked) => checked && setDbSource('override')}
              />
            </FormGroup>

            {dbSource === 'override' && (
              <div style={{ marginTop: '16px' }}>
                <DatabaseConfigFields
                  value={dbConfig}
                  onChange={setDbConfig}
                  idPrefix="site-db"
                  namespace={namespace}
                />
              </div>
            )}
          </FormSection>

          <FormSection
            title="Redis"
            description="Inherited from the bench. Frappe's cache, queues and socketio are shared by every site on a bench, so this is configured there rather than per site."
          >
            {!bench ? (
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>Select a bench to see the Redis it provides.</HelperTextItem>
                </HelperText>
              </FormHelperText>
            ) : benchRedis?.external ? (
              <Alert variant="info" isInline title="External Redis">
                This site will use{' '}
                <code>
                  {benchRedis.host ?? 'the configured host'}:{benchRedis.port ?? 6379}
                </code>
                {benchRedis.connectionSecretRef?.name ? (
                  <>
                    {' '}with credentials from <code>{benchRedis.connectionSecretRef.name}</code>
                  </>
                ) : (
                  ' with no authentication'
                )}
                .
              </Alert>
            ) : (
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    The bench runs its own{' '}
                    <Label isCompact color="teal">
                      {benchRedis?.type ?? 'redis'}
                    </Label>{' '}
                    instance. To point this tenant at an external Redis, change it on the bench.
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            )}
          </FormSection>

          <FormSection title="Networking">
            <Grid hasGutter>
              <GridItem md={12}>
                <FormGroup fieldId="site-route-enabled">
                  <Checkbox
                    id="site-route-enabled"
                    label="Expose the site through an OpenShift Route"
                    description="Turn this off if traffic reaches the site through your own ingress or service mesh."
                    isChecked={routeEnabled}
                    onChange={(_e, checked) => setRouteEnabled(checked)}
                  />
                </FormGroup>
              </GridItem>
              {routeEnabled && (
                <>
                  <GridItem md={6}>
                    <FormGroup label="Route host" fieldId="site-route-host">
                      <TextInput
                        id="site-route-host"
                        value={routeHost}
                        onChange={(_e, v) => setRouteHost(v)}
                        placeholder={siteName || 'defaults to the site name'}
                      />
                      <FormHelperText>
                        <HelperText>
                          <HelperTextItem>
                            Overrides the generated hostname. Leave empty to use the site name.
                          </HelperTextItem>
                        </HelperText>
                      </FormHelperText>
                    </FormGroup>
                  </GridItem>
                  <GridItem md={6}>
                    <FormGroup label="TLS termination" fieldId="site-tls">
                      <FormSelect
                        id="site-tls"
                        value={tlsTermination}
                        onChange={(_e, v) => setTlsTermination(v)}
                        aria-label="TLS termination"
                      >
                        <FormSelectOption value="edge" label="Edge" />
                        <FormSelectOption value="reencrypt" label="Re-encrypt" />
                        <FormSelectOption value="passthrough" label="Passthrough" />
                      </FormSelect>
                    </FormGroup>
                  </GridItem>
                </>
              )}
            </Grid>
          </FormSection>

          <FormSection title="Administration">
            <Grid hasGutter>
              <GridItem md={6}>
                <FormGroup label="Admin password secret" fieldId="site-admin-secret">
                  <TextInput
                    id="site-admin-secret"
                    value={adminSecret}
                    onChange={(_e, v) => setAdminSecret(v)}
                    placeholder="operator generates one"
                  />
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem>
                        Secret in {namespace || 'this namespace'} holding the Administrator password.
                        Leave empty and the operator generates one.
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                </FormGroup>
              </GridItem>
              <GridItem md={6}>
                <FormGroup label="On delete" fieldId="site-deletion-policy">
                  <FormSelect
                    id="site-deletion-policy"
                    value={deletionPolicy}
                    onChange={(_e, v) => setDeletionPolicy(v)}
                    aria-label="Deletion policy"
                  >
                    <FormSelectOption value="Retain" label="Retain the database" />
                    <FormSelectOption value="Delete" label="Delete the database and user" />
                  </FormSelect>
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem variant={deletionPolicy === 'Delete' ? 'warning' : 'default'}>
                        {deletionPolicy === 'Delete'
                          ? 'Deleting the FrappeSite drops the tenant database. A GitOps prune would too.'
                          : 'Tenant data survives an accidental delete or a GitOps prune.'}
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                </FormGroup>
              </GridItem>
              <GridItem md={12}>
                <FormGroup fieldId="site-skip-init">
                  <Checkbox
                    id="site-skip-init"
                    label="The database already holds a Frappe schema"
                    description="Skips bench new-site; the init job only runs migrations and configuration updates. Use when adopting an existing database."
                    isChecked={skipInit}
                    onChange={(_e, checked) => setSkipInit(checked)}
                  />
                </FormGroup>
              </GridItem>
            </Grid>
          </FormSection>

          <ManifestPreview manifest={manifest} />

          <CreateFormFooter
            submitLabel="Create site"
            isSubmitDisabled={!canSubmit}
            inProgress={inProgress}
            error={error}
            onSubmit={onSubmit}
            onCancel={() => navigateTo('/frappe/sites')}
            yamlHref={createYAMLPath(FrappeSiteGVK, namespace || undefined)}
          />
        </Form>
      </div>
    </>
  );
};

/**
 * The console renders this through the extension's `$codeRef`, so the themed
 * wrapper has to be the default export — it is what puts the plugin's bundled
 * PatternFly styles and theme scope around the tree.
 */
const SiteCreateFormPage: React.FC<React.ComponentProps<typeof SiteCreateForm>> = (props) => (
  <PluginRoot>
    <SiteCreateForm {...props} />
  </PluginRoot>
);

export default SiteCreateFormPage;
