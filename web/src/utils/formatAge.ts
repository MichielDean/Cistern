export function formatAge(iso: string): string {
  const created = new Date(iso).getTime();
  if (isNaN(created)) return '--';
  const now = Date.now();
  const diff = now - created;
  if (diff < 0) return '0m';
  const minutes = Math.floor(diff / 60000);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}