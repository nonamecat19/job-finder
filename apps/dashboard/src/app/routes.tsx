import { Route, Routes } from 'react-router-dom';
import { RequireProfileConfig } from './RequireProfileConfig';
import ContactsPage from '../features/contacts/ContactsPage';
import FeedPage from '../features/feed/FeedPage';
import JobDetailPage from '../features/job-detail/JobDetailPage';
import ProfilePage from '../features/profile/ProfilePage';
import SourcesPage from '../features/sources/SourcesPage';
import StatusPage from '../features/status/StatusPage';
import TailorPage from '../features/tailor/TailorPage';
import TrackerPage from '../features/tracker/TrackerPage';

export function AppRoutes() {
  return (
    <Routes>
      <Route
        path="/"
        element={
          <RequireProfileConfig>
            <FeedPage />
          </RequireProfileConfig>
        }
      />
      <Route
        path="/jobs/:id"
        element={
          <RequireProfileConfig>
            <JobDetailPage />
          </RequireProfileConfig>
        }
      />
      <Route path="/profile" element={<ProfilePage />} />
      <Route path="/contacts" element={<ContactsPage />} />
      <Route path="/tailor" element={<TailorPage />} />
      <Route path="/sources" element={<SourcesPage />} />
      <Route path="/status" element={<StatusPage />} />
      <Route
        path="/tracker"
        element={
          <RequireProfileConfig>
            <TrackerPage />
          </RequireProfileConfig>
        }
      />
    </Routes>
  );
}
