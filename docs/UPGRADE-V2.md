# AssetFlow v2 — perusahaan, cabang, audit user, maintenance kategori

## Domain dan hak akses

Perusahaan (organizations) memiliki banyak branches. Branch memiliki banyak locations. Asset menunjuk location, sehingga branch aset mengikuti lokasi secara konsisten. Tidak ada branch_id ganda pada asset yang dapat berbeda dengan lokasi. Kategori dipakai bersama dalam perusahaan.

| Role/scope | Kemampuan |
|---|---|
| Admin pusat | Seluruh cabang, buat cabang, user scope, edit policy kategori |
| Manager company-wide | Operasi/approval seluruh cabang; tetap maker–checker |
| Manager cabang | Operasi/approval hanya cabang yang diberikan |
| Operator cabang | Registrasi/assignment/maintenance/stocktake hanya cabang yang diberikan |
| Auditor cabang | Read aset, domain audit, aktivitas yang relevan dengan scope |

Transfer antar-cabang: pembuat dan checker harus mempunyai akses ke cabang asal dan tujuan. User terbatas satu cabang tidak dapat mengirim aset ke cabang yang tidak diizinkan; admin pusat atau user multi-cabang menangani transfer tersebut. Lokasi dan kategori referensi tetap harus satu perusahaan. Setelah transfer, aset hilang dari register asal dan muncul di register tujuan. Request menyimpan source/target branch snapshot. Maintenance menyimpan branch saat work order dibuat; histori tidak ikut berpindah secara diam-diam.

Audit perubahan mencatat cabang resource dan related branch untuk transfer. Aktivitas mencatat snapshot scope actor plus resource branch bila diketahui. Auditor cabang tidak melihat login company-wide/failed login tanpa branch context milik orang lain; admin pusat melihat seluruh log perusahaan. Event sendiri tetap dapat dibaca dalam activity apabila role mengizinkan.

## Audit dua lapis

1. **Domain audit**: create/assignment/return/transfer/disposal/maintenance/stocktake/master/access/password changes, actor, before/after, request ID, branch. Ditulis dalam transaksi mutasi: audit gagal berarti mutasi rollback.
2. **User activity**: seluruh request `/api/*`, termasuk login sukses/gagal/throttled, logout, read, mutation, validation/access/CSRF denial, unknown route. HTTP status, timestamp (UI WIB), duration, request ID, socket peer IP dan user agent. Tidak mencatat body, password atau session token. Query string tidak dicatat. Export CSV diproses backend dan mencatat jumlah baris serta filter page/branch/status; bukan download tanpa audit. Login menyimpan attempted employee ID untuk investigasi; ini identitas yang diajukan, belum tentu user yang benar. Email tetap menjadi data akun, sedangkan ID karyawan otomatis dibuat dari ID pengguna saat migrasi.

Activity menggunakan response buffering dan harus tersimpan sebelum respons dilepas. Jika penulisan activity gagal, respons 503. Mutasi domain mungkin sudah commit beserta domain audit; refresh state sebelum retry. Ini fail-closed pada pelaporan keberhasilan, bukan transaksi atomik lintas domain mutasi dan activity. Tidak ada generic idempotency middleware.

Semua tabel audit menolak UPDATE/DELETE via trigger. WORM/retention/partitioning enterprise masih perlu external immutable archive dan DBA procedure. Health probes/static pages bukan aksi aplikasi dan tidak masuk log API. Klik UI yang tidak mengirim request (misalnya membuka modal) bukan event server; perubahan dan pembacaan data tetap diaudit.

Peer IP adalah alamat koneksi ke Go, biasanya reverse proxy. Original client IP membutuhkan trustworthy edge logs; backend tidak mempercayai header spoofable secara langsung.

## Maintenance kategori

- Kategori: `maintenance_interval_days` 0–3650; 0 = manual only. Instruksi maksimal 2000 karakter.
- Asset baru: next_maintenance_date = purchase_date + interval; null jika interval 0.
- Work order complete: next due = tanggal completion bisnis (Asia/Jakarta) + policy kategori saat completion.
- Schedule/start/cancel tidak menggeser next due. Disposal mengosongkan due date.
- Dashboard menampilkan maintenance_due_assets dan overdue work orders. Register menampilkan tanggal next due; dialog schedule memakainya sebagai default.
- Edit policy memakai category version. Opsi apply_to_existing menghitung dari last completed date/purchase date, mengubah version aset eligible, dan melewati pending request/job aktif. Audit satu aksi policy menyimpan affected count; bukan audit per baris rekalkulasi.
- Actual work order tetap dibuat operator. Tidak ada background worker yang otomatis membuat jobs atau mengirim email.

## Upgrade database existing

1. Backup, verifikasi restore terpisah, simpan source/image lama.
2. Build paket v2 dan jalankan migration secara eksplisit:
```bash
docker compose build
docker compose run --rm migrate
docker compose up -d --force-recreate api web
```
3. Untuk runtime role terpisah, jalankan ulang grants spesifik `docs/least-privilege.sql` dengan owner role setelah migration.
4. Jangan jalankan bootstrap pada DB existing.
5. Migration v2 membuat HQ per perusahaan existing dan memetakan semua lokasi lama ke HQ. Asset, request, maintenance dan stocktake dipertahankan. Audit lama tidak diubah; branch metadata legacy tetap null.
6. Existing users mempertahankan company-wide scope supaya tidak terkunci. **Admin wajib meninjau dan mengubah user non-admin ke cabang yang sesuai sebelum perluasan rollout.** User baru default scoped dan memerlukan branch assignment.
7. Buat cabang sebenarnya serta lokasi baru, kemudian transfer aset dari HQ melalui approval. Jangan mengubah branch lokasi lama langsung karena dapat merusak makna histori/snapshot.
8. Atur interval maintenance kategori; pilih apply_to_existing bila ingin menghitung baseline due untuk aset lama.
9. Jalankan native PostgreSQL integration tests dan UAT negatif scope cabang di staging.

Migration runner menjalankan seluruh versi berurutan per transaksi dengan advisory lock, tidak mengulang versi yang sudah applied, dan tidak mempunyai destructive down migration. Migration 5 menambahkan kode organisasi unik `ORG-` + ID organisasi 6 digit dan mengisi kode untuk seluruh organisasi yang sudah ada. Bootstrap baru mencetak kode ini pada log `organization_code`. Command migrate memakai timeout statement 240 detik dan total 5 menit; API tetap 10 detik per statement. Dataset besar perlu rehearsal, maintenance window untuk backfill/index dan disk headroom; sesuaikan timeout melalui review deployment jika rehearsal melampaui batas. Image/DB acceptance tetap wajib di server target.
