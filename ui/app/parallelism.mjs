export const PARALLEL_OPTIONS = ['auto', '1', '2', '4', '8', '12', '16', '20', '24', '32', '48', '64'];

export function parallelism(mode, hardwareThreads) {
  const threads = Number.isFinite(hardwareThreads) && hardwareThreads >= 1 ? Math.floor(hardwareThreads) : 4;
  // CPU-capacity estimate, not a measurement of free RAM or current system load.
  const automatic = Math.max(1, Math.min(64, threads - Math.max(1, Math.ceil(threads / 8))));
  return mode !== 'auto' && PARALLEL_OPTIONS.includes(mode) ? Number(mode) : automatic;
}
