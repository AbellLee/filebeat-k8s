import { agentHealthy, derivePolicyName, inputConfigFromYaml, inputConfigToYaml, timeAgo, uniqueClusterIds } from './utils';

describe('pages/utils input_config YAML helpers', () => {
  test('parses input-level config objects with arrays and scalar values', () => {
    expect(
      inputConfigFromYaml(`
scan_frequency: 10s
ignore_older: 72h
harvester_limit: 5
exclude_files:
  - "\\\\.gz$"
recursive_glob.enabled: true
`)
    ).toEqual({
      scan_frequency: '10s',
      ignore_older: '72h',
      harvester_limit: 5,
      exclude_files: ['\\.gz$'],
      'recursive_glob.enabled': true,
    });
  });

  test('rejects reserved generated input fields', () => {
    expect(() => inputConfigFromYaml('paths:\n  - /tmp/*.log')).toThrow(/reserved field paths/);
    expect(() => inputConfigFromYaml('processors: []')).toThrow(/reserved field processors/);
  });

  test('rejects non-object top-level YAML', () => {
    expect(() => inputConfigFromYaml('- scan_frequency')).toThrow(/object\/map/);
    expect(() => inputConfigFromYaml('plain-string')).toThrow(/object\/map/);
  });

  test('formats existing input_config for editing', () => {
    expect(inputConfigToYaml({ scan_frequency: '10s', exclude_files: ['\\\\.gz$'] })).toContain('scan_frequency: 10s');
  });
});

describe('pages/utils agent heartbeat helpers', () => {
  beforeEach(() => {
    jest.useFakeTimers();
    jest.setSystemTime(new Date('2026-06-15T12:00:00Z'));
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  test('does not show a far-future heartbeat as just seen', () => {
    expect(timeAgo('2026-06-15T20:00:00Z')).toBe('clock skew');
  });

  test('does not mark far-future heartbeat timestamps healthy', () => {
    expect(
      agentHealthy({
        id: 'dev:node-a',
        cluster_id: 'dev',
        node_name: 'node-a',
        last_heartbeat_at: '2026-06-15T20:00:00Z',
      })
    ).toBe(false);
  });

  test('allows small clock skew for healthy agents', () => {
    expect(
      agentHealthy({
        id: 'dev:node-a',
        cluster_id: 'dev',
        node_name: 'node-a',
        last_heartbeat_at: '2026-06-15T12:00:10Z',
      })
    ).toBe(true);
  });
});

describe('pages/utils policy form helpers', () => {
  test('derives container_stdio names from target scope', () => {
    expect(
      derivePolicyName({
        id: '',
        name: '',
        cluster_id: 'dev',
        namespace: 'payment',
        controller_type: 'deployment',
        controller_name: 'payment-api',
        container_name: 'app',
        log_type: 'container_stdio',
        enabled: true,
        priority: 100,
        current_revision: 0,
      })
    ).toBe('payment / deployment/payment-api / app / container_stdio');
  });

  test('derives container_file names with log path', () => {
    expect(
      derivePolicyName({
        id: '',
        name: '',
        cluster_id: 'dev',
        namespace: 'payment',
        controller_type: 'deployment',
        controller_name: 'payment-api',
        container_name: 'app',
        log_type: 'container_file',
        log_path: '/app/logs/access.log',
        enabled: true,
        priority: 100,
        current_revision: 0,
      })
    ).toBe('payment / deployment/payment-api / app / container_file / /app/logs/access.log');
  });

  test('derives host_file names from cluster and host path', () => {
    expect(
      derivePolicyName({
        id: '',
        name: '',
        cluster_id: 'prod-hk',
        log_type: 'host_file',
        log_path: '/var/log/messages',
        enabled: true,
        priority: 100,
        current_revision: 0,
      })
    ).toBe('prod-hk / host_file / /var/log/messages');
  });

  test('returns an empty name until required target fields are complete', () => {
    expect(
      derivePolicyName({
        id: '',
        name: '',
        cluster_id: 'dev',
        namespace: 'payment',
        controller_type: 'deployment',
        controller_name: 'payment-api',
        log_type: 'container_stdio',
        enabled: true,
        priority: 100,
        current_revision: 0,
      })
    ).toBe('');
    expect(
      derivePolicyName({
        id: '',
        name: '',
        cluster_id: 'dev',
        log_type: 'host_file',
        enabled: true,
        priority: 100,
        current_revision: 0,
      })
    ).toBe('');
  });

  test('returns sorted unique non-empty cluster ids from agents', () => {
    expect(
      uniqueClusterIds([
        { id: 'prod:node-a', cluster_id: ' prod ', node_name: 'node-a' },
        { id: 'dev:node-a', cluster_id: 'dev', node_name: 'node-a' },
        { id: 'empty:node-a', cluster_id: ' ', node_name: 'node-a' },
        { id: 'prod:node-b', cluster_id: 'prod', node_name: 'node-b' },
      ])
    ).toEqual(['dev', 'prod']);
  });
});
