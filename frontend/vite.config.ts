import path from "node:path";
import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import VueI18nPlugin from "@intlify/unplugin-vue-i18n/vite";
import legacy from "@vitejs/plugin-legacy";
import { compression } from "vite-plugin-compression2";

const plugins = [
  vue({
    template: {
      compilerOptions: {
        isCustomElement: (tag) =>
          new Set(["media-i18n", "video-player", "video-skin"]).has(tag),
      },
    },
  }),
  VueI18nPlugin({
    include: [path.resolve(import.meta.dirname, "./src/i18n/**/*.json")],
  }),
  legacy({
    // defaults already drop IE support
    targets: ["defaults"],
  }),
  compression({ include: /\.js$/, deleteOriginalAssets: false }),
];

const resolve = {
  alias: {
    // vue: "@vue/compat",
    "@/": `${path.resolve(import.meta.dirname, "src")}/`,
  },
};

// https://vitejs.dev/config/
export default defineConfig(({ command }) => {
  if (command === "serve") {
    return {
      plugins,
      resolve,
      server: {
        proxy: {
          "/api/command": {
            target: "ws://127.0.0.1:8080",
            ws: true,
          },
          "/api": "http://127.0.0.1:8080",
        },
      },
    };
  } else {
    // command === 'build'
    return {
      plugins,
      resolve,
      base: "",
      build: {
        rollupOptions: {
          input: {
            index: path.resolve(import.meta.dirname, "./public/index.html"),
          },
          output: {},
        },
      },
      experimental: {
        renderBuiltUrl(filename, { hostType }) {
          if (hostType === "js") {
            return { runtime: `window.__prependStaticUrl("${filename}")` };
          } else if (hostType === "html") {
            return `[{[ .StaticURL ]}]/${filename}`;
          } else {
            return { relative: true };
          }
        },
      },
    };
  }
});
