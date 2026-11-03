<template>
  <div
    v-if="store.allUploads.length"
    class="upload-files"
    :class="{ closed: !open }"
  >
    <div class="card floating">
      <div class="card-title">
        <h2>{{ t("upload.transfers") }} ({{ store.allUploads.length }})</h2>
        <span v-if="store.totalBytes"
          >{{ percentTotal }}% {{ t("upload.transferred") }} ·
          {{ filesize(Math.max(0, speed)) }}/s · {{ eta }}</span
        ><button
          class="action"
          @click="store.clearCompleted"
          :aria-label="t('upload.clearCompleted')"
          :title="t('upload.clearCompleted')"
        >
          <i class="material-icons">clear_all</i></button
        ><button
          class="action"
          @click="abortAll"
          :aria-label="t('upload.cancelUnfinished')"
          :title="t('upload.cancelUnfinished')"
        >
          <i class="material-icons">cancel</i></button
        ><button
          class="action"
          @click="open = !open"
          :aria-label="t('upload.toggleList')"
        >
          <i class="material-icons">{{
            open ? "keyboard_arrow_down" : "keyboard_arrow_up"
          }}</i>
        </button>
      </div>
      <div v-show="open" class="card-content transfer-list">
        <div
          v-for="item in store.allUploads"
          :key="item.id"
          class="transfer-row"
          :data-status="item.status"
        >
          <div class="transfer-title">
            <span class="transfer-name" :title="item.path">{{ item.name }}</span
            ><span>{{ t("upload.states." + item.status) }}</span>
          </div>
          <div
            class="transfer-progress"
            role="progressbar"
            :aria-label="item.name"
            :aria-valuenow="percent(item)"
            aria-valuemin="0"
            aria-valuemax="100"
          >
            <div :style="{ width: percent(item) + '%' }"></div>
          </div>
          <div class="transfer-detail">
            <span
              >{{ filesize(Math.max(0, item.sentBytes)) }} /
              {{ filesize(item.totalBytes) }}</span
            ><span
              v-if="item.scan"
              :class="{
                'scan-warning': [
                  'skipped',
                  'disabled',
                  'unknown',
                  'error',
                  'infected',
                ].includes(item.scan.status),
              }"
              >{{ t("upload.scan." + item.scan.status) }}</span
            >
          </div>
          <p v-if="item.message" class="transfer-message">{{ item.message }}</p>
          <div class="transfer-actions">
            <button
              v-if="item.status === 'uploading'"
              class="button button--flat"
              @click="store.pause(item)"
            >
              {{ t("upload.pause") }}</button
            ><button
              v-if="canResume(item) && item.file"
              class="button button--flat"
              @click="store.resume(item)"
            >
              {{ t("upload.resume") }}</button
            ><label
              v-if="canResume(item) && !item.file"
              class="button button--flat select-original"
              >{{ t("upload.selectOriginal")
              }}<input
                type="file"
                @change="selectOriginal(item, $event)"
                :aria-label="t('upload.selectOriginal')" /></label
            ><button
              v-if="item.status === 'failed' && item.canRetry"
              class="button button--flat"
              @click="store.retry(item)"
            >
              {{ t("upload.retryProcessing") }}</button
            ><button
              v-if="!['ready', 'cancelled', 'expired'].includes(item.status)"
              class="button button--flat"
              @click="store.cancel(item)"
            >
              {{ t("upload.cancel") }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
<script setup lang="ts">
import { useUploadStore } from "@/stores/upload";
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { filesize } from "filesize";
const { t } = useI18n(),
  store = useUploadStore(),
  open = ref(true);
onMounted(() => store.start());
const speed = ref(0);
let previousTime = Date.now(),
  previousBytes = 0;
watch(
  () => store.sentBytes,
  (bytes) => {
    const now = Date.now(),
      seconds = (now - previousTime) / 1000;
    speed.value =
      seconds > 0 && bytes >= previousBytes
        ? (bytes - previousBytes) / seconds
        : 0;
    previousBytes = bytes;
    previousTime = now;
  }
);
watch(
  () => store.totalBytes,
  () => {
    speed.value = 0;
    previousBytes = store.sentBytes;
    previousTime = Date.now();
  }
);
const eta = computed(() => {
  const n = Math.ceil((store.totalBytes - store.sentBytes) / speed.value);
  return !Number.isFinite(n) || n <= 0
    ? "--:--:--"
    : [Math.floor(n / 3600), Math.floor(n / 60) % 60, n % 60]
        .map((v) => String(v).padStart(2, "0"))
        .join(":");
});
const percentTotal = computed(() =>
  Math.min(
    100,
    (store.sentBytes / Math.max(1, store.totalBytes)) * 100
  ).toFixed(1)
);
function percent(i: Upload) {
  return i.totalBytes
    ? Math.min(100, Math.max(0, (i.sentBytes / i.totalBytes) * 100))
    : i.status === "ready"
      ? 100
      : 0;
}
function canResume(i: Upload) {
  return (
    ["paused", "needs_file"].includes(i.status) ||
    (i.status === "failed" && i.canRetry === undefined)
  );
}
function selectOriginal(i: Upload, e: Event) {
  const input = e.target as HTMLInputElement,
    file = input.files?.[0];
  if (file) {
    try {
      store.selectFile(i, file);
    } catch (error) {
      i.message = String(error);
    }
  }
  input.value = "";
}
function abortAll() {
  if (confirm(t("upload.abortUpload"))) void store.abort();
}
</script>
<style scoped>
.transfer-list {
  max-height: 55vh;
  overflow: auto;
}
.transfer-row {
  padding: 0.75em 0;
  border-bottom: 1px solid var(--divider);
}
.transfer-title,
.transfer-detail {
  display: flex;
  justify-content: space-between;
  gap: 1em;
}
.transfer-title {
  font-weight: 600;
}
.transfer-name {
  max-width: 65%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.transfer-progress {
  height: 5px;
  background: rgba(128, 128, 128, 0.25);
  margin: 0.5em 0;
}
.transfer-progress > div {
  height: 100%;
  background: #40c4ff;
  transition: width 0.2s;
}
.transfer-detail,
.transfer-message {
  font-size: 0.85em;
}
.transfer-message {
  overflow-wrap: anywhere;
}
.transfer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25em;
}
.transfer-actions .button {
  margin: 0;
  padding: 0.4em 0.7em;
}
.scan-warning {
  color: var(--red);
}
.select-original {
  position: relative;
  overflow: hidden;
}
.select-original input {
  position: absolute;
  inset: 0;
  opacity: 0;
  cursor: pointer;
  width: 100%;
}
</style>
