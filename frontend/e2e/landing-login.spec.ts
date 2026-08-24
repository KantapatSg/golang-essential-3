import { test, expect } from '@playwright/test'

test('Order Relay landing explains the transport boundaries and opens login', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: /One order.*Every boundary visible/i })).toBeVisible()
  await expect(page.getByText('Fiber REST')).toBeVisible()
  await expect(page.getByText('gRPC', { exact: true })).toBeVisible()
  await expect(page.getByText('Kafka', { exact: true })).toBeVisible()
  await expect(page.getByText('ClickHouse', { exact: true })).toBeVisible()
  await page.screenshot({ path: 'test-results/order-relay-landing.png', fullPage: true })
  await page.getByRole('link', { name: /Enter console/i }).click()
  await expect(page).toHaveURL(/\/login/)
  await expect(page.getByLabel('Email')).toBeVisible()
  await expect(page.getByRole('button', { name: /Sign in/i })).toBeVisible()
})
