import { Trash2 } from 'lucide-react';
import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import { Button } from '../../components/ui';
import { useClearJobs } from '../feed/hooks';
import AiFeatureSettingsCard from './AiFeatureSettingsCard';
import AppearanceSettingsCard from './AppearanceSettingsCard';
import ResumeShapeCard from './ResumeShapeCard';

export default function SettingsPage() {
  const clear = useClearJobs();

  const onClearAll = () => {
    if (window.confirm('Delete ALL vacancies and their match results, documents, and applications? This cannot be undone.')) {
      clear.mutate();
    }
  };

  return (
    <div>
      <PageHeader
        title="Settings"
        description="Manage account-level preferences."
      />
      <DashboardGrid>
        <Tile span="standard" title="Appearance">
          <AppearanceSettingsCard />
        </Tile>
        <Tile span="standard" title="AI features">
          <AiFeatureSettingsCard />
        </Tile>
        <Tile span="standard" title="Resume shape">
          <ResumeShapeCard />
        </Tile>
        <Tile span="standard" title="Danger zone">
          <p className="text-sm text-muted">
            Permanently delete all vacancies along with their match results, documents, and applications.
          </p>
          <Button variant="danger" className="mt-3" onClick={onClearAll} disabled={clear.isPending}>
            <Trash2 className="h-4 w-4" aria-hidden="true" />
            {clear.isPending ? 'clearing…' : 'clear all jobs'}
          </Button>
        </Tile>
      </DashboardGrid>
    </div>
  );
}
