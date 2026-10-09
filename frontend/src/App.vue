<script setup lang="ts">
import {
  ref,
  reactive,
  onMounted,
  onUnmounted,
  computed,
  watch,
  nextTick,
} from "vue";
import {
  LayoutDashboard,
  Boxes,
  ArrowLeftRight,
  Wrench,
  MapPin,
  Layers,
  ShieldCheck,
  Users,
  LogOut,
  Plus,
  Search,
  ChevronLeft,
  ChevronRight,
  ArrowUpRight,
  PackageCheck,
  X,
  RefreshCw,
  ClipboardCheck,
  KeyRound,
  Building2,
  History,
} from "lucide-vue-next";
import { api, money, date, timestamp, type User, type Asset } from "./api";
const me = ref<User | null>(null),
  initializing = ref(true),
  loading = ref(false),
  saving = ref(false),
  error = ref(""),
  toast = ref(""),
  page = ref("dashboard"),
  modal = ref(""),
  selected = ref<Asset | null>(null);
const login = reactive({ email: "", password: "", org_id: 1 }),
  stats = ref<Record<string, number>>({}),
  assets = ref<Asset[]>([]),
  records = ref<any[]>([]),
  locations = ref<any[]>([]),
  categories = ref<any[]>([]),
  total = ref(0),
  current = ref(1),
  search = ref(""),
  status = ref("");
const branches = ref<any[]>([]),
  branch = ref(0),
  activityEvent = ref("");
const assetLocations = computed(() =>
  locations.value.filter((l) => l.branch_id === form.value.asset_branch_id),
);
const form = ref<Record<string, any>>({});
const canWrite = computed(() => !!me.value && me.value.role !== "auditor"),
  canManage = computed(() =>
    ["admin", "manager"].includes(me.value?.role || ""),
  ),
  canAudit = computed(() =>
    ["admin", "manager", "auditor"].includes(me.value?.role || ""),
  );
const nav = computed(() => [
  { key: "dashboard", label: "Overview", icon: LayoutDashboard },
  { key: "assets", label: "Asset register", icon: Boxes },
  { key: "requests", label: "Approvals", icon: ArrowLeftRight },
  { key: "maintenance", label: "Maintenance", icon: Wrench },
  { key: "stocktakes", label: "Stocktake", icon: ClipboardCheck },
  { key: "branches", label: "Branches", icon: Building2 },
  { key: "locations", label: "Locations", icon: MapPin },
  { key: "categories", label: "Categories", icon: Layers },
  ...(canAudit.value
    ? [
        { key: "audit", label: "Audit trail", icon: ShieldCheck },
        { key: "activity", label: "User activity", icon: History },
      ]
    : []),
  ...(me.value?.role === "admin"
    ? [{ key: "users", label: "Team & access", icon: Users }]
    : []),
]);
const title = computed(
  () => nav.value.find((n) => n.key === page.value)?.label || "Overview",
);
const modalTitle = computed(
  () =>
    (
      ({
        branch: "Tambah cabang",
        policy: "Maintenance policy kategori",
        asset: "Register asset",
        assign: "Assign custodian",
        request: "Transfer / disposal request",
        maintenance: "Schedule maintenance",
        location: "Tambah lokasi",
        category: "Tambah kategori",
        user: "Tambah user",
        decision: form.value.approve ? "Approve request" : "Reject request",
        complete: "Complete maintenance",
        return: "Return asset",
        start: "Start maintenance",
        cancel: "Cancel maintenance",
        stocktake: "Open stocktake",
        observe: "Observe asset tag",
        closeStocktake: "Close stocktake",
        stocktakeItems: "Stocktake snapshot",
        password: "Change password",
        access: "Edit user access",
      }) as Record<string, string>
    )[modal.value] || modal.value,
);
let previousFocus: HTMLElement | null = null;
watch(modal, async (value, old) => {
  if (value && !old) previousFocus = document.activeElement as HTMLElement;
  await nextTick();
  if (value)
    document
      .querySelector<HTMLElement>(
        ".modal input, .modal select, .modal textarea, .modal button",
      )
      ?.focus();
  else previousFocus?.focus();
});
function dialogKeydown(event: KeyboardEvent) {
  if (event.key === "Escape" && !saving.value) {
    modal.value = "";
    return;
  }
  if (event.key !== "Tab") return;
  const root = event.currentTarget as HTMLElement;
  const elements = Array.from(
    root.querySelectorAll<HTMLElement>(
      'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), summary, [tabindex="0"]',
    ),
  ).filter((e) => e.offsetParent !== null);
  const first = elements[0],
    last = elements.at(-1);
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last?.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first?.focus();
  }
}
let toastTimer: ReturnType<typeof setTimeout> | undefined;
function notify(s: string) {
  toast.value = s;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => (toast.value = ""), 4000);
}
let loadGeneration = 0;
async function load() {
  const generation = ++loadGeneration;
  const view = page.value;
  const selectedBranch = branch.value;
  loading.value = true;
  error.value = "";
  records.value = [];
  assets.value = [];
  stats.value = {};
  total.value = 0;
  try {
    if (view === "dashboard") {
      const [summary, data] = await Promise.all([
        api("/dashboard?branch_id=" + selectedBranch),
        api("/assets?size=5&branch_id=" + selectedBranch),
      ]);
      if (generation !== loadGeneration) return;
      stats.value = summary;
      assets.value = data.items;
    } else if (view === "assets") {
      const data = await api(
        "/assets?" +
          new URLSearchParams({
            page: String(current.value),
            size: "25",
            search: search.value,
            status: status.value,
            branch_id: String(selectedBranch),
          }),
      );
      if (generation !== loadGeneration) return;
      assets.value = data.items;
      total.value = data.total;
    } else {
      const data = await api(
        "/" +
          view +
          "?page=" +
          current.value +
          "&size=25&branch_id=" +
          selectedBranch +
          "&event=" +
          activityEvent.value,
      );
      if (generation !== loadGeneration) return;
      records.value = data;
    }
  } catch (e) {
    if (generation === loadGeneration) error.value = (e as Error).message;
  } finally {
    if (generation === loadGeneration) loading.value = false;
  }
}
async function masters() {
  [locations.value, categories.value, branches.value] = await Promise.all([
    api("/locations"),
    api("/categories"),
    api("/branches"),
  ]);
}
async function signIn() {
  saving.value = true;
  error.value = "";
  try {
    me.value = await api("/login", login);
    login.password = "";
    await masters();
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
async function logout() {
  try {
    await api("/logout", {});
    ++loadGeneration;
    loading.value = false;
    me.value = null;
    branch.value = 0;
    assets.value = [];
    records.value = [];
  } catch (e) {
    error.value = (e as Error).message;
  }
}
function expired() {
  ++loadGeneration;
  loading.value = false;
  me.value = null;
  modal.value = "";
  error.value = "Sesi berakhir. Silakan login kembali.";
}
async function navigate(key: string) {
  page.value = key;
  current.value = 1;
  await load();
}
async function turn(delta: number) {
  current.value = Math.max(1, current.value + delta);
  await load();
}
async function filter() {
  current.value = 1;
  await load();
}
function open(kind: string, a: Asset | null = null) {
  error.value = "";
  selected.value = a;
  modal.value = kind;
  form.value = {};
  const today = new Date().toLocaleDateString("en-CA");
  if (kind === "asset")
    form.value = {
      tag: "",
      name: "",
      serial_number: "",
      asset_branch_id: branch.value || branches.value[0]?.id,
      category_id: categories.value[0]?.id,
      location_id: locations.value.find(
        (l) => l.branch_id === (branch.value || branches.value[0]?.id),
      )?.id,
      purchase_date: today,
      purchase_cost: 0,
      salvage_value: 0,
      useful_life_months: 48,
      warranty_until: "",
    };
  if (kind === "request")
    form.value = {
      asset_id: a?.id,
      version: a?.version,
      kind: "transfer",
      target_location_id: locations.value.find((l) => l.id !== a?.location_id)
        ?.id,
      reason: "",
    };
  if (kind === "maintenance")
    form.value = {
      asset_id: a?.id,
      version: a?.version,
      title: "",
      due_date: a?.next_maintenance_date || today,
    };
  if (kind === "branch") form.value = { code: "", name: "", address: "" };
  if (kind === "location")
    form.value = { name: "", branch_id: branch.value || branches.value[0]?.id };
  if (kind === "category")
    form.value = {
      name: "",
      useful_life_months: 48,
      maintenance_interval_days: 0,
      maintenance_instructions: "",
    };
  if (kind === "stocktake")
    form.value = { title: "", location_id: locations.value[0]?.id };
  if (kind === "user")
    form.value = {
      name: "",
      email: "",
      password: "",
      role: "operator",
      all_branches: false,
      branch_ids: branch.value ? [branch.value] : [],
    };
}
async function submit() {
  saving.value = true;
  error.value = "";
  try {
    let endpoint = "";
    let body = { ...form.value };
    switch (modal.value) {
      case "branch":
        endpoint = "/branches";
        break;
      case "policy":
        endpoint = `/categories/${body.id}/policy`;
        body = {
          maintenance_interval_days: body.maintenance_interval_days,
          maintenance_instructions: body.maintenance_instructions,
          version: body.version,
          apply_to_existing: !!body.apply_to_existing,
        };
        break;
      case "stocktake":
        endpoint = "/stocktakes";
        break;
      case "observe":
        endpoint = `/stocktakes/${body.id}/observe`;
        body = { tag: body.tag, notes: body.notes || "" };
        break;
      case "closeStocktake":
        endpoint = `/stocktakes/${body.id}/close`;
        body = { acknowledge_discrepancies: !!body.acknowledge };
        break;
      case "password":
        endpoint = "/password";
        body = {
          current_password: body.current_password,
          new_password: body.new_password,
        };
        break;
      case "access":
        endpoint = `/users/${body.id}/access`;
        body = {
          role: body.role,
          active: body.active,
          all_branches: body.role === "admin" || !!body.all_branches,
          branch_ids: body.branch_ids || [],
        };
        break;
      case "asset":
        endpoint = "/assets";
        delete body.asset_branch_id;
        body.warranty_until = body.warranty_until || null;
        break;
      case "location":
        endpoint = "/locations";
        break;
      case "category":
        endpoint = "/categories";
        break;
      case "user":
        endpoint = "/users";
        break;
      case "assign":
        endpoint = `/assets/${selected.value!.id}/action`;
        body = {
          action: "assign",
          custodian: body.custodian,
          version: selected.value!.version,
        };
        break;
      case "request":
        endpoint = "/requests";
        if (body.kind === "dispose") body.target_location_id = null;
        break;
      case "maintenance":
        endpoint = "/maintenance";
        break;
      case "decision":
        endpoint = `/requests/${body.id}/decision`;
        body = { approve: body.approve, note: body.note || "" };
        break;
      case "complete":
        endpoint = `/maintenance/${body.id}/action`;
        body = {
          action: "complete",
          version: body.version,
          cost: body.cost || 0,
          notes: body.notes || "",
        };
        break;
      case "return":
        endpoint = `/assets/${selected.value!.id}/action`;
        body = { action: "return", version: selected.value!.version };
        break;
      case "start":
      case "cancel":
        endpoint = `/maintenance/${body.id}/action`;
        body = {
          action: modal.value,
          version: body.version,
          cost: 0,
          notes: "",
        };
        break;
    }
    await api(endpoint, body);
    if (modal.value === "password") {
      me.value = null;
      modal.value = "";
      notify("Password diperbarui. Silakan login kembali.");
      return;
    }
    modal.value = "";
    notify("Perubahan berhasil disimpan");
    await masters();
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
async function viewStocktake(r: any) {
  open("stocktakeItems");
  form.value = {
    id: r.id,
    title: r.title,
    page: 1,
    items: [],
    status: r.status,
  };
  await stocktakePage(0);
}
async function stocktakePage(delta: number) {
  try {
    form.value.page = Math.max(1, form.value.page + delta);
    form.value.items = await api(
      `/stocktakes/${form.value.id}/items?page=${form.value.page}&size=25`,
    );
  } catch (e) {
    error.value = (e as Error).message;
  }
}
function decision(r: any, approve: boolean) {
  open("decision");
  form.value = { id: r.id, approve, note: "" };
}
function maintenanceAction(r: any, action: string) {
  open(action);
  form.value = { id: r.id, version: r.asset_version, cost: 0, notes: "" };
}
async function exportCSV() {
  saving.value = true;
  error.value = "";
  try {
    const response = await fetch("/api/exports/assets", {
      method: "POST",
      credentials: "same-origin",
      headers: {
        "Content-Type": "application/json",
        "X-Requested-With": "AssetFlow",
      },
      body: JSON.stringify({
        page: current.value,
        size: 25,
        search: search.value,
        status: status.value,
        branch_id: branch.value,
      }),
    });
    if (!response.ok) {
      const result = await response.json();
      throw new Error(result.error || "Export gagal");
    }
    const blob = await response.blob(),
      url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "assets-current-page.csv";
    link.click();
    URL.revokeObjectURL(url);
    notify("Export CSV berhasil dan diaudit");
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
onMounted(async () => {
  window.addEventListener("session-expired", expired);
  try {
    me.value = await api("/me");
    await masters();
    await load();
  } catch {
    me.value = null;
    error.value = "";
  } finally {
    initializing.value = false;
  }
});
onUnmounted(() => {
  window.removeEventListener("session-expired", expired);
  clearTimeout(toastTimer);
});
</script>
<template>
  <div v-if="initializing" class="boot">Loading AssetFlow…</div>
  <div v-else-if="!me" class="login-wrap">
    <section class="login-story">
      <div class="brand">
        <PackageCheck :size="30" /> AssetFlow<span class="edition"
          >WORKSPACE</span
        >
      </div>
      <div>
        <span class="eyebrow">CONTROL EVERY ASSET</span>
        <h1>Your assets.<br />Your accountability.</h1>
        <p>
          Satu tempat untuk inventaris, penanggung jawab, maintenance, dan
          seluruh riwayat aset organisasi.
        </p>
        <div class="story-line">
          01 / Visibility &nbsp; 02 / Governance &nbsp; 03 / Lifecycle
        </div>
      </div>
      <small>Asset management · Go + Vue</small>
    </section>
    <section class="login-form">
      <div class="login-box">
        <span class="eyebrow">WELCOME BACK</span>
        <h2>Masuk ke workspace</h2>
        <p>Gunakan akun organisasi Anda.</p>
        <form @submit.prevent="signIn">
          <label
            >Organization ID<input
              v-model.number="login.org_id"
              type="number"
              min="1"
              required /></label
          ><label
            >Email<input
              v-model="login.email"
              type="email"
              autocomplete="username"
              required /></label
          ><label
            >Password<input
              v-model="login.password"
              type="password"
              autocomplete="current-password"
              required
              maxlength="72"
          /></label>
          <div v-if="error" class="alert" role="alert">{{ error }}</div>
          <button class="primary" :disabled="saving">
            {{ saving ? "Memproses…" : "Masuk workspace" }}
            <ArrowUpRight :size="17" />
          </button>
        </form>
        <small>Akun pertama dibuat melalui perintah bootstrap.</small>
      </div>
    </section>
  </div>
  <div v-else class="workspace" :inert="!!modal">
    <aside>
      <div class="brand"><PackageCheck :size="27" /> AssetFlow</div>
      <div class="org">
        <span class="org-icon">AF</span>
        <div>
          <b>Organization {{ me.org_id }}</b
          ><small>Asset workspace</small>
        </div>
      </div>
      <span class="nav-label">WORKSPACE</span>
      <nav>
        <button
          v-for="n in nav"
          :key="n.key"
          @click="navigate(n.key)"
          :class="{ active: page === n.key }"
        >
          <component :is="n.icon" :size="18" />{{ n.label
          }}<span
            v-if="n.key === 'requests' && stats.pending_requests"
            class="nav-count"
            >{{ stats.pending_requests }}</span
          >
        </button>
      </nav>
      <div class="aside-bottom">
        <span class="live-dot"></span> Connected workspace<small
          >Governance built into every move.</small
        >
      </div>
    </aside>
    <div class="main">
      <header>
        <div class="breadcrumb">Workspace <span>/</span> {{ title }}</div>
        <div class="user">
          <span class="avatar">{{ me.name.slice(0, 1) }}</span>
          <div>
            <b>{{ me.name }}</b
            ><small>{{ me.role }}</small>
          </div>
          <button
            class="icon"
            @click="open('password')"
            aria-label="Change password"
          >
            <KeyRound :size="18" /></button
          ><button class="icon" @click="logout" aria-label="Logout">
            <LogOut :size="18" />
          </button>
        </div>
      </header>
      <main>
        <div class="page-heading">
          <div>
            <span class="eyebrow">ASSET OPERATIONS</span>
            <h1>{{ title }}</h1>
            <p>
              {{
                page === "dashboard"
                  ? "A clear view of your assets, responsibilities, and next actions."
                  : "Kelola data dan proses aset dengan riwayat yang dapat ditelusuri."
              }}
            </p>
          </div>
          <div class="buttons">
            <select
              class="branch-filter"
              v-model.number="branch"
              @change="filter"
              aria-label="Filter cabang"
            >
              <option :value="0">Semua cabang yang diizinkan</option>
              <option v-for="b in branches" :key="b.id" :value="b.id">
                {{ b.code }} · {{ b.name }}
              </option>
            </select>
            <button class="secondary" @click="load" :disabled="loading">
              <RefreshCw :size="16" /> Refresh</button
            ><button
              v-if="page === 'branches' && me.role === 'admin'"
              class="primary"
              @click="open('branch')"
            >
              <Plus :size="17" /> Cabang</button
            ><button
              v-if="page === 'assets' && canWrite"
              class="primary"
              @click="open('asset')"
            >
              <Plus :size="17" /> Register asset</button
            ><button
              v-if="page === 'locations' && canManage"
              class="primary"
              @click="open('location')"
            >
              <Plus :size="17" /> Lokasi</button
            ><button
              v-if="page === 'categories' && canManage"
              class="primary"
              @click="open('category')"
            >
              <Plus :size="17" /> Kategori</button
            ><button
              v-if="page === 'stocktakes' && canWrite"
              class="primary"
              @click="open('stocktake')"
            >
              <Plus :size="17" /> Stocktake</button
            ><button
              v-if="page === 'users' && me.role === 'admin'"
              class="primary"
              @click="open('user')"
            >
              <Plus :size="17" /> User
            </button>
          </div>
        </div>
        <div v-if="error && !modal" class="alert" role="alert">{{ error }}</div>
        <div v-if="loading" class="loading" aria-live="polite">
          Memuat data…
        </div>
        <template v-if="page === 'dashboard'"
          ><div class="hint due-callout">
            {{ stats.maintenance_due_assets || 0 }} aset telah jatuh tempo
            maintenance berdasarkan policy kategori. Buka register aset untuk
            menjadwalkan pekerjaan.
          </div>
          <div class="kpis">
            <article class="kpi featured">
              <span>Total registered assets <Boxes :size="20" /></span
              ><strong>{{ stats.total || 0 }}</strong
              ><small>Across all locations</small>
            </article>
            <article class="kpi">
              <span>Available <PackageCheck :size="20" /></span
              ><strong>{{ stats.available || 0 }}</strong
              ><small>Ready to be assigned</small>
            </article>
            <article class="kpi">
              <span>Assigned <Users :size="20" /></span
              ><strong>{{ stats.assigned || 0 }}</strong
              ><small>With a custodian</small>
            </article>
            <article class="kpi">
              <span>Under maintenance <Wrench :size="20" /></span
              ><strong>{{ stats.maintenance || 0 }}</strong
              ><small>In progress</small>
            </article>
          </div>
          <div class="overview-grid">
            <section class="panel value-panel">
              <span class="eyebrow">PORTFOLIO VALUE</span>
              <h2>{{ money(stats.purchase_value) }}</h2>
              <p>Total acquisition cost of non-disposed assets.</p>
              <div class="bar">
                <div
                  :style="{
                    width: stats.total
                      ? ((stats.assigned || 0) / stats.total) * 100 + '%'
                      : '0%',
                  }"
                ></div>
              </div>
              <div class="legend">
                <span><i></i> Assigned {{ stats.assigned || 0 }}</span
                ><span>Disposed {{ stats.disposed || 0 }}</span>
              </div>
            </section>
            <section class="panel attention">
              <span class="eyebrow">REQUIRES ATTENTION</span
              ><button @click="navigate('requests')">
                <div>
                  <b>Pending approvals</b
                  ><small>Transfer & disposal requests</small>
                </div>
                <strong>{{ stats.pending_requests || 0 }}</strong
                ><ArrowUpRight :size="18" /></button
              ><button @click="navigate('maintenance')">
                <div>
                  <b>Overdue maintenance</b
                  ><small>Open jobs past their due date</small>
                </div>
                <strong class="amber">{{
                  stats.overdue_maintenance || 0
                }}</strong
                ><ArrowUpRight :size="18" />
              </button>
            </section>
          </div>
          <section class="panel">
            <div class="panel-heading">
              <h3>Recently registered</h3>
              <button class="text-btn" @click="navigate('assets')">
                View register <ArrowUpRight :size="16" />
              </button>
            </div>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>Asset</th>
                    <th>Category</th>
                    <th>Location</th>
                    <th>Status</th>
                    <th>Acquisition cost</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="a in assets" :key="a.id">
                    <td>
                      <b>{{ a.name }}</b
                      ><small>{{ a.tag }}</small>
                    </td>
                    <td>{{ a.category_name }}</td>
                    <td>{{ a.location_name }}</td>
                    <td>
                      <span class="badge" :class="a.status">{{
                        a.status
                      }}</span>
                    </td>
                    <td>{{ money(a.purchase_cost) }}</td>
                  </tr>
                  <tr v-if="!assets.length">
                    <td colspan="5" class="empty">
                      Belum ada aset. Mulai dengan registrasi aset pertama.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section></template
        >
        <template v-else-if="page === 'assets'"
          ><section class="panel">
            <form class="filters" @submit.prevent="filter">
              <div class="search">
                <Search :size="18" /><input
                  v-model="search"
                  placeholder="Cari nama, tag, atau serial…"
                  maxlength="200"
                />
              </div>
              <select v-model="status" @change="filter">
                <option value="">All statuses</option>
                <option
                  v-for="s in [
                    'available',
                    'assigned',
                    'maintenance',
                    'disposed',
                  ]"
                  :key="s"
                >
                  {{ s }}
                </option></select
              ><button class="secondary">Cari</button
              ><button
                type="button"
                class="secondary"
                @click="exportCSV"
                :disabled="!assets.length"
              >
                Export halaman CSV
              </button>
            </form>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>Asset / Tag</th>
                    <th>Location & custodian</th>
                    <th>Status</th>
                    <th>Cost / book value</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="a in assets" :key="a.id">
                    <td>
                      <b>{{ a.name }}</b
                      ><small>{{ a.tag }} · {{ a.category_name }}</small
                      ><small>SN: {{ a.serial_number || "—" }}</small>
                    </td>
                    <td>
                      {{ a.location_name }}<small>{{ a.branch_name }}</small
                      ><small>{{ a.custodian || "Unassigned" }}</small>
                    </td>
                    <td>
                      <span class="badge" :class="a.status">{{
                        a.status
                      }}</span>
                    </td>
                    <td>
                      {{ money(a.purchase_cost)
                      }}<small>{{ money(a.book_value) }} estimasi buku</small
                      ><small v-if="a.next_maintenance_date"
                        >Maintenance: {{ date(a.next_maintenance_date) }}</small
                      >
                    </td>
                    <td>
                      <div class="row-actions" v-if="canWrite">
                        <template v-if="a.status === 'available'"
                          ><button @click="open('assign', a)">Assign</button
                          ><button @click="open('request', a)">
                            Transfer / disposal</button
                          ><button @click="open('maintenance', a)">
                            Maintenance
                          </button></template
                        ><button
                          v-if="a.status === 'assigned'"
                          @click="open('return', a)"
                        >
                          Return</button
                        ><span
                          v-if="['disposed', 'maintenance'].includes(a.status)"
                          >—</span
                        >
                      </div>
                      <span v-else>Read only</span>
                    </td>
                  </tr>
                  <tr v-if="!assets.length">
                    <td colspan="5" class="empty">
                      Tidak ada aset sesuai filter.
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="pagination">
              <span>{{ total }} assets · Halaman {{ current }}</span>
              <div>
                <button
                  class="icon"
                  @click="turn(-1)"
                  :disabled="current === 1 || loading"
                  aria-label="Previous page"
                >
                  <ChevronLeft :size="18" /></button
                ><button
                  class="icon"
                  @click="turn(1)"
                  :disabled="current * 25 >= total || loading"
                  aria-label="Next page"
                >
                  <ChevronRight :size="18" />
                </button>
              </div>
            </div></section
        ></template>
        <section v-else class="panel">
          <div v-if="page === 'activity'" class="filters">
            <select
              v-model="activityEvent"
              @change="filter"
              aria-label="Activity event"
            >
              <option value="">All activity</option>
              <option
                v-for="event in [
                  'login_success',
                  'login_failed',
                  'login_throttled',
                  'logout',
                  'action',
                  'read',
                  'denied',
                ]"
                :key="event"
              >
                {{ event }}
              </option>
            </select>
          </div>
          <div class="table-scroll">
            <table v-if="page === 'requests'">
              <thead>
                <tr>
                  <th>Asset</th>
                  <th>Request</th>
                  <th>Requester</th>
                  <th>Status</th>
                  <th>Decision</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    <b>{{ r.name }}</b
                    ><small>{{ r.tag }}</small>
                  </td>
                  <td>
                    {{ r.kind }}
                    <small>{{ r.target_location || "Retire asset" }}</small
                    ><small>{{ r.reason }}</small>
                  </td>
                  <td>
                    {{ r.requester
                    }}<small>{{ timestamp(r.created_at) }}</small>
                  </td>
                  <td>
                    <span class="badge" :class="r.status">{{ r.status }}</span>
                  </td>
                  <td>
                    <div
                      v-if="
                        r.status === 'pending' &&
                        canManage &&
                        r.requested_by !== me.id
                      "
                      class="row-actions"
                    >
                      <button @click="decision(r, true)">Approve</button
                      ><button @click="decision(r, false)">Reject</button>
                    </div>
                    <small v-else>{{
                      r.status === "pending"
                        ? "Menunggu checker lain"
                        : r.decision_note || "—"
                    }}</small>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'maintenance'">
              <thead>
                <tr>
                  <th>Job / Asset</th>
                  <th>Due date</th>
                  <th>Status</th>
                  <th>Cost</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    <b>{{ r.title }}</b
                    ><small>{{ r.tag }} · {{ r.name }}</small>
                  </td>
                  <td>{{ date(r.due_date) }}</td>
                  <td>
                    <span class="badge" :class="r.status">{{ r.status }}</span>
                  </td>
                  <td>{{ money(r.cost) }}</td>
                  <td>
                    <div class="row-actions" v-if="canWrite">
                      <template v-if="r.status === 'scheduled'"
                        ><button @click="maintenanceAction(r, 'start')">
                          Start</button
                        ><button @click="maintenanceAction(r, 'cancel')">
                          Cancel
                        </button></template
                      ><button
                        v-if="r.status === 'in_progress'"
                        @click="maintenanceAction(r, 'complete')"
                      >
                        Complete
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'stocktakes'">
              <thead>
                <tr>
                  <th>Stocktake</th>
                  <th>Status</th>
                  <th>Observed / expected</th>
                  <th>Missing</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    <b>{{ r.title }}</b
                    ><small>{{ r.location_name }}</small>
                  </td>
                  <td>
                    <span class="badge">{{ r.status }}</span>
                  </td>
                  <td>{{ r.observed }} / {{ r.expected }}</td>
                  <td>{{ r.missing }}</td>
                  <td>
                    <div class="row-actions">
                      <button @click="viewStocktake(r)">Snapshot</button
                      ><template v-if="r.status === 'open'"
                        ><button
                          v-if="canWrite"
                          @click="
                            open('observe');
                            form = { id: r.id, tag: '', notes: '' };
                          "
                        >
                          Observe tag</button
                        ><button
                          v-if="canManage"
                          @click="
                            open('closeStocktake');
                            form = { id: r.id, acknowledge: false };
                          "
                        >
                          Close
                        </button></template
                      >
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'activity'">
              <thead>
                <tr>
                  <th>Time / user</th>
                  <th>Event</th>
                  <th>Request</th>
                  <th>Status</th>
                  <th>Metadata</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    {{ timestamp(r.created_at)
                    }}<small>{{
                      r.actor_name || r.attempted_email || "Unauthenticated"
                    }}</small>
                  </td>
                  <td>{{ r.event }}</td>
                  <td>{{ r.method }} {{ r.path }}</td>
                  <td>
                    <span
                      class="badge"
                      :class="r.status_code >= 400 ? 'rejected' : 'completed'"
                      >{{ r.status_code }}</span
                    >
                  </td>
                  <td>
                    <details>
                      <summary>Details</summary>
                      <small
                        >{{ r.request_id }}<br />Peer IP: {{ r.peer_ip
                        }}<br />{{ r.user_agent }}<br />{{
                          r.duration_ms
                        }}
                        ms</small
                      >
                    </details>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'branches'">
              <thead>
                <tr>
                  <th>Code</th>
                  <th>Branch</th>
                  <th>Address</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>{{ r.code }}</td>
                  <td>{{ r.name }}</td>
                  <td>{{ r.address || "—" }}</td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'audit'">
              <thead>
                <tr>
                  <th>Time / Actor</th>
                  <th>Action</th>
                  <th>Entity</th>
                  <th>Changes</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    {{ timestamp(r.created_at) }}<small>{{ r.actor }}</small>
                  </td>
                  <td>{{ r.action }}</td>
                  <td>
                    {{ r.entity }} #{{ r.entity_id
                    }}<small>Branch {{ r.branch_id || "Company" }}</small>
                  </td>
                  <td>
                    <details>
                      <summary>View payload</summary>
                      <small
                        >Request: {{ r.request_id }} · IP:
                        {{ r.peer_ip }}</small
                      >
                      <pre>{{
                        JSON.stringify(
                          { before: r.before_data, after: r.after_data },
                          null,
                          2,
                        )
                      }}</pre>
                    </details>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'users'">
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Email</th>
                  <th>Role</th>
                  <th>Status</th>
                  <th>Branch scope</th>
                  <th>Access</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>{{ r.name }}</td>
                  <td>{{ r.email }}</td>
                  <td>
                    <span class="badge">{{ r.role }}</span>
                  </td>
                  <td>{{ r.active ? "Active" : "Inactive" }}</td>
                  <td>
                    {{
                      r.all_branches
                        ? "Company-wide"
                        : (r.branch_ids || [])
                            .map(
                              (id: number) =>
                                branches.find((b) => b.id === id)?.code || id,
                            )
                            .join(", ")
                    }}
                  </td>
                  <td>
                    <button
                      v-if="r.id !== me.id"
                      class="text-btn"
                      @click="
                        open('access');
                        form = {
                          id: r.id,
                          role: r.role,
                          active: r.active,
                          all_branches: r.all_branches,
                          branch_ids: [...(r.branch_ids || [])],
                        };
                      "
                    >
                      Edit access
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Name</th>
                  <th v-if="page === 'locations'">Branch</th>
                  <th v-if="page === 'categories'">Useful life</th>
                  <th v-if="page === 'categories'">Maintenance policy</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>#{{ r.id }}</td>
                  <td>{{ r.name }}</td>
                  <td v-if="page === 'locations'">
                    {{ r.branch_code }} · {{ r.branch_name }}
                  </td>
                  <td v-if="page === 'categories'">
                    {{ r.useful_life_months }} months
                  </td>
                  <td v-if="page === 'categories'">
                    {{
                      r.maintenance_interval_days
                        ? `Every ${r.maintenance_interval_days} days`
                        : "Manual only"
                    }}<small>{{ r.maintenance_instructions }}</small
                    ><button
                      v-if="me.role === 'admin'"
                      class="text-btn"
                      @click="
                        open('policy');
                        form = {
                          id: r.id,
                          version: r.version,
                          maintenance_interval_days:
                            r.maintenance_interval_days,
                          maintenance_instructions: r.maintenance_instructions,
                          apply_to_existing: false,
                        };
                      "
                    >
                      Edit policy
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
            <div v-if="!records.length" class="empty">Belum ada data.</div>
          </div>
          <div
            v-if="
              [
                'requests',
                'maintenance',
                'audit',
                'stocktakes',
                'activity',
              ].includes(page)
            "
            class="pagination"
          >
            <span>Halaman {{ current }} · Maksimal 25 baris</span>
            <div>
              <button
                class="icon"
                @click="turn(-1)"
                :disabled="current === 1 || loading"
                aria-label="Previous page"
              >
                <ChevronLeft :size="18" /></button
              ><button
                class="icon"
                @click="turn(1)"
                :disabled="records.length < 25 || loading"
                aria-label="Next page"
              >
                <ChevronRight :size="18" />
              </button>
            </div>
          </div>
        </section>
        <footer>
          AssetFlow <span>Operational clarity. Accountable ownership.</span>
        </footer>
      </main>
    </div>
  </div>
  <div v-if="toast" class="toast" role="status">{{ toast }}</div>
  <div
    v-if="modal"
    class="modal-backdrop"
    @click.self="!saving && (modal = '')"
  >
    <section
      class="modal"
      role="dialog"
      aria-modal="true"
      aria-labelledby="dialog-title"
      @keydown="dialogKeydown"
    >
      <div class="modal-heading">
        <h2 id="dialog-title">{{ modalTitle }}</h2>
        <button
          class="icon"
          @click="modal = ''"
          :disabled="saving"
          aria-label="Close"
        >
          <X :size="20" />
        </button>
      </div>
      <form @submit.prevent="submit">
        <template v-if="modal === 'branch'"
          ><label
            >Code<input
              v-model="form.code"
              required
              maxlength="30"
              placeholder="JKT-01" /></label
          ><label
            >Name<input
              v-model="form.name"
              required
              minlength="2"
              maxlength="150" /></label
          ><label
            >Address<textarea
              v-model="form.address"
              maxlength="1000"
            ></textarea></label></template
        ><template v-if="modal === 'location'"
          ><label
            >Branch<select v-model.number="form.branch_id" required>
              <option v-for="b in branches" :key="b.id" :value="b.id">
                {{ b.code }} · {{ b.name }}
              </option>
            </select></label
          ></template
        ><template v-if="['category', 'policy'].includes(modal)"
          ><label
            >Maintenance interval (days)<input
              v-model.number="form.maintenance_interval_days"
              type="number"
              min="0"
              max="3650"
              required /></label
          ><label
            >Maintenance instructions<textarea
              v-model="form.maintenance_instructions"
              maxlength="2000"
            ></textarea>
          </label>
          <p class="hint">
            0 = manual only. Interval dipakai pada aset baru dan setelah
            maintenance selesai.
          </p>
          <label v-if="modal === 'policy'" class="check-label"
            ><input type="checkbox" v-model="form.apply_to_existing" /> Terapkan
            juga ke aset existing tanpa request/job aktif.</label
          ></template
        ><template v-if="['user', 'access'].includes(modal)"
          ><label class="check-label"
            ><input
              type="checkbox"
              v-model="form.all_branches"
              :disabled="form.role === 'admin'"
            />
            Company-wide branch access (admin selalu company-wide)</label
          >
          <fieldset v-if="!form.all_branches && form.role !== 'admin'">
            <legend>Allowed branches</legend>
            <label v-for="b in branches" :key="b.id" class="check-label"
              ><input
                type="checkbox"
                v-model="form.branch_ids"
                :value="b.id"
              />{{ b.code }} · {{ b.name }}</label
            >
          </fieldset></template
        >
        <template v-if="modal === 'stocktake'"
          ><label
            >Title<input
              v-model="form.title"
              required
              minlength="3"
              maxlength="200" /></label
          ><label
            >Location<select v-model.number="form.location_id" required>
              <option v-for="l in locations" :key="l.id" :value="l.id">
                {{ l.branch_name ? l.branch_name + " · " : "" }}{{ l.name }}
              </option>
            </select></label
          >
          <p class="hint">
            Snapshot aset non-disposed di lokasi ini. Perubahan setelah snapshot
            akan terlihat sebagai discrepancy.
          </p></template
        ><template v-if="modal === 'observe'"
          ><label
            >Asset tag<input
              v-model="form.tag"
              required
              maxlength="80"
              placeholder="Scan atau ketik tag" /></label
          ><label
            >Notes<textarea
              v-model="form.notes"
              maxlength="1000"
            ></textarea></label></template
        ><template v-if="modal === 'closeStocktake'"
          ><p>
            Snapshot akan dikunci. Missing asset harus diinvestigasi; aksi ini
            tidak mengubah status aset.
          </p>
          <label class="check-label"
            ><input type="checkbox" v-model="form.acknowledge" /> Saya mengakui
            discrepancy missing/changed yang ada.</label
          ></template
        ><template v-if="modal === 'stocktakeItems'"
          ><p>{{ form.title }}</p>
          <div class="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Tag</th>
                  <th>Observed</th>
                  <th>Version</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="i in form.items" :key="i.asset_id">
                  <td>
                    {{ i.expected_tag }}<small>{{ i.name }}</small>
                  </td>
                  <td>
                    {{ i.observed ? "Yes" : "Missing"
                    }}<small>{{ i.notes }}</small>
                  </td>
                  <td>
                    {{ i.expected_version }} → {{ i.current_version
                    }}<small v-if="i.expected_version !== i.current_version"
                      >Changed after snapshot</small
                    >
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="pagination">
            <span>Page {{ form.page }}</span>
            <div>
              <button
                type="button"
                @click="stocktakePage(-1)"
                :disabled="form.page === 1"
                class="secondary"
              >
                Prev</button
              ><button
                type="button"
                @click="stocktakePage(1)"
                :disabled="form.items.length < 25"
                class="secondary"
              >
                Next
              </button>
            </div>
          </div></template
        ><template v-if="modal === 'password'"
          ><label
            >Current password<input
              type="password"
              v-model="form.current_password"
              required
              maxlength="72"
              autocomplete="current-password" /></label
          ><label
            >New password<input
              type="password"
              v-model="form.new_password"
              required
              minlength="12"
              maxlength="72"
              autocomplete="new-password"
          /></label>
          <p>Seluruh sesi akan dicabut setelah password berubah.</p></template
        ><template v-if="modal === 'access'"
          ><label
            >Role<select v-model="form.role">
              <option
                v-for="role in ['admin', 'manager', 'operator', 'auditor']"
                :key="role"
              >
                {{ role }}
              </option>
            </select></label
          ><label class="check-label"
            ><input type="checkbox" v-model="form.active" /> Active
            account</label
          >
          <p>Seluruh sesi user ini akan dicabut.</p></template
        ><template v-if="modal === 'asset'"
          ><div class="form-grid">
            <label
              >Branch<select
                v-model.number="form.asset_branch_id"
                required
                @change="form.location_id = assetLocations[0]?.id"
              >
                <option v-for="b in branches" :key="b.id" :value="b.id">
                  {{ b.code }} · {{ b.name }}
                </option>
              </select></label
            >
            <label
              >Asset tag<input
                v-model="form.tag"
                required
                maxlength="80"
                placeholder="AST-000001" /></label
            ><label
              >Name<input
                v-model="form.name"
                required
                minlength="2"
                maxlength="200" /></label
            ><label
              >Serial number<input
                v-model="form.serial_number"
                maxlength="200" /></label
            ><label
              >Category<select
                v-model.number="form.category_id"
                required
                @change="
                  form.useful_life_months =
                    categories.find((c) => c.id === form.category_id)
                      ?.useful_life_months || 48
                "
              >
                <option v-for="c in categories" :key="c.id" :value="c.id">
                  {{ c.name }}
                </option>
              </select></label
            ><label
              >Location<select v-model.number="form.location_id" required>
                <option v-for="l in assetLocations" :key="l.id" :value="l.id">
                  {{ l.branch_name ? l.branch_name + " · " : "" }}{{ l.name }}
                </option>
              </select></label
            ><label
              >Purchase date<input
                v-model="form.purchase_date"
                type="date"
                required /></label
            ><label
              >Purchase cost (Rp)<input
                v-model.number="form.purchase_cost"
                type="number"
                min="0"
                max="1000000000000000"
                step="1"
                required /></label
            ><label
              >Salvage value (Rp)<input
                v-model.number="form.salvage_value"
                type="number"
                min="0"
                :max="form.purchase_cost"
                step="1"
                required /></label
            ><label
              >Useful life (months)<input
                v-model.number="form.useful_life_months"
                type="number"
                min="1"
                max="1200"
                required /></label
            ><label
              >Warranty until<input
                v-model="form.warranty_until"
                type="date"
                :min="form.purchase_date"
            /></label></div></template
        ><label v-if="modal === 'assign'"
          >Nama / ID penanggung jawab<input
            v-model="form.custodian"
            required
            minlength="2"
            maxlength="200" /></label
        ><template v-if="modal === 'request'"
          ><p>{{ selected?.tag }} · {{ selected?.name }}</p>
          <label
            >Request type<select v-model="form.kind">
              <option value="transfer">Transfer</option>
              <option value="dispose">Disposal</option>
            </select></label
          ><label v-if="form.kind === 'transfer'"
            >Target location<select
              v-model.number="form.target_location_id"
              required
            >
              <option
                v-for="l in locations.filter(
                  (l) => l.id !== selected?.location_id,
                )"
                :key="l.id"
                :value="l.id"
              >
                {{ l.branch_name ? l.branch_name + " · " : "" }}{{ l.name }}
              </option>
            </select></label
          ><label
            >Reason<textarea
              v-model="form.reason"
              minlength="3"
              maxlength="1000"
              required
            ></textarea>
          </label>
          <p class="hint">
            Perubahan berlaku setelah approval oleh admin/manager yang berbeda.
          </p></template
        ><template v-if="modal === 'maintenance'"
          ><p>{{ selected?.tag }} · {{ selected?.name }}</p>
          <label
            >Job title<input
              v-model="form.title"
              required
              minlength="3"
              maxlength="200" /></label
          ><label
            >Due date<input
              v-model="form.due_date"
              type="date"
              required /></label></template
        ><template v-if="['location', 'category', 'user'].includes(modal)"
          ><label
            >Name<input
              v-model="form.name"
              required
              minlength="2"
              maxlength="100" /></label
          ><label v-if="modal === 'category'"
            >Useful life (months)<input
              v-model.number="form.useful_life_months"
              type="number"
              min="1"
              max="1200"
              required /></label></template
        ><template v-if="modal === 'user'"
          ><label
            >Email<input
              v-model="form.email"
              type="email"
              required
              maxlength="254" /></label
          ><label
            >Initial password<input
              v-model="form.password"
              type="password"
              minlength="12"
              maxlength="72"
              autocomplete="new-password"
              required /></label
          ><label
            >Role<select v-model="form.role">
              <option
                v-for="role in ['admin', 'manager', 'operator', 'auditor']"
                :key="role"
              >
                {{ role }}
              </option>
            </select></label
          ></template
        ><label v-if="modal === 'decision'"
          >Decision note<textarea
            v-model="form.note"
            :required="!form.approve"
            :minlength="form.approve ? 0 : 3"
            maxlength="1000"
          ></textarea></label
        ><template v-if="modal === 'complete'"
          ><label
            >Actual cost (Rp)<input
              v-model.number="form.cost"
              type="number"
              min="0"
              step="1"
              max="1000000000000000"
              required /></label
          ><label
            >Notes<textarea
              v-model="form.notes"
              maxlength="2000"
            ></textarea></label
        ></template>
        <p v-if="['return', 'start', 'cancel'].includes(modal)">
          Konfirmasi aksi ini. Perubahan status dan audit akan disimpan.
        </p>
        <div v-if="error" class="alert" role="alert">{{ error }}</div>
        <div class="modal-footer">
          <button
            type="button"
            class="secondary"
            @click="modal = ''"
            :disabled="saving"
          >
            Batal</button
          ><button
            v-if="modal !== 'stocktakeItems'"
            class="primary"
            :disabled="saving"
          >
            {{ saving ? "Menyimpan…" : "Simpan & konfirmasi" }}
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
