<template>
  <Teleport to="body">
    <div
      class="context-menu"
      ref="contextMenu"
      v-show="show"
      :style="{
        top: `${top}px`,
        left: `${left}px`,
        maxHeight: `${maxHeight}px`,
      }"
      @click="handleMenuClick"
      @contextmenu.prevent.stop
    >
      <slot />
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";

const emit = defineEmits(["hide"]);
const props = defineProps<{ show: boolean; pos: { x: number; y: number } }>();
const contextMenu = ref<HTMLElement | null>(null);

const viewportPadding = 8;
const left = ref(viewportPadding);
const top = ref(viewportPadding);
const maxHeight = ref(120);
let resizeObserver: ResizeObserver | null = null;

const updateMenuPosition = () => {
  const menu = contextMenu.value;
  if (!menu || !props.show) return;

  const viewportWidth =
    document.documentElement.clientWidth || window.innerWidth;
  const viewportHeight =
    document.documentElement.clientHeight || window.innerHeight;

  const availableHeight = Math.max(1, viewportHeight - viewportPadding * 2);
  maxHeight.value = availableHeight;

  // scrollHeight measures all actions, including those that overflow the
  // scrollable menu. This keeps positioning correct when conditional actions
  // are added or removed after the menu opens.
  const menuWidth = menu.getBoundingClientRect().width;
  const visibleHeight = Math.min(menu.scrollHeight, availableHeight);

  const maxLeft = Math.max(
    viewportPadding,
    viewportWidth - menuWidth - viewportPadding
  );
  const maxTop = Math.max(
    viewportPadding,
    viewportHeight - visibleHeight - viewportPadding
  );

  left.value = Math.max(viewportPadding, Math.min(props.pos.x, maxLeft));
  top.value = Math.max(viewportPadding, Math.min(props.pos.y, maxTop));
};

const startObservingMenu = () => {
  resizeObserver?.disconnect();
  resizeObserver = null;

  if (!contextMenu.value || typeof ResizeObserver === "undefined") return;

  resizeObserver = new ResizeObserver(() => {
    window.requestAnimationFrame(updateMenuPosition);
  });
  resizeObserver.observe(contextMenu.value);
};

const stopObservingMenu = () => {
  resizeObserver?.disconnect();
  resizeObserver = null;
};

const isEventInsideMenu = (event: Event) => {
  const target = event.target;
  return target instanceof Node && contextMenu.value?.contains(target);
};

const hideContextMenu = () => {
  emit("hide");
};

const handleMenuClick = (event: MouseEvent) => {
  const target = event.target;
  if (target instanceof Element && target.closest(".action")) {
    hideContextMenu();
  }
};

const handlePointerDown = (event: Event) => {
  if (!isEventInsideMenu(event)) {
    hideContextMenu();
  }
};

const handleContextMenu = (event: Event) => {
  if (!isEventInsideMenu(event)) {
    hideContextMenu();
  }
};

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === "Escape") {
    hideContextMenu();
  }
};

const handleScroll = (event: Event) => {
  // Keep the menu open while its own scroll container moves. Close it when
  // the page or another scrollable region moves so it never becomes detached
  // from the item that opened it.
  if (isEventInsideMenu(event)) return;
  hideContextMenu();
};

const addDismissListeners = () => {
  document.addEventListener("pointerdown", handlePointerDown, true);
  document.addEventListener("contextmenu", handleContextMenu, true);
  document.addEventListener("keydown", handleKeydown, true);
  window.addEventListener("blur", hideContextMenu);
  window.addEventListener("resize", hideContextMenu);
  window.addEventListener("scroll", handleScroll, true);
};

const removeDismissListeners = () => {
  document.removeEventListener("pointerdown", handlePointerDown, true);
  document.removeEventListener("contextmenu", handleContextMenu, true);
  document.removeEventListener("keydown", handleKeydown, true);
  window.removeEventListener("blur", hideContextMenu);
  window.removeEventListener("resize", hideContextMenu);
  window.removeEventListener("scroll", handleScroll, true);
};

watch(
  () => [props.show, props.pos.x, props.pos.y] as const,
  async ([show]) => {
    removeDismissListeners();
    stopObservingMenu();

    if (!show) return;

    await nextTick();
    updateMenuPosition();
    startObservingMenu();
    window.requestAnimationFrame(updateMenuPosition);
    addDismissListeners();
  },
  { immediate: true }
);

onBeforeUnmount(() => {
  removeDismissListeners();
  stopObservingMenu();
});
</script>

<style scoped>
.context-menu {
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
  scrollbar-gutter: stable;
  touch-action: pan-y;
  -webkit-overflow-scrolling: touch;
}
</style>
