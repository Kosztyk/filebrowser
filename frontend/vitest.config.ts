import { defineConfig, mergeConfig } from "vitest/config";
import viteConfig from "./vite.config.ts";

export default mergeConfig(
  viteConfig({ command: "serve", mode: "test" }),
  defineConfig({
    test: {
      include: ["src/**/__tests__/**/*.test.ts"],
    },
  })
);
