import { fetchURL, fetchJSON, removePrefix, createURL } from "./utils";

export interface ShareConfig {
  backend: "filebrowser" | "erugo";
  enabled: boolean;
}

export interface ShareCreateOptions {
  password?: string;
  expires?: string | number;
  unit?: string;
  name?: string;
  description?: string;
  recipientEmail?: string;
  recipientName?: string;
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

export async function create(
  url: string,
  passwordOrOptions: string | ShareCreateOptions = "",
  expires: string | number = "",
  unit = "hours"
) {
  url = removePrefix(url);
  url = `/api/share${url}`;

  let options: ShareCreateOptions;
  if (typeof passwordOrOptions === "object") {
    options = passwordOrOptions;
  } else {
    options = {
      password: passwordOrOptions,
      expires,
      unit,
    };
  }

  const body = JSON.stringify({
    password: options.password ?? "",
    expires: options.expires !== undefined ? options.expires.toString() : "",
    unit: options.unit ?? "hours",
    name: options.name ?? "",
    description: options.description ?? "",
    recipientEmail: options.recipientEmail ?? "",
    recipientName: options.recipientName ?? "",
  });

  return fetchJSON<Share>(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body,
  });
}

export function getShareURL(share: Share) {
  if (share.url) return share.url;
  return createURL("share/" + share.hash, {});
}
