export function formatElapsed(ns: number): string {
  if (typeof ns !== 'number' || isNaN(ns)) return '--';
  const totalSeconds = Math.floor(ns / 1e9);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}h${minutes.toString().padStart(2, '0')}m`;
  }
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}