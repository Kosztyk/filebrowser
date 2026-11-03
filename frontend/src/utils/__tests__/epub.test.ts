import { readFile } from "node:fs/promises";
import { describe, expect, it, vi } from "vitest";
import ePub from "epubjs";

describe("EPUB with the maintained xmldom parser", () => {
  it("opens a zipped EPUB, parses its XML and loads the spine and navigation", async () => {
    const bytes = await readFile("tests/fixtures/sample.epub");
    const buffer = bytes.buffer.slice(
      bytes.byteOffset,
      bytes.byteOffset + bytes.byteLength
    );
    // Keep DOMParser absent so epub.js exercises its xmldom fallback. Its
    // archive cleanup uses window.URL even when opening an in-memory book.
    vi.stubGlobal("window", { URL, decodeURIComponent });
    const book = ePub();
    try {
      await book.open(buffer);
      const metadata = await book.loaded.metadata;
      const navigation = await book.loaded.navigation;
      expect(metadata.title).toBe("Compatibility Book");
      expect(book.spine.get(0).href).toBe("chapter.xhtml");
      expect(navigation.toc[0].label).toBe("Chapter One");
      expect(navigation.toc[0].href).toBe("chapter.xhtml");
    } finally {
      book.destroy();
      vi.unstubAllGlobals();
    }
  });
});
