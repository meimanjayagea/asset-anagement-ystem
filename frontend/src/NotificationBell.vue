<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from "vue";
import { Bell, Check, X } from "lucide-vue-next";
import { api, date } from "./api";
const props = defineProps<{ branchId: number }>();
const items = ref<any[]>([]),
  open = ref(false),
  error = ref("");
let timer: ReturnType<typeof setInterval>;
const unread = computed(() => items.value.filter((item) => !item.read).length);
async function load() {
  try {
    items.value = await api(`/notifications?branch_id=${props.branchId}`);
    error.value = "";
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function read(item: any) {
  try {
    await api("/notifications/read", { key: item.key });
    item.read = true;
  } catch (e) {
    error.value = (e as Error).message;
  }
}
watch(() => props.branchId, load);
onMounted(() => {
  load();
  timer = setInterval(() => {
    if (!document.hidden) load();
  }, 60000);
});
onUnmounted(() => clearInterval(timer));
</script>
<template>
  <div class="notification-control">
    <button
      class="icon"
      title="Notifikasi"
      aria-label="Notifikasi"
      :aria-expanded="open"
      @click="
        open = !open;
        open && load();
      "
    >
      <Bell :size="19" /><span v-if="unread" class="notification-count">{{
        unread
      }}</span>
    </button>
    <section
      v-if="open"
      class="notification-menu"
      aria-label="Notifikasi jatuh tempo"
    >
      <header>
        <h2>Notifikasi</h2>
        <button
          class="icon"
          title="Tutup"
          aria-label="Tutup notifikasi"
          @click="open = false"
        >
          <X :size="18" />
        </button>
      </header>
      <p v-if="error" role="alert" class="alert">{{ error }}</p>
      <p v-if="!items.length">Tidak ada pengingat jatuh tempo.</p>
      <article
        v-for="item in items"
        :key="item.key"
        :class="{ read: item.read }"
      >
        <div>
          <strong>{{ item.tag }} · {{ item.name }}</strong
          ><small
            >{{
              item.kind === "loan"
                ? "Pengembalian pinjaman"
                : item.kind === "work_order"
                  ? "Work order"
                  : "Servis berkala"
            }}
            · {{ date(item.due_at) }}
            <b v-if="item.overdue">Terlambat</b></small
          >
        </div>
        <button
          v-if="!item.read"
          class="icon"
          title="Tandai dibaca"
          aria-label="Tandai dibaca"
          @click="read(item)"
        >
          <Check :size="17" />
        </button>
      </article>
    </section>
  </div>
</template>
<style scoped>
.notification-control {
  position: relative;
}
.notification-count {
  font-size: 11px;
  min-width: 18px;
  line-height: 18px;
  background: var(--danger);
  color: var(--surface);
  border-radius: 4px;
}
.notification-menu {
  position: absolute;
  right: 0;
  top: 40px;
  width: min(380px, calc(100vw - 32px));
  max-height: 70vh;
  overflow: auto;
  z-index: 50;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 6px;
  box-shadow: 0 6px 20px #0002;
  padding: 16px;
}
.notification-menu header,
.notification-menu article {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.notification-menu h2 {
  font-size: 18px;
  margin: 0;
}
.notification-menu article {
  padding: 12px 0;
  border-bottom: 1px solid var(--line);
}
.notification-menu small {
  display: block;
  margin-top: 4px;
}
.read {
  opacity: 0.65;
}
</style>
