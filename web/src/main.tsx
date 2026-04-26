import React from 'react';
import ReactDOM from 'react-dom/client';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { AppLayout } from './App';
import { Dashboard } from './pages/Dashboard';
import { DropletsList } from './pages/DropletsList';
import { DropletDetail } from './pages/DropletDetail';
import { AqueductDetail } from './pages/AqueductDetail';
import { CastellariusPage } from './pages/CastellariusPage';
import { DoctorPage } from './pages/DoctorPage';
import { LogsPage } from './pages/LogsPage';
import { ReposSkillsPage } from './pages/ReposSkillsPage';
import { CreateDroplet } from './pages/CreateDroplet';
import { FilterPage } from './pages/FilterPage';
import { ImportPage } from './pages/ImportPage';
import { NotFound } from './pages/NotFound';
import { ErrorBoundary } from './components/ErrorBoundary';
import './index.css';

const router = createBrowserRouter([
  {
    path: '/app',
    element: <AppLayout />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'droplets', element: <DropletsList /> },
      { path: 'droplets/new', element: <CreateDroplet /> },
      { path: 'droplets/:id', element: <DropletDetail /> },
      { path: 'aqueducts/:name', element: <AqueductDetail /> },
      { path: 'castellarius', element: <CastellariusPage /> },
      { path: 'doctor', element: <DoctorPage /> },
      { path: 'logs', element: <LogsPage /> },
      { path: 'repos', element: <ReposSkillsPage /> },
      { path: 'filter', element: <FilterPage /> },
      { path: 'import', element: <ImportPage /> },
      { path: '*', element: <NotFound /> },
    ],
  },
]);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ErrorBoundary>
      <RouterProvider router={router} />
    </ErrorBoundary>
  </React.StrictMode>,
);