import { afterEach, expect, it, vi } from "vitest";
import { operationID } from "../operationId";
afterEach(() => vi.unstubAllGlobals());
it("creates operation keys when randomUUID is unavailable on an HTTP LAN origin", () => {
  vi.stubGlobal("crypto", {
    getRandomValues: (a: Uint8Array) => {
      a.fill(171);
      return a;
    },
  });
  expect(operationID()).toBe("ab".repeat(16));
});
