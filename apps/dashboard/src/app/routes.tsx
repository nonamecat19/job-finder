import { Route, Routes } from 'react-router-dom';
import FeedPage from '../features/feed/FeedPage';
import JobDetailPage from '../features/job-detail/JobDetailPage';
import ProfilePage from '../features/profile/ProfilePage';
import SourcesPage from '../features/sources/SourcesPage';
import TrackerPage from '../features/tracker/TrackerPage';

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/" element={<FeedPage />} />
      <Route path="/jobs/:id" element={<JobDetailPage />} />
      <Route path="/profile" element={<ProfilePage />} />
      <Route path="/sources" element={<SourcesPage />} />
      <Route path="/tracker" element={<TrackerPage />} />
    </Routes>
  );
}
