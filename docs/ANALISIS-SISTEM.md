# Analisis mendalam — AssetFlow

## 1. Tujuan bisnis dan asumsi

Sistem mengendalikan keberadaan, kepemilikan operasional, biaya, dan lifecycle aset agar keputusan pembelian/pemanfaatan/penggantian dapat diaudit. Asumsi awal: aset fisik unik (laptop, mesin, kendaraan, furniture, alat operasional), IDR, timezone bisnis Asia/Jakarta, organisasi memiliki admin dan checker berbeda. Sistem bukan stok barang habis pakai; inventory kuantitatif perlu bounded context tersendiri.

Definisi penting:

| Konsep | Definisi | Implementasi |
|---|---|---|
| Asset | Unit fisik dengan tag unik per organisasi | Ya |
| Category | Kelompok + default masa manfaat + interval/instruksi maintenance | Ya |
| Branch | Cabang perusahaan dengan code unik | Ya, per organisasi |
| Location | Lokasi fisik di dalam cabang | Ya, FK cabang |
| Custodian | Penanggung jawab pemakaian | Teks wajib pada assignment |
| Organization | Batas akses data pengguna | org_id dari session, bukan payload |
| Acquisition cost | Biaya perolehan IDR | Integer rupiah, tidak memakai float |
| Book estimate | Biaya minus depresiasi bulanan | Straight-line estimate |
| Request | Usulan transfer/disposal yang belum efektif | Maker–checker |
| Work order | Jadwal pekerjaan maintenance | Schedule/start/complete/cancel |
| Stocktake | Snapshot ekspektasi aset lokasi | Observasi tag + discrepancy |
| Audit event | Bukti actor/action/before/after | Append-only |
| User activity | Bukti login dan setiap request API | Append-only, scope perusahaan/cabang |

KPI bisnis: aset teridentifikasi, aset tanpa penanggung jawab, utilisasi, backlog maintenance, overdue, approval backlog, discrepancy stocktake, nilai perolehan non-disposed. Target bisnis ditetapkan setelah baseline 30 hari; jangan menganggap angka simulasi sebagai hasil produksi.

## 2. Personas dan akses

| Capability | Admin | Manager | Operator | Auditor |
|---|---|---|---|---|
| Dashboard/register/master read | Ya | Ya | Ya | Ya |
| Register/assign/return | Ya | Ya | Ya | Tidak |
| Request transfer/disposal | Ya | Ya | Ya | Tidak |
| Approve/reject request orang lain | Ya | Ya | Tidak | Tidak |
| Schedule/start/complete maintenance | Ya | Ya | Ya | Tidak |
| Create/observe stocktake | Ya | Ya | Ya | Tidak |
| Close stocktake | Ya | Ya | Tidak | Tidak |
| Create lokasi/kategori | Ya | Ya | Tidak | Tidak |
| Audit read | Ya | Ya | Tidak | Ya |
| Create/change user access | Ya | Tidak | Tidak | Tidak |
| Buat cabang / edit maintenance policy | Ya | Tidak | Tidak | Tidak |
| User activity read sesuai scope | Ya | Ya | Tidak | Ya |
| Own password change | Ya | Ya | Ya | Ya |

Role adalah batas API; menyembunyikan tombol bukan mekanisme otorisasi. Admin tidak dikecualikan dari maker–checker. Pengguna aktif terikat satu organisasi. Scope v2: admin selalu company-wide; role lain dapat company-wide atau daftar cabang eksplisit. Dashboard, assets, locations, requests, maintenance, stocktake dan audit menerapkan scope di backend. Filter UI tidak dapat memperluas hak. Transfer memerlukan akses asal + tujuan.

## 3. Lifecycle dan state machine

| Current | Aksi | Next | Syarat |
|---|---|---|---|
| available | assign | assigned | Custodian terisi; tidak ada request/maintenance aktif |
| assigned | return | available | Version sesuai; custodian dikosongkan |
| available | maintenance start | maintenance | Job scheduled, version sesuai |
| maintenance | maintenance complete | available | Job in_progress, biaya valid |
| available | transfer approved | available | Checker berbeda, versi request masih sesuai |
| available | dispose approved | disposed | Checker berbeda, request valid |
| disposed | aksi operasional | Ditolak | Terminal |

Sistem sengaja meminta pengembalian sebelum transfer/disposal aset assigned. Ini membuat serah terima eksplisit. Maintenance saat assigned juga membutuhkan return terlebih dahulu. Jika bisnis membutuhkan maintenance tanpa release custodian, tambahkan orthogonal availability/condition state dan revise constraint; jangan mengakali status.

Request lifecycle: pending → approved/rejected. Request tidak dapat diputuskan dua kali. Approval dan perubahan asset ada dalam transaksi yang sama. Penolakan membutuhkan alasan. Jika versi berubah, checker menolak lalu maker membuat request baru.

Maintenance lifecycle: scheduled → in_progress → completed; scheduled → cancelled. Pembatalan job yang telah berjalan belum didukung karena memerlukan model outcome (failed/rescheduled/aborted), pelepasan aset, dan biaya parsial.

## 4. Aturan data dan finansial

1. Tag dinormalisasi uppercase + trim, unik per organisasi. Serial non-kosong unik per organisasi. Tidak ada global uniqueness lintas organisasi.
2. Asset kategori/lokasi wajib milik organisasi yang sama melalui composite foreign key.
3. Nama 2–200 karakter; asset tag 1–80; note/reason dibatasi panjang. HTTP body maksimal 1 MiB.
4. Purchase cost non-negatif, salvage antara 0 dan cost, life 1–1200 bulan. Batas input monetary 10^15 rupiah agar tetap aman untuk JS integer dan operasi int64.
5. Warranty tanggal valid dan tidak sebelum purchase date. Purchase date tidak boleh lebih dari sekitar satu hari ke depan; timezone boundary harus dibahas pada UAT global.
6. Lifecycle status tidak boleh dikirim bebas pada registrasi; aset baru available.
7. Satu pending request dan satu maintenance aktif per aset melalui partial unique index; transaksi mengambil asset row lock untuk menghindari race lintas workflow.
8. DB CHECK menjaga assigned hanya dengan custodian non-kosong; selain assigned custodian kosong.
9. Status disposed dipertahankan untuk histori; tidak ada hard delete aset.
10. Version naik ketika aset berubah. Client mengirim version dari read; konflik → 409 → refresh. Backend tidak menerapkan last-write-wins.

Estimasi nilai buku: gunakan jumlah bulan penuh sejak purchase_date, clamp ke [0,life].

`book = cost − floor((cost − salvage) × elapsed_months / life)`

Perhitungan di Go membagi dan menangani sisa untuk menghindari overflow multiplication. Sebelum acquisition nilai tetap cost; pada akhir umur manfaat salvage. Formula belum menangani kapitalisasi biaya maintenance, revaluation, impairment, depreciation stop date, kalender tutup buku, metode saldo menurun, aset tanah, komponen aset, aset sewa, atau disposal proceeds. Finance harus menyetujui accounting policy sebelum integrasi GL. Dashboard acquisition cost mengecualikan disposed; nilai buku ditampilkan per baris sebagai estimasi.

## 5. Stocktake dan rekonsiliasi

- Open stocktake per lokasi membuat snapshot aset non-disposed pada saat INSERT SELECT.
- Satu stocktake open per organisasi/lokasi.
- Snapshot menyimpan asset_id, expected_tag, expected_version.
- Observe menerima tag uppercase yang ada di snapshot. Unknown tag ditolak dengan penjelasan investigasi; tidak otomatis membuat asset baru.
- Duplicate observe ditolak sehingga double scanning tidak mengubah bukti.
- UI menampilkan observed/missing dan expected/current version.
- Close memerlukan acknowledgement bila masih missing atau ada version berubah.
- Close tidak menandai aset hilang/disposed. Adjustment harus melalui proses investigasi dan approval terpisah.
- Snapshot tidak membekukan aktivitas aset. Manager harus menetapkan cut-off operasional bila membutuhkan final count tanpa pergerakan; concurrent update setelah pengecekan close bisa terjadi. Untuk enterprise tambahkan frozen snapshot result per item, timestamp cut-off, movement reconciliation, unknown-found queue, dan signed approvals.

## 6. Arsitektur dan boundary

Modular monolith dipilih untuk konsistensi transaksi dan biaya operasi. Frontend static Vue ditangani nginx, seluruh API same-origin diproxy ke Go. Go net/http menyediakan routing; pgx pool mengakses PostgreSQL. Session state berada di DB, API dapat direplikasi tanpa sticky session. Counter brute-force masih in-memory sehingga perlu shared gateway/Redis untuk replica.

Modul implementasi: Identity/access, Asset register, Workflow approval, Maintenance, Stocktake, Audit, Dashboard, Master data. Domain dan HTTP masih berdekatan dalam internal/app untuk menjaga deliverable ringkas. Jika tim membesar, pecah package handler/service/repository per bounded context tanpa langsung memecah service deployment.

| Pilihan | Performance | Cost | Security | Maintenance | Keputusan |
|---|---|---|---|---|---|
| Modular monolith | Transaksi lokal, latency rendah | Rendah | Satu perimeter, RBAC terpusat | Sederhana | Baseline |
| Microservices awal | Extra network dan distributed consistency | Tinggi | Banyak identities/perimeter | On-call kompleks | Tunda hingga bottleneck nyata |
| PostgreSQL | Constraint/lock/index matang | Rendah–menengah | Role, TLS, backup | Operasional umum | Primary DB |
| Redis | Cepat untuk shared limiter/cache | Infrastruktur tambahan | Wajib auth/network boundary | Extra dependency | Enterprise stage |
| Vue SPA | Static delivery, reactive workspace | Rendah | Hindari untrusted HTML | Familiar frontend tooling | Implemented |
| Cookie session | Immediate revocation melalui DB | DB lookup per request | HttpOnly, CSRF/origin checks | Sederhana | Implemented |
| OIDC/SSO | Managed identity/ MFA | Provider/operation cost | Lifecycle enterprise lebih kuat | Federasi/integration | Roadmap |

## 7. Konsistensi, konkurensi, keamanan

Asset lock didapat sebelum request/maintenance lock agar urutan konsisten. Partial unique index adalah backstop untuk duplicate requests/jobs. Semua mutasi bisnis dan audit dibungkus transaksi; audit gagal → rollback. Read daftar memakai pagination size maksimal 100 dan ORDER BY id stabil; count asset dihitung dengan window agar snapshot konsisten pada halaman berisi data. Halaman kosong memiliki fallback count query terpisah.

Pencarian menggunakan parameter binding. Leading-wildcard ILIKE tidak memanfaatkan index B-tree biasa. Di skala besar tambahkan pg_trgm GIN pada name/tag/serial serta ukur EXPLAIN ANALYZE; untuk navigasi dalam gunakan keyset cursor `(id < last_id)`. Offset saat ini maksimum page 100000 tetapi tetap mahal pada dataset besar. Jangan menjanjikan million-row SLA tanpa query plan dan benchmark.

Activity v2: seluruh API request dicatat, termasuk read/failed login/denied; body request, password, token dan query string tidak disimpan. Audit aktivitas terpisah dari domain audit transaksional.

Security implemented: bcrypt cost 12; random session 256-bit dengan hash SHA-256 disimpan DB; TTL 8 jam fixed; HttpOnly/SameSiteStrict/Secure pada HTTPS; same-origin/custom-header CSRF defense; JSON content-type check; no CORS wildcard; org-scoped queries; finite body/query lengths; query timeouts; role authorization; request ID dan structured logging; tidak mencatat password/token pada logs; user disable/access change/password change mencabut sesi.

Audit trigger mencegah update/delete ordinary SQL, tetapi DB owner/superuser tetap dapat melepas trigger. Enterprise audit memerlukan runtime role terbatas, immutable external sink/WORM dan separation of duties. Runtime role terpisah dari migration/owner wajib saat go-live.

Threat residual: password reset/SSO/MFA belum ada; issuer enrollment belum ada; login per-process counter memungkinkan distributed brute force tanpa shared limiter; audit retention belum dikelola; database session lookup memerlukan capacity sizing; UI memiliki focus trapping/Escape dialog tetapi belum full accessibility audit. Database TLS diperlukan bila DB berada di host berbeda. Backup mengandung informasi pengguna/custodian dan wajib dienkripsi serta dikendalikan aksesnya.

## 8. Strategi scaling

Angka berikut **profil perencanaan**, bukan hasil benchmark paket.

| Profil | Target contoh | Topology awal | Yang perlu dibuktikan |
|---|---|---|---|
| Small | 1–5 lokasi, <10k aset, <30 pengguna aktif | 1 server 2–4 vCPU, 4–8GB; Compose | UAT, backup/restore, latency |
| Medium | 5–100 lokasi, 10k–250k aset | 2+ API replicas, managed PG, shared limiter, object storage | Index trigram, keyset, pool sizing, read capacity |
| Large | >100 lokasi, >250k aset / banyak organisasi | HA gateway/API, PG HA/PITR, queue/outbox, worker, SSO, observability | Load tests, tenant scope, RLS, DR, partitioning audit |

Pool baseline max 20/API replica. Total connections = replicas × pool + migration/admin + headroom. Naikkan replica tanpa menghitung DB connection budget akan mengurangi availability. PgBouncer dapat ditambahkan dengan compatibility prepared statement configuration yang diverifikasi. Read replicas hanya untuk read yang boleh lag; approve/query version tetap primary.

Microservices baru dibenarkan jika modul mempunyai scaling/team/release boundary berbeda. Notification, attachment processing, report batch, integration worker dapat dipisah terlebih dahulu. Gunakan transactional outbox, idempotent consumer, retries exponential + dead-letter, dan reconciliation untuk integrasi ERP.

## 9. Feature gap dan roadmap dengan acceptance

| Stage | Modul | Acceptance |
|---|---|---|
| Before go-live | DB runtime role, TLS, backups, restore, shared edge limiter, scan, UAT | Evidence dan runbook owner ditandatangani |
| P1 | Asset correction approval + optimistic edit | Before/after audit, tidak overwrite accounting period |
| P1 | Employee/vendor directory + assignment acknowledgement | FK valid, offboarding asset reconciliation |
| P1 | Bulk CSV import dry-run, row errors, async full export | Tenant scope, idempotency, formula injection mitigation |
| P1 | Label PDF/QR generator + camera scanning | Print/scanner UAT, canonical tag, permission |
| P1 | Attachments S3 compatible | Content limits, malware scanning, signed URL, retention |
| P2 | Background recurring work-order + notifications | Retry/idempotency, local timezone schedule, overdue escalation |
| P2 | Finance ledger, disposal proceeds, capitalization, ERP | Reconciled journal, finance-approved policy, closed periods |
| P2 | OIDC/MFA, access per room/location, delegated administration | Negative authorization tests, access review |
| P2 | Stocktake found/unknown queue, immutable close snapshot | Reconciliation cut-off, maker/checker adjustment |
| P3 | Multi-level threshold approvals | Rules versioned; no self approval at any stage |
| P3 | Insurance, lease, licenses, service contracts | Renewal alerts, ownership model, document linking |
| P3 | Mobile offline / high volume event ingestion | Conflict strategy, signed sync, replay-safe writes |

Procurement can remain external ERP in initial rollout: receive PO-approved asset and register. If built later, separate requisition → budget approval → PO → receipt → capitalization, with vendor and goods-receipt proof. Asset hierarchy/componentization, IoT condition data, GIS, spare parts and consumable inventory are separate extensions, not fields added indiscriminately to assets.

## 10. Detail v2

Lihat `UPGRADE-V2.md` untuk company–branch model, policy maintenance, fail-closed activity logging, kompatibilitas v1 dan pembatasan histori. Maintenance due dihitung otomatis; work order tetap dijadwalkan operator (tidak ada worker email/job otomatis).

## 11. Rollout dan governance

Discovery: asset types, ownership vs lease, locations, accounting policy, approvals, user directory, retention, RPO/RTO. Clean existing spreadsheet; assign unique tags; deduplicate serials; confirm opening balances. Pilot 1 lokasi, validate assignment and stocktake; compare data manual. Training admin/operator/checker; agree SOP return/offboarding. Roll out per lokasi with signed reconciliation and support window.

Owners: business asset owner, Finance policy owner, IT service owner, IAM admin, Data steward, DR owner. Weekly review overdue/request age, monthly utilization/discrepancy, periodic access review. Define approval SLA and escalation ownership before notifications are automated.

Success is accurate, governed asset data and recoverable operations; frontend appearance alone is insufficient production acceptance.

## Referensi teknis

- Go 1.26: https://go.dev/doc/go1.26
- Go support/release history: https://go.dev/doc/devel/release
- Vue security: https://vuejs.org/guide/best-practices/security
- PostgreSQL row locking: https://www.postgresql.org/docs/17/explicit-locking.html

Versi paket direkam melalui go.mod/go.sum dan package-lock.json. Scan ulang pada setiap upgrade; hasil scan adalah point-in-time.
