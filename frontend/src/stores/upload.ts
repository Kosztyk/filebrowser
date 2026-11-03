import { defineStore } from "pinia";
import { computed, inject, markRaw, ref, watch } from "vue";
import { useAuthStore } from "./auth";
import { useFileStore } from "./file";
import { files as api } from "@/api";
import { fetchJSON, removePrefix, StatusError } from "@/api/utils";
import * as transfers from "@/api/tus";
import { tusSettings } from "@/utils/constants";
import { encodePath } from "@/utils/url";
import buttons from "@/utils/buttons";
let sequence = 0;
const processing = new Set(["received", "scanning", "committing"]),
  working = new Set(["queued", "uploading", "pausing", ...processing]);
const beforeUnload = (event: BeforeUnloadEvent) => {
  event.preventDefault();
};
export const useUploadStore = defineStore("upload", () => {
  const showError = inject<IToastError>("$showError"),
    auth = useAuthStore(),
    allUploads = ref<Upload[]>([]),
    running = ref(new Set<string>());
  let initializedFor: number | undefined,
    pulse: number | undefined,
    polling = false,
    pollAbort: AbortController | undefined;
  const activeUploads = computed(
    () => new Set(allUploads.value.filter((item) => working.has(item.status)))
  );
  const totalBytes = computed(() =>
      [...activeUploads.value].reduce((n, i) => n + i.totalBytes, 0)
    ),
    sentBytes = computed(() =>
      [...activeUploads.value].reduce((n, i) => n + i.sentBytes, 0)
    ),
    pendingUploadCount = computed(() => activeUploads.value.size);
  function applyInfo(item: Upload, info: ServerUpload) {
    item.serverID = info.id;
    item.sentBytes = info.offset;
    item.rawProgress.sentBytes = info.offset;
    item.scan = info.scan;
    item.message = info.message || info.scan.message;
    item.modified = info.modified;
    item.canRetry = info.canRetry;
    item.status =
      ["uploading", "paused"].includes(info.status) &&
      !item.file &&
      !running.value.has(item.id)
        ? "needs_file"
        : info.status;
    if (info.status === "ready") item.file = null;
  }
  function progress(
    item: Upload,
    event: { loaded: number; state?: string; info?: ServerUpload; id?: string }
  ) {
    item.rawProgress.sentBytes = event.loaded;
    if (event.id) item.serverID = event.id;
    if (event.state) item.status = event.state;
    if (event.info) applyInfo(item, event.info);
  }
  function reloadFiles() {
    useFileStore().reload = true;
  }
  async function run(item: Upload, retryProcessing = false) {
    const owner = auth.user?.id;
    running.value.add(item.id);
    item.status = retryProcessing ? "received" : "uploading";
    item.message = "";
    const report = (e: {
      loaded: number;
      state?: string;
      info?: ServerUpload;
      id?: string;
    }) => progress(item, e);
    try {
      if (retryProcessing && item.serverID) {
        const r = await transfers.retryProcessing(item.serverID, report);
        if (owner === auth.user?.id) applyInfo(item, r);
      } else {
        const r = await api.post(
          item.path,
          item.file || "",
          item.overwrite,
          report,
          item.id,
          item.serverID
        );
        if (owner !== auth.user?.id) return;
        if (r && typeof r === "object" && "scan" in r)
          applyInfo(item, r as ServerUpload);
        else {
          item.status = "ready";
          item.sentBytes = item.totalBytes;
          item.file = null;
        }
      }
      reloadFiles();
    } catch (error) {
      if (owner !== auth.user?.id) return;
      if (error instanceof transfers.UploadProblem) {
        if (error.info) applyInfo(item, error.info);
        else item.status = error.kind;
        if (error.info?.status === "ready") {
          reloadFiles();
          return;
        }
        if (["paused", "cancelled"].includes(error.kind)) return;
      } else item.status = "failed";
      if (
        item.serverID &&
        !(error instanceof transfers.UploadProblem && error.info)
      ) {
        try {
          const info = await fetchJSON<ServerUpload>(
            "/api/uploads/" + item.serverID
          );
          if (owner !== auth.user?.id) return;
          applyInfo(item, info);
          if (["uploading", "paused"].includes(info.status))
            item.status = "paused";
        } catch {
          item.status = "failed";
        }
      }
      item.message = error instanceof Error ? error.message : String(error);
      showError?.(error instanceof Error ? error : String(error));
    } finally {
      running.value.delete(item.id);
      processQueue();
    }
  }
  function processQueue() {
    const limit = tusSettings?.concurrentUploads || 5;
    for (const item of allUploads.value) {
      if (running.value.size >= limit) break;
      if (item.status === "queued") void run(item);
    }
    if (
      !running.value.size &&
      !allUploads.value.some((i) => i.status === "queued")
    )
      buttons.done("upload");
  }
  function upload(
    path: string,
    name: string,
    file: File | null,
    overwrite: boolean,
    type: ResourceType
  ) {
    const previous = file ? resumeFor(path, file) : undefined;
    if (previous && file) {
      previous.file = file;
      resume(previous);
      return;
    }
    allUploads.value.push({
      id: "local-" + Date.now() + "-" + ++sequence,
      path,
      name,
      file,
      overwrite,
      type,
      totalBytes: file?.size || 0,
      sentBytes: 0,
      status: "queued",
      rawProgress: markRaw({ sentBytes: 0 }),
    });
    buttons.loading("upload");
    processQueue();
  }
  async function pause(item: Upload) {
    item.status = "pausing";
    try {
      await transfers.pause(item.id);
      if (item.status === "pausing") item.status = "paused";
    } catch (error) {
      item.status = "failed";
      item.message = String(error);
    }
  }
  function resume(item: Upload) {
    if (item.file) {
      item.status = "queued";
      processQueue();
    }
  }
  function selectFile(item: Upload, file: File) {
    if (
      file.size !== item.totalBytes ||
      (item.modified && String(file.lastModified) !== item.modified)
    )
      throw new Error(
        "Select the same original file (size and modification time must match)"
      );
    item.file = file;
    resume(item);
  }
  async function retry(item: Upload) {
    if (item.canRetry && item.serverID) await run(item, true);
    else if (item.file) resume(item);
    else item.status = "needs_file";
  }
  async function cancel(item: Upload) {
    try {
      const result = await transfers.cancel(item.id, item.serverID);
      if (result?.status === "ready") {
        applyInfo(item, result);
        reloadFiles();
        return;
      }
      item.status = "cancelled";
      item.file = null;
      item.message = "";
    } catch (error) {
      item.status = "failed";
      item.message = String(error);
    }
  }
  async function abort() {
    for (const i of allUploads.value)
      if (i.status === "queued") {
        i.status = "cancelled";
        i.file = null;
      }
    for (const i of allUploads.value.filter(
      (i) => !["ready", "cancelled", "expired"].includes(i.status)
    ))
      await cancel(i);
  }
  async function clearCompleted() {
    for (const i of [...allUploads.value]) {
      if (!["ready", "cancelled", "expired"].includes(i.status)) continue;
      if (i.serverID) {
        try {
          await transfers.cancel(i.id, i.serverID, true);
        } catch {
          continue;
        }
      }
      allUploads.value = allUploads.value.filter((row) => row.id !== i.id);
    }
  }
  async function initialize() {
    if (!auth.user?.id || initializedFor === auth.user.id) return;
    const owner = auth.user.id;
    initializedFor = owner;
    try {
      const list = await fetchJSON<ServerUpload[]>("/api/uploads");
      if (auth.user?.id !== owner) return;
      for (const info of list.reverse()) {
        if (allUploads.value.some((i) => i.serverID === info.id)) continue;
        const item: Upload = {
          id: "server-" + info.id,
          path: "/files" + encodePath(info.path),
          name: info.name,
          file: null,
          type: "blob",
          overwrite: true,
          totalBytes: info.length,
          sentBytes: info.offset,
          status: info.status,
          serverID: info.id,
          rawProgress: markRaw({ sentBytes: info.offset }),
        };
        applyInfo(item, info);
        allUploads.value.push(item);
      }
    } catch {
      initializedFor = undefined;
    }
  }
  async function poll() {
    if (polling || !auth.user?.id) return;
    const rows = allUploads.value.filter(
      (i) =>
        i.serverID &&
        !running.value.has(i.id) &&
        (processing.has(i.status) || i.status === "needs_file")
    );
    if (!rows.length) return;
    const owner = auth.user.id;
    polling = true;
    pollAbort = new AbortController();
    try {
      for (const i of rows) {
        let info: ServerUpload;
        try {
          info = await fetchJSON<ServerUpload>("/api/uploads/" + i.serverID, {
            signal: pollAbort.signal,
          });
        } catch (error) {
          if (
            error instanceof StatusError &&
            [404, 410].includes(error.status || 0)
          ) {
            i.status = "expired";
            i.file = null;
            continue;
          }
          throw error;
        }
        if (auth.user?.id !== owner) return;
        applyInfo(i, info);
        if (i.status === "ready") reloadFiles();
      }
    } catch {
      /* Retain checkpoints on disconnection. */
    } finally {
      polling = false;
    }
  }
  watch(
    () => auth.user?.id,
    () => {
      transfers.detachAll();
      pollAbort?.abort();
      allUploads.value = [];
      running.value = new Set();
      initializedFor = undefined;
      void initialize();
    }
  );
  watch(
    () => activeUploads.value.size,
    (count) => {
      window.removeEventListener("beforeunload", beforeUnload);
      if (count) window.addEventListener("beforeunload", beforeUnload);
    }
  );
  function start() {
    if (pulse === undefined)
      pulse = window.setInterval(() => {
        for (const i of allUploads.value)
          if (running.value.has(i.id))
            i.sentBytes = Math.min(i.totalBytes, i.rawProgress.sentBytes);
        void poll();
      }, 1000);
    void initialize();
  }
  function logicalPath(path: string) {
    try {
      return decodeURIComponent(removePrefix(path));
    } catch {
      return removePrefix(path);
    }
  }
  function resumeFor(path: string, file: File) {
    return allUploads.value.find(
      (i) =>
        i.serverID &&
        logicalPath(i.path) === logicalPath(path) &&
        i.totalBytes === file.size &&
        i.modified === String(file.lastModified) &&
        ["needs_file", "paused"].includes(i.status)
    );
  }
  return {
    allUploads,
    activeUploads,
    totalBytes,
    sentBytes,
    pendingUploadCount,
    upload,
    pause,
    resume,
    selectFile,
    retry,
    cancel,
    abort,
    clearCompleted,
    start,
    resumeFor,
  };
});
