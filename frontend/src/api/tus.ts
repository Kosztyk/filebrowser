import * as tus from "tus-js-client";
import { jwtDecode } from "jwt-decode";
import { baseURL, tusEndpoint, tusSettings, origin } from "@/utils/constants";
import { useAuthStore } from "@/stores/auth";
import { renew } from "@/utils/auth";
import { fetchJSON, fetchURL, removePrefix, StatusError } from "@/api/utils";
export class UploadProblem extends Error {
  constructor(
    message: string,
    public kind: "paused" | "cancelled" | "failed",
    public info?: ServerUpload
  ) {
    super(message);
  }
}
type Progress = (event: {
  loaded: number;
  state?: string;
  info?: ServerUpload;
  id?: string;
}) => void;
type Control = {
  upload: tus.Upload;
  stop: AbortController;
  reject: (error: Error) => void;
  progress: Progress;
};
const controls = new Map<string, Control>();
let renewal: Promise<void> | null = null;
async function credentials(force = false) {
  const auth = useAuthStore();
  let soon = false;
  try {
    soon =
      jwtDecode<{ exp: number }>(auth.jwt).exp * 1000 - Date.now() < 300000;
  } catch {
    /* The server validates credentials. */
  }
  if ((soon || force) && !renewal)
    renewal = renew(auth.jwt).finally(() => {
      renewal = null;
    });
  if (renewal) await renewal;
  return auth.jwt;
}
export function sessionID(value?: string | null) {
  if (!value) return;
  try {
    const u = new URL(value, origin),
      prefix = `${baseURL}/api/uploads/`;
    if (
      u.origin !== origin ||
      u.search ||
      u.hash ||
      !u.pathname.startsWith(prefix)
    )
      return;
    const id = u.pathname.slice(prefix.length);
    return /^[a-f0-9]{32}$/.test(id) ? id : undefined;
  } catch {
    return;
  }
}
function trusted(value: string) {
  const u = new URL(value, origin);
  return (
    u.origin === origin &&
    (u.pathname.startsWith(`${baseURL}/api/tus/`) || Boolean(sessionID(value)))
  );
}
function delay(signal?: AbortSignal) {
  return new Promise<void>((resolve, reject) => {
    const abort = () => {
      clearTimeout(timer);
      reject(new DOMException("Paused", "AbortError"));
    };
    const timer = window.setTimeout(() => {
      signal?.removeEventListener("abort", abort);
      resolve();
    }, 1000);
    if (signal?.aborted) abort();
    else signal?.addEventListener("abort", abort, { once: true });
  });
}
export async function waitForPublication(
  id: string,
  progress: Progress,
  signal?: AbortSignal
): Promise<ServerUpload> {
  for (;;) {
    await credentials();
    const info = await fetchJSON<ServerUpload>(`/api/uploads/${id}`, {
      signal,
    });
    progress({ loaded: info.offset, state: info.status, info, id });
    if (info.status === "ready") return info;
    if (["failed", "infected", "expired", "cancelled"].includes(info.status))
      throw new UploadProblem(
        info.message || info.scan.message || "Upload was not published",
        info.status === "cancelled" ? "cancelled" : "failed",
        info
      );
    await delay(signal);
  }
}
export async function upload(
  filePath: string,
  content: ApiContent = "",
  overwrite = false,
  report: Progress = () => {},
  key?: string,
  resumeID?: string
) {
  if (!tusSettings || !(content instanceof Blob))
    throw new Error("Resumable uploads require a file");
  filePath = removePrefix(filePath);
  const controlKey = key || filePath,
    auth = useAuthStore(),
    modified = content instanceof File ? String(content.lastModified) : "0",
    filename = content instanceof File ? content.name : "file";
  const fingerprint = JSON.stringify([
      "filebrowser-v2",
      origin,
      baseURL,
      auth.user?.id,
      filePath,
      filename,
      content.size,
      modified,
      overwrite,
    ]),
    stop = new AbortController();
  return new Promise<ServerUpload>((resolve, reject) => {
    const instance = new tus.Upload(content, {
      endpoint: `${origin}${baseURL}${tusEndpoint}${filePath}?override=${overwrite}`,
      uploadUrl: resumeID
        ? `${origin}${baseURL}/api/uploads/${resumeID}`
        : undefined,
      metadata: { filename, modified },
      chunkSize: tusSettings.chunkSize,
      parallelUploads: 1,
      retryDelays:
        tusSettings.retryCount > 0
          ? Array.from({ length: tusSettings.retryCount }, (_, i) =>
              i === 0 ? 0 : Math.min(1000 * 2 ** (i - 1), 20000)
            )
          : undefined,
      fingerprint: async () => fingerprint,
      storeFingerprintForResuming: true,
      removeFingerprintOnSuccess: true,
      onBeforeRequest: async (request) => {
        if (!trusted(request.getURL()))
          throw new Error("Refusing credentials outside this upload service");
        request.setHeader("X-Auth", await credentials());
      },
      onAfterResponse: async (_request, response) => {
        const id =
          sessionID(instance.url) || sessionID(response.getHeader("Location"));
        if (id)
          report({
            loaded: Number(response.getHeader("Upload-Offset") || 0),
            state: response.getHeader("Upload-State") || "uploading",
            id,
          });
        if (response.getHeader("X-Renew-Token") === "true")
          await credentials(true);
      },
      onShouldRetry: (error) => {
        const s = error.originalResponse?.getStatus() || 0;
        return s === 409
          ? error.originalRequest?.getMethod() === "PATCH"
          : s === 0 || (s >= 500 && s !== 507);
      },
      onUploadUrlAvailable: () => {
        const id = sessionID(instance.url);
        if (id) report({ loaded: 0, id });
      },
      onProgress: (loaded) => report({ loaded }),
      onError: (error) => {
        controls.delete(controlKey);
        reject(
          new UploadProblem(
            error instanceof tus.DetailedError
              ? error.originalResponse?.getBody() || error.message
              : error.message,
            "failed"
          )
        );
      },
      onSuccess: () => {
        const id = sessionID(instance.url);
        if (!id) {
          controls.delete(controlKey);
          reject(new Error("Upload has no session ID"));
          return;
        }
        report({ loaded: content.size, state: "received", id });
        waitForPublication(id, report, stop.signal)
          .then(resolve, reject)
          .finally(() => controls.delete(controlKey));
      },
    });
    controls.set(controlKey, {
      upload: instance,
      stop,
      reject,
      progress: report,
    });
    const start = async () => {
      if (!resumeID) {
        const previous = await instance.findPreviousUploads().catch(() => []),
          match = previous
            .filter((item) => sessionID(item.uploadUrl))
            .sort((a, b) => b.creationTime.localeCompare(a.creationTime))[0];
        if (match) instance.resumeFromPreviousUpload(match);
      }
      if (!stop.signal.aborted) instance.start();
    };
    start().catch((error) => {
      controls.delete(controlKey);
      reject(error);
    });
  });
}
export async function pause(key: string) {
  const c = controls.get(key);
  if (!c) return;
  c.stop.abort();
  await c.upload.abort(false);
  const id = sessionID(c.upload.url);
  try {
    if (id) {
      const info = await fetchJSON<ServerUpload>(`/api/uploads/${id}/pause`, {
        method: "POST",
      });
      c.progress({ loaded: info.offset, state: info.status, info, id });
    }
    c.reject(new UploadProblem("Upload paused", "paused"));
  } catch (error) {
    c.reject(error instanceof Error ? error : new Error(String(error)));
    throw error;
  } finally {
    controls.delete(key);
  }
}
export async function cancel(
  key: string,
  knownID?: string,
  dismiss = false
): Promise<ServerUpload | undefined> {
  const c = controls.get(key);
  if (c) {
    c.stop.abort();
    await c.upload.abort(false);
  }
  const id = knownID || sessionID(c?.upload.url);
  try {
    if (id)
      await fetchURL(`/api/uploads/${id}${dismiss ? "?dismiss=true" : ""}`, {
        method: "DELETE",
      });
    c?.reject(new UploadProblem("Upload cancelled", "cancelled"));
  } catch (error) {
    if (
      !dismiss &&
      id &&
      error instanceof StatusError &&
      error.status === 409
    ) {
      try {
        const info = await fetchJSON<ServerUpload>(`/api/uploads/${id}`);
        if (info.status === "ready") {
          c?.reject(
            new UploadProblem("File already published", "failed", info)
          );
          return info;
        }
      } catch (lookup) {
        c?.reject(lookup instanceof Error ? lookup : new Error(String(lookup)));
        throw lookup;
      }
    }
    if (!(error instanceof StatusError && error.status === 404)) {
      c?.reject(error instanceof Error ? error : new Error(String(error)));
      throw error;
    }
    c?.reject(new UploadProblem("Upload expired", "cancelled"));
  } finally {
    controls.delete(key);
  }
}
export function detachAll() {
  for (const [key, c] of controls) {
    c.stop.abort();
    void c.upload.abort(false);
    c.reject(new UploadProblem("Upload paused", "paused"));
    controls.delete(key);
  }
}
export function abortAllUploads() {
  detachAll();
}
export async function retryProcessing(id: string, progress: Progress) {
  await fetchJSON(`/api/uploads/${id}/retry`, { method: "POST" });
  return waitForPublication(id, progress);
}
export async function useTus(content: ApiContent) {
  return tus.isSupported && content instanceof Blob;
}
