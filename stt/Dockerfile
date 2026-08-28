# syntax=docker/dockerfile:1

# --- Go service build -------------------------------------------------------
FROM golang:1.26.7-bookworm AS go-build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/stt ./cmd/stt

# --- whisper.cpp build (CPU only) -----------------------------------------
FROM debian:bookworm-slim AS whisper-build
ARG WHISPER_CPP_VERSION=v1.9.1
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates cmake git build-essential \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth 1 --branch "${WHISPER_CPP_VERSION}" https://github.com/ggml-org/whisper.cpp.git
WORKDIR /src/whisper.cpp
# GGML_NATIVE=OFF keeps the binary portable across Railway build/run hosts.
RUN cmake -S . -B build \
      -DCMAKE_BUILD_TYPE=Release \
      -DBUILD_SHARED_LIBS=OFF \
      -DGGML_NATIVE=OFF \
      -DWHISPER_BUILD_TESTS=OFF \
      -DWHISPER_BUILD_EXAMPLES=ON \
    && cmake --build build --config Release --target whisper-cli -j"$(nproc)"

# --- Runtime -------------------------------------------------------------
FROM debian:bookworm-slim
ARG MODEL_URL=https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo-q5_0.bin
ARG MODEL_SHA256=394221709cd5ad1f40c46e6031ca61bce88931e6e088c188294c6d5a55ffa7e2
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates curl ffmpeg tini \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --system stt \
    && useradd --system --gid stt --home-dir /nonexistent --shell /usr/sbin/nologin stt \
    && install -d -o stt -g stt /data/inbox /data/work /data/output /models
COPY --from=go-build /out/stt /usr/local/bin/stt
COPY --from=whisper-build /src/whisper.cpp/build/bin/whisper-cli /usr/local/bin/whisper-cli
RUN curl --fail --location --retry 4 --output /models/ggml-large-v3-turbo-q5_0.bin "${MODEL_URL}" \
    && echo "${MODEL_SHA256}  /models/ggml-large-v3-turbo-q5_0.bin" | sha256sum --check \
    && chown stt:stt /models/ggml-large-v3-turbo-q5_0.bin

ENV STT_DATA_DIR=/data \
    STT_MODEL_PATH=/models/ggml-large-v3-turbo-q5_0.bin \
    STT_MODEL_NAME=large-v3-turbo-q5_0 \
    STT_MAX_WORKERS=1 \
    STT_INITIAL_WORKERS=1 \
    STT_THREADS=4 \
    STT_MIN_FREE_BYTES=536870912
USER stt
EXPOSE 8000
HEALTHCHECK --interval=20s --timeout=5s --start-period=30s --retries=3 \
  CMD curl --fail --silent "http://127.0.0.1:${PORT:-8000}/healthz" >/dev/null || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/stt"]
