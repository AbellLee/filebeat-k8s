import React, { ChangeEvent, FormEvent, useState } from 'react';
import { lastValueFrom } from 'rxjs';
import { css } from '@emotion/css';
import { AppPluginMeta, GrafanaTheme2, PluginConfigPageProps, PluginMeta } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { Alert, Button, Field, FieldSet, Input, SecretInput, useStyles2 } from '@grafana/ui';
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
      setError('controlServerUrl is required');
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
      setMessage(`Control Server readyz: ${result.data.status}`);
    } catch (err) {
      setError((err as Error).message);
    }
  };

  return (
    <form onSubmit={onSubmit}>
      <FieldSet label="日志采集插件设置">
        {message && <Alert title="连接正常" severity="success">{message}</Alert>}
        {error && <Alert title="配置错误" severity="error">{error}</Alert>}

        <div className={s.note}>
          多云日志路径适配由 control-sidecar 在每个节点自动探测。推荐优先使用 container_stdio；
          container_file 依赖容器 rootfs 能力，实际可用性请在 Agents 页面查看。
        </div>

        <Field
          label="controlServerUrl"
          description="Grafana 插件后端访问 filebeat-k8s control-server 的地址。Docker Compose 默认使用 http://control-server:8080。"
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

        <Field label="adminToken" description="可选。未来 control-server 管理 API 加鉴权后，会以 Authorization: Bearer 转发。">
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
            保存设置
          </Button>
          <Button type="button" variant="secondary" onClick={testConnection}>
            测试连接
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
