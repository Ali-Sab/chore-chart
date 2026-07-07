import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { vi, describe, it, expect, beforeEach, afterEach } from 'vitest'
import App from './App'
import type { Schedule, GroupInfo } from './types'

const MONTHS = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]

const mockSchedule: Schedule = {
  '2026-01-01': [
    { name: 'Kitchen Trash', person: 'Alison' },
    { name: 'Bathroom Trash', person: 'Kevina' },
  ],
  '2026-01-08': [
    { name: 'Kitchen Trash', person: 'Alison' },
  ],
}

const mockGroup: GroupInfo = { id: 1, name: 'home', members: ['Alison', 'Kevina'] }
const mockMe = { username: 'Alison' }

// Mocks /api/groups, /api/me, and /api/groups/{id}/schedule.
function mockFetchOk(schedule: unknown, group = mockGroup, me = mockMe) {
  return vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
    const url = String(input)
    if (url.includes('api/me')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(me) } as Response)
    }
    if (url.match(/api\/groups\/\d+\/schedule/)) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(schedule) } as Response)
    }
    if (url.match(/api\/groups\/\d+\/chores/)) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) } as Response)
    }
    if (url.includes('api/groups')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([group]) } as Response)
    }
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
  })
}

function mockFetchError() {
  return vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('network error'))
}

beforeEach(() => {
  vi.restoreAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('App', () => {
  it('shows loading state initially', () => {
    vi.spyOn(globalThis, 'fetch').mockReturnValue(new Promise(() => {}))
    render(<App />)
    expect(screen.getByText(/loading/i)).toBeInTheDocument()
  })

  it('renders all 12 month names after schedule loads', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    for (const month of MONTHS) {
      await waitFor(() => expect(screen.getAllByText(month).length).toBeGreaterThan(0))
    }
  })

  it('shows error state when fetch fails', async () => {
    mockFetchError()
    render(<App />)
    await waitFor(() => expect(screen.getByText(/failed to load/i)).toBeInTheDocument())
  })

  it('renders page title', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => expect(screen.getByText('Chore calendar')).toBeInTheDocument())
  })

  it('renders subtitle with group members from /api/groups', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => expect(screen.getByText(/Alison & Kevina/)).toBeInTheDocument())
  })

  it('renders filter buttons driven by group members', async () => {
    mockFetchOk(mockSchedule, { id: 1, name: 'home', members: ['Alison', 'Kevina'] })
    render(<App />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Alison' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Kevina' })).toBeInTheDocument()
    })
  })

  it('renders group tab for active group', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'home' })).toBeInTheDocument())
  })

  it('renders create new group button', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Create new group' })).toBeInTheDocument())
  })

  it('renders Manage button', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => expect(screen.getByRole('button', { name: 'Manage group' })).toBeInTheDocument())
  })

  it('shows no-groups screen for user with no groups', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input)
      if (url.includes('api/me')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockMe) } as Response)
      }
      if (url.includes('api/groups')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) } as Response)
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
    })
    render(<App />)
    await waitFor(() => expect(screen.getByText(/Welcome/i)).toBeInTheDocument())
    expect(screen.getByRole('button', { name: 'Create group' })).toBeInTheDocument()
  })

  it('opens detail panel when a date with chores is clicked', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const allOnes = screen.getAllByText('1')
    fireEvent.click(allOnes[0])

    await waitFor(() => {
      expect(screen.getByText('Kitchen Trash')).toBeInTheDocument()
      expect(screen.getByText('Bathroom Trash')).toBeInTheDocument()
    })
  })

  it('closes detail panel when same date is clicked again', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const allOnes = screen.getAllByText('1')
    fireEvent.click(allOnes[0])
    await waitFor(() => expect(screen.getByText('Kitchen Trash')).toBeInTheDocument())

    fireEvent.click(allOnes[0])
    await waitFor(() => expect(screen.queryByText('Kitchen Trash')).not.toBeInTheDocument())
  })

  it('filter All button shows both people', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const allOnes = screen.getAllByText('1')
    fireEvent.click(allOnes[0])

    await waitFor(() => {
      expect(screen.getByText('Kitchen Trash')).toBeInTheDocument()
      expect(screen.getByText('Bathroom Trash')).toBeInTheDocument()
    })
  })

  it('Alison filter hides Kevina chores in detail panel', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Alison' }))
    fireEvent.click(screen.getAllByText('1')[0])

    await waitFor(() => {
      expect(screen.getByText('Kitchen Trash')).toBeInTheDocument()
      expect(screen.queryByText('Bathroom Trash')).not.toBeInTheDocument()
    })
  })

  it('Kevina filter hides Alison chores in detail panel', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Kevina' }))
    fireEvent.click(screen.getAllByText('1')[0])

    await waitFor(() => {
      expect(screen.getByText('Bathroom Trash')).toBeInTheDocument()
      expect(screen.queryByText('Kitchen Trash')).not.toBeInTheDocument()
    })
  })

  it('shows no-chores message for date with no scheduled chores', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const allTwos = screen.getAllByText('2')
    fireEvent.click(allTwos[0])

    await waitFor(() => {
      expect(screen.getByText(/no chores scheduled/i)).toBeInTheDocument()
    })
  })

  it('close button dismisses the detail panel', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getAllByText('1')[0])
    await waitFor(() => expect(screen.getByText('Kitchen Trash')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Close detail panel' }))
    await waitFor(() => expect(screen.queryByText('Kitchen Trash')).not.toBeInTheDocument())
  })

  it('Escape key closes the detail panel', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getAllByText('1')[0])
    await waitFor(() => expect(screen.getByText('Kitchen Trash')).toBeInTheDocument())

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await waitFor(() => expect(screen.queryByText('Kitchen Trash')).not.toBeInTheDocument())
  })

  it('hint text hides after first day click', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    expect(screen.getByText(/click any day/i)).toBeInTheDocument()

    fireEvent.click(screen.getAllByText('1')[0])
    await waitFor(() => expect(screen.queryByText(/click any day/i)).not.toBeInTheDocument())
  })

  it('shows filter label in detail count when filtered', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Alison' }))
    fireEvent.click(screen.getAllByText('1')[0])

    await waitFor(() => expect(screen.getByText(/chore · Alison/)).toBeInTheDocument())
  })

  it('Enter key on a day cell opens the detail panel', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const dayCell = screen.getByRole('button', { name: 'January 1, 2 chores' })
    fireEvent.keyDown(dayCell, { key: 'Enter' })

    await waitFor(() => expect(screen.getByText('Kitchen Trash')).toBeInTheDocument())
  })

  it('Space key on a day cell opens the detail panel', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const dayCell = screen.getByRole('button', { name: 'January 1, 2 chores' })
    fireEvent.keyDown(dayCell, { key: ' ' })

    await waitFor(() => expect(screen.getByText('Kitchen Trash')).toBeInTheDocument())
  })

  it('dark mode toggle button is visible', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))
    expect(screen.getByRole('button', { name: 'Toggle dark mode' })).toBeInTheDocument()
  })

  it('clicking dark mode toggle changes the button icon', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const toggleBtn = screen.getByRole('button', { name: 'Toggle dark mode' })
    const initialIcon = toggleBtn.textContent
    fireEvent.click(toggleBtn)
    expect(toggleBtn.textContent).not.toBe(initialIcon)
  })

  it('Log out button is visible', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))
    expect(screen.getByRole('button', { name: 'Log out' })).toBeInTheDocument()
  })

  it('selected day cell has aria-pressed=true, unselected has aria-pressed=false', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const dayCell = screen.getByRole('button', { name: 'January 1, 2 chores' })
    expect(dayCell).toHaveAttribute('aria-pressed', 'false')

    fireEvent.click(dayCell)
    await waitFor(() => expect(dayCell).toHaveAttribute('aria-pressed', 'true'))

    fireEvent.click(dayCell)
    await waitFor(() => expect(dayCell).toHaveAttribute('aria-pressed', 'false'))
  })

  it('detail panel shows full formatted date label', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getAllByText('1')[0])

    await waitFor(() => expect(screen.getByText('Thursday, January 1, 2026')).toBeInTheDocument())
  })

  it('filter count label clears person name when switching back to All', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Alison' }))
    fireEvent.click(screen.getAllByText('1')[0])
    await waitFor(() => expect(screen.getByText(/chore · Alison/)).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'All' }))
    await waitFor(() => expect(screen.queryByText(/chore · Alison/)).not.toBeInTheDocument())
  })

  it('switching filter while panel is open updates the chore list', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getAllByText('1')[0])
    await waitFor(() => {
      expect(screen.getByText('Kitchen Trash')).toBeInTheDocument()
      expect(screen.getByText('Bathroom Trash')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByRole('button', { name: 'Kevina' }))
    await waitFor(() => {
      expect(screen.getByText('Bathroom Trash')).toBeInTheDocument()
      expect(screen.queryByText('Kitchen Trash')).not.toBeInTheDocument()
    })
  })

  it('empty schedule renders calendar months with no chore dot labels', async () => {
    mockFetchOk({})
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    const choreButtons = screen.queryAllByRole('button', { name: /chore/ })
    expect(choreButtons).toHaveLength(0)
  })

  it('opens manage panel when Manage button is clicked', async () => {
    // Mock chores endpoint for the manage panel
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input)
      if (url.includes('api/me')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockMe) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/schedule/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockSchedule) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/chores/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) } as Response)
      }
      if (url.includes('api/groups')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([mockGroup]) } as Response)
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
    })
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Manage group' }))
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Manage group' })).toBeInTheDocument())
  })

  it('closes manage panel when close button is clicked', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input)
      if (url.includes('api/me')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockMe) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/schedule/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockSchedule) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/chores/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) } as Response)
      }
      if (url.includes('api/groups')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([mockGroup]) } as Response)
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
    })
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Manage group' }))
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Manage group' })).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Close manage panel' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Manage group' })).not.toBeInTheDocument())
  })

  it('opens create group dialog when + button is clicked', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Create new group' }))
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Create group' })).toBeInTheDocument())
  })

  it('closes create group dialog when Cancel is clicked', async () => {
    mockFetchOk(mockSchedule)
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getByRole('button', { name: 'Create new group' }))
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Create group' })).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Create group' })).not.toBeInTheDocument())
  })

  it('Escape key closes manage panel before closing detail panel', async () => {
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input)
      if (url.includes('api/me')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockMe) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/schedule/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockSchedule) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/chores/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) } as Response)
      }
      if (url.includes('api/groups')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([mockGroup]) } as Response)
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
    })
    render(<App />)
    await waitFor(() => screen.getByText('January'))

    fireEvent.click(screen.getAllByText('1')[0])
    await waitFor(() => expect(screen.getByText('Kitchen Trash')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: 'Manage group' }))
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Manage group' })).toBeInTheDocument())

    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    await waitFor(() => expect(screen.queryByRole('dialog', { name: 'Manage group' })).not.toBeInTheDocument())
    // Detail panel should still be open
    expect(screen.getByText('Kitchen Trash')).toBeInTheDocument()
  })

  it('multiple group tabs are shown when user is in multiple groups', async () => {
    const group2: GroupInfo = { id: 2, name: 'office', members: ['Alison'] }
    vi.spyOn(globalThis, 'fetch').mockImplementation((input) => {
      const url = String(input)
      if (url.includes('api/me')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve(mockMe) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/schedule/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
      }
      if (url.match(/api\/groups\/\d+\/chores/)) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([]) } as Response)
      }
      if (url.includes('api/groups')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve([mockGroup, group2]) } as Response)
      }
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) } as Response)
    })
    render(<App />)
    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'home' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'office' })).toBeInTheDocument()
    })
  })
})
