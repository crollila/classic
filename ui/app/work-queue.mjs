// Pull work only when a lane becomes free; cancellation never queues the remaining jobs.
export async function runQueue(jobs, concurrency, isCurrent, run) {
  let cursor = 0;
  await Promise.all(Array.from({length: Math.min(concurrency, jobs.length)}, async () => {
    while (isCurrent() && cursor < jobs.length) {
      await run(jobs[cursor++]);
      await new Promise(resolve => setTimeout(resolve, 0));
    }
  }));
}
