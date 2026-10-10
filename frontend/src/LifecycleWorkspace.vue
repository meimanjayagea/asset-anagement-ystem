<script setup lang="ts">
import {
  ref,
  reactive,
  computed,
  watch,
  onMounted,
  onUnmounted,
  nextTick,
} from "vue";
import {
  Search,
  Camera,
  QrCode,
  Plus,
  X,
  Save,
  Download,
  Printer,
  ChevronLeft,
  ChevronRight,
  ImageIcon,
  Wrench,
  ArrowUpRight,
  ClipboardCheck,
  RefreshCw,
} from "lucide-vue-next";
import { api, money, date, type User, type Asset } from "./api";
import PhotoUploader from "./PhotoUploader.vue";
const props = defineProps<{ branchId: number; user: User }>();
const tab = ref("catalog"),
  assets = ref<Asset[]>([]),
  loans = ref<any[]>([]),
  planned = ref<any[]>([]),
  technicians = ref<any[]>([]),
  jobs = ref<any[]>([]),
  audits = ref<any[]>([]),
  locations = ref<any[]>([]),
  items = ref<any[]>([]),
  photos = ref<any[]>([]);
const uploading = ref(false),
  busy = ref(false),
  saving = ref(false),
  error = ref(""),
  search = ref(""),
  current = ref(1),
  total = ref(0),
  detail = ref<Asset | null>(null),
  spec = ref<any>({}),
  preview = ref(0),
  mode = ref(""),
  photoIDs = ref<number[]>([]),
  activeAudit = ref<any>(null),
  selectedJob = ref<any>(null),
  scanOpen = ref(false),
  video = ref<HTMLVideoElement | null>(null);
const modalTitle = computed(
  () =>
    (
      ({
        spec: "Spesifikasi aset",
        loan: "Pengajuan pinjam",
        work: "Work order",
        checkout: "Kondisi serah terima",
        return: "Kondisi pengembalian",
        complete: "Hasil servis",
        disposal: "Pengajuan disposal",
        audit: "Opname lapangan",
        observation: "Temuan fisik",
      }) as Record<string, string>
    )[mode.value],
);
const form = reactive<any>({}),
  month = ref(new Date().toISOString().slice(0, 7));
let generation = 0;
let scannerControls: { stop: () => void } | undefined;
function can(cap: string) {
  return props.user.capabilities.includes(cap);
}
const tabs = computed(() =>
  [
    { key: "catalog", label: "Katalog visual" },
    { key: "loans", label: "Peminjaman" },
    { key: "work", label: "Servis & perbaikan" },
    { key: "audit", label: "Opname lapangan", show: can("stocktakes.read") },
    { key: "reports", label: "Laporan", show: can("reports.read") },
  ].filter((item) => item.show !== false),
);
const available = computed(() =>
  assets.value.filter((asset) => asset.status === "available"),
);
const selectedAsset = computed(
  () =>
    assets.value.find((asset) => asset.id === form.asset_id) ||
    (detail.value?.id === form.asset_id ? detail.value : null),
);
const calendar = computed(() => {
  const [year, m] = month.value.split("-").map(Number);
  const offset = (new Date(year, m - 1, 1).getDay() + 6) % 7;
  return [
    ...Array(offset).fill(0),
    ...Array.from({ length: new Date(year, m, 0).getDate() }, (_, i) => i + 1),
  ];
});
function dayJobs(day: number) {
  return [...jobs.value, ...planned.value].filter(
    (job) => job.due_date === `${month.value}-${String(day).padStart(2, "0")}`,
  );
}
function changeMonth(delta: number) {
  const [y, m] = month.value.split("-").map(Number);
  const d = new Date(y, m - 1 + delta, 1);
  month.value = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}`;
}
async function allRows(path: string, query = "") {
  const out: any[] = [];
  for (let page = 1; page <= 50; page++) {
    const rows = await api(`${path}?${query}&size=100&page=${page}`);
    out.push(...rows);
    if (rows.length < 100) return out;
  }
  throw new Error("Data melebihi 5000 baris. Persempit cabang atau lokasi.");
}
async function load() {
  const stamp = ++generation;
  busy.value = true;
  error.value = "";
  try {
    const query = `branch_id=${props.branchId}&size=100`;
    const [a, l] = await Promise.all([
      api(
        `/assets?${query}&search=${encodeURIComponent(search.value)}&page=${current.value}`,
      ),
      api(`/locations?${query}`),
    ]);
    if (stamp !== generation) return;
    assets.value = a.items;
    total.value = a.total;
    locations.value = l;
    if (tab.value === "loans") loans.value = await allRows("/loans", query);
    if (tab.value === "work" && can("maintenance.read")) {
      [jobs.value, planned.value] = await Promise.all([
        allRows("/maintenance", query),
        allRows("/maintenance/planned", query),
      ]);
      if (can("maintenance.manage"))
        technicians.value = await api(`/maintenance/assignees?${query}`);
    }
    if (tab.value === "audit")
      audits.value = await allRows("/stocktakes", query);
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    if (stamp === generation) busy.value = false;
  }
}
async function view(asset: Asset) {
  detail.value = asset;
  error.value = "";
  try {
    const [d, p] = await Promise.all([
      api(`/assets/${asset.id}/catalog`),
      api(`/assets/${asset.id}/photos`),
    ]);
    if (detail.value?.id !== asset.id) return;
    if (!d.length) throw new Error("Aset tidak ditemukan");
    spec.value = d[0];
    photos.value = p;
    preview.value = p[0]?.id || 0;
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function refreshDetail() {
  await load();
  if (detail.value) {
    const asset = assets.value.find((item) => item.id === detail.value?.id);
    if (asset) await view(asset);
  }
}
function open(kind: string, record?: any) {
  if (kind === "work" && can("maintenance.manage"))
    void api(`/maintenance/assignees?branch_id=${props.branchId}`)
      .then((rows) => (technicians.value = rows))
      .catch((e) => (error.value = e.message));
  mode.value = kind;
  photoIDs.value = [];
  error.value = "";
  Object.keys(form).forEach((key) => delete form[key]);
  const asset = detail.value || available.value[0];
  Object.assign(form, {
    asset_id: asset?.id || 0,
    version: asset?.version || 1,
    reason: "",
    notes: "",
    due_at: "",
    due_date: new Date().toISOString().slice(0, 10),
    kind: can("maintenance.manage") ? "preventive" : "repair",
    title: "",
    checklist_text: "",
    cost: 0,
    parts: [],
    condition: "good",
    disposal_method: "write_off",
    disposal_proceeds: 0,
  });
  if (kind === "spec")
    Object.assign(form, spec.value, { version: detail.value?.version });
  if (record) {
    Object.assign(form, record, {
      version: record.asset_version || record.version,
      title: record.title || "Servis berkala",
    });
    selectedJob.value = record;
  }
  if (kind === "complete")
    form.checklist = (record?.checklist || []).map((item: any) => ({
      ...item,
      done: false,
    }));
  if (kind === "observation")
    Object.assign(form, {
      asset_id: record.asset_id,
      tag: record.expected_tag,
      finding: "present",
      found_location_id: null,
    });
}
async function submit() {
  saving.value = true;
  error.value = "";
  try {
    const asset = selectedAsset.value;
    const version = form.version || asset?.version;
    switch (mode.value) {
      case "spec":
        await api(`/assets/${detail.value!.id}/catalog`, {
          brand: form.brand || "",
          model: form.model || "",
          specifications: form.specifications || "",
          rfid_tag: form.rfid_tag || "",
          version,
        });
        break;
      case "loan":
        await api("/loans", {
          asset_id: form.asset_id,
          version: asset?.version,
          due_at: new Date(form.due_at).toISOString(),
          reason: form.reason,
        });
        break;
      case "work":
        await api(form.kind === "repair" ? "/repairs" : "/maintenance", {
          asset_id: form.asset_id,
          version: asset?.version || form.version,
          title: form.title,
          due_date: form.due_date,
          kind: form.kind,
          assigned_to: form.assigned_to || null,
          photo_ids: photoIDs.value,
          checklist: form.checklist_text
            .split("\n")
            .filter((value: string) => value.trim())
            .map((label: string) => ({ label: label.trim(), done: false })),
        });
        break;
      case "checkout":
      case "return":
        await api(`/loans/${form.id}/action`, {
          action: mode.value,
          notes: form.notes,
          condition: form.condition,
          version,
          photo_ids: photoIDs.value,
        });
        break;
      case "complete":
        await api(`/maintenance/${form.id}/action`, {
          action: "complete",
          cost: Number(form.cost),
          notes: form.notes,
          version,
          photo_ids: photoIDs.value,
          checklist: form.checklist,
          parts: form.parts.map((part: any) => ({
            name: part.name,
            quantity: Number(part.quantity),
            cost: Number(part.cost),
          })),
        });
        break;
      case "disposal":
        await api("/requests", {
          asset_id: form.asset_id,
          kind: "dispose",
          version: asset?.version,
          reason: form.reason,
          disposal_method: form.disposal_method,
          disposal_proceeds: form.disposal_method==='sale'?Number(form.disposal_proceeds):0,
          photo_ids: photoIDs.value,
        });
        break;
      case "audit":
        await api("/stocktakes", {
          title: form.title,
          location_id: form.location_id,
        });
        break;
      case "observation":
        await api(`/stocktakes/${activeAudit.value.id}/observe`, {
          tag: form.tag,
          notes: form.notes,
          finding: form.finding,
          found_location_id:
            form.finding === "relocated" ? form.found_location_id : null,
          photo_ids: photoIDs.value,
        });
        await auditItems(activeAudit.value);
        break;
    }
    mode.value = "";
    await refreshDetail();
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
async function jobAction(job: any, action: string) {
  error.value = "";
  try {
    await api(`/maintenance/${job.id}/action`, {
      action,
      version: job.asset_version,
      notes: "",
      cost: 0,
    });
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function cancelLoan(loan: any) {
  const notes = window.prompt("Alasan pembatalan");
  if (!notes) return;
  try {
    await api(`/loans/${loan.id}/action`, { action: "cancel", notes });
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function auditItems(audit: any) {
  activeAudit.value = audit;
  try {
    items.value = await allRows(`/stocktakes/${audit.id}/items`);
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function closeAudit() {
  if (!window.confirm("Tutup opname dan akui selisih yang tercatat?")) return;
  try {
    await api(`/stocktakes/${activeAudit.value.id}/close`, {
      acknowledge_discrepancies: true,
    });
    activeAudit.value = null;
    await load();
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function scan(code: string) {
  stopScanner();
  let value = code;
  try {
    const url = new URL(code);
    value = url.searchParams.get("asset_tag") || "";
  } catch {
    value = code.replace(/^ASSET:/, "");
  }
  try {
    if (tab.value === "audit" && activeAudit.value) {
      const item = items.value.find(
        (item) => item.expected_tag.toUpperCase() === value.toUpperCase(),
      );
      if (!item) throw new Error("Tag di luar snapshot opname");
      if (item.finding !== "unverified")
        throw new Error("Aset sudah diverifikasi");
      open("observation", item);
      return;
    }
    const result = await api(
      `/assets/lookup?code=${encodeURIComponent(value)}`,
    );
    search.value = result.tag;
    current.value = 1;
    await load();
    const asset = assets.value.find((item) => item.id === result.id);
    if (asset) await view(asset);
    else throw new Error("Aset tidak ditemukan dalam cabang aktif");
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function startScanner() {
  error.value = "";
  scanOpen.value = true;
  await nextTick();
  try {
    const { BrowserMultiFormatReader } = await import("@zxing/browser");
    const reader = new BrowserMultiFormatReader();
    const controls = await reader.decodeFromConstraints(
      { video: { facingMode: "environment" } },
      video.value!,
      (result) => {
        if (result) scan(result.getText());
      },
    );
    if (!scanOpen.value) controls.stop();
    else scannerControls = controls;
  } catch (e) {
    stopScanner();
    error.value = `Kamera tidak tersedia: ${(e as Error).message}`;
  }
}
function stopScanner() {
  scannerControls?.stop();
  scannerControls = undefined;
  scanOpen.value = false;
}
async function scanImage(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  const url = URL.createObjectURL(file);
  try {
    const { BrowserMultiFormatReader } = await import("@zxing/browser");
    const result = await new BrowserMultiFormatReader().decodeFromImageUrl(url);
    await scan(result.getText());
  } catch {
    error.value = "QR/barcode tidak ditemukan pada gambar";
  } finally {
    URL.revokeObjectURL(url);
    input.value = "";
  }
}
async function labelPDF() {
  if (!detail.value) return;
  try {
    const asset = detail.value;
    const [{ default: QRCode }, { default: JsBarcode }, { jsPDF }] =
      await Promise.all([
        import("qrcode"),
        import("jsbarcode"),
        import("jspdf"),
      ]);
    const qr = await QRCode.toDataURL(
      `${location.origin}/?asset_tag=${encodeURIComponent(asset.tag)}`,
      { width: 220, margin: 2 },
    );
    const canvas = document.createElement("canvas");
    const printable = /^[\x20-\x7e]+$/.test(asset.tag);
    if (printable)
      JsBarcode(canvas, asset.tag, {
        format: "CODE128",
        displayValue: true,
        width: 2,
        height: 45,
      });
    const width = printable
      ? Math.max(100, Math.ceil((asset.tag.length * 11 + 35) * 0.25) + 12)
      : 100;
    const pdf = new jsPDF({
      unit: "mm",
      format: [width, 80],
      orientation: "landscape",
    });
    pdf.setFontSize(11);
    pdf.text(pdf.splitTextToSize(asset.name, width - 12).slice(0, 2), 6, 8);
    pdf.addImage(qr, "PNG", 6, 16, 34, 34);
    if (printable)
      pdf.addImage(canvas.toDataURL(), "PNG", 6, 56, width - 12, 18);
    pdf.setFontSize(8);
    pdf.text(pdf.splitTextToSize(asset.tag, width - 49).slice(0, 3), 43, 30);
    pdf.text(`Org ${props.user.organization_name}`.slice(0, 60), 6, 53);
    pdf.save(`label-${asset.tag.replace(/[^a-z0-9_-]/gi, "_")}.pdf`);
  } catch (e) {
    error.value = (e as Error).message;
  }
}
async function exportReport(format: "pdf" | "xlsx") {
  saving.value = true;
  error.value = "";
  try {
    const rows = await api(`/reports/lifecycle?branch_id=${props.branchId}`);
    const columns = [
      "tag",
      "name",
      "brand",
      "model",
      "status",
      "location",
      "custodian",
      "purchase_cost",
      "maintenance_cost",
      "services",
    ];
    if (can("finance.read")) {
      const report = await api(
        `/finance/depreciation?branch_id=${props.branchId}&period=${month.value}`,
      );
      const values = new Map(
        report.items.map((row: any) => [row.asset_id, row]),
      );
      for (const row of rows) {
        const finance = values.get(row.id) as any;
        Object.assign(row, {
          opening_book_value: finance?.opening_book_value ?? 0,
          depreciation_charge: finance?.depreciation_charge ?? 0,
          closing_book_value: finance?.closing_book_value ?? 0,
        });
      }
      columns.push(
        "opening_book_value",
        "depreciation_charge",
        "closing_book_value",
      );
    }
    if (format === "xlsx") {
      const { default: ExcelJS } = await import("exceljs");
      const workbook = new ExcelJS.Workbook();
      const sheet = workbook.addWorksheet("Lifecycle");
      sheet.columns = columns.map((key) => ({ header: key, key, width: 22 }));
      sheet.addRows(
        rows.map((row: any) =>
          Object.fromEntries(columns.map((key) => [key, row[key] ?? ""])),
        ),
      );
      sheet.views = [{ state: "frozen", ySplit: 1 }];
      sheet.autoFilter = {
        from: "A1",
        to: `${String.fromCharCode(64 + columns.length)}1`,
      };
      const data = await workbook.xlsx.writeBuffer();
      download(
        new Blob([new Uint8Array(data)], {
          type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
        }),
        "asset-lifecycle.xlsx",
      );
    } else {
      const { jsPDF } = await import("jspdf");
      const pdf = new jsPDF({ orientation: "landscape" });
      pdf.setFontSize(14);
      pdf.text(`Asset Lifecycle - ${month.value}`, 10, 12);
      pdf.setFontSize(8);
      let y = 24;
      pdf.text(
        "Tag / Nama / Status / Lokasi / Biaya servis / Depresiasi / Nilai buku",
        10,
        y,
      );
      y += 8;
      for (const row of rows) {
        const lines = pdf.splitTextToSize(
          `${row.tag} | ${row.name} | ${row.status} | ${row.location} | ${row.maintenance_cost ?? "-"} | ${row.depreciation_charge ?? "-"} | ${row.closing_book_value ?? "-"}`,
          275,
        );
        if (y + lines.length * 4 > 195) {
          pdf.addPage();
          y = 12;
        }
        pdf.text(lines, 10, y);
        y += lines.length * 4 + 4;
      }
      const net = rows.reduce(
        (sum: number, row: any) => sum + Number(row.closing_book_value || 0),
        0,
      );
      if (can("finance.read")) {
        if (y > 185) {
          pdf.addPage();
          y = 12;
        }
        pdf.text(`Total nilai buku: ${net}`, 10, y + 5);
      }
      pdf.save("asset-lifecycle.pdf");
    }
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    saving.value = false;
  }
}
function download(blob: Blob, name: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = url;
  anchor.download = name;
  anchor.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
watch(tab, () => {
  current.value = 1;
  detail.value = null;
  activeAudit.value = null;
  load();
});
watch(
  () => props.branchId,
  () => {
    detail.value = null;
    mode.value = "";
    current.value = 1;
    load();
  },
);
onMounted(async () => {
  await load();
  const code = new URLSearchParams(location.search).get("asset_tag");
  if (code) await scan(code);
});
function keyboard(event: KeyboardEvent) {
  if (event.key === "Escape" && !saving.value && !uploading.value) {
    if (mode.value) mode.value = "";
    else if (scanOpen.value) stopScanner();
    else detail.value = null;
  }
  if (event.key === "Tab") {
    const dialog =
      document.querySelector(".form-overlay [role=dialog]") ||
      document.querySelector(".life-overlay [role=dialog]");
    if (!dialog) return;
    const controls = Array.from(
      dialog.querySelectorAll<HTMLElement>(
        "button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),a[href]",
      ),
    ).filter((element) => element.offsetParent !== null);
    const first = controls[0],
      last = controls.at(-1);
    if (!first || !last) return;
    if (
      event.shiftKey &&
      (document.activeElement === first ||
        !dialog.contains(document.activeElement))
    ) {
      event.preventDefault();
      last.focus();
    } else if (
      !event.shiftKey &&
      (document.activeElement === last ||
        !dialog.contains(document.activeElement))
    ) {
      event.preventDefault();
      first.focus();
    }
  }
}
onMounted(() => document.addEventListener("keydown", keyboard));
onUnmounted(() => {
  stopScanner();
  document.removeEventListener("keydown", keyboard);
});
</script>

<template>
  <div class="lifecycle-workspace">
    <nav class="life-tabs" aria-label="Proses aset">
      <button
        v-for="item in tabs"
        :key="item.key"
        :class="{ active: tab === item.key }"
        @click="
          tab = item.key;
          selectedJob = null;
        "
      >
        {{ item.label }}
      </button>
    </nav>
    <p v-if="error && !mode" class="alert" role="alert">{{ error }}</p>
    <div class="life-toolbar">
      <form
        @submit.prevent="
          current = 1;
          load();
        "
      >
        <input
          v-model="search"
          aria-label="Cari aset"
          placeholder="Nama, nomor seri, atau tag"
        /><button class="icon" title="Cari aset" aria-label="Cari aset">
          <Search :size="18" />
        </button>
      </form>
      <button
        class="icon"
        title="Segarkan"
        aria-label="Segarkan proses"
        @click="load"
      >
        <RefreshCw :size="18" /></button
      ><template
        v-if="
          tab === 'catalog' ||
          (tab === 'audit' && activeAudit && can('stocktakes.observe'))
        "
        ><button class="secondary" @click="startScanner">
          <Camera :size="17" />Scan</button
        ><label class="secondary upload-code" title="Scan gambar QR/barcode"
          ><QrCode :size="17" /><input
            type="file"
            accept="image/*"
            aria-label="Scan gambar QR/barcode"
            @change="scanImage" /></label></template
      ><button
        v-if="tab === 'loans' && can('loans.request')"
        class="primary"
        @click="open('loan')"
      >
        <Plus :size="17" />Pinjam aset</button
      ><button
        v-if="tab === 'work' && can('maintenance.request')"
        class="primary"
        @click="open('work')"
      >
        <Plus :size="17" />Work order</button
      ><button
        v-if="tab === 'audit' && can('stocktakes.manage')"
        class="primary"
        @click="open('audit')"
      >
        <Plus :size="17" />Opname
      </button>
    </div>
    <p v-if="busy" role="status">Memuat...</p>
    <template v-if="tab === 'catalog'">
      <div class="visual-catalog">
        <article v-for="asset in assets" :key="asset.id" class="visual-asset">
          <button
            class="asset-image"
            :aria-label="`Detail ${asset.tag}`"
            @click="view(asset)"
          >
            <img
              v-if="asset.cover_photo_id"
              :src="`/api/photos/${asset.cover_photo_id}`"
              :alt="asset.name"
              loading="lazy"
            /><ImageIcon v-else :size="42" />
          </button>
          <div class="visual-info">
            <span class="badge" :class="asset.status">{{ asset.status }}</span>
            <h2>
              <button @click="view(asset)">{{ asset.name }}</button>
            </h2>
            <p>{{ asset.tag }} · {{ asset.category_name }}</p>
            <p>{{ asset.brand }} {{ asset.model }}</p>
            <small>{{ asset.branch_name }} / {{ asset.location_name }}</small
            ><strong v-if="can('assets.finance')">{{
              money(asset.book_value)
            }}</strong>
          </div>
        </article>
      </div>
      <p v-if="!assets.length && !busy">Tidak ada aset.</p>
      <footer class="life-pagination">
        <span>{{ total }} aset</span
        ><button
          class="icon"
          title="Halaman sebelumnya"
          aria-label="Halaman sebelumnya"
          :disabled="current === 1"
          @click="
            current--;
            load();
          "
        >
          <ChevronLeft :size="18" /></button
        ><span>{{ current }}</span
        ><button
          class="icon"
          title="Halaman berikutnya"
          aria-label="Halaman berikutnya"
          :disabled="current * 100 >= total"
          @click="
            current++;
            load();
          "
        >
          <ChevronRight :size="18" />
        </button>
      </footer>
    </template>
    <div v-else-if="tab === 'loans'" class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>Aset</th>
            <th>Peminjam</th>
            <th>Bukti kondisi</th>
            <th>Jatuh tempo</th>
            <th>Status</th>
            <th>Aksi</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="loan in loans" :key="loan.id">
            <td>
              {{ loan.tag }}<small>{{ loan.name }}</small>
            </td>
            <td>{{ loan.borrower }}</td>
            <td>
              <a
                v-for="id in [
                  ...(loan.before_photo_ids || []),
                  ...(loan.after_photo_ids || []),
                ]"
                :key="id"
                :href="`/api/photos/${id}`"
                target="_blank"
                rel="noopener"
                ><img
                  class="audit-proof"
                  :src="`/api/photos/${id}`"
                  alt="Bukti pinjaman"
              /></a>
            </td>
            <td>{{ date(loan.due_at) }}</td>
            <td>{{ loan.status }}</td>
            <td>
              <div class="row-actions">
                <button
                  v-if="loan.status === 'pending' && can('loans.manage')"
                  @click="open('checkout', loan)"
                >
                  Serahkan</button
                ><button
                  v-if="loan.status === 'checked_out' && can('loans.manage')"
                  @click="open('return', loan)"
                >
                  Terima kembali</button
                ><button
                  v-if="
                    loan.status === 'pending' &&
                    (loan.borrower_id === user.id || can('loans.manage'))
                  "
                  @click="cancelLoan(loan)"
                >
                  Batalkan
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="!loans.length">Tidak ada peminjaman.</p>
    </div>
    <template v-else-if="tab === 'work'">
      <header class="calendar-heading">
        <h2>Kalender servis</h2>
        <button
          class="icon"
          title="Bulan sebelumnya"
          aria-label="Bulan sebelumnya"
          @click="changeMonth(-1)"
        >
          <ChevronLeft :size="18" /></button
        ><input
          v-model="month"
          type="month"
          aria-label="Bulan kalender"
        /><button
          class="icon"
          title="Bulan berikutnya"
          aria-label="Bulan berikutnya"
          @click="changeMonth(1)"
        >
          <ChevronRight :size="18" />
        </button>
      </header>
      <div class="service-calendar">
        <strong
          v-for="day in ['Sen', 'Sel', 'Rab', 'Kam', 'Jum', 'Sab', 'Min']"
          :key="day"
          >{{ day }}</strong
        >
        <div
          v-for="(day, index) in calendar"
          :key="index"
          :class="{ blank: !day }"
        >
          <span v-if="day">{{ day }}</span
          ><button
            v-for="job in dayJobs(day)"
            :key="job.id"
            :title="`${job.tag}: ${job.title}`"
            @click="
              job.status === 'planned' && can('maintenance.manage')
                ? open('work', job)
                : (selectedJob = job)
            "
          >
            {{ job.tag }}
          </button>
        </div>
      </div>
      <div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Aset / work order</th>
              <th>Jenis</th>
              <th>Jadwal</th>
              <th>Status</th>
              <th>Biaya</th>
              <th>Aksi</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="job in jobs" :key="job.id">
              <td>
                {{ job.tag }}<small>{{ job.title }}</small>
              </td>
              <td>{{ job.kind }}</td>
              <td>{{ date(job.due_date) }}</td>
              <td>{{ job.status }}</td>
              <td>{{ job.cost == null ? "-" : money(job.cost) }}</td>
              <td>
                <div class="row-actions">
                  <button
                    class="icon"
                    title="Riwayat servis"
                    aria-label="Riwayat servis"
                    @click="selectedJob = job"
                  >
                    <ClipboardCheck :size="17" /></button
                  ><button
                    v-if="
                      job.status === 'scheduled' && can('maintenance.manage')
                    "
                    @click="jobAction(job, 'start')"
                  >
                    Mulai</button
                  ><button
                    v-if="
                      job.status === 'in_progress' && can('maintenance.manage')
                    "
                    @click="open('complete', job)"
                  >
                    Selesaikan</button
                  ><button
                    v-if="
                      job.status === 'scheduled' && can('maintenance.manage')
                    "
                    @click="jobAction(job, 'cancel')"
                  >
                    Batalkan
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
      <section v-if="selectedJob && !mode" class="service-detail">
        <header>
          <h2>{{ selectedJob.title }}</h2>
          <button
            class="icon"
            title="Tutup detail servis"
            aria-label="Tutup detail servis"
            @click="selectedJob = null"
          >
            <X :size="18" />
          </button>
        </header>
        <p>{{ selectedJob.notes }}</p>
        <div class="photo-strip">
          <a
            v-for="id in [
              ...(selectedJob.before_photo_ids || []),
              ...(selectedJob.after_photo_ids || []),
            ]"
            :key="id"
            :href="`/api/photos/${id}`"
            target="_blank"
            rel="noopener"
            ><img
              class="audit-proof"
              :src="`/api/photos/${id}`"
              alt="Bukti servis"
          /></a>
        </div>
        <ul>
          <li v-for="item in selectedJob.checklist || []" :key="item.label">
            {{ item.done ? "Selesai" : "Belum" }} · {{ item.label }}
          </li>
        </ul>
        <table>
          <thead>
            <tr>
              <th>Suku cadang</th>
              <th>Jumlah</th>
              <th>Biaya</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(part, index) in selectedJob.parts || []" :key="index">
              <td>{{ part.name }}</td>
              <td>{{ part.quantity }}</td>
              <td>{{ part.cost == null ? "-" : money(part.cost) }}</td>
            </tr>
          </tbody>
        </table>
      </section>
    </template>
    <template v-else-if="tab === 'audit'"
      ><div class="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Opname / lokasi</th>
              <th>Status</th>
              <th>Snapshot</th>
              <th>Terverifikasi</th>
              <th>Hilang</th>
              <th>Rusak</th>
              <th>Pindah</th>
              <th>Aksi</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="audit in audits" :key="audit.id">
              <td>
                {{ audit.title }}<small>{{ audit.location_name }}</small>
              </td>
              <td>{{ audit.status }}</td>
              <td>{{ audit.expected }}</td>
              <td>{{ audit.observed }}</td>
              <td>{{ audit.missing }}</td>
              <td>{{ audit.damaged }}</td>
              <td>{{ audit.relocated }}</td>
              <td><button @click="auditItems(audit)">Rekonsiliasi</button></td>
            </tr>
          </tbody>
        </table>
      </div>
      <section v-if="activeAudit" class="audit-items">
        <header>
          <h2>{{ activeAudit.title }}</h2>
          <button
            v-if="activeAudit.status === 'open' && can('stocktakes.close')"
            class="secondary"
            @click="closeAudit"
          >
            Tutup opname
          </button>
        </header>
        <div class="table-scroll">
          <table>
            <thead>
              <tr>
                <th>Aset</th>
                <th>Temuan</th>
                <th>Catatan</th>
                <th>Bukti</th>
                <th>Aksi</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in items" :key="item.asset_id">
                <td>
                  {{ item.expected_tag }}<small>{{ item.name }}</small>
                </td>
                <td>{{ item.finding }}</td>
                <td>{{ item.notes }}</td>
                <td>
                  <a
                    v-for="id in item.photo_ids || []"
                    :key="id"
                    :href="`/api/photos/${id}`"
                    target="_blank"
                    rel="noopener"
                    ><img
                      class="audit-proof"
                      :src="`/api/photos/${id}`"
                      :alt="`Bukti ${item.expected_tag}`"
                  /></a>
                </td>
                <td>
                  <button
                    v-if="
                      item.finding === 'unverified' &&
                      activeAudit.status === 'open' &&
                      can('stocktakes.observe')
                    "
                    @click="open('observation', item)"
                  >
                    Verifikasi
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section></template
    >
    <section v-else-if="tab === 'reports'" class="life-reports">
      <h2>Nilai dan biaya aset</h2>
      <label>Periode<input v-model="month" type="month" required /></label>
      <div class="buttons">
        <button
          class="secondary"
          :disabled="saving || uploading"
          @click="exportReport('pdf')"
        >
          <Download :size="17" />PDF</button
        ><button
          class="secondary"
          :disabled="saving || uploading"
          @click="exportReport('xlsx')"
        >
          <Download :size="17" />Excel
        </button>
      </div>
    </section>
    <div v-if="detail" class="life-overlay" @click.self="detail = null">
      <section
        class="asset-detail"
        role="dialog"
        aria-modal="true"
        aria-labelledby="asset-detail-title"
      >
        <header>
          <div>
            <small>{{ detail.tag }}</small>
            <h2 id="asset-detail-title">{{ detail.name }}</h2>
          </div>
          <button
            class="icon"
            title="Tutup detail"
            aria-label="Tutup detail aset"
            @click="detail = null"
          >
            <X :size="20" />
          </button>
        </header>
        <p v-if="error && !mode" class="alert" role="alert">{{ error }}</p>
        <div class="asset-detail-columns">
          <div class="asset-gallery">
            <img
              v-if="preview"
              class="photo-main"
              :src="`/api/photos/${preview}`"
              :alt="
                photos.find((photo) => photo.id === preview)?.caption ||
                detail.name
              "
            />
            <div v-else class="photo-empty"><ImageIcon :size="42" /></div>
            <div class="photo-strip">
              <button
                v-for="photo in photos"
                :key="photo.id"
                :title="`${photo.purpose}: ${photo.caption}`"
                :aria-label="photo.caption || `Foto ${photo.id}`"
                :class="{ selected: preview === photo.id }"
                @click="preview = photo.id"
              >
                <img
                  :src="`/api/photos/${photo.id}`"
                  :alt="photo.caption || photo.purpose"
                />
              </button>
            </div>
            <PhotoUploader
              v-if="can('assets.write')"
              :key="detail.id"
              :asset-id="detail.id"
              purpose="catalog"
              v-model="photoIDs"
              @busy="uploading = $event"
              @uploaded="view(detail!)"
            />
          </div>
          <div class="asset-spec">
            <dl>
              <dt>Merek / model</dt>
              <dd>{{ spec.brand || "-" }} / {{ spec.model || "-" }}</dd>
              <dt>No. seri</dt>
              <dd>{{ detail.serial_number || "-" }}</dd>
              <dt>Kategori</dt>
              <dd>{{ detail.category_name }}</dd>
              <dt>Lokasi</dt>
              <dd>
                {{ detail.branch_name }} / {{ detail.location_name
                }}<small
                  >{{ spec.building }} / {{ spec.floor }} /
                  {{ spec.room }}</small
                >
              </dd>
              <dt>Penanggung jawab</dt>
              <dd>{{ detail.custodian || "-" }}</dd>
              <dt>Vendor</dt>
              <dd>{{ detail.supplier_name || "-" }}</dd>
              <dt>Pembelian / garansi</dt>
              <dd>
                {{ date(detail.purchase_date) }} /
                {{ date(detail.warranty_until || "") }}
              </dd>
              <template v-if="can('assets.finance')"
                ><dt>Perolehan / nilai buku</dt>
                <dd>
                  {{ money(detail.purchase_cost) }} /
                  {{ money(detail.book_value) }}
                </dd>
                <dt>Total servis</dt>
                <dd>{{ money(spec.maintenance_total || 0) }}</dd></template
              >
              <dt>RFID</dt>
              <dd>{{ spec.rfid_tag || "-" }}</dd>
              <dt>Spesifikasi</dt>
              <dd class="spec-text">{{ spec.specifications || "-" }}</dd>
            </dl>
            <div class="detail-actions">
              <button class="secondary" @click="labelPDF">
                <Printer :size="17" />Label QR / barcode</button
              ><button
                v-if="can('assets.write')"
                class="secondary"
                @click="open('spec')"
              >
                <Save :size="17" />Spesifikasi</button
              ><button
                v-if="detail.status === 'available' && can('loans.request')"
                class="secondary"
                @click="open('loan')"
              >
                <ArrowUpRight :size="17" />Pinjam</button
              ><button
                v-if="can('maintenance.request')"
                class="secondary"
                @click="
                  open('work');
                  form.kind = 'repair';
                "
              >
                <Wrench :size="17" />Perbaikan</button
              ><button
                v-if="detail.status === 'available' && can('requests.create')"
                class="secondary"
                @click="open('disposal')"
              >
                Disposal
              </button>
            </div>
          </div>
        </div>
      </section>
    </div>
    <div
      v-if="mode"
      class="life-overlay form-overlay"
      @click.self="!saving && !uploading && (mode = '')"
    >
      <section
        class="life-form"
        role="dialog"
        aria-modal="true"
        aria-labelledby="life-form-title"
      >
        <header>
          <h2 id="life-form-title">{{ modalTitle }}</h2>
          <button
            class="icon"
            :disabled="saving || uploading"
            title="Tutup formulir"
            aria-label="Tutup formulir proses"
            @click="mode = ''"
          >
            <X :size="20" />
          </button>
        </header>
        <form @submit.prevent="submit">
          <p v-if="error" class="alert" role="alert">{{ error }}</p>
          <template v-if="mode === 'spec'"
            ><label>Merek<input v-model="form.brand" maxlength="100" /></label
            ><label>Model<input v-model="form.model" maxlength="100" /></label
            ><label
              >Spesifikasi<textarea
                v-model="form.specifications"
                maxlength="4000"
                rows="4"
              /></label
            ><label
              >Tag RFID<input v-model="form.rfid_tag" maxlength="128" /></label
          ></template>
          <template v-if="['loan', 'work', 'disposal'].includes(mode)"
            ><label
              >Aset<select aria-label="Aset"
                v-model.number="form.asset_id"
                required
                :disabled="!!detail || uploading"
              >
                <option
                  v-for="asset in assets"
                  :key="asset.id"
                  :value="asset.id"
                >
                  {{ asset.tag }} · {{ asset.name }} · {{ asset.status }}
                </option>
              </select></label
            ></template
          >
          <template v-if="mode === 'loan'"
            ><label
              >Jatuh tempo<input
                v-model="form.due_at"
                type="datetime-local"
                required /></label
            ><label
              >Alasan<textarea
                v-model="form.reason"
                minlength="3"
                maxlength="1000"
                required
              /></label
          ></template>
          <template v-if="mode === 'work'"
            ><label
              >Jenis<select v-model="form.kind" aria-label="Jenis">
                <option v-if="can('maintenance.manage')" value="preventive">
                  Preventive
                </option>
                <option value="repair">Perbaikan</option>
              </select></label
            ><label v-if="can('maintenance.manage')"
              >Teknisi<select v-model.number="form.assigned_to" aria-label="Teknisi">
                <option :value="null">Belum ditugaskan</option>
                <option
                  v-for="person in technicians"
                  :key="person.id"
                  :value="person.id"
                >
                  {{ person.name }} · {{ person.employee_id }}
                </option>
              </select></label
            ><label
              >Judul<input
                v-model="form.title"
                minlength="3"
                maxlength="200"
                required /></label
            ><label
              >Jadwal<input
                v-model="form.due_date"
                type="date"
                required /></label
            ><label
              >Checklist servis<textarea
                v-model="form.checklist_text"
                maxlength="10000"
                rows="4"
              /></label
            ><PhotoUploader
              v-if="form.kind === 'repair' && form.asset_id"
              :key="form.asset_id"
              :asset-id="form.asset_id"
              purpose="damage"
              v-model="photoIDs"
              @busy="uploading = $event"
          /></template>
          <template v-if="mode === 'disposal'"
            ><label
              >Metode<select v-model="form.disposal_method" aria-label="Metode">
                <option value="write_off">Pengafkiran</option>
                <option value="sale">Penjualan</option>
                <option value="abandonment">Peninggalan</option>
              </select></label
            ><label
              >Alasan<textarea
                v-model="form.reason"
                minlength="3"
                maxlength="1000"
                required
              /></label
            ><label v-if="can('assets.finance') && form.disposal_method==='sale'"
              >Hasil penjualan<input
                v-model.number="form.disposal_proceeds"
                type="number"
                min="0" /></label
            ><PhotoUploader
              v-if="form.asset_id"
              :key="form.asset_id"
              :asset-id="form.asset_id"
              purpose="disposal"
              v-model="photoIDs"
              @busy="uploading = $event"
          /></template>
          <template v-if="['checkout', 'return', 'complete'].includes(mode)"
            ><label
              >Catatan kondisi<textarea
                v-model="form.notes"
                minlength="3"
                :maxlength="mode === 'complete' ? 2000 : 1000"
                required
              /></label
            ><label v-if="mode === 'return'"
              >Kondisi<select v-model="form.condition" aria-label="Kondisi">
                <option value="good">Baik</option>
                <option value="damaged">Rusak</option>
              </select></label
            ><template v-if="mode === 'complete'"
              ><label
                >Total biaya servis<input
                  v-model.number="form.cost"
                  type="number"
                  min="0"
                  required /></label
              ><label
                v-for="(item, index) in form.checklist"
                :key="index"
                class="check-label"
                ><input type="checkbox" v-model="item.done" required />{{
                  item.label
                }}</label
              >
              <div
                v-for="(part, index) in form.parts"
                :key="index"
                class="part-row"
              >
                <label
                  >Suku cadang<input
                    v-model="part.name"
                    maxlength="200"
                    required /></label
                ><label
                  >Jumlah<input
                    v-model.number="part.quantity"
                    type="number"
                    min="1"
                    required /></label
                ><label
                  >Biaya<input
                    v-model.number="part.cost"
                    type="number"
                    min="0"
                    required /></label
                ><button
                  class="icon"
                  type="button"
                  title="Hapus suku cadang"
                  aria-label="Hapus suku cadang"
                  @click="form.parts.splice(index, 1)"
                >
                  <X :size="16" />
                </button>
              </div>
              <button
                type="button"
                class="secondary"
                @click="form.parts.push({ name: '', quantity: 1, cost: 0 })"
              >
                <Plus :size="16" />Suku cadang
              </button></template
            ><PhotoUploader
              :key="`${mode}-${form.asset_id}`"
              :asset-id="form.asset_id"
              :purpose="mode === 'complete' ? 'repair' : mode"
              v-model="photoIDs"
              @busy="uploading = $event"
          /></template>
          <template v-if="mode === 'audit'"
            ><label
              >Judul<input
                v-model="form.title"
                minlength="3"
                maxlength="200"
                required /></label
            ><label
              >Lokasi<select v-model.number="form.location_id" required aria-label="Lokasi">
                <option
                  v-for="item in locations"
                  :key="item.id"
                  :value="item.id"
                >
                  {{ item.branch_name }} / {{ item.name }}
                </option>
              </select></label
            ></template
          >
          <template v-if="mode === 'observation'"
            ><img
              v-if="form.cover_photo_id"
              class="photo-main"
              :src="`/api/photos/${form.cover_photo_id}`"
              alt="Foto resmi aset" /><label
              >Tag<input :value="form.tag" readonly /></label
            ><label
              >Temuan<select v-model="form.finding" aria-label="Temuan">
                <option value="present">Ada / sesuai</option>
                <option value="missing">Hilang</option>
                <option value="damaged">Rusak</option>
                <option value="relocated">Berpindah lokasi</option>
              </select></label
            ><label v-if="form.finding === 'relocated'"
              >Lokasi ditemukan<select
                v-model.number="form.found_location_id"
                required
              >
                <option
                  v-for="item in locations"
                  :key="item.id"
                  :value="item.id"
                >
                  {{ item.name }}
                </option>
              </select></label
            ><label
              >Catatan<textarea v-model="form.notes" maxlength="1000" /></label
            ><PhotoUploader
              v-if="form.finding !== 'present'"
              :key="form.asset_id"
              :asset-id="form.asset_id"
              purpose="audit"
              v-model="photoIDs"
              @busy="uploading = $event"
          /></template>
          <footer>
            <button
              type="button"
              class="secondary"
              :disabled="saving || uploading"
              @click="mode = ''"
            >
              Batal</button
            ><button class="primary" :disabled="saving || uploading">
              <Save :size="17" />{{ saving ? "Menyimpan..." : "Simpan" }}
            </button>
          </footer>
        </form>
      </section>
    </div>
    <div v-if="scanOpen" class="life-overlay">
      <section
        class="scan-dialog"
        role="dialog"
        aria-modal="true"
        aria-label="Pemindai QR/barcode"
      >
        <header>
          <h2>Scan aset</h2>
          <button
            class="icon"
            title="Tutup kamera"
            aria-label="Tutup kamera"
            @click="stopScanner"
          >
            <X :size="20" />
          </button>
        </header>
        <video ref="video" autoplay muted playsinline />
      </section>
    </div>
  </div>
</template>

<style scoped>
.lifecycle-workspace {
  min-width: 0;
}
.life-tabs {
  display: flex;
  gap: 4px;
  border-bottom: 1px solid var(--line);
  overflow-x: auto;
  margin-bottom: 16px;
}
.life-tabs button {
  white-space: nowrap;
  padding: 12px 16px;
  border: 0;
  border-bottom: 3px solid transparent;
  background: transparent;
  color: var(--muted);
  border-radius: 0;
}
.life-tabs .active {
  border-color: var(--sky);
  color: var(--ink);
}
.life-toolbar,
.life-toolbar form,
.life-pagination,
.calendar-heading,
.detail-actions {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}
.life-toolbar {
  margin: 12px 0 18px;
}
.life-toolbar form {
  flex: 1;
  min-width: 170px;
}
.life-toolbar input {
  min-width: 0;
  flex: 1;
}
.upload-code {
  position: relative;
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: 6px;
  cursor: pointer;
}
.upload-code input {
  position: absolute;
  opacity: 0;
  width: 1px;
  height: 1px;
}
.visual-catalog {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: 16px;
}
.visual-asset {
  border: 1px solid var(--line);
  border-radius: 6px;
  overflow: hidden;
  background: var(--surface);
}
.asset-image {
  width: 100%;
  aspect-ratio: 4/3;
  background: var(--surface-muted);
  border: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 0;
  padding: 0;
  color: var(--muted);
}
.asset-image img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.visual-info {
  padding: 14px;
}
.visual-info h2 {
  font-size: 17px;
  margin: 10px 0 4px;
}
.visual-info h2 button {
  background: none;
  border: 0;
  padding: 0;
  text-align: left;
  color: var(--ink);
  font-size: inherit;
}
.visual-info p {
  font-size: 13px;
  margin: 5px 0;
  overflow-wrap: anywhere;
}
.visual-info strong {
  display: block;
  margin-top: 10px;
}
.life-pagination {
  justify-content: flex-end;
  margin: 16px 0;
}
.calendar-heading h2 {
  font-size: 18px;
  flex: 1;
}
.calendar-heading input {
  width: 165px;
}
.service-calendar {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
  margin: 14px 0 24px;
  border-left: 1px solid var(--line);
}
.service-calendar > strong {
  text-align: center;
  padding: 8px;
  font-size: 12px;
  background: var(--surface-muted);
}
.service-calendar > div {
  min-height: 80px;
  padding: 8px;
  border-right: 1px solid var(--line);
  border-bottom: 1px solid var(--line);
  overflow: hidden;
}
.service-calendar button {
  display: block;
  max-width: 100%;
  font-size: 11px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin: 4px 0;
  padding: 3px;
  border: 0;
  background: var(--sky-soft);
  color: var(--sky);
}
.life-overlay {
  position: fixed;
  inset: 0;
  z-index: 80;
  background: #0006;
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 20px;
}
.asset-detail,
.life-form,
.scan-dialog {
  background: var(--surface);
  border-radius: 6px;
  padding: 24px;
  max-height: 90vh;
  overflow: auto;
  min-width: 0;
  border: 1px solid var(--line);
}
.asset-detail {
  width: min(1000px, 100%);
}
.asset-detail header,
.life-form header,
.service-detail header,
.scan-dialog header,
.audit-items header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}
.asset-detail h2,
.life-form h2,
.service-detail h2 {
  font-size: 20px;
  margin: 5px 0 16px;
}
.asset-detail-columns {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: 24px;
}
.photo-main,
.photo-empty {
  width: 100%;
  aspect-ratio: 4/3;
  object-fit: contain;
  background: var(--surface-muted);
  border: 1px solid var(--line);
}
.photo-empty {
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--muted);
}
.photo-strip {
  display: flex;
  gap: 8px;
  overflow: auto;
  margin: 10px 0;
}
.photo-strip button {
  flex: 0 0 68px;
  height: 60px;
  padding: 2px;
  border: 2px solid transparent;
}
.photo-strip .selected {
  border-color: var(--sky);
}
.photo-strip img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.asset-spec dl {
  display: grid;
  grid-template-columns: minmax(110px, 35%) minmax(0, 1fr);
  gap: 10px;
  font-size: 13px;
}
.asset-spec dt {
  color: var(--muted);
}
.asset-spec dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.spec-text {
  white-space: pre-wrap;
}
.form-overlay {
  z-index: 90;
}
.life-form {
  width: min(600px, 100%);
}
.life-form label {
  display: flex;
  flex-direction: column;
  gap: 5px;
  margin: 12px 0;
  font-size: 13px;
}
.life-form .check-label {
  flex-direction: row;
}
.life-form footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}
.part-row {
  display: grid;
  grid-template-columns: 2fr 1fr 1fr auto;
  gap: 8px;
  align-items: center;
}
.part-row input {
  min-width: 0;
  width: 100%;
}
.scan-dialog {
  width: min(600px, 100%);
}
.scan-dialog video {
  width: 100%;
  aspect-ratio: 4/3;
  object-fit: cover;
}
.service-detail,
.audit-items,
.life-reports {
  border-top: 1px solid var(--line);
  padding: 20px 0;
  margin-top: 20px;
}
.audit-proof {
  width: 56px;
  height: 44px;
  object-fit: contain;
}
.asset-gallery,
.asset-spec {
  min-width: 0;
}
@media (max-width: 700px) {
  .visual-catalog {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
  .visual-info {
    padding: 10px;
  }
  .visual-info h2 {
    font-size: 15px;
  }
  .life-overlay {
    padding: 10px;
  }
  .asset-detail,
  .life-form {
    padding: 16px;
    max-height: 94vh;
  }
  .asset-detail-columns {
    grid-template-columns: 1fr;
  }
  .service-calendar > div {
    padding: 4px;
    min-height: 64px;
  }
  .part-row {
    grid-template-columns: 1fr 60px 90px auto;
  }
  .life-tabs button {
    padding: 10px;
    font-size: 13px;
  }
  .calendar-heading h2 {
    width: 100%;
    flex: auto;
  }
  .asset-spec dl {
    font-size: 13px;
  }
}
</style>
