---
title: "Get started with PrintMarshal"
description: "Install PrintMarshal with Docker, Docker Compose, Helm, or systemd. Establish read-only monitoring, access controls, and backups before device writes."
---

## Deploy on your terms

Choose Docker, Docker Compose, Helm, or a systemd-managed binary. Each runs the same application with persistent storage. Use one active server and place it where it can reach your authorized printer networks.

The commands below are copied from the [installation wiki](https://github.com/printmarshal/printmarshal/blob/main/wiki/Installation-and-deployment.md) and run from the repository root. Obtain the source from [GitHub](https://github.com/printmarshal/printmarshal). These are installation instructions, not commands run by this website.

For local builds, `go.mod` requires Go 1.27.3. Building the SPA also needs Node and npm; the container build uses Node 22. Docker builds those components inside its build stages.

## Docker

Build the image locally, then keep application data in a named volume. Replace `change-me` with a strong, unique initial password before running this example. The optional version tag in the build comment is illustrative, not a claim that a matching release image is published.

```bash
make docker                       # printmarshal:<git describe> and printmarshal:latest
# or: docker build --build-arg VERSION=v0.1.0 -t printmarshal:v0.1.0 .

docker run -d --name printmarshal --restart unless-stopped \
  --network host \
  -e PM_ADMIN_PASSWORD='change-me' \
  -v pm-data:/data \
  printmarshal:latest

docker logs -f printmarshal       # "http listening addr=:8080"
```

The image runs as a non-root user. Host networking exposes the application and trap listener on the host and provides access to multicast discovery. The defaults are HTTP on port 8080 and SNMP traps on UDP 1162. Plan host firewall rules and HTTPS access before exposing the service beyond an evaluation environment.

## Docker Compose

Compose builds the image, uses host networking, and keeps `/data` in a named volume. This example creates a protected environment file with generated bootstrap and encryption secrets; keep that file out of shared source and back it up securely.

```bash
cp deploy/printmarshal.example.json deploy/printmarshal.json      # optional, for notifiers etc.
printf 'PM_ADMIN_PASSWORD=%s\nPM_SECRET_KEY=%s\n' "$(openssl rand -base64 18)" "$(openssl rand -base64 32)" > deploy/printmarshal.env
chmod 600 deploy/printmarshal.env
docker compose -f deploy/docker-compose.yml up -d --build
docker compose -f deploy/docker-compose.yml logs -f
```

Copying the example JSON is optional and does not mount it by itself. To use it, enable the JSON volume mount and `PM_CONFIG` setting in the Compose file as described in the wiki. Keep the encryption key with your database backups.

## Helm

Build and publish an image to your own registry, choose an immutable tag, and label a suitable Kubernetes node for printer-network access. Replace the example registry and image version below; the selector requires the corresponding node label.

```bash
helm install printmarshal deploy/helm/printmarshal -n printmarshal --create-namespace \
  --set image.repository=registry.example.com/printmarshal --set image.tag=v0.1.0 \
  --set secrets.adminPassword="$(openssl rand -base64 18)" \
  --set 'nodeSelector.printmarshal\.io/printer-network=true'
```

The chart defaults to host networking, persistent storage, and a single active server. Configure ingress/TLS, storage, secrets, and node placement for your environment. The chart can retain its generated encryption secret across upgrades; do not replace that key without a recovery plan. Plain Kubernetes manifests are also included in the repository.

## systemd binary

Build the web assets before the server so the binary embeds the full interface. The example below installs the binary and supplied unit on a Linux host; edit the environment file before starting the service.

```bash
make web build                                   # or download a release binary
sudo install -m 0755 bin/printmarshal /usr/local/bin/printmarshal
sudo useradd --system --home-dir /var/lib/printmarshal --shell /usr/sbin/nologin printmarshal
sudo install -d -m 0750 -o root -g printmarshal /etc/printmarshal
sudo install -m 0640 -g printmarshal deploy/systemd/printmarshal.env.example /etc/printmarshal/printmarshal.env
sudo install -m 0644 deploy/systemd/printmarshal.service /etc/systemd/system/
sudoedit /etc/printmarshal/printmarshal.env      # PM_ADMIN_PASSWORD, PM_TRAP_LISTEN, PM_LOG_FORMAT, ...
sudo systemctl daemon-reload && sudo systemctl enable --now printmarshal
journalctl -u printmarshal -f
```

The unit runs under the `printmarshal` account and uses `/var/lib/printmarshal` for persistent data. It supplies process hardening and the capability needed to bind a privileged trap port when configured. Review the environment file and network access for the host you are deploying.

## First run

1. **Open the interface.** The default local address is `http://localhost:8080`. For remote access, terminate HTTPS at a reverse proxy or ingress and set `PM_SECURE_COOKIES=true`.
2. **Sign in as `admin`.** Use the password supplied through `PM_ADMIN_PASSWORD`. If it was omitted, read `<dataDir>/initial-admin-password`; in the container that is `/data/initial-admin-password`.
3. **Change the initial password.** Delete the generated password file if present. `PM_ADMIN_PASSWORD` only bootstraps an empty database; changing it later does not reset the account.
4. **Add read credentials.** Use the least privilege the devices need. Keep write credentials separate and bind them only when you intend to use supported write operations.
5. **Run discovery.** Choose an authorized target range or multicast discovery and inspect the resulting identity, capabilities, and poll errors. Scheduled discovery is off by default.
6. **Establish operations.** Configure alerts, access roles, retention, and backups. Test writes in the simulator before expanding to authorized hardware.

Multicast discovery requires suitable network access; active target scanning uses SNMP and needs matching credentials. Traps must reach the configured UDP listener with the original device source address intact. See the wiki before substituting bridge networking for the supplied host-network examples.

## Preserve state before upgrading

Back up the database, encryption key, firmware files, configuration, and externally supplied secrets. Preserve generated SAML files separately when that provider is enabled. The supplied backup helper targets the default SQLite database and does not capture every deployment secret or custom database path.

Migrations run at startup. Ordinary interrupted jobs are marked failed and require resubmission; firmware campaigns have a separate paused recovery flow. PostgreSQL changes the storage backend, not the single-active-server model.

## Keep the guide close

- [Administrator and developer wiki](https://github.com/printmarshal/printmarshal/blob/main/wiki/Home.md)
- [Installation, networking, backups, and upgrades](https://github.com/printmarshal/printmarshal/blob/main/wiki/Installation-and-deployment.md)
- [Complete configuration reference](https://github.com/printmarshal/printmarshal/blob/main/wiki/Configuration-reference.md)
- [Identity and access](https://github.com/printmarshal/printmarshal/blob/main/wiki/Identity-and-access.md)
- [Troubleshooting](https://github.com/printmarshal/printmarshal/blob/main/wiki/Troubleshooting-and-FAQ.md)
- [Simulator and driver development](https://github.com/printmarshal/printmarshal/blob/main/wiki/Vendor-plugins-and-drivers.md)
