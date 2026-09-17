# --- build stage ---
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/server ./cmd/server

# --- runtime stage ---
FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates
WORKDIR /app
COPY --from=build /out/server ./server
ENV ADDR=:8080
ENV DATA_DIR=/app/data
ENV FFMPEG_PATH=ffmpeg
ENV CAMERAS_CONFIG=/app/cameras.json
RUN mkdir -p /app/data
EXPOSE 8080
ENTRYPOINT ["./server"]
