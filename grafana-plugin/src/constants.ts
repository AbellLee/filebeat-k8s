import pluginJson from './plugin.json';

export const PLUGIN_BASE_URL = `/a/${pluginJson.id}`;

export enum ROUTES {
  Overview = 'overview',
  Policies = 'policies',
  PolicyNew = 'policies/new',
  Agents = 'agents',
}
