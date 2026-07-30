import { Chip } from '../../components/ui';

export default function EnglishLevelBadge({ level }: { level?: string | null }) {
  if (!level) return null;
  return <Chip>{level}</Chip>;
}
