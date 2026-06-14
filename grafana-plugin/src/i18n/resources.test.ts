import { CHINESE_SIMPLIFIED, DEFAULT_LANGUAGE } from '@grafana/i18n';
import { hasPluginResources, loadPluginResources } from './resources';

describe('i18n resources', () => {
  test('loads zh-Hans resources', async () => {
    const resources = (await loadPluginResources(CHINESE_SIMPLIFIED)) as {
      'filebeat-k8s-app': { overview: { title: string }; settings: { title: string } };
    };

    expect(resources['filebeat-k8s-app'].overview.title).toBe('日志采集总览');
    expect(resources['filebeat-k8s-app'].settings.title).toBe('日志采集插件设置');
  });

  test('uses source fallback resources for en-US', async () => {
    await expect(loadPluginResources(DEFAULT_LANGUAGE)).resolves.toEqual({});
  });

  test('reports supported languages', () => {
    expect(hasPluginResources(DEFAULT_LANGUAGE)).toBe(true);
    expect(hasPluginResources(CHINESE_SIMPLIFIED)).toBe(true);
    expect(hasPluginResources('ja-JP')).toBe(false);
  });
});
