<template>
  <div
    v-if="store.req?.page"
    class="directory-pages"
    aria-label="Folder controls"
  >
    <div class="directory-filter">
      <i class="material-icons filter-icon" aria-hidden="true">filter_list</i>
      <input
        class="input"
        type="text"
        :value="filter"
        :aria-label="t('files.filterNames')"
        :placeholder="t('files.filterPlaceholder')"
        maxlength="200"
        autocomplete="off"
        :spellcheck="false"
        @input="changeFilter(($event.target as HTMLInputElement).value)"
        @keydown.enter.prevent="load(0)"
        @keydown.esc.prevent.stop="clearFilter"
      />
      <button
        v-if="filter"
        class="action clear-filter"
        type="button"
        :aria-label="t('files.clearFilter')"
        :title="t('files.clearFilter')"
        @click="clearFilter"
      >
        <i class="material-icons" aria-hidden="true">close</i>
      </button>
    </div>
    <select
      class="input directory-sort"
      :aria-label="t('files.sortOrder')"
      :value="`${sorting.by}:${sorting.asc}`"
      @change="changeSort(($event.target as HTMLSelectElement).value)"
    >
      <option value="name:false">{{ t("files.sortNameAZ") }}</option>
      <option value="name:true">{{ t("files.sortNameZA") }}</option>
      <option value="size:true">{{ t("files.sortSizeSmall") }}</option>
      <option value="size:false">{{ t("files.sortSizeLarge") }}</option>
      <option value="modified:false">{{ t("files.sortNewest") }}</option>
      <option value="modified:true">{{ t("files.sortOldest") }}</option>
    </select>
    <button
      type="button"
      class="action"
      :aria-label="t('files.refreshListing')"
      :title="t('files.refreshListing')"
      @click="load(0, true)"
    >
      <i class="material-icons" :class="{ spin: busy }" aria-hidden="true"
        >refresh</i
      >
    </button>
    <div class="directory-page-info">
      <span class="directory-count" role="status" aria-live="polite">
        <template v-if="page.total > page.limit">
          {{ page.offset + 1 }}–{{
            Math.min(page.offset + page.limit, page.total)
          }}
          {{ t("files.of") }} {{ page.total.toLocaleString() }}
        </template>
        <template v-else>{{
          t("files.directoryItemCount", page.total, {
            named: { count: page.total.toLocaleString() },
          })
        }}</template>
      </span>
      <template v-if="page.total > page.limit">
        <button
          type="button"
          class="action"
          :aria-label="t('files.previousPage')"
          :title="t('files.previousPage')"
          :disabled="busy || !page.offset"
          @click="load(Math.max(0, page.offset - page.limit))"
        >
          <i class="material-icons" aria-hidden="true">chevron_left</i>
        </button>
        <button
          type="button"
          class="action"
          :aria-label="t('files.nextPage')"
          :title="t('files.nextPage')"
          :disabled="busy || page.offset + page.limit >= page.total"
          @click="load(page.offset + page.limit)"
        >
          <i class="material-icons" aria-hidden="true">chevron_right</i>
        </button>
      </template>
    </div>
    <div v-if="store.selectedCount" class="directory-selection">
      <span>{{
        t(
          page.total > page.limit
            ? "files.directorySelectedAcrossPages"
            : "files.directorySelected",
          store.selectedCount
        )
      }}</span>
      <button
        type="button"
        class="button button--flat"
        @click="store.clearSelection()"
      >
        {{ t("files.clearSelection") }}
      </button>
    </div>
    <p v-if="error" class="directory-error" role="alert">{{ error }}</p>
  </div>
</template>
<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useFileStore } from "@/stores/file";
import { useAuthStore } from "@/stores/auth";
import { fetchPage } from "@/api/files";
import { update as updateUser } from "@/api/users";

const store = useFileStore();
const auth = useAuthStore();
const { t } = useI18n();
const page = computed(() => store.req!.page!);
const filter = ref(store.req?.page?.filter || "");
const sorting = ref({ ...(store.req?.sorting || { by: "name", asc: false }) });
const busy = ref(false);
const error = ref("");
let controller: AbortController | undefined;
let debounce: ReturnType<typeof setTimeout> | undefined;
let request = 0;
let saveSorting = Promise.resolve();

function cancelPending() {
  clearTimeout(debounce);
  debounce = undefined;
  controller?.abort();
  request++;
  busy.value = false;
}

watch(
  () => store.req?.path,
  () => {
    cancelPending();
    filter.value = store.req?.page?.filter || "";
    sorting.value = { ...(store.req?.sorting || { by: "name", asc: false }) };
    error.value = "";
  }
);
watch(
  () => store.req,
  (req) => {
    if (busy.value || debounce) return;
    filter.value = req?.page?.filter || "";
    if (req?.sorting) sorting.value = { ...req.sorting };
  }
);
onBeforeUnmount(cancelPending);

function changeFilter(value: string) {
  cancelPending();
  store.requestVersion++;
  filter.value = value;
  error.value = "";
  // A different filter must not leave hidden files selected for destructive actions.
  store.clearSelection();
  if (store.req?.page)
    store.directoryQuery = {
      url: store.req.url,
      page: {
        ...store.req.page,
        filter: value,
        sort: sorting.value.by,
        asc: sorting.value.asc,
        offset: 0,
        snapshot: "",
      },
    };
  debounce = setTimeout(() => void load(0), 250);
}
function clearFilter() {
  changeFilter("");
  void load(0);
}
function changeSort(value: string) {
  const [by, asc] = value.split(":");
  setSorting(by, asc === "true");
}
function setSorting(by: string, asc: boolean) {
  sorting.value = { by, asc };
  void load(0);
  const id = auth.user?.id;
  if (id === undefined) return;
  auth.updateUser({ sorting: { by, asc } });
  // Serialize preference writes so rapid changes cannot persist an older order.
  saveSorting = saveSorting
    .then(() => updateUser({ id, sorting: { by, asc } }, ["sorting"]))
    .catch((e) => {
      error.value = String(e);
    });
}
function sortBy(by: string) {
  setSorting(by, sorting.value.by === by ? !sorting.value.asc : by !== "name");
}
defineExpose({ sortBy });

async function load(offset: number, refresh = false) {
  cancelPending();
  if (!store.req?.page) return;
  const current = store.req;
  const p = current.page!;
  const id = request;
  const abort = new AbortController();
  controller = abort;
  busy.value = true;
  error.value = "";
  const sameQuery =
    p.filter === filter.value &&
    p.sort === sorting.value.by &&
    p.asc === sorting.value.asc;
  const query = {
    ...p,
    offset,
    snapshot: !refresh && sameQuery ? p.snapshot : "",
    filter: filter.value,
    sort: sorting.value.by,
    asc: sorting.value.asc,
  };
  // Automatic refreshes must use the newest controls, even before this fetch finishes.
  store.directoryQuery = { url: current.url, page: query };
  const version = ++store.requestVersion;
  try {
    const result = await fetchPage(
      current.url,
      { ...query, refresh },
      abort.signal
    );
    if (
      id !== request ||
      version !== store.requestVersion ||
      abort.signal.aborted ||
      store.req?.path !== current.path
    )
      return;
    store.updateRequest(result);
    window.scrollTo(0, 0);
  } catch (e) {
    if (
      id === request &&
      version === store.requestVersion &&
      !abort.signal.aborted
    )
      error.value = String(e);
  } finally {
    if (id === request) busy.value = false;
  }
}
</script>
<style scoped>
.directory-pages {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5em;
  padding: 0.35em 0 0.75em;
  margin-bottom: 0.5em;
  color: var(--textSecondary);
  font-size: 0.9em;
  border-bottom: 1px solid var(--divider);
}
.directory-filter {
  position: relative;
  flex: 1 1 12em;
  max-width: 22em;
  min-width: 0;
}
.directory-filter input {
  width: 100%;
  padding-inline: 2.25em;
}
.directory-filter .filter-icon {
  position: absolute;
  inset-inline-start: 0.5em;
  top: 50%;
  transform: translateY(-50%);
  font-size: 1.3em;
  pointer-events: none;
}
.directory-filter .clear-filter {
  position: absolute;
  inset-inline-end: 0.15em;
  top: 50%;
  transform: translateY(-50%);
}
.directory-filter .clear-filter i {
  font-size: 1.15em;
}
.directory-sort {
  width: auto;
  max-width: 100%;
  cursor: pointer;
  padding-inline: 0.5em;
}
.directory-page-info {
  margin-inline-start: auto;
  display: flex;
  align-items: center;
  gap: 0.25em;
}
.directory-count {
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
  padding-inline: 0.4em;
}
.directory-selection {
  display: flex;
  align-items: center;
  gap: 0.5em;
  padding-inline-start: 0.75em;
  border-inline-start: 1px solid var(--divider);
}
.directory-selection .button {
  text-transform: none;
  padding: 0.5em;
}
.directory-error {
  flex-basis: 100%;
  margin: 0.25em 0 0;
  color: var(--red);
}
@media (max-width: 736px) {
  .directory-filter {
    max-width: none;
    flex-basis: calc(100% - 3em);
  }
  .directory-sort {
    order: 1;
  }
  .directory-page-info {
    order: 2;
  }
  .directory-selection,
  .directory-error {
    order: 3;
  }
  .directory-selection {
    flex-basis: 100%;
    border: 0;
    padding: 0;
  }
}
</style>
