<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from "vue";
import { Archive, ArchiveRestore, Database, Check, X, ShieldCheck, TriangleAlert } from "lucide-vue-next";
import { confirmation, settleConfirmation } from "./confirmation";
import { locale } from "./preferences";

const dialog = ref<HTMLDialogElement | null>(null);
const cancel = ref<HTMLButtonElement | null>(null);
const reason = ref("");
const copy = (id: string, en: string) => locale.value === "id" ? id : en;
const icon = computed(() => ({ archive: Archive, restore: ArchiveRestore, database: Database, check: Check, cancel: X })[confirmation.value?.icon || "archive"]);
let previousFocus: HTMLElement | null = null;
let previousOverflow = "";
let locked = false;

function restoreBackground() {
  if (!locked) return;
  locked = false;
  document.body.style.overflow = previousOverflow;
  if (previousFocus?.isConnected) previousFocus.focus({ preventScroll: true });
}

watch(confirmation, async value => {
  if (!value) {
    dialog.value?.close();
    restoreBackground();
    return;
  }
  reason.value = "";
  previousFocus = document.activeElement as HTMLElement;
  previousOverflow = document.body.style.overflow;
  document.body.style.overflow = "hidden";
  locked = true;
  await nextTick();
  if (!confirmation.value || !dialog.value) return;
  // The native top layer keeps keyboard and pointer input out of every underlying form.
  dialog.value.showModal();
  cancel.value?.focus();
});

function dismiss() { settleConfirmation(null); }
function keydown(event: KeyboardEvent) {
  if (event.key !== "Tab" || !dialog.value) return;
  const controls = Array.from(dialog.value.querySelectorAll<HTMLElement>("button:not([disabled]),textarea:not([disabled])"));
  const first = controls[0], last = controls.at(-1);
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first?.focus();
  }
}
function backdrop(event: MouseEvent) {
  const target = dialog.value;
  if (!target || event.target !== target) return;
  const rect = target.getBoundingClientRect();
  if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) dismiss();
}
function submit() {
  if (confirmation.value?.reasonLabel && !reason.value.trim()) return;
  settleConfirmation({ reason: reason.value.trim() });
}
onUnmounted(() => { settleConfirmation(null); dialog.value?.close(); restoreBackground(); });
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="confirmation-dialog" :data-tone="confirmation?.tone || 'warning'"
      role="alertdialog" aria-modal="true" aria-labelledby="confirmation-title" aria-describedby="confirmation-description"
      @cancel.prevent="dismiss" @click="backdrop" @keydown="keydown">
      <form v-if="confirmation" class="confirmation-content" @submit.prevent="submit">
        <header class="confirmation-heading">
          <span class="confirmation-symbol" aria-hidden="true"><component :is="icon" :size="24" :stroke-width="1.8" /></span>
          <button type="button" class="confirmation-close" :title="copy('Tutup','Close')" :aria-label="copy('Tutup','Close')" @click="dismiss"><X :size="19" /></button>
        </header>
        <div class="confirmation-body">
          <h2 id="confirmation-title">{{ confirmation.title }}</h2>
          <p id="confirmation-description">{{ confirmation.message }}</p>
          <div v-if="confirmation.subject" class="confirmation-subject"><span>{{ copy('Data yang dipilih','Selected record') }}</span><strong>{{ confirmation.subject }}</strong></div>
          <label v-if="confirmation.reasonLabel" class="confirmation-reason">{{ confirmation.reasonLabel }}<textarea v-model="reason" rows="3" required maxlength="2000" /></label>
        </div>
        <footer class="confirmation-footer">
          <button ref="cancel" type="button" class="confirmation-cancel" @click="dismiss">{{ copy('Batal','Cancel') }}</button>
          <button type="submit" class="confirmation-submit" :disabled="!!confirmation.reasonLabel && !reason.trim()"><component :is="confirmation.tone === 'positive' ? ShieldCheck : TriangleAlert" :size="16" aria-hidden="true" />{{ confirmation.confirmLabel }}</button>
        </footer>
      </form>
    </dialog>
  </Teleport>
</template>

<style scoped>
.confirmation-dialog{--action:#8a5415;--action-hover:#6e4210;--symbol:var(--warning);--symbol-bg:var(--warning-soft);box-sizing:border-box;width:min(460px,calc(100% - 32px));max-height:calc(100dvh - 32px);margin:auto;padding:0;overflow-y:auto;border:1px solid var(--line);border-radius:8px;background:var(--surface);color:var(--ink);box-shadow:0 24px 80px #07131d40;letter-spacing:0}
.confirmation-dialog[data-tone="danger"]{--action:#a13b36;--action-hover:#862f2b;--symbol:var(--danger);--symbol-bg:var(--danger-soft)}
.confirmation-dialog[data-tone="positive"]{--action:#24634f;--action-hover:#1b4c3c;--symbol:var(--green);--symbol-bg:color-mix(in srgb,var(--green) 12%,var(--surface))}
.confirmation-dialog::backdrop{background:#07131d80;backdrop-filter:blur(3px)}
.confirmation-dialog[open]{animation:confirmation-enter .16s ease-out}
.confirmation-content{display:block;margin:0;padding:0}
.confirmation-heading{display:flex;align-items:center;justify-content:space-between;padding:24px 24px 0;gap:16px}
.confirmation-symbol{width:48px;height:48px;display:grid;place-items:center;flex:none;border-radius:8px;background:var(--symbol-bg);color:var(--symbol)}
.confirmation-close{display:grid;place-items:center;width:36px;height:36px;flex:none;background:transparent;color:var(--muted);border:0;border-radius:4px;padding:0}
.confirmation-close:hover{background:var(--surface-muted);color:var(--ink)}
.confirmation-body{padding:18px 24px 24px}
.confirmation-body h2{font-size:20px;font-weight:650;line-height:1.35;margin:0 0 10px;overflow-wrap:anywhere;letter-spacing:0}
.confirmation-body p{font-size:14px;line-height:1.65;margin:0;color:var(--muted);overflow-wrap:anywhere}
.confirmation-subject{border-left:3px solid var(--line);margin-top:20px;padding:2px 0 2px 12px;min-width:0}
.confirmation-subject span{display:block;font-size:12px;color:var(--muted);margin-bottom:5px}
.confirmation-subject strong{display:block;font-size:14px;line-height:1.5;font-weight:600;overflow-wrap:anywhere}
.confirmation-reason{display:block;font-size:13px;line-height:1.5;margin-top:20px}
.confirmation-reason textarea{display:block;box-sizing:border-box;width:100%;min-height:88px;max-height:180px;resize:vertical;margin-top:7px;font-size:14px;line-height:1.5}
.confirmation-footer{display:flex;justify-content:flex-end;gap:10px;margin:0;padding:16px 24px;background:var(--canvas);border-top:1px solid var(--line)}
.confirmation-footer button{display:inline-flex;align-items:center;justify-content:center;gap:8px;min-height:42px;padding:10px 16px;border-radius:4px;font-size:14px;font-weight:600;line-height:1.4;letter-spacing:0;white-space:normal;overflow-wrap:anywhere}
.confirmation-cancel{background:var(--surface);border:1px solid var(--line);color:var(--ink)}
.confirmation-cancel:hover{background:var(--surface-muted)}
.confirmation-submit{background:var(--action);border:1px solid transparent;color:#fff}
.confirmation-submit:hover:not(:disabled){background:var(--action-hover)}
.confirmation-submit svg{flex:none}
.confirmation-dialog button:focus-visible,.confirmation-dialog textarea:focus-visible{outline:2px solid var(--focus);outline-offset:3px}
.confirmation-submit:disabled{opacity:.5;cursor:not-allowed}
@keyframes confirmation-enter{from{opacity:0;transform:translateY(8px)}to{opacity:1;transform:translateY(0)}}
@media(max-width:480px){.confirmation-heading{padding:20px 20px 0}.confirmation-body{padding:16px 20px 20px}.confirmation-footer{padding:14px 20px;display:grid;grid-template-columns:1fr 1fr}.confirmation-footer button{min-width:0;padding:10px 12px}.confirmation-body h2{font-size:19px}}
@media(prefers-reduced-motion:reduce){.confirmation-dialog[open]{animation:none}.confirmation-dialog::backdrop{backdrop-filter:none}}
</style>
