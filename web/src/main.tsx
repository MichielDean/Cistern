import React from 'react';
import ReactDOM from 'react-dom/client';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { AppLayout } from './App';
import { Dashboard } from './pages/Dashboard';
import { PlaceholderPage } from './pages/Placeholder';
import './index.css';

const router = createBrowserRouter([
  {
    path: '/app',
    element: <AppLayout />,
    children: [
      { index: true, element: <Dashboard /> },
      { path: 'droplets', element: <PlaceholderPage title="Droplets" /> },
      { path: 'castellarius', element: <PlaceholderPage title="Castellarius" /> },
      { path: 'doctor', element: <PlaceholderPage title="Doctor" /> },
      { path: 'logs', element: <PlaceholderPage title="Logs" /> },
      { path: 'repos', element: <PlaceholderPage title="Repos / Skills" /> },
    ],
  },
]);

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <RouterProvider router={router} />
  </React.StrictMode>,
);