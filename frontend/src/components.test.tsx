import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, vi } from 'vitest'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { AuthProvider } from './auth'
import { getAccessToken, setAccessToken } from './api'
import { State } from './components'
import { Landing, LegacyTasks, Login, Notifications, OrderAnalytics, Orders, Products } from './pages'

function testClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 }, mutations: { retry: false } } })
}

function renderWithData(ui: React.ReactNode, path = '/app') {
  const client = testClient()
  return render(<MemoryRouter initialEntries={[path]}><QueryClientProvider client={client}>{ui}</QueryClientProvider></MemoryRouter>)
}

afterEach(() => {
  vi.restoreAllMocks()
  setAccessToken(null)
})

test('landing presents Order Management and its sync/async boundaries', () => {
  render(<MemoryRouter><Landing /></MemoryRouter>)
  expect(screen.getByRole('heading', { name: /One order.*Every boundary visible/i })).toBeInTheDocument()
  expect(screen.getByText('Fiber REST')).toBeInTheDocument()
  expect(screen.getByText('gRPC')).toBeInTheDocument()
  expect(screen.getByText('Kafka')).toBeInTheDocument()
  expect(screen.getByText('ClickHouse')).toBeInTheDocument()
})

test('error state is announced accessibly', () => {
  render(<State kind="error" text="Order service unavailable" />)
  expect(screen.getByRole('alert')).toHaveTextContent('Order service unavailable')
})

test('login submits credentials and stores access in memory', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const url = String(input)
    if (url.includes('/refresh')) return new Response('', { status: 401 })
    if (url.includes('/login')) return Response.json({ access_token: 'access', user_id: 'member-1', role: 'member', token_type: 'Bearer', expires_in: 900 })
    return Response.json({})
  })
  const user = userEvent.setup()
  render(<MemoryRouter><AuthProvider><Login /></AuthProvider></MemoryRouter>)
  await user.click(screen.getByRole('button', { name: /sign in/i }))
  expect(getAccessToken()).toBe('access')
  expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/auth/login'), expect.objectContaining({ method: 'POST' }))
})

test('member builds an order and is routed to its live detail', async () => {
  const orderID = 'order-12345678'
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input)
    if (url.includes('/products')) return Response.json({ products: [{ id: 'prod-desk', name: 'Standing Desk', unit_price_minor: 25000, currency: 'USD', available: 4 }], total: 1 })
    if (url.endsWith('/api/v1/orders') && init?.method === 'POST') return Response.json({ id: orderID, customer_id: 'member-1', items: [], total: { amount_minor: 25000, currency: 'USD' }, status: 'PENDING', payment_scenario: 'success', reason: '', created_at: '2026-08-24T00:00:00Z', updated_at: '2026-08-24T00:00:00Z' })
    return Response.json({})
  })
  const user = userEvent.setup()
  renderWithData(<Routes><Route path="/app/products" element={<Products />} /><Route path="/app/orders/:id" element={<h1>Live order detail</h1>} /></Routes>, '/app/products')

  await user.click(await screen.findByRole('button', { name: 'Increase Standing Desk' }))
  await user.click(screen.getByRole('button', { name: /Send into workflow/i }))
  expect(await screen.findByRole('heading', { name: 'Live order detail' })).toBeInTheDocument()
  const createCall = fetchMock.mock.calls.find(([input, init]) => String(input).endsWith('/api/v1/orders') && init?.method === 'POST')
  expect(createCall?.[1]).toEqual(expect.objectContaining({ method: 'POST', body: JSON.stringify({ items: [{ product_id: 'prod-desk', quantity: 1 }], payment_scenario: 'success' }) }))
})

test('order list renders the terminal state produced by the async workflow', async () => {
  vi.spyOn(globalThis, 'fetch').mockResolvedValue(Response.json({ items: [{ id: 'order-confirmed', customer_id: 'member-1', items: [{ product_id: 'prod-desk', name: 'Desk', quantity: 1, unit_price: { amount_minor: 25000, currency: 'USD' }, line_total: { amount_minor: 25000, currency: 'USD' } }], total: { amount_minor: 25000, currency: 'USD' }, status: 'CONFIRMED', payment_scenario: 'success', reason: '', created_at: '2026-08-24T00:00:00Z', updated_at: '2026-08-24T00:00:01Z' }], total: 1 }))
  renderWithData(<Orders />, '/app/orders')
  expect(await screen.findByRole('link', { name: /Inspect/i })).toHaveAttribute('href', '/app/orders/order-confirmed')
  expect(screen.getAllByText('CONFIRMED').length).toBeGreaterThan(1)
})

test('notification projection can mark an order signal as read', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input)
    if (url.includes('/notifications/n-1/read') && init?.method === 'PATCH') return Response.json({})
    return Response.json({ items: [{ id: 'n-1', user_id: 'member-1', order_id: 'order-1', type: 'ORDER_CONFIRMED', message: 'Order confirmed', read: false, created_at: '2026-08-24T00:00:00Z' }], total: 1 })
  })
  const user = userEvent.setup()
  renderWithData(<Notifications />, '/app/notifications')
  await user.click(await screen.findByRole('button', { name: 'Mark read' }))
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/notifications/n-1/read'), expect.objectContaining({ method: 'PATCH' })))
})

test('admin analytics renders ClickHouse summary and event funnel', async () => {
  vi.spyOn(globalThis, 'fetch').mockImplementation(async input => {
    const url = String(input)
    if (url.includes('/summary')) return Response.json({ created: 8, confirmed: 5, rejected: 1, cancelled: 2, revenue_minor: 125000, currency: 'USD', through: '2026-08-24T00:00:00Z' })
    return Response.json({ created: 8, reserved: 7, paid: 5, confirmed: 5, rejected: 1, cancelled: 2, through: '2026-08-24T00:00:00Z' })
  })
  renderWithData(<OrderAnalytics />, '/app/admin/analytics')
  expect(await screen.findByText('$1,250.00')).toBeInTheDocument()
  expect(screen.getByText('Conversion path')).toBeInTheDocument()
  expect(screen.getByText('Reserved')).toBeInTheDocument()
})

test('legacy Task CRUD remains available only on the compatibility route', async () => {
  const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = String(input)
    if (url.includes('/tasks?')) return Response.json({ items: [{ id: 'task-1', owner_id: 'member-1', title: 'Legacy task', description: '', status: 'todo', created_at: '', updated_at: '' }] })
    if (url.includes('/tasks/task-1') && init?.method === 'PUT') return Response.json({})
    return Response.json({})
  })
  const user = userEvent.setup()
  renderWithData(<LegacyTasks />, '/app/legacy/tasks')
  await user.selectOptions(await screen.findByRole('combobox', { name: 'Status for Legacy task' }), 'doing')
  await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('/tasks/task-1'), expect.objectContaining({ method: 'PUT' })))
  expect(screen.getByText(/active portfolio use case is Order Management/i)).toBeInTheDocument()
})
