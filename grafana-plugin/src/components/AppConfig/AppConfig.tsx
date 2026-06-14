import React, { ChangeEvent, FormEvent, useState } from 'react';
import { lastValueFrom } from 'rxjs';
import { css } from '@emotion/css';
import { AppPluginMeta, GrafanaTheme2, PluginConfigPageProps, PluginMeta } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { Alert, Button, Field, FieldSet, Input, SecretInput, useStyles2 } from '@grafana/ui';
import { t, Trans } from '@grafana/i18n';
import pluginJson from '../../plugin.json';
import { AppPluginSettings } from '../../types';
import { testIds } from '../testIds';

type State = {
  controlServerUrl: string;
  isAdminTokenSet: boolean;
  adminToken: string;
};

export interface AppConfigProps extends PluginConfigPageProps<AppPluginMeta<AppPluginSettings>> {}

const AppConfig = ({ plugin }: AppConfigProps) => {
  const s = useStyles2(getStyles);
  const { enabled, pinned, jsonData, secureJsonFields } = plugin.meta;
  const [state, setState] = useState<State>({
    controlServerUrl: jsonData?.controlServerUrl || '',
    adminToken: '',
    isAdminTokenSet: Boolean(secureJsonFields?.adminToken),
  });
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');

  const onChange = (event: ChangeEvent<HTMLInputElement>) => {
    setState({ ...state, [event.target.name]: event.target.value.trim() });
  };

  const onResetAdminToken = () => {
    setState({ ...state, adminToken: '', isAdminTokenSet: false });
  };

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    if (!state.controlServerUrl) {
      setError(t('filebeat-k8s-app.settings.controlServerRequired', 'controlServerUrl is required'));
      return;
    }
    updatePluginAndReload(plugin.meta.id, {
      enabled,
      pinned,
      jsonData: {
        controlServerUrl: state.controlServerUrl,
      },
      secureJsonData: state.isAdminTokenSet
        ? undefined
        : {
            adminToken: state.adminToken,
          },
    }).catch((err) => setError(err.message));
  };

  const testConnection = async () => {
    setMessage('');
    setError('');
    try {
      const response = getBackendSrv().fetch<{ status: string }>({
        url: `/api/plugins/${pluginJson.id}/resources/readyz`,
      });
      const result = await lastValueFrom(response);
      setMessage(t('filebeat-k8s-app.settings.readyzMessage', 'Control Server readyz: {{status}}', { status: result.data.status }));
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <form onSubmit={onSubmit}>
      <FieldSet label={t('filebeat-k8s-app.settings.title', 'Log collection plugin settings')}>
        {message && (
          <Alert title={t('filebeat-k8s-app.common.normal', 'Connection healthy')} severity="success">
            {message}
          </Alert>
        )}
        {error && (
          <Alert title={t('filebeat-k8s-app.common.configError', 'Configuration error')} severity="error">
            {error}
          </Alert>
        )}

        <div className={s.note}>
          <Trans
            i18nKey="filebeat-k8s-app.settings.note"
            defaults="Multi-cloud log path adaptation is detected automatically by control-sidecar on each node. Prefer container_stdio; container_file depends on container rootfs capability, and actual availability is shown on the Agents page."
          />
        </div>

        <Field
          label="controlServerUrl"
          description={t(
            'filebeat-k8s-app.settings.controlServerDescription',
            'The URL used by the Grafana plugin backend to access filebeat-k8s control-server. Docker Compose defaults to http://control-server:8080.'
          )}
        >
          <Input
            width={72}
            name="controlServerUrl"
            id="config-control-server-url"
            data-testid={testIds.appConfig.controlServerUrl}
            value={state.controlServerUrl}
            placeholder="http://control-server:8080"
            onChange={onChange}
          />
        </Field>

        <Field
          label="adminToken"
          description={t(
            'filebeat-k8s-app.settings.adminTokenDescription',
            'Optional. When control-server management API authentication is added, it will be forwarded as Authorization: Bearer.'
          )}
        >
          <SecretInput
            width={72}
            id="config-admin-token"
            data-testid={testIds.appConfig.adminToken}
            name="adminToken"
            value={state.adminToken}
            isConfigured={state.isAdminTokenSet}
            placeholder="optional admin token"
            onChange={onChange}
            onReset={onResetAdminToken}
          />
        </Field>

        <div className={s.actions}>
          <Button type="submit" data-testid={testIds.appConfig.submit}>
            {t('filebeat-k8s-app.common.saveSettings', 'Save settings')}
          </Button>
          <Button type="button" variant="secondary" onClick={testConnection}>
            {t('filebeat-k8s-app.common.testConnection', 'Test connection')}
          </Button>
        </div>
      </FieldSet>
    </form>
  );
};

export default AppConfig;

const getStyles = (theme: GrafanaTheme2) => ({
  actions: css`
    display: flex;
    gap: ${theme.spacing(1)};
    margin-top: ${theme.spacing(3)};
  `,
  note: css`
    margin-bottom: ${theme.spacing(2)};
    color: ${theme.colors.text.secondary};
    max-width: 720px;
  `,
});

const updatePluginAndReload = async (pluginId: string, data: Partial<PluginMeta<AppPluginSettings>>) => {
  await updatePlugin(pluginId, data);
  window.location.reload();
};

const updatePlugin = async (pluginId: string, data: Partial<PluginMeta<AppPluginSettings>>) => {
  const response = getBackendSrv().fetch({
    url: `/api/plugins/${pluginId}/settings`,
    method: 'POST',
    data,
  });

  return lastValueFrom(response);
};
