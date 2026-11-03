<template>
  <base-modal v-if="modal != null" :prompt="currentPromptName" @closed="close">
    <keep-alive>
      <component :is="modal" />
    </keep-alive>
  </base-modal>
</template>

<script setup lang="ts">
import { computed, defineAsyncComponent } from "vue";
import { storeToRefs } from "pinia";
import { useLayoutStore } from "@/stores/layout";

import BaseModal from "./BaseModal.vue";
const Help = defineAsyncComponent(() => import("./Help.vue"));
const Info = defineAsyncComponent(() => import("./Info.vue"));
const Delete = defineAsyncComponent(() => import("./Delete.vue"));
const DeleteUser = defineAsyncComponent(() => import("./DeleteUser.vue"));
const Download = defineAsyncComponent(() => import("./Download.vue"));
const Rename = defineAsyncComponent(() => import("./Rename.vue"));
const Move = defineAsyncComponent(() => import("./Move.vue"));
const Copy = defineAsyncComponent(() => import("./Copy.vue"));
const NewFile = defineAsyncComponent(() => import("./NewFile.vue"));
const NewDir = defineAsyncComponent(() => import("./NewDir.vue"));
const Replace = defineAsyncComponent(() => import("./Replace.vue"));
const Share = defineAsyncComponent(() => import("./Share.vue"));
const ErugoReverseShare = defineAsyncComponent(
  () => import("./ErugoReverseShare.vue")
);
const ShareDelete = defineAsyncComponent(() => import("./ShareDelete.vue"));
const Upload = defineAsyncComponent(() => import("./Upload.vue"));
const DiscardEditorChanges = defineAsyncComponent(
  () => import("./DiscardEditorChanges.vue")
);
const ResolveConflict = defineAsyncComponent(
  () => import("./ResolveConflict.vue")
);
const CurrentPassword = defineAsyncComponent(
  () => import("./CurrentPassword.vue")
);
const Convert = defineAsyncComponent(() => import("./Convert.vue"));

const layoutStore = useLayoutStore();

const { currentPromptName } = storeToRefs(layoutStore);

const components = new Map<string, any>([
  ["info", Info],
  ["help", Help],
  ["delete", Delete],
  ["rename", Rename],
  ["move", Move],
  ["copy", Copy],
  ["newFile", NewFile],
  ["newDir", NewDir],
  ["download", Download],
  ["replace", Replace],
  ["share", Share],
  ["erugo-reverse-share", ErugoReverseShare],
  ["upload", Upload],
  ["share-delete", ShareDelete],
  ["deleteUser", DeleteUser],
  ["discardEditorChanges", DiscardEditorChanges],
  ["resolve-conflict", ResolveConflict],
  ["current-password", CurrentPassword],
  ["convert", Convert],
]);

const modal = computed(() => {
  const modal = components.get(currentPromptName.value!);
  if (!modal) null;

  return modal;
});

const close = () => {
  if (!layoutStore.currentPrompt) return;
  layoutStore.closeHovers();
};
</script>
