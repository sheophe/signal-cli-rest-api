ARG LIBSIGNAL_CLIENT_VERSION=0.32.1
ARG SIGNAL_CLI_VERSION=0.12.4
ARG SWAG_VERSION=1.16.2

ARG BUILD_VERSION_ARG=unset

FROM golang:1.21-bookworm AS buildcontainer

ARG LIBSIGNAL_CLIENT_VERSION
ARG SIGNAL_CLI_VERSION
ARG SWAG_VERSION
ARG BUILD_VERSION_ARG

COPY ext/libraries/libsignal-client/v${LIBSIGNAL_CLIENT_VERSION} /tmp/libsignal-client-libraries
COPY ext/libraries/libsignal-client/signal-cli-native.patch /tmp/signal-cli-native.patch

# use architecture specific libsignal_jni.so
RUN arch="$(uname -m)"; \
	case "$arch" in \
	aarch64) cp /tmp/libsignal-client-libraries/arm64/libsignal_jni.so /tmp/libsignal_jni.so ;; \
	armv7l) cp /tmp/libsignal-client-libraries/armv7/libsignal_jni.so /tmp/libsignal_jni.so ;; \
	x86_64) cp /tmp/libsignal-client-libraries/x86-64/libsignal_jni.so /tmp/libsignal_jni.so ;; \
	*) echo "Unknown architecture" && exit 1 ;; \
	esac;

RUN dpkg-reconfigure debconf --frontend=noninteractive \
	&& apt-get -qq update \
	&& apt-get -qqy install --no-install-recommends \
	wget software-properties-common git locales zip unzip \
	file build-essential libz-dev zlib1g-dev < /dev/null > /dev/null \
	&& rm -rf /var/lib/apt/lists/* 

RUN export ARCH=$(uname -m | sed -u 's/arm64/aarch64/g;s/x86_64/x64/g;s/amd64/x64/g') \
	&& echo $ARCH \
	&& cd /tmp/ \
	&& wget https://download.oracle.com/java/21/latest/jdk-21_linux-$(echo $ARCH)_bin.tar.gz \
	&& tar zxvf jdk-21_linux-$(echo $ARCH)_bin.tar.gz

ENV PATH="${PATH}:/tmp/jdk-21.0.1/bin"

RUN sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && \
	dpkg-reconfigure --frontend=noninteractive locales && \
	update-locale LANG=en_US.UTF-8

ENV JAVA_OPTS="-Djdk.lang.Process.launchMechanism=vfork"

ENV LANG en_US.UTF-8

RUN cd /tmp/ \
	&& git clone https://github.com/swaggo/swag.git swag-${SWAG_VERSION} \	
	&& cd swag-${SWAG_VERSION} \
	&& git checkout -q v${SWAG_VERSION} \
	&& make build -s < /dev/null > /dev/null \
	&& cp /tmp/swag-${SWAG_VERSION}/swag /usr/bin/swag \
	&& rm -r /tmp/swag-${SWAG_VERSION}

RUN cd /tmp/ \
	&& git clone https://github.com/sheophe/signal-cli.git signal-cli \	
	&& cd signal-cli \
	&& git checkout master \
	&& ./gradlew build \
	&& tar xf build/distributions/signal-cli-${SIGNAL_CLI_VERSION}.tar

# replace libsignal-client

RUN ls /tmp/signal-cli/signal-cli-${SIGNAL_CLI_VERSION}/lib/libsignal-client-${LIBSIGNAL_CLIENT_VERSION}.jar || (echo "\n\nsignal-client jar file with version ${LIBSIGNAL_CLIENT_VERSION} not found. Maybe the version needs to be bumped in the signal-cli-rest-api Dockerfile?\n\n" && echo "Available version: \n" && ls /tmp/signal-cli-${SIGNAL_CLI_VERSION}/lib/libsignal-client-* && echo "\n\n" && exit 1)

RUN cd /tmp/ \
	&& zip -u /tmp/signal-cli/signal-cli-${SIGNAL_CLI_VERSION}/lib/libsignal-client-${LIBSIGNAL_CLIENT_VERSION}.jar libsignal_jni.so \
	&& zip -r signal-cli.zip signal-cli/signal-cli-${SIGNAL_CLI_VERSION}/* \
	&& unzip -q /tmp/signal-cli.zip -d /opt \
	&& rm -f /tmp/signal-cli.zip

COPY src/api /tmp/signal-cli-rest-api-src/api
COPY src/client /tmp/signal-cli-rest-api-src/client
COPY src/utils /tmp/signal-cli-rest-api-src/utils
COPY src/scripts /tmp/signal-cli-rest-api-src/scripts
COPY src/main.go /tmp/signal-cli-rest-api-src/
COPY src/go.mod /tmp/signal-cli-rest-api-src/
COPY src/go.sum /tmp/signal-cli-rest-api-src/

# build signal-cli-rest-api
RUN cd /tmp/signal-cli-rest-api-src && swag init && go test ./client -v && go build

# build supervisorctl_config_creator
RUN cd /tmp/signal-cli-rest-api-src/scripts && go build -o jsonrpc2-helper 

# Start a fresh container for release container
FROM eclipse-temurin:21.0.1_12-jre-jammy

ENV GIN_MODE=release

ENV HTTP_PORT=8080
ENV HTTPS_PORT=443

ARG SIGNAL_CLI_VERSION
ARG BUILD_VERSION_ARG

ENV BUILD_VERSION=$BUILD_VERSION_ARG

RUN dpkg-reconfigure debconf --frontend=noninteractive \
	&& apt-get -qq update \
	&& apt-get -qq install -y --no-install-recommends util-linux supervisor netcat < /dev/null > /dev/null \
	&& rm -rf /var/lib/apt/lists/* 

COPY --from=buildcontainer /tmp/signal-cli-rest-api-src/signal-cli-rest-api /usr/bin/signal-cli-rest-api
COPY --from=buildcontainer /opt/signal-cli/signal-cli-${SIGNAL_CLI_VERSION} /opt/signal-cli
COPY --from=buildcontainer /tmp/signal-cli-rest-api-src/scripts/jsonrpc2-helper /usr/bin/jsonrpc2-helper
COPY entrypoint.sh /entrypoint.sh


RUN groupadd -g 1000 signal-api \
	&& useradd --no-log-init -M -d /home -s /bin/bash -u 1000 -g 1000 signal-api \
	&& ln -s /opt/signal-cli/bin/signal-cli /usr/bin/signal-cli \
	&& mkdir -p /home/.local/share/signal-cli

# remove the temporary created signal-cli-native on armv7, as GRAALVM doesn't support 32bit
RUN arch="$(uname -m)"; \
	case "$arch" in \
	armv7l) echo "GRAALVM doesn't support 32bit" && rm /opt/signal-cli/bin/signal-cli-native /usr/bin/signal-cli-native  ;; \
	esac;

EXPOSE ${HTTP_PORT}
EXPOSE ${HTTPS_PORT}

ENV SIGNAL_CLI_CONFIG_DIR=/home/.local/share/signal-cli
ENV SIGNAL_CLI_UID=1000
ENV SIGNAL_CLI_GID=1000

ENTRYPOINT ["/entrypoint.sh"]

HEALTHCHECK --interval=20s --timeout=10s --retries=3 \
	CMD curl -f http://localhost:${PORT}/v1/health || exit 1
