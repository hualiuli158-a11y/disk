import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './app/App';
import './styles/styles.css';
import './styles/hana-overrides.css';
import './styles/reading-workspace.css';

createRoot(document.getElementById('root')!).render(
  <React.StrictMode><App /></React.StrictMode>,
);
