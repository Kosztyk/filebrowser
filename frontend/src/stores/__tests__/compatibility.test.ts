// @vitest-environment jsdom
import { createApp } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";
import { describe, expect, it } from "vitest";
import createPinia from "@/stores";
import { useFileStore } from "@/stores/file";

describe("Pinia 4 and Vue Router 5", () => {
  it("installs the router plugin and retains file selection across refreshes", () => {
    const router = createRouter({ history: createMemoryHistory(), routes: [] });
    const pinia = createPinia(router);
    createApp({}).use(pinia);
    const store = useFileStore(pinia);
    expect(store.router).toBe(router);
    const item = { index: 0, url: "/files/a" } as ResourceItem;
    store.updateRequest({ items: [item] } as Resource);
    store.selected = [0];
    store.updateRequest({ items: [{ ...item, index: 1 }] } as Resource);
    expect(store.selected).toEqual([1]);
    store.clearFile();
    expect(store.selectedCount).toBe(0);
    expect(store.req).toBeNull();
  });
});
