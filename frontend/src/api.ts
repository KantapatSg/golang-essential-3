import type { Activity, AnalyticsSummary, StatusCount, Task, TimeseriesPoint, TokenResponse, Product, Order, Notification } from './types'

let accessToken: string | null = null
// ใช้ relative URL ใน local Compose เพื่อให้ Nginx ทำ same-origin proxy และใช้
// VITE_API_BASE_URL ใน Render Static Site ที่อยู่คนละ origin กับ Gateway
const apiBaseURL = (import.meta.env.VITE_API_BASE_URL ?? '').replace(/\/$/, '')
export const setAccessToken = (token: string | null) => { accessToken = token }
export const getAccessToken = () => accessToken

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  const response = await fetch(`${apiBaseURL}${path}`, { ...init, headers, credentials: 'include' })
  if (!response.ok) { const body = await response.json().catch(() => ({})); throw new Error(body.error || `Request failed (${response.status})`) }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
export const api = {
  login: (email: string, password: string) => request<TokenResponse>('/api/v1/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),
  refresh: () => request<TokenResponse>('/api/v1/auth/refresh', { method: 'POST', body: JSON.stringify({}) }),
  logout: () => request<void>('/api/v1/auth/logout', { method: 'POST', body: JSON.stringify({}) }),
  tasks: () => request<{ items: Task[] }>('/api/v1/tasks?page=1&page_size=100'),
  task: (id: string) => request<Task>(`/api/v1/tasks/${id}`),
  createTask: (payload: Pick<Task, 'title' | 'description'>) => request<Task>('/api/v1/tasks', { method: 'POST', body: JSON.stringify(payload) }),
  updateTask: (id: string, payload: Pick<Task, 'title' | 'description' | 'status'>) => request<Task>(`/api/v1/tasks/${id}`, { method: 'PUT', body: JSON.stringify(payload) }),
  deleteTask: (id: string) => request<void>(`/api/v1/tasks/${id}`, { method: 'DELETE' }),
  activities: () => request<Activity[]>('/api/v1/activities'),
  analyticsSummary: () => request<AnalyticsSummary>('/api/v1/analytics/summary'),
  analyticsTimeseries: () => request<{ points: TimeseriesPoint[]; generated_at: string; data_through: string }>('/api/v1/analytics/timeseries'),
  analyticsStatuses: () => request<{ statuses: StatusCount[]; generated_at: string; data_through: string }>('/api/v1/analytics/statuses'),
  gatewayHealth: () => request<{ status: string }>('/health/ready'),
  products: () => request<{ products: Product[]; total: number }>('/api/v1/products?page=1&page_size=100'),
  orders: () => request<{ items: Order[]; total: number }>('/api/v1/orders?page=1&page_size=100'),
  order: (id: string) => request<Order>(`/api/v1/orders/${id}`),
  orderActivities: (id: string) => request<Array<{ id: string; order_id: string; event_type: string; occurred_at: string }>>(`/api/v1/activities?order_id=${id}`),
  createOrder: (items: { product_id: string; quantity: number }[], payment_scenario: string) => request<Order>('/api/v1/orders', { method: 'POST', headers: { 'Idempotency-Key': crypto.randomUUID() }, body: JSON.stringify({ items, payment_scenario }) }),
  notifications: () => request<{ items: Notification[]; total: number }>('/api/v1/notifications?page=1&page_size=100'),
  unreadNotifications: () => request<{ count: number }>('/api/v1/notifications/unread-count'),
  markNotificationRead: (id: string) => request<Notification>(`/api/v1/notifications/${id}/read`, { method: 'PATCH' }),
}
