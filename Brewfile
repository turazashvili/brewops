# BrewOps — Brewfile (it's a Dockerfile, but for coffee)
#
# "There is coffee all over the world. Increasingly, in a world
#  in which computing is ubiquitous, the computists want to make
#  coffee." — RFC 2324, Section 1
#
# Build:  docker build -f Brewfile -t brewops .
# Run:    docker run -p 8418:8418 brewops
# Taste:  curl -X BREW http://localhost:8418/pot -d 'start'

# Stage 1: The Barista grinds the beans (compiles Go binaries)
FROM golang:1.24-alpine AS barista

WORKDIR /coffeeshop

# Import the bean inventory (dependencies)
# Pure stdlib, zero external deps. The way coffee should be: no additives.
COPY go.mod ./

# Pour in the fresh grounds (source code)
COPY cmd/ cmd/
COPY internal/ internal/

# Brew the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /brewopsd ./cmd/brewopsd

# Froth the CLI tool
RUN CGO_ENABLED=0 GOOS=linux go build -o /brew-ctl ./cmd/brew-ctl

# Stage 2: Serve it up (minimal runtime image)
FROM alpine:latest AS serving

# A coffee pot needs a timezone to know when it's "the morning rush"
RUN apk add --no-cache tzdata ca-certificates

# Install the brewed binaries
COPY --from=barista /brewopsd /usr/local/bin/
COPY --from=barista /brew-ctl /usr/local/bin/

# Set up the dashboard
COPY web/ /usr/share/brewops/web/

# The HTCPCP port: 8418 (a nod to HTTP 418)
EXPOSE 8418

# Environment: always production. We don't do staging for coffee.
ENV PORT=8418

# Start the coffee daemon
ENTRYPOINT ["brewopsd"]

# Serving temperature: hot.
# Serving size: one container.
# Handle with caution. (Safe: if-user-awake)
