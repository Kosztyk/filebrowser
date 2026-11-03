import { createApp, defineComponent, h } from "vue";
import { createI18n } from "vue-i18n";
import MediaPlayer from "../../src/components/files/MediaPlayer.vue";
import { VueReader } from "vue-reader";
import { renderMarkdown } from "../../src/utils/markdown";

const mode = new URLSearchParams(location.search).get("mode");
const component = defineComponent({
  setup() {
    if (mode === "epub") {
      return () =>
        h(VueReader, {
          url: "/tests/fixtures/sample.epub",
          location: 0,
          "onUpdate:location": (cfi: string) => {
            document.body.dataset.location = cfi;
          },
        });
    }
    if (mode === "markdown") {
      void renderMarkdown("Inline $x^2$\n\n<img src=x onerror=alert(1)>").then(
        (html) => {
          document.querySelector("#markdown")!.innerHTML = html;
        }
      );
      return () => h("div", { id: "markdown" });
    }
    return () =>
      h("div", { style: "width:640px;height:360px" }, [
        h(MediaPlayer, {
          source: "/tests/fixtures/sample.webm",
          subtitles: ["/tests/fixtures/English.vtt"],
          options: { muted: true },
        }),
      ]);
  },
});
createApp(component)
  .use(
    createI18n({
      legacy: false,
      locale: "en",
      messages: { en: { buttons: { download: "Download" } } },
    })
  )
  .mount("#app");
