# livestream-viewer Home Assistant Add-on

Native hardware-accelerated livestream viewer and video wall. Use **Open Web UI** to manage sources, screens, and the tour. The REST API and embedded web UI run via Home Assistant ingress.

This scaffold boots the control plane (API + UI). Display output on an attached panel lands in a later stage. The add-on runs on Home Assistant OS hosts generally; low-cost ARM boards (including Raspberry Pi) are common deployment targets and are optimised for once the display engine lands.

## Installation

1. In Home Assistant, go to **Settings** → **Add-ons** → **Add-on store**.
2. Click the three dots (⋮) → **Repositories**.
3. Add this repository URL: `https://github.com/dchote/livestream-viewer`
4. Find **livestream-viewer** in the add-on list and click **Install**.
5. Configure options if needed, then **Start** the add-on.

## Configuration

- **HTTP port** — Management UI and API (default 8099). Used by ingress.
- **Log level** — `debug`, `info`, `warn`, or `error`.
- **Display engine** — Start the display engine. Requires `/dev/dri` when driving a panel. Leave enabled on a host with an attached display; disable for control-plane-only use.

Data (database, uploads, JWT secret) is stored in the add-on’s persistent `/data` directory.

Default login after first start: username `admin`, password `admin`. Change this before exposing the UI.

## Web UI (ingress)

Click **Open Web UI** in the add-on panel. Preview, Stream Sources, and Display Strategy are the primary pages.

Swagger is at `/docs` on the add-on port.

## Local image build

From the repository root (not required for Supervisor installs that pull GHCR):

```bash
docker build -f addon/Dockerfile -t livestream-viewer-addon .
docker run --rm -p 8099:8099 livestream-viewer-addon
```

## Forks

If you install from a fork, update `image` in `addon/config.yaml` to your registry, or build and push your own images.

## Support

- [GitHub repository](https://github.com/dchote/livestream-viewer)
