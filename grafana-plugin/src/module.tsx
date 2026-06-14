import React, { Suspense, lazy, useEffect, useState } from 'react';
import { AppPlugin, type AppRootProps } from '@grafana/data';
import { LoadingPlaceholder } from '@grafana/ui';
import type { AppConfigProps } from './components/AppConfig/AppConfig';
import { pluginTranslationsReady } from './i18n/init';
import { AppPluginSettings } from './types';

const LazyApp = lazy(() => import('./components/App/App'));
const LazyAppConfig = lazy(() => import('./components/AppConfig/AppConfig'));

const App = (props: AppRootProps) => (
  <TranslationsGate>
    <Suspense fallback={<LoadingPlaceholder text="" />}>
      <LazyApp {...props} />
    </Suspense>
  </TranslationsGate>
);

const AppConfig = (props: AppConfigProps) => (
  <TranslationsGate>
    <Suspense fallback={<LoadingPlaceholder text="" />}>
      <LazyAppConfig {...props} />
    </Suspense>
  </TranslationsGate>
);

function TranslationsGate({ children }: { children: React.ReactNode }) {
  const [ready, setReady] = useState(false);

  useEffect(() => {
    let mounted = true;
    pluginTranslationsReady.finally(() => {
      if (mounted) {
        setReady(true);
      }
    });
    return () => {
      mounted = false;
    };
  }, []);

  if (!ready) {
    return <LoadingPlaceholder text="Loading translations..." />;
  }

  return <>{children}</>;
}

export const plugin = new AppPlugin<AppPluginSettings>().setRootPage(App).addConfigPage({
  title: 'Settings',
  icon: 'cog',
  body: AppConfig,
  id: 'configuration',
});
