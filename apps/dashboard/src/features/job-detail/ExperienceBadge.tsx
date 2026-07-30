import { Chip } from '../../components/ui';

export default function ExperienceBadge({ level, minYears }: { level?: string | null; minYears?: number | null }) {
  if (!level) return null;
  const label = minYears != null ? `${minYears}+ years` : level;
  return <Chip>{label}</Chip>;
}
