import React from 'react';
import { Route, Routes } from 'react-router-dom';
import { AppRootProps } from '@grafana/data';
import { ROUTES } from '../../constants';
const OverviewPage = React.lazy(() => import('../../pages/OverviewPage'));
const PoliciesPage = React.lazy(() => import('../../pages/PoliciesPage'));
const PolicyFormPage = React.lazy(() => import('../../pages/PolicyFormPage'));
const PolicyDetailPage = React.lazy(() => import('../../pages/PolicyDetailPage'));
const AgentsPage = React.lazy(() => import('../../pages/AgentsPage'));

function App(props: AppRootProps) {
  return (
    <Routes>
      <Route path={ROUTES.Overview} element={<OverviewPage />} />
      <Route path={ROUTES.PolicyNew} element={<PolicyFormPage />} />
      <Route path={`${ROUTES.Policies}/:id/edit`} element={<PolicyFormPage />} />
      <Route path={`${ROUTES.Policies}/:id`} element={<PolicyDetailPage />} />
      <Route path={ROUTES.Policies} element={<PoliciesPage />} />
      <Route path={ROUTES.Agents} element={<AgentsPage />} />
      <Route path="*" element={<OverviewPage />} />
    </Routes>
  );
}

export default App;
