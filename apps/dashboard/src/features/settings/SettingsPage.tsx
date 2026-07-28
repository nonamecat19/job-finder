import { PageHeader } from '../../components/layout/PageHeader';
import { DashboardGrid, Tile } from '../../components/layout';
import AiFeatureSettingsCard from './AiFeatureSettingsCard';
import LlmSettingsCard from './LlmSettingsCard';

export default function SettingsPage() {
  return (
    <div>
      <PageHeader
        title="Settings"
        description="Manage account-level preferences."
      />
      <DashboardGrid>
        <Tile span="standard" title="AI models">
          <LlmSettingsCard />
        </Tile>
        <Tile span="standard" title="AI features">
          <AiFeatureSettingsCard />
        </Tile>
      </DashboardGrid>
    </div>
  );
}
