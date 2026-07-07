import { test, expect, type Page } from '@playwright/test'

// ─── Fixtures ─────────────────────────────────────────────────────────────────

const MONTHS = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]

const mockSchedule = {
  '2026-01-01': [
    { name: 'Kitchen Trash', person: 'Alison' },
    { name: 'Bathroom Trash', person: 'Kevina' },
  ],
  '2026-01-08': [
    { name: 'Kitchen Trash', person: 'Alison' },
  ],
  '2026-02-01': [
    { name: 'Bathroom Floor', person: 'Alison' },
    { name: 'Toilets', person: 'Kevina' },
  ],
}

const mockGroup = { id: 1, name: 'home', members: ['Alison', 'Kevina'] }
const mockMe = { username: 'Alison' }

// Mount all mocks before navigating. A single route handler dispatches by URL.
async function loadWithMock(page: Page, schedule = mockSchedule) {
  await page.route('**/api/**', route => {
    const url = route.request().url()
    if (url.includes('/api/me')) {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(mockMe) })
    }
    if (/\/api\/groups\/\d+\/schedule/.test(url)) {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(schedule) })
    }
    if (/\/api\/groups\/\d+\/chores/.test(url)) {
      return route.fulfill({ contentType: 'application/json', body: '[]' })
    }
    if (url.includes('/api/groups')) {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify([mockGroup]) })
    }
    return route.fulfill({ contentType: 'application/json', body: '{}' })
  })
  await page.goto('/chores/')
  await expect(page.getByText('Chore calendar')).toBeVisible()
}

// ─── Loading & error states ───────────────────────────────────────────────────

test('shows loading state while fetch is pending', async ({ page }) => {
  await page.route('**/api/**', () => { /* stall forever */ })
  await page.goto('/chores/')
  await expect(page.getByText(/loading/i)).toBeVisible()
})

test('redirects to OAuth login when API returns 401', async ({ page }) => {
  await page.route('**/api/**', route =>
    route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthenticated' }) }),
  )
  await page.route('**/oauth/start**', route =>
    route.fulfill({ status: 200, contentType: 'text/html', body: '<html><body>login</body></html>' }),
  )
  await page.goto('/chores/')
  await expect(page).toHaveURL(/oauth\/start/)
})

test('shows error state when network fails', async ({ page }) => {
  await page.route('**/api/**', route => route.abort('failed'))
  await page.goto('/chores/')
  await expect(page.getByText(/failed to load/i)).toBeVisible()
})

// ─── Calendar renders correctly ───────────────────────────────────────────────

test('renders page title and subtitle', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByText('Chore calendar')).toBeVisible()
  await expect(page.getByText(/2026/)).toBeVisible()
})

test('renders all 12 month names', async ({ page }) => {
  await loadWithMock(page)
  for (const month of MONTHS) {
    await expect(page.getByText(month).first()).toBeVisible()
  }
})

test('renders person legend items', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.locator('header')).toContainText('Alison')
  await expect(page.locator('header')).toContainText('Kevina')
})

test('shows hint text', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByText(/click any day/i)).toBeVisible()
})

// ─── Group tabs ───────────────────────────────────────────────────────────────

test('renders group tab for the active group', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByRole('button', { name: 'home' })).toBeVisible()
})

test('renders create new group button', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByRole('button', { name: 'Create new group' })).toBeVisible()
})

test('renders Manage button', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByRole('button', { name: 'Manage group' })).toBeVisible()
})

test('shows no-groups welcome screen when user has no groups', async ({ page }) => {
  await page.route('**/api/**', route => {
    const url = route.request().url()
    if (url.includes('/api/me')) {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(mockMe) })
    }
    if (url.includes('/api/groups')) {
      return route.fulfill({ contentType: 'application/json', body: '[]' })
    }
    return route.fulfill({ contentType: 'application/json', body: '{}' })
  })
  await page.goto('/chores/')
  await expect(page.getByText(/welcome/i)).toBeVisible()
  await expect(page.getByRole('button', { name: 'Create group' })).toBeVisible()
})

test('opens create group dialog when + button is clicked', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Create new group' }).click()
  await expect(page.getByRole('dialog', { name: 'Create group' })).toBeVisible()
})

test('closes create group dialog when Cancel is clicked', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Create new group' }).click()
  await expect(page.getByRole('dialog', { name: 'Create group' })).toBeVisible()
  await page.getByRole('button', { name: 'Cancel' }).click()
  await expect(page.getByRole('dialog', { name: 'Create group' })).not.toBeVisible()
})

test('renders multiple group tabs when user is in multiple groups', async ({ page }) => {
  const group2 = { id: 2, name: 'office', members: ['Alison'] }
  await page.route('**/api/**', route => {
    const url = route.request().url()
    if (url.includes('/api/me')) {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(mockMe) })
    }
    if (/\/api\/groups\/\d+\/schedule/.test(url)) {
      return route.fulfill({ contentType: 'application/json', body: '{}' })
    }
    if (/\/api\/groups\/\d+\/chores/.test(url)) {
      return route.fulfill({ contentType: 'application/json', body: '[]' })
    }
    if (url.includes('/api/groups')) {
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify([mockGroup, group2]) })
    }
    return route.fulfill({ contentType: 'application/json', body: '{}' })
  })
  await page.goto('/chores/')
  await expect(page.getByText('Chore calendar')).toBeVisible()
  await expect(page.getByRole('button', { name: 'home' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'office' })).toBeVisible()
})

// ─── Manage panel ─────────────────────────────────────────────────────────────

test('opens manage panel when Manage button is clicked', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Manage group' }).click()
  await expect(page.getByRole('dialog', { name: 'Manage group' })).toBeVisible()
})

test('manage panel shows group members', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Manage group' }).click()
  const dialog = page.getByRole('dialog', { name: 'Manage group' })
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Leave group' })).toBeVisible()
  await expect(dialog.getByRole('button', { name: 'Remove Kevina' })).toBeVisible()
})

test('closes manage panel with close button', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Manage group' }).click()
  await expect(page.getByRole('dialog', { name: 'Manage group' })).toBeVisible()
  await page.getByRole('button', { name: 'Close manage panel' }).click()
  await expect(page.getByRole('dialog', { name: 'Manage group' })).not.toBeVisible()
})

test('Escape key closes manage panel', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Manage group' }).click()
  await expect(page.getByRole('dialog', { name: 'Manage group' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Manage group' })).not.toBeVisible()
})

test('Escape on manage panel does not close open detail panel', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()

  await page.getByRole('button', { name: 'Manage group' }).click()
  await expect(page.getByRole('dialog', { name: 'Manage group' })).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog', { name: 'Manage group' })).not.toBeVisible()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()
})

// ─── Detail panel ─────────────────────────────────────────────────────────────

test('clicking a day with chores opens detail panel', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText('Kitchen Trash')).toBeVisible()
  await expect(page.getByText('Bathroom Trash')).toBeVisible()
})

test('clicking a day with no chores shows no-chores message', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('2').first().click()

  await expect(page.getByText(/no chores scheduled/i)).toBeVisible()
})

test('clicking the same day again closes the detail panel', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  const dayOne = januaryCard.getByText('1').first()

  await dayOne.click()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()

  await dayOne.click()
  await expect(page.getByText('Kitchen Trash')).not.toBeVisible()
})

test('clicking a different day updates the detail panel', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()

  await januaryCard.getByText('1').first().click()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()
  await expect(page.getByText('Bathroom Trash')).toBeVisible()

  await januaryCard.getByText('8').first().click()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()
  await expect(page.getByText('Bathroom Trash')).not.toBeVisible()
})

// ─── Filter buttons ───────────────────────────────────────────────────────────

test('All filter is active by default', async ({ page }) => {
  await loadWithMock(page)
  const allBtn = page.getByRole('button', { name: 'All' })
  await expect(allBtn).toHaveClass(/active-all/)
})

test('Alison filter shows only Alison chores in detail panel', async ({ page }) => {
  await loadWithMock(page)

  await page.getByRole('button', { name: 'Alison' }).click()

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText('Kitchen Trash')).toBeVisible()
  await expect(page.getByText('Bathroom Trash')).not.toBeVisible()
})

test('Kevina filter shows only Kevina chores in detail panel', async ({ page }) => {
  await loadWithMock(page)

  await page.getByRole('button', { name: 'Kevina' }).click()

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText('Bathroom Trash')).toBeVisible()
  await expect(page.getByText('Kitchen Trash')).not.toBeVisible()
})

test('switching back to All restores both people', async ({ page }) => {
  await loadWithMock(page)

  await page.getByRole('button', { name: 'Alison' }).click()
  await page.getByRole('button', { name: 'All' }).click()

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText('Kitchen Trash')).toBeVisible()
  await expect(page.getByText('Bathroom Trash')).toBeVisible()
})

test('Alison filter button gets active class', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Alison' }).click()
  await expect(page.getByRole('button', { name: 'Alison' })).toHaveClass(/active-person-0/)
})

test('Kevina filter button gets active class', async ({ page }) => {
  await loadWithMock(page)
  await page.getByRole('button', { name: 'Kevina' }).click()
  await expect(page.getByRole('button', { name: 'Kevina' })).toHaveClass(/active-person-1/)
})

// ─── Chore count in detail panel ─────────────────────────────────────────────

test('detail panel shows correct chore count', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText('2 chores')).toBeVisible()
})

test('detail panel shows singular "chore" for one item', async ({ page }) => {
  await loadWithMock(page)

  await page.getByRole('button', { name: 'Alison' }).click()

  const februaryCard = page.locator('div').filter({ hasText: /^February/ }).first()
  await februaryCard.getByText('1').first().click()

  await expect(page.getByText('1 chore')).toBeVisible()
  await expect(page.getByText('1 chores')).not.toBeVisible()
})

// ─── Close / dismiss ──────────────────────────────────────────────────────────

test('close button dismisses the detail panel', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()

  await page.getByRole('button', { name: 'Close detail panel' }).click()
  await expect(page.getByText('Kitchen Trash')).not.toBeVisible()
})

test('Escape key closes the detail panel', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()
  await expect(page.getByText('Kitchen Trash')).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(page.getByText('Kitchen Trash')).not.toBeVisible()
})

// ─── Hint text ────────────────────────────────────────────────────────────────

test('hint text disappears after first day click', async ({ page }) => {
  await loadWithMock(page)

  await expect(page.getByText(/click any day/i)).toBeVisible()

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText(/click any day/i)).not.toBeVisible()
})

// ─── Filter label in detail count ────────────────────────────────────────────

test('detail count shows active filter name when filtered', async ({ page }) => {
  await loadWithMock(page)

  await page.getByRole('button', { name: 'Alison' }).click()

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText(/chore · Alison/)).toBeVisible()
})

// ─── Today button ─────────────────────────────────────────────────────────────

test('Today button is visible in the header', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByRole('button', { name: 'Today' })).toBeVisible()
})

// ─── Keyboard accessibility ───────────────────────────────────────────────────

test('Enter key on a day cell opens the detail panel', async ({ page }) => {
  await loadWithMock(page)

  const dayCell = page.getByRole('button', { name: 'January 1, 2 chores' })
  await dayCell.focus()
  await page.keyboard.press('Enter')

  await expect(page.getByText('Kitchen Trash')).toBeVisible()
})

test('Space key on a day cell opens the detail panel', async ({ page }) => {
  await loadWithMock(page)

  const dayCell = page.getByRole('button', { name: 'January 1, 2 chores' })
  await dayCell.focus()
  await page.keyboard.press('Space')

  await expect(page.getByText('Kitchen Trash')).toBeVisible()
})

// ─── Dark mode toggle ─────────────────────────────────────────────────────────

test('dark mode toggle button is visible in the header', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByRole('button', { name: 'Toggle dark mode' })).toBeVisible()
})

test('clicking dark mode toggle changes the button icon', async ({ page }) => {
  await loadWithMock(page)

  const toggleBtn = page.getByRole('button', { name: 'Toggle dark mode' })
  const initialText = await toggleBtn.textContent()
  await toggleBtn.click()
  await expect(toggleBtn).not.toHaveText(initialText ?? '')
})

// ─── Log out ──────────────────────────────────────────────────────────────────

test('Log out button is visible in the header', async ({ page }) => {
  await loadWithMock(page)
  await expect(page.getByRole('button', { name: 'Log out' })).toBeVisible()
})

// ─── aria-pressed ─────────────────────────────────────────────────────────────

test('selected day cell has aria-pressed=true', async ({ page }) => {
  await loadWithMock(page)

  const dayCell = page.getByRole('button', { name: 'January 1, 2 chores' })
  await expect(dayCell).toHaveAttribute('aria-pressed', 'false')

  await dayCell.click()
  await expect(dayCell).toHaveAttribute('aria-pressed', 'true')
})

test('closing the panel resets aria-pressed to false', async ({ page }) => {
  await loadWithMock(page)

  const dayCell = page.getByRole('button', { name: 'January 1, 2 chores' })
  await dayCell.click()
  await expect(dayCell).toHaveAttribute('aria-pressed', 'true')

  await page.getByRole('button', { name: 'Close detail panel' }).click()
  await expect(dayCell).toHaveAttribute('aria-pressed', 'false')
})

// ─── Detail panel date label ──────────────────────────────────────────────────

test('detail panel shows full formatted date label', async ({ page }) => {
  await loadWithMock(page)

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText('Thursday, January 1, 2026')).toBeVisible()
})

// ─── Filter count label ───────────────────────────────────────────────────────

test('filter count label clears person name when switching back to All', async ({ page }) => {
  await loadWithMock(page)

  await page.getByRole('button', { name: 'Alison' }).click()

  const januaryCard = page.locator('div').filter({ hasText: /^January/ }).first()
  await januaryCard.getByText('1').first().click()

  await expect(page.getByText(/chore · Alison/)).toBeVisible()

  await page.getByRole('button', { name: 'All' }).click()
  await expect(page.getByText(/chore · Alison/)).not.toBeVisible()
})

// ─── February day count ───────────────────────────────────────────────────────

test('February 2026 grid has exactly 28 day buttons', async ({ page }) => {
  await loadWithMock(page)

  const februaryCard = page.locator('div').filter({ hasText: /^February/ }).first()
  const dayButtons = februaryCard.getByRole('button')
  await expect(dayButtons).toHaveCount(28)
})

// ─── Chore dot indicators ─────────────────────────────────────────────────────

test('days with chores show dot indicators, days without do not', async ({ page }) => {
  await loadWithMock(page)

  const jan1 = page.getByRole('button', { name: 'January 1, 2 chores' })
  await expect(jan1).toBeVisible()

  const jan2 = page.getByRole('button', { name: 'January 2', exact: true })
  await expect(jan2).toBeVisible()
})

// ─── Empty schedule ───────────────────────────────────────────────────────────

test('empty schedule renders all 12 months with no chore dot labels', async ({ page }) => {
  await loadWithMock(page, {})

  for (const month of MONTHS) {
    await expect(page.getByText(month).first()).toBeVisible()
  }

  const choreButtons = page.getByRole('button', { name: /chore/ })
  await expect(choreButtons).toHaveCount(0)
})
