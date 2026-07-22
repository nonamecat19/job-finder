export function postAgeLabel(postedAt: string | null | undefined): string | null {
  if (!postedAt) return null;
  const now = Date.now();
  const posted = new Date(postedAt).getTime();
  if (isNaN(posted)) return null;
  const diffMs = now - posted;
  if (diffMs < 0) return null;
  const hours = Math.floor(diffMs / 3600000);
  if (hours < 1) return 'just now';
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  if (days < 30) return `${days}d ago`;
  const months = Math.floor(days / 30);
  return `${months}mo ago`;
}
