import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { BrowserRouter, Navigate, Route, Routes, useParams } from 'react-router-dom'
import { AuthProvider } from './auth'
import { Protected, Shell } from './components'
import {
  AdminActivities,
  AdminInventory,
  AdminPayments,
  Landing,
  LegacyTasks,
  Login,
  Notifications,
  OrderAnalytics,
  OrderDetail,
  Orders,
  Overview,
  Products,
  System,
} from './pages'
import './styles.css'

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 5_000 } } })

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <AuthProvider>
          <Routes>
            <Route path="/" element={<Landing />} />
            <Route path="/login" element={<Login />} />
            <Route element={<Protected />}>
              <Route element={<Shell />}>
                <Route path="/app" element={<Overview />} />
                <Route path="/app/products" element={<Products />} />
                <Route path="/app/orders" element={<Orders />} />
                <Route path="/app/orders/:id" element={<OrderRoute />} />
                <Route path="/app/notifications" element={<Notifications />} />
                <Route path="/app/legacy/tasks" element={<LegacyTasks />} />
                <Route path="/app/tasks" element={<Navigate to="/app/legacy/tasks" replace />} />
                <Route element={<Protected role="admin" />}>
                  <Route path="/app/admin/orders" element={<Orders adminView />} />
                  <Route path="/app/admin/inventory" element={<AdminInventory />} />
                  <Route path="/app/admin/payments" element={<AdminPayments />} />
                  <Route path="/app/admin/activities" element={<AdminActivities />} />
                  <Route path="/app/admin/analytics" element={<OrderAnalytics />} />
                  <Route path="/app/admin/system" element={<System />} />
                  <Route path="/app/activities" element={<Navigate to="/app/admin/activities" replace />} />
                  <Route path="/app/analytics" element={<Navigate to="/app/admin/analytics" replace />} />
                  <Route path="/app/system" element={<Navigate to="/app/admin/system" replace />} />
                </Route>
              </Route>
            </Route>
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </AuthProvider>
      </BrowserRouter>
    </QueryClientProvider>
  )
}

function OrderRoute() {
  const { id } = useParams()
  return id ? <OrderDetail id={id} /> : <Navigate to="/app/orders" replace />
}
