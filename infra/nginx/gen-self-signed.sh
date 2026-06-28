#!/usr/bin/env bash
# Sinh self-signed cert cho LogMon local production-like (HTTPS giống prod thật,
# không cần domain). Prod internet thay bằng Let's Encrypt (certbot HTTP-01).
# Dùng: infra/nginx/gen-self-signed.sh [CN]   (mặc định CN=localhost)
set -euo pipefail

CN="${1:-localhost}"
CERT_DIR="$(cd "$(dirname "$0")" && pwd)/certs"
mkdir -p "$CERT_DIR"

if [[ -f "$CERT_DIR/fullchain.pem" && -f "$CERT_DIR/privkey.pem" ]]; then
  echo "Cert đã tồn tại tại $CERT_DIR — bỏ qua (xoá để tạo lại)."
  exit 0
fi

openssl req -x509 -nodes -newkey rsa:2048 -days 365 \
  -keyout "$CERT_DIR/privkey.pem" \
  -out "$CERT_DIR/fullchain.pem" \
  -subj "/CN=$CN" \
  -addext "subjectAltName=DNS:$CN,DNS:localhost,IP:127.0.0.1"

echo "Đã tạo self-signed cert (CN=$CN) tại $CERT_DIR"
