import { PageHeader, SectionTitle } from '../../components/layout/PageHeader';
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
      <section>
        <SectionTitle>AI models</SectionTitle>
        <LlmSettingsCard />
      </section>
      <section>
        <SectionTitle>AI features</SectionTitle>
        <AiFeatureSettingsCard />
      </section>
    </div>
  );
}
