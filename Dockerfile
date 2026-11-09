FROM node:24-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS server
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /lumen ./cmd/lumen

FROM alpine:3.22
RUN apk add --no-cache ffmpeg ca-certificates tzdata
COPY --from=server /lumen /usr/local/bin/lumen
ENV LUMEN_DATA_DIR=/config LUMEN_ADDR=:8096
VOLUME ["/config"]
EXPOSE 8096
ENTRYPOINT ["/usr/local/bin/lumen"]
