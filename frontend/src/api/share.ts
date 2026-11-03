import { waitForJob } from "./jobs";
import { fetchURL, fetchJSON, removePrefix, createURL } from "./utils";

export interface ShareConfig {
  backend: "filebrowser" | "erugo";
  enabled: boolean;
  reverseShare: boolean;
}

export interface ShareCreateOptions {
  password?: string;
  expires?: string | number;
  unit?: string;
  name?: string;
  description?: string;
  recipientEmail?: string;
  recipientName?: string;
  downloadSpeedLimitKbps?: number;
}

export interface ErugoReverseShareOptions {
  recipientEmail: string;
  recipientName: string;
  message?: string;
  uploadSpeedLimitKbps?: number;
}

export interface ErugoReverseShareResponse {
  status: string;
  message: string;
  inviteId: number;
  recipientEmail: string;
  recipientName: string;
  uploadSpeedLimitKbps: number;
  expiresAt?: string;
}

export async function list() {
  return fetchJSON<Share[]>("/api/shares");
}

export async function config() {
  return fetchJSON<ShareConfig>("/api/share-config");
}

export async function get(url: string) {
  url = removePrefix(url);
  return fetchJSON<Share[]>(`/api/share${url}`);
}

export async function remove(hash: string) {
  await fetchURL(`/api/share/${hash}`, {
    method: "DELETE",
  });
}

/**
 * Create a built-in File Browser share using the original upstream request
 * shape. Keeping this separate from Erugo prevents the optional integration
 * from changing or breaking the developer-provided share implementation.
 */
export async function createNative(
  url: string,
  password = "",
  expires: string | number = "",
  unit = "hours"
) {
  url = removePrefix(url);
  url = `/api/share${url}`;

  if (expires !== "") {
    url += `?expires=${encodeURIComponent(expires.toString())}&unit=${encodeURIComponent(unit)}`;
  }

  let body = "{}";
  if (password !== "" || expires !== "" || unit !== "hours") {
    body = JSON.stringify({
      password,
      expires: expires.toString(),
      unit,
    });
  }

  return fetchJSON<Share>(url, {
    method: "POST",
    body,
  });
}

/** Create an external Erugo share. */
export async function createErugo(url: string, options: ShareCreateOptions) {
  url = removePrefix(url);
  url = `/api/share${url}`;

  const body = JSON.stringify({
    password: options.password ?? "",
    expires: options.expires !== undefined ? options.expires.toString() : "",
    unit: options.unit ?? "hours",
    name: options.name ?? "",
    description: options.description ?? "",
    recipientEmail: options.recipientEmail ?? "",
    recipientName: options.recipientName ?? "",
    downloadSpeedLimitKbps: options.downloadSpeedLimitKbps ?? 0,
  });

  const result = await fetchJSON<Share & { jobId?: string }>(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body,
  });
  return result.jobId ? waitForJob<Share>(result.jobId) : result;
}

/** Create an Erugo reverse-share invitation through File Browser's backend. */
export async function createErugoReverse(options: ErugoReverseShareOptions) {
  return fetchJSON<ErugoReverseShareResponse>("/api/erugo/reverse-share", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      recipientEmail: options.recipientEmail,
      recipientName: options.recipientName,
      message: options.message ?? "",
      uploadSpeedLimitKbps: options.uploadSpeedLimitKbps ?? 0,
    }),
  });
}

/** Backward-compatible wrapper for any existing callers. */
export async function create(
  url: string,
  passwordOrOptions: string | ShareCreateOptions = "",
  expires: string | number = "",
  unit = "hours"
) {
  if (typeof passwordOrOptions === "object") {
    return createErugo(url, passwordOrOptions);
  }

  return createNative(url, passwordOrOptions, expires, unit);
}

export function getShareURL(share: Share) {
  if (share.url) return share.url;
  return createURL("share/" + share.hash, {});
}
