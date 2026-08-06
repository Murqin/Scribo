#!/bin/sh
# Reconcile the volume's ownership with the user the bot will run as, then drop
# privileges.
#
# A fixed image UID is not enough for the platforms this image targets. CasaOS,
# Cosmos Cloud and Portainer all bind-mount a host directory onto /data, and
# that directory belongs to root or to the panel's own account — the bot would
# start, fail to write modes.json and the history file, and look broken. PUID and
# PGID are the convention homelab users already expect for exactly this.
set -e

PUID="${PUID:-1000}"
PGID="${PGID:-1000}"

if [ "$(id -u)" = "0" ]; then
    # Only touch the volume when the ownership actually differs: on a large
    # history file a recursive chown at every restart is pure I/O for nothing.
    if [ "$(stat -c %u /data)" != "$PUID" ] || [ "$(stat -c %g /data)" != "$PGID" ]; then
        chown -R "$PUID:$PGID" /data
    fi

    # Numeric ids are handed to su-exec directly, so the image needs no account
    # for them and PUID can be any value the host uses.
    exec su-exec "$PUID:$PGID" "$@"
fi

# Already non-root: the platform pinned a user (compose `user:`, Podman's
# --userns, Kubernetes runAsUser) and that choice outranks PUID. Nothing can be
# chowned from here, so a wrong host ownership is reported rather than hidden
# behind whatever error the first write happens to produce.
if [ ! -w /data ]; then
    echo "scribo: /data is not writable by uid $(id -u) — transcripts and modes.json cannot be saved." >&2
    echo "scribo: chown the mounted directory to that uid on the host, or drop the container's user override." >&2
fi

exec "$@"
