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
  Clock3,
  Moon,
  Sun,
  Landmark,
  Languages,
  Archive,
  ArchiveRestore,
  Pencil,
} from "lucide-vue-next";
import FinanceWorkspace from "./FinanceWorkspace.vue";
import {
  api,
  ApiError,
  money,
  date,
  timestamp,
  type User,
  type Asset,
  type LoginOrganization,
  type RoleOption,
} from "./api";
import { locale, theme, t, toggleTheme } from "./preferences";
const me = ref<User | null>(null),
  initializing = ref(true),
  loading = ref(false),
  saving = ref(false),
  error = ref(""),
  toast = ref(""),
  page = ref("dashboard"),
  modal = ref(""),
  selected = ref<Asset | null>(null);
const login = reactive({ employee_id: "", password: "", org_code: "" }),
  loginOrganizations = ref<LoginOrganization[]>([]),
  roleOptions = ref<RoleOption[]>([]),
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
  showArchived = ref(false),
  activityEvent = ref(""),
  historyRecords = ref<any[]>([]);
const assetLocations = computed(() =>
  locations.value.filter((l) => l.branch_id === form.value.asset_branch_id),
);
const form = ref<Record<string, any>>({});
function can(capability: string) {
  return !!me.value?.capabilities?.includes(capability);
}
const roleNames: Record<string, string> = {
  admin: "role.admin",
  branch_admin: "role.branch_admin",
  manager: "role.manager",
  operator: "role.operator",
  staff: "role.staff",
  employee: "role.employee",
  finance: "role.finance",
  it_support: "role.it_support",
  it_developer: "role.it_developer",
  auditor: "role.auditor",
};
function roleLabel(role: string) {
  return roleNames[role] ? t(roleNames[role]) : role;
}
function copy(id: string, en: string) {
  return locale.value === "id" ? id : en;
}
const canWrite = computed(() => can("assets.write"));
const nav = computed(() =>
  [
    { key: "dashboard", label: t("nav.dashboard"), icon: LayoutDashboard, cap: "dashboard.read" },
    { key: "assets", label: t("nav.assets"), icon: Boxes, cap: "assets.read" },
    { key: "requests", label: t("nav.requests"), icon: ArrowLeftRight, cap: "requests.read" },
    { key: "maintenance", label: t("nav.maintenance"), icon: Wrench, cap: "maintenance.read" },
    { key: "stocktakes", label: t("nav.stocktakes"), icon: ClipboardCheck, cap: "stocktakes.read" },
    { key: "branches", label: t("nav.branches"), icon: Building2, cap: "branches.read" },
    { key: "locations", label: t("nav.locations"), icon: MapPin, cap: "locations.read" },
    { key: "categories", label: t("nav.categories"), icon: Layers, cap: "categories.read" },
    { key: "audit", label: t("nav.audit"), icon: ShieldCheck, cap: "audit.read" },
    { key: "activity", label: t("nav.activity"), icon: History, cap: "activity.read" },
    { key: "users", label: t("nav.users"), icon: Users, cap: "users.read" },
    { key: "finance", label: t("nav.finance"), icon: Landmark, cap: can("finance.read") ? "finance.read" : "reports.read", show: can("finance.read") || can("reports.read") },
  ].filter((item: any) => item.show !== false && can(item.cap)),
);
watch(
  () => form.value.role,
  (role) => {
    if (role === "admin") {
      form.value.all_branches = true;
      form.value.branch_ids = [];
    }
    if (role === "branch_admin") {
      form.value.all_branches = false;
      form.value.branch_ids = (form.value.branch_ids || []).slice(0, 1);
    }
  },
);
function setBranchScope(id: number, checked: boolean) {
  const selected = form.value.branch_ids || [];
  if (form.value.role === "branch_admin") {
    form.value.branch_ids = checked ? [id] : [];
  } else if (checked) {
    form.value.branch_ids = [...new Set([...selected, id])];
  } else {
    form.value.branch_ids = selected.filter((value: number) => value !== id);
  }
}
const title = computed(
  () => nav.value.find((n) => n.key === page.value)?.label || t("page.dashboard"),
);
const modalTitle = computed(
  () =>
    (
      ({
        branch: copy("Tambah cabang", "Add branch"),
        policy: copy("Kebijakan kategori", "Category policy"),
        asset: t("asset.register"),
        editAsset: t("asset.edit"),
        assign: copy("Tetapkan penanggung jawab", "Assign custodian"),
        request: copy("Permintaan transfer / pelepasan", "Transfer / disposal request"),
        maintenance: copy("Jadwalkan pemeliharaan", "Schedule maintenance"),
        location: copy("Tambah lokasi", "Add location"),
        category: copy("Tambah kategori", "Add category"),
        user: copy("Tambah pengguna", "Add user"),
        decision: form.value.approve ? copy("Setujui permintaan", "Approve request") : copy("Tolak permintaan", "Reject request"),
        complete: copy("Selesaikan pemeliharaan", "Complete maintenance"),
        return: copy("Kembalikan aset", "Return asset"),
        start: copy("Mulai pemeliharaan", "Start maintenance"),
        cancel: copy("Batalkan pemeliharaan", "Cancel maintenance"),
        stocktake: copy("Buka stock opname", "Open stocktake"),
        observe: copy("Catat tag aset", "Observe asset tag"),
        closeStocktake: copy("Tutup stock opname", "Close stocktake"),
        stocktakeItems: copy("Snapshot stock opname", "Stocktake snapshot"),
        password: copy("Ubah password", "Change password"),
        access: copy("Ubah akses pengguna", "Edit user access"),
        history: t("asset.history"),
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
    if (view === "finance") {
      return;
    } else if (view === "dashboard") {
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
            archived: String(showArchived.value),
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
          activityEvent.value +
          ((["users", "branches", "locations", "categories"].includes(view) && showArchived.value)
            ? "&archived=true"
            : ""),
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
    api("/locations?branch_id=" + branch.value),
    api("/categories?branch_id=" + branch.value),
    api("/branches"),
  ]);
}
async function loadAssignableRoles() {
  if (!can("users.manage")) {
    roleOptions.value = [];
    return;
  }
  roleOptions.value = await api<RoleOption[]>("/user-roles");
}
async function signIn() {
  saving.value = true;
  error.value = "";
  try {
    const signedIn = await api<User>("/login", {
      employee_id: login.employee_id,
      password: login.password,
      org_code: login.org_code,
    });
    me.value = signedIn;
    branch.value = signedIn.active_branch_id;
    login.password = "";
    showArchived.value = false;
    await loadAssignableRoles();
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
    showArchived.value = false;
    roleOptions.value = [];
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
  showArchived.value = false;
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
async function changeBranch() {
  current.value = 1;
  await masters();
  await load();
}
function open(kind: string, a: Asset | null = null) {
  error.value = "";
  selected.value = a;
  modal.value = kind;
  form.value = {};
  const today = new Date().toLocaleDateString("en-CA");
  if (kind === "asset" || kind === "editAsset")
    form.value = {
      tag: "",
      name: "",
      serial_number: "",
      asset_branch_id: a?.branch_id || branch.value || branches.value[0]?.id,
      category_id: a?.category_id || categories.value[0]?.id,
      location_id: a?.location_id || locations.value.find(
        (l) => l.branch_id === (branch.value || branches.value[0]?.id),
      )?.id,
      purchase_date: today,
      purchase_cost: 0,
      salvage_value: 0,
      useful_life_months: 48,
      depreciation_method: "straight_line",
      depreciation_start_date: today,
      supplier_name: "",
      acquisition_reference: "",
      warranty_until: "",
      ...(a ? {
        tag: a.tag,
        name: a.name,
        serial_number: a.serial_number,
        purchase_date: a.purchase_date,
        depreciation_method: a.depreciation_method,
        depreciation_start_date: a.depreciation_start_date,
        supplier_name: a.supplier_name,
        acquisition_reference: a.acquisition_reference,
        purchase_cost: a.purchase_cost,
        salvage_value: a.salvage_value,
        useful_life_months: a.useful_life_months,
        warranty_until: a.warranty_until || "",
        version: a.version,
      } : {}),
    };
  if (kind === "request")
    form.value = {
      asset_id: a?.id,
      version: a?.version,
      kind: "transfer",
      disposal_proceeds: 0,
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
      branch_id: branch.value,
    };
  if (kind === "stocktake")
    form.value = { title: "", location_id: locations.value[0]?.id };
  if (kind === "user")
    form.value = {
      name: "",
      email: "",
      password: "",
      role: roleOptions.value.find((role) => role.id === "operator")?.id || roleOptions.value[0]?.id || "operator",
      all_branches: false,
      branch_ids: branch.value ? [branch.value] : [],
    };
  if (kind === "user" || kind === "access") {
    void loadAssignableRoles().catch((e) => (error.value = (e as Error).message));
  }
}
async function viewAssetHistory(a: Asset) {
  selected.value = a;
  historyRecords.value = [];
  modal.value = "history";
  error.value = "";
  try {
    historyRecords.value = await api(`/assets/${a.id}/history?branch_id=${branch.value}&size=100`);
  } catch (e) {
    error.value = (e as Error).message;
  }
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
          depreciation_method: body.depreciation_method || "",
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
      case "editAsset":
        endpoint = modal.value === "editAsset" ? `/assets/${selected.value!.id}` : "/assets";
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
        else body.disposal_proceeds = 0;
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
    const createdUser = modal.value === "user";
    const result = await api<{ employee_id?: string }>(endpoint, body);
    if (modal.value === "password") {
      me.value = null;
      modal.value = "";
      notify(copy("Password diperbarui. Silakan masuk kembali.", "Password updated. Please sign in again."));
      return;
    }
    modal.value = "";
    notify(
      createdUser && result?.employee_id
        ? copy(`Pengguna dibuat. ID karyawan: ${result.employee_id}`, `User created. Employee ID: ${result.employee_id}`)
        : copy("Perubahan berhasil disimpan", "Changes saved successfully"),
    );
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
    notify(copy("Ekspor CSV berhasil dan tercatat di audit", "CSV export completed and audited"));
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
async function archiveAsset(a: Asset) {
  if (!window.confirm(copy(`Arsipkan aset ${a.tag}? Data tetap tersimpan dan bisa dipulihkan.`, `Archive asset ${a.tag}? Its data will be retained and can be restored.`))) return;
  try {
    await api(`/assets/${a.id}`, undefined, "DELETE");
    notify(copy("Aset diarsipkan", "Asset archived"));
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function restoreAsset(a: Asset) {
  try {
    await api(`/assets/${a.id}/restore`, {});
    notify(copy("Aset dipulihkan", "Asset restored"));
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function archiveUser(r: any) {
  if (!window.confirm(copy(`Arsipkan akun ${r.name}? Sesi aktifnya akan dicabut.`, `Archive ${r.name}'s account? Their active sessions will be revoked.`))) return;
  try {
    await api(`/users/${r.id}`, undefined, "DELETE");
    notify(copy("Akun diarsipkan", "Account archived"));
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function restoreUser(r: any) {
  try {
    await api(`/users/${r.id}/restore`, {});
    notify(copy("Akun dipulihkan", "Account restored"));
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function archiveBranch(r: any) {
  if (!window.confirm(copy(`Arsipkan cabang ${r.code}? Cabang pusat tidak bisa diarsipkan.`, `Archive branch ${r.code}? Head office cannot be archived.`))) return;
  try {
    await api(`/branches/${r.id}`, undefined, "DELETE");
    notify(copy("Cabang diarsipkan", "Branch archived"));
    await masters();
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function restoreBranch(r: any) {
  try {
    await api(`/branches/${r.id}/restore`, {});
    notify(copy("Cabang dipulihkan", "Branch restored"));
    await masters();
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function archiveMaster(kind: "locations" | "categories", r: any) {
  const label = kind === "locations" ? copy("lokasi", "location") : copy("kategori", "category");
  if (!window.confirm(copy(`Arsipkan ${label} ${r.name}? Data tetap tersimpan.`, `Archive ${label} ${r.name}? Its data will be retained.`))) return;
  try {
    await api(`/${kind}/${r.id}`, undefined, "DELETE");
    notify(copy(`${label} diarsipkan`, `${label[0].toUpperCase()}${label.slice(1)} archived`));
    await masters();
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function restoreMaster(kind: "locations" | "categories", r: any) {
  try {
    await api(`/${kind}/${r.id}/restore`, {});
    notify(copy("Data dipulihkan", "Record restored"));
    await masters();
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
onMounted(async () => {
  window.addEventListener("session-expired", expired);
  try {
    const options = await api<{ organizations: LoginOrganization[] }>("/login/options");
    loginOrganizations.value = options.organizations;
  } catch (e) {
    error.value = (e as Error).message;
    initializing.value = false;
    return;
  }
  try {
    const sessionUser = await api<User>("/me");
    me.value = sessionUser;
    branch.value = sessionUser.active_branch_id;
    await masters();
    await loadAssignableRoles();
    await load();
  } catch (e) {
    me.value = null;
    error.value = e instanceof ApiError && e.status === 401 ? "" : (e as Error).message;
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
  <div v-if="initializing" class="boot">{{ copy("Memuat AssetFlow…", "Loading AssetFlow…") }}</div>
  <div v-else-if="!me" class="login-wrap">
    <div class="appearance-tools login-preferences">
      <label :aria-label="t('common.language')"><Languages :size="16" /><select v-model="locale"><option value="id">Bahasa Indonesia</option><option value="en">English</option></select></label>
      <button class="icon" @click="toggleTheme" :aria-label="t('common.theme')"><Sun v-if="theme === 'dark'" :size="17" /><Moon v-else :size="17" /></button>
    </div>
    <section class="login-story">
      <div class="brand">
        <PackageCheck :size="30" /> AssetFlow<span class="edition"
          >{{ copy("RUANG KERJA", "WORKSPACE") }}</span
        >
      </div>
      <div>
        <span class="eyebrow">{{ copy("KENDALIKAN SETIAP ASET", "CONTROL EVERY ASSET") }}</span>
        <h1>{{ copy("Aset Anda.", "Your assets.") }}<br />{{ copy("Tanggung jawab Anda.", "Your accountability.") }}</h1>
        <p>
          {{ copy("Satu tempat untuk inventaris, penanggung jawab, pemeliharaan, dan seluruh riwayat aset organisasi.", "One place for inventory, custodians, maintenance, and the complete history of organizational assets.") }}
        </p>
        <div class="story-line">
          {{ copy("01 / Visibilitas &nbsp; 02 / Tata kelola &nbsp; 03 / Siklus hidup", "01 / Visibility &nbsp; 02 / Governance &nbsp; 03 / Lifecycle") }}
        </div>
      </div>
      <small>{{ copy("Manajemen aset · Go + Vue", "Asset management · Go + Vue") }}</small>
    </section>
    <section class="login-form">
      <div class="login-box">
        <span class="eyebrow">{{ t("login.welcome") }}</span>
        <h2>{{ t("login.title") }}</h2>
        <p>{{ t("login.description") }}</p>
        <form @submit.prevent="signIn">
          <label>{{ copy("Kode organisasi", "Organization code") }}<input
            v-model.trim="login.org_code"
            type="text"
            autocomplete="organization"
            list="organization-codes"
            :placeholder="copy('Contoh: ORG-000001', 'Example: ORG-000001')"
            required
          /></label>
          <datalist id="organization-codes">
            <option
              v-for="organization in loginOrganizations"
              :key="organization.id"
              :value="organization.code"
              :label="organization.name"
            />
          </datalist>
          <label>{{ t("login.employee") }}<input
            v-model.trim="login.employee_id"
            type="text"
            autocomplete="username"
            :placeholder="copy('Contoh: EMP-1', 'Example: EMP-1')"
            required
          /></label>
          <label>{{ t("login.password") }}<input
            v-model="login.password"
            type="password"
            autocomplete="current-password"
            required
            maxlength="72"
          /></label>
          <div v-if="error" class="alert" role="alert">{{ error }}</div>
          <button class="primary" :disabled="saving">
            {{ saving ? t("login.processing") : t("login.submit") }}
            <ArrowUpRight :size="17" />
          </button>
        </form>
      </div>
    </section>
  </div>
  <div v-else class="workspace" :inert="!!modal">
    <aside>
      <div class="brand"><PackageCheck :size="27" /> AssetFlow</div>
      <div class="org">
        <span class="org-icon">AF</span>
        <div>
          <b>{{ me.organization_name }}</b
          ><small>{{ me.all_branches ? copy("Pusat · semua cabang", "Head office · all branches") : copy("Ruang kerja cabang", "Branch workspace") }}</small>
        </div>
      </div>
      <span class="nav-label">{{ copy("RUANG KERJA", "WORKSPACE") }}</span>
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
        <span class="live-dot"></span> {{ copy("Workspace terhubung", "Connected workspace") }}<small
          >{{ copy("Tata kelola di setiap perubahan.", "Governance built into every move.") }}</small
        >
      </div>
    </aside>
    <div class="main">
      <header>
        <div class="breadcrumb">{{ copy("Ruang kerja", "Workspace") }} <span>/</span> {{ title }}</div>
        <div class="user">
          <div class="appearance-tools">
            <label :aria-label="t('common.language')"><Languages :size="16" /><select v-model="locale"><option value="id">ID</option><option value="en">EN</option></select></label>
            <button class="icon" @click="toggleTheme" :aria-label="t('common.theme')"><Sun v-if="theme === 'dark'" :size="17" /><Moon v-else :size="17" /></button>
          </div>
          <span class="avatar">{{ me.name.slice(0, 1) }}</span>
          <div>
            <b>{{ me.name }}</b
            ><small>{{ roleLabel(me.role) }}</small>
          </div>
          <button
            class="icon"
            @click="open('password')"
                :aria-label="locale === 'id' ? 'Ubah password' : 'Change password'"
          >
            <KeyRound :size="18" /></button
          ><button class="icon" @click="logout" :aria-label="locale === 'id' ? 'Keluar' : 'Logout'">
            <LogOut :size="18" />
          </button>
        </div>
      </header>
      <main>
        <div class="page-heading">
          <div>
              <span class="eyebrow">{{ locale === "id" ? "OPERASIONAL ASET" : "ASSET OPERATIONS" }}</span>
            <h1>{{ title }}</h1>
            <p>
              {{
                page === "dashboard"
                  ? t("dashboard.description")
                  : locale === "id" ? "Kelola data dan proses aset dengan riwayat yang dapat ditelusuri." : "Manage asset records and workflows with a traceable history."
              }}
            </p>
          </div>
          <div class="buttons">
            <select
              v-if="me.all_branches || me.branch_ids.length > 1"
              class="branch-filter"
              v-model.number="branch"
              @change="changeBranch"
              :aria-label="copy('Filter cabang', 'Branch filter')"
            >
              <option v-if="me.all_branches" :value="0">{{ t("common.allBranches") }}</option>
              <option v-for="b in branches" :key="b.id" :value="b.id">
                {{ b.code }} · {{ b.name }}
              </option>
            </select>
            <button class="secondary" @click="load" :disabled="loading">
              <RefreshCw :size="16" /> {{ t("common.refresh") }}</button
            ><button
              v-if="page === 'branches' && can('branches.manage')"
              class="primary"
              @click="open('branch')"
            >
              <Plus :size="17" /> {{ t("common.branch") }}</button
            ><button
              v-if="page === 'assets' && canWrite"
              class="primary"
              @click="open('asset')"
            >
              <Plus :size="17" /> {{ t("asset.register") }}</button
            ><button
              v-if="page === 'locations' && can('locations.manage')"
              class="primary"
              @click="open('location')"
            >
              <Plus :size="17" /> {{ copy("Lokasi", "Location") }}</button
            ><button
              v-if="page === 'categories' && can('categories.manage')"
              class="primary"
              @click="open('category')"
            >
              <Plus :size="17" /> {{ copy("Kategori", "Category") }}</button
            ><button
              v-if="page === 'stocktakes' && can('stocktakes.manage')"
              class="primary"
              @click="open('stocktake')"
            >
              <Plus :size="17" /> {{ copy("Stock opname", "Stocktake") }}</button
            ><button
              v-if="page === 'users' && can('users.manage')"
              class="primary"
              @click="open('user')"
            >
              <Plus :size="17" /> {{ copy("Pengguna", "User") }}
            </button>
            <button
              v-if="(page === 'assets' && can('assets.archive')) || (page === 'users' && can('users.archive')) || (page === 'branches' && can('branches.manage')) || (page === 'locations' && can('locations.archive')) || (page === 'categories' && can('categories.archive'))"
              class="secondary"
              @click="showArchived = !showArchived; filter()"
              :aria-pressed="showArchived"
            >
              <ArchiveRestore :size="16" />
              {{ showArchived ? t("common.activeData") : t("common.archive") }}
            </button>
          </div>
        </div>
        <div v-if="error && !modal" class="alert" role="alert">{{ error }}</div>
        <div v-if="loading" class="loading" aria-live="polite">
          {{ t("common.loading") }}
        </div>
        <template v-if="page === 'dashboard'"
          ><div class="hint due-callout">
              {{ stats.maintenance_due_assets || 0 }} {{ locale === "id" ? "aset telah jatuh tempo pemeliharaan berdasarkan kebijakan kategori." : "assets are past their category maintenance interval." }}
          </div>
          <div class="kpis">
            <article class="kpi featured">
              <span>{{ t("dashboard.total") }} <Boxes :size="20" /></span
              ><strong>{{ stats.total || 0 }}</strong
              ><small>{{ locale === "id" ? "Di seluruh lokasi" : "Across all locations" }}</small>
            </article>
            <article class="kpi">
              <span>{{ t("dashboard.available") }} <PackageCheck :size="20" /></span
              ><strong>{{ stats.available || 0 }}</strong
              ><small>{{ locale === "id" ? "Siap ditugaskan" : "Ready to be assigned" }}</small>
            </article>
            <article class="kpi">
              <span>{{ t("dashboard.assigned") }} <Users :size="20" /></span
              ><strong>{{ stats.assigned || 0 }}</strong
              ><small>{{ locale === "id" ? "Memiliki penanggung jawab" : "With a custodian" }}</small>
            </article>
            <article class="kpi">
              <span>{{ t("dashboard.maintenance") }} <Wrench :size="20" /></span
              ><strong>{{ stats.maintenance || 0 }}</strong
              ><small>{{ t("status.in_progress") }}</small>
            </article>
          </div>
          <div class="overview-grid">
            <section v-if="can('assets.finance')" class="panel value-panel">
              <span class="eyebrow">{{ t("dashboard.portfolio") }}</span>
              <h2>{{ money(stats.purchase_value) }}</h2>
              <p>{{ locale === "id" ? "Total nilai perolehan aset yang belum dilepas." : "Total acquisition cost of non-disposed assets." }}</p>
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
                <span><i></i> {{ copy("Digunakan", "Assigned") }} {{ stats.assigned || 0 }}</span
                ><span>{{ copy("Dilepas", "Disposed") }} {{ stats.disposed || 0 }}</span>
              </div>
            </section>
            <section class="panel attention">
              <span class="eyebrow">{{ t("dashboard.attention") }}</span
              ><button @click="navigate('requests')">
                <div>
                  <b>{{ t("dashboard.pending") }}</b
                  ><small>{{ locale === "id" ? "Permintaan transfer dan pelepasan" : "Transfer and disposal requests" }}</small>
                </div>
                <strong>{{ stats.pending_requests || 0 }}</strong
                ><ArrowUpRight :size="18" /></button
              ><button @click="navigate('maintenance')">
                <div>
                  <b>{{ t("dashboard.overdue") }}</b
                  ><small>{{ locale === "id" ? "Pekerjaan melewati tanggal jatuh tempo" : "Open jobs past their due date" }}</small>
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
              <h3>{{ t("dashboard.recent") }}</h3>
              <button class="text-btn" @click="navigate('assets')">
                {{ t("dashboard.viewRegister") }} <ArrowUpRight :size="16" />
              </button>
            </div>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>{{ copy("Aset", "Asset") }}</th>
                    <th>{{ t("asset.category") }}</th>
                    <th>{{ t("common.location") }}</th>
                    <th>{{ t("common.status") }}</th>
                    <th v-if="can('assets.finance')">{{ copy("Harga perolehan", "Acquisition cost") }}</th>
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
                        t("status." + a.status)
                      }}</span>
                    </td>
                    <td v-if="can('assets.finance')">{{ money(a.purchase_cost) }}</td>
                  </tr>
                  <tr v-if="!assets.length">
                    <td colspan="5" class="empty">
                      {{ t("dashboard.empty") }}
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
                  :placeholder="copy('Cari nama, tag, atau nomor seri…', 'Search name, tag, or serial…')"
                  maxlength="200"
                />
              </div>
              <select v-model="status" @change="filter">
                  <option value="">{{ copy("Semua status", "All statuses") }}</option>
                <option
                  v-for="s in [
                    'available',
                    'assigned',
                    'maintenance',
                    'disposed',
                  ]"
                  :key="s"
                >
                  {{ t("status." + s) }}
                </option></select
              ><button class="secondary">{{ t("common.search") }}</button
              ><button
                type="button"
                class="secondary"
                @click="exportCSV"
                :disabled="!assets.length"
              >
                {{ copy("Ekspor halaman CSV", "Export page CSV") }}
              </button>
            </form>
            <div class="table-scroll">
              <table>
                <thead>
                  <tr>
                    <th>{{ copy("Aset / Tag", "Asset / Tag") }}</th>
                    <th>{{ copy("Lokasi & penanggung jawab", "Location & custodian") }}</th>
                    <th>{{ t("common.status") }}</th>
                    <th>{{ copy("Harga / nilai buku", "Cost / book value") }}</th>
                    <th>{{ t("common.actions") }}</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="a in assets" :key="a.id">
                    <td>
                      <b>{{ a.name }}</b
                      ><small>{{ a.tag }} · {{ a.category_name }}</small
                      ><small>{{ copy("No. seri", "Serial") }}: {{ a.serial_number || "—" }}</small>
                    </td>
                    <td>
                      {{ a.location_name }}<small>{{ a.branch_name }}</small
                      ><small>{{ a.custodian || (locale === "id" ? "Belum ditugaskan" : "Unassigned") }}</small>
                    </td>
                    <td>
                      <span class="badge" :class="a.status">{{
                        t("status." + a.status)
                      }}</span>
                    </td>
                    <td>
                      <template v-if="can('assets.finance')">{{ money(a.purchase_cost)
                      }}<small>{{ money(a.book_value) }} {{ t("asset.bookValue") }}</small
                      ></template><small v-if="a.next_maintenance_date"
                        >{{ copy("Pemeliharaan", "Maintenance") }}: {{ date(a.next_maintenance_date) }}</small
                      >
                    </td>
                    <td>
                      <div class="row-actions">
                        <template v-if="!a.deleted_at">
                          <button v-if="can('assets.history')" class="icon" @click="viewAssetHistory(a)" :aria-label="t('asset.history')" :title="t('asset.history')"><Clock3 :size="15" /></button>
                          <button
                            v-if="can('assets.write')"
                            @click="open('editAsset', a)"
                          ><Pencil :size="14" /> {{ copy("Ubah", "Edit") }}</button>
                          <button
                            v-if="a.status === 'available' && can('assets.operate')"
                            @click="open('assign', a)"
                          >{{ copy("Tetapkan", "Assign") }}</button>
                          <button
                            v-if="a.status === 'available' && can('requests.create')"
                            @click="open('request', a)"
                          >{{ copy("Transfer / pelepasan", "Transfer / disposal") }}</button>
                          <button
                            v-if="a.status === 'available' && can('maintenance.manage')"
                            @click="open('maintenance', a)"
                          >{{ t("nav.maintenance") }}</button>
                          <button
                            v-if="a.status === 'assigned' && can('assets.operate')"
                            @click="open('return', a)"
                          >{{ copy("Kembalikan", "Return") }}</button>
                          <button
                            v-if="can('assets.archive')"
                            @click="archiveAsset(a)"
                          ><Archive :size="14" /> {{ copy("Arsipkan", "Archive") }}</button>
                        </template>
                        <button
                          v-else-if="can('assets.archive')"
                          @click="restoreAsset(a)"
                        ><ArchiveRestore :size="14" /> {{ copy("Pulihkan", "Restore") }}</button>
                      </div>
                    </td>
                  </tr>
                  <tr v-if="!assets.length">
                    <td colspan="5" class="empty">
                      {{ copy("Tidak ada aset yang sesuai dengan filter.", "No assets match these filters.") }}
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="pagination">
              <span>{{ total }} {{ copy("aset · Halaman", "assets · Page") }} {{ current }}</span>
              <div>
                <button
                  class="icon"
                  @click="turn(-1)"
                  :disabled="current === 1 || loading"
                  :aria-label="copy('Halaman sebelumnya', 'Previous page')"
                >
                  <ChevronLeft :size="18" /></button
                ><button
                  class="icon"
                  @click="turn(1)"
                  :disabled="current * 25 >= total || loading"
                  :aria-label="copy('Halaman berikutnya', 'Next page')"
                >
                  <ChevronRight :size="18" />
                </button>
              </div>
            </div></section
        ></template>
        <FinanceWorkspace v-else-if="page === 'finance'" :branch-id="branch" :user-id="me.id" :finance="can('finance.read')" :can-manage="can('finance.manage')" :can-propose="can('valuation.propose')" :can-decide="can('valuation.decide')" :contracts-read="can('contracts.read')" :contracts-manage="can('contracts.manage')" :locale="locale" />
        <section v-else class="panel">
          <div v-if="page === 'activity'" class="filters">
            <select
              v-model="activityEvent"
              @change="filter"
              aria-label="Activity event"
            >
              <option value="">{{ copy("Semua aktivitas", "All activity") }}</option>
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
                  <th>{{ copy("Aset", "Asset") }}</th>
                  <th>{{ copy("Permintaan", "Request") }}</th>
                  <th>{{ copy("Pemohon", "Requester") }}</th>
                  <th>{{ t("common.status") }}</th>
                  <th>{{ copy("Keputusan", "Decision") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    <b>{{ r.name }}</b
                    ><small>{{ r.tag }}</small>
                  </td>
                  <td>
                    {{ r.kind === "transfer" ? copy("Transfer", "Transfer") : copy("Pelepasan", "Disposal") }}
                    <small>{{ r.target_location || copy("Lepas aset", "Retire asset") }}</small
                    ><small>{{ r.reason }}</small>
                  </td>
                  <td>
                    {{ r.requester
                    }}<small>{{ timestamp(r.created_at) }}</small>
                  </td>
                  <td>
                    <span class="badge" :class="r.status">{{ t("status." + r.status) }}</span>
                  </td>
                  <td>
                    <div
                      v-if="
                        r.status === 'pending' &&
                        can('requests.decide') &&
                        r.requested_by !== me.id
                      "
                      class="row-actions"
                    >
                      <button @click="decision(r, true)">{{ copy("Setujui", "Approve") }}</button
                      ><button @click="decision(r, false)">{{ copy("Tolak", "Reject") }}</button>
                    </div>
                    <small v-else>{{
                      r.status === "pending"
                        ? copy("Menunggu pemeriksa lain", "Awaiting another checker")
                        : r.decision_note || "—"
                    }}</small>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'maintenance'">
              <thead>
                <tr>
                  <th>{{ copy("Pekerjaan / Aset", "Job / Asset") }}</th>
                  <th>{{ copy("Jatuh tempo", "Due date") }}</th>
                  <th>{{ t("common.status") }}</th>
                  <th v-if="can('assets.finance')">{{ copy("Biaya", "Cost") }}</th>
                  <th>{{ t("common.actions") }}</th>
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
                    <span class="badge" :class="r.status">{{ t("status." + r.status) }}</span>
                  </td>
                  <td v-if="can('assets.finance')">{{ r.cost == null ? "—" : money(r.cost) }}</td>
                  <td>
                    <div class="row-actions" v-if="can('maintenance.manage')">
                      <template v-if="r.status === 'scheduled'"
                        ><button @click="maintenanceAction(r, 'start')">
                          {{ copy("Mulai", "Start") }}</button
                        ><button @click="maintenanceAction(r, 'cancel')">
                          {{ copy("Batalkan", "Cancel") }}
                        </button></template
                      ><button
                        v-if="r.status === 'in_progress'"
                        @click="maintenanceAction(r, 'complete')"
                      >
                        {{ copy("Selesaikan", "Complete") }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'stocktakes'">
              <thead>
                <tr>
                  <th>{{ copy("Stock opname", "Stocktake") }}</th>
                  <th>{{ t("common.status") }}</th>
                  <th>{{ copy("Teramati / diharapkan", "Observed / expected") }}</th>
                  <th>{{ copy("Belum ditemukan", "Missing") }}</th>
                  <th>{{ t("common.actions") }}</th>
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
                      <button @click="viewStocktake(r)">{{ copy("Snapshot", "Snapshot") }}</button
                      ><template v-if="r.status === 'open'"
                        ><button
                          v-if="can('stocktakes.observe')"
                          @click="
                            open('observe');
                            form = { id: r.id, tag: '', notes: '' };
                          "
                        >
                          {{ copy("Catat tag", "Observe tag") }}</button
                        ><button
                          v-if="can('stocktakes.close')"
                          @click="
                            open('closeStocktake');
                            form = { id: r.id, acknowledge: false };
                          "
                        >
                          {{ copy("Tutup", "Close") }}
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
                  <th>{{ copy("Waktu / pengguna", "Time / user") }}</th>
                  <th>{{ copy("Peristiwa", "Event") }}</th>
                  <th>{{ copy("Permintaan", "Request") }}</th>
                  <th>{{ t("common.status") }}</th>
                  <th>Metadata</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>
                    {{ timestamp(r.created_at)
                    }}<small>{{
                      r.actor_name || r.attempted_employee_id || r.attempted_email || copy("Tidak terautentikasi", "Unauthenticated")
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
                      <summary>{{ copy("Detail", "Details") }}</summary>
                      <small
                        >{{ r.request_id }}<br />Peer IP: {{ r.peer_ip
                        }}<br />{{ r.user_agent }}<br />{{
                          r.duration_ms
                        }}
                        {{ copy("md", "ms") }}</small
                      >
                    </details>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'branches'">
              <thead>
                <tr>
                  <th>{{ copy("Kode", "Code") }}</th>
                  <th>{{ t("common.branch") }}</th>
                  <th>{{ copy("Alamat", "Address") }}</th>
                  <th>{{ t("common.actions") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>{{ r.code }}</td>
                  <td>{{ r.name }}</td>
                  <td>{{ r.address || "—" }}</td>
                  <td>
                    <button
                      v-if="!showArchived && can('branches.manage') && r.code !== 'HQ'"
                      class="text-btn"
                      @click="archiveBranch(r)"
                    ><Archive :size="14" /> Arsipkan</button>
                    <button
                      v-if="showArchived && can('branches.manage')"
                      class="text-btn"
                      @click="restoreBranch(r)"
                    ><ArchiveRestore :size="14" /> Pulihkan</button>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else-if="page === 'audit'">
              <thead>
                <tr>
                  <th>{{ copy("Waktu / pelaku", "Time / Actor") }}</th>
                  <th>{{ copy("Tindakan", "Action") }}</th>
                  <th>{{ copy("Objek", "Entity") }}</th>
                  <th>{{ copy("Perubahan", "Changes") }}</th>
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
                    }}<small>{{ copy("Cabang", "Branch") }} {{ r.branch_id || copy("Pusat", "Company") }}</small>
                  </td>
                  <td>
                    <details>
                      <summary>{{ copy("Lihat data", "View payload") }}</summary>
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
                  <th>{{ copy("Nama", "Name") }}</th>
                  <th>{{ t("login.employee") }}</th>
                  <th>Email</th>
                  <th>{{ copy("Peran", "Role") }}</th>
                  <th>{{ t("common.status") }}</th>
                  <th>{{ copy("Cakupan cabang", "Branch scope") }}</th>
                  <th>{{ copy("Akses", "Access") }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="r in records" :key="r.id">
                  <td>{{ r.name }}</td>
                  <td>{{ r.employee_id }}</td>
                  <td>{{ r.email }}</td>
                  <td>
                    <span class="badge">{{ roleLabel(r.role) }}</span>
                  </td>
                  <td>{{ r.deleted_at ? copy("Diarsipkan", "Archived") : (r.active ? copy("Aktif", "Active") : copy("Nonaktif", "Inactive")) }}</td>
                  <td>
                    {{
                      r.all_branches
                        ? copy("Seluruh organisasi", "Company-wide")
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
                      v-if="!showArchived && r.id !== me.id && can('users.manage')"
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
                      {{ copy("Ubah akses", "Edit access") }}
                    </button>
                    <button
                      v-if="!showArchived && r.id !== me.id && can('users.archive')"
                      class="text-btn"
                      @click="archiveUser(r)"
                    ><Archive :size="14" /> Arsipkan</button>
                    <button
                      v-if="showArchived && can('users.archive')"
                      class="text-btn"
                      @click="restoreUser(r)"
                    ><ArchiveRestore :size="14" /> Pulihkan</button>
                  </td>
                </tr>
              </tbody>
            </table>
            <table v-else>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>{{ copy("Nama", "Name") }}</th>
                  <th v-if="page === 'locations'">{{ t("common.branch") }}</th>
                  <th v-if="page === 'categories'">{{ copy("Cakupan", "Scope") }}</th>
                  <th v-if="page === 'categories'">{{ t("asset.life") }}</th>
                  <th v-if="page === 'categories'">{{ copy("Kebijakan pemeliharaan", "Maintenance policy") }}</th>
                  <th>{{ t("common.actions") }}</th>
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
                    {{ r.branch_name || t("common.allBranches") }}
                  </td>
                  <td v-if="page === 'categories'">
                    {{ r.useful_life_months }} {{ copy("bulan", "months") }}
                  </td>
                  <td v-if="page === 'categories'">
                    {{
                      r.maintenance_interval_days
                        ? copy(`Setiap ${r.maintenance_interval_days} hari`, `Every ${r.maintenance_interval_days} days`)
                        : copy("Manual saja", "Manual only")
                    }}<small>{{ r.maintenance_instructions }}</small
                    ><button
                      v-if="can('categories.manage') && (me?.role === 'admin' || r.branch_id === branch)"
                      class="text-btn"
                      @click="
                        open('policy');
                        form = {
                          id: r.id,
                          version: r.version,
                          maintenance_interval_days:
                            r.maintenance_interval_days,
                          maintenance_instructions: r.maintenance_instructions,
                          depreciation_method: r.depreciation_method,
                          apply_to_existing: false,
                        };
                      "
                    >
                      {{ copy("Ubah kebijakan", "Edit policy") }}
                    </button>
                  </td>
                  <td>
                    <button
                      v-if="!showArchived && page === 'locations' && can('locations.archive')"
                      class="text-btn"
                      @click="archiveMaster('locations', r)"
                    ><Archive :size="14" /> {{ copy("Arsipkan", "Archive") }}</button>
                    <button
                      v-if="!showArchived && page === 'categories' && can('categories.archive') && (me?.role === 'admin' || r.branch_id === branch)"
                      class="text-btn"
                      @click="archiveMaster('categories', r)"
                    ><Archive :size="14" /> {{ copy("Arsipkan", "Archive") }}</button>
                    <button
                      v-if="showArchived && page === 'locations' && can('locations.archive')"
                      class="text-btn"
                      @click="restoreMaster('locations', r)"
                    ><ArchiveRestore :size="14" /> {{ copy("Pulihkan", "Restore") }}</button>
                    <button
                      v-if="showArchived && page === 'categories' && can('categories.archive') && (me?.role === 'admin' || r.branch_id === branch)"
                      class="text-btn"
                      @click="restoreMaster('categories', r)"
                    ><ArchiveRestore :size="14" /> {{ copy("Pulihkan", "Restore") }}</button>
                  </td>
                </tr>
              </tbody>
            </table>
            <div v-if="!records.length" class="empty">{{ copy("Belum ada data.", "No records yet.") }}</div>
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
            <span>{{ copy("Halaman", "Page") }} {{ current }} · {{ copy("Maksimal 25 baris", "Up to 25 rows") }}</span>
            <div>
              <button
                class="icon"
                @click="turn(-1)"
                :disabled="current === 1 || loading"
                :aria-label="copy('Halaman sebelumnya', 'Previous page')"
              >
                <ChevronLeft :size="18" /></button
              ><button
                class="icon"
                @click="turn(1)"
                :disabled="records.length < 25 || loading"
                :aria-label="copy('Halaman berikutnya', 'Next page')"
              >
                <ChevronRight :size="18" />
              </button>
            </div>
          </div>
        </section>
        <footer>
          AssetFlow <span>{{ copy("Operasional yang jelas. Kepemilikan yang akuntabel.", "Operational clarity. Accountable ownership.") }}</span>
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
          :aria-label="copy('Tutup', 'Close')"
        >
          <X :size="20" />
        </button>
      </div>
      <form @submit.prevent="submit">
        <template v-if="modal === 'branch'"
          ><label
            >{{ copy("Kode", "Code") }}<input
              v-model="form.code"
              required
              maxlength="30"
              placeholder="JKT-01" /></label
          ><label
            >{{ copy("Nama", "Name") }}<input
              v-model="form.name"
              required
              minlength="2"
              maxlength="150" /></label
          ><label
            >{{ copy("Alamat", "Address") }}<textarea
              v-model="form.address"
              maxlength="1000"
            ></textarea></label></template
        ><template v-if="modal === 'location'"
          ><label
            >{{ t("common.branch") }}<select v-model.number="form.branch_id" required>
              <option v-for="b in branches" :key="b.id" :value="b.id">
                {{ b.code }} · {{ b.name }}
              </option>
            </select></label
          ></template
        ><template v-if="['category', 'policy'].includes(modal)"
          ><label v-if="modal === 'category' && me?.role === 'admin'"
            >{{ copy("Cakupan kategori", "Category scope") }}<select v-model.number="form.branch_id">
              <option :value="0">{{ copy("Semua cabang", "All branches") }}</option>
              <option v-for="b in branches" :key="b.id" :value="b.id">
                {{ b.code }} · {{ b.name }}
              </option>
            </select></label
          ><label
            >{{ copy("Interval pemeliharaan (hari)", "Maintenance interval (days)") }}<input
              v-model.number="form.maintenance_interval_days"
              type="number"
              min="0"
              max="3650"
              required /></label
          ><label
            >{{ copy("Instruksi pemeliharaan", "Maintenance instructions") }}<textarea
              v-model="form.maintenance_instructions"
              maxlength="2000"
            ></textarea>
          </label>
          <label v-if="modal === 'category' && can('assets.finance')"
            >{{ t("asset.method") }}<select v-model="form.depreciation_method"><option value="straight_line">{{ t("asset.straight") }}</option><option value="declining_balance">{{ t("asset.declining") }}</option><option value="non_depreciable">{{ t("asset.nonDepreciable") }}</option></select></label
          ><label v-if="modal === 'policy' && can('assets.finance')"
            >{{ t("asset.method") }}<select v-model="form.depreciation_method"><option value="straight_line">{{ t("asset.straight") }}</option><option value="declining_balance">{{ t("asset.declining") }}</option><option value="non_depreciable">{{ t("asset.nonDepreciable") }}</option></select></label>
          <p class="hint">
            {{ copy("0 = manual saja. Interval diterapkan pada aset baru dan setelah pemeliharaan selesai.", "0 = manual only. The interval applies to new assets and after maintenance is completed.") }}
          </p>
          <label v-if="modal === 'policy'" class="check-label"
            ><input type="checkbox" v-model="form.apply_to_existing" /> {{ copy("Terapkan juga ke aset yang ada tanpa permintaan atau pekerjaan aktif.", "Also apply to existing assets without an active request or work order.") }}</label
          ></template
        ><template v-if="['user', 'access'].includes(modal)">
          <template v-if="me?.role === 'admin'">
            <label class="check-label"
              ><input
                type="checkbox"
                v-model="form.all_branches"
                :disabled="form.role === 'admin' || form.role === 'branch_admin'"
              />
              {{ copy("Akses seluruh cabang", "Access all branches") }}</label
            >
            <fieldset v-if="!form.all_branches && form.role !== 'admin'">
              <legend>{{ form.role === 'branch_admin' ? copy("Cabang administrator", "Administrator branch") : copy("Cakupan cabang", "Branch scope") }}</legend>
              <label v-for="b in branches" :key="b.id" class="check-label">
                <input
                  :type="form.role === 'branch_admin' ? 'radio' : 'checkbox'"
                  name="user-branch-scope"
                  :checked="form.branch_ids?.includes(b.id)"
                  @change="setBranchScope(b.id, ($event.target as HTMLInputElement).checked)"
                />{{ b.code }} · {{ b.name }}
              </label>
            </fieldset>
          </template>
          <p v-else class="hint">{{ copy("Akun baru dan perubahan akses hanya berlaku di cabang Anda.", "New accounts and access changes are limited to your branch.") }}</p>
        </template>
        <template v-if="modal === 'stocktake'"
          ><label
            >{{ copy("Judul", "Title") }}<input
              v-model="form.title"
              required
              minlength="3"
              maxlength="200" /></label
          ><label
            >{{ t("common.location") }}<select v-model.number="form.location_id" required>
              <option v-for="l in locations" :key="l.id" :value="l.id">
                {{ l.branch_name ? l.branch_name + " · " : "" }}{{ l.name }}
              </option>
            </select></label
          >
          <p class="hint">
            {{ copy("Snapshot aset yang belum dilepas di lokasi ini. Perubahan setelah snapshot akan ditandai sebagai selisih.", "Snapshot of non-disposed assets at this location. Changes after the snapshot are flagged as discrepancies.") }}
          </p></template
        ><template v-if="modal === 'observe'"
          ><label
            >{{ t("asset.tag") }}<input
              v-model="form.tag"
              required
              maxlength="80"
              :placeholder="copy('Pindai atau ketik tag', 'Scan or enter a tag')" /></label
          ><label
            >{{ copy("Catatan", "Notes") }}<textarea
              v-model="form.notes"
              maxlength="1000"
            ></textarea></label></template
        ><template v-if="modal === 'closeStocktake'"
          ><p>
            {{ copy("Snapshot akan dikunci. Aset yang belum ditemukan harus diselidiki; tindakan ini tidak mengubah status aset.", "The snapshot will be locked. Missing assets must be investigated; this action does not change asset status.") }}
          </p>
          <label class="check-label"
            ><input type="checkbox" v-model="form.acknowledge" /> {{ copy("Saya memahami selisih aset yang belum ditemukan atau berubah.", "I acknowledge the missing or changed asset discrepancies.") }}</label
          ></template
        ><template v-if="modal === 'stocktakeItems'"
          ><p>{{ form.title }}</p>
          <div class="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>{{ t("asset.tag") }}</th>
                  <th>{{ copy("Teramati", "Observed") }}</th>
                  <th>Version</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="i in form.items" :key="i.asset_id">
                  <td>
                    {{ i.expected_tag }}<small>{{ i.name }}</small>
                  </td>
                  <td>
                    {{ i.observed ? copy("Ya", "Yes") : copy("Belum ditemukan", "Missing")
                    }}<small>{{ i.notes }}</small>
                  </td>
                  <td>
                    {{ i.expected_version }} → {{ i.current_version
                    }}<small v-if="i.expected_version !== i.current_version"
                      >{{ copy("Berubah setelah snapshot", "Changed after snapshot") }}</small
                    >
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="pagination">
            <span>{{ copy("Halaman", "Page") }} {{ form.page }}</span>
            <div>
              <button
                type="button"
                @click="stocktakePage(-1)"
                :disabled="form.page === 1"
                class="secondary"
              >
                {{ copy("Sebelumnya", "Prev") }}</button
              ><button
                type="button"
                @click="stocktakePage(1)"
                :disabled="form.items.length < 25"
                class="secondary"
              >
                {{ copy("Berikutnya", "Next") }}
              </button>
            </div>
          </div></template
        ><template v-if="modal === 'history'">
          <p><b>{{ selected?.name }}</b> · {{ selected?.tag }}</p>
          <div class="table-scroll"><table><thead><tr><th>{{ locale === "id" ? "Waktu" : "When" }}</th><th>{{ locale === "id" ? "Perubahan" : "Movement" }}</th><th>{{ locale === "id" ? "Dari → ke" : "From → to" }}</th><th>{{ locale === "id" ? "Pengguna" : "Actor" }}</th></tr></thead><tbody>
            <tr v-for="entry in historyRecords" :key="entry.id"><td>{{ timestamp(entry.occurred_at) }}</td><td>{{ entry.event }}<small>{{ entry.note }}</small></td><td>{{ entry.from_location || "—" }} → {{ entry.to_location || "—" }}<small>{{ entry.from_custodian || "—" }} → {{ entry.to_custodian || "—" }}</small></td><td>{{ entry.actor }}</td></tr>
            <tr v-if="!historyRecords.length"><td colspan="4" class="empty">{{ locale === "id" ? "Belum ada riwayat." : "No history yet." }}</td></tr>
          </tbody></table></div>
        </template><template v-if="modal === 'password'"
          ><label
            >{{ copy("Password saat ini", "Current password") }}<input
              type="password"
              v-model="form.current_password"
              required
              maxlength="72"
              autocomplete="current-password" /></label
          ><label
            >{{ copy("Password baru", "New password") }}<input
              type="password"
              v-model="form.new_password"
              required
              minlength="12"
              maxlength="72"
              autocomplete="new-password"
          /></label>
          <p>{{ copy("Semua sesi akan dicabut setelah password diubah.", "All sessions will be revoked after the password changes.") }}</p></template
        ><template v-if="modal === 'access'"
          ><label
            >{{ copy("Peran", "Role") }}<select v-model="form.role">
              <option
                v-for="role in roleOptions"
                :key="role.id"
                :value="role.id"
              >
                {{ role.label }}
              </option>
            </select></label
          ><label class="check-label"
            ><input type="checkbox" v-model="form.active" /> {{ copy("Akun aktif", "Active account") }}</label
          >
          <p>{{ copy("Semua sesi pengguna ini akan dicabut.", "All sessions for this user will be revoked.") }}</p></template
        ><template v-if="['asset', 'editAsset'].includes(modal)"
          ><div class="form-grid">
            <label v-if="modal === 'asset'"
              >{{ t("common.branch") }}<select
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
              >{{ t("asset.tag") }}<input
                v-model="form.tag"
                required
                maxlength="80"
                placeholder="AST-000001" /></label
            ><label
              >{{ copy("Nama", "Name") }}<input
                v-model="form.name"
                required
                minlength="2"
                maxlength="200" /></label
            ><label
              >{{ t("asset.serial") }}<input
                v-model="form.serial_number"
                maxlength="200" /></label
            ><label
              >{{ t("asset.category") }}<select
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
              >{{ t("common.location") }}<select v-model.number="form.location_id" required>
                <option v-for="l in assetLocations" :key="l.id" :value="l.id">
                  {{ l.branch_name ? l.branch_name + " · " : "" }}{{ l.name }}
                </option>
              </select></label
            ><label
              >{{ t("asset.purchaseDate") }}<input
                v-model="form.purchase_date"
                type="date"
                required
                @change="form.depreciation_start_date = form.depreciation_start_date < form.purchase_date ? form.purchase_date : form.depreciation_start_date" /></label
            ><label v-if="can('assets.finance')"
              >{{ t("asset.method") }}<select v-model="form.depreciation_method" required>
                <option value="straight_line">{{ t("asset.straight") }}</option>
                <option value="declining_balance">{{ t("asset.declining") }}</option>
                <option value="non_depreciable">{{ t("asset.nonDepreciable") }}</option>
              </select></label
            ><label v-if="can('assets.finance')"
              >{{ t("asset.start") }}<input v-model="form.depreciation_start_date" type="date" :min="form.purchase_date" required /></label
            ><label
              >{{ t("asset.supplier") }}<input v-model="form.supplier_name" maxlength="200" /></label
            ><label
              >{{ t("asset.reference") }}<input v-model="form.acquisition_reference" maxlength="100" /></label
            ><label
              v-if="can('assets.finance')"
              >{{ t("asset.cost") }}<input
                v-model.number="form.purchase_cost"
                type="number"
                min="0"
                max="1000000000000000"
                step="1"
                required /></label
            ><label
              v-if="can('assets.finance')"
              >{{ t("asset.salvage") }}<input
                v-model.number="form.salvage_value"
                type="number"
                min="0"
                :max="form.purchase_cost"
                step="1"
                required /></label
            ><label
              >{{ t("asset.life") }}<input
                v-model.number="form.useful_life_months"
                type="number"
                min="1"
                max="1200"
                required /></label
            ><label
              >{{ t("asset.warranty") }}<input
                v-model="form.warranty_until"
                type="date"
                :min="form.purchase_date"
            /></label></div></template
        ><label v-if="modal === 'assign'"
          >{{ copy("Nama / ID penanggung jawab", "Custodian name / ID") }}<input
            v-model="form.custodian"
            required
            minlength="2"
            maxlength="200" /></label
        ><template v-if="modal === 'request'"
          ><p>{{ selected?.tag }} · {{ selected?.name }}</p>
          <label
            >{{ copy("Jenis permintaan", "Request type") }}<select v-model="form.kind">
              <option value="transfer">{{ copy("Transfer", "Transfer") }}</option>
              <option value="dispose">{{ copy("Pelepasan", "Disposal") }}</option>
            </select></label
          ><label v-if="form.kind === 'transfer'"
            >{{ copy("Lokasi tujuan", "Target location") }}<select
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
            >{{ t("common.reason") }}<textarea
              v-model="form.reason"
              minlength="3"
              maxlength="1000"
              required
            ></textarea>
          </label>
          <label v-if="form.kind === 'dispose' && can('assets.finance')">{{ locale === "id" ? "Hasil pelepasan (Rp)" : "Disposal proceeds (IDR)" }}<input v-model.number="form.disposal_proceeds" type="number" min="0" max="1000000000000000" step="1" required /></label>
          <p class="hint">
            {{ copy("Perubahan berlaku setelah disetujui oleh admin atau manajer lain.", "Changes take effect after approval by a different administrator or manager.") }}
          </p></template
        ><template v-if="modal === 'maintenance'"
          ><p>{{ selected?.tag }} · {{ selected?.name }}</p>
          <label
            >{{ copy("Judul pekerjaan", "Job title") }}<input
              v-model="form.title"
              required
              minlength="3"
              maxlength="200" /></label
          ><label
            >{{ copy("Tanggal jatuh tempo", "Due date") }}<input
              v-model="form.due_date"
              type="date"
              required /></label></template
        ><template v-if="['location', 'category', 'user'].includes(modal)"
          ><label
            >{{ copy("Nama", "Name") }}<input
              v-model="form.name"
              required
              minlength="2"
              maxlength="100" /></label
          ><label v-if="modal === 'category'"
            >{{ t("asset.life") }}<input
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
            >{{ copy("Password awal", "Initial password") }}<input
              v-model="form.password"
              type="password"
              minlength="12"
              maxlength="72"
              autocomplete="new-password"
              required /></label
          ><label
            >{{ copy("Peran", "Role") }}<select v-model="form.role">
              <option
                v-for="role in roleOptions"
                :key="role.id"
                :value="role.id"
              >
                {{ role.label }}
              </option>
            </select></label
          ></template
        ><label v-if="modal === 'decision'"
          >{{ copy("Catatan keputusan", "Decision note") }}<textarea
            v-model="form.note"
            :required="!form.approve"
            :minlength="form.approve ? 0 : 3"
            maxlength="1000"
          ></textarea></label
        ><template v-if="modal === 'complete'"
          ><label
            >{{ copy("Biaya aktual (Rp)", "Actual cost (IDR)") }}<input
              v-model.number="form.cost"
              type="number"
              min="0"
              step="1"
              max="1000000000000000"
              required /></label
          ><label
            >{{ copy("Catatan", "Notes") }}<textarea
              v-model="form.notes"
              maxlength="2000"
            ></textarea></label
        ></template>
        <p v-if="['return', 'start', 'cancel'].includes(modal)">
          {{ copy("Konfirmasi tindakan ini. Perubahan status dan audit akan disimpan.", "Confirm this action. The status change and audit record will be saved.") }}
        </p>
        <div v-if="error" class="alert" role="alert">{{ error }}</div>
        <div class="modal-footer">
          <button
            type="button"
            class="secondary"
            @click="modal = ''"
            :disabled="saving"
          >
            {{ t("common.cancel") }}</button
          ><button
            v-if="modal !== 'stocktakeItems' && modal !== 'history'"
            class="primary"
            :disabled="saving"
          >
            {{ saving ? copy("Menyimpan…", "Saving…") : copy("Simpan & konfirmasi", "Save & confirm") }}
          </button>
        </div>
      </form>
    </section>
  </div>
</template>
