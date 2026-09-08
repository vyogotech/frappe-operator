import * as React from 'react';
import { k8sCreate, useActiveNamespace } from '@openshift-console/dynamic-plugin-sdk';
import {
  Button,
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
  TextInput,
  Title,
} from '@patternfly/react-core';
import { MinusCircleIcon, PlusCircleIcon } from '@patternfly/react-icons';
import { apiVersion, FrappeBenchGVK, FrappeBenchModel } from '../../frappe/models';
import { AppSource } from '../../frappe/types';
import { createYAMLPath, navigateTo, queryParam, resourcePath, toDNS1123 } from '../../frappe/utils';
import { CreateFormFooter, FormSection, ManifestPreview, NamespaceSelect } from './formControls';
import RedisConfigFields, {
  buildRedisConfig,
  defaultRedisConfig,
  redisConfigErrors,
  RedisConfigValue,
} from './RedisConfigFields';
import DatabaseConfigFields, {
  buildDatabaseConfig,
  databaseConfigErrors,
  defaultDatabaseConfig,
  DatabaseConfigValue,
} from './DatabaseConfigFields';
import PluginRoot from '../PluginRoot';

// Versions the operator's base images are published for. `frappeVersion` is a
// free-form string in the CRD, so "Other" keeps the escape hatch open.
const FRAPPE_VERSIONS = ['version-15', 'version-14', 'develop'];

type AppRow = AppSource & { key: number };
type RepoRow = { key: number; name: string; url: string; priority: number };

let nextKey = 0;
const newKey = () => (nextKey += 1);

const isDNS1123 = (value: string) => /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(value);

/**
 * Form-driven FrappeBench creation. Covers the fields an operator actually sets
 * when standing up a bench — version, apps and where they come from, storage,
 * image, database defaults, domain and FPM registries — and leaves the long
 * tail (autoscaling, pod config, namespace policy) to the YAML editor.
 */
export const BenchCreateForm: React.FC = () => {
  const [activeNamespace] = useActiveNamespace();

  const [namespace, setNamespace] = React.useState(
    () => queryParam('namespace') || (activeNamespace !== '#ALL_NS#' ? activeNamespace : ''),
  );
  const [name, setName] = React.useState('');
  const [frappeVersion, setFrappeVersion] = React.useState(FRAPPE_VERSIONS[0]);
  const [customVersion, setCustomVersion] = React.useState('');

  const [apps, setApps] = React.useState<AppRow[]>([
    { key: newKey(), name: 'erpnext', source: 'image' },
  ]);

  const [storageSize, setStorageSize] = React.useState('10Gi');
  const [storageClassName, setStorageClassName] = React.useState('');

  const [imageRepository, setImageRepository] = React.useState('');
  const [imageTag, setImageTag] = React.useState('');
  const [pullPolicy, setPullPolicy] = React.useState('IfNotPresent');

  const [dbConfig, setDbConfig] = React.useState<DatabaseConfigValue>(defaultDatabaseConfig);
  const [redisConfig, setRedisConfig] = React.useState<RedisConfigValue>(defaultRedisConfig);

  const [domainSuffix, setDomainSuffix] = React.useState('');
  const [autoDetect, setAutoDetect] = React.useState(true);

  const [gitEnabled, setGitEnabled] = React.useState(false);
  const [repos, setRepos] = React.useState<RepoRow[]>([]);

  const [inProgress, setInProgress] = React.useState(false);
  const [error, setError] = React.useState<string | undefined>();

  const effectiveVersion = frappeVersion === 'other' ? customVersion : frappeVersion;

  // ── App rows ───────────────────────────────────────────────────────────────

  const updateApp = (key: number, patch: Partial<AppRow>) =>
    setApps((prev) => prev.map((a) => (a.key === key ? { ...a, ...patch } : a)));

  const gitApps = apps.filter((a) => a.source === 'git');

  // A git-sourced app is cloned and built on the bench, which the operator only
  // allows when git installs are enabled for the bench.
  React.useEffect(() => {
    if (gitApps.length > 0 && !gitEnabled) {
      setGitEnabled(true);
    }
  }, [gitApps.length, gitEnabled]);

  // ── Validation ─────────────────────────────────────────────────────────────

  const nameError = name && !isDNS1123(name) ? 'Use lowercase letters, digits and dashes.' : '';
  const appErrors = apps.filter((a) => {
    if (!a.name) return true;
    if (a.source === 'fpm') return !a.org || !a.version;
    if (a.source === 'git') return !a.gitUrl;
    return false;
  });
  const dbErrors = databaseConfigErrors(dbConfig);
  const redisErrors = redisConfigErrors(redisConfig);
  const canSubmit =
    !!name &&
    !nameError &&
    !!namespace &&
    !!effectiveVersion &&
    appErrors.length === 0 &&
    dbErrors.length === 0 &&
    redisErrors.length === 0;

  // ── Manifest ───────────────────────────────────────────────────────────────

  const manifest = React.useMemo(() => {
    const specApps = apps
      .filter((a) => a.name)
      .map((a) => ({
        name: a.name,
        source: a.source,
        ...(a.source === 'fpm' ? { org: a.org, version: a.version } : {}),
        ...(a.source === 'git'
          ? { gitUrl: a.gitUrl, ...(a.gitBranch ? { gitBranch: a.gitBranch } : {}) }
          : {}),
      }));

    const validRepos = repos.filter((r) => r.name && r.url);

    return {
      apiVersion,
      kind: 'FrappeBench',
      metadata: { name: name || '<name>', namespace: namespace || '<namespace>' },
      spec: {
        frappeVersion: effectiveVersion,
        ...(specApps.length > 0 ? { apps: specApps } : {}),
        storageSize,
        ...(storageClassName ? { storageClassName } : {}),
        ...(imageRepository || imageTag
          ? {
              imageConfig: {
                ...(imageRepository ? { repository: imageRepository } : {}),
                ...(imageTag ? { tag: imageTag } : {}),
                pullPolicy,
              },
            }
          : {}),
        dbConfig: buildDatabaseConfig(dbConfig),
        redisConfig: buildRedisConfig(redisConfig),
        domainConfig: {
          autoDetect,
          ...(domainSuffix ? { suffix: domainSuffix } : {}),
        },
        gitConfig: { enabled: gitEnabled },
        ...(validRepos.length > 0
          ? {
              fpmConfig: {
                repositories: validRepos.map((r) => ({
                  name: r.name,
                  url: r.url,
                  priority: r.priority,
                })),
              },
            }
          : {}),
      },
    };
  }, [
    name,
    namespace,
    effectiveVersion,
    apps,
    storageSize,
    storageClassName,
    imageRepository,
    imageTag,
    pullPolicy,
    dbConfig,
    redisConfig,
    domainSuffix,
    autoDetect,
    gitEnabled,
    repos,
  ]);

  const onSubmit = React.useCallback(async () => {
    setInProgress(true);
    setError(undefined);
    try {
      await k8sCreate({ model: FrappeBenchModel, data: manifest });
      navigateTo(resourcePath(FrappeBenchGVK, name, namespace));
    } catch (e) {
      setError((e as { message?: string })?.message ?? 'The API server rejected the request.');
      setInProgress(false);
    }
  }, [manifest, name, namespace]);

  return (
    <>
      <div className="co-m-pane__body">
        <Title headingLevel="h1" size="2xl">
          Create Frappe Bench
        </Title>
        <FormHelperText style={{ marginTop: '8px' }}>
          <HelperText>
            <HelperTextItem>
              A bench is the shared Frappe runtime — image, apps, workers and storage — that tenant
              sites are provisioned onto.
            </HelperTextItem>
          </HelperText>
        </FormHelperText>
      </div>

      <div className="co-m-pane__body">
        <Form onSubmit={(e) => e.preventDefault()}>
          <FormSection title="Bench">
            <Grid hasGutter>
              <GridItem md={6}>
                <NamespaceSelect value={namespace} onChange={setNamespace} />
              </GridItem>
              <GridItem md={6}>
                <FormGroup label="Name" isRequired fieldId="bench-name">
                  <TextInput
                    id="bench-name"
                    value={name}
                    onChange={(_e, v) => setName(v)}
                    validated={nameError ? 'error' : 'default'}
                    placeholder="production-bench"
                  />
                  {nameError && (
                    <FormHelperText>
                      <HelperText>
                        <HelperTextItem variant="error">{nameError}</HelperTextItem>
                      </HelperText>
                    </FormHelperText>
                  )}
                </FormGroup>
              </GridItem>
              <GridItem md={6}>
                <FormGroup label="Frappe version" isRequired fieldId="bench-version">
                  <FormSelect
                    id="bench-version"
                    value={frappeVersion}
                    onChange={(_e, v) => setFrappeVersion(v)}
                    aria-label="Frappe version"
                  >
                    {FRAPPE_VERSIONS.map((v) => (
                      <FormSelectOption key={v} value={v} label={v} />
                    ))}
                    <FormSelectOption value="other" label="Other…" />
                  </FormSelect>
                </FormGroup>
              </GridItem>
              {frappeVersion === 'other' && (
                <GridItem md={6}>
                  <FormGroup label="Version string" isRequired fieldId="bench-version-custom">
                    <TextInput
                      id="bench-version-custom"
                      value={customVersion}
                      onChange={(_e, v) => setCustomVersion(v)}
                      placeholder="version-16"
                    />
                  </FormGroup>
                </GridItem>
              )}
            </Grid>
          </FormSection>

          <FormSection
            title="Apps"
            description="Apps baked into the bench. Sites choose which of these to install at provisioning time."
          >
            {apps.map((app) => (
              <Grid hasGutter key={app.key} style={{ marginBottom: '12px' }}>
                <GridItem md={3}>
                  <FormGroup label="App name" isRequired fieldId={`app-name-${app.key}`}>
                    <TextInput
                      id={`app-name-${app.key}`}
                      value={app.name}
                      onChange={(_e, v) => updateApp(app.key, { name: v })}
                      placeholder="erpnext"
                    />
                  </FormGroup>
                </GridItem>
                <GridItem md={2}>
                  <FormGroup label="Source" fieldId={`app-source-${app.key}`}>
                    <FormSelect
                      id={`app-source-${app.key}`}
                      value={app.source}
                      onChange={(_e, v) => updateApp(app.key, { source: v as AppSource['source'] })}
                      aria-label="App source"
                    >
                      <FormSelectOption value="image" label="In image" />
                      <FormSelectOption value="fpm" label="FPM" />
                      <FormSelectOption value="git" label="Git" />
                    </FormSelect>
                  </FormGroup>
                </GridItem>

                {app.source === 'fpm' && (
                  <>
                    <GridItem md={3}>
                      <FormGroup label="Organization" isRequired fieldId={`app-org-${app.key}`}>
                        <TextInput
                          id={`app-org-${app.key}`}
                          value={app.org ?? ''}
                          onChange={(_e, v) => updateApp(app.key, { org: v })}
                          placeholder="frappe"
                        />
                      </FormGroup>
                    </GridItem>
                    <GridItem md={3}>
                      <FormGroup label="Version" isRequired fieldId={`app-ver-${app.key}`}>
                        <TextInput
                          id={`app-ver-${app.key}`}
                          value={app.version ?? ''}
                          onChange={(_e, v) => updateApp(app.key, { version: v })}
                          placeholder="1.0.0"
                        />
                      </FormGroup>
                    </GridItem>
                  </>
                )}

                {app.source === 'git' && (
                  <>
                    <GridItem md={4}>
                      <FormGroup label="Repository URL" isRequired fieldId={`app-git-${app.key}`}>
                        <TextInput
                          id={`app-git-${app.key}`}
                          value={app.gitUrl ?? ''}
                          onChange={(_e, v) => updateApp(app.key, { gitUrl: v })}
                          placeholder="https://github.com/frappe/hrms"
                        />
                      </FormGroup>
                    </GridItem>
                    <GridItem md={2}>
                      <FormGroup label="Branch" fieldId={`app-branch-${app.key}`}>
                        <TextInput
                          id={`app-branch-${app.key}`}
                          value={app.gitBranch ?? ''}
                          onChange={(_e, v) => updateApp(app.key, { gitBranch: v })}
                          placeholder="default"
                        />
                      </FormGroup>
                    </GridItem>
                  </>
                )}

                {app.source === 'image' && (
                  <GridItem md={6}>
                    <FormGroup label=" " fieldId={`app-image-note-${app.key}`}>
                      <FormHelperText>
                        <HelperText>
                          <HelperTextItem>
                            Already present in the container image — nothing is fetched at runtime.
                          </HelperTextItem>
                        </HelperText>
                      </FormHelperText>
                    </FormGroup>
                  </GridItem>
                )}

                <GridItem md={1}>
                  <FormGroup label=" " fieldId={`app-remove-${app.key}`}>
                    <Button
                      variant="plain"
                      aria-label={`Remove ${app.name || 'app'}`}
                      onClick={() => setApps((prev) => prev.filter((a) => a.key !== app.key))}
                    >
                      <MinusCircleIcon />
                    </Button>
                  </FormGroup>
                </GridItem>
              </Grid>
            ))}
            <Button
              variant="link"
              isInline
              icon={<PlusCircleIcon />}
              onClick={() => setApps((prev) => [...prev, { key: newKey(), name: '', source: 'image' }])}
            >
              Add app
            </Button>
          </FormSection>

          <FormSection title="Storage and image">
            <Grid hasGutter>
              <GridItem md={3}>
                <FormGroup label="Bench volume size" fieldId="bench-storage">
                  <TextInput
                    id="bench-storage"
                    value={storageSize}
                    onChange={(_e, v) => setStorageSize(v)}
                  />
                </FormGroup>
              </GridItem>
              <GridItem md={3}>
                <FormGroup label="Storage class" fieldId="bench-storage-class">
                  <TextInput
                    id="bench-storage-class"
                    value={storageClassName}
                    onChange={(_e, v) => setStorageClassName(v)}
                    placeholder="cluster default"
                  />
                </FormGroup>
              </GridItem>
              <GridItem md={4}>
                <FormGroup label="Image repository" fieldId="bench-image-repo">
                  <TextInput
                    id="bench-image-repo"
                    value={imageRepository}
                    onChange={(_e, v) => setImageRepository(v)}
                    placeholder="operator default"
                  />
                </FormGroup>
              </GridItem>
              <GridItem md={2}>
                <FormGroup label="Image tag" fieldId="bench-image-tag">
                  <TextInput
                    id="bench-image-tag"
                    value={imageTag}
                    onChange={(_e, v) => setImageTag(v)}
                    placeholder="auto"
                  />
                </FormGroup>
              </GridItem>
              <GridItem md={3}>
                <FormGroup label="Pull policy" fieldId="bench-pull-policy">
                  <FormSelect
                    id="bench-pull-policy"
                    value={pullPolicy}
                    onChange={(_e, v) => setPullPolicy(v)}
                    aria-label="Pull policy"
                  >
                    <FormSelectOption value="IfNotPresent" label="IfNotPresent" />
                    <FormSelectOption value="Always" label="Always" />
                    <FormSelectOption value="Never" label="Never" />
                  </FormSelect>
                </FormGroup>
              </GridItem>
            </Grid>
          </FormSection>

          <FormSection
            title="Database defaults"
            description="Inherited by every site on this bench unless the site overrides them."
          >
            <DatabaseConfigFields
              value={dbConfig}
              onChange={setDbConfig}
              idPrefix="bench-db"
              namespace={namespace}
            />
          </FormSection>

          <FormSection
            title="Redis"
            description="Frappe's cache, background queues and socketio channels. Every site on this bench shares it — the CRD has no per-site override."
          >
            <RedisConfigFields
              value={redisConfig}
              onChange={setRedisConfig}
              namespace={namespace}
            />
          </FormSection>

          <FormSection title="Domains and app sources">
            <Grid hasGutter>
              <GridItem md={6}>
                <FormGroup label="Domain suffix" fieldId="bench-domain-suffix">
                  <TextInput
                    id="bench-domain-suffix"
                    value={domainSuffix}
                    onChange={(_e, v) => setDomainSuffix(v)}
                    placeholder=".apps.example.com"
                  />
                  <FormHelperText>
                    <HelperText>
                      <HelperTextItem>
                        Appended to site names that are not already fully qualified.
                      </HelperTextItem>
                    </HelperText>
                  </FormHelperText>
                </FormGroup>
              </GridItem>
              <GridItem md={6}>
                <FormGroup label="Cluster domain detection" fieldId="bench-autodetect">
                  <Checkbox
                    id="bench-autodetect"
                    label="Detect the ingress domain from the cluster"
                    isChecked={autoDetect}
                    onChange={(_e, checked) => setAutoDetect(checked)}
                  />
                </FormGroup>
              </GridItem>
              <GridItem md={12}>
                <FormGroup fieldId="bench-git-enabled">
                  <Checkbox
                    id="bench-git-enabled"
                    label="Allow installing apps from Git"
                    description="Leave off in air-gapped clusters so apps can only come from FPM registries or the image."
                    isChecked={gitEnabled}
                    isDisabled={gitApps.length > 0}
                    onChange={(_e, checked) => setGitEnabled(checked)}
                  />
                  {gitApps.length > 0 && (
                    <FormHelperText>
                      <HelperText>
                        <HelperTextItem>
                          Required because {gitApps.length} app(s) above use a Git source.
                        </HelperTextItem>
                      </HelperText>
                    </FormHelperText>
                  )}
                </FormGroup>
              </GridItem>
            </Grid>
          </FormSection>

          <FormSection
            title="FPM registries"
            description="Added to the operator-level defaults. Lower priority numbers are searched first."
          >
            {repos.map((repo) => (
              <Grid hasGutter key={repo.key} style={{ marginBottom: '12px' }}>
                <GridItem md={3}>
                  <FormGroup label="Name" fieldId={`repo-name-${repo.key}`}>
                    <TextInput
                      id={`repo-name-${repo.key}`}
                      value={repo.name}
                      onChange={(_e, v) =>
                        setRepos((prev) =>
                          prev.map((r) => (r.key === repo.key ? { ...r, name: v } : r)),
                        )
                      }
                      placeholder="company-private"
                    />
                  </FormGroup>
                </GridItem>
                <GridItem md={6}>
                  <FormGroup label="URL" fieldId={`repo-url-${repo.key}`}>
                    <TextInput
                      id={`repo-url-${repo.key}`}
                      value={repo.url}
                      onChange={(_e, v) =>
                        setRepos((prev) =>
                          prev.map((r) => (r.key === repo.key ? { ...r, url: v } : r)),
                        )
                      }
                      placeholder="https://fpm.example.com"
                    />
                  </FormGroup>
                </GridItem>
                <GridItem md={2}>
                  <FormGroup label="Priority" fieldId={`repo-priority-${repo.key}`}>
                    <TextInput
                      id={`repo-priority-${repo.key}`}
                      type="number"
                      value={repo.priority}
                      onChange={(_e, v) =>
                        setRepos((prev) =>
                          prev.map((r) =>
                            r.key === repo.key ? { ...r, priority: Number(v) || 50 } : r,
                          ),
                        )
                      }
                    />
                  </FormGroup>
                </GridItem>
                <GridItem md={1}>
                  <FormGroup label=" " fieldId={`repo-remove-${repo.key}`}>
                    <Button
                      variant="plain"
                      aria-label={`Remove ${repo.name || 'registry'}`}
                      onClick={() => setRepos((prev) => prev.filter((r) => r.key !== repo.key))}
                    >
                      <MinusCircleIcon />
                    </Button>
                  </FormGroup>
                </GridItem>
              </Grid>
            ))}
            <Button
              variant="link"
              isInline
              icon={<PlusCircleIcon />}
              onClick={() =>
                setRepos((prev) => [...prev, { key: newKey(), name: '', url: '', priority: 50 }])
              }
            >
              Add registry
            </Button>
          </FormSection>

          <ManifestPreview manifest={manifest} />

          <CreateFormFooter
            submitLabel="Create bench"
            isSubmitDisabled={!canSubmit}
            inProgress={inProgress}
            error={error}
            onSubmit={onSubmit}
            onCancel={() => navigateTo('/frappe/benches')}
            yamlHref={createYAMLPath(FrappeBenchGVK, namespace || undefined)}
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
const BenchCreateFormPage: React.FC<React.ComponentProps<typeof BenchCreateForm>> = (props) => (
  <PluginRoot>
    <BenchCreateForm {...props} />
  </PluginRoot>
);

export default BenchCreateFormPage;
