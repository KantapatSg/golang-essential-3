export type Role = 'admin' | 'member'
export type User = { id: string; role: Role; email?: string }
export type Task = { id: string; owner_id: string; title: string; description: string; status: 'todo' | 'doing' | 'done'; created_at: string; updated_at: string }
export type Activity = { id: string; event_id: string; event_type: string; task_id: string; actor_id: string; occurred_at: string }
export type TokenResponse = { access_token: string; refresh_token?: string; token_type: string; expires_in: number; user_id: string; role: Role }
export type AnalyticsSummary = { total_events: number; created: number; updated: number; deleted: number; generated_at: string; data_through: string }
export type TimeseriesPoint = { day: string; created: number; updated: number; deleted: number }
export type StatusCount = { status: string; count: number }
