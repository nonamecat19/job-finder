import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useConfigStatus } from '../features/profile/hooks';
import { EmptyState, Spinner } from '../components/ui';

export function RequireProfileConfig({ children }: { children: ReactNode }) {
  const { data, isLoading } = useConfigStatus();

  if (isLoading) return <Spinner label="checking profile…" />;

  if (!data?.hasConfig) {
    return (
      <EmptyState>
        Set up your profile first — upload a RenderCV config to unlock matching and document
        generation.{' '}
        <Link to="/profile" className="font-semibold text-primary underline">
          Go to profile
        </Link>
      </EmptyState>
    );
  }

  return <>{children}</>;
}
