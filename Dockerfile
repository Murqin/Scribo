# syntax=docker/dockerfile:1

# The build stage always runs on the runner's native architecture and Go
# cross-compiles to TARGETARCH. Emulating a whole Go toolchain under QEMU to
# produce an arm64 binary would cost minutes per image for no gain, because
# CGO_ENABLED=0 means nothing in this build needs the target's libc.
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build

WORKDIR /src

# Dependencies resolve from go.mod/go.sum alone, so this layer survives every
# source edit.
COPY go.mod go.sum ./
RUN go mod download

# Copied path by path rather than with `COPY . .`: the build context of a
# working checkout holds a real .env, compiled binaries and dist/, and none of
# them belong in an image layer. .dockerignore repeats this as a second line of
# defence.
COPY main.go ./
COPY bot/ bot/
COPY budget/ budget/
COPY config/ config/
COPY history/ history/
COPY i18n/ i18n/
COPY mode/ mode/
COPY provider/ provider/

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags "-s -w" -o /out/scribo main.go

FROM alpine:3.20

# ca-certificates: every call to Telegram, Google and OpenRouter is HTTPS, and a
#   static Go binary carries no trust store of its own.
# tzdata: the spending ceiling rolls over on local calendar day and month
#   boundaries, so an unset zone would reset the daily cap at UTC midnight
#   instead of the operator's.
# su-exec: lets the entrypoint drop root after fixing up the volume.
RUN apk add --no-cache ca-certificates tzdata su-exec

COPY --from=build /out/scribo /usr/local/bin/scribo
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh

# /data is the working directory, not just a mount point: the bot resolves
# modes.json and the default HISTORY_FILE relative to the process's cwd, so
# putting cwd on the volume is what makes both of them persist.
RUN mkdir -p /data
WORKDIR /data
VOLUME ["/data"]

ENV TZ=UTC \
    PUID=1000 \
    PGID=1000

# Scribo reaches Telegram by outbound long polling, so the container needs no
# published port and no reverse proxy. Deliberately no EXPOSE.

# Shallow by design: with no listening socket there is nothing to probe, and the
# bot exits on any unrecoverable error rather than lingering in a broken state.
# This only distinguishes "process gone but container still up" from healthy.
HEALTHCHECK --interval=1m --timeout=5s --start-period=10s --retries=3 \
    CMD pgrep -x scribo >/dev/null || exit 1

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["scribo"]
