# Use a specific version of golang for reproducible builds
FROM harbor.yf-online.service.163.org/golang/golang:1.15.3 AS builder

# Set the working directory inside the container
WORKDIR /horizon

# Use Go proxy in China
ENV GOPROXY=https://goproxy.cn,direct

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/app -ldflags '-s -w' ./core/main.go

# Use a minimal base image for the runtime
FROM harbor.yf-online.service.163.org/alpine/alpine:3.9 AS runtime

# Update the package repository and install necessary packages
RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories && \
    apk update && \
    apk add --no-cache bash curl git tzdata && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# Define build arguments
ARG GROUP=netease
ARG USER=appops
ARG GROUP_ID=10001
ARG USER_ID=10001

# Create a non-root user
RUN addgroup --gid $GROUP_ID $GROUP && \
    adduser -h /home/$USER -u $USER_ID -G $GROUP -D $USER

# Configure the name service switch
RUN echo "hosts: files dns" > /etc/nsswitch.conf

# Switch to the non-root user
USER $USER

# Copy the built application from the builder stage
COPY --from=builder --chown=$USER:$GROUP /horizon/bin/app /usr/local/bin/app

# Set the entrypoint
ENTRYPOINT ["/usr/local/bin/app"]