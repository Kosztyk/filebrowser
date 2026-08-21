<template>
  <div class="card floating" id="share">
    <div class="card-title">
      <h2>
        {{
          erugoEnabled ? $t("settings.erugoShareTitle") : $t("buttons.share")
        }}
      </h2>
    </div>

    <template v-if="listing">
      <div class="card-content">
        <table>
          <tr>
            <th>#</th>
            <th>{{ $t("settings.shareDuration") }}</th>
            <th></th>
            <th v-if="!erugoEnabled"></th>
            <th v-if="!erugoEnabled"></th>
          </tr>

          <tr v-for="link in links" :key="link.url || link.hash">
            <td>{{ link.name || link.hash }}</td>
            <td>
              <template v-if="link.expire !== 0">{{
                humanTime(link.expire)
              }}</template>
              <template v-else>{{ $t("permanent") }}</template>
              <template v-if="link.emailSent">
                <br />
                <span class="small">{{ $t("settings.erugoEmailSent") }}</span>
              </template>
              <template v-if="link.downloadSpeedLimitKbps">
                <br />
                <span class="small">{{
                  formatBandwidthLimit(link.downloadSpeedLimitKbps)
                }}</span>
              </template>
            </td>
            <td class="small">
              <button
                class="action"
                :aria-label="$t('buttons.copyToClipboard')"
                :title="$t('buttons.copyToClipboard')"
                @click="copyToClipboard(buildLink(link))"
              >
                <i class="material-icons">content_paste</i>
              </button>
            </td>
            <td v-if="!erugoEnabled" class="small">
              <button
                class="action"
                :aria-label="$t('buttons.copyDownloadLinkToClipboard')"
                :title="$t('buttons.copyDownloadLinkToClipboard')"
                :disabled="!!link.hasPassword"
                @click="copyToClipboard(buildDownloadLink(link))"
              >
                <i class="material-icons">content_paste_go</i>
              </button>
            </td>
            <td v-if="!erugoEnabled" class="small">
              <button
                class="action"
                @click="deleteLink($event, link)"
                :aria-label="$t('buttons.delete')"
                :title="$t('buttons.delete')"
              >
                <i class="material-icons">delete</i>
              </button>
            </td>
          </tr>
        </table>
      </div>

      <div class="card-action">
        <button
          type="button"
          class="button button--flat button--grey"
          @click="closeHovers"
          :aria-label="$t('buttons.close')"
          :title="$t('buttons.close')"
          tabindex="2"
        >
          {{ $t("buttons.close") }}
        </button>
        <button
          id="focus-prompt"
          type="button"
          class="button button--flat button--blue"
          @click="() => switchListing()"
          :aria-label="$t('buttons.new')"
          :title="$t('buttons.new')"
          tabindex="1"
        >
          {{ $t("buttons.new") }}
        </button>
      </div>
    </template>

    <template v-else>
      <div class="card-content">
        <p v-if="inlineError" class="text--red small erugo-inline-error">
          {{ inlineError }}
        </p>
        <template v-if="erugoEnabled">
          <p class="small">{{ $t("settings.erugoShareHelp") }}</p>

          <p>{{ $t("settings.erugoShareName") }}</p>
          <input
            class="input input--block"
            type="text"
            v-model.trim="shareName"
            :placeholder="defaultShareName"
            tabindex="1"
          />

          <p>{{ $t("settings.erugoDescription") }}</p>
          <textarea
            class="input input--block input--textarea"
            v-model.trim="description"
            tabindex="2"
          ></textarea>

          <p>{{ $t("settings.erugoRecipientEmail") }}</p>
          <input
            class="input input--block"
            type="email"
            v-model.trim="recipientEmail"
            placeholder="recipient@example.com"
            tabindex="3"
          />

          <p>{{ $t("settings.erugoRecipientName") }}</p>
          <input
            class="input input--block"
            type="text"
            v-model.trim="recipientName"
            tabindex="4"
          />

          <p>{{ $t("settings.erugoDownloadSpeedLimit") }}</p>
          <select
            class="input input--block"
            v-model.number="downloadSpeedLimitKbps"
            tabindex="5"
          >
            <option
              v-for="option in bandwidthOptions"
              :key="option.value"
              :value="option.value"
            >
              {{ option.label }}
            </option>
          </select>
          <span class="small">{{
            $t("settings.erugoDownloadSpeedLimitHelp")
          }}</span>
        </template>

        <p>{{ $t("settings.shareDuration") }}</p>
        <div class="input-group input">
          <vue-number-input
            center
            controls
            size="small"
            :max="2147483647"
            :min="0"
            @keyup.enter="submit"
            v-model="time"
            :tabindex="erugoEnabled ? 6 : 1"
          />
          <select
            class="right"
            v-model="unit"
            :aria-label="$t('time.unit')"
            :tabindex="erugoEnabled ? 7 : 2"
          >
            <option value="seconds">{{ $t("time.seconds") }}</option>
            <option value="minutes">{{ $t("time.minutes") }}</option>
            <option value="hours">{{ $t("time.hours") }}</option>
            <option value="days">{{ $t("time.days") }}</option>
          </select>
        </div>

        <template v-if="!erugoEnabled">
          <p>{{ $t("prompts.optionalPassword") }}</p>
          <input
            class="input input--block"
            type="password"
            v-model.trim="password"
            tabindex="3"
          />
        </template>
      </div>

      <div class="card-action">
        <button
          type="button"
          class="button button--flat button--grey"
          @click="() => switchListing()"
          :aria-label="$t('buttons.cancel')"
          :title="$t('buttons.cancel')"
          tabindex="9"
        >
          {{ $t("buttons.cancel") }}
        </button>
        <button
          id="focus-prompt"
          type="button"
          class="button button--flat button--blue"
          @click="submit"
          :disabled="working"
          :aria-busy="working ? 'true' : 'false'"
          :aria-label="$t('buttons.share')"
          :title="$t('buttons.share')"
          tabindex="8"
        >
          {{ working ? $t("settings.erugoSharing") : $t("buttons.share") }}
        </button>
      </div>
    </template>
  </div>
</template>

<script>
import { mapActions, mapState } from "pinia";
import { useFileStore } from "@/stores/file";
import * as api from "@/api/index";
import dayjs from "dayjs";
import { useLayoutStore } from "@/stores/layout";
import { copy } from "@/utils/clipboard";

export default {
  name: "share",
  data: function () {
    return {
      time: 0,
      unit: "hours",
      links: [],
      clip: null,
      password: "",
      listing: true,
      erugoEnabled: false,
      shareName: "",
      description: "",
      recipientEmail: "",
      recipientName: "",
      downloadSpeedLimitKbps: 0,
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
  computed: {
    ...mapState(useFileStore, [
      "req",
      "selected",
      "selectedCount",
      "isListing",
    ]),
    url() {
      if (!this.isListing) {
        return this.$route.path;
      }

      if (this.selectedCount === 0 || this.selectedCount > 1) {
        // This shouldn't happen.
        return;
      }

      return this.req.items[this.selected[0]].url;
    },
    defaultShareName() {
      if (!this.url) return "";
      const parts = this.url.split("/").filter(Boolean);
      return decodeURIComponent(parts[parts.length - 1] || "");
    },
  },
  async beforeMount() {
    // Erugo is optional. If the capability endpoint is unavailable for any
    // reason (for example during a rolling frontend/backend update), always
    // fall back to File Browser's original share implementation.
    try {
      const config = await api.share.config();
      this.erugoEnabled = config.backend === "erugo" && config.enabled === true;
    } catch (e) {
      console.warn(
        "Unable to read Erugo share configuration; using native File Browser sharing.",
        e
      );
      this.erugoEnabled = false;
    }

    if (this.erugoEnabled) {
      this.listing = false;
      this.shareName = this.defaultShareName;
      return;
    }

    try {
      const links = await api.share.get(this.url);
      this.links = Array.isArray(links) ? links : [];
      this.sort();

      if (this.links.length == 0) {
        this.listing = false;
      }
    } catch (e) {
      this.$showError(e);
    }
  },
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers"]),
    copyToClipboard: function (text) {
      copy({ text }).then(
        () => {
          // clipboard successfully set
          this.$showSuccess(this.$t("success.linkCopied"));
        },
        () => {
          // clipboard write failed
          copy({ text }, { permission: true }).then(
            () => {
              // clipboard successfully set
              this.$showSuccess(this.$t("success.linkCopied"));
            },
            (e) => {
              // clipboard write failed
              this.$showError(e);
            }
          );
        }
      );
    },
    validateErugoShare: function () {
      if (!this.erugoEnabled) {
        return true;
      }

      if (!this.url) {
        this.inlineError = this.$t("settings.erugoNoSelectedFile");
        return false;
      }

      if (this.recipientEmail) {
        const validEmail = /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(
          this.recipientEmail
        );
        if (!validEmail) {
          this.inlineError = this.$t("settings.erugoInvalidRecipientEmail");
          return false;
        }
      }

      return true;
    },
    submit: async function () {
      if (this.working) {
        return;
      }

      this.inlineError = "";
      if (!this.validateErugoShare()) {
        this.$showError(this.inlineError);
        return;
      }

      this.working = true;
      try {
        let res = null;

        if (this.erugoEnabled) {
          res = await api.share.createErugo(this.url, {
            expires: this.time ? this.time : "",
            unit: this.unit,
            name: this.shareName || this.defaultShareName,
            description: this.description,
            recipientEmail: this.recipientEmail,
            recipientName: this.recipientName,
            downloadSpeedLimitKbps: this.downloadSpeedLimitKbps,
          });
        } else if (!this.time) {
          res = await api.share.createNative(this.url, this.password);
        } else {
          res = await api.share.createNative(
            this.url,
            this.password,
            this.time,
            this.unit
          );
        }

        this.links.push(res);
        this.sort();

        if (this.erugoEnabled) {
          this.copyToClipboard(this.buildLink(res));
          this.$showSuccess(this.$t("settings.erugoShareCreated"));
        }

        this.time = 0;
        this.unit = "hours";
        this.password = "";
        this.shareName = this.defaultShareName;
        this.description = "";
        this.recipientEmail = "";
        this.recipientName = "";
        this.downloadSpeedLimitKbps = 0;

        this.listing = true;
      } catch (e) {
        const message = e && e.message ? e.message : String(e);
        this.inlineError = message;
        this.$showError(e);
      } finally {
        this.working = false;
      }
    },
    deleteLink: async function (event, link) {
      event.preventDefault();
      if (this.erugoEnabled || link.externalService === "erugo") {
        return;
      }
      try {
        await api.share.remove(link.hash);
        this.links = this.links.filter((item) => item.hash !== link.hash);

        if (this.links.length == 0) {
          this.listing = false;
        }
      } catch (e) {
        this.$showError(e);
      }
    },
    humanTime(time) {
      return dayjs(time * 1000).fromNow();
    },
    formatBandwidthLimit(value) {
      const kbps = Number(value || 0);
      if (kbps <= 0) return this.$t("settings.erugoUnlimited");
      return `${kbps / 1000} Mbps`;
    },
    buildLink(share) {
      return api.share.getShareURL(share);
    },
    buildDownloadLink(share) {
      return api.pub.getDownloadURL(
        {
          hash: share.hash,
          path: "",
        },
        true
      );
    },
    sort() {
      this.links = this.links.sort((a, b) => {
        if (a.expire === 0) return -1;
        if (b.expire === 0) return 1;
        return new Date(a.expire) - new Date(b.expire);
      });
    },
    switchListing() {
      if (this.links.length == 0 && !this.listing) {
        this.closeHovers();
      }

      this.listing = !this.listing;
    },
  },
};
</script>
