import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '../../test/test-utils'
import { mockFreshMatchNotification } from '../../test/factories'
import { api } from '../../api'
import NotificationBell from './NotificationBell'

vi.mock('../../api', () => ({
  api: {
    notifications: { list: vi.fn(), markSeen: vi.fn(), unseenCount: vi.fn() },
  },
}))

describe('NotificationBell', () => {
  beforeEach(() => {
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 0 })
    vi.mocked(api.notifications.list).mockResolvedValue([])
  })

  it('renders bell button without badge when count is 0', async () => {
    renderWithProviders(<NotificationBell />)
    const btn = screen.getByRole('button', { name: /notifications/i })
    expect(btn).toBeInTheDocument()
    await waitFor(() => {
      expect(screen.queryByText(/99\+/)).not.toBeInTheDocument()
    })
  })

  it('shows badge with unseen count', async () => {
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 5 })
    renderWithProviders(<NotificationBell />)
    await waitFor(() => {
      expect(screen.getByText('5')).toBeInTheDocument()
    })
  })

  it('caps badge at 99+', async () => {
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 150 })
    renderWithProviders(<NotificationBell />)
    await waitFor(() => {
      expect(screen.getByText('99+')).toBeInTheDocument()
    })
  })

  it('opens dropdown on bell click and fetches notifications', async () => {
    const user = userEvent.setup()
    const notif = mockFreshMatchNotification()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 1 })
    vi.mocked(api.notifications.list).mockResolvedValue([notif])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.getByText('Notifications')).toBeInTheDocument()
      expect(screen.getByText('Senior React Developer')).toBeInTheDocument()
      expect(screen.getByText('Acme Corp')).toBeInTheDocument()
      expect(screen.getByText('Match score: 85')).toBeInTheDocument()
    })
  })

  it('shows fresh chip for fresh notifications', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 1 })
    vi.mocked(api.notifications.list).mockResolvedValue([mockFreshMatchNotification({ fresh: true })])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.getByText('fresh')).toBeInTheDocument()
    })
  })

  it('does not show fresh chip for non-fresh notifications', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 1 })
    vi.mocked(api.notifications.list).mockResolvedValue([mockFreshMatchNotification({ fresh: false })])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.queryByText('fresh')).not.toBeInTheDocument()
    })
  })

  it('shows mark as seen button for unseen notifications', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 1 })
    vi.mocked(api.notifications.list).mockResolvedValue([mockFreshMatchNotification({ seen: false })])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Mark as seen' })).toBeInTheDocument()
    })
  })

  it('hides mark as seen button for seen notifications', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 1 })
    vi.mocked(api.notifications.list).mockResolvedValue([mockFreshMatchNotification({ seen: true })])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Mark as seen' })).not.toBeInTheDocument()
    })
  })

  it('calls markSeen when mark as seen button is clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.markSeen).mockResolvedValue(undefined)
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 1 })
    vi.mocked(api.notifications.list).mockResolvedValue([mockFreshMatchNotification({ id: 'notif-1', seen: false })])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    const markBtn = await screen.findByRole('button', { name: 'Mark as seen' })
    await user.click(markBtn)

    await waitFor(() => {
      expect(api.notifications.markSeen).toHaveBeenCalledWith('notif-1')
    })
  })

  it('shows empty state when no notifications', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 0 })
    vi.mocked(api.notifications.list).mockResolvedValue([])

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.getByText('No notifications yet.')).toBeInTheDocument()
    })
  })

  it('closes dropdown when close button is clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 0 })

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.getByText('Notifications')).toBeInTheDocument()
    })

    await user.click(screen.getByText('close'))
    await waitFor(() => {
      expect(screen.queryByText('Notifications')).not.toBeInTheDocument()
    })
  })

  it('closes dropdown when backdrop is clicked', async () => {
    const user = userEvent.setup()
    vi.mocked(api.notifications.unseenCount).mockResolvedValue({ count: 0 })

    renderWithProviders(<NotificationBell />)
    await user.click(screen.getByRole('button', { name: /notifications/i }))

    await waitFor(() => {
      expect(screen.getByText('Notifications')).toBeInTheDocument()
    })

    const backdrop = document.querySelector('.fixed.inset-0.z-30')
    expect(backdrop).toBeInTheDocument()
    if (backdrop) await user.click(backdrop)

    await waitFor(() => {
      expect(screen.queryByText('Notifications')).not.toBeInTheDocument()
    })
  })
})
