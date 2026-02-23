import { createContext, useContext } from 'react'
import { apiClient, type APIClient } from './client'

const APIContext = createContext<APIClient>(apiClient)

export function APIProvider({ client, children }: { client?: APIClient; children: React.ReactNode }) {
  return <APIContext.Provider value={client ?? apiClient}>{children}</APIContext.Provider>
}

export function useAPI() {
  return useContext(APIContext)
}
