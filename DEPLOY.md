# 🚀 Tutorial Deploy WA Bridge dengan Docker

## Prasyarat Server
- Linux (Ubuntu/Debian/CentOS) atau VPS
- Docker & Docker Compose terinstall
- Port 3000 terbuka di firewall

---

## Langkah 1: Upload File ke Server

### Opsi A — Git (Recommended)
```bash
# Di PC lokal
cd c:\Users\p\Pictures\wa
git init
git add .
git commit -m "WA Bridge v1.0"
git remote add origin https://github.com/username/wa-bridge.git
git push -u origin main

# Di server
git clone https://github.com/username/wa-bridge.git
cd wa-bridge
```

### Opsi B — SCP Manual
```bash
# Di PC lokal, zip folder (TANPA .exe, data/, .env)
# Upload ke server
scp wa-bridge.zip user@server-ip:/home/user/

# Di server
unzip wa-bridge.zip -d wa-bridge
cd wa-bridge
```

> ⚠️ File `.exe` **TIDAK perlu** diupload. Docker akan compile sendiri di dalam container.

---

## Langkah 2: Konfigurasi .env

```bash
cp .env.example .env
nano .env
```

```env
# ===== Server =====
PORT=3000
JWT_SECRET=buat_random_string_panjang_disini_32char

# ===== Chatwoot (Default/Fallback) =====
CHATWOOT_API_URL=https://chat.asawidia.com
CHATWOOT_API_KEY=bQxvgQES6HkQqXBqqsf56EB1
CHATWOOT_ACCOUNT_ID=2
CHATWOOT_INBOX_ID=4

# ===== Filters =====
IGNORE_BROADCAST=true
IGNORE_STATUS=true
IGNORE_GROUPS=false
IGNORE_CHANNELS=true
```

---

## Langkah 3: Jalankan dengan Docker

```bash
docker compose up -d --build
```

Tunggu ~2 menit (download dependencies + compile). Cek status:
```bash
docker compose logs -f    # Lihat log realtime
docker compose ps          # Cek status container
```

Output yang diharapkan:
```
wa-bridge  | ╔═══════════════════════════════════════╗
wa-bridge  | ║     WA Bridge — GOWS Engine (Go)      ║
wa-bridge  | ╠═══════════════════════════════════════╣
wa-bridge  | ║  🌐 Dashboard: http://localhost:3000   ║
wa-bridge  | ╚═══════════════════════════════════════╝
```

---

## Langkah 4: Akses Dashboard

Buka browser: `http://IP-SERVER:3000`

1. **Pertama kali** → daftar username/password baru
2. **Login** → buat Session → scan QR Code
3. **Status "connected"** → WA Bridge siap!

---

## Langkah 5: Hubungkan ke Chatwoot (Multi-Session)

### Skenario: 3 nomor WA → 3 Inbox Chatwoot

#### Di Chatwoot (chat.asawidia.com):

| Inbox | Channel | Webhook URL |
|-------|---------|-------------|
| WA Sales (ID: 4) | API | `http://IP-SERVER:3000/webhook/chatwoot/sales` |
| WA Support (ID: 7) | API | `http://IP-SERVER:3000/webhook/chatwoot/support` |
| WA Billing (ID: 9) | API | `http://IP-SERVER:3000/webhook/chatwoot/billing` |

#### Di WA Bridge Dashboard:

Klik **"+ New Session"** untuk setiap nomor WA:

| Session ID | Chatwoot Inbox ID | Scan QR |
|-----------|-------------------|---------|
| `sales` | `4` | QR HP Sales |
| `support` | `7` | QR HP Support |
| `billing` | `9` | QR HP Billing |

#### Cara Kerja:
```
Customer WA → HP Sales → Session "sales" → Chatwoot Inbox #4
Agent reply Inbox #4 → webhook/chatwoot/sales → Session "sales" → HP Sales → Customer

Customer WA → HP Support → Session "support" → Chatwoot Inbox #7
Agent reply Inbox #7 → webhook/chatwoot/support → Session "support" → HP Support → Customer
```

### FAQ Multi-Session:

**Q: Kenapa .env hanya ada 1 CHATWOOT_INBOX_ID?**
A: `.env` adalah **fallback default**. Setiap session bisa punya Inbox ID sendiri yang diset saat membuat session di dashboard. Jika session tidak diset Inbox ID, baru pakai yang di `.env`.

**Q: Bagaimana jika saya hanya punya 1 nomor WA?**
A: Cukup 1 session "default" + 1 inbox di Chatwoot. Semuanya sudah bekerja dengan konfigurasi `.env` saat ini.

---

## Perintah Berguna

```bash
# Restart
docker compose restart

# Stop
docker compose down

# Update (setelah git pull)
docker compose up -d --build

# Lihat log
docker compose logs -f --tail=50

# Masuk ke container
docker compose exec wa-bridge sh

# Backup database
docker compose cp wa-bridge:/app/data ./backup-data
```

---

## Troubleshooting

| Masalah | Solusi |
|---------|--------|
| Chatwoot "Connection refused" | Webhook URL menggunakan `localhost`. Ganti ke IP server WA Bridge |
| QR code tidak muncul | Cek log: `docker compose logs -f` |
| Session hilang setelah restart | Pastikan volume `wa_data` terpasang |
| Build error | Jalankan `docker compose build --no-cache` |
