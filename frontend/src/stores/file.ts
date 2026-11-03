import { defineStore } from "pinia";

export const useFileStore = defineStore("file", {
  // convert to a function
  state: (): {
    requestVersion: number;
    directoryQuery: { url: string; page: DirectoryPage } | null;
    req: Resource | null;
    oldReq: Resource | null;
    reload: boolean;
    selected: number[];
    multiple: boolean;
    isFiles: boolean;
    preselect: string | null;
  } => ({
    requestVersion: 0,
    directoryQuery: null,
    req: null,
    oldReq: null,
    reload: false,
    selected: [],
    multiple: false,
    isFiles: false,
    preselect: null,
  }),
  getters: {
    selectedCount: (state) => state.selected.length,
    // route: () => {
    //   const routerStore = useRouterStore();
    //   return routerStore.router.currentRoute;
    // },
    // isFiles: (state) => {
    //   const layoutStore = useLayoutStore();
    //   return !layoutStore.loading && state.route._value.name === "Files";
    // },
    isListing: (state) => {
      return state.isFiles && state?.req?.isDir;
    },
  },
  actions: {
    // no context as first argument, use `this` instead
    toggleMultiple() {
      this.multiple = !this.multiple;
    },
    updateRequest(value: Resource | null) {
      const selectedItems = this.selected.map((i) => this.req?.items[i]);
      if (value?.page && this.req?.path === value.path) {
        const hidden = selectedItems
          .filter(
            (item): item is ResourceItem =>
              !!item && !value.items.some((i) => i.url === item.url)
          )
          .map((i) => ({ ...i, pageVisible: false }));
        value.items.push(...hidden);
        value.items.forEach((item, index) => (item.index = index));
      }
      this.oldReq = this.req;
      this.req = value;
      if (value?.page)
        this.directoryQuery = { url: value.url, page: { ...value.page } };
      else if (!value) this.directoryQuery = null;

      this.selected = [];

      if (!this.req?.items) return;
      this.selected = this.req.items
        .filter((item) =>
          selectedItems.some((rItem) => rItem?.url === item.url)
        )
        .map((item) => item.index);
    },
    clearSelection() {
      this.selected = [];
      this.preselect = null;
    },
    removeSelected(value: any) {
      const i = this.selected.indexOf(value);
      if (i === -1) return;
      this.selected.splice(i, 1);
    },
    // easily reset state using `$reset`
    clearFile() {
      this.$reset();
    },
  },
});
