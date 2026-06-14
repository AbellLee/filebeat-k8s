import { initPluginTranslations } from '@grafana/i18n';
import pluginJson from '../plugin.json';
import { loadPluginResources } from './resources';

export const pluginTranslationsReady = initPluginTranslations(pluginJson.id, [loadPluginResources]).catch((error) => {
  // Keep the plugin usable even when translation loading fails.
  console.error('Failed to initialize filebeat-k8s plugin translations', error);
  return { language: 'en-US' };
});
