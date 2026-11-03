// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import {
  bookStorageKey,
  migrateBookPosition,
  readBookPosition,
} from "../bookStorage";
describe("book positions", () => {
  beforeEach(() => localStorage.clear());
  it("separates accounts, books and fields", () => {
    expect(
      new Set([
        bookStorageKey(1, "/a"),
        bookStorageKey(2, "/a"),
        bookStorageKey(1, "/b"),
        bookStorageKey(1, "/a", "font"),
      ]).size
    ).toBe(4);
  });
  it("migrates legacy location into only the first opened book", () => {
    localStorage.setItem("book-progress", '"epubcfi(/6/2)"');
    const first = bookStorageKey(1, "/a");
    migrateBookPosition(first);
    migrateBookPosition(bookStorageKey(1, "/b"));
    expect(readBookPosition(localStorage.getItem(first))).toBe("epubcfi(/6/2)");
    expect(localStorage.getItem(bookStorageKey(1, "/b"))).toBeNull();
  });
  it("accepts old numeric and string locations, tolerates corrupt data", () => {
    expect(readBookPosition("17")).toBe(17);
    expect(readBookPosition('"epubcfi(x)"')).toBe("epubcfi(x)");
    expect(readBookPosition("garbage")).toBe(0);
    expect(readBookPosition("{}")).toBe(0);
  });
});
