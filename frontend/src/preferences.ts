import { ref, watch } from "vue";

export type Locale = "id" | "en";
export type Theme = "light" | "dark";

const storedLocale = typeof localStorage === "undefined" ? "id" : localStorage.getItem("assetflow-locale");
const storedTheme = typeof localStorage === "undefined" ? "light" : localStorage.getItem("assetflow-theme");
export const locale = ref<Locale>(storedLocale === "en" ? "en" : "id");
export const theme = ref<Theme>(storedTheme === "dark" ? "dark" : "light");

const messages: Record<Locale, Record<string, string>> = {
  id: {
    "nav.dashboard": "Ringkasan", "nav.assets": "Daftar aset", "nav.requests": "Persetujuan", "nav.maintenance": "Pemeliharaan", "nav.stocktakes": "Stock opname", "nav.branches": "Cabang", "nav.locations": "Lokasi", "nav.categories": "Kategori", "nav.audit": "Jejak audit", "nav.activity": "Aktivitas pengguna", "nav.users": "Tim & akses", "nav.finance": "Keuangan & laporan",
    "page.dashboard": "Ringkasan", "page.assets": "Daftar aset", "page.requests": "Persetujuan", "page.maintenance": "Pemeliharaan", "page.stocktakes": "Stock opname", "page.branches": "Cabang", "page.locations": "Lokasi", "page.categories": "Kategori", "page.audit": "Jejak audit", "page.activity": "Aktivitas pengguna", "page.users": "Tim & akses", "page.finance": "Keuangan & laporan",
    "common.refresh": "Muat ulang", "common.loading": "Memuat data…", "common.search": "Cari", "common.archive": "Arsip", "common.activeData": "Data aktif", "common.cancel": "Batal", "common.save": "Simpan", "common.close": "Tutup", "common.actions": "Aksi", "common.status": "Status", "common.branch": "Cabang", "common.location": "Lokasi", "common.name": "Nama", "common.reason": "Alasan", "common.period": "Periode", "common.allBranches": "Pusat · semua cabang", "common.language": "Bahasa", "common.theme": "Tema",
    "login.welcome": "Selamat datang kembali", "login.title": "Masuk ke workspace", "login.description": "Gunakan akun organisasi Anda.", "login.organization": "Organisasi", "login.branch": "Masuk sebagai", "login.employee": "ID karyawan", "login.email": "Email", "login.password": "Password", "login.submit": "Masuk workspace", "login.processing": "Memproses…",
    "dashboard.description": "Pantau aset, tanggung jawab, dan tindak lanjut berikutnya.", "dashboard.total": "Total aset terdaftar", "dashboard.available": "Tersedia", "dashboard.assigned": "Digunakan", "dashboard.maintenance": "Dalam pemeliharaan", "dashboard.portfolio": "Nilai portofolio", "dashboard.attention": "Perlu perhatian", "dashboard.pending": "Persetujuan tertunda", "dashboard.overdue": "Pemeliharaan terlambat", "dashboard.recent": "Aset terbaru", "dashboard.viewRegister": "Lihat daftar", "dashboard.empty": "Belum ada aset. Mulai dengan mendaftarkan aset pertama.",
    "asset.register": "Daftarkan aset", "asset.edit": "Ubah aset", "asset.tag": "Tag aset", "asset.serial": "Nomor seri", "asset.category": "Kategori", "asset.purchaseDate": "Tanggal perolehan", "asset.cost": "Harga perolehan (Rp)", "asset.salvage": "Nilai residu (Rp)", "asset.life": "Masa manfaat (bulan)", "asset.method": "Metode penyusutan", "asset.start": "Mulai penyusutan", "asset.supplier": "Pemasok", "asset.reference": "Referensi perolehan", "asset.warranty": "Garansi sampai", "asset.bookValue": "Nilai buku estimasi", "asset.history": "Riwayat aset", "asset.straight": "Garis lurus", "asset.declining": "Saldo menurun ganda", "asset.nonDepreciable": "Tidak disusutkan",
    "finance.depreciation": "Penyusutan", "finance.valuations": "Penilaian ulang", "finance.contracts": "Garansi & kontrak", "finance.journals": "Jurnal CSV", "finance.settings": "Pemetaan akun", "finance.compliance": "Kepatuhan", "finance.export": "Unduh CSV jurnal", "finance.propose": "Ajukan penilaian", "finance.addContract": "Tambah kontrak", "finance.approve": "Setujui", "finance.reject": "Tolak", "finance.saveAccounts": "Simpan pemetaan", "finance.pending": "Menunggu keputusan", "finance.empty": "Tidak ada data untuk pilihan ini.", "finance.disclaimer": "Estimasi operasional; kebijakan akuntansi dan pemetaan akun perlu divalidasi oleh tim finance.",
    "status.available": "Tersedia", "status.assigned": "Digunakan", "status.maintenance": "Pemeliharaan", "status.disposed": "Dilepas", "status.pending": "Menunggu", "status.approved": "Disetujui", "status.rejected": "Ditolak", "status.scheduled": "Terjadwal", "status.in_progress": "Berlangsung", "status.completed": "Selesai", "status.cancelled": "Dibatalkan",
    "role.admin": "Administrator Pusat", "role.branch_admin": "Administrator Cabang", "role.manager": "Manajer", "role.operator": "Operator", "role.staff": "Staf", "role.employee": "Karyawan", "role.finance": "Keuangan", "role.it_support": "IT Support", "role.it_developer": "IT Developer", "role.auditor": "Auditor",
  },
  en: {
    "nav.dashboard": "Overview", "nav.assets": "Asset register", "nav.requests": "Approvals", "nav.maintenance": "Maintenance", "nav.stocktakes": "Stocktake", "nav.branches": "Branches", "nav.locations": "Locations", "nav.categories": "Categories", "nav.audit": "Audit trail", "nav.activity": "User activity", "nav.users": "Team & access", "nav.finance": "Finance & reports",
    "page.dashboard": "Overview", "page.assets": "Asset register", "page.requests": "Approvals", "page.maintenance": "Maintenance", "page.stocktakes": "Stocktake", "page.branches": "Branches", "page.locations": "Locations", "page.categories": "Categories", "page.audit": "Audit trail", "page.activity": "User activity", "page.users": "Team & access", "page.finance": "Finance & reports",
    "common.refresh": "Refresh", "common.loading": "Loading data…", "common.search": "Search", "common.archive": "Archive", "common.activeData": "Active data", "common.cancel": "Cancel", "common.save": "Save", "common.close": "Close", "common.actions": "Actions", "common.status": "Status", "common.branch": "Branch", "common.location": "Location", "common.name": "Name", "common.reason": "Reason", "common.period": "Period", "common.allBranches": "Head office · all branches", "common.language": "Language", "common.theme": "Theme",
    "login.welcome": "Welcome back", "login.title": "Sign in to your workspace", "login.description": "Use your organization account.", "login.organization": "Organization", "login.branch": "Sign in scope", "login.employee": "Employee ID", "login.email": "Email", "login.password": "Password", "login.submit": "Sign in", "login.processing": "Signing in…",
    "dashboard.description": "A clear view of your assets, responsibilities, and next actions.", "dashboard.total": "Total registered assets", "dashboard.available": "Available", "dashboard.assigned": "Assigned", "dashboard.maintenance": "Under maintenance", "dashboard.portfolio": "Portfolio value", "dashboard.attention": "Requires attention", "dashboard.pending": "Pending approvals", "dashboard.overdue": "Overdue maintenance", "dashboard.recent": "Recently registered", "dashboard.viewRegister": "View register", "dashboard.empty": "No assets yet. Register your first asset to get started.",
    "asset.register": "Register asset", "asset.edit": "Edit asset", "asset.tag": "Asset tag", "asset.serial": "Serial number", "asset.category": "Category", "asset.purchaseDate": "Acquisition date", "asset.cost": "Acquisition cost (IDR)", "asset.salvage": "Residual value (IDR)", "asset.life": "Useful life (months)", "asset.method": "Depreciation method", "asset.start": "Depreciation starts", "asset.supplier": "Supplier", "asset.reference": "Acquisition reference", "asset.warranty": "Warranty until", "asset.bookValue": "Estimated book value", "asset.history": "Asset history", "asset.straight": "Straight line", "asset.declining": "Double declining balance", "asset.nonDepreciable": "Non-depreciable",
    "finance.depreciation": "Depreciation", "finance.valuations": "Revaluations", "finance.contracts": "Warranty & contracts", "finance.journals": "Journal CSV", "finance.settings": "Account mapping", "finance.compliance": "Compliance", "finance.export": "Download journal CSV", "finance.propose": "Propose valuation", "finance.addContract": "Add contract", "finance.approve": "Approve", "finance.reject": "Reject", "finance.saveAccounts": "Save mapping", "finance.pending": "Awaiting decision", "finance.empty": "No data for this selection.", "finance.disclaimer": "Operational estimates only; accounting policy and account mappings should be validated by your finance team.",
    "status.available": "Available", "status.assigned": "Assigned", "status.maintenance": "Maintenance", "status.disposed": "Disposed", "status.pending": "Pending", "status.approved": "Approved", "status.rejected": "Rejected", "status.scheduled": "Scheduled", "status.in_progress": "In progress", "status.completed": "Completed", "status.cancelled": "Cancelled",
    "role.admin": "Head office administrator", "role.branch_admin": "Branch administrator", "role.manager": "Manager", "role.operator": "Operator", "role.staff": "Staff", "role.employee": "Employee", "role.finance": "Finance", "role.it_support": "IT Support", "role.it_developer": "IT Developer", "role.auditor": "Auditor",
  },
};

export function t(key: string): string {
  return messages[locale.value][key] || key;
}

function applyTheme(value: Theme) {
  if (typeof document !== "undefined") document.documentElement.dataset.theme = value;
}

watch(locale, (value) => {
  if (typeof localStorage !== "undefined") localStorage.setItem("assetflow-locale", value);
  if (typeof document !== "undefined") document.documentElement.lang = value;
});
watch(theme, (value) => {
  if (typeof localStorage !== "undefined") localStorage.setItem("assetflow-theme", value);
  applyTheme(value);
});
applyTheme(theme.value);

export function toggleTheme() { theme.value = theme.value === "light" ? "dark" : "light"; }
