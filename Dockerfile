# Specifies a parent image
FROM golang:1.25-bookworm

# Creates an app directory to hold your app’s source code
WORKDIR /app

# Copies everything from your root directory into /app
COPY . .

# Installs Go dependencies
RUN go mod download

# Builds your app with optional configuration
RUN go build -o /app/main /app/cmd/server/main.go

# Bind all interfaces inside the container (the default "localhost" binds
# loopback only, making the published port unreachable).
ENV INDRI_LISTEN_ADDRESS=0.0.0.0

# Tells Docker which network port your container listens on (INDRI_LISTEN_PORT)
EXPOSE 5002

# Specifies the executable command that runs when the container starts
CMD ["/app/main"]
