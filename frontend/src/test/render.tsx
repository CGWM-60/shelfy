import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { APIProvider } from '../api/context'
import type { APIClient } from '../api/client'

export function renderRoute(element: React.ReactNode, client: APIClient, path = '/') {
  return render(
    <APIProvider client={client}>
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="*" element={element} />
        </Routes>
      </MemoryRouter>
    </APIProvider>
  )
}
