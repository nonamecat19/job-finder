import { AppRoutes } from './app/routes';
import { AppShell } from './app/shell';

export default function App() {
  return (
    <AppShell>
      <AppRoutes />
    </AppShell>
  );
}
