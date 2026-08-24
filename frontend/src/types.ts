export type Role = 'admin' | 'member'

export type User = {
  id: string
  role: Role
  email?: string
}

export type TokenResponse = {
  access_token: string
  refresh_token?: string
  token_type: string
  expires_in: number
  user_id: string
  role: Role
}

export type Money = {
  amount_minor: number
  currency: string
}

export type Product = {
  id: string
  name: string
  unit_price_minor: number
  currency: string
  available: number
}

export type OrderItem = {
  product_id: string
  name: string
  quantity: number
  unit_price: Money
  line_total: Money
}

export type OrderStatus =
  | 'PENDING'
  | 'STOCK_RESERVED'
  | 'CONFIRMED'
  | 'CANCELLED'
  | 'REJECTED'
  | string

export type Order = {
  id: string
  customer_id: string
  items: OrderItem[]
  total: Money
  status: OrderStatus
  payment_scenario: string
  reason: string
  created_at: string
  updated_at: string
}

export type OrderActivity = {
  id: string
  order_id: string
  event_type: string
  reason?: string
  occurred_at: string
}

export type Notification = {
  id: string
  user_id: string
  order_id: string
  type: string
  message: string
  read: boolean
  created_at: string
}

export type Reservation = {
  id: string
  order_id: string
  product_id: string
  quantity: number
  status: string
  reason: string
  created_at: string
}

export type Payment = {
  id: string
  order_id: string
  status: string
  amount_minor: number
  currency: string
  reason: string
  created_at: string
}

export type OrderSummary = {
  created: number
  confirmed: number
  rejected: number
  cancelled: number
  revenue_minor: number
  currency: string
  through: string
}

export type OrderFunnel = {
  created: number
  reserved: number
  paid: number
  confirmed: number
  rejected: number
  cancelled: number
  through: string
}

// Legacy Task contracts remain available only for the rollback/learning lab.
export type Task = {
  id: string
  owner_id: string
  title: string
  description: string
  status: 'todo' | 'doing' | 'done'
  created_at: string
  updated_at: string
}

export type AnalyticsSummary = {
  total_events: number
  created: number
  updated: number
  deleted: number
  generated_at: string
  data_through: string
}

export type TimeseriesPoint = {
  day: string
  created: number
  updated: number
  deleted: number
}

export type StatusCount = {
  status: string
  count: number
}
