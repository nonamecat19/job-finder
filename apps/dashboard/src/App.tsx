import { NavLink, Route, Routes } from 'react-router-dom';
import FeedPage from './pages/FeedPage';
import JobDetailPage from './pages/JobDetailPage';
import ProfilePage from './pages/ProfilePage';
import SourcesPage from './pages/SourcesPage';
import TrackerPage from './pages/TrackerPage';

const tabs = [
  { to: '/', label: 'Feed' },
  { to: '/tracker', label: 'Tracker' },
  { to: '/sources', label: 'Sources' },
  { to: '/profile', label: 'Profile' },
];

export default function App() {
  return (
    <div className="min-h-screen">
      <header className="bg-slate-900 text-white">
        <div className="mx-auto flex max-w-6xl items-center gap-6 px-4 py-3">
          <span className="text-lg font-bold tracking-tight">
            job<span className="text-sky-400">finder</span>
          </span>
          <nav className="flex gap-1">
            {tabs.map((t) => (
              <NavLink
                key={t.to}
                to={t.to}
                end={t.to === '/'}
                className={({ isActive }) =>
                  `rounded px-3 py-1.5 text-sm ${
                    isActive ? 'bg-sky-600 text-white' : 'text-slate-300 hover:bg-slate-800'
                  }`
                }
              >
                {t.label}
              </NavLink>
            ))}
          </nav>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-6">
        <Routes>
          <Route path="/" element={<FeedPage />} />
          <Route path="/jobs/:id" element={<JobDetailPage />} />
          <Route path="/profile" element={<ProfilePage />} />
          <Route path="/sources" element={<SourcesPage />} />
          <Route path="/tracker" element={<TrackerPage />} />
        </Routes>
      </main>
    </div>
  );
}
