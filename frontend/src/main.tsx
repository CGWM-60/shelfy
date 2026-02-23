import React from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import { APIProvider } from './api/context'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <APIProvider>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </APIProvider>
  </React.StrictMode>
)
