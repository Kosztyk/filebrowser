export function bookStorageKey(
  user: string | number | undefined,
  path: string,
  field = "position"
) {
  return `filebrowser:book:${encodeURIComponent(String(user ?? "guest"))}:${encodeURIComponent(path)}:${field}`;
}
export function readBookPosition(raw: string | null): string | number {
  if (raw === null) return 0;
  try {
    const v = JSON.parse(raw);
    return typeof v === "string" ||
      (typeof v === "number" && Number.isFinite(v))
      ? v
      : 0;
  } catch {
    return raw.startsWith("epubcfi(") ? raw : 0;
  }
}
export function migrateBookPosition(key: string) {
  if (localStorage.getItem(key) !== null) return;
  const old = localStorage.getItem("book-progress");
  if (old !== null) {
    localStorage.setItem(key, JSON.stringify(readBookPosition(old)));
    localStorage.removeItem("book-progress");
  }
}
