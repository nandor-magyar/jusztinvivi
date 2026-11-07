# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM alpine:3.24 AS hugo
ARG HUGO_VERSION=0.168.0
ARG BUILDARCH
ARG VERSION=dev
RUN apk add --no-cache gzip \
 && cd /tmp \
 && wget -q https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_${HUGO_VERSION}_linux-${BUILDARCH}.tar.gz \
 && wget -q https://github.com/gohugoio/hugo/releases/download/v${HUGO_VERSION}/hugo_${HUGO_VERSION}_checksums.txt \
 && grep " hugo_${HUGO_VERSION}_linux-${BUILDARCH}.tar.gz$" hugo_${HUGO_VERSION}_checksums.txt | sha256sum -c - \
 && tar -xzf hugo_${HUGO_VERSION}_linux-${BUILDARCH}.tar.gz -C /usr/local/bin hugo \
 && rm -f /tmp/hugo_*
WORKDIR /src
COPY site/ ./site/
RUN hugo --minify --gc -s site -d ../public \
 && echo "${VERSION}" > public/version.txt \
 && find public -type f \( -name '*.html' -o -name '*.css' -o -name '*.js' -o -name '*.xml' -o -name '*.txt' -o -name '*.svg' -o -name '*.json' \) \
      -size +1k -exec gzip -k -9 -n {} +

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS go
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod main.go ./
COPY --from=hugo /src/public ./public
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /jusztinvivien .

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
LABEL org.opencontainers.image.title="jusztinvivien" \
      org.opencontainers.image.source="https://github.com/nandor-magyar/jusztinvivi" \
      org.opencontainers.image.version="${VERSION}"
COPY --from=go /jusztinvivien /jusztinvivien
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s CMD ["/jusztinvivien", "-healthcheck"]
ENTRYPOINT ["/jusztinvivien"]
