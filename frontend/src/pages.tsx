import { useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './api'
import { useAuth } from './auth'
import {
  ErrorMessage,
  Money,
  OrderRail,
  PageHeader,
  SectionEmpty,
  ShortID,
  State,
  StatusBadge,
} from './components'
import type { Order, OrderStatus, Product } from './types'

const terminalStates = new Set<OrderStatus>(['CONFIRMED', 'CANCELLED', 'REJECTED'])

function dateTime(value?: string) {
  if (!value) return 'waiting for event'
  return new Intl.DateTimeFormat('en-GB', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

function reasonLabel(reason?: string) {
  if (!reason) return ''
  return reason.replace(/_/g, ' ').toLowerCase()
}

function eventLabel(event: string) {
  return event.replace(/([a-z])([A-Z])/g, '$1 $2')
}

export function Landing() {
  return (
    <div className="landing">
      <header className="landing-nav">
        <Link className="wordmark" to="/"><span className="mark">OR</span><span><strong>Order Relay</strong><small>Go systems portfolio</small></span></Link>
        <div className="landing-actions"><a href="/swagger/">API map</a><Link className="button button-quiet" to="/login">Enter console <span>↗</span></Link></div>
      </header>
      <main>
        <section className="hero">
          <div className="hero-copy">
            <p className="eyebrow">Order processing · event driven</p>
            <h1>One order.<br /><em>Every boundary visible.</em></h1>
            <p className="lede">Create through REST, coordinate through gRPC, then watch Inventory, Payment, Notifications and Analytics move independently through Kafka.</p>
            <div className="hero-actions"><Link className="button" to="/login">Run the workflow <span>→</span></Link><a className="button button-quiet" href="/openapi.yaml">Read OpenAPI</a></div>
          </div>
          <div className="dispatch-card" aria-label="Order workflow preview">
            <div className="dispatch-head"><span>ROUTE MANIFEST</span><span className="live-signal">LOCAL / READY</span></div>
            <div className="manifest-id"><small>ORDER</small><strong>8F2A—19C4</strong><span>PENDING</span></div>
            <OrderRail status="CONFIRMED" />
            <div className="transport-grid">
              <div><small>PUBLIC EDGE</small><b>Fiber REST</b></div>
              <div><small>SYNC CORE</small><b>gRPC</b></div>
              <div><small>ASYNC RAIL</small><b>Kafka</b></div>
              <div><small>ANALYTICS</small><b>ClickHouse</b></div>
            </div>
            <div className="barcode" aria-hidden="true" />
          </div>
        </section>
        <section className="boundary-story">
          <article><span>REQUEST</span><h2>Commit the promise.</h2><p>Order and Outbox commit together. The browser receives <code>PENDING</code> without waiting for every downstream service.</p></article>
          <article><span>EVENT</span><h2>Move the work.</h2><p>Kafka fans the event to Inventory, Payment, Activity, Notification and Analytics with idempotent consumers.</p></article>
          <article><span>OPERATE</span><h2>See the truth.</h2><p>PostgreSQL owns current state, ClickHouse owns business history, and Prometheus reports the health of the path.</p></article>
        </section>
      </main>
    </div>
  )
}

export function Login() {
  const { login } = useAuth()
  const navigate = useNavigate()
  const [email, setEmail] = useState('member@example.com')
  const [password, setPassword] = useState('member123')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  return (
    <div className="auth-page">
      <section className="auth-aside">
        <Link className="wordmark" to="/"><span className="mark">OR</span><span><strong>Order Relay</strong><small>operations console</small></span></Link>
        <p className="eyebrow">Demo access / local only</p>
        <h1>Step inside the<br /><em>order pipeline.</em></h1>
        <p>Use Member to place and track orders. Use Admin to inspect inventory, payments, activity and analytics.</p>
      </section>
      <form className="auth-card" onSubmit={async event => {
        event.preventDefault()
        setBusy(true)
        setError('')
        try {
          await login(email, password)
          navigate('/app')
        } catch (cause) {
          setError(cause instanceof Error ? cause.message : 'Sign in failed')
        } finally {
          setBusy(false)
        }
      }}>
        <p className="eyebrow">Identity service</p><h2>Open a session</h2>
        <label>Email<input aria-label="Email" type="email" value={email} onChange={event => setEmail(event.target.value)} required /></label>
        <label>Password<input aria-label="Password" type="password" value={password} onChange={event => setPassword(event.target.value)} required /></label>
        {error && <State kind="error" text={error} />}
        <button className="button" disabled={busy}>{busy ? 'Opening session…' : 'Sign in'} <span>→</span></button>
        <div className="credential-grid"><button type="button" onClick={() => { setEmail('member@example.com'); setPassword('member123') }}>Member demo</button><button type="button" onClick={() => { setEmail('admin@example.com'); setPassword('admin123') }}>Admin demo</button></div>
        <p className="hint">Credentials are deterministic local fixtures. No payment card data is accepted anywhere in this project.</p>
      </form>
    </div>
  )
}

export function Overview() {
  const { user } = useAuth()
  const orders = useQuery({ queryKey: ['orders'], queryFn: api.orders, refetchInterval: 5_000 })
  const products = useQuery({ queryKey: ['products'], queryFn: api.products })
  const unread = useQuery({ queryKey: ['notifications', 'unread'], queryFn: api.unreadNotifications, refetchInterval: 5_000 })
  const recent = orders.data?.items.slice(0, 3) ?? []
  const active = orders.data?.items.filter(order => !terminalStates.has(order.status)).length ?? 0

  return (
    <>
      <PageHeader eyebrow="Order operations / dashboard" title={user?.role === 'admin' ? 'System-wide order desk' : 'Your order desk'}>
        <Link className="button" to="/app/products">Create an order →</Link>
      </PageHeader>
      <section className="metric-strip">
        <article><span>VISIBLE ORDERS</span><strong>{orders.data?.total ?? '—'}</strong><small>{user?.role === 'admin' ? 'across all customers' : 'owned by this account'}</small></article>
        <article><span>IN FLIGHT</span><strong>{active}</strong><small>eventual workflow</small></article>
        <article><span>CATALOG ITEMS</span><strong>{products.data?.total ?? '—'}</strong><small>Inventory-owned</small></article>
        <article><span>UNREAD SIGNALS</span><strong>{unread.data?.count ?? '—'}</strong><small>Notification projection</small></article>
      </section>
      <section className="dashboard-grid">
        <div className="panel panel-wide">
          <div className="panel-head"><div><p className="eyebrow">Recent traffic</p><h2>Order journey</h2></div><Link to={user?.role === 'admin' ? '/app/admin/orders' : '/app/orders'}>View all →</Link></div>
          {orders.isPending && <State kind="loading" text="Reading the Order service…" />}
          {orders.isError && <ErrorMessage error={orders.error} />}
          {!orders.isPending && recent.length === 0 && <SectionEmpty title="No orders on the rail" copy="Choose a product and send the first order into the workflow." action={<Link className="button button-quiet" to="/app/products">Open catalog</Link>} />}
          <div className="order-list compact-list">{recent.map(order => <OrderRow order={order} key={order.id} />)}</div>
        </div>
        <aside className="panel architecture-panel">
          <p className="eyebrow">What happens next</p><h2>Sync ends at PENDING.</h2>
          <ol><li><b>REST</b><span>Gateway validates the JWT and request.</span></li><li><b>gRPC</b><span>Order quotes authoritative product prices.</span></li><li><b>DB + Outbox</b><span>The aggregate and event commit atomically.</span></li><li><b>Kafka</b><span>Inventory and Payment continue independently.</span></li></ol>
        </aside>
      </section>
    </>
  )
}

export function Products() {
  const navigate = useNavigate()
  const [selected, setSelected] = useState<Record<string, number>>({})
  const [scenario, setScenario] = useState<'success' | 'decline'>('success')
  const query = useQuery({ queryKey: ['products'], queryFn: api.products })
  const chosen = Object.entries(selected).filter(([, quantity]) => quantity > 0)
  const previewTotal = useMemo(() => chosen.reduce((sum, [id, quantity]) => {
    const product = query.data?.products.find(item => item.id === id)
    return sum + (product?.unit_price_minor ?? 0) * quantity
  }, 0), [chosen, query.data?.products])
  const create = useMutation({
    mutationFn: () => api.createOrder(chosen.map(([product_id, quantity]) => ({ product_id, quantity })), scenario),
    onSuccess: order => navigate(`/app/orders/${order.id}`),
  })

  const change = (product: Product, delta: number) => {
    setSelected(current => ({ ...current, [product.id]: Math.max(0, Math.min(10, (current[product.id] ?? 0) + delta)) }))
  }

  return (
    <>
      <PageHeader eyebrow="Order operations / catalog" title="Build an order"><span className="eventual">Prices quoted synchronously · stock decided asynchronously</span></PageHeader>
      <div className="catalog-layout">
        <section>
          {query.isPending && <State kind="loading" text="Loading the Inventory catalog…" />}
          {query.isError && <ErrorMessage error={query.error} />}
          <div className="product-grid">{query.data?.products.map(product => {
            const quantity = selected[product.id] ?? 0
            return <article className={`product-card ${product.available === 0 ? 'product-risk' : ''}`} key={product.id} data-product-id={product.id}>
              <div className="product-ticket"><span>{product.id.replace('prod-', '').toUpperCase()}</span><small>{product.available > 0 ? `${product.available} available` : 'failure fixture · 0 available'}</small></div>
              <h2>{product.name}</h2><p className="product-price"><Money value={{ amount_minor: product.unit_price_minor, currency: product.currency }} /></p>
              <p>{product.available > 0 ? 'Ready for asynchronous reservation.' : 'Select one to demonstrate InventoryRejected after the Order is accepted.'}</p>
              <div className="stepper"><button aria-label={`Decrease ${product.name}`} onClick={() => change(product, -1)} disabled={quantity === 0}>−</button><output aria-label={`Quantity for ${product.name}`}>{quantity}</output><button aria-label={`Increase ${product.name}`} onClick={() => change(product, 1)}>+</button></div>
            </article>
          })}</div>
        </section>
        <aside className="order-builder">
          <p className="eyebrow">Dispatch manifest</p><h2>Order preview</h2>
          {chosen.length === 0 ? <p className="muted">Choose at least one product. The backend will recalculate authoritative prices before creating the order.</p> : <ul>{chosen.map(([id, quantity]) => { const product = query.data?.products.find(item => item.id === id); return <li key={id}><span>{product?.name} × {quantity}</span><b><Money value={{ amount_minor: (product?.unit_price_minor ?? 0) * quantity, currency: product?.currency ?? 'USD' }} /></b></li> })}</ul>}
          <div className="builder-total"><span>Preview total</span><strong><Money value={{ amount_minor: previewTotal, currency: 'USD' }} /></strong></div>
          <fieldset><legend>Payment simulator</legend><label className={scenario === 'success' ? 'selected-option' : ''}><input type="radio" name="scenario" value="success" checked={scenario === 'success'} onChange={() => setScenario('success')} /><span><b>Approve</b><small>PaymentCompleted</small></span></label><label className={scenario === 'decline' ? 'selected-option' : ''}><input type="radio" name="scenario" value="decline" checked={scenario === 'decline'} onChange={() => setScenario('decline')} /><span><b>Decline</b><small>PaymentFailed + stock release</small></span></label></fieldset>
          <button className="button button-full" disabled={create.isPending || chosen.length === 0} onClick={() => create.mutate()}>{create.isPending ? 'Committing order…' : 'Send into workflow →'}</button>
          {create.isError && <ErrorMessage error={create.error} />}
          <p className="builder-note">The response stops at <code>PENDING</code>. Inventory and Payment continue through Kafka.</p>
        </aside>
      </div>
    </>
  )
}

function OrderRow({ order }: { order: Order }) {
  return (
    <article className="order-row">
      <div className="order-identity"><span>ORDER</span><ShortID value={order.id} /><small>{dateTime(order.created_at)}</small></div>
      <div className="order-route"><OrderRail status={order.status} compact /><span>{order.items.length} line{order.items.length === 1 ? '' : 's'} · <Money value={order.total} /></span></div>
      <StatusBadge status={order.status} />
      <Link className="row-link" to={`/app/orders/${order.id}`}>Inspect →</Link>
    </article>
  )
}

export function Orders({ adminView = false }: { adminView?: boolean }) {
  const [filter, setFilter] = useState('ALL')
  const query = useQuery({ queryKey: ['orders'], queryFn: api.orders, refetchInterval: 5_000 })
  const items = query.data?.items.filter(order => filter === 'ALL' || order.status === filter) ?? []

  return (
    <>
      <PageHeader eyebrow={adminView ? 'Operator / all orders' : 'Order operations / my orders'} title={adminView ? 'All customer orders' : 'My orders'}><span className="eventual">Polling current state every 5 seconds</span></PageHeader>
      <div className="filter-bar" aria-label="Order status filters">{['ALL', 'PENDING', 'CONFIRMED', 'CANCELLED', 'REJECTED'].map(status => <button className={filter === status ? 'active' : ''} key={status} onClick={() => setFilter(status)}>{status.replace('_', ' ')}</button>)}</div>
      {query.isPending && <State kind="loading" text="Reading Order state…" />}
      {query.isError && <ErrorMessage error={query.error} />}
      {!query.isPending && items.length === 0 && <SectionEmpty title="No matching orders" copy={filter === 'ALL' ? 'Create an order from the catalog to start the workflow.' : `No order is currently ${filter.toLowerCase()}.`} action={!adminView && filter === 'ALL' ? <Link className="button button-quiet" to="/app/products">Open catalog</Link> : undefined} />}
      <div className="order-list">{items.map(order => <OrderRow order={order} key={order.id} />)}</div>
    </>
  )
}

export function OrderDetail({ id }: { id: string }) {
  const query = useQuery({ queryKey: ['order', id], queryFn: () => api.order(id), refetchInterval: 2_000 })
  const timeline = useQuery({ queryKey: ['order-activities', id], queryFn: () => api.orderActivities(id), refetchInterval: 2_000 })
  const order = query.data

  return (
    <>
      <PageHeader eyebrow="Order operations / live detail" title={order ? `Order ${order.id.slice(0, 8).toUpperCase()}` : 'Loading order'}>{order && <StatusBadge status={order.status} />}</PageHeader>
      {query.isPending && <State kind="loading" text="Reading the transactional Order record…" />}
      {query.isError && <ErrorMessage error={query.error} />}
      {order && <>
        <section className="journey-panel">
          <div className="journey-copy"><p className="eyebrow">Current state</p><h2>{order.status.replace(/_/g, ' ')}</h2><p>{order.reason ? `Workflow reason: ${reasonLabel(order.reason)}.` : terminalStates.has(order.status) ? 'The workflow reached a terminal state.' : 'Downstream services are continuing asynchronously. This page refreshes from the backend.'}</p></div>
          <OrderRail status={order.status} />
        </section>
        <div className="detail-grid">
          <section className="panel">
            <div className="panel-head"><div><p className="eyebrow">Manifest</p><h2>Items and total</h2></div><strong className="large-money"><Money value={order.total} /></strong></div>
            <div className="manifest-table">{order.items.map(item => <div key={item.product_id}><span><b>{item.name}</b><small>{item.product_id}</small></span><span>{item.quantity} × <Money value={item.unit_price} /></span><strong><Money value={item.line_total} /></strong></div>)}</div>
            <dl className="order-meta"><div><dt>Created</dt><dd>{dateTime(order.created_at)}</dd></div><div><dt>Payment fixture</dt><dd>{order.payment_scenario}</dd></div><div><dt>Customer</dt><dd><ShortID value={order.customer_id} /></dd></div></dl>
          </section>
          <section className="panel">
            <div className="panel-head"><div><p className="eyebrow">Kafka projection</p><h2>Activity timeline</h2></div><span className="eventual">eventual</span></div>
            {timeline.isPending && <State kind="loading" text="Waiting for projected events…" />}
            {timeline.isError && <ErrorMessage error={timeline.error} />}
            {!timeline.isPending && timeline.data?.length === 0 && <State kind="empty" text="The consumer has not projected an event yet." />}
            <ol className="event-timeline">{timeline.data?.map((event, index) => <li key={event.id}><span>{index + 1}</span><div><strong>{eventLabel(event.event_type)}</strong><small>{dateTime(event.occurred_at)}</small></div></li>)}</ol>
          </section>
        </div>
      </>}
    </>
  )
}

export function Notifications() {
  const client = useQueryClient()
  const query = useQuery({ queryKey: ['notifications'], queryFn: api.notifications, refetchInterval: 5_000 })
  const invalidate = async () => {
    await Promise.all([
      client.invalidateQueries({ queryKey: ['notifications'] }),
      client.invalidateQueries({ queryKey: ['notifications', 'unread'] }),
    ])
  }
  const read = useMutation({ mutationFn: api.markNotificationRead, onSuccess: invalidate })
  const readAll = useMutation({ mutationFn: api.markAllNotificationsRead, onSuccess: invalidate })
  const unread = query.data?.items.filter(item => !item.read).length ?? 0

  return (
    <>
      <PageHeader eyebrow="Order operations / inbox" title="Notifications"><button className="button button-quiet" disabled={unread === 0 || readAll.isPending} onClick={() => readAll.mutate()}>{readAll.isPending ? 'Updating…' : 'Mark all read'}</button></PageHeader>
      <p className="page-intro">This inbox is a persisted Kafka projection. It can arrive after the Order response without blocking the core workflow.</p>
      {query.isPending && <State kind="loading" text="Reading Notification state…" />}
      {query.isError && <ErrorMessage error={query.error} />}
      {!query.isPending && query.data?.items.length === 0 && <SectionEmpty title="Inbox clear" copy="Order state changes will appear here after the Notification consumer processes their events." />}
      <div className="notification-list">{query.data?.items.map(notification => <article className={notification.read ? 'notification-read' : ''} key={notification.id}><span className="notification-signal" /><div><p>{notification.type.replace(/_/g, ' ')}</p><h2>{notification.message}</h2><small>Order <ShortID value={notification.order_id} /> · {dateTime(notification.created_at)}</small></div><button disabled={notification.read || read.isPending} onClick={() => read.mutate(notification.id)}>{notification.read ? 'Read' : 'Mark read'}</button></article>)}</div>
    </>
  )
}

export function AdminInventory() {
  const products = useQuery({ queryKey: ['products'], queryFn: api.products })
  const reservations = useQuery({ queryKey: ['admin', 'reservations'], queryFn: api.reservations, refetchInterval: 5_000 })
  return <><PageHeader eyebrow="Operator / inventory" title="Stock and reservations"><span className="eventual">Inventory service owns this state</span></PageHeader><section className="metric-strip compact-metrics">{products.data?.products.map(product => <article key={product.id}><span>{product.name}</span><strong>{product.available}</strong><small>available units</small></article>)}</section><section className="panel table-panel"><div className="panel-head"><div><p className="eyebrow">Reservation ledger</p><h2>{reservations.data?.total ?? 0} records</h2></div></div>{reservations.isPending && <State kind="loading" text="Reading reservations…" />}{reservations.isError && <ErrorMessage error={reservations.error} />}<div className="data-table"><div className="table-row table-head"><span>Order</span><span>Product</span><span>Qty</span><span>Status</span><span>Created</span></div>{reservations.data?.reservations.map(item => <div className="table-row" key={item.id}><span><ShortID value={item.order_id} /></span><span>{item.product_id}</span><span>{item.quantity}</span><span><StatusBadge status={item.status} /></span><span>{dateTime(item.created_at)}</span></div>)}</div>{!reservations.isPending && reservations.data?.reservations.length === 0 && <SectionEmpty title="No reservations yet" copy="Create an order to watch Inventory reserve or reject stock." />}</section></>
}

export function AdminPayments() {
  const query = useQuery({ queryKey: ['admin', 'payments'], queryFn: api.payments, refetchInterval: 5_000 })
  return <><PageHeader eyebrow="Operator / payments" title="Payment outcomes"><span className="eventual">Deterministic simulator · no card data</span></PageHeader><section className="panel table-panel">{query.isPending && <State kind="loading" text="Reading Payment state…" />}{query.isError && <ErrorMessage error={query.error} />}<div className="data-table"><div className="table-row table-head"><span>Order</span><span>Amount</span><span>Status</span><span>Reason</span><span>Created</span></div>{query.data?.payments.map(payment => <div className="table-row" key={payment.id}><span><ShortID value={payment.order_id} /></span><span><Money value={{ amount_minor: payment.amount_minor, currency: payment.currency }} /></span><span><StatusBadge status={payment.status} /></span><span>{reasonLabel(payment.reason) || '—'}</span><span>{dateTime(payment.created_at)}</span></div>)}</div>{!query.isPending && query.data?.payments.length === 0 && <SectionEmpty title="No payment attempts" copy="Payment begins only after InventoryReserved is consumed from Kafka." />}</section></>
}

export function AdminActivities() {
  const query = useQuery({ queryKey: ['admin', 'order-activities'], queryFn: api.adminOrderActivities, refetchInterval: 5_000 })
  return <><PageHeader eyebrow="Operator / audit" title="Order activity"><span className="eventual">Immutable event projection</span></PageHeader>{query.isPending && <State kind="loading" text="Following the order event stream…" />}{query.isError && <ErrorMessage error={query.error} />}{!query.isPending && query.data?.items.length === 0 && <SectionEmpty title="No order events projected" copy="Create an order to populate the audit trail." />}<div className="activity-stream">{query.data?.items.map(event => <article key={event.id}><span className="activity-sequence" /><div><strong>{eventLabel(event.event_type)}</strong><small>Order <ShortID value={event.order_id} /></small></div><time>{dateTime(event.occurred_at)}</time><Link to={`/app/orders/${event.order_id}`}>Inspect →</Link></article>)}</div></>
}

export function OrderAnalytics() {
  const summary = useQuery({ queryKey: ['admin', 'order-summary'], queryFn: api.orderSummary, refetchInterval: 10_000 })
  const funnel = useQuery({ queryKey: ['admin', 'order-funnel'], queryFn: api.orderFunnel, refetchInterval: 10_000 })
  const data = summary.data
  const stages = funnel.data ? [
    ['Created', funnel.data.created ?? 0], ['Reserved', funnel.data.reserved ?? 0], ['Paid', funnel.data.paid ?? 0], ['Confirmed', funnel.data.confirmed ?? 0],
  ] as const : []
  const maximum = Math.max(1, funnel.data?.created ?? 0)

  return <><PageHeader eyebrow="Operator / business analytics" title="Order funnel"><span className="eventual">ClickHouse projection · through {dateTime(data?.through)}</span></PageHeader>{(summary.isPending || funnel.isPending) && <State kind="loading" text="Querying ClickHouse projection…" />}{(summary.isError || funnel.isError) && <ErrorMessage error={summary.error || funnel.error} />}<section className="metric-strip"><article><span>CREATED</span><strong>{data?.created ?? 0}</strong><small>accepted orders</small></article><article><span>CONFIRMED</span><strong>{data?.confirmed ?? 0}</strong><small>completed workflow</small></article><article><span>CANCELLED</span><strong>{data?.cancelled ?? 0}</strong><small>payment compensation</small></article><article><span>REJECTED</span><strong>{data?.rejected ?? 0}</strong><small>inventory outcome</small></article></section><div className="analytics-layout"><section className="panel"><div className="panel-head"><div><p className="eyebrow">Conversion path</p><h2>Event funnel</h2></div></div><div className="funnel">{stages.map(([label, count]) => <div key={label}><span>{label}</span><div><i style={{ width: `${Math.max(4, count / maximum * 100)}%` }} /></div><strong>{count}</strong></div>)}</div></section><section className="revenue-card"><span>CONFIRMED REVENUE</span><strong><Money value={{ amount_minor: data?.revenue_minor ?? 0, currency: data?.currency || 'USD' }} /></strong><p>Calculated from analytical events, not the transactional Order table.</p><a href="http://localhost:3001" target="_blank" rel="noreferrer">Open Grafana business dashboard ↗</a></section></div></>
}

export function System() {
  const query = useQuery({ queryKey: ['system', 'gateway'], queryFn: api.gatewayHealth, retry: 0 })
  const services = ['Identity', 'Order', 'Inventory', 'Payment', 'Activity', 'Notification', 'Analytics']
  return <><PageHeader eyebrow="Operator / system" title="Platform signals" /><p className="page-intro">The browser can probe only the public Gateway. Prometheus observes private gRPC services and their metrics inside the Compose network.</p><section className="system-grid"><article className="gateway-health"><span className={`health-light ${query.data?.status === 'ready' ? 'health-up' : ''}`} /><div><small>PUBLIC EDGE</small><h2>API Gateway</h2><p>{query.isPending ? 'checking' : query.isError ? 'unreachable' : query.data?.status}</p></div><code>/health/ready</code></article><article><small>PRIVATE SERVICE MAP</small><div className="service-cloud">{services.map(service => <span key={service}>{service}</span>)}</div></article><a className="system-link" href="http://localhost:9090" target="_blank" rel="noreferrer"><span>PROMETHEUS</span><strong>Inspect scrape targets</strong><small>Operational metrics →</small></a><a className="system-link" href="http://localhost:3001" target="_blank" rel="noreferrer"><span>GRAFANA</span><strong>Open provisioned dashboards</strong><small>Platform + business views →</small></a></section></>
}

export function LegacyTasks() {
  const client = useQueryClient()
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const query = useQuery({ queryKey: ['legacy', 'tasks'], queryFn: api.tasks })
  const refresh = () => client.invalidateQueries({ queryKey: ['legacy', 'tasks'] })
  const create = useMutation({ mutationFn: () => api.createTask({ title, description }), onSuccess: () => { setTitle(''); setDescription(''); void refresh() } })
  const update = useMutation({ mutationFn: ({ id, task }: { id: string; task: { title: string; description: string; status: 'todo' | 'doing' | 'done' } }) => api.updateTask(id, task), onSuccess: refresh })
  const remove = useMutation({ mutationFn: api.deleteTask, onSuccess: refresh })
  return <><PageHeader eyebrow="Legacy lab / v0.1 baseline" title="Task compatibility"><span className="legacy-flag">Not part of Order portfolio navigation</span></PageHeader><p className="page-intro">This additive route keeps the original Task CRUD available for study and rollback evidence. The active portfolio use case is Order Management.</p><div className="legacy-layout"><form onSubmit={event => { event.preventDefault(); if (title.trim()) create.mutate() }}><label>Title<input value={title} onChange={event => setTitle(event.target.value)} /></label><label>Description<textarea rows={4} value={description} onChange={event => setDescription(event.target.value)} /></label><button className="button" disabled={!title.trim()}>Create legacy task</button></form><div>{query.data?.items.map(task => <article className="legacy-task" key={task.id}><div><strong>{task.title}</strong><p>{task.description || 'No description'}</p></div><select aria-label={`Status for ${task.title}`} value={task.status} onChange={event => update.mutate({ id: task.id, task: { title: task.title, description: task.description, status: event.target.value as 'todo' | 'doing' | 'done' } })}><option value="todo">todo</option><option value="doing">doing</option><option value="done">done</option></select><button aria-label={`Delete ${task.title}`} onClick={() => remove.mutate(task.id)}>×</button></article>)}</div></div></>
}
