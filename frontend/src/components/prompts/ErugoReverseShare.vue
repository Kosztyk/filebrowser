<template>
  <div class="card floating" id="erugo-reverse-share">
    <div class="card-title">
      <h2>{{ $t("settings.erugoReverseShareTitle") }}</h2>
    </div>

    <div class="card-content">
      <p class="small">{{ $t("settings.erugoReverseShareHelp") }}</p>
      <p v-if="inlineError" class="text--red small erugo-inline-error">
        {{ inlineError }}
      </p>

      <p>{{ $t("settings.erugoReverseRecipientName") }}</p>
      <input
        id="focus-prompt"
        class="input input--block"
        type="text"
        v-model.trim="recipientName"
        tabindex="1"
      />

      <p>{{ $t("settings.erugoReverseRecipientEmail") }}</p>
      <input
        class="input input--block"
        type="email"
        v-model.trim="recipientEmail"
        placeholder="recipient@example.com"
        tabindex="2"
      />

      <p>{{ $t("settings.erugoReverseShareMessage") }}</p>
      <textarea
        class="input input--block input--textarea"
        v-model.trim="message"
        tabindex="3"
      ></textarea>

      <p>{{ $t("settings.erugoUploadSpeedLimit") }}</p>
      <select
        class="input input--block"
        v-model.number="uploadSpeedLimitKbps"
        tabindex="4"
      >
        <option
          v-for="option in bandwidthOptions"
          :key="option.value"
          :value="option.value"
        >
          {{ option.label }}
        </option>
      </select>
      <span class="small">{{ $t("settings.erugoUploadSpeedLimitHelp") }}</span>
    </div>

    <div class="card-action">
      <button
        type="button"
        class="button button--flat button--grey"
        @click="closeHovers"
        :disabled="working"
        tabindex="6"
      >
        {{ $t("buttons.cancel") }}
      </button>
      <button
        type="button"
        class="button button--flat button--blue"
        @click="submit"
        :disabled="working"
        :aria-busy="working ? 'true' : 'false'"
        tabindex="5"
      >
        {{
          working
            ? $t("settings.erugoReverseShareCreating")
            : $t("settings.erugoReverseShare")
        }}
      </button>
    </div>
  </div>
</template>

<script>
import { mapActions } from "pinia";
import { useLayoutStore } from "@/stores/layout";
import * as api from "@/api/index";

export default {
  name: "erugo-reverse-share",
  data() {
    return {
      recipientName: "",
      recipientEmail: "",
      message: "",
      uploadSpeedLimitKbps: 0,
      bandwidthOptions: [
        { value: 0, label: "Unlimited" },
        { value: 1000, label: "1 Mbps" },
        { value: 2000, label: "2 Mbps" },
        { value: 5000, label: "5 Mbps" },
        { value: 10000, label: "10 Mbps" },
        { value: 20000, label: "20 Mbps" },
        { value: 25000, label: "25 Mbps" },
        { value: 50000, label: "50 Mbps" },
        { value: 75000, label: "75 Mbps" },
        { value: 100000, label: "100 Mbps" },
        { value: 200000, label: "200 Mbps" },
        { value: 500000, label: "500 Mbps" },
        { value: 1000000, label: "1 Gbps" },
      ],
      working: false,
      inlineError: "",
    };
  },
  inject: ["$showError", "$showSuccess"],
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers"]),
    validate() {
      if (!this.recipientName || !this.recipientEmail) {
        this.inlineError = this.$t("settings.erugoRecipientRequired");
        return false;
      }

      const validEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(this.recipientEmail);
      if (!validEmail) {
        this.inlineError = this.$t("settings.erugoInvalidRecipientEmail");
        return false;
      }
      return true;
    },
    async submit() {
      if (this.working) return;
      this.inlineError = "";
      if (!this.validate()) {
        this.$showError(this.inlineError);
        return;
      }

      this.working = true;
      try {
        await api.share.createErugoReverse({
          recipientEmail: this.recipientEmail,
          recipientName: this.recipientName,
          message: this.message,
          uploadSpeedLimitKbps: this.uploadSpeedLimitKbps,
        });
        this.$showSuccess(this.$t("settings.erugoReverseShareCreated"));
        this.closeHovers();
      } catch (error) {
        this.inlineError =
          error && error.message ? error.message : String(error);
        this.$showError(error);
      } finally {
        this.working = false;
      }
    },
  },
};
</script>
