import * as React from 'react';
import {
  Alert,
  Checkbox,
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
import { RedisConfig } from '../../frappe/types';

/** Flat form state for a bench's `redisConfig` block. */
export interface RedisConfigValue {
  type: 'redis' | 'dragonfly';
  external: boolean;
  host: string;
  port: string;
  connectionSecret: string;
  image: string;
  maxMemory: string;
  storageSize: string;
}

export const defaultRedisConfig = (): RedisConfigValue => ({
  type: 'redis',
  external: false,
  host: '',
  port: '6379',
  connectionSecret: '',
  image: '',
  maxMemory: '',
  storageSize: '',
});

/**
 * When Redis is external the operator provisions nothing, so a reachable host
 * is the one thing it cannot fall back on.
 */
export const redisConfigErrors = (value: RedisConfigValue): string[] => {
  if (!value.external) {
    return [];
  }
  const errors: string[] = [];
  if (!value.host) {
    errors.push('An external Redis needs a host.');
  }
  if (value.port && !/^\d+$/.test(value.port)) {
    errors.push('The Redis port must be a number.');
  }
  return errors;
};

/**
 * Projects the form state onto the CRD's `redisConfig`. Note that `port` is an
 * integer here, unlike the database config where it is a string.
 */
export const buildRedisConfig = (value: RedisConfigValue): RedisConfig => {
  if (value.external) {
    return {
      type: value.type,
      external: true,
      host: value.host,
      ...(value.port ? { port: Number(value.port) } : {}),
      ...(value.connectionSecret ? { connectionSecretRef: { name: value.connectionSecret } } : {}),
    };
  }

  return {
    type: value.type,
    ...(value.image ? { image: value.image } : {}),
    ...(value.maxMemory ? { maxMemory: value.maxMemory } : {}),
    ...(value.storageSize ? { storageSize: value.storageSize } : {}),
  };
};

export interface RedisConfigFieldsProps {
  value: RedisConfigValue;
  onChange: (value: RedisConfigValue) => void;
  namespace?: string;
}

/**
 * Redis / Dragonfly configuration for a bench. Frappe uses Redis for its cache,
 * queue and socketio channels, and every site on the bench shares this one
 * configuration — the CRD has no per-site override.
 */
export const RedisConfigFields: React.FC<RedisConfigFieldsProps> = ({
  value,
  onChange,
  namespace,
}) => {
  const set = (patch: Partial<RedisConfigValue>) => onChange({ ...value, ...patch });
  const portInvalid = !!value.port && !/^\d+$/.test(value.port);

  return (
    <Grid hasGutter>
      <GridItem md={4}>
        <FormGroup label="Engine" fieldId="bench-redis-type">
          <FormSelect
            id="bench-redis-type"
            value={value.type}
            onChange={(_e, v) => set({ type: v as 'redis' | 'dragonfly' })}
            aria-label="Redis engine"
          >
            <FormSelectOption value="redis" label="Redis" />
            <FormSelectOption value="dragonfly" label="Dragonfly" />
          </FormSelect>
          <FormHelperText>
            <HelperText>
              <HelperTextItem>
                Backs the Frappe cache, the background job queues and socketio.
              </HelperTextItem>
            </HelperText>
          </FormHelperText>
        </FormGroup>
      </GridItem>

      <GridItem md={8}>
        <FormGroup label="Deployment" fieldId="bench-redis-external">
          <Checkbox
            id="bench-redis-external"
            label="Use an existing Redis outside this bench"
            description="The operator provisions nothing and points every site on the bench at the endpoint below."
            isChecked={value.external}
            onChange={(_e, checked) => set({ external: checked })}
          />
        </FormGroup>
      </GridItem>

      {value.external ? (
        <>
          <GridItem md={5}>
            <FormGroup label="Host" isRequired fieldId="bench-redis-host">
              <TextInput
                id="bench-redis-host"
                value={value.host}
                onChange={(_e, v) => set({ host: v })}
                validated={value.host ? 'default' : 'error'}
                placeholder="redis.example.com"
              />
            </FormGroup>
          </GridItem>
          <GridItem md={3}>
            <FormGroup label="Port" fieldId="bench-redis-port">
              <TextInput
                id="bench-redis-port"
                value={value.port}
                onChange={(_e, v) => set({ port: v })}
                validated={portInvalid ? 'error' : 'default'}
                placeholder="6379"
              />
              {portInvalid && (
                <FormHelperText>
                  <HelperText>
                    <HelperTextItem variant="error">Enter a port number.</HelperTextItem>
                  </HelperText>
                </FormHelperText>
              )}
            </FormGroup>
          </GridItem>
          <GridItem md={4}>
            <FormGroup label="Connection secret" fieldId="bench-redis-secret">
              <TextInput
                id="bench-redis-secret"
                value={value.connectionSecret}
                onChange={(_e, v) => set({ connectionSecret: v })}
                placeholder="unauthenticated"
              />
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>
                    Secret in {namespace || 'this namespace'} holding the Redis credentials. Leave
                    empty for an unauthenticated endpoint.
                  </HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          </GridItem>
          <GridItem md={12}>
            <Alert variant="info" isInline title="The operator does not manage this Redis">
              Availability, eviction policy and persistence stay yours. Frappe needs the cache,
              queue and socketio databases to be writable by these credentials.
            </Alert>
          </GridItem>
        </>
      ) : (
        <>
          <GridItem md={4}>
            <FormGroup label="Max memory" fieldId="bench-redis-max-memory">
              <TextInput
                id="bench-redis-max-memory"
                value={value.maxMemory}
                onChange={(_e, v) => set({ maxMemory: v })}
                placeholder="operator default"
              />
              <FormHelperText>
                <HelperText>
                  <HelperTextItem>Sets the cache eviction ceiling, for example 512Mi.</HelperTextItem>
                </HelperText>
              </FormHelperText>
            </FormGroup>
          </GridItem>
          <GridItem md={4}>
            <FormGroup label="Volume size" fieldId="bench-redis-storage">
              <TextInput
                id="bench-redis-storage"
                value={value.storageSize}
                onChange={(_e, v) => set({ storageSize: v })}
                placeholder="operator default"
              />
            </FormGroup>
          </GridItem>
          <GridItem md={4}>
            <FormGroup label="Image" fieldId="bench-redis-image">
              <TextInput
                id="bench-redis-image"
                value={value.image}
                onChange={(_e, v) => set({ image: v })}
                placeholder="operator default"
              />
            </FormGroup>
          </GridItem>
        </>
      )}
    </Grid>
  );
};

export default RedisConfigFields;
