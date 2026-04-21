#!/usr/bin/env bash
set -euo pipefail

echo "== Android overlay patch =="

# ======================
# Настройки
# ======================
APP_ID="${APP_ID:-com.example.myfork}"

ANDROID_DIR="clients/android"
OVERLAY_DIR="${OVERLAY_DIR:-patches/hungcabinet/android/overlay}"

echo "APP_ID=$APP_ID"
echo "Overlay source: $OVERLAY_DIR"
echo "Target: $ANDROID_DIR"

# ======================
# 1. Проверка наличия overlay (падаем, если нет)
# ======================

if [ ! -d "$OVERLAY_DIR" ]; then
  echo "❌ ERROR: Overlay directory not found!"
  echo "   Expected path: $OVERLAY_DIR"
  echo "   Please create the overlay directory with your custom files."
  exit 1
fi

echo "Overlay directory found. Applying..."

# ======================
# 2. Применяем overlay (копируем файлы поверх)
# ======================

cp -a "$OVERLAY_DIR/." "$ANDROID_DIR/"

echo "✅ Overlay successfully applied!"

# ======================
# 3. Принудительно устанавливаем applicationId (на всякий случай)
# ======================

BUILD_KTS="$ANDROID_DIR/app/build.gradle.kts"

if [ -f "$BUILD_KTS" ]; then
  echo "Ensuring correct applicationId in build.gradle.kts..."
  sed -i -E \
    "s/applicationId[[:space:]]*=[[:space:]]*\"[^\"]+\"/applicationId = \"$APP_ID\"/g" \
    "$BUILD_KTS"
fi

echo "== Done =="