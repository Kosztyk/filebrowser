<template>
  <div class="card">
    <div class="card-title"><h2>About / System</h2></div>
    <div class="card-content">
      <p role="alert" v-if="error">{{ error }}</p>
      <p role="status">{{ message }}</p>
      <dl v-if="system">
        <template
          v-for="key in [
            'version',
            'build',
            'sourceChecksum',
            'go',
            'os',
            'arch',
          ]"
          :key="key"
          ><dt>{{ key }}</dt>
          <dd>{{ system[key] }}</dd></template
        >
      </dl>
      <h3>Signed-in sessions</h3>
      <p>
        Revocation blocks new requests and renewal. An open download finishes
        its current response.
      </p>
      <table>
        <thead>
          <tr>
            <th>Device</th>
            <th>Last activity</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="s in sessions" :key="s.id">
            <td>
              {{ s.current ? "This session: " : ""
              }}{{ s.agent || "Unknown client" }}
            </td>
            <td>{{ new Date(s.lastSeen).toLocaleString() }}</td>
            <td><button @click="revoke(s.id, s.current)">Revoke</button></td>
          </tr>
        </tbody>
      </table>
      <button @click="revoke('all', true)">Sign out all my sessions</button
      ><template v-if="auth.user?.perm.admin"
        ><h3>Integration diagnostics</h3>
        <article v-for="name in integrations" :key="name">
          <h4>{{ name }}</h4>
          <button @click="check(name)" :disabled="checking === name">
            Test saved connection
          </button>
          <div v-if="diagnostics[name]">
            <p>
              {{ diagnostics[name].status }} ·
              {{
                diagnostics[name].checked
                  ? new Date(diagnostics[name].checked).toLocaleString()
                  : ""
              }}
            </p>
            <p>{{ diagnostics[name].message }}</p>
            <p>Reported version: {{ diagnostics[name].version }}</p>
            <dl>
              <template
                v-for="(value, key) in diagnostics[name].capabilities"
                :key="key"
                ><dt>{{ key }}</dt>
                <dd>
                  {{ Array.isArray(value) ? value.join(", ") : value }}
                </dd></template
              >
            </dl>
          </div>
        </article>
        <form v-if="settings" @submit.prevent="save">
          <h3>Background work and extraction limits</h3>
          <p>
            Restart File Browser after changing worker count. A separate worker
            keeps directory pages responsive.
          </p>
          <label v-for="(label, key) in limitLabels" :key="key"
            >{{ label
            }}<input
              type="number"
              min="1"
              step="1"
              v-model.number="settings.work[key]"
              required
          /></label>
          <h3>ConvertX timeouts</h3>
          <p>Use durations such as 15s, 30m or 2h.</p>
          <label v-for="(label, key) in timeoutLabels" :key="key"
            >{{ label
            }}<input v-model="settings.convertx[key]" placeholder="2h" /></label
          ><button type="submit">Save limits and timeouts</button>
        </form>
        <details v-if="system?.dependencies">
          <summary>Backend package versions</summary>
          <dl>
            <template v-for="(value, key) in system.dependencies" :key="key"
              ><dt>{{ key }}</dt>
              <dd>{{ value }}</dd></template
            >
          </dl>
        </details></template
      >
    </div>
  </div>
</template>
<script setup lang="ts">
import { onMounted, ref } from "vue";
import { fetchJSON, fetchURL } from "@/api/utils";
import { useAuthStore } from "@/stores/auth";
import { logout } from "@/utils/auth";
import * as settingsApi from "@/api/settings";
const auth = useAuthStore();
const system = ref<any>();
const settings = ref<ISettings>();
const sessions = ref<any[]>([]);
const diagnostics = ref<Record<string, any>>({});
const error = ref("");
const message = ref("");
const checking = ref("");
const integrations = ["convertx", "erugo", "collabora", "clamav"];
const limitLabels = {
  workers: "Concurrent action workers (1–8)",
  maxJobs: "Maximum queued and running jobs",
  archiveBytes: "Maximum expanded archive bytes",
  archiveEntries: "Maximum archive entries",
  archiveDepth: "Maximum archive path depth",
  archiveSeconds: "Archive time budget in seconds",
  reserveBytes: "Minimum free bytes on destination",
  artifactBytes: "Prepared-download storage budget in bytes",
};
const timeoutLabels = {
  connectTimeout: "Connection / TLS",
  uploadTimeout: "File upload",
  processTimeout: "Upstream processing",
  downloadTimeout: "Converted output download",
} as const;
async function load() {
  try {
    system.value = await fetchJSON("/api/system");
    sessions.value = await fetchJSON("/api/sessions");
    if (auth.user?.perm.admin) {
      settings.value = await settingsApi.get();
      settings.value.work = { ...system.value.limits, ...settings.value.work };
      for (const name of integrations)
        diagnostics.value[name] = await fetchJSON(`/api/diagnostics/${name}`);
    }
  } catch (e) {
    error.value = String(e);
  }
}
async function revoke(id: string, current = false) {
  try {
    await fetchURL(`/api/sessions?id=${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
    if (current) logout();
    else await load();
  } catch (e) {
    error.value = String(e);
  }
}
async function check(name: string) {
  checking.value = name;
  try {
    diagnostics.value[name] = await fetchJSON(`/api/diagnostics/${name}`, {
      method: "POST",
    });
  } catch (e) {
    error.value = String(e);
  } finally {
    checking.value = "";
  }
}
async function save() {
  try {
    await settingsApi.update(settings.value!);
    message.value = "Settings saved.";
    await load();
  } catch (e) {
    error.value = String(e);
  }
}
onMounted(load);
</script>
<style scoped>
label {
  display: block;
  margin: 0.7rem 0;
}
input {
  max-width: 24rem;
}
dl {
  display: grid;
  grid-template-columns: minmax(8rem, 1fr) 2fr;
  gap: 0.5rem;
  overflow-wrap: anywhere;
}
dd {
  margin: 0;
}
article {
  padding: 1rem 0;
  border-bottom: 1px solid #8885;
}
td,
th {
  text-align: left;
  padding: 0.6rem;
}
table {
  width: 100%;
}
button {
  margin: 0.4rem;
}
</style>
