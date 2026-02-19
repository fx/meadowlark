import { expect, test } from '@playwright/test'

test.describe('theme toggle', () => {
  test.beforeEach(async ({ page }) => {
    // Clear theme from localStorage before each test
    await page.goto('/')
    await page.evaluate(() => localStorage.removeItem('meadowlark-theme'))
    await page.reload()
  })

  test('defaults to system preference (light)', async ({ page }) => {
    // Emulate light mode system preference
    await page.emulateMedia({ colorScheme: 'light' })
    await page.goto('/')

    const html = page.locator('html')
    await expect(html).toHaveClass(/light/)
    await expect(html).not.toHaveClass(/dark/)
  })

  test('defaults to system preference (dark)', async ({ page }) => {
    // Emulate dark mode system preference
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.goto('/')

    const html = page.locator('html')
    await expect(html).toHaveClass(/dark/)
  })

  test('theme toggle opens dropdown without errors', async ({ page }) => {
    // Listen for console errors
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto('/')

    // Click the theme toggle button
    const toggleButton = page.getByRole('button', { name: 'Toggle theme' })
    await expect(toggleButton).toBeVisible()
    await toggleButton.click()

    // Dropdown should appear with Light/Dark/System options
    const lightOption = page.getByRole('menuitem', { name: 'Light' })
    await expect(lightOption).toBeVisible()

    const darkOption = page.getByRole('menuitem', { name: 'Dark' })
    await expect(darkOption).toBeVisible()

    const systemOption = page.getByRole('menuitem', { name: 'System' })
    await expect(systemOption).toBeVisible()

    // No JS errors should have occurred (catches getBoundingClientRect etc.)
    expect(errors).toEqual([])
  })

  test('switching to dark mode applies dark class', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.emulateMedia({ colorScheme: 'light' })
    await page.goto('/')

    // Open dropdown and click Dark
    await page.getByRole('button', { name: 'Toggle theme' }).click()
    await page.getByRole('menuitem', { name: 'Dark' }).click()

    // html should now have dark class
    const html = page.locator('html')
    await expect(html).toHaveClass(/dark/)

    // Verify it persisted to localStorage
    const stored = await page.evaluate(() => localStorage.getItem('meadowlark-theme'))
    expect(stored).toBe('dark')

    expect(errors).toEqual([])
  })

  test('switching to light mode applies light class', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.emulateMedia({ colorScheme: 'dark' })
    await page.goto('/')

    // Open dropdown and click Light
    await page.getByRole('button', { name: 'Toggle theme' }).click()
    await page.getByRole('menuitem', { name: 'Light' }).click()

    const html = page.locator('html')
    await expect(html).toHaveClass(/light/)
    await expect(html).not.toHaveClass(/dark/)

    const stored = await page.evaluate(() => localStorage.getItem('meadowlark-theme'))
    expect(stored).toBe('light')

    expect(errors).toEqual([])
  })

  test('theme persists across page reload', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.emulateMedia({ colorScheme: 'light' })
    await page.goto('/')

    // Switch to dark
    await page.getByRole('button', { name: 'Toggle theme' }).click()
    await page.getByRole('menuitem', { name: 'Dark' }).click()
    await expect(page.locator('html')).toHaveClass(/dark/)

    // Reload — should still be dark (no flash of light)
    await page.reload()
    await expect(page.locator('html')).toHaveClass(/dark/)

    expect(errors).toEqual([])
  })
})
