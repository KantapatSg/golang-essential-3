import type { Activity, Task, TokenResponse } from './types'

let accessToken: string | null = null
export const setAccessToken = (token: string | null) => { accessToken = token }
export const getAccessToken = () => accessToken

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('Content-Type', 'application/json')
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  const response = await fetch(path, { ...init, headers, credentials: 'include' })
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
}
