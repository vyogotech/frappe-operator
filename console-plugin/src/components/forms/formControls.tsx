import * as React from 'react';
import {
  K8sResourceCommon,
  ResourceYAMLEditor,
  useK8sWatchResource,
} from '@openshift-console/dynamic-plugin-sdk';
import {
  ActionGroup,
  Alert,
  Button,
  Card,
  CardBody,
  CardTitle,
  ExpandableSection,
  FormGroup,
  FormHelperText,
  FormSelect,
  FormSelectOption,
  HelperText,
  HelperTextItem,
  Spinner,
} from '@patternfly/react-core';

// ── Namespace picker ─────────────────────────────────────────────────────────

export interface NamespaceSelectProps {
  value: string;
  onChange: (namespace: string) => void;
  id?: string;
}

/**
 * Namespace picker backed by the cluster's Project list, falling back to
 * Namespace on plain Kubernetes where Project does not exist.
 */
export const NamespaceSelect: React.FC<NamespaceSelectProps> = ({
  value,
  onChange,
  id = 'namespace',
}) => {
  const [projects, projectsLoaded, projectsError] = useK8sWatchResource<K8sResourceCommon[]>({
    groupVersionKind: { group: 'project.openshift.io', version: 'v1', kind: 'Project' },
    isList: true,
  });
  const [namespaces, namespacesLoaded] = useK8sWatchResource<K8sResourceCommon[]>(
    projectsError
      ? { groupVersionKind: { version: 'v1', kind: 'Namespace' }, isList: true }
      : null,
  );

  const options = React.useMemo(() => {
    const source = projectsError ? namespaces : projects;
    return (source ?? [])
      .map((ns) => ns.metadata?.name ?? '')
      .filter(Boolean)
      .sort();
  }, [projects, namespaces, projectsError]);

  const loaded = projectsError ? namespacesLoaded : projectsLoaded;

  return (
    <FormGroup label="Namespace" isRequired fieldId={id}>
      <FormSelect
        id={id}
        value={value}
        onChange={(_e, v) => onChange(v)}
        aria-label="Namespace"
        isDisabled={!loaded}
      >
        {!value && <FormSelectOption value="" label="Select a namespace" isDisabled />}
        {options.map((ns) => (
          <FormSelectOption key={ns} value={ns} label={ns} />
        ))}
        {/* Keep a pre-seeded namespace selectable even before the list loads. */}
        {value && !options.includes(value) && (
          <FormSelectOption key={value} value={value} label={value} />
        )}
      </FormSelect>
    </FormGroup>
  );
};

// ── Section ──────────────────────────────────────────────────────────────────

export const FormSection: React.FC<{
  title: string;
  description?: React.ReactNode;
  children: React.ReactNode;
}> = ({ title, description, children }) => (
  <Card style={{ marginBottom: '16px' }}>
    <CardTitle>{title}</CardTitle>
    <CardBody>
      {description && (
        <FormHelperText style={{ marginBottom: '16px' }}>
          <HelperText>
            <HelperTextItem>{description}</HelperTextItem>
          </HelperText>
        </FormHelperText>
      )}
      {children}
    </CardBody>
  </Card>
);

// ── YAML preview ─────────────────────────────────────────────────────────────

/**
 * Read-only view of the manifest the form will submit. The console serialises
 * the object itself, so the preview always matches what is sent — and users who
 * want to hand-edit are pointed at the console's own YAML create page.
 */
export const ManifestPreview: React.FC<{ manifest: Record<string, unknown> }> = ({ manifest }) => {
  const [expanded, setExpanded] = React.useState(false);

  return (
    <ExpandableSection
      toggleText={expanded ? 'Hide generated YAML' : 'Show generated YAML'}
      isExpanded={expanded}
      onToggle={(_e, isExpanded) => setExpanded(isExpanded)}
    >
      <div style={{ height: '420px', marginBottom: '16px' }}>
        {expanded && <ResourceYAMLEditor initialResource={manifest} readOnly hideHeader />}
      </div>
    </ExpandableSection>
  );
};

// ── Footer ───────────────────────────────────────────────────────────────────

export interface CreateFormFooterProps {
  submitLabel: string;
  isSubmitDisabled?: boolean;
  inProgress?: boolean;
  error?: string;
  onSubmit: () => void;
  onCancel: () => void;
  /** Console YAML create page for the same kind, offered as an escape hatch. */
  yamlHref: string;
}

export const CreateFormFooter: React.FC<CreateFormFooterProps> = ({
  submitLabel,
  isSubmitDisabled,
  inProgress,
  error,
  onSubmit,
  onCancel,
  yamlHref,
}) => (
  <>
    {error && (
      <Alert variant="danger" isInline title="Could not create the resource" style={{ marginBottom: '16px' }}>
        {error}
      </Alert>
    )}
    <ActionGroup>
      <Button
        variant="primary"
        onClick={onSubmit}
        isDisabled={isSubmitDisabled || inProgress}
        icon={inProgress ? <Spinner size="sm" /> : undefined}
      >
        {submitLabel}
      </Button>
      <Button variant="secondary" onClick={onCancel} isDisabled={inProgress}>
        Cancel
      </Button>
      <Button variant="link" component="a" href={yamlHref} isInline style={{ marginLeft: '8px' }}>
        Edit YAML instead
      </Button>
    </ActionGroup>
  </>
);
