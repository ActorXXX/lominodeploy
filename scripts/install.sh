#!/usr/bin/env bash
# LominoDeploy — Script de instalación
# Uso: curl -sSL https://license.lominodev.com/binaries/install.sh | bash

set -euo pipefail

LOMINODEV_URL="https://license.lominodev.com"
INSTALL_PATH="/usr/local/bin/lominodeploy"
SERVICE_NAME="lominodeploy"
CONFIG_DIR="/etc/lominodeploy"
PORT=8888

RED='\033[0;31m'
GRN='\033[0;32m'
YLW='\033[1;33m'
BLU='\033[0;34m'
RST='\033[0m'
BOLD='\033[1m'

info()    { echo -e "${BLU}[INFO]${RST} $*"; }
success() { echo -e "${GRN}[✓]${RST} $*"; }
warn()    { echo -e "${YLW}[AVISO]${RST} $*"; }
error()   { echo -e "${RED}[ERROR]${RST} $*" >&2; exit 1; }

echo -e "\n${BOLD}  LominoDeploy — Instalador${RST}"
echo -e "  Agente de despliegue y gestión LominoDev\n"

# ── Verificar root ───────────────────────────────────────────────────────────
if [ "$(id -u)" -ne 0 ]; then
    error "Este script debe ejecutarse como root o con sudo."
fi

# ── Detectar arquitectura ────────────────────────────────────────────────────
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    arm64)   GOARCH="arm64" ;;
    *)       error "Arquitectura no soportada: $ARCH" ;;
esac
info "Arquitectura detectada: ${ARCH} → ${GOARCH}"

# ── Detectar OS ──────────────────────────────────────────────────────────────
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_NAME="${PRETTY_NAME:-Linux}"
else
    OS_NAME="Linux"
fi
info "Sistema operativo: ${OS_NAME}"

# ── Verificar conectividad ───────────────────────────────────────────────────
info "Verificando conectividad con LominoLLS..."
if ! curl -sf --max-time 10 "${LOMINODEV_URL}/binaries/version.json" > /dev/null; then
    error "No se puede conectar a ${LOMINODEV_URL}. Verifica tu conexión a internet."
fi
success "Conectividad OK"

# ── Obtener versión más reciente ─────────────────────────────────────────────
info "Obteniendo información de versión..."
VERSION_JSON=$(curl -sf --max-time 15 "${LOMINODEV_URL}/binaries/version.json")
LATEST_VERSION=$(echo "$VERSION_JSON" | grep -o '"version":"[^"]*"' | cut -d'"' -f4)
EXPECTED_SHA256=$(echo "$VERSION_JSON" | grep -o "\"linux_${GOARCH}\":\"[^\"]*\"" | cut -d'"' -f4)

if [ -z "$LATEST_VERSION" ]; then
    error "No se pudo determinar la versión más reciente."
fi
info "Versión disponible: ${LATEST_VERSION}"

# ── Verificar si ya está instalado ──────────────────────────────────────────
if [ -f "$INSTALL_PATH" ]; then
    CURRENT_VERSION=$("$INSTALL_PATH" --version 2>/dev/null || echo "desconocida")
    if [ "$CURRENT_VERSION" = "$LATEST_VERSION" ]; then
        warn "LominoDeploy ${LATEST_VERSION} ya está instalado y actualizado."
        systemctl status "$SERVICE_NAME" --no-pager -l 2>/dev/null || true
        exit 0
    fi
    warn "Actualizando de ${CURRENT_VERSION} a ${LATEST_VERSION}..."
fi

# ── Descargar binario ────────────────────────────────────────────────────────
BINARY_URL="${LOMINODEV_URL}/binaries/lominodeploy-linux-${GOARCH}"
TMP_BIN=$(mktemp /tmp/lominodeploy.XXXXXX)
trap "rm -f $TMP_BIN" EXIT

info "Descargando lominodeploy-linux-${GOARCH}..."
if ! curl -fL --max-time 120 --progress-bar "${BINARY_URL}" -o "$TMP_BIN"; then
    error "No se pudo descargar el binario desde ${BINARY_URL}"
fi

# ── Verificar SHA256 ─────────────────────────────────────────────────────────
if [ -n "$EXPECTED_SHA256" ]; then
    info "Verificando integridad del binario..."
    ACTUAL_SHA256=$(sha256sum "$TMP_BIN" | cut -d' ' -f1)
    if [ "$ACTUAL_SHA256" != "$EXPECTED_SHA256" ]; then
        error "Verificación SHA256 fallida. El binario puede estar corrupto.\n  Esperado: ${EXPECTED_SHA256}\n  Obtenido: ${ACTUAL_SHA256}"
    fi
    success "Verificación SHA256 correcta"
else
    warn "No se pudo verificar SHA256 (no disponible para esta arquitectura)"
fi

# ── Instalar binario ─────────────────────────────────────────────────────────
chmod +x "$TMP_BIN"
mv "$TMP_BIN" "$INSTALL_PATH"
success "Binario instalado en ${INSTALL_PATH}"

# ── Crear directorio de configuración ───────────────────────────────────────
mkdir -p "$CONFIG_DIR"
chmod 755 "$CONFIG_DIR"

# ── Detectar IP del servidor ─────────────────────────────────────────────────
PUBLIC_IP=$(curl -sf --max-time 5 https://ifconfig.me 2>/dev/null || echo "")
LOCAL_IP=$(ip route get 1 2>/dev/null | awk '{print $7; exit}' || hostname -I 2>/dev/null | awk '{print $1}' || echo "")

if [ -n "$PUBLIC_IP" ] && [ -n "$LOCAL_IP" ] && [ "$PUBLIC_IP" != "$LOCAL_IP" ]; then
    ACCESS_IP="${PUBLIC_IP} (pública) / ${LOCAL_IP} (local)"
    PRIMARY_IP="${PUBLIC_IP}"
elif [ -n "$PUBLIC_IP" ]; then
    ACCESS_IP="${PUBLIC_IP}"
    PRIMARY_IP="${PUBLIC_IP}"
else
    ACCESS_IP="${LOCAL_IP}"
    PRIMARY_IP="${LOCAL_IP}"
fi

# ── Crear servicio systemd ───────────────────────────────────────────────────
info "Creando servicio systemd..."
cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=LominoDeploy — Agente de despliegue LominoDev
Documentation=https://lominodev.com/docs/lominodeploy
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_PATH}
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=lominodeploy

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME" --quiet
systemctl restart "$SERVICE_NAME"

sleep 2

if systemctl is-active --quiet "$SERVICE_NAME"; then
    success "Servicio ${SERVICE_NAME} activo"
else
    warn "El servicio puede estar iniciando. Verifica con: systemctl status ${SERVICE_NAME}"
fi

# ── Configurar firewall (si ufw está activo) ─────────────────────────────────
if command -v ufw &>/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
    ufw allow ${PORT}/tcp --comment "LominoDeploy" > /dev/null 2>&1 || true
    info "Regla de firewall agregada para puerto ${PORT}"
fi

# ── Mensaje final ─────────────────────────────────────────────────────────────
echo -e "\n${GRN}${BOLD}  ✅ LominoDeploy ${LATEST_VERSION} instalado correctamente${RST}\n"
echo -e "  ${BOLD}Panel de gestión disponible en:${RST}"
echo -e "  → ${BLU}http://${PRIMARY_IP}:${PORT}${RST}\n"

if [ -n "$PUBLIC_IP" ] && [ -n "$LOCAL_IP" ] && [ "$PUBLIC_IP" != "$LOCAL_IP" ]; then
    echo -e "  IP pública:   ${PUBLIC_IP}"
    echo -e "  IP local:     ${LOCAL_IP}"
fi

echo -e "  ${YLW}Abre la URL en tu navegador para comenzar la configuración.${RST}"
echo -e "\n  Comandos útiles:"
echo -e "    systemctl status ${SERVICE_NAME}   # estado del servicio"
echo -e "    journalctl -u ${SERVICE_NAME} -f   # logs en tiempo real"
echo -e "    systemctl restart ${SERVICE_NAME}  # reiniciar\n"
