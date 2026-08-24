import { expect, test, type Page } from '@playwright/test'

declare const process: { env: Record<string, string | undefined> }

test.skip(process.env.E2E_REAL_BACKEND !== '1', 'requires the full Docker Compose backend')

async function login(page: Page, role: 'Member' | 'Admin') {
  await page.goto('/login')
  await page.getByRole('button', { name: `${role} demo` }).click()
  await page.getByRole('button', { name: 'Sign in' }).click()
  await expect(page).toHaveURL(/\/app$/)
}

async function createOrder(page: Page, productID: string, scenario: 'Approve' | 'Decline', terminal: string) {
  await page.goto('/app/products')
  const product = page.locator(`[data-product-id="${productID}"]`)
  await product.getByRole('button', { name: /Increase/ }).click()
  if (scenario === 'Decline') await page.getByText('Decline', { exact: true }).click()
  await page.getByRole('button', { name: /Send into workflow/i }).click()
  await expect(page).toHaveURL(/\/app\/orders\/[\w-]+/)
  await expect(page.getByRole('heading', { name: terminal })).toBeVisible({ timeout: 45_000 })
  await expect(page.getByRole('heading', { name: 'Activity timeline' })).toBeVisible()
}

test('member sees success, payment decline, out-of-stock and notifications through the UI', async ({ page }) => {
  await login(page, 'Member')
  await createOrder(page, 'prod-mug', 'Approve', 'CONFIRMED')
  await createOrder(page, 'prod-coffee', 'Decline', 'CANCELLED')
  await createOrder(page, 'prod-shirt', 'Approve', 'REJECTED')

  await page.goto('/app/notifications')
  await expect(page.getByRole('heading', { name: 'Notifications' })).toBeVisible()
  await expect(page.locator('.notification-list article').first()).toBeVisible({ timeout: 30_000 })
  await page.getByRole('button', { name: 'Mark all read' }).click()
})

test('admin navigation exposes every Order operations projection', async ({ page }) => {
  await login(page, 'Admin')
  for (const [label, heading] of [
    ['All orders', 'All customer orders'],
    ['Inventory', 'Stock and reservations'],
    ['Payments', 'Payment outcomes'],
    ['Activity', 'Order activity'],
    ['Analytics', 'Order funnel'],
    ['System', 'Platform signals'],
  ]) {
    await page.getByRole('link', { name: label, exact: true }).click()
    await expect(page.getByRole('heading', { name: heading })).toBeVisible()
  }
})
