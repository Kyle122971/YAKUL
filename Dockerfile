FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
# Static build
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o engine .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/engine .

# THE OOM FIX: Tell Go to be aggressive with memory
ENV GOGC=50 
ENV GOMEMLIMIT=7GiB

EXPOSE 4021
CMD ["./engine"]
