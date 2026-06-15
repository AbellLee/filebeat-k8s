import { agentHealthy, inputConfigFromYaml, inputConfigToYaml, timeAgo } from './utils';

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
