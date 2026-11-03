<template>
  <details v-if="path" @toggle="load">
    <summary>Previous contents</summary>
    <p>
      Up to five previous versions, retained for seven days within a 100 GiB
      budget per directory.
    </p>
    <p role="alert" v-if="error">{{ error }}</p>
    <p v-if="!versions.length">No previous version is available.</p>
    <p v-for="v in versions" :key="v.id">
      {{ new Date(v.created).toLocaleString() }} ·
      {{ v.size.toLocaleString() }} bytes
      <button v-if="auth.user?.perm.modify" @click="restore(v.id)">
        Restore contents
      </button>
    </p>
  </details>
</template>
<script setup lang="ts">
import { computed, ref } from "vue";
import { useFileStore } from "@/stores/file";
import { useAuthStore } from "@/stores/auth";
import { fetchJSON, fetchURL } from "@/api/utils";
import { encodePath } from "@/utils/url";
const store = useFileStore();
const auth = useAuthStore();
const path = computed(() => {
  const file = store.req?.isDir
    ? store.req.items[store.selected[0]]
    : store.req;
  return store.selected.length <= 1 && file && !file.isDir && !file.archive
    ? file.path
    : "";
});
const versions = ref<any[]>([]);
const error = ref("");
async function load() {
  if (!path.value) return;
  try {
    versions.value = await fetchJSON(`/api/history${encodePath(path.value)}`);
  } catch (e) {
    error.value = String(e);
  }
}
async function restore(id: string) {
  try {
    await fetchURL(`/api/history${encodePath(path.value)}?id=${id}`, {
      method: "POST",
    });
    await load();
    store.reload = true;
  } catch (e) {
    error.value = String(e);
  }
}
</script>
