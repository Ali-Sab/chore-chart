import { useState, useEffect, useCallback, useRef, Fragment } from 'react'
import type { Schedule, Filter, ChoreEntry, GroupInfo, ChoreInfo } from './types'

const SCHEDULE_YEAR = 2026

const MONTHS = [
  'January', 'February', 'March', 'April', 'May', 'June',
  'July', 'August', 'September', 'October', 'November', 'December',
]
const DAY_HEADERS = ['Su', 'Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa']
const TODAY = new Date().toISOString().slice(0, 10)
const TODAY_MONTH = parseInt(TODAY.split('-')[1], 10) - 1

function personVars(index: number) {
  return {
    dot:  `var(--person-${index})`,
    bg:   `var(--person-${index}-bg)`,
    text: `var(--person-${index}-text)`,
  }
}

// ── Header ────────────────────────────────────────────────────────────────────

function PageHeader({
  people,
  filter,
  onFilter,
  onScrollToToday,
  dark,
  onToggleDark,
}: {
  people: string[]
  filter: Filter
  onFilter: (f: Filter) => void
  onScrollToToday: () => void
  dark: boolean
  onToggleDark: () => void
}) {
  const btnClass = (f: Filter) => {
    if (f === filter) {
      if (f === 'all') return 'filter-btn active-all'
      const idx = people.indexOf(f)
      if (idx >= 0) return `filter-btn active-person-${idx}`
    }
    return 'filter-btn'
  }

  const handleLogout = () => {
    fetch(import.meta.env.BASE_URL + 'api/auth/logout', { method: 'POST' })
      .then(r => r.json())
      .then((data: { location: string }) => { window.location.replace(data.location) })
      .catch(() => { window.location.replace(import.meta.env.BASE_URL + 'oauth/start') })
  }

  return (
    <header style={styles.pageHeader}>
      <div>
        <div style={styles.pageTitle}>Chore calendar</div>
        <div style={styles.pageSub}>
          {SCHEDULE_YEAR}{people.length > 0 ? ` · ${people.join(' & ')}` : ''}
        </div>
      </div>
      <div style={styles.controls} className="header-controls">
        {people.length > 0 && (
          <div style={styles.legend}>
            {people.map((name, i) => (
              <span key={name} style={styles.legendItem}>
                <span style={{ ...styles.legendDot, background: personVars(i).dot }} />
                {name}
              </span>
            ))}
          </div>
        )}
        <button className="today-btn" onClick={onScrollToToday}>Today</button>
        <button className="today-btn" onClick={onToggleDark} aria-label="Toggle dark mode">
          {dark ? '☀︎' : '☽'}
        </button>
        <div style={styles.filterGroup}>
          <button className={btnClass('all')} onClick={() => onFilter('all')}>All</button>
          {people.map(name => (
            <button key={name} className={btnClass(name)} onClick={() => onFilter(name)}>
              {name}
            </button>
          ))}
        </div>
        <a href="/accounts/" className="today-btn" style={{ textDecoration: 'none' }}>My Account</a>
        <button className="today-btn" onClick={handleLogout}>Log out</button>
      </div>
    </header>
  )
}

// ── Group tabs ────────────────────────────────────────────────────────────────

function GroupTabs({
  groups,
  activeGroupId,
  onSelect,
  onAdd,
  onManage,
}: {
  groups: GroupInfo[]
  activeGroupId: number | null
  onSelect: (id: number) => void
  onAdd: () => void
  onManage: () => void
}) {
  return (
    <div style={styles.groupTabsWrapper}>
      <div style={styles.groupTabsInner}>
        {groups.map(g => (
          <button
            key={g.id}
            className={g.id === activeGroupId ? 'group-tab group-tab-active' : 'group-tab'}
            onClick={() => onSelect(g.id)}
          >
            {g.name}
          </button>
        ))}
        <button
          className="group-tab-add"
          onClick={onAdd}
          aria-label="Create new group"
          title="Create new group"
        >
          +
        </button>
      </div>
      {activeGroupId !== null && (
        <button className="today-btn" onClick={onManage} aria-label="Manage group">
          Manage
        </button>
      )}
    </div>
  )
}

// ── No groups screen ──────────────────────────────────────────────────────────

function NoGroupsScreen({ onCreate }: { onCreate: () => void }) {
  return (
    <div style={styles.noGroupsScreen}>
      <div style={styles.noGroupsTitle}>Welcome!</div>
      <div style={styles.noGroupsBody}>
        You're not in any group yet. Create one to get started, or ask a housemate to add you.
      </div>
      <button style={styles.primaryBtn} onClick={onCreate}>Create group</button>
    </div>
  )
}

// ── Create group dialog ───────────────────────────────────────────────────────

function CreateGroupDialog({
  baseURL,
  onCreated,
  onCancel,
}: {
  baseURL: string
  onCreated: (g: GroupInfo) => void
  onCancel: () => void
}) {
  const [name, setName] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleCreate = async () => {
    if (!name.trim()) return
    setLoading(true)
    setError('')
    try {
      const r = await fetch(baseURL + 'api/groups', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      })
      const body = await r.json()
      if (r.ok) {
        onCreated(body as GroupInfo)
      } else {
        setError((body as { error?: string }).error ?? 'Failed to create group.')
      }
    } catch {
      setError('Network error. Please try again.')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div style={styles.overlay} role="dialog" aria-modal="true" aria-label="Create group">
      <div style={{ ...styles.overlayPanel, maxWidth: 360 }}>
        <div style={styles.overlayHeader}>
          <div style={styles.overlayTitle}>Create group</div>
          <button style={styles.closeBtn} onClick={onCancel} aria-label="Dismiss create group dialog">✕</button>
        </div>
        <div style={styles.overlayBody}>
          <input
            style={styles.input}
            value={name}
            onChange={e => setName(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleCreate() }}
            placeholder="Group name"
            autoFocus
            aria-label="Group name"
          />
          <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
            <button
              style={styles.primaryBtn}
              onClick={handleCreate}
              disabled={loading || !name.trim()}
              aria-label="Create"
            >
              {loading ? 'Creating…' : 'Create'}
            </button>
            <button style={styles.secondaryBtn} onClick={onCancel}>Cancel</button>
          </div>
          {error && <div style={styles.errorMsg}>{error}</div>}
        </div>
      </div>
    </div>
  )
}

// ── Manage panel ──────────────────────────────────────────────────────────────

function ManagePanel({
  group,
  currentUsername,
  baseURL,
  onClose,
  onGroupUpdated,
  onGroupDeleted,
  onChoresChanged,
}: {
  group: GroupInfo
  currentUsername: string
  baseURL: string
  onClose: () => void
  onGroupUpdated: (g: GroupInfo) => void
  onGroupDeleted: (id: number) => void
  onChoresChanged: () => void
}) {
  const [chores, setChores] = useState<ChoreInfo[]>([])
  const [choreLoading, setChoreLoading] = useState(true)
  const [newChoreName, setNewChoreName] = useState('')
  const [newChoreAssignee, setNewChoreAssignee] = useState(group.members[0] ?? '')
  const [newChoreCadence, setNewChoreCadence] = useState('')
  const [newMemberName, setNewMemberName] = useState('')
  const [choreError, setChoreError] = useState('')
  const [memberError, setMemberError] = useState('')

  useEffect(() => {
    fetch(baseURL + `api/groups/${group.id}/chores`)
      .then(r => r.ok ? r.json() : [])
      .then((data: ChoreInfo[]) => { setChores(data); setChoreLoading(false) })
      .catch(() => setChoreLoading(false))
  }, [group.id, baseURL])

  const addChore = async () => {
    const cadence = parseFloat(newChoreCadence)
    if (!newChoreName.trim() || !newChoreAssignee || isNaN(cadence) || cadence <= 0) {
      setChoreError('Fill in all chore fields with valid values.')
      return
    }
    setChoreError('')
    try {
      const r = await fetch(baseURL + `api/groups/${group.id}/chores`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: newChoreName.trim(), assigned_to: newChoreAssignee, cadence_weeks: cadence }),
      })
      const body = await r.json()
      if (r.ok) {
        setChores(c => [...c, body as ChoreInfo])
        setNewChoreName('')
        setNewChoreCadence('')
        onChoresChanged()
      } else {
        setChoreError((body as { error?: string }).error ?? 'Failed to add chore.')
      }
    } catch {
      setChoreError('Network error. Please try again.')
    }
  }

  const removeChore = async (choreId: number) => {
    const r = await fetch(baseURL + `api/groups/${group.id}/chores/${choreId}`, { method: 'DELETE' })
    if (r.ok) {
      setChores(c => c.filter(ch => ch.id !== choreId))
      onChoresChanged()
    }
  }

  const addMember = async () => {
    if (!newMemberName.trim()) return
    setMemberError('')
    try {
      const r = await fetch(baseURL + `api/groups/${group.id}/members`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: newMemberName.trim() }),
      })
      const body = await r.json()
      if (r.ok) {
        setNewMemberName('')
        onGroupUpdated(body as GroupInfo)
      } else {
        setMemberError((body as { error?: string }).error ?? 'Failed to add member.')
      }
    } catch {
      setMemberError('Network error. Please try again.')
    }
  }

  const removeMember = async (username: string) => {
    const isSelf = username === currentUsername
    const r = await fetch(
      baseURL + `api/groups/${group.id}/members/${encodeURIComponent(username)}`,
      { method: 'DELETE' },
    )
    if (!r.ok) return
    if (r.status === 200) {
      // Group was deleted (last member left).
      onGroupDeleted(group.id)
    } else {
      // 204: member removed, group still exists.
      if (isSelf) {
        // Current user left — remove group from their list.
        onGroupDeleted(group.id)
      } else {
        onGroupUpdated({ ...group, members: group.members.filter(m => m !== username) })
      }
    }
  }

  return (
    <div style={styles.overlay} role="dialog" aria-modal="true" aria-label="Manage group">
      <div style={styles.overlayPanel}>
        <div style={styles.overlayHeader}>
          <div style={styles.overlayTitle}>Manage · {group.name}</div>
          <button style={styles.closeBtn} onClick={onClose} aria-label="Close manage panel">✕</button>
        </div>

        <div style={styles.overlayBody}>
          {/* Members */}
          <div style={styles.manageSection}>
            <div style={styles.manageSectionTitle}>Members</div>
            <div style={styles.memberList}>
              {group.members.map(m => (
                <div key={m} style={styles.memberRow}>
                  <span style={styles.memberName}>{m}</span>
                  <button
                    style={styles.removeBtn}
                    onClick={() => removeMember(m)}
                    aria-label={m === currentUsername ? 'Leave group' : `Remove ${m}`}
                  >
                    {m === currentUsername ? 'Leave' : '✕'}
                  </button>
                </div>
              ))}
            </div>
            <div style={styles.addRow}>
              <input
                style={styles.input}
                value={newMemberName}
                onChange={e => setNewMemberName(e.target.value)}
                onKeyDown={e => { if (e.key === 'Enter') addMember() }}
                placeholder="Username to add"
                aria-label="Username to add"
              />
              <button style={styles.primaryBtn} onClick={addMember} aria-label="Add member">Add</button>
            </div>
            {memberError && <div style={styles.errorMsg}>{memberError}</div>}
          </div>

          {/* Chores */}
          <div style={styles.manageSection}>
            <div style={styles.manageSectionTitle}>Chores</div>
            {choreLoading ? (
              <div style={styles.loadingMsg}>Loading…</div>
            ) : chores.length === 0 ? (
              <div style={styles.noChores}>No chores yet.</div>
            ) : (
              <div style={styles.choreManageList}>
                {chores.map(c => (
                  <div key={c.id} style={styles.choreManageRow}>
                    <div style={styles.choreManageInfo}>
                      <span style={styles.choreManageName}>{c.name}</span>
                      <span style={styles.choreManageMeta}>{c.assigned_to} · every {c.cadence_weeks}w</span>
                    </div>
                    <button
                      style={styles.removeBtn}
                      onClick={() => removeChore(c.id)}
                      aria-label={`Remove chore ${c.name}`}
                    >✕</button>
                  </div>
                ))}
              </div>
            )}
            <div style={styles.addChoreForm}>
              <input
                style={styles.input}
                value={newChoreName}
                onChange={e => setNewChoreName(e.target.value)}
                placeholder="Chore name"
                aria-label="Chore name"
              />
              <select
                style={styles.select}
                value={newChoreAssignee}
                onChange={e => setNewChoreAssignee(e.target.value)}
                aria-label="Assign to"
              >
                {group.members.map(m => <option key={m} value={m}>{m}</option>)}
              </select>
              <input
                style={{ ...styles.input, width: 80 }}
                type="number"
                min={0.1}
                step={0.5}
                value={newChoreCadence}
                onChange={e => setNewChoreCadence(e.target.value)}
                placeholder="Weeks"
                aria-label="Cadence in weeks"
              />
              <button style={styles.primaryBtn} onClick={addChore} aria-label="Add chore">Add</button>
            </div>
            {choreError && <div style={styles.errorMsg}>{choreError}</div>}
          </div>
        </div>
      </div>
    </div>
  )
}

// ── Month card ────────────────────────────────────────────────────────────────

function MonthCard({
  monthIndex,
  isCurrent,
  isPast,
  people,
  getFiltered,
  selected,
  onSelect,
}: {
  monthIndex: number
  isCurrent: boolean
  isPast: boolean
  people: string[]
  getFiltered: (key: string) => ChoreEntry[]
  selected: string | null
  onSelect: (key: string) => void
}) {
  const daysInMonth = new Date(Date.UTC(SCHEDULE_YEAR, monthIndex + 1, 0)).getUTCDate()
  const startDow    = new Date(Date.UTC(SCHEDULE_YEAR, monthIndex, 1)).getUTCDay()

  const cells = []

  for (let i = 0; i < startDow; i++) {
    cells.push(<div key={`e${i}`} style={styles.dayCellEmpty} />)
  }

  for (let d = 1; d <= daysInMonth; d++) {
    const key        = `${SCHEDULE_YEAR}-${String(monthIndex + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    const ch         = getFiltered(key)
    const isToday    = key === TODAY
    const isPastDay  = key < TODAY
    const isSelected = key === selected

    const dotIndices = people
      .map((name, i) => ({ i, has: ch.some(c => c.person === name) }))
      .filter(x => x.has)
      .map(x => x.i)

    cells.push(
      <div
        key={key}
        role="button"
        tabIndex={0}
        aria-label={`${MONTHS[monthIndex]} ${d}${ch.length > 0 ? `, ${ch.length} chore${ch.length !== 1 ? 's' : ''}` : ''}`}
        aria-pressed={isSelected}
        style={{
          ...styles.dayCell,
          ...(isSelected ? styles.dayCellSelected : {}),
          ...(isPastDay && !isToday ? { opacity: 0.38 } : {}),
        }}
        onClick={() => onSelect(key)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onSelect(key)
          }
        }}
      >
        <div style={{ ...styles.dayNum, ...(isToday ? styles.dayNumToday : {}) }}>
          {d}
        </div>
        <div style={styles.dayDots}>
          {dotIndices.map(i => (
            <span key={i} style={{ ...styles.dotSm, background: personVars(i).dot }} />
          ))}
        </div>
      </div>,
    )
  }

  return (
    <div
      id={`month-${monthIndex}`}
      style={{
        ...styles.monthCard,
        ...(isPast ? { opacity: 0.5 } : {}),
        ...(isCurrent ? { borderColor: 'var(--today-ring, #3b82f6)' } : {}),
      }}
    >
      <div style={{ ...styles.monthName, ...(isCurrent ? { color: 'var(--today-ring, #3b82f6)' } : {}) }}>
        {MONTHS[monthIndex]}
      </div>
      <div style={styles.dayHeaders}>
        {DAY_HEADERS.map(d => <div key={d} style={styles.dayHdr}>{d}</div>)}
      </div>
      <div style={styles.daysGrid}>{cells}</div>
    </div>
  )
}

// ── Detail panel ──────────────────────────────────────────────────────────────

function DetailPanel({
  dateKey,
  chores,
  people,
  filter,
  onClose,
}: {
  dateKey: string
  chores: ChoreEntry[]
  people: string[]
  filter: Filter
  onClose: () => void
}) {
  const d = new Date(dateKey + 'T00:00:00Z')
  const label = d.toLocaleDateString('en-US', {
    weekday: 'long', year: 'numeric', month: 'long', day: 'numeric', timeZone: 'UTC',
  })
  const filterLabel = filter !== 'all' ? ` · ${filter}` : ''

  return (
    <div style={styles.detailPanel}>
      <div style={styles.detailHeader}>
        <div style={styles.detailDate}>
          {label}
          <span style={styles.detailCount}>
            {chores.length} chore{chores.length !== 1 ? 's' : ''}{filterLabel}
          </span>
        </div>
        <button style={styles.closeBtn} onClick={onClose} aria-label="Close detail panel">✕</button>
      </div>
      <div style={styles.choreList}>
        {chores.length === 0 ? (
          <div style={styles.noChores}>No chores scheduled.</div>
        ) : (
          chores.map((c) => {
            const idx = people.indexOf(c.person)
            const vars = personVars(idx >= 0 ? idx : 0)
            return (
              <div key={`${c.person}-${c.name}`} style={styles.choreItem}>
                <span style={{ ...styles.choreBadge, background: vars.bg, color: vars.text }}>
                  {c.person}
                </span>
                <span>{c.name}</span>
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}

// ── App ───────────────────────────────────────────────────────────────────────

export default function App() {
  const base = import.meta.env.BASE_URL

  const [appStatus, setAppStatus] = useState<'loading' | 'no-groups' | 'ok' | 'error'>('loading')
  const [currentUsername, setCurrentUsername] = useState('')
  const [groups, setGroups] = useState<GroupInfo[]>([])
  const [activeGroupId, setActiveGroupId] = useState<number | null>(null)
  const [schedule, setSchedule] = useState<Schedule>({})
  const [scheduleVersion, setScheduleVersion] = useState(0)
  const [filter, setFilter] = useState<Filter>('all')
  const [selected, setSelected] = useState<string | null>(null)
  const [hasInteracted, setHasInteracted] = useState(false)
  const [showManage, setShowManage] = useState(false)
  const [showCreate, setShowCreate] = useState(false)
  const [dark, setDark] = useState(() => {
    const stored = localStorage.getItem('theme')
    if (stored) return stored === 'dark'
    return window.matchMedia('(prefers-color-scheme: dark)').matches
  })
  const [cols, setCols] = useState(() => {
    const w = typeof window !== 'undefined' ? window.innerWidth : 0
    if (w > 0 && w < 640) return 1
    if (w > 0 && w < 900) return 2
    return 3
  })
  const panelRef = useRef<HTMLDivElement>(null)

  const activeGroup = groups.find(g => g.id === activeGroupId) ?? null
  const people = activeGroup?.members ?? []

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', dark ? 'dark' : 'light')
    localStorage.setItem('theme', dark ? 'dark' : 'light')
  }, [dark])

  // Load groups and current user on mount.
  useEffect(() => {
    Promise.all([
      fetch(base + 'api/groups'),
      fetch(base + 'api/me'),
    ]).then(async ([groupsRes, meRes]) => {
      if (groupsRes.status === 401 || meRes.status === 401) {
        const returnTo = encodeURIComponent(window.location.pathname + window.location.search)
        window.location.replace(base + 'oauth/start?return_to=' + returnTo)
        return
      }
      if (!groupsRes.ok || !meRes.ok) throw new Error('non-ok response')
      const [groupsData, meData]: [GroupInfo[], { username: string }] = await Promise.all([
        groupsRes.json(),
        meRes.json(),
      ])
      setCurrentUsername(meData.username ?? '')
      setGroups(groupsData)
      if (groupsData.length === 0) {
        setAppStatus('no-groups')
      } else {
        setActiveGroupId(groupsData[0].id)
        setAppStatus('ok')
      }
    }).catch(() => setAppStatus('error'))
  }, [base])

  // Load schedule whenever active group or chores change.
  useEffect(() => {
    if (!activeGroupId) return
    fetch(base + `api/groups/${activeGroupId}/schedule`)
      .then(r => r.ok ? r.json() : {})
      .then((data: Schedule) => setSchedule(data))
      .catch(() => setSchedule({}))
  }, [base, activeGroupId, scheduleVersion])

  // Scroll to current month on first ok load.
  useEffect(() => {
    if (appStatus === 'ok') {
      document.getElementById(`month-${TODAY_MONTH}`)?.scrollIntoView({ block: 'start' })
    }
  }, [appStatus])

  // Auto-scroll detail panel into view.
  useEffect(() => {
    if (selected) {
      requestAnimationFrame(() => {
        panelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
      })
    }
  }, [selected])

  // Escape key closes overlays and detail panel.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key !== 'Escape') return
      if (showManage) { setShowManage(false); return }
      if (showCreate) { setShowCreate(false); return }
      setSelected(null)
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [showManage, showCreate])

  // Responsive column count.
  useEffect(() => {
    const handler = () => {
      if (window.innerWidth < 640) setCols(1)
      else if (window.innerWidth < 900) setCols(2)
      else setCols(3)
    }
    window.addEventListener('resize', handler)
    return () => window.removeEventListener('resize', handler)
  }, [])

  const getFiltered = useCallback(
    (key: string): ChoreEntry[] => {
      const ch = schedule[key] ?? []
      return filter === 'all' ? ch : ch.filter(c => c.person === filter)
    },
    [schedule, filter],
  )

  const handleSelect = (key: string) => {
    setHasInteracted(true)
    setSelected(prev => (prev === key ? null : key))
  }

  const scrollToToday = () => {
    document.getElementById(`month-${TODAY_MONTH}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }

  const handleTabChange = (id: number) => {
    if (id === activeGroupId) return
    setActiveGroupId(id)
    setFilter('all')
    setSelected(null)
    setSchedule({})
    setShowManage(false)
    setScheduleVersion(0)
  }

  const handleGroupCreated = (g: GroupInfo) => {
    setGroups(prev => [...prev, g])
    setActiveGroupId(g.id)
    setFilter('all')
    setSelected(null)
    setSchedule({})
    setShowCreate(false)
    setAppStatus('ok')
  }

  const handleGroupUpdated = (g: GroupInfo) => {
    setGroups(prev => prev.map(gr => gr.id === g.id ? g : gr))
    if (filter !== 'all' && !g.members.includes(filter)) {
      setFilter('all')
    }
  }

  const handleGroupDeleted = (id: number) => {
    setGroups(prev => {
      const remaining = prev.filter(g => g.id !== id)
      if (remaining.length === 0) {
        setActiveGroupId(null)
        setSchedule({})
        setAppStatus('no-groups')
      } else {
        setActiveGroupId(remaining[0].id)
        setSchedule({})
      }
      setFilter('all')
      setSelected(null)
      setShowManage(false)
      return remaining
    })
  }

  const selectedMonthIndex = selected ? parseInt(selected.split('-')[1], 10) - 1 : -1

  const rows: number[][] = []
  for (let i = 0; i < 12; i += cols) {
    rows.push(Array.from({ length: Math.min(cols, 12 - i) }, (_, j) => i + j))
  }

  if (appStatus === 'loading') {
    return <p style={styles.statusMsg}>Loading…</p>
  }

  if (appStatus === 'error') {
    return <p style={{ ...styles.statusMsg, color: 'var(--error, #c0392b)' }}>Failed to load schedule. Please refresh.</p>
  }

  const personFilterCSS = people.map((_, i) => `
    .filter-btn.active-person-${i} { background: var(--person-${i}-bg); color: var(--person-${i}-text); font-weight: 500; }
  `).join('')

  return (
    <>
      <PageHeader
        people={people}
        filter={filter}
        onFilter={setFilter}
        onScrollToToday={scrollToToday}
        dark={dark}
        onToggleDark={() => setDark(d => !d)}
      />

      <GroupTabs
        groups={groups}
        activeGroupId={activeGroupId}
        onSelect={handleTabChange}
        onAdd={() => setShowCreate(true)}
        onManage={() => setShowManage(true)}
      />

      {appStatus === 'no-groups' && (
        <NoGroupsScreen onCreate={() => setShowCreate(true)} />
      )}

      {appStatus === 'ok' && (
        <>
          <div style={{ ...styles.calendarGrid, gridTemplateColumns: `repeat(${cols}, 1fr)` }}>
            {rows.map((row, rowIndex) => (
              <Fragment key={rowIndex}>
                {row.map(m => (
                  <MonthCard
                    key={m}
                    monthIndex={m}
                    isCurrent={m === TODAY_MONTH}
                    isPast={m < TODAY_MONTH}
                    people={people}
                    getFiltered={getFiltered}
                    selected={selected}
                    onSelect={handleSelect}
                  />
                ))}
                {Math.floor(selectedMonthIndex / cols) === rowIndex && selected && (
                  <div ref={panelRef} style={{ gridColumn: '1 / -1' }}>
                    <DetailPanel
                      dateKey={selected}
                      chores={getFiltered(selected)}
                      people={people}
                      filter={filter}
                      onClose={() => setSelected(null)}
                    />
                  </div>
                )}
              </Fragment>
            ))}
          </div>

          {!hasInteracted && <p style={styles.hint}>Click any day to see scheduled chores</p>}
        </>
      )}

      {showManage && activeGroup && (
        <ManagePanel
          group={activeGroup}
          currentUsername={currentUsername}
          baseURL={base}
          onClose={() => setShowManage(false)}
          onGroupUpdated={handleGroupUpdated}
          onGroupDeleted={handleGroupDeleted}
          onChoresChanged={() => setScheduleVersion(v => v + 1)}
        />
      )}

      {showCreate && (
        <CreateGroupDialog
          baseURL={base}
          onCreated={handleGroupCreated}
          onCancel={() => setShowCreate(false)}
        />
      )}

      <style>{`
        .filter-btn {
          font-size: 12px; padding: 4px 12px; border-radius: 20px; border: none;
          background: transparent; color: var(--text-muted); cursor: pointer;
          font-family: var(--font); font-weight: 400; transition: all 0.15s ease;
        }
        .filter-btn:hover { color: var(--text); }
        .filter-btn.active-all { background: var(--text); color: #fff; }
        ${personFilterCSS}

        .today-btn {
          font-size: 11px; padding: 4px 10px; border-radius: 20px;
          border: 1px solid var(--border-med); background: transparent;
          color: var(--text-muted); cursor: pointer; font-family: var(--mono, monospace);
          transition: all 0.15s ease;
        }
        .today-btn:hover { color: var(--text); border-color: var(--text-muted); }

        .group-tab {
          font-size: 12px; padding: 5px 14px; border-radius: 20px;
          border: 1px solid var(--border-med); background: transparent;
          color: var(--text-muted); cursor: pointer; font-family: var(--font);
          transition: all 0.15s ease; white-space: nowrap;
        }
        .group-tab:hover { color: var(--text); }
        .group-tab.group-tab-active {
          background: var(--text); color: var(--bg); border-color: var(--text);
        }
        .group-tab-add {
          font-size: 16px; width: 28px; height: 28px; border-radius: 50%;
          border: 1px solid var(--border-med); background: transparent;
          color: var(--text-muted); cursor: pointer;
          display: flex; align-items: center; justify-content: center;
          transition: all 0.15s ease; flex-shrink: 0; line-height: 1;
        }
        .group-tab-add:hover { color: var(--text); border-color: var(--text-muted); }

        @media (max-width: 640px) {
          .filter-btn { padding: 9px 16px; font-size: 13px; }
          .today-btn  { padding: 9px 14px; font-size: 12px; }
          .header-controls { gap: 10px; }
        }
      `}</style>
    </>
  )
}

// ── Styles ────────────────────────────────────────────────────────────────────

const styles: Record<string, React.CSSProperties> = {
  pageHeader: {
    maxWidth: 960, margin: '0 auto 1.25rem', display: 'flex',
    alignItems: 'flex-end', justifyContent: 'space-between', gap: '1rem',
    flexWrap: 'wrap', paddingBottom: '1.25rem', borderBottom: '1px solid var(--border-med)',
  },
  pageTitle: { fontSize: 22, fontWeight: 500, letterSpacing: '-0.3px' },
  pageSub: { fontSize: 13, color: 'var(--text-muted)', marginTop: 3, fontFamily: 'var(--mono)' },
  controls: { display: 'flex', alignItems: 'center', gap: 16, flexWrap: 'wrap' },
  legend: { display: 'flex', gap: 12 },
  legendItem: { display: 'flex', alignItems: 'center', gap: 6, fontSize: 12, color: 'var(--text-muted)' },
  legendDot: { width: 8, height: 8, borderRadius: '50%', display: 'inline-block' },
  filterGroup: {
    display: 'flex', gap: 4, background: 'var(--surface)',
    border: '1px solid var(--border-med)', borderRadius: 20, padding: 3,
  },
  groupTabsWrapper: {
    maxWidth: 960, margin: '0 auto 1rem', display: 'flex',
    alignItems: 'center', gap: 10,
  },
  groupTabsInner: {
    display: 'flex', gap: 6, overflowX: 'auto', flex: 1,
    alignItems: 'center', paddingBottom: 2,
  },
  calendarGrid: { maxWidth: 960, margin: '0 auto', display: 'grid', gap: 12 },
  monthCard: {
    background: 'var(--surface)', border: '1px solid var(--border)',
    borderRadius: 'var(--radius-lg)', padding: 14,
    transition: 'border-color 0.15s ease, opacity 0.15s ease',
  },
  monthName: {
    fontSize: 13, fontWeight: 500, color: 'var(--text)', marginBottom: 10,
    letterSpacing: '0.3px', transition: 'color 0.15s ease',
  },
  dayHeaders: { display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', marginBottom: 4 },
  dayHdr: {
    fontSize: 9, color: 'var(--text-faint)', textAlign: 'center',
    fontFamily: 'var(--mono)', fontWeight: 500, textTransform: 'uppercase', letterSpacing: '0.5px',
  },
  daysGrid: { display: 'grid', gridTemplateColumns: 'repeat(7, 1fr)', gap: 1 },
  dayCell: {
    display: 'flex', flexDirection: 'column', alignItems: 'center',
    padding: '4px 1px', cursor: 'pointer', borderRadius: 'var(--radius-sm)',
    minHeight: 36, transition: 'opacity 0.1s ease',
  },
  dayCellEmpty: { minHeight: 36 },
  dayCellSelected: {
    background: 'var(--bg)', outline: '1.5px solid var(--border-med)', outlineOffset: -1,
  },
  dayNum: {
    fontSize: 10, color: 'var(--text)', width: 19, height: 19,
    display: 'flex', alignItems: 'center', justifyContent: 'center',
    borderRadius: '50%', fontFamily: 'var(--mono)', fontWeight: 400,
  },
  dayNumToday: { background: 'var(--today-ring)', color: '#fff', fontWeight: 500 },
  dayDots: {
    display: 'flex', gap: 2, marginTop: 2, height: 5,
    alignItems: 'center', justifyContent: 'center',
  },
  dotSm: { width: 4, height: 4, borderRadius: '50%', display: 'inline-block', flexShrink: 0 },
  detailPanel: {
    background: 'var(--surface)', border: '1px solid var(--border-med)',
    borderRadius: 'var(--radius-lg)', padding: '16px 18px',
    animation: 'fadeIn 0.15s ease', marginTop: 4,
  },
  detailHeader: {
    display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between',
    gap: 8, marginBottom: 10,
  },
  detailDate: {
    fontSize: 14, fontWeight: 500, color: 'var(--text)',
    display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap',
  },
  detailCount: { fontSize: 11, color: 'var(--text-muted)', fontFamily: 'var(--mono)', fontWeight: 400 },
  closeBtn: {
    background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-muted)',
    fontSize: 14, padding: '8px 10px', lineHeight: 1, borderRadius: 4, flexShrink: 0,
  },
  choreList: { display: 'flex', flexDirection: 'column', gap: 5 },
  choreItem: {
    display: 'flex', alignItems: 'center', gap: 8, fontSize: 13,
    color: 'var(--text)', padding: '5px 0', borderBottom: '1px solid var(--border)',
  },
  choreBadge: { fontSize: 11, padding: '2px 9px', borderRadius: 20, fontWeight: 500, flexShrink: 0 },
  noChores: { fontSize: 13, color: 'var(--text-muted)' },
  hint: {
    maxWidth: 960, margin: '10px auto 0', fontSize: 11,
    color: 'var(--text-faint)', textAlign: 'center', fontFamily: 'var(--mono)',
  },
  statusMsg: { textAlign: 'center', marginTop: '4rem', fontSize: 14, color: 'var(--text-muted)', fontFamily: 'var(--mono)' },
  // Overlay (manage panel + create dialog)
  overlay: {
    position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.45)', zIndex: 100,
    display: 'flex', alignItems: 'flex-start', justifyContent: 'center',
    padding: '48px 16px', overflowY: 'auto',
  },
  overlayPanel: {
    background: 'var(--bg)', borderRadius: 'var(--radius-lg)', width: '100%',
    maxWidth: 480, boxShadow: '0 8px 32px rgba(0,0,0,0.18)',
    border: '1px solid var(--border-med)',
  },
  overlayHeader: {
    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
    padding: '16px 20px 0', marginBottom: 4,
  },
  overlayTitle: { fontSize: 15, fontWeight: 500, color: 'var(--text)' },
  overlayBody: { padding: '12px 20px 20px' },
  manageSection: { marginTop: 16 },
  manageSectionTitle: {
    fontSize: 11, fontWeight: 600, color: 'var(--text-faint)',
    textTransform: 'uppercase', letterSpacing: '0.6px', marginBottom: 8,
    fontFamily: 'var(--mono)',
  },
  memberList: { display: 'flex', flexDirection: 'column', gap: 4, marginBottom: 8 },
  memberRow: {
    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
    padding: '6px 0', borderBottom: '1px solid var(--border)',
  },
  memberName: { fontSize: 13, color: 'var(--text)' },
  removeBtn: {
    background: 'none', border: 'none', cursor: 'pointer', color: 'var(--text-faint)',
    fontSize: 12, padding: '4px 8px', borderRadius: 4, fontFamily: 'var(--font)',
  },
  addRow: { display: 'flex', gap: 8, marginTop: 8 },
  choreManageList: { display: 'flex', flexDirection: 'column', gap: 2, marginBottom: 10 },
  choreManageRow: {
    display: 'flex', alignItems: 'center', justifyContent: 'space-between',
    padding: '6px 0', borderBottom: '1px solid var(--border)',
  },
  choreManageInfo: { display: 'flex', flexDirection: 'column', gap: 2 },
  choreManageName: { fontSize: 13, color: 'var(--text)' },
  choreManageMeta: { fontSize: 11, color: 'var(--text-faint)', fontFamily: 'var(--mono)' },
  addChoreForm: { display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 8 },
  input: {
    fontSize: 13, padding: '6px 10px', borderRadius: 6,
    border: '1px solid var(--border-med)', background: 'var(--surface)',
    color: 'var(--text)', fontFamily: 'var(--font)', flex: 1, minWidth: 0,
    outline: 'none',
  },
  select: {
    fontSize: 13, padding: '6px 8px', borderRadius: 6,
    border: '1px solid var(--border-med)', background: 'var(--surface)',
    color: 'var(--text)', fontFamily: 'var(--font)', cursor: 'pointer',
    outline: 'none',
  },
  primaryBtn: {
    fontSize: 13, padding: '7px 16px', borderRadius: 20, border: 'none',
    background: 'var(--text)', color: 'var(--bg)', cursor: 'pointer',
    fontFamily: 'var(--font)', fontWeight: 500, flexShrink: 0,
    transition: 'opacity 0.15s ease',
  },
  secondaryBtn: {
    fontSize: 13, padding: '7px 16px', borderRadius: 20,
    border: '1px solid var(--border-med)', background: 'transparent',
    color: 'var(--text-muted)', cursor: 'pointer', fontFamily: 'var(--font)',
  },
  errorMsg: { fontSize: 12, color: 'var(--error, #c0392b)', marginTop: 6 },
  loadingMsg: { fontSize: 13, color: 'var(--text-muted)', fontFamily: 'var(--mono)' },
  noGroupsScreen: {
    maxWidth: 480, margin: '4rem auto', padding: '0 16px', textAlign: 'center',
  },
  noGroupsTitle: { fontSize: 22, fontWeight: 500, color: 'var(--text)', marginBottom: 12 },
  noGroupsBody: { fontSize: 14, color: 'var(--text-muted)', marginBottom: 24, lineHeight: 1.6 },
}
