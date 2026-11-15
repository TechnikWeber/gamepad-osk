#!/usr/bin/env bash
# Builds gamepad-osk with its own SDL3 and SDL3_ttf for systems that do not
# package SDL3 (Ubuntu 24.04, Debian 12, Linux Mint 22 ...).
#
# Runs inside a debian:12 container, so the result needs only glibc 2.36
# and the usual desktop libraries (wayland-client, libX11):
#
#   podman run --rm --security-opt label=disable -v "$PWD":/src -v "$(go env GOROOT)":/go:ro \
#       debian:12 /src/packaging/build-portable.sh
#
# Output: dist/gamepad-osk-portable-x86_64.tar.gz
#   gamepad-osk            binary, finds its libraries via RPATH
#   lib/libSDL3.so.0       SDL3 (SDL_VERSION)
#   lib/libSDL3_ttf.so.0   SDL3_ttf (uses the system FreeType and HarfBuzz)
set -euo pipefail

SDL_VERSION=3.4.20
TTF_VERSION=3.2.2
export DEBIAN_FRONTEND=noninteractive GOTOOLCHAIN=local
# Go: from PATH (CI: actions/setup-go) or mounted at /go (podman, see above)
command -v go >/dev/null || export PATH=/go/bin:$PATH GOROOT=/go
SRC=${SRC:-/src}
export GOFLAGS=-buildvcs=false GOCACHE=/tmp/gocache GOPATH=/tmp/gopath

apt-get update -qq
apt-get install -y -qq --no-install-recommends build-essential cmake ninja-build pkg-config \
    curl ca-certificates file libwayland-dev wayland-protocols libxkbcommon-dev libegl-dev \
    libgl-dev libgles-dev libx11-dev libxext-dev libxrandr-dev libxcursor-dev libxi-dev \
    libxfixes-dev libxss-dev libxtst-dev libdrm-dev libgbm-dev libdecor-0-dev libdbus-1-dev \
    libudev-dev libpipewire-0.3-dev libpulse-dev libfreetype-dev libharfbuzz-dev >/dev/null

work=/tmp/build
prefix=/opt/sdl3
mkdir -p "$work"
cd "$work"
curl -fsSL "https://github.com/libsdl-org/SDL/releases/download/release-$SDL_VERSION/SDL3-$SDL_VERSION.tar.gz" | tar xz
curl -fsSL "https://github.com/libsdl-org/SDL_ttf/releases/download/release-$TTF_VERSION/SDL3_ttf-$TTF_VERSION.tar.gz" | tar xz

cmake -S "SDL3-$SDL_VERSION" -B sdl-build -G Ninja -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX=$prefix -DSDL_SHARED=ON -DSDL_STATIC=OFF -DSDL_TESTS=OFF -DSDL_EXAMPLES=OFF
cmake --build sdl-build
cmake --install sdl-build >/dev/null

cmake -S "SDL3_ttf-$TTF_VERSION" -B ttf-build -G Ninja -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX=$prefix -DCMAKE_PREFIX_PATH=$prefix -DSDLTTF_VENDORED=OFF \
    -DSDLTTF_HARFBUZZ=ON -DSDLTTF_PLUTOSVG=OFF -DSDLTTF_SAMPLES=OFF
cmake --build ttf-build
cmake --install ttf-build >/dev/null

out=/tmp/out/gamepad-osk
mkdir -p "$out/lib"
cp -L $prefix/lib/libSDL3.so.0 $prefix/lib/libSDL3_ttf.so.0 "$out/lib/"
strip --strip-unneeded "$out"/lib/*.so.0

cd "$SRC"
PKG_CONFIG_PATH=$prefix/lib/pkgconfig \
CGO_LDFLAGS='-Wl,-rpath,$ORIGIN/lib:$ORIGIN/../lib/gamepad-osk' \
    go build -trimpath -ldflags "-s -w" -o "$out/gamepad-osk" .

cp LICENSE "$out/"
mkdir -p "$SRC/dist"
tar -C /tmp/out -czf "$SRC/dist/gamepad-osk-portable-x86_64.tar.gz" gamepad-osk
chown -R "$(stat -c %u:%g "$SRC")" "$SRC/dist"
echo "Built dist/gamepad-osk-portable-x86_64.tar.gz"
"$out/gamepad-osk" --version
