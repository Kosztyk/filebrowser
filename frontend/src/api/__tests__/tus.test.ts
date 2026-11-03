// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
const m = vi.hoisted(() => ({
  auth: { jwt: "old", user: { id: 1 } },
  instances: [] as Array<{ options: Record<string, any>; url: string }>,
  json: vi.fn(),
  fetch: vi.fn(),
  renew: vi.fn(),
}));
vi.mock("@/utils/constants", () => ({
  baseURL: "/fb",
  origin: "https://files.example",
  tusEndpoint: "/api/tus",
  tusSettings: { chunkSize: 5, retryCount: 2 },
}));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => m.auth }));
vi.mock("@/utils/auth", () => ({ renew: m.renew }));
vi.mock("jwt-decode", () => ({
  jwtDecode: () => ({ exp: Date.now() / 1000 + 3600 }),
}));
vi.mock("@/api/utils", () => ({
  fetchJSON: m.json,
  fetchURL: m.fetch,
  removePrefix: (v: string) => v.replace(/^\/files/, ""),
  StatusError: class extends Error {
    constructor(
      message: string,
      public status: number
    ) {
      super(message);
    }
  },
}));
vi.mock("tus-js-client", () => ({
  isSupported: true,
  Upload: class {
    url = "https://files.example/fb/api/uploads/" + "a".repeat(32);
    constructor(
      _file: Blob,
      public options: Record<string, any>
    ) {
      m.instances.push(this);
    }
    async abort() {}
    start() {}
    async findPreviousUploads() {
      return [];
    }
    resumeFromPreviousUpload() {}
  },
  DetailedError: class extends Error {},
}));
import {
  cancel,
  detachAll,
  sessionID,
  upload,
  waitForPublication,
} from "../tus";
import { StatusError } from "@/api/utils";
const id = "a".repeat(32),
  info = (status: string) => ({
    id,
    offset: 5,
    length: 5,
    status,
    scan: { status: "pending" },
  });
beforeEach(() => {
  vi.clearAllMocks();
  m.instances.length = 0;
  m.auth.jwt = "old";
  m.auth.user.id = 1;
});
afterEach(() => {
  detachAll();
  vi.useRealTimers();
});
describe("durable upload controls", () => {
  it("trusts only local upload URLs", () => {
    expect(sessionID(`/fb/api/uploads/${id}`)).toBe(id);
    for (const value of [
      `https://evil.example/fb/api/uploads/${id}`,
      `/fb/api/uploads/${id}?x=1`,
      `/fb/api/uploads/${id}/more`,
      "/api/uploads/" + id,
    ])
      expect(sessionID(value)).toBeUndefined();
  });
  it("uses current credentials for every request", async () => {
    const pending = upload(
        "/files/test",
        new File(["hello"], "test"),
        false,
        undefined,
        "key"
      ).catch(() => {}),
      i = m.instances[0],
      setHeader = vi.fn();
    m.auth.jwt = "new";
    await i.options.onBeforeRequest({ getURL: () => i.url, setHeader });
    expect(setHeader).toHaveBeenCalledWith("X-Auth", "new");
    setHeader.mockClear();
    await expect(
      i.options.onBeforeRequest({
        getURL: () => "https://evil.example/upload",
        setHeader,
      })
    ).rejects.toThrow(/credentials/);
    expect(setHeader).not.toHaveBeenCalled();
    detachAll();
    await pending;
  });
  it("scopes fingerprints to account and destination", async () => {
    const file = new File(["hello"], "test", { lastModified: 123 }),
      a = upload("/files/a", file, false, undefined, "a").catch(() => {}),
      first = await m.instances[0].options.fingerprint();
    m.auth.user.id = 2;
    const b = upload("/files/a", file, false, undefined, "b").catch(() => {}),
      second = await m.instances[1].options.fingerprint(),
      c = upload("/files/b", file, false, undefined, "c").catch(() => {});
    expect(first).not.toBe(second);
    expect(second).not.toBe(await m.instances[2].options.fingerprint());
    detachAll();
    await Promise.all([a, b, c]);
  });
  it("settles cancel when reconciliation fails", async () => {
    const p = upload(
        "/files/test",
        new File(["hello"], "test"),
        false,
        undefined,
        "key"
      ),
      assertion = expect(p).rejects.toThrow("status unavailable");
    m.fetch.mockRejectedValue(new StatusError("published", 409));
    m.json.mockRejectedValue(new Error("status unavailable"));
    await expect(cancel("key", id)).rejects.toThrow("status unavailable");
    await assertion;
  });
  it("reports a publication that completed before cancellation", async () => {
    const p = upload(
        "/files/test",
        new File(["hello"], "test"),
        false,
        undefined,
        "key"
      ),
      assertion = expect(p).rejects.toMatchObject({
        info: { status: "ready" },
      });
    m.fetch.mockRejectedValue(new StatusError("published", 409));
    m.json.mockResolvedValue(info("ready"));
    expect((await cancel("key", id))?.status).toBe("ready");
    await assertion;
  });
  it("keeps scanning distinct from ready and unknown results", async () => {
    vi.useFakeTimers();
    const progress = vi.fn();
    m.json
      .mockResolvedValueOnce(info("scanning"))
      .mockResolvedValueOnce(info("ready"));
    const p = waitForPublication(id, progress);
    await vi.advanceTimersByTimeAsync(1000);
    expect((await p).status).toBe("ready");
    expect(progress.mock.calls.map((c) => c[0].state)).toEqual([
      "scanning",
      "ready",
    ]);
    m.json.mockResolvedValue({
      ...info("failed"),
      scan: { status: "unknown", message: "Unknown response" },
    });
    await expect(waitForPublication(id, progress)).rejects.toMatchObject({
      info: { scan: { status: "unknown" } },
    });
  });
});
