import type {
  AnalyticsSummary,
  Notification,
  Order,
  OrderActivity,
  OrderFunnel,
  OrderSummary,
  Payment,
  Product,
  Reservation,
  StockMovement,
  StatusCount,
  Task,
  TimeseriesPoint,
  TokenResponse,
} from './types'

let accessToken: string | null = null

// Compose ใช้ same-origin proxy ส่วน Render สามารถกำหนด Gateway คนละ origin ผ่าน build-time env ได้
const apiBaseURL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')

export const setAccessToken = (token: string | null) => { accessToken = token }
export const getAccessToken = () => accessToken

async function requestMeta<T>(path: string, init: RequestInit = {}): Promise<{ data: T; headers: Headers }> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)

  const response = await fetch(`${apiBaseURL}${path}`, {
    ...init,
    headers,
    credentials: 'include',
  })

  if (!response.ok) {
    const body = await response.json().catch(() => ({})) as { error?: string }
    throw new Error(body.error || `Request failed (${response.status})`)
  }
  if (response.status === 204) return { data: undefined as T, headers: response.headers }
  return { data: await response.json() as T, headers: response.headers }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const result = await requestMeta<T>(path, init)
  return result.data
}

function list<T>(items: T[] | null | undefined): T[] {
  return items ?? []
}

export const api = {
  login: (email: string, password: string) =>
    request<TokenResponse>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),
  refresh: () =>
    request<TokenResponse>('/api/v1/auth/refresh', { method: 'POST', body: JSON.stringify({}) }),
  logout: () =>
    request<void>('/api/v1/auth/logout', { method: 'POST', body: JSON.stringify({}) }),

  products: async () => {
    const response = await requestMeta<{ products?: Product[]; total?: number }>('/api/v1/products?page=1&page_size=100')
    return { products: list(response.data.products).map(product => ({ ...product, available: product.available ?? 0 })), total: response.data.total ?? 0, cacheStatus: response.headers.get('X-Cache-Status') ?? 'BYPASS' }
  },
  orders: async () => {
    const response = await request<{ items?: Order[]; total?: number }>('/api/v1/orders?page=1&page_size=100')
    return { items: list(response.items), total: response.total ?? 0 }
  },
  order: (id: string) => request<Order>(`/api/v1/orders/${id}`),
  createOrder: (items: { product_id: string; quantity: number }[], payment_scenario: string) =>
    request<Order>('/api/v1/orders', {
      method: 'POST',
      headers: { 'Idempotency-Key': crypto.randomUUID() },
      body: JSON.stringify({ items, payment_scenario }),
    }),
  orderActivities: async (id: string) =>
    list(await request<OrderActivity[]>(`/api/v1/activities?order_id=${encodeURIComponent(id)}`)),

  notifications: async () => {
    const response = await request<{ items?: Notification[]; total?: number }>('/api/v1/notifications?page=1&page_size=100')
    return { items: list(response.items), total: response.total ?? 0 }
  },
  unreadNotifications: async () => {
    const response = await request<{ count?: number }>('/api/v1/notifications/unread-count')
    return { count: response.count ?? 0 }
  },
  markNotificationRead: (id: string) =>
    request<Notification>(`/api/v1/notifications/${id}/read`, { method: 'PATCH' }),
  markAllNotificationsRead: () =>
    request<void>('/api/v1/notifications/read-all', { method: 'POST', body: JSON.stringify({}) }),

  reservations: async () => {
    const response = await request<{ reservations?: Reservation[]; total?: number }>('/api/v1/admin/inventory/reservations')
    return { reservations: list(response.reservations), total: response.total ?? 0 }
  },
  inventory: async () => {
    const response = await request<{ products?: Product[]; total?: number }>('/api/v1/admin/inventory')
    return { products: list(response.products), total: response.total ?? 0 }
  },
  adjustStock: (payload: { product_id: string; delta: number; reason: string }) =>
    request<{ product: Product; movement: StockMovement; replayed: boolean }>('/api/v1/admin/inventory/adjustments', { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() }, body: JSON.stringify(payload) }),
  stockMovements: async (productID?: string) => {
    const response = await request<{ movements?: StockMovement[]; total?: number }>(`/api/v1/admin/inventory/movements${productID ? `?product_id=${encodeURIComponent(productID)}` : ''}`)
    return { movements: list(response.movements), total: response.total ?? 0 }
  },
  payments: async () => {
    const response = await request<{ payments?: Payment[]; total?: number }>('/api/v1/admin/payments')
    return { payments: list(response.payments), total: response.total ?? 0 }
  },
  orderSummary: () => request<OrderSummary>('/api/v1/admin/analytics/orders/summary'),
  orderFunnel: () => request<OrderFunnel>('/api/v1/admin/analytics/orders/funnel'),
  adminOrderActivities: async () => {
    const response = await request<{ items?: OrderActivity[]; total?: number }>('/api/v1/admin/activities/orders')
    return { items: list(response.items), total: response.total ?? 0 }
  },
  gatewayHealth: () => request<{ status: string }>('/health/ready'),

  // Legacy Task API remains callable for study and rollback compatibility, but is not the portfolio default.
  tasks: () => request<{ items: Task[] }>('/api/v1/tasks?page=1&page_size=100'),
  createTask: (payload: Pick<Task, 'title' | 'description'>) =>
    request<Task>('/api/v1/tasks', { method: 'POST', body: JSON.stringify(payload) }),
  updateTask: (id: string, payload: Pick<Task, 'title' | 'description' | 'status'>) =>
    request<Task>(`/api/v1/tasks/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteTask: (id: string) => request<void>(`/api/v1/tasks/${id}`, { method: 'DELETE' }),
  legacyAnalyticsSummary: () => request<AnalyticsSummary>('/api/v1/analytics/summary'),
  legacyAnalyticsTimeseries: () =>
    request<{ points: TimeseriesPoint[]; generated_at: string; data_through: string }>('/api/v1/analytics/timeseries'),
  legacyAnalyticsStatuses: () =>
    request<{ statuses: StatusCount[]; generated_at: string; data_through: string }>('/api/v1/analytics/statuses'),
}
