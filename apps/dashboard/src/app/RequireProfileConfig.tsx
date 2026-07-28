import { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { useConfigStatus } from '../features/profile/hooks';
import { EmptyState, LoadingRegion, SkeletonBlock } from '../components/ui';

export function RequireProfileConfig({ children }: { children: ReactNode }) {
  const { data, isLoading } = useConfigStatus();

  if (isLoading) {
    return (
      <LoadingRegion label="checking profile…" className="space-y-3">
        <SkeletonBlock className="h-24 w-full" />
      </LoadingRegion>
    );
  }

  if (!data?.hasConfig) {
    return (
      <EmptyState>
        Set up your profile first — upload a RenderCV config to unlock matching and document
        generation.{' '}
        <Link to="/profile" className="font-semibold text-accent underline">
          Go to profile
        </Link>
      </EmptyState>
    );
  }

  return <>{children}</>;
}
