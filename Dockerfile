FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o rls .

FROM alpine:latest
COPY --from=builder /app/rls /usr/local/bin/rls
EXPOSE 8080
ENTRYPOINT ["rls"]
