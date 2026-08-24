import { useQuery } from '@tanstack/react-query'
import { Link, NavLink, Navigate, Outlet, useLocation } from 'react-router-dom'
import { api } from './api'
import { useAuth } from './auth'
import type { Money as MoneyValue, OrderStatus, Role } from './types'

const memberNavigation = [
  { to: '/app', label: 'Dashboard', end: true },
  { to: '/app/products', label: 'Catalog' },
  { to: '/app/orders', label: 'Orders' },
]

const adminNavigation = [
  { to: '/app/admin/orders', label: 'All orders' },
  { to: '/app/admin/inventory', label: 'Inventory' },
  { to: '/app/admin/payments', label: 'Payments' },
  { to: '/app/admin/activities', label: 'Activity' },
  { to: '/app/admin/analytics', label: 'Analytics' },
  { to: '/app/admin/system', label: 'System' },
]

export function Shell() {
  const { user, logout } = useAuth()
  const unread = useQuery({
    queryKey: ['notifications', 'unread'],
    queryFn: api.unreadNotifications,
    refetchInterval: 5_000,
  })

  return (
    <div className="shell">
      <header className="topbar">
        <Link className="wordmark" to="/app" aria-label="Order Relay dashboard">
          <span className="mark">OR</span>
          <span><strong>Order Relay</strong><small>operations console</small></span>
        </Link>
        <nav className="primary-nav" aria-label="Primary navigation">
          {memberNavigation.map(item => (
            <NavLink key={item.to} to={item.to} end={item.end}>{item.label}</NavLink>
          ))}
          {user?.role === 'admin' && adminNavigation.map(item => (
            <NavLink key={item.to} to={item.to}>{item.label}</NavLink>
          ))}
        </nav>
        <div className="account">
          <Link className="notification-link" to="/app/notifications" aria-label={`${unread.data?.count ?? 0} unread notifications`}>
            Inbox
            {(unread.data?.count ?? 0) > 0 && <span className="notification-count">{unread.data?.count}</span>}
          </Link>
          <span className="role-chip">{user?.role}</span>
          <button className="text-button" onClick={() => void logout()}>Log out</button>
        </div>
      </header>
      <main className="main"><Outlet /></main>
      <footer className="app-footer">
        <span>Order Relay · REST edge / gRPC core / Kafka workflow</span>
        <Link to="/app/legacy/tasks">Legacy Task lab</Link>
      </footer>
    </div>
  )
}

export function Protected({ role }: { role?: Role }) {
  const { user, loading } = useAuth()
  const location = useLocation()
  if (loading) return <div className="route-state"><State kind="loading" text="Restoring your session…" /></div>
  if (!user) return <Navigate to="/login" state={{ from: location }} replace />
  if (role && user.role !== role) return <Forbidden />
  return <Outlet />
}

export function State({ kind, text }: { kind: 'loading' | 'empty' | 'error'; text: string }) {
  return <div className={`state state-${kind}`} role={kind === 'error' ? 'alert' : 'status'}><span className="state-dot" />{text}</div>
}

export function Forbidden() {
  return (
    <section className="center-panel">
      <p className="eyebrow">403 · operator access</p>
      <h1>This lane is reserved for administrators.</h1>
      <p>Member access is limited to the catalog, owned orders, and personal notifications.</p>
      <Link className="button" to="/app/orders">Back to my orders</Link>
    </section>
  )
}

export function PageHeader({ eyebrow, title, children }: { eyebrow: string; title: string; children?: React.ReactNode }) {
  return <div className="page-header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1></div>{children}</div>
}

export function StatusBadge({ status }: { status: OrderStatus }) {
  return <span className={`status-badge status-${status.toLowerCase()}`}>{status.replace(/_/g, ' ')}</span>
}

export function Money({ value }: { value: MoneyValue | { amount_minor?: number; currency?: string } }) {
  const amount = value.amount_minor ?? 0
  const currency = value.currency || 'USD'
  return <>{new Intl.NumberFormat('en-US', { style: 'currency', currency }).format(amount / 100)}</>
}

const steps = [
  { key: 'received', label: 'Received' },
  { key: 'reserved', label: 'Stock' },
  { key: 'paid', label: 'Payment' },
  { key: 'final', label: 'Final' },
]

function progress(status: OrderStatus) {
  switch (status) {
    case 'PENDING': return 1
    case 'STOCK_RESERVED': return 2
    case 'CONFIRMED': return 4
    case 'CANCELLED': return 4
    case 'REJECTED': return 2
    default: return 1
  }
}

export function OrderRail({ status, compact = false }: { status: OrderStatus; compact?: boolean }) {
  const current = progress(status)
  return (
    <div className={`order-rail ${compact ? 'order-rail-compact' : ''}`} aria-label={`Order progress: ${status}`}>
      {steps.map((step, index) => {
        const reached = index < current
        const failed = index === current - 1 && (status === 'CANCELLED' || status === 'REJECTED')
        return (
          <div className={`rail-step ${reached ? 'rail-reached' : ''} ${failed ? 'rail-failed' : ''}`} key={step.key}>
            <span>{index + 1}</span><small>{step.label}</small>
          </div>
        )
      })}
    </div>
  )
}

export function ShortID({ value }: { value: string }) {
  return <code className="short-id">{value ? value.slice(0, 8).toUpperCase() : '—'}</code>
}

export function SectionEmpty({ title, copy, action }: { title: string; copy: string; action?: React.ReactNode }) {
  return <div className="empty-panel"><span className="empty-scan" /><h2>{title}</h2><p>{copy}</p>{action}</div>
}

export function ErrorMessage({ error }: { error: unknown }) {
  return <State kind="error" text={error instanceof Error ? error.message : 'The service could not complete this request.'} />
}
