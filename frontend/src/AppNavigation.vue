<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch, type Component } from "vue";
import { Boxes, ChevronDown, Database, PackageCheck, ShieldCheck, Wrench, X, Settings } from "lucide-vue-next";
import { locale } from "./preferences";

type NavItem = { key: string; label: string; icon: Component };
const props = defineProps<{
  items: NavItem[];
  active: string;
  organization: string;
  allBranches: boolean;
  pending: number;
  mobileOpen: boolean;
}>();
const emit = defineEmits<{ navigate: [key: string]; close: [] }>();
const sidebar = ref<HTMLElement | null>(null);
const mobile = ref(false);
const expanded = ref<string[]>(["inventory"]);
const copy = (id: string, en: string) => locale.value === "id" ? id : en;
const groups = computed(() => [
  { key: "inventory", label: copy("Aset & inventaris", "Assets & inventory"), icon: Boxes, keys: ["assets", "lifecycle"] },
  { key: "operations", label: copy("Operasional", "Operations"), icon: Wrench, keys: ["requests", "maintenance", "stocktakes"] },
  { key: "master", label: copy("Data master", "Master data"), icon: Database, keys: ["branches", "locations", "categories"] },
  { key: "setup", label: "General Setup", icon: Settings, keys: ["general-codes", "general-code-details"] },
  { key: "control", label: copy("Kontrol & akses", "Control & access"), icon: ShieldCheck, keys: ["audit", "activity", "users"] },
].map(group => ({ ...group, items: props.items.filter(item => group.keys.includes(item.key)) }))
  .filter(group => group.items.length));
const dashboard = computed(() => props.items.find(item => item.key === "dashboard"));
const finance = computed(() => props.items.find(item => item.key === "finance"));
const modalOpen = computed(() => mobile.value && props.mobileOpen);
function toggle(key: string) {
  expanded.value = expanded.value.includes(key)
    ? expanded.value.filter(value => value !== key) : [...expanded.value, key];
}
function revealActive() {
  const scroller = sidebar.value?.querySelector<HTMLElement>(".app-navigation");
  const active = scroller?.querySelector<HTMLElement>('[aria-current="page"]');
  if (!scroller || !active || active.offsetParent === null) return;
  const viewport = scroller.getBoundingClientRect(), target = active.getBoundingClientRect();
  if (target.top < viewport.top) scroller.scrollTop += target.top - viewport.top - 8;
  else if (target.bottom > viewport.bottom) scroller.scrollTop += target.bottom - viewport.bottom + 8;
}
watch(() => props.active, async () => {
  const group = groups.value.find(group => group.keys.includes(props.active));
  if (group && !expanded.value.includes(group.key)) expanded.value.push(group.key);
  await nextTick();
  revealActive();
}, { immediate: true });

let media: MediaQueryList;
let previousOverflow = "";
let locked = false;
let previousFocus: HTMLElement | null = null;
function unlock() {
  if (locked) document.body.style.overflow = previousOverflow;
  locked = false;
}
function syncMobile() {
  mobile.value = media.matches;
  if (!mobile.value) emit("close");
}
watch(modalOpen, async open => {
  if (open) {
    previousFocus = document.activeElement as HTMLElement;
    previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    locked = true;
    await nextTick();
    sidebar.value?.querySelector<HTMLButtonElement>(".nav-close")?.focus();
    revealActive();
  } else {
    unlock();
    await nextTick();
    if (previousFocus?.isConnected && previousFocus.offsetParent !== null) previousFocus.focus();
    previousFocus = null;
  }
});
function keydown(event: KeyboardEvent) {
  if (!modalOpen.value) return;
  if (event.key === "Escape") { event.preventDefault(); emit("close"); return; }
  if (event.key !== "Tab") return;
  const buttons = Array.from(sidebar.value?.querySelectorAll<HTMLButtonElement>("button:not([disabled])") || [])
    .filter(button => button.offsetParent !== null);
  const first = buttons[0], last = buttons.at(-1);
  if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus(); }
  else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus(); }
}
onMounted(() => {
  media = window.matchMedia("(max-width: 960px)");
  syncMobile();
  media.addEventListener("change", syncMobile);
});
onUnmounted(() => { media?.removeEventListener("change", syncMobile); unlock(); });
</script>

<template>
  <div v-if="modalOpen" class="nav-backdrop" aria-hidden="true" @click="emit('close')"></div>
  <aside id="app-navigation" ref="sidebar" class="app-sidebar" :class="{ 'is-open': modalOpen }"
    :role="modalOpen ? 'dialog' : undefined" :aria-modal="modalOpen ? true : undefined"
    :aria-label="copy('Navigasi utama', 'Main navigation')" @keydown="keydown">
    <div class="nav-brand">
      <div class="brand"><PackageCheck :size="27" aria-hidden="true" /> AssetFlow</div>
      <button v-if="mobile" class="nav-close" :aria-label="copy('Tutup menu', 'Close menu')"
        :title="copy('Tutup menu', 'Close menu')" @click="emit('close')"><X :size="20" /></button>
    </div>
    <div class="nav-organization">
      <b :title="organization">{{ organization }}</b>
      <small>{{ allBranches ? copy('Pusat · semua cabang', 'Head office · all branches') : copy('Ruang kerja cabang', 'Branch workspace') }}</small>
    </div>
    <nav class="app-navigation" :aria-label="copy('Menu aplikasi', 'Application menu')" tabindex="0">
      <button v-if="dashboard" class="nav-item" :class="{ active: active === dashboard.key }"
        :aria-current="active === dashboard.key ? 'page' : undefined" @click="emit('navigate', dashboard.key)">
        <component :is="dashboard.icon" :size="18" aria-hidden="true" /><span>{{ dashboard.label }}</span>
      </button>
      <section v-for="group in groups" :key="group.key" class="nav-group">
        <button class="nav-group-toggle" :class="{ 'contains-active': group.keys.includes(active) }"
          :aria-label="group.label" :aria-expanded="expanded.includes(group.key)" :aria-controls="'nav-group-' + group.key" @click="toggle(group.key)">
          <component :is="group.icon" :size="18" aria-hidden="true" /><span>{{ group.label }}</span>
          <span v-if="group.key === 'operations' && pending && !expanded.includes(group.key)" class="nav-count">{{ pending }}</span>
          <ChevronDown :size="16" class="nav-chevron" :class="{ expanded: expanded.includes(group.key) }" aria-hidden="true" />
        </button>
        <div :id="'nav-group-' + group.key" v-show="expanded.includes(group.key)" class="nav-children">
          <button v-for="item in group.items" :key="item.key" class="nav-item" :class="{ active: active === item.key }"
            :aria-current="active === item.key ? 'page' : undefined" @click="emit('navigate', item.key)">
            <component :is="item.icon" :size="16" aria-hidden="true" /><span>{{ item.label }}</span>
            <span v-if="item.key === 'requests' && pending" class="nav-count">{{ pending }}</span>
          </button>
        </div>
      </section>
      <button v-if="finance" class="nav-item" :class="{ active: active === finance.key }"
        :aria-current="active === finance.key ? 'page' : undefined" @click="emit('navigate', finance.key)">
        <component :is="finance.icon" :size="18" aria-hidden="true" /><span>{{ finance.label }}</span>
      </button>
    </nav>
    <div class="nav-status"><span class="live-dot"></span>{{ copy('Workspace terhubung', 'Connected workspace') }}</div>
  </aside>
</template>

<style scoped>
.app-sidebar { position: fixed; inset: 0 auto 0 0; z-index: 40; width: 244px; height: 100vh; height: 100dvh; padding: 24px 14px 16px; display: flex; flex-direction: column; gap: 20px; background: var(--nav); color: var(--nav-text); }
.nav-brand { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-shrink: 0; }
.nav-brand .brand { padding: 0 8px; font-size: 21px; margin: 0; }
.nav-brand svg, .app-navigation svg { flex-shrink: 0; }
.nav-organization { flex-shrink: 0; padding: 12px 10px; border-block: 1px solid color-mix(in srgb, var(--nav-text) 20%, var(--nav)); }
.nav-organization b { display: block; font-size: 13px; overflow-wrap: anywhere; }
.nav-organization small { margin-top: 4px; color: var(--nav-text); font-size: 11px; }
.app-navigation { flex: 1; min-height: 0; overflow-y: auto; overflow-x: hidden; overscroll-behavior: contain; scrollbar-width: thin; scrollbar-color: #7298aa transparent; display: flex; flex-direction: column; gap: 8px; padding: 3px 5px 12px 3px; scroll-padding: 12px; }
.app-navigation button { width: 100%; min-height: 44px; padding: 10px; gap: 10px; font-size: 13px; line-height: 1.4; text-align: left; white-space: normal; color: var(--nav-text); border-radius: 6px; background: transparent; justify-content: flex-start; }
.app-navigation button > span:not(.nav-count) { flex: 1; min-width: 0; overflow-wrap: anywhere; }
.app-navigation button:hover, .nav-close:hover { background: color-mix(in srgb, var(--nav-text) 12%, var(--nav)); }
.app-navigation button.active { background: #d9eff9; color: #163f54; font-weight: 650; }
.nav-group-toggle.contains-active { color: #eff8fc; background: color-mix(in srgb, var(--nav-text) 8%, var(--nav)); }
.nav-chevron { transition: transform .15s; }
.nav-chevron.expanded { transform: rotate(180deg); }
.nav-children { margin: 4px 0 0 14px; padding-left: 8px; border-left: 1px solid color-mix(in srgb, var(--nav-text) 25%, var(--nav)); }
.nav-children button { margin: 3px 0; font-size: 12px; }
.nav-count { flex: 0 0 auto; margin-left: auto; background: #d9eff9; color: #163f54; }
.nav-status { flex-shrink: 0; font-size: 11px; padding: 0 10px; }
.nav-close { width: 40px; height: 40px; padding: 8px; border-radius: 6px; background: transparent; color: var(--nav-text); }
.nav-backdrop { position: fixed; inset: 0; background: #07131d99; z-index: 59; }
@media (max-width: 960px) {
  .app-sidebar { display: none; width: min(320px, calc(100vw - 32px)); z-index: 60; padding: 18px 12px 16px; }
  .app-sidebar.is-open { display: flex; }
  .app-navigation { flex-direction: column; }
  .app-navigation button svg { display: block; }
}
@media (max-height: 480px) {
  .app-sidebar { gap: 10px; padding-top: 12px; }
  .nav-organization { padding-block: 7px; }
  .nav-status { display: none; }
}
@media (prefers-reduced-motion: reduce) { .nav-chevron { transition: none; } }
</style>
