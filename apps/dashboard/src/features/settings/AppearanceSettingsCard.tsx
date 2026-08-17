import { Moon, Sun } from 'lucide-react';
import { Switch } from '../../components/ui';
import { useTheme } from '../../lib/theme';

export default function AppearanceSettingsCard() {
  const { theme, setTheme } = useTheme();
  const dark = theme === 'dark';

  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <p className="mb-1 text-sm font-semibold text-foreground">Dark theme</p>
        <p className="text-sm text-muted">Switch the dashboard between light and dark appearance.</p>
      </div>
      <div className="flex items-center gap-2 text-muted">
        <Sun className="h-4 w-4" aria-hidden="true" />
        <Switch checked={dark} onChange={(checked) => setTheme(checked ? 'dark' : 'light')} label="Dark theme" />
        <Moon className="h-4 w-4" aria-hidden="true" />
      </div>
    </div>
  );
}
