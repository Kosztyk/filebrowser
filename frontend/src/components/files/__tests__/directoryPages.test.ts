// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createApp, nextTick } from "vue";
import { createPinia, setActivePinia } from "pinia";
import { createI18n } from "vue-i18n";
import DirectoryPages from "../DirectoryPages.vue";
import { useFileStore } from "@/stores/file";
import english from "@/i18n/en.json";
const { fetchPage, updateUser, auth } = vi.hoisted(() => ({
  fetchPage: vi.fn(),
  updateUser: vi.fn(),
  auth: { user: { id: 1 }, updateUser: vi.fn() },
}));
vi.mock("@/api/files", () => ({ fetchPage }));
vi.mock("@/api/users", () => ({ update: updateUser }));
vi.mock("@/stores/auth", () => ({ useAuthStore: () => auth }));

function resource(filter = "", offset = 0, name = "file.txt"): Resource {
  return {
    path: "/",
    url: "/files/",
    isDir: true,
    sorting: { by: "name", asc: false },
    items: [{ index: 0, name, url: `/files/${name}`, pageVisible: true }],
    page: {
      filter,
      offset,
      limit: 200,
      total: 501,
      sort: "name",
      asc: false,
      snapshot: "snapshot",
    },
  } as Resource;
}
let app: ReturnType<typeof createApp>;
let host: HTMLElement;
let store: ReturnType<typeof useFileStore>;
async function flush() {
  await Promise.resolve();
  await Promise.resolve();
  await nextTick();
}
function type(value: string) {
  const input = host.querySelector("input")!;
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
}
beforeEach(() => {
  vi.useFakeTimers();
  vi.clearAllMocks();
  vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  const pinia = createPinia();
  setActivePinia(pinia);
  store = useFileStore();
  store.updateRequest(resource());
  host = document.createElement("div");
  document.body.append(host);
  app = createApp(DirectoryPages)
    .use(pinia)
    .use(
      createI18n({ legacy: false, locale: "en", messages: { en: english } })
    );
  app.mount(host);
  fetchPage.mockImplementation(async (_url, query) => ({
    ...resource(query.filter, query.offset),
    sorting: { by: query.sort, asc: query.asc },
    page: { ...resource().page, ...query },
  }));
  updateUser.mockResolvedValue(undefined);
});
afterEach(() => {
  app.unmount();
  host.remove();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("directory controls", () => {
  it("filters as you type and clears the filter without an Apply step", async () => {
    type("fi");
    await vi.advanceTimersByTimeAsync(100);
    type("file-500");
    await vi.advanceTimersByTimeAsync(249);
    expect(fetchPage).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    await flush();
    expect(fetchPage.mock.calls[0][1]).toMatchObject({
      filter: "file-500",
      offset: 0,
      snapshot: "",
    });
    expect(host.textContent).not.toContain("Apply");
    (
      host.querySelector('[aria-label="Clear filter"]') as HTMLButtonElement
    ).click();
    await flush();
    expect(fetchPage.mock.calls.at(-1)![1].filter).toBe("");
    expect((host.querySelector("input") as HTMLInputElement).value).toBe("");
  });
  it("cancels an old search and ignores a late response", async () => {
    let finish!: (r: Resource) => void;
    fetchPage.mockImplementationOnce(
      () =>
        new Promise<Resource>((resolve) => {
          finish = resolve;
        })
    );
    type("old");
    await vi.advanceTimersByTimeAsync(250);
    const signal = fetchPage.mock.calls[0][2] as AbortSignal;
    type("new");
    expect(signal.aborted).toBe(true);
    await vi.advanceTimersByTimeAsync(250);
    await flush();
    finish(resource("old"));
    await flush();
    expect(store.req?.page?.filter).toBe("new");
    expect((host.querySelector("input") as HTMLInputElement).value).toBe("new");
    expect(host.querySelector('[role="alert"]')).toBeNull();
  });
  it("sorts immediately, retains the filter and persists the preference", async () => {
    store.updateRequest(resource("needle"));
    await flush();
    const select = host.querySelector("select")!;
    select.value = "modified:false";
    select.dispatchEvent(new Event("change"));
    expect(fetchPage).toHaveBeenCalledTimes(1);
    await flush();
    expect(fetchPage.mock.calls[0][1]).toMatchObject({
      filter: "needle",
      sort: "modified",
      asc: false,
      offset: 0,
      snapshot: "",
    });
    expect(updateUser).toHaveBeenCalledWith(
      { id: 1, sorting: { by: "modified", asc: false } },
      ["sorting"]
    );
    expect(store.req?.sorting).toEqual({ by: "modified", asc: false });
  });
  it("clears visible and off-page selections and pending preselection", async () => {
    store.selected = [0];
    store.updateRequest(resource("", 200, "second.txt"));
    store.selected.push(0);
    store.preselect = "/file.txt";
    await flush();
    expect(store.selectedCount).toBe(2);
    const clear = [...host.querySelectorAll("button")].find((b) =>
      b.textContent?.includes("Clear selection")
    )!;
    clear.click();
    await flush();
    expect(store.selectedCount).toBe(0);
    expect(store.preselect).toBeNull();
    store.updateRequest(resource());
    expect(store.selectedCount).toBe(0);
    expect(host.querySelector(".directory-selection")).toBeNull();
  });
  it("shares pending controls with automatic refreshes and rejects their older responses", async () => {
    let finish!: (r: Resource) => void;
    fetchPage.mockImplementationOnce(
      () =>
        new Promise<Resource>((resolve) => {
          finish = resolve;
        })
    );
    type("needle");
    expect(store.directoryQuery?.page).toMatchObject({
      filter: "needle",
      offset: 0,
      snapshot: "",
    });
    await vi.advanceTimersByTimeAsync(250);
    // A later automatic refresh has used the pending query and finished first.
    store.requestVersion++;
    store.updateRequest(resource("needle", 0, "fresh.txt"));
    finish(resource("needle", 0, "stale.txt"));
    await flush();
    expect(store.req?.items[0].name).toBe("fresh.txt");
    expect((host.querySelector("input") as HTMLInputElement).value).toBe(
      "needle"
    );
  });
  it("never replaces another folder with an in-flight result", async () => {
    let finish!: (r: Resource) => void;
    fetchPage.mockImplementationOnce(
      () =>
        new Promise<Resource>((resolve) => {
          finish = resolve;
        })
    );
    type("late");
    await vi.advanceTimersByTimeAsync(250);
    store.updateRequest({
      ...resource(),
      path: "/another",
      url: "/files/another/",
    });
    await flush();
    finish(resource("late"));
    await flush();
    expect(store.req?.path).toBe("/another");
    expect((fetchPage.mock.calls[0][2] as AbortSignal).aborted).toBe(true);
  });
});
