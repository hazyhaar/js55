#!/usr/bin/env bash
# ==============================================================================
# js55 — Script de Déploiement vers horos-prod (37.187.150.79)
# Cible : Sous-domaine js55.hazyhaar.fr (Port local 8557)
# ==============================================================================

set -euo pipefail

TARGET_HOST="horos-prod"
OPT_DIR="/opt/js55"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== 1. Compilation du binaire Linux pur Go 1.27 statique (CGO_ENABLED=0) ==="
cd "${ROOT_DIR}"
GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/js55 ./cmd/js55
echo "✓ Binaire compilé avec succès : bin/js55 ($(du -h bin/js55 | cut -f1))"

echo "=== 2. Préparation du répertoire distant sur ${TARGET_HOST} ==="
ssh "${TARGET_HOST}" "sudo mkdir -p ${OPT_DIR} && sudo chown -R ubuntu:ubuntu ${OPT_DIR}"

echo "=== 3. Transfert du binaire statique et du service systemd ==="
scp "${ROOT_DIR}/bin/js55" "${TARGET_HOST}:/tmp/js55"
ssh "${TARGET_HOST}" "sudo mv /tmp/js55 ${OPT_DIR}/js55 && sudo chmod 0755 ${OPT_DIR}/js55 && sudo chown root:root ${OPT_DIR}/js55"

scp "${SCRIPT_DIR}/js55.service" "${TARGET_HOST}:/tmp/js55.service"
ssh "${TARGET_HOST}" "sudo mv /tmp/js55.service /etc/systemd/system/js55.service && sudo systemctl daemon-reload"

echo "=== 4. Configuration Nginx pour js55.hazyhaar.fr ==="
scp "${SCRIPT_DIR}/nginx-js55.conf" "${TARGET_HOST}:/tmp/js55.conf"
ssh "${TARGET_HOST}" "sudo mv /tmp/js55.conf /etc/nginx/sites-available/js55.hazyhaar.fr && sudo ln -sf /etc/nginx/sites-available/js55.hazyhaar.fr /etc/nginx/sites-enabled/ && sudo nginx -t && sudo systemctl reload nginx"

echo "=== 5. Activation et démarrage de js55.service ==="
ssh "${TARGET_HOST}" "sudo systemctl enable --now js55.service && sudo systemctl restart js55.service"

echo "=== 6. Vérification du statut HTTP local sur .79 ==="
ssh "${TARGET_HOST}" "sleep 1 && curl -s http://127.0.0.1:8557/health" || true

echo ""
echo "=== Déploiement achevé avec succès sur ${TARGET_HOST} ==="
echo "Portail web et bac à sable js55 actifs en arrière-plan (127.0.0.1:8557)."
