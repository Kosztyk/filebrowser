// @vitest-environment jsdom
import { describe, expect, it } from "vitest";
import { mediaSourceType, subtitleLabel } from "@/utils/media";

describe("media URLs", () => {
  it("retains the MKV container hint with query authentication", () => {
    expect(mediaSourceType("/api/raw/movie.MKV?auth=test")).toBe("video/mp4");
    expect(mediaSourceType("/api/raw/movie.webm?auth=test")).toBeUndefined();
  });
  it("labels relative and escaped subtitle paths without throwing", () => {
    expect(subtitleLabel("/api/subtitles/English%20CC.vtt?auth=test")).toBe(
      "English CC"
    );
    expect(subtitleLabel("/api/subtitles/bad%ZZ.vtt")).toBe("bad%ZZ");
  });
});
