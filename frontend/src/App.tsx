import { BrowserRouter, Routes, Route, useParams } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AuthProvider } from './auth'
import { Shell, Protected } from './components'
import { Analytics, Activities, Landing, Login, Overview, System, Tasks, Products, Orders, OrderDetail, Notifications } from './pages'
import './styles.css'

const queryClient = new QueryClient({ defaultOptions: { queries: { retry: 1, staleTime: 15_000 } } })
export default function App() { return <QueryClientProvider client={queryClient}><BrowserRouter><AuthProvider><Routes><Route path="/" element={<Landing />} /><Route path="/login" element={<Login />} /><Route element={<Protected />}><Route element={<Shell />}><Route path="/app" element={<Overview />} /><Route path="/app/tasks" element={<Tasks />} /><Route path="/app/products" element={<Products />} /><Route path="/app/orders" element={<Orders />} /><Route path="/app/orders/:id" element={<OrderRoute />} /><Route path="/app/notifications" element={<Notifications />} /><Route element={<Protected role="admin" />}><Route path="/app/activities" element={<Activities />} /><Route path="/app/analytics" element={<Analytics />} /><Route path="/app/system" element={<System />} /></Route></Route></Route></Routes></AuthProvider></BrowserRouter></QueryClientProvider> }
function OrderRoute() { const { id } = useParams(); return id ? <OrderDetail id={id} /> : null }
