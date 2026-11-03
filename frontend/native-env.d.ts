// TypeScript 7 does not yet implement Vue SFC transformation. vue-tsc checks
// every included SFC using Microsoft's TS 6 API compatibility package; the
// native compiler independently checks the application's TypeScript modules.
declare module "*.vue" {
  import type { DefineComponent } from "vue";
  const component: DefineComponent;
  export default component;
}
