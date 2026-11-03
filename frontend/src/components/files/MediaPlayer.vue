<template>
  <media-i18n class="video-i18n" :lang="locale">
    <video-player ref="player" class="video-max" :poster="options.poster">
      <video-skin class="video-max">
        <video
          ref="media"
          :key="source"
          class="video-max"
          :autoplay="Boolean(options.autoplay)"
          :muted="Boolean(options.muted)"
          :loop="Boolean(options.loop)"
          preload="metadata"
          playsinline
        >
          <source :src="source" :type="mediaSourceType(source)" />
          <track
            v-for="(sub, index) in subtitles"
            :key="sub"
            kind="subtitles"
            :src="sub"
            :label="subtitleLabel(sub)"
            :default="index === 0"
          />
          <p>
            {{ $t("buttons.download") }}:
            <a :href="source">{{ $t("buttons.download") }}</a>
          </p>
        </video>
      </video-skin>
    </video-player>
  </media-i18n>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import type { VideoPlayerElement } from "@videojs/html/video";
import "@videojs/html/i18n";
import "@videojs/html/video/player";
import "@videojs/html/video/skin";
import { mediaSourceType, subtitleLabel } from "@/utils/media";

// The v10 skin includes keyboard shortcuts, touch gestures, rate controls,
// fullscreen and captions. Do not load v8-only player plugins.
withDefaults(
  defineProps<{
    source: string;
    subtitles?: string[];
    options?: {
      autoplay?: boolean;
      muted?: boolean;
      loop?: boolean;
      poster?: string;
    };
  }>(),
  { subtitles: () => [], options: () => ({}) }
);

const { locale } = useI18n();
const player = ref<VideoPlayerElement | null>(null);
const media = ref<HTMLVideoElement | null>(null);

onBeforeUnmount(() => {
  // Stop media/network activity as soon as the preview closes. The player
  // element tears down its own store when disconnected from the document.
  media.value?.pause();
  media.value?.removeAttribute("src");
  media.value?.querySelectorAll("source").forEach((source) => {
    source.removeAttribute("src");
  });
  media.value?.load();
});
</script>

<style scoped>
.video-i18n,
.video-max {
  display: block;
  width: 100%;
  height: 100%;
  min-height: 0;
}

video {
  object-fit: contain;
}
</style>
