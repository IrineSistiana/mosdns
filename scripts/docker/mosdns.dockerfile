FROM --platform=${TARGETPLATFORM} alpine:latest
LABEL maintainer="IrineSistiana"

WORKDIR /root
ARG TARGETPLATFORM
ARG TAG
ARG REPOSITORY
COPY install-mosdns.sh /root/install-mosdns.sh

RUN set -ex \
	&& chmod +x /root/install-mosdns.sh \
	&& ./install-mosdns.sh "${TARGETPLATFORM}" "${TAG}" "${REPOSITORY}"

VOLUME /etc/mosdns
EXPOSE 53/udp 53/tcp
CMD [ "/usr/bin/mosdns", "-dir", "/etc/mosdns" ]