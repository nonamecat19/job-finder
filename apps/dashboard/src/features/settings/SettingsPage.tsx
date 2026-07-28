import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
import { DashboardGrid, GridCard } from '../../components/layout';
import { Surface } from '../../components/ui';
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
        <GridCard span="narrow">
          <section>
            <SectionTitle>AI models</SectionTitle>
            <LlmSettingsCard />
          </section>
        </GridCard>
        <GridCard span="narrow">
          <section>
            <SectionTitle>AI features</SectionTitle>
            <AiFeatureSettingsCard />
          </section>
        </GridCard>
      </DashboardGrid>
    </div>
  );
}
