// App.tsx (SPEC-12 §3) — provider composition: HeroUI, TanStack Query, then
// the router. Theme and daemon state flow down from the lib stores.
import {QueryClient, QueryClientProvider} from '@tanstack/react-query'
import {Toast} from '@heroui/react'
import {AppRouter} from './router'

const queryClient = new QueryClient({
  defaultOptions: {queries: {retry: false}},
})

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <Toast.Provider placement="bottom end" />
      <AppRouter />
    </QueryClientProvider>
  )
}