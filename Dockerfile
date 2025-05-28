# Stage 1: Build Go app
# =====================
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install git and deps
RUN apk add --no-cache git

COPY go.mod go.sum main.go ./

RUN go mod download
RUN go build -o server .

# Stage 2: Runtime with LaTeX
# ===========================
FROM debian:bullseye-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y \
    ffmpeg \
    ca-certificates \
    texlive-latex-base \
    texlive-latex-recommended \
    texlive-latex-extra \
    texlive-fonts-recommended \
    texlive-fonts-extra \
    texlive-lang-english \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

WORKDIR /app

RUN mkdir -p /app/tmp

VOLUME /app/tmp

COPY --from=builder /app/server .
COPY ./static /app/static

EXPOSE 8080

CMD ["./server"]

