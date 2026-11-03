<template>
  <div class="bounded-text">
    <HeaderBar showMenu showLogo />
    <h2>{{ store.req?.name }}</h2>
    <p>
      Read-only preview: up to 2,000 rows or 256 KiB per page. Download the file
      for complete editing.
    </p>
    <label v-if="isCsv"
      >Delimiter
      <select v-model="delimiter" @change="reset">
        <option value=",">Comma</option>
        <option value=";">Semicolon</option>
        <option value="&#9;">Tab</option>
      </select></label
    >
    <p role="alert" v-if="error">{{ error }}</p>
    <button :disabled="busy || !history.length" @click="previous">
      Previous</button
    ><button :disabled="busy || !data?.hasMore" @click="next">Next</button
    ><span v-if="data">
      Bytes {{ data.offset.toLocaleString() }}–{{
        data.next.toLocaleString()
      }}
      of {{ data.size.toLocaleString() }}</span
    >
    <table v-if="data?.rows">
      <tbody>
        <tr v-for="(row, i) in data.rows" :key="i">
          <td v-for="(cell, j) in row" :key="j">{{ cell }}</td>
        </tr>
      </tbody>
    </table>
    <pre v-else>{{ data?.text }}</pre>
  </div>
</template>
<script setup lang="ts">
import { computed, onMounted, ref, onBeforeUnmount, watch } from "vue";
import HeaderBar from "@/components/header/HeaderBar.vue";
import { useFileStore } from "@/stores/file";
import { fetchURL } from "@/api/utils";
import { encodePath } from "@/utils/url";
const store = useFileStore();
const data = ref<{
  text?: string;
  rows?: string[][];
  offset: number;
  next: number;
  size: number;
  version: string;
  hasMore: boolean;
}>();
const history = ref<number[]>([]);
const error = ref("");
const busy = ref(false);
const delimiter = ref(",");
const isCsv = computed(() => store.req?.extension.toLowerCase() === ".csv");
let controller = new AbortController();
async function load(offset = 0) {
  controller.abort();
  controller = new AbortController();
  const current = controller;
  busy.value = true;
  error.value = "";
  try {
    const q = new URLSearchParams({
      offset: String(offset),
      version: data.value?.version || "",
      bytes: String(256 * 1024),
    });
    if (isCsv.value) {
      q.set("csv", "true");
      q.set("delimiter", delimiter.value);
    }
    const res = await fetchURL(
      `/api/text-preview${encodePath(store.req!.path)}?${q}`,
      { signal: current.signal }
    );
    const next = await res.json();
    if (!current.signal.aborted) data.value = next;
  } catch (e) {
    if (!current.signal.aborted) error.value = String(e);
  } finally {
    if (controller === current) busy.value = false;
  }
}
async function next() {
  if (!data.value) return;
  const current = data.value.offset;
  await load(data.value.next);
  if (!error.value) history.value.push(current);
}
async function previous() {
  const offset = history.value.at(-1);
  if (offset === undefined) return;
  await load(offset);
  if (!error.value) history.value.pop();
}
function reset() {
  data.value = undefined;
  history.value = [];
  void load();
}
onMounted(() => load());
watch(() => store.req?.path, reset);
onBeforeUnmount(() => controller.abort());
</script>
<style scoped>
.bounded-text {
  padding: 1rem;
}
.bounded-text pre {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 65vh;
  overflow: auto;
}
td {
  max-width: 25rem;
  overflow-wrap: anywhere;
  border: 1px solid #8885;
  padding: 0.4rem;
}
table {
  display: block;
  max-height: 65vh;
  overflow: auto;
}
button {
  margin: 0.5rem;
}
</style>
