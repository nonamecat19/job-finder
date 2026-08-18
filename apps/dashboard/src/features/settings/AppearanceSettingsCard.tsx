import { Moon, Sun } from 'lucide-react';
import { Switch } from '../../components/ui';
import { useTheme } from '../../lib/theme';

export default function AppearanceSettingsCard() {
  const { theme, setTheme, resumeDark, setResumeDark } = useTheme();
  const dark = theme === 'dark';

  return (
    <div className="flex flex-col gap-4">
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

      <div className="flex items-center justify-between gap-4 border-t border-border pt-4">
        <div>
          <p className="mb-1 text-sm font-semibold text-foreground">Dark resume preview</p>
          <p className="text-sm text-muted">
            Show resume PDFs on a dark sheet with light text. Only affects previews — downloads and printing keep the
            original document.
          </p>
        </div>
        <Switch checked={resumeDark} onChange={setResumeDark} label="Dark resume preview" />
      </div>
    </div>
  );
}
