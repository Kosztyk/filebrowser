<template>
  <details class="job-panel" v-if="auth.isLoggedIn">
    <summary>
      Background jobs <span v-if="active">({{ active }} active)</span>
    </summary>
    <p role="alert" v-if="error">{{ error }}</p>
    <p v-if="!jobs.length">No background jobs.</p>
    <article v-for="j in jobs.slice(0, 100)" :key="j.id">
      <strong>{{ j.kind }}</strong> · {{ j.state }} · {{ j.phase
      }}<progress
        v-if="['queued', 'running'].includes(j.state)"
        :value="j.total ? j.completed : undefined"
        :max="j.total || undefined"
      /><span v-if="j.completed">
        {{ j.completed.toLocaleString()
        }}{{ j.total ? " / " + j.total.toLocaleString() : "" }}</span
      >
      <p v-if="j.message">{{ j.message }}</p>
      <button
        v-if="['queued', 'running'].includes(j.state)"
        @click="control(j.id, 'cancel')"
      >
        Cancel</button
      ><button
        v-if="j.state === 'failed' && j.retryable"
        @click="control(j.id, 'retry')"
      >
        Retry</button
      ><template v-if="j.state === 'needs_review'"
        ><p>
          Check the external service or destination before continuing. A lost
          response may still have completed the operation.
        </p>
        <button
          v-if="
            ['convert', 'erugo'].includes(j.kind) && j.phase === 'submitting'
          "
          @click="control(j.id, 'review_retry')"
        >
          I checked: retry the submission</button
        ><button @click="control(j.id, 'dismiss')">
          Close after review
        </button></template
      ><a
        v-if="j.kind === 'download' && j.state === 'succeeded'"
        :href="artifactURL(j.id)"
        >Download {{ j.result?.name }}</a
      ><a
        v-if="j.kind === 'erugo' && j.state === 'succeeded' && j.result?.url"
        :href="j.result.url"
        target="_blank"
        rel="noopener noreferrer"
        >Open share</a
      >
    </article>
  </details>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from "vue";
import { useAuthStore } from "@/stores/auth";
import {
  listJobs,
  controlJob,
  artifactURL,
  type BackgroundJob,
} from "@/api/jobs";
const auth = useAuthStore();
const jobs = ref<BackgroundJob[]>([]);
const error = ref("");
const active = computed(
  () => jobs.value.filter((j) => ["queued", "running"].includes(j.state)).length
);
let timer: number;
let pending = false;
async function refresh() {
  if (pending || !auth.isLoggedIn) return;
  pending = true;
  const owner = auth.user?.id;
  try {
    const data = await listJobs();
    if (owner === auth.user?.id) {
      jobs.value = data;
      error.value = "";
    }
  } catch (e) {
    error.value = String(e);
  } finally {
    pending = false;
  }
}
async function control(id: string, action: string) {
  try {
    await controlJob(id, action);
    await refresh();
  } catch (e) {
    error.value = String(e);
  }
}
onMounted(() => {
  void refresh();
  timer = window.setInterval(refresh, 3000);
});
onBeforeUnmount(() => clearInterval(timer));
watch(
  () => auth.user?.id,
  () => {
    jobs.value = [];
    void refresh();
  }
);
</script>
<style scoped>
.job-panel {
  position: fixed;
  bottom: 1rem;
  left: 1rem;
  z-index: 90;
  background: var(--surfacePrimary, #fff);
  color: var(--textPrimary, #222);
  box-shadow: 0 2px 10px #7777;
  padding: 0.7rem;
  max-width: min(34rem, 90vw);
  max-height: 60vh;
  overflow: auto;
}
summary {
  cursor: pointer;
}
article {
  padding: 0.7rem 0;
  border-top: 1px solid #8885;
}
button,
a {
  margin-right: 0.7rem;
}
progress {
  display: block;
  width: 100%;
}
</style>
