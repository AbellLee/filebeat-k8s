import React from 'react';
import { BrowserRouter, Route, Routes, useInRouterContext } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';
import { PLUGIN_BASE_URL, ROUTES } from '../../constants';
const OverviewPage = React.lazy(() => import('../../pages/OverviewPage'));
const PoliciesPage = React.lazy(() => import('../../pages/PoliciesPage'));
const PolicyFormPage = React.lazy(() => import('../../pages/PolicyFormPage'));
const PolicyDetailPage = React.lazy(() => import('../../pages/PolicyDetailPage'));
const AgentsPage = React.lazy(() => import('../../pages/AgentsPage'));

function App(props: AppRootProps) {
  const hasRouter = useInRouterContext();
  if (hasRouter) {
    return <AppRoutes />;
  }

  return (
    <BrowserRouter basename={normalizeBasename(props.basename)}>
      <AppRoutes />
    </BrowserRouter>
  );
}

function AppRoutes() {
  return (
    <Routes>
      <Route path={ROUTES.Overview} element={<OverviewPage />} />
      <Route path={ROUTES.PolicyNew} element={<PolicyFormPage />} />
      <Route path={`${ROUTES.Policies}/:id/edit`} element={<PolicyFormPage />} />
      <Route path={`${ROUTES.Policies}/:id`} element={<PolicyDetailPage />} />
      <Route path={ROUTES.Policies} element={<PoliciesPage />} />
      <Route path={ROUTES.Agents} element={<AgentsPage />} />
      <Route path={`${PLUGIN_BASE_URL}/${ROUTES.Overview}`} element={<OverviewPage />} />
      <Route path={`${PLUGIN_BASE_URL}/${ROUTES.PolicyNew}`} element={<PolicyFormPage />} />
      <Route path={`${PLUGIN_BASE_URL}/${ROUTES.Policies}/:id/edit`} element={<PolicyFormPage />} />
      <Route path={`${PLUGIN_BASE_URL}/${ROUTES.Policies}/:id`} element={<PolicyDetailPage />} />
      <Route path={`${PLUGIN_BASE_URL}/${ROUTES.Policies}`} element={<PoliciesPage />} />
      <Route path={`${PLUGIN_BASE_URL}/${ROUTES.Agents}`} element={<AgentsPage />} />
      <Route path="*" element={<OverviewPage />} />
    </Routes>
  );
}

function normalizeBasename(basename: string) {
  if (!basename) {
    return PLUGIN_BASE_URL;
  }
  return basename.startsWith('/') ? basename : `/${basename}`;
}

export default App;
