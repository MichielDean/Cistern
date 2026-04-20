export const MAX_BUFFER_SIZE = 50 * 1024;

export function truncateBuffer(prev: string, chunk: string): string {
  const next = prev + chunk;
  if (next.length > MAX_BUFFER_SIZE) {
    return next.slice(next.length - MAX_BUFFER_SIZE);
  }
  return next;
}

export function isAuthCloseCode(code: number): boolean {
  return code === 1008 || code === 4001;
}