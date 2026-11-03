export function mediaSourceType(source: string): string | undefined {
  const path = new URL(source, window.location.origin).pathname;
  // Preserve the previous player's container hint for compatible MKV codecs.
  return /\.mkv$/i.test(path) ? "video/mp4" : undefined;
}

export function subtitleLabel(source: string): string {
  const url = new URL(source, window.location.origin);
  const filename = url.pathname.split("/").pop() || "";
  try {
    return decodeURIComponent(filename).replace(/\.[^/.]+$/, "");
  } catch {
    return filename.replace(/\.[^/.]+$/, "");
  }
}
