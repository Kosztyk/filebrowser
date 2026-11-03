import { fetchURL, fetchJSON, createURL } from "./utils";
export interface BackgroundJob {
  id: string;
  kind: string;
  owner: number;
  state: string;
  phase: string;
  message?: string;
  completed: number;
  total: number;
  created: string;
  retryable: boolean;
  result?: any;
}
export function listJobs() {
  return fetchJSON<BackgroundJob[]>("/api/jobs");
}
export async function waitForJob<T>(
  id: string,
  signal?: AbortSignal
): Promise<T> {
  for (;;) {
    if (signal?.aborted) throw new DOMException("Cancelled", "AbortError");
    const r = await fetchURL(`/api/jobs/${encodeURIComponent(id)}`, { signal });
    const j: BackgroundJob = await r.json();
    if (j.state === "succeeded") return j.result as T;
    if (!["queued", "running"].includes(j.state))
      throw new Error(
        j.message || `Operation ${j.state}. See background jobs.`
      );
    await new Promise<void>((resolve, reject) => {
      const done = () => {
        signal?.removeEventListener("abort", abort);
        resolve();
      };
      const timer = window.setTimeout(done, 750);
      const abort = () => {
        clearTimeout(timer);
        reject(new DOMException("Cancelled", "AbortError"));
      };
      signal?.addEventListener("abort", abort, { once: true });
    });
  }
}
export async function controlJob(id: string, action: string) {
  await fetchURL(`/api/jobs/${id}?action=${action}`, { method: "POST" });
}
export function artifactURL(id: string) {
  return createURL(`api/downloads/${encodeURIComponent(id)}`);
}
