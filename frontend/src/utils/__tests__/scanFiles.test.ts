import { describe, expect, it, vi } from "vitest";
import { scanFiles } from "../upload";
vi.mock("@/api", () => ({ files: {} }));
vi.mock("@/api/utils", () => ({ removePrefix: vi.fn() }));
vi.mock("@/stores/layout", () => ({ useLayoutStore: vi.fn() }));
vi.mock("@/stores/upload", () => ({ useUploadStore: vi.fn() }));
vi.mock("@/utils/url", () => ({ default: {} }));
const file = new File(["fixture"], "file.txt", { type: "text/plain" });
const transfer = (items: unknown[], files: File[] = []) =>
  ({ items, files }) as unknown as DataTransfer;
const entryFile = (name: string) => ({
  isFile: true,
  name,
  file: (success: (f: File) => void) =>
    queueMicrotask(() => success(new File(["x"], name))),
});

describe("real drop scanning", () => {
  it("uses getAsFile when the browser returns no filesystem entry", async () => {
    const result = await scanFiles(
      transfer([
        { kind: "file", webkitGetAsEntry: () => null, getAsFile: () => file },
      ])
    );
    expect(result).toMatchObject([
      { name: "file.txt", file, size: 7, isDir: false },
    ]);
  });
  it("normalizes file-only data transfers to upload entries", async () => {
    const result = await scanFiles(transfer([], [file]));
    expect(result[0].file).toBe(file);
    expect(result[0].fullPath).toBe("file.txt");
  });
  it("reads every directory batch and preserves top-level siblings", async () => {
    const batches = [[entryFile("a.txt")], [entryFile("b.txt")], []];
    const directory = {
      isDirectory: true,
      name: "folder",
      createReader: () => ({
        readEntries: (done: (items: unknown[]) => void) =>
          done(batches.shift()!),
      }),
    };
    const result = await scanFiles(
      transfer([
        { kind: "file", webkitGetAsEntry: () => directory },
        { kind: "file", webkitGetAsEntry: () => null, getAsFile: () => file },
      ])
    );
    expect(result.map((item) => item.fullPath).sort()).toEqual([
      "file.txt",
      "folder",
      "folder/a.txt",
      "folder/b.txt",
    ]);
  });
  it("rejects inaccessible entries and settles an empty drop", async () => {
    const error = new Error("Cannot read file");
    await expect(
      scanFiles(
        transfer([
          {
            kind: "file",
            webkitGetAsEntry: () => ({
              isFile: true,
              file: (_done: unknown, fail: (error: Error) => void) =>
                fail(error),
            }),
          },
        ])
      )
    ).rejects.toThrow("Cannot read file");
    await expect(scanFiles(transfer([{ kind: "string" }]))).resolves.toEqual(
      []
    );
  });
});
