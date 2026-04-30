# V16 Mini PC Access Playbook

Bu fayl `fedora` mini PC ga kirish uchun yagona, toza qo'llanma.

## Server identifikatsiyasi

- Hostname: `fedora`
- LAN IP: `10.42.0.80`
- Tailscale IP: `100.92.208.128`
- SSH user: `wikki`
- SSH password: `Aywi2008_`

## 1) LAN orqali kirish (eng tez)

Bir xil lokal tarmoqda bo'lsangiz:

```bash
ssh wikki@10.42.0.80
```

## 2) Internet orqali Bore tunnel (asosiy va backup)

Hozir ishlayotgan portlar:

- Primary: `22023`
- Backup: `22024`
- Legacy: `22022` (bor, lekin ba'zida konflikt bo'lishi mumkin)

Ulanish:

```bash
ssh -p 22023 wikki@bore.pub
```

Agar birinchisi ishlamasa:

```bash
ssh -p 22024 wikki@bore.pub
```

Legacy variant:

```bash
ssh -p 22022 wikki@bore.pub
```

## 3) Tailscale orqali kirish

Avval lokal kompyuteringizda:

```bash
systemctl is-active tailscaled
tailscale status
tailscale ping fedora
```

Keyin SSH:

```bash
tailscale ssh wikki@fedora
```

Yoki IP bilan:

```bash
tailscale ssh wikki@100.92.208.128
```

## Tez health-check komandalar

Serverga kirgandan keyin:

```bash
hostname
whoami
uptime
```

`gscale-erp-read` tekshirish:

```bash
systemctl status gscale-erp-read.service --no-pager -l
curl -sS http://127.0.0.1:8090/healthz
```

## Hozirgi auto-recovery holati

Bore tunnel servislar bootda avtomatik turadi va network o'zgarsa qayta tiklanadi:

- `mini-pc-ssh-bore.service` (22022)
- `mini-pc-ssh-bore-22023.service` (22023)
- `mini-pc-ssh-bore-22024.service` (22024)

Tekshirish:

```bash
systemctl is-active mini-pc-ssh-bore.service
systemctl is-active mini-pc-ssh-bore-22023.service
systemctl is-active mini-pc-ssh-bore-22024.service
```

## Muammo bo'lsa

1. LAN ishlamasa:
   - `ping 10.42.0.80`
   - `ssh wikki@10.42.0.80`
2. Bore ishlamasa:
   - avval `22023`, keyin `22024` ni sinang
3. Tailscale ishlamasa:
   - `sudo systemctl start tailscaled`
   - `tailscale up`

