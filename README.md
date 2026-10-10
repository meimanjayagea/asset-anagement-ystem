# AssetFlow v2 — Multi-branch Asset Management System

Go + Vue 3/TypeScript + PostgreSQL + Docker Compose. Bahasa UI Indonesia/English.

**Delivery:** executable core untuk aset fisik, perusahaan–cabang–lokasi, satu organisasi per pengguna; schema mendukung multiple organizations. Ini baseline yang perlu UAT, load test dan deployment acceptance sebelum go-live. Bukan klaim sertifikasi production atau implementasi penuh CMMS/ERP.

## Yang sudah tersedia

- Session login (HttpOnly/SameSite cookie), password bcrypt, logout/revocation, ganti password.
- RBAC admin, manager, operator, auditor; company-wide atau akses ke beberapa cabang, enforced di API.
- Master cabang, lokasi per cabang, filter dashboard/register/workflow per cabang.
- Asset register: tag/serial unik, kategori, lokasi, custodian, tanggal pembelian, warranty, biaya dan estimasi nilai buku.
- Assignment/return dengan version conflict protection.
- Transfer/disposal request, approval berbeda orang, alasan penolakan, transaksi atomik.
- Maintenance schedule/start/complete/cancel; biaya aktual; dashboard overdue; interval dan instruksi per kategori; tanggal maintenance berikutnya otomatis.
- Stocktake snapshot lokasi, observe tag, missing/changed discrepancy, close acknowledgement.
- Dashboard, search/filter/pagination, CSV halaman aktif dengan spreadsheet injection protection.
- Master lokasi/kategori; tambah user, ubah akses/nonaktifkan user dan cabut seluruh sesinya.
- Audit perubahan append-only dengan before/after dan cabang asal/tujuan.
- Audit aktivitas setiap API request: login sukses/gagal/throttled, logout, read, action, denied, request ID, peer IP, user agent, HTTP status dan durasi. Password/session token tidak dicatat.
- Migration command, bootstrap command, non-root containers, healthchecks, graceful shutdown, structured log, CI, backup script.

## Update v2

Lihat `docs/UPGRADE-V2.md` untuk migration existing database, setup cabang dan user scope. **Backup dahulu; data existing dipetakan ke HQ dan histori audit lama dipertahankan.**

## Jalankan di Docker

Prasyarat: Docker Engine/Desktop dengan Compose v2, port 8088 bebas.

```bash
cp .env.example .env
# Edit .env: ganti seluruh placeholder password.
# Untuk POSTGRES_PASSWORD gunakan 64 karakter hex (URL-safe).
# Contoh generate: openssl rand -hex 32

docker compose up -d --build
docker compose --profile setup run --rm bootstrap
```

Buka **http://localhost:8088**. Organization ID pertama = 1. Login menggunakan BOOTSTRAP_EMAIL / BOOTSTRAP_PASSWORD yang Anda set. Tidak ada default akun/password yang otomatis aktif.

Bootstrap sekali pada database kosong. Sesudah berhasil, hapus `BOOTSTRAP_PASSWORD` dari `.env`. Tambahkan akun manager kedua agar approval maker–checker dapat diuji.

```bash
docker compose ps
docker compose logs --tail=100 api
docker compose logs migrate
curl http://localhost:8088/health/ready
```

Compose hanya mengekspos web pada loopback. API/DB tidak memiliki host port. `docker compose down` mempertahankan volume; `down -v` menghapus data.

## Production pada server

1. Set `APP_ORIGIN=https://assets.example.com`, `COOKIE_SECURE=true`.
2. Pertahankan `BIND_ADDRESS=127.0.0.1`; reverse proxy host ke `127.0.0.1:8088` dan terminasi TLS. Jika proxy container, hubungkan ke network Compose tanpa mengekspos DB/API.
3. Provision runtime DB role least privilege, migration role terpisah; contoh `docs/least-privilege.sql`. Compose awal adalah konfigurasi single-server dan belum HA.
4. Ganti image tag menjadi digest yang telah lolos scan CI; simpan secrets di secret manager sesuai infrastruktur.
5. Jalankan staging/UAT, integration tests, restore rehearsal dan load test; lihat `docs/PRODUCTION.md`.
6. Rate limiting pada reverse proxy terpusat wajib jika menggunakan beberapa replica. Counter login internal hanya per proses.

TLS tidak disediakan oleh Compose. HTTP insecure hanya diterima untuk origin localhost/127.0.0.1.

## Development

```bash
# Backend (Go 1.26)
cd backend
go mod download
DATABASE_URL='postgres://user:password@localhost:5432/assetflow?sslmode=disable' go run ./cmd/server migrate
# bootstrap dengan DATABASE_URL + ORG_NAME + BOOTSTRAP_EMAIL + BOOTSTRAP_PASSWORD
DATABASE_URL='...' APP_ORIGIN=http://localhost:5173 COOKIE_SECURE=false go run ./cmd/server

# Frontend (Node 24)
cd frontend
npm ci
npm run dev
```

Vite proxy `/api` ke backend :8080. Frontend :5173, cookie same-origin; backend memvalidasi Origin sesuai APP_ORIGIN.

## Validasi

```bash
cd backend
go test -race ./...
go vet ./...
# Dedicated disposable database; test creates and drops a dedicated schema.
TEST_DATABASE_URL='postgres://postgres:test@localhost:5432/assetflow?sslmode=disable' go test -race -run TestAPIIntegration -v ./internal/app

go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
cd ../frontend
npm ci
npm run build
npm audit --audit-level=high
```

Lihat `docs/VALIDATION.md` untuk hasil validasi aktual paket ini. Tanpa TEST_DATABASE_URL, integration test **skip**, bukan pass.

## Struktur

```text
backend/cmd/server       startup, migrations, bootstrap, shutdown
backend/internal/app     HTTP API, domain rules, PostgreSQL transactions, tests
backend/migrations       schema SQL (copy embedded wajib sinkron)
frontend/src             Vue workspace + typed API client
validation               portable v1 → v2 SQL verification
frontend/nginx.conf      same-origin proxy dan security headers
compose.yml              database, migration job, API, web, bootstrap profile
scripts/backup.sh         pg_dump custom format
.github/workflows        build, tests, vulnerability checks
```

Dokumentasi utama:

- `docs/ANALISIS-SISTEM.md`: domain, scope, personas, rules, scale, trade-offs, roadmap.
- `docs/API.md`: kontrak endpoint dan contoh payload.
- `docs/PRODUCTION.md`: topology, deployment, SLO, backup/restore, rollout, risiko.
- `docs/UAT.md`: acceptance scenarios.
- `docs/VALIDATION.md`: bukti verifikasi dan batasannya.

## Batas implementasi yang harus diketahui

Tidak ada onboarding organisasi self-service, master employee directory, edit umum aset setelah registrasi, background work-order/notification scheduler (tanggal due sudah otomatis), email notifications, full CSV import/export, attachments, QR camera scanner, ERP journal, fiscal depreciation, OIDC/MFA, approval multilevel, warranty reminder worker, offline mobile, atau soft-delete master. Tag stocktake dapat diketik atau diinput USB scanner sebagai keyboard. Custodian adalah teks wajib, belum foreign key pegawai. Perubahan master/kesalahan registrasi saat ini perlu workflow koreksi terkontrol oleh DBA; jangan mengubah audit.

Roadmap dan acceptance gates untuk masing-masing ada di analisis. Batas ini berarti **enterprise rollout masih membutuhkan pekerjaan lanjutan**, walaupun core source dan Docker packaging sudah disediakan.


