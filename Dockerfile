FROM golang:1.21-alpine

# 1. Install system dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /app

# 2. Force-initialize the module and dependencies inside the container
COPY . .
RUN go mod tidy
RUN go mod download

# 3. Build with static linking (Standard for Northflank)
RUN CGO_ENABLED=0 GOOS=linux go build -o /lattice_engine .

# 4. Final settings
EXPOSE 4021
CMD ["/lattice_engine"]
